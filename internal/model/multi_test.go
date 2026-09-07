package model

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
)

// mockReviewer implements ReviewProvider for testing.
type mockReviewer struct {
	result    *ReviewResult
	err       error
	callCount atomic.Int32
	closed    atomic.Int32
}

func (m *mockReviewer) Review(_ context.Context, _, _ string) (*ReviewResult, error) {
	m.callCount.Add(1)
	return m.result, m.err
}

func (m *mockReviewer) Close() {
	m.closed.Add(1)
}

// ---------------------------------------------------------------------------
// findingsMatch
// ---------------------------------------------------------------------------

func TestFindingsMatch(t *testing.T) {
	tests := []struct {
		name  string
		a, b  Finding
		match bool
	}{
		{"same file+line+category", Finding{File: "main.go", Line: 42, Category: "bug"}, Finding{File: "main.go", Line: 42, Category: "bug"}, true},
		{"nearby lines within 3", Finding{File: "main.go", Line: 42, Category: "bug"}, Finding{File: "main.go", Line: 44, Category: "bug"}, true},
		{"exactly 3 lines apart", Finding{File: "main.go", Line: 42, Category: "bug"}, Finding{File: "main.go", Line: 45, Category: "bug"}, true},
		{"lines too far apart", Finding{File: "main.go", Line: 42, Category: "bug"}, Finding{File: "main.go", Line: 50, Category: "bug"}, false},
		{"different file", Finding{File: "main.go", Line: 42, Category: "bug"}, Finding{File: "util.go", Line: 42, Category: "bug"}, false},
		{"different category", Finding{File: "main.go", Line: 42, Category: "bug"}, Finding{File: "main.go", Line: 42, Category: "security"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findingsMatch(tt.a, tt.b); got != tt.match {
				t.Errorf("findingsMatch() = %v, want %v", got, tt.match)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// mergeResults — including regression test for canonical drift
// ---------------------------------------------------------------------------

func TestMergeResults_ThresholdFilters(t *testing.T) {
	results := []*ReviewResult{
		{Summary: "A", Findings: []Finding{
			{File: "main.go", Line: 10, Category: "bug", Title: "null deref", Body: "short"},
			{File: "main.go", Line: 50, Category: "security", Title: "injection", Body: "only A"},
		}},
		{Summary: "B", Findings: []Finding{
			{File: "main.go", Line: 10, Category: "bug", Title: "nil pointer", Body: "longer description of null deref issue"},
		}},
		{Summary: "C", Findings: []Finding{
			{File: "main.go", Line: 11, Category: "bug", Title: "nil check", Body: "medium"},
		}},
	}
	merged := mergeResults(results, 2)

	if len(merged.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(merged.Findings))
	}
	if merged.Findings[0].Body != "longer description of null deref issue" {
		t.Errorf("expected longest body as canonical, got %q", merged.Findings[0].Body)
	}
}

func TestMergeResults_AllAgree(t *testing.T) {
	f := Finding{File: "main.go", Line: 10, Category: "bug", Title: "issue", Body: "desc"}
	merged := mergeResults([]*ReviewResult{{Summary: "A", Findings: []Finding{f}}, {Summary: "B", Findings: []Finding{f}}}, 2)
	if len(merged.Findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(merged.Findings))
	}
}

func TestMergeResults_SingleModel(t *testing.T) {
	merged := mergeResults([]*ReviewResult{{Summary: "S", Findings: []Finding{{File: "a.go", Line: 1, Category: "bug", Body: "x"}}}}, 1)
	if len(merged.Findings) != 1 {
		t.Errorf("expected 1, got %d", len(merged.Findings))
	}
}

func TestMergeResults_NilResults(t *testing.T) {
	merged := mergeResults([]*ReviewResult{nil, nil}, 1)
	if len(merged.Findings) != 0 {
		t.Errorf("expected 0, got %d", len(merged.Findings))
	}
}

func TestMergeResults_UsageAggregation(t *testing.T) {
	results := []*ReviewResult{
		{
			Summary:  "A",
			Findings: []Finding{{File: "a.go", Line: 1, Category: "bug", Title: "x", Body: "y"}},
			Usage:    &TokenUsage{InputTokens: 1000, OutputTokens: 200, TotalTokens: 1200},
		},
		{
			Summary:  "B",
			Findings: []Finding{{File: "a.go", Line: 1, Category: "bug", Title: "x", Body: "y"}},
			Usage:    &TokenUsage{InputTokens: 1500, OutputTokens: 300, TotalTokens: 1800},
		},
	}
	merged := mergeResults(results, 2)
	if merged.Usage == nil {
		t.Fatal("expected usage to be set")
	}
	if merged.Usage.InputTokens != 2500 {
		t.Errorf("expected 2500 input tokens, got %d", merged.Usage.InputTokens)
	}
	if merged.Usage.OutputTokens != 500 {
		t.Errorf("expected 500 output tokens, got %d", merged.Usage.OutputTokens)
	}
	if merged.Usage.TotalTokens != 3000 {
		t.Errorf("expected 3000 total tokens, got %d", merged.Usage.TotalTokens)
	}
}

func TestMergeResults_UsageNilWhenNoUsage(t *testing.T) {
	results := []*ReviewResult{
		{Summary: "A", Findings: []Finding{{File: "a.go", Line: 1, Category: "bug", Title: "x", Body: "y"}}},
		{Summary: "B", Findings: []Finding{{File: "a.go", Line: 1, Category: "bug", Title: "x", Body: "y"}}},
	}
	merged := mergeResults(results, 2)
	if merged.Usage != nil {
		t.Error("expected usage to be nil when no results have usage")
	}
}

func TestMergeResults_SummaryMerge(t *testing.T) {
	merged := mergeResults([]*ReviewResult{{Summary: "A."}, {Summary: "B."}, {Summary: "A."}}, 1)
	if merged.Summary != "A. B." {
		t.Errorf("got %q", merged.Summary)
	}
}

func TestMergeResults_HighThreshold(t *testing.T) {
	f := Finding{File: "a.go", Line: 1, Category: "bug", Body: "x"}
	merged := mergeResults([]*ReviewResult{{Findings: []Finding{f}}, {Findings: []Finding{f}}}, 3)
	if len(merged.Findings) != 0 {
		t.Errorf("expected 0 with threshold=3, got %d", len(merged.Findings))
	}
}

func TestMergeResults_MultipleFindingsDedup(t *testing.T) {
	results := []*ReviewResult{
		{Findings: []Finding{
			{File: "a.go", Line: 10, Category: "bug", Body: "short"},
			{File: "b.go", Line: 20, Category: "security", Body: "short"},
		}},
		{Findings: []Finding{
			{File: "a.go", Line: 11, Category: "bug", Body: "longer description"},
			{File: "b.go", Line: 22, Category: "security", Body: "longer sec"},
			{File: "c.go", Line: 5, Category: "style", Body: "unique"},
		}},
	}
	merged := mergeResults(results, 2)
	if len(merged.Findings) != 2 {
		t.Fatalf("expected 2, got %d", len(merged.Findings))
	}
}

// REGRESSION: This test verifies that canonical drift does NOT cause
// transitive chaining. Lines 10 and 15 are 5 apart (>3), so they must
// NOT be merged even if an intermediate finding at line 12 exists.
func TestMergeResults_NoTransitiveDrift(t *testing.T) {
	results := []*ReviewResult{
		{Findings: []Finding{
			{File: "main.go", Line: 10, Category: "bug", Title: "a", Body: "short"},
		}},
		{Findings: []Finding{
			// Line 12 is within ±3 of 10, so it matches the first group.
			// It has a longer body, so it becomes canonical.
			// If the anchor drifts to 12, then line 15 (±3 of 12) would also match.
			{File: "main.go", Line: 12, Category: "bug", Title: "b", Body: "this is the longest body for the group"},
		}},
		{Findings: []Finding{
			// Line 15 is NOT within ±3 of anchor (10). Must NOT merge.
			{File: "main.go", Line: 15, Category: "bug", Title: "c", Body: "different issue"},
		}},
	}

	merged := mergeResults(results, 2)

	// Only the L10/L12 group should pass threshold=2.
	// L15 only appears once → filtered.
	if len(merged.Findings) != 1 {
		t.Fatalf("expected 1 finding (no transitive drift), got %d", len(merged.Findings))
	}
	if merged.Findings[0].Line != 12 {
		t.Errorf("expected canonical at line 12 (longest body), got line %d", merged.Findings[0].Line)
	}
}

// ---------------------------------------------------------------------------
// MultiProvider.Review — concurrent execution with mocks
// ---------------------------------------------------------------------------

func TestMultiProviderReview_Concurrent(t *testing.T) {
	m1 := &mockReviewer{result: &ReviewResult{
		Summary: "M1",
		Findings: []Finding{
			{File: "main.go", Line: 10, Category: "bug", Severity: "HIGH", Title: "issue", Body: "from m1"},
		},
	}}
	m2 := &mockReviewer{result: &ReviewResult{
		Summary: "M2",
		Findings: []Finding{
			{File: "main.go", Line: 11, Category: "bug", Severity: "HIGH", Title: "same issue", Body: "from m2 with longer body"},
		},
	}}

	mp := NewMultiProviderFromReviewers([]ReviewProvider{m1, m2}, 2)
	result, err := mp.Review(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if m1.callCount.Load() != 1 {
		t.Errorf("m1 called %d times, want 1", m1.callCount.Load())
	}
	if m2.callCount.Load() != 1 {
		t.Errorf("m2 called %d times, want 1", m2.callCount.Load())
	}
	if len(result.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result.Findings))
	}
	if result.Findings[0].Body != "from m2 with longer body" {
		t.Errorf("expected longest body, got %q", result.Findings[0].Body)
	}
}

func TestMultiProviderReview_PartialError(t *testing.T) {
	// 2 models, threshold 2: if 1 fails, threshold cannot be met and review fails.
	m1 := &mockReviewer{result: &ReviewResult{Summary: "ok", Findings: nil}}
	m2 := &mockReviewer{err: fmt.Errorf("model overloaded")}

	mp := NewMultiProviderFromReviewers([]ReviewProvider{m1, m2}, 2)
	_, err := mp.Review(context.Background(), "sys", "user")
	if err == nil {
		t.Fatal("expected error when threshold cannot be met, got nil")
	}
	if m1.callCount.Load() == 0 && m2.callCount.Load() == 0 {
		t.Error("expected models to be called")
	}
}

func TestMultiProviderReview_ResilientToNonFatalFailure(t *testing.T) {
	// 3 models, threshold 2: if 1 model fails with a transient error,
	// the remaining 2 models still meet threshold and review succeeds.
	f := Finding{File: "main.go", Line: 10, Category: "bug", Severity: "HIGH", Title: "issue", Body: "desc"}
	m1 := &mockReviewer{result: &ReviewResult{Summary: "M1", Findings: []Finding{f}}}
	m2 := &mockReviewer{result: &ReviewResult{Summary: "M2", Findings: []Finding{f}}}
	m3 := &mockReviewer{err: fmt.Errorf("temporary 503 service unavailable")}

	mp := NewMultiProviderFromReviewers([]ReviewProvider{m1, m2, m3}, 2)
	result, err := mp.Review(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("expected review to succeed despite 1 provider failure, got: %v", err)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result.Findings))
	}
	if result.Findings[0].Title != "issue" {
		t.Errorf("expected finding title 'issue', got %s", result.Findings[0].Title)
	}
}

func TestMultiProviderReview_SingleModel(t *testing.T) {
	m := &mockReviewer{result: &ReviewResult{
		Summary:  "single",
		Findings: []Finding{{File: "a.go", Line: 1, Category: "bug", Body: "x"}},
	}}

	mp := NewMultiProviderFromReviewers([]ReviewProvider{m}, 1)
	result, err := mp.Review(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(result.Findings))
	}
}

// ---------------------------------------------------------------------------
// MultiProvider.Close
// ---------------------------------------------------------------------------

func TestMultiProviderClose(t *testing.T) {
	m1 := &mockReviewer{}
	m2 := &mockReviewer{}
	mp := NewMultiProviderFromReviewers([]ReviewProvider{m1, m2}, 2)
	mp.Close()
	if m1.closed.Load() != 1 {
		t.Error("m1 not closed")
	}
	if m2.closed.Load() != 1 {
		t.Error("m2 not closed")
	}
}
