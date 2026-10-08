package reviewer

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/OpticDiff/code-reviewer/internal/cache"
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

// bigDiffs returns one-hunk diffs of roughly 1000 tokens each, so a limit of
// 1600 puts each file in its own chunk.
func bigDiffs(files ...string) []diff.FileDiff {
	ds := makeTestDiffs(files...)
	for i := range ds {
		ds[i].Hunks[0].Lines[0].Content = strings.Repeat("x", 4000)
	}
	return ds
}

const chunkTestLimit = 1600

func chunkCfg(t *testing.T) *config.Config {
	t.Helper()
	diff.ModelTokenLimits["test-chunk"] = chunkTestLimit
	t.Cleanup(func() { delete(diff.ModelTokenLimits, "test-chunk") })
	return &config.Config{
		NoCache: true, CIMode: true, Model: "test-chunk",
		ChunkStrategy: config.ChunkStrategySplit, MinSeverity: config.SeverityLow,
		CommentMode: config.CommentModeNotes, CIProjectID: "1", CIMergeRequestID: "2",
	}
}

func runChunks(t *testing.T, cfg *config.Config, diffs []diff.FileDiff, wantChunks int, results ...*model.ReviewResult) (*seqModel, *mockVCS) {
	t.Helper()
	sm := &seqModel{results: results}
	vc := &mockVCS{}
	ds := &mockDiffSource{diffs: diffs}
	if _, err := NewWithDiffSource(cfg, sm, vc, ds).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(sm.prompts) != wantChunks {
		t.Fatalf("expected %d chunk reviews, got %d", wantChunks, len(sm.prompts))
	}
	return sm, vc
}

func TestRun_EveryChunkSeesAllChangedFiles(t *testing.T) {
	sm, _ := runChunks(t, chunkCfg(t), bigDiffs("a/one.go", "b/two.go"), 2,
		&model.ReviewResult{Summary: "s1"}, &model.ReviewResult{Summary: "s2"})

	for i, p := range sm.prompts {
		for _, want := range []string{"a/one.go (+1 -0)", "b/two.go (+1 -0)"} {
			if !strings.Contains(p, want) {
				t.Errorf("chunk %d prompt missing %q", i+1, want)
			}
		}
	}
}

func TestRun_ChangedFilesIncludeCachedFiles(t *testing.T) {
	cfg := chunkCfg(t)
	cfg.NoCache = false
	cfg.CacheDir = t.TempDir()
	cfg.CacheMaxAge = time.Hour

	all := bigDiffs("a/cached.go", "b/two.go", "c/three.go")
	c, err := cache.New(cfg.CacheDir, cfg.CacheMaxAge)
	if err != nil {
		t.Fatal(err)
	}
	promptHash := cache.PromptHash(cfg.CustomPrompt, "", "", cfg.Focus, cfg.ExtraRules, "")
	key := cache.CacheKey(cache.DiffHash(all[0]), cfg.Model, promptHash)
	if err := c.Store(key, cache.Entry{FilePath: "a/cached.go", DiffHash: cache.DiffHash(all[0]), Model: cfg.Model}); err != nil {
		t.Fatal(err)
	}

	sm, _ := runChunks(t, cfg, all, 2, &model.ReviewResult{Summary: "s1"}, &model.ReviewResult{Summary: "s2"})
	for i, p := range sm.prompts {
		if !strings.Contains(p, "a/cached.go (+1 -0)") {
			t.Errorf("chunk %d prompt missing the cached file in the changed-file list", i+1)
		}
	}
}

func TestRun_SummaryCoversAllChunks(t *testing.T) {
	_, vc := runChunks(t, chunkCfg(t), bigDiffs("a/one.go", "b/two.go"), 2,
		&model.ReviewResult{Summary: "First chunk looks fine."}, &model.ReviewResult{Summary: "Second chunk has a race."})
	for _, want := range []string{"First chunk looks fine.", "Second chunk has a race."} {
		if !strings.Contains(vc.submitReviewReq.Summary, want) {
			t.Errorf("summary %q missing %q", vc.submitReviewReq.Summary, want)
		}
	}
}

func TestMergeChunkSummaries(t *testing.T) {
	if got := mergeChunkSummaries([]string{"same", " same ", "", "other"}); got != "same\n\nother" {
		t.Errorf("dedupe/skip-empty: got %q", got)
	}
	if got := mergeChunkSummaries([]string{"", "  "}); got != "" {
		t.Errorf("all empty: got %q", got)
	}
}

func TestRun_EmptyChunkSummarySkipped(t *testing.T) {
	_, vc := runChunks(t, chunkCfg(t), bigDiffs("a/one.go", "b/two.go"), 2,
		&model.ReviewResult{Summary: ""}, &model.ReviewResult{Summary: "Only the second."})
	if !strings.Contains(vc.submitReviewReq.Summary, "Only the second.") {
		t.Errorf("summary %q lost the non-empty chunk summary", vc.submitReviewReq.Summary)
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
	out := buildChangedFilesSection(d, 1000)
	if !strings.Contains(out, "old.go (+0 -2)") {
		t.Errorf("deleted file not listed by old path with counts:\n%s", out)
	}
}

func TestBuildChangedFilesSection_CappedByCount(t *testing.T) {
	var d []diff.FileDiff
	for i := 0; i < maxListedFiles+5; i++ {
		d = append(d, diff.FileDiff{NewPath: "f.go"})
	}
	if out := buildChangedFilesSection(d, 1<<20); !strings.Contains(out, "and 5 more") {
		t.Errorf("expected overflow marker, got:\n%s", out)
	}
}

func TestBuildChangedFilesSection_BoundedByTokenBudget(t *testing.T) {
	var d []diff.FileDiff
	for i := 0; i < 100; i++ {
		d = append(d, diff.FileDiff{NewPath: "some/long/path/to/a/file.go"})
	}
	const budget = 50
	out := buildChangedFilesSection(d, budget)
	if got := len(out) / 4; got > budget {
		t.Errorf("section is ~%d tokens, budget %d:\n%s", got, budget, out)
	}
	if !strings.Contains(out, "more") {
		t.Errorf("expected overflow marker:\n%s", out)
	}
	if out := buildChangedFilesSection(d, 1); out != "" {
		t.Errorf("expected empty section when nothing fits, got %q", out)
	}
}
