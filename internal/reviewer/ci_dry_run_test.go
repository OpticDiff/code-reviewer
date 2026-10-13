package reviewer

import (
	"context"
	"strings"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/config"
	"github.com/OpticDiff/code-reviewer/internal/model"
	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

func ciDryRunCfg() *config.Config {
	return &config.Config{
		NoCache: true, CIMode: true, DryRun: true, OutputJSON: true, Model: "m",
		ChunkStrategy: config.ChunkStrategyFail, MinSeverity: config.SeverityLow,
		CommentMode: config.CommentModeDiscussions, CIProjectID: "1", CIMergeRequestID: "2",
	}
}

func ciDryRunChanges() *vcs.MRChanges {
	return &vcs.MRChanges{
		Title: "Add main",
		Changes: []vcs.DiffEntry{{
			OldPath: "main.go", NewPath: "main.go",
			Diff: "@@ -1,1 +1,2 @@\n package main\n+x := compute()\n",
		}},
	}
}

func assertNoWrites(t *testing.T, c *mockVCS) {
	t.Helper()
	if c.postNoteCalls+c.submitReviewCalls+c.cleanCalls+c.createDiscussionCalls+c.deleteNoteCalls+c.approveCalls != 0 {
		t.Errorf("dry-run wrote to the platform: postNote=%d submitReview=%d clean=%d createDiscussion=%d deleteNote=%d approve=%d",
			c.postNoteCalls, c.submitReviewCalls, c.cleanCalls, c.createDiscussionCalls, c.deleteNoteCalls, c.approveCalls)
	}
}

func TestRun_CIDryRunReadsMergeRequestWithoutWriting(t *testing.T) {
	client := &mockVCS{mrChanges: ciDryRunChanges()}
	mm := &mockModel{result: &model.ReviewResult{Summary: "s", Findings: []model.Finding{
		{File: "main.go", Line: 2, Severity: "HIGH", Category: "bug", Title: "t", Body: "b"},
	}}}

	count, err := New(ciDryRunCfg(), mm, client).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if count != 1 || mm.calls != 1 {
		t.Errorf("count=%d model calls=%d, want 1 and 1", count, mm.calls)
	}
	if client.getMRChangesCalls != 1 {
		t.Errorf("GetMRChanges calls = %d, want 1", client.getMRChangesCalls)
	}
	assertNoWrites(t, client)
}

func TestRun_CIDryRunAppliesDismissedThreads(t *testing.T) {
	finding := model.Finding{File: "main.go", Line: 2, Severity: "LOW", Category: "bug", Title: "t", Body: "b"}
	posted := fingerprinted(finding, dismissedTestDiffs("x := compute()"))
	client := &threadVCS{
		mockVCS:   &mockVCS{mrChanges: ciDryRunChanges()},
		dismissed: []vcs.DismissedFinding{personDismissal(posted)},
	}
	mm := &mockModel{result: &model.ReviewResult{Summary: "s", Findings: []model.Finding{finding}}}

	count, err := New(ciDryRunCfg(), mm, client).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0: the dismissed finding must be suppressed in the preview too", count)
	}
	if !strings.Contains(mm.lastSystemPrompt, "main.go:2") {
		t.Error("dismissed location should reach the prompt in dry-run")
	}
	assertNoWrites(t, client.mockVCS)
}

func TestRun_CIDryRunAppliesOpenThreads(t *testing.T) {
	finding := model.Finding{File: "main.go", Line: 2, Severity: "MEDIUM", Category: "bug", Title: "t", Body: "b"}
	posted := fingerprinted(finding, dismissedTestDiffs("x := compute()"))
	client := &threadVCS{
		mockVCS: &mockVCS{mrChanges: ciDryRunChanges()},
		open:    []vcs.OpenFinding{openThreadFor(posted)},
	}
	mm := &mockModel{result: &model.ReviewResult{Summary: "s", Findings: []model.Finding{finding}}}

	count, err := New(ciDryRunCfg(), mm, client).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1: open findings still count toward the summary and exit status", count)
	}
	if client.openCalls != 1 {
		t.Errorf("ListOpenFindings calls = %d, want 1 in dry-run", client.openCalls)
	}
	assertNoWrites(t, client.mockVCS)
}

func TestRun_CIWithoutClientIsAnErrorNotAPanic(t *testing.T) {
	mm := &mockModel{result: &model.ReviewResult{Summary: "s"}}
	_, err := New(ciDryRunCfg(), mm, nil).Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "platform client") {
		t.Fatalf("Run error = %v, want a clear missing-client error", err)
	}
}
