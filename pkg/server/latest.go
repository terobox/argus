package server

import (
	"sync"

	"github.com/terobox/argus/pkg/metrics"
)

// latestStore keeps the most recent sample — plus derived network rates — for
// each node in memory, so the overview can render live metrics without a DB
// round-trip on every status tick. It is lossy by design: a server restart drops
// it and it repopulates from the next ingest (agents report every ~15s).
type latestStore struct {
	mu sync.Mutex
	m  map[string]*nodeLatest
}

type nodeLatest struct {
	sample metrics.Sample
	rxRate float64
	txRate float64
}

func newLatestStore() *latestStore {
	return &latestStore{m: make(map[string]*nodeLatest)}
}

// put records s as the node's latest sample, recomputing the network rates
// against the previous sample's counters.
func (l *latestStore) put(nodeID string, s metrics.Sample) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cur := l.m[nodeID]
	if cur == nil {
		cur = &nodeLatest{}
		l.m[nodeID] = cur
	}
	if cur.sample.TS > 0 {
		if dt := float64(s.TS - cur.sample.TS); dt > 0 {
			cur.rxRate = netRate(cur.sample.Net.RxBytes, s.Net.RxBytes, dt)
			cur.txRate = netRate(cur.sample.Net.TxBytes, s.Net.TxBytes, dt)
		}
	}
	cur.sample = s
}

// get returns a snapshot of the node's latest metrics, or ok=false if the node
// has not reported since the server started.
func (l *latestStore) get(nodeID string) (nodeLatest, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cur, ok := l.m[nodeID]
	if !ok {
		return nodeLatest{}, false
	}
	return *cur, true
}

// netRate converts a cumulative counter to a per-second rate; a counter that
// decreased (e.g. a reboot reset) yields 0 rather than a huge negative spike.
func netRate(prev, cur uint64, dt float64) float64 {
	if cur < prev || dt <= 0 {
		return 0
	}
	return float64(cur-prev) / dt
}
