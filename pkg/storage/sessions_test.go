package storage

import (
	"context"
	"testing"
)

func TestSessionInsertGetDelete(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	sess := Session{TokenHash: "h1", Username: "admin", CSRF: "c1", CreatedAt: 1, ExpiresAt: 100}
	if err := s.InsertSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.GetSession(ctx, "h1")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if got != sess {
		t.Fatalf("got %+v, want %+v", got, sess)
	}

	if err := s.DeleteSession(ctx, "h1"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.GetSession(ctx, "h1"); ok {
		t.Fatal("session still present after delete")
	}
}

func TestDeleteExpiredSessions(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	mk := func(h string, exp int64) Session {
		return Session{TokenHash: h, Username: "admin", CSRF: "c", CreatedAt: 1, ExpiresAt: exp}
	}
	if err := s.InsertSession(ctx, mk("expired", 10)); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertSession(ctx, mk("fresh", 9999)); err != nil {
		t.Fatal(err)
	}

	n, err := s.DeleteExpiredSessions(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("deleted %d, want 1", n)
	}
	if _, ok, _ := s.GetSession(ctx, "expired"); ok {
		t.Fatal("expired session not deleted")
	}
	if _, ok, _ := s.GetSession(ctx, "fresh"); !ok {
		t.Fatal("fresh session wrongly deleted")
	}
}
