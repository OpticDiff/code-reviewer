package reviewer

import (
	"fmt"
	"strings"

	"github.com/OpticDiff/code-reviewer/internal/diff"
)

// maxListedFiles caps the changed-file overview so it stays small on very
// large diffs.
const maxListedFiles = 200

// changedFilesBudgetDivisor sets the share of the token limit the overview may
// use (1/20). It comes out of the 20% the chunker reserves for the system
// prompt and response.
const changedFilesBudgetDivisor = 20

// buildChangedFilesSection lists every changed file with its added and removed
// line counts. Chunks are reviewed independently, so this gives each one the
// shape of the whole change. The listing is cut short once it would exceed
// maxTokens (estimated at 4 characters per token); it returns "" if not even
// the first entry fits.
func buildChangedFilesSection(diffs []diff.FileDiff, maxTokens int) string {
	const header = "=== All changed files (some are in other chunks) ===\n"
	var sb strings.Builder
	sb.WriteString(header)
	listed := 0
	for i, d := range diffs {
		if i == maxListedFiles {
			break
		}
		path := d.NewPath
		if path == "" {
			path = d.OldPath
		}
		added, removed := 0, 0
		for _, h := range d.Hunks {
			for _, l := range h.Lines {
				switch l.Type {
				case diff.LineAdded:
					added++
				case diff.LineRemoved:
					removed++
				}
			}
		}
		line := fmt.Sprintf("%s (+%d -%d)\n", path, added, removed)
		if (sb.Len()+len(line)+len("... and 999 more\n"))/4 > maxTokens {
			break
		}
		sb.WriteString(line)
		listed++
	}
	if listed == 0 {
		return ""
	}
	if listed < len(diffs) {
		fmt.Fprintf(&sb, "... and %d more\n", len(diffs)-listed)
	}
	sb.WriteString("\n")
	return sb.String()
}

// mergeChunkSummaries joins the per-chunk summaries, in chunk order, skipping
// empty and repeated ones.
func mergeChunkSummaries(summaries []string) string {
	seen := make(map[string]bool, len(summaries))
	var parts []string
	for _, s := range summaries {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		parts = append(parts, s)
	}
	return strings.Join(parts, "\n\n")
}
