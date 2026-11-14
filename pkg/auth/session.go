package auth

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/terobox/argus/pkg/storage"
)

// DefaultSessionTTL is how long a login session remains valid when the config
// does not override it.
const DefaultSessionTTL = 7 * 24 * time.Hour

// SessionManager creates, validates, and revokes login sessions. Sessions are
// persisted to the database (so they survive a restart) but validated against an
// in-memory cache first, so the steady-state request path never touches the
// database. A cache miss (e.g. after a restart) falls back to the database and
// repopulates the cache.
type SessionManager struct {
	store  *storage.Store
	ttl    time.Duration
	logger *slog.Logger

	mu    sync.Mutex
	cache map[string]cachedSession
}

type cachedSession struct {
	username  string
	csrf      string
	expiresAt int64
}

// NewSessionManager returns a SessionManager with ttl (defaulted if <= 0).
func NewSessionManager(store *storage.Store, ttl time.Duration, logger *slog.Logger) *SessionManager {
	if ttl <= 0 {
		ttl = DefaultSessionTTL
	}
	return &SessionManager{
		store:  store,
		ttl:    ttl,
		logger: logger,
		cache:  make(map[string]cachedSession),
	}
}

// Create persists a new session for username and returns the plaintext cookie
// token and the CSRF token. Only the SHA-256 digest of the cookie token is
// stored.
func (m *SessionManager) Create(ctx context.Context, username string) (token, csrf string, err error) {
	token, err = RandomToken(32)
	if err != nil {
		return "", "", err
	}
	csrf, err = RandomToken(32)
	if err != nil {
		return "", "", err
	}
	now := time.Now()
	sess := storage.Session{
		TokenHash: storage.HashToken(token),
		Username:  username,
		CSRF:      csrf,
		CreatedAt: now.Unix(),
		ExpiresAt: now.Add(m.ttl).Unix(),
	}
	if err := m.store.InsertSession(ctx, sess); err != nil {
		return "", "", err
	}
	m.mu.Lock()
	m.cache[sess.TokenHash] = cachedSession{username: username, csrf: csrf, expiresAt: sess.ExpiresAt}
	m.mu.Unlock()
	return token, csrf, nil
}

// Validate returns the session's username and CSRF token if token is valid and
// unexpired. It consults the cache first and falls back to the database on a
// miss (e.g. after a restart), repopulating the cache on a hit.
func (m *SessionManager) Validate(ctx context.Context, token string) (username, csrf string, ok bool) {
	hash := storage.HashToken(token)
	now := time.Now().Unix()

	m.mu.Lock()
	if c, found := m.cache[hash]; found {
		if c.expiresAt > now {
			m.mu.Unlock()
			return c.username, c.csrf, true
		}
		delete(m.cache, hash)
	}
	m.mu.Unlock()

	sess, found, err := m.store.GetSession(ctx, hash)
	if err != nil {
		m.logger.Error("get session", "err", err)
		return "", "", false
	}
	if !found || sess.ExpiresAt <= now {
		if found {
			_ = m.store.DeleteSession(ctx, hash)
		}
		return "", "", false
	}
	m.mu.Lock()
	m.cache[hash] = cachedSession{username: sess.Username, csrf: sess.CSRF, expiresAt: sess.ExpiresAt}
	m.mu.Unlock()
	return sess.Username, sess.CSRF, true
}

// Revoke deletes a session immediately (logout or revocation).
func (m *SessionManager) Revoke(ctx context.Context, token string) {
	hash := storage.HashToken(token)
	m.mu.Lock()
	delete(m.cache, hash)
	m.mu.Unlock()
	if err := m.store.DeleteSession(ctx, hash); err != nil {
		m.logger.Warn("delete session", "err", err)
	}
}

// Sweep removes expired sessions from the cache and database.
func (m *SessionManager) Sweep(ctx context.Context) {
	now := time.Now().Unix()
	m.mu.Lock()
	for h, c := range m.cache {
		if c.expiresAt <= now {
			delete(m.cache, h)
		}
	}
	m.mu.Unlock()
	if _, err := m.store.DeleteExpiredSessions(ctx, now); err != nil {
		m.logger.Warn("sweep sessions", "err", err)
	}
}
