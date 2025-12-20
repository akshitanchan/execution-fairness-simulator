// Package domain defines the core types used across the simulation:
// orders, trades, events, and supporting enums/constants
package domain

import (
	"fmt"
	"strings"
)

// --- Price representation ---
// Prices are fixed-point int64 with 4 decimal places
// e.g. $100.0050 is stored as 1_000_050

const PriceScale = 10_000

// PriceToFloat converts a fixed-point price to float64 for display
func PriceToFloat(p int64) float64 {
	return float64(p) / float64(PriceScale)
}

// FloatToPrice converts a float64 to fixed-point price
func FloatToPrice(f float64) int64 {
	return int64(f * float64(PriceScale))
}

// FormatPrice returns a human-readable price string
func FormatPrice(p int64) string {
	return fmt.Sprintf("%.4f", PriceToFloat(p))
}

// --- Enums ---

type Side int8

const (
	Buy  Side = 1
	Sell Side = -1
)

func (s Side) String() string {
	if s == Buy {
		return "BUY"
	}
	return "SELL"
}

func (s Side) Opposite() Side {
	return -s
}

// MarshalJSON serializes Side as a human-readable string
func (s Side) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

// UnmarshalJSON deserializes Side from a string or integer
func (s *Side) UnmarshalJSON(data []byte) error {
	str := strings.Trim(string(data), `"`)
	switch str {
	case "BUY", "1":
		*s = Buy
	case "SELL", "-1":
		*s = Sell
	default:
		return fmt.Errorf("unknown Side: %s", str)
	}
	return nil
}

type OrderType int8

const (
	LimitOrder OrderType = iota
	MarketOrder
	CancelOrder
)

func (t OrderType) String() string {
	switch t {
	case LimitOrder:
		return "LIMIT"
	case MarketOrder:
		return "MARKET"
	case CancelOrder:
		return "CANCEL"
	default:
		return "UNKNOWN"
	}
}

// MarshalJSON serializes OrderType as a human-readable string
func (t OrderType) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.String() + `"`), nil
}

// UnmarshalJSON deserializes OrderType from a string or integer
func (t *OrderType) UnmarshalJSON(data []byte) error {
	str := strings.Trim(string(data), `"`)
	switch str {
	case "LIMIT", "0":
		*t = LimitOrder
	case "MARKET", "1":
		*t = MarketOrder
	case "CANCEL", "2":
		*t = CancelOrder
	default:
		return fmt.Errorf("unknown OrderType: %s", str)
	}
	return nil
}

type EventType int8

const (
	EventOrderAccepted EventType = iota
	EventOrderCanceled
	EventTradeExecuted
	EventBBOUpdate
	EventSignal
	EventReQuote
	EventSimStart
	EventSimEnd
)

func (e EventType) String() string {
	switch e {
	case EventOrderAccepted:
		return "ORDER_ACCEPTED"
	case EventOrderCanceled:
		return "ORDER_CANCELED"
	case EventTradeExecuted:
		return "TRADE_EXECUTED"
	case EventBBOUpdate:
		return "BBO_UPDATE"
	case EventSignal:
		return "SIGNAL"
	case EventReQuote:
		return "REQUOTE"
	case EventSimStart:
		return "SIM_START"
	case EventSimEnd:
		return "SIM_END"
	default:
		return "UNKNOWN"
	}
}

// MarshalJSON serializes EventType as a human-readable string
func (e EventType) MarshalJSON() ([]byte, error) {
	return []byte(`"` + e.String() + `"`), nil
}

// UnmarshalJSON deserializes EventType from a string or integer
func (e *EventType) UnmarshalJSON(data []byte) error {
	str := strings.Trim(string(data), `"`)
	switch str {
	case "ORDER_ACCEPTED", "0":
		*e = EventOrderAccepted
	case "ORDER_CANCELED", "1":
		*e = EventOrderCanceled
	case "TRADE_EXECUTED", "2":
		*e = EventTradeExecuted
	case "BBO_UPDATE", "3":
		*e = EventBBOUpdate
	case "SIGNAL", "4":
		*e = EventSignal
	case "REQUOTE", "5":
		*e = EventReQuote
	case "SIM_START", "6":
		*e = EventSimStart
	case "SIM_END", "7":
		*e = EventSimEnd
	default:
		return fmt.Errorf("unknown EventType: %s", str)
	}
	return nil
}

// --- Core structures ---

// Order represents a limit, market, or cancel instruction
type Order struct {
	ID           uint64    `json:"id"`
	TraderID     string    `json:"trader_id"`
	Side         Side      `json:"side"`
	Type         OrderType `json:"type"`
	Price        int64     `json:"price"` // 0 for market orders
	Qty          int64     `json:"qty"`
	RemainingQty int64     `json:"remaining_qty"`
	DecisionTime int64     `json:"decision_time"`       // nanos: when trader decided
	ArrivalTime  int64     `json:"arrival_time"`        // nanos: after latency
	SeqNo        uint64    `json:"seq_no"`              // global FIFO tie-break
	CancelID     uint64    `json:"cancel_id,omitempty"` // for CancelOrder: target order ID
	QueuePos     int       `json:"queue_pos,omitempty"` // 1-based queue position at placement
}

// IsFilled returns true if the order has been fully filled
func (o *Order) IsFilled() bool {
	return o.RemainingQty <= 0
}

// Trade represents a matched execution
type Trade struct {
	ID          uint64 `json:"id"`
	BuyOrderID  uint64 `json:"buy_order_id"`
	SellOrderID uint64 `json:"sell_order_id"`
	BuyTrader   string `json:"buy_trader"`
	SellTrader  string `json:"sell_trader"`
	Price       int64  `json:"price"`
	Qty         int64  `json:"qty"`
	Timestamp   int64  `json:"timestamp"`
	// Explicit passive/aggressor identity for attribution in analytics
	PassiveOrderID   uint64 `json:"passive_order_id,omitempty"`
	AggressorOrderID uint64 `json:"aggressor_order_id,omitempty"`
	// Queue position of the resting (passive) order at fill time
	RestingQueuePos int `json:"resting_queue_pos,omitempty"`
}

// BBO represents best bid and offer snapshot
type BBO struct {
	BidPrice int64 `json:"bid_price"`
	BidQty   int64 `json:"bid_qty"`
	AskPrice int64 `json:"ask_price"`
	AskQty   int64 `json:"ask_qty"`
	MidPrice int64 `json:"mid_price"` // (bid+ask)/2
}

// Signal represents a trading signal broadcast to all traders
type Signal struct {
	Value    float64 `json:"value"`     // signal strength / direction
	MidPrice int64   `json:"mid_price"` // mid at signal time
}

// Event is the core unit in the event loop and event log
type Event struct {
	SeqNo     uint64    `json:"seq_no"`
	Timestamp int64     `json:"timestamp"`
	Type      EventType `json:"type"`
	TraderID  string    `json:"trader_id,omitempty"` // set for trader-specific events (e.g. re-quote)

	// Exactly one of these is set depending on Type
	Order  *Order  `json:"order,omitempty"`
	Trade  *Trade  `json:"trade,omitempty"`
	BBO    *BBO    `json:"bbo,omitempty"`
	Signal *Signal `json:"signal,omitempty"`
}
