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
	return New(store, logger, a, nil, ""), store
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
