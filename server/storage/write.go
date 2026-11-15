package storage

import (
	"context"
	"fmt"

	"github.com/terobox/argus/pkg/metrics"
)

const upsertRaw = `INSERT OR REPLACE INTO samples_raw (node_id, ` + rawColumns + `)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

// WriteSamples upserts a batch of samples for one node in a single transaction.
// Rows are keyed by (node_id, ts): re-reporting an existing timestamp (e.g. an
// offline backfill) replaces the previous row, which is what makes ingestion
// idempotent and order-independent. The returned count is the number of rows
// written and is for observability, not correctness.
func (s *Store) WriteSamples(ctx context.Context, nodeID string, samples []metrics.Sample) (int64, error) {
	if len(samples) == 0 {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, upsertRaw)
	if err != nil {
		return 0, fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	var n int64
	for _, sm := range samples {
		res, err := stmt.ExecContext(ctx, insertArgs(nodeID, sm)...)
		if err != nil {
			return 0, fmt.Errorf("insert ts=%d: %w", sm.TS, err)
		}
		if affected, err := res.RowsAffected(); err == nil {
			n += affected
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return n, nil
}

// insertArgs maps a Sample onto the ordered column list in rawColumns.
// Slow/optional fields become NULL when absent, matching the nullable columns.
func insertArgs(nodeID string, s metrics.Sample) []any {
	var diskTotal, diskUsed, logged any
	if s.DiskCap != nil {
		diskTotal, diskUsed = s.DiskCap.TotalBytes, s.DiskCap.UsedBytes
	}
	if s.LoggedUsers != nil {
		logged = *s.LoggedUsers
	}
	return []any{
		nodeID, s.TS,
		s.CPU.UsedPercent,
		s.Load.Load1, s.Load.Load5, s.Load.Load15,
		s.Mem.TotalBytes, s.Mem.AvailableBytes, s.Mem.UsedBytes,
		s.Swap.TotalBytes, s.Swap.UsedBytes,
		int64(s.Net.RxBytes), int64(s.Net.TxBytes),
		int64(s.DiskIO.ReadBytes), int64(s.DiskIO.WriteBytes),
		diskTotal, diskUsed, logged,
	}
}
