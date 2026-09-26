package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/OpticDiff/code-reviewer/bench/internal/report"
	"github.com/OpticDiff/code-reviewer/bench/internal/runner"
	"github.com/OpticDiff/code-reviewer/bench/internal/scorer"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: aacr-bench <command> [options]")
		fmt.Fprintln(os.Stderr, "Commands: run, record")
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "run":
		runCmd := flag.NewFlagSet("run", flag.ExitOnError)
		mode := runCmd.String("mode", "replay", "Execution mode: live or replay")
		casesDir := runCmd.String("cases", "bench/cases", "Path to benchmark cases")
		output := runCmd.String("output", "table", "Output format: table, json, markdown")
		// model, category, language args would be added here

		runCmd.Parse(os.Args[2:])

		rMode := runner.ModeReplay
		if *mode == "live" {
			rMode = runner.ModeLive
		}

		// Only support replay mode for now as requested
		if rMode == runner.ModeLive {
			fmt.Fprintln(os.Stderr, "Live mode not fully implemented yet in this version")
			os.Exit(1)
		}

		r := runner.New(*casesDir, rMode)

		ctx := context.Background()
		results, err := r.RunAll(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error running benchmark: %v\n", err)
			os.Exit(1)
		}

		reportData := scorer.ScoreResults(results)

		switch *output {
		case "json":
			report.WriteJSON(os.Stdout, reportData)
		case "markdown":
			report.WriteMarkdown(os.Stdout, reportData)
		case "table", "":
			report.PrintTable(os.Stdout, reportData)
		default:
			fmt.Fprintf(os.Stderr, "Unknown output format: %s\n", *output)
			os.Exit(1)
		}

	case "record":
		fmt.Fprintln(os.Stderr, "Record command not yet implemented")
		os.Exit(1)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		os.Exit(1)
	}
}
