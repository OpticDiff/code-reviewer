package reviewer

import (
	"context"

	"github.com/OpticDiff/code-reviewer/internal/model"
	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

// ModelReviewer abstracts AI model interactions for testability.
type ModelReviewer interface {
	Review(ctx context.Context, systemPrompt, userPrompt string) (*model.ReviewResult, error)
	Close()
}

// VCSClient abstracts version control platform API operations for testability.
// Implementations exist for GitLab (internal/gitlab) and GitHub (internal/github).
type VCSClient interface {
	vcs.VCSProvider
}
