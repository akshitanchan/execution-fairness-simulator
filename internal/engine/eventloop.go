// Package engine provides a deterministic discrete-event simulation loop
package engine

import (
	"container/heap"

	"github.com/akshitanchan/execution-fairness-simulator/internal/domain"
)

// EventHandler processes an event and may return new events to enqueue
type EventHandler func(event *domain.Event) []*domain.Event

// eventHeap is a min-heap of events ordered by (Timestamp, SeqNo)
type eventHeap []*domain.Event

func (h eventHeap) Len() int      { return len(h) }
func (h eventHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h eventHeap) Less(i, j int) bool {
	if h[i].Timestamp != h[j].Timestamp {
		return h[i].Timestamp < h[j].Timestamp
	}
	return h[i].SeqNo < h[j].SeqNo
}

func (h *eventHeap) Push(x interface{}) {
	*h = append(*h, x.(*domain.Event))
}

func (h *eventHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil // avoid memory leak
	*h = old[:n-1]
	return item
}

// EventLoop is the deterministic simulation event loop
type EventLoop struct {
	queue   eventHeap
	seqNo   uint64
	handler EventHandler

	// Stats
	EventsProcessed uint64
	CurrentTime     int64
}

// NewEventLoop creates a new event loop with the given handler
func NewEventLoop(handler EventHandler) *EventLoop {
	el := &EventLoop{
		handler: handler,
	}
	heap.Init(&el.queue)
	return el
}

// Schedule adds an event to the priority queue
// The event's SeqNo is set automatically for deterministic ordering
func (el *EventLoop) Schedule(event *domain.Event) {
	el.seqNo++
	event.SeqNo = el.seqNo
	heap.Push(&el.queue, event)
}

// ScheduleWithSeqNo adds an event with a pre-assigned SeqNo
// Use only when replaying from a log
func (el *EventLoop) ScheduleWithSeqNo(event *domain.Event) {
	heap.Push(&el.queue, event)
}

// Run processes events until the queue is empty
func (el *EventLoop) Run() {
	for el.queue.Len() > 0 {
		event := heap.Pop(&el.queue).(*domain.Event)
		el.CurrentTime = event.Timestamp
		el.EventsProcessed++

		newEvents := el.handler(event)
		for _, e := range newEvents {
			el.Schedule(e)
		}
	}
}

// RunUntil processes events until the given timestamp (inclusive)
// Returns true if the queue still has events
func (el *EventLoop) RunUntil(maxTime int64) bool {
	for el.queue.Len() > 0 {
		// Peek at the next event
		next := el.queue[0]
		if next.Timestamp > maxTime {
			return true
		}

		event := heap.Pop(&el.queue).(*domain.Event)
		el.CurrentTime = event.Timestamp
		el.EventsProcessed++

		newEvents := el.handler(event)
		for _, e := range newEvents {
			el.Schedule(e)
		}
	}
	return false
}

// Pending returns the number of events still in the queue
func (el *EventLoop) Pending() int {
	return el.queue.Len()
}
