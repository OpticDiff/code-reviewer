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
				// Strip auth token if redirected to a different host.
				if len(via) > 0 && req.URL.Host != via[0].URL.Host {
					req.Header.Del("Authorization")
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

	glReq := CreatePullCommentRequest{
		Body:     req.Body + "\n" + botMarker,
		CommitID: req.Position.HeadSHA,
		Path:     req.Position.NewPath,
		Line:     *req.Position.NewLine,
		Side:     "RIGHT",
	}

	if err := c.post(ctx, apiURL, glReq, nil); err != nil {
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

// CleanPreviousReviews deletes all bot-tagged notes on an issue/PR.
func (c *Client) CleanPreviousReviews(ctx context.Context, projectID, prNumber string) (int, error) {
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
func (c *Client) SubmitReview(ctx context.Context, projectID, prNumber string, req vcs.SubmitReviewRequest) error {
	// 1. Clean previous bot comments.
	deleted, err := c.CleanPreviousReviews(ctx, projectID, prNumber)
	if err != nil {
		slog.Warn("failed to clean previous reviews", "error", err)
	} else if deleted > 0 {
		slog.Info(fmt.Sprintf("cleaned %d previous bot comment(s)", deleted))
	}

	// 2. Build review request.
	reviewReq := CreateReviewRequest{
		Event: "COMMENT",
		Body:  req.Summary + "\n" + botMarker,
	}

	for _, comment := range req.Comments {
		reviewReq.Comments = append(reviewReq.Comments, ReviewCommentRequest{
			Path: comment.Path,
			Line: comment.Line,
			Body: comment.Body,
			Side: "RIGHT",
		})
	}

	apiURL := fmt.Sprintf("%s/repos/%s/pulls/%s/reviews", c.baseURL, projectID, prNumber)

	// 3. Post review
	if err := c.post(ctx, apiURL, reviewReq, nil); err != nil {
		return fmt.Errorf("posting review: %w", err)
	}
	
	slog.Info("posted GitHub review", "comments", len(req.Comments))

	return nil
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
	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, url, nil)
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("Authorization", "token "+c.token)
		req.Header.Set("Accept", "application/vnd.github.v3+json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, "", fmt.Errorf("HTTP request failed: %w", err)
		}

		if resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0" {
			_ = resp.Body.Close()
			if attempt >= maxRetries {
				return nil, "", fmt.Errorf("GitHub API rate limited after %d retries", maxRetries)
			}
			wait := c.retryDelay(resp.Header.Get("Retry-After"))
			slog.Warn("GitHub rate limited, retrying",
				"attempt", attempt+1,
				"wait", wait,
			)
			select {
			case <-time.After(wait):
				continue
			case <-ctx.Done():
				return nil, "", ctx.Err()
			}
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			_ = resp.Body.Close()
			return nil, "", fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(body))
		}

		linkHeader := resp.Header.Get("Link")
		raw, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, "", fmt.Errorf("reading response: %w", err)
		}
		return raw, linkHeader, nil
	}
	return nil, "", fmt.Errorf("GitHub API rate limited after %d retries", maxRetries)
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) post(ctx context.Context, url string, body interface{}, out interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *Client) delete(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

func (c *Client) do(req *http.Request, out interface{}) error {
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			req = req.Clone(req.Context())
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("HTTP request failed: %w", err)
		}

		if resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0" || resp.StatusCode == http.StatusTooManyRequests {
			_ = resp.Body.Close()
			if attempt >= maxRetries {
				return fmt.Errorf("GitHub API rate limited after %d retries", maxRetries)
			}
			wait := c.retryDelay(resp.Header.Get("Retry-After"))
			slog.Warn("GitHub rate limited, retrying",
				"attempt", attempt+1,
				"wait", wait,
			)
			select {
			case <-time.After(wait):
				continue
			case <-req.Context().Done():
				return req.Context().Err()
			}
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			_ = resp.Body.Close()
			return fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(body))
		}

		defer func() { _ = resp.Body.Close() }()

		if out != nil {
			if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
				return fmt.Errorf("decoding response: %w", err)
			}
		}

		return nil
	}

	return fmt.Errorf("GitHub API rate limited after %d retries", maxRetries)
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
