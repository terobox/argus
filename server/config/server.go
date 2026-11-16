package config

import (
	"fmt"
	"time"

	base "github.com/terobox/argus/pkg/config"
)

// Duration is an alias for the shared YAML duration type, so the rest of this
// package (and its tests) can keep referring to Duration unqualified.
type Duration = base.Duration

// ServerConfig is the server's on-disk configuration. A config file may omit any
// field; LoadServer starts from DefaultServer and overlays only the keys present
// in the file.
type ServerConfig struct {
	ListenAddr   string      `yaml:"listen_addr"`
	DBPath       string      `yaml:"db_path"`
	JobInterval  Duration    `yaml:"job_interval"`
	SessionTTL   Duration    `yaml:"session_ttl"`
	CookieSecure bool        `yaml:"cookie_secure"`
	Alert        AlertConfig `yaml:"alert"`
}

// DefaultServer returns the server defaults.
func DefaultServer() ServerConfig {
	return ServerConfig{
		ListenAddr:   ":8095",
		DBPath:       "argus.db",
		JobInterval:  Duration(15 * time.Second),
		SessionTTL:   Duration(7 * 24 * time.Hour),
		CookieSecure: false, // true when serving over HTTPS
		Alert:        defaultAlert(),
	}
}

// LoadServer loads path, applying defaults for any key the file omits, and
// validates the result.
func LoadServer(path string) (ServerConfig, error) {
	cfg := DefaultServer()
	if err := base.Load(path, &cfg); err != nil {
		return cfg, err
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Validate reports the first problem found in c.
func (c ServerConfig) Validate() error {
	if c.ListenAddr == "" {
		return fmt.Errorf("listen_addr is required")
	}
	if c.DBPath == "" {
		return fmt.Errorf("db_path is required")
	}
	if c.JobInterval.D() <= 0 {
		return fmt.Errorf("job_interval must be > 0")
	}
	if c.SessionTTL.D() <= 0 {
		return fmt.Errorf("session_ttl must be > 0")
	}
	if err := c.Alert.Validate(); err != nil {
		return err
	}
	return nil
}
