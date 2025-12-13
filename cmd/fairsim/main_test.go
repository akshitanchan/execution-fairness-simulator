package main

import (
	"path/filepath"
	"testing"

	"github.com/akshitanchan/execution-fairness-simulator/internal/domain"
	"github.com/akshitanchan/execution-fairness-simulator/internal/eventlog"
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
