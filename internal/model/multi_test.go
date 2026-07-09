package model

import (
	"testing"
)

func TestFindingsMatch(t *testing.T) {
	tests := []struct {
		name  string
		a, b  Finding
		match bool
	}{
		{
			"same file+line+category",
			Finding{File: "main.go", Line: 42, Category: "bug"},
			Finding{File: "main.go", Line: 42, Category: "bug"},
			true,
		},
		{
			"nearby lines within 3",
			Finding{File: "main.go", Line: 42, Category: "bug"},
			Finding{File: "main.go", Line: 44, Category: "bug"},
			true,
		},
		{
			"exactly 3 lines apart",
			Finding{File: "main.go", Line: 42, Category: "bug"},
			Finding{File: "main.go", Line: 45, Category: "bug"},
			true,
		},
		{
			"lines too far apart",
			Finding{File: "main.go", Line: 42, Category: "bug"},
			Finding{File: "main.go", Line: 50, Category: "bug"},
			false,
		},
		{
			"different file",
			Finding{File: "main.go", Line: 42, Category: "bug"},
			Finding{File: "util.go", Line: 42, Category: "bug"},
			false,
		},
		{
			"different category",
			Finding{File: "main.go", Line: 42, Category: "bug"},
			Finding{File: "main.go", Line: 42, Category: "security"},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findingsMatch(tt.a, tt.b)
			if got != tt.match {
				t.Errorf("findingsMatch() = %v, want %v", got, tt.match)
			}
		})
	}
}

func TestMergeResults_ThresholdFilters(t *testing.T) {
	results := []*ReviewResult{
		{
			Summary: "Model A found issues.",
			Findings: []Finding{
				{File: "main.go", Line: 10, Category: "bug", Title: "null deref", Body: "short"},
				{File: "main.go", Line: 50, Category: "security", Title: "injection", Body: "only model A sees this"},
			},
		},
		{
			Summary: "Model B found issues.",
			Findings: []Finding{
				{File: "main.go", Line: 10, Category: "bug", Title: "nil pointer", Body: "longer description of null deref issue"},
			},
		},
		{
			Summary: "Model C agreed.",
			Findings: []Finding{
				{File: "main.go", Line: 11, Category: "bug", Title: "nil check", Body: "medium"},
			},
		},
	}

	merged := mergeResults(results, 2)

	// The "injection" finding only appears in 1 model, should be filtered out.
	// The "null deref" finding appears in all 3 (lines 10, 10, 11 — within 3 lines, same category).
	if len(merged.Findings) != 1 {
		t.Fatalf("expected 1 finding after threshold filter, got %d", len(merged.Findings))
	}

	// The canonical finding should have the longest body.
	f := merged.Findings[0]
	if f.Body != "longer description of null deref issue" {
		t.Errorf("expected longest body as canonical, got %q", f.Body)
	}
}

func TestMergeResults_AllAgree(t *testing.T) {
	finding := Finding{File: "main.go", Line: 10, Category: "bug", Title: "issue", Body: "desc"}
	results := []*ReviewResult{
		{Summary: "A", Findings: []Finding{finding}},
		{Summary: "B", Findings: []Finding{finding}},
	}

	merged := mergeResults(results, 2)
	if len(merged.Findings) != 1 {
		t.Errorf("expected 1 finding when all models agree, got %d", len(merged.Findings))
	}
}

func TestMergeResults_SingleModel(t *testing.T) {
	results := []*ReviewResult{
		{
			Summary: "Single model.",
			Findings: []Finding{
				{File: "main.go", Line: 10, Category: "bug", Title: "issue", Body: "desc"},
			},
		},
	}

	merged := mergeResults(results, 1)
	if len(merged.Findings) != 1 {
		t.Errorf("expected 1 finding with threshold=1, got %d", len(merged.Findings))
	}
}

func TestMergeResults_NilResults(t *testing.T) {
	results := []*ReviewResult{nil, nil}
	merged := mergeResults(results, 1)
	if len(merged.Findings) != 0 {
		t.Errorf("expected 0 findings for nil results, got %d", len(merged.Findings))
	}
}

func TestMergeResults_SummaryMerge(t *testing.T) {
	results := []*ReviewResult{
		{Summary: "Model A summary."},
		{Summary: "Model B summary."},
		{Summary: "Model A summary."}, // duplicate
	}

	merged := mergeResults(results, 1)
	if merged.Summary != "Model A summary. Model B summary." {
		t.Errorf("expected merged unique summaries, got %q", merged.Summary)
	}
}

func TestMergeResults_HighThreshold(t *testing.T) {
	finding := Finding{File: "main.go", Line: 10, Category: "bug", Title: "issue", Body: "desc"}
	results := []*ReviewResult{
		{Summary: "A", Findings: []Finding{finding}},
		{Summary: "B", Findings: []Finding{finding}},
	}

	// Threshold 3 but only 2 models — nothing should pass.
	merged := mergeResults(results, 3)
	if len(merged.Findings) != 0 {
		t.Errorf("expected 0 findings with threshold=3 and 2 models, got %d", len(merged.Findings))
	}
}

func TestMergeResults_MultipleFindingsDedup(t *testing.T) {
	results := []*ReviewResult{
		{
			Summary: "A",
			Findings: []Finding{
				{File: "a.go", Line: 10, Category: "bug", Title: "bug1", Body: "short"},
				{File: "b.go", Line: 20, Category: "security", Title: "sec1", Body: "short"},
			},
		},
		{
			Summary: "B",
			Findings: []Finding{
				{File: "a.go", Line: 11, Category: "bug", Title: "bug1-variant", Body: "longer description here"},
				{File: "b.go", Line: 22, Category: "security", Title: "sec1-variant", Body: "longer sec"},
				{File: "c.go", Line: 5, Category: "style", Title: "only-B", Body: "unique to B"},
			},
		},
	}

	merged := mergeResults(results, 2)

	// a.go bug and b.go security should be kept (appear in both).
	// c.go style only in B — should be filtered.
	if len(merged.Findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(merged.Findings))
	}
}
