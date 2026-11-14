package config

import (
	"testing"
	"time"
)

func TestServerAuthDefaults(t *testing.T) {
	cfg := DefaultServer()
	if cfg.SessionTTL.D() != 7*24*time.Hour {
		t.Errorf("session_ttl = %v, want 7d", cfg.SessionTTL.D())
	}
	if cfg.CookieSecure {
		t.Errorf("cookie_secure default should be false")
	}
}

func TestServerAuthOverrides(t *testing.T) {
	path := writeConfig(t, "session_ttl: 2h\ncookie_secure: true\n")
	cfg, err := LoadServer(path)
	if err != nil {
		t.Fatalf("LoadServer: %v", err)
	}
	if cfg.SessionTTL.D() != 2*time.Hour {
		t.Errorf("session_ttl = %v, want 2h", cfg.SessionTTL.D())
	}
	if !cfg.CookieSecure {
		t.Errorf("cookie_secure = false, want true")
	}
}

func TestServerSessionTTLValidate(t *testing.T) {
	c := DefaultServer()
	c.SessionTTL = 0
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for zero session_ttl")
	}
}
