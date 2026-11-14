package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/terobox/argus/pkg/metrics"
	"github.com/terobox/argus/pkg/storage"
)

func TestSeriesEndpoint(t *testing.T) {
	srv, store := newTestServer(t)
	seedAdmin(t, store, "admin", "secret")
	if err := store.UpsertNode(context.Background(), storage.Node{
		NodeID: "n1", TokenHash: storage.HashToken("tok"), FirstSeen: 1, LastSeen: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.WriteSamples(context.Background(), "n1", []metrics.Sample{testSample(100), testSample(101)}); err != nil {
		t.Fatal(err)
	}

	cookie, _ := login(t, srv.Handler(), "admin", "secret")
	rr := doJSON(t, srv.Handler(), http.MethodGet, "/api/v1/nodes/n1/series?from=100&to=101", cookie, "", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var resp seriesResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Resolution != storage.ResRaw {
		t.Errorf("resolution = %q, want %q", resp.Resolution, storage.ResRaw)
	}
	if len(resp.Points) != 2 {
		t.Fatalf("got %d points, want 2", len(resp.Points))
	}
}

func TestSeriesRequiresAuth(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := doJSON(t, srv.Handler(), http.MethodGet, "/api/v1/nodes/n1/series?from=100&to=101", "", "", nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}
