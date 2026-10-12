package reviewer

import (
	"context"
	"strings"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

// addedDiffs builds a diff in which every line of main.go is added, numbered from 1.
func addedDiffs(lines ...string) []diff.FileDiff {
	dl := make([]diff.DiffLine, len(lines))
	for i, l := range lines {
		dl[i] = diff.DiffLine{Type: diff.LineAdded, Content: l, NewLineNo: i + 1}
	}
	return []diff.FileDiff{{NewPath: "main.go", Hunks: []diff.Hunk{{Lines: dl}}}}
}

// openThreadFor returns the thread a finding would have opened.
func openThreadFor(f model.Finding) vcs.OpenFinding {
	return vcs.OpenFinding{
		FingerprintInfo: vcs.FingerprintInfo{Fingerprint: f.Fingerprint, Anchor: f.Anchor, AnchorLines: f.AnchorLines},
		Path:            f.File,
		Line:            f.Line,
	}
}

func TestFingerprint_RuleFindingsUseRuleNotCategory(t *testing.T) {
	d := dismissedTestDiffs("x := compute()")
	base := model.Finding{File: "main.go", Line: 2, Category: "custom", RuleSource: "repo"}
	a, b := base, base
	a.RuleName, b.RuleName = "no-compute", "Check-Errors"
	plain := base

	fa, fb, fp := fingerprinted(a, d), fingerprinted(b, d), fingerprinted(plain, d)
	if fa.Fingerprint == fb.Fingerprint {
		t.Error("two rules sharing a category must not share a fingerprint")
	}
	if fa.Fingerprint == fp.Fingerprint {
		t.Error("a rule finding must not share a fingerprint with an unattributed one of the same category")
	}
	a.RuleName = " NO-COMPUTE "
	if again := fingerprinted(a, d); again.Fingerprint != fa.Fingerprint {
		t.Error("rule name case and padding must not change the fingerprint")
	}
}

func TestFilterStanding(t *testing.T) {
	code := []string{"a()", "x := compute()", "b()", "c()"}
	posted := fingerprinted(model.Finding{File: "main.go", Line: 2, Category: "bug", Severity: "MEDIUM"}, addedDiffs(code...))
	thread := openThreadFor(posted)

	tests := []struct {
		name      string
		finding   model.Finding
		diffs     []diff.FileDiff
		open      []vcs.OpenFinding
		standing  int
		fresh     int
		wantKeep  []string
		wantFresh bool // the finding is still posted
	}{
		{
			name:     "unchanged finding keeps its thread",
			finding:  model.Finding{File: "main.go", Line: 2, Category: "bug", Severity: "MEDIUM", Title: "reworded"},
			diffs:    addedDiffs(code...),
			open:     []vcs.OpenFinding{thread},
			standing: 1,
			wantKeep: []string{thread.Fingerprint},
		},
		{
			name:     "same code on a shifted line still matches",
			finding:  model.Finding{File: "main.go", Line: 4, Category: "bug", Severity: "MEDIUM"},
			diffs:    addedDiffs(append([]string{"p()", "q()"}, code...)...),
			open:     []vcs.OpenFinding{thread},
			standing: 1,
			wantKeep: []string{thread.Fingerprint},
		},
		{
			name:    "no open thread posts the finding",
			finding: model.Finding{File: "main.go", Line: 2, Category: "bug", Severity: "MEDIUM"},
			diffs:   addedDiffs(code...),
			fresh:   1,
		},
		{
			name:    "edited code yields a new thread",
			finding: model.Finding{File: "main.go", Line: 2, Category: "bug", Severity: "MEDIUM"},
			diffs:   addedDiffs("a()", "x, err := compute()", "b()", "c()"),
			open:    []vcs.OpenFinding{thread},
			fresh:   1,
		},
		{
			name:    "edited neighbour yields a new thread",
			finding: model.Finding{File: "main.go", Line: 2, Category: "bug", Severity: "MEDIUM"},
			diffs:   addedDiffs("a()", "x := compute()", "b2()", "c()"),
			open:    []vcs.OpenFinding{thread},
			fresh:   1,
		},
		{
			name:    "moved code with other neighbours yields a new thread",
			finding: model.Finding{File: "main.go", Line: 4, Category: "bug", Severity: "MEDIUM"},
			diffs:   addedDiffs("a()", "b()", "c()", "x := compute()", "d()"),
			open:    []vcs.OpenFinding{thread},
			fresh:   1,
		},
		{
			name:    "other category on the same lines is a different finding",
			finding: model.Finding{File: "main.go", Line: 2, Category: "security", Severity: "HIGH"},
			diffs:   addedDiffs(code...),
			open:    []vcs.OpenFinding{thread},
			fresh:   1,
		},
		{
			name:     "span drift over unchanged code matches the same thread",
			finding:  model.Finding{File: "main.go", Line: 2, EndLine: 3, Category: "bug", Severity: "MEDIUM"},
			diffs:    addedDiffs(code...),
			open:     []vcs.OpenFinding{thread},
			standing: 1,
			wantKeep: []string{thread.Fingerprint},
		},
		{
			name:     "span drift to a wider range containing the anchor matches",
			finding:  model.Finding{File: "main.go", Line: 1, EndLine: 4, Category: "bug", Severity: "MEDIUM"},
			diffs:    addedDiffs(code...),
			open:     []vcs.OpenFinding{thread},
			standing: 1,
			wantKeep: []string{thread.Fingerprint},
		},
		{
			name:    "span drift to another category does not match",
			finding: model.Finding{File: "main.go", Line: 2, EndLine: 3, Category: "security", Severity: "HIGH"},
			diffs:   addedDiffs(code...),
			open:    []vcs.OpenFinding{thread},
			fresh:   1,
		},
		{
			name:    "span that does not touch the anchored line does not match",
			finding: model.Finding{File: "main.go", Line: 3, EndLine: 4, Category: "bug", Severity: "MEDIUM"},
			diffs:   addedDiffs(code...),
			open:    []vcs.OpenFinding{thread},
			fresh:   1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := fingerprinted(tt.finding, tt.diffs)
			fresh, standing, keep := FilterStanding([]model.Finding{f}, tt.open, tt.diffs)
			if len(standing) != tt.standing || len(fresh) != tt.fresh {
				t.Errorf("fresh=%d standing=%d, want fresh=%d standing=%d", len(fresh), len(standing), tt.fresh, tt.standing)
			}
			if strings.Join(keep, ",") != strings.Join(tt.wantKeep, ",") {
				t.Errorf("keep = %v, want %v", keep, tt.wantKeep)
			}
		})
	}
}

func TestFilterStanding_OneThreadServesOneFinding(t *testing.T) {
	d := addedDiffs("a()", "x := compute()", "b()")
	f := fingerprinted(model.Finding{File: "main.go", Line: 2, Category: "bug"}, d)
	fresh, standing, keep := FilterStanding([]model.Finding{f, f}, []vcs.OpenFinding{openThreadFor(f)}, d)
	if len(standing) != 1 || len(fresh) != 1 || len(keep) != 1 {
		t.Errorf("fresh=%d standing=%d keep=%v, want one of each", len(fresh), len(standing), keep)
	}
}

func TestFilterStanding_UnfingerprintedFindingsArePosted(t *testing.T) {
	f := model.Finding{File: "main.go", Line: 99, Category: "bug"}
	fresh, standing, _ := FilterStanding([]model.Finding{f}, []vcs.OpenFinding{{FingerprintInfo: vcs.FingerprintInfo{}}}, addedDiffs("a()"))
	if len(fresh) != 1 || len(standing) != 0 {
		t.Errorf("fresh=%d standing=%d, want the unanchored finding posted", len(fresh), len(standing))
	}
}

// openRun runs one review in CI mode against a platform that already holds the given open threads.
func openRun(t *testing.T, findings []model.Finding, current []diff.FileDiff, open []vcs.OpenFinding) (int, *threadVCS) {
	t.Helper()
	mm := &mockModel{result: &model.ReviewResult{Summary: "s", Findings: findings}}
	client := &threadVCS{
		mockVCS: &mockVCS{mrVersions: []vcs.DiffVersion{{ID: 1, HeadSHA: "a", BaseSHA: "b", StartSHA: "c"}}},
		open:    open,
	}
	r := NewWithDiffSource(dismissedRunCfg(), mm, client, &mockDiffSource{diffs: current})
	count, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return count, client
}

// threadsOpenedBy returns the threads the posted inline comments would create.
func threadsOpenedBy(t *testing.T, client *threadVCS) []vcs.OpenFinding {
	t.Helper()
	var out []vcs.OpenFinding
	for _, c := range client.submitReviewReq.Comments {
		fi, ok := vcs.ParseFingerprint(c.Body)
		if !ok {
			t.Fatalf("posted comment carries no fingerprint:\n%s", c.Body)
		}
		out = append(out, vcs.OpenFinding{FingerprintInfo: fi, Path: c.Path, Line: c.Line})
	}
	return out
}

func TestRun_UnchangedFindingIsPostedOnceAcrossRuns(t *testing.T) {
	d := addedDiffs("a()", "x := compute()", "b()")
	finding := model.Finding{File: "main.go", Line: 2, Severity: "MEDIUM", Category: "bug", Title: "Unchecked value", Body: "details"}

	first, c1 := openRun(t, []model.Finding{finding}, d, nil)
	if first != 1 || len(c1.submitReviewReq.Comments) != 1 {
		t.Fatalf("first run: count=%d comments=%d, want 1 and 1", first, len(c1.submitReviewReq.Comments))
	}
	open := threadsOpenedBy(t, c1)

	for run := 2; run <= 3; run++ {
		count, c := openRun(t, []model.Finding{finding}, d, open)
		req := c.submitReviewReq
		if len(req.Comments) != 0 {
			t.Errorf("run %d re-posted %d comment(s), want none", run, len(req.Comments))
		}
		if len(req.OpenFingerprints) != 1 || req.OpenFingerprints[0] != open[0].Fingerprint {
			t.Errorf("run %d OpenFingerprints = %v, want the open thread kept", run, req.OpenFingerprints)
		}
		if count != 1 {
			t.Errorf("run %d count = %d, want 1: the finding is still open", run, count)
		}
		if !strings.Contains(req.Summary, "Unchecked value") || !strings.Contains(req.Summary, "| 🟡 MEDIUM | 1 |") {
			t.Errorf("run %d summary must still list the open finding:\n%s", run, req.Summary)
		}
	}
}

func TestRun_FixedFindingReleasesItsThread(t *testing.T) {
	d := addedDiffs("a()", "x := compute()", "b()")
	finding := model.Finding{File: "main.go", Line: 2, Severity: "MEDIUM", Category: "bug", Title: "Unchecked value", Body: "details"}
	_, c1 := openRun(t, []model.Finding{finding}, d, nil)

	count, c := openRun(t, nil, addedDiffs("a()", "x, err := compute()", "b()"), threadsOpenedBy(t, c1))
	if count != 0 || len(c.submitReviewReq.Comments) != 0 {
		t.Errorf("count=%d comments=%d, want a clean run", count, len(c.submitReviewReq.Comments))
	}
	if len(c.submitReviewReq.OpenFingerprints) != 0 {
		t.Errorf("OpenFingerprints = %v, want the fixed finding's thread released for cleanup", c.submitReviewReq.OpenFingerprints)
	}
}

func TestRun_EditedCodeOpensANewThread(t *testing.T) {
	finding := model.Finding{File: "main.go", Line: 2, Severity: "MEDIUM", Category: "bug", Title: "Unchecked value", Body: "details"}
	_, c1 := openRun(t, []model.Finding{finding}, addedDiffs("a()", "x := compute()", "b()"), nil)

	edited := addedDiffs("a()", "x := compute(ctx)", "b()")
	count, c := openRun(t, []model.Finding{finding}, edited, threadsOpenedBy(t, c1))
	if count != 1 || len(c.submitReviewReq.Comments) != 1 {
		t.Errorf("count=%d comments=%d, want the finding on edited code posted as a new thread", count, len(c.submitReviewReq.Comments))
	}
	if len(c.submitReviewReq.OpenFingerprints) != 0 {
		t.Errorf("OpenFingerprints = %v, want the stale thread released", c.submitReviewReq.OpenFingerprints)
	}
}

func TestRun_OpenHighFindingStillFailsGate(t *testing.T) {
	d := addedDiffs("a()", "x := compute()", "b()")
	finding := model.Finding{File: "main.go", Line: 2, Severity: "HIGH", Category: "bug", Title: "Unchecked value", Body: "details"}
	_, c1 := openRun(t, []model.Finding{finding}, d, nil)

	count, c := openRun(t, []model.Finding{finding}, d, threadsOpenedBy(t, c1))
	if count != 1 {
		t.Errorf("count = %d, want 1: an open HIGH finding must keep failing the gate", count)
	}
	if len(c.submitReviewReq.Comments) != 0 {
		t.Error("the open HIGH finding must not be posted a second time")
	}
}

func TestRun_NewFindingNextToOpenOneIsPosted(t *testing.T) {
	d := addedDiffs("a()", "x := compute()", "b()")
	old := model.Finding{File: "main.go", Line: 2, Severity: "MEDIUM", Category: "bug", Title: "Unchecked value", Body: "details"}
	_, c1 := openRun(t, []model.Finding{old}, d, nil)

	added := model.Finding{File: "main.go", Line: 2, Severity: "HIGH", Category: "security", Title: "Injection", Body: "details"}
	count, c := openRun(t, []model.Finding{old, added}, d, threadsOpenedBy(t, c1))
	if count != 2 || len(c.submitReviewReq.Comments) != 1 || !strings.Contains(c.submitReviewReq.Comments[0].Body, "Injection") {
		t.Errorf("count=%d comments=%+v, want only the new finding posted", count, c.submitReviewReq.Comments)
	}
}

func TestRun_DismissedFindingStaysSuppressedAlongsideOpenThreads(t *testing.T) {
	d := addedDiffs("a()", "x := compute()", "b()")
	finding := model.Finding{File: "main.go", Line: 2, Severity: "LOW", Category: "bug", Title: "Unchecked value", Body: "details"}
	posted := fingerprinted(finding, d)

	mm := &mockModel{result: &model.ReviewResult{Summary: "s", Findings: []model.Finding{finding}}}
	client := &threadVCS{
		mockVCS:   &mockVCS{mrVersions: []vcs.DiffVersion{{ID: 1, HeadSHA: "a", BaseSHA: "b", StartSHA: "c"}}},
		dismissed: []vcs.DismissedFinding{{Fingerprint: posted.Fingerprint, Anchor: posted.Anchor, AnchorLines: posted.AnchorLines, Path: "main.go", Line: 2}},
		open:      []vcs.OpenFinding{{FingerprintInfo: vcs.FingerprintInfo{Fingerprint: "ffff", Anchor: "ffff", AnchorLines: 1}, Path: "other.go", Line: 1}},
	}
	count, err := NewWithDiffSource(dismissedRunCfg(), mm, client, &mockDiffSource{diffs: d}).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	req := client.submitReviewReq
	if count != 0 || len(req.Comments) != 0 {
		t.Errorf("count=%d comments=%d, want the dismissed LOW finding suppressed", count, len(req.Comments))
	}
	if len(req.KeepFingerprints) != 1 || len(req.OpenFingerprints) != 0 {
		t.Errorf("KeepFingerprints=%v OpenFingerprints=%v, want the dismissed thread kept and the unrelated open one released", req.KeepFingerprints, req.OpenFingerprints)
	}
}
