package reviewer

import (
	"context"

	"github.com/OpticDiff/code-reviewer/internal/gitlab"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

// ModelReviewer abstracts AI model interactions for testability.
type ModelReviewer interface {
	Review(ctx context.Context, systemPrompt, userPrompt string) (*model.ReviewResult, error)
	Close()
}

// VCSClient abstracts version control platform API operations for testability.
type VCSClient interface {
	GetMRChanges(ctx context.Context, projectID, mrIID string) (*gitlab.MRChangesResponse, error)
	GetMRVersions(ctx context.Context, projectID, mrIID string) ([]gitlab.DiffVersion, error)
	CompareCommits(ctx context.Context, projectID, from, to string) ([]string, error)
	PostNote(ctx context.Context, projectID, mrIID, body string) (*gitlab.Note, error)
	CreateDiscussion(ctx context.Context, projectID, mrIID string, req gitlab.CreateDiscussionRequest) error
	ListBotNotes(ctx context.Context, projectID, mrIID string) ([]gitlab.Note, error)
	DeleteNote(ctx context.Context, projectID, mrIID string, noteID int) error
	CleanPreviousReviews(ctx context.Context, projectID, mrIID string) (int, error)
}
