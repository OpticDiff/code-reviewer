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
