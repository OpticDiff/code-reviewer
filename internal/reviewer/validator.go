package reviewer

import (
	"fmt"
	"log/slog"

	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

// ValidateFindings checks that each finding references a valid line in the diff.
// Invalid findings are dropped and logged. Returns the filtered set.
func ValidateFindings(findings []model.Finding, diffs []diff.FileDiff) []model.Finding {
	// Build a lookup: file -> set of valid new line numbers.
	validLines := make(map[string]map[int]bool)
	for _, d := range diffs {
		path := d.NewPath
		if path == "" {
			path = d.OldPath
		}
		lines := make(map[int]bool)
		for _, h := range d.Hunks {
			for _, l := range h.Lines {
				if l.Type == diff.LineAdded && l.NewLineNo > 0 {
					lines[l.NewLineNo] = true
				}
			}
		}
		validLines[path] = lines
	}

	var valid []model.Finding
	dropped := 0

	for _, f := range findings {
		fileLines, fileExists := validLines[f.File]
		if !fileExists {
			// Try partial path match (model might return different path format).
			matched := false
			for path, lines := range validLines {
				if pathMatch(f.File, path) {
					fileLines = lines
					fileExists = true
					f.File = path // Normalize to the actual path.
					matched = true
					break
				}
			}
			if !matched {
				slog.Warn("dropping finding: file not in diff",
					"file", f.File,
					"title", f.Title,
				)
				dropped++
				continue
			}
		}

		if f.Line <= 0 || !fileLines[f.Line] {
			// Line not in the changed set. Check if it's at least in the file's hunks.
			if f.Line > 0 && isInHunkRange(f.File, f.Line, diffs) {
				// Line is in a hunk but not a changed line — still useful as a note.
				valid = append(valid, f)
				continue
			}
			slog.Warn("dropping finding: invalid line number",
				"file", f.File,
				"line", f.Line,
				"title", f.Title,
			)
			dropped++
			continue
		}

		valid = append(valid, f)
	}

	if dropped > 0 {
		slog.Info(fmt.Sprintf("validation: %d findings valid, %d dropped (invalid line refs)", len(valid), dropped))
	}

	return valid
}

// pathMatch checks if two file paths refer to the same file,
// handling cases where one might be a suffix of the other.
func pathMatch(a, b string) bool {
	if a == b {
		return true
	}
	// Handle "a/foo/bar.go" vs "foo/bar.go".
	if len(a) > len(b) && len(a) > len(b)+1 {
		return a[len(a)-len(b)-1:] == "/"+b
	}
	if len(b) > len(a) && len(b) > len(a)+1 {
		return b[len(b)-len(a)-1:] == "/"+a
	}
	return false
}

// isInHunkRange checks if a line number falls within any hunk of the given file.
func isInHunkRange(file string, line int, diffs []diff.FileDiff) bool {
	for _, d := range diffs {
		path := d.NewPath
		if path == "" {
			path = d.OldPath
		}
		if path != file {
			continue
		}
		for _, h := range d.Hunks {
			end := h.NewStart + h.NewCount
			if line >= h.NewStart && line < end {
				return true
			}
		}
	}
	return false
}
