# Execution Fairness Simulator

Deterministic exchange simulator that measures how latency affects execution quality under price-time priority. Compares two traders running identical strategies with different latency profiles and generates a fairness report.

## Requirements

- Go 1.24+

## Quick Start

```bash
make build

# Run all scenarios
make demo

# Or run one at a time
./fairsim run --scenario calm --seed 42
./fairsim run --scenario spike --seed 42

# View report
./fairsim report --last-run

# Tests
make test
```

## Scenarios

Three built-in market regimes:

- **Calm** — tight spread ($0.02), deep book, steady flow. Baseline for comparison.
- **Thin** — wide spread ($0.05), sparse book, higher cancel rate. Amplifies queue-position effects.
- **Spike** — periodic burst windows (500ms every 2s) where order rate quadruples and cancels double.

All run for 10 simulated seconds with the same initial mid price ($100.00).

## Metrics

Per-trader, computed from the event log:

| Metric | Description |
|--------|-------------|
| Fill Rate | Filled orders ÷ executable orders |
| Slippage (bps) | Exec price vs mid at decision time |
| Time-to-Fill | Distribution of fill latencies |
| Adverse Selection | Price move against position, 100ms post-fill |
| Queue Position | Average position at placement and at fill |

## Output

Each run writes to `runs/<run_id>/`:

- `events.jsonl` — full event log
- `config.json`, `trades.json`, `metrics.json` — structured data
- `report.md` — markdown fairness report
- `plots.txt` — ASCII histograms and CDFs

## Determinism

Same `seed + scenario` reproduces identical output, verified by SHA-256 hash of the event log. Achieved via single-threaded event loop, seeded RNG, sorted map iteration, and fixed-point pricing (int64 × 10⁴).
