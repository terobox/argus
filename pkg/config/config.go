// Package config holds helpers shared by the agent and server configuration
// loaders: the YAML Duration type and the non-strict file loader.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a time.Duration that marshals to and from a human-readable
// string ("1s", "5m") in YAML, instead of a bare integer.
type Duration time.Duration

// UnmarshalYAML parses a Go duration string.
func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(v)
	return nil
}

// MarshalYAML renders d as a duration string.
func (d Duration) MarshalYAML() (interface{}, error) {
	return time.Duration(d).String(), nil
}

// D returns the value as a time.Duration.
func (d Duration) D() time.Duration { return time.Duration(d) }

// Load reads and decodes the YAML file at path into dst. It is non-strict:
// unknown keys are ignored so an older binary can read a newer config file
// without failing. The tradeoff is that a typo'd key silently falls back to its
// zero/default value; callers should Validate() the result.
func Load(path string, dst any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(b, dst); err != nil {
		return fmt.Errorf("parse config %s: %w", path, err)
	}
	return nil
}
