package storage

import (
	"context"
	"fmt"
	"time"
)

// Series resolution labels, returned so the caller knows which tier a series
// came from (and therefore its bucket size).
const (
	ResRaw = "1s"
	Res15s = "15s"
	Res5m  = "5m"
)

// SeriesPoint is a dashboard-facing metric point, independent of which tier it
// came from. Net and disk counters are converted to per-second rates between
// consecutive points, which is what a chart shows; the first point's rates are
// zero. cpu is the bucket average (equal to the instantaneous value for the raw
// tier); cpu_peak is the bucket maximum (equal to cpu for the raw tier).
type SeriesPoint struct {
	TS            int64   `json:"ts"`
	CPU           float64 `json:"cpu"`
	CPUPeak       float64 `json:"cpu_peak"`
	Load1         float64 `json:"load1"`
	Load5         float64 `json:"load5"`
	Load15        float64 `json:"load15"`
	MemTotal      int64   `json:"mem_total"`
	MemAvail      int64   `json:"mem_avail"`
	MemUsed       int64   `json:"mem_used"`
	SwapTotal     int64   `json:"swap_total"`
	SwapUsed      int64   `json:"swap_used"`
	NetRxRate     float64 `json:"net_rx_rate"`
	NetTxRate     float64 `json:"net_tx_rate"`
	DiskReadRate  float64 `json:"disk_read_rate"`
	DiskWriteRate float64 `json:"disk_write_rate"`
	DiskTotal     *int64  `json:"disk_total,omitempty"`
	DiskUsed      *int64  `json:"disk_used,omitempty"`
	LoggedUsers   *int    `json:"logged_users,omitempty"`
}

// QuerySeries returns a resolution-appropriate series for nodeID over [from, to].
// The tier is chosen from the span — <=1h raw, <=24h 15s, else 5m — mirroring
// the retention windows so the chosen tier actually still holds the requested
// range.
func (s *Store) QuerySeries(ctx context.Context, nodeID string, from, to int64) ([]SeriesPoint, string, error) {
	if to <= from {
		return nil, "", fmt.Errorf("invalid range: to %d <= from %d", to, from)
	}
	span := to - from

	var pts []point
	var res string
	switch {
	case span <= int64(retainRaw/time.Second):
		samples, err := s.QueryRaw(ctx, nodeID, from, to)
		if err != nil {
			return nil, "", err
		}
		pts = make([]point, len(samples))
		for i := range samples {
			pts[i] = sampleToPoint(samples[i])
		}
		res = ResRaw
	case span <= int64(retain15s/time.Second):
		rows, err := s.queryAgg(ctx, select15s, nodeID, from, to)
		if err != nil {
			return nil, "", err
		}
		pts = make([]point, len(rows))
		for i := range rows {
			pts[i] = aggRowToPoint(rows[i])
		}
		res = Res15s
	default:
		rows, err := s.queryAgg(ctx, select5m, nodeID, from, to)
		if err != nil {
			return nil, "", err
		}
		pts = make([]point, len(rows))
		for i := range rows {
			pts[i] = aggRowToPoint(rows[i])
		}
		res = Res5m
	}
	return deriveRates(pts), res, nil
}

// deriveRates converts cumulative counters to per-second rates between
// consecutive points. A counter that decreases (a reboot reset) yields a zero
// rate for that interval rather than a huge negative spike.
func deriveRates(pts []point) []SeriesPoint {
	out := make([]SeriesPoint, len(pts))
	for i, p := range pts {
		sp := SeriesPoint{
			TS:        p.ts,
			CPU:       p.cpu,
			CPUPeak:   p.cpuPeak,
			Load1:     p.load1,
			Load5:     p.load5,
			Load15:    p.load15,
			MemTotal:  p.memTotal,
			MemAvail:  p.memAvail,
			MemUsed:   p.memUsed,
			SwapTotal: p.swapTotal,
			SwapUsed:  p.swapUsed,
		}
		if p.hasDisk {
			sp.DiskTotal = &p.diskTotal
			sp.DiskUsed = &p.diskUsed
		}
		if p.hasLogged {
			v := p.loggedUsers
			sp.LoggedUsers = &v
		}
		if i > 0 {
			prev := pts[i-1]
			if dt := float64(p.ts - prev.ts); dt > 0 {
				sp.NetRxRate = counterRate(prev.netRx, p.netRx, dt)
				sp.NetTxRate = counterRate(prev.netTx, p.netTx, dt)
				sp.DiskReadRate = counterRate(prev.diskR, p.diskR, dt)
				sp.DiskWriteRate = counterRate(prev.diskW, p.diskW, dt)
			}
		}
		out[i] = sp
	}
	return out
}

func counterRate(prev, cur uint64, dt float64) float64 {
	if cur < prev {
		return 0
	}
	return float64(cur-prev) / dt
}
