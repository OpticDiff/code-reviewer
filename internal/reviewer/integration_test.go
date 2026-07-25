package reviewer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/config"
	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

// ---------------------------------------------------------------------------
// Enhanced capturing mocks
// ---------------------------------------------------------------------------

// capturingVCS extends mockVCS to capture the content of VCS API calls
// so integration tests can assert on the actual payloads.
type capturingVCS struct {
	mockVCS

	// Captured payloads.
	postedNotes          []string
	createdDiscussions   []vcs.InlineCommentRequest
	discussionErr        error // If set, CreateDiscussion returns this error.
}

func (c *capturingVCS) PostNote(ctx context.Context, projectID, mrIID, body string) (*vcs.Comment, error) {
	c.postNoteCalls++
	c.postedNotes = append(c.postedNotes, body)
	return &vcs.Comment{ID: c.postNoteCalls}, c.postNoteErr
}

func (c *capturingVCS) CreateDiscussion(ctx context.Context, projectID, mrIID string, req vcs.InlineCommentRequest) error {
	c.createDiscussionCalls++
	c.createdDiscussions = append(c.createdDiscussions, req)
	return c.discussionErr
}

// mockSummarizeModel implements ModelReviewer + SummarizeProvider for
// two-pass intent and summary pipeline testing.
type mockSummarizeModel struct {
	mockModel
	summaryResult *model.SummaryResult
	summaryErr    error
	summarizeCalls int
}

func (m *mockSummarizeModel) Summarize(ctx context.Context, systemPrompt, userPrompt string) (*model.SummaryResult, error) {
	m.summarizeCalls++
	return m.summaryResult, m.summaryErr
}

// mockExplainModel implements ModelReviewer + ExplainProvider.
type mockExplainModel struct {
	mockModel
	explanation string
	explainUsage *model.TokenUsage
	explainErr  error
	explainCalls int
}

func (m *mockExplainModel) Explain(ctx context.Context, systemPrompt, userPrompt string) (string, *model.TokenUsage, error) {
	m.explainCalls++
	return m.explanation, m.explainUsage, m.explainErr
}

// ---------------------------------------------------------------------------
// Shared test fixtures
// ---------------------------------------------------------------------------

// integrationDiffs returns a realistic multi-file diff fixture.
func integrationDiffs() []diff.FileDiff {
	return []diff.FileDiff{
		{
			NewPath: "internal/auth/handler.go",
			Hunks: []diff.Hunk{
				{
					Header:   "@@ -10,3 +10,5 @@ func HandleLogin()",
					NewStart: 10,
					NewCount: 5,
					OldStart: 10,
					OldCount: 3,
					Lines: []diff.DiffLine{
						{Type: diff.LineContext, Content: "func HandleLogin() {", OldLineNo: 10, NewLineNo: 10},
						{Type: diff.LineAdded, Content: `    token := getToken()`, OldLineNo: 0, NewLineNo: 11},
						{Type: diff.LineAdded, Content: `    user := token.Claims.Subject`, OldLineNo: 0, NewLineNo: 12},
						{Type: diff.LineContext, Content: "    return nil", OldLineNo: 11, NewLineNo: 13},
						{Type: diff.LineContext, Content: "}", OldLineNo: 12, NewLineNo: 14},
					},
				},
			},
		},
		{
			NewPath: "internal/middleware/rate.go",
			Hunks: []diff.Hunk{
				{
					Header:   "@@ -1,3 +1,4 @@",
					NewStart: 1,
					NewCount: 4,
					OldStart: 1,
					OldCount: 3,
					Lines: []diff.DiffLine{
						{Type: diff.LineContext, Content: "package middleware", OldLineNo: 1, NewLineNo: 1},
						{Type: diff.LineAdded, Content: "// RateLimit applies per-user rate limiting", OldLineNo: 0, NewLineNo: 2},
						{Type: diff.LineContext, Content: "func RateLimit() {}", OldLineNo: 2, NewLineNo: 3},
						{Type: diff.LineContext, Content: "// end", OldLineNo: 3, NewLineNo: 4},
					},
				},
			},
		},
	}
}

// ciConfig returns a base CI config for integration tests.
func ciConfig() *config.Config {
	return &config.Config{
		CIMode:           true,
		Model:            "gemini-2.5-flash",
		ChunkStrategy:    config.ChunkStrategyFail,
		MinSeverity:      config.SeverityLow,
		CommentMode:      config.CommentModeNotes,
		CIProjectID:      "42",
		CIMergeRequestID: "99",
	}
}

// ---------------------------------------------------------------------------
// Test 1: CI Notes — verify posted content
// ---------------------------------------------------------------------------

func TestIntegration_CINotes_PostedContent(t *testing.T) {
	findings := []model.Finding{
		{File: "internal/auth/handler.go", Line: 11, Severity: "HIGH", Category: "security", Title: "Nil pointer dereference", Body: "token may be nil before accessing Claims"},
		{File: "internal/middleware/rate.go", Line: 2, Severity: "LOW", Category: "style", Title: "Comment formatting", Body: "Use godoc-style comments"},
	}

	mm := &mockModel{
		result: &model.ReviewResult{
			Summary:  "2 issues found across auth and middleware",
			Findings: findings,
			Usage:    &model.TokenUsage{InputTokens: 1200, OutputTokens: 300, TotalTokens: 1500},
		},
	}

	client := &capturingVCS{}
	ds := &mockDiffSource{diffs: integrationDiffs(), title: "Fix auth handler", desc: "Fixes nil check"}
	r := NewWithDiffSource(ciConfig(), mm, client, ds)

	count, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 findings, got %d", count)
	}

	// Verify CleanPreviousReviews was called.
	if client.cleanCalls != 1 {
		t.Errorf("expected 1 CleanPreviousReviews call, got %d", client.cleanCalls)
	}

	// Verify exactly 1 note was posted (summary).
	if len(client.postedNotes) != 1 {
		t.Fatalf("expected 1 posted note, got %d", len(client.postedNotes))
	}

	note := client.postedNotes[0]

	// Verify summary structure.
	assertions := []struct {
		desc    string
		want    string
	}{
		{"header", "## 📋 Code Review Summary"},
		{"summary text", "2 issues found across auth and middleware"},
		{"severity table", "| Severity | Count |"},
		{"HIGH count", "🟠 HIGH | 1"},
		{"LOW count", "🔵 LOW | 1"},
		{"finding 1 title", "Nil pointer dereference"},
		{"finding 1 file:line", "`internal/auth/handler.go:11`"},
		{"finding 2 title", "Comment formatting"},
		{"finding 2 file:line", "`internal/middleware/rate.go:2`"},
	}

	for _, a := range assertions {
		if !strings.Contains(note, a.want) {
			t.Errorf("posted note missing %s: wanted %q in:\n%s", a.desc, a.want, note)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 2: CI Discussions — verify inline comment positions
// ---------------------------------------------------------------------------

func TestIntegration_CIDiscussions_Positions(t *testing.T) {
	findings := []model.Finding{
		{File: "internal/auth/handler.go", Line: 11, Severity: "HIGH", Category: "security", Title: "Nil deref", Body: "token may be nil"},
		{File: "internal/middleware/rate.go", Line: 2, Severity: "MEDIUM", Category: "style", Title: "Comment style", Body: "use godoc"},
	}

	mm := &mockModel{
		result: &model.ReviewResult{
			Summary:  "Found issues",
			Findings: findings,
		},
	}

	version := vcs.DiffVersion{
		ID: 5, HeadSHA: "abc123", BaseSHA: "def456", StartSHA: "ghi789",
	}
	client := &capturingVCS{
		mockVCS: mockVCS{
			mrVersions: []vcs.DiffVersion{version},
		},
	}

	cfg := ciConfig()
	cfg.CommentMode = config.CommentModeDiscussions
	ds := &mockDiffSource{diffs: integrationDiffs()}
	r := NewWithDiffSource(cfg, mm, client, ds)

	count, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 findings, got %d", count)
	}

	// Verify 1 summary note + 2 discussions.
	if len(client.postedNotes) != 1 {
		t.Errorf("expected 1 summary note, got %d", len(client.postedNotes))
	}
	if len(client.createdDiscussions) != 2 {
		t.Fatalf("expected 2 discussions, got %d", len(client.createdDiscussions))
	}

	// Verify first discussion position.
	d1 := client.createdDiscussions[0]
	if d1.Position == nil {
		t.Fatal("expected position on first discussion")
	}
	if d1.Position.HeadSHA != "abc123" {
		t.Errorf("HeadSHA = %q, want abc123", d1.Position.HeadSHA)
	}
	if d1.Position.BaseSHA != "def456" {
		t.Errorf("BaseSHA = %q, want def456", d1.Position.BaseSHA)
	}
	if d1.Position.StartSHA != "ghi789" {
		t.Errorf("StartSHA = %q, want ghi789", d1.Position.StartSHA)
	}
	if d1.Position.NewPath != "internal/auth/handler.go" {
		t.Errorf("NewPath = %q, want internal/auth/handler.go", d1.Position.NewPath)
	}
	if d1.Position.NewLine == nil || *d1.Position.NewLine != 11 {
		t.Errorf("NewLine = %v, want 11", d1.Position.NewLine)
	}

	// Verify second discussion position.
	d2 := client.createdDiscussions[1]
	if d2.Position.NewPath != "internal/middleware/rate.go" {
		t.Errorf("NewPath = %q, want internal/middleware/rate.go", d2.Position.NewPath)
	}
	if d2.Position.NewLine == nil || *d2.Position.NewLine != 2 {
		t.Errorf("NewLine = %v, want 2", d2.Position.NewLine)
	}

	// Verify discussion body content.
	if !strings.Contains(d1.Body, "Nil deref") {
		t.Errorf("discussion body missing title, got: %s", d1.Body)
	}
	if !strings.Contains(d1.Body, "**[HIGH]**") {
		t.Errorf("discussion body missing severity badge, got: %s", d1.Body)
	}
}

// ---------------------------------------------------------------------------
// Test 3: CI Discussions — fallback to PostNote on CreateDiscussion error
// ---------------------------------------------------------------------------

func TestIntegration_CIDiscussions_FallbackOnError(t *testing.T) {
	// Use line 1 which matches makeTestDiffs' added line (NewLineNo: 1).
	findings := []model.Finding{
		{File: "main.go", Line: 1, Severity: "HIGH", Category: "bug", Title: "Error ignored", Body: "return value not checked"},
	}

	mm := &mockModel{
		result: &model.ReviewResult{Summary: "Issues found", Findings: findings},
	}

	version := vcs.DiffVersion{ID: 1, HeadSHA: "h", BaseSHA: "b", StartSHA: "s"}
	client := &capturingVCS{
		mockVCS: mockVCS{
			mrVersions: []vcs.DiffVersion{version},
		},
		discussionErr: fmt.Errorf("422 Unprocessable Entity"),
	}

	cfg := ciConfig()
	cfg.CommentMode = config.CommentModeDiscussions
	diffs := makeTestDiffs("main.go")
	ds := &mockDiffSource{diffs: diffs}
	r := NewWithDiffSource(cfg, mm, client, ds)

	count, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 finding, got %d", count)
	}

	// 1 summary note + 1 fallback note.
	if len(client.postedNotes) != 2 {
		t.Fatalf("expected 2 notes (summary + fallback), got %d", len(client.postedNotes))
	}

	// Fallback note should contain file:line prefix (line 1, not 5).
	fallback := client.postedNotes[1]
	if !strings.Contains(fallback, "**main.go:1**") {
		t.Errorf("fallback note missing file:line prefix, got: %s", fallback)
	}
	if !strings.Contains(fallback, "Error ignored") {
		t.Errorf("fallback note missing finding title, got: %s", fallback)
	}
}

// ---------------------------------------------------------------------------
// Test 4: SARIF output — end-to-end file verification
// ---------------------------------------------------------------------------

func TestIntegration_SARIF_Output(t *testing.T) {
	sarifPath := filepath.Join(t.TempDir(), "results.sarif")

	// Build diffs with hunks whose line ranges cover the finding line numbers.
	sarifDiffs := []diff.FileDiff{
		{
			NewPath: "auth.go",
			Hunks: []diff.Hunk{{
				Header: "@@ -40,5 +40,6 @@", NewStart: 40, NewCount: 6, OldStart: 40, OldCount: 5,
				Lines: []diff.DiffLine{
					{Type: diff.LineContext, Content: "ctx", OldLineNo: 40, NewLineNo: 40},
					{Type: diff.LineContext, Content: "ctx", OldLineNo: 41, NewLineNo: 41},
					{Type: diff.LineAdded, Content: "query := buildSQL(input)", OldLineNo: 0, NewLineNo: 42},
					{Type: diff.LineContext, Content: "ctx", OldLineNo: 42, NewLineNo: 43},
				},
			}},
		},
		{
			NewPath: "handler.go",
			Hunks: []diff.Hunk{{
				Header: "@@ -8,5 +8,7 @@", NewStart: 8, NewCount: 7, OldStart: 8, OldCount: 5,
				Lines: []diff.DiffLine{
					{Type: diff.LineContext, Content: "ctx", OldLineNo: 8, NewLineNo: 8},
					{Type: diff.LineContext, Content: "ctx", OldLineNo: 9, NewLineNo: 9},
					{Type: diff.LineAdded, Content: "ptr := getPtr()", OldLineNo: 0, NewLineNo: 10},
					{Type: diff.LineContext, Content: "ctx", OldLineNo: 10, NewLineNo: 11},
				},
			},
			{
				Header: "@@ -23,3 +25,4 @@", NewStart: 23, NewCount: 4, OldStart: 23, OldCount: 3,
				Lines: []diff.DiffLine{
					{Type: diff.LineContext, Content: "ctx", OldLineNo: 23, NewLineNo: 24},
					{Type: diff.LineAdded, Content: "badName := true", OldLineNo: 0, NewLineNo: 25},
					{Type: diff.LineContext, Content: "ctx", OldLineNo: 24, NewLineNo: 26},
				},
			}},
		},
	}

	findings := []model.Finding{
		{File: "auth.go", Line: 42, Severity: "CRITICAL", Category: "security", Title: "SQL injection", Body: "user input concatenated into query"},
		{File: "handler.go", Line: 10, Severity: "MEDIUM", Category: "bug", Title: "Missing nil check", Body: "pointer may be nil"},
		{File: "handler.go", Line: 25, Severity: "LOW", Category: "style", Title: "Naming", Body: "use camelCase"},
	}

	mm := &mockModel{
		result: &model.ReviewResult{Summary: "Mixed severity", Findings: findings},
	}

	cfg := ciConfig()
	cfg.SARIFOutput = sarifPath
	ds := &mockDiffSource{diffs: sarifDiffs}
	r := NewWithDiffSource(cfg, mm, &capturingVCS{}, ds)

	count, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 findings, got %d", count)
	}

	// Read and parse SARIF.
	data, err := os.ReadFile(sarifPath)
	if err != nil {
		t.Fatalf("failed to read SARIF file: %v", err)
	}

	var report sarifReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("invalid SARIF JSON: %v", err)
	}

	// Verify schema.
	if report.Version != "2.1.0" {
		t.Errorf("SARIF version = %q, want 2.1.0", report.Version)
	}
	if len(report.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(report.Runs))
	}

	run := report.Runs[0]

	// Verify tool identity.
	if run.Tool.Driver.Name != "code-reviewer" {
		t.Errorf("tool name = %q, want code-reviewer", run.Tool.Driver.Name)
	}

	// Verify results.
	if len(run.Results) != 3 {
		t.Fatalf("expected 3 SARIF results, got %d", len(run.Results))
	}

	// First result: CRITICAL security → error level.
	r0 := run.Results[0]
	if r0.RuleID != "security" {
		t.Errorf("result[0].ruleId = %q, want security", r0.RuleID)
	}
	if r0.Level != "error" {
		t.Errorf("result[0].level = %q, want error (CRITICAL maps to error)", r0.Level)
	}
	if r0.Locations[0].PhysicalLocation.ArtifactLocation.URI != "auth.go" {
		t.Errorf("result[0] URI = %q, want auth.go", r0.Locations[0].PhysicalLocation.ArtifactLocation.URI)
	}
	if r0.Locations[0].PhysicalLocation.Region.StartLine != 42 {
		t.Errorf("result[0] startLine = %d, want 42", r0.Locations[0].PhysicalLocation.Region.StartLine)
	}

	// Second result: MEDIUM bug → warning level.
	if run.Results[1].Level != "warning" {
		t.Errorf("result[1].level = %q, want warning (MEDIUM maps to warning)", run.Results[1].Level)
	}

	// Third result: LOW style → note level.
	if run.Results[2].Level != "note" {
		t.Errorf("result[2].level = %q, want note (LOW maps to note)", run.Results[2].Level)
	}

	// Verify rules are deduplicated by category.
	if len(run.Tool.Driver.Rules) != 3 {
		t.Errorf("expected 3 rules (security, bug, style), got %d", len(run.Tool.Driver.Rules))
	}
}

// ---------------------------------------------------------------------------
// Test 5: Summary mode — CI note content
// ---------------------------------------------------------------------------

func TestIntegration_Summary_CINote(t *testing.T) {
	summaryResult := &model.SummaryResult{
		Title:          "Fix rate limiter bypass for authenticated users",
		Description:    "The rate limiter was checking IP before auth middleware.",
		Intent:         "Ensure rate limiting applies per-user after authentication",
		Classification: "fix",
		ScopeAreas:     []string{"auth", "middleware"},
		RiskLevel:      "medium",
		Confidence:     0.96,
	}

	mm := &mockSummarizeModel{
		mockModel: mockModel{
			result: &model.ReviewResult{Summary: "ok"},
		},
		summaryResult: summaryResult,
	}

	client := &capturingVCS{}
	ds := &mockDiffSource{diffs: integrationDiffs(), title: "Fix rate limiter"}
	cfg := ciConfig()
	cfg.Summarize = true
	r := NewWithDiffSource(cfg, mm, client, ds)

	count, err := r.RunSummary(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("RunSummary should always return 0, got %d", count)
	}

	// Verify summarize was called.
	if mm.summarizeCalls != 1 {
		t.Errorf("expected 1 Summarize call, got %d", mm.summarizeCalls)
	}

	// Verify note was posted.
	if len(client.postedNotes) != 1 {
		t.Fatalf("expected 1 posted note, got %d", len(client.postedNotes))
	}

	note := client.postedNotes[0]

	assertions := []struct {
		desc string
		want string
	}{
		{"header", "## 📋 MR Summary"},
		{"classification", "`fix`"},
		{"confidence", "96% confidence"},
		{"risk", "medium"},
		{"scope", "auth, middleware"},
		{"title", "Fix rate limiter bypass for authenticated users"},
		{"description", "rate limiter was checking IP before auth middleware"},
		{"intent", "Ensure rate limiting applies per-user after authentication"},
		{"breaking changes", "**Breaking Changes:** None"},
	}

	for _, a := range assertions {
		if !strings.Contains(note, a.want) {
			t.Errorf("summary note missing %s: wanted %q in:\n%s", a.desc, a.want, note)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 6: Explain mode — CI note content
// ---------------------------------------------------------------------------

func TestIntegration_Explain_CINote(t *testing.T) {
	mm := &mockExplainModel{
		mockModel: mockModel{
			result: &model.ReviewResult{Summary: "ok"},
		},
		explanation: "This change adds per-user rate limiting by moving the rate limit check after the authentication middleware. The `getToken()` call extracts the JWT, and `Claims.Subject` identifies the user.",
		explainUsage: &model.TokenUsage{InputTokens: 800, OutputTokens: 200, TotalTokens: 1000},
	}

	client := &capturingVCS{}
	ds := &mockDiffSource{diffs: integrationDiffs(), title: "Fix rate limiter"}
	cfg := ciConfig()
	cfg.Explain = true
	r := NewWithDiffSource(cfg, mm, client, ds)

	count, err := r.RunExplain(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("RunExplain should always return 0, got %d", count)
	}

	if mm.explainCalls != 1 {
		t.Errorf("expected 1 Explain call, got %d", mm.explainCalls)
	}

	if len(client.postedNotes) != 1 {
		t.Fatalf("expected 1 posted note, got %d", len(client.postedNotes))
	}

	note := client.postedNotes[0]

	assertions := []struct {
		desc string
		want string
	}{
		{"header", "## 🔍 Diff Explanation"},
		{"explanation body", "per-user rate limiting"},
		{"technical detail", "Claims.Subject"},
	}

	for _, a := range assertions {
		if !strings.Contains(note, a.want) {
			t.Errorf("explain note missing %s: wanted %q in:\n%s", a.desc, a.want, note)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 7: Intent-aware review — two-pass CI note
// ---------------------------------------------------------------------------

func TestIntegration_IntentAware_CINote(t *testing.T) {
	intentResult := &model.SummaryResult{
		Classification: "fix",
		Intent:         "Fix nil pointer in auth handler",
		RiskLevel:      "high",
		ScopeAreas:     []string{"auth"},
		Confidence:     0.92,
	}

	findings := []model.Finding{
		{File: "internal/auth/handler.go", Line: 11, Severity: "HIGH", Category: "bug", Title: "Nil deref", Body: "token used before nil check"},
	}

	mm := &mockSummarizeModel{
		mockModel: mockModel{
			result: &model.ReviewResult{
				Summary:  "Review with intent context",
				Findings: findings,
			},
		},
		summaryResult: intentResult,
	}

	client := &capturingVCS{}
	ds := &mockDiffSource{diffs: integrationDiffs(), title: "Fix auth NPE"}
	cfg := ciConfig()
	cfg.IntentReview = true
	r := NewWithDiffSource(cfg, mm, client, ds)

	count, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 finding, got %d", count)
	}

	// Verify two-pass: Summarize (pass 1) + Review (pass 2).
	if mm.summarizeCalls != 1 {
		t.Errorf("expected 1 Summarize call (pass 1), got %d", mm.summarizeCalls)
	}
	if mm.calls != 1 {
		t.Errorf("expected 1 Review call (pass 2), got %d", mm.calls)
	}

	// Verify posted note contains intent markdown.
	if len(client.postedNotes) != 1 {
		t.Fatalf("expected 1 posted note, got %d", len(client.postedNotes))
	}

	note := client.postedNotes[0]

	// Intent section should be prepended.
	if !strings.Contains(note, "🎯 Inferred Intent") {
		t.Error("posted note missing intent section header")
	}
	if !strings.Contains(note, "`fix`") {
		t.Error("posted note missing classification")
	}
	if !strings.Contains(note, "Fix nil pointer in auth handler") {
		t.Error("posted note missing intent text")
	}
	if !strings.Contains(note, "`auth`") {
		t.Error("posted note missing scope area")
	}

	// Review findings should also be present.
	if !strings.Contains(note, "Nil deref") {
		t.Error("posted note missing review finding title")
	}
}

// ---------------------------------------------------------------------------
// Test 8: Token budget — files trimmed when over budget
// ---------------------------------------------------------------------------

func TestIntegration_TokenBudget_TrimFiles(t *testing.T) {
	// Create diffs that exceed a tight budget. Each file with a line is ~10 tokens.
	// With MaxTokens=50, only a subset should reach the model.
	largeDiffs := makeTestDiffs("file1.go", "file2.go", "file3.go", "file4.go", "file5.go")

	findings := []model.Finding{
		{File: "file1.go", Line: 1, Severity: "LOW", Category: "style", Title: "t1", Body: "b1"},
	}
	mm := &mockModel{
		result: &model.ReviewResult{Summary: "budget test", Findings: findings},
	}

	cfg := ciConfig()
	cfg.MaxTokens = 50 // Very tight budget.
	ds := &mockDiffSource{diffs: largeDiffs}
	r := NewWithDiffSource(cfg, mm, &capturingVCS{}, ds)

	_, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Model should still be called (some files fit the budget).
	if mm.calls < 1 {
		t.Error("expected at least 1 model call even with tight budget")
	}

	// The model prompt should NOT contain all 5 files.
	prompt := mm.lastUserPrompt
	fileCount := 0
	for _, f := range []string{"file1.go", "file2.go", "file3.go", "file4.go", "file5.go"} {
		if strings.Contains(prompt, f) {
			fileCount++
		}
	}
	if fileCount == 5 {
		t.Error("expected token budget to trim some files, but all 5 were in the prompt")
	}
}

// ---------------------------------------------------------------------------
// Test 9: Excluded patterns — filtered files don't reach model
// ---------------------------------------------------------------------------

func TestIntegration_ExcludedPatterns(t *testing.T) {
	diffs := []diff.FileDiff{
		{
			NewPath: "internal/auth/handler.go",
			Hunks: []diff.Hunk{{
				Header: "@@ -1,1 +1,2 @@", NewStart: 1, NewCount: 2, OldStart: 1, OldCount: 1,
				Lines: []diff.DiffLine{{Type: diff.LineAdded, Content: "// auth change", OldLineNo: 0, NewLineNo: 1}},
			}},
		},
		{
			NewPath: "api/v1/service.pb.go",
			Hunks: []diff.Hunk{{
				Header: "@@ -1,1 +1,2 @@", NewStart: 1, NewCount: 2, OldStart: 1, OldCount: 1,
				Lines: []diff.DiffLine{{Type: diff.LineAdded, Content: "// generated", OldLineNo: 0, NewLineNo: 1}},
			}},
		},
		{
			NewPath: "vendor/lib/util.go",
			Hunks: []diff.Hunk{{
				Header: "@@ -1,1 +1,2 @@", NewStart: 1, NewCount: 2, OldStart: 1, OldCount: 1,
				Lines: []diff.DiffLine{{Type: diff.LineAdded, Content: "// vendor", OldLineNo: 0, NewLineNo: 1}},
			}},
		},
		{
			NewPath: "go.sum",
			Hunks: []diff.Hunk{{
				Header: "@@ -1,1 +1,2 @@", NewStart: 1, NewCount: 2, OldStart: 1, OldCount: 1,
				Lines: []diff.DiffLine{{Type: diff.LineAdded, Content: "module hash", OldLineNo: 0, NewLineNo: 1}},
			}},
		},
	}

	mm := &mockModel{
		result: &model.ReviewResult{Summary: "filtered", Findings: nil},
	}

	cfg := ciConfig()
	cfg.ExcludedPatterns = []string{"go.sum", "*.lock", "vendor/*", "*.pb.go"}
	ds := &mockDiffSource{diffs: diffs}
	r := NewWithDiffSource(cfg, mm, &capturingVCS{}, ds)

	_, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Model should be called with only the non-excluded file.
	if mm.calls != 1 {
		t.Fatalf("expected 1 model call, got %d", mm.calls)
	}

	prompt := mm.lastUserPrompt
	if !strings.Contains(prompt, "handler.go") {
		t.Error("expected handler.go in model prompt (not excluded)")
	}
	for _, excluded := range []string{"service.pb.go", "vendor/lib/util.go", "go.sum"} {
		if strings.Contains(prompt, excluded) {
			t.Errorf("excluded file %q should not be in model prompt", excluded)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 10: PostNote error — propagated to caller
// ---------------------------------------------------------------------------

func TestIntegration_PostNoteError(t *testing.T) {
	mm := &mockModel{
		result: &model.ReviewResult{
			Summary:  "will fail to post",
			Findings: []model.Finding{{File: "main.go", Line: 1, Severity: "LOW", Category: "style", Title: "t", Body: "b"}},
		},
	}

	client := &capturingVCS{
		mockVCS: mockVCS{
			postNoteErr: fmt.Errorf("403 Forbidden: insufficient permissions"),
		},
	}

	ds := &mockDiffSource{diffs: makeTestDiffs("main.go")}
	r := NewWithDiffSource(ciConfig(), mm, client, ds)

	_, err := r.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from PostNote failure, got nil")
	}
	if !strings.Contains(err.Error(), "posting summary") {
		t.Errorf("expected 'posting summary' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "403 Forbidden") {
		t.Errorf("expected '403 Forbidden' in error, got: %v", err)
	}
}
