package config

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestExampleConfig keeps the committed agent/config.example.yaml in sync with
// the schema, including rejecting unknown keys the non-strict loader would
// otherwise ignore.
func TestExampleConfig(t *testing.T) {
	cfg, err := LoadAgent("../config.example.yaml")
	if err != nil {
		t.Fatalf("load example config: %v", err)
	}
	if cfg.ServerURL == "" || cfg.NodeID == "" || cfg.Token == "" {
		t.Fatalf("example config is missing required fields: %+v", cfg)
	}

	f, err := os.Open("../config.example.yaml")
	if err != nil {
		t.Fatalf("open example config: %v", err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	var strict AgentConfig
	if err := dec.Decode(&strict); err != nil {
		t.Fatalf("example config has unknown or invalid keys: %v", err)
	}
}
