package reviewer

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/config"
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

func TestCheckSuggestions_HugeEndLineIsUnverifiable(t *testing.T) {
	in := []model.Finding{{File: "pkg/service.go", Line: 12, EndLine: 1 << 40, Suggestion: "\tvar cfg Config\n\tx()"}}
	got, dropped := CheckSuggestions(in, suggestionDiffs())
	if dropped != 0 || got[0].Suggestion == "" {
		t.Errorf("unverifiable range must keep the suggestion, dropped=%d", dropped)
	}
}

func TestCheckSuggestions_QuoteNormalization(t *testing.T) {
	for name, quote := range map[string]string{
		"plus marker":      "+\tcfg, err := load()",
		"minus marker":     "- cfg, err := load()",
		"inner whitespace": "cfg,   err  :=\tload()",
		"ellipsis":         "...\ncfg, err := load()\n...",
	} {
		t.Run(name, func(t *testing.T) {
			f := model.Finding{File: "pkg/service.go", Line: 12, ExistingCode: quote, Suggestion: "\tcfg, err := loadConfig()"}
			if _, dropped := CheckSuggestions([]model.Finding{f}, suggestionDiffs()); dropped != 0 {
				t.Errorf("suggestion wrongly dropped for quote %q", quote)
			}
		})
	}
}

func TestCheckSuggestions_EdgeExemptions(t *testing.T) {
	// Trailing edge: replacement ends like the original last line of the range,
	// which also equals the line below (duplicated source lines).
	src := []diff.FileDiff{{NewPath: "a.go", Hunks: []diff.Hunk{{Lines: []diff.DiffLine{
		{Type: diff.LineContext, NewLineNo: 1, Content: "call(x)"},
		{Type: diff.LineContext, NewLineNo: 2, Content: "call(x)"},
	}}}}}
	f := model.Finding{File: "a.go", Line: 1, Suggestion: "prep()\ncall(x)"}
	if _, dropped := CheckSuggestions([]model.Finding{f}, src); dropped != 0 {
		t.Error("trailing-edge exemption: suggestion ending like the original line must be kept")
	}
	// Leading edge: starts like the original first line, equal to the line above.
	f = model.Finding{File: "a.go", Line: 2, Suggestion: "call(x)\ncheck()"}
	if _, dropped := CheckSuggestions([]model.Finding{f}, src); dropped != 0 {
		t.Error("leading-edge exemption: suggestion starting like the original line must be kept")
	}
}

func TestReanchor_KeepsSpan(t *testing.T) {
	f := model.Finding{Line: 10, EndLine: 12}
	reanchor(&f, 20)
	if f.Line != 20 || f.EndLine != 22 {
		t.Errorf("got %d-%d, want 20-22", f.Line, f.EndLine)
	}
	single := model.Finding{Line: 10}
	reanchor(&single, 5)
	if single.Line != 5 || single.EndLine != 0 {
		t.Errorf("got %d-%d, want 5-0", single.Line, single.EndLine)
	}
}

func TestRun_AuditRecordsSuggestionsDropped(t *testing.T) {
	testDiffs := suggestionDiffs()
	logPath := filepath.Join(t.TempDir(), "audit.jsonl")
	cfg := &config.Config{NoCache: true,
		DiffMode:      true,
		Model:         "gemini-2.5-flash",
		ChunkStrategy: config.ChunkStrategyFail,
		MinSeverity:   config.SeverityLow,
		DryRun:        true,
		AuditLog:      logPath,
	}
	mm := &mockModel{result: &model.ReviewResult{Summary: "s", Findings: []model.Finding{{
		File: "pkg/service.go", Line: 12, Severity: "HIGH", Category: "bug", Title: "t", Body: "b",
		Suggestion: "\tvar cfg Config\n\tcfg, err := loadConfig()",
	}}}}
	r := NewWithDiffSource(cfg, mm, &mockVCS{}, &mockDiffSource{diffs: testDiffs})
	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	var entry AuditEntry
	if err := json.Unmarshal(bytes.TrimSpace(data), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.SuggestionsDropped != 1 || entry.FindingsCount != 1 {
		t.Errorf("suggestions_dropped=%d findings=%d, want 1 and 1", entry.SuggestionsDropped, entry.FindingsCount)
	}
}
