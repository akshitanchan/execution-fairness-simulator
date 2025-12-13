package engine

import (
	"testing"

	"github.com/akshitanchan/execution-fairness-simulator/internal/domain"
)

func TestEventLoopOrdering(t *testing.T) {
	var processed []uint64

	handler := func(event *domain.Event) []*domain.Event {
		processed = append(processed, event.SeqNo)
		return nil
	}

	el := NewEventLoop(handler)

	// Schedule events out of order.
	el.Schedule(&domain.Event{Timestamp: 300, Type: domain.EventSignal})
	el.Schedule(&domain.Event{Timestamp: 100, Type: domain.EventSignal})
	el.Schedule(&domain.Event{Timestamp: 200, Type: domain.EventSignal})

	el.Run()

	if len(processed) != 3 {
		t.Fatalf("expected 3 events, got %d", len(processed))
	}

	// SeqNos are assigned 1,2,3. Timestamps are 300,100,200.
	// Order should be: ts=100(seq=2), ts=200(seq=3), ts=300(seq=1).
	expectedSeqs := []uint64{2, 3, 1}
	for i, seq := range expectedSeqs {
		if processed[i] != seq {
			t.Errorf("event %d: expected seq %d, got %d", i, seq, processed[i])
		}
	}
}

func TestEventLoopSameTimestampFIFO(t *testing.T) {
	var processed []uint64

	handler := func(event *domain.Event) []*domain.Event {
		// Use the order's ID to track which event was processed.
		if event.Order != nil {
			processed = append(processed, event.Order.ID)
		}
		return nil
	}

	el := NewEventLoop(handler)

	// Three events at the same timestamp — should be processed in SeqNo order.
	el.Schedule(&domain.Event{Timestamp: 100, Type: domain.EventOrderAccepted,
		Order: &domain.Order{ID: 10}})
	el.Schedule(&domain.Event{Timestamp: 100, Type: domain.EventOrderAccepted,
		Order: &domain.Order{ID: 20}})
	el.Schedule(&domain.Event{Timestamp: 100, Type: domain.EventOrderAccepted,
		Order: &domain.Order{ID: 30}})

	el.Run()

	expected := []uint64{10, 20, 30}
	for i, id := range expected {
		if processed[i] != id {
			t.Errorf("event %d: expected order %d, got %d", i, id, processed[i])
		}
	}
}

func TestEventLoopHandlerEnqueuesNewEvents(t *testing.T) {
	var count int

	handler := func(event *domain.Event) []*domain.Event {
		count++
		// First event spawns two more.
		if event.Timestamp == 0 {
			return []*domain.Event{
				{Timestamp: 10, Type: domain.EventSignal},
				{Timestamp: 20, Type: domain.EventSignal},
			}
		}
		return nil
	}

	el := NewEventLoop(handler)
	el.Schedule(&domain.Event{Timestamp: 0, Type: domain.EventSimStart})
	el.Run()

	if count != 3 {
		t.Errorf("expected 3 events processed, got %d", count)
	}
}

func TestRunUntil(t *testing.T) {
	var count int

	handler := func(event *domain.Event) []*domain.Event {
		count++
		return nil
	}

	el := NewEventLoop(handler)
	el.Schedule(&domain.Event{Timestamp: 100, Type: domain.EventSignal})
	el.Schedule(&domain.Event{Timestamp: 200, Type: domain.EventSignal})
	el.Schedule(&domain.Event{Timestamp: 300, Type: domain.EventSignal})

	hasMore := el.RunUntil(200)

	if count != 2 {
		t.Errorf("expected 2 events processed, got %d", count)
	}
	if !hasMore {
		t.Error("expected hasMore=true")
	}
	if el.Pending() != 1 {
		t.Errorf("expected 1 pending, got %d", el.Pending())
	}
}
