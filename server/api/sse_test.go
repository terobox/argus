package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/terobox/argus/pkg/metrics"
	"github.com/terobox/argus/pkg/protocol"
	"github.com/terobox/argus/server/auth"
	"github.com/terobox/argus/server/storage"
)

func TestHubFanout(t *testing.T) {
	h := newHub()
	a := h.subscribe()
	b := h.subscribe()

	h.publish([]byte("hello"))
	if got := string(<-a); got != "hello" {
		t.Fatalf("a got %q", got)
	}
	if got := string(<-b); got != "hello" {
		t.Fatalf("b got %q", got)
	}

	h.unsubscribe(a)
	h.publish([]byte("world"))
	if got := string(<-b); got != "world" {
		t.Fatalf("b got %q after unsubscribing a", got)
	}
	select {
	case msg := <-a:
		t.Fatalf("a received %q after unsubscribe", msg)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestSSEStream(t *testing.T) {
	srv, store := newTestServer(t)
	seedAdmin(t, store, "admin", "secret")
	if err := store.UpsertNode(context.Background(), storage.Node{
		NodeID: "n1", TokenHash: storage.HashToken("tok"), FirstSeen: 1, LastSeen: 1,
	}); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	cookie, _ := login(t, srv.Handler(), "admin", "secret")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+protocol.StreamPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)

	// Initial status event carries the node list.
	data, err := readSSE(reader, "status")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(data, "n1") {
		t.Fatalf("status data missing node: %q", data)
	}

	// Trigger an ingest to publish a sample event.
	rep := protocol.Report{
		Version: protocol.Version, NodeID: "n1", CollectedAt: 100,
		Samples: []metrics.Sample{testSample(100)},
	}
	if rr := postReport(t, srv.Handler(), rep, "tok", false); rr.Code != http.StatusNoContent {
		t.Fatalf("ingest = %d", rr.Code)
	}

	data, err = readSSE(reader, "sample")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(data, "n1") {
		t.Fatalf("sample data missing node: %q", data)
	}
}

// readSSE reads SSE event blocks until it finds one named wantEvent, returning
// the concatenated data payload.
func readSSE(r *bufio.Reader, wantEvent string) (string, error) {
	var event string
	var data []string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if event == wantEvent {
				return strings.Join(data, "\n"), nil
			}
			event, data = "", nil
			continue
		}
		switch {
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
}
