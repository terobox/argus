package storage

import (
	"context"
	"time"
)

// Lookback windows for DownsampleRecent. recentRawLookback must be at least as
// large as the agent's offline buffer (10 minutes) so a backfilled batch is
// still rolled up when the next job tick runs after the samples arrive.
const (
	recentRawLookback = 15 * time.Minute
	recent15sLookback = 1 * time.Hour
)

// DownsampleRecent rolls up both tiers for nodeID using data up to the last
// complete bucket before now (unix seconds). It re-runs a bounded lookback
// window — idempotent — so late-arriving or backfilled points are covered
// without tracking a per-node watermark. Call it periodically for every node
// and then run Retention.
//
// Note: a bucket that is only partially covered by source data (e.g. the first
// bucket after a gap) is aggregated over whatever points it has rather than
// being held back until complete. That keeps the model simple; the only cost is
// a slightly noisy first bucket after any outage.
func (s *Store) DownsampleRecent(ctx context.Context, nodeID string, now int64) error {
	to := now / bucket15s * bucket15s
	from := to - int64(recentRawLookback/time.Second)
	if _, err := s.DownsampleRaw(ctx, nodeID, from, to); err != nil {
		return err
	}

	to = now / bucket5m * bucket5m
	from = to - int64(recent15sLookback/time.Second)
	if _, err := s.Downsample15s(ctx, nodeID, from, to); err != nil {
		return err
	}
	return nil
}
