package reviewer

import (
	"context"
	"strings"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/config"
	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

func dismissedTestDiffs(content string) []diff.FileDiff {
	return []diff.FileDiff{{
		NewPath: "main.go",
		Hunks: []diff.Hunk{{
			Header: "@@ -1,1 +1,2 @@", NewStart: 1, NewCount: 2, OldStart: 1, OldCount: 1,
			Lines: []diff.DiffLine{
				{Type: diff.LineContext, Content: "package main", OldLineNo: 1, NewLineNo: 1},
				{Type: diff.LineAdded, Content: content, NewLineNo: 2},
			},
		}},
	}}
}

func fingerprinted(f model.Finding, d []diff.FileDiff) model.Finding {
	fs := []model.Finding{f}
	AssignFingerprints(fs, d)
	return fs[0]
}

func TestFingerprint_StableAcrossRewordingAndCategoryCase(t *testing.T) {
	d := dismissedTestDiffs("x := compute()")
	a := fingerprinted(model.Finding{File: "main.go", Line: 2, Category: "Security", Title: "Unchecked value"}, d)
	b := fingerprinted(model.Finding{File: "main.go", Line: 2, Category: " security ", Title: "Result ignored"}, d)
	if a.Fingerprint == "" || a.Fingerprint != b.Fingerprint {
		t.Errorf("fingerprint should ignore wording and category case: %q vs %q", a.Fingerprint, b.Fingerprint)
	}
}

func TestFingerprint_ChangesWhenAnchoredLinesChange(t *testing.T) {
	f := model.Finding{File: "main.go", Line: 2, Category: "bug"}
	before := fingerprinted(f, dismissedTestDiffs("x := compute()"))
	after := fingerprinted(f, dismissedTestDiffs("x, err := compute()"))
	if before.Fingerprint == after.Fingerprint {
		t.Error("fingerprint must change when the anchored lines change")
	}
}

func TestFingerprint_DistinguishesIdenticalLinesInDifferentPlaces(t *testing.T) {
	d := []diff.FileDiff{{
		NewPath: "main.go",
		Hunks: []diff.Hunk{{Lines: []diff.DiffLine{
			{Type: diff.LineAdded, Content: "a()", NewLineNo: 1},
			{Type: diff.LineAdded, Content: "return err", NewLineNo: 2},
			{Type: diff.LineAdded, Content: "b()", NewLineNo: 3},
			{Type: diff.LineAdded, Content: "return err", NewLineNo: 4},
			{Type: diff.LineAdded, Content: "c()", NewLineNo: 5},
		}}},
	}}
	first := fingerprinted(model.Finding{File: "main.go", Line: 2, Category: "bug"}, d)
	second := fingerprinted(model.Finding{File: "main.go", Line: 4, Category: "bug"}, d)
	if first.Fingerprint == "" || first.Fingerprint == second.Fingerprint {
		t.Error("identical line text in different places must not share a fingerprint")
	}
}

func TestFingerprint_EmptyWhenNotAnchored(t *testing.T) {
	got := fingerprinted(model.Finding{File: "other.go", Line: 9, Category: "bug"}, dismissedTestDiffs("x"))
	if got.Fingerprint != "" {
		t.Errorf("Fingerprint = %q, want empty for a line outside the diff", got.Fingerprint)
	}
}

func TestFilterDismissed(t *testing.T) {
	d := dismissedTestDiffs("x := compute()")
	f := fingerprinted(model.Finding{File: "main.go", Line: 2, Category: "bug", Severity: "LOW"}, d)
	other := fingerprinted(model.Finding{File: "main.go", Line: 2, Category: "security", Severity: "LOW"}, d)

	kept, suppressed := FilterDismissed([]model.Finding{f, other}, []vcs.DismissedFinding{{Fingerprint: f.Fingerprint}})
	if len(suppressed) != 1 || len(kept) != 1 || kept[0].Category != "security" {
		t.Errorf("kept %v suppressed %v, want only the security finding kept", kept, suppressed)
	}
}

func TestFilterDismissed_AuthorOnlyDoesNotSilenceHighSeverity(t *testing.T) {
	d := dismissedTestDiffs("x := compute()")
	crit := fingerprinted(model.Finding{File: "main.go", Line: 2, Category: "bug", Severity: "CRITICAL"}, d)
	low := fingerprinted(model.Finding{File: "main.go", Line: 2, Category: "style", Severity: "LOW"}, d)
	dis := []vcs.DismissedFinding{
		{Fingerprint: crit.Fingerprint, AuthorOnly: true},
		{Fingerprint: low.Fingerprint, AuthorOnly: true},
	}
	kept, suppressed := FilterDismissed([]model.Finding{crit, low}, dis)
	if len(kept) != 1 || kept[0].Severity != "CRITICAL" || len(suppressed) != 1 {
		t.Errorf("kept %v suppressed %v: author-only dismissal must keep CRITICAL and may suppress LOW", kept, suppressed)
	}
}

func TestFilterDismissed_KeepsUnfingerprinted(t *testing.T) {
	f := model.Finding{File: "main.go", Line: 2}
	kept, suppressed := FilterDismissed([]model.Finding{f}, []vcs.DismissedFinding{{Fingerprint: ""}})
	if len(suppressed) != 0 || len(kept) != 1 {
		t.Errorf("unfingerprinted finding must never be suppressed, kept=%d suppressed=%d", len(kept), len(suppressed))
	}
}

func TestKeepFingerprints_OnlyWhileAnchorUnchanged(t *testing.T) {
	d := dismissedTestDiffs("x := compute()")
	f := fingerprinted(model.Finding{File: "main.go", Line: 2, Category: "bug"}, d)
	dis := vcs.DismissedFinding{Fingerprint: f.Fingerprint, Anchor: f.Anchor, AnchorLines: f.AnchorLines, Path: "main.go", Line: 2}

	if got := KeepFingerprints([]vcs.DismissedFinding{dis}, nil, d); len(got) != 1 {
		t.Errorf("unchanged anchor: keep = %v, want the thread kept", got)
	}
	if got := KeepFingerprints([]vcs.DismissedFinding{dis}, nil, dismissedTestDiffs("fixed()")); len(got) != 0 {
		t.Errorf("changed anchor: keep = %v, want the stale thread released", got)
	}
	if got := KeepFingerprints([]vcs.DismissedFinding{dis}, []model.Finding{f}, d); len(got) != 0 {
		t.Errorf("re-posted finding: keep = %v, want old thread released", got)
	}
}

func TestFormatDismissedPrompt_PathsOnly(t *testing.T) {
	if FormatDismissedPrompt(nil) != "" {
		t.Error("no dismissed findings should yield an empty prompt section")
	}
	got := FormatDismissedPrompt([]vcs.DismissedFinding{
		{Path: "main.go", Line: 2},
		{Path: "outdated.go", Line: 0},
		{Path: "author.go", Line: 7, AuthorOnly: true},
	})
	for _, want := range []string{"- main.go:2\n", "- outdated.go\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt section missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "author.go") || strings.Contains(got, ":0") {
		t.Errorf("author-only threads and zero lines must not appear:\n%s", got)
	}
}

func TestFormatInlineComment_EmbedsFingerprint(t *testing.T) {
	f := model.Finding{Severity: "HIGH", Title: "t", Body: "b", Fingerprint: "abc123", Anchor: "def456", AnchorLines: 2}
	got, ok := vcs.ParseFingerprint(formatInlineComment(f))
	if !ok || got.Fingerprint != "abc123" || got.Anchor != "def456" || got.AnchorLines != 2 {
		t.Errorf("inline comment fingerprint = %+v, %v", got, ok)
	}
}

// threadVCS is a mockVCS that also reports dismissed threads.
type threadVCS struct {
	*mockVCS
	dismissed []vcs.DismissedFinding
}

func (t *threadVCS) ListDismissedFindings(context.Context, string, string) ([]vcs.DismissedFinding, error) {
	return t.dismissed, nil
}

func dismissedRunCfg() *config.Config {
	return &config.Config{
		NoCache: true, CIMode: true, Model: "m",
		ChunkStrategy: config.ChunkStrategyFail, MinSeverity: config.SeverityLow,
		CommentMode: config.CommentModeDiscussions, CIProjectID: "1", CIMergeRequestID: "2",
	}
}

func runWithDismissed(t *testing.T, severity string, current []diff.FileDiff, dis func(model.Finding) vcs.DismissedFinding) (int, *mockModel, *threadVCS) {
	t.Helper()
	finding := model.Finding{File: "main.go", Line: 2, Severity: severity, Category: "bug", Title: "Unchecked value", Body: "details"}
	posted := fingerprinted(finding, dismissedTestDiffs("x := compute()"))
	mm := &mockModel{result: &model.ReviewResult{Summary: "s", Findings: []model.Finding{finding}}}
	client := &threadVCS{
		mockVCS:   &mockVCS{mrVersions: []vcs.DiffVersion{{ID: 1, HeadSHA: "a", BaseSHA: "b", StartSHA: "c"}}},
		dismissed: []vcs.DismissedFinding{dis(posted)},
	}
	r := NewWithDiffSource(dismissedRunCfg(), mm, client, &mockDiffSource{diffs: current})
	count, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return count, mm, client
}

func personDismissal(f model.Finding) vcs.DismissedFinding {
	return vcs.DismissedFinding{Fingerprint: f.Fingerprint, Anchor: f.Anchor, AnchorLines: f.AnchorLines, Path: "main.go", Line: 2}
}

func TestRun_SuppressedLowFindingIsNotPostedAndDoesNotFailGate(t *testing.T) {
	count, mm, client := runWithDismissed(t, "LOW", dismissedTestDiffs("x := compute()"), personDismissal)
	if count != 0 {
		t.Errorf("count = %d, want 0 for a dismissed LOW finding", count)
	}
	if !strings.Contains(mm.lastSystemPrompt, "main.go:2") {
		t.Error("dismissed location should be passed to the model prompt")
	}
	if strings.Contains(mm.lastSystemPrompt, "Unchecked value") {
		t.Error("no comment text may reach the prompt")
	}
	if client.submitReviewReq == nil || len(client.submitReviewReq.Comments) != 0 {
		t.Errorf("no inline comment should be posted, got %+v", client.submitReviewReq)
	}
	if len(client.submitReviewReq.KeepFingerprints) != 1 {
		t.Errorf("KeepFingerprints = %v, want the dismissed thread kept", client.submitReviewReq.KeepFingerprints)
	}
}

func TestRun_SuppressedCriticalStillFailsGate(t *testing.T) {
	count, _, client := runWithDismissed(t, "CRITICAL", dismissedTestDiffs("x := compute()"), personDismissal)
	if count != 1 {
		t.Errorf("count = %d, want 1: a dismissed CRITICAL must still fail the CI gate", count)
	}
	if len(client.submitReviewReq.Comments) != 0 {
		t.Error("the dismissed CRITICAL must not be re-posted")
	}
}

func TestRun_AuthorOnlyDismissalRepostsCritical(t *testing.T) {
	count, _, client := runWithDismissed(t, "CRITICAL", dismissedTestDiffs("x := compute()"), func(f model.Finding) vcs.DismissedFinding {
		d := personDismissal(f)
		d.AuthorOnly = true
		return d
	})
	if count != 1 || len(client.submitReviewReq.Comments) != 1 {
		t.Errorf("count=%d comments=%d, want the CRITICAL re-posted", count, len(client.submitReviewReq.Comments))
	}
}

func TestRun_ReRaisesDismissedFindingWhenCodeChanged(t *testing.T) {
	count, _, client := runWithDismissed(t, "HIGH", dismissedTestDiffs("y := compute2()"), personDismissal)
	if count != 1 || len(client.submitReviewReq.Comments) != 1 {
		t.Errorf("count=%d comments=%d, want finding re-posted after the anchored lines changed", count, len(client.submitReviewReq.Comments))
	}
	if len(client.submitReviewReq.KeepFingerprints) != 0 {
		t.Errorf("KeepFingerprints = %v, want stale thread released", client.submitReviewReq.KeepFingerprints)
	}
}

func TestFormatInlineComment_NeutralizesModelWrittenMarkers(t *testing.T) {
	forged := vcs.FingerprintMarker(vcs.FingerprintInfo{Fingerprint: "bbbb", Anchor: "bbbb", AnchorLines: 1})
	f := model.Finding{
		Severity: "HIGH", Title: "t " + forged, Body: "b " + forged + " <!-- code-reviewer -->",
		Fingerprint: "aaaa", Anchor: "aaaa", AnchorLines: 1,
	}
	got := formatInlineComment(f)
	if strings.Count(got, "<!--") != 1 {
		t.Errorf("only the tool's own marker may remain:\n%s", got)
	}
	if info, ok := vcs.ParseFingerprint(got); !ok || info.Fingerprint != "aaaa" {
		t.Errorf("ParseFingerprint = %+v, %v; want the real fingerprint", info, ok)
	}
}

func TestAssignFingerprints_ClampsHugeEndLine(t *testing.T) {
	f := []model.Finding{{File: "main.go", Line: 2, EndLine: 1_000_000_000, Category: "bug"}}
	AssignFingerprints(f, dismissedTestDiffs("x := compute()"))
	if f[0].Fingerprint == "" || f[0].AnchorLines != 1 {
		t.Errorf("fingerprint=%q anchorLines=%d, want span clamped to the diff (1 line)", f[0].Fingerprint, f[0].AnchorLines)
	}
}

func TestAnchorUnchanged_RejectsHugeSpan(t *testing.T) {
	d := vcs.DismissedFinding{Path: "main.go", Anchor: "ab", AnchorLines: 1_000_000_000}
	if anchorUnchanged(d, dismissedTestDiffs("x")) {
		t.Error("an oversized span must be rejected without scanning")
	}
}
