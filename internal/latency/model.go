// Package latency implements the latency + jitter model
package latency

import (
	"math/rand"
)

type Model struct {
	BaseNs int64 // base latency in nanoseconds
	JitterNs int64 // max jitter in nanoseconds (uniform [0, JitterNs))
	rng    *rand.Rand
}

func NewModel(baseNs, jitterNs int64, seed int64) *Model {
	return &Model{
		BaseNs:   baseNs,
		JitterNs: jitterNs,
		rng:      rand.New(rand.NewSource(seed)),
	}
}

func (m *Model) Apply(decisionTime int64) int64 {
	jitter := int64(0)
	if m.JitterNs > 0 {
		jitter = m.rng.Int63n(m.JitterNs)
	}
	return decisionTime + m.BaseNs + jitter
}

func MsToNs(ms int64) int64 {
	return ms * 1_000_000
}
