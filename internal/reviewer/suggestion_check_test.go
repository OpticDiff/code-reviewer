package reviewer

import (
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

func suggestionDiffs() []diff.FileDiff {
	return []diff.FileDiff{{
		NewPath: "pkg/service.go",
		Hunks: []diff.Hunk{{
			NewStart: 10,
			NewCount: 6,
			Lines: []diff.DiffLine{
				{Type: diff.LineContext, NewLineNo: 10, Content: "func run() error {"},
				{Type: diff.LineContext, NewLineNo: 11, Content: "\tvar cfg Config"},
				{Type: diff.LineAdded, NewLineNo: 12, Content: "\tcfg, err := load()"},
				{Type: diff.LineRemoved, Content: "\told()"},
				{Type: diff.LineAdded, NewLineNo: 13, Content: "\tuse(cfg)"},
				{Type: diff.LineContext, NewLineNo: 14, Content: "\treturn nil"},
				{Type: diff.LineContext, NewLineNo: 15, Content: "}"},
			},
		}},
	}}
}

func TestCheckSuggestions(t *testing.T) {
	tests := []struct {
		name        string
		finding     model.Finding
		wantKept    bool
		wantDropped int
	}{
		{
			name:     "clean single-line replacement is kept",
			finding:  model.Finding{File: "pkg/service.go", Line: 12, Suggestion: "\tcfg, err := loadConfig()"},
			wantKept: true,
		},
		{
			name:        "replacement repeats declaration above the range",
			finding:     model.Finding{File: "pkg/service.go", Line: 12, Suggestion: "\tvar cfg Config\n\tcfg, err := loadConfig()"},
			wantDropped: 1,
		},
		{
			name:        "replacement repeats line just below the range",
			finding:     model.Finding{File: "pkg/service.go", Line: 13, Suggestion: "\tuse(cfg)\n\treturn nil"},
			wantDropped: 1,
		},
		{
			name:        "replacement repeats several lines above the range",
			finding:     model.Finding{File: "pkg/service.go", Line: 13, Suggestion: "\tvar cfg Config\n\tcfg, err := load()\n\tuse(&cfg)"},
			wantDropped: 1,
		},
		{
			name:     "range covering the repeated lines is kept",
			finding:  model.Finding{File: "pkg/service.go", Line: 11, EndLine: 12, Suggestion: "\tvar cfg Config\n\tcfg, err := loadConfig()"},
			wantKept: true,
		},
		{
			name:     "punctuation-only line above is not a duplicate",
			finding:  model.Finding{File: "pkg/service.go", Line: 14, Suggestion: "\treturn err\n}"},
			wantKept: true,
		},
		{
			name:     "replacement starting like the original first line is kept",
			finding:  model.Finding{File: "pkg/service.go", Line: 12, Suggestion: "\tcfg, err := load()\n\tcheck(err)"},
			wantKept: true,
		},
		{
			name:        "existing code anchored on different lines",
			finding:     model.Finding{File: "pkg/service.go", Line: 14, ExistingCode: "cfg, err := load()", Suggestion: "\treturn err"},
			wantDropped: 1,
		},
		{
			name:     "existing code inside the range is kept",
			finding:  model.Finding{File: "pkg/service.go", Line: 12, ExistingCode: "cfg, err := load()", Suggestion: "\tcfg, err := loadConfig()"},
			wantKept: true,
		},
		{
			name:     "existing code fragment of a range line is kept",
			finding:  model.Finding{File: "pkg/service.go", Line: 12, ExistingCode: "err := load()", Suggestion: "\tcfg, err := loadConfig()"},
			wantKept: true,
		},
		{
			name:     "range outside the diff cannot be verified and is kept",
			finding:  model.Finding{File: "pkg/service.go", Line: 40, Suggestion: "\tvar cfg Config\n\tx()"},
			wantKept: true,
		},
		{
			name:     "unknown file is kept",
			finding:  model.Finding{File: "other.go", Line: 12, Suggestion: "\tvar cfg Config"},
			wantKept: true,
		},
		{
			name:     "finding without suggestion is untouched",
			finding:  model.Finding{File: "pkg/service.go", Line: 12},
			wantKept: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.finding.Suggestion
			got, dropped := CheckSuggestions([]model.Finding{tt.finding}, suggestionDiffs())
			if dropped != tt.wantDropped {
				t.Errorf("dropped = %d, want %d", dropped, tt.wantDropped)
			}
			if len(got) != 1 {
				t.Fatalf("finding must be preserved, got %d findings", len(got))
			}
			if tt.wantKept && got[0].Suggestion != want {
				t.Errorf("suggestion = %q, want it kept", got[0].Suggestion)
			}
			if !tt.wantKept && got[0].Suggestion != "" {
				t.Errorf("suggestion = %q, want it cleared", got[0].Suggestion)
			}
		})
	}
}

func TestCheckSuggestions_DoesNotMutateInput(t *testing.T) {
	in := []model.Finding{{File: "pkg/service.go", Line: 12, Suggestion: "\tvar cfg Config\n\tx()"}}
	CheckSuggestions(in, suggestionDiffs())
	if in[0].Suggestion == "" {
		t.Error("input slice was mutated")
	}
}
