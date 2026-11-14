// Package metrics defines the metric types shared between the agent (which
// collects them) and the server (which stores and serves them), plus the
// validation rules that keep ingested data sane.
package metrics

import (
	"fmt"
	"math"
)

// Sample is a single point-in-time observation of a host's resources.
//
// Fast fields (CPU, Load, Mem, Swap, Net, DiskIO) are collected every tick
// (~1s). Slow fields (DiskCap, LoggedUsers) are collected on a slower cadence
// (~30s) and are nil in the ticks where they were not sampled, so their absence
// is explicit rather than implied by a zero value.
type Sample struct {
	// TS is the collection time in unix seconds, from the agent's clock.
	TS int64 `json:"ts"`

	CPU    CPU    `json:"cpu"`
	Load   Load   `json:"load"`
	Mem    Mem    `json:"mem"`
	Swap   Swap   `json:"swap"`
	Net    Net    `json:"net"`
	DiskIO DiskIO `json:"disk_io"`

	// Slow fields. Nil means "not collected in this tick".
	DiskCap     *DiskCapacity `json:"disk_cap,omitempty"`
	LoggedUsers *int          `json:"logged_users,omitempty"`
}

// CPU reports total (all-core) CPU utilization.
type CPU struct {
	// UsedPercent is overall CPU busy percentage, clamped to [0, 100].
	UsedPercent float64 `json:"used_percent"`
}

// Load reports the standard 1/5/15-minute load averages.
type Load struct {
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
}

// Mem reports main memory, in bytes.
//
// UsedBytes is derived as TotalBytes - AvailableBytes. AvailableBytes (from
// /proc/meminfo MemAvailable) is the value worth alerting on: it accounts for
// reclaimable page cache, unlike a naive "free" figure.
type Mem struct {
	TotalBytes     int64 `json:"total_bytes"`
	AvailableBytes int64 `json:"available_bytes"`
	UsedBytes      int64 `json:"used_bytes"`
}

// Swap reports swap space, in bytes.
type Swap struct {
	TotalBytes int64 `json:"total_bytes"`
	UsedBytes  int64 `json:"used_bytes"`
}

// Net reports cumulative byte counters for the primary network interface
// (the one carrying the default route), since boot.
type Net struct {
	RxBytes uint64 `json:"rx_bytes"`
	TxBytes uint64 `json:"tx_bytes"`
}

// DiskIO reports cumulative byte counters for the root filesystem's backing
// device, since boot.
type DiskIO struct {
	ReadBytes  uint64 `json:"read_bytes"`
	WriteBytes uint64 `json:"write_bytes"`
}

// DiskCapacity reports the capacity of the root filesystem, in bytes.
type DiskCapacity struct {
	TotalBytes int64 `json:"total_bytes"`
	UsedBytes  int64 `json:"used_bytes"`
}

// Validate reports the first problem found in s. It rejects clearly corrupt
// data (non-finite numbers, negative quantities, impossible timestamps) while
// leaving cross-field and range concerns that belong to the collector or the
// ingest path alone. A nil DiskCap or LoggedUsers is valid.
func (s Sample) Validate() error {
	if s.TS <= 0 {
		return fmt.Errorf("ts must be positive, got %d", s.TS)
	}
	if err := finite("cpu.used_percent", s.CPU.UsedPercent); err != nil {
		return err
	}
	if s.CPU.UsedPercent < 0 || s.CPU.UsedPercent > 100 {
		return fmt.Errorf("cpu.used_percent out of range [0,100]: %v", s.CPU.UsedPercent)
	}
	if err := nonNegativeFloat("load1", s.Load.Load1); err != nil {
		return err
	}
	if err := nonNegativeFloat("load5", s.Load.Load5); err != nil {
		return err
	}
	if err := nonNegativeFloat("load15", s.Load.Load15); err != nil {
		return err
	}
	if err := nonNegativeInt("mem.total_bytes", s.Mem.TotalBytes); err != nil {
		return err
	}
	if err := nonNegativeInt("mem.available_bytes", s.Mem.AvailableBytes); err != nil {
		return err
	}
	if err := nonNegativeInt("mem.used_bytes", s.Mem.UsedBytes); err != nil {
		return err
	}
	if s.Mem.TotalBytes > 0 && s.Mem.UsedBytes > s.Mem.TotalBytes {
		return fmt.Errorf("mem.used_bytes (%d) exceeds total (%d)", s.Mem.UsedBytes, s.Mem.TotalBytes)
	}
	if err := nonNegativeInt("swap.total_bytes", s.Swap.TotalBytes); err != nil {
		return err
	}
	if err := nonNegativeInt("swap.used_bytes", s.Swap.UsedBytes); err != nil {
		return err
	}
	if s.Swap.TotalBytes > 0 && s.Swap.UsedBytes > s.Swap.TotalBytes {
		return fmt.Errorf("swap.used_bytes (%d) exceeds total (%d)", s.Swap.UsedBytes, s.Swap.TotalBytes)
	}
	if s.DiskCap != nil {
		if s.DiskCap.TotalBytes <= 0 {
			return fmt.Errorf("disk_cap.total_bytes must be positive, got %d", s.DiskCap.TotalBytes)
		}
		if err := nonNegativeInt("disk_cap.used_bytes", s.DiskCap.UsedBytes); err != nil {
			return err
		}
		if s.DiskCap.UsedBytes > s.DiskCap.TotalBytes {
			return fmt.Errorf("disk_cap.used_bytes (%d) exceeds total (%d)", s.DiskCap.UsedBytes, s.DiskCap.TotalBytes)
		}
	}
	if s.LoggedUsers != nil && *s.LoggedUsers < 0 {
		return fmt.Errorf("logged_users must be >= 0, got %d", *s.LoggedUsers)
	}
	return nil
}

// finite reports an error when v is NaN or +/-Inf.
func finite(name string, v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Errorf("%s must be finite, got %v", name, v)
	}
	return nil
}

// nonNegativeFloat reports an error when v is not finite or is negative.
func nonNegativeFloat(name string, v float64) error {
	if err := finite(name, v); err != nil {
		return err
	}
	if v < 0 {
		return fmt.Errorf("%s must be >= 0, got %v", name, v)
	}
	return nil
}

func nonNegativeInt(name string, v int64) error {
	if v < 0 {
		return fmt.Errorf("%s must be >= 0, got %d", name, v)
	}
	return nil
}
