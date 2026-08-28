package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/OpticDiff/code-reviewer/internal/cache"
	"github.com/OpticDiff/code-reviewer/internal/config"
	ctxpkg "github.com/OpticDiff/code-reviewer/internal/context"
	gh "github.com/OpticDiff/code-reviewer/internal/github"
	"github.com/OpticDiff/code-reviewer/internal/gitlab"
	"github.com/OpticDiff/code-reviewer/internal/hook"
	"github.com/OpticDiff/code-reviewer/internal/initcmd"
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

	// Handle "hook" subcommand before config.Load() since it doesn't need model config.
	if len(os.Args) >= 2 && os.Args[1] == "hook" {
		if err := runHook(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Handle "init" subcommand — interactive config generator.
	if len(os.Args) >= 2 && os.Args[1] == "init" {
		if err := runInit(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if len(os.Args) >= 2 && os.Args[1] == "cache" {
		if err := runCache(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
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
		"proxy_enabled", cfg.ProxyURL != "",
		"review_md", cfg.ReviewMD != "",
	)

	// Create model provider(s).
	var modelProvider reviewer.ModelReviewer
	if cfg.APIURL != "" {
		// HTTP provider: any OpenAI-compatible endpoint.
		slog.Info("using HTTP provider", "api_url", cfg.APIURL, "model", cfg.Model)
		provider, err := model.NewHTTPProvider(cfg.APIURL, cfg.APIKey, cfg.Model)
		if err != nil {
			return 0, fmt.Errorf("creating HTTP provider: %w", err)
		}
		modelProvider = provider
	} else if len(cfg.Models) > 1 {
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
	var vcsClient reviewer.VCSClient
	if cfg.CIMode && !cfg.DryRun {
		switch cfg.Platform {
		case "github":
			vcsClient = gh.NewClient(cfg.GitHubBaseURL, cfg.GitHubToken)
			slog.Info("using GitHub VCS client", "base_url", cfg.GitHubBaseURL)
		default:
			vcsClient = gitlab.NewClient(cfg.GitLabBaseURL, cfg.GitLabToken)
		}
	}

	// Create context provider for repo-aware reviews.
	var ctxProvider ctxpkg.Provider
	if !cfg.DisableContext {
		ctxProvider = ctxpkg.NewDefaultProvider()
		slog.Info("repo-aware context discovery enabled")
	}

	rev := reviewer.NewWithContext(cfg, modelProvider, vcsClient, ctxProvider)

	var exitCode int
	if cfg.Explain {
		exitCode, err = rev.RunExplain(ctx)
	} else if cfg.Summarize {
		exitCode, err = rev.RunSummary(ctx)
	} else {
		exitCode, err = rev.Run(ctx)
	}
	if err != nil {
		return 0, err
	}

	if exitCode > 0 {
		slog.Info(fmt.Sprintf("review complete: %d finding(s)", exitCode))
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

// runHook dispatches hook subcommands.
func runHook(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: code-reviewer hook <install|uninstall>")
	}
	switch args[0] {
	case "install":
		return hook.Install()
	case "uninstall":
		return hook.Uninstall()
	default:
		return fmt.Errorf("unknown hook command: %q (valid: install, uninstall)", args[0])
	}
}

// runInit dispatches the init subcommand.
func runInit(args []string) error {
	var opts initcmd.Options
	for _, arg := range args {
		switch arg {
		case "--yes", "-y":
			opts.Yes = true
		case "--force":
			opts.Force = true
		default:
			return fmt.Errorf("unknown init flag: %q (valid: --yes/-y, --force)", arg)
		}
	}
	return initcmd.Run(opts)
}

func runCache(args []string) error {
	var cacheDir string
	var subCmd string
	for i := 0; i < len(args); i++ {
		if args[i] == "--cache-dir" {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return fmt.Errorf("--cache-dir requires a value")
			}
			cacheDir = args[i+1]
			i++
		} else if strings.HasPrefix(args[i], "--cache-dir=") {
			cacheDir = strings.TrimPrefix(args[i], "--cache-dir=")
		} else if strings.HasPrefix(args[i], "-") {
			return fmt.Errorf("unknown cache flag: %q", args[i])
		} else if subCmd == "" {
			subCmd = args[i]
		} else {
			return fmt.Errorf("unexpected cache argument: %q", args[i])
		}
	}

	if cacheDir == "" {
		cacheDir = os.Getenv("REVIEW_CACHE_DIR")
	}

	if subCmd == "" {
		return fmt.Errorf("usage: code-reviewer cache <clear|stats>")
	}

	c, err := cache.New(cacheDir, 0)
	if err != nil {
		return fmt.Errorf("initializing cache: %w", err)
	}

	switch subCmd {
	case "clear":
		if err := c.Clear(); err != nil {
			return fmt.Errorf("clearing cache: %w", err)
		}
		fmt.Println("Cache cleared.")
		return nil
	case "stats":
		entries, size, oldest, err := c.Stats()
		if err != nil {
			return fmt.Errorf("getting cache stats: %w", err)
		}
		fmt.Printf("Cache directory: %s\n", c.Dir)
		fmt.Printf("Entries:         %d\n", entries)
		fmt.Printf("Total size:      %s\n", formatSize(size))
		if entries > 0 {
			fmt.Printf("Oldest entry:    %s\n", oldest.Format("2006-01-02 15:04:05"))
		}
		return nil
	default:
		return fmt.Errorf("unknown cache command: %q (valid: clear, stats)", subCmd)
	}
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
