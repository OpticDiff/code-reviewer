package vcs

import "context"

// DiffFetcher retrieves diffs from a VCS platform.
type DiffFetcher interface {
	GetMRChanges(ctx context.Context, projectID, mrIID string) (*MRChanges, error)
	GetMRVersions(ctx context.Context, projectID, mrIID string) ([]DiffVersion, error)
	CompareCommits(ctx context.Context, projectID, from, to string) ([]string, error)
}

// NotePoster posts review comments/notes.
type NotePoster interface {
	PostNote(ctx context.Context, projectID, mrIID, body string) (*Comment, error)
	CreateDiscussion(ctx context.Context, projectID, mrIID string, req InlineCommentRequest) error
	ListBotNotes(ctx context.Context, projectID, mrIID string) ([]Comment, error)
	DeleteNote(ctx context.Context, projectID, mrIID string, noteID int) error
	CleanPreviousReviews(ctx context.Context, projectID, mrIID string, changedFiles []string) (int, error)
	SubmitReview(ctx context.Context, projectID, mrIID string, req SubmitReviewRequest) error
}

// Approver manages merge request approval.
type Approver interface {
	ApproveReview(ctx context.Context, projectID, reviewID, headSHA string) error
}

// DescriptionUpdater updates MR/PR descriptions.
type DescriptionUpdater interface {
	GetDescription(ctx context.Context, projectID, mrIID string) (string, error)
	SetDescription(ctx context.Context, projectID, mrIID, description string) error
}

// VCSProvider combines all VCS capabilities. Existing clients implement this.
type VCSProvider interface {
	DiffFetcher
	NotePoster
	Approver
	DescriptionUpdater
	SetProfile(profile string)
}
