package metrics

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/akshitanchan/execution-fairness-simulator/internal/domain"
	"github.com/akshitanchan/execution-fairness-simulator/internal/eventlog"
)

func TestFillRateCountsFilledOrderOnceWithPartialFills(t *testing.T) {
	events := []*domain.Event{
		{
			Timestamp: 100,
			Type:      domain.EventOrderAccepted,
			Order: &domain.Order{
				ID:           1,
				TraderID:     "fast",
				Side:         domain.Buy,
				Type:         domain.LimitOrder,
				Price:        domain.FloatToPrice(100.00),
				Qty:          10,
				RemainingQty: 10,
				DecisionTime: 90,
				ArrivalTime:  100,
			},
		},
		{
			Timestamp: 110,
			Type:      domain.EventTradeExecuted,
			Trade: &domain.Trade{
				ID:          1,
				BuyOrderID:  1,
				SellOrderID: 5001,
				BuyTrader:   "fast",
				SellTrader:  "background",
				Price:       domain.FloatToPrice(100.00),
				Qty:         4,
				Timestamp:   110,
			},
		},
		{
			Timestamp: 120,
			Type:      domain.EventTradeExecuted,
			Trade: &domain.Trade{
				ID:          2,
				BuyOrderID:  1,
				SellOrderID: 5002,
				BuyTrader:   "fast",
				SellTrader:  "background",
				Price:       domain.FloatToPrice(100.01),
				Qty:         6,
				Timestamp:   120,
			},
		},
	}

	m := ComputeFromEvents(events)
	fast := m["fast"]
	if fast == nil {
		t.Fatal("missing fast trader metrics")
	}

	if fast.TotalFills != 2 {
		t.Fatalf("expected 2 fills, got %d", fast.TotalFills)
	}
	if fast.FillRate != 1.0 {
		t.Fatalf("expected fill rate 1.0, got %f", fast.FillRate)
	}
}

func TestFillRateNeverExceedsOne(t *testing.T) {
	events := []*domain.Event{
		{
			Timestamp: 100,
			Type:      domain.EventOrderAccepted,
			Order: &domain.Order{
				ID:           1,
				TraderID:     "fast",
				Side:         domain.Buy,
				Type:         domain.LimitOrder,
				Price:        domain.FloatToPrice(100.00),
				Qty:          10,
				RemainingQty: 10,
				DecisionTime: 90,
				ArrivalTime:  100,
			},
		},
		{
			Timestamp: 101,
			Type:      domain.EventOrderAccepted,
			Order: &domain.Order{
				ID:           2,
				TraderID:     "fast",
				Side:         domain.Sell,
				Type:         domain.LimitOrder,
				Price:        domain.FloatToPrice(100.02),
				Qty:          10,
				RemainingQty: 10,
				DecisionTime: 91,
				ArrivalTime:  101,
			},
		},
		{
			Timestamp: 110,
			Type:      domain.EventTradeExecuted,
			Trade: &domain.Trade{
				ID:          1,
				BuyOrderID:  1,
				SellOrderID: 5001,
				BuyTrader:   "fast",
				SellTrader:  "background",
				Price:       domain.FloatToPrice(100.00),
				Qty:         4,
				Timestamp:   110,
			},
		},
		{
			Timestamp: 120,
			Type:      domain.EventTradeExecuted,
			Trade: &domain.Trade{
				ID:          2,
				BuyOrderID:  1,
				SellOrderID: 5002,
				BuyTrader:   "fast",
				SellTrader:  "background",
				Price:       domain.FloatToPrice(100.01),
				Qty:         6,
				Timestamp:   120,
			},
		},
	}

	m := ComputeFromEvents(events)
	fast := m["fast"]
	if fast == nil {
		t.Fatal("missing fast trader metrics")
	}

	if fast.FillRate > 1.0 {
		t.Fatalf("fill rate exceeded 1.0: %f", fast.FillRate)
	}
	if fast.FillRate != 0.5 {
		t.Fatalf("expected fill rate 0.5, got %f", fast.FillRate)
	}
}

func TestComputeFromEventsMatchesComputeFromLog(t *testing.T) {
	events := []*domain.Event{
		{
			Timestamp: 50,
			Type:      domain.EventBBOUpdate,
			BBO: &domain.BBO{
				BidPrice: domain.FloatToPrice(99.99),
				BidQty:   20,
				AskPrice: domain.FloatToPrice(100.01),
				AskQty:   20,
				MidPrice: domain.FloatToPrice(100.00),
			},
		},
		{
			Timestamp: 100,
			Type:      domain.EventOrderAccepted,
			Order: &domain.Order{
				ID:           10,
				TraderID:     "fast",
				Side:         domain.Buy,
				Type:         domain.LimitOrder,
				Price:        domain.FloatToPrice(100.00),
				Qty:          5,
				RemainingQty: 5,
				DecisionTime: 90,
				ArrivalTime:  100,
				QueuePos:     2,
			},
		},
		{
			Timestamp: 101,
			Type:      domain.EventOrderAccepted,
			Order: &domain.Order{
				ID:           20,
				TraderID:     "slow",
				Side:         domain.Sell,
				Type:         domain.LimitOrder,
				Price:        domain.FloatToPrice(100.02),
				Qty:          5,
				RemainingQty: 5,
				DecisionTime: 91,
				ArrivalTime:  101,
				QueuePos:     3,
			},
		},
		{
			Timestamp: 120,
			Type:      domain.EventTradeExecuted,
			Trade: &domain.Trade{
				ID:              1,
				BuyOrderID:      10,
				SellOrderID:     7001,
				BuyTrader:       "fast",
				SellTrader:      "background",
				Price:           domain.FloatToPrice(100.00),
				Qty:             5,
				Timestamp:       120,
				RestingQueuePos: 1,
			},
		},
		{
			Timestamp: 130,
			Type:      domain.EventTradeExecuted,
			Trade: &domain.Trade{
				ID:              2,
				BuyOrderID:      7002,
				SellOrderID:     20,
				BuyTrader:       "background",
				SellTrader:      "slow",
				Price:           domain.FloatToPrice(100.02),
				Qty:             5,
				Timestamp:       130,
				RestingQueuePos: 1,
			},
		},
	}

	logPath := filepath.Join(t.TempDir(), "events.jsonl")
	w, err := eventlog.NewWriter(logPath)
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	for _, event := range events {
		if err := w.Write(event); err != nil {
			t.Fatalf("write event: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	fromEvents := ComputeFromEvents(events)
	fromLog, err := ComputeFromLog(logPath)
	if err != nil {
		t.Fatalf("compute from log: %v", err)
	}

	if !reflect.DeepEqual(fromEvents, fromLog) {
		t.Fatalf("metrics mismatch between ComputeFromEvents and ComputeFromLog")
	}
}
