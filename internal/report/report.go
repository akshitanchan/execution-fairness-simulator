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

type Report struct {
	config *scenario.Config
	fast   *metrics.TraderMetrics
	slow   *metrics.TraderMetrics
	outDir string
}

func NewReport(cfg *scenario.Config, metricsMap map[string]*metrics.TraderMetrics, outDir string) *Report {
	return &Report{
		config: cfg,
		fast:   metricsMap[cfg.FastTrader.ID],
		slow:   metricsMap[cfg.SlowTrader.ID],
		outDir: outDir,
	}
}

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
		sb.WriteString("Insufficient data.\n")
		return sb.String()
	}

	latencyDiff := r.config.SlowTrader.BaseLatencyMs - r.config.FastTrader.BaseLatencyMs
	fillDelta := (r.fast.FillRate - r.slow.FillRate) * 100
	slipDelta := r.fast.SlippageBps - r.slow.SlippageBps

	sb.WriteString(fmt.Sprintf("Latency gap: %d ms (fast %d ms, slow %d ms + %d ms jitter)\n\n",
		latencyDiff, r.config.FastTrader.BaseLatencyMs,
		r.config.SlowTrader.BaseLatencyMs, r.config.SlowTrader.JitterMs))

	if r.fast.AvgQueuePosPlace > 0 || r.slow.AvgQueuePosPlace > 0 {
		sb.WriteString(fmt.Sprintf("- Queue position at placement: fast %.1f, slow %.1f\n",
			r.fast.AvgQueuePosPlace, r.slow.AvgQueuePosPlace))
	}
	if r.fast.AvgQueuePosFill > 0 || r.slow.AvgQueuePosFill > 0 {
		sb.WriteString(fmt.Sprintf("- Queue position at fill: fast %.1f, slow %.1f\n",
			r.fast.AvgQueuePosFill, r.slow.AvgQueuePosFill))
	}

	sb.WriteString(fmt.Sprintf("- Fill rate: fast %.1f%%, slow %.1f%% (delta %+.1f pp)\n",
		r.fast.FillRate*100, r.slow.FillRate*100, fillDelta))

	sb.WriteString(fmt.Sprintf("- Missed fills (canceled before any fill): fast %d, slow %d\n",
		r.fast.CanceledBeforeFill, r.slow.CanceledBeforeFill))

	sb.WriteString(fmt.Sprintf("- Slippage: fast %.2f bps, slow %.2f bps (delta %+.2f bps)\n",
		r.fast.SlippageBps, r.slow.SlippageBps, slipDelta))

	sb.WriteString(fmt.Sprintf("- Adverse selection: fast %.2f bps, slow %.2f bps\n",
		r.fast.AdverseSelectionBps, r.slow.AdverseSelectionBps))

	if r.fast.AvgTimeToFillNs > 0 && r.slow.AvgTimeToFillNs > 0 {
		sb.WriteString(fmt.Sprintf("- Avg time-to-fill: fast %.2f ms, slow %.2f ms (%.1fx)\n",
			r.fast.AvgTimeToFillNs, r.slow.AvgTimeToFillNs,
			r.slow.AvgTimeToFillNs/r.fast.AvgTimeToFillNs))
	}

	sb.WriteString(fmt.Sprintf("\nScenario: %s\n", r.config.Name))

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
