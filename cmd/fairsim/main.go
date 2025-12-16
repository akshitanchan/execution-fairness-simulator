package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/akshitanchan/execution-fairness-simulator/internal/eventlog"
	"github.com/akshitanchan/execution-fairness-simulator/internal/metrics"
	"github.com/akshitanchan/execution-fairness-simulator/internal/report"
	"github.com/akshitanchan/execution-fairness-simulator/internal/scenario"
	"github.com/akshitanchan/execution-fairness-simulator/internal/sim"
)

const defaultRunsDir = "runs"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		cmdRun(os.Args[2:])
	case "report":
		cmdReport(os.Args[2:])
	case "demo":
		cmdDemo(os.Args[2:])
	case "replay":
		cmdReplay(os.Args[2:])
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func cmdReplay(args []string) {
	if err := runReplay(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runReplay(args []string) error {
	runDir := ""
	runId := ""
	logPath := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--run-id":
			i++
			if i < len(args) {
				runId = args[i]
			}
		case "--run-dir":
			i++
			if i < len(args) {
				runDir = args[i]
			}
		case "--log":
			i++
			if i < len(args) {
				logPath = args[i]
			}
		}
	}
	if runId != "" && runDir == "" {
		runDir = filepath.Join(defaultRunsDir, runId)
	}
	if runDir == "" && logPath != "" {
		runDir = filepath.Dir(logPath)
	}
	if logPath == "" && runDir != "" {
		logPath = filepath.Join(runDir, "events.jsonl")
	}
	if logPath == "" {
		return fmt.Errorf("--run-id, --run-dir, or --log required")
	}

	configPath := filepath.Join(runDir, "config.json")
	if _, err := os.Stat(configPath); err != nil {
		return fmt.Errorf("could not access config at %s: %w", configPath, err)
	}
	if _, err := os.Stat(logPath); err != nil {
		return fmt.Errorf("could not access event log at %s: %w", logPath, err)
	}

	configFile, err := os.Open(configPath)
	if err != nil {
		return fmt.Errorf("could not open config: %w", err)
	}
	defer configFile.Close()
	cfg := &scenario.Config{}
	if err := json.NewDecoder(configFile).Decode(cfg); err != nil {
		return fmt.Errorf("could not decode config: %w", err)
	}

	targetHash, err := simHashFile(logPath)
	if err != nil {
		return fmt.Errorf("could not hash target event log: %w", err)
	}

	fmt.Printf("Analyzing event log: %s\n", logPath)
	metricsByTrader, err := computeMetricsFromEventLog(logPath)
	if err != nil {
		return fmt.Errorf("could not recompute metrics from event log: %w", err)
	}
	fmt.Println("\nMetrics Summary (Replay):")
	report.PrintSummary(cfg, metricsByTrader)

	// Deterministically regenerate the run and compare event-log hashes.
	tmpDir, err := os.MkdirTemp("", "fairsim-replay-*")
	if err != nil {
		return fmt.Errorf("create temp directory for deterministic replay: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	replayRunner, err := sim.NewRunner(cfg, tmpDir)
	if err != nil {
		return fmt.Errorf("initialize deterministic replay runner: %w", err)
	}
	replayResult, err := replayRunner.Run()
	if err != nil {
		return fmt.Errorf("run deterministic replay: %w", err)
	}

	fmt.Printf("\nDeterministic replay log: %s\n", replayResult.LogPath)
	if targetHash == replayResult.LogHash {
		fmt.Printf("Event log hash matches deterministic replay: %s...\n", targetHash[:16])
	} else {
		fmt.Printf("Event log hash MISMATCH!\nTarget: %s...\nReplay: %s...\n", targetHash[:16], replayResult.LogHash[:16])
	}

	return nil
}

func simHashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h), nil
}

func computeMetricsFromEventLog(logPath string) (map[string]*metrics.TraderMetrics, error) {
	reader, err := eventlog.NewReader(logPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	events, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	return metrics.ComputeFromEvents(events), nil
}

func printUsage() {
	fmt.Println(`Usage: fairsim <command> [options]

Commands:
  run      Run a simulation scenario
  demo     Run all scenarios and generate consolidated report
  report   Generate a fairness report
  replay   Analyze a run log and verify deterministic replay

Run options:
  --scenario <name>   Scenario: calm, thin, spike (required)
  --seed <n>          Random seed (default: 42)

Demo options:
  --seed <n>          Random seed (default: 42)

Report options:
  --last-run          Use the most recent run
  --run-dir <path>    Path to a specific run directory

Replay options:
  --run-id <id>       Run id (e.g. calm_seed42)
  --run-dir <path>    Path to a specific run directory
  --log <path>        Path to event log (defaults to <run-dir>/events.jsonl)`)
}

func cmdRun(args []string) {
	scenarioName := ""
	seed := int64(42)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--scenario":
			i++
			if i < len(args) {
				scenarioName = args[i]
			}
		case "--seed":
			i++
			if i < len(args) {
				fmt.Sscanf(args[i], "%d", &seed)
			}
		}
	}

	if scenarioName == "" {
		fmt.Fprintln(os.Stderr, "Error: --scenario is required (calm, thin, spike)")
		os.Exit(1)
	}

	cfg := scenario.GetConfig(scenarioName, seed)
	if cfg == nil {
		fmt.Fprintf(os.Stderr, "Error: unknown scenario '%s'\n", scenarioName)
		os.Exit(1)
	}

	fmt.Printf("Running scenario: %s (seed=%d)\n", scenarioName, seed)

	runner, err := sim.NewRunner(cfg, defaultRunsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing: %v\n", err)
		os.Exit(1)
	}

	result, err := runner.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running simulation: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Simulation complete.\n")
	fmt.Printf("  Events processed: %d\n", result.EventCount)
	fmt.Printf("  Trades executed:  %d\n", result.TradeCount)
	fmt.Printf("  Wall time:        %v\n", result.Duration)
	fmt.Printf("  Log hash:         %s\n", result.LogHash[:16]+"...")
	fmt.Printf("  Output:           %s\n", result.OutputDir)

	metricsByTrader, err := metrics.ComputeFromLog(result.LogPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not compute metrics: %v\n", err)
		return
	}

	fmt.Println("\nMetrics Summary:")
	report.PrintSummary(cfg, metricsByTrader)

	reportGen := report.NewReport(cfg, metricsByTrader, result.OutputDir)
	if err := reportGen.Generate(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not generate report: %v\n", err)
	} else {
		fmt.Printf("\nReport written to: %s/report.md\n", result.OutputDir)
	}
}

func cmdReport(args []string) {
	runDir := ""
	lastRun := false
	runId := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--last-run":
			lastRun = true
		case "--run-dir":
			i++
			if i < len(args) {
				runDir = args[i]
			}
		case "--run-id":
			i++
			if i < len(args) {
				runId = args[i]
			}
		}
	}

	if lastRun {
		data, err := os.ReadFile(defaultRunsDir + "/last-run")
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error: no last run found. Run a simulation first.")
			os.Exit(1)
		}
		runDir = string(data)
	}

	if runId != "" && runDir == "" {
		runDir = defaultRunsDir + "/" + runId
	}

	if runDir == "" {
		fmt.Fprintln(os.Stderr, "Error: --last-run, --run-dir, or --run-id required")
		os.Exit(1)
	}

	reportPath := runDir + "/report.md"
	data, err := os.ReadFile(reportPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading report: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(data))

	plotsPath := runDir + "/plots.txt"
	plotData, err := os.ReadFile(plotsPath)
	if err == nil {
		fmt.Println(string(plotData))
	}
}

func cmdDemo(args []string) {
	seed := int64(42)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--seed":
			i++
			if i < len(args) {
				fmt.Sscanf(args[i], "%d", &seed)
			}
		}
	}

	scenarios := []string{"calm", "thin", "spike"}
	var results []report.ScenarioResult

	for _, name := range scenarios {
		cfg := scenario.GetConfig(name, seed)
		fmt.Printf("Running scenario: %s (seed=%d)...\n", name, seed)

		runner, err := sim.NewRunner(cfg, defaultRunsDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing %s: %v\n", name, err)
			os.Exit(1)
		}

		result, err := runner.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error running %s: %v\n", name, err)
			os.Exit(1)
		}

		fmt.Printf("  %s: %d events, %d trades, %v\n",
			name, result.EventCount, result.TradeCount, result.Duration)

		metricsByTrader, err := metrics.ComputeFromLog(result.LogPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not compute metrics for %s: %v\n", name, err)
			continue
		}

		reportGen := report.NewReport(cfg, metricsByTrader, result.OutputDir)
		if err := reportGen.Generate(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: report generation failed for %s: %v\n", name, err)
		}

		results = append(results, report.ScenarioResult{
			Config:  cfg,
			Metrics: metricsByTrader,
			RunDir:  result.OutputDir,
		})
	}

	report.PrintCrossSummary(results)

	crossReport := report.NewCrossReport(results, defaultRunsDir)
	if err := crossReport.Generate(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cross-scenario report failed: %v\n", err)
	} else {
		fmt.Printf("\nCross-scenario report: %s/cross-scenario-report.md\n", defaultRunsDir)
	}
}
