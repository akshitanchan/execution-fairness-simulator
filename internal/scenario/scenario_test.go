package scenario

import (
	"testing"

	"github.com/akshitanchan/execution-fairness-simulator/internal/domain"
)

func TestCalmGeneratorReproducibility(t *testing.T) {
	cfg := DefaultCalm(42)
	g1 := NewCalmGenerator(cfg)
	events1 := g1.Generate()

	cfg2 := DefaultCalm(42)
	g2 := NewCalmGenerator(cfg2)
	events2 := g2.Generate()

	if len(events1) != len(events2) {
		t.Fatalf("different event counts: %d vs %d", len(events1), len(events2))
	}

	for i := range events1 {
		if events1[i].Timestamp != events2[i].Timestamp {
			t.Fatalf("event %d: different timestamps %d vs %d",
				i, events1[i].Timestamp, events2[i].Timestamp)
		}
	}
}

func TestGeneratorsProduceNonEmptyFlow(t *testing.T) {
	for _, name := range []string{"calm", "thin", "spike"} {
		cfg := GetConfig(name, 123)
		gen := NewGenerator(cfg)
		events := gen.Generate()

		if len(events) < 100 {
			t.Errorf("%s: expected >100 events, got %d", name, len(events))
		}

		// Check we have both buy and sell orders.
		var buys, sells int
		for _, e := range events {
			if e.Order != nil && e.Order.Type != domain.CancelOrder {
				if e.Order.Side == domain.Buy {
					buys++
				} else {
					sells++
				}
			}
		}
		if buys == 0 {
			t.Errorf("%s: no buy orders generated", name)
		}
		if sells == 0 {
			t.Errorf("%s: no sell orders generated", name)
		}

		// Check signals exist.
		var signals int
		for _, e := range events {
			if e.Type == domain.EventSignal {
				signals++
			}
		}
		if signals == 0 {
			t.Errorf("%s: no signals generated", name)
		}
	}
}

func TestGeneratorsTimestampOrdering(t *testing.T) {
	for _, name := range []string{"calm", "thin", "spike"} {
		cfg := GetConfig(name, 42)
		gen := NewGenerator(cfg)
		events := gen.Generate()

		for i := 1; i < len(events); i++ {
			if events[i].Timestamp < events[i-1].Timestamp {
				t.Fatalf("%s: events not sorted at index %d: %d < %d",
					name, i, events[i].Timestamp, events[i-1].Timestamp)
			}
		}
	}
}

func TestSpikeGeneratorHasBurstPeriods(t *testing.T) {
	cfg := DefaultSpike(42)
	gen := NewSpikeGenerator(cfg)
	events := gen.Generate()

	// Count events in burst windows vs outside.
	p := cfg.Scenario
	inBurst := func(ts int64) bool {
		for t := p.BurstIntervalNs; t < cfg.Duration; t += p.BurstIntervalNs {
			if ts >= t && ts < t+p.BurstWindowNs {
				return true
			}
		}
		return false
	}

	var burstCount, normalCount int
	for _, e := range events {
		if e.Timestamp == 0 {
			continue // skip initial book
		}
		if inBurst(e.Timestamp) {
			burstCount++
		} else {
			normalCount++
		}
	}

	// Ensure both burst and non-burst windows contain events.
	if burstCount == 0 {
		t.Error("no events in burst windows")
	}
	if normalCount == 0 {
		t.Error("no events outside burst windows")
	}
}
