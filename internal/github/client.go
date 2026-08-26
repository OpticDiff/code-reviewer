package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

const (
	apiRateDelay   = 100 * time.Millisecond
	maxRetries     = 3
	defaultRetryMs = 1000
)

// Client is an HTTP client for the GitHub REST API v3.
type Client struct {
	baseURL     string
	token       string
	httpClient  *http.Client
	retryBaseMs int // default retry delay in ms; 0 uses defaultRetryMs. Overridable for tests.
}

// NewClient creates a new GitHub API client.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// Reject cross-origin redirects to prevent SSRF.
				if len(via) > 0 && req.URL.Host != via[0].URL.Host {
					return fmt.Errorf("refusing cross-origin redirect from %s to %s", via[0].URL.Host, req.URL.Host)
				}
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
	}
}

// GetMRChanges fetches the file changes for a pull request.
func (c *Client) GetMRChanges(ctx context.Context, projectID, prNumber string) (*vcs.MRChanges, error) {
	prURL := fmt.Sprintf("%s/repos/%s/pulls/%s", c.baseURL, projectID, prNumber)
	var pr PullRequest
	if err := c.get(ctx, prURL, &pr); err != nil {
		return nil, fmt.Errorf("fetching PR metadata: %w", err)
	}

	filesURL := fmt.Sprintf("%s/repos/%s/pulls/%s/files?per_page=100", c.baseURL, projectID, prNumber)
	var files []PullFile
	if err := c.getPaginated(ctx, filesURL, func(raw json.RawMessage) error {
		var page []PullFile
		if err := json.Unmarshal(raw, &page); err != nil {
			return err
		}
		files = append(files, page...)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("fetching PR files: %w", err)
	}

	vcsChanges := make([]vcs.DiffEntry, len(files))
	for i, f := range files {
		vcsChanges[i] = f.toVCSDiffEntry()
	}

	prID, _ := strconv.Atoi(prNumber)

	return &vcs.MRChanges{
		ID:          pr.Number,
		IID:         prID,
		Title:       pr.Title,
		Description: pr.Body,
		State:       pr.State,
		Draft:       pr.Draft,
		Changes:     vcsChanges,
	}, nil
}

// GetMRVersions fetches the diff versions for a pull request.
func (c *Client) GetMRVersions(ctx context.Context, projectID, prNumber string) ([]vcs.DiffVersion, error) {
	url := fmt.Sprintf("%s/repos/%s/pulls/%s", c.baseURL, projectID, prNumber)
	var pr PullRequest
	if err := c.get(ctx, url, &pr); err != nil {
		return nil, fmt.Errorf("fetching PR metadata: %w", err)
	}

	return []vcs.DiffVersion{
		{
			ID:        1, // GitHub doesn't have version IDs like GitLab
			HeadSHA:   pr.Head.SHA,
			BaseSHA:   pr.Base.SHA,
			StartSHA:  pr.Base.SHA,
			CreatedAt: time.Now(),
		},
	}, nil
}

// GetDescription returns the current body of a pull request.
func (c *Client) GetDescription(ctx context.Context, owner, prNumber string) (string, error) {
	apiURL := fmt.Sprintf("%s/repos/%s/pulls/%s", c.baseURL, owner, prNumber)
	var pr struct {
		Body string `json:"body"`
	}
	if err := c.get(ctx, apiURL, &pr); err != nil {
		return "", fmt.Errorf("getting PR description: %w", err)
	}
	return pr.Body, nil
}

// SetDescription updates the body of a pull request.
func (c *Client) SetDescription(ctx context.Context, owner, prNumber, description string) error {
	apiURL := fmt.Sprintf("%s/repos/%s/pulls/%s", c.baseURL, owner, prNumber)
	type descReq struct {
		Body string `json:"body"`
	}
	data, err := json.Marshal(descReq{Body: description})
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPatch, apiURL, bytes.NewReader(data), "application/json", nil)
}

// CompareCommits returns the list of files changed between two commits.
func (c *Client) CompareCommits(ctx context.Context, projectID, from, to string) ([]string, error) {
	apiURL := fmt.Sprintf("%s/repos/%s/compare/%s...%s", c.baseURL, projectID, url.PathEscape(from), url.PathEscape(to))
	var resp CompareResponse
	if err := c.get(ctx, apiURL, &resp); err != nil {
		return nil, fmt.Errorf("comparing commits: %w", err)
	}
	
	files := make([]string, 0, len(resp.Files))
	for _, f := range resp.Files {
		files = append(files, f.Filename)
	}
	return files, nil
}

// PostNote creates a simple note (comment) on an issue/PR.
func (c *Client) PostNote(ctx context.Context, projectID, prNumber, body string) (*vcs.Comment, error) {
	apiURL := fmt.Sprintf("%s/repos/%s/issues/%s/comments", c.baseURL, projectID, prNumber)
	req := CreateIssueCommentRequest{Body: body + "\n" + botMarker}

	var comment IssueComment
	if err := c.post(ctx, apiURL, req, &comment); err != nil {
		return nil, fmt.Errorf("posting comment: %w", err)
	}
	return comment.toVCS(), nil
}

// CreateDiscussion creates a single inline comment (diff-anchored) on a pull request.
func (c *Client) CreateDiscussion(ctx context.Context, projectID, prNumber string, req vcs.InlineCommentRequest) error {
	apiURL := fmt.Sprintf("%s/repos/%s/pulls/%s/comments", c.baseURL, projectID, prNumber)
	
	if req.Position == nil || req.Position.NewLine == nil {
		return fmt.Errorf("position and new line required for inline comments")
	}

	ghReq := CreatePullCommentRequest{
		Body:     req.Body + "\n" + botMarker,
		CommitID: req.Position.HeadSHA,
		Path:     req.Position.NewPath,
		Line:     *req.Position.NewLine,
		Side:     "RIGHT",
	}

	if err := c.post(ctx, apiURL, ghReq, nil); err != nil {
		return fmt.Errorf("creating discussion: %w", err)
	}
	return nil
}

// ListBotNotes returns all comments on an issue/PR that were posted by this tool.
func (c *Client) ListBotNotes(ctx context.Context, projectID, prNumber string) ([]vcs.Comment, error) {
	initialURL := fmt.Sprintf("%s/repos/%s/issues/%s/comments?per_page=100", c.baseURL, projectID, prNumber)

	var allNotes []IssueComment
	if err := c.getPaginated(ctx, initialURL, func(raw json.RawMessage) error {
		var page []IssueComment
		if err := json.Unmarshal(raw, &page); err != nil {
			return err
		}
		allNotes = append(allNotes, page...)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("listing notes: %w", err)
	}

	var botNotes []vcs.Comment
	for _, n := range allNotes {
		if strings.Contains(n.Body, botMarker) {
			botNotes = append(botNotes, *n.toVCS())
		}
	}
	return botNotes, nil
}

// DeleteNote removes a comment from an issue/PR.
func (c *Client) DeleteNote(ctx context.Context, projectID, prNumber string, noteID int) error {
	apiURL := fmt.Sprintf("%s/repos/%s/issues/comments/%d", c.baseURL, projectID, noteID)
	if err := c.delete(ctx, apiURL); err != nil {
		return fmt.Errorf("deleting note %d: %w", noteID, err)
	}
	return nil
}

// CleanPreviousReviews deletes bot-tagged notes on an issue/PR.
// If changedFiles is non-empty, only notes referencing those files are deleted;
// summary notes are always deleted. On GitHub, issue comments are typically
// summary-only (inline comments are part of reviews), so all bot comments are deleted.
func (c *Client) CleanPreviousReviews(ctx context.Context, projectID, prNumber string, changedFiles []string) (int, error) {
	notes, err := c.ListBotNotes(ctx, projectID, prNumber)
	if err != nil {
		return 0, err
	}

	deleted := 0
	for _, n := range notes {
		if err := c.DeleteNote(ctx, projectID, prNumber, n.ID); err != nil {
			continue // Non-fatal
		}
		deleted++
		time.Sleep(apiRateDelay)
	}
	return deleted, nil
}

// SubmitReview posts a complete review to the pull request via a single API call.
// On 422 (e.g. stale commit, line outside diff), it degrades gracefully:
//  1. Retry as summary-only review (no inline comments).
//  2. If that also fails, fall back to a plain issue comment via PostNote.
//
// This ensures exactly one notification to the PR author in all cases.
func (c *Client) SubmitReview(ctx context.Context, projectID, prNumber string, req vcs.SubmitReviewRequest) error {
	// 1. Clean previous bot comments.
	if req.CleanupMode == "resolve" {
		slog.Warn("GitHub does not support resolving previous reviews; falling back to delete mode")
	}
	deleted, err := c.CleanPreviousReviews(ctx, projectID, prNumber, req.ChangedFiles)
	if err != nil {
		slog.Warn("failed to clean previous reviews", "error", err)
	} else if deleted > 0 {
		slog.Info(fmt.Sprintf("cleaned %d previous bot comment(s)", deleted))
	}

	// 2. Pre-validate comments: drop structurally invalid ones before POST.
	var validComments []ReviewCommentRequest
	var dropped int
	for _, comment := range req.Comments {
		if comment.Path == "" || comment.Line <= 0 {
			slog.Warn("dropping invalid review comment",
				"path", comment.Path,
				"line", comment.Line,
			)
			dropped++
			continue
		}
		commentBody := comment.Body
		if comment.Suggestion != "" {
			commentBody += fmt.Sprintf("\n\n```suggestion\n%s\n```", comment.Suggestion)
		}
		rc := ReviewCommentRequest{
			Path: comment.Path,
			Line: comment.Line,
			Body: commentBody,
			Side: "RIGHT",
		}
		if comment.EndLine > comment.Line {
			startLine := comment.Line
			rc.StartLine = &startLine
			rc.Line = comment.EndLine
		}
		validComments = append(validComments, rc)
	}
	if dropped > 0 {
		slog.Info("pre-validation filtered comments", "valid", len(validComments), "dropped", dropped)
	}

	// 3. Build review request, pinning to the reviewed commit.
	reviewReq := CreateReviewRequest{
		Event: "COMMENT",
		Body:  req.Summary + "\n" + botMarker,
	}
	if req.Version != nil && req.Version.HeadSHA != "" {
		reviewReq.CommitID = req.Version.HeadSHA
	}
	reviewReq.Comments = validComments

	apiURL := fmt.Sprintf("%s/repos/%s/pulls/%s/reviews", c.baseURL, projectID, prNumber)

	// 4. POST review.
	if err := c.post(ctx, apiURL, reviewReq, nil); err != nil {
		if !is422(err) {
			return fmt.Errorf("posting review: %w", err)
		}

		// 422: likely stale commit or comment on line outside diff.
		// Degrade to summary-only review (no inline comments).
		slog.Warn("batched review rejected (422), retrying as summary-only",
			"error", err,
			"dropped_comments", len(validComments),
		)

		reviewReq.Comments = nil
		if err := c.post(ctx, apiURL, reviewReq, nil); err != nil {
			if !is422(err) {
				return fmt.Errorf("posting summary-only review: %w", err)
			}

			// Summary-only also rejected (stale commit_id).
			// Last resort: plain issue comment.
			slog.Warn("summary-only review also rejected (422), falling back to issue comment",
				"error", err,
			)
			if _, err := c.PostNote(ctx, projectID, prNumber, req.Summary); err != nil {
				return fmt.Errorf("posting summary as comment: %w", err)
			}
		}

		slog.Info("review posted in degraded mode (summary only)")
		return nil
	}

	slog.Info("posted GitHub review", "comments", len(validComments))
	return nil
}

// ApproveReview approves a pull request by submitting an APPROVE review.
// headSHA pins the approval to the reviewed commit via commit_id.
func (c *Client) ApproveReview(ctx context.Context, projectID, prNumber, headSHA string) error {
	apiURL := fmt.Sprintf("%s/repos/%s/pulls/%s/reviews", c.baseURL, projectID, prNumber)
	req := CreateReviewRequest{
		CommitID: headSHA,
		Event:    "APPROVE",
		Body:     "✅ AI Code Reviewer: 0 findings. Auto-approved.",
	}
	return c.post(ctx, apiURL, req, nil)
}

// is422 checks if an error is a GitHub API 422 Unprocessable Entity response.
func is422(err error) bool {
	return err != nil && strings.Contains(err.Error(), "GitHub API error 422")
}

// getPaginated fetches all pages of a paginated GitHub API response.
func (c *Client) getPaginated(ctx context.Context, initialURL string, decode func(json.RawMessage) error) error {
	nextURL := initialURL
	for nextURL != "" {
		raw, linkHeader, err := c.doRaw(ctx, http.MethodGet, nextURL)
		if err != nil {
			return err
		}

		if err := decode(raw); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}

		nextURL = parseLinkNext(linkHeader)
		if nextURL != "" && !c.isSameOrigin(nextURL) {
			return fmt.Errorf("pagination URL %q has a different origin than the configured GitHub host; refusing to follow (possible SSRF)", nextURL)
		}
	}
	return nil
}

func (c *Client) doRaw(ctx context.Context, method, url string) ([]byte, string, error) {
	resp, err := c.executeWithRetry(ctx, method, url, nil, "")
	if err != nil {
		return nil, "", err
	}

	linkHeader := resp.Header.Get("Link")
	raw, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, "", fmt.Errorf("reading response: %w", err)
	}
	return raw, linkHeader, nil
}

func (c *Client) isSameOrigin(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return false
	}
	return parsed.Scheme == base.Scheme && parsed.Host == base.Host
}

func parseLinkNext(header string) string {
	if header == "" {
		return ""
	}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if strings.Contains(part, `rel="next"`) {
			start := strings.Index(part, "<")
			end := strings.Index(part, ">")
			if start >= 0 && end > start {
				return part[start+1 : end]
			}
		}
	}
	return ""
}

func (c *Client) get(ctx context.Context, url string, out interface{}) error {
	return c.do(ctx, http.MethodGet, url, nil, "", out)
}

func (c *Client) post(ctx context.Context, url string, body interface{}, out interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}
	return c.do(ctx, http.MethodPost, url, bytes.NewReader(data), "application/json", out)
}

func (c *Client) delete(ctx context.Context, url string) error {
	return c.do(ctx, http.MethodDelete, url, nil, "", nil)
}

func (c *Client) do(ctx context.Context, method, url string, body io.Reader, contentType string, out interface{}) error {
	resp, err := c.executeWithRetry(ctx, method, url, body, contentType)
	if err != nil {
		return err
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			_ = resp.Body.Close()
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	_ = resp.Body.Close()
	return nil
}

// executeWithRetry handles HTTP request execution with rate-limit retry.
// Returns the response on success. Caller must close the body.
func (c *Client) executeWithRetry(ctx context.Context, method, url string, body io.Reader, contentType string) (*http.Response, error) {
	for attempt := 0; attempt <= maxRetries; attempt++ {
		var reqBody io.Reader
		if body != nil {
			seeker, ok := body.(io.Seeker)
			if !ok {
				return nil, fmt.Errorf("executeWithRetry: request body must implement io.Seeker for retries")
			}
			if _, err := seeker.Seek(0, io.SeekStart); err != nil {
				return nil, fmt.Errorf("seeking request body: %w", err)
			}
			reqBody = body
		}

		req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
		if err != nil {
			return nil, err
		}
		
		req.Header.Set("Authorization", "token "+c.token)
		req.Header.Set("Accept", "application/vnd.github.v3+json")
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("HTTP request failed: %w", err)
		}

		if c.isRateLimited(resp) {
			_ = resp.Body.Close()
			if attempt >= maxRetries {
				return nil, fmt.Errorf("GitHub API rate limited after %d retries", maxRetries)
			}
			wait := c.retryDelay(resp.Header.Get("Retry-After"))
			slog.Warn("GitHub rate limited, retrying",
				"attempt", attempt+1,
				"status", resp.StatusCode,
				"wait", wait,
			)
			select {
			case <-time.After(wait):
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			_ = resp.Body.Close()
			return nil, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(bodyBytes))
		}

		return resp, nil
	}
	return nil, fmt.Errorf("GitHub API rate limited after %d retries", maxRetries)
}

func (c *Client) retryDelay(retryAfter string) time.Duration {
	baseMs := c.retryBaseMs
	if baseMs <= 0 {
		baseMs = defaultRetryMs
	}
	if retryAfter == "" {
		return time.Duration(baseMs) * time.Millisecond
	}
	secs, err := strconv.Atoi(retryAfter)
	if err != nil || secs <= 0 {
		return time.Duration(baseMs) * time.Millisecond
	}
	if secs > 60 {
		secs = 60
	}
	return time.Duration(secs) * time.Second
}

// isRateLimited returns true if the response indicates a rate limit that should be retried.
// GitHub has three flavors:
//   - 429 Too Many Requests (primary rate limit)
//   - 403 with X-RateLimit-Remaining: 0 (primary rate limit)
//   - 403 with "secondary rate limit" or "abuse detection" in body (secondary/abuse)
//
// For secondary rate limits, the body is peeked but not consumed; the caller must
// close resp.Body.
func (c *Client) isRateLimited(resp *http.Response) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if resp.StatusCode != http.StatusForbidden {
		return false
	}
	// Primary rate limit: explicit header.
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		return true
	}
	// Secondary rate limit: peek at body for abuse detection keywords.
	// Read a small prefix to check without consuming the whole body.
	peek, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	bodyStr := strings.ToLower(string(peek))
	return strings.Contains(bodyStr, "secondary rate limit") ||
		strings.Contains(bodyStr, "abuse detection")
}
