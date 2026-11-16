package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "server.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadServerDefaults(t *testing.T) {
	path := writeConfig(t, "listen_addr: :9000\n")
	cfg, err := LoadServer(path)
	if err != nil {
		t.Fatalf("LoadServer: %v", err)
	}
	if cfg.ListenAddr != ":9000" {
		t.Errorf("listen_addr = %q, want :9000", cfg.ListenAddr)
	}
	if cfg.DBPath != "argus.db" {
		t.Errorf("db_path = %q, want argus.db (default)", cfg.DBPath)
	}
	if cfg.JobInterval.D() != 15*time.Second {
		t.Errorf("job_interval = %v, want 15s", cfg.JobInterval.D())
	}
}

// TestLoadServerIgnoresRemovedWebDir documents the compatibility behavior of
// the non-strict loader: configs written for older releases may still carry
// web_dir, which is now ignored instead of rejected.
func TestLoadServerIgnoresRemovedWebDir(t *testing.T) {
	path := writeConfig(t, "web_dir: web/dist\n")
	if _, err := LoadServer(path); err != nil {
		t.Fatalf("legacy web_dir key must be ignored: %v", err)
	}
}

func TestLoadServerOverrides(t *testing.T) {
	path := writeConfig(t, "listen_addr: 127.0.0.1:8095\ndb_path: /var/lib/argus/argus.db\njob_interval: 30s\n")
	cfg, err := LoadServer(path)
	if err != nil {
		t.Fatalf("LoadServer: %v", err)
	}
	if cfg.ListenAddr != "127.0.0.1:8095" {
		t.Errorf("listen_addr = %q", cfg.ListenAddr)
	}
	if cfg.DBPath != "/var/lib/argus/argus.db" {
		t.Errorf("db_path = %q", cfg.DBPath)
	}
	if cfg.JobInterval.D() != 30*time.Second {
		t.Errorf("job_interval = %v, want 30s", cfg.JobInterval.D())
	}
}

func TestServerValidate(t *testing.T) {
	valid := DefaultServer()

	tests := []struct {
		name   string
		mutate func(*ServerConfig)
	}{
		{"missing listen addr", func(c *ServerConfig) { c.ListenAddr = "" }},
		{"missing db path", func(c *ServerConfig) { c.DBPath = "" }},
		{"zero job interval", func(c *ServerConfig) { c.JobInterval = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := valid
			tt.mutate(&c)
			if err := c.Validate(); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}

	if err := valid.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}
