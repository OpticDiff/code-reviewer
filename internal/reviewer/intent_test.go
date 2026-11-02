package reviewer

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/OpticDiff/code-reviewer/internal/cache"
	"github.com/OpticDiff/code-reviewer/internal/config"
	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

// intentMockModel implements both ModelReviewer and SummarizeProvider.
type intentMockModel struct {
	reviewResult      *model.ReviewResult
	reviewErr         error
	summaryResult     *model.SummaryResult
	summaryErr        error
	summarizeCalled   bool
	lastSystemPrompt  string
	summaryUserPrompt string
}

func (m *intentMockModel) Review(ctx context.Context, systemPrompt, userPrompt string) (*model.ReviewResult, error) {
	m.lastSystemPrompt = systemPrompt
	return m.reviewResult, m.reviewErr
}

func (m *intentMockModel) Close() {}

func (m *intentMockModel) Summarize(ctx context.Context, systemPrompt, userPrompt string) (*model.SummaryResult, error) {
	m.summarizeCalled = true
	m.summaryUserPrompt = userPrompt
	return m.summaryResult, m.summaryErr
}

// intentMockDiffSource provides diffs for intent tests.
type intentMockDiffSource struct {
	diffs []diff.FileDiff
	title string
	desc  string
	err   error
}

func (m *intentMockDiffSource) GetDiffs(ctx context.Context) ([]diff.FileDiff, string, string, error) {
	return m.diffs, m.title, m.desc, m.err
}

func testIntentDiffs() []diff.FileDiff {
	return []diff.FileDiff{{
		NewPath: "internal/auth/handler.go",
		Hunks: []diff.Hunk{{
			Header:   "@@ -10,0 +10,3 @@",
			NewStart: 10, NewCount: 3,
			OldStart: 10, OldCount: 0,
			Lines: []diff.DiffLine{
				{Type: diff.LineAdded, NewLineNo: 10, Content: "func Login(w http.ResponseWriter, r *http.Request) {"},
				{Type: diff.LineAdded, NewLineNo: 11, Content: "    token := r.Header.Get(\"Authorization\")"},
				{Type: diff.LineAdded, NewLineNo: 12, Content: "}"},
			},
		}},
	}}
}

func TestIntentReview_InjectsContext(t *testing.T) {
	mock := &intentMockModel{
		reviewResult: &model.ReviewResult{Summary: "OK", Findings: nil},
		summaryResult: &model.SummaryResult{
			Title:          "Add login endpoint",
			Classification: "feat",
			Intent:         "Add login endpoint",
			RiskLevel:      "medium",
			ScopeAreas:     []string{"auth"},
		},
	}
	cfg := &config.Config{NoCache: true,
		IntentReview:  true,
		DryRun:        true,
		Focus:         []string{"bugs"},
		ChunkStrategy: config.ChunkStrategyFail,
		MinSeverity:   config.SeverityLow,
	}
	rev := NewWithDiffSource(cfg, mock, nil, &intentMockDiffSource{
		diffs: testIntentDiffs(), title: "Add login", desc: "Adds login endpoint",
	})
	_, err := rev.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.summarizeCalled {
		t.Error("expected Summarize to be called")
	}
	if !strings.Contains(mock.lastSystemPrompt, "DEVELOPER INTENT") {
		t.Error("expected intent context in system prompt")
	}
}

func TestIntentReview_FallbackOnError(t *testing.T) {
	mock := &intentMockModel{
		reviewResult: &model.ReviewResult{Summary: "OK", Findings: nil},
		summaryErr:   fmt.Errorf("model timeout"),
	}
	cfg := &config.Config{NoCache: true,
		IntentReview:  true,
		DryRun:        true,
		Focus:         []string{"bugs"},
		ChunkStrategy: config.ChunkStrategyFail,
		MinSeverity:   config.SeverityLow,
	}
	rev := NewWithDiffSource(cfg, mock, nil, &intentMockDiffSource{
		diffs: testIntentDiffs(), title: "Fix bug", desc: "",
	})
	_, err := rev.Run(context.Background())
	if err != nil {
		t.Fatalf("expected graceful fallback, got error: %v", err)
	}
	if !mock.summarizeCalled {
		t.Error("expected Summarize to be attempted")
	}
	// Should NOT have intent context (fallback to standard review).
	if strings.Contains(mock.lastSystemPrompt, "DEVELOPER INTENT") {
		t.Error("should not have intent context on error")
	}
}

func TestIntentReview_DisabledByDefault(t *testing.T) {
	mock := &intentMockModel{
		reviewResult:  &model.ReviewResult{Summary: "OK", Findings: nil},
		summaryResult: &model.SummaryResult{Classification: "feat"},
	}
	cfg := &config.Config{NoCache: true,
		IntentReview:  false,
		DryRun:        true,
		Focus:         []string{"bugs"},
		ChunkStrategy: config.ChunkStrategyFail,
		MinSeverity:   config.SeverityLow,
	}
	rev := NewWithDiffSource(cfg, mock, nil, &intentMockDiffSource{
		diffs: testIntentDiffs(), title: "test", desc: "",
	})
	_, err := rev.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.summarizeCalled {
		t.Error("Summarize should NOT be called when IntentReview is false")
	}
}

// The intent pass judges the change as a whole, so files whose findings come
// from the review cache must still be part of what it is given.
func TestIntentReview_SeesCachedFiles(t *testing.T) {
	addedFile := func(path, line string) diff.FileDiff {
		return diff.FileDiff{
			NewPath: path,
			Hunks: []diff.Hunk{{
				Header:   "@@ -0,0 +1,1 @@",
				NewStart: 1, NewCount: 1,
				Lines: []diff.DiffLine{{Type: diff.LineAdded, NewLineNo: 1, Content: line}},
			}},
		}
	}
	code := addedFile("internal/auth/handler.go", "func Login() {}")
	tests := addedFile("internal/auth/handler_test.go", "func TestLogin(t *testing.T) {}")

	cacheDir := t.TempDir()
	cfg := &config.Config{
		IntentReview:  true,
		DryRun:        true,
		Focus:         []string{"bugs"},
		ChunkStrategy: config.ChunkStrategyFail,
		MinSeverity:   config.SeverityLow,
		CacheDir:      cacheDir,
		CacheMaxAge:   time.Hour,
	}

	c, err := cache.New(cacheDir, time.Hour)
	if err != nil {
		t.Fatalf("creating cache: %v", err)
	}
	promptHash := cache.PromptHash(cfg.CustomPrompt, "", "", cfg.Focus, cfg.ExtraRules, "")
	key := cache.CacheKey(cache.DiffHash(tests), cfg.Model, promptHash)
	if err := c.Store(key, cache.Entry{
		FilePath: tests.NewPath, DiffHash: cache.DiffHash(tests), Model: cfg.Model,
	}); err != nil {
		t.Fatalf("storing cache entry: %v", err)
	}

	mock := &intentMockModel{
		reviewResult:  &model.ReviewResult{Summary: "OK"},
		summaryResult: &model.SummaryResult{Title: "Add login", Classification: "feat", Intent: "Add login"},
	}
	rev := NewWithDiffSource(cfg, mock, nil, &intentMockDiffSource{
		diffs: []diff.FileDiff{code, tests}, title: "Add login",
	})
	if _, err := rev.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.summarizeCalled {
		t.Fatal("expected Summarize to be called")
	}
	for _, path := range []string{code.NewPath, tests.NewPath} {
		if !strings.Contains(mock.summaryUserPrompt, path) {
			t.Errorf("intent pass was not given %s; it must see the whole change", path)
		}
	}
	for _, content := range []string{
		"func Login() {}",
		"func TestLogin(t *testing.T) {}",
	} {
		if !strings.Contains(mock.summaryUserPrompt, content) {
			t.Errorf("intent pass was not given diff content %q; it must see the whole change", content)
		}
	}
}

// TestIntentReview_SkippedWhenFullDiffExceedsTokenLimit verifies that Pass 1
// intent inference is skipped when the combined diff (including cached files)
// exceeds the configured model token limit.
func TestIntentReview_SkippedWhenFullDiffExceedsTokenLimit(t *testing.T) {
	addedFile := func(path, line string) diff.FileDiff {
		return diff.FileDiff{
			NewPath: path,
			Hunks: []diff.Hunk{{
				Header:   "@@ -0,0 +1,1 @@",
				NewStart: 1, NewCount: 1,
				Lines: []diff.DiffLine{{Type: diff.LineAdded, NewLineNo: 1, Content: line}},
			}},
		}
	}
	code := addedFile("internal/auth/handler.go", "func Login() {}")
	cachedLarge := addedFile("internal/auth/large_test.go", strings.Repeat("x", 400))

	cacheDir := t.TempDir()
	cfg := &config.Config{
		IntentReview:  true,
		DryRun:        true,
		Focus:         []string{"bugs"},
		ChunkStrategy: config.ChunkStrategySplit,
		TokenLimit:    50, // code fits, but code + cachedLarge exceeds 50 tokens
		MinSeverity:   config.SeverityLow,
		CacheDir:      cacheDir,
		CacheMaxAge:   time.Hour,
	}

	c, err := cache.New(cacheDir, time.Hour)
	if err != nil {
		t.Fatalf("creating cache: %v", err)
	}
	promptHash := cache.PromptHash(cfg.CustomPrompt, "", "", cfg.Focus, cfg.ExtraRules, "")
	key := cache.CacheKey(cache.DiffHash(cachedLarge), cfg.Model, promptHash)
	if err := c.Store(key, cache.Entry{
		FilePath: cachedLarge.NewPath, DiffHash: cache.DiffHash(cachedLarge), Model: cfg.Model,
	}); err != nil {
		t.Fatalf("storing cache entry: %v", err)
	}

	mock := &intentMockModel{
		reviewResult:  &model.ReviewResult{Summary: "OK"},
		summaryResult: &model.SummaryResult{Title: "Add login", Classification: "feat", Intent: "Add login"},
	}
	rev := NewWithDiffSource(cfg, mock, nil, &intentMockDiffSource{
		diffs: []diff.FileDiff{code, cachedLarge}, title: "Add login",
	})
	if _, err := rev.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.summarizeCalled {
		t.Error("expected Summarize to be skipped when auditDiffs exceeds tokenLimit")
	}
}
