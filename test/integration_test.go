package test

import (
	"math"
	"testing"

	"github.com/akshitanchan/execution-fairness-simulator/internal/metrics"
	"github.com/akshitanchan/execution-fairness-simulator/internal/scenario"
	"github.com/akshitanchan/execution-fairness-simulator/internal/sim"
)

// TestIntegrationAllScenarios runs all scenarios end-to-end and checks
// that the simulation produces meaningful results.
func TestIntegrationAllScenarios(t *testing.T) {
	for _, name := range []string{"calm", "thin", "spike"} {
		t.Run(name, func(t *testing.T) {
			cfg := scenario.GetConfig(name, 42)
			dir := t.TempDir()

			runner, err := sim.NewRunner(cfg, dir)
			if err != nil {
				t.Fatal(err)
			}

			result, err := runner.Run()
			if err != nil {
				t.Fatal(err)
			}

			// Must have processed events.
			if result.EventCount == 0 {
				t.Error("no events processed")
			}

			// Must have trades.
			if result.TradeCount == 0 {
				t.Error("no trades")
			}

			// Must be able to compute metrics.
			m, err := metrics.ComputeFromLog(result.LogPath)
			if err != nil {
				t.Fatal(err)
			}

			// Both traders must have metrics.
			fast, ok := m["fast"]
			if !ok {
				t.Fatal("no metrics for fast trader")
			}
			slow, ok := m["slow"]
			if !ok {
				t.Fatal("no metrics for slow trader")
			}

			// Both must have sent orders.
			if fast.OrdersSent == 0 {
				t.Error("fast trader sent no orders")
			}
			if slow.OrdersSent == 0 {
				t.Error("slow trader sent no orders")
			}

			// Both must have fills.
			if fast.TotalFills == 0 {
				t.Error("fast trader has no fills")
			}
			if slow.TotalFills == 0 {
				t.Error("slow trader has no fills")
			}

			t.Logf("  Events: %d, Trades: %d", result.EventCount, result.TradeCount)
			t.Logf("  Fast fills: %d (rate %.1f%%), Slow fills: %d (rate %.1f%%)",
				fast.TotalFills, fast.FillRate*100,
				slow.TotalFills, slow.FillRate*100)
			t.Logf("  Fast TTF: %.2f ms, Slow TTF: %.2f ms",
				fast.AvgTimeToFillNs, slow.AvgTimeToFillNs)
		})
	}
}

// TestLatencyImpactEvidence verifies the spec requirement that latency
// changes produce measurable outcome differences.
func TestLatencyImpactEvidence(t *testing.T) {
	measurableDiffs := 0

	for _, name := range []string{"calm", "thin", "spike"} {
		t.Run(name, func(t *testing.T) {
			cfg := scenario.GetConfig(name, 42)
			dir := t.TempDir()

			runner, err := sim.NewRunner(cfg, dir)
			if err != nil {
				t.Fatal(err)
			}

			result, err := runner.Run()
			if err != nil {
				t.Fatal(err)
			}

			m, err := metrics.ComputeFromLog(result.LogPath)
			if err != nil {
				t.Fatal(err)
			}

			fast := m["fast"]
			slow := m["slow"]
			if fast == nil || slow == nil {
				t.Fatal("missing trader metrics")
			}

			fillRateDeltaPP := (fast.FillRate - slow.FillRate) * 100
			slippageDeltaBps := fast.SlippageBps - slow.SlippageBps

			t.Logf("  Fill-rate delta: %.2f pp (fast - slow)", fillRateDeltaPP)
			t.Logf("  Slippage delta: %.2f bps (fast - slow)", slippageDeltaBps)

			// Moderate materiality threshold:
			// |fill-rate delta| >= 5 pp OR |slippage delta| >= 0.5 bps.
			if math.Abs(fillRateDeltaPP) >= 5 || math.Abs(slippageDeltaBps) >= 0.5 {
				measurableDiffs++
			}
		})
	}

	if measurableDiffs < 2 {
		t.Errorf("expected measurable latency impact in at least 2 scenarios, got %d", measurableDiffs)
	}
}
