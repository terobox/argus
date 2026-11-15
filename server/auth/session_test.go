package auth

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/terobox/argus/server/storage"
)

func openStore(t *testing.T) *storage.Store {
	t.Helper()
	s, err := storage.Open(filepath.Join(t.TempDir(), "argus.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

func newManager(t *testing.T, s *storage.Store, ttl time.Duration) *SessionManager {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewSessionManager(s, ttl, logger)
}

func TestSessionCreateValidateRevoke(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	m := newManager(t, s, time.Hour)

	token, csrf, err := m.Create(ctx, "admin")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if token == "" || csrf == "" {
		t.Fatal("empty token or csrf")
	}

	user, gotCSRF, ok := m.Validate(ctx, token)
	if !ok {
		t.Fatal("validate failed after create")
	}
	if user != "admin" || gotCSRF != csrf {
		t.Fatalf("got user=%q csrf=%q, want admin/%q", user, gotCSRF, csrf)
	}

	m.Revoke(ctx, token)
	if _, _, ok := m.Validate(ctx, token); ok {
		t.Fatal("validate succeeded after revoke")
	}
}

func TestSessionDBFallbackAfterRestart(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)

	m1 := newManager(t, s, time.Hour)
	token, csrf, err := m1.Create(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}

	// A fresh manager shares the store but has an empty cache, simulating a
	// restart. Validation must fall back to the database.
	m2 := newManager(t, s, time.Hour)
	user, gotCSRF, ok := m2.Validate(ctx, token)
	if !ok {
		t.Fatal("validate failed after simulated restart")
	}
	if user != "admin" || gotCSRF != csrf {
		t.Fatalf("got user=%q csrf=%q", user, gotCSRF)
	}
}

func TestSessionExpired(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)

	// Insert a session that is already expired; the cache is empty so Validate
	// exercises the database path.
	expired := storage.Session{
		TokenHash: storage.HashToken("deadbeef"),
		Username:  "admin",
		CSRF:      "csrf",
		CreatedAt: 1,
		ExpiresAt: time.Now().Add(-time.Minute).Unix(),
	}
	if err := s.InsertSession(ctx, expired); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := newManager(t, s, time.Hour).Validate(ctx, "deadbeef"); ok {
		t.Fatal("expired session validated")
	}
}

func TestSessionCacheExpiry(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	m := newManager(t, s, 20*time.Millisecond)

	token, _, err := m.Create(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if _, _, ok := m.Validate(ctx, token); ok {
		t.Fatal("cache-expired session validated")
	}
}
