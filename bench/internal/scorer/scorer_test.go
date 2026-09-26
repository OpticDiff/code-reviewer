package scorer

import (
	"testing"

	"github.com/OpticDiff/code-reviewer/bench/internal/dataset"
	"github.com/OpticDiff/code-reviewer/bench/internal/runner"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

func TestScoreCase(t *testing.T) {
	expected := dataset.ExpectedResult{
		Findings: []dataset.ExpectedFinding{
			{File: "main.go", Line: 10, Category: "security", Title: "SQL Injection"},
			{File: "main.go", Line: 20, Category: "security", Title: "Hardcoded Creds"},
		},
		FalsePositives: []dataset.FalsePositive{
			{File: "test.go", Line: 15, Category: "security"},
		},
	}

	actuals := []model.Finding{
		{File: "main.go", Line: 11, Category: "security", Title: "SQL Injection found"}, // match finding 1
		{File: "test.go", Line: 15, Category: "security", Title: "Test error"},          // false positive
		{File: "other.go", Line: 5, Category: "style", Title: "Bad name"},               // noise
	}

	c := dataset.Case{
		Name:     "case1",
		Expected: expected,
	}

	res := runner.CaseResult{
		Case:           c,
		ActualFindings: actuals,
	}

	score := ScoreCase(res)

	if score.Score.TruePositives != 1 {
		t.Errorf("Expected 1 TP, got %d", score.Score.TruePositives)
	}
	if score.Score.FalseNegatives != 1 {
		t.Errorf("Expected 1 FN, got %d", score.Score.FalseNegatives)
	}
	if score.Score.FalsePositives != 1 {
		t.Errorf("Expected 1 FP, got %d", score.Score.FalsePositives)
	}
	if score.Score.Noise != 1 {
		t.Errorf("Expected 1 Noise, got %d", score.Score.Noise)
	}
	if score.Score.Precision != 0.5 { // 1 TP / (1 TP + 1 FP)
		t.Errorf("Expected Precision 0.5, got %f", score.Score.Precision)
	}
	if score.Score.Recall != 0.5 { // 1 TP / (1 TP + 1 FN)
		t.Errorf("Expected Recall 0.5, got %f", score.Score.Recall)
	}
}
