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
	GetMRChanges(ctx context.Context, projectID, mrIID string) (*vcs.MRChanges, error)
	GetMRVersions(ctx context.Context, projectID, mrIID string) ([]vcs.DiffVersion, error)
	CompareCommits(ctx context.Context, projectID, from, to string) ([]string, error)
	PostNote(ctx context.Context, projectID, mrIID, body string) (*vcs.Comment, error)
	CreateDiscussion(ctx context.Context, projectID, mrIID string, req vcs.InlineCommentRequest) error
	ListBotNotes(ctx context.Context, projectID, mrIID string) ([]vcs.Comment, error)
	DeleteNote(ctx context.Context, projectID, mrIID string, noteID int) error
	CleanPreviousReviews(ctx context.Context, projectID, mrIID string) (int, error)
	// SubmitReview posts a complete code review as a single atomic operation.
	// On GitHub: single POST /pulls/{pr}/reviews (1 API call, 1 notification).
	// On GitLab: CleanPreviousReviews + PostNote(summary) + N×CreateDiscussion.
	SubmitReview(ctx context.Context, projectID, mrIID string, req vcs.SubmitReviewRequest) error
}
