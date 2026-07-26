package github

import (
	"time"
	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

const botMarker = "<!-- code-reviewer -->"

// PullFile is a file entry from GET /repos/{owner}/{repo}/pulls/{pr}/files
type PullFile struct {
	SHA              string `json:"sha"`
	Filename         string `json:"filename"`
	Status           string `json:"status"` // added, removed, modified, renamed, copied
	Patch            string `json:"patch"`
	PreviousFilename string `json:"previous_filename,omitempty"`
}

func (f *PullFile) toVCSDiffEntry() vcs.DiffEntry {
	return vcs.DiffEntry{
		OldPath:     f.previousPath(),
		NewPath:     f.Filename,
		Diff:        f.Patch,
		NewFile:     f.Status == "added",
		RenamedFile: f.Status == "renamed",
		DeletedFile: f.Status == "removed",
	}
}

func (f *PullFile) previousPath() string {
	if f.PreviousFilename != "" {
		return f.PreviousFilename
	}
	return f.Filename
}

// PullRequest is the response from GET /repos/{owner}/{repo}/pulls/{pr}
type PullRequest struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	State  string `json:"state"`
	Draft  bool   `json:"draft"`
	Head   PRRef  `json:"head"`
	Base   PRRef  `json:"base"`
}

// PRRef is a branch reference in a PR.
type PRRef struct {
	SHA string `json:"sha"`
	Ref string `json:"ref"`
}

// IssueComment is a comment from GET /repos/{owner}/{repo}/issues/{pr}/comments
type IssueComment struct {
	ID        int       `json:"id"`
	Body      string    `json:"body"`
	User      User      `json:"user"`
	CreatedAt time.Time `json:"created_at"`
}

func (c *IssueComment) toVCS() *vcs.Comment {
	return &vcs.Comment{
		ID:        c.ID,
		Body:      c.Body,
		Author:    c.User.Login,
		System:    false,
		CreatedAt: c.CreatedAt,
	}
}

// User is a GitHub user.
type User struct {
	Login string `json:"login"`
	ID    int    `json:"id"`
}

// CompareResponse is from GET /repos/{owner}/{repo}/compare/{base}...{head}
type CompareResponse struct {
	Files []PullFile `json:"files"`
}

// CreateReviewRequest is the payload for POST /repos/{owner}/{repo}/pulls/{pr}/reviews
type CreateReviewRequest struct {
	CommitID string                 `json:"commit_id,omitempty"` // Pin to reviewed commit; prevents 422 on force-push.
	Event    string                 `json:"event"`
	Body     string                 `json:"body"`
	Comments []ReviewCommentRequest `json:"comments,omitempty"`
}

// ReviewCommentRequest is a single inline comment within a review.
type ReviewCommentRequest struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Body string `json:"body"`
	Side string `json:"side"` // RIGHT for new file side
}

// CreateIssueCommentRequest is for POST /repos/{owner}/{repo}/issues/{pr}/comments
type CreateIssueCommentRequest struct {
	Body string `json:"body"`
}

// CreatePullCommentRequest is for POST /repos/{owner}/{repo}/pulls/{pr}/comments (single inline)
type CreatePullCommentRequest struct {
	Body     string `json:"body"`
	CommitID string `json:"commit_id"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Side     string `json:"side"`
}
