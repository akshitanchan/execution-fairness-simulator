# Execution Fairness Report

**Scenario:** spike | **Seed:** 42

## Latency Configuration

| Trader | Base Latency (ms) | Jitter (ms) |
|--------|-------------------|-------------|
| fast   | 1                | 0          |
| slow   | 50               | 10         |

## Execution Metrics

| Metric | Fast | Slow | Delta |
|--------|------|------|-------|
| Orders Sent | 88 | 104 | -16 |
| Limit Orders | 69 | 67 | +2 |
| Market Orders | 1 | 1 | +0 |
| Total Fills | 77 | 77 | +0 |
| Total Qty Filled | 286 | 253 | +33 |
| Fill Rate | 84.2857 | 76.4706 | +7.8151 |
| Avg Exec Price | 99.9966 | 99.9962 | +0.0004 |
| Avg Slippage | -0.0229 | -0.0238 | +0.0009 |
| Slippage (bps) | -2.2867 | -2.3794 | +0.0927 |
| Avg Time-to-Fill (ms) | 178.6551 | 263.2958 | -84.6407 |
| Avg Queue Pos (place) | 4.8696 | 4.9552 | -0.0857 |
| Avg Queue Pos (fill) | 1.0000 | 1.0000 | +0.0000 |
| Adverse Selection (bps) | 1.8377 | 1.4935 | +0.3442 |

## Time-to-Fill Distribution (ms)

| Percentile | Fast | Slow |
|------------|------|------|
| P25 | 68.01 | 122.26 |
| P50 | 158.53 | 223.89 |
| P75 | 244.04 | 412.24 |
| P90 | 365.77 | 467.52 |
| P99 | 516.84 | 621.27 |

## Fairness Analysis

### Message Arrival Ordering

The fast trader's messages arrive **49 ms** earlier than the slow trader's. This means when both traders react to the same signal, the fast trader's order is processed first—securing better queue position at the intended price level.

**Queue position at placement**: fast = 4.9, slow = 5.0. The fast trader consistently joins the queue closer to the front, giving it priority over the slow trader at the same price level.

**Queue position at fill**: fast = 1.0, slow = 1.0. A lower fill queue position means the order was nearer the front when it executed.

### Fill Rate Impact

The fast trader achieved a fill rate **7.8 pp higher** than the slow trader. This gap arises because:
- The fast trader joins the queue earlier, gaining priority over the slow trader at the same price level.
- By the time the slow trader's order arrives, available liquidity may already be consumed.
- Cancel-and-replace operations take effect sooner for the fast trader, reducing stale-order exposure.

### Missed Fills

Orders canceled without any fill — fast: **13**, slow: **28**.
The slow trader missed **15 more fills** due to orders going stale before any contra-side liquidity arrived. The 49 ms additional latency means cancels take longer to process, leaving stale orders exposed. Out of 36 cancels sent by the slow trader, 28 targeted orders that never received a fill.

### Slippage Analysis

Fast trader slippage: **-2.29 bps** | Slow trader slippage: **-2.38 bps** (delta: +0.09 bps)

### Adverse Selection

Fast trader: **1.84 bps** | Slow trader: **1.49 bps**

Adverse selection measures price movement against the trader's position after a fill. The slow trader experiences less adverse selection, likely because it only gets filled when the market doesn't move away—a form of selection bias that reduces fill rate but improves per-fill quality.

### Time-to-Fill

The slow trader's average time-to-fill is **1.5x** that of the fast trader. This reflects both the latency gap itself and the cascading effect: later arrival → worse queue position → longer wait for fills.

### Scenario Context: spike

During burst windows, the rapid increase in market orders and cancellations creates a volatile environment. The fast trader benefits from being able to cancel and re-quote faster during these windows, while the slow trader's stale orders are more exposed to adverse fills.
