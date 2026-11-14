// Package alert evaluates node health against configured rules and dispatches
// notifications (ntfy webhooks) on transitions and, while a rule stays firing,
// at most once per cooldown. The on-disk YAML form of the configuration lives in
// pkg/config; cmd/server converts it into the plain runtime Config here, which
// keeps this package testable without a config file.
package alert

import "time"

// Config holds the runtime alerting configuration. All durations are plain
// time.Duration values; percentages are in [0, 100].
type Config struct {
	Enabled       bool
	WebhookURL    string
	OfflineAfter  time.Duration // how long without a report before "offline"
	Sustain       time.Duration // window averaged for the resource rules
	CPUThreshold  float64       // percent
	MemThreshold  float64       // percent
	DiskThreshold float64       // percent
	Cooldown      time.Duration // minimum gap between re-fires while firing
}
