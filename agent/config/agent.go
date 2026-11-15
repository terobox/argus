package config

import (
	"fmt"
	"net/url"
	"time"

	base "github.com/terobox/argus/pkg/config"
)

// Duration is an alias for the shared YAML duration type, so the rest of this
// package (and its tests) can keep referring to Duration unqualified.
type Duration = base.Duration

// AgentConfig is the agent's on-disk configuration. A config file may omit any
// field; LoadAgent starts from DefaultAgent and overlays only the keys present
// in the file.
type AgentConfig struct {
	ServerURL       string   `yaml:"server_url"`
	NodeID          string   `yaml:"node_id"`
	Token           string   `yaml:"token"`
	CollectInterval Duration `yaml:"collect_interval"`
	SlowInterval    Duration `yaml:"slow_interval"`
	PushInterval    Duration `yaml:"push_interval"`
	BufferCapacity  int      `yaml:"buffer_capacity"`
	PublicIP        string   `yaml:"public_ip,omitempty"`
	Gzip            bool     `yaml:"gzip"`
}

// DefaultAgent returns the agent's defaults. They are deliberately tuned to the
// retention tiers in the server's storage layer: 1s collection rolled into 15s push batches,
// with a 10-minute in-memory buffer for offline resilience.
func DefaultAgent() AgentConfig {
	return AgentConfig{
		CollectInterval: Duration(1 * time.Second),
		SlowInterval:    Duration(30 * time.Second),
		PushInterval:    Duration(15 * time.Second),
		BufferCapacity:  600, // 10 minutes at 1s
		Gzip:            true,
	}
}

// LoadAgent loads path, applying defaults for any key the file omits, and
// validates the result.
func LoadAgent(path string) (AgentConfig, error) {
	cfg := DefaultAgent()
	if err := base.Load(path, &cfg); err != nil {
		return cfg, err
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Validate reports the first problem found in c.
func (c AgentConfig) Validate() error {
	if c.ServerURL == "" {
		return fmt.Errorf("server_url is required")
	}
	u, err := url.Parse(c.ServerURL)
	if err != nil {
		return fmt.Errorf("server_url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("server_url scheme must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("server_url must include a host")
	}
	if c.NodeID == "" {
		return fmt.Errorf("node_id is required")
	}
	if c.Token == "" {
		return fmt.Errorf("token is required")
	}
	if c.CollectInterval.D() <= 0 {
		return fmt.Errorf("collect_interval must be > 0")
	}
	if c.SlowInterval.D() <= 0 {
		return fmt.Errorf("slow_interval must be > 0")
	}
	if c.PushInterval.D() <= 0 {
		return fmt.Errorf("push_interval must be > 0")
	}
	if c.SlowInterval.D() < c.CollectInterval.D() {
		return fmt.Errorf("slow_interval (%s) must be >= collect_interval (%s)",
			time.Duration(c.SlowInterval), time.Duration(c.CollectInterval))
	}
	if c.PushInterval.D() < c.CollectInterval.D() {
		return fmt.Errorf("push_interval (%s) must be >= collect_interval (%s)",
			time.Duration(c.PushInterval), time.Duration(c.CollectInterval))
	}
	if c.BufferCapacity <= 0 {
		return fmt.Errorf("buffer_capacity must be > 0")
	}
	return nil
}
