package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/terobox/argus/pkg/metrics"
	"github.com/terobox/argus/pkg/protocol"
	"github.com/terobox/argus/server/auth"
	"github.com/terobox/argus/server/storage"
)

func newTestServer(t *testing.T) (*Server, *storage.Store) {
	t.Helper()
	ctx := context.Background()
	store, err := storage.Open(filepath.Join(t.TempDir(), "argus.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := auth.New(store, logger, false, auth.DefaultSessionTTL)
	return New(store, logger, a, nil), store
}

// TestUnknownRoutes pins the behavior of the API-only mux: there is no SPA
// catch-all, unknown paths 404, the root is a JSON welcome, and a known path
// with the wrong method gets the ServeMux 405.
func TestUnknownRoutes(t *testing.T) {
	s, _ := newTestServer(t)
	h := s.Handler()

	tests := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"root", http.MethodGet, "/", http.StatusOK},
		{"root wrong method", http.MethodPut, "/", http.StatusMethodNotAllowed},
		{"unknown api", http.MethodGet, "/api/v1/unknown", http.StatusNotFound},
		{"wrong method", http.MethodPut, protocol.NodesPath, http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, httptest.NewRequest(tt.method, tt.path, nil))
			if rr.Code != tt.want {
				t.Fatalf("%s %s = %d, want %d", tt.method, tt.path, rr.Code, tt.want)
			}
		})
	}
}

func TestRootWelcome(t *testing.T) {
	s, _ := newTestServer(t)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content-type = %q, want application/json", ct)
	}
	var resp struct {
		Message    string `json:"message"`
		Repository string `json:"repository"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode root: %v", err)
	}
	if !strings.Contains(resp.Message, "Argus") {
		t.Fatalf("message = %q, want a welcome naming Argus", resp.Message)
	}
	if resp.Repository == "" {
		t.Fatal("repository = empty, want the project URL")
	}
}

func setupStatus(t *testing.T, h http.Handler) bool {
	t.Helper()
	rr := doJSON(t, h, http.MethodGet, protocol.SetupStatusPath, "", "", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("setup status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Required bool `json:"setup_required"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode setup status: %v", err)
	}
	return resp.Required
}

func TestSetupFlow(t *testing.T) {
	srv, store := newTestServer(t)
	h := srv.Handler()

	if !setupStatus(t, h) {
		t.Fatal("setup should be required on a fresh database")
	}

	// A too-short password is rejected before anything is created.
	rr := doJSON(t, h, http.MethodPost, protocol.SetupPath, "", "", map[string]string{
		"username": "admin", "password": "short",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("short password = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	if !setupStatus(t, h) {
		t.Fatal("a rejected setup must not create an admin")
	}

	rr = doJSON(t, h, http.MethodPost, protocol.SetupPath, "", "", map[string]string{
		"username": "admin", "password": "dev-password",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("setup = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Username string `json:"username"`
		CSRF     string `json:"csrf_token"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode setup: %v", err)
	}
	if resp.Username != "admin" || resp.CSRF == "" {
		t.Fatalf("setup response = %+v, want username and csrf_token", resp)
	}
	if len(rr.Result().Cookies()) == 0 {
		t.Fatal("setup must sign the new admin in")
	}

	if setupStatus(t, h) {
		t.Fatal("setup should be closed after the first admin is created")
	}

	// A second setup attempt is refused.
	rr = doJSON(t, h, http.MethodPost, protocol.SetupPath, "", "", map[string]string{
		"username": "other", "password": "another-password",
	})
	if rr.Code != http.StatusConflict {
		t.Fatalf("second setup = %d, want 409", rr.Code)
	}

	// The credentials created by setup work for a normal login.
	login(t, h, "admin", "dev-password")

	if n, err := store.CountAdmins(context.Background()); err != nil {
		t.Fatalf("count admins: %v", err)
	} else if n != 1 {
		t.Fatalf("admins = %d, want 1", n)
	}
}

func TestSetupStatusWithExistingAdmin(t *testing.T) {
	srv, store := newTestServer(t)
	seedAdmin(t, store, "admin", "secret")
	if setupStatus(t, srv.Handler()) {
		t.Fatal("setup must be closed when an admin already exists")
	}
}

func TestHealthz(t *testing.T) {
	s, _ := newTestServer(t)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rr.Code, http.StatusOK, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content-type = %q, want application/json", ct)
	}
}

func TestHealthzDatabaseDown(t *testing.T) {
	s, store := newTestServer(t)
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func testSample(ts int64) metrics.Sample {
	return metrics.Sample{
		TS:   ts,
		CPU:  metrics.CPU{UsedPercent: 25},
		Load: metrics.Load{Load1: 0.5, Load5: 0.4, Load15: 0.3},
		Mem:  metrics.Mem{TotalBytes: 16 << 30, AvailableBytes: 8 << 30, UsedBytes: 8 << 30},
		Swap: metrics.Swap{TotalBytes: 4 << 30, UsedBytes: 1 << 30},
		Net:  metrics.Net{RxBytes: 1, TxBytes: 2},
	}
}

func postReport(t *testing.T, h http.Handler, rep protocol.Report, token string, gzipBody bool) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var buf bytes.Buffer
	if gzipBody {
		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write(body); err != nil {
			t.Fatalf("gzip write: %v", err)
		}
		if err := zw.Close(); err != nil {
			t.Fatalf("gzip close: %v", err)
		}
	} else {
		buf.Write(body)
	}

	req := httptest.NewRequest(http.MethodPost, protocol.IngestPath, &buf)
	req.Header.Set("Content-Type", "application/json")
	if gzipBody {
		req.Header.Set("Content-Encoding", "gzip")
	}
	if token != "" {
		req.Header.Set(protocol.NodeTokenHeader, token)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func seedAdmin(t *testing.T, store *storage.Store, username, password string) {
	t.Helper()
	h, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAdmin(context.Background(), storage.Admin{
		Username: username, PasswordHash: h, CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
}

// login performs a successful login and returns the session cookie value and
// the CSRF token.
func login(t *testing.T, h http.Handler, username, password string) (cookie, csrf string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req := httptest.NewRequest(http.MethodPost, protocol.LoginPath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login = %d, body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		CSRF string `json:"csrf_token"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	cookies := rr.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no session cookie set")
	}
	return cookies[0].Value, resp.CSRF
}

// doJSON issues a JSON request, attaching the session cookie and CSRF token when
// given.
func doJSON(t *testing.T, h http.Handler, method, path, cookie, csrf string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	}
	if csrf != "" {
		req.Header.Set(auth.CSRFHeader, csrf)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestIngestValidReport(t *testing.T) {
	srv, store := newTestServer(t)
	if err := store.UpsertNode(context.Background(), storage.Node{
		NodeID: "n1", TokenHash: storage.HashToken("tok"), FirstSeen: 1, LastSeen: 1,
	}); err != nil {
		t.Fatal(err)
	}
	rep := protocol.Report{
		Version: protocol.Version, NodeID: "n1", AgentVer: "dev", CollectedAt: 100,
		Samples: []metrics.Sample{testSample(100), testSample(101)},
	}
	rr := postReport(t, srv.Handler(), rep, "tok", false)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", rr.Code, rr.Body.String())
	}
	got, err := store.QueryRaw(context.Background(), "n1", 100, 101)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d samples, want 2", len(got))
	}
}

func TestIngestGzip(t *testing.T) {
	srv, store := newTestServer(t)
	if err := store.UpsertNode(context.Background(), storage.Node{
		NodeID: "n1", TokenHash: storage.HashToken("tok"), FirstSeen: 1, LastSeen: 1,
	}); err != nil {
		t.Fatal(err)
	}
	rep := protocol.Report{
		Version: protocol.Version, NodeID: "n1", CollectedAt: 100,
		Samples: []metrics.Sample{testSample(100)},
	}
	if rr := postReport(t, srv.Handler(), rep, "tok", true); rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", rr.Code, rr.Body.String())
	}
	got, err := store.QueryRaw(context.Background(), "n1", 100, 100)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d samples, want 1", len(got))
	}
}

func TestIngestWrongToken(t *testing.T) {
	srv, store := newTestServer(t)
	if err := store.UpsertNode(context.Background(), storage.Node{
		NodeID: "n1", TokenHash: storage.HashToken("tok"), FirstSeen: 1, LastSeen: 1,
	}); err != nil {
		t.Fatal(err)
	}
	rep := protocol.Report{
		Version: protocol.Version, NodeID: "n1", CollectedAt: 100,
		Samples: []metrics.Sample{testSample(100)},
	}
	if rr := postReport(t, srv.Handler(), rep, "wrong", false); rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestIngestUnknownNode(t *testing.T) {
	srv, _ := newTestServer(t)
	rep := protocol.Report{
		Version: protocol.Version, NodeID: "ghost", CollectedAt: 100,
		Samples: []metrics.Sample{testSample(100)},
	}
	if rr := postReport(t, srv.Handler(), rep, "tok", false); rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestIngestInvalidReport(t *testing.T) {
	srv, store := newTestServer(t)
	if err := store.UpsertNode(context.Background(), storage.Node{
		NodeID: "n1", TokenHash: storage.HashToken("tok"), FirstSeen: 1, LastSeen: 1,
	}); err != nil {
		t.Fatal(err)
	}

	// Missing node_id.
	rep := protocol.Report{
		Version: protocol.Version, CollectedAt: 100,
		Samples: []metrics.Sample{testSample(100)},
	}
	if rr := postReport(t, srv.Handler(), rep, "tok", false); rr.Code != http.StatusBadRequest {
		t.Fatalf("missing node_id status = %d, want 400", rr.Code)
	}

	// Duplicate (non-increasing) timestamps.
	rep = protocol.Report{
		Version: protocol.Version, NodeID: "n1", CollectedAt: 100,
		Samples: []metrics.Sample{testSample(100), testSample(100)},
	}
	if rr := postReport(t, srv.Handler(), rep, "tok", false); rr.Code != http.StatusBadRequest {
		t.Fatalf("duplicate ts status = %d, want 400", rr.Code)
	}
}

func TestIngestMalformedJSON(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, protocol.IngestPath, bytes.NewBufferString("{not json"))
	req.Header.Set(protocol.NodeTokenHeader, "tok")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestIngestUpdatesLastSeen(t *testing.T) {
	srv, store := newTestServer(t)
	if err := store.UpsertNode(context.Background(), storage.Node{
		NodeID: "n1", TokenHash: storage.HashToken("tok"), FirstSeen: 1, LastSeen: 1,
	}); err != nil {
		t.Fatal(err)
	}
	rep := protocol.Report{
		Version: protocol.Version, NodeID: "n1", CollectedAt: 5000,
		Samples: []metrics.Sample{testSample(100)},
	}
	if rr := postReport(t, srv.Handler(), rep, "tok", false); rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
	nodes, err := store.ListNodes(context.Background())
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	if len(nodes) != 1 || nodes[0].LastSeen != 5000 {
		t.Fatalf("nodes = %+v, want last_seen 5000", nodes)
	}
}

func TestNodesRequireAuth(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := doJSON(t, srv.Handler(), http.MethodGet, protocol.NodesPath, "", "", nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	srv, store := newTestServer(t)
	seedAdmin(t, store, "admin", "secret")

	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "nope"})
	req := httptest.NewRequest(http.MethodPost, protocol.LoginPath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestAuthAndNodeCRUD(t *testing.T) {
	srv, store := newTestServer(t)
	seedAdmin(t, store, "admin", "secret")
	h := srv.Handler()

	cookie, csrf := login(t, h, "admin", "secret")

	// Create a node (needs CSRF).
	rr := doJSON(t, h, http.MethodPost, protocol.NodesPath, cookie, csrf, map[string]string{"node_id": "node-1"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create node = %d, body=%s", rr.Code, rr.Body.String())
	}
	var created createNodeResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Token == "" {
		t.Fatal("no token in create response")
	}

	// The new token must authenticate an ingest.
	ing := protocol.Report{
		Version: protocol.Version, NodeID: "node-1", CollectedAt: 100,
		Samples: []metrics.Sample{testSample(100)},
	}
	if rr := postReport(t, h, ing, created.Token, false); rr.Code != http.StatusNoContent {
		t.Fatalf("ingest with new token = %d, want 204", rr.Code)
	}

	// List nodes (GET, no CSRF needed).
	rr = doJSON(t, h, http.MethodGet, protocol.NodesPath, cookie, "", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list nodes = %d, want 200", rr.Code)
	}

	// Mutation without CSRF must be rejected.
	rr = doJSON(t, h, http.MethodPost, protocol.NodesPath, cookie, "", map[string]string{"node_id": "node-2"})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("create node without csrf = %d, want 403", rr.Code)
	}

	// Delete node (needs CSRF).
	rr = doJSON(t, h, http.MethodDelete, protocol.NodesPath, cookie, csrf, map[string]string{"node_id": "node-1"})
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete node = %d, want 204", rr.Code)
	}

	// Revoked node can no longer ingest.
	if rr := postReport(t, h, ing, created.Token, false); rr.Code != http.StatusUnauthorized {
		t.Fatalf("ingest after revoke = %d, want 401", rr.Code)
	}
}

func TestRotateNodeToken(t *testing.T) {
	srv, store := newTestServer(t)
	if err := store.UpsertNode(context.Background(), storage.Node{
		NodeID: "n1", TokenHash: storage.HashToken("old"), FirstSeen: 1, LastSeen: 1, Tags: []string{"prod"},
	}); err != nil {
		t.Fatal(err)
	}
	seedAdmin(t, store, "admin", "secret")
	h := srv.Handler()
	cookie, csrf := login(t, h, "admin", "secret")

	// Rotation is a mutation: it needs the CSRF token.
	rr := doJSON(t, h, http.MethodPost, "/api/v1/nodes/n1/token", cookie, "", nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("rotate without csrf = %d, want 403", rr.Code)
	}

	rr = doJSON(t, h, http.MethodPost, "/api/v1/nodes/n1/token", cookie, csrf, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("rotate = %d; body=%s", rr.Code, rr.Body.String())
	}
	var resp createNodeResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode rotate: %v", err)
	}
	if resp.NodeID != "n1" || resp.Token == "" {
		t.Fatalf("rotate response = %+v, want node_id and token", resp)
	}

	rep := protocol.Report{
		Version: protocol.Version, NodeID: "n1", CollectedAt: 100,
		Samples: []metrics.Sample{testSample(100)},
	}
	if rr := postReport(t, h, rep, "old", false); rr.Code != http.StatusUnauthorized {
		t.Fatalf("old token after rotate = %d, want 401", rr.Code)
	}
	if rr := postReport(t, h, rep, resp.Token, false); rr.Code != http.StatusNoContent {
		t.Fatalf("new token after rotate = %d, want 204; body=%s", rr.Code, rr.Body.String())
	}

	// Rotation preserves the node's tags and timestamps.
	nodes, err := store.ListNodeSummaries(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(nodes) != 1 || len(nodes[0].Tags) != 1 || nodes[0].Tags[0] != "prod" {
		t.Fatalf("nodes = %+v, want tags preserved", nodes)
	}

	rr = doJSON(t, h, http.MethodPost, "/api/v1/nodes/ghost/token", cookie, csrf, nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("rotate unknown node = %d, want 404", rr.Code)
	}
}
