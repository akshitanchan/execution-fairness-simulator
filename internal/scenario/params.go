// Package scenario defines scenario parameters and the generator interface
package scenario

import (
	"github.com/akshitanchan/execution-fairness-simulator/internal/domain"
	"github.com/akshitanchan/execution-fairness-simulator/internal/latency"
)

// Config holds all parameters for a simulation run
type Config struct {
	Name     string `json:"name"`
	Seed     int64  `json:"seed"`
	Duration int64  `json:"duration_ns"` // total simulation duration in nanos

	// Trader configs
	FastTrader  TraderConfig `json:"fast_trader"`
	SlowTrader  TraderConfig `json:"slow_trader"`

	// Scenario-specific parameters
	Scenario ScenarioParams `json:"scenario"`
}

// TraderConfig holds trader-specific parameters
type TraderConfig struct {
	ID           string `json:"id"`
	BaseLatencyMs int64 `json:"base_latency_ms"`
	JitterMs      int64 `json:"jitter_ms"`
}

// ScenarioParams holds background order flow parameters
type ScenarioParams struct {
	InitialMidPrice     int64   `json:"initial_mid_price"`    // fixed-point
	InitialSpread       int64   `json:"initial_spread"`       // fixed-point
	OrderIntervalNs     int64   `json:"order_interval_ns"`    // mean inter-arrival
	MarketOrderRatio    float64 `json:"market_order_ratio"`   // fraction of orders that are market
	CancelRate          float64 `json:"cancel_rate"`          // probability of cancel per interval
	MinOrderSize        int64   `json:"min_order_size"`
	MaxOrderSize        int64   `json:"max_order_size"`
	PriceTickSize       int64   `json:"price_tick_size"`
	MaxPriceLevels      int     `json:"max_price_levels"`     // how many levels to populate
	SignalIntervalNs    int64   `json:"signal_interval_ns"`   // how often signals fire

	// Thin-book specific
	DepthPerLevel int64 `json:"depth_per_level,omitempty"`

	// Spike specific
	BurstWindowNs   int64   `json:"burst_window_ns,omitempty"`
	BurstIntervalNs int64   `json:"burst_interval_ns,omitempty"`
	BurstRate       float64 `json:"burst_rate,omitempty"`       // multiplier on arrival rate during bursts
	BurstCancelMul  float64 `json:"burst_cancel_mul,omitempty"` // cancel rate multiplier during bursts
	BurstMarketMul  float64 `json:"burst_market_mul,omitempty"` // market order ratio multiplier during bursts
	BurstSizeMul    float64 `json:"burst_size_mul,omitempty"`   // order size multiplier during bursts
	BurstCancelCap  float64 `json:"burst_cancel_cap,omitempty"` // max cancel rate during bursts
	BurstMarketCap  float64 `json:"burst_market_cap,omitempty"` // max market ratio during bursts
}

// Generator produces background order flow events
type Generator interface {
	// Generate returns all background events for the scenario duration
	Generate() []*domain.Event
}

// DefaultCalm returns the default configuration for a calm market scenario
func DefaultCalm(seed int64) *Config {
	return &Config{
		Name:     "calm",
		Seed:     seed,
		Duration: latency.MsToNs(10_000), // 10 seconds
		FastTrader: TraderConfig{
			ID:            "fast",
			BaseLatencyMs: 1,
			JitterMs:      0,
		},
		SlowTrader: TraderConfig{
			ID:            "slow",
			BaseLatencyMs: 50,
			JitterMs:      10,
		},
		Scenario: ScenarioParams{
			InitialMidPrice:  domain.FloatToPrice(100.0),
			InitialSpread:    domain.FloatToPrice(0.02),
			OrderIntervalNs:  latency.MsToNs(5),
			MarketOrderRatio: 0.15,
			CancelRate:       0.10,
			MinOrderSize:     1,
			MaxOrderSize:     10,
			PriceTickSize:    domain.FloatToPrice(0.01),
			MaxPriceLevels:   5,
			SignalIntervalNs: latency.MsToNs(200),
			DepthPerLevel:    20,
		},
	}
}

// DefaultThin returns the default configuration for a thin book scenario
func DefaultThin(seed int64) *Config {
	return &Config{
		Name:     "thin",
		Seed:     seed,
		Duration: latency.MsToNs(10_000),
		FastTrader: TraderConfig{
			ID:            "fast",
			BaseLatencyMs: 1,
			JitterMs:      0,
		},
		SlowTrader: TraderConfig{
			ID:            "slow",
			BaseLatencyMs: 50,
			JitterMs:      10,
		},
		Scenario: ScenarioParams{
			InitialMidPrice:  domain.FloatToPrice(100.0),
			InitialSpread:    domain.FloatToPrice(0.05),
			OrderIntervalNs:  latency.MsToNs(20),
			MarketOrderRatio: 0.25,
			CancelRate:       0.15,
			MinOrderSize:     1,
			MaxOrderSize:     5,
			PriceTickSize:    domain.FloatToPrice(0.01),
			MaxPriceLevels:   3,
			SignalIntervalNs: latency.MsToNs(200),
			DepthPerLevel:    5,
		},
	}
}

// DefaultSpike returns the default configuration for a burst/spike scenario
func DefaultSpike(seed int64) *Config {
	return &Config{
		Name:     "spike",
		Seed:     seed,
		Duration: latency.MsToNs(10_000),
		FastTrader: TraderConfig{
			ID:            "fast",
			BaseLatencyMs: 1,
			JitterMs:      0,
		},
		SlowTrader: TraderConfig{
			ID:            "slow",
			BaseLatencyMs: 50,
			JitterMs:      10,
		},
		Scenario: ScenarioParams{
			InitialMidPrice:  domain.FloatToPrice(100.0),
			InitialSpread:    domain.FloatToPrice(0.03),
			OrderIntervalNs:  latency.MsToNs(8),
			MarketOrderRatio: 0.20,
			CancelRate:       0.25,
			MinOrderSize:     1,
			MaxOrderSize:     15,
			PriceTickSize:    domain.FloatToPrice(0.01),
			MaxPriceLevels:   5,
			SignalIntervalNs: latency.MsToNs(150),
			DepthPerLevel:    15,
			BurstWindowNs:    latency.MsToNs(500),
			BurstIntervalNs:  latency.MsToNs(2000),
			BurstRate:        4.0,
			BurstCancelMul:   2.0,
			BurstMarketMul:   2.0,
			BurstSizeMul:     2.0,
			BurstCancelCap:   0.5,
			BurstMarketCap:   0.6,
		},
	}
}

// GetConfig returns the default config for a named scenario
func GetConfig(name string, seed int64) *Config {
	switch name {
	case "calm":
		return DefaultCalm(seed)
	case "thin":
		return DefaultThin(seed)
	case "spike":
		return DefaultSpike(seed)
	default:
		return nil
	}
}
