// Package sim wires the order book, event loop, traders, and scenario generator into a run
package sim

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/akshitanchan/execution-fairness-simulator/internal/domain"
	"github.com/akshitanchan/execution-fairness-simulator/internal/engine"
	"github.com/akshitanchan/execution-fairness-simulator/internal/eventlog"
	"github.com/akshitanchan/execution-fairness-simulator/internal/latency"
	"github.com/akshitanchan/execution-fairness-simulator/internal/orderbook"
	"github.com/akshitanchan/execution-fairness-simulator/internal/scenario"
	"github.com/akshitanchan/execution-fairness-simulator/internal/trader"
)

type RunResult struct {
	RunID      string           `json:"run_id"`
	Config     *scenario.Config `json:"config"`
	EventCount uint64           `json:"event_count"`
	TradeCount int              `json:"trade_count"`
	Duration   time.Duration    `json:"wall_duration"`
	LogPath    string           `json:"log_path"`
	LogHash    string           `json:"log_hash"`
	OutputDir  string           `json:"output_dir"`
}

type Runner struct {
	cfg       *scenario.Config
	book      *orderbook.Book
	loop      *engine.EventLoop
	logWriter *eventlog.Writer

	fastAgent *trader.Agent
	slowAgent *trader.Agent

	// Current BBO for signal dispatch
	currentBBO *domain.BBO

	// Collected trades
	trades []domain.Trade

	// Output directory
	outputDir string
}

func NewRunner(cfg *scenario.Config, baseOutputDir string) (*Runner, error) {
	runID := fmt.Sprintf("%s_seed%d", cfg.Name, cfg.Seed)
	outputDir := filepath.Join(baseOutputDir, runID)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	logPath := filepath.Join(outputDir, "events.jsonl")
	logWriter, err := eventlog.NewWriter(logPath)
	if err != nil {
		return nil, fmt.Errorf("create event log: %w", err)
	}

	r := &Runner{
		cfg:        cfg,
		book:       orderbook.New(),
		logWriter:  logWriter,
		outputDir:  outputDir,
		currentBBO: &domain.BBO{},
	}

	r.loop = engine.NewEventLoop(r.handleEvent)

	// Create trader agents with deterministic seeds derived from main seed
	fastLat := latency.NewModel(
		latency.MsToNs(cfg.FastTrader.BaseLatencyMs),
		latency.MsToNs(cfg.FastTrader.JitterMs),
		cfg.Seed+1,
	)
	slowLat := latency.NewModel(
		latency.MsToNs(cfg.SlowTrader.BaseLatencyMs),
		latency.MsToNs(cfg.SlowTrader.JitterMs),
		cfg.Seed+2,
	)

	r.fastAgent = trader.NewAgent(cfg.FastTrader.ID, fastLat, cfg.Seed+3, 1_000_000)
	r.slowAgent = trader.NewAgent(cfg.SlowTrader.ID, slowLat, cfg.Seed+4, 2_000_000)

	return r, nil
}

func (r *Runner) Run() (*RunResult, error) {
	startWall := time.Now()

	r.logEvent(&domain.Event{
		Timestamp: 0,
		Type:      domain.EventSimStart,
	})

	gen := scenario.NewGenerator(r.cfg)
	bgEvents := gen.Generate()
	for _, e := range bgEvents {
		r.loop.Schedule(e)
	}

	// Schedule periodic re-quote events for both traders
	reQuoteInterval := r.fastAgent.Strategy.ReQuoteIntervalNs
	if reQuoteInterval > 0 {
		for t := reQuoteInterval; t < r.cfg.Duration; t += reQuoteInterval {
			r.loop.Schedule(&domain.Event{
				Timestamp: t,
				Type:      domain.EventReQuote,
				TraderID:  r.fastAgent.ID,
			})
			r.loop.Schedule(&domain.Event{
				Timestamp: t,
				Type:      domain.EventReQuote,
				TraderID:  r.slowAgent.ID,
			})
		}
	}

	r.loop.Schedule(&domain.Event{
		Timestamp: r.cfg.Duration,
		Type:      domain.EventSimEnd,
	})

	r.loop.Run()

	if err := r.logWriter.Close(); err != nil {
		return nil, fmt.Errorf("close event log: %w", err)
	}

	logPath := filepath.Join(r.outputDir, "events.jsonl")
	hash, err := hashFile(logPath)
	if err != nil {
		return nil, fmt.Errorf("hash log: %w", err)
	}

	cfgPath := filepath.Join(r.outputDir, "config.json")
	cfgData, _ := json.MarshalIndent(r.cfg, "", "  ")
	os.WriteFile(cfgPath, cfgData, 0644)

	tradesPath := filepath.Join(r.outputDir, "trades.json")
	tradesData, _ := json.MarshalIndent(r.trades, "", "  ")
	os.WriteFile(tradesPath, tradesData, 0644)

	lastRunPath := filepath.Join(filepath.Dir(r.outputDir), "last-run")
	os.WriteFile(lastRunPath, []byte(r.outputDir), 0644)

	return &RunResult{
		RunID:      filepath.Base(r.outputDir),
		Config:     r.cfg,
		EventCount: r.loop.EventsProcessed,
		TradeCount: len(r.trades),
		Duration:   time.Since(startWall),
		LogPath:    logPath,
		LogHash:    hash,
		OutputDir:  r.outputDir,
	}, nil
}

func (r *Runner) handleEvent(event *domain.Event) []*domain.Event {
	var newEvents []*domain.Event

	switch event.Type {
	case domain.EventOrderAccepted:
		newEvents = r.handleOrder(event)

	case domain.EventSignal:
		newEvents = r.handleSignal(event)

	case domain.EventReQuote:
		newEvents = r.handleReQuote(event)

	case domain.EventSimStart, domain.EventSimEnd:
		r.logEvent(event)

	case domain.EventTradeExecuted, domain.EventBBOUpdate, domain.EventOrderCanceled:
		// These are logged when produced; no further dispatch needed
	}

	return newEvents
}

func (r *Runner) handleOrder(event *domain.Event) []*domain.Event {
	order := event.Order
	if order == nil {
		return nil
	}

	var newEvents []*domain.Event

	trades, bbo := r.book.ProcessOrder(order, event.Timestamp)

	r.book.AssertInvariants()

	// Record queue position at placement for limit orders that rested
	if order.Type == domain.LimitOrder && order.RemainingQty > 0 {
		order.QueuePos = r.book.QueuePosition(order.ID)
	}

	// Log accepted (after processing so QueuePos is populated)
	r.logEvent(event)

	// Track trader active orders for limit orders that rest
	if order.Type == domain.LimitOrder {
		if a := r.agentFor(order.TraderID); a != nil {
			a.ActiveOrders[order.ID] = order
		}
	}

	if order.Type == domain.CancelOrder {
		cancelEvent := &domain.Event{
			Timestamp: event.Timestamp,
			Type:      domain.EventOrderCanceled,
			Order:     order,
		}
		r.logEvent(cancelEvent)

		if a := r.agentFor(order.TraderID); a != nil {
			a.OnCancelAck(order.CancelID)
		}
	}

	for i := range trades {
		trade := &trades[i]
		r.trades = append(r.trades, *trade)

		tradeEvent := &domain.Event{
			Timestamp: event.Timestamp,
			Type:      domain.EventTradeExecuted,
			Trade:     trade,
		}
		r.logEvent(tradeEvent)

		// Notify agents of fills
		if a := r.agentFor(trade.BuyTrader); a != nil {
			a.OnFill(trade, trade.BuyOrderID)
		}
		if a := r.agentFor(trade.SellTrader); a != nil {
			a.OnFill(trade, trade.SellOrderID)
		}
	}

	// Log BBO update
	if bbo != nil {
		r.currentBBO = bbo
		bboEvent := &domain.Event{
			Timestamp: event.Timestamp,
			Type:      domain.EventBBOUpdate,
			BBO:       bbo,
		}
		r.logEvent(bboEvent)
	}

	return newEvents
}

func (r *Runner) handleSignal(event *domain.Event) []*domain.Event {
	signal := event.Signal
	if signal == nil {
		return nil
	}

	signal.MidPrice = r.currentBBO.MidPrice
	r.logEvent(event)

	var newEvents []*domain.Event
	for _, agent := range []*trader.Agent{r.fastAgent, r.slowAgent} {
		orders := agent.OnSignal(signal, r.currentBBO, event.Timestamp)
		newEvents = append(newEvents, r.scheduleOrders(agent, orders)...)
	}
	return newEvents
}

func (r *Runner) handleReQuote(event *domain.Event) []*domain.Event {
	if r.currentBBO.BidPrice == 0 || r.currentBBO.AskPrice == 0 {
		return nil
	}

	agent := r.agentFor(event.TraderID)
	if agent == nil {
		return nil
	}

	neutralSignal := &domain.Signal{
		Value:    0,
		MidPrice: r.currentBBO.MidPrice,
	}

	orders := agent.OnSignal(neutralSignal, r.currentBBO, event.Timestamp)
	return r.scheduleOrders(agent, orders)
}

func (r *Runner) scheduleOrders(agent *trader.Agent, orders []*domain.Order) []*domain.Event {
	var events []*domain.Event
	for _, order := range orders {
		arrivalTime := agent.Latency.Apply(order.DecisionTime)
		order.ArrivalTime = arrivalTime
		events = append(events, &domain.Event{
			Timestamp: arrivalTime,
			Type:      domain.EventOrderAccepted,
			Order:     order,
		})
	}
	return events
}

func (r *Runner) logEvent(event *domain.Event) {
	if err := r.logWriter.Write(event); err != nil {
		panic(fmt.Sprintf("failed to write event: %v", err))
	}
}

func (r *Runner) agentFor(traderID string) *trader.Agent {
	switch traderID {
	case r.fastAgent.ID:
		return r.fastAgent
	case r.slowAgent.ID:
		return r.slowAgent
	default:
		return nil
	}
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h), nil
}
