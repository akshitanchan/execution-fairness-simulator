// Package report — cross-scenario consolidated comparison
package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/akshitanchan/execution-fairness-simulator/internal/metrics"
	"github.com/akshitanchan/execution-fairness-simulator/internal/scenario"
)

// ScenarioResult bundles a config with its computed metrics
type ScenarioResult struct {
	Config  *scenario.Config
	Metrics map[string]*metrics.TraderMetrics
	RunDir  string
}

// CrossReport generates a consolidated report comparing metrics across scenarios
type CrossReport struct {
	results []ScenarioResult
	outDir  string
}

// NewCrossReport creates a cross-scenario report
func NewCrossReport(results []ScenarioResult, outDir string) *CrossReport {
	return &CrossReport{results: results, outDir: outDir}
}

// Generate writes the consolidated report
func (cr *CrossReport) Generate() error {
	if err := os.MkdirAll(cr.outDir, 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	content := cr.renderMarkdown()
	reportPath := filepath.Join(cr.outDir, "cross-scenario-report.md")
	if err := os.WriteFile(reportPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write cross report: %w", err)
	}

	// Also save structured data
	dataPath := filepath.Join(cr.outDir, "cross-scenario-metrics.json")
	data, _ := json.MarshalIndent(cr.buildSummary(), "", "  ")
	return os.WriteFile(dataPath, data, 0644)
}

type scenarioSummary struct {
	Scenario string                         `json:"scenario"`
	Fast     *metrics.TraderMetrics         `json:"fast"`
	Slow     *metrics.TraderMetrics         `json:"slow"`
}

func (cr *CrossReport) buildSummary() []scenarioSummary {
	var summaries []scenarioSummary
	for _, r := range cr.results {
		summaries = append(summaries, scenarioSummary{
			Scenario: r.Config.Name,
			Fast:     r.Metrics[r.Config.FastTrader.ID],
			Slow:     r.Metrics[r.Config.SlowTrader.ID],
		})
	}
	return summaries
}

func (cr *CrossReport) renderMarkdown() string {
	var sb strings.Builder

	sb.WriteString("# Cross-Scenario Fairness Comparison\n\n")
	sb.WriteString("This report consolidates results from multiple market scenarios to show how latency advantages vary with market conditions.\n\n")

	// Summary table
	sb.WriteString("## Summary Table\n\n")
	sb.WriteString("| Metric | ")
	for _, r := range cr.results {
		sb.WriteString(fmt.Sprintf("%s (F) | %s (S) | ", r.Config.Name, r.Config.Name))
	}
	sb.WriteString("\n|--------|")
	for range cr.results {
		sb.WriteString("--------|--------|")
	}
	sb.WriteString("\n")

	type rowDef struct {
		label string
		get   func(m *metrics.TraderMetrics) float64
		fmt   string
	}

	rows := []rowDef{
		{"Fill Rate (%)", func(m *metrics.TraderMetrics) float64 { return m.FillRate * 100 }, "%.1f"},
		{"Slippage (bps)", func(m *metrics.TraderMetrics) float64 { return m.SlippageBps }, "%.2f"},
		{"Avg TTF (ms)", func(m *metrics.TraderMetrics) float64 { return m.AvgTimeToFillNs }, "%.1f"},
		{"Queue Pos Place", func(m *metrics.TraderMetrics) float64 { return m.AvgQueuePosPlace }, "%.1f"},
		{"Queue Pos Fill", func(m *metrics.TraderMetrics) float64 { return m.AvgQueuePosFill }, "%.1f"},
		{"Adv Select (bps)", func(m *metrics.TraderMetrics) float64 { return m.AdverseSelectionBps }, "%.2f"},
		{"Total Fills", func(m *metrics.TraderMetrics) float64 { return float64(m.TotalFills) }, "%.0f"},
		{"Total Qty", func(m *metrics.TraderMetrics) float64 { return float64(m.TotalQtyFilled) }, "%.0f"},
	}

	for _, row := range rows {
		sb.WriteString(fmt.Sprintf("| %s | ", row.label))
		for _, r := range cr.results {
			fast := r.Metrics[r.Config.FastTrader.ID]
			slow := r.Metrics[r.Config.SlowTrader.ID]
			if fast != nil && slow != nil {
				sb.WriteString(fmt.Sprintf(row.fmt+" | "+row.fmt+" | ", row.get(fast), row.get(slow)))
			} else {
				sb.WriteString("N/A | N/A | ")
			}
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	// Delta table (fast - slow)
	sb.WriteString("## Latency Impact (Fast − Slow)\n\n")
	sb.WriteString("| Metric |")
	for _, r := range cr.results {
		sb.WriteString(fmt.Sprintf(" %s |", r.Config.Name))
	}
	sb.WriteString("\n|--------|")
	for range cr.results {
		sb.WriteString("--------|")
	}
	sb.WriteString("\n")

	for _, row := range rows {
		sb.WriteString(fmt.Sprintf("| %s |", row.label))
		for _, r := range cr.results {
			fast := r.Metrics[r.Config.FastTrader.ID]
			slow := r.Metrics[r.Config.SlowTrader.ID]
			if fast != nil && slow != nil {
				delta := row.get(fast) - row.get(slow)
				sb.WriteString(fmt.Sprintf(" %+.2f |", delta))
			} else {
				sb.WriteString(" N/A |")
			}
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	// Cross-scenario analysis
	sb.WriteString("## Cross-Scenario Analysis\n\n")
	sb.WriteString(cr.generateCrossAnalysis())

	return sb.String()
}

func (cr *CrossReport) generateCrossAnalysis() string {
	var sb strings.Builder

	sb.WriteString("### Where Latency Matters Most\n\n")

	type scenarioDelta struct {
		name       string
		fillDelta  float64
		slipDelta  float64
		ttfDelta   float64
		queueDelta float64
	}

	var deltas []scenarioDelta
	for _, r := range cr.results {
		fast := r.Metrics[r.Config.FastTrader.ID]
		slow := r.Metrics[r.Config.SlowTrader.ID]
		if fast == nil || slow == nil {
			continue
		}
		deltas = append(deltas, scenarioDelta{
			name:       r.Config.Name,
			fillDelta:  (fast.FillRate - slow.FillRate) * 100,
			slipDelta:  fast.SlippageBps - slow.SlippageBps,
			ttfDelta:   fast.AvgTimeToFillNs - slow.AvgTimeToFillNs,
			queueDelta: fast.AvgQueuePosPlace - slow.AvgQueuePosPlace,
		})
	}

	if len(deltas) == 0 {
		sb.WriteString("No scenario data available for comparison.\n")
		return sb.String()
	}

	// Find scenario with largest fill rate gap
	maxFillScenario := deltas[0]
	for _, d := range deltas[1:] {
		if abs(d.fillDelta) > abs(maxFillScenario.fillDelta) {
			maxFillScenario = d
		}
	}

	sb.WriteString(fmt.Sprintf("- **Fill Rate**: The largest gap appears in **%s** (%+.1f pp), ",
		maxFillScenario.name, maxFillScenario.fillDelta))
	sb.WriteString("indicating this market regime amplifies the latency advantage most for execution likelihood.\n")

	// Find scenario with largest slippage gap
	maxSlipScenario := deltas[0]
	for _, d := range deltas[1:] {
		if abs(d.slipDelta) > abs(maxSlipScenario.slipDelta) {
			maxSlipScenario = d
		}
	}

	sb.WriteString(fmt.Sprintf("- **Slippage**: The **%s** scenario shows the widest slippage gap (%+.2f bps), ",
		maxSlipScenario.name, maxSlipScenario.slipDelta))
	sb.WriteString("suggesting execution price quality diverges most under these conditions.\n")

	sb.WriteString("\n### Key Takeaways\n\n")
	sb.WriteString("1. Latency advantages compound: faster arrival → better queue position → higher fill rate → less slippage.\n")
	sb.WriteString("2. Thin or volatile markets amplify the gap because liquidity is scarce and replenished slowly.\n")
	sb.WriteString("3. In calm, deep markets the advantage exists but is smaller in magnitude — depth buffers the impact.\n")

	return sb.String()
}

// PrintCrossSummary prints a condensed cross-scenario summary to stdout
func PrintCrossSummary(results []ScenarioResult) {
	fmt.Println("\n=== Cross-Scenario Comparison ===")
	fmt.Println()
	fmt.Printf("  %-20s", "Metric")
	for _, r := range results {
		fmt.Printf(" %12s(F) %12s(S)", r.Config.Name, r.Config.Name)
	}
	fmt.Println()
	fmt.Printf("  %-20s", strings.Repeat("-", 20))
	for range results {
		fmt.Printf(" %12s %12s", strings.Repeat("-", 12), strings.Repeat("-", 12))
	}
	fmt.Println()

	type rowFn struct {
		label string
		get   func(m *metrics.TraderMetrics) float64
		fmt   string
	}

	rows := []rowFn{
		{"Fill Rate (%)", func(m *metrics.TraderMetrics) float64 { return m.FillRate * 100 }, "%12.1f"},
		{"Slippage (bps)", func(m *metrics.TraderMetrics) float64 { return m.SlippageBps }, "%12.2f"},
		{"Avg TTF (ms)", func(m *metrics.TraderMetrics) float64 { return m.AvgTimeToFillNs }, "%12.1f"},
		{"Queue Pos Place", func(m *metrics.TraderMetrics) float64 { return m.AvgQueuePosPlace }, "%12.1f"},
		{"Adv Select (bps)", func(m *metrics.TraderMetrics) float64 { return m.AdverseSelectionBps }, "%12.2f"},
	}

	for _, row := range rows {
		fmt.Printf("  %-20s", row.label)
		for _, r := range results {
			fast := r.Metrics[r.Config.FastTrader.ID]
			slow := r.Metrics[r.Config.SlowTrader.ID]
			if fast != nil && slow != nil {
				fmt.Printf(" "+row.fmt+" "+row.fmt, row.get(fast), row.get(slow))
			}
		}
		fmt.Println()
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
