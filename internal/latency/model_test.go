package latency

import (
	"testing"
)

func TestModelDeterminism(t *testing.T) {
	m1 := NewModel(MsToNs(5), MsToNs(2), 42)
	m2 := NewModel(MsToNs(5), MsToNs(2), 42)

	for i := 0; i < 1000; i++ {
		decision := int64(i) * MsToNs(10)
		a1 := m1.Apply(decision)
		a2 := m2.Apply(decision)
		if a1 != a2 {
			t.Fatalf("non-deterministic at iteration %d: %d != %d", i, a1, a2)
		}
	}
}

func TestModelBaseLatency(t *testing.T) {
	m := NewModel(MsToNs(10), 0, 42) // no jitter

	for i := 0; i < 100; i++ {
		decision := int64(i) * MsToNs(1)
		arrival := m.Apply(decision)
		expected := decision + MsToNs(10)
		if arrival != expected {
			t.Fatalf("expected %d, got %d", expected, arrival)
		}
	}
}

func TestModelJitterBounds(t *testing.T) {
	base := MsToNs(5)
	jitter := MsToNs(3)
	m := NewModel(base, jitter, 99)

	for i := 0; i < 10000; i++ {
		decision := int64(0)
		arrival := m.Apply(decision)
		delay := arrival - decision

		if delay < base {
			t.Fatalf("delay %d < base %d", delay, base)
		}
		if delay >= base+jitter {
			t.Fatalf("delay %d >= base+jitter %d", delay, base+jitter)
		}
	}
}

func TestMsToNs(t *testing.T) {
	if MsToNs(1) != 1_000_000 {
		t.Errorf("MsToNs(1) = %d, want 1000000", MsToNs(1))
	}
	if MsToNs(50) != 50_000_000 {
		t.Errorf("MsToNs(50) = %d, want 50000000", MsToNs(50))
	}
}
