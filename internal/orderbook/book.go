// Package orderbook implements a single-instrument limit order book
// with price-time priority matching
package orderbook

import (
	"fmt"
	"sort"

	"github.com/akshitanchan/execution-fairness-simulator/internal/domain"
)

// PriceLevel holds all resting orders at a single price, in FIFO order
type PriceLevel struct {
	Price  int64
	Orders []*domain.Order
}

// TotalQty returns the sum of remaining quantities at this level
func (pl *PriceLevel) TotalQty() int64 {
	var total int64
	for _, o := range pl.Orders {
		total += o.RemainingQty
	}
	return total
}

func (pl *PriceLevel) removeFilledOrders() {
	n := 0
	for _, o := range pl.Orders {
		if o.RemainingQty > 0 {
			pl.Orders[n] = o
			n++
		}
	}
	pl.Orders = pl.Orders[:n]
}

// Book is a single-instrument limit order book
type Book struct {
	Bids []*PriceLevel // sorted descending by price (best bid first)
	Asks []*PriceLevel // sorted ascending by price (best ask first)

	// orderIndex maps order ID to the order pointer for fast cancel lookup
	orderIndex map[uint64]*domain.Order

	nextTradeID uint64

	lastBBO domain.BBO
}

// New creates an empty order book
func New() *Book {
	return &Book{
		orderIndex: make(map[uint64]*domain.Order),
	}
}

// ProcessOrder handles a limit, market, or cancel order
// Returns any trades generated and the updated BBO
func (b *Book) ProcessOrder(order *domain.Order, timestamp int64) ([]domain.Trade, *domain.BBO) {
	switch order.Type {
	case domain.LimitOrder:
		return b.processLimit(order, timestamp)
	case domain.MarketOrder:
		return b.processMarket(order, timestamp)
	case domain.CancelOrder:
		return b.processCancel(order)
	default:
		panic(fmt.Sprintf("unknown order type: %d", order.Type))
	}
}

// processLimit inserts a limit order, matching aggressively first
func (b *Book) processLimit(order *domain.Order, timestamp int64) ([]domain.Trade, *domain.BBO) {
	order.RemainingQty = order.Qty
	trades := b.match(order, timestamp)

	// If not fully filled, rest on the book
	if order.RemainingQty > 0 {
		b.insert(order)
	}

	bbo := b.BBO()
	return trades, bbo
}

// processMarket sweeps the book. No resting
func (b *Book) processMarket(order *domain.Order, timestamp int64) ([]domain.Trade, *domain.BBO) {
	order.RemainingQty = order.Qty
	trades := b.match(order, timestamp)
	bbo := b.BBO()
	return trades, bbo
}

// processCancel removes remaining quantity of the target order
func (b *Book) processCancel(cancel *domain.Order) ([]domain.Trade, *domain.BBO) {
	target, exists := b.orderIndex[cancel.CancelID]
	if !exists || target.RemainingQty <= 0 {
		// Already filled or unknown — no-op
		return nil, b.BBO()
	}

	target.RemainingQty = 0
	b.removeOrder(target)
	delete(b.orderIndex, target.ID)

	return nil, b.BBO()
}

// match attempts to fill the incoming order against the opposite side
func (b *Book) match(incoming *domain.Order, timestamp int64) []domain.Trade {
	var trades []domain.Trade
	var oppositeSide *[]*PriceLevel

	if incoming.Side == domain.Buy {
		oppositeSide = &b.Asks
	} else {
		oppositeSide = &b.Bids
	}

	for incoming.RemainingQty > 0 && len(*oppositeSide) > 0 {
		level := (*oppositeSide)[0]

		// Price check for limit orders
		if incoming.Type == domain.LimitOrder {
			if incoming.Side == domain.Buy && incoming.Price < level.Price {
				break // incoming bid too low
			}
			if incoming.Side == domain.Sell && incoming.Price > level.Price {
				break // incoming ask too high
			}
		}

		// Walk orders at this level in FIFO order
		for i := 0; i < len(level.Orders) && incoming.RemainingQty > 0; {
			resting := level.Orders[i]
			fillQty := min64(incoming.RemainingQty, resting.RemainingQty)

			incoming.RemainingQty -= fillQty
			resting.RemainingQty -= fillQty

			b.nextTradeID++
			trade := domain.Trade{
				ID:               b.nextTradeID,
				Price:            resting.Price, // trade at resting order's price
				Qty:              fillQty,
				Timestamp:        timestamp,
				PassiveOrderID:   resting.ID,
				AggressorOrderID: incoming.ID,
				RestingQueuePos:  i + 1, // 1-based position in FIFO queue
			}
			if incoming.Side == domain.Buy {
				trade.BuyOrderID = incoming.ID
				trade.SellOrderID = resting.ID
				trade.BuyTrader = incoming.TraderID
				trade.SellTrader = resting.TraderID
			} else {
				trade.SellOrderID = incoming.ID
				trade.BuyOrderID = resting.ID
				trade.SellTrader = incoming.TraderID
				trade.BuyTrader = resting.TraderID
			}
			trades = append(trades, trade)

			if resting.RemainingQty <= 0 {
				delete(b.orderIndex, resting.ID)
				// Remove from slice by advancing
				level.Orders = append(level.Orders[:i], level.Orders[i+1:]...)
			} else {
				i++
			}
		}

		// Remove empty levels
		if len(level.Orders) == 0 {
			*oppositeSide = (*oppositeSide)[1:]
		}
	}

	return trades
}

// insert places a resting order into the book at the appropriate level
func (b *Book) insert(order *domain.Order) {
	b.orderIndex[order.ID] = order

	if order.Side == domain.Buy {
		b.Bids = insertIntoLevels(b.Bids, order, true)
	} else {
		b.Asks = insertIntoLevels(b.Asks, order, false)
	}
}

// insertIntoLevels inserts an order into a sorted price level slice
// descending=true for bids, false for asks
func insertIntoLevels(levels []*PriceLevel, order *domain.Order, descending bool) []*PriceLevel {
	// Find the level for this price
	idx := sort.Search(len(levels), func(i int) bool {
		if descending {
			return levels[i].Price <= order.Price
		}
		return levels[i].Price >= order.Price
	})

	if idx < len(levels) && levels[idx].Price == order.Price {
		// Append to existing level (FIFO)
		levels[idx].Orders = append(levels[idx].Orders, order)
		return levels
	}

	// Insert new level
	newLevel := &PriceLevel{
		Price:  order.Price,
		Orders: []*domain.Order{order},
	}
	levels = append(levels, nil)
	copy(levels[idx+1:], levels[idx:])
	levels[idx] = newLevel
	return levels
}

// removeOrder removes an order from its price level
func (b *Book) removeOrder(order *domain.Order) {
	var levels *[]*PriceLevel
	if order.Side == domain.Buy {
		levels = &b.Bids
	} else {
		levels = &b.Asks
	}

	for i, level := range *levels {
		if level.Price != order.Price {
			continue
		}
		for j, o := range level.Orders {
			if o.ID == order.ID {
				level.Orders = append(level.Orders[:j], level.Orders[j+1:]...)
				if len(level.Orders) == 0 {
					*levels = append((*levels)[:i], (*levels)[i+1:]...)
				}
				return
			}
		}
	}
}

// BBO returns the current best bid and offer
func (b *Book) BBO() *domain.BBO {
	bbo := &domain.BBO{}

	if len(b.Bids) > 0 {
		bbo.BidPrice = b.Bids[0].Price
		bbo.BidQty = b.Bids[0].TotalQty()
	}
	if len(b.Asks) > 0 {
		bbo.AskPrice = b.Asks[0].Price
		bbo.AskQty = b.Asks[0].TotalQty()
	}
	if bbo.BidPrice > 0 && bbo.AskPrice > 0 {
		bbo.MidPrice = (bbo.BidPrice + bbo.AskPrice) / 2
	}

	return bbo
}

// QueuePosition returns the position (1-based) of an order at its price level
// Returns 0 if the order is not found on the book
func (b *Book) QueuePosition(orderID uint64) int {
	order, exists := b.orderIndex[orderID]
	if !exists {
		return 0
	}

	var levels []*PriceLevel
	if order.Side == domain.Buy {
		levels = b.Bids
	} else {
		levels = b.Asks
	}

	for _, level := range levels {
		if level.Price != order.Price {
			continue
		}
		for i, o := range level.Orders {
			if o.ID == orderID {
				return i + 1
			}
		}
	}
	return 0
}

// Depth returns the number of price levels on each side
func (b *Book) Depth() (bidLevels, askLevels int) {
	return len(b.Bids), len(b.Asks)
}

// TotalVolume returns total resting volume on each side
func (b *Book) TotalVolume() (bidVol, askVol int64) {
	for _, level := range b.Bids {
		bidVol += level.TotalQty()
	}
	for _, level := range b.Asks {
		askVol += level.TotalQty()
	}
	return
}

// AssertInvariants checks all book invariants. Panics on violation
func (b *Book) AssertInvariants() {
	// 1. Bids sorted descending
	for i := 1; i < len(b.Bids); i++ {
		if b.Bids[i].Price >= b.Bids[i-1].Price {
			panic(fmt.Sprintf("bid levels not sorted descending: %d >= %d at index %d",
				b.Bids[i].Price, b.Bids[i-1].Price, i))
		}
	}

	// 2. Asks sorted ascending
	for i := 1; i < len(b.Asks); i++ {
		if b.Asks[i].Price <= b.Asks[i-1].Price {
			panic(fmt.Sprintf("ask levels not sorted ascending: %d <= %d at index %d",
				b.Asks[i].Price, b.Asks[i-1].Price, i))
		}
	}

	// 3. No crossed book
	if len(b.Bids) > 0 && len(b.Asks) > 0 {
		if b.Bids[0].Price >= b.Asks[0].Price {
			panic(fmt.Sprintf("crossed book: best bid %d >= best ask %d",
				b.Bids[0].Price, b.Asks[0].Price))
		}
	}

	// 4. No empty levels
	for _, level := range b.Bids {
		if len(level.Orders) == 0 {
			panic(fmt.Sprintf("empty bid level at price %d", level.Price))
		}
	}
	for _, level := range b.Asks {
		if len(level.Orders) == 0 {
			panic(fmt.Sprintf("empty ask level at price %d", level.Price))
		}
	}

	// 5. No negative remaining quantities
	for _, level := range b.Bids {
		for _, o := range level.Orders {
			if o.RemainingQty < 0 {
				panic(fmt.Sprintf("negative remaining qty on bid order %d: %d", o.ID, o.RemainingQty))
			}
			if o.RemainingQty == 0 {
				panic(fmt.Sprintf("zero remaining qty order %d still on book", o.ID))
			}
		}
	}
	for _, level := range b.Asks {
		for _, o := range level.Orders {
			if o.RemainingQty < 0 {
				panic(fmt.Sprintf("negative remaining qty on ask order %d: %d", o.ID, o.RemainingQty))
			}
			if o.RemainingQty == 0 {
				panic(fmt.Sprintf("zero remaining qty order %d still on book", o.ID))
			}
		}
	}

	// 6. orderIndex consistency
	count := 0
	for _, level := range b.Bids {
		count += len(level.Orders)
	}
	for _, level := range b.Asks {
		count += len(level.Orders)
	}
	if count != len(b.orderIndex) {
		panic(fmt.Sprintf("orderIndex size %d != book order count %d", len(b.orderIndex), count))
	}
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
