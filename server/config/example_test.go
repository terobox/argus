package config

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestExampleConfig keeps the committed server/config.example.yaml in sync with
// the schema. LoadServer is intentionally non-strict, so an unknown key (for
// example a resurrected web_dir) would be silently ignored; the strict decode
// below is what actually catches drift.
func TestExampleConfig(t *testing.T) {
	cfg, err := LoadServer("../config.example.yaml")
	if err != nil {
		t.Fatalf("load example config: %v", err)
	}
	if cfg.ListenAddr == "" || cfg.DBPath == "" {
		t.Fatalf("example config is missing required fields: %+v", cfg)
	}

	f, err := os.Open("../config.example.yaml")
	if err != nil {
		t.Fatalf("open example config: %v", err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	var strict ServerConfig
	if err := dec.Decode(&strict); err != nil {
		t.Fatalf("example config has unknown or invalid keys: %v", err)
	}
}
