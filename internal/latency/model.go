// Package latency implements the configurable latency + jitter model.
package latency

import (
	"math/rand"
)

// Model applies deterministic latency + jitter to messages.
type Model struct {
	BaseNs int64 // base latency in nanoseconds
	JitterNs int64 // max jitter in nanoseconds (uniform [0, JitterNs))
	rng    *rand.Rand
}

// NewModel creates a latency model with the given parameters and seed.
func NewModel(baseNs, jitterNs int64, seed int64) *Model {
	return &Model{
		BaseNs:   baseNs,
		JitterNs: jitterNs,
		rng:      rand.New(rand.NewSource(seed)),
	}
}

// Apply returns the arrival time given a decision time.
func (m *Model) Apply(decisionTime int64) int64 {
	jitter := int64(0)
	if m.JitterNs > 0 {
		jitter = m.rng.Int63n(m.JitterNs)
	}
	return decisionTime + m.BaseNs + jitter
}

// MsToNs converts milliseconds to nanoseconds.
func MsToNs(ms int64) int64 {
	return ms * 1_000_000
}
