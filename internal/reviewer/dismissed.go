package reviewer

import (
	"fmt"
	"strings"

	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

// newSideLines returns the trimmed new-side lines of file's diff, keyed by
// line number.
func newSideLines(file string, diffs []diff.FileDiff) map[int]string {
	lines := make(map[int]string)
	for _, d := range diffs {
		if d.NewPath != file {
			continue
		}
		for _, h := range d.Hunks {
			for _, l := range h.Lines {
				if l.Type != diff.LineRemoved && l.NewLineNo > 0 {
					lines[l.NewLineNo] = strings.TrimSpace(l.Content)
				}
			}
		}
	}
	return lines
}

// oldSideLines returns the trimmed old-side lines of file's diff, keyed by
// line number. It covers deleted lines and context lines.
func oldSideLines(file string, diffs []diff.FileDiff) map[int]string {
	lines := make(map[int]string)
	for _, d := range diffs {
		path := d.NewPath
		if path == "" {
			path = d.OldPath
		}
		if path != file && d.OldPath != file {
			continue
		}
		for _, h := range d.Hunks {
			for _, l := range h.Lines {
				if l.Type != diff.LineAdded && l.OldLineNo > 0 {
					lines[l.OldLineNo] = strings.TrimSpace(l.Content)
				}
			}
		}
	}
	return lines
}

var anchorHash = vcs.AnchorHash

// AssignFingerprints sets Fingerprint, Anchor and AnchorLines on every
// finding that is anchored inside the diff. The fingerprint combines the
// anchor hash with the lowercased category, and deliberately ignores the
// title, body and line number, which vary between model runs. Any edit to the
// anchored lines or their neighbours yields a new fingerprint. Findings that
// cannot be anchored keep an empty fingerprint and are never suppressed.
func AssignFingerprints(findings []model.Finding, diffs []diff.FileDiff) {
	byFileNew := make(map[string]map[int]string)
	byFileOld := make(map[string]map[int]string)
	for i := range findings {
		f := &findings[i]
		var lines map[int]string
		var startLine, endLine int
		if f.Line <= 0 && f.OldLine > 0 {
			lines = byFileOld[f.File]
			if lines == nil {
				lines = oldSideLines(f.File, diffs)
				byFileOld[f.File] = lines
			}
			startLine, endLine = f.OldLine, f.OldLine
		} else {
			lines = byFileNew[f.File]
			if lines == nil {
				lines = newSideLines(f.File, diffs)
				byFileNew[f.File] = lines
			}
			startLine, endLine = f.Line, f.EndLine
		}
		info, ok := vcs.ComputeFingerprint(f.File, f.Category, startLine, endLine, lines)
		if !ok {
			continue
		}
		f.Fingerprint = info.Fingerprint
		f.Anchor = info.Anchor
		f.AnchorLines = info.AnchorLines
	}
}

// highSeverityRank is the rank at and above which a dismissal needs someone
// other than the merge/pull request author.
const highSeverityRank = 3

// FilterDismissed splits findings into those to keep and those suppressed
// because a thread with the same fingerprint was dismissed. A dismissal that
// came only from the MR/PR author never suppresses HIGH or CRITICAL findings.
// Findings without a fingerprint are always kept.
func FilterDismissed(findings []model.Finding, dismissed []vcs.DismissedFinding) (kept, suppressed []model.Finding) {
	if len(dismissed) == 0 {
		return findings, nil
	}
	known := make(map[string]vcs.DismissedFinding, len(dismissed))
	for _, d := range dismissed {
		if d.Fingerprint == "" {
			continue
		}
		// Prefer the stronger dismissal when several threads share a fingerprint.
		if prev, ok := known[d.Fingerprint]; ok && !prev.AuthorOnly {
			continue
		}
		known[d.Fingerprint] = d
	}
	kept = make([]model.Finding, 0, len(findings))
	for _, f := range findings {
		d, ok := known[f.Fingerprint]
		if f.Fingerprint == "" || !ok || (d.AuthorOnly && severityRank(f.Severity) >= highSeverityRank) {
			kept = append(kept, f)
			continue
		}
		suppressed = append(suppressed, f)
	}
	return kept, suppressed
}

// BlockingCount returns how many of the findings are HIGH or CRITICAL, the
// ones that must keep failing the CI gate even when they are not re-posted.
func BlockingCount(findings []model.Finding) int {
	n := 0
	for _, f := range findings {
		if severityRank(f.Severity) >= highSeverityRank {
			n++
		}
	}
	return n
}

// anchorUnchanged reports whether the lines a dismissed thread was anchored to
// still appear unmodified, with the same neighbours, in the current diff.
func anchorUnchanged(d vcs.DismissedFinding, diffs []diff.FileDiff) bool {
	if d.Anchor == "" || d.AnchorLines < 1 || d.AnchorLines > vcs.MaxAnchorLines {
		return false
	}
	check := func(lines map[int]string) bool {
		for start := range lines {
			if h, ok := anchorHash(d.Path, lines, start, start+d.AnchorLines-1); ok && h == d.Anchor {
				return true
			}
		}
		return false
	}
	if d.OldSide {
		if check(oldSideLines(d.Path, diffs)) {
			return true
		}
		return check(newSideLines(d.Path, diffs))
	}
	if check(newSideLines(d.Path, diffs)) {
		return true
	}
	return check(oldSideLines(d.Path, diffs))
}

// KeepFingerprints returns the fingerprints of dismissed threads that cleanup
// should leave in place: those whose anchored code is unchanged and that are
// not being re-posted as a new finding this run.
func KeepFingerprints(dismissed []vcs.DismissedFinding, posted []model.Finding, diffs []diff.FileDiff) []string {
	reposted := make(map[string]bool, len(posted))
	for _, f := range posted {
		reposted[f.Fingerprint] = true
	}
	var keep []string
	for _, d := range dismissed {
		if d.Fingerprint != "" && !reposted[d.Fingerprint] && anchorUnchanged(d, diffs) {
			keep = append(keep, d.Fingerprint)
		}
	}
	return keep
}

// FormatDismissedPrompt renders previously dismissed locations as a prompt
// section. It carries only file paths and line numbers, never comment text:
// everything else in a thread is untrusted input. Threads dismissed solely by
// the MR/PR author are left out, because they may not silence serious findings.
func FormatDismissedPrompt(dismissed []vcs.DismissedFinding) string {
	var sb strings.Builder
	for _, d := range dismissed {
		if d.AuthorOnly || d.Path == "" {
			continue
		}
		if d.OldSide && d.Line > 0 {
			fmt.Fprintf(&sb, "- %s (removed line %d)\n", d.Path, d.Line)
		} else if d.Line > 0 {
			fmt.Fprintf(&sb, "- %s:%d\n", d.Path, d.Line)
		} else {
			fmt.Fprintf(&sb, "- %s\n", d.Path)
		}
	}
	if sb.Len() == 0 {
		return ""
	}
	return "## Previously dismissed findings\n\n" +
		"Reviewers already closed out findings at these locations. Do not report them again unless the code there has materially changed:\n" +
		sb.String()
}
