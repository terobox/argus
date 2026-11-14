package storage

import (
	"context"
	"testing"

	"github.com/terobox/argus/pkg/metrics"
)

func TestAggregateBucketsAndSemantics(t *testing.T) {
	// 14 points in bucket [0,15), 15 points in bucket [15,30).
	var pts []point
	for ts := int64(1); ts <= 14; ts++ {
		pts = append(pts, point{
			ts: ts, cpu: float64(ts), cpuPeak: float64(ts),
			load1: 1, load5: 2, load15: 3,
			memTotal: 1000, memAvail: 800, memUsed: 200,
			swapTotal: 512, swapUsed: 64,
			netRx: uint64(ts), netTx: uint64(ts * 2),
			diskR: 1, diskW: 2,
			hasLogged: true, loggedUsers: 1,
		})
	}
	for ts := int64(15); ts <= 29; ts++ {
		pts = append(pts, point{
			ts: ts, cpu: float64(ts), cpuPeak: float64(ts),
			load1: 1, load5: 2, load15: 3,
			memTotal: 1000, memAvail: 800, memUsed: 200,
			swapTotal: 512, swapUsed: 64,
			netRx: uint64(ts), netTx: uint64(ts * 2),
			diskR: 1, diskW: 2,
			hasLogged: true, loggedUsers: 2,
		})
	}

	rows := aggregate(pts, 15)
	if len(rows) != 2 {
		t.Fatalf("got %d buckets, want 2", len(rows))
	}

	// Bucket 0: cpu avg = avg(1..14) = 7.5, peak 14, netRx last = 14.
	if rows[0].ts != 0 {
		t.Errorf("bucket0 ts = %d, want 0", rows[0].ts)
	}
	if rows[0].cpuAvg != 7.5 {
		t.Errorf("bucket0 cpuAvg = %v, want 7.5", rows[0].cpuAvg)
	}
	if rows[0].cpuPeak != 14 {
		t.Errorf("bucket0 cpuPeak = %v, want 14", rows[0].cpuPeak)
	}
	if rows[0].netRx != 14 || rows[0].netTx != 28 {
		t.Errorf("bucket0 net = (%d, %d), want (14, 28)", rows[0].netRx, rows[0].netTx)
	}
	if rows[0].memTotal != 1000 || rows[0].memAvail != 800 {
		t.Errorf("bucket0 mem mismatch: total=%d avail=%d", rows[0].memTotal, rows[0].memAvail)
	}
	if rows[0].loggedUsers == nil || *rows[0].loggedUsers != 1 {
		t.Errorf("bucket0 loggedUsers = %v, want 1", rows[0].loggedUsers)
	}

	// Bucket 15: cpu avg = avg(15..29) = 22, peak 29, netRx last = 29.
	if rows[1].ts != 15 || rows[1].cpuAvg != 22 || rows[1].cpuPeak != 29 || rows[1].netRx != 29 {
		t.Errorf("bucket15 = %+v, want ts=15 avg=22 peak=29 netRx=29", rows[1])
	}
	if rows[1].loggedUsers == nil || *rows[1].loggedUsers != 2 {
		t.Errorf("bucket15 loggedUsers = %v, want 2", rows[1].loggedUsers)
	}
}

func TestDownsampleRawEndToEnd(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	const node = "node-1"

	var samples []metrics.Sample
	for ts := int64(0); ts < 45; ts++ {
		samples = append(samples, testSample(ts))
	}
	if _, err := s.WriteSamples(ctx, node, samples); err != nil {
		t.Fatalf("write: %v", err)
	}

	n, err := s.DownsampleRaw(ctx, node, 0, 45)
	if err != nil {
		t.Fatalf("downsample: %v", err)
	}
	if n != 3 {
		t.Fatalf("got %d buckets, want 3", n)
	}

	rows, err := s.queryAgg(ctx, select15s, node, 0, 44)
	if err != nil {
		t.Fatalf("query 15s: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	if rows[0].ts != 0 || rows[1].ts != 15 || rows[2].ts != 30 {
		t.Fatalf("bucket ts = %d,%d,%d, want 0,15,30", rows[0].ts, rows[1].ts, rows[2].ts)
	}
	// testSample sets cpu = ts%100, so bucket0 avg = avg(0..14) = 7, peak 14.
	if rows[0].cpuAvg != 7 || rows[0].cpuPeak != 14 {
		t.Errorf("bucket0 cpu = (avg %v, peak %v), want (7, 14)", rows[0].cpuAvg, rows[0].cpuPeak)
	}
	if rows[0].netRx != 14 {
		t.Errorf("bucket0 netRx = %d, want 14", rows[0].netRx)
	}
}

func TestDownsample15sEndToEnd(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	const node = "node-1"

	var rows []aggRow
	for i := int64(0); i < 20; i++ {
		rows = append(rows, aggRow{
			ts: i * 15, cpuAvg: float64(i), cpuPeak: float64(i),
			load1: 0.5, load5: 0.5, load15: 0.5,
			memTotal: 1000, memAvail: 500, memUsed: 500,
			swapTotal: 100, swapUsed: 10,
			netRx: uint64(i * 15), netTx: uint64(i * 15),
			diskR: 1, diskW: 2,
		})
	}
	if _, err := s.writeAggRows(ctx, upsert15s, node, rows); err != nil {
		t.Fatalf("write 15s: %v", err)
	}

	n, err := s.Downsample15s(ctx, node, 0, 300)
	if err != nil {
		t.Fatalf("downsample 15s: %v", err)
	}
	if n != 1 {
		t.Fatalf("got %d buckets, want 1", n)
	}

	got, err := s.queryAgg(ctx, select5m, node, 0, 299)
	if err != nil {
		t.Fatalf("query 5m: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
	if got[0].ts != 0 {
		t.Errorf("ts = %d, want 0", got[0].ts)
	}
	if got[0].cpuAvg != 9.5 || got[0].cpuPeak != 19 {
		t.Errorf("cpu = (avg %v, peak %v), want (9.5, 19)", got[0].cpuAvg, got[0].cpuPeak)
	}
	if got[0].netRx != 285 {
		t.Errorf("netRx = %d, want 285", got[0].netRx)
	}
}

func TestDownsampleIdempotent(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	const node = "node-1"

	var samples []metrics.Sample
	for ts := int64(0); ts < 45; ts++ {
		samples = append(samples, testSample(ts))
	}
	if _, err := s.WriteSamples(ctx, node, samples); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := s.DownsampleRaw(ctx, node, 0, 45); err != nil {
		t.Fatalf("downsample 1: %v", err)
	}
	if n, err := s.DownsampleRaw(ctx, node, 0, 45); err != nil || n != 3 {
		t.Fatalf("downsample 2: n=%d err=%v, want n=3 no error", n, err)
	}
	rows, err := s.queryAgg(ctx, select15s, node, 0, 44)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows after double downsample, want 3 (no duplicates)", len(rows))
	}
}
