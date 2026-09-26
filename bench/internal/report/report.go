package report

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/OpticDiff/code-reviewer/bench/internal/scorer"
)

func PrintTable(w io.Writer, r *scorer.Report) {
	fmt.Fprintf(w, "\nOVERALL RESULTS\n")
	fmt.Fprintf(w, "===============\n")
	fmt.Fprintf(w, "True Positives:  %d\n", r.Overall.TruePositives)
	fmt.Fprintf(w, "False Negatives: %d\n", r.Overall.FalseNegatives)
	fmt.Fprintf(w, "False Positives: %d\n", r.Overall.FalsePositives)
	fmt.Fprintf(w, "Noise:           %d\n", r.Overall.Noise)
	fmt.Fprintf(w, "Precision:       %.2f\n", r.Overall.Precision)
	fmt.Fprintf(w, "Recall:          %.2f\n", r.Overall.Recall)
	fmt.Fprintf(w, "F1 Score:        %.2f\n\n", r.Overall.F1)

	if len(r.ByCategory) > 0 {
		fmt.Fprintf(w, "BY CATEGORY\n")
		fmt.Fprintf(w, "===========\n")
		for cat, score := range r.ByCategory {
			fmt.Fprintf(w, "%-15s | P: %.2f | R: %.2f | F1: %.2f | TP: %d, FN: %d, FP: %d, N: %d\n",
				cat, score.Precision, score.Recall, score.F1,
				score.TruePositives, score.FalseNegatives, score.FalsePositives, score.Noise)
		}
		fmt.Fprintf(w, "\n")
	}

	fmt.Fprintf(w, "CASE DETAILS\n")
	fmt.Fprintf(w, "============\n")
	for _, c := range r.Cases {
		fmt.Fprintf(w, "- %s: TP:%d FN:%d FP:%d (F1: %.2f)\n", c.CaseName, c.Score.TruePositives, c.Score.FalseNegatives, c.Score.FalsePositives, c.Score.F1)
		if len(c.Missed) > 0 {
			fmt.Fprintf(w, "    Missed: %s\n", strings.Join(c.Missed, ", "))
		}
		if len(c.FalsePos) > 0 {
			fmt.Fprintf(w, "    False Pos: %s\n", strings.Join(c.FalsePos, ", "))
		}
	}
}

func WriteJSON(w io.Writer, r *scorer.Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

func WriteMarkdown(w io.Writer, r *scorer.Report) error {
	fmt.Fprintf(w, "# AACR-Bench Report\n\n")
	
	fmt.Fprintf(w, "## Overall Metrics\n\n")
	fmt.Fprintf(w, "| Metric | Value |\n")
	fmt.Fprintf(w, "|--------|-------|\n")
	fmt.Fprintf(w, "| True Positives | %d |\n", r.Overall.TruePositives)
	fmt.Fprintf(w, "| False Negatives | %d |\n", r.Overall.FalseNegatives)
	fmt.Fprintf(w, "| False Positives | %d |\n", r.Overall.FalsePositives)
	fmt.Fprintf(w, "| Noise | %d |\n", r.Overall.Noise)
	fmt.Fprintf(w, "| **Precision** | **%.2f** |\n", r.Overall.Precision)
	fmt.Fprintf(w, "| **Recall** | **%.2f** |\n", r.Overall.Recall)
	fmt.Fprintf(w, "| **F1 Score** | **%.2f** |\n\n", r.Overall.F1)

	var categories []string
	for cat := range r.ByCategory {
		categories = append(categories, cat)
	}
	sort.Strings(categories)

	if len(categories) > 0 {
		fmt.Fprintf(w, "## By Category\n\n")
		fmt.Fprintf(w, "| Category | Precision | Recall | F1 Score | TP | FN | FP | Noise |\n")
		fmt.Fprintf(w, "|----------|-----------|--------|----------|---|---|---|---|\n")
		for _, cat := range categories {
			score := r.ByCategory[cat]
			fmt.Fprintf(w, "| %s | %.2f | %.2f | %.2f | %d | %d | %d | %d |\n",
				cat, score.Precision, score.Recall, score.F1,
				score.TruePositives, score.FalseNegatives, score.FalsePositives, score.Noise)
		}
		fmt.Fprintf(w, "\n")
	}

	return nil
}
