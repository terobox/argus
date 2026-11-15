//go:build !linux

package collect

import (
	"errors"
	"time"

	"github.com/terobox/argus/pkg/metrics"
)

// Sampler is a no-op on non-Linux platforms. It exists so the agent compiles
// everywhere; the real implementation lives in sampler_linux.go.
type Sampler struct{ agentVer string }

// NewSampler returns a non-functional Sampler on non-Linux platforms.
func NewSampler(agentVer string) *Sampler { return &Sampler{agentVer: agentVer} }

// Sample always fails on non-Linux.
func (s *Sampler) Sample(slowEvery time.Duration) (metrics.Sample, error) {
	return metrics.Sample{}, errors.New("collect: sampling requires linux")
}

// HostInfo always fails on non-Linux.
func (s *Sampler) HostInfo(publicIP string) (metrics.HostInfo, error) {
	return metrics.HostInfo{}, errors.New("collect: host info requires linux")
}
