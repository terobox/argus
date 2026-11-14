package storage

import (
	"context"
	"testing"

	"github.com/terobox/argus/pkg/metrics"
)

func TestDownsampleRecentExcludesFillingBucket(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	const node = "n"

	// now = 300 is a complete boundary for both the 15s and 5m buckets. Write
	// samples in the complete 15s bucket [285, 300) and one in the still-filling
	// bucket [300, 315).
	samples := []metrics.Sample{
		testSample(285), testSample(290), testSample(295),
		testSample(310),
	}
	if _, err := s.WriteSamples(ctx, node, samples); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := s.DownsampleRecent(ctx, node, 300); err != nil {
		t.Fatalf("DownsampleRecent: %v", err)
	}

	var n15 int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM samples_15s WHERE node_id = ?", node).Scan(&n15); err != nil {
		t.Fatalf("count 15s: %v", err)
	}
	if n15 != 1 {
		t.Fatalf("samples_15s rows = %d, want 1 (filling bucket 300 must be excluded)", n15)
	}
	var ts int64
	if err := s.db.QueryRowContext(ctx,
		"SELECT ts FROM samples_15s WHERE node_id = ?", node).Scan(&ts); err != nil {
		t.Fatalf("15s ts: %v", err)
	}
	if ts != 285 {
		t.Errorf("15s bucket ts = %d, want 285", ts)
	}

	var n5m int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM samples_5m WHERE node_id = ?", node).Scan(&n5m); err != nil {
		t.Fatalf("count 5m: %v", err)
	}
	if n5m != 1 {
		t.Errorf("samples_5m rows = %d, want 1 (both tiers must run)", n5m)
	}
}
