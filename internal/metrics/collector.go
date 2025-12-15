// Package metrics collects per-trader execution quality metrics
// from the event log and trade records.
package metrics

import (
	"io"
	"sort"

	"github.com/akshitanchan/execution-fairness-simulator/internal/domain"
	"github.com/akshitanchan/execution-fairness-simulator/internal/eventlog"
)

// TraderMetrics holds computed metrics for a single trader.
type TraderMetrics struct {
	TraderID string `json:"trader_id"`

	// Order counts.
	OrdersSent   int `json:"orders_sent"`
	LimitOrders  int `json:"limit_orders"`
	MarketOrders int `json:"market_orders"`
	CancelsSent  int `json:"cancels_sent"`

	// Fill metrics.
	TotalFills     int     `json:"total_fills"`
	TotalQtyFilled int64   `json:"total_qty_filled"`
	FillRate       float64 `json:"fill_rate"` // filled executable orders / executable orders

	// Missed fill tracking.
	CanceledBeforeFill int `json:"canceled_before_fill"` // orders canceled without any fill

	// Price metrics.
	AvgExecPrice float64 `json:"avg_exec_price"`
	AvgSlippage  float64 `json:"avg_slippage"` // vs mid at decision time
	SlippageBps  float64 `json:"slippage_bps"` // in basis points

	// Time metrics.
	AvgTimeToFillNs float64   `json:"avg_time_to_fill_ns"`
	TimeToFillDist  []float64 `json:"time_to_fill_dist"` // all time-to-fill values in ms

	// Queue position metrics.
	AvgQueuePosPlace float64 `json:"avg_queue_pos_place"` // at placement
	AvgQueuePosFill  float64 `json:"avg_queue_pos_fill"`  // at fill

	// Adverse selection.
	AvgPriceMoveAfterFill float64 `json:"avg_price_move_after_fill"` // in price units
	AdverseSelectionBps   float64 `json:"adverse_selection_bps"`

	// Raw data for plotting.
	SlippageValues []float64 `json:"slippage_values,omitempty"`
}

// Collector accumulates metrics from events.
type Collector struct {
	traderMetrics map[string]*traderAccum
	bboHistory    []bboSnapshot
	tradeHistory  []tradeRecord
}

type traderAccum struct {
	id           string
	ordersSent   int
	limitOrders  int
	marketOrders int
	cancelsSent  int

	// Track orders for time-to-fill.
	orderTimes map[uint64]orderInfo // orderID -> info

	// Track which orders have received fills.
	filledOrders map[uint64]bool // orderID -> filled

	// Track cancel targets.
	cancelTargets []uint64 // orderIDs that were canceled

	fills []fillInfo
}

type orderInfo struct {
	decisionTime  int64
	arrivalTime   int64
	side          domain.Side
	price         int64
	midAtDecision int64
	queuePosPlace int // queue position at placement
}

type fillInfo struct {
	tradePrice    int64
	fillQty       int64
	decisionTime  int64
	fillTime      int64
	midAtDecision int64
	queuePosFill  int
	side          domain.Side
}

type bboSnapshot struct {
	timestamp int64
	bbo       domain.BBO
}

type tradeRecord struct {
	timestamp int64
	price     int64
}

// NewCollector creates a new metrics collector.
func NewCollector() *Collector {
	return &Collector{
		traderMetrics: make(map[string]*traderAccum),
	}
}

func (c *Collector) getAccum(traderID string) *traderAccum {
	if a, ok := c.traderMetrics[traderID]; ok {
		return a
	}
	a := &traderAccum{
		id:           traderID,
		orderTimes:   make(map[uint64]orderInfo),
		filledOrders: make(map[uint64]bool),
	}
	c.traderMetrics[traderID] = a
	return a
}

// ProcessEvent ingests a single event.
func (c *Collector) ProcessEvent(event *domain.Event) {
	switch event.Type {
	case domain.EventOrderAccepted:
		if event.Order != nil {
			c.processOrder(event)
		}
	case domain.EventTradeExecuted:
		if event.Trade != nil {
			c.processTrade(event)
		}
	case domain.EventOrderCanceled:
		if event.Order != nil {
			c.processCancel(event)
		}
	case domain.EventBBOUpdate:
		if event.BBO != nil {
			c.bboHistory = append(c.bboHistory, bboSnapshot{
				timestamp: event.Timestamp,
				bbo:       *event.BBO,
			})
		}
	}
}

func (c *Collector) processOrder(event *domain.Event) {
	order := event.Order
	if order.TraderID == "background" {
		return // skip background orders
	}

	a := c.getAccum(order.TraderID)
	a.ordersSent++

	switch order.Type {
	case domain.LimitOrder:
		a.limitOrders++
		midAtDecision := c.midAtTime(order.DecisionTime)
		a.orderTimes[order.ID] = orderInfo{
			decisionTime:  order.DecisionTime,
			arrivalTime:   order.ArrivalTime,
			side:          order.Side,
			price:         order.Price,
			midAtDecision: midAtDecision,
			queuePosPlace: order.QueuePos,
		}
	case domain.MarketOrder:
		a.marketOrders++
		midAtDecision := c.midAtTime(order.DecisionTime)
		a.orderTimes[order.ID] = orderInfo{
			decisionTime:  order.DecisionTime,
			arrivalTime:   order.ArrivalTime,
			side:          order.Side,
			midAtDecision: midAtDecision,
		}
	case domain.CancelOrder:
		a.cancelsSent++
	}
}

func (c *Collector) processCancel(event *domain.Event) {
	order := event.Order
	if order.TraderID == "background" {
		return
	}

	a := c.getAccum(order.TraderID)
	if order.CancelID > 0 {
		a.cancelTargets = append(a.cancelTargets, order.CancelID)
	}
}

func (c *Collector) processTrade(event *domain.Event) {
	trade := event.Trade
	c.tradeHistory = append(c.tradeHistory, tradeRecord{
		timestamp: trade.Timestamp,
		price:     trade.Price,
	})

	// Record fill for the buyer.
	c.recordFill(trade.BuyTrader, trade.BuyOrderID, trade, event.Timestamp, domain.Buy)
	// Record fill for the seller.
	c.recordFill(trade.SellTrader, trade.SellOrderID, trade, event.Timestamp, domain.Sell)
}

func (c *Collector) recordFill(traderID string, orderID uint64, trade *domain.Trade, fillTime int64, side domain.Side) {
	if traderID == "background" {
		return
	}

	a := c.getAccum(traderID)
	a.filledOrders[orderID] = true
	info, exists := a.orderTimes[orderID]
	var midAtDecision int64
	var decisionTime int64
	var queuePosFill int
	if exists {
		midAtDecision = info.midAtDecision
		decisionTime = info.decisionTime
	}
	// The resting queue position only applies to the passive order.
	if trade.PassiveOrderID > 0 && orderID == trade.PassiveOrderID {
		queuePosFill = trade.RestingQueuePos
	}

	a.fills = append(a.fills, fillInfo{
		tradePrice:    trade.Price,
		fillQty:       trade.Qty,
		decisionTime:  decisionTime,
		fillTime:      fillTime,
		midAtDecision: midAtDecision,
		queuePosFill:  queuePosFill,
		side:          side,
	})
}

// midAtTime returns the mid price at a given time by searching BBO history.
func (c *Collector) midAtTime(t int64) int64 {
	if len(c.bboHistory) == 0 {
		return 0
	}
	// Binary search for the latest BBO before or at time t.
	idx := sort.Search(len(c.bboHistory), func(i int) bool {
		return c.bboHistory[i].timestamp > t
	})
	if idx == 0 {
		return c.bboHistory[0].bbo.MidPrice
	}
	return c.bboHistory[idx-1].bbo.MidPrice
}

// priceAfterFill returns the mid price N ms after a fill time.
func (c *Collector) priceAfterDuration(fillTime int64, durationNs int64) int64 {
	targetTime := fillTime + durationNs
	return c.midAtTime(targetTime)
}

// Compute calculates final metrics for all tracked traders.
func (c *Collector) Compute() map[string]*TraderMetrics {
	result := make(map[string]*TraderMetrics)

	for traderID, a := range c.traderMetrics {
		m := &TraderMetrics{
			TraderID:     traderID,
			OrdersSent:   a.ordersSent,
			LimitOrders:  a.limitOrders,
			MarketOrders: a.marketOrders,
			CancelsSent:  a.cancelsSent,
			TotalFills:   len(a.fills),
		}

		// Fill rate is order-level: executable orders with >=1 fill / executable orders.
		totalExecutableOrders := len(a.orderTimes)
		if totalExecutableOrders > 0 {
			filledExecutableOrders := 0
			for orderID := range a.orderTimes {
				if a.filledOrders[orderID] {
					filledExecutableOrders++
				}
			}
			m.FillRate = float64(filledExecutableOrders) / float64(totalExecutableOrders)
		}

		var totalPrice float64
		var totalSlippage float64
		var totalTimeToFill float64
		var totalQty int64
		var totalQueuePosPlace float64
		var queuePosPlaceCount int
		var totalQueuePosFill float64
		var queuePosFillCount int

		// Compute average queue position at placement from order records.
		for _, info := range a.orderTimes {
			if info.queuePosPlace > 0 {
				totalQueuePosPlace += float64(info.queuePosPlace)
				queuePosPlaceCount++
			}
		}

		for _, fill := range a.fills {
			qty := fill.fillQty
			totalQty += qty
			totalPrice += domain.PriceToFloat(fill.tradePrice) * float64(qty)

			// Slippage: signed difference from mid at decision time.
			if fill.midAtDecision > 0 {
				var slippage float64
				if fill.side == domain.Buy {
					// Buying: slippage = exec_price - mid (positive = worse for buyer)
					slippage = domain.PriceToFloat(fill.tradePrice) - domain.PriceToFloat(fill.midAtDecision)
				} else {
					// Selling: slippage = mid - exec_price (positive = worse for seller)
					slippage = domain.PriceToFloat(fill.midAtDecision) - domain.PriceToFloat(fill.tradePrice)
				}
				totalSlippage += slippage * float64(qty)
				m.SlippageValues = append(m.SlippageValues, slippage)
			}

			// Time to fill.
			if fill.decisionTime > 0 {
				ttf := float64(fill.fillTime-fill.decisionTime) / 1e6 // to ms
				totalTimeToFill += ttf
				m.TimeToFillDist = append(m.TimeToFillDist, ttf)
			}

			// Adverse selection: price move 100ms after fill.
			priceAfter := c.priceAfterDuration(fill.fillTime, 100_000_000) // 100ms
			if priceAfter > 0 && fill.tradePrice > 0 {
				var move float64
				if fill.side == domain.Buy {
					// For buyer: adverse if price went down after buy
					move = domain.PriceToFloat(priceAfter) - domain.PriceToFloat(fill.tradePrice)
				} else {
					// For seller: adverse if price went up after sell
					move = domain.PriceToFloat(fill.tradePrice) - domain.PriceToFloat(priceAfter)
				}
				m.AvgPriceMoveAfterFill += move
			}

			// Queue position at fill.
			if fill.queuePosFill > 0 {
				totalQueuePosFill += float64(fill.queuePosFill)
				queuePosFillCount++
			}
		}

		m.TotalQtyFilled = totalQty

		if totalQty > 0 {
			m.AvgExecPrice = totalPrice / float64(totalQty)
			m.AvgSlippage = totalSlippage / float64(totalQty)
			midPrice := domain.PriceToFloat(c.midAtTime(0))
			if midPrice > 0 {
				m.SlippageBps = (m.AvgSlippage / midPrice) * 10000
			}
		}

		if len(a.fills) > 0 {
			m.AvgTimeToFillNs = totalTimeToFill / float64(len(a.fills))
			m.AvgPriceMoveAfterFill /= float64(len(a.fills))

			midPrice := domain.PriceToFloat(c.midAtTime(0))
			if midPrice > 0 {
				m.AdverseSelectionBps = (m.AvgPriceMoveAfterFill / midPrice) * 10000
			}
		}

		// Queue position averages.
		if queuePosPlaceCount > 0 {
			m.AvgQueuePosPlace = totalQueuePosPlace / float64(queuePosPlaceCount)
		}
		if queuePosFillCount > 0 {
			m.AvgQueuePosFill = totalQueuePosFill / float64(queuePosFillCount)
		}

		// Canceled-before-fill: count cancel targets that were never filled.
		for _, canceledID := range a.cancelTargets {
			if !a.filledOrders[canceledID] {
				m.CanceledBeforeFill++
			}
		}

		// Sort time-to-fill for CDF plotting.
		sort.Float64s(m.TimeToFillDist)

		result[traderID] = m
	}

	return result
}

// ComputeFromLog reads an event log and computes metrics.
func ComputeFromLog(logPath string) (map[string]*TraderMetrics, error) {
	reader, err := eventlog.NewReader(logPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	c := NewCollector()
	for {
		event, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		c.ProcessEvent(event)
	}

	return c.Compute(), nil
}

// ComputeFromEvents computes metrics directly from an in-memory event stream.
func ComputeFromEvents(events []*domain.Event) map[string]*TraderMetrics {
	c := NewCollector()
	for _, event := range events {
		if event == nil {
			continue
		}
		c.ProcessEvent(event)
	}
	return c.Compute()
}
