package scenario

import (
	"math/rand"
	"sort"

	"github.com/akshitanchan/execution-fairness-simulator/internal/domain"
)

// backgroundGen is the common background order flow generator
type backgroundGen struct {
	cfg    *Config
	rng    *rand.Rand
	nextID uint64
}

func newBackgroundGen(cfg *Config) *backgroundGen {
	return &backgroundGen{
		cfg:    cfg,
		rng:    rand.New(rand.NewSource(cfg.Seed)),
		nextID: 100_000, // background orders start at high IDs to avoid collision
	}
}

func (g *backgroundGen) nextOrderID() uint64 {
	g.nextID++
	return g.nextID
}

func (g *backgroundGen) randSize() int64 {
	p := g.cfg.Scenario
	if p.MaxOrderSize <= p.MinOrderSize {
		return p.MinOrderSize
	}
	return p.MinOrderSize + g.rng.Int63n(p.MaxOrderSize-p.MinOrderSize+1)
}

func (g *backgroundGen) randSide() domain.Side {
	if g.rng.Float64() < 0.5 {
		return domain.Buy
	}
	return domain.Sell
}

// generateInitialBook creates initial resting limit orders to seed the book
func (g *backgroundGen) generateInitialBook() []*domain.Event {
	p := g.cfg.Scenario
	var events []*domain.Event

	halfSpread := p.InitialSpread / 2
	bestBid := p.InitialMidPrice - halfSpread
	bestAsk := p.InitialMidPrice + halfSpread

	// Populate bid levels
	for lvl := 0; lvl < p.MaxPriceLevels; lvl++ {
		price := bestBid - int64(lvl)*p.PriceTickSize
		for i := int64(0); i < p.DepthPerLevel; i++ {
			id := g.nextOrderID()
			order := &domain.Order{
				ID:       id,
				TraderID: "background",
				Side:     domain.Buy,
				Type:     domain.LimitOrder,
				Price:    price,
				Qty:      g.randSize(),
			}
			events = append(events, &domain.Event{
				Timestamp: 0,
				Type:      domain.EventOrderAccepted,
				Order:     order,
			})
		}
	}

	// Populate ask levels
	for lvl := 0; lvl < p.MaxPriceLevels; lvl++ {
		price := bestAsk + int64(lvl)*p.PriceTickSize
		for i := int64(0); i < p.DepthPerLevel; i++ {
			id := g.nextOrderID()
			order := &domain.Order{
				ID:       id,
				TraderID: "background",
				Side:     domain.Sell,
				Type:     domain.LimitOrder,
				Price:    price,
				Qty:      g.randSize(),
			}
			events = append(events, &domain.Event{
				Timestamp: 0,
				Type:      domain.EventOrderAccepted,
				Order:     order,
			})
		}
	}

	return events
}

// generateSignals creates periodic signal events
func (g *backgroundGen) generateSignals() []*domain.Event {
	var events []*domain.Event
	interval := g.cfg.Scenario.SignalIntervalNs
	if interval <= 0 {
		return nil
	}

	for t := interval; t < g.cfg.Duration; t += interval {
		// Signal value is sampled from N(0, 0.5^2)
		value := g.rng.NormFloat64() * 0.5
		events = append(events, &domain.Event{
			Timestamp: t,
			Type:      domain.EventSignal,
			Signal: &domain.Signal{
				Value: value,
			},
		})
	}
	return events
}

// CalmGenerator produces steady-state background order flow
type CalmGenerator struct {
	*backgroundGen
}

func NewCalmGenerator(cfg *Config) *CalmGenerator {
	return &CalmGenerator{backgroundGen: newBackgroundGen(cfg)}
}

func (g *CalmGenerator) Generate() []*domain.Event {
	events := g.generateInitialBook()
	events = append(events, g.generateSignals()...)

	p := g.cfg.Scenario
	var restingIDs []uint64 // track IDs for potential cancels

	for t := p.OrderIntervalNs; t < g.cfg.Duration; t += p.OrderIntervalNs {
		// Small random timing jitter
		jitter := g.rng.Int63n(p.OrderIntervalNs / 2)
		eventTime := t + jitter

		if eventTime >= g.cfg.Duration {
			break
		}

		// Decide: cancel, market, or limit
		roll := g.rng.Float64()

		if roll < p.CancelRate && len(restingIDs) > 0 {
			// Cancel a random resting order
			idx := g.rng.Intn(len(restingIDs))
			cancelID := restingIDs[idx]
			restingIDs = append(restingIDs[:idx], restingIDs[idx+1:]...)

			id := g.nextOrderID()
			events = append(events, &domain.Event{
				Timestamp: eventTime,
				Type:      domain.EventOrderAccepted,
				Order: &domain.Order{
					ID:       id,
					TraderID: "background",
					Type:     domain.CancelOrder,
					CancelID: cancelID,
				},
			})
		} else if roll < p.CancelRate+p.MarketOrderRatio {
			// Market order
			id := g.nextOrderID()
			events = append(events, &domain.Event{
				Timestamp: eventTime,
				Type:      domain.EventOrderAccepted,
				Order: &domain.Order{
					ID:       id,
					TraderID: "background",
					Side:     g.randSide(),
					Type:     domain.MarketOrder,
					Qty:      g.randSize(),
				},
			})
		} else {
			// Limit order near the mid
			id := g.nextOrderID()
			side := g.randSide()
			// Place within a few ticks of mid
			offset := g.rng.Int63n(int64(p.MaxPriceLevels)) * p.PriceTickSize
			var price int64
			if side == domain.Buy {
				price = p.InitialMidPrice - p.InitialSpread/2 - offset
			} else {
				price = p.InitialMidPrice + p.InitialSpread/2 + offset
			}

			order := &domain.Order{
				ID:       id,
				TraderID: "background",
				Side:     side,
				Type:     domain.LimitOrder,
				Price:    price,
				Qty:      g.randSize(),
			}
			events = append(events, &domain.Event{
				Timestamp: eventTime,
				Type:      domain.EventOrderAccepted,
				Order:     order,
			})
			restingIDs = append(restingIDs, id)
		}
	}

	// Sort by timestamp for deterministic processing
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Timestamp < events[j].Timestamp
	})

	return events
}

// ThinGenerator produces low-depth order flow with sporadic sweeps
type ThinGenerator struct {
	*backgroundGen
}

func NewThinGenerator(cfg *Config) *ThinGenerator {
	return &ThinGenerator{backgroundGen: newBackgroundGen(cfg)}
}

func (g *ThinGenerator) Generate() []*domain.Event {
	events := g.generateInitialBook()
	events = append(events, g.generateSignals()...)

	p := g.cfg.Scenario
	var restingIDs []uint64

	for t := p.OrderIntervalNs; t < g.cfg.Duration; t += p.OrderIntervalNs {
		jitter := g.rng.Int63n(p.OrderIntervalNs / 4)
		eventTime := t + jitter
		if eventTime >= g.cfg.Duration {
			break
		}

		roll := g.rng.Float64()

		if roll < p.CancelRate && len(restingIDs) > 0 {
			idx := g.rng.Intn(len(restingIDs))
			cancelID := restingIDs[idx]
			restingIDs = append(restingIDs[:idx], restingIDs[idx+1:]...)

			id := g.nextOrderID()
			events = append(events, &domain.Event{
				Timestamp: eventTime,
				Type:      domain.EventOrderAccepted,
				Order: &domain.Order{
					ID:       id,
					TraderID: "background",
					Type:     domain.CancelOrder,
					CancelID: cancelID,
				},
			})
		} else if roll < p.CancelRate+p.MarketOrderRatio {
			// Sporadic market sweep — larger size to move price
			id := g.nextOrderID()
			sweepSize := g.randSize() * 2 // larger to cause slippage
			events = append(events, &domain.Event{
				Timestamp: eventTime,
				Type:      domain.EventOrderAccepted,
				Order: &domain.Order{
					ID:       id,
					TraderID: "background",
					Side:     g.randSide(),
					Type:     domain.MarketOrder,
					Qty:      sweepSize,
				},
			})
		} else {
			// Limit order — thin depth
			id := g.nextOrderID()
			side := g.randSide()
			offset := g.rng.Int63n(int64(p.MaxPriceLevels)) * p.PriceTickSize
			var price int64
			if side == domain.Buy {
				price = p.InitialMidPrice - p.InitialSpread/2 - offset
			} else {
				price = p.InitialMidPrice + p.InitialSpread/2 + offset
			}

			order := &domain.Order{
				ID:       id,
				TraderID: "background",
				Side:     side,
				Type:     domain.LimitOrder,
				Price:    price,
				Qty:      g.randSize(),
			}
			events = append(events, &domain.Event{
				Timestamp: eventTime,
				Type:      domain.EventOrderAccepted,
				Order:     order,
			})
			restingIDs = append(restingIDs, id)
		}
	}

	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Timestamp < events[j].Timestamp
	})
	return events
}

// SpikeGenerator produces order flow with periodic burst windows
type SpikeGenerator struct {
	*backgroundGen
}

func NewSpikeGenerator(cfg *Config) *SpikeGenerator {
	return &SpikeGenerator{backgroundGen: newBackgroundGen(cfg)}
}

func (g *SpikeGenerator) Generate() []*domain.Event {
	events := g.generateInitialBook()
	events = append(events, g.generateSignals()...)

	p := g.cfg.Scenario
	var restingIDs []uint64

	// Determine burst windows
	type window struct{ start, end int64 }
	var bursts []window
	if p.BurstIntervalNs > 0 && p.BurstWindowNs > 0 {
		for t := p.BurstIntervalNs; t < g.cfg.Duration; t += p.BurstIntervalNs {
			bursts = append(bursts, window{t, t + p.BurstWindowNs})
		}
	}

	inBurst := func(t int64) bool {
		for _, w := range bursts {
			if t >= w.start && t < w.end {
				return true
			}
		}
		return false
	}

	// During bursts, interval is reduced by BurstRate
	t := p.OrderIntervalNs
	for t < g.cfg.Duration {
		interval := p.OrderIntervalNs
		isBurst := inBurst(t)
		if isBurst && p.BurstRate > 0 {
			interval = int64(float64(p.OrderIntervalNs) / p.BurstRate)
			if interval < 1 {
				interval = 1
			}
		}

		jitter := g.rng.Int63n(interval/2 + 1)
		eventTime := t + jitter
		if eventTime >= g.cfg.Duration {
			break
		}

		cancelRate := p.CancelRate
		marketRatio := p.MarketOrderRatio
		if isBurst {
			cancelRate *= p.BurstCancelMul
			marketRatio *= p.BurstMarketMul
			if p.BurstCancelCap > 0 && cancelRate > p.BurstCancelCap {
				cancelRate = p.BurstCancelCap
			}
			if p.BurstMarketCap > 0 && marketRatio > p.BurstMarketCap {
				marketRatio = p.BurstMarketCap
			}
		}

		roll := g.rng.Float64()

		if roll < cancelRate && len(restingIDs) > 0 {
			idx := g.rng.Intn(len(restingIDs))
			cancelID := restingIDs[idx]
			restingIDs = append(restingIDs[:idx], restingIDs[idx+1:]...)

			id := g.nextOrderID()
			events = append(events, &domain.Event{
				Timestamp: eventTime,
				Type:      domain.EventOrderAccepted,
				Order: &domain.Order{
					ID:       id,
					TraderID: "background",
					Type:     domain.CancelOrder,
					CancelID: cancelID,
				},
			})
		} else if roll < cancelRate+marketRatio {
			id := g.nextOrderID()
			size := g.randSize()
			if isBurst && p.BurstSizeMul > 0 {
				size = int64(float64(size) * p.BurstSizeMul)
			}
			events = append(events, &domain.Event{
				Timestamp: eventTime,
				Type:      domain.EventOrderAccepted,
				Order: &domain.Order{
					ID:       id,
					TraderID: "background",
					Side:     g.randSide(),
					Type:     domain.MarketOrder,
					Qty:      size,
				},
			})
		} else {
			id := g.nextOrderID()
			side := g.randSide()
			offset := g.rng.Int63n(int64(p.MaxPriceLevels)) * p.PriceTickSize
			var price int64
			if side == domain.Buy {
				price = p.InitialMidPrice - p.InitialSpread/2 - offset
			} else {
				price = p.InitialMidPrice + p.InitialSpread/2 + offset
			}

			order := &domain.Order{
				ID:       id,
				TraderID: "background",
				Side:     side,
				Type:     domain.LimitOrder,
				Price:    price,
				Qty:      g.randSize(),
			}
			events = append(events, &domain.Event{
				Timestamp: eventTime,
				Type:      domain.EventOrderAccepted,
				Order:     order,
			})
			restingIDs = append(restingIDs, id)
		}

		t += interval
	}

	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Timestamp < events[j].Timestamp
	})
	return events
}

// NewGenerator creates the appropriate generator for a config
func NewGenerator(cfg *Config) Generator {
	switch cfg.Name {
	case "calm":
		return NewCalmGenerator(cfg)
	case "thin":
		return NewThinGenerator(cfg)
	case "spike":
		return NewSpikeGenerator(cfg)
	default:
		return NewCalmGenerator(cfg) // fallback
	}
}
