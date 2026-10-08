package reviewer

import (
	"fmt"
	"strings"

	"github.com/OpticDiff/code-reviewer/internal/diff"
)

// maxListedFiles caps the changed-file overview so it stays small on very
// large diffs.
const maxListedFiles = 200

// buildChangedFilesSection lists every changed file with its added and removed
// line counts. Chunks are reviewed independently, so this gives each one the
// shape of the whole change.
func buildChangedFilesSection(diffs []diff.FileDiff) string {
	var sb strings.Builder
	sb.WriteString("=== All files changed in this review (some are in other chunks) ===\n")
	for i, d := range diffs {
		if i == maxListedFiles {
			fmt.Fprintf(&sb, "... and %d more\n", len(diffs)-maxListedFiles)
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
		fmt.Fprintf(&sb, "%s (+%d -%d)\n", path, added, removed)
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
