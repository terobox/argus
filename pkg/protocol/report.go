// Package protocol defines the versioned wire format the agent uses to report
// metrics to the server.
package protocol

import (
	"fmt"

	"github.com/terobox/argus/pkg/metrics"
)

// Version is the current report protocol version. The server rejects reports
// with a different version so the wire format can evolve without breaking
// running agents.
const Version = 1

// MaxNodeIDLen bounds the node identifier length accepted in a report.
const MaxNodeIDLen = 128

// Report is one batch of samples pushed by an agent, typically a ~15s window.
type Report struct {
	Version     int              `json:"version"`
	NodeID      string           `json:"node_id"`
	AgentVer    string           `json:"agent_version"`
	CollectedAt int64            `json:"collected_at"` // agent clock, unix seconds
	Samples     []metrics.Sample `json:"samples"`
}

// Validate reports the first problem found in r. Samples must arrive in
// strictly increasing timestamp order within a batch; duplicates and
// out-of-order points indicate an agent bug and are rejected here rather than
// silently corrupting a curve.
func (r Report) Validate() error {
	if r.Version != Version {
		return fmt.Errorf("unsupported protocol version %d (want %d)", r.Version, Version)
	}
	if r.NodeID == "" {
		return fmt.Errorf("node_id is required")
	}
	if len(r.NodeID) > MaxNodeIDLen {
		return fmt.Errorf("node_id too long: %d > %d", len(r.NodeID), MaxNodeIDLen)
	}
	if r.CollectedAt <= 0 {
		return fmt.Errorf("collected_at must be positive, got %d", r.CollectedAt)
	}
	prev := int64(0)
	for i, s := range r.Samples {
		if err := s.Validate(); err != nil {
			return fmt.Errorf("sample %d: %w", i, err)
		}
		if s.TS <= prev {
			return fmt.Errorf("sample %d: ts must be strictly increasing, got %d after %d", i, s.TS, prev)
		}
		prev = s.TS
	}
	return nil
}
