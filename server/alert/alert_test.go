package alert

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/terobox/argus/pkg/metrics"
	"github.com/terobox/argus/server/storage"
)

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "argus.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return store
}

func newTestEvaluator(t *testing.T, store *storage.Store, sender Sender, cfg Config) *Evaluator {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ev := NewEvaluator(store, sender, cfg, logger)
	t.Cleanup(ev.Close)
	return ev
}

// recordingSender records notifications for test assertions. The dispatch worker
// calls Notify from a single goroutine, so appends are serialized; the mutex
// makes reads after Close safe too.
type recordingSender struct {
	mu  sync.Mutex
	got []notification
}

func (r *recordingSender) Notify(_ context.Context, title, message string, priority int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, notification{title: title, message: message, priority: priority})
	return nil
}

func (r *recordingSender) notifications() []notification {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]notification(nil), r.got...)
}

// waitNotifications blocks until at least want notifications have been recorded,
// so assertions are independent of the async dispatch worker's scheduling.
func waitNotifications(t *testing.T, s *recordingSender, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(s.notifications()) >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d notifications, have %d", want, len(s.notifications()))
}

func TestDispatcherNotify(t *testing.T) {
	var method, title, priority, tags, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		title = r.Header.Get("Title")
		priority = r.Header.Get("Priority")
		tags = r.Header.Get("Tags")
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := NewDispatcher(srv.URL).Notify(context.Background(), "node offline", "it is down", PriorityHigh); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if method != http.MethodPost {
		t.Errorf("method = %q, want POST", method)
	}
	if title != "node offline" {
		t.Errorf("title = %q", title)
	}
	if priority != "4" {
		t.Errorf("priority = %q, want 4", priority)
	}
	if tags != "warning" {
		t.Errorf("tags = %q, want warning", tags)
	}
	if body != "it is down" {
		t.Errorf("body = %q", body)
	}
}

func TestDispatcherNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if err := NewDispatcher(srv.URL).Notify(context.Background(), "t", "m", PriorityHigh); err == nil {
		t.Fatal("expected error for non-2xx response")
	}
}

func TestValueExtractors(t *testing.T) {
	if v, ok := cpuValue(storage.SeriesPoint{CPU: 42.5}); !ok || v != 42.5 {
		t.Fatalf("cpuValue = %v, %v", v, ok)
	}
	if v, ok := memValue(storage.SeriesPoint{MemTotal: 100, MemUsed: 42}); !ok || v != 42 {
		t.Fatalf("memValue = %v, %v", v, ok)
	}
	if _, ok := memValue(storage.SeriesPoint{MemTotal: 0, MemUsed: 10}); ok {
		t.Fatal("memValue with zero total should be !ok")
	}
	total, used := int64(100), int64(90)
	if v, ok := diskValue(storage.SeriesPoint{DiskTotal: &total, DiskUsed: &used}); !ok || v != 90 {
		t.Fatalf("diskValue = %v, %v", v, ok)
	}
	if _, ok := diskValue(storage.SeriesPoint{DiskTotal: &total}); ok {
		t.Fatal("diskValue without used should be !ok")
	}
}

func TestOfflineTransitionAndRecovery(t *testing.T) {
	store := newTestStore(t)
	now := time.Unix(1_700_000_000, 0)
	if err := store.UpsertNode(context.Background(), storage.Node{
		NodeID: "n1", TokenHash: storage.HashToken("x"), FirstSeen: now.Unix() - 3600, LastSeen: now.Unix() - 120,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertNodeInfo(context.Background(), "n1", metrics.HostInfo{Hostname: "web-01"}, now.Unix()-3600); err != nil {
		t.Fatal(err)
	}

	sender := &recordingSender{}
	ev := newTestEvaluator(t, store, sender, Config{OfflineAfter: time.Minute, Cooldown: time.Hour})
	ev.now = func() time.Time { return now }

	// First pass: transition to offline fires once.
	ev.Evaluate(context.Background())
	waitNotifications(t, sender, 1)
	got := sender.notifications()
	if len(got) != 1 || got[0].title != "web-01: offline" || got[0].priority != PriorityHigh {
		t.Fatalf("offline alert = %+v", got)
	}

	// Second pass with no change must not duplicate.
	ev.Evaluate(context.Background())
	time.Sleep(20 * time.Millisecond) // give a stray duplicate a chance to land
	if n := len(sender.notifications()); n != 1 {
		t.Fatalf("after unchanged pass: %d notifications, want 1", n)
	}

	// Node reports again -> recovery.
	if err := store.TouchNode(context.Background(), "n1", now.Unix()); err != nil {
		t.Fatal(err)
	}
	ev.Evaluate(context.Background())
	waitNotifications(t, sender, 2)
	got = sender.notifications()
	if len(got) != 2 || got[1].title != "web-01: offline recovered" || got[1].priority != PriorityNormal {
		t.Fatalf("recovery = %+v", got)
	}
}

func TestThresholdAlertAndRecovery(t *testing.T) {
	store := newTestStore(t)
	now := time.Unix(1_700_000_000, 0)
	if err := store.UpsertNode(context.Background(), storage.Node{
		NodeID: "n1", TokenHash: storage.HashToken("x"), FirstSeen: now.Unix() - 3600, LastSeen: now.Unix(),
	}); err != nil {
		t.Fatal(err)
	}

	high := make([]metrics.Sample, 10)
	for i := range high {
		high[i] = metrics.Sample{TS: now.Unix() - int64(i), CPU: metrics.CPU{UsedPercent: 95}}
	}
	if _, err := store.WriteSamples(context.Background(), "n1", high); err != nil {
		t.Fatal(err)
	}

	sender := &recordingSender{}
	ev := newTestEvaluator(t, store, sender, Config{
		OfflineAfter: time.Minute, Sustain: 5 * time.Minute,
		CPUThreshold: 90, MemThreshold: 90, DiskThreshold: 90, Cooldown: time.Hour,
	})
	ev.now = func() time.Time { return now }

	ev.Evaluate(context.Background())
	waitNotifications(t, sender, 1)
	got := sender.notifications()
	if len(got) != 1 || got[0].title != "n1: high CPU" || got[0].priority != PriorityHigh {
		t.Fatalf("cpu alert = %+v", got)
	}

	// Replace with low CPU -> recovery.
	low := make([]metrics.Sample, 10)
	for i := range low {
		low[i] = metrics.Sample{TS: now.Unix() - int64(i), CPU: metrics.CPU{UsedPercent: 5}}
	}
	if _, err := store.WriteSamples(context.Background(), "n1", low); err != nil {
		t.Fatal(err)
	}
	ev.Evaluate(context.Background())
	waitNotifications(t, sender, 2)
	got = sender.notifications()
	if len(got) != 2 || got[1].title != "n1: CPU recovered" || got[1].priority != PriorityNormal {
		t.Fatalf("cpu recovery = %+v", got)
	}
}

func TestPersistenceAcrossRestart(t *testing.T) {
	store := newTestStore(t)
	now := time.Unix(1_700_000_000, 0)
	if err := store.UpsertNode(context.Background(), storage.Node{
		NodeID: "n1", TokenHash: storage.HashToken("x"), FirstSeen: now.Unix() - 3600, LastSeen: now.Unix() - 120,
	}); err != nil {
		t.Fatal(err)
	}

	first := &recordingSender{}
	ev1 := newTestEvaluator(t, store, first, Config{OfflineAfter: time.Minute, Cooldown: time.Hour})
	ev1.now = func() time.Time { return now }
	ev1.Evaluate(context.Background())
	waitNotifications(t, first, 1)

	// A new evaluator on the same store must load the persisted firing state and
	// not re-fire the same offline transition.
	second := &recordingSender{}
	ev2 := newTestEvaluator(t, store, second, Config{OfflineAfter: time.Minute, Cooldown: time.Hour})
	ev2.now = func() time.Time { return now }
	ev2.Evaluate(context.Background())
	time.Sleep(20 * time.Millisecond)
	if n := len(second.notifications()); n != 0 {
		t.Fatalf("restart re-fired: %d notifications, want 0", n)
	}
}

func TestCooldownRefire(t *testing.T) {
	store := newTestStore(t)
	base := time.Unix(1_700_000_000, 0)
	if err := store.UpsertNode(context.Background(), storage.Node{
		NodeID: "n1", TokenHash: storage.HashToken("x"), FirstSeen: base.Unix() - 3600, LastSeen: base.Unix() - 120,
	}); err != nil {
		t.Fatal(err)
	}

	sender := &recordingSender{}
	ev := newTestEvaluator(t, store, sender, Config{OfflineAfter: time.Minute, Cooldown: 30 * time.Second})
	current := base
	ev.now = func() time.Time { return current }

	ev.Evaluate(context.Background())
	waitNotifications(t, sender, 1)

	// Still firing, within cooldown: no re-fire.
	ev.Evaluate(context.Background())
	time.Sleep(20 * time.Millisecond)
	if n := len(sender.notifications()); n != 1 {
		t.Fatalf("within cooldown: %d notifications, want 1", n)
	}

	// Cooldown elapsed: re-fire.
	current = base.Add(31 * time.Second)
	ev.Evaluate(context.Background())
	waitNotifications(t, sender, 2)
	got := sender.notifications()
	if len(got) != 2 || got[1].priority != PriorityHigh {
		t.Fatalf("cooldown re-fire = %+v", got)
	}
}
