package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/terobox/argus/pkg/metrics"
)

// nodeOfflineAfter marks a node offline when it has not reported for this long.
// Agents push every ~15s, so this tolerates several missed batches before the
// dashboard flips a node to offline.
const nodeOfflineAfter = 90 * time.Second

// statusInterval is how often a connected SSE client receives a fresh node list
// (which also serves as the keepalive that prevents idle proxies from timing
// out the connection).
const statusInterval = 5 * time.Second

// hub is a minimal in-process broadcast fan-out for SSE clients. Delivery is
// lossy for slow consumers: a client whose buffer is full drops the event
// rather than blocking the ingest path that publishes it.
type hub struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func newHub() *hub {
	return &hub{subs: make(map[chan []byte]struct{})}
}

func (h *hub) subscribe() chan []byte {
	ch := make(chan []byte, 64)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *hub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

func (h *hub) publish(msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- msg:
		default:
		}
	}
}

// sampleEvent is the payload of an SSE "sample" event, pushed when an agent
// reports.
type sampleEvent struct {
	NodeID string         `json:"node_id"`
	Sample metrics.Sample `json:"sample"`
}

// nodeMetrics is the live block of a status entry, sourced from the in-memory
// latest sample. It is omitted entirely when a node has not reported a sample.
type nodeMetrics struct {
	CPUPercent float64 `json:"cpu_percent"`
	MemUsed    int64   `json:"mem_used"`
	MemTotal   int64   `json:"mem_total"`
	DiskUsed   int64   `json:"disk_used"`
	DiskTotal  int64   `json:"disk_total"`
	NetRxRate  float64 `json:"net_rx_rate"`
	NetTxRate  float64 `json:"net_tx_rate"`
}

// nodeStatus is one node's entry in a "status" event.
type nodeStatus struct {
	NodeID    string       `json:"node_id"`
	Hostname  string       `json:"hostname"`
	FirstSeen int64        `json:"first_seen"`
	LastSeen  int64        `json:"last_seen"`
	Online    bool         `json:"online"`
	CPUCores  int          `json:"cpu_cores"`
	Tags      []string     `json:"tags"`
	Live      *nodeMetrics `json:"live,omitempty"`
}

// handleStream serves GET /api/v1/stream as a Server-Sent Events stream. It
// requires an admin session (the browser sends the cookie automatically on a
// same-origin EventSource). It emits "sample" events on ingest and a periodic
// "status" event with the node list and online state.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.hub.subscribe()
	defer s.hub.unsubscribe(ch)

	// Send an initial status so a fresh connection has a node list.
	s.writeStatus(w, flusher, r.Context())

	status := time.NewTicker(statusInterval)
	defer status.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			fmt.Fprintf(w, "event: sample\ndata: %s\n\n", msg)
			flusher.Flush()
		case <-status.C:
			s.writeStatus(w, flusher, r.Context())
		}
	}
}

// writeStatus sends the current node list with online state. Errors are
// non-fatal: the stream continues and the next tick retries.
func (s *Server) writeStatus(w http.ResponseWriter, flusher http.Flusher, ctx context.Context) {
	nodes, err := s.store.ListNodeSummaries(ctx)
	if err != nil {
		s.logger.Warn("sse status", "err", err)
		return
	}
	now := time.Now().Unix()
	out := make([]nodeStatus, 0, len(nodes))
	for _, n := range nodes {
		st := nodeStatus{
			NodeID:    n.NodeID,
			Hostname:  n.Hostname,
			FirstSeen: n.FirstSeen,
			LastSeen:  n.LastSeen,
			Online:    now-n.LastSeen <= int64(nodeOfflineAfter/time.Second),
			CPUCores:  n.CPUCores,
			Tags:      n.Tags,
		}
		if lv, ok := s.latest.get(n.NodeID); ok {
			m := &nodeMetrics{
				CPUPercent: lv.sample.CPU.UsedPercent,
				MemUsed:    lv.sample.Mem.UsedBytes,
				MemTotal:   lv.sample.Mem.TotalBytes,
				NetRxRate:  lv.rxRate,
				NetTxRate:  lv.txRate,
			}
			if lv.sample.DiskCap != nil {
				m.DiskUsed = lv.sample.DiskCap.UsedBytes
				m.DiskTotal = lv.sample.DiskCap.TotalBytes
			}
			st.Live = m
		}
		out = append(out, st)
	}
	payload, err := json.Marshal(out)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: status\ndata: %s\n\n", payload)
	flusher.Flush()
}
