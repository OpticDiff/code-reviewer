package reviewer

import (
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

func TestResolvePosition(t *testing.T) {
	parsedDiff := &ParsedDiff{
		FileDiffs: map[string]*diff.FileDiff{
			"main.go": {
				NewPath: "main.go",
				Hunks: []diff.Hunk{
					{
						NewStart: 10,
						NewCount: 5,
						Lines: []diff.DiffLine{
							{Type: diff.LineContext, NewLineNo: 10, Content: "func main() {"},
							{Type: diff.LineRemoved, OldLineNo: 10, Content: "    oldCall()"},
							{Type: diff.LineAdded, NewLineNo: 11, Content: "    newCall()"},
							{Type: diff.LineAdded, NewLineNo: 12, Content: "    if true {"},
							{Type: diff.LineContext, NewLineNo: 13, Content: "        fmt.Println(\"hi\")"},
						},
					},
				},
			},
		},
		FullFiles: map[string][]string{
			"other.go": {
				"package main",
				"func other() {",
				"    fmt.Println(\"other\")",
				"}",
			},
			"main.go": {
				"package main",
				"import \"fmt\"",
				"func main() {",
				"    newCall()",
				"    if true {",
				"        fmt.Println(\"hi\")",
				"    }",
				"}",
			},
		},
	}

	tests := []struct {
		name     string
		finding  model.Finding
		expected *ResolvedPosition
	}{
		{
			name: "Tier 1: match new side hunk",
			finding: model.Finding{
				File:        "main.go",
				CodeSnippet: "newCall()\nif true {",
			},
			expected: &ResolvedPosition{File: "main.go", Line: 11},
		},
		{
			name: "Tier 2: match old side hunk",
			finding: model.Finding{
				File:        "main.go",
				CodeSnippet: "oldCall()",
			},
			expected: &ResolvedPosition{File: "main.go", Line: 10},
		},
		{
			name: "Tier 3: full file content scan",
			finding: model.Finding{
				File:        "main.go",
				CodeSnippet: "package main\nimport \"fmt\"",
			},
			expected: &ResolvedPosition{File: "main.go", Line: 1},
		},
		{
			name: "Tier 4: cross-file relocation (hunk match)",
			finding: model.Finding{
				File:        "wrong.go",
				CodeSnippet: "newCall()",
			},
			expected: &ResolvedPosition{File: "main.go", Line: 11},
		},
		{
			name: "Tier 4: cross-file relocation (full file match)",
			finding: model.Finding{
				File:        "wrong.go",
				CodeSnippet: "func other() {",
			},
			expected: &ResolvedPosition{File: "other.go", Line: 2},
		},
		{
			name: "Whitespace normalization",
			finding: model.Finding{
				File:        "main.go",
				CodeSnippet: "   + newCall() \n\n if true {",
			},
			expected: &ResolvedPosition{File: "main.go", Line: 11},
		},
		{
			name: "Fallback to simple line-range check when no snippet",
			finding: model.Finding{
				File: "main.go",
				Line: 12,
			},
			expected: &ResolvedPosition{File: "main.go", Line: 12},
		},
		{
			name: "No match returns nil",
			finding: model.Finding{
				File:        "main.go",
				CodeSnippet: "nonexistent code",
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolvePosition(parsedDiff, tt.finding)
			if tt.expected == nil {
				if got != nil {
					t.Errorf("ResolvePosition() = %v, expected nil", got)
				}
			} else {
				if got == nil || got.File != tt.expected.File || got.Line != tt.expected.Line {
					t.Errorf("ResolvePosition() = %v, expected %v", got, tt.expected)
				}
			}
		})
	}
}

func TestResolvePosition_EmptyCodeSnippet(t *testing.T) {
	parsedDiff := &ParsedDiff{
		FileDiffs: map[string]*diff.FileDiff{
			"main.go": {
				Hunks: []diff.Hunk{{NewStart: 10, NewCount: 5}},
			},
		},
	}
	f := model.Finding{File: "main.go", Line: 12}
	pos := ResolvePosition(parsedDiff, f)
	if pos == nil || pos.Line != 12 {
		t.Errorf("expected line 12, got %v", pos)
	}
}

func TestResolvePosition_NilParsedDiff(t *testing.T) {
	f := model.Finding{File: "main.go", Line: 12}
	pos := ResolvePosition(nil, f)
	if pos != nil {
		t.Errorf("expected nil for nil diff, got %v", pos)
	}
}

func TestResolvePosition_MultipleHunks(t *testing.T) {
	parsedDiff := &ParsedDiff{
		FileDiffs: map[string]*diff.FileDiff{
			"main.go": {
				Hunks: []diff.Hunk{
					{NewStart: 1, NewCount: 2},
					{
						NewStart: 10, NewCount: 5,
						Lines: []diff.DiffLine{
							{Type: diff.LineAdded, NewLineNo: 12, Content: "target()"},
						},
					},
					{NewStart: 20, NewCount: 2},
				},
			},
		},
	}
	f := model.Finding{File: "main.go", CodeSnippet: "target()"}
	pos := ResolvePosition(parsedDiff, f)
	if pos == nil || pos.Line != 12 {
		t.Errorf("expected line 12, got %v", pos)
	}
}

func TestResolveSimple_NoFullFiles(t *testing.T) {
	parsedDiff := &ParsedDiff{
		FileDiffs: map[string]*diff.FileDiff{
			"main.go": {
				Hunks: []diff.Hunk{{NewStart: 10, NewCount: 5}},
			},
		},
		FullFiles: nil,
	}
	pos := resolveSimple(parsedDiff, "main.go", 12)
	if pos == nil || pos.Line != 12 {
		t.Errorf("expected line 12, got %v", pos)
	}
}

func TestNormalizeLines_Tabs(t *testing.T) {
	lines := splitAndNormalize("\t\t  func main() { \n\t}")
	if len(lines) != 2 || lines[0] != "func main() {" || lines[1] != "}" {
		t.Errorf("unexpected normalized lines: %v", lines)
	}
}

func TestFindConsecutiveMatch_SingleLine(t *testing.T) {
	parsedDiff := &ParsedDiff{
		FullFiles: map[string][]string{
			"main.go": {"func main() {", "target()", "}"},
		},
	}
	f := model.Finding{File: "main.go", CodeSnippet: "target()"}
	pos := ResolvePosition(parsedDiff, f)
	if pos == nil || pos.Line != 2 {
		t.Errorf("expected line 2, got %v", pos)
	}
}

func TestFindConsecutiveMatch_AtEnd(t *testing.T) {
	parsedDiff := &ParsedDiff{
		FullFiles: map[string][]string{
			"main.go": {"func main() {", "target()", "}"},
		},
	}
	f := model.Finding{File: "main.go", CodeSnippet: "}"}
	pos := ResolvePosition(parsedDiff, f)
	if pos == nil || pos.Line != 3 {
		t.Errorf("expected line 3, got %v", pos)
	}
}
