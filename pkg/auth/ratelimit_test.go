package auth

import (
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	r := NewRateLimiter(3, time.Hour)
	for i := 0; i < 3; i++ {
		if !r.Allow("key") {
			t.Fatalf("attempt %d blocked early", i)
		}
	}
	if r.Allow("key") {
		t.Fatal("attempt over limit allowed")
	}
	// A different key is independent.
	if !r.Allow("other") {
		t.Fatal("other key blocked")
	}
}

func TestRateLimiterWindowReset(t *testing.T) {
	r := NewRateLimiter(1, 10*time.Millisecond)
	if !r.Allow("key") {
		t.Fatal("first attempt blocked")
	}
	if r.Allow("key") {
		t.Fatal("second attempt within window allowed")
	}
	time.Sleep(20 * time.Millisecond)
	if !r.Allow("key") {
		t.Fatal("attempt after window blocked")
	}
}
