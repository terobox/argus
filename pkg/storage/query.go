package storage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/terobox/argus/pkg/metrics"
)

const selectRaw = `SELECT ` + rawColumns + ` FROM samples_raw WHERE node_id = ? AND ts BETWEEN ? AND ? ORDER BY ts`

// QueryRaw returns the raw 1s samples for a node within [from, to], ordered by
// timestamp ascending. It is the fine-grained resolution used for the hot
// window (~1h).
func (s *Store) QueryRaw(ctx context.Context, nodeID string, from, to int64) ([]metrics.Sample, error) {
	rows, err := s.db.QueryContext(ctx, selectRaw, nodeID, from, to)
	if err != nil {
		return nil, fmt.Errorf("query raw: %w", err)
	}
	defer rows.Close()

	out := make([]metrics.Sample, 0, 64)
	for rows.Next() {
		sm, err := scanSample(rows)
		if err != nil {
			return nil, fmt.Errorf("scan raw: %w", err)
		}
		out = append(out, sm)
	}
	return out, rows.Err()
}

// QueryLatest returns the most recent raw sample for nodeID, or ok=false if the
// node has no samples yet. It lets the detail view render current metrics
// immediately, before the next live ingest arrives.
func (s *Store) QueryLatest(ctx context.Context, nodeID string) (metrics.Sample, bool, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+rawColumns+` FROM samples_raw WHERE node_id = ? ORDER BY ts DESC LIMIT 1`,
		nodeID)
	sm, err := scanSample(row)
	if err == sql.ErrNoRows {
		return metrics.Sample{}, false, nil
	}
	if err != nil {
		return metrics.Sample{}, false, fmt.Errorf("query latest: %w", err)
	}
	return sm, true, nil
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface{ Scan(dest ...any) error }

// scanSample reads one row in rawColumns order back into a Sample. Counter
// columns are stored signed; they are cast back to uint64 on the assumption
// (guaranteed at ingest time) that they are non-negative and far below 2^63.
func scanSample(sc scanner) (metrics.Sample, error) {
	var s metrics.Sample
	var netRx, netTx, diskR, diskW int64
	var diskTotal, diskUsed, logged sql.NullInt64

	err := sc.Scan(
		&s.TS, &s.CPU.UsedPercent,
		&s.Load.Load1, &s.Load.Load5, &s.Load.Load15,
		&s.Mem.TotalBytes, &s.Mem.AvailableBytes, &s.Mem.UsedBytes,
		&s.Swap.TotalBytes, &s.Swap.UsedBytes,
		&netRx, &netTx, &diskR, &diskW,
		&diskTotal, &diskUsed, &logged,
	)
	if err != nil {
		return s, err
	}
	s.Net.RxBytes = uint64(netRx)
	s.Net.TxBytes = uint64(netTx)
	s.DiskIO.ReadBytes = uint64(diskR)
	s.DiskIO.WriteBytes = uint64(diskW)
	if diskTotal.Valid && diskUsed.Valid {
		s.DiskCap = &metrics.DiskCapacity{TotalBytes: diskTotal.Int64, UsedBytes: diskUsed.Int64}
	}
	if logged.Valid {
		v := int(logged.Int64)
		s.LoggedUsers = &v
	}
	return s, nil
}
