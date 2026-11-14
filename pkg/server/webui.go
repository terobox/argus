package server

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// newUIHandler serves the embedded frontend with SPA fallback. Existing files
// (index.html, assets/*) are served directly; any other path — a client-side
// route such as /nodes/xyz — returns index.html so the router can take over.
// API paths are never shadowed here: they are matched by more specific mux
// patterns before "/" is reached, and this handler 404s /api as a safety net.
func newUIHandler(dist fs.FS) http.Handler {
	if dist == nil {
		return http.NotFoundHandler()
	}
	fileServer := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		if p := path.Clean(r.URL.Path); len(p) >= 4 && p[:4] == "/api" {
			http.NotFound(w, r)
			return
		}

		// Security headers for all UI responses.
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; img-src 'self' data: blob:; font-src 'self' data:")

		name := r.URL.Path
		if name == "/" {
			name = "/index.html"
		}
		if f, err := dist.Open(strings.TrimPrefix(name, "/")); err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: rewrite to the root (serving index.html via the directory
		// index, avoiding FileServer's /index.html -> ./ redirect) without
		// mutating the original URL.
		r2 := new(http.Request)
		*r2 = *r
		u := *r.URL
		u.Path = "/"
		r2.URL = &u
		fileServer.ServeHTTP(w, r2)
	})
}
