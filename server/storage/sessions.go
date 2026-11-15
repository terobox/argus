package storage

import (
	"context"
	"database/sql"
	"fmt"
)

// Session is a persisted login session. TokenHash is the SHA-256 hex digest of
// the session cookie value (see HashToken); the cookie value itself is never
// stored. CSRF is a random token returned to the client and required on
// mutating requests.
type Session struct {
	TokenHash string
	Username  string
	CSRF      string
	CreatedAt int64
	ExpiresAt int64
}

// InsertSession persists a new session.
func (s *Store) InsertSession(ctx context.Context, sess Session) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, username, csrf_token, created_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
		sess.TokenHash, sess.Username, sess.CSRF, sess.CreatedAt, sess.ExpiresAt)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// GetSession returns the session with tokenHash, or ok=false if absent.
func (s *Store) GetSession(ctx context.Context, tokenHash string) (Session, bool, error) {
	var sess Session
	err := s.db.QueryRowContext(ctx,
		`SELECT token_hash, username, csrf_token, created_at, expires_at FROM sessions WHERE token_hash = ?`, tokenHash).
		Scan(&sess.TokenHash, &sess.Username, &sess.CSRF, &sess.CreatedAt, &sess.ExpiresAt)
	if err == sql.ErrNoRows {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, fmt.Errorf("get session: %w", err)
	}
	return sess, true, nil
}

// DeleteSession removes a session (logout or revocation).
func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteExpiredSessions removes sessions whose expires_at has passed, returning
// the number removed. It backs the periodic session sweep.
func (s *Store) DeleteExpiredSessions(ctx context.Context, now int64) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions rows: %w", err)
	}
	return n, nil
}
