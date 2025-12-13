# Execution Fairness Report

**Scenario:** thin | **Seed:** 42

## Latency Configuration

| Trader | Base Latency (ms) | Jitter (ms) |
|--------|-------------------|-------------|
| fast   | 1                | 0          |
| slow   | 50               | 10         |

## Execution Metrics

| Metric | Fast | Slow | Delta |
|--------|------|------|-------|
| Orders Sent | 90 | 97 | -7 |
| Limit Orders | 57 | 45 | +12 |
| Market Orders | 3 | 3 | +0 |
| Total Fills | 65 | 40 | +25 |
| Total Qty Filled | 204 | 121 | +83 |
| Fill Rate | 70.0000 | 54.1667 | +15.8333 |
| Avg Exec Price | 99.9971 | 100.0011 | -0.0040 |
| Avg Slippage | -0.0225 | -0.0200 | -0.0025 |
| Slippage (bps) | -2.2500 | -1.9959 | -0.2541 |
| Avg Time-to-Fill (ms) | 301.4727 | 367.4258 | -65.9530 |
| Avg Queue Pos (place) | 5.2281 | 6.3556 | -1.1275 |
| Avg Queue Pos (fill) | 1.0000 | 1.0000 | +0.0000 |
| Adverse Selection (bps) | 2.2154 | 1.6750 | +0.5404 |

## Time-to-Fill Distribution (ms)

| Percentile | Fast | Slow |
|------------|------|------|
| P25 | 181.60 | 81.72 |
| P50 | 322.70 | 402.57 |
| P75 | 481.97 | 561.66 |
| P90 | 543.29 | 622.32 |
| P99 | 568.61 | 643.05 |

## Fairness Analysis

### Message Arrival Ordering

The fast trader's messages arrive **49 ms** earlier than the slow trader's. This means when both traders react to the same signal, the fast trader's order is processed first—securing better queue position at the intended price level.

**Queue position at placement**: fast = 5.2, slow = 6.4. The fast trader consistently joins the queue closer to the front, giving it priority over the slow trader at the same price level.

**Queue position at fill**: fast = 1.0, slow = 1.0. A lower fill queue position means the order was nearer the front when it executed.

### Fill Rate Impact

The fast trader achieved a fill rate **15.8 pp higher** than the slow trader. This gap arises because:
- The fast trader joins the queue earlier, gaining priority over the slow trader at the same price level.
- By the time the slow trader's order arrives, available liquidity may already be consumed.
- Cancel-and-replace operations take effect sooner for the fast trader, reducing stale-order exposure.

### Missed Fills

Orders canceled without any fill — fast: **22**, slow: **32**.
The slow trader missed **10 more fills** due to orders going stale before any contra-side liquidity arrived. The 49 ms additional latency means cancels take longer to process, leaving stale orders exposed. Out of 49 cancels sent by the slow trader, 32 targeted orders that never received a fill.

### Slippage Analysis

Fast trader slippage: **-2.25 bps** | Slow trader slippage: **-2.00 bps** (delta: -0.25 bps)

### Adverse Selection

Fast trader: **2.22 bps** | Slow trader: **1.68 bps**

Adverse selection measures price movement against the trader's position after a fill. The slow trader experiences less adverse selection, likely because it only gets filled when the market doesn't move away—a form of selection bias that reduces fill rate but improves per-fill quality.

### Time-to-Fill

The slow trader's average time-to-fill is **1.2x** that of the fast trader. This reflects both the latency gap itself and the cascading effect: later arrival → worse queue position → longer wait for fills.

### Scenario Context: thin

A thin book magnifies the latency advantage. With limited depth at top levels, the fast trader captures scarce liquidity. Sporadic market sweeps create opportunities that are disproportionately captured by the faster trader.
