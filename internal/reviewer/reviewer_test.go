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
	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

// ---------------------------------------------------------------------------
// Mock implementations
// ---------------------------------------------------------------------------

type mockModel struct {
	result         *model.ReviewResult
	err            error
	calls          int
	lastUserPrompt string
}

func (m *mockModel) Review(ctx context.Context, systemPrompt, userPrompt string) (*model.ReviewResult, error) {
	m.calls++
	m.lastUserPrompt = userPrompt
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
	compareCommitsCalls   int
	approveCalls          int
	approvedSHA           string

	// Captured args.
	compareFromSHA string
	compareToSHA   string

	// Configure responses.
	mrChanges       *vcs.MRChanges
	mrChangesErr    error
	mrVersions      []vcs.DiffVersion
	mrVersionErr    error
	postNoteErr     error
	cleanResult     int
	cleanErr        error
	compareFiles    []string
	compareFilesErr error
	approveErr      error

	submitReviewCalls int
	submitReviewReq   *vcs.SubmitReviewRequest
	submitReviewErr   error
}

func (m *mockVCS) GetMRChanges(ctx context.Context, projectID, mrIID string) (*vcs.MRChanges, error) {
	m.getMRChangesCalls++
	return m.mrChanges, m.mrChangesErr
}

func (m *mockVCS) GetMRVersions(ctx context.Context, projectID, mrIID string) ([]vcs.DiffVersion, error) {
	m.getMRVersionsCalls++
	return m.mrVersions, m.mrVersionErr
}

func (m *mockVCS) CompareCommits(ctx context.Context, projectID, from, to string) ([]string, error) {
	m.compareCommitsCalls++
	m.compareFromSHA = from
	m.compareToSHA = to
	return m.compareFiles, m.compareFilesErr
}

func (m *mockVCS) PostNote(ctx context.Context, projectID, mrIID, body string) (*vcs.Comment, error) {
	m.postNoteCalls++
	return &vcs.Comment{ID: m.postNoteCalls}, m.postNoteErr
}

func (m *mockVCS) CreateDiscussion(ctx context.Context, projectID, mrIID string, req vcs.InlineCommentRequest) error {
	m.createDiscussionCalls++
	return nil
}

func (m *mockVCS) ListBotNotes(ctx context.Context, projectID, mrIID string) ([]vcs.Comment, error) {
	m.listBotNotesCalls++
	return nil, nil
}

func (m *mockVCS) DeleteNote(ctx context.Context, projectID, mrIID string, noteID int) error {
	m.deleteNoteCalls++
	return nil
}

func (m *mockVCS) CleanPreviousReviews(ctx context.Context, projectID, mrIID string, changedFiles []string) (int, error) {
	m.cleanCalls++
	return m.cleanResult, m.cleanErr
}

func (m *mockVCS) SubmitReview(ctx context.Context, projectID, mrIID string, req vcs.SubmitReviewRequest) error {
	m.submitReviewCalls++
	m.submitReviewReq = &req
	if m.submitReviewErr != nil {
		return m.submitReviewErr
	}
	// Delegate to individual methods so tests asserting on postNoteCalls/cleanCalls still pass.
	if _, err := m.CleanPreviousReviews(ctx, projectID, mrIID, req.ChangedFiles); err != nil {
		return fmt.Errorf("cleaning previous reviews: %w", err)
	}
	if _, err := m.PostNote(ctx, projectID, mrIID, req.Summary); err != nil {
		return fmt.Errorf("posting summary: %w", err)
	}
	return nil
}

func (m *mockVCS) ApproveReview(ctx context.Context, projectID, reviewID, headSHA string) error {
	m.approveCalls++
	m.approvedSHA = headSHA
	return m.approveErr
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

// ---------------------------------------------------------------------------
// Security: Command injection prevention
// ---------------------------------------------------------------------------

func TestGetLocalDiffs_RejectsFlagRef(t *testing.T) {
	tests := []struct {
		name string
		ref  string
	}{
		{"double-dash flag", "--output=/tmp/evil"},
		{"single-dash flag", "-p"},
		{"flag with equals", "--format=email"},
		{"no-index flag", "--no-index"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reviewer{
				cfg: &config.Config{NoCache: true,
					DiffMode: true,
					DiffRef:  tt.ref,
				},
			}
			_, _, _, err := r.getLocalDiffs()
			if err == nil {
				t.Errorf("expected error for flag-like ref %q, got nil", tt.ref)
			}
			if err != nil && !strings.Contains(err.Error(), "must not start with '-'") {
				t.Errorf("expected flag rejection error, got: %v", err)
			}
		})
	}
}

func TestGetLocalDiffs_AcceptsValidRef(t *testing.T) {
	// Valid refs should not be rejected (they may still fail because of
	// git state, but should not be rejected by our validation).
	validRefs := []string{"main", "origin/HEAD", "HEAD~3", "v1.0.0", "abc123"}
	for _, ref := range validRefs {
		t.Run(ref, func(t *testing.T) {
			r := &Reviewer{
				cfg: &config.Config{NoCache: true,
					DiffMode: true,
					DiffRef:  ref,
				},
			}
			_, _, _, err := r.getLocalDiffs()
			// May fail because of git state, but must NOT fail with "must not start with '-'"
			if err != nil && strings.Contains(err.Error(), "must not start with '-'") {
				t.Errorf("valid ref %q was incorrectly rejected", ref)
			}
		})
	}
}

func TestGetFileDiffs_RejectsFlagPath(t *testing.T) {
	r := &Reviewer{
		cfg: &config.Config{NoCache: true,
			Files: []string{"good.go", "--output=/tmp/evil"},
		},
	}
	_, _, _, err := r.getFileDiffs()
	if err == nil {
		t.Error("expected error for flag-like file path, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "must not start with '-'") {
		t.Errorf("expected flag rejection error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// mockDiffSource
// ---------------------------------------------------------------------------

type mockDiffSource struct {
	diffs   []diff.FileDiff
	title   string
	desc    string
	err     error
	calls   int
}

func (m *mockDiffSource) GetDiffs(ctx context.Context) ([]diff.FileDiff, string, string, error) {
	m.calls++
	return m.diffs, m.title, m.desc, m.err
}

// ---------------------------------------------------------------------------
// Run() tests via DiffSource
// ---------------------------------------------------------------------------

func TestRun_EmptyDiffs(t *testing.T) {
	cfg := &config.Config{NoCache: true,
		DiffMode:      true,
		Model:         "gemini-2.5-flash",
		ChunkStrategy: config.ChunkStrategyFail,
		MinSeverity:   config.SeverityLow,
		DryRun:        true,
	}
	mm := &mockModel{
		result: &model.ReviewResult{Summary: "ok", Findings: nil},
	}
	ds := &mockDiffSource{diffs: nil}
	r := NewWithDiffSource(cfg, mm, &mockVCS{}, ds)

	count, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 findings, got %d", count)
	}
	if ds.calls != 1 {
		t.Errorf("expected 1 DiffSource call, got %d", ds.calls)
	}
	// Model should not be called when there are no diffs.
	if mm.calls != 0 {
		t.Errorf("expected 0 model calls for empty diffs, got %d", mm.calls)
	}
}

func TestRun_WithFindings(t *testing.T) {
	testDiffs := []diff.FileDiff{
		{
			NewPath: "main.go",
			Hunks: []diff.Hunk{
				{
					Header:   "@@ -1,3 +1,4 @@",
					NewStart: 1,
					NewCount: 4,
					OldStart: 1,
					OldCount: 3,
					Lines: []diff.DiffLine{
						{Type: diff.LineContext, Content: "package main", OldLineNo: 1, NewLineNo: 1},
						{Type: diff.LineAdded, Content: "// added", OldLineNo: 0, NewLineNo: 2},
						{Type: diff.LineContext, Content: "func main() {}", OldLineNo: 2, NewLineNo: 3},
					},
				},
			},
		},
	}

	cfg := &config.Config{NoCache: true,
		DiffMode:      true,
		Model:         "gemini-2.5-flash",
		ChunkStrategy: config.ChunkStrategyFail,
		MinSeverity:   config.SeverityLow,
		DryRun:        true,
	}

	findings := []model.Finding{
		{File: "main.go", Line: 2, Severity: "HIGH", Category: "bug", Title: "issue1", Body: "details1"},
		{File: "main.go", Line: 3, Severity: "LOW", Category: "style", Title: "issue2", Body: "details2"},
	}
	mm := &mockModel{
		result: &model.ReviewResult{Summary: "found stuff", Findings: findings},
	}
	ds := &mockDiffSource{diffs: testDiffs, title: "Test MR", desc: "Test desc"}
	r := NewWithDiffSource(cfg, mm, &mockVCS{}, ds)

	count, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 findings, got %d", count)
	}
	if mm.calls != 1 {
		t.Errorf("expected 1 model call, got %d", mm.calls)
	}
}

func TestRun_SeverityFilter(t *testing.T) {
	testDiffs := []diff.FileDiff{
		{
			NewPath: "main.go",
			Hunks: []diff.Hunk{
				{
					Header:   "@@ -1,2 +1,3 @@",
					NewStart: 1,
					NewCount: 3,
					OldStart: 1,
					OldCount: 2,
					Lines: []diff.DiffLine{
						{Type: diff.LineContext, Content: "package main", OldLineNo: 1, NewLineNo: 1},
						{Type: diff.LineAdded, Content: "// new", OldLineNo: 0, NewLineNo: 2},
						{Type: diff.LineContext, Content: "func main() {}", OldLineNo: 2, NewLineNo: 3},
					},
				},
			},
		},
	}

	cfg := &config.Config{NoCache: true,
		DiffMode:      true,
		Model:         "gemini-2.5-flash",
		ChunkStrategy: config.ChunkStrategyFail,
		MinSeverity:   config.SeverityHigh,
		DryRun:        true,
	}

	findings := []model.Finding{
		{File: "main.go", Line: 2, Severity: "HIGH", Category: "bug", Title: "high", Body: "high issue"},
		{File: "main.go", Line: 2, Severity: "LOW", Category: "style", Title: "low", Body: "low issue"},
		{File: "main.go", Line: 2, Severity: "CRITICAL", Category: "security", Title: "crit", Body: "critical issue"},
	}
	mm := &mockModel{
		result: &model.ReviewResult{Summary: "mixed", Findings: findings},
	}
	ds := &mockDiffSource{diffs: testDiffs}
	r := NewWithDiffSource(cfg, mm, &mockVCS{}, ds)

	count, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only HIGH and CRITICAL should remain (LOW filtered out).
	if count != 2 {
		t.Errorf("expected 2 findings after severity filter, got %d", count)
	}
}

func TestRun_ModelError(t *testing.T) {
	testDiffs := []diff.FileDiff{
		{
			NewPath: "main.go",
			Hunks: []diff.Hunk{
				{
					Header:   "@@ -1,1 +1,2 @@",
					NewStart: 1,
					NewCount: 2,
					OldStart: 1,
					OldCount: 1,
					Lines: []diff.DiffLine{
						{Type: diff.LineAdded, Content: "hello", OldLineNo: 0, NewLineNo: 1},
					},
				},
			},
		},
	}

	cfg := &config.Config{NoCache: true,
		DiffMode:      true,
		Model:         "gemini-2.5-flash",
		ChunkStrategy: config.ChunkStrategyFail,
		MinSeverity:   config.SeverityLow,
		DryRun:        true,
	}

	mm := &mockModel{
		err: fmt.Errorf("model unavailable"),
	}
	ds := &mockDiffSource{diffs: testDiffs}
	r := NewWithDiffSource(cfg, mm, &mockVCS{}, ds)

	_, err := r.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from model, got nil")
	}
	if !strings.Contains(err.Error(), "model unavailable") {
		t.Errorf("expected model error to be propagated, got: %v", err)
	}
}

func TestRun_DiffSourceError(t *testing.T) {
	cfg := &config.Config{NoCache: true,
		DiffMode:      true,
		Model:         "gemini-2.5-flash",
		ChunkStrategy: config.ChunkStrategyFail,
		MinSeverity:   config.SeverityLow,
		DryRun:        true,
	}
	mm := &mockModel{}
	ds := &mockDiffSource{err: fmt.Errorf("diff fetch failed")}
	r := NewWithDiffSource(cfg, mm, &mockVCS{}, ds)

	_, err := r.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from DiffSource, got nil")
	}
	if !strings.Contains(err.Error(), "diff fetch failed") {
		t.Errorf("expected diff source error to be propagated, got: %v", err)
	}
}

func TestRun_JSONOutput(t *testing.T) {
	testDiffs := []diff.FileDiff{
		{
			NewPath: "main.go",
			Hunks: []diff.Hunk{
				{
					Header:   "@@ -1,1 +1,2 @@",
					NewStart: 1,
					NewCount: 2,
					OldStart: 1,
					OldCount: 1,
					Lines: []diff.DiffLine{
						{Type: diff.LineAdded, Content: "new code", OldLineNo: 0, NewLineNo: 1},
					},
				},
			},
		},
	}

	cfg := &config.Config{NoCache: true,
		DiffMode:      true,
		Model:         "gemini-2.5-flash",
		ChunkStrategy: config.ChunkStrategyFail,
		MinSeverity:   config.SeverityLow,
		DryRun:        true,
		OutputJSON:    true,
	}

	mm := &mockModel{
		result: &model.ReviewResult{
			Summary:  "json test",
			Findings: []model.Finding{{File: "main.go", Line: 1, Severity: "LOW", Category: "style", Title: "t", Body: "b"}},
		},
	}
	ds := &mockDiffSource{diffs: testDiffs}
	r := NewWithDiffSource(cfg, mm, &mockVCS{}, ds)

	count, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 finding, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// filterBySeverity — additional table-driven tests
// ---------------------------------------------------------------------------

func TestFilterBySeverity_EmptyFindings(t *testing.T) {
	got := filterBySeverity(nil, config.SeverityLow)
	if got != nil {
		t.Errorf("expected nil for empty findings, got %v", got)
	}
}

func TestFilterBySeverity_AllSeverityLevels(t *testing.T) {
	findings := []model.Finding{
		{File: "a.go", Line: 1, Severity: "LOW", Category: "style", Title: "l", Body: "l"},
		{File: "b.go", Line: 2, Severity: "MEDIUM", Category: "style", Title: "m", Body: "m"},
		{File: "c.go", Line: 3, Severity: "HIGH", Category: "bug", Title: "h", Body: "h"},
		{File: "d.go", Line: 4, Severity: "CRITICAL", Category: "security", Title: "c", Body: "c"},
	}

	tests := []struct {
		name        string
		minSeverity config.Severity
		wantFiles   []string
	}{
		{"low keeps all", config.SeverityLow, []string{"a.go", "b.go", "c.go", "d.go"}},
		{"medium filters low", config.SeverityMedium, []string{"b.go", "c.go", "d.go"}},
		{"high filters low+medium", config.SeverityHigh, []string{"c.go", "d.go"}},
		{"critical filters all but critical", config.SeverityCritical, []string{"d.go"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterBySeverity(findings, tt.minSeverity)
			if len(got) != len(tt.wantFiles) {
				t.Fatalf("filterBySeverity(%s): got %d findings, want %d", tt.minSeverity, len(got), len(tt.wantFiles))
			}
			for i, f := range got {
				if f.File != tt.wantFiles[i] {
					t.Errorf("finding[%d].File = %s, want %s", i, f.File, tt.wantFiles[i])
				}
			}
		})
	}
}

func TestFilterBySeverity_MixedUnknownSeverity(t *testing.T) {
	findings := []model.Finding{
		{File: "a.go", Line: 1, Severity: "INVALID", Category: "x", Title: "unknown", Body: "?"},
		{File: "b.go", Line: 2, Severity: "HIGH", Category: "x", Title: "high", Body: "!"},
		{File: "c.go", Line: 3, Severity: "LOW", Category: "x", Title: "low", Body: "."},
	}

	got := filterBySeverity(findings, config.SeverityHigh)
	// INVALID should be included (unknown severities are kept), HIGH kept, LOW filtered.
	if len(got) != 2 {
		t.Errorf("expected 2 findings (INVALID+HIGH), got %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// buildNumberedDiff — additional tests
// ---------------------------------------------------------------------------

func TestBuildNumberedDiff_MultiplFiles(t *testing.T) {
	diffs := []diff.FileDiff{
		{
			NewPath: "file1.go",
			Hunks: []diff.Hunk{
				{
					Header:   "@@ -1,1 +1,2 @@",
					NewStart: 1,
					NewCount: 2,
					OldStart: 1,
					OldCount: 1,
					Lines: []diff.DiffLine{
						{Type: diff.LineContext, Content: "package file1", OldLineNo: 1, NewLineNo: 1},
						{Type: diff.LineAdded, Content: "// new", OldLineNo: 0, NewLineNo: 2},
					},
				},
			},
		},
		{
			NewPath: "file2.go",
			Hunks: []diff.Hunk{
				{
					Header:   "@@ -5,2 +5,2 @@",
					NewStart: 5,
					NewCount: 2,
					OldStart: 5,
					OldCount: 2,
					Lines: []diff.DiffLine{
						{Type: diff.LineRemoved, Content: "old func", OldLineNo: 5, NewLineNo: 0},
						{Type: diff.LineAdded, Content: "new func", OldLineNo: 0, NewLineNo: 5},
					},
				},
			},
		},
	}

	result := buildNumberedDiff(diffs)

	if !strings.Contains(result, "=== File: file1.go ===") {
		t.Error("expected file1.go header")
	}
	if !strings.Contains(result, "=== File: file2.go ===") {
		t.Error("expected file2.go header")
	}
	if !strings.Contains(result, "+ // new") {
		t.Error("expected added line in file1")
	}
	if !strings.Contains(result, "- old func") {
		t.Error("expected removed line in file2")
	}
	if !strings.Contains(result, "+ new func") {
		t.Error("expected added line in file2")
	}
}

func TestBuildNumberedDiff_EmptyDiffs(t *testing.T) {
	result := buildNumberedDiff(nil)
	if result != "" {
		t.Errorf("expected empty string for nil diffs, got %q", result)
	}
}

func TestBuildNumberedDiff_OnlyRemovedLines(t *testing.T) {
	diffs := []diff.FileDiff{
		{
			NewPath: "cleanup.go",
			Hunks: []diff.Hunk{
				{
					Header:   "@@ -10,3 +10,0 @@",
					OldStart: 10,
					OldCount: 3,
					NewStart: 10,
					NewCount: 0,
					Lines: []diff.DiffLine{
						{Type: diff.LineRemoved, Content: "dead code 1", OldLineNo: 10, NewLineNo: 0},
						{Type: diff.LineRemoved, Content: "dead code 2", OldLineNo: 11, NewLineNo: 0},
						{Type: diff.LineRemoved, Content: "dead code 3", OldLineNo: 12, NewLineNo: 0},
					},
				},
			},
		},
	}

	result := buildNumberedDiff(diffs)
	if !strings.Contains(result, "- dead code 1") {
		t.Error("expected removed line 1")
	}
	if !strings.Contains(result, "  10 - dead code 1") {
		t.Error("expected line number 10 for first removed line")
	}
	if !strings.Contains(result, "  12 - dead code 3") {
		t.Error("expected line number 12 for third removed line")
	}
}

// ---------------------------------------------------------------------------
// NewWithDiffSource constructor test
// ---------------------------------------------------------------------------

func TestNewWithDiffSource(t *testing.T) {
	cfg := &config.Config{NoCache: true,Model: "test"}
	mm := &mockModel{}
	mockClient := &mockVCS{}
	ds := &mockDiffSource{}

	r := NewWithDiffSource(cfg, mm, mockClient, ds)
	if r.cfg != cfg {
		t.Error("cfg not set")
	}
	if r.provider != mm {
		t.Error("provider not set")
	}
	if r.glClient != mockClient {
		t.Error("glClient not set")
	}
	if r.diffSource != ds {
		t.Error("diffSource not set")
	}
}

func TestNew(t *testing.T) {
	cfg := &config.Config{NoCache: true,Model: "test"}
	mm := &mockModel{}
	mockClient := &mockVCS{}

	r := New(cfg, mm, mockClient)
	if r.cfg != cfg {
		t.Error("cfg not set")
	}
	if r.provider != mm {
		t.Error("provider not set")
	}
	if r.glClient != mockClient {
		t.Error("glClient not set")
	}
	if r.diffSource != nil {
		t.Error("diffSource should be nil for New()")
	}
}

func TestRun_TerminalOutput(t *testing.T) {
	testDiffs := []diff.FileDiff{
		{
			NewPath: "main.go",
			Hunks: []diff.Hunk{
				{
					Header:   "@@ -1,1 +1,2 @@",
					NewStart: 1,
					NewCount: 2,
					OldStart: 1,
					OldCount: 1,
					Lines: []diff.DiffLine{
						{Type: diff.LineAdded, Content: "new line", OldLineNo: 0, NewLineNo: 1},
					},
				},
			},
		},
	}

	cfg := &config.Config{NoCache: true,
		DiffMode:      true,
		Model:         "gemini-2.5-flash",
		ChunkStrategy: config.ChunkStrategyFail,
		MinSeverity:   config.SeverityLow,
		NoColor:       true,
	}

	mm := &mockModel{
		result: &model.ReviewResult{
			Summary:  "terminal test",
			Findings: []model.Finding{{File: "main.go", Line: 1, Severity: "LOW", Category: "style", Title: "t", Body: "b"}},
		},
	}
	ds := &mockDiffSource{diffs: testDiffs}
	r := NewWithDiffSource(cfg, mm, &mockVCS{}, ds)

	count, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 finding, got %d", count)
	}
}

func TestRun_CIModeNotes(t *testing.T) {
	testDiffs := []diff.FileDiff{
		{
			NewPath: "main.go",
			Hunks: []diff.Hunk{
				{
					Header:   "@@ -1,1 +1,2 @@",
					NewStart: 1,
					NewCount: 2,
					OldStart: 1,
					OldCount: 1,
					Lines: []diff.DiffLine{
						{Type: diff.LineAdded, Content: "code", OldLineNo: 0, NewLineNo: 1},
					},
				},
			},
		},
	}

	cfg := &config.Config{NoCache: true,
		CIMode:           true,
		Model:            "gemini-2.5-flash",
		ChunkStrategy:    config.ChunkStrategyFail,
		MinSeverity:      config.SeverityLow,
		CommentMode:      config.CommentModeNotes,
		CIProjectID:      "123",
		CIMergeRequestID: "456",
	}

	mm := &mockModel{
		result: &model.ReviewResult{
			Summary:  "CI test",
			Findings: []model.Finding{{File: "main.go", Line: 1, Severity: "HIGH", Category: "bug", Title: "ci-issue", Body: "details"}},
		},
	}
	mockClient := &mockVCS{}
	ds := &mockDiffSource{diffs: testDiffs}
	r := NewWithDiffSource(cfg, mm, mockClient, ds)

	count, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 finding, got %d", count)
	}
	if mockClient.postNoteCalls != 1 {
		t.Errorf("expected 1 PostNote call, got %d", mockClient.postNoteCalls)
	}
	if mockClient.cleanCalls != 1 {
		t.Errorf("expected 1 CleanPreviousReviews call, got %d", mockClient.cleanCalls)
	}
}

func TestRun_CIModeDiscussions(t *testing.T) {
	testDiffs := []diff.FileDiff{
		{
			NewPath: "main.go",
			Hunks: []diff.Hunk{
				{
					Header:   "@@ -1,1 +1,2 @@",
					NewStart: 1,
					NewCount: 2,
					OldStart: 1,
					OldCount: 1,
					Lines: []diff.DiffLine{
						{Type: diff.LineAdded, Content: "code", OldLineNo: 0, NewLineNo: 1},
					},
				},
			},
		},
	}

	cfg := &config.Config{NoCache: true,
		CIMode:           true,
		Model:            "gemini-2.5-flash",
		ChunkStrategy:    config.ChunkStrategyFail,
		MinSeverity:      config.SeverityLow,
		CommentMode:      config.CommentModeDiscussions,
		CIProjectID:      "123",
		CIMergeRequestID: "456",
	}

	mm := &mockModel{
		result: &model.ReviewResult{
			Summary:  "CI disc test",
			Findings: []model.Finding{{File: "main.go", Line: 1, Severity: "HIGH", Category: "bug", Title: "disc-issue", Body: "details"}},
		},
	}
	mockClient := &mockVCS{
		mrVersions: []vcs.DiffVersion{{ID: 1, HeadSHA: "abc123", BaseSHA: "def456", StartSHA: "ghi789"}},
	}
	ds := &mockDiffSource{diffs: testDiffs}
	r := NewWithDiffSource(cfg, mm, mockClient, ds)

	count, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 finding, got %d", count)
	}
	if mockClient.getMRVersionsCalls != 1 {
		t.Errorf("expected 1 GetMRVersions call, got %d", mockClient.getMRVersionsCalls)
	}
}

// ---------------------------------------------------------------------------
// Incremental review tests
// ---------------------------------------------------------------------------

func makeTestDiffs(files ...string) []diff.FileDiff {
	var diffs []diff.FileDiff
	for _, f := range files {
		diffs = append(diffs, diff.FileDiff{
			NewPath: f,
			Hunks: []diff.Hunk{
				{
					Header:   "@@ -1,1 +1,2 @@",
					NewStart: 1,
					NewCount: 2,
					OldStart: 1,
					OldCount: 1,
					Lines: []diff.DiffLine{
						{Type: diff.LineAdded, Content: "change in " + f, OldLineNo: 0, NewLineNo: 1},
					},
				},
			},
		})
	}
	return diffs
}

func TestRun_IncrementalReview(t *testing.T) {
	allDiffs := makeTestDiffs("main.go", "util.go", "docs.go")

	cfg := &config.Config{NoCache: true,
		CIMode:           true,
		Model:            "gemini-2.5-flash",
		ChunkStrategy:    config.ChunkStrategyFail,
		MinSeverity:      config.SeverityLow,
		CommentMode:      config.CommentModeNotes,
		CIProjectID:      "123",
		CIMergeRequestID: "456",
		Incremental:      true,
	}

	mm := &mockModel{
		result: &model.ReviewResult{
			Summary:  "incremental",
			Findings: []model.Finding{{File: "main.go", Line: 1, Severity: "LOW", Category: "style", Title: "t", Body: "b"}},
		},
	}
	mockClient := &mockVCS{
		mrVersions: []vcs.DiffVersion{
			{ID: 2, HeadSHA: "new-head", BaseSHA: "base", StartSHA: "start"},
			{ID: 1, HeadSHA: "old-head", BaseSHA: "base", StartSHA: "start"},
		},
		compareFiles: []string{"main.go"}, // Only main.go changed in latest push.
	}
	ds := &mockDiffSource{diffs: allDiffs}
	r := NewWithDiffSource(cfg, mm, mockClient, ds)

	count, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 finding, got %d", count)
	}
	if mm.calls != 1 {
		t.Errorf("expected 1 model call, got %d", mm.calls)
	}
	// Verify only the filtered file reached the model.
	if !strings.Contains(mm.lastUserPrompt, "main.go") {
		t.Error("expected model prompt to contain 'main.go'")
	}
	if strings.Contains(mm.lastUserPrompt, "util.go") {
		t.Error("expected model prompt to NOT contain 'util.go' (should be filtered)")
	}
	if strings.Contains(mm.lastUserPrompt, "docs.go") {
		t.Error("expected model prompt to NOT contain 'docs.go' (should be filtered)")
	}
	if mockClient.compareCommitsCalls != 1 {
		t.Errorf("expected 1 CompareCommits call, got %d", mockClient.compareCommitsCalls)
	}
	if mockClient.compareFromSHA != "old-head" {
		t.Errorf("expected from SHA 'old-head', got %q", mockClient.compareFromSHA)
	}
	if mockClient.compareToSHA != "new-head" {
		t.Errorf("expected to SHA 'new-head', got %q", mockClient.compareToSHA)
	}
}

func TestRun_IncrementalReview_FirstPush(t *testing.T) {
	allDiffs := makeTestDiffs("main.go")

	cfg := &config.Config{NoCache: true,
		CIMode:           true,
		Model:            "gemini-2.5-flash",
		ChunkStrategy:    config.ChunkStrategyFail,
		MinSeverity:      config.SeverityLow,
		CommentMode:      config.CommentModeNotes,
		CIProjectID:      "123",
		CIMergeRequestID: "456",
		Incremental:      true,
	}

	mm := &mockModel{
		result: &model.ReviewResult{
			Summary:  "first push",
			Findings: []model.Finding{{File: "main.go", Line: 1, Severity: "LOW", Category: "style", Title: "t", Body: "b"}},
		},
	}
	mockClient := &mockVCS{
		mrVersions: []vcs.DiffVersion{
			{ID: 1, HeadSHA: "first-head", BaseSHA: "base", StartSHA: "start"},
		},
	}
	ds := &mockDiffSource{diffs: allDiffs}
	r := NewWithDiffSource(cfg, mm, mockClient, ds)

	count, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 finding, got %d", count)
	}
	if mockClient.getMRVersionsCalls != 1 {
		t.Errorf("expected 1 GetMRVersions call, got %d", mockClient.getMRVersionsCalls)
	}
	// CompareCommits should NOT be called for first push (only 1 version).
	if mockClient.compareCommitsCalls != 0 {
		t.Errorf("expected 0 CompareCommits calls for first push, got %d", mockClient.compareCommitsCalls)
	}
}

func TestRun_IncrementalReview_VersionErrorFallback(t *testing.T) {
	allDiffs := makeTestDiffs("main.go")

	cfg := &config.Config{NoCache: true,
		CIMode:           true,
		Model:            "gemini-2.5-flash",
		ChunkStrategy:    config.ChunkStrategyFail,
		MinSeverity:      config.SeverityLow,
		CommentMode:      config.CommentModeNotes,
		CIProjectID:      "123",
		CIMergeRequestID: "456",
		Incremental:      true,
	}

	mm := &mockModel{
		result: &model.ReviewResult{
			Summary:  "fallback",
			Findings: []model.Finding{{File: "main.go", Line: 1, Severity: "LOW", Category: "style", Title: "t", Body: "b"}},
		},
	}
	mockClient := &mockVCS{
		mrVersionErr: fmt.Errorf("versions API error"),
	}
	ds := &mockDiffSource{diffs: allDiffs}
	r := NewWithDiffSource(cfg, mm, mockClient, ds)

	count, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Model should still be called (full review fallback).
	if mm.calls != 1 {
		t.Errorf("expected 1 model call (full review fallback), got %d", mm.calls)
	}
	if count != 1 {
		t.Errorf("expected 1 finding, got %d", count)
	}
	// CompareCommits should NOT be called when versions fail.
	if mockClient.compareCommitsCalls != 0 {
		t.Errorf("expected 0 CompareCommits calls, got %d", mockClient.compareCommitsCalls)
	}
}

func TestRun_IncrementalReview_CompareErrorFallback(t *testing.T) {
	allDiffs := makeTestDiffs("main.go", "util.go")

	cfg := &config.Config{NoCache: true,
		CIMode:           true,
		Model:            "gemini-2.5-flash",
		ChunkStrategy:    config.ChunkStrategyFail,
		MinSeverity:      config.SeverityLow,
		CommentMode:      config.CommentModeNotes,
		CIProjectID:      "123",
		CIMergeRequestID: "456",
		Incremental:      true,
	}

	mm := &mockModel{
		result: &model.ReviewResult{
			Summary: "compare fallback",
			Findings: []model.Finding{
				{File: "main.go", Line: 1, Severity: "LOW", Category: "style", Title: "t1", Body: "b1"},
				{File: "util.go", Line: 1, Severity: "LOW", Category: "style", Title: "t2", Body: "b2"},
			},
		},
	}
	mockClient := &mockVCS{
		mrVersions: []vcs.DiffVersion{
			{ID: 2, HeadSHA: "new-head", BaseSHA: "base", StartSHA: "start"},
			{ID: 1, HeadSHA: "old-head", BaseSHA: "base", StartSHA: "start"},
		},
		compareFilesErr: fmt.Errorf("compare API error"),
	}
	ds := &mockDiffSource{diffs: allDiffs}
	r := NewWithDiffSource(cfg, mm, mockClient, ds)

	count, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Model should still be called with all files (full review fallback).
	if mm.calls != 1 {
		t.Errorf("expected 1 model call (full review fallback), got %d", mm.calls)
	}
	if count != 2 {
		t.Errorf("expected 2 findings (all files reviewed), got %d", count)
	}
}

func TestRun_CacheNotPollutedOnTokenBudgetExceeded(t *testing.T) {
	allDiffs := makeTestDiffs("file1.go", "file2.go")

	diff.ModelTokenLimits["test-tiny"] = 15
	defer delete(diff.ModelTokenLimits, "test-tiny")

	cacheDir := t.TempDir()
	cfg := &config.Config{
		NoCache:       false,
		CacheDir:      cacheDir,
		CacheMaxAge:   time.Hour,
		Model:         "test-tiny",
		ChunkStrategy: config.ChunkStrategySplit,
		MinSeverity:   config.SeverityLow,
		MaxTokens:     500, // Budget is 500 tokens.
	}

	// First chunk will return 1000 tokens, exceeding the 500 limit and breaking out.
	mm := &mockModel{
		result: &model.ReviewResult{
			Summary:  "chunk 1 review",
			Findings: []model.Finding{},
			Usage:    &model.TokenUsage{TotalTokens: 1000},
		},
	}
	mockClient := &mockVCS{}
	ds := &mockDiffSource{diffs: allDiffs}
	r := NewWithDiffSource(cfg, mm, mockClient, ds)

	_, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Model should only be called once because token budget was exceeded.
	if mm.calls != 1 {
		t.Fatalf("expected 1 model call, got %d", mm.calls)
	}

	promptHash := cache.PromptHash(cfg.CustomPrompt, "", cfg.Focus, cfg.ExtraRules, "")
	key1 := cache.CacheKey(cache.DiffHash(allDiffs[0]), cfg.Model, promptHash)
	key2 := cache.CacheKey(cache.DiffHash(allDiffs[1]), cfg.Model, promptHash)

	// file1.go was reviewed and should be in cache.
	if _, ok := r.cache.Lookup(key1); !ok {
		t.Errorf("expected reviewed file1.go to be cached")
	}

	// file2.go was skipped due to token budget and MUST NOT be cached as clean!
	if _, ok := r.cache.Lookup(key2); ok {
		t.Errorf("unreviewed file2.go was leaked into cache as clean findings")
	}
}

func TestRun_AutoApproveOnFullyCachedPR(t *testing.T) {
	allDiffs := makeTestDiffs("file1.go", "file2.go")

	cacheDir := t.TempDir()
	cfg := &config.Config{
		NoCache:          false,
		CacheDir:         cacheDir,
		CacheMaxAge:      time.Hour,
		Model:            "gemini-2.5-flash",
		ChunkStrategy:    config.ChunkStrategyFail,
		MinSeverity:      config.SeverityLow,
		CIMode:           true,
		AutoApprove:      true,
		CIProjectID:      "123",
		CIMergeRequestID: "456",
		CommentMode:      config.CommentModeNotes,
	}

	c, err := cache.New(cacheDir, time.Hour)
	if err != nil {
		t.Fatalf("creating cache: %v", err)
	}

	// Pre-populate cache with clean entries for both files.
	promptHash := cache.PromptHash(cfg.CustomPrompt, "", cfg.Focus, cfg.ExtraRules, "")
	key1 := cache.CacheKey(cache.DiffHash(allDiffs[0]), cfg.Model, promptHash)
	_ = c.Store(key1, cache.Entry{FilePath: "file1.go", DiffHash: cache.DiffHash(allDiffs[0]), Model: cfg.Model, Findings: nil})
	key2 := cache.CacheKey(cache.DiffHash(allDiffs[1]), cfg.Model, promptHash)
	_ = c.Store(key2, cache.Entry{FilePath: "file2.go", DiffHash: cache.DiffHash(allDiffs[1]), Model: cfg.Model, Findings: nil})

	mm := &mockModel{}
	mockClient := &mockVCS{
		mrVersions: []vcs.DiffVersion{
			{ID: 1, HeadSHA: "cached-pr-head-sha"},
		},
	}
	ds := &mockDiffSource{diffs: allDiffs}
	r := NewWithDiffSource(cfg, mm, mockClient, ds)

	count, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Zero model calls since all files were cached.
	if mm.calls != 0 {
		t.Errorf("expected 0 model calls on cached PR, got %d", mm.calls)
	}
	if count != 0 {
		t.Errorf("expected 0 findings, got %d", count)
	}

	// Auto-approve MUST succeed and be pinned to HEAD SHA even when all files were cached.
	if mockClient.approveCalls != 1 {
		t.Fatalf("expected 1 ApproveReview call, got %d", mockClient.approveCalls)
	}
	if mockClient.approvedSHA != "cached-pr-head-sha" {
		t.Errorf("expected approved SHA 'cached-pr-head-sha', got %s", mockClient.approvedSHA)
	}
}
