// Package gitlab provides a client for the GitLab REST API v4,
// focused on merge request operations needed for code review.
package gitlab

import (
	"time"

	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

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

// toVCS converts a GitLab MR response to the platform-agnostic type.
func (r *MRChangesResponse) toVCS() *vcs.MRChanges {
	changes := make([]vcs.DiffEntry, len(r.Changes))
	for i, c := range r.Changes {
		changes[i] = vcs.DiffEntry{
			OldPath:     c.OldPath,
			NewPath:     c.NewPath,
			Diff:        c.Diff,
			NewFile:     c.NewFile,
			RenamedFile: c.RenamedFile,
			DeletedFile: c.DeletedFile,
		}
	}
	return &vcs.MRChanges{
		ID:          r.ID,
		IID:         r.IID,
		Title:       r.Title,
		Description: r.Description,
		State:       r.State,
		Draft:       r.Draft,
		Changes:     changes,
	}
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

// toVCS converts a GitLab diff version to the platform-agnostic type.
func (v *DiffVersion) toVCS() vcs.DiffVersion {
	return vcs.DiffVersion{
		ID:        v.ID,
		HeadSHA:   v.HeadSHA,
		BaseSHA:   v.BaseSHA,
		StartSHA:  v.StartSHA,
		CreatedAt: v.CreatedAt,
	}
}

// Note represents a comment on an MR.
type Note struct {
	ID        int                 `json:"id"`
	Body      string              `json:"body"`
	Author    Author              `json:"author"`
	System    bool                `json:"system"`
	CreatedAt time.Time           `json:"created_at"`
	Position  *DiscussionPosition `json:"position,omitempty"`
}

// toVCS converts a GitLab note to the platform-agnostic type.
func (n *Note) toVCS() *vcs.Comment {
	return &vcs.Comment{
		ID:        n.ID,
		Body:      n.Body,
		Author:    n.Author.Username,
		System:    n.System,
		CreatedAt: n.CreatedAt,
	}
}

// Author represents a GitLab user.
type Author struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

type DiscussionLineRef struct {
	NewLine int    `json:"new_line"`
	Type    string `json:"type"` // "new"
}

type DiscussionLineRange struct {
	Start DiscussionLineRef `json:"start"`
	End   DiscussionLineRef `json:"end"`
}

// DiscussionPosition specifies where an inline comment should be anchored in the diff.
type DiscussionPosition struct {
	PositionType string `json:"position_type"`
	BaseSHA      string `json:"base_sha"`
	HeadSHA      string `json:"head_sha"`
	StartSHA     string `json:"start_sha"`
	OldPath      string `json:"old_path,omitempty"`
	NewPath      string `json:"new_path"`
	OldLine      *int                 `json:"old_line,omitempty"`
	NewLine      *int                 `json:"new_line,omitempty"`
	LineRange    *DiscussionLineRange `json:"line_range,omitempty"`
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

// DraftNote is an unpublished review comment from GET /draft_notes.
type DraftNote struct {
	ID       int                 `json:"id"`
	Note     string              `json:"note"`
	Position *DiscussionPosition `json:"position,omitempty"`
}

// CreateDraftNoteRequest is the request body for POST /draft_notes.
type CreateDraftNoteRequest struct {
	Note     string              `json:"note"`
	Position *DiscussionPosition `json:"position,omitempty"`
}

