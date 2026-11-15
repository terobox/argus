package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/terobox/argus/pkg/metrics"
	"github.com/terobox/argus/pkg/protocol"
	"github.com/terobox/argus/server/storage"
)

func postHostInfo(t *testing.T, h http.Handler, rep protocol.HostReport, token string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, protocol.HostPath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set(protocol.NodeTokenHeader, token)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestHostInfoIngest(t *testing.T) {
	srv, store := newTestServer(t)
	if err := store.UpsertNode(context.Background(), storage.Node{
		NodeID: "n1", TokenHash: storage.HashToken("tok"), FirstSeen: 1, LastSeen: 1,
	}); err != nil {
		t.Fatal(err)
	}
	rep := protocol.HostReport{
		Version: protocol.Version, NodeID: "n1", AgentVer: "dev",
		Info: metrics.HostInfo{
			Hostname: "web-1", OS: "linux", Kernel: "6.6", Arch: "amd64",
			CPUModel: "x", CPUCores: 8, MemTotal: 32 << 30, AgentVer: "dev",
		},
	}
	if rr := postHostInfo(t, srv.Handler(), rep, "tok"); rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", rr.Code, rr.Body.String())
	}
	hi, ok, err := store.GetNodeInfo(context.Background(), "n1")
	if err != nil || !ok {
		t.Fatalf("GetNodeInfo = ok %v, err %v", ok, err)
	}
	if hi.Hostname != "web-1" || hi.CPUCores != 8 {
		t.Errorf("hi = %+v", hi)
	}
}

func TestHostInfoWrongToken(t *testing.T) {
	srv, store := newTestServer(t)
	if err := store.UpsertNode(context.Background(), storage.Node{
		NodeID: "n1", TokenHash: storage.HashToken("tok"), FirstSeen: 1, LastSeen: 1,
	}); err != nil {
		t.Fatal(err)
	}
	rep := protocol.HostReport{
		Version: protocol.Version, NodeID: "n1", AgentVer: "dev",
		Info: metrics.HostInfo{Hostname: "web-1", AgentVer: "dev"},
	}
	if rr := postHostInfo(t, srv.Handler(), rep, "wrong"); rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestNodeDetailAndListHostname(t *testing.T) {
	srv, store := newTestServer(t)
	seedAdmin(t, store, "admin", "secret")
	h := srv.Handler()
	if err := store.UpsertNode(context.Background(), storage.Node{
		NodeID: "n1", TokenHash: storage.HashToken("tok"), FirstSeen: 1, LastSeen: 2,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertNodeInfo(context.Background(), "n1",
		metrics.HostInfo{Hostname: "web-1", OS: "linux", AgentVer: "dev"}, 3); err != nil {
		t.Fatal(err)
	}

	cookie, _ := login(t, h, "admin", "secret")

	// List includes hostname.
	rr := doJSON(t, h, http.MethodGet, protocol.NodesPath, cookie, "", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list = %d, body=%s", rr.Code, rr.Body.String())
	}
	var list []nodeView
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Hostname != "web-1" {
		t.Fatalf("list = %+v", list)
	}

	// Detail returns full host info.
	rr = doJSON(t, h, http.MethodGet, "/api/v1/nodes/n1", cookie, "", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("detail = %d, body=%s", rr.Code, rr.Body.String())
	}
	var det nodeDetail
	if err := json.Unmarshal(rr.Body.Bytes(), &det); err != nil {
		t.Fatal(err)
	}
	if det.HostInfo == nil || det.HostInfo.Hostname != "web-1" || det.HostInfo.OS != "linux" {
		t.Fatalf("detail = %+v", det)
	}

	// Unknown node -> 404.
	rr = doJSON(t, h, http.MethodGet, "/api/v1/nodes/ghost", cookie, "", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("unknown detail = %d, want 404", rr.Code)
	}
}
