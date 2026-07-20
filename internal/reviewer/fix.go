package reviewer

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/OpticDiff/code-reviewer/internal/model"
)

// ApplyFix holds metadata about a single applied fix.
type ApplyFix struct {
	File       string
	Line       int
	Title      string
	Suggestion string
	Applied    bool
	Reason     string // Non-empty if skipped.
}

// ApplyFixes takes review findings with suggestions and applies them to the
// working tree. Returns a list of applied/skipped fixes. Files are resolved
// relative to repoRoot (or cwd if empty).
func ApplyFixes(findings []model.Finding, repoRoot string) []ApplyFix {
	// Collect fixable findings.
	var fixes []ApplyFix
	for _, f := range findings {
		if f.Suggestion == "" {
			continue
		}
		fixes = append(fixes, ApplyFix{
			File:       f.File,
			Line:       f.Line,
			Title:      f.Title,
			Suggestion: f.Suggestion,
		})
	}

	if len(fixes) == 0 {
		return nil
	}

	// Group by file and sort by line descending (apply bottom-up to preserve line numbers).
	byFile := map[string][]int{}
	fixMap := map[string]map[int]*ApplyFix{}
	for i := range fixes {
		f := &fixes[i]
		byFile[f.File] = append(byFile[f.File], i)
		if fixMap[f.File] == nil {
			fixMap[f.File] = map[int]*ApplyFix{}
		}
		fixMap[f.File][f.Line] = f
	}

	for file, idxs := range byFile {
		// Sort by line descending so we apply from bottom to top.
		sort.Slice(idxs, func(a, b int) bool {
			return fixes[idxs[a]].Line > fixes[idxs[b]].Line
		})

		filePath := file
		if repoRoot != "" {
			filePath = filepath.Join(repoRoot, file)
		}

		content, err := os.ReadFile(filePath)
		if err != nil {
			for _, idx := range idxs {
				fixes[idx].Reason = fmt.Sprintf("cannot read file: %v", err)
			}
			continue
		}

		lines := strings.Split(string(content), "\n")

		for _, idx := range idxs {
			fix := &fixes[idx]
			lineIdx := fix.Line - 1 // Convert to 0-indexed.

			if lineIdx < 0 || lineIdx >= len(lines) {
				fix.Reason = fmt.Sprintf("line %d out of range (file has %d lines)", fix.Line, len(lines))
				continue
			}

			// The suggestion replaces the target line. If the suggestion is
			// multi-line, it replaces one original line with multiple new lines.
			suggestionLines := strings.Split(strings.TrimRight(fix.Suggestion, "\n"), "\n")

			// Replace: remove original line, insert suggestion lines.
			newLines := make([]string, 0, len(lines)+len(suggestionLines)-1)
			newLines = append(newLines, lines[:lineIdx]...)
			newLines = append(newLines, suggestionLines...)
			newLines = append(newLines, lines[lineIdx+1:]...)
			lines = newLines

			fix.Applied = true
			slog.Info("applied fix",
				"file", fix.File,
				"line", fix.Line,
				"title", fix.Title,
			)
		}

		// Write back.
		newContent := strings.Join(lines, "\n")
		if err := os.WriteFile(filePath, []byte(newContent), 0o644); err != nil {
			slog.Warn("failed to write file", "file", filePath, "error", err)
			// Mark all fixes for this file as failed.
			for _, idx := range idxs {
				if fixes[idx].Applied {
					fixes[idx].Applied = false
					fixes[idx].Reason = fmt.Sprintf("write failed: %v", err)
				}
			}
		}
	}

	return fixes
}

// FormatFixSummary renders a human-readable summary of applied fixes.
func FormatFixSummary(fixes []ApplyFix, useColor bool) string {
	if len(fixes) == 0 {
		return "No suggestions to apply.\n"
	}

	var sb strings.Builder
	applied := 0
	skipped := 0
	for _, f := range fixes {
		if f.Applied {
			applied++
		} else {
			skipped++
		}
	}

	if useColor {
		sb.WriteString("\n" + ansiBold + ansiCyan)
	}
	sb.WriteString("🔧 Auto-Fix Summary\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	if useColor {
		sb.WriteString(ansiReset)
	}
	sb.WriteString("\n\n")

	for _, f := range fixes {
		if f.Applied {
			if useColor {
				fmt.Fprintf(&sb, "  %s✅%s %s:%d — %s\n", ansiGreen, ansiReset, f.File, f.Line, f.Title)
			} else {
				fmt.Fprintf(&sb, "  ✅ %s:%d — %s\n", f.File, f.Line, f.Title)
			}
		} else {
			if useColor {
				fmt.Fprintf(&sb, "  %s⏭️%s  %s:%d — %s (%s)\n", ansiYellow, ansiReset, f.File, f.Line, f.Title, f.Reason)
			} else {
				fmt.Fprintf(&sb, "  ⏭️  %s:%d — %s (%s)\n", f.File, f.Line, f.Title, f.Reason)
			}
		}
	}

	sb.WriteString("\n")
	fmt.Fprintf(&sb, "  Applied: %d  Skipped: %d\n\n", applied, skipped)

	return sb.String()
}
