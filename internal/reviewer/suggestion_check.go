package reviewer

import (
	"log/slog"
	"strings"
	"unicode"

	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

// CheckSuggestions drops suggestions that would corrupt the file if applied,
// returning the findings (with the offending suggestions cleared) and the
// number of suggestions dropped. The findings themselves are always kept.
//
// A suggestion replaces the lines Line..EndLine (just Line when EndLine is
// unset). It is dropped when:
//   - its leading lines repeat the lines directly above that range, or its
//     trailing lines repeat the lines directly below it, so applying it would
//     duplicate code the range did not cover;
//   - the finding quotes existing code and none of it is in the range, so the
//     anchor sits on different lines than the code being rewritten.
//
// Ranges that are not fully visible in the diff cannot be verified and are
// left alone. Lines holding no letters or digits (braces, blanks) never count
// as duplicates because they repeat legitimately. The input slice is not
// modified.
func CheckSuggestions(findings []model.Finding, diffs []diff.FileDiff) ([]model.Finding, int) {
	sources := make(map[string]map[int]string, len(diffs))
	for i := range diffs {
		d := &diffs[i]
		path := d.NewPath
		if path == "" {
			path = d.OldPath
		}
		lines := make(map[int]string)
		for _, h := range d.Hunks {
			for _, l := range h.Lines {
				if l.Type != diff.LineRemoved && l.NewLineNo > 0 {
					lines[l.NewLineNo] = strings.TrimSpace(l.Content)
				}
			}
		}
		sources[path] = lines
	}

	out := make([]model.Finding, len(findings))
	copy(out, findings)
	dropped := 0
	for i := range out {
		f := &out[i]
		if f.Suggestion == "" {
			continue
		}
		src, ok := sources[f.File]
		if !ok {
			continue
		}
		if reason := suggestionProblem(*f, src); reason != "" {
			slog.Warn("dropping suggestion, keeping finding",
				"file", f.File,
				"line", f.Line,
				"title", f.Title,
				"reason", reason,
			)
			f.Suggestion = ""
			dropped++
		}
	}
	return out, dropped
}

// suggestionProblem returns why the finding's suggestion is unsafe to apply
// against src (new-side line number to trimmed content), or "" if it looks fine.
func suggestionProblem(f model.Finding, src map[int]string) string {
	start, end := f.Line, f.Line
	if f.EndLine > f.Line {
		end = f.EndLine
	}
	rangeLines := make([]string, 0, end-start+1)
	for n := start; n <= end; n++ {
		line, ok := src[n]
		if !ok {
			return ""
		}
		rangeLines = append(rangeLines, line)
	}

	repl := strings.Split(strings.TrimRight(f.Suggestion, "\n"), "\n")
	for i := range repl {
		repl[i] = strings.TrimSpace(repl[i])
	}

	if k := repeatedAbove(repl, src, start); k > 0 && repl[0] != rangeLines[0] {
		return "replacement repeats lines above the range"
	}
	if k := repeatedBelow(repl, src, end); k > 0 && repl[len(repl)-1] != rangeLines[len(rangeLines)-1] {
		return "replacement repeats lines below the range"
	}

	quoted := f.CodeSnippet
	if quoted == "" {
		quoted = f.ExistingCode
	}
	if !quotedCodeInRange(quoted, rangeLines) {
		return "quoted code is not in the suggestion range"
	}
	return ""
}

// repeatedAbove returns how many leading replacement lines equal the source
// lines directly above start, counting only if one of them is substantive.
func repeatedAbove(repl []string, src map[int]string, start int) int {
	for k := len(repl); k >= 1; k-- {
		match := true
		for j := 0; j < k; j++ {
			if line, ok := src[start-k+j]; !ok || line != repl[j] {
				match = false
				break
			}
		}
		if match && anySubstantive(repl[:k]) {
			return k
		}
	}
	return 0
}

// repeatedBelow returns how many trailing replacement lines equal the source
// lines directly below end, counting only if one of them is substantive.
func repeatedBelow(repl []string, src map[int]string, end int) int {
	for k := len(repl); k >= 1; k-- {
		match := true
		for j := 0; j < k; j++ {
			if line, ok := src[end+1+j]; !ok || line != repl[len(repl)-k+j] {
				match = false
				break
			}
		}
		if match && anySubstantive(repl[len(repl)-k:]) {
			return k
		}
	}
	return 0
}

// quotedCodeInRange reports whether at least one substantive line of the
// quoted code appears (possibly as a fragment of a line) in the range. Empty or purely structural quotes carry
// no signal and pass.
func quotedCodeInRange(quoted string, rangeLines []string) bool {
	var lines []string
	for _, l := range strings.Split(quoted, "\n") {
		if l = strings.TrimSpace(l); isSubstantive(l) {
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		return true
	}
	text := strings.Join(rangeLines, "\n")
	for _, q := range lines {
		if strings.Contains(text, q) {
			return true
		}
	}
	return false
}

func anySubstantive(lines []string) bool {
	for _, l := range lines {
		if isSubstantive(l) {
			return true
		}
	}
	return false
}

// isSubstantive reports whether a line contains a letter or digit.
func isSubstantive(line string) bool {
	return strings.IndexFunc(line, func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r)
	}) >= 0
}
