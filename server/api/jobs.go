package api

import (
	"context"
	"time"
)

// retentionEveryDownsample runs retention once per this many downsample ticks,
// which at the default 15s job interval is ~5 minutes. Retention's per-table
// DELETE is keyed only on ts (not node_id), so it scans each table; running it
// infrequently amortizes that cost instead of competing with ingest writes.
const retentionEveryDownsample = 20

// RunJobs runs the periodic background jobs — per-node downsampling, auth
// cleanup (expired sessions, stale rate-limit keys), and, less often, retention
// — until ctx is cancelled. It is not safe to call more than once concurrently
// on the same Server.
func (s *Server) RunJobs(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 15 * time.Second
	}

	// Roll up anything already written and clean anything already stale before
	// the first tick.
	s.downsample(ctx)
	s.retention(ctx)
	s.sweepAuth(ctx)
	s.sweepAlerts(ctx)

	t := time.NewTicker(interval)
	defer t.Stop()
	ticks := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.downsample(ctx)
			s.sweepAuth(ctx)
			s.sweepAlerts(ctx)
			ticks++
			if ticks%retentionEveryDownsample == 0 {
				s.retention(ctx)
			}
		}
	}
}

// sweepAlerts evaluates alert rules (offline + thresholds). It is guarded for a
// nil evaluator so RunJobs remains safe to call on a Server constructed without
// alerting enabled.
func (s *Server) sweepAlerts(ctx context.Context) {
	if s.alerts != nil {
		s.alerts.Evaluate(ctx)
	}
}

// sweepAuth clears expired sessions and stale rate-limit keys. It is guarded for
// a nil auth so RunJobs remains safe to call on a Server constructed without
// one.
func (s *Server) sweepAuth(ctx context.Context) {
	if s.auth != nil {
		s.auth.Sweep(ctx)
	}
}

// downsample rolls up the recent window for every registered node. The recent
// window is idempotent and bounded, so re-running it each tick is cheap and
// covers late/backfilled points without a watermark.
func (s *Server) downsample(ctx context.Context) {
	now := time.Now().Unix()
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		s.logger.Error("list nodes", "err", err)
		return
	}
	for _, n := range nodes {
		if err := s.store.DownsampleRecent(ctx, n.NodeID, now); err != nil {
			s.logger.Error("downsample", "node_id", n.NodeID, "err", err)
		}
	}
}

// retention deletes rows older than each tier's window. It must run only after
// downsampling has covered the same range (see storage.Retention); both run on
// the same single connection, and downsample always runs first.
func (s *Server) retention(ctx context.Context) {
	if err := s.store.Retention(ctx, time.Now().Unix()); err != nil {
		s.logger.Error("retention", "err", err)
	}
}
