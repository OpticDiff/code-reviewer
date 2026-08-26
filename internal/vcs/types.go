// Package vcs defines platform-agnostic types for version control system
// operations. These types decouple the reviewer engine from specific VCS
// platforms (GitLab, GitHub, Bitbucket), enabling multi-platform support.
package vcs

import (
	"context"
	"time"
)

// MRChanges holds the file changes from a merge/pull request.
type MRChanges struct {
	ID          int
	IID         int
	Title       string
	Description string
	State       string
	Draft       bool
	Changes     []DiffEntry
}

// DiffEntry represents a single file change in a merge/pull request.
type DiffEntry struct {
	OldPath     string
	NewPath     string
	Diff        string
	NewFile     bool
	RenamedFile bool
	DeletedFile bool
}

// DiffVersion represents a point-in-time snapshot of the MR/PR diff,
// used for incremental review and inline comment positioning.
type DiffVersion struct {
	ID        int
	HeadSHA   string
	BaseSHA   string
	StartSHA  string
	CreatedAt time.Time
}

// Comment represents a comment on a merge/pull request.
type Comment struct {
	ID        int
	Body      string
	Author    string
	System    bool
	CreatedAt time.Time
}

// InlineCommentPosition specifies where an inline comment should be
// anchored in the diff.
type InlineCommentPosition struct {
	BaseSHA  string
	HeadSHA  string
	StartSHA string
	OldPath  string
	NewPath  string
	OldLine  *int
	NewLine  *int
	EndLine  *int
}

// InlineCommentRequest is the payload for creating an inline
// diff-anchored comment (GitLab discussion, GitHub review comment, etc.).
type InlineCommentRequest struct {
	Body     string
	Position *InlineCommentPosition
}

// ReviewComment is a pre-formatted inline comment for a batched review
// submission. Unlike InlineCommentRequest, it carries only the essential
// positioning info — the platform client handles SHA context internally.
type ReviewComment struct {
	Path       string // File path relative to repo root.
	Line       int    // Start line (or single line).
	EndLine    int    // End line (0 means single-line comment).
	Body       string // Pre-formatted markdown body.
	Suggestion string // Raw replacement code (empty if no suggestion).
}

// SubmitReviewRequest is the payload for submitting a complete code review
// as a single atomic operation. On GitHub this maps to a single API call
// (POST /pulls/{pr}/reviews). On GitLab it maps to PostNote + N×CreateDiscussion.
type SubmitReviewRequest struct {
	Summary      string          // Top-level review body (markdown).
	Comments     []ReviewComment // Inline comments anchored to diff positions.
	Version      *DiffVersion    // SHA context for inline comment positioning (may be nil).
	CleanupMode  string          // "delete" or "resolve" — controls how old bot comments are handled.
	ChangedFiles []string        // If set, only clean comments referencing these files (incremental mode).
}

// DescriptionUpdater is implemented by VCS clients that support updating
// MR/PR descriptions. This is a separate interface for interface segregation.
type DescriptionUpdater interface {
	GetDescription(ctx context.Context, projectID, mrIID string) (string, error)
	SetDescription(ctx context.Context, projectID, mrIID, description string) error
}

// VCSApprover is implemented by VCS clients that support approving MRs/PRs.
// This is a separate interface for interface segregation — not all platforms
// or token scopes may support approvals.
// headSHA pins the approval to the reviewed revision. On GitLab, a mismatched
// SHA returns 409 Conflict. On GitHub, it sets commit_id on the review.
type VCSApprover interface {
	ApproveReview(ctx context.Context, projectID, reviewID, headSHA string) error
}
