package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akshitanchan/execution-fairness-simulator/internal/domain"
	"github.com/akshitanchan/execution-fairness-simulator/internal/eventlog"
	"github.com/akshitanchan/execution-fairness-simulator/internal/latency"
	"github.com/akshitanchan/execution-fairness-simulator/internal/scenario"
	"github.com/akshitanchan/execution-fairness-simulator/internal/sim"
)

func TestComputeMetricsFromEventLog(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "events.jsonl")

	w, err := eventlog.NewWriter(logPath)
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}

	events := []*domain.Event{
		{
			Timestamp: 10,
			Type:      domain.EventOrderAccepted,
			Order: &domain.Order{
				ID:           101,
				TraderID:     "fast",
				Side:         domain.Buy,
				Type:         domain.LimitOrder,
				Price:        domain.FloatToPrice(100.00),
				Qty:          5,
				RemainingQty: 5,
				DecisionTime: 9,
				ArrivalTime:  10,
			},
		},
		{
			Timestamp: 20,
			Type:      domain.EventTradeExecuted,
			Trade: &domain.Trade{
				ID:          1,
				BuyOrderID:  101,
				SellOrderID: 5001,
				BuyTrader:   "fast",
				SellTrader:  "background",
				Price:       domain.FloatToPrice(100.00),
				Qty:         5,
				Timestamp:   20,
			},
		},
	}

	for _, event := range events {
		if err := w.Write(event); err != nil {
			t.Fatalf("write event: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	m, err := computeMetricsFromEventLog(logPath)
	if err != nil {
		t.Fatalf("compute metrics from event log: %v", err)
	}

	fast := m["fast"]
	if fast == nil {
		t.Fatal("missing fast trader metrics")
	}
	if fast.FillRate != 1.0 {
		t.Fatalf("expected order-level fill rate 1.0, got %f", fast.FillRate)
	}
}

func TestRunReplayVerifiesDeterministicMatch(t *testing.T) {
	cfg := scenario.DefaultCalm(777)
	cfg.Duration = latency.MsToNs(200)
	cfg.Scenario.SignalIntervalNs = latency.MsToNs(40)
	cfg.Scenario.OrderIntervalNs = latency.MsToNs(10)
	cfg.Scenario.MaxPriceLevels = 2
	cfg.Scenario.DepthPerLevel = 2

	baseDir := t.TempDir()
	runner, err := sim.NewRunner(cfg, baseDir)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	result, err := runner.Run()
	if err != nil {
		t.Fatalf("run simulation: %v", err)
	}

	output := captureStdout(t, func() {
		if err := runReplay([]string{"--run-dir", result.OutputDir}); err != nil {
			t.Fatalf("run replay: %v", err)
		}
	})

	if !strings.Contains(output, "Event log hash matches deterministic replay") {
		t.Fatalf("expected deterministic replay hash match output, got:\n%s", output)
	}
}

func TestRunReplayDetectsHashMismatch(t *testing.T) {
	cfg := scenario.DefaultCalm(123)
	cfg.Duration = latency.MsToNs(200)
	cfg.Scenario.SignalIntervalNs = latency.MsToNs(40)
	cfg.Scenario.OrderIntervalNs = latency.MsToNs(10)
	cfg.Scenario.MaxPriceLevels = 2
	cfg.Scenario.DepthPerLevel = 2

	baseDir := t.TempDir()
	runner, err := sim.NewRunner(cfg, baseDir)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	result, err := runner.Run()
	if err != nil {
		t.Fatalf("run simulation: %v", err)
	}

	origLogBytes, err := os.ReadFile(result.LogPath)
	if err != nil {
		t.Fatalf("read original log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(origLogBytes)), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines in log, got %d", len(lines))
	}
	mutatedLogPath := filepath.Join(t.TempDir(), "mutated-events.jsonl")
	mutated := strings.Join(lines[:len(lines)-1], "\n") + "\n"
	if err := os.WriteFile(mutatedLogPath, []byte(mutated), 0644); err != nil {
		t.Fatalf("write mutated log: %v", err)
	}

	output := captureStdout(t, func() {
		if err := runReplay([]string{"--run-dir", result.OutputDir, "--log", mutatedLogPath}); err != nil {
			t.Fatalf("run replay with mutated log: %v", err)
		}
	})

	if !strings.Contains(output, "Event log hash MISMATCH") {
		t.Fatalf("expected deterministic replay hash mismatch output, got:\n%s", output)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}

	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}
	return string(out)
}
