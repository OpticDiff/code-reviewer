package reviewer

import (
	"sort"

	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

// startTolerance is how many lines apart, in the current file, a finding's
// start may be from the code an open thread is anchored to and still count as
// the same issue. Across runs on unchanged code the model keeps the start line
// of a finding far more reliably than its end line or category; a handful of
// real repeats were off by up to three lines.
const startTolerance = 3

// FilterStanding splits findings into those to post and those an open thread
// of the tool still covers. keep lists the fingerprints of the threads that
// cover a finding, one per thread, for cleanup to leave open; every other open
// thread is stale. Findings without a fingerprint are always posted.
//
// The fingerprint hashes the model's category and end line, and both change
// between runs over identical code, so equality alone misses most repeats.
// A thread therefore also covers a finding that sits in the same place: same
// file and diff side, start line within startTolerance of the thread's
// anchored code, whatever category and end line the model chose. "Same place"
// is only trusted while the thread's anchored lines and their neighbours are
// unchanged; once the code is edited the old thread no longer describes it and
// a new one is opened. This is deliberately looser than dismissal suppression
// (FilterDismissed), which stays exact because hiding a finding is costlier
// than showing it twice.
//
// Candidate pairs are paired nearest first and each thread covers at most one
// finding, so two findings close to one thread cannot both be swallowed by it.
// Ties fall back to the order the findings were given in, then to the thread's
// fingerprint, so the outcome does not depend on how the platform lists threads.
//
// A thread whose badge shows a lower severity than the finding it covers is
// released rather than kept: a quietly outdated badge would understate a
// blocking issue, so the finding is posted again and cleanup removes the old
// thread. A lower new severity keeps the thread, which errs on the side of the
// stricter earlier judgement; the summary and exit status use the new one.
func FilterStanding(findings []model.Finding, open []vcs.OpenFinding, diffs []diff.FileDiff) (fresh, standing []model.Finding, keep []string) {
	if len(open) == 0 {
		return findings, nil, nil
	}
	taken := make([]bool, len(open))
	covering := make([]int, len(findings))
	for i := range covering {
		covering[i] = -1
	}
	for _, c := range openCandidates(findings, open, diffs) {
		if covering[c.finding] < 0 && !taken[c.thread] {
			covering[c.finding], taken[c.thread] = c.thread, true
		}
	}

	for i, f := range findings {
		j := covering[i]
		if j < 0 || raisedSeverity(f, open[j]) {
			fresh = append(fresh, f)
			continue
		}
		standing = append(standing, f)
		keep = append(keep, open[j].Fingerprint)
	}
	return fresh, standing, keep
}

// raisedSeverity reports whether the finding is more severe than the badge on
// the thread that would cover it. A thread whose badge cannot be read is never
// treated as outdated.
func raisedSeverity(f model.Finding, o vcs.OpenFinding) bool {
	return o.Severity != "" && severityRank(f.Severity) > severityRank(o.Severity)
}

// openCandidate pairs a finding with a thread that could cover it.
type openCandidate struct {
	finding, thread int
	distance        int  // Lines between the finding's start and the thread's code.
	exact           bool // The fingerprints are equal.
}

// openCandidates lists every finding and thread that could belong together,
// best pairs first.
func openCandidates(findings []model.Finding, open []vcs.OpenFinding, diffs []diff.FileDiff) []openCandidate {
	sides := map[sideKey]map[int]string{}
	linesOf := func(file string, oldSide bool) map[int]string {
		k := sideKey{file, oldSide}
		if _, ok := sides[k]; !ok {
			if oldSide {
				sides[k] = oldSideLines(file, diffs)
			} else {
				sides[k] = newSideLines(file, diffs)
			}
		}
		return sides[k]
	}

	var out []openCandidate
	for i, f := range findings {
		if f.Fingerprint == "" {
			continue
		}
		removed := f.Line <= 0 && f.OldLine > 0
		for j, o := range open {
			switch {
			case o.Fingerprint == "":
			case o.Fingerprint == f.Fingerprint:
				out = append(out, openCandidate{finding: i, thread: j, exact: true})
			case f.File == o.Path && removed == o.OldSide:
				if d, ok := distanceToAnchor(f, o, linesOf(f.File, removed)); ok {
					out = append(out, openCandidate{finding: i, thread: j, distance: d})
				}
			}
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		x, y := out[a], out[b]
		switch {
		case x.distance != y.distance:
			return x.distance < y.distance
		case x.exact != y.exact:
			return x.exact
		case x.finding != y.finding:
			return x.finding < y.finding
		}
		return open[x.thread].Fingerprint < open[y.thread].Fingerprint
	})
	return out
}

type sideKey struct {
	file    string
	oldSide bool
}

// distanceToAnchor returns how far the finding's start line is from the code
// the thread is anchored to, when that code is still in the diff exactly as it
// was and the thread could have been opened for this finding.
func distanceToAnchor(f model.Finding, o vcs.OpenFinding, lines map[int]string) (int, bool) {
	if o.AnchorLines < 1 || o.AnchorLines > vcs.MaxAnchorLines {
		return 0, false
	}
	// A rule finding must come from the same rule; the thread's fingerprint is
	// its anchor hashed with the rule, so hashing that anchor with this
	// finding's rule says whether they agree. Rule and plain findings are
	// different kinds of finding even over the same lines.
	if ruled := f.RuleName != ""; ruled != o.RuleAttributed || (ruled && vcs.FingerprintFor(o.Anchor, fingerprintKey(f)) != o.Fingerprint) {
		return 0, false
	}
	start := f.Line
	if f.Line <= 0 && f.OldLine > 0 {
		start = f.OldLine
	}
	// Nearest first, so the first hit is the distance.
	for d := 0; d <= startTolerance; d++ {
		for _, s := range []int{start - d, start + d} {
			if s < 1 {
				continue
			}
			if h, ok := anchorHash(o.Path, lines, s, s+o.AnchorLines-1); ok && h == o.Anchor {
				return d, true
			}
		}
	}
	return 0, false
}
