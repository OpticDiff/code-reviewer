package reviewer

import (
	"fmt"

	"github.com/OpticDiff/code-reviewer/internal/config"
)

// ApprovalDecision captures whether auto-approval is safe and why/why not.
type ApprovalDecision struct {
	Approved bool
	Reason   string
}

// EvaluateAutoApprove checks all safety guards for auto-approval.
// All guards must pass for the approval to be granted.
func EvaluateAutoApprove(
	cfg *config.Config,
	filesReviewed int,
	findingsCount int,
	skippedFiles []string,
	budgetExceeded bool,
	scopeOversized bool,
	truncated bool,
	isDraft bool,
) ApprovalDecision {
	if !cfg.AutoApprove {
		return ApprovalDecision{Reason: "auto-approve not enabled"}
	}
	if !cfg.CIMode || cfg.DryRun {
		return ApprovalDecision{Reason: "not in CI mode or dry-run"}
	}
	if isDraft {
		return ApprovalDecision{Reason: "MR/PR is in draft state"}
	}
	if findingsCount > 0 {
		return ApprovalDecision{Reason: fmt.Sprintf("%d finding(s) detected", findingsCount)}
	}
	if filesReviewed == 0 {
		return ApprovalDecision{Reason: "no files were reviewed"}
	}
	if len(skippedFiles) > 0 {
		return ApprovalDecision{Reason: fmt.Sprintf("%d file(s) skipped by token budget", len(skippedFiles))}
	}
	if budgetExceeded {
		return ApprovalDecision{Reason: "runtime token budget exceeded before all chunks reviewed"}
	}
	if scopeOversized {
		return ApprovalDecision{Reason: "MR/PR exceeds scope limit"}
	}
	if truncated {
		return ApprovalDecision{Reason: "model response was truncated (token limit hit)"}
	}
	return ApprovalDecision{
		Approved: true,
		Reason:   fmt.Sprintf("0 findings across %d file(s), all safety guards passed", filesReviewed),
	}
}
