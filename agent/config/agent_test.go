package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadAgentAppliesDefaults(t *testing.T) {
	path := writeConfig(t, "server_url: http://example.com\nnode_id: n1\ntoken: t1\n")
	cfg, err := LoadAgent(path)
	if err != nil {
		t.Fatalf("LoadAgent: %v", err)
	}
	if cfg.CollectInterval.D() != time.Second {
		t.Errorf("collect_interval = %v, want 1s", cfg.CollectInterval.D())
	}
	if cfg.SlowInterval.D() != 30*time.Second {
		t.Errorf("slow_interval = %v, want 30s", cfg.SlowInterval.D())
	}
	if cfg.PushInterval.D() != 15*time.Second {
		t.Errorf("push_interval = %v, want 15s", cfg.PushInterval.D())
	}
	if cfg.BufferCapacity != 600 {
		t.Errorf("buffer_capacity = %d, want 600", cfg.BufferCapacity)
	}
	if !cfg.Gzip {
		t.Errorf("gzip = false, want true (default)")
	}
}

func TestLoadAgentOverrides(t *testing.T) {
	path := writeConfig(t, `server_url: https://x.example.com:8095
node_id: us-west-01
token: secret
collect_interval: 2s
slow_interval: 60s
push_interval: 10s
buffer_capacity: 100
gzip: false
public_ip: 203.0.113.7
`)
	cfg, err := LoadAgent(path)
	if err != nil {
		t.Fatalf("LoadAgent: %v", err)
	}
	if cfg.CollectInterval.D() != 2*time.Second {
		t.Errorf("collect_interval = %v, want 2s", cfg.CollectInterval.D())
	}
	if cfg.BufferCapacity != 100 {
		t.Errorf("buffer_capacity = %d, want 100", cfg.BufferCapacity)
	}
	if cfg.Gzip {
		t.Errorf("gzip = true, want false")
	}
	if cfg.PublicIP != "203.0.113.7" {
		t.Errorf("public_ip = %q", cfg.PublicIP)
	}
}

func TestAgentValidate(t *testing.T) {
	valid := DefaultAgent()
	valid.ServerURL = "http://example.com"
	valid.NodeID = "n1"
	valid.Token = "t1"

	tests := []struct {
		name   string
		mutate func(*AgentConfig)
	}{
		{"missing server url", func(c *AgentConfig) { c.ServerURL = "" }},
		{"bad scheme", func(c *AgentConfig) { c.ServerURL = "ftp://example.com" }},
		{"missing host", func(c *AgentConfig) { c.ServerURL = "http://" }},
		{"missing node id", func(c *AgentConfig) { c.NodeID = "" }},
		{"missing token", func(c *AgentConfig) { c.Token = "" }},
		{"zero collect", func(c *AgentConfig) { c.CollectInterval = 0 }},
		{"push below collect", func(c *AgentConfig) { c.PushInterval = Duration(500 * time.Millisecond) }},
		{"slow below collect", func(c *AgentConfig) { c.SlowInterval = Duration(500 * time.Millisecond) }},
		{"zero buffer", func(c *AgentConfig) { c.BufferCapacity = 0 }},
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

func TestDurationUnmarshal(t *testing.T) {
	path := writeConfig(t, "server_url: http://example.com\nnode_id: n\ntoken: t\npush_interval: 2m30s\n")
	cfg, err := LoadAgent(path)
	if err != nil {
		t.Fatalf("LoadAgent: %v", err)
	}
	if cfg.PushInterval.D() != 150*time.Second {
		t.Errorf("push_interval = %v, want 2m30s", cfg.PushInterval.D())
	}
}

func TestLoadAgentBadDuration(t *testing.T) {
	path := writeConfig(t, "server_url: http://example.com\nnode_id: n\ntoken: t\ncollect_interval: fast\n")
	if _, err := LoadAgent(path); err == nil {
		t.Fatal("expected error for invalid duration, got nil")
	}
}
