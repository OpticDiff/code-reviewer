package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/OpticDiff/code-reviewer/bench/internal/report"
	"github.com/OpticDiff/code-reviewer/bench/internal/runner"
	"github.com/OpticDiff/code-reviewer/bench/internal/scorer"
	"github.com/OpticDiff/code-reviewer/internal/model"
	"github.com/OpticDiff/code-reviewer/internal/reviewer"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: aacr-bench <command> [options]")
		fmt.Fprintln(os.Stderr, "Commands: run, record")
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "run", "record":
		cmd := flag.NewFlagSet(command, flag.ExitOnError)
		
		var mode string
		if command == "run" {
			cmd.StringVar(&mode, "mode", "replay", "Execution mode: live or replay")
		} else {
			mode = "live"
		}
		
		casesDir := cmd.String("cases", "bench/cases", "Path to benchmark cases")
		output := cmd.String("output", "table", "Output format: table, json, markdown")
		
		modelName := cmd.String("model", "", "Model name (e.g., gemini-2.5-pro)")
		apiURL := cmd.String("api-url", "", "API URL for HTTP provider (OpenAI-compatible)")
		apiKey := cmd.String("api-key", "", "API key (or set CODE_REVIEWER_API_KEY env)")
		category := cmd.String("category", "", "Filter cases by category")
		language := cmd.String("language", "", "Filter cases by language")

		if err := cmd.Parse(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
			os.Exit(1)
		}

		rMode := runner.ModeReplay
		if mode == "live" {
			rMode = runner.ModeLive
		}

		ctx := context.Background()
		var provider reviewer.ModelReviewer
		var err error

		if rMode == runner.ModeLive {
			if *modelName == "" {
				fmt.Fprintln(os.Stderr, "Live mode requires --model flag")
				os.Exit(1)
			}
			
			key := *apiKey
			if key == "" {
				key = os.Getenv("CODE_REVIEWER_API_KEY")
			}
			
			if *apiURL != "" {
				provider, err = model.NewHTTPProvider(*apiURL, key, *modelName)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error creating HTTP provider: %v\n", err)
					os.Exit(1)
				}
			} else {
				provider, err = model.NewProvider(ctx, "", "", *modelName, "")
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error creating provider: %v\n", err)
					os.Exit(1)
				}
			}
		}

		opts := []runner.Option{
			runner.WithLanguage(*language),
			runner.WithCategory(*category),
		}

		if provider != nil {
			opts = append(opts, runner.WithProvider(provider))
		}

		if command == "record" {
			opts = append(opts, runner.WithRecordDir(*casesDir))
		}

		r := runner.New(*casesDir, rMode, opts...)

		results, err := r.RunAll(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error running benchmark: %v\n", err)
			os.Exit(1)
		}

		reportData := scorer.ScoreResults(results)

		switch *output {
		case "json":
			if err := report.WriteJSON(os.Stdout, reportData); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing JSON: %v\n", err)
				os.Exit(1)
			}
		case "markdown":
			if err := report.WriteMarkdown(os.Stdout, reportData); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing markdown: %v\n", err)
				os.Exit(1)
			}
		case "table", "":
			report.PrintTable(os.Stdout, reportData)
		default:
			fmt.Fprintf(os.Stderr, "Unknown output format: %s\n", *output)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		os.Exit(1)
	}
}
