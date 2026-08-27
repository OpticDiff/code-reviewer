package reviewer

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/config"
	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

// intentMockModel implements both ModelReviewer and SummarizeProvider.
type intentMockModel struct {
	reviewResult     *model.ReviewResult
	reviewErr        error
	summaryResult    *model.SummaryResult
	summaryErr       error
	summarizeCalled  bool
	lastSystemPrompt string
}

func (m *intentMockModel) Review(ctx context.Context, systemPrompt, userPrompt string) (*model.ReviewResult, error) {
	m.lastSystemPrompt = systemPrompt
	return m.reviewResult, m.reviewErr
}

func (m *intentMockModel) Close() {}

func (m *intentMockModel) Summarize(ctx context.Context, systemPrompt, userPrompt string) (*model.SummaryResult, error) {
	m.summarizeCalled = true
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
