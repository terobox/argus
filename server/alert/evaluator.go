package alert

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/terobox/argus/server/storage"
)

// rule identifiers, also used as the alert_state key and in messages.
const (
	ruleOffline = "offline"
	ruleCPU     = "cpu"
	ruleMem     = "mem"
	ruleDisk    = "disk"
)

// minPoints is the fewest samples a resource rule needs in its sustain window
// before it may fire, guarding against a barely-populated window (e.g. an agent
// that just started) producing a false "sustained overload".
const minPoints = 3

// notification is one message queued for the dispatch worker.
type notification struct {
	title    string
	message  string
	priority int
}

// Evaluator evaluates alert rules against stored node state and dispatches
// notifications on transitions (and, while a rule stays firing, at most once per
// cooldown). The server runs Evaluate from its single jobs goroutine; the
// dispatch worker runs separately so a slow webhook cannot stall ingest,
// downsampling, or retention.
type Evaluator struct {
	store  *storage.Store
	sender Sender
	cfg    Config
	logger *slog.Logger

	mu     sync.Mutex
	state  map[string]storage.AlertState // key nodeID + "\x00" + rule
	loaded bool

	// now is injectable for deterministic tests; it defaults to time.Now.
	now func() time.Time

	notifyCh  chan notification
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// NewEvaluator returns an Evaluator and starts its dispatch worker. sender must
// be non-nil.
func NewEvaluator(store *storage.Store, sender Sender, cfg Config, logger *slog.Logger) *Evaluator {
	e := &Evaluator{
		store:    store,
		sender:   sender,
		cfg:      cfg,
		logger:   logger,
		state:    make(map[string]storage.AlertState),
		now:      time.Now,
		notifyCh: make(chan notification, 256),
	}
	e.wg.Add(1)
	go e.dispatchLoop()
	return e
}

// Close flushes queued notifications and stops the dispatch worker. It is safe
// to call more than once, and must not be called concurrently with Evaluate.
func (e *Evaluator) Close() {
	e.closeOnce.Do(func() {
		close(e.notifyCh)
		e.wg.Wait()
	})
}

// Evaluate runs one evaluation pass over every registered node. It loads the
// persisted alert state once on first use, then reuses it in memory, writing
// through to the database on every transition.
func (e *Evaluator) Evaluate(ctx context.Context) {
	e.mu.Lock()
	if !e.loaded {
		states, err := e.store.LoadAlertStates(ctx)
		if err != nil {
			e.logger.Warn("alert: load state", "err", err)
		} else {
			for _, st := range states {
				e.state[stateKey(st.NodeID, st.Rule)] = st
			}
		}
		e.loaded = true
	}
	e.mu.Unlock()

	nodes, err := e.store.ListNodeSummaries(ctx)
	if err != nil {
		e.logger.Error("alert: list nodes", "err", err)
		return
	}

	now := e.now().Unix()
	offlineAfter := int64(e.cfg.OfflineAfter / time.Second)
	if offlineAfter < 1 {
		offlineAfter = 1
	}

	for _, n := range nodes {
		// A node is offline once it has not reported for OfflineAfter. Only
		// online nodes are evaluated for resource thresholds, so an offline node
		// produces a single "offline" alert rather than a pile of stale
		// "high CPU" alerts.
		offline := now-n.LastSeen > offlineAfter
		e.evalOffline(ctx, n, offline, now)
		if !offline {
			e.evalThreshold(ctx, n, now, ruleCPU, e.cfg.CPUThreshold, cpuValue)
			e.evalThreshold(ctx, n, now, ruleMem, e.cfg.MemThreshold, memValue)
			e.evalThreshold(ctx, n, now, ruleDisk, e.cfg.DiskThreshold, diskValue)
		}
	}
}

// evalOffline evaluates the offline rule.
func (e *Evaluator) evalOffline(ctx context.Context, n storage.NodeSummary, offline bool, now int64) {
	host := hostOrID(n)
	e.apply(ctx, n, ruleOffline, offline, now,
		fmt.Sprintf("%s: offline", host),
		fmt.Sprintf("%s (node %s) has not reported for %s.", host, n.NodeID,
			humanDuration(time.Duration(now-n.LastSeen)*time.Second)))
}

// valueFn extracts a rule's percentage from a series point, reporting ok=false
// when the point lacks the field (e.g. disk capacity is only sampled every ~30s).
type valueFn func(storage.SeriesPoint) (float64, bool)

func cpuValue(p storage.SeriesPoint) (float64, bool) { return p.CPU, true }

func memValue(p storage.SeriesPoint) (float64, bool) {
	if p.MemTotal <= 0 {
		return 0, false
	}
	return float64(p.MemUsed) / float64(p.MemTotal) * 100, true
}

func diskValue(p storage.SeriesPoint) (float64, bool) {
	if p.DiskTotal == nil || p.DiskUsed == nil || *p.DiskTotal <= 0 {
		return 0, false
	}
	return float64(*p.DiskUsed) / float64(*p.DiskTotal) * 100, true
}

// evalThreshold evaluates a resource rule against the average value over the
// sustain window. Averaging (rather than checking the instantaneous value) is
// what makes "sustained > threshold" true: a lone spike cannot drag a window
// average across the line.
func (e *Evaluator) evalThreshold(ctx context.Context, n storage.NodeSummary, now int64, rule string, threshold float64, val valueFn) {
	if threshold <= 0 {
		return
	}
	sustainSecs := int64(e.cfg.Sustain / time.Second)
	if sustainSecs < 1 {
		sustainSecs = 1
	}
	points, _, err := e.store.QuerySeries(ctx, n.NodeID, now-sustainSecs, now)
	if err != nil {
		e.logger.Warn("alert: query series", "node_id", n.NodeID, "rule", rule, "err", err)
		return
	}

	var sum float64
	var count int
	for _, p := range points {
		if v, ok := val(p); ok {
			sum += v
			count++
		}
	}
	if count < minPoints {
		return
	}

	avg := sum / float64(count)
	host := hostOrID(n)
	label := ruleLabel(rule)
	e.apply(ctx, n, rule, avg > threshold, now,
		fmt.Sprintf("%s: high %s", host, label),
		fmt.Sprintf("%s (node %s): %s averaged %.1f%% over the last %s (threshold %.0f%%).",
			host, n.NodeID, label, avg, e.cfg.Sustain, threshold))
}

// apply transitions a rule's firing state and, on a transition (or a cooldown
// re-fire), persists it and queues a notification.
func (e *Evaluator) apply(ctx context.Context, n storage.NodeSummary, rule string, firing bool, now int64, alertTitle, alertMsg string) {
	key := stateKey(n.NodeID, rule)

	e.mu.Lock()
	st := e.state[key]
	var kind string // "alert", "recover", or "" (no change)
	if firing {
		if !st.Firing {
			st.Firing = true
			st.LastNotified = now
			kind = "alert"
		} else if now-st.LastNotified >= int64(e.cfg.Cooldown/time.Second) {
			st.LastNotified = now
			kind = "alert"
		}
	} else if st.Firing {
		st.Firing = false
		st.LastNotified = now
		kind = "recover"
	}
	e.state[key] = st
	e.mu.Unlock()

	if kind == "" {
		return
	}

	// Persist before notifying so a restart never re-fires a transition the
	// previous run already reported.
	if err := e.store.SaveAlertState(ctx, storage.AlertState{
		NodeID: n.NodeID, Rule: rule, Firing: st.Firing, LastNotified: st.LastNotified,
	}); err != nil {
		e.logger.Warn("alert: persist state", "node_id", n.NodeID, "rule", rule, "err", err)
	}

	host := hostOrID(n)
	var title, msg string
	var priority int
	if kind == "alert" {
		title, msg, priority = alertTitle, alertMsg, PriorityHigh
	} else {
		title = fmt.Sprintf("%s: %s recovered", host, ruleLabel(rule))
		msg = fmt.Sprintf("%s (node %s) is back to normal.", host, n.NodeID)
		priority = PriorityNormal
	}
	e.enqueue(notification{title: title, message: msg, priority: priority})
}

// enqueue queues a notification without blocking evaluation; if the queue is
// full the message is dropped rather than stalling the jobs loop. A dropped
// alert's state is already persisted, so it re-fires after the cooldown.
func (e *Evaluator) enqueue(n notification) {
	select {
	case e.notifyCh <- n:
	default:
		e.logger.Warn("alert: notification queue full, dropping", "title", n.title)
	}
}

// dispatchLoop sends queued notifications on a single worker goroutine, keeping
// slow webhook calls off the jobs loop.
func (e *Evaluator) dispatchLoop() {
	defer e.wg.Done()
	for n := range e.notifyCh {
		if err := e.sender.Notify(context.Background(), n.title, n.message, n.priority); err != nil {
			e.logger.Error("alert: notify", "title", n.title, "err", err)
		}
	}
}

func stateKey(nodeID, rule string) string { return nodeID + "\x00" + rule }

func hostOrID(n storage.NodeSummary) string {
	if n.Hostname != "" {
		return n.Hostname
	}
	return n.NodeID
}

func ruleLabel(rule string) string {
	switch rule {
	case ruleCPU:
		return "CPU"
	case ruleMem:
		return "memory"
	case ruleDisk:
		return "disk"
	case ruleOffline:
		return "offline"
	}
	return rule
}

func humanDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
