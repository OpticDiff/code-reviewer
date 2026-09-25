package reviewer

import (
	"strings"

	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

// ResolvedPosition represents a finding anchored to an actual diff position.
type ResolvedPosition struct {
	File string
	Line int
}

// HunkSide indicates which side of a diff hunk to search.
type HunkSide int

const (
	HunkSideNew HunkSide = iota
	HunkSideOld
)

// ParsedDiff represents the parsed diff data and file contents.
type ParsedDiff struct {
	FileDiffs map[string]*diff.FileDiff // Keyed by new file path
	FullFiles map[string][]string       // Keyed by new file path, lines of the file
}

// ResolvePosition attempts to anchor a finding's code snippet to an exact position
// in the diff using a 4-tier progressive matching algorithm.
// Tier 1: Match against new-side hunk lines (added + context)
// Tier 2: Match against old-side hunk lines (deleted + context)
// Tier 3: Full file content scan
// Tier 4: Cross-file relocation (LLM may attribute code to wrong file)
func ResolvePosition(parsedDiff *ParsedDiff, f model.Finding) *ResolvedPosition {
	if parsedDiff == nil {
		return nil
	}
	snippet := f.CodeSnippet
	if snippet == "" {
		snippet = f.ExistingCode
	}

	if snippet == "" {
		return resolveSimple(parsedDiff, f.File, f.Line)
	}

	snippetLines := splitAndNormalize(snippet)
	if len(snippetLines) == 0 {
		return resolveSimple(parsedDiff, f.File, f.Line)
	}

	// Tier 1 & 2
	if fd, ok := parsedDiff.FileDiffs[f.File]; ok {
		// Tier 1: New side
		if pos := matchInHunks(fd, snippetLines, HunkSideNew); pos != nil {
			return &ResolvedPosition{File: f.File, Line: *pos}
		}

		// Tier 2: Old side
		if pos := matchInHunks(fd, snippetLines, HunkSideOld); pos != nil {
			return &ResolvedPosition{File: f.File, Line: *pos}
		}
	}

	// Tier 3: Full file content scan
	if lines, ok := parsedDiff.FullFiles[f.File]; ok {
		if pos := matchInLines(lines, snippetLines); pos != nil {
			return &ResolvedPosition{File: f.File, Line: *pos}
		}
	}

	// Tier 4: Cross-file relocation
	for file, fd := range parsedDiff.FileDiffs {
		if file == f.File {
			continue
		}
		if pos := matchInHunks(fd, snippetLines, HunkSideNew); pos != nil {
			return &ResolvedPosition{File: file, Line: *pos}
		}
		if pos := matchInHunks(fd, snippetLines, HunkSideOld); pos != nil {
			return &ResolvedPosition{File: file, Line: *pos}
		}
	}

	// Tier 4 part 2: Cross-file full content scan
	for file, lines := range parsedDiff.FullFiles {
		if file == f.File {
			continue
		}
		if pos := matchInLines(lines, snippetLines); pos != nil {
			return &ResolvedPosition{File: file, Line: *pos}
		}
	}

	return nil // Unresolvable
}

func resolveSimple(parsedDiff *ParsedDiff, file string, line int) *ResolvedPosition {
	fd, ok := parsedDiff.FileDiffs[file]
	if !ok {
		return nil
	}
	for _, h := range fd.Hunks {
		if line >= h.NewStart && line < h.NewStart+h.NewCount {
			return &ResolvedPosition{File: file, Line: line}
		}
	}
	return nil
}

func matchInHunks(fd *diff.FileDiff, snippetLines []string, side HunkSide) *int {
	for _, h := range fd.Hunks {
		var linesToMatch []struct {
			normalized string
			lineNum    int
		}

		for _, l := range h.Lines {
			if side == HunkSideNew && l.Type == diff.LineRemoved {
				continue
			}
			if side == HunkSideOld && l.Type == diff.LineAdded {
				continue
			}

			norm := normalizeLine(l.Content)
			num := l.NewLineNo
			if side == HunkSideOld {
				num = l.OldLineNo
			}

			if norm != "" { // Only match against non-empty lines
				linesToMatch = append(linesToMatch, struct {
					normalized string
					lineNum    int
				}{norm, num})
			}
		}

		if len(linesToMatch) < len(snippetLines) {
			continue
		}

		for i := 0; i <= len(linesToMatch)-len(snippetLines); i++ {
			match := true
			for j, snipLine := range snippetLines {
				if linesToMatch[i+j].normalized != snipLine {
					match = false
					break
				}
			}
			if match {
				return &linesToMatch[i].lineNum
			}
		}
	}
	return nil
}

func matchInLines(fileLines []string, snippetLines []string) *int {
	var linesToMatch []struct {
		normalized string
		lineNum    int
	}

	for i, l := range fileLines {
		norm := normalizeLine(l)
		if norm != "" {
			linesToMatch = append(linesToMatch, struct {
				normalized string
				lineNum    int
			}{norm, i + 1}) // 1-indexed line numbers
		}
	}

	if len(linesToMatch) < len(snippetLines) {
		return nil
	}

	for i := 0; i <= len(linesToMatch)-len(snippetLines); i++ {
		match := true
		for j, snipLine := range snippetLines {
			if linesToMatch[i+j].normalized != snipLine {
				match = false
				break
			}
		}
		if match {
			return &linesToMatch[i].lineNum
		}
	}
	return nil
}

func normalizeLine(line string) string {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
		line = strings.TrimSpace(line[1:])
	}
	return line
}

func splitAndNormalize(snippet string) []string {
	var result []string
	lines := strings.Split(snippet, "\n")
	for _, l := range lines {
		norm := normalizeLine(l)
		if norm != "" {
			result = append(result, norm)
		}
	}
	return result
}
