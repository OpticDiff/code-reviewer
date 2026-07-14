// Package context provides repo-aware code context for reviews.
// It discovers symbols changed in a diff, then finds usages of those
// symbols elsewhere in the repository to help the reviewer catch
// cross-file regressions.
package context

import "github.com/OpticDiff/code-reviewer/internal/diff"

// SymbolChange represents a symbol that was modified in the diff.
type SymbolChange struct {
	Name     string // e.g. "ValidateSession"
	Kind     string // "function", "method", "class", "type", "interface"
	File     string // file where it was changed
	Language string // "go", "kotlin", "java", "python", "typescript"
	Line     int    // line number in the new file
}

// CodeSnippet is a reference to a changed symbol found in an unchanged file.
type CodeSnippet struct {
	File    string // e.g. "internal/handler/auth.go"
	Line    int    // line number
	Content string // the matching line(s)
	Symbol  string // which symbol matched (e.g. "ValidateSession")
}

// extractedLines returns a set of line numbers that were added or modified
// in the given file diff. Used to filter symbols to only those whose
// definitions overlap with changed lines.
func extractedLines(fd diff.FileDiff) map[int]bool {
	lines := make(map[int]bool)
	for _, h := range fd.Hunks {
		for _, l := range h.Lines {
			if l.Type == diff.LineAdded {
				lines[l.NewLineNo] = true
			}
		}
	}
	return lines
}
