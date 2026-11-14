// Package storage wraps the SQLite database behind a small, safe API: schema
// migration, idempotent batch writes, and range queries over the raw (1s)
// sample window.
package storage

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// Store is a handle to the SQLite database. It opens a single connection (see
// Open) so all access is serialized through database/sql's pool, which removes
// SQLITE_BUSY contention for this single-process server. WAL mode keeps writes
// durable and the journal small.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the SQLite database at path and verifies it
// is reachable. path may be a filesystem path or ":memory:" for tests.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// Single connection = single writer + serialized readers. At argus's scale
	// (hundreds of nodes, 15s batch writes, a few dashboard readers) this is the
	// simplest correct configuration and avoids SQLITE_BUSY entirely. If read
	// concurrency ever becomes a bottleneck, raise this alongside WAL; do not do
	// so before downsampling/retention jobs are understood to be write-safe.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// Migrate applies all pending schema migrations. It is idempotent.
func (s *Store) Migrate(ctx context.Context) error {
	for _, stmt := range migrations {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

// dsn builds a modernc.org/sqlite connection string that enables WAL, a
// busy timeout so transient lock contention retries instead of erroring, and
// synchronous=NORMAL for a good durability/latency tradeoff. Note: if path
// contains characters that are special in a DSN (space, '?', '&'), it must be
// percent-encoded by the caller; production paths like /var/lib/argus/argus.db
// are fine as-is.
func dsn(path string) string {
	return "file:" + path +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(ON)"
}
