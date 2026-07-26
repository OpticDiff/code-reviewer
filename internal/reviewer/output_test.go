package reviewer

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/config"
	"github.com/OpticDiff/code-reviewer/internal/model"
	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

// ---------------------------------------------------------------------------
// ColorTerminalOutput tests (new colored output)
// ---------------------------------------------------------------------------

func TestColorTerminalOutput_NoColor_FallsBackToPlain(t *testing.T) {
	result := &model.ReviewResult{
		Summary: "test",
		Findings: []model.Finding{
			{File: "main.go", Line: 1, Severity: "HIGH", Title: "Bug", Body: "desc"},
		},
	}
	// useColor=false should produce output identical to TerminalOutput.
	out := ColorTerminalOutput(result, false)
	plain := TerminalOutput(result)
	if out != plain {
		t.Error("ColorTerminalOutput(false) should equal TerminalOutput")
	}
}

func TestColorTerminalOutput_WithColor_HasANSI(t *testing.T) {
	result := &model.ReviewResult{
		Summary: "Found issues.",
		Findings: []model.Finding{
			{File: "main.go", Line: 42, Severity: "CRITICAL", Title: "SQL Injection", Body: "User input in query.", Suggestion: "use parameterized queries"},
			{File: "main.go", Line: 55, Severity: "HIGH", Title: "Resource leak", Body: "Unclosed body."},
			{File: "config.go", Line: 10, Severity: "MEDIUM", Title: "Missing validation", Body: "No input check."},
			{File: "util.go", Line: 5, Severity: "LOW", Title: "Naming", Body: "Use better name."},
		},
	}
	out := ColorTerminalOutput(result, true)

	// Verify ANSI codes are present.
	if !strings.Contains(out, "\033[") {
		t.Error("expected ANSI escape codes in colored output")
	}

	// Verify box drawing.
	if !strings.Contains(out, "┌") {
		t.Error("expected box-drawing top border")
	}
	if !strings.Contains(out, "└") {
		t.Error("expected box-drawing bottom border")
	}

	// Verify severity badges.
	if !strings.Contains(out, "CRITICAL") {
		t.Error("expected CRITICAL severity in output")
	}
	if !strings.Contains(out, "🔴") {
		t.Error("expected 🔴 in severity counts")
	}

	// Verify file separators.
	if !strings.Contains(out, "── main.go") {
		t.Error("expected file separator for main.go")
	}

	// Verify suggestion block.
	if !strings.Contains(out, "Suggestion:") {
		t.Error("expected 'Suggestion:' label")
	}

	// Verify summary counts.
	if !strings.Contains(out, "1 critical") {
		t.Error("expected '1 critical' in summary")
	}
	if !strings.Contains(out, "1 high") {
		t.Error("expected '1 high' in summary")
	}
}

func TestColorTerminalOutput_NoFindings_ShowsClean(t *testing.T) {
	result := &model.ReviewResult{
		Summary:  "Clean code.",
		Findings: nil,
	}
	out := ColorTerminalOutput(result, true)
	if !strings.Contains(out, "No issues found") {
		t.Error("expected 'No issues found' in colored empty output")
	}
	if !strings.Contains(out, "┌") {
		t.Error("expected box drawing even with no findings")
	}
}

// ---------------------------------------------------------------------------
// formatSummaryNote tests (GitLab posting)
// ---------------------------------------------------------------------------

func TestFormatSummaryNote_WithFindings(t *testing.T) {
	result := &model.ReviewResult{
		Summary: "Mixed issues.",
		Findings: []model.Finding{
			{File: "a.go", Line: 1, Severity: "CRITICAL", Title: "critical", Body: "c"},
			{File: "a.go", Line: 2, Severity: "HIGH", Title: "high", Body: "h"},
			{File: "a.go", Line: 3, Severity: "HIGH", Title: "high2", Body: "h2"},
			{File: "a.go", Line: 4, Severity: "LOW", Title: "low", Body: "l"},
		},
	}
	out := formatSummaryNote(result)

	if !strings.Contains(out, "📋 Code Review Summary") {
		t.Error("expected summary header")
	}
	if !strings.Contains(out, "CRITICAL") {
		t.Error("expected CRITICAL in table")
	}
	// HIGH should show count 2.
	if !strings.Contains(out, "2") {
		t.Error("expected count of 2 for HIGH")
	}
}

func TestFormatSummaryNote_NoFindings(t *testing.T) {
	result := &model.ReviewResult{
		Summary:  "All clean.",
		Findings: nil,
	}
	out := formatSummaryNote(result)
	if !strings.Contains(out, "No issues found") {
		t.Error("expected 'No issues found' in summary note")
	}
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestSeverityEmoji_AllCases(t *testing.T) {
	tests := []struct {
		severity string
		emoji    string
	}{
		{"CRITICAL", "🔴"},
		{"critical", "🔴"},
		{"HIGH", "🟠"},
		{"high", "🟠"},
		{"MEDIUM", "🟡"},
		{"LOW", "🔵"},
		{"UNKNOWN", "⚪"},
		{"", "⚪"},
	}
	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			got := severityEmoji(tt.severity)
			if got != tt.emoji {
				t.Errorf("severityEmoji(%q) = %q, want %q", tt.severity, got, tt.emoji)
			}
		})
	}
}

func TestSeverityColor(t *testing.T) {
	tests := []struct {
		severity string
		color    string
	}{
		{"CRITICAL", ansiRed},
		{"HIGH", ansiOrange},
		{"MEDIUM", ansiYellow},
		{"LOW", ansiBlue},
		{"UNKNOWN", ansiWhite},
	}
	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			got := severityColor(tt.severity)
			if got != tt.color {
				t.Errorf("severityColor(%q) = %q, want %q", tt.severity, got, tt.color)
			}
		})
	}
}

func TestFormatInlineComment_WithoutSuggestion(t *testing.T) {
	f := model.Finding{
		Severity: "HIGH",
		Title:    "Resource leak",
		Body:     "File handle not closed.",
	}
	out := formatInlineComment(f)
	if !strings.Contains(out, "🟠") {
		t.Error("expected HIGH emoji")
	}
	if !strings.Contains(out, "Resource leak") {
		t.Error("expected title")
	}
	if strings.Contains(out, "```suggestion") {
		t.Error("should not have suggestion block when suggestion is empty")
	}
}
func TestPostReview_PassesSuggestionAndCleanupMode(t *testing.T) {
	tests := []struct {
		name        string
		suggestion  string
		cleanupMode config.CleanupMode
	}{
		{"with_suggestion_and_delete", "fixed := sanitize(input)", config.CleanupModeDelete},
		{"with_suggestion_and_resolve", "return fmt.Errorf(\"wrap: %w\", err)", config.CleanupModeResolve},
		{"no_suggestion", "", config.CleanupModeDelete},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &outputMockVCS{}
			cfg := &config.Config{
				CIMode:           true,
				CIProjectID:      "proj",
				CIMergeRequestID: "1",
				CommentMode:      config.CommentModeDiscussions,
				CleanupMode:      tt.cleanupMode,
			}
			result := &model.ReviewResult{
				Summary: "Review",
				Findings: []model.Finding{
					{File: "a.go", Line: 5, Severity: "HIGH", Category: "bug", Title: "issue", Body: "desc", Suggestion: tt.suggestion},
				},
			}
			version := &vcs.DiffVersion{HeadSHA: "h", BaseSHA: "b", StartSHA: "s"}

			if err := PostReview(context.Background(), cfg, mockClient, result, version); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			req := mockClient.submitReviewReq
			if req == nil {
				t.Fatal("expected SubmitReview to be called")
			}
			if req.CleanupMode != string(tt.cleanupMode) {
				t.Errorf("CleanupMode = %q, want %q", req.CleanupMode, tt.cleanupMode)
			}
			if len(req.Comments) != 1 {
				t.Fatalf("expected 1 comment, got %d", len(req.Comments))
			}
			if req.Comments[0].Suggestion != tt.suggestion {
				t.Errorf("Suggestion = %q, want %q", req.Comments[0].Suggestion, tt.suggestion)
			}
		})
	}
}


func TestTokenUsageRendering(t *testing.T) {
	finding := model.Finding{File: "a.go", Line: 1, Severity: "LOW", Category: "style", Title: "test", Body: "body"}

	tests := []struct {
		name         string
		findings     []model.Finding
		usage        *model.TokenUsage
		wantContains []string // substrings that must appear
		wantAbsent   []string // substrings that must not appear
	}{
		{
			name:         "with findings and usage",
			findings:     []model.Finding{finding},
			usage:        &model.TokenUsage{InputTokens: 1500, OutputTokens: 200, TotalTokens: 1700},
			wantContains: []string{"1500", "200", "1700"},
		},
		{
			name:       "nil usage hidden",
			findings:   nil,
			usage:      nil,
			wantAbsent: []string{"Tokens:"},
		},
		{
			name:         "no findings but usage shown",
			findings:     nil,
			usage:        &model.TokenUsage{InputTokens: 3000, OutputTokens: 100, TotalTokens: 3100},
			wantContains: []string{"3000", "3100"},
		},
		{
			name:       "findings present but nil usage hidden",
			findings:   []model.Finding{finding},
			usage:      nil,
			wantAbsent: []string{"Tokens:"},
		},
	}

	renderers := []struct {
		name   string
		render func(*model.ReviewResult) string
	}{
		{"PlainText", func(r *model.ReviewResult) string { return TerminalOutput(r) }},
		{"Color", func(r *model.ReviewResult) string { return ColorTerminalOutput(r, true) }},
	}

	for _, tt := range tests {
		for _, renderer := range renderers {
			t.Run(renderer.name+"/"+tt.name, func(t *testing.T) {
				result := &model.ReviewResult{
					Summary:  "Review.",
					Findings: tt.findings,
					Usage:    tt.usage,
				}
				out := renderer.render(result)
				for _, s := range tt.wantContains {
					if !strings.Contains(out, s) {
						t.Errorf("expected %q in output, got:\n%s", s, out)
					}
				}
				for _, s := range tt.wantAbsent {
					if strings.Contains(out, s) {
						t.Errorf("unexpected %q in output, got:\n%s", s, out)
					}
				}
			})
		}
	}
}

// ---------------------------------------------------------------------------
// outputMockVCS — flexible mock for PostToGitLab tests
// ---------------------------------------------------------------------------

type outputMockVCS struct {
	mu sync.Mutex

	// Call tracking.
	postNoteCalls         int
	createDiscussionCalls int
	cleanCalls            int

	// CreateDiscussion behavior: called with the current call index (0-based).
	// Return nil for success, non-nil for failure.
	createDiscussionFunc func(callIndex int) error

	// Fixed responses.
	postNoteErr error
	cleanResult int
	cleanErr    error

	submitReviewCalls int
	submitReviewReq   *vcs.SubmitReviewRequest
	submitReviewErr   error

	getDescriptionCalls int
	getDescription      string
	getDescriptionErr   error
	setDescriptionCalls int
	setDescriptionVal   string
	setDescriptionErr   error
}

func (m *outputMockVCS) GetMRChanges(context.Context, string, string) (*vcs.MRChanges, error) {
	return nil, nil
}

func (m *outputMockVCS) GetMRVersions(context.Context, string, string) ([]vcs.DiffVersion, error) {
	return nil, nil
}

func (m *outputMockVCS) CompareCommits(context.Context, string, string, string) ([]string, error) {
	return nil, nil
}

func (m *outputMockVCS) PostNote(_ context.Context, _, _, _ string) (*vcs.Comment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.postNoteCalls++
	return &vcs.Comment{ID: m.postNoteCalls}, m.postNoteErr
}

func (m *outputMockVCS) CreateDiscussion(_ context.Context, _, _ string, _ vcs.InlineCommentRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := m.createDiscussionCalls
	m.createDiscussionCalls++
	if m.createDiscussionFunc != nil {
		return m.createDiscussionFunc(idx)
	}
	return nil
}

func (m *outputMockVCS) ListBotNotes(context.Context, string, string) ([]vcs.Comment, error) {
	return nil, nil
}

func (m *outputMockVCS) DeleteNote(context.Context, string, string, int) error {
	return nil
}

func (m *outputMockVCS) CleanPreviousReviews(context.Context, string, string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanCalls++
	return m.cleanResult, m.cleanErr
}

func (m *outputMockVCS) SubmitReview(_ context.Context, _, _ string, req vcs.SubmitReviewRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.submitReviewCalls++
	m.submitReviewReq = &req
	return m.submitReviewErr
}

func (m *outputMockVCS) GetDescription(ctx context.Context, projectID, mrIID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getDescriptionCalls++
	return m.getDescription, m.getDescriptionErr
}

func (m *outputMockVCS) SetDescription(ctx context.Context, projectID, mrIID, description string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setDescriptionCalls++
	m.setDescriptionVal = description
	return m.setDescriptionErr
}

// Compile-time check that outputMockVCS implements VCSClient and DescriptionUpdater.
var _ VCSClient = (*outputMockVCS)(nil)
var _ vcs.DescriptionUpdater = (*outputMockVCS)(nil)

// ---------------------------------------------------------------------------
// PostToGitLab tests
// ---------------------------------------------------------------------------

func TestPostReview_BuildsSubmitRequest(t *testing.T) {
	mockClient := &outputMockVCS{}

	cfg := &config.Config{
		CommentMode:      config.CommentModeDiscussions,
		CIProjectID:      "proj",
		CIMergeRequestID: "1",
	}

	result := &model.ReviewResult{
		Summary: "Found 2 issues.",
		Findings: []model.Finding{
			{File: "main.go", Line: 10, Severity: "HIGH", Title: "Bug", Body: "desc"},
			{File: "util.go", Line: 5, Severity: "LOW", Title: "Style", Body: "nit"},
		},
	}

	version := &vcs.DiffVersion{
		ID: 1, HeadSHA: "head", BaseSHA: "base", StartSHA: "start",
	}

	err := PostReview(context.Background(), cfg, mockClient, result, version)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mockClient.submitReviewCalls != 1 {
		t.Fatalf("expected 1 SubmitReview call, got %d", mockClient.submitReviewCalls)
	}

	req := mockClient.submitReviewReq
	if !strings.Contains(req.Summary, "📋 Code Review Summary") {
		t.Error("summary missing header")
	}
	if len(req.Comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(req.Comments))
	}
	if req.Comments[0].Path != "main.go" || req.Comments[0].Line != 10 {
		t.Errorf("comment[0] = %s:%d, want main.go:10", req.Comments[0].Path, req.Comments[0].Line)
	}
	if req.Version != version {
		t.Error("expected version to be passed through")
	}
}

func TestPostReview_NotesMode_NoComments(t *testing.T) {
	mockClient := &outputMockVCS{}

	cfg := &config.Config{
		CommentMode:      config.CommentModeNotes,
		CIProjectID:      "proj",
		CIMergeRequestID: "1",
	}

	result := &model.ReviewResult{
		Summary: "Found 1 issue.",
		Findings: []model.Finding{
			{File: "main.go", Line: 10, Severity: "HIGH", Title: "Bug", Body: "desc"},
		},
	}

	err := PostReview(context.Background(), cfg, mockClient, result, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := mockClient.submitReviewReq
	if len(req.Comments) != 0 {
		t.Errorf("notes mode should not include inline comments, got %d", len(req.Comments))
	}
}

func TestPostReview_UpdateDescription(t *testing.T) {
	mockClient := &outputMockVCS{
		getDescription: "Old description",
	}

	cfg := &config.Config{
		CommentMode:       config.CommentModeNotes,
		CIProjectID:       "proj",
		CIMergeRequestID:  "1",
		UpdateDescription: true,
	}

	result := &model.ReviewResult{
		Summary: "Summary update",
	}

	err := PostReview(context.Background(), cfg, mockClient, result, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mockClient.getDescriptionCalls != 1 {
		t.Errorf("expected 1 GetDescription call, got %d", mockClient.getDescriptionCalls)
	}
	if mockClient.setDescriptionCalls != 1 {
		t.Errorf("expected 1 SetDescription call, got %d", mockClient.setDescriptionCalls)
	}
	if !strings.Contains(mockClient.setDescriptionVal, "Summary update") {
		t.Errorf("expected description to contain summary, got %q", mockClient.setDescriptionVal)
	}
}
