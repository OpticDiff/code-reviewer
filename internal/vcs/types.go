// Package vcs defines platform-agnostic types for version control system
// operations. These types decouple the reviewer engine from specific VCS
// platforms (GitLab, GitHub, Bitbucket), enabling multi-platform support.
package vcs

import "time"

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
}

// InlineCommentRequest is the payload for creating an inline
// diff-anchored comment (GitLab discussion, GitHub review comment, etc.).
type InlineCommentRequest struct {
	Body     string
	Position *InlineCommentPosition
}
