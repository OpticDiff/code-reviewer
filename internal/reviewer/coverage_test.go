package reviewer

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/config"
	"github.com/OpticDiff/code-reviewer/internal/model"
	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

func coverageChanges() *vcs.MRChanges {
	return &vcs.MRChanges{
		Title: "t",
		Changes: []vcs.DiffEntry{
			{OldPath: "a.go", NewPath: "a.go", Diff: "@@ -1,1 +1,2 @@\n a\n+b\n"},
			{OldPath: "big.go", NewPath: "big.go", TooLarge: true},
			{OldPath: "fold.go", NewPath: "fold.go", Collapsed: true},
			{OldPath: "empty.go", NewPath: "empty.go"},
			{OldPath: "go.sum", NewPath: "go.sum", TooLarge: true},
		},
	}
}

func TestRun_CoverageReportsUnreviewedFiles(t *testing.T) {
	auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
	cfg := &config.Config{
		NoCache:          true,
		CIMode:           true,
		AutoApprove:      true,
		AuditLog:         auditPath,
		Model:            "gemini-2.5-flash",
		ChunkStrategy:    config.ChunkStrategyFail,
		MinSeverity:      config.SeverityLow,
		CommentMode:      config.CommentModeNotes,
		CIProjectID:      "123",
		CIMergeRequestID: "456",
		ExcludedPatterns: []string{"go.sum"},
	}
	client := &mockVCS{mrChanges: coverageChanges()}
	mm := &mockModel{result: &model.ReviewResult{Summary: "ok"}}

	if _, err := New(cfg, mm, client).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := "Reviewed 1 of 4 files. Not reviewed: 3 (too_large 1, collapsed 1, empty_patch 1)."
	if client.submitReviewReq == nil || !strings.Contains(client.submitReviewReq.Summary, want) {
		t.Errorf("summary missing coverage line %q; got %+v", want, client.submitReviewReq)
	}
	if client.approveCalls != 0 {
		t.Errorf("auto-approve must refuse when files were not reviewed, got %d approve calls", client.approveCalls)
	}

	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("reading audit log: %v", err)
	}
	var entry AuditEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("decoding audit entry: %v", err)
	}
	if len(entry.FilesSkipped) != 3 {
		t.Errorf("files_skipped = %v, want 3 entries", entry.FilesSkipped)
	}
	reasons := map[string]string{}
	for _, s := range entry.FilesSkippedDetail {
		reasons[s.Path] = s.Reason
	}
	wantReasons := map[string]string{"big.go": "too_large", "fold.go": "collapsed", "empty.go": "empty_patch"}
	for p, r := range wantReasons {
		if reasons[p] != r {
			t.Errorf("reason for %s = %q, want %q (all: %v)", p, reasons[p], r, reasons)
		}
	}
}

func TestRun_NoCoverageLineWhenEverythingReviewed(t *testing.T) {
	cfg := &config.Config{
		NoCache: true, CIMode: true, Model: "gemini-2.5-flash",
		ChunkStrategy: config.ChunkStrategyFail, MinSeverity: config.SeverityLow,
		CommentMode: config.CommentModeNotes, CIProjectID: "1", CIMergeRequestID: "2",
	}
	changes := &vcs.MRChanges{Changes: coverageChanges().Changes[:1]}
	client := &mockVCS{mrChanges: changes}
	mm := &mockModel{result: &model.ReviewResult{Summary: "ok"}}

	if _, err := New(cfg, mm, client).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if client.submitReviewReq == nil || strings.Contains(client.submitReviewReq.Summary, "Not reviewed") {
		t.Errorf("unexpected coverage line in %+v", client.submitReviewReq)
	}
}

func TestCoverageLine(t *testing.T) {
	skipped := []SkippedFile{
		{Path: "a", Reason: SkipEmptyPatch}, {Path: "b", Reason: SkipEmptyPatch},
		{Path: "c", Reason: SkipTooLarge}, {Path: "d", Reason: SkipBudget},
		{Path: "e", Reason: SkipParseError},
	}
	got := coverageLine(10, 15, skipped)
	want := "Reviewed 10 of 15 files. Not reviewed: 5 (too_large 1, empty_patch 2, budget 1, parse_error 1)."
	if got != want {
		t.Errorf("coverageLine = %q, want %q", got, want)
	}
	if got := coverageLine(3, 3, nil); got != "" {
		t.Errorf("coverageLine with nothing skipped = %q, want empty", got)
	}
}

func TestSkipReasonFor(t *testing.T) {
	tests := []struct {
		name  string
		entry vcs.DiffEntry
		want  string
	}{
		{"normal patch", vcs.DiffEntry{Diff: "@@ -1 +1 @@\n-a\n+b\n"}, ""},
		{"too large", vcs.DiffEntry{TooLarge: true}, SkipTooLarge},
		{"collapsed", vcs.DiffEntry{Collapsed: true}, SkipCollapsed},
		{"pure rename", vcs.DiffEntry{OldPath: "a", NewPath: "b", RenamedFile: true}, ""},
		{"empty new file", vcs.DiffEntry{NewPath: "a", NewFile: true}, ""},
		{"deleted empty file", vcs.DiffEntry{OldPath: "a", DeletedFile: true}, ""},
		{"modified file with no patch", vcs.DiffEntry{OldPath: "a", NewPath: "a"}, SkipEmptyPatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := skipReasonFor(tt.entry); got != tt.want {
				t.Errorf("skipReasonFor = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEmptyPatchDoesNotBlockApproval(t *testing.T) {
	c := buildCoverage(1, []SkippedFile{{Path: "bin.png", Reason: SkipEmptyPatch}, {Path: "big.go", Reason: SkipTooLarge}}, nil, nil, nil)
	got := c.blockingPaths()
	if len(got) != 1 || got[0] != "big.go" {
		t.Errorf("blockingPaths = %v, want [big.go]", got)
	}
}

func TestRun_AllFilesUnreviewablePostsCoverageNote(t *testing.T) {
	cfg := &config.Config{
		NoCache: true, CIMode: true, Model: "gemini-2.5-flash",
		ChunkStrategy: config.ChunkStrategyFail, MinSeverity: config.SeverityLow,
		CommentMode: config.CommentModeNotes, CIProjectID: "1", CIMergeRequestID: "2",
	}
	changes := &vcs.MRChanges{Changes: []vcs.DiffEntry{{OldPath: "big.go", NewPath: "big.go", TooLarge: true}}}
	client := &mockVCS{mrChanges: changes}
	mm := &mockModel{result: &model.ReviewResult{Summary: "ok"}}

	if _, err := New(cfg, mm, client).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if client.submitReviewCalls != 1 || client.submitReviewReq == nil ||
		!strings.Contains(client.submitReviewReq.Summary, "Reviewed 0 of 1 files") {
		t.Errorf("want one SubmitReview carrying the coverage line, got calls=%d req=%+v", client.submitReviewCalls, client.submitReviewReq)
	}
	if client.cleanCalls != 1 {
		t.Errorf("cleanCalls = %d; the note must replace earlier bot summaries so reruns do not stack", client.cleanCalls)
	}
	if strings.Contains(client.submitReviewReq.Summary, "No issues found") {
		t.Errorf("coverage-only note must not claim success: %q", client.submitReviewReq.Summary)
	}
	if mm.calls != 0 {
		t.Errorf("model called %d times, want 0", mm.calls)
	}
}

func TestRun_IncrementalCoverageIgnoresParseFailuresOutsideChangedSet(t *testing.T) {
	run := func(incremental bool) []string {
		auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
		cfg := &config.Config{
			NoCache: true, CIMode: true, Incremental: incremental, AuditLog: auditPath,
			Model: "gemini-2.5-flash", ChunkStrategy: config.ChunkStrategyFail,
			MinSeverity: config.SeverityLow, CommentMode: config.CommentModeNotes,
			CIProjectID: "1", CIMergeRequestID: "2",
		}
		client := &mockVCS{
			mrChanges: &vcs.MRChanges{Changes: []vcs.DiffEntry{
				{OldPath: "a.go", NewPath: "a.go", Diff: "@@ -1,1 +1,2 @@\n a\n+b\n"},
				{OldPath: "broken.go", NewPath: "broken.go", Diff: "@@ nonsense\n"},
			}},
			mrVersions:   []vcs.DiffVersion{{ID: 2, HeadSHA: "new"}, {ID: 1, HeadSHA: "old"}},
			compareFiles: []string{"a.go"},
		}
		mm := &mockModel{result: &model.ReviewResult{Summary: "ok"}}
		if _, err := New(cfg, mm, client).Run(context.Background()); err != nil {
			t.Fatalf("Run: %v", err)
		}
		data, err := os.ReadFile(auditPath)
		if err != nil {
			t.Fatalf("reading audit log: %v", err)
		}
		var entry AuditEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return entry.FilesSkipped
	}

	if got := run(false); len(got) != 1 || got[0] != "broken.go" {
		t.Fatalf("control: full review files_skipped = %v, want [broken.go]", got)
	}
	if got := run(true); len(got) != 0 {
		t.Errorf("incremental files_skipped = %v, want none (broken.go is outside the changed set)", got)
	}
}
