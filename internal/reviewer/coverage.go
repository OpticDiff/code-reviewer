package reviewer

import (
	"fmt"
	"strings"

	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

// Reasons a file was not reviewed, as reported in the audit log and the
// coverage line.
const (
	SkipTooLarge   = "too_large"
	SkipCollapsed  = "collapsed"
	SkipEmptyPatch = "empty_patch"
	SkipBudget     = "budget"
	SkipParseError = "parse_error"
)

// skipReasonOrder fixes the order in which reasons appear in the coverage line.
var skipReasonOrder = []string{SkipTooLarge, SkipCollapsed, SkipEmptyPatch, SkipBudget, SkipParseError}

// SkippedFile is a file that was part of the change but never reviewed.
type SkippedFile struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// splitUnreviewable separates files that carry no reviewable patch from the
// rest. Files without a recorded SkipReason are always kept.
func splitUnreviewable(diffs []diff.FileDiff) ([]diff.FileDiff, []SkippedFile) {
	var skipped []SkippedFile
	kept := make([]diff.FileDiff, 0, len(diffs))
	for _, d := range diffs {
		if d.SkipReason == "" {
			kept = append(kept, d)
			continue
		}
		path := d.NewPath
		if path == "" {
			path = d.OldPath
		}
		skipped = append(skipped, SkippedFile{Path: path, Reason: d.SkipReason})
	}
	return kept, skipped
}

// skipReasonFor classifies a file whose platform-supplied patch is unusable,
// or returns "" when the patch is present or legitimately absent.
//
// Renames, empty new files and deletions of empty files carry no hunks by
// design, so an empty diff is only a gap for a modified file. Binary and
// mode-only changes also arrive with an empty diff and cannot be told apart
// from a truncated one with the fields the platforms return, so they are
// reported as empty_patch too; see coverage.blockingPaths for why that does not
// block approval.
func skipReasonFor(e vcs.DiffEntry) string {
	switch {
	case e.TooLarge:
		return SkipTooLarge
	case e.Collapsed:
		return SkipCollapsed
	case e.Diff == "" && !e.RenamedFile && !e.NewFile && !e.DeletedFile:
		return SkipEmptyPatch
	}
	return ""
}

// coverage summarises how much of the change was reviewed.
type coverage struct {
	total   int
	skipped []SkippedFile
}

// buildCoverage assembles the skipped-file list from every source. inScope is
// the number of files that entered the budget stage; unreviewable files and
// parse failures never did, so they add to the total. Files matching
// excludedPatterns are not counted.
func buildCoverage(inScope int, unreviewable []SkippedFile, parseFailed, budgetSkipped []string, excludedPatterns []string) coverage {
	c := coverage{total: inScope + len(unreviewable)}
	c.skipped = append(c.skipped, unreviewable...)
	for _, p := range parseFailed {
		if diff.IsExcluded(p, excludedPatterns) {
			continue
		}
		c.total++
		c.skipped = append(c.skipped, SkippedFile{Path: p, Reason: SkipParseError})
	}
	for _, p := range budgetSkipped {
		c.skipped = append(c.skipped, SkippedFile{Path: p, Reason: SkipBudget})
	}
	return c
}

// paths returns the skipped file paths in order.
func (c coverage) paths() []string {
	if len(c.skipped) == 0 {
		return nil
	}
	out := make([]string, len(c.skipped))
	for i, s := range c.skipped {
		out[i] = s.Path
	}
	return out
}

// blockingPaths returns the skipped paths that must stop auto-approval.
// empty_patch is excluded: it is ambiguous (binary and mode-only changes look
// the same as a truncated patch), so blocking on it would refuse some merge
// requests permanently with nothing the author can fix. It is still reported.
func (c coverage) blockingPaths() []string {
	var out []string
	for _, s := range c.skipped {
		if s.Reason != SkipEmptyPatch {
			out = append(out, s.Path)
		}
	}
	return out
}

// line renders the coverage line, or "" when every file was reviewed.
func (c coverage) line() string {
	return coverageLine(c.total-len(c.skipped), c.total, c.skipped)
}

// coverageLine renders, e.g., "Reviewed 104 of 174 files. Not reviewed: 70
// (too_large 3, empty_patch 67).". It returns "" when nothing was skipped.
func coverageLine(reviewed, total int, skipped []SkippedFile) string {
	if len(skipped) == 0 {
		return ""
	}
	counts := make(map[string]int)
	for _, s := range skipped {
		counts[s.Reason]++
	}
	var parts []string
	for _, reason := range skipReasonOrder {
		if n := counts[reason]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", reason, n))
		}
	}
	return fmt.Sprintf("Reviewed %d of %d files. Not reviewed: %d (%s).",
		reviewed, total, len(skipped), strings.Join(parts, ", "))
}
