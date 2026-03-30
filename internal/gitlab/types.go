// Package gitlab provides a client for the GitLab REST API v4,
// focused on merge request operations needed for code review.
package gitlab

import "time"

// MRChangesResponse is the response from GET /projects/:id/merge_requests/:iid/changes.
type MRChangesResponse struct {
	ID          int         `json:"id"`
	IID         int         `json:"iid"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	State       string      `json:"state"`
	Draft       bool        `json:"draft"`
	Changes     []DiffEntry `json:"changes"`
}

// DiffEntry represents a single file change in an MR.
type DiffEntry struct {
	OldPath     string `json:"old_path"`
	NewPath     string `json:"new_path"`
	Diff        string `json:"diff"`
	NewFile     bool   `json:"new_file"`
	RenamedFile bool   `json:"renamed_file"`
	DeletedFile bool   `json:"deleted_file"`
}

// DiffVersion represents a version of the MR diff (from the versions API).
type DiffVersion struct {
	ID        int       `json:"id"`
	HeadSHA   string    `json:"head_commit_sha"`
	BaseSHA   string    `json:"base_commit_sha"`
	StartSHA  string    `json:"start_commit_sha"`
	CreatedAt time.Time `json:"created_at"`
}

// Note represents a comment on an MR.
type Note struct {
	ID        int       `json:"id"`
	Body      string    `json:"body"`
	Author    Author    `json:"author"`
	System    bool      `json:"system"`
	CreatedAt time.Time `json:"created_at"`
}

// Author represents a GitLab user.
type Author struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

// DiscussionPosition specifies where an inline comment should be anchored in the diff.
type DiscussionPosition struct {
	PositionType string `json:"position_type"`
	BaseSHA      string `json:"base_sha"`
	HeadSHA      string `json:"head_sha"`
	StartSHA     string `json:"start_sha"`
	OldPath      string `json:"old_path,omitempty"`
	NewPath      string `json:"new_path"`
	OldLine      *int   `json:"old_line,omitempty"`
	NewLine      *int   `json:"new_line,omitempty"`
}

// CreateDiscussionRequest is the request body for creating an inline discussion.
type CreateDiscussionRequest struct {
	Body     string              `json:"body"`
	Position *DiscussionPosition `json:"position,omitempty"`
}

// CreateNoteRequest is the request body for creating a simple MR note.
type CreateNoteRequest struct {
	Body string `json:"body"`
}
