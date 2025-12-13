// Package trader implements trading agents that react to signals
// with configurable latency.
package trader

import (
	"math/rand"
	"sort"

	"github.com/akshitanchan/execution-fairness-simulator/internal/domain"
	"github.com/akshitanchan/execution-fairness-simulator/internal/latency"
)

// Agent represents a trader with latency and a strategy.
type Agent struct {
	ID       string
	Latency  *latency.Model
	Strategy *Strategy

	rng      *rand.Rand
	nextID   uint64
	idBase   uint64

	// Active orders this agent has on the book.
	ActiveOrders map[uint64]*domain.Order
}

// NewAgent creates a new trading agent.
func NewAgent(id string, lat *latency.Model, seed int64, idBase uint64) *Agent {
	return &Agent{
		ID:           id,
		Latency:      lat,
		Strategy:     NewStrategy(),
		rng:          rand.New(rand.NewSource(seed)),
		idBase:       idBase,
		nextID:       idBase,
		ActiveOrders: make(map[uint64]*domain.Order),
	}
}

func (a *Agent) allocateID() uint64 {
	a.nextID++
	return a.nextID
}

// OnSignal processes a signal event and returns orders to submit.
// The orders have DecisionTime set; the caller applies latency to get ArrivalTime.
func (a *Agent) OnSignal(signal *domain.Signal, bbo *domain.BBO, currentTime int64) []*domain.Order {
	if bbo.BidPrice == 0 || bbo.AskPrice == 0 {
		return nil // no market to trade against
	}

	return a.Strategy.Decide(a, signal, bbo, currentTime)
}

// OnFill notifies the agent that one of its orders was filled.
// Note: RemainingQty is already updated by the matching engine since
// we share the same *Order pointer. We only clean up ActiveOrders.
func (a *Agent) OnFill(trade *domain.Trade, orderID uint64) {
	order, exists := a.ActiveOrders[orderID]
	if !exists {
		return
	}
	if order.RemainingQty <= 0 {
		delete(a.ActiveOrders, orderID)
	}
}

// OnCancel notifies the agent that one of its orders was cancelled.
func (a *Agent) OnCancelAck(orderID uint64) {
	delete(a.ActiveOrders, orderID)
}

// Strategy defines the simple post-at-best + rebalance logic.
type Strategy struct {
	// ReQuoteInterval: how long to wait before re-quoting (in nanos).
	ReQuoteIntervalNs int64
	// CancelTimeoutNs: cancel unfilled orders after this duration.
	CancelTimeoutNs int64
	// CrossThreshold: if signal exceeds this, cross with market order.
	CrossThreshold float64
	// TargetQty: quantity to post.
	TargetQty int64

	lastSignalValue float64
	lastActionTime  int64
}

// NewStrategy creates a strategy with default parameters.
func NewStrategy() *Strategy {
	return &Strategy{
		ReQuoteIntervalNs: latency.MsToNs(100),
		CancelTimeoutNs:   latency.MsToNs(500),
		CrossThreshold:    1.0,
		TargetQty:         5,
	}
}

// Decide generates orders based on the current signal and book state.
func (s *Strategy) Decide(agent *Agent, signal *domain.Signal, bbo *domain.BBO, currentTime int64) []*domain.Order {
	var orders []*domain.Order

	// 1. Cancel stale orders that have been resting too long.
	// Sort keys for deterministic iteration.
	activeIDs := make([]uint64, 0, len(agent.ActiveOrders))
	for id := range agent.ActiveOrders {
		activeIDs = append(activeIDs, id)
	}
	sort.Slice(activeIDs, func(i, j int) bool { return activeIDs[i] < activeIDs[j] })
	for _, id := range activeIDs {
		order := agent.ActiveOrders[id]
		age := currentTime - order.DecisionTime
		if age > s.CancelTimeoutNs {
			cancelOrder := &domain.Order{
				ID:           agent.allocateID(),
				TraderID:     agent.ID,
				Type:         domain.CancelOrder,
				CancelID:     id,
				DecisionTime: currentTime,
			}
			orders = append(orders, cancelOrder)
		}
	}

	// 2. Decide action based on signal.
	// Strong signal → cross with market order.
	if signal.Value > s.CrossThreshold || signal.Value < -s.CrossThreshold {
		var side domain.Side
		if signal.Value > 0 {
			side = domain.Buy
		} else {
			side = domain.Sell
		}

		marketOrder := &domain.Order{
			ID:           agent.allocateID(),
			TraderID:     agent.ID,
			Side:         side,
			Type:         domain.MarketOrder,
			Qty:          s.TargetQty,
			DecisionTime: currentTime,
		}
		orders = append(orders, marketOrder)
		s.lastSignalValue = signal.Value
		s.lastActionTime = currentTime
		return orders
	}

	// 3. Otherwise, post limit orders at best bid/ask.
	// Only if we don't already have orders on this side.
	hasBid, hasAsk := false, false
	for _, id := range activeIDs {
		o := agent.ActiveOrders[id]
		_ = o
		if o.Side == domain.Buy {
			hasBid = true
		}
		if o.Side == domain.Sell {
			hasAsk = true
		}
	}

	if !hasBid && bbo.BidPrice > 0 {
		bidOrder := &domain.Order{
			ID:           agent.allocateID(),
			TraderID:     agent.ID,
			Side:         domain.Buy,
			Type:         domain.LimitOrder,
			Price:        bbo.BidPrice,
			Qty:          s.TargetQty,
			DecisionTime: currentTime,
		}
		orders = append(orders, bidOrder)
	}

	if !hasAsk && bbo.AskPrice > 0 {
		askOrder := &domain.Order{
			ID:           agent.allocateID(),
			TraderID:     agent.ID,
			Side:         domain.Sell,
			Type:         domain.LimitOrder,
			Price:        bbo.AskPrice,
			Qty:          s.TargetQty,
			DecisionTime: currentTime,
		}
		orders = append(orders, askOrder)
	}

	s.lastSignalValue = signal.Value
	s.lastActionTime = currentTime
	return orders
}
