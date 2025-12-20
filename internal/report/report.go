// Package report generates the fairness comparison report
package report

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/akshitanchan/execution-fairness-simulator/internal/domain"
	"github.com/akshitanchan/execution-fairness-simulator/internal/metrics"
	"github.com/akshitanchan/execution-fairness-simulator/internal/scenario"
)

// Report generates and writes the fairness report
type Report struct {
	config *scenario.Config
	fast   *metrics.TraderMetrics
	slow   *metrics.TraderMetrics
	outDir string
}

// NewReport creates a report generator
func NewReport(cfg *scenario.Config, metricsMap map[string]*metrics.TraderMetrics, outDir string) *Report {
	return &Report{
		config: cfg,
		fast:   metricsMap[cfg.FastTrader.ID],
		slow:   metricsMap[cfg.SlowTrader.ID],
		outDir: outDir,
	}
}

// Generate produces the full report
func (r *Report) Generate() error {
	// Save metrics as JSON
	metricsPath := filepath.Join(r.outDir, "metrics.json")
	metricsData, _ := json.MarshalIndent(map[string]*metrics.TraderMetrics{
		"fast": r.fast,
		"slow": r.slow,
	}, "", "  ")
	if err := os.WriteFile(metricsPath, metricsData, 0644); err != nil {
		return fmt.Errorf("write metrics: %w", err)
	}

	// Generate text/markdown report
	reportPath := filepath.Join(r.outDir, "report.md")
	content := r.renderMarkdown()
	if err := os.WriteFile(reportPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}

	// Generate ASCII plots in a separate plots.txt artifact
	plotPath := filepath.Join(r.outDir, "plots.txt")
	plots := r.renderPlots()
	if err := os.WriteFile(plotPath, []byte(plots), 0644); err != nil {
		return fmt.Errorf("write plots: %w", err)
	}

	return nil
}

func (r *Report) renderMarkdown() string {
	var sb strings.Builder

	sb.WriteString("# Execution Fairness Report\n\n")
	sb.WriteString(fmt.Sprintf("**Scenario:** %s | **Seed:** %d\n\n", r.config.Name, r.config.Seed))

	// Latency config table
	sb.WriteString("## Latency Configuration\n\n")
	sb.WriteString("| Trader | Base Latency (ms) | Jitter (ms) |\n")
	sb.WriteString("|--------|-------------------|-------------|\n")
	sb.WriteString(fmt.Sprintf("| fast   | %d                | %d          |\n",
		r.config.FastTrader.BaseLatencyMs, r.config.FastTrader.JitterMs))
	sb.WriteString(fmt.Sprintf("| slow   | %d               | %d         |\n\n",
		r.config.SlowTrader.BaseLatencyMs, r.config.SlowTrader.JitterMs))

	// Side-by-side metrics
	sb.WriteString("## Execution Metrics\n\n")
	sb.WriteString("| Metric | Fast | Slow | Delta |\n")
	sb.WriteString("|--------|------|------|-------|\n")

	if r.fast != nil && r.slow != nil {
		r.addRow(&sb, "Orders Sent", float64(r.fast.OrdersSent), float64(r.slow.OrdersSent), false)
		r.addRow(&sb, "Limit Orders", float64(r.fast.LimitOrders), float64(r.slow.LimitOrders), false)
		r.addRow(&sb, "Market Orders", float64(r.fast.MarketOrders), float64(r.slow.MarketOrders), false)
		r.addRow(&sb, "Total Fills", float64(r.fast.TotalFills), float64(r.slow.TotalFills), false)
		r.addRow(&sb, "Total Qty Filled", float64(r.fast.TotalQtyFilled), float64(r.slow.TotalQtyFilled), false)
		r.addRow(&sb, "Fill Rate", r.fast.FillRate*100, r.slow.FillRate*100, true)
		r.addRow(&sb, "Avg Exec Price", r.fast.AvgExecPrice, r.slow.AvgExecPrice, true)
		r.addRow(&sb, "Avg Slippage", r.fast.AvgSlippage, r.slow.AvgSlippage, true)
		r.addRow(&sb, "Slippage (bps)", r.fast.SlippageBps, r.slow.SlippageBps, true)
		r.addRow(&sb, "Avg Time-to-Fill (ms)", r.fast.AvgTimeToFillNs, r.slow.AvgTimeToFillNs, true)
		r.addRow(&sb, "Avg Queue Pos (place)", r.fast.AvgQueuePosPlace, r.slow.AvgQueuePosPlace, true)
		r.addRow(&sb, "Avg Queue Pos (fill)", r.fast.AvgQueuePosFill, r.slow.AvgQueuePosFill, true)
		r.addRow(&sb, "Adverse Selection (bps)", r.fast.AdverseSelectionBps, r.slow.AdverseSelectionBps, true)
	}
	sb.WriteString("\n")

	// Time-to-fill distribution summary
	sb.WriteString("## Time-to-Fill Distribution (ms)\n\n")
	sb.WriteString("| Percentile | Fast | Slow |\n")
	sb.WriteString("|------------|------|------|\n")
	if r.fast != nil && r.slow != nil {
		for _, p := range []float64{0.25, 0.50, 0.75, 0.90, 0.99} {
			fv := percentile(r.fast.TimeToFillDist, p)
			sv := percentile(r.slow.TimeToFillDist, p)
			sb.WriteString(fmt.Sprintf("| P%.0f | %.2f | %.2f |\n", p*100, fv, sv))
		}
	}
	sb.WriteString("\n")

	// Explanation section
	sb.WriteString("## Fairness Analysis\n\n")
	sb.WriteString(r.generateExplanation())

	return sb.String()
}

func (r *Report) addRow(sb *strings.Builder, label string, fast, slow float64, isFloat bool) {
	delta := fast - slow
	var fmtStr string
	if isFloat {
		fmtStr = "| %s | %.4f | %.4f | %+.4f |\n"
	} else {
		fmtStr = "| %s | %.0f | %.0f | %+.0f |\n"
	}
	sb.WriteString(fmt.Sprintf(fmtStr, label, fast, slow, delta))
}

func (r *Report) generateExplanation() string {
	var sb strings.Builder

	if r.fast == nil || r.slow == nil {
		sb.WriteString("Insufficient data to generate explanation.\n")
		return sb.String()
	}

	// 1. Arrival order differences
	sb.WriteString("### Message Arrival Ordering\n\n")
	latencyDiff := r.config.SlowTrader.BaseLatencyMs - r.config.FastTrader.BaseLatencyMs
	sb.WriteString(fmt.Sprintf("The fast trader's messages arrive **%d ms** earlier than the slow trader's. ",
		latencyDiff))
	sb.WriteString("This means when both traders react to the same signal, the fast trader's order is ")
	sb.WriteString("processed first—securing better queue position at the intended price level.\n\n")

	// Queue position data
	if r.fast.AvgQueuePosPlace > 0 || r.slow.AvgQueuePosPlace > 0 {
		sb.WriteString(fmt.Sprintf("**Queue position at placement**: fast = %.1f, slow = %.1f. ",
			r.fast.AvgQueuePosPlace, r.slow.AvgQueuePosPlace))
		if r.fast.AvgQueuePosPlace < r.slow.AvgQueuePosPlace {
			sb.WriteString("The fast trader consistently joins the queue closer to the front, ")
			sb.WriteString("giving it priority over the slow trader at the same price level.\n\n")
		} else {
			sb.WriteString("Queue positions are similar, suggesting depth absorbs the latency difference in this scenario.\n\n")
		}
	}
	if r.fast.AvgQueuePosFill > 0 || r.slow.AvgQueuePosFill > 0 {
		sb.WriteString(fmt.Sprintf("**Queue position at fill**: fast = %.1f, slow = %.1f. ",
			r.fast.AvgQueuePosFill, r.slow.AvgQueuePosFill))
		sb.WriteString("A lower fill queue position means the order was nearer the front when it executed.\n\n")
	}

	// 2. Fill rate analysis
	sb.WriteString("### Fill Rate Impact\n\n")
	fillDelta := (r.fast.FillRate - r.slow.FillRate) * 100
	if math.Abs(fillDelta) > 1.0 {
		if fillDelta > 0 {
			sb.WriteString(fmt.Sprintf("The fast trader achieved a fill rate **%.1f pp higher** than the slow trader. ", fillDelta))
			sb.WriteString("This gap arises because:\n")
			sb.WriteString("- The fast trader joins the queue earlier, gaining priority over the slow trader at the same price level.\n")
			sb.WriteString("- By the time the slow trader's order arrives, available liquidity may already be consumed.\n")
			sb.WriteString("- Cancel-and-replace operations take effect sooner for the fast trader, reducing stale-order exposure.\n\n")
		} else {
			sb.WriteString(fmt.Sprintf("The fast trader's fill rate is **%.1f pp lower** than the slow trader in this run. ", math.Abs(fillDelta)))
			sb.WriteString("Despite faster arrival on average, fill-rate inversions can occur when:\n")
			sb.WriteString("- The slow trader gets fewer but more selectively timed fills.\n")
			sb.WriteString("- Cancel/replace timing changes which resting orders remain eligible during liquidity bursts.\n")
			sb.WriteString("- Queue position advantages show up more strongly in time-to-fill or size-filled metrics than in filled-order ratio.\n\n")
		}
	} else {
		sb.WriteString(fmt.Sprintf("Fill rates are similar (delta: %.1f pp), suggesting the scenario's depth ", fillDelta))
		sb.WriteString("was sufficient to absorb both traders' orders most of the time.\n\n")
	}

	// Missed fills analysis
	sb.WriteString("### Missed Fills\n\n")
	sb.WriteString(fmt.Sprintf("Orders canceled without any fill — fast: **%d**, slow: **%d**.\n",
		r.fast.CanceledBeforeFill, r.slow.CanceledBeforeFill))
	if r.slow.CanceledBeforeFill > r.fast.CanceledBeforeFill {
		diff := r.slow.CanceledBeforeFill - r.fast.CanceledBeforeFill
		sb.WriteString(fmt.Sprintf("The slow trader missed **%d more fills** due to orders going stale ",
			diff))
		sb.WriteString(fmt.Sprintf("before any contra-side liquidity arrived. The %d ms additional latency means cancels ",
			latencyDiff))
		sb.WriteString("take longer to process, leaving stale orders exposed. ")
		sb.WriteString(fmt.Sprintf("Out of %d cancels sent by the slow trader, %d targeted orders that never received a fill.\n\n",
			r.slow.CancelsSent, r.slow.CanceledBeforeFill))
	} else {
		sb.WriteString("Both traders show similar missed-fill counts in this scenario.\n\n")
	}

	// 3. Slippage analysis
	sb.WriteString("### Slippage Analysis\n\n")
	slipDelta := r.fast.SlippageBps - r.slow.SlippageBps
	sb.WriteString(fmt.Sprintf("Fast trader slippage: **%.2f bps** | Slow trader slippage: **%.2f bps** (delta: %+.2f bps)\n\n",
		r.fast.SlippageBps, r.slow.SlippageBps, slipDelta))
	if math.Abs(slipDelta) > 0.5 {
		sb.WriteString("The slippage difference reflects the impact of queue position on execution price. ")
		sb.WriteString("Earlier arrival means the fast trader can:\n")
		sb.WriteString("- Join the queue at the intended price before it shifts.\n")
		sb.WriteString("- Execute market orders before adverse book movements.\n\n")
	}

	// 4. Adverse selection
	sb.WriteString("### Adverse Selection\n\n")
	sb.WriteString(fmt.Sprintf("Fast trader: **%.2f bps** | Slow trader: **%.2f bps**\n\n",
		r.fast.AdverseSelectionBps, r.slow.AdverseSelectionBps))
	sb.WriteString("Adverse selection measures price movement against the trader's position after a fill. ")
	if r.slow.AdverseSelectionBps < r.fast.AdverseSelectionBps {
		sb.WriteString("The slow trader experiences less adverse selection, likely because it only gets filled ")
		sb.WriteString("when the market doesn't move away—a form of selection bias that reduces fill rate but ")
		sb.WriteString("improves per-fill quality.\n\n")
	} else {
		sb.WriteString("Both traders face similar adverse selection, indicating that post-fill price dynamics ")
		sb.WriteString("are not strongly correlated with arrival timing in this scenario.\n\n")
	}

	// 5. Time-to-fill
	sb.WriteString("### Time-to-Fill\n\n")
	if r.fast.AvgTimeToFillNs > 0 && r.slow.AvgTimeToFillNs > 0 {
		ttfRatio := r.slow.AvgTimeToFillNs / r.fast.AvgTimeToFillNs
		sb.WriteString(fmt.Sprintf("The slow trader's average time-to-fill is **%.1fx** that of the fast trader. ",
			ttfRatio))
		sb.WriteString("This reflects both the latency gap itself and the cascading effect: ")
		sb.WriteString("later arrival → worse queue position → longer wait for fills.\n\n")
	}

	// 6. Scenario-specific notes
	sb.WriteString("### Scenario Context: " + r.config.Name + "\n\n")
	switch r.config.Name {
	case "calm":
		sb.WriteString("In a calm market with stable mid and tight spread, latency advantages manifest ")
		sb.WriteString("primarily through queue position. The deep book means fills are available for both ")
		sb.WriteString("traders, but the fast trader consistently executes first.\n")
	case "thin":
		sb.WriteString("A thin book magnifies the latency advantage. With limited depth at top levels, ")
		sb.WriteString("the fast trader captures scarce liquidity. Sporadic market sweeps create ")
		sb.WriteString("opportunities that are disproportionately captured by the faster trader.\n")
	case "spike":
		sb.WriteString("During burst windows, the rapid increase in market orders and cancellations ")
		sb.WriteString("creates a volatile environment. The fast trader benefits from being able to ")
		sb.WriteString("cancel and re-quote faster during these windows, while the slow trader's ")
		sb.WriteString("stale orders are more exposed to adverse fills.\n")
	}

	return sb.String()
}

func (r *Report) renderPlots() string {
	var sb strings.Builder

	sb.WriteString("=== Slippage Distribution (ASCII Histogram) ===\n\n")

	if r.fast != nil && len(r.fast.SlippageValues) > 0 {
		sb.WriteString("Fast Trader:\n")
		sb.WriteString(asciiHistogram(r.fast.SlippageValues, 20))
		sb.WriteString("\n")
	}
	if r.slow != nil && len(r.slow.SlippageValues) > 0 {
		sb.WriteString("Slow Trader:\n")
		sb.WriteString(asciiHistogram(r.slow.SlippageValues, 20))
		sb.WriteString("\n")
	}

	sb.WriteString("=== Time-to-Fill CDF (ASCII) ===\n\n")

	if r.fast != nil && len(r.fast.TimeToFillDist) > 0 {
		sb.WriteString("Fast Trader:\n")
		sb.WriteString(asciiCDF(r.fast.TimeToFillDist))
		sb.WriteString("\n")
	}
	if r.slow != nil && len(r.slow.TimeToFillDist) > 0 {
		sb.WriteString("Slow Trader:\n")
		sb.WriteString(asciiCDF(r.slow.TimeToFillDist))
		sb.WriteString("\n")
	}

	return sb.String()
}

// asciiHistogram draws a simple text histogram
func asciiHistogram(values []float64, bins int) string {
	if len(values) == 0 {
		return "  (no data)\n"
	}

	minV, maxV := values[0], values[0]
	for _, v := range values {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}

	if minV == maxV {
		return fmt.Sprintf("  all values = %.4f\n", minV)
	}

	binWidth := (maxV - minV) / float64(bins)
	counts := make([]int, bins)
	maxCount := 0

	for _, v := range values {
		idx := int((v - minV) / binWidth)
		if idx >= bins {
			idx = bins - 1
		}
		counts[idx]++
		if counts[idx] > maxCount {
			maxCount = counts[idx]
		}
	}

	var sb strings.Builder
	barMax := 40
	for i, c := range counts {
		lo := minV + float64(i)*binWidth
		hi := lo + binWidth
		barLen := 0
		if maxCount > 0 {
			barLen = c * barMax / maxCount
		}
		bar := strings.Repeat("█", barLen)
		sb.WriteString(fmt.Sprintf("  %+8.4f to %+8.4f | %s (%d)\n", lo, hi, bar, c))
	}
	return sb.String()
}

// asciiCDF draws a simple text CDF
func asciiCDF(sorted []float64) string {
	if len(sorted) == 0 {
		return "  (no data)\n"
	}

	var sb strings.Builder
	steps := 10
	for i := 1; i <= steps; i++ {
		p := float64(i) / float64(steps)
		val := percentile(sorted, p)
		barLen := int(p * 40)
		bar := strings.Repeat("▓", barLen)
		sb.WriteString(fmt.Sprintf("  P%3.0f: %8.2f ms | %s\n", p*100, val, bar))
	}
	return sb.String()
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := p * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper || upper >= len(sorted) {
		return sorted[lower]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}

// PrintSummary writes a brief summary to stdout
func PrintSummary(cfg *scenario.Config, m map[string]*metrics.TraderMetrics) {
	fast := m[cfg.FastTrader.ID]
	slow := m[cfg.SlowTrader.ID]

	if fast == nil || slow == nil {
		fmt.Println("  No trader metrics available.")
		return
	}

	fmt.Printf("  %-25s %12s %12s %12s\n", "Metric", "Fast", "Slow", "Delta")
	fmt.Printf("  %-25s %12s %12s %12s\n", strings.Repeat("-", 25), strings.Repeat("-", 12), strings.Repeat("-", 12), strings.Repeat("-", 12))

	printRow := func(label string, f, s float64, format string) {
		fmt.Printf("  %-25s "+format+" "+format+" "+format+"\n",
			label, f, s, f-s)
	}

	printRow("Fill Rate (%)", fast.FillRate*100, slow.FillRate*100, "%12.2f")
	printRow("Avg Exec Price", fast.AvgExecPrice, slow.AvgExecPrice, "%12.4f")
	printRow("Slippage (bps)", fast.SlippageBps, slow.SlippageBps, "%12.2f")
	printRow("Avg TTF (ms)", fast.AvgTimeToFillNs, slow.AvgTimeToFillNs, "%12.2f")
	printRow("Queue Pos Place", fast.AvgQueuePosPlace, slow.AvgQueuePosPlace, "%12.2f")
	printRow("Queue Pos Fill", fast.AvgQueuePosFill, slow.AvgQueuePosFill, "%12.2f")
	printRow("Adv Select (bps)", fast.AdverseSelectionBps, slow.AdverseSelectionBps, "%12.2f")
	printRow("Total Fills", float64(fast.TotalFills), float64(slow.TotalFills), "%12.0f")
	printRow("Total Qty", float64(fast.TotalQtyFilled), float64(slow.TotalQtyFilled), "%12.0f")

	mid := domain.PriceToFloat(cfg.Scenario.InitialMidPrice)
	_ = mid
}
