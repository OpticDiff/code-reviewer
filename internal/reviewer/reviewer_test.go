package reviewer

import (
	"context"
	"strings"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/config"
	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/gitlab"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

// ---------------------------------------------------------------------------
// Mock implementations
// ---------------------------------------------------------------------------

type mockModel struct {
	result *model.ReviewResult
	err    error
	calls  int
}

func (m *mockModel) Review(ctx context.Context, systemPrompt, userPrompt string) (*model.ReviewResult, error) {
	m.calls++
	return m.result, m.err
}

func (m *mockModel) Close() {}

type mockVCS struct {
	// Track calls.
	postNoteCalls         int
	cleanCalls            int
	createDiscussionCalls int
	listBotNotesCalls     int
	deleteNoteCalls       int
	getMRChangesCalls     int
	getMRVersionsCalls    int

	// Configure responses.
	mrChanges    *gitlab.MRChangesResponse
	mrChangesErr error
	mrVersions   []gitlab.DiffVersion
	mrVersionErr error
	postNoteErr  error
	cleanResult  int
	cleanErr     error
}

func (m *mockVCS) GetMRChanges(ctx context.Context, projectID, mrIID string) (*gitlab.MRChangesResponse, error) {
	m.getMRChangesCalls++
	return m.mrChanges, m.mrChangesErr
}

func (m *mockVCS) GetMRVersions(ctx context.Context, projectID, mrIID string) ([]gitlab.DiffVersion, error) {
	m.getMRVersionsCalls++
	return m.mrVersions, m.mrVersionErr
}

func (m *mockVCS) PostNote(ctx context.Context, projectID, mrIID, body string) (*gitlab.Note, error) {
	m.postNoteCalls++
	return &gitlab.Note{ID: m.postNoteCalls}, m.postNoteErr
}

func (m *mockVCS) CreateDiscussion(ctx context.Context, projectID, mrIID string, req gitlab.CreateDiscussionRequest) error {
	m.createDiscussionCalls++
	return nil
}

func (m *mockVCS) ListBotNotes(ctx context.Context, projectID, mrIID string) ([]gitlab.Note, error) {
	m.listBotNotesCalls++
	return nil, nil
}

func (m *mockVCS) DeleteNote(ctx context.Context, projectID, mrIID string, noteID int) error {
	m.deleteNoteCalls++
	return nil
}

func (m *mockVCS) CleanPreviousReviews(ctx context.Context, projectID, mrIID string) (int, error) {
	m.cleanCalls++
	return m.cleanResult, m.cleanErr
}

// ---------------------------------------------------------------------------
// buildNumberedDiff
// ---------------------------------------------------------------------------

func TestBuildNumberedDiff(t *testing.T) {
	diffs := []diff.FileDiff{
		{
			NewPath: "main.go",
			Hunks: []diff.Hunk{
				{
					Header:   "@@ -10,3 +10,4 @@ func main()",
					NewStart: 10,
					NewCount: 4,
					OldStart: 10,
					OldCount: 3,
					Lines: []diff.DiffLine{
						{Type: diff.LineContext, Content: "existing line", OldLineNo: 10, NewLineNo: 10},
						{Type: diff.LineRemoved, Content: "old line", OldLineNo: 11, NewLineNo: 0},
						{Type: diff.LineAdded, Content: "new line", OldLineNo: 0, NewLineNo: 11},
						{Type: diff.LineAdded, Content: "another new", OldLineNo: 0, NewLineNo: 12},
						{Type: diff.LineContext, Content: "more context", OldLineNo: 12, NewLineNo: 13},
					},
				},
			},
		},
	}

	result := buildNumberedDiff(diffs)

	// Verify file header.
	if !strings.Contains(result, "=== File: main.go ===") {
		t.Error("expected file header in output")
	}

	// Verify hunk header.
	if !strings.Contains(result, "@@ -10,3 +10,4 @@ func main()") {
		t.Error("expected hunk header in output")
	}

	// Verify added lines have + prefix.
	if !strings.Contains(result, "+ new line") {
		t.Error("expected '+' prefix for added lines")
	}

	// Verify removed lines have - prefix.
	if !strings.Contains(result, "- old line") {
		t.Error("expected '-' prefix for removed lines")
	}

	// Verify context lines have space prefix.
	// The format is "%4d %s %s" so context looks like "  10   existing line".
	lines := strings.Split(result, "\n")
	foundContext := false
	for _, l := range lines {
		if strings.Contains(l, "existing line") && !strings.Contains(l, "+") && !strings.Contains(l, "-") {
			foundContext = true
			break
		}
	}
	if !foundContext {
		t.Error("expected context line with space prefix")
	}
}

func TestBuildNumberedDiff_EmptyNewPath(t *testing.T) {
	diffs := []diff.FileDiff{
		{
			OldPath: "deleted.go",
			NewPath: "",
			Hunks: []diff.Hunk{
				{
					Header:   "@@ -1,3 +0,0 @@",
					OldStart: 1,
					OldCount: 3,
					Lines: []diff.DiffLine{
						{Type: diff.LineRemoved, Content: "gone", OldLineNo: 1, NewLineNo: 0},
					},
				},
			},
		},
	}

	result := buildNumberedDiff(diffs)
	if !strings.Contains(result, "=== File: deleted.go ===") {
		t.Error("expected fallback to OldPath when NewPath is empty")
	}
}

// ---------------------------------------------------------------------------
// filterBySeverity
// ---------------------------------------------------------------------------

func TestFilterBySeverity(t *testing.T) {
	findings := []model.Finding{
		{File: "a.go", Line: 1, Severity: "LOW", Category: "style", Title: "low", Body: "low issue"},
		{File: "b.go", Line: 2, Severity: "MEDIUM", Category: "style", Title: "medium", Body: "medium issue"},
		{File: "c.go", Line: 3, Severity: "HIGH", Category: "bug", Title: "high", Body: "high issue"},
		{File: "d.go", Line: 4, Severity: "CRITICAL", Category: "security", Title: "critical", Body: "critical issue"},
	}

	tests := []struct {
		name        string
		minSeverity config.Severity
		wantCount   int
	}{
		{"LOW keeps all", config.SeverityLow, 4},
		{"MEDIUM drops LOW", config.SeverityMedium, 3},
		{"HIGH drops LOW and MEDIUM", config.SeverityHigh, 2},
		{"CRITICAL keeps only CRITICAL", config.SeverityCritical, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterBySeverity(findings, tt.minSeverity)
			if len(got) != tt.wantCount {
				t.Errorf("filterBySeverity(%s) returned %d findings, want %d", tt.minSeverity, len(got), tt.wantCount)
			}
		})
	}
}

func TestFilterBySeverity_UnknownSeverityIncluded(t *testing.T) {
	findings := []model.Finding{
		{File: "a.go", Line: 1, Severity: "UNKNOWN", Category: "style", Title: "mystery", Body: "details"},
	}

	got := filterBySeverity(findings, config.SeverityHigh)
	if len(got) != 1 {
		t.Errorf("expected unknown severity to be included, got %d findings", len(got))
	}
}

// ---------------------------------------------------------------------------
// TerminalOutput
// ---------------------------------------------------------------------------

func TestTerminalOutput_NoFindings(t *testing.T) {
	result := &model.ReviewResult{
		Summary:  "All looks good",
		Findings: nil,
	}

	output := TerminalOutput(result)

	if !strings.Contains(output, "No issues found") {
		t.Error("expected 'No issues found' message for zero findings")
	}
	if !strings.Contains(output, "All looks good") {
		t.Error("expected summary in output")
	}
}

func TestTerminalOutput_WithFindings(t *testing.T) {
	result := &model.ReviewResult{
		Summary: "Found some issues",
		Findings: []model.Finding{
			{File: "main.go", Line: 10, Severity: "HIGH", Category: "bug", Title: "nil deref", Body: "possible nil pointer"},
			{File: "main.go", Line: 20, Severity: "LOW", Category: "style", Title: "naming", Body: "poor name choice"},
			{File: "util.go", Line: 5, Severity: "MEDIUM", Category: "performance", Title: "alloc", Body: "unnecessary allocation"},
		},
	}

	output := TerminalOutput(result)

	// Verify findings count.
	if !strings.Contains(output, "3") {
		t.Error("expected findings count in output")
	}

	// Verify grouping by file.
	if !strings.Contains(output, "## File: main.go") {
		t.Error("expected main.go file header")
	}
	if !strings.Contains(output, "## File: util.go") {
		t.Error("expected util.go file header")
	}

	// Verify individual findings appear.
	if !strings.Contains(output, "nil deref") {
		t.Error("expected finding title 'nil deref' in output")
	}
	if !strings.Contains(output, "[HIGH]") {
		t.Error("expected severity tag in output")
	}

	// Verify file order matches insertion order.
	mainIdx := strings.Index(output, "## File: main.go")
	utilIdx := strings.Index(output, "## File: util.go")
	if mainIdx > utilIdx {
		t.Error("expected main.go to appear before util.go (insertion order)")
	}
}

func TestTerminalOutput_WithSuggestion(t *testing.T) {
	result := &model.ReviewResult{
		Summary: "Suggestions available",
		Findings: []model.Finding{
			{
				File:       "fix.go",
				Line:       42,
				Severity:   "MEDIUM",
				Category:   "bug",
				Title:      "fix this",
				Body:       "needs correction",
				Suggestion: "corrected := true",
			},
		},
	}

	output := TerminalOutput(result)
	if !strings.Contains(output, "```suggestion") {
		t.Error("expected suggestion code block in output")
	}
	if !strings.Contains(output, "corrected := true") {
		t.Error("expected suggestion content in output")
	}
}

// ---------------------------------------------------------------------------
// Mock infrastructure verification
// ---------------------------------------------------------------------------

func TestMockModel_TracksCallCount(t *testing.T) {
	m := &mockModel{
		result: &model.ReviewResult{Summary: "test", Findings: nil},
	}

	ctx := context.Background()
	_, _ = m.Review(ctx, "system", "user")
	_, _ = m.Review(ctx, "system", "user")

	if m.calls != 2 {
		t.Errorf("expected 2 calls, got %d", m.calls)
	}
}

func TestMockVCS_ImplementsInterface(t *testing.T) {
	// Compile-time check that mockVCS implements VCSClient.
	var _ VCSClient = (*mockVCS)(nil)
}

func TestMockModel_ImplementsInterface(t *testing.T) {
	// Compile-time check that mockModel implements ModelReviewer.
	var _ ModelReviewer = (*mockModel)(nil)
}
