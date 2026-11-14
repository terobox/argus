package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestUIHandlerServesIndexAndAssets(t *testing.T) {
	dist := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<html>app</html>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log(1)")},
	}
	h := newUIHandler(dist)

	// Root serves index.html.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK || rr.Body.String() != "<html>app</html>" {
		t.Fatalf("root = %d %q", rr.Code, rr.Body.String())
	}

	// Asset served directly.
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if rr.Code != http.StatusOK || rr.Body.String() != "console.log(1)" {
		t.Fatalf("asset = %d %q", rr.Code, rr.Body.String())
	}

	// Client-side route falls back to index.html.
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/nodes/xyz", nil))
	if rr.Code != http.StatusOK || rr.Body.String() != "<html>app</html>" {
		t.Fatalf("spa fallback = %d %q", rr.Code, rr.Body.String())
	}

	// API paths are 404, not index.html.
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/nope", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("api = %d, want 404", rr.Code)
	}
}

func TestUIHandlerNilDist(t *testing.T) {
	h := newUIHandler(nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("nil dist = %d, want 404", rr.Code)
	}
}
