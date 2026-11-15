package auth

import (
	"context"
	"crypto/subtle"
	"net/http"
)

// CookieName is the session cookie set on login and cleared on logout.
const CookieName = "argus_session"

// CSRFHeader is the request header carrying the CSRF token on mutating requests.
const CSRFHeader = "X-CSRF-Token"

type ctxKey int

const (
	ctxAdmin ctxKey = iota
	ctxCSRF
)

// Admin returns the authenticated admin username from the request context, or
// "" when the request is not authenticated.
func Admin(ctx context.Context) string {
	v, _ := ctx.Value(ctxAdmin).(string)
	return v
}

// RequireAdmin authenticates the request via the session cookie and attaches the
// admin username and session CSRF token to the context. Unauthenticated or
// expired sessions receive 401.
func (a *Auth) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(CookieName)
		if err != nil || cookie.Value == "" {
			httpError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		username, csrf, ok := a.sessions.Validate(r.Context(), cookie.Value)
		if !ok {
			httpError(w, http.StatusUnauthorized, "invalid or expired session")
			return
		}
		ctx := context.WithValue(r.Context(), ctxAdmin, username)
		ctx = context.WithValue(ctx, ctxCSRF, csrf)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireCSRF rejects mutating requests (anything but GET/HEAD/OPTIONS) whose
// X-CSRF-Token header does not match the session's CSRF token. It must wrap a
// handler already wrapped by RequireAdmin, which places the token in the
// context.
func (a *Auth) RequireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			want, _ := r.Context().Value(ctxCSRF).(string)
			got := r.Header.Get(CSRFHeader)
			if want == "" || !safeCSRF(want, got) {
				httpError(w, http.StatusForbidden, "invalid CSRF token")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// safeCSRF compares two CSRF tokens in constant time. A length mismatch is a
// mismatch (ConstantTimeCompare already returns 0 then); the explicit branch
// keeps the intent clear.
func safeCSRF(want, got string) bool {
	if len(want) != len(got) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}
