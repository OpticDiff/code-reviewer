package reviewer

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/OpticDiff/code-reviewer/internal/model"
)

// severityRank returns a numeric rank for severity comparison.
// Higher rank = more severe.
func severityRank(sev string) int {
	switch strings.ToUpper(strings.TrimSpace(sev)) {
	case "CRITICAL":
		return 4
	case "HIGH":
		return 3
	case "MEDIUM":
		return 2
	case "LOW":
		return 1
	default:
		return 0
	}
}

// DeduplicateFindings collapses duplicate findings from multi-chunk reviews.
// Two findings match if they share the same file, category, and are within
// 3 lines of each other (using model.FindingsMatch). When duplicates are
// found, the higher-severity, more-detailed finding is kept.
func DeduplicateFindings(findings []model.Finding) []model.Finding {
	if len(findings) <= 1 {
		return findings
	}

	type group struct {
		anchor    model.Finding // Fixed comparison point.
		canonical model.Finding // Best finding to display.
	}

	var groups []*group

	for _, f := range findings {
		matched := false
		for _, g := range groups {
			if model.FindingsMatch(f, g.anchor) {
				// Keep the better finding as canonical.
				if isBetterFinding(f, g.canonical) {
					g.canonical = f
				}
				matched = true
				break
			}
		}
		if !matched {
			groups = append(groups, &group{
				anchor:    f,
				canonical: f,
			})
		}
	}

	result := make([]model.Finding, 0, len(groups))
	for _, g := range groups {
		result = append(result, g.canonical)
	}

	removed := len(findings) - len(result)
	if removed > 0 {
		slog.Info(fmt.Sprintf("deduplicated findings: %d → %d (removed %d duplicate(s))",
			len(findings), len(result), removed))
	}

	return result
}

// isBetterFinding returns true if candidate is a better finding than current.
// Prefers higher severity, then longer body, then presence of suggestion.
func isBetterFinding(candidate, current model.Finding) bool {
	candRank := severityRank(candidate.Severity)
	currRank := severityRank(current.Severity)

	if candRank != currRank {
		return candRank > currRank
	}
	if len(candidate.Body) != len(current.Body) {
		return len(candidate.Body) > len(current.Body)
	}
	// Prefer the one with a suggestion.
	if candidate.Suggestion != "" && current.Suggestion == "" {
		return true
	}
	// Prefer the one with a more specific EndLine.
	if candidate.EndLine > 0 && current.EndLine == 0 {
		return true
	}
	return false
}
