package config

import (
	"fmt"
	"net/url"
	"time"
)

// AlertConfig is the server's alerting configuration. Offline detection and the
// resource thresholds are evaluated on the server's job interval; notifications
// go to a single ntfy webhook.
type AlertConfig struct {
	Enabled       bool     `yaml:"enabled"`
	WebhookURL    string   `yaml:"webhook_url"`
	OfflineAfter  Duration `yaml:"offline_after"`
	Sustain       Duration `yaml:"sustain"`
	CPUThreshold  float64  `yaml:"cpu_threshold"`
	MemThreshold  float64  `yaml:"mem_threshold"`
	DiskThreshold float64  `yaml:"disk_threshold"`
	Cooldown      Duration `yaml:"cooldown"`
}

// defaultAlert returns the alerting defaults applied when the config file omits
// the alert section.
func defaultAlert() AlertConfig {
	return AlertConfig{
		OfflineAfter:  Duration(time.Minute),
		Sustain:       Duration(5 * time.Minute),
		CPUThreshold:  90,
		MemThreshold:  90,
		DiskThreshold: 90,
		Cooldown:      Duration(time.Hour),
	}
}

// Validate reports the first problem found in a.
func (a AlertConfig) Validate() error {
	if a.OfflineAfter.D() <= 0 {
		return fmt.Errorf("alert.offline_after must be > 0")
	}
	if a.Sustain.D() <= 0 {
		return fmt.Errorf("alert.sustain must be > 0")
	}
	if a.Cooldown.D() <= 0 {
		return fmt.Errorf("alert.cooldown must be > 0")
	}
	for _, t := range []struct {
		name  string
		value float64
	}{
		{"cpu_threshold", a.CPUThreshold},
		{"mem_threshold", a.MemThreshold},
		{"disk_threshold", a.DiskThreshold},
	} {
		if t.value < 0 || t.value > 100 {
			return fmt.Errorf("alert.%s must be within [0, 100], got %v", t.name, t.value)
		}
	}
	if a.Enabled {
		if a.WebhookURL == "" {
			return fmt.Errorf("alert.webhook_url is required when alert.enabled is true")
		}
		u, err := url.Parse(a.WebhookURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("alert.webhook_url must be an absolute http(s) URL, got %q", a.WebhookURL)
		}
	}
	return nil
}
