package reviewer

import (
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/diff"
)

func makeDiffs(count int, linesPerFile int) []diff.FileDiff {
	var diffs []diff.FileDiff
	for i := range count {
		lines := make([]diff.DiffLine, linesPerFile)
		for j := range linesPerFile {
			lines[j] = diff.DiffLine{
				Type:    diff.LineAdded,
				Content: "func example() { return nil }",
			}
			_ = j
		}
		diffs = append(diffs, diff.FileDiff{
			NewPath: "file" + string(rune('a'+i)) + ".go",
			Hunks:   []diff.Hunk{{Lines: lines}},
		})
	}
	return diffs
}

func TestEstimateCost(t *testing.T) {
	diffs := makeDiffs(3, 10)
	est := EstimateCost(diffs)

	if est.FileCount != 3 {
		t.Errorf("FileCount = %d, want 3", est.FileCount)
	}
	if est.InputTokens <= 0 {
		t.Error("InputTokens should be > 0")
	}
	if est.OutputEstimate <= 0 {
		t.Error("OutputEstimate should be > 0")
	}
	if est.TotalEstimate != est.InputTokens+est.OutputEstimate {
		t.Error("TotalEstimate should be InputTokens + OutputEstimate")
	}
	// Output should be ~25% of input.
	ratio := float64(est.OutputEstimate) / float64(est.InputTokens)
	if ratio < 0.24 || ratio > 0.26 {
		t.Errorf("output/input ratio = %.2f, expected ~0.25", ratio)
	}
}

func TestEstimateCost_Empty(t *testing.T) {
	est := EstimateCost(nil)
	if est.TotalEstimate != 0 {
		t.Errorf("empty diffs should have 0 estimate, got %d", est.TotalEstimate)
	}
	if est.FileCount != 0 {
		t.Errorf("empty diffs should have 0 files, got %d", est.FileCount)
	}
}

func TestTrimToBudget_WithinBudget(t *testing.T) {
	diffs := makeDiffs(3, 5)
	trimmed, skipped := TrimToBudget(diffs, 1_000_000)

	if len(trimmed) != 3 {
		t.Errorf("should not trim when within budget, got %d files", len(trimmed))
	}
	if len(skipped) != 0 {
		t.Errorf("should have no skipped files, got %d", len(skipped))
	}
}

func TestTrimToBudget_OverBudget(t *testing.T) {
	diffs := makeDiffs(5, 20)
	est := EstimateCost(diffs)

	// Set budget to ~60% of estimate — should trim some files.
	budget := int(float64(est.TotalEstimate) * 0.6)
	trimmed, skipped := TrimToBudget(diffs, budget)

	if len(trimmed) >= 5 {
		t.Errorf("should have trimmed some files, still have %d", len(trimmed))
	}
	if len(skipped) == 0 {
		t.Error("should have skipped files")
	}
	if len(trimmed)+len(skipped) != 5 {
		t.Errorf("trimmed (%d) + skipped (%d) should equal original (5)", len(trimmed), len(skipped))
	}

	// Trimmed result should fit within budget.
	trimmedEst := EstimateCost(trimmed)
	if trimmedEst.TotalEstimate > budget {
		t.Errorf("trimmed estimate (%d) exceeds budget (%d)", trimmedEst.TotalEstimate, budget)
	}
}

func TestTrimToBudget_Unlimited(t *testing.T) {
	diffs := makeDiffs(5, 20)
	trimmed, skipped := TrimToBudget(diffs, 0) // 0 = unlimited

	if len(trimmed) != 5 {
		t.Errorf("unlimited budget should not trim, got %d files", len(trimmed))
	}
	if len(skipped) != 0 {
		t.Errorf("unlimited budget should have no skipped files, got %d", len(skipped))
	}
}

func TestTrimToBudget_VeryTightBudget(t *testing.T) {
	diffs := makeDiffs(5, 20)
	trimmed, skipped := TrimToBudget(diffs, 1) // impossibly tight

	// Should trim down to 0 files.
	if len(trimmed) != 0 {
		t.Errorf("budget=1 should trim all files, got %d", len(trimmed))
	}
	if len(skipped) != 5 {
		t.Errorf("should skip all 5 files, got %d", len(skipped))
	}
}
