package storage

import (
	"context"
	"testing"
)

func TestAdminUpsertGetCount(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	if n, err := s.CountAdmins(ctx); err != nil || n != 0 {
		t.Fatalf("count empty = %d, %v; want 0", n, err)
	}
	if _, ok, err := s.GetAdmin(ctx, "admin"); err != nil || ok {
		t.Fatalf("get absent = ok %v, %v; want absent", ok, err)
	}

	if err := s.UpsertAdmin(ctx, Admin{Username: "admin", PasswordHash: "hash1", CreatedAt: 10}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	a, ok, err := s.GetAdmin(ctx, "admin")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if a.PasswordHash != "hash1" || a.CreatedAt != 10 {
		t.Fatalf("got %+v", a)
	}
	if n, _ := s.CountAdmins(ctx); n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}

	// Upsert replaces rather than duplicates.
	if err := s.UpsertAdmin(ctx, Admin{Username: "admin", PasswordHash: "hash2", CreatedAt: 20}); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.CountAdmins(ctx); n != 1 {
		t.Fatalf("count after replace = %d, want 1", n)
	}
}

func TestCreateInitialAdmin(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	created, err := s.CreateInitialAdmin(ctx, Admin{Username: "admin", PasswordHash: "hash1", CreatedAt: 10})
	if err != nil {
		t.Fatalf("create initial admin: %v", err)
	}
	if !created {
		t.Fatal("first CreateInitialAdmin should create the row")
	}

	// A second call must not replace or add an admin.
	created, err = s.CreateInitialAdmin(ctx, Admin{Username: "other", PasswordHash: "hash2", CreatedAt: 20})
	if err != nil {
		t.Fatalf("second create initial admin: %v", err)
	}
	if created {
		t.Fatal("second CreateInitialAdmin must be a no-op")
	}
	if n, _ := s.CountAdmins(ctx); n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
	a, ok, err := s.GetAdmin(ctx, "admin")
	if err != nil || !ok {
		t.Fatalf("get admin: ok=%v err=%v", ok, err)
	}
	if a.PasswordHash != "hash1" {
		t.Fatalf("initial admin was replaced: %+v", a)
	}
	if _, ok, _ := s.GetAdmin(ctx, "other"); ok {
		t.Fatal("second admin must not exist")
	}
}
