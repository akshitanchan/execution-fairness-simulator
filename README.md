# Execution Fairness Simulator

A deterministic exchange simulation that quantifies how latency affects execution outcomes under price-time priority. Produces a "fairness report" comparing identical strategies run with different latency profiles, backed by queue position and event-log evidence.

## Requirements

- Go 1.24+ (validated with Go 1.24.x)

## Quick Start

```bash
# Build
make build

# Run all scenarios and generate report
make demo

# Or run individually
./fairsim run --scenario calm --seed 42
./fairsim run --scenario thin --seed 42
./fairsim run --scenario spike --seed 42

# View the report for the last run
./fairsim report --last-run

# Run tests
make test
```

## Matching Rules

The order book implements **price-time priority**:

| Rule | Description |
|------|-------------|
| **Price priority** | Better-priced orders execute first. Bids sorted descending, asks ascending. |
| **Time priority** | At the same price, orders are filled in FIFO (insertion) order. |
| **Partial fills** | Supported (partially filled orders remain on the book). |
| **Market orders** | Sweep the opposite side until filled or book is empty. |
| **Limit orders** | Match aggressively first, then rest if any quantity remains. |
| **Cancels** | Remove remaining quantity; previously filled quantity is unaffected. |

**Invariants** (asserted after every order):
- `best_bid < best_ask` (crossed books resolved by matching)
- Total volume at each level = sum of resting orders
- No negative remaining quantities
- No empty price levels on the book

## Latency Model

Each trader has:
- `base_latency_ms` - Fixed propagation delay
- `jitter_ms` - Uniform random jitter `[0, jitter_ms)` from a seeded RNG

```
arrival_time = decision_time + base_latency + uniform(0, jitter)
```

Both traders receive the same signal at the same time. Their response orders are delayed by their individual latency before reaching the exchange. Message ordering is fully deterministic given the seed.

**Default Configuration:**

| Trader | Base Latency | Jitter |
|--------|-------------|--------|
| fast   | 1 ms        | 0 ms   |
| slow   | 50 ms       | 10 ms  |

## Scenarios

### Calm Market
Stable mid-price, tight spread, steady order flow.

| Parameter | Value |
|-----------|-------|
| Initial Mid | $100.00 |
| Spread | $0.02 |
| Order Interval | 5 ms |
| Market Order Ratio | 15% |
| Cancel Rate | 10% |
| Book Depth | 5 levels × 20 orders |
| Duration | 10 seconds |

### Thin Book
Low depth, sporadic sweeps, higher slippage risk.

| Parameter | Value |
|-----------|-------|
| Initial Mid | $100.00 |
| Spread | $0.05 |
| Order Interval | 20 ms |
| Market Order Ratio | 25% |
| Cancel Rate | 15% |
| Book Depth | 3 levels × 5 orders |
| Duration | 10 seconds |

### Burst / Spike
Periodic windows of intense activity, rapid cancels.

| Parameter | Value |
|-----------|-------|
| Initial Mid | $100.00 |
| Spread | $0.03 |
| Order Interval | 8 ms (÷4 during bursts) |
| Market Order Ratio | 20% (×2 during bursts) |
| Cancel Rate | 25% (×2 during bursts) |
| Burst Window | 500 ms every 2000 ms |
| Book Depth | 5 levels × 15 orders |
| Duration | 10 seconds |

## Strategy

Both traders run the same strategy for fair comparison:

1. **Post at best bid/ask** - Place limit orders at the current best price
2. **Cancel stale orders** - Cancel unfilled orders after 500 ms timeout
3. **Cross on strong signal** - Submit a market order when signal exceeds threshold (±1.0)

The strategy is intentionally simple because the goal is measuring latency impact, not alpha.

## Metrics

Per-trader metrics computed from the event log:

| Metric | Description |
|--------|-------------|
| Fill Rate | Filled executable orders ÷ executable orders (order-level, 0-100%) |
| Avg Exec Price | Volume-weighted average execution price |
| Slippage (bps) | Execution price vs mid at decision time |
| Time-to-Fill | Distribution of fill latencies in ms |
| Adverse Selection | Price movement against position, 100ms post-fill |

## Report Output

Each run produces in `runs/<run_id>/`:

| File | Contents |
|------|----------|
| `events.jsonl` | Append-only event log (all order accepts, trades, BBO updates) |
| `config.json` | Full scenario configuration |
| `trades.json` | All executed trades |
| `metrics.json` | Per-trader computed metrics |
| `report.md` | Markdown fairness report with tables and analysis |
| `plots.txt` | ASCII histograms and CDF plots |

## Determinism

A single `seed + scenario` reproduces:
- The identical event log (verified by SHA-256 hash)
- Identical fills and metrics (bit-for-bit float equality)
- The same report

This is achieved by:
- Single-threaded event loop (no goroutines)
- All randomness from seeded `math/rand`
- Sorted iteration over maps (no reliance on Go map order)
- Fixed-point prices (`int64 × 10⁴`) avoiding float comparison issues

## Project Structure

```
cmd/fairsim/         CLI entry point
internal/
  domain/            Core types: Order, Trade, Event, enums
  orderbook/         Limit order book with price-time priority matching
  engine/            Discrete-event loop (min-heap priority queue)
  eventlog/          Append-only JSONL writer/reader
  latency/           Deterministic latency + jitter model
  scenario/          Scenario parameters + generators (calm/thin/spike)
  trader/            Agent shell + post-at-best strategy
  sim/               Simulation orchestrator
  metrics/           Per-trader metrics collector
  report/            Markdown/text report + ASCII plots + explanation
test/                Determinism + integration tests
```

## Design Notes

See [docs/design.md](docs/design.md) for tradeoff analysis.
