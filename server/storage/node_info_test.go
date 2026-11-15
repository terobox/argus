package storage

import (
	"context"
	"testing"

	"github.com/terobox/argus/pkg/metrics"
)

func TestNodeInfoUpsertAndGet(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	// Absent -> ok=false.
	if _, ok, err := s.GetNodeInfo(ctx, "n1"); err != nil || ok {
		t.Fatalf("GetNodeInfo(absent) = ok %v, err %v; want ok=false, nil", ok, err)
	}

	hi := metrics.HostInfo{
		Hostname: "web-1", OS: "linux", Kernel: "6.6", Arch: "amd64",
		CPUModel: "x", CPUCores: 4, MemTotal: 16 << 30, PublicIP: "1.2.3.4", AgentVer: "dev",
	}
	if err := s.UpsertNodeInfo(ctx, "n1", hi, 100); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.GetNodeInfo(ctx, "n1")
	if err != nil || !ok {
		t.Fatalf("GetNodeInfo = ok %v, err %v", ok, err)
	}
	if got.Hostname != "web-1" || got.CPUCores != 4 || got.MemTotal != 16<<30 || got.PublicIP != "1.2.3.4" {
		t.Errorf("got = %+v", got)
	}

	// Upsert replaces in place (no duplicate rows).
	hi2 := hi
	hi2.Hostname = "web-2"
	if err := s.UpsertNodeInfo(ctx, "n1", hi2, 200); err != nil {
		t.Fatal(err)
	}
	got, _, _ = s.GetNodeInfo(ctx, "n1")
	if got.Hostname != "web-2" {
		t.Errorf("after upsert hostname = %q, want web-2", got.Hostname)
	}
}

func TestListNodeSummaries(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	seed := []struct{ id, host string }{{"a", "host-a"}, {"b", ""}, {"c", "host-c"}}
	for _, n := range seed {
		if err := s.UpsertNode(ctx, Node{NodeID: n.id, TokenHash: HashToken("t"), FirstSeen: 1, LastSeen: 2}); err != nil {
			t.Fatal(err)
		}
		if n.host != "" {
			if err := s.UpsertNodeInfo(ctx, n.id, metrics.HostInfo{Hostname: n.host, AgentVer: "dev"}, 1); err != nil {
				t.Fatal(err)
			}
		}
	}

	got, err := s.ListNodeSummaries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d summaries, want 3", len(got))
	}
	// Ordered by node_id.
	if got[0].NodeID != "a" || got[1].NodeID != "b" || got[2].NodeID != "c" {
		t.Fatalf("order = %+v", got)
	}
	if got[0].Hostname != "host-a" {
		t.Errorf("a hostname = %q, want host-a", got[0].Hostname)
	}
	if got[1].Hostname != "" {
		t.Errorf("b hostname = %q, want empty", got[1].Hostname)
	}
}

func TestGetNode(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	if _, ok, err := s.GetNode(ctx, "nope"); err != nil || ok {
		t.Fatalf("GetNode(absent) = ok %v, err %v", ok, err)
	}
	if err := s.UpsertNode(ctx, Node{NodeID: "n1", TokenHash: HashToken("t"), FirstSeen: 1, LastSeen: 2}); err != nil {
		t.Fatal(err)
	}
	n, ok, err := s.GetNode(ctx, "n1")
	if err != nil || !ok {
		t.Fatalf("GetNode = ok %v, err %v", ok, err)
	}
	if n.NodeID != "n1" || n.LastSeen != 2 {
		t.Errorf("node = %+v", n)
	}
}
