package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"time"

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

	// Bound provider initialization so CI doesn't hang on auth issues.
	initCtx, initCancel := context.WithTimeout(ctx, 30*time.Second)
	defer initCancel()

	exitCode, err := run(ctx, initCtx)
	if err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
	os.Exit(exitCode)
}

func run(ctx, initCtx context.Context) (int, error) {
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
		"proxy_url", cfg.ProxyURL,
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
		mp, err := model.NewMultiProvider(initCtx, cfg.GCPProject, cfg.GCPLocation, cfg.Models, threshold, cfg.ProxyURL)
		if err != nil {
			return 0, wrapProviderError(err)
		}
		modelProvider = mp
	} else {
		provider, err := model.NewProvider(initCtx, cfg.GCPProject, cfg.GCPLocation, cfg.Model, cfg.ProxyURL)
		if err != nil {
			return 0, wrapProviderError(err)
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

// wrapProviderError detects authentication errors and adds actionable
// remediation guidance. Used for both single-model and multi-model paths.
func wrapProviderError(err error) error {
	errMsg := strings.ToLower(err.Error())
	if strings.Contains(errMsg, "credentials") ||
		strings.Contains(errMsg, "oauth2") ||
		strings.Contains(errMsg, "authentication") {
		return fmt.Errorf(
			"vertex AI authentication failed: %w\n\n"+
				"To fix, set up Application Default Credentials:\n"+
				"  Local:  gcloud auth application-default login\n"+
				"  CI/CD:  Use Workload Identity Federation or set GOOGLE_APPLICATION_CREDENTIALS",
			err)
	}
	return fmt.Errorf("initializing model provider: %w", err)
}
