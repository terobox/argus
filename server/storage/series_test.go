package storage

import (
	"context"
	"testing"

	"github.com/terobox/argus/pkg/metrics"
)

func TestQuerySeriesTierSelection(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	// Resolution is chosen purely by span, even with no data present.
	cases := []struct {
		span int64
		want string
	}{
		{100, ResRaw},
		{3600, ResRaw},   // exactly the raw window
		{3601, Res15s},   // just past raw
		{86400, Res15s},  // exactly the 15s window
		{86401, Res5m},   // just past 15s
		{1000000, Res5m}, // long history
	}
	for _, c := range cases {
		_, res, err := s.QuerySeries(ctx, "n1", 0, c.span)
		if err != nil {
			t.Fatalf("span %d: %v", c.span, err)
		}
		if res != c.want {
			t.Errorf("span %d: res = %q, want %q", c.span, res, c.want)
		}
	}
}

func TestQuerySeriesRates(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	const node = "n1"

	sm := func(ts int64, rx uint64) metrics.Sample {
		return metrics.Sample{
			TS:  ts,
			CPU: metrics.CPU{UsedPercent: float64(ts % 100)},
			Mem: metrics.Mem{TotalBytes: 16 << 30, AvailableBytes: 8 << 30, UsedBytes: 8 << 30},
			Net: metrics.Net{RxBytes: rx, TxBytes: rx * 2},
		}
	}
	if _, err := s.WriteSamples(ctx, node, []metrics.Sample{
		sm(100, 0), sm(105, 50), sm(110, 50),
	}); err != nil {
		t.Fatal(err)
	}

	pts, res, err := s.QuerySeries(ctx, node, 100, 110)
	if err != nil {
		t.Fatal(err)
	}
	if res != ResRaw {
		t.Fatalf("res = %q, want %q", res, ResRaw)
	}
	if len(pts) != 3 {
		t.Fatalf("got %d points, want 3", len(pts))
	}
	if pts[0].NetRxRate != 0 {
		t.Errorf("pt0 rx rate = %v, want 0", pts[0].NetRxRate)
	}
	// 50 bytes over 5s = 10/s.
	if pts[1].NetRxRate != 10 {
		t.Errorf("pt1 rx rate = %v, want 10", pts[1].NetRxRate)
	}
	if pts[2].NetRxRate != 0 {
		t.Errorf("pt2 rx rate = %v, want 0", pts[2].NetRxRate)
	}
}

func TestQuerySeriesCounterReset(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	const node = "n1"

	sm := func(ts int64, rx uint64) metrics.Sample {
		return metrics.Sample{
			TS:  ts,
			CPU: metrics.CPU{UsedPercent: 10},
			Net: metrics.Net{RxBytes: rx},
		}
	}
	// Counter goes backwards (reboot): the interval rate must be 0, not negative.
	if _, err := s.WriteSamples(ctx, node, []metrics.Sample{sm(100, 100), sm(110, 10)}); err != nil {
		t.Fatal(err)
	}
	pts, _, err := s.QuerySeries(ctx, node, 100, 110)
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) != 2 {
		t.Fatalf("got %d points, want 2", len(pts))
	}
	if pts[1].NetRxRate != 0 {
		t.Errorf("reset interval rx rate = %v, want 0", pts[1].NetRxRate)
	}
}

func TestQuerySeriesInvalidRange(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	if _, _, err := s.QuerySeries(ctx, "n1", 100, 100); err == nil {
		t.Fatal("expected error for empty range")
	}
}
