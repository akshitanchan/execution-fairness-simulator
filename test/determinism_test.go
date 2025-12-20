package test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/akshitanchan/execution-fairness-simulator/internal/metrics"
	"github.com/akshitanchan/execution-fairness-simulator/internal/report"
	"github.com/akshitanchan/execution-fairness-simulator/internal/scenario"
	"github.com/akshitanchan/execution-fairness-simulator/internal/sim"
)

// TestDeterminism verifies that the same seed + config produces
// identical event logs, metrics, and reports across two runs
func TestDeterminism(t *testing.T) {
	for _, name := range []string{"calm", "thin", "spike"} {
		t.Run(name, func(t *testing.T) {
			seed := int64(12345)

			// First run
			cfg1 := scenario.GetConfig(name, seed)
			dir1 := t.TempDir()
			runner1, err := sim.NewRunner(cfg1, dir1)
			if err != nil {
				t.Fatal(err)
			}
			result1, err := runner1.Run()
			if err != nil {
				t.Fatal(err)
			}

			// Generate report for run 1
			m1, err := metrics.ComputeFromLog(result1.LogPath)
			if err != nil {
				t.Fatal(err)
			}
			rpt1 := report.NewReport(cfg1, m1, result1.OutputDir)
			if err := rpt1.Generate(); err != nil {
				t.Fatalf("report gen run1: %v", err)
			}

			// Second run with identical config
			cfg2 := scenario.GetConfig(name, seed)
			dir2 := t.TempDir()
			runner2, err := sim.NewRunner(cfg2, dir2)
			if err != nil {
				t.Fatal(err)
			}
			result2, err := runner2.Run()
			if err != nil {
				t.Fatal(err)
			}

			// Generate report for run 2
			m2, err := metrics.ComputeFromLog(result2.LogPath)
			if err != nil {
				t.Fatal(err)
			}
			rpt2 := report.NewReport(cfg2, m2, result2.OutputDir)
			if err := rpt2.Generate(); err != nil {
				t.Fatalf("report gen run2: %v", err)
			}

			// Event counts must match
			if result1.EventCount != result2.EventCount {
				t.Errorf("event count mismatch: %d vs %d",
					result1.EventCount, result2.EventCount)
			}

			// Trade counts must match
			if result1.TradeCount != result2.TradeCount {
				t.Errorf("trade count mismatch: %d vs %d",
					result1.TradeCount, result2.TradeCount)
			}

			// Log hashes must match
			hash1 := hashFileT(t, result1.LogPath)
			hash2 := hashFileT(t, result2.LogPath)
			if hash1 != hash2 {
				t.Errorf("log hash mismatch:\n  run1: %s\n  run2: %s", hash1, hash2)
			}

			// Report files must be identical
			reportHash1 := hashFileT(t, filepath.Join(result1.OutputDir, "report.md"))
			reportHash2 := hashFileT(t, filepath.Join(result2.OutputDir, "report.md"))
			if reportHash1 != reportHash2 {
				t.Errorf("report.md hash mismatch:\n  run1: %s\n  run2: %s", reportHash1, reportHash2)
			}

			metricsHash1 := hashFileT(t, filepath.Join(result1.OutputDir, "metrics.json"))
			metricsHash2 := hashFileT(t, filepath.Join(result2.OutputDir, "metrics.json"))
			if metricsHash1 != metricsHash2 {
				t.Errorf("metrics.json hash mismatch:\n  run1: %s\n  run2: %s", metricsHash1, metricsHash2)
			}

			// Metrics must be identical
			for _, traderID := range []string{"fast", "slow"} {
				tm1, ok1 := m1[traderID]
				tm2, ok2 := m2[traderID]
				if ok1 != ok2 {
					t.Errorf("%s: trader presence mismatch", traderID)
					continue
				}
				if !ok1 {
					continue
				}

				if tm1.TotalFills != tm2.TotalFills {
					t.Errorf("%s fills: %d vs %d", traderID, tm1.TotalFills, tm2.TotalFills)
				}
				if tm1.TotalQtyFilled != tm2.TotalQtyFilled {
					t.Errorf("%s qty: %d vs %d", traderID, tm1.TotalQtyFilled, tm2.TotalQtyFilled)
				}
				if tm1.AvgExecPrice != tm2.AvgExecPrice {
					t.Errorf("%s avg price: %f vs %f", traderID, tm1.AvgExecPrice, tm2.AvgExecPrice)
				}
				if tm1.AvgSlippage != tm2.AvgSlippage {
					t.Errorf("%s slippage: %f vs %f", traderID, tm1.AvgSlippage, tm2.AvgSlippage)
				}
				if tm1.AvgQueuePosPlace != tm2.AvgQueuePosPlace {
					t.Errorf("%s queue pos place: %f vs %f", traderID, tm1.AvgQueuePosPlace, tm2.AvgQueuePosPlace)
				}
				if tm1.AvgQueuePosFill != tm2.AvgQueuePosFill {
					t.Errorf("%s queue pos fill: %f vs %f", traderID, tm1.AvgQueuePosFill, tm2.AvgQueuePosFill)
				}
			}
		})
	}
}

func hashFileT(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}
