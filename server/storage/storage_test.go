package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/terobox/argus/pkg/metrics"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "argus.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s
}

func intPtr(v int) *int { return &v }

func testSample(ts int64) metrics.Sample {
	return metrics.Sample{
		TS:          ts,
		CPU:         metrics.CPU{UsedPercent: float64(ts % 100)},
		Load:        metrics.Load{Load1: 0.1, Load5: 0.2, Load15: 0.3},
		Mem:         metrics.Mem{TotalBytes: 16 << 30, AvailableBytes: 8 << 30, UsedBytes: 8 << 30},
		Swap:        metrics.Swap{TotalBytes: 4 << 30, UsedBytes: 1 << 30},
		Net:         metrics.Net{RxBytes: uint64(ts), TxBytes: uint64(ts) * 2},
		DiskIO:      metrics.DiskIO{ReadBytes: 100, WriteBytes: 200},
		DiskCap:     &metrics.DiskCapacity{TotalBytes: 100 << 30, UsedBytes: 40 << 30},
		LoggedUsers: intPtr(2),
	}
}

func TestWriteAndQueryRoundtrip(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	const node = "node-1"

	samples := []metrics.Sample{testSample(100), testSample(101), testSample(102)}
	if _, err := s.WriteSamples(ctx, node, samples); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := s.QueryRaw(ctx, node, 100, 102)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d samples, want 3", len(got))
	}
	for i := range got {
		if got[i].TS != samples[i].TS {
			t.Errorf("ts[%d] = %d, want %d", i, got[i].TS, samples[i].TS)
		}
		if got[i].CPU.UsedPercent != samples[i].CPU.UsedPercent {
			t.Errorf("cpu[%d] = %v, want %v", i, got[i].CPU.UsedPercent, samples[i].CPU.UsedPercent)
		}
		if got[i].Net.RxBytes != samples[i].Net.RxBytes {
			t.Errorf("net_rx[%d] = %d, want %d", i, got[i].Net.RxBytes, samples[i].Net.RxBytes)
		}
		if got[i].DiskCap == nil || got[i].DiskCap.TotalBytes != samples[i].DiskCap.TotalBytes {
			t.Errorf("disk_cap[%d] mismatch", i)
		}
		if got[i].LoggedUsers == nil || *got[i].LoggedUsers != *samples[i].LoggedUsers {
			t.Errorf("logged_users[%d] mismatch", i)
		}
	}
}

func TestWriteIsIdempotentUpsert(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	const node = "node-1"

	if _, err := s.WriteSamples(ctx, node, []metrics.Sample{testSample(100)}); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Re-write the same timestamp with different values; should replace, not duplicate.
	updated := testSample(100)
	updated.CPU.UsedPercent = 99
	if _, err := s.WriteSamples(ctx, node, []metrics.Sample{updated}); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	got, err := s.QueryRaw(ctx, node, 100, 100)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1 (upsert must not duplicate)", len(got))
	}
	if got[0].CPU.UsedPercent != 99 {
		t.Errorf("cpu = %v, want 99", got[0].CPU.UsedPercent)
	}
}

func TestQueryRawEmptyAndRange(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	const node = "node-1"

	if got, err := s.QueryRaw(ctx, node, 0, 1000); err != nil {
		t.Fatalf("query empty: %v", err)
	} else if len(got) != 0 {
		t.Fatalf("got %d samples, want 0", len(got))
	}

	if _, err := s.WriteSamples(ctx, node, []metrics.Sample{testSample(100), testSample(200)}); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := s.QueryRaw(ctx, node, 150, 250)
	if err != nil {
		t.Fatalf("query range: %v", err)
	}
	if len(got) != 1 || got[0].TS != 200 {
		t.Fatalf("got %v, want only ts=200", got)
	}
}

func TestWALEnabled(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	var mode string
	if err := s.db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}
}

func TestNilSlowFieldsRoundtrip(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	const node = "node-1"

	sm := testSample(100)
	sm.DiskCap = nil
	sm.LoggedUsers = nil
	if _, err := s.WriteSamples(ctx, node, []metrics.Sample{sm}); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := s.QueryRaw(ctx, node, 100, 100)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
	if got[0].DiskCap != nil {
		t.Errorf("DiskCap = %v, want nil", got[0].DiskCap)
	}
	if got[0].LoggedUsers != nil {
		t.Errorf("LoggedUsers = %v, want nil", got[0].LoggedUsers)
	}
}
