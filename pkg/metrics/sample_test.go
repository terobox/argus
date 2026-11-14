package metrics

import (
	"math"
	"testing"
)

func intPtr(v int) *int { return &v }

func validSample() Sample {
	return Sample{
		TS:          1700000000,
		CPU:         CPU{UsedPercent: 42.5},
		Load:        Load{Load1: 0.5, Load5: 0.4, Load15: 0.3},
		Mem:         Mem{TotalBytes: 16 << 30, AvailableBytes: 8 << 30, UsedBytes: 8 << 30},
		Swap:        Swap{TotalBytes: 4 << 30, UsedBytes: 1 << 30},
		Net:         Net{RxBytes: 1000, TxBytes: 2000},
		DiskIO:      DiskIO{ReadBytes: 500, WriteBytes: 600},
		DiskCap:     &DiskCapacity{TotalBytes: 100 << 30, UsedBytes: 40 << 30},
		LoggedUsers: intPtr(3),
	}
}

func TestSampleValidate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		if err := validSample().Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("nil slow fields are valid", func(t *testing.T) {
		s := validSample()
		s.DiskCap = nil
		s.LoggedUsers = nil
		if err := s.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	tests := []struct {
		name   string
		mutate func(*Sample)
	}{
		{"zero ts", func(s *Sample) { s.TS = 0 }},
		{"negative cpu", func(s *Sample) { s.CPU.UsedPercent = -0.1 }},
		{"cpu above 100", func(s *Sample) { s.CPU.UsedPercent = 100.5 }},
		{"nan cpu", func(s *Sample) { s.CPU.UsedPercent = math.NaN() }},
		{"inf cpu", func(s *Sample) { s.CPU.UsedPercent = math.Inf(1) }},
		{"negative load1", func(s *Sample) { s.Load.Load1 = -1 }},
		{"nan load5", func(s *Sample) { s.Load.Load5 = math.NaN() }},
		{"negative mem total", func(s *Sample) { s.Mem.TotalBytes = -1 }},
		{"negative mem available", func(s *Sample) { s.Mem.AvailableBytes = -1 }},
		{"mem used exceeds total", func(s *Sample) { s.Mem.UsedBytes = s.Mem.TotalBytes + 1 }},
		{"negative swap used", func(s *Sample) { s.Swap.UsedBytes = -1 }},
		{"swap used exceeds total", func(s *Sample) { s.Swap.UsedBytes = s.Swap.TotalBytes + 1 }},
		{"zero disk total", func(s *Sample) { s.DiskCap.TotalBytes = 0 }},
		{"negative disk used", func(s *Sample) { s.DiskCap.UsedBytes = -1 }},
		{"disk used exceeds total", func(s *Sample) { s.DiskCap.UsedBytes = s.DiskCap.TotalBytes + 1 }},
		{"negative logged users", func(s *Sample) { s.LoggedUsers = intPtr(-1) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSample()
			tt.mutate(&s)
			if err := s.Validate(); err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}
