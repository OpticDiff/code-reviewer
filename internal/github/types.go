package github

import (
	"time"
	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

const botMarker = "<!-- code-reviewer -->"

// PullFile is a file entry from GET /repos/{owner}/{repo}/pulls/{pr}/files
type PullFile struct {
	// SHA is the git object ID of the file.
	SHA              string `json:"sha"`
	// Filename is the path of the file.
	Filename         string `json:"filename"`
	// Status is the change status (e.g., added, removed, modified, renamed, copied).
	Status           string `json:"status"`
	// Patch is the unified diff patch for the file.
	Patch            string `json:"patch"`
	// PreviousFilename is the old path if the file was renamed.
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
	// Number is the PR number.
	Number int    `json:"number"`
	// Title is the title of the PR.
	Title  string `json:"title"`
	// Body is the description of the PR.
	Body   string `json:"body"`
	// State is the current state of the PR (e.g., open, closed).
	State  string `json:"state"`
	// Draft indicates if the PR is a draft.
	Draft  bool   `json:"draft"`
	// Head represents the source branch of the PR.
	Head   PRRef  `json:"head"`
	// Base represents the target branch of the PR.
	Base   PRRef  `json:"base"`
}

// PRRef is a branch reference in a PR.
type PRRef struct {
	// SHA is the commit SHA of the reference.
	SHA string `json:"sha"`
	// Ref is the branch name of the reference.
	Ref string `json:"ref"`
}

// IssueComment is a comment from GET /repos/{owner}/{repo}/issues/{pr}/comments
type IssueComment struct {
	// ID is the unique identifier of the comment.
	ID        int       `json:"id"`
	// Body is the text content of the comment.
	Body      string    `json:"body"`
	// User is the author of the comment.
	User      User      `json:"user"`
	// CreatedAt is the timestamp when the comment was created.
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
	// Login is the username of the user.
	Login string `json:"login"`
	// ID is the unique identifier of the user.
	ID    int    `json:"id"`
}

// CompareResponse is from GET /repos/{owner}/{repo}/compare/{base}...{head}
type CompareResponse struct {
	// Files is the list of files changed between the two commits.
	Files []PullFile `json:"files"`
}

// CreateReviewRequest is the payload for POST /repos/{owner}/{repo}/pulls/{pr}/reviews
type CreateReviewRequest struct {
	// CommitID pins the review to a specific commit, preventing 422 errors on force-push.
	CommitID string                 `json:"commit_id,omitempty"`
	// Event is the type of review action (e.g., COMMENT, APPROVE, REQUEST_CHANGES).
	Event    string                 `json:"event"`
	// Body is the main text of the review.
	Body     string                 `json:"body"`
	// Comments are the inline comments included in the review.
	Comments []ReviewCommentRequest `json:"comments,omitempty"`
}

// ReviewCommentRequest is a single inline comment within a review.
type ReviewCommentRequest struct {
	// Path is the file path for the comment.
	Path string `json:"path"`
	// Line is the line number for the comment.
	Line int    `json:"line"`
	// Body is the text content of the inline comment.
	Body string `json:"body"`
	// Side indicates which side of the diff the comment applies to (e.g., RIGHT for new file side).
	Side string `json:"side"`
}

// CreateIssueCommentRequest is for POST /repos/{owner}/{repo}/issues/{pr}/comments
type CreateIssueCommentRequest struct {
	// Body is the text content of the issue comment.
	Body string `json:"body"`
}

// CreatePullCommentRequest is for POST /repos/{owner}/{repo}/pulls/{pr}/comments (single inline)
type CreatePullCommentRequest struct {
	// Body is the text content of the inline comment.
	Body     string `json:"body"`
	// CommitID is the SHA of the commit the comment applies to.
	CommitID string `json:"commit_id"`
	// Path is the file path for the comment.
	Path     string `json:"path"`
	// Line is the line number for the comment.
	Line     int    `json:"line"`
	// Side indicates which side of the diff the comment applies to.
	Side     string `json:"side"`
}
