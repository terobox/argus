package storage

import (
	"context"
	"testing"
)

func TestHashToken(t *testing.T) {
	a := HashToken("secret")
	if a != HashToken("secret") {
		t.Fatal("HashToken is not deterministic")
	}
	if a == HashToken("secret2") {
		t.Fatal("different tokens must hash differently")
	}
	if len(a) != 64 {
		t.Fatalf("hash length = %d, want 64 hex chars", len(a))
	}
}

func TestUpsertAndVerifyNodeToken(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	now := int64(1000)

	if err := s.UpsertNode(ctx, Node{NodeID: "n1", TokenHash: HashToken("tok"), FirstSeen: now, LastSeen: now}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if ok, err := s.VerifyNodeToken(ctx, "n1", "tok"); err != nil || !ok {
		t.Fatalf("verify correct token = (%v, %v), want (true, nil)", ok, err)
	}
	if ok, err := s.VerifyNodeToken(ctx, "n1", "wrong"); err != nil || ok {
		t.Fatalf("verify wrong token = (%v, %v), want (false, nil)", ok, err)
	}
	if ok, err := s.VerifyNodeToken(ctx, "unknown", "tok"); err != nil || ok {
		t.Fatalf("verify unknown node = (%v, %v), want (false, nil)", ok, err)
	}
}

func TestTouchAndListNodes(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	if err := s.UpsertNode(ctx, Node{NodeID: "b", TokenHash: HashToken("tb"), FirstSeen: 1, LastSeen: 1}); err != nil {
		t.Fatalf("upsert b: %v", err)
	}
	if err := s.UpsertNode(ctx, Node{NodeID: "a", TokenHash: HashToken("ta"), FirstSeen: 1, LastSeen: 1}); err != nil {
		t.Fatalf("upsert a: %v", err)
	}
	if err := s.TouchNode(ctx, "a", 42); err != nil {
		t.Fatalf("touch: %v", err)
	}

	nodes, err := s.ListNodes(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("got %d nodes, want 2", len(nodes))
	}
	if nodes[0].NodeID != "a" || nodes[1].NodeID != "b" {
		t.Fatalf("order = %q, %q; want a, b", nodes[0].NodeID, nodes[1].NodeID)
	}
	if nodes[0].LastSeen != 42 {
		t.Errorf("node a last_seen = %d, want 42", nodes[0].LastSeen)
	}
}
