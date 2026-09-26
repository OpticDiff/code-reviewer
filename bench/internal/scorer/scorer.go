package scorer

import (
	"math"
	"strings"

	"github.com/OpticDiff/code-reviewer/bench/internal/dataset"
	"github.com/OpticDiff/code-reviewer/bench/internal/runner"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

type Score struct {
	TruePositives  int
	FalseNegatives int
	FalsePositives int
	Noise          int // findings matching neither expected nor false_positive
	Precision      float64
	Recall         float64
	F1             float64
}

type Report struct {
	Overall    Score
	ByLanguage map[string]Score
	ByCategory map[string]Score
	Cases      []CaseScore
	Errored    []ErroredCase
}

type ErroredCase struct {
	CaseName string
	Error    string
}

type CaseScore struct {
	CaseName string
	Score    Score
	Matched  []string // titles of matched expected findings
	Missed   []string // titles of missed expected findings
	FalsePos []string // titles of false positive findings
}

func BenchMatch(expected dataset.ExpectedFinding, actual model.Finding) bool {
	if expected.File != actual.File {
		return false
	}
	if expected.Category != "" && !strings.EqualFold(expected.Category, actual.Category) {
		return false
	}
	if math.Abs(float64(expected.Line-actual.Line)) > 5 {
		return false
	}
	return true
}

func MatchFalsePositive(expected dataset.FalsePositive, actual model.Finding) bool {
	if expected.File != actual.File {
		return false
	}
	if expected.Category != "" && !strings.EqualFold(expected.Category, actual.Category) {
		return false
	}
	if math.Abs(float64(expected.Line-actual.Line)) > 5 {
		return false
	}
	return true
}

func ScoreCase(result runner.CaseResult) CaseScore {
	cScore := CaseScore{
		CaseName: result.Case.Name,
		Score:    Score{},
		Matched:  []string{},
		Missed:   []string{},
		FalsePos: []string{},
	}

	actualFindings := make([]model.Finding, len(result.ActualFindings))
	copy(actualFindings, result.ActualFindings)

	// Check for true positives (expected findings)
	for _, expected := range result.Case.Expected.Findings {
		matched := false
		for i, actual := range actualFindings {
			if BenchMatch(expected, actual) {
				cScore.Score.TruePositives++
				cScore.Matched = append(cScore.Matched, expected.Title)
				matched = true
				// Remove matched finding
				actualFindings = append(actualFindings[:i], actualFindings[i+1:]...)
				break
			}
		}
		if !matched {
			cScore.Score.FalseNegatives++
			cScore.Missed = append(cScore.Missed, expected.Title)
		}
	}

	// Check remaining actual findings for false positives or noise
	for _, actual := range actualFindings {
		isFalsePositive := false
		for _, fp := range result.Case.Expected.FalsePositives {
			if MatchFalsePositive(fp, actual) {
				cScore.Score.FalsePositives++
				cScore.FalsePos = append(cScore.FalsePos, actual.Title)
				isFalsePositive = true
				break
			}
		}
		if !isFalsePositive {
			cScore.Score.Noise++
		}
	}

	cScore.Score = calculateDerived(cScore.Score)
	return cScore
}

func ScoreResults(results []runner.CaseResult) *Report {
	report := &Report{
		ByLanguage: make(map[string]Score),
		ByCategory: make(map[string]Score),
		Cases:      make([]CaseScore, 0, len(results)),
	}

	for _, res := range results {
		if res.Error != nil {
			report.Errored = append(report.Errored, ErroredCase{
				CaseName: res.Case.Name,
				Error:    res.Error.Error(),
			})
			continue
		}
		cScore := ScoreCase(res)
		report.Cases = append(report.Cases, cScore)
		report.Overall = addScores(report.Overall, cScore.Score)
		
		langScore := report.ByLanguage[res.Case.Language]
		report.ByLanguage[res.Case.Language] = addScores(langScore, cScore.Score)

		catScore := report.ByCategory[res.Case.Category]
		report.ByCategory[res.Case.Category] = addScores(catScore, cScore.Score)
	}

	report.Overall = calculateDerived(report.Overall)
	for k, v := range report.ByLanguage {
		report.ByLanguage[k] = calculateDerived(v)
	}
	for k, v := range report.ByCategory {
		report.ByCategory[k] = calculateDerived(v)
	}

	return report
}

func addScores(a, b Score) Score {
	return Score{
		TruePositives:  a.TruePositives + b.TruePositives,
		FalseNegatives: a.FalseNegatives + b.FalseNegatives,
		FalsePositives: a.FalsePositives + b.FalsePositives,
		Noise:          a.Noise + b.Noise,
	}
}

func calculateDerived(s Score) Score {
	if s.TruePositives+s.FalsePositives > 0 {
		s.Precision = float64(s.TruePositives) / float64(s.TruePositives+s.FalsePositives)
	}
	if s.TruePositives+s.FalseNegatives > 0 {
		s.Recall = float64(s.TruePositives) / float64(s.TruePositives+s.FalseNegatives)
	}
	if s.Precision+s.Recall > 0 {
		s.F1 = 2 * (s.Precision * s.Recall) / (s.Precision + s.Recall)
	}
	return s
}
