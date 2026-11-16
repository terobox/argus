package auth

import (
	"strings"
	"testing"
)

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

func TestValidateCredentials(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
		wantErr  bool
	}{
		{"valid", "admin", "dev-password", false},
		{"empty username", "  ", "dev-password", true},
		{"long username", strings.Repeat("a", maxUsernameLen+1), "dev-password", true},
		{"short password", "admin", "short", true},
		{"bcrypt limit", "admin", strings.Repeat("a", maxPasswordBytes+1), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUsername(tt.username)
			if err == nil {
				err = validatePassword(tt.password)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("validate(%q, %d bytes) err = %v, wantErr %v",
					tt.username, len(tt.password), err, tt.wantErr)
			}
		})
	}
}
