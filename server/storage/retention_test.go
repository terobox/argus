package storage

import (
	"context"
	"testing"

	"github.com/terobox/argus/pkg/metrics"
)

func TestRetention(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	const node = "node-1"
	now := int64(1_700_000_000)

	// Raw: one old (> 1h), one recent.
	if _, err := s.WriteSamples(ctx, node, []metrics.Sample{
		testSample(now - 2*3600),
		testSample(now - 10),
	}); err != nil {
		t.Fatalf("write raw: %v", err)
	}

	// 15s: one old (> 24h), one recent.
	if _, err := s.writeAggRows(ctx, upsert15s, node, []aggRow{
		{ts: now - 48*3600, cpuAvg: 1},
		{ts: now - 100, cpuAvg: 2},
	}); err != nil {
		t.Fatalf("write 15s: %v", err)
	}

	// 5m: one old (> 30d), one recent.
	if _, err := s.writeAggRows(ctx, upsert5m, node, []aggRow{
		{ts: now - 60*24*3600, cpuAvg: 1},
		{ts: now - 1000, cpuAvg: 2},
	}); err != nil {
		t.Fatalf("write 5m: %v", err)
	}

	if err := s.Retention(ctx, now); err != nil {
		t.Fatalf("retention: %v", err)
	}

	if rows, err := s.QueryRaw(ctx, node, 0, now); err != nil {
		t.Fatalf("query raw: %v", err)
	} else if len(rows) != 1 || rows[0].TS != now-10 {
		t.Fatalf("raw retained %d rows, want 1 (recent)", len(rows))
	}

	if rows, err := s.queryAgg(ctx, select15s, node, 0, now); err != nil {
		t.Fatalf("query 15s: %v", err)
	} else if len(rows) != 1 || rows[0].ts != now-100 {
		t.Fatalf("15s retained %d rows, want 1 (recent)", len(rows))
	}

	if rows, err := s.queryAgg(ctx, select5m, node, 0, now); err != nil {
		t.Fatalf("query 5m: %v", err)
	} else if len(rows) != 1 || rows[0].ts != now-1000 {
		t.Fatalf("5m retained %d rows, want 1 (recent)", len(rows))
	}
}
