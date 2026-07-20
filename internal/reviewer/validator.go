package reviewer

import (
	"fmt"
	"log/slog"
	"strings"

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

	valid = sanitizeSuggestions(valid)

	return valid
}

// sanitizeSuggestions cleans up suggestion fields that contain common model
// artifacts: markdown code fences, diff markers, explanatory prefixes, and
// trailing prose. Suggestions that appear to be prose rather than code are
// cleared to prevent invalid code from being applied.
func sanitizeSuggestions(findings []model.Finding) []model.Finding {
	for i := range findings {
		s := findings[i].Suggestion
		if s == "" {
			continue
		}

		// Strip markdown code fences (```lang\n...\n```).
		s = stripCodeFences(s)

		// Strip leading/trailing whitespace.
		s = strings.TrimSpace(s)

		// Strip diff markers (lines starting with + or -).
		lines := strings.Split(s, "\n")
		var cleaned []string
		allMarkers := true
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "-") {
				// Strip the marker prefix, keep the code.
				cleaned = append(cleaned, strings.TrimPrefix(strings.TrimPrefix(trimmed, "+"), "-"))
			} else {
				allMarkers = false
				cleaned = append(cleaned, line)
			}
		}
		if allMarkers && len(cleaned) > 0 {
			s = strings.Join(cleaned, "\n")
		}

		// Strip common explanatory prefixes.
		for _, prefix := range []string{"Fix:", "fix:", "Suggestion:", "suggestion:", "Corrected:", "corrected:"} {
			s = strings.TrimPrefix(s, prefix)
		}
		s = strings.TrimSpace(s)

		// Drop suggestions that look like prose rather than code.
		// Heuristic: if the suggestion is a single line with no code-like
		// characters and reads like a sentence, clear it.
		if !strings.Contains(s, "\n") && looksLikeProse(s) {
			slog.Debug("clearing prose-like suggestion",
				"file", findings[i].File,
				"title", findings[i].Title,
				"suggestion", s,
			)
			s = ""
		}

		findings[i].Suggestion = s
	}
	return findings
}

// stripCodeFences removes markdown code fences from a string.
func stripCodeFences(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) < 2 {
		return s
	}
	first := strings.TrimSpace(lines[0])
	last := strings.TrimSpace(lines[len(lines)-1])
	if strings.HasPrefix(first, "```") && last == "```" {
		return strings.Join(lines[1:len(lines)-1], "\n")
	}
	return s
}

// looksLikeProse returns true if the string appears to be natural language
// rather than code. Uses simple heuristics.
func looksLikeProse(s string) bool {
	if len(s) < 10 {
		return false
	}
	// Code typically has: =, (, ), {, }, ;, :, [, ], <, >
	codeChars := 0
	for _, c := range s {
		switch c {
		case '=', '(', ')', '{', '}', ';', '[', ']', '<', '>':
			codeChars++
		}
	}
	if codeChars > 0 {
		return false
	}
	// Starts with capital, ends with period → likely prose.
	if len(s) > 0 && s[0] >= 'A' && s[0] <= 'Z' && s[len(s)-1] == '.' {
		return true
	}
	// Contains common prose words with no code characters.
	proseWords := []string{" the ", " should ", " instead ", " use ", " consider ", " rather ", " make sure "}
	for _, w := range proseWords {
		if strings.Contains(strings.ToLower(s), w) {
			return true
		}
	}
	return false
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
