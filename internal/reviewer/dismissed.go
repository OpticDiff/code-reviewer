package reviewer

import (
	"crypto/sha256"
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

// anchorHash hashes the file, the lines start..end and the lines directly
// around them. Including the neighbours keeps identical one-line anchors in
// different places (for example "return err") from sharing a hash. It returns
// false when the first anchored line is not part of the diff.
func anchorHash(file string, lines map[int]string, start, end int) (string, bool) {
	if _, ok := lines[start]; !ok {
		return "", false
	}
	var body []string
	for n := start; n <= end; n++ {
		body = append(body, lines[n])
	}
	sum := sha256.Sum256([]byte(file + "\x00" + lines[start-1] + "\x1f" + strings.Join(body, "\n") + "\x1f" + lines[end+1]))
	return fmt.Sprintf("%x", sum)[:16], true
}

// findingSpan returns the finding's line range clamped to the lines the diff
// shows for the file and to vcs.MaxAnchorLines, so a model-supplied end_line
// cannot make hashing unbounded.
func findingSpan(f model.Finding, lines map[int]string) (start, end int) {
	end = f.EndLine
	if end < f.Line {
		end = f.Line
	}
	last := 0
	for n := range lines {
		if n > last {
			last = n
		}
	}
	if end > last {
		end = last
	}
	if end < f.Line {
		end = f.Line
	}
	if end-f.Line+1 > vcs.MaxAnchorLines {
		end = f.Line + vcs.MaxAnchorLines - 1
	}
	return f.Line, end
}

// AssignFingerprints sets Fingerprint, Anchor and AnchorLines on every
// finding that is anchored inside the diff. The fingerprint combines the
// anchor hash with the lowercased category, and deliberately ignores the
// title, body and line number, which vary between model runs. Any edit to the
// anchored lines or their neighbours yields a new fingerprint. Findings that
// cannot be anchored keep an empty fingerprint and are never suppressed.
func AssignFingerprints(findings []model.Finding, diffs []diff.FileDiff) {
	byFile := make(map[string]map[int]string)
	for i := range findings {
		f := &findings[i]
		lines, ok := byFile[f.File]
		if !ok {
			lines = newSideLines(f.File, diffs)
			byFile[f.File] = lines
		}
		start, end := findingSpan(*f, lines)
		anchor, ok := anchorHash(f.File, lines, start, end)
		if !ok {
			continue
		}
		category := strings.ToLower(strings.TrimSpace(f.Category))
		sum := sha256.Sum256([]byte(anchor + "\x00" + category))
		f.Fingerprint = fmt.Sprintf("%x", sum)[:16]
		f.Anchor = anchor
		f.AnchorLines = end - start + 1
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
	lines := newSideLines(d.Path, diffs)
	for start := range lines {
		if h, ok := anchorHash(d.Path, lines, start, start+d.AnchorLines-1); ok && h == d.Anchor {
			return true
		}
	}
	return false
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
		if d.Line > 0 {
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
