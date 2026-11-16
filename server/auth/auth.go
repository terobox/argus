package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/terobox/argus/server/storage"
)

// loginRateLimit is the login endpoint's throttle: per IP+username, this many
// attempts per loginRateWindow.
const (
	loginRateLimit    = 5
	loginRateWindow   = time.Minute
	maxLoginBodyBytes = 4 << 10 // 4 KiB — far more than a username+password needs
)

// Admin credential limits. bcrypt refuses inputs longer than 72 bytes, so check
// up front and return a clear error instead of a hashing failure.
const (
	maxUsernameLen   = 64
	minPasswordBytes = 8
	maxPasswordBytes = 72
)

func validateUsername(username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("username is required")
	}
	if len(username) > maxUsernameLen {
		return fmt.Errorf("username must be at most %d characters", maxUsernameLen)
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < minPasswordBytes {
		return fmt.Errorf("password must be at least %d characters", minPasswordBytes)
	}
	if len(password) > maxPasswordBytes {
		return fmt.Errorf("password must be at most %d bytes", maxPasswordBytes)
	}
	return nil
}

// Auth bundles login/logout handling, session management, CSRF, and the
// middleware applied to protected routes.
type Auth struct {
	store     *storage.Store
	sessions  *SessionManager
	limiter   *RateLimiter
	logger    *slog.Logger
	secure    bool
	ttl       time.Duration
	dummyHash string
}

// New returns an Auth bound to store. secure marks the session cookie Secure
// (use true only when serving over HTTPS). ttl is the session lifetime.
func New(store *storage.Store, logger *slog.Logger, secure bool, ttl time.Duration) *Auth {
	if ttl <= 0 {
		ttl = DefaultSessionTTL
	}
	// A dummy hash lets login compare against bcrypt even for unknown users, so
	// response timing does not reveal which usernames exist.
	dummyHash, _ := HashPassword("argus-dummy-password-for-timing")
	return &Auth{
		store:     store,
		sessions:  NewSessionManager(store, ttl, logger),
		limiter:   NewRateLimiter(loginRateLimit, loginRateWindow),
		logger:    logger,
		secure:    secure,
		ttl:       ttl,
		dummyHash: dummyHash,
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// HandleLogin verifies credentials (rate-limited per IP+username) and issues a
// session cookie. The CSRF token is returned in the body for the client to send
// back via X-CSRF-Token on mutating requests.
func (a *Auth) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLoginBodyBytes)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Username = strings.TrimSpace(req.Username)

	key := clientIP(r) + "|" + req.Username
	if !a.limiter.Allow(key) {
		httpError(w, http.StatusTooManyRequests, "too many attempts; try again later")
		return
	}

	admin, found, err := a.store.GetAdmin(r.Context(), req.Username)
	if err != nil {
		a.logger.Error("get admin", "err", err)
		httpError(w, http.StatusInternalServerError, "internal error")
		return
	}

	hash := a.dummyHash
	if found {
		hash = admin.PasswordHash
	}
	if !CheckPassword(hash, req.Password) {
		httpError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, csrf, err := a.sessions.Create(r.Context(), admin.Username)
	if err != nil {
		a.logger.Error("create session", "err", err)
		httpError(w, http.StatusInternalServerError, "internal error")
		return
	}

	a.setSessionCookie(w, token, int(a.ttl.Seconds()))
	writeJSON(w, http.StatusOK, map[string]string{"csrf_token": csrf})
}

// HandleLogout revokes the session (if any) and clears the cookie. It is
// intentionally not CSRF-gated: logging out is not a state change that benefits
// an attacker, and it must still clear the cookie when the session has expired.
func (a *Auth) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(CookieName); err == nil && cookie.Value != "" {
		a.sessions.Revoke(r.Context(), cookie.Value)
	}
	a.setSessionCookie(w, "", -1)
	w.WriteHeader(http.StatusNoContent)
}

// HandleMe returns the authenticated username and the session's CSRF token. It
// must be wrapped by RequireAdmin, which places both values in the context. The
// client restores the token from here on a page reload, so it never loses the
// ability to make mutating requests without a fresh login.
func (a *Auth) HandleMe(w http.ResponseWriter, r *http.Request) {
	csrf, _ := r.Context().Value(ctxCSRF).(string)
	writeJSON(w, http.StatusOK, map[string]string{
		"username":   Admin(r.Context()),
		"csrf_token": csrf,
	})
}

// HandleSetupStatus reports whether the first-run setup is still available,
// i.e. whether the database has no admin yet. It is public because it must work
// before any account exists, and it deliberately reveals nothing else.
func (a *Auth) HandleSetupStatus(w http.ResponseWriter, r *http.Request) {
	n, err := a.store.CountAdmins(r.Context())
	if err != nil {
		a.logger.Error("count admins", "err", err)
		httpError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]bool{"setup_required": n == 0})
}

// HandleSetup creates the first admin and signs it in, so the setup wizard can
// drop the operator straight into the dashboard. It is a no-op returning 409
// once any admin exists: CreateInitialAdmin decides that atomically, which also
// makes two concurrent first-setup requests safe.
func (a *Auth) HandleSetup(w http.ResponseWriter, r *http.Request) {
	if !a.limiter.Allow("setup|" + clientIP(r)) {
		httpError(w, http.StatusTooManyRequests, "too many attempts; try again later")
		return
	}

	n, err := a.store.CountAdmins(r.Context())
	if err != nil {
		a.logger.Error("count admins", "err", err)
		httpError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if n > 0 {
		httpError(w, http.StatusConflict, "setup already completed")
		return
	}

	var req loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLoginBodyBytes)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if err := validateUsername(req.Username); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validatePassword(req.Password); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		a.logger.Error("hash setup password", "err", err)
		httpError(w, http.StatusInternalServerError, "internal error")
		return
	}
	created, err := a.store.CreateInitialAdmin(r.Context(), storage.Admin{
		Username:     req.Username,
		PasswordHash: hash,
		CreatedAt:    time.Now().Unix(),
	})
	if err != nil {
		a.logger.Error("create initial admin", "err", err)
		httpError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !created {
		httpError(w, http.StatusConflict, "setup already completed")
		return
	}

	token, csrf, err := a.sessions.Create(r.Context(), req.Username)
	if err != nil {
		// The admin now exists, so the client can simply fall back to login.
		a.logger.Error("create setup session", "err", err)
		httpError(w, http.StatusInternalServerError, "internal error")
		return
	}
	a.setSessionCookie(w, token, int(a.ttl.Seconds()))
	writeJSON(w, http.StatusOK, map[string]string{
		"username":   req.Username,
		"csrf_token": csrf,
	})
}

// Sweep removes expired sessions and stale rate-limit keys. Call periodically.
func (a *Auth) Sweep(ctx context.Context) {
	a.sessions.Sweep(ctx)
	a.limiter.Sweep()
}

func (a *Auth) setSessionCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   a.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// Bootstrap creates the initial admin if none exists, using the given password.
// It is a no-op when password is empty (no admin is configured) or an admin
// already exists (so a restart never silently resets the password). It reports
// whether an admin was created. This is the headless alternative to the web
// setup wizard, driven by ARGUS_ADMIN_USERNAME/ARGUS_ADMIN_PASSWORD.
func Bootstrap(ctx context.Context, store *storage.Store, username, password string) (bool, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return false, nil
	}
	if err := validateUsername(username); err != nil {
		return false, fmt.Errorf("bootstrap admin: %w", err)
	}
	if err := validatePassword(password); err != nil {
		return false, fmt.Errorf("bootstrap admin: %w", err)
	}
	// Skip the expensive hash when an admin already exists; CreateInitialAdmin
	// below is still the atomic authority on "first admin".
	n, err := store.CountAdmins(ctx)
	if err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}
	hash, err := HashPassword(password)
	if err != nil {
		return false, err
	}
	return store.CreateInitialAdmin(ctx, storage.Admin{
		Username:     username,
		PasswordHash: hash,
		CreatedAt:    time.Now().Unix(),
	})
}

func clientIP(r *http.Request) string {
	// Use the direct peer address; X-Forwarded-For is spoofable and only
	// trustworthy behind a reverse proxy this server does not assume.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, code int, msg string) {
	http.Error(w, msg, code)
}
