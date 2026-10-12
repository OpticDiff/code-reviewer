package reviewer

import (
	"context"
	"fmt"
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

// openThreadFor returns the thread a finding would have opened, with the
// severity and rule attribution its comment header would carry.
func openThreadFor(f model.Finding) vcs.OpenFinding {
	return vcs.OpenFinding{
		FingerprintInfo: vcs.FingerprintInfo{Fingerprint: f.Fingerprint, Anchor: f.Anchor, AnchorLines: f.AnchorLines},
		Path:            f.File,
		Line:            f.Line,
		Severity:        f.Severity,
		RuleAttributed:  f.RuleName != "",
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

// numberedCode returns n distinct lines so every line has distinct neighbours.
func numberedCode(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("step%d()", i+1)
	}
	return out
}

func TestFilterStanding(t *testing.T) {
	code := numberedCode(12)
	d := addedDiffs(code...)
	posted := fingerprinted(model.Finding{File: "main.go", Line: 5, Category: "bug", Severity: "MEDIUM"}, d)
	thread := openThreadFor(posted)
	wide := fingerprinted(model.Finding{File: "main.go", Line: 5, EndLine: 7, Category: "bug", Severity: "MEDIUM"}, d)
	wideThread := openThreadFor(wide)

	otherCode := make([]string, len(code))
	for i := range otherCode {
		otherCode[i] = fmt.Sprintf("other%d()", i+1)
	}
	edited := func(line int, text string) []diff.FileDiff {
		c := append([]string(nil), code...)
		c[line-1] = text
		return addedDiffs(c...)
	}

	tests := []struct {
		name     string
		finding  model.Finding
		diffs    []diff.FileDiff
		open     []vcs.OpenFinding
		standing bool // the thread covers the finding
	}{
		{"identical finding", model.Finding{File: "main.go", Line: 5, Category: "bug", Severity: "MEDIUM", Title: "reworded"}, d, []vcs.OpenFinding{thread}, true},
		{"no open thread", model.Finding{File: "main.go", Line: 5, Category: "bug", Severity: "MEDIUM"}, d, nil, false},

		// The model changes category between runs over the same code.
		{"category drift", model.Finding{File: "main.go", Line: 5, Category: "architecture", Severity: "MEDIUM"}, d, []vcs.OpenFinding{thread}, true},
		{"category drift with padding and case", model.Finding{File: "main.go", Line: 5, Category: " Scope ", Severity: "MEDIUM"}, d, []vcs.OpenFinding{thread}, true},

		// The model changes its end line between runs.
		{"longer span than the thread", model.Finding{File: "main.go", Line: 5, EndLine: 9, Category: "bug", Severity: "MEDIUM"}, d, []vcs.OpenFinding{thread}, true},
		{"shorter span than the thread", model.Finding{File: "main.go", Line: 5, Category: "bug", Severity: "MEDIUM"}, d, []vcs.OpenFinding{wideThread}, true},
		{"span drift and category drift", model.Finding{File: "main.go", Line: 5, EndLine: 6, Category: "scope", Severity: "MEDIUM"}, d, []vcs.OpenFinding{wideThread}, true},

		// The start line tolerance is +-3 lines.
		{"start 3 lines before", model.Finding{File: "main.go", Line: 2, Category: "bug", Severity: "MEDIUM"}, d, []vcs.OpenFinding{thread}, true},
		{"start 3 lines after", model.Finding{File: "main.go", Line: 8, Category: "bug", Severity: "MEDIUM"}, d, []vcs.OpenFinding{thread}, true},
		{"start 4 lines before", model.Finding{File: "main.go", Line: 1, Category: "bug", Severity: "MEDIUM"}, d, []vcs.OpenFinding{thread}, false},
		{"start 4 lines after", model.Finding{File: "main.go", Line: 9, Category: "bug", Severity: "MEDIUM"}, d, []vcs.OpenFinding{thread}, false},
		{"span reaching the anchor from far away", model.Finding{File: "main.go", Line: 1, EndLine: 5, Category: "bug", Severity: "MEDIUM"}, d, []vcs.OpenFinding{thread}, false},

		// The thread's code moved: the tolerance is measured from where it is now.
		{"code shifted down", model.Finding{File: "main.go", Line: 7, Category: "bug", Severity: "MEDIUM"}, addedDiffs(append([]string{"p()", "q()"}, code...)...), []vcs.OpenFinding{thread}, true},
		{"code shifted down, finding 3 lines past it", model.Finding{File: "main.go", Line: 10, Category: "bug", Severity: "MEDIUM"}, addedDiffs(append([]string{"p()", "q()"}, code...)...), []vcs.OpenFinding{thread}, true},
		{"code shifted down, finding 4 lines past it", model.Finding{File: "main.go", Line: 11, Category: "bug", Severity: "MEDIUM"}, addedDiffs(append([]string{"p()", "q()"}, code...)...), []vcs.OpenFinding{thread}, false},

		// Edited code is a new finding, however close the lines are.
		{"anchored line edited", model.Finding{File: "main.go", Line: 5, Category: "bug", Severity: "MEDIUM"}, edited(5, "step5(ctx)"), []vcs.OpenFinding{thread}, false},
		{"anchored line edited, category drift", model.Finding{File: "main.go", Line: 5, Category: "scope", Severity: "MEDIUM"}, edited(5, "step5(ctx)"), []vcs.OpenFinding{thread}, false},
		{"line above the anchor edited", model.Finding{File: "main.go", Line: 5, Category: "bug", Severity: "MEDIUM"}, edited(4, "step4(ctx)"), []vcs.OpenFinding{thread}, false},
		{"line below the anchor edited", model.Finding{File: "main.go", Line: 5, Category: "bug", Severity: "MEDIUM"}, edited(6, "step6(ctx)"), []vcs.OpenFinding{thread}, false},
		{"last line of a wide anchor edited", model.Finding{File: "main.go", Line: 5, Category: "bug", Severity: "MEDIUM"}, edited(7, "step7(ctx)"), []vcs.OpenFinding{wideThread}, false},
		{"anchored code replaced by other code", model.Finding{File: "main.go", Line: 5, Category: "bug", Severity: "MEDIUM"}, addedDiffs(otherCode...), []vcs.OpenFinding{thread}, false},

		// Same place is not enough: file and side must agree.
		{"other file", model.Finding{File: "main.go", Line: 5, Category: "bug", Severity: "MEDIUM"}, d, []vcs.OpenFinding{withPath(thread, "other.go")}, false},
		{"thread on the old side", model.Finding{File: "main.go", Line: 5, Category: "bug", Severity: "MEDIUM"}, d, []vcs.OpenFinding{withOldSide(thread)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := fingerprinted(tt.finding, tt.diffs)
			fresh, standing, keep := FilterStanding([]model.Finding{f}, tt.open, tt.diffs)
			if got := len(standing) == 1; got != tt.standing || len(fresh)+len(standing) != 1 {
				t.Fatalf("fresh=%d standing=%d, want standing=%v", len(fresh), len(standing), tt.standing)
			}
			if tt.standing && (len(keep) != 1 || keep[0] != tt.open[0].Fingerprint) {
				t.Errorf("keep = %v, want the covering thread %q", keep, tt.open[0].Fingerprint)
			}
			if !tt.standing && len(keep) != 0 {
				t.Errorf("keep = %v, want nothing kept", keep)
			}
		})
	}
}

// A thread on another file or side has another fingerprint, so only the
// location rules can cover the finding.
func withPath(o vcs.OpenFinding, path string) vcs.OpenFinding {
	o.Path, o.Fingerprint = path, "ffffffffffffffff"
	return o
}

func withOldSide(o vcs.OpenFinding) vcs.OpenFinding {
	o.OldSide, o.Fingerprint = true, "ffffffffffffffff"
	return o
}

func TestFilterStanding_RemovedLineFindings(t *testing.T) {
	removed := func(lines ...string) []diff.FileDiff {
		dl := make([]diff.DiffLine, len(lines))
		for i, l := range lines {
			dl[i] = diff.DiffLine{Type: diff.LineRemoved, Content: l, OldLineNo: i + 1}
		}
		return []diff.FileDiff{{OldPath: "main.go", NewPath: "main.go", Hunks: []diff.Hunk{{Lines: dl}}}}
	}
	d := removed(numberedCode(10)...)
	posted := fingerprinted(model.Finding{File: "main.go", OldLine: 5, Category: "bug", Severity: "MEDIUM"}, d)
	thread := openThreadFor(posted)
	thread.OldSide, thread.Line = true, 5

	drifted := fingerprinted(model.Finding{File: "main.go", OldLine: 7, EndLine: 9, Category: "scope", Severity: "MEDIUM"}, d)
	if fresh, standing, _ := FilterStanding([]model.Finding{drifted}, []vcs.OpenFinding{thread}, d); len(standing) != 1 || len(fresh) != 0 {
		t.Errorf("old side: fresh=%d standing=%d, want the thread to cover the drifted finding", len(fresh), len(standing))
	}
	far := fingerprinted(model.Finding{File: "main.go", OldLine: 9, Category: "bug", Severity: "MEDIUM"}, d)
	if fresh, standing, _ := FilterStanding([]model.Finding{far}, []vcs.OpenFinding{thread}, d); len(standing) != 0 || len(fresh) != 1 {
		t.Errorf("old side: fresh=%d standing=%d, want a finding 4 lines away posted", len(fresh), len(standing))
	}
	newSide := thread
	newSide.OldSide = false
	if fresh, standing, _ := FilterStanding([]model.Finding{drifted}, []vcs.OpenFinding{newSide}, d); len(standing) != 0 || len(fresh) != 1 {
		t.Errorf("a new-side thread must not cover an old-side finding: fresh=%d standing=%d", len(fresh), len(standing))
	}
}

func TestFilterStanding_Rules(t *testing.T) {
	d := addedDiffs(numberedCode(12)...)
	ruleFinding := func(name, category string) model.Finding {
		return fingerprinted(model.Finding{File: "main.go", Line: 5, Category: category, Severity: "MEDIUM", RuleName: name, RuleSource: "repo"}, d)
	}
	plain := fingerprinted(model.Finding{File: "main.go", Line: 5, Category: "bug", Severity: "MEDIUM"}, d)
	noCompute := ruleFinding("no-compute", "custom")

	moved := func(f model.Finding) model.Finding { // same finding one line later, so only the fuzzy match can cover it
		f.Line, f.Fingerprint, f.Anchor, f.AnchorLines = 6, "", "", 0
		return fingerprinted(f, d)
	}
	tests := []struct {
		name     string
		thread   vcs.OpenFinding
		finding  model.Finding
		standing bool
	}{
		{"same rule", openThreadFor(noCompute), moved(ruleFinding("no-compute", "custom")), true},
		{"same rule, case and padding", openThreadFor(noCompute), moved(ruleFinding(" No-Compute ", "custom")), true},
		{"same rule, other category", openThreadFor(noCompute), moved(ruleFinding("no-compute", "security")), true},
		{"other rule with the shared category", openThreadFor(noCompute), moved(ruleFinding("check-errors", "custom")), false},
		{"plain finding against a rule thread", openThreadFor(noCompute), moved(plain), false},
		{"rule finding against a plain thread", openThreadFor(plain), moved(noCompute), false},
		{"rule finding, identical fingerprint", openThreadFor(noCompute), noCompute, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fresh, standing, _ := FilterStanding([]model.Finding{tt.finding}, []vcs.OpenFinding{tt.thread}, d)
			if (len(standing) == 1) != tt.standing || len(fresh)+len(standing) != 1 {
				t.Errorf("fresh=%d standing=%d, want standing=%v", len(fresh), len(standing), tt.standing)
			}
		})
	}
}

func TestFilterStanding_OneThreadServesOneFinding(t *testing.T) {
	d := addedDiffs(numberedCode(12)...)
	at := func(line int) model.Finding {
		return fingerprinted(model.Finding{File: "main.go", Line: line, Category: "bug", Severity: "MEDIUM", Title: fmt.Sprintf("at %d", line)}, d)
	}
	thread := openThreadFor(at(5))

	t.Run("identical findings", func(t *testing.T) {
		f := at(5)
		fresh, standing, keep := FilterStanding([]model.Finding{f, f}, []vcs.OpenFinding{thread}, d)
		if len(standing) != 1 || len(fresh) != 1 || len(keep) != 1 {
			t.Errorf("fresh=%d standing=%d keep=%v, want one of each", len(fresh), len(standing), keep)
		}
	})
	t.Run("the nearer finding wins whatever the order", func(t *testing.T) {
		for _, order := range [][]int{{3, 6}, {6, 3}} {
			var in []model.Finding
			for _, l := range order {
				in = append(in, at(l))
			}
			fresh, standing, _ := FilterStanding(in, []vcs.OpenFinding{thread}, d)
			if len(standing) != 1 || standing[0].Line != 6 || len(fresh) != 1 || fresh[0].Line != 3 {
				t.Errorf("order %v: standing=%v fresh=%v, want line 6 to keep the thread", order, lines(standing), lines(fresh))
			}
		}
	})
	t.Run("an exact match beats a drifted one listed first", func(t *testing.T) {
		fresh, standing, _ := FilterStanding([]model.Finding{at(4), at(5)}, []vcs.OpenFinding{thread}, d)
		if len(standing) != 1 || standing[0].Line != 5 || len(fresh) != 1 || fresh[0].Line != 4 {
			t.Errorf("standing=%v fresh=%v, want the identical finding to keep the thread", lines(standing), lines(fresh))
		}
	})
	t.Run("equidistant findings: the first one wins, deterministically", func(t *testing.T) {
		for i := 0; i < 20; i++ {
			fresh, standing, _ := FilterStanding([]model.Finding{at(4), at(6)}, []vcs.OpenFinding{thread}, d)
			if len(standing) != 1 || standing[0].Line != 4 || len(fresh) != 1 || fresh[0].Line != 6 {
				t.Fatalf("standing=%v fresh=%v, want the first finding to keep the thread", lines(standing), lines(fresh))
			}
		}
	})
	t.Run("two threads, one finding: the nearer thread is kept", func(t *testing.T) {
		near, far := openThreadFor(at(8)), openThreadFor(at(4))
		for _, open := range [][]vcs.OpenFinding{{near, far}, {far, near}} {
			fresh, standing, keep := FilterStanding([]model.Finding{at(7)}, open, d)
			if len(standing) != 1 || len(fresh) != 0 || len(keep) != 1 || keep[0] != near.Fingerprint {
				t.Errorf("standing=%d fresh=%d keep=%v, want only the nearer thread %q kept", len(standing), len(fresh), keep, near.Fingerprint)
			}
		}
	})
	t.Run("two threads, two findings pair up by distance", func(t *testing.T) {
		a, b := openThreadFor(at(3)), openThreadFor(at(8))
		fresh, standing, keep := FilterStanding([]model.Finding{at(9), at(4)}, []vcs.OpenFinding{a, b}, d)
		if len(standing) != 2 || len(fresh) != 0 || strings.Join(keep, ",") != b.Fingerprint+","+a.Fingerprint {
			t.Errorf("standing=%d fresh=%d keep=%v, want both covered, each by its nearer thread", len(standing), len(fresh), keep)
		}
	})
}

func lines(fs []model.Finding) []int {
	out := make([]int, len(fs))
	for i, f := range fs {
		out[i] = f.Line
	}
	return out
}

func TestFilterStanding_Severity(t *testing.T) {
	d := addedDiffs(numberedCode(12)...)
	at := func(line int, category, severity string) model.Finding {
		return fingerprinted(model.Finding{File: "main.go", Line: line, Category: category, Severity: severity}, d)
	}
	tests := []struct {
		name      string
		thread    string
		finding   model.Finding
		wantFresh bool
	}{
		{"same severity", "MEDIUM", at(5, "bug", "MEDIUM"), false},
		{"same severity, other case", "MEDIUM", at(5, "bug", "medium"), false},
		{"severity raised on an identical fingerprint", "MEDIUM", at(5, "bug", "HIGH"), true},
		{"severity raised with category drift", "MEDIUM", at(5, "scope", "HIGH"), true},
		{"severity raised, other case", "medium", at(5, "scope", "CRITICAL"), true},
		{"severity lowered keeps the thread", "HIGH", at(5, "scope", "LOW"), false},
		{"severity lowered on an identical fingerprint", "HIGH", at(5, "bug", "MEDIUM"), false},
		{"thread severity unknown keeps the thread", "", at(5, "scope", "HIGH"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thread := openThreadFor(at(5, "bug", "MEDIUM"))
			thread.Severity = tt.thread
			fresh, standing, keep := FilterStanding([]model.Finding{tt.finding}, []vcs.OpenFinding{thread}, d)
			if tt.wantFresh {
				if len(fresh) != 1 || len(standing) != 0 || len(keep) != 0 {
					t.Errorf("fresh=%d standing=%d keep=%v, want the finding re-posted and its stale thread released", len(fresh), len(standing), keep)
				}
				return
			}
			if len(standing) != 1 || len(fresh) != 0 || len(keep) != 1 {
				t.Errorf("fresh=%d standing=%d keep=%v, want the thread kept", len(fresh), len(standing), keep)
			}
		})
	}
}

func TestFilterStanding_UnfingerprintedFindingsArePosted(t *testing.T) {
	f := model.Finding{File: "main.go", Line: 99, Category: "bug"}
	fresh, standing, _ := FilterStanding([]model.Finding{f}, []vcs.OpenFinding{{FingerprintInfo: vcs.FingerprintInfo{}}}, addedDiffs("a()"))
	if len(fresh) != 1 || len(standing) != 0 {
		t.Errorf("fresh=%d standing=%d, want the unanchored finding posted", len(fresh), len(standing))
	}
}

// The header the matcher reads back must stay in step with the one the
// comment formatter writes.
func TestFindingHeaderRoundTrip(t *testing.T) {
	for _, f := range []model.Finding{
		{Severity: "HIGH", Title: "plain"},
		{Severity: "low", Title: "repo rule", RuleName: "no-compute", RuleSource: "repo"},
		{Severity: "CRITICAL", Title: "platform rule", RuleName: "no-secrets", RuleSource: "platform", RuleFile: "policy.yaml"},
		{Severity: "MEDIUM", Title: "named rule without a source", RuleName: "invented"},
	} {
		h, ok := vcs.ParseFindingHeader(formatInlineComment(f))
		if !ok || h.Severity != f.Severity || h.RuleAttributed != (f.RuleName != "") {
			t.Errorf("%q: header = %+v ok=%v", f.Title, h, ok)
		}
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
		h, _ := vcs.ParseFindingHeader(c.Body)
		out = append(out, vcs.OpenFinding{FingerprintInfo: fi, Path: c.Path, Line: c.Line, Severity: h.Severity, RuleAttributed: h.RuleAttributed})
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

func TestRun_DriftedFindingKeepsItsThread(t *testing.T) {
	d := addedDiffs(numberedCode(12)...)
	first := model.Finding{File: "main.go", Line: 5, Severity: "MEDIUM", Category: "scope", Title: "Config line", Body: "details"}
	_, c1 := openRun(t, []model.Finding{first}, d, nil)
	open := threadsOpenedBy(t, c1)

	// The model files the same issue under another category, a longer span and a nearby start line.
	again := model.Finding{File: "main.go", Line: 4, EndLine: 9, Severity: "MEDIUM", Category: "bug", Title: "Config value", Body: "other words"}
	count, c := openRun(t, []model.Finding{again}, d, open)
	req := c.submitReviewReq
	if len(req.Comments) != 0 {
		t.Errorf("re-posted %d comment(s) for a drifted repeat, want none", len(req.Comments))
	}
	if len(req.OpenFingerprints) != 1 || req.OpenFingerprints[0] != open[0].Fingerprint {
		t.Errorf("OpenFingerprints = %v, want the first thread kept", req.OpenFingerprints)
	}
	if count != 1 || !strings.Contains(req.Summary, "Config value") {
		t.Errorf("count=%d, summary must list the repeat once:\n%s", count, req.Summary)
	}
}

func TestRun_RaisedSeverityIsPostedAgain(t *testing.T) {
	d := addedDiffs(numberedCode(12)...)
	first := model.Finding{File: "main.go", Line: 5, Severity: "MEDIUM", Category: "bug", Title: "Config line", Body: "details"}
	_, c1 := openRun(t, []model.Finding{first}, d, nil)
	open := threadsOpenedBy(t, c1)

	for _, repeat := range []model.Finding{
		{File: "main.go", Line: 5, Severity: "HIGH", Category: "bug", Title: "Config line", Body: "details"},
		{File: "main.go", Line: 6, Severity: "HIGH", Category: "security", Title: "Config line", Body: "details"},
	} {
		count, c := openRun(t, []model.Finding{repeat}, d, open)
		req := c.submitReviewReq
		if len(req.Comments) != 1 || !strings.Contains(req.Comments[0].Body, "**[HIGH]**") {
			t.Fatalf("comments = %+v, want the finding posted again at its new severity", req.Comments)
		}
		if len(req.OpenFingerprints) != 0 {
			t.Errorf("OpenFingerprints = %v, want the thread with the old badge released for cleanup", req.OpenFingerprints)
		}
		if count != 1 || !strings.Contains(req.Summary, "| 🟠 HIGH | 1 |") {
			t.Errorf("count=%d, summary must count the finding once at the new severity:\n%s", count, req.Summary)
		}
	}
}

func TestRun_LoweredSeverityKeepsTheThreadButCountsTheNewSeverity(t *testing.T) {
	d := addedDiffs(numberedCode(12)...)
	first := model.Finding{File: "main.go", Line: 5, Severity: "HIGH", Category: "bug", Title: "Config line", Body: "details"}
	_, c1 := openRun(t, []model.Finding{first}, d, nil)

	repeat := first
	repeat.Severity = "LOW"
	count, c := openRun(t, []model.Finding{repeat}, d, threadsOpenedBy(t, c1))
	req := c.submitReviewReq
	if len(req.Comments) != 0 || len(req.OpenFingerprints) != 1 {
		t.Errorf("comments=%d OpenFingerprints=%v, want the thread kept", len(req.Comments), req.OpenFingerprints)
	}
	if count != 1 || !strings.Contains(req.Summary, "| 🔵 LOW | 1 |") || strings.Contains(req.Summary, "HIGH") {
		t.Errorf("count=%d, summary must count the new severity:\n%s", count, req.Summary)
	}
}
