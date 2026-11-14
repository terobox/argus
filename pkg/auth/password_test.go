package auth

import "testing"

func TestHashAndCheck(t *testing.T) {
	h, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if h == "" || h == "correct horse battery staple" {
		t.Fatal("hash not produced")
	}
	if !CheckPassword(h, "correct horse battery staple") {
		t.Fatal("correct password rejected")
	}
	if CheckPassword(h, "wrong") {
		t.Fatal("wrong password accepted")
	}
}

func TestHashIsSalted(t *testing.T) {
	a, _ := HashPassword("secret")
	b, _ := HashPassword("secret")
	if a == b {
		t.Fatal("bcrypt hashes should be salted and differ")
	}
}

func TestCheckPasswordBadHash(t *testing.T) {
	if CheckPassword("not-a-bcrypt-hash", "secret") {
		t.Fatal("malformed hash should never match")
	}
}
