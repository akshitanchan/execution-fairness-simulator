# Design Notes

## What's Modeled

- **Single-instrument limit order book** with correct price-time priority
- **Latency as a first-class parameter** — configurable per-trader base + jitter
- **Three distinct market regimes** (calm, thin, spike) with different depth/volatility
- **Deterministic replay** — same seed always produces same results
- **Continuous invariant checking** — book correctness asserted after every order

## What's Intentionally Not Modeled

| Omission | Reason |
|----------|--------|
| Multi-venue / SOR | Adds complexity without changing the core fairness mechanism |
| Network topology / TCP | Latency is a simple delay model, not a network simulation |
| Multiple instruments | One instrument is sufficient to demonstrate queue effects |
| Realistic market microstructure | This is a mechanism study, not a market simulation |
| Order types beyond limit/market/cancel | IOC, FOK, stop orders add complexity without fairness insight |
| Self-trade prevention | Background traders have a single ID; agents don't cross each other |
| Regulatory constraints | No regulation modeling (circuit breakers, short-sell rules, etc.) |

## Key Tradeoffs

### Single-threaded event loop vs goroutines
We chose a single-threaded discrete-event loop over a concurrent design. This guarantees determinism without locks or barriers. The performance cost is negligible for our simulation scale (~2K-3K events in 10 simulated seconds).

### Fixed-point prices vs float64
Using `int64 × 10,000` for prices avoids float comparison bugs in the order book. The tradeoff is slightly more verbose price arithmetic, but correctness is non-negotiable in a matching engine.

### Seeded RNG per component
Each component (fast trader, slow trader, background generator, latency models) gets its own RNG seeded from `global_seed + offset`. This means adding or removing one component doesn't change the random sequence of others.

### ASCII plots vs gonum/plot
We chose ASCII plots over Go plotting libraries to avoid CGo dependencies and keep the binary portable. The tradeoff is less visual polish, but the data is also available as JSON for custom visualization.

### Strategy simplicity
The strategy (post-at-best + timeout cancel + market cross) is deliberately simple. Complex strategies would obscure the latency effect we're measuring. The strategy produces clear queue effects: both traders compete for the same price levels, and the one who arrives first gets filled first.

## Validation Approach

1. **Unit tests** — 10 order book tests cover FIFO, multi-level sweeps, cancels, invariants
2. **Event loop tests** — Ordering, same-timestamp FIFO, event generation
3. **Latency tests** — Determinism, bounds checking
4. **Scenario tests** — Non-empty flow, timestamp ordering, burst detection
5. **Determinism tests** — SHA-256 hash equality across runs
6. **Integration tests** — Full scenario runs with metric validation
7. **Latency evidence tests** — Measurable differences in at least 2 of 3 scenarios
