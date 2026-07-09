package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"

	"github.com/OpticDiff/code-reviewer/internal/config"
	"github.com/OpticDiff/code-reviewer/internal/gitlab"
	"github.com/OpticDiff/code-reviewer/internal/model"
	"github.com/OpticDiff/code-reviewer/internal/reviewer"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	// Handle --version before any other setup.
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-version" {
			fmt.Println("code-reviewer", version)
			os.Exit(0)
		}
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	exitCode, err := run(ctx)
	if err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
	os.Exit(exitCode)
}

func run(ctx context.Context) (int, error) {
	cfg, err := config.Load()
	if err != nil {
		return 0, fmt.Errorf("configuration: %w", err)
	}

	slog.Info("code-reviewer starting",
		"version", version,
		"mode", cfg.Mode(),
		"model", cfg.Model,
		"focus", cfg.Focus,
		"min_severity", cfg.MinSeverity.String(),
		"chunk_strategy", cfg.ChunkStrategy,
		"dry_run", cfg.DryRun,
	)

	// Create model provider(s).
	var modelProvider reviewer.ModelReviewer
	if len(cfg.Models) > 1 {
		// Multi-model consensus mode.
		threshold := cfg.ConsensusThreshold
		if threshold < 1 {
			threshold = 2 // Default: 2 models must agree.
		}
		slog.Info("multi-model consensus mode",
			"models", cfg.Models,
			"threshold", threshold,
		)
		mp, err := model.NewMultiProvider(ctx, cfg.GCPProject, cfg.GCPLocation, cfg.Models, threshold)
		if err != nil {
			return 0, fmt.Errorf("initializing multi-model provider: %w", err)
		}
		modelProvider = mp
	} else {
		provider, err := model.NewProvider(ctx, cfg.GCPProject, cfg.GCPLocation, cfg.Model)
		if err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "credentials") ||
				strings.Contains(errMsg, "oauth2") ||
				strings.Contains(errMsg, "authentication") {
				return 0, fmt.Errorf(
					"vertex AI authentication failed: %w\n\n"+
						"To fix, set up Application Default Credentials:\n"+
						"  Local:  gcloud auth application-default login\n"+
						"  CI/CD:  Use Workload Identity Federation or set GOOGLE_APPLICATION_CREDENTIALS",
					err)
			}
			return 0, fmt.Errorf("initializing model provider: %w", err)
		}
		modelProvider = provider
	}
	defer modelProvider.Close()

	// Use interface type so nil remains an untyped nil (not a typed nil
	// *gitlab.Client wrapped in an interface, which would pass nil checks).
	var glClient reviewer.VCSClient
	if cfg.CIMode && !cfg.DryRun {
		glClient = gitlab.NewClient(cfg.GitLabBaseURL, cfg.GitLabToken)
	}

	rev := reviewer.New(cfg, modelProvider, glClient)
	findingCount, err := rev.Run(ctx)
	if err != nil {
		return 0, err
	}

	if findingCount > 0 {
		slog.Info(fmt.Sprintf("review complete: %d finding(s)", findingCount))
		return 1, nil
	}

	slog.Info("review complete: no findings")
	return 0, nil
}
