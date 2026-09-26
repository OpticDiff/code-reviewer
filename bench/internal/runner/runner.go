package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/OpticDiff/code-reviewer/bench/internal/dataset"
	"github.com/OpticDiff/code-reviewer/internal/config"
	"github.com/OpticDiff/code-reviewer/internal/model"
	"github.com/OpticDiff/code-reviewer/internal/reviewer"
)

type Mode string

const (
	ModeLive   Mode = "live"
	ModeReplay Mode = "replay"
)

type Runner struct {
	casesDir string
	provider reviewer.ModelReviewer
	config   *config.Config
	mode     Mode
}

type CaseResult struct {
	Case           dataset.Case
	ActualFindings []model.Finding
	Duration       time.Duration
	TokenUsage     *model.TokenUsage
	Error          error
}

type Option func(*Runner)

func WithProvider(p reviewer.ModelReviewer) Option {
	return func(r *Runner) {
		r.provider = p
		r.mode = ModeLive
	}
}

func WithConfig(cfg *config.Config) Option {
	return func(r *Runner) {
		r.config = cfg
	}
}

func New(casesDir string, mode Mode, opts ...Option) *Runner {
	r := &Runner{
		casesDir: casesDir,
		mode:     mode,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *Runner) RunAll(ctx context.Context) ([]CaseResult, error) {
	cases, err := dataset.LoadCases(r.casesDir)
	if err != nil {
		return nil, fmt.Errorf("loading cases: %w", err)
	}

	var results []CaseResult
	for _, c := range cases {
		res, err := r.RunCase(ctx, c)
		if err != nil {
			return nil, fmt.Errorf("running case %s: %w", c.Name, err)
		}
		results = append(results, *res)
	}
	return results, nil
}

func (r *Runner) RunCase(ctx context.Context, c dataset.Case) (*CaseResult, error) {
	start := time.Now()

	var result *model.ReviewResult
	var err error

	if r.mode == ModeReplay {
		result, err = loadReplayResponse(c.Dir)
		if err != nil {
			return nil, fmt.Errorf("replay %s: %w", c.Name, err)
		}
	} else {
		if r.provider == nil {
			return nil, fmt.Errorf("live mode requires a model provider (use WithProvider)")
		}
		sysPrompt := buildBenchSystemPrompt(c, r.config)
		userPrompt := buildBenchUserPrompt(c)
		result, err = r.provider.Review(ctx, sysPrompt, userPrompt)
		if err != nil {
			return &CaseResult{Case: c, Error: err, Duration: time.Since(start)}, nil
		}
	}

	duration := time.Since(start)

	return &CaseResult{
		Case:           c,
		ActualFindings: result.Findings,
		Duration:       duration,
		TokenUsage:     result.Usage,
	}, nil
}

func loadReplayResponse(dir string) (*model.ReviewResult, error) {
	path := filepath.Join(dir, "response.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading response.json: %w", err)
	}

	var result model.ReviewResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing response.json: %w", err)
	}
	return &result, nil
}

func buildBenchSystemPrompt(c dataset.Case, cfg *config.Config) string {
	var extraRules string
	if cfg != nil {
		extraRules = cfg.ExtraRules
	}
	return model.BuildPrompt(nil, extraRules)
}

func buildBenchUserPrompt(c dataset.Case) string {
	// Provide minimal context snippet if needed, for now just basic text
	return model.BuildUserPromptWithContext(
		fmt.Sprintf("Benchmark Case: %s", c.Name),
		fmt.Sprintf("Category: %s, Tags: %v", c.Category, c.Tags),
		c.DiffText,
		nil,
	)
}
