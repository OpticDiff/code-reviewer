package reviewer

import (
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/config"
)

func TestEvaluateAutoApprove(t *testing.T) {
	baseCfg := func() *config.Config {
		return &config.Config{
			AutoApprove: true,
			CIMode:      true,
		}
	}

	tests := []struct {
		name           string
		cfg            *config.Config
		filesReviewed  int
		findingsCount  int
		skippedFiles   []string
		budgetExceeded bool
		scopeOversized bool
		truncated      bool
		isDraft        bool
		wantApproved   bool
		wantReason     string
	}{
		{
			name:          "clean review approved",
			cfg:           baseCfg(),
			filesReviewed: 5,
			wantApproved:  true,
			wantReason:    "0 findings across 5 file(s), all safety guards passed",
		},
		{
			name:          "not enabled",
			cfg:           &config.Config{CIMode: true},
			filesReviewed: 5,
			wantApproved:  false,
			wantReason:    "auto-approve not enabled",
		},
		{
			name:          "not CI mode",
			cfg:           &config.Config{AutoApprove: true, DiffMode: true},
			filesReviewed: 5,
			wantApproved:  false,
			wantReason:    "not in CI mode or dry-run",
		},
		{
			name:          "dry run",
			cfg:           &config.Config{AutoApprove: true, CIMode: true, DryRun: true},
			filesReviewed: 5,
			wantApproved:  false,
			wantReason:    "not in CI mode or dry-run",
		},
		{
			name:          "draft MR",
			cfg:           baseCfg(),
			filesReviewed: 5,
			isDraft:       true,
			wantApproved:  false,
			wantReason:    "MR/PR is in draft state",
		},
		{
			name:          "has findings",
			cfg:           baseCfg(),
			filesReviewed: 5,
			findingsCount: 3,
			wantApproved:  false,
			wantReason:    "3 finding(s) detected",
		},
		{
			name:         "zero files reviewed",
			cfg:          baseCfg(),
			wantApproved: false,
			wantReason:   "no files were reviewed",
		},
		{
			name:          "skipped files",
			cfg:           baseCfg(),
			filesReviewed: 3,
			skippedFiles:  []string{"big.go", "huge.go"},
			wantApproved:  false,
			wantReason:    "2 file(s) not reviewed (budget trim or parse failure)",
		},
		{
			name:           "budget exceeded",
			cfg:            baseCfg(),
			filesReviewed:  5,
			budgetExceeded: true,
			wantApproved:   false,
			wantReason:     "runtime token budget exceeded before all chunks reviewed",
		},
		{
			name:           "scope oversized",
			cfg:            baseCfg(),
			filesReviewed:  5,
			scopeOversized: true,
			wantApproved:   false,
			wantReason:     "MR/PR exceeds scope limit",
		},
		{
			name:          "truncated response",
			cfg:           baseCfg(),
			filesReviewed: 5,
			truncated:     true,
			wantApproved:  false,
			wantReason:    "model response was truncated (token limit hit)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := EvaluateAutoApprove(
				tt.cfg, tt.filesReviewed, tt.findingsCount,
				tt.skippedFiles, tt.budgetExceeded, tt.scopeOversized,
				tt.truncated, tt.isDraft,
			)
			if decision.Approved != tt.wantApproved {
				t.Errorf("Approved = %v, want %v", decision.Approved, tt.wantApproved)
			}
			if decision.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", decision.Reason, tt.wantReason)
			}
		})
	}
}
