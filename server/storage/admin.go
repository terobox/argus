package storage

import (
	"context"
	"database/sql"
	"fmt"
)

// Admin is a dashboard user. PasswordHash is a bcrypt digest (see
// pkg (server/auth).HashPassword); the plaintext password is never stored.
type Admin struct {
	Username     string
	PasswordHash string
	CreatedAt    int64
}

// UpsertAdmin inserts or replaces an admin row. Bootstrap uses it to create the
// initial admin; a later password reset overwrites the same row.
func (s *Store) UpsertAdmin(ctx context.Context, a Admin) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO admins (username, password_hash, created_at) VALUES (?, ?, ?)`,
		a.Username, a.PasswordHash, a.CreatedAt)
	if err != nil {
		return fmt.Errorf("upsert admin: %w", err)
	}
	return nil
}

// GetAdmin returns the admin named username, or ok=false if absent.
func (s *Store) GetAdmin(ctx context.Context, username string) (Admin, bool, error) {
	var a Admin
	err := s.db.QueryRowContext(ctx,
		`SELECT username, password_hash, created_at FROM admins WHERE username = ?`, username).
		Scan(&a.Username, &a.PasswordHash, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return Admin{}, false, nil
	}
	if err != nil {
		return Admin{}, false, fmt.Errorf("get admin: %w", err)
	}
	return a, true, nil
}

// CountAdmins returns the number of admin rows.
func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admins`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count admins: %w", err)
	}
	return n, nil
}
