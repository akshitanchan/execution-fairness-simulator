// Package engine provides the deterministic discrete-event simulation loop
package engine

import (
	"container/heap"

	"github.com/akshitanchan/execution-fairness-simulator/internal/domain"
)

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

type EventLoop struct {
	queue   eventHeap
	seqNo   uint64
	handler EventHandler

	// Stats
	EventsProcessed uint64
	CurrentTime     int64
}

func NewEventLoop(handler EventHandler) *EventLoop {
	el := &EventLoop{
		handler: handler,
	}
	heap.Init(&el.queue)
	return el
}

// Schedule adds an event to the priority queue.
// SeqNo is auto-assigned for deterministic ordering.
func (el *EventLoop) Schedule(event *domain.Event) {
	el.seqNo++
	event.SeqNo = el.seqNo
	heap.Push(&el.queue, event)
}

// ScheduleWithSeqNo adds an event with a pre-assigned SeqNo (for replay).
func (el *EventLoop) ScheduleWithSeqNo(event *domain.Event) {
	heap.Push(&el.queue, event)
}

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

// RunUntil processes events up to maxTime (inclusive).
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

func (el *EventLoop) Pending() int {
	return el.queue.Len()
}
