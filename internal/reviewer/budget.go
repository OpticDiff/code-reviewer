package reviewer

import (
	"fmt"
	"log/slog"

	"github.com/OpticDiff/code-reviewer/internal/diff"
)

// CostEstimate holds pre-flight token estimates for a review run.
type CostEstimate struct {
	InputTokens    int // Estimated input tokens from diff content.
	OutputEstimate int // Estimated output tokens (~25% of input for review tasks).
	TotalEstimate  int // InputTokens + OutputEstimate.
	FileCount      int // Number of files in the review.
}

// EstimateCost returns a pre-flight cost estimate without calling the model.
// Output tokens are estimated at ~25% of input tokens based on observed
// review task ratios.
func EstimateCost(diffs []diff.FileDiff) CostEstimate {
	input := diff.EstimateTokens(diffs)
	output := int(float64(input) * 0.25)
	return CostEstimate{
		InputTokens:    input,
		OutputEstimate: output,
		TotalEstimate:  input + output,
		FileCount:      len(diffs),
	}
}

// TrimToBudget removes lowest-priority files from the end of the slice
// until the estimated total tokens fits within maxTokens.
// Assumes diffs are already sorted by priority (highest first).
// Returns the trimmed slice and the list of skipped file paths.
func TrimToBudget(diffs []diff.FileDiff, maxTokens int) ([]diff.FileDiff, []string) {
	if maxTokens <= 0 {
		return diffs, nil
	}

	est := EstimateCost(diffs)
	if est.TotalEstimate <= maxTokens {
		return diffs, nil
	}

	// Remove files from the end (lowest priority) until we're within budget.
	var skipped []string
	trimmed := make([]diff.FileDiff, len(diffs))
	copy(trimmed, diffs)

	for len(trimmed) > 0 && EstimateCost(trimmed).TotalEstimate > maxTokens {
		last := trimmed[len(trimmed)-1]
		skipped = append(skipped, last.NewPath)
		trimmed = trimmed[:len(trimmed)-1]
	}

	return trimmed, skipped
}

// LogBudgetStatus logs pre-flight budget information.
func LogBudgetStatus(est CostEstimate, maxTokens int, skippedFiles []string) {
	if maxTokens <= 0 {
		slog.Info("pre-flight estimate",
			"estimated_tokens", est.TotalEstimate,
			"files", est.FileCount)
		return
	}

	pct := 0
	if maxTokens > 0 {
		pct = int(float64(est.TotalEstimate) / float64(maxTokens) * 100)
	}

	if len(skippedFiles) > 0 {
		slog.Warn("token budget exceeded, reviewing highest-priority files",
			"estimated_tokens", est.TotalEstimate,
			"budget", maxTokens,
			"budget_pct", fmt.Sprintf("%d%%", pct),
			"files_reviewing", est.FileCount,
			"files_skipped", len(skippedFiles))
	} else {
		slog.Info("pre-flight estimate",
			"estimated_tokens", est.TotalEstimate,
			"budget", maxTokens,
			"budget_pct", fmt.Sprintf("%d%%", pct),
			"files", est.FileCount)
	}
}
