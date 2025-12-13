# Execution Fairness Report

**Scenario:** calm | **Seed:** 42

## Latency Configuration

| Trader | Base Latency (ms) | Jitter (ms) |
|--------|-------------------|-------------|
| fast   | 1                | 0          |
| slow   | 50               | 10         |

## Execution Metrics

| Metric | Fast | Slow | Delta |
|--------|------|------|-------|
| Orders Sent | 96 | 104 | -8 |
| Limit Orders | 47 | 46 | +1 |
| Market Orders | 0 | 0 | +0 |
| Total Fills | 26 | 24 | +2 |
| Total Qty Filled | 84 | 67 | +17 |
| Fill Rate | 40.4255 | 32.6087 | +7.8168 |
| Avg Exec Price | 100.0010 | 100.0025 | -0.0016 |
| Avg Slippage | -0.0103 | -0.0100 | -0.0003 |
| Slippage (bps) | -1.0298 | -1.0000 | -0.0298 |
| Avg Time-to-Fill (ms) | 466.7556 | 530.1796 | -63.4241 |
| Avg Queue Pos (place) | 19.6383 | 20.2826 | -0.6443 |
| Avg Queue Pos (fill) | 1.0000 | 1.0000 | +0.0000 |
| Adverse Selection (bps) | 0.9615 | 1.0000 | -0.0385 |

## Time-to-Fill Distribution (ms)

| Percentile | Fast | Slow |
|------------|------|------|
| P25 | 425.44 | 481.97 |
| P50 | 461.15 | 578.07 |
| P75 | 541.87 | 616.49 |
| P90 | 580.94 | 631.08 |
| P99 | 587.39 | 647.99 |

## Fairness Analysis

### Message Arrival Ordering

The fast trader's messages arrive **49 ms** earlier than the slow trader's. This means when both traders react to the same signal, the fast trader's order is processed first—securing better queue position at the intended price level.

**Queue position at placement**: fast = 19.6, slow = 20.3. The fast trader consistently joins the queue closer to the front, giving it priority over the slow trader at the same price level.

**Queue position at fill**: fast = 1.0, slow = 1.0. A lower fill queue position means the order was nearer the front when it executed.

### Fill Rate Impact

The fast trader achieved a fill rate **7.8 pp higher** than the slow trader. This gap arises because:
- The fast trader joins the queue earlier, gaining priority over the slow trader at the same price level.
- By the time the slow trader's order arrives, available liquidity may already be consumed.
- Cancel-and-replace operations take effect sooner for the fast trader, reducing stale-order exposure.

### Missed Fills

Orders canceled without any fill — fast: **45**, slow: **44**.
Both traders show similar missed-fill counts in this scenario.

### Slippage Analysis

Fast trader slippage: **-1.03 bps** | Slow trader slippage: **-1.00 bps** (delta: -0.03 bps)

### Adverse Selection

Fast trader: **0.96 bps** | Slow trader: **1.00 bps**

Adverse selection measures price movement against the trader's position after a fill. Both traders face similar adverse selection, indicating that post-fill price dynamics are not strongly correlated with arrival timing in this scenario.

### Time-to-Fill

The slow trader's average time-to-fill is **1.1x** that of the fast trader. This reflects both the latency gap itself and the cascading effect: later arrival → worse queue position → longer wait for fills.

### Scenario Context: calm

In a calm market with stable mid and tight spread, latency advantages manifest primarily through queue position. The deep book means fills are available for both traders, but the fast trader consistently executes first.
