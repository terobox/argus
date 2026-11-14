package storage

import (
	"context"
	"fmt"
	"time"
)

// Retention windows: the fine-grained table keeps only the hot recent window;
// coarser tables keep progressively longer histories. These mirror the
// downsampling bucket sizes (15s and 5m).
const (
	retainRaw = 1 * time.Hour
	retain15s = 24 * time.Hour
	retain5m  = 30 * 24 * time.Hour
)

// Retention deletes rows older than each table's window, based on now (unix
// seconds). It must run only after downsampling has covered the same range,
// otherwise data that has not yet been rolled up would be discarded. The caller
// (the server's background job) is responsible for that ordering.
func (s *Store) Retention(ctx context.Context, now int64) error {
	dels := []struct {
		stmt   string
		cutoff int64
	}{
		{`DELETE FROM samples_raw WHERE ts < ?`, now - int64(retainRaw/time.Second)},
		{`DELETE FROM samples_15s WHERE ts < ?`, now - int64(retain15s/time.Second)},
		{`DELETE FROM samples_5m WHERE ts < ?`, now - int64(retain5m/time.Second)},
	}
	for _, d := range dels {
		if _, err := s.db.ExecContext(ctx, d.stmt, d.cutoff); err != nil {
			return fmt.Errorf("retention: %w", err)
		}
	}
	return nil
}
