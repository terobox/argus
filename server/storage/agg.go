package storage

import (
	"context"
	"database/sql"
	"fmt"
)

// aggColumns lists the metric columns of the downsampled tables in the order
// used by every INSERT and SELECT. node_id is handled separately.
const aggColumns = "ts, cpu_avg, cpu_peak, load1, load5, load15, mem_total, mem_avail, mem_used, swap_total, swap_used, net_rx, net_tx, disk_r, disk_w, disk_total, disk_used, logged_users"

const upsert15s = `INSERT OR REPLACE INTO samples_15s (node_id, ` + aggColumns + `)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

const upsert5m = `INSERT OR REPLACE INTO samples_5m (node_id, ` + aggColumns + `)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

const select15s = `SELECT ` + aggColumns + ` FROM samples_15s WHERE node_id = ? AND ts BETWEEN ? AND ? ORDER BY ts`
const select5m = `SELECT ` + aggColumns + ` FROM samples_5m WHERE node_id = ? AND ts BETWEEN ? AND ? ORDER BY ts`

// writeAggRows upserts a batch of aggregated rows into the table named by
// upsertSQL, in a single transaction. Like WriteSamples, it is idempotent: rows
// are keyed by (node_id, ts) and re-running a window overwrites, never
// duplicates.
func (s *Store) writeAggRows(ctx context.Context, upsertSQL, nodeID string, rows []aggRow) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, upsertSQL)
	if err != nil {
		return 0, fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, r := range rows {
		if _, err := stmt.ExecContext(ctx, aggArgs(nodeID, r)...); err != nil {
			return 0, fmt.Errorf("insert agg ts=%d: %w", r.ts, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return len(rows), nil
}

// queryAgg reads aggregated rows for a node within [from, to], ordered by ts.
func (s *Store) queryAgg(ctx context.Context, selectSQL, nodeID string, from, to int64) ([]aggRow, error) {
	rows, err := s.db.QueryContext(ctx, selectSQL, nodeID, from, to)
	if err != nil {
		return nil, fmt.Errorf("query agg: %w", err)
	}
	defer rows.Close()

	out := make([]aggRow, 0, 64)
	for rows.Next() {
		r, err := scanAggRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan agg: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// aggArgs maps an aggRow onto the ordered column list in aggColumns.
func aggArgs(nodeID string, r aggRow) []any {
	var diskTotal, diskUsed, logged any
	if r.diskTotal != nil {
		diskTotal = *r.diskTotal
	}
	if r.diskUsed != nil {
		diskUsed = *r.diskUsed
	}
	if r.loggedUsers != nil {
		logged = *r.loggedUsers
	}
	return []any{
		nodeID, r.ts,
		r.cpuAvg, r.cpuPeak,
		r.load1, r.load5, r.load15,
		r.memTotal, r.memAvail, r.memUsed,
		r.swapTotal, r.swapUsed,
		int64(r.netRx), int64(r.netTx),
		int64(r.diskR), int64(r.diskW),
		diskTotal, diskUsed, logged,
	}
}

// scanAggRow reads one row in aggColumns order back into an aggRow. Counter
// columns are stored signed and cast back to uint64 (see scanSample).
func scanAggRow(sc scanner) (aggRow, error) {
	var r aggRow
	var netRx, netTx, diskR, diskW int64
	var diskTotal, diskUsed, logged sql.NullInt64

	err := sc.Scan(
		&r.ts, &r.cpuAvg, &r.cpuPeak,
		&r.load1, &r.load5, &r.load15,
		&r.memTotal, &r.memAvail, &r.memUsed,
		&r.swapTotal, &r.swapUsed,
		&netRx, &netTx, &diskR, &diskW,
		&diskTotal, &diskUsed, &logged,
	)
	if err != nil {
		return r, err
	}
	r.netRx = uint64(netRx)
	r.netTx = uint64(netTx)
	r.diskR = uint64(diskR)
	r.diskW = uint64(diskW)
	if diskTotal.Valid {
		r.diskTotal = &diskTotal.Int64
	}
	if diskUsed.Valid {
		r.diskUsed = &diskUsed.Int64
	}
	if logged.Valid {
		v := int(logged.Int64)
		r.loggedUsers = &v
	}
	return r, nil
}
