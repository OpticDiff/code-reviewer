package reviewer

import (
	"context"
	"strings"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/config"
	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

// seqModel returns a distinct result per call and records every user prompt.
type seqModel struct {
	results []*model.ReviewResult
	prompts []string
}

func (m *seqModel) Review(_ context.Context, _, userPrompt string) (*model.ReviewResult, error) {
	m.prompts = append(m.prompts, userPrompt)
	return m.results[len(m.prompts)-1], nil
}

func (m *seqModel) Close() {}

func runTwoChunks(t *testing.T, results ...*model.ReviewResult) *seqModel {
	t.Helper()
	diff.ModelTokenLimits["test-tiny"] = 15
	t.Cleanup(func() { delete(diff.ModelTokenLimits, "test-tiny") })

	cfg := &config.Config{
		NoCache:       true,
		Model:         "test-tiny",
		ChunkStrategy: config.ChunkStrategySplit,
		MinSeverity:   config.SeverityLow,
	}
	sm := &seqModel{results: results}
	ds := &mockDiffSource{diffs: makeTestDiffs("a/one.go", "b/two.go")}
	if _, err := NewWithDiffSource(cfg, sm, &mockVCS{}, ds).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(sm.prompts) != 2 {
		t.Fatalf("expected 2 chunk reviews, got %d", len(sm.prompts))
	}
	return sm
}

func TestRun_EveryChunkSeesAllChangedFiles(t *testing.T) {
	sm := runTwoChunks(t, &model.ReviewResult{Summary: "s1"}, &model.ReviewResult{Summary: "s2"})

	for i, p := range sm.prompts {
		for _, want := range []string{"a/one.go (+1 -0)", "b/two.go (+1 -0)"} {
			if !strings.Contains(p, want) {
				t.Errorf("chunk %d prompt missing %q:\n%s", i+1, want, p)
			}
		}
	}
}

func TestRun_SummaryCoversAllChunks(t *testing.T) {
	diff.ModelTokenLimits["test-tiny"] = 15
	defer delete(diff.ModelTokenLimits, "test-tiny")
	cfg := &config.Config{
		NoCache: true, CIMode: true, Model: "test-tiny",
		ChunkStrategy: config.ChunkStrategySplit, MinSeverity: config.SeverityLow,
		CommentMode: config.CommentModeNotes, CIProjectID: "1", CIMergeRequestID: "2",
	}
	sm := &seqModel{results: []*model.ReviewResult{{Summary: "First chunk looks fine."}, {Summary: "Second chunk has a race."}}}
	vc := &mockVCS{}
	ds := &mockDiffSource{diffs: makeTestDiffs("a/one.go", "b/two.go")}
	if _, err := NewWithDiffSource(cfg, sm, vc, ds).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if vc.submitReviewReq == nil {
		t.Fatal("no review submitted")
	}
	for _, want := range []string{"First chunk looks fine.", "Second chunk has a race."} {
		if !strings.Contains(vc.submitReviewReq.Summary, want) {
			t.Errorf("summary %q missing %q", vc.submitReviewReq.Summary, want)
		}
	}
}

func TestBuildNumberedDiff_KeepsHunkScopeHint(t *testing.T) {
	d := []diff.FileDiff{{NewPath: "x.go", Hunks: []diff.Hunk{{
		Header: "@@ -40,3 +40,4 @@ func (s *Server) Handle(ctx context.Context) error {",
		Lines:  []diff.DiffLine{{Type: diff.LineAdded, Content: "y", NewLineNo: 41}},
	}}}}
	if out := buildNumberedDiff(d); !strings.Contains(out, "func (s *Server) Handle(ctx context.Context) error {") {
		t.Errorf("scope hint dropped from numbered diff:\n%s", out)
	}
}

func TestBuildChangedFilesSection(t *testing.T) {
	d := []diff.FileDiff{{
		OldPath: "old.go", NewPath: "",
		IsDelete: true,
		Hunks: []diff.Hunk{{Lines: []diff.DiffLine{
			{Type: diff.LineRemoved}, {Type: diff.LineRemoved}, {Type: diff.LineContext},
		}}},
	}}
	out := buildChangedFilesSection(d)
	if !strings.Contains(out, "old.go (+0 -2)") {
		t.Errorf("deleted file not listed by old path with counts:\n%s", out)
	}
}

func TestBuildChangedFilesSection_Capped(t *testing.T) {
	var d []diff.FileDiff
	for i := 0; i < maxListedFiles+5; i++ {
		d = append(d, diff.FileDiff{NewPath: "f.go"})
	}
	if out := buildChangedFilesSection(d); !strings.Contains(out, "and 5 more") {
		t.Errorf("expected overflow marker, got:\n%s", out)
	}
}
