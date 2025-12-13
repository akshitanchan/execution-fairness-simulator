package orderbook

import (
	"testing"

	"github.com/akshitanchan/execution-fairness-simulator/internal/domain"
)

func makeLimit(id uint64, side domain.Side, price, qty int64) *domain.Order {
	return &domain.Order{
		ID:       id,
		TraderID: "test",
		Side:     side,
		Type:     domain.LimitOrder,
		Price:    price,
		Qty:      qty,
	}
}

func makeMarket(id uint64, side domain.Side, qty int64) *domain.Order {
	return &domain.Order{
		ID:       id,
		TraderID: "test",
		Side:     side,
		Type:     domain.MarketOrder,
		Qty:      qty,
	}
}

func makeCancel(id uint64, cancelID uint64) *domain.Order {
	return &domain.Order{
		ID:       id,
		TraderID: "test",
		Type:     domain.CancelOrder,
		CancelID: cancelID,
	}
}

// TestFIFOWithinPriceLevel verifies that orders at the same price are
// filled in arrival (insertion) order.
func TestFIFOWithinPriceLevel(t *testing.T) {
	book := New()

	// Place 3 sell orders at the same price.
	book.ProcessOrder(makeLimit(1, domain.Sell, 1000, 10), 0)
	book.ProcessOrder(makeLimit(2, domain.Sell, 1000, 10), 0)
	book.ProcessOrder(makeLimit(3, domain.Sell, 1000, 10), 0)
	book.AssertInvariants()

	// A buy market order for 15 should fill orders 1 (10) and 2 (5 partial).
	trades, _ := book.ProcessOrder(makeMarket(100, domain.Buy, 15), 1)
	book.AssertInvariants()

	if len(trades) != 2 {
		t.Fatalf("expected 2 trades, got %d", len(trades))
	}

	// First fill: order 1, full 10.
	if trades[0].SellOrderID != 1 || trades[0].Qty != 10 {
		t.Errorf("trade 0: expected sell order 1 qty 10, got sell %d qty %d",
			trades[0].SellOrderID, trades[0].Qty)
	}

	// Second fill: order 2, partial 5.
	if trades[1].SellOrderID != 2 || trades[1].Qty != 5 {
		t.Errorf("trade 1: expected sell order 2 qty 5, got sell %d qty %d",
			trades[1].SellOrderID, trades[1].Qty)
	}

	// Order 2 should have 5 remaining, order 3 untouched.
	pos2 := book.QueuePosition(2)
	pos3 := book.QueuePosition(3)
	if pos2 != 1 {
		t.Errorf("order 2 should be at position 1, got %d", pos2)
	}
	if pos3 != 2 {
		t.Errorf("order 3 should be at position 2, got %d", pos3)
	}
}

// TestMarketOrderSweepsMultipleLevels verifies that a large market order
// sweeps across multiple price levels.
func TestMarketOrderSweepsMultipleLevels(t *testing.T) {
	book := New()

	// Build an ask side with 3 levels.
	book.ProcessOrder(makeLimit(1, domain.Sell, 100, 5), 0)
	book.ProcessOrder(makeLimit(2, domain.Sell, 101, 5), 0)
	book.ProcessOrder(makeLimit(3, domain.Sell, 102, 5), 0)
	book.AssertInvariants()

	// Buy market order for 12: should sweep 100(5) + 101(5) + 102(2).
	trades, bbo := book.ProcessOrder(makeMarket(100, domain.Buy, 12), 1)
	book.AssertInvariants()

	if len(trades) != 3 {
		t.Fatalf("expected 3 trades, got %d", len(trades))
	}
	if trades[0].Price != 100 || trades[0].Qty != 5 {
		t.Errorf("trade 0: expected price 100 qty 5, got %d/%d", trades[0].Price, trades[0].Qty)
	}
	if trades[1].Price != 101 || trades[1].Qty != 5 {
		t.Errorf("trade 1: expected price 101 qty 5, got %d/%d", trades[1].Price, trades[1].Qty)
	}
	if trades[2].Price != 102 || trades[2].Qty != 2 {
		t.Errorf("trade 2: expected price 102 qty 2, got %d/%d", trades[2].Price, trades[2].Qty)
	}

	// Remaining: 3 at price 102.
	if bbo.AskPrice != 102 || bbo.AskQty != 3 {
		t.Errorf("expected ask 102/3, got %d/%d", bbo.AskPrice, bbo.AskQty)
	}
}

// TestCancelRemovesRemainingOnly verifies that cancel removes the resting
// order without affecting previously filled quantity.
func TestCancelRemovesRemainingOnly(t *testing.T) {
	book := New()

	// Place sell order of 10.
	book.ProcessOrder(makeLimit(1, domain.Sell, 100, 10), 0)
	book.AssertInvariants()

	// Partially fill it with a buy of 3.
	trades, _ := book.ProcessOrder(makeMarket(2, domain.Buy, 3), 1)
	book.AssertInvariants()

	if len(trades) != 1 || trades[0].Qty != 3 {
		t.Fatalf("expected 1 trade of qty 3, got %d trades", len(trades))
	}

	// Cancel the remaining.
	book.ProcessOrder(makeCancel(3, 1), 2)
	book.AssertInvariants()

	// Book should be empty.
	bidLevels, askLevels := book.Depth()
	if bidLevels != 0 || askLevels != 0 {
		t.Errorf("expected empty book, got %d bid levels, %d ask levels", bidLevels, askLevels)
	}
}

// TestCancelUnknownOrderIsNoop verifies that canceling a non-existent order
// doesn't panic or corrupt the book.
func TestCancelUnknownOrderIsNoop(t *testing.T) {
	book := New()
	book.ProcessOrder(makeLimit(1, domain.Sell, 100, 10), 0)
	book.AssertInvariants()

	// Cancel non-existent order.
	book.ProcessOrder(makeCancel(2, 999), 1)
	book.AssertInvariants()

	_, askLevels := book.Depth()
	if askLevels != 1 {
		t.Errorf("expected 1 ask level, got %d", askLevels)
	}
}

// TestCrossedLimitOrderMatchesImmediately verifies that a crossing limit
// order is matched immediately (no crossed book).
func TestCrossedLimitOrderMatchesImmediately(t *testing.T) {
	book := New()

	// Resting ask at 100.
	book.ProcessOrder(makeLimit(1, domain.Sell, 100, 10), 0)
	book.AssertInvariants()

	// Crossing bid at 101 (higher than best ask).
	trades, _ := book.ProcessOrder(makeLimit(2, domain.Buy, 101, 5), 1)
	book.AssertInvariants()

	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	if trades[0].Price != 100 {
		t.Errorf("expected trade at resting price 100, got %d", trades[0].Price)
	}
	if trades[0].Qty != 5 {
		t.Errorf("expected trade qty 5, got %d", trades[0].Qty)
	}
}

// TestBBOUpdates verifies BBO is correct after various operations.
func TestBBOUpdates(t *testing.T) {
	book := New()

	bbo := book.BBO()
	if bbo.BidPrice != 0 || bbo.AskPrice != 0 {
		t.Error("expected zero BBO on empty book")
	}

	book.ProcessOrder(makeLimit(1, domain.Buy, 99, 10), 0)
	book.ProcessOrder(makeLimit(2, domain.Sell, 101, 10), 0)
	book.AssertInvariants()

	bbo = book.BBO()
	if bbo.BidPrice != 99 {
		t.Errorf("expected bid 99, got %d", bbo.BidPrice)
	}
	if bbo.AskPrice != 101 {
		t.Errorf("expected ask 101, got %d", bbo.AskPrice)
	}
	if bbo.MidPrice != 100 {
		t.Errorf("expected mid 100, got %d", bbo.MidPrice)
	}

	// Add a better bid.
	book.ProcessOrder(makeLimit(3, domain.Buy, 100, 5), 0)
	book.AssertInvariants()
	bbo = book.BBO()
	if bbo.BidPrice != 100 {
		t.Errorf("expected bid 100 after improvement, got %d", bbo.BidPrice)
	}
}

// TestPartialFillKeepsOrderOnBook verifies that partially filled limit orders
// remain on the book with reduced quantity.
func TestPartialFillKeepsOrderOnBook(t *testing.T) {
	book := New()

	book.ProcessOrder(makeLimit(1, domain.Sell, 100, 10), 0)
	book.ProcessOrder(makeMarket(2, domain.Buy, 3), 1)
	book.AssertInvariants()

	bbo := book.BBO()
	if bbo.AskQty != 7 {
		t.Errorf("expected 7 remaining at ask, got %d", bbo.AskQty)
	}
}

// TestEmptyBookMarketOrderNoTrades verifies a market order on an empty
// opposite side produces no trades.
func TestEmptyBookMarketOrderNoTrades(t *testing.T) {
	book := New()

	trades, _ := book.ProcessOrder(makeMarket(1, domain.Buy, 10), 0)
	book.AssertInvariants()

	if len(trades) != 0 {
		t.Errorf("expected 0 trades on empty book, got %d", len(trades))
	}
}

// TestMultipleBidLevels verifies correct bid-side sorting and matching.
func TestMultipleBidLevels(t *testing.T) {
	book := New()

	book.ProcessOrder(makeLimit(1, domain.Buy, 98, 10), 0)
	book.ProcessOrder(makeLimit(2, domain.Buy, 100, 5), 0)
	book.ProcessOrder(makeLimit(3, domain.Buy, 99, 8), 0)
	book.AssertInvariants()

	bbo := book.BBO()
	if bbo.BidPrice != 100 {
		t.Errorf("expected best bid 100, got %d", bbo.BidPrice)
	}

	// Sell market sweeps best bid first.
	trades, _ := book.ProcessOrder(makeMarket(10, domain.Sell, 7), 1)
	book.AssertInvariants()

	if len(trades) != 2 {
		t.Fatalf("expected 2 trades, got %d", len(trades))
	}
	if trades[0].Price != 100 || trades[0].Qty != 5 {
		t.Errorf("trade 0: expected 100/5, got %d/%d", trades[0].Price, trades[0].Qty)
	}
	if trades[1].Price != 99 || trades[1].Qty != 2 {
		t.Errorf("trade 1: expected 99/2, got %d/%d", trades[1].Price, trades[1].Qty)
	}
}

// TestQueuePosition verifies queue position tracking.
func TestQueuePosition(t *testing.T) {
	book := New()

	book.ProcessOrder(makeLimit(1, domain.Buy, 100, 10), 0)
	book.ProcessOrder(makeLimit(2, domain.Buy, 100, 5), 0)
	book.ProcessOrder(makeLimit(3, domain.Buy, 100, 8), 0)
	book.AssertInvariants()

	if pos := book.QueuePosition(1); pos != 1 {
		t.Errorf("order 1 position: expected 1, got %d", pos)
	}
	if pos := book.QueuePosition(2); pos != 2 {
		t.Errorf("order 2 position: expected 2, got %d", pos)
	}
	if pos := book.QueuePosition(3); pos != 3 {
		t.Errorf("order 3 position: expected 3, got %d", pos)
	}

	// Non-existent order.
	if pos := book.QueuePosition(999); pos != 0 {
		t.Errorf("non-existent order: expected 0, got %d", pos)
	}
}
