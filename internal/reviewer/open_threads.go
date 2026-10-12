package reviewer

import (
	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

// FilterStanding splits findings into those to post and those a thread the
// tool opened earlier still covers. A thread covers a finding when it has the
// same fingerprint or, when the model picked a different line span over the
// same unchanged code, when the finding has the thread's category (or rule)
// and its span touches the thread's anchored lines. Each thread covers at most
// one finding. keep lists the fingerprints of the threads that cover a
// finding, one per thread, for cleanup to leave open; every other open thread
// is stale. Findings without a fingerprint are always posted.
func FilterStanding(findings []model.Finding, open []vcs.OpenFinding, diffs []diff.FileDiff) (fresh, standing []model.Finding, keep []string) {
	if len(open) == 0 {
		return findings, nil, nil
	}
	taken := make([]bool, len(open))
	covering := make([]int, len(findings))
	for i := range covering {
		covering[i] = -1
	}
	claim := func(match func(f model.Finding, o vcs.OpenFinding) bool) {
		for i, f := range findings {
			if covering[i] >= 0 || f.Fingerprint == "" {
				continue
			}
			for j, o := range open {
				if !taken[j] && o.Fingerprint != "" && match(f, o) {
					taken[j], covering[i] = true, j
					break
				}
			}
		}
	}
	claim(func(f model.Finding, o vcs.OpenFinding) bool { return f.Fingerprint == o.Fingerprint })
	claim(func(f model.Finding, o vcs.OpenFinding) bool { return coversSpan(o, f, diffs) })

	for i, f := range findings {
		if covering[i] < 0 {
			fresh = append(fresh, f)
			continue
		}
		standing = append(standing, f)
		keep = append(keep, open[covering[i]].Fingerprint)
	}
	return fresh, standing, keep
}

// coversSpan reports whether the thread's anchored lines still appear
// unchanged, with the same neighbours, inside the span of a finding of the
// thread's category. The thread's fingerprint is the anchor hashed with its
// category, so hashing the thread's anchor with the finding's category tells
// whether the categories agree without knowing the thread's category.
func coversSpan(o vcs.OpenFinding, f model.Finding, diffs []diff.FileDiff) bool {
	removed := f.Line <= 0 && f.OldLine > 0
	if f.File != o.Path || removed != o.OldSide || o.AnchorLines < 1 || o.AnchorLines > vcs.MaxAnchorLines || f.AnchorLines < 1 {
		return false
	}
	if vcs.FingerprintFor(o.Anchor, fingerprintKey(f)) != o.Fingerprint {
		return false
	}
	start, lines := f.Line, newSideLines(f.File, diffs)
	if removed {
		start, lines = f.OldLine, oldSideLines(f.File, diffs)
	}
	end := start + f.AnchorLines - 1
	for s := start - o.AnchorLines + 1; s <= end; s++ {
		if h, ok := anchorHash(o.Path, lines, s, s+o.AnchorLines-1); ok && h == o.Anchor {
			return true
		}
	}
	return false
}
