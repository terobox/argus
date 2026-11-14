package metrics

import "fmt"

// HostInfo is the mostly-static identifying information about a host. The agent
// sends it on registration and periodically refreshes it (rarely).
type HostInfo struct {
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Kernel   string `json:"kernel"`
	Arch     string `json:"arch"`
	CPUModel string `json:"cpu_model"`
	CPUCores int    `json:"cpu_cores"`
	MemTotal int64  `json:"mem_total"`
	// PublicIP is optional and, when set, is configured manually on the agent;
	// it is never probed automatically, for privacy reasons.
	PublicIP string `json:"public_ip,omitempty"`
	AgentVer string `json:"agent_version"`
}

// Validate reports the first problem found in h.
func (h HostInfo) Validate() error {
	if h.Hostname == "" {
		return fmt.Errorf("hostname is required")
	}
	if h.AgentVer == "" {
		return fmt.Errorf("agent_version is required")
	}
	if h.CPUCores < 0 {
		return fmt.Errorf("cpu_cores must be >= 0, got %d", h.CPUCores)
	}
	if h.MemTotal < 0 {
		return fmt.Errorf("mem_total must be >= 0, got %d", h.MemTotal)
	}
	return nil
}
