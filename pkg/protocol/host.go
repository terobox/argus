package protocol

import (
	"fmt"

	"github.com/terobox/argus/pkg/metrics"
)

// HostReport is a one-shot report of a host's mostly-static identifying
// information. The agent sends it on startup and refreshes it rarely.
type HostReport struct {
	Version  int              `json:"version"`
	NodeID   string           `json:"node_id"`
	AgentVer string           `json:"agent_version"`
	Info     metrics.HostInfo `json:"host_info"`
}

// Validate reports the first problem found in h.
func (h HostReport) Validate() error {
	if h.Version != Version {
		return fmt.Errorf("unsupported protocol version %d (want %d)", h.Version, Version)
	}
	if h.NodeID == "" {
		return fmt.Errorf("node_id is required")
	}
	if len(h.NodeID) > MaxNodeIDLen {
		return fmt.Errorf("node_id too long: %d > %d", len(h.NodeID), MaxNodeIDLen)
	}
	return h.Info.Validate()
}
