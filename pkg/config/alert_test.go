package config

import (
	"testing"
	"time"
)

func TestAlertDefaults(t *testing.T) {
	a := DefaultServer().Alert
	if a.Enabled {
		t.Fatal("alerting should default to disabled")
	}
	if a.OfflineAfter.D() != time.Minute {
		t.Errorf("offline_after = %v, want 1m", a.OfflineAfter.D())
	}
	if a.Sustain.D() != 5*time.Minute {
		t.Errorf("sustain = %v, want 5m", a.Sustain.D())
	}
	if a.CPUThreshold != 90 || a.MemThreshold != 90 || a.DiskThreshold != 90 {
		t.Errorf("thresholds = %v/%v/%v, want 90/90/90", a.CPUThreshold, a.MemThreshold, a.DiskThreshold)
	}
	if a.Cooldown.D() != time.Hour {
		t.Errorf("cooldown = %v, want 1h", a.Cooldown.D())
	}
}

func TestLoadServerAlert(t *testing.T) {
	path := writeConfig(t, "alert:\n  enabled: true\n  webhook_url: https://ntfy.sh/topic\n  offline_after: 30s\n  cpu_threshold: 95\n")
	cfg, err := LoadServer(path)
	if err != nil {
		t.Fatalf("LoadServer: %v", err)
	}
	a := cfg.Alert
	if !a.Enabled {
		t.Error("enabled = false, want true")
	}
	if a.WebhookURL != "https://ntfy.sh/topic" {
		t.Errorf("webhook_url = %q", a.WebhookURL)
	}
	if a.OfflineAfter.D() != 30*time.Second {
		t.Errorf("offline_after = %v, want 30s", a.OfflineAfter.D())
	}
	if a.CPUThreshold != 95 {
		t.Errorf("cpu_threshold = %v, want 95", a.CPUThreshold)
	}
	if a.MemThreshold != 90 {
		t.Errorf("mem_threshold = %v, want 90 (default preserved)", a.MemThreshold)
	}
}

func TestAlertValidate(t *testing.T) {
	base := DefaultServer()

	tests := []struct {
		name   string
		mutate func(*AlertConfig)
	}{
		{"enabled without webhook", func(a *AlertConfig) { a.Enabled = true }},
		{"enabled with non-url", func(a *AlertConfig) { a.Enabled = true; a.WebhookURL = "not-a-url" }},
		{"enabled with relative url", func(a *AlertConfig) { a.Enabled = true; a.WebhookURL = "/topic" }},
		{"negative cpu threshold", func(a *AlertConfig) { a.CPUThreshold = -1 }},
		{"cpu threshold over 100", func(a *AlertConfig) { a.CPUThreshold = 101 }},
		{"zero offline after", func(a *AlertConfig) { a.OfflineAfter = 0 }},
		{"zero cooldown", func(a *AlertConfig) { a.Cooldown = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := base
			tt.mutate(&c.Alert)
			if err := c.Validate(); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}

	c := base
	c.Alert.Enabled = true
	c.Alert.WebhookURL = "https://ntfy.sh/topic"
	if err := c.Validate(); err != nil {
		t.Fatalf("valid alert config rejected: %v", err)
	}
}
