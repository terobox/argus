package storage

import (
	"context"
	"math"

	"github.com/terobox/argus/pkg/metrics"
)

// Bucket sizes for the downsampling tiers, in seconds. They are wall-clock
// aligned: a bucket's key is floor(ts/bucket)*bucket.
const (
	bucket15s int64 = 15
	bucket5m  int64 = 300
)

// point is the common, resolution-independent shape fed into aggregate(). Both
// raw samples and already-aggregated rows are projected onto it, which lets one
// aggregator serve both tiers.
type point struct {
	ts                   int64
	cpu, cpuPeak         float64
	load1, load5, load15 float64
	memTotal, memAvail   int64
	memUsed              int64
	swapTotal, swapUsed  int64
	netRx, netTx         uint64
	diskR, diskW         uint64
	diskTotal, diskUsed  int64
	hasDisk              bool
	loggedUsers          int
	hasLogged            bool
}

// aggRow mirrors one row in samples_15s / samples_5m.
type aggRow struct {
	ts                   int64
	cpuAvg, cpuPeak      float64
	load1, load5, load15 float64
	memTotal, memAvail   int64
	memUsed              int64
	swapTotal, swapUsed  int64
	netRx, netTx         uint64
	diskR, diskW         uint64
	diskTotal, diskUsed  *int64
	loggedUsers          *int
}

func sampleToPoint(s metrics.Sample) point {
	p := point{
		ts:        s.TS,
		cpu:       s.CPU.UsedPercent,
		cpuPeak:   s.CPU.UsedPercent,
		load1:     s.Load.Load1,
		load5:     s.Load.Load5,
		load15:    s.Load.Load15,
		memTotal:  s.Mem.TotalBytes,
		memAvail:  s.Mem.AvailableBytes,
		memUsed:   s.Mem.UsedBytes,
		swapTotal: s.Swap.TotalBytes,
		swapUsed:  s.Swap.UsedBytes,
		netRx:     s.Net.RxBytes,
		netTx:     s.Net.TxBytes,
		diskR:     s.DiskIO.ReadBytes,
		diskW:     s.DiskIO.WriteBytes,
	}
	if s.DiskCap != nil {
		p.diskTotal, p.diskUsed, p.hasDisk = s.DiskCap.TotalBytes, s.DiskCap.UsedBytes, true
	}
	if s.LoggedUsers != nil {
		p.loggedUsers, p.hasLogged = *s.LoggedUsers, true
	}
	return p
}

func aggRowToPoint(r aggRow) point {
	p := point{
		ts:        r.ts,
		cpu:       r.cpuAvg,
		cpuPeak:   r.cpuPeak,
		load1:     r.load1,
		load5:     r.load5,
		load15:    r.load15,
		memTotal:  r.memTotal,
		memAvail:  r.memAvail,
		memUsed:   r.memUsed,
		swapTotal: r.swapTotal,
		swapUsed:  r.swapUsed,
		netRx:     r.netRx,
		netTx:     r.netTx,
		diskR:     r.diskR,
		diskW:     r.diskW,
	}
	if r.diskTotal != nil && r.diskUsed != nil {
		p.diskTotal, p.diskUsed, p.hasDisk = *r.diskTotal, *r.diskUsed, true
	}
	if r.loggedUsers != nil {
		p.loggedUsers, p.hasLogged = *r.loggedUsers, true
	}
	return p
}

// DownsampleRaw aggregates samples_raw into samples_15s for nodeID over the
// half-open window [from, to). from and to must be multiples of bucket15s so
// only complete buckets are summarized; the caller is responsible for excluding
// the still-filling bucket. Re-running a window is idempotent.
func (s *Store) DownsampleRaw(ctx context.Context, nodeID string, from, to int64) (int, error) {
	samples, err := s.QueryRaw(ctx, nodeID, from, to-1)
	if err != nil {
		return 0, err
	}
	pts := make([]point, len(samples))
	for i := range samples {
		pts[i] = sampleToPoint(samples[i])
	}
	return s.writeAggRows(ctx, upsert15s, nodeID, aggregate(pts, bucket15s))
}

// Downsample15s aggregates samples_15s into samples_5m for nodeID over the
// half-open window [from, to). Same alignment and idempotency contract as
// DownsampleRaw, with bucket5m boundaries.
func (s *Store) Downsample15s(ctx context.Context, nodeID string, from, to int64) (int, error) {
	rows, err := s.queryAgg(ctx, select15s, nodeID, from, to-1)
	if err != nil {
		return 0, err
	}
	pts := make([]point, len(rows))
	for i := range rows {
		pts[i] = aggRowToPoint(rows[i])
	}
	return s.writeAggRows(ctx, upsert5m, nodeID, aggregate(pts, bucket5m))
}

// aggregate groups points into wall-clock-aligned buckets of bucketSec seconds
// and reduces each bucket to a single aggRow using per-metric semantics:
//
//   - gauges (cpu, load, mem.used/avail, swap.used): average
//   - cpu additionally: peak (max over the bucket)
//   - cumulative counters (net, disk io): last value in the bucket; rates are
//     derived at read time from consecutive buckets, not stored here
//   - capacities (mem.total, swap.total, disk) and logged_users: last value
//
// pts must be sorted by ts ascending, which the storage queries guarantee.
func aggregate(pts []point, bucketSec int64) []aggRow {
	out := make([]aggRow, 0)
	var acc *bucketAcc
	for _, p := range pts {
		b := p.ts / bucketSec * bucketSec
		if acc == nil || acc.start != b {
			if acc != nil {
				out = append(out, acc.finish())
			}
			acc = &bucketAcc{start: b}
		}
		acc.add(p)
	}
	if acc != nil {
		out = append(out, acc.finish())
	}
	return out
}

type bucketAcc struct {
	start                   int64
	n                       int
	cpuSum, cpuPeak         float64
	load1Sum, load5Sum      float64
	load15Sum               float64
	memAvailSum, memUsedSum int64
	swapUsedSum             int64
	memTotal, swapTotal     int64
	netRx, netTx            uint64
	diskR, diskW            uint64
	diskTotal, diskUsed     int64
	hasDisk                 bool
	loggedUsers             int
	hasLogged               bool
}

func (a *bucketAcc) add(p point) {
	a.n++
	a.cpuSum += p.cpu
	if p.cpuPeak > a.cpuPeak {
		a.cpuPeak = p.cpuPeak
	}
	a.load1Sum += p.load1
	a.load5Sum += p.load5
	a.load15Sum += p.load15
	a.memAvailSum += p.memAvail
	a.memUsedSum += p.memUsed
	a.swapUsedSum += p.swapUsed
	// Last-value fields: pts is sorted, so each assignment ends on the final
	// point in the bucket.
	a.memTotal = p.memTotal
	a.swapTotal = p.swapTotal
	a.netRx = p.netRx
	a.netTx = p.netTx
	a.diskR = p.diskR
	a.diskW = p.diskW
	if p.hasDisk {
		a.diskTotal, a.diskUsed, a.hasDisk = p.diskTotal, p.diskUsed, true
	}
	if p.hasLogged {
		a.loggedUsers, a.hasLogged = p.loggedUsers, true
	}
}

func (a *bucketAcc) finish() aggRow {
	n := float64(a.n)
	r := aggRow{
		ts:        a.start,
		cpuAvg:    a.cpuSum / n,
		cpuPeak:   a.cpuPeak,
		load1:     a.load1Sum / n,
		load5:     a.load5Sum / n,
		load15:    a.load15Sum / n,
		memTotal:  a.memTotal,
		memAvail:  int64(math.Round(float64(a.memAvailSum) / n)),
		memUsed:   int64(math.Round(float64(a.memUsedSum) / n)),
		swapTotal: a.swapTotal,
		swapUsed:  int64(math.Round(float64(a.swapUsedSum) / n)),
		netRx:     a.netRx,
		netTx:     a.netTx,
		diskR:     a.diskR,
		diskW:     a.diskW,
	}
	if a.hasDisk {
		r.diskTotal = &a.diskTotal
		r.diskUsed = &a.diskUsed
	}
	if a.hasLogged {
		v := a.loggedUsers
		r.loggedUsers = &v
	}
	return r
}
