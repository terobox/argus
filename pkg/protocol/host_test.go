package protocol

import (
	"strings"
	"testing"

	"github.com/terobox/argus/pkg/metrics"
)

func TestHostReportValidate(t *testing.T) {
	base := HostReport{
		Version:  Version,
		NodeID:   "n1",
		AgentVer: "dev",
		Info:     metrics.HostInfo{Hostname: "web-1", OS: "linux", AgentVer: "dev"},
	}

	cases := []struct {
		name    string
		mutate  func(*HostReport)
		wantErr bool
	}{
		{"ok", func(*HostReport) {}, false},
		{"bad version", func(h *HostReport) { h.Version = 0 }, true},
		{"missing node id", func(h *HostReport) { h.NodeID = "" }, true},
		{"too-long node id", func(h *HostReport) { h.NodeID = strings.Repeat("a", MaxNodeIDLen+1) }, true},
		{"missing hostname", func(h *HostReport) { h.Info.Hostname = "" }, true},
		{"missing agent ver", func(h *HostReport) { h.Info.AgentVer = "" }, true},
		{"negative cores", func(h *HostReport) { h.Info.CPUCores = -1 }, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := base
			c.mutate(&h)
			err := h.Validate()
			if (err != nil) != c.wantErr {
				t.Fatalf("Validate() = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}
