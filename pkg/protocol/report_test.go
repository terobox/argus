package protocol

import (
	"testing"

	"github.com/terobox/argus/pkg/metrics"
)

func sample(ts int64) metrics.Sample {
	return metrics.Sample{TS: ts, CPU: metrics.CPU{UsedPercent: 10}}
}

func validReport() Report {
	return Report{
		Version:     Version,
		NodeID:      "node-abc123",
		AgentVer:    "0.1.0",
		CollectedAt: 1700000015,
		Samples:     []metrics.Sample{sample(1700000001), sample(1700000002)},
	}
}

func TestReportValidate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		if err := validReport().Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty samples is valid", func(t *testing.T) {
		r := validReport()
		r.Samples = nil
		if err := r.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	tests := []struct {
		name   string
		mutate func(*Report)
	}{
		{"wrong version", func(r *Report) { r.Version = 2 }},
		{"empty node id", func(r *Report) { r.NodeID = "" }},
		{"zero collected at", func(r *Report) { r.CollectedAt = 0 }},
		{"out of order samples", func(r *Report) { r.Samples[0], r.Samples[1] = r.Samples[1], r.Samples[0] }},
		{"duplicate timestamps", func(r *Report) { r.Samples[1].TS = r.Samples[0].TS }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := validReport()
			tt.mutate(&r)
			if err := r.Validate(); err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}
