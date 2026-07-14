package gitlab

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
	botMarker      = "<!-- code-reviewer -->"
	apiRateDelay   = 100 * time.Millisecond
	maxRetries     = 3
	defaultRetryMs = 1000
)

// Client is an HTTP client for the GitLab REST API v4.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	retryBaseMs int // default retry delay in ms; 0 uses defaultRetryMs. Overridable for tests.
}

// NewClient creates a new GitLab API client.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/") + "/api/v4",
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// Strip auth token if redirected to a different host.
				if len(via) > 0 && req.URL.Host != via[0].URL.Host {
					req.Header.Del("PRIVATE-TOKEN")
				}
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
	}
}

// CompareCommits returns the list of files changed between two commits.
// Uses straight comparison (from..to) and checks for compare_timeout.
func (c *Client) CompareCommits(ctx context.Context, projectID, from, to string) ([]string, error) {
	apiURL := fmt.Sprintf("%s/projects/%s/repository/compare?from=%s&to=%s&straight=true",
		c.baseURL, url.PathEscape(projectID), url.QueryEscape(from), url.QueryEscape(to))
	var resp struct {
		CompareTimeout bool `json:"compare_timeout"`
		Diffs          []struct {
			NewPath  string `json:"new_path"`
			Collapsed bool  `json:"collapsed"`
			TooLarge  bool  `json:"too_large"`
		} `json:"diffs"`
	}
	if err := c.get(ctx, apiURL, &resp); err != nil {
		return nil, fmt.Errorf("comparing commits: %w", err)
	}
	if resp.CompareTimeout {
		return nil, fmt.Errorf("GitLab compare timed out; cannot determine changed files")
	}
	files := make([]string, 0, len(resp.Diffs))
	for _, d := range resp.Diffs {
		if d.Collapsed || d.TooLarge {
			return nil, fmt.Errorf("GitLab compare returned incomplete diffs (collapsed/too_large); cannot determine changed files")
		}
		files = append(files, d.NewPath)
	}
	return files, nil
}

// GetMRChanges fetches the file changes for a merge request.
func (c *Client) GetMRChanges(ctx context.Context, projectID, mrIID string) (*vcs.MRChanges, error) {
	url := fmt.Sprintf("%s/projects/%s/merge_requests/%s/changes", c.baseURL, url.PathEscape(projectID), mrIID)
	var resp MRChangesResponse
	if err := c.get(ctx, url, &resp); err != nil {
		return nil, fmt.Errorf("fetching MR changes: %w", err)
	}
	return resp.toVCS(), nil
}

// GetMRVersions fetches the diff versions for a merge request.
// Returns versions sorted by creation time (most recent first).
func (c *Client) GetMRVersions(ctx context.Context, projectID, mrIID string) ([]vcs.DiffVersion, error) {
	url := fmt.Sprintf("%s/projects/%s/merge_requests/%s/versions", c.baseURL, url.PathEscape(projectID), mrIID)
	var versions []DiffVersion
	if err := c.get(ctx, url, &versions); err != nil {
		return nil, fmt.Errorf("fetching MR versions: %w", err)
	}
	result := make([]vcs.DiffVersion, len(versions))
	for i, v := range versions {
		result[i] = v.toVCS()
	}
	return result, nil
}

// PostNote creates a simple note (comment) on a merge request.
func (c *Client) PostNote(ctx context.Context, projectID, mrIID, body string) (*vcs.Comment, error) {
	url := fmt.Sprintf("%s/projects/%s/merge_requests/%s/notes", c.baseURL, url.PathEscape(projectID), mrIID)
	req := CreateNoteRequest{Body: body + "\n" + botMarker}

	var note Note
	if err := c.post(ctx, url, req, &note); err != nil {
		return nil, fmt.Errorf("posting note: %w", err)
	}
	return note.toVCS(), nil
}

// CreateDiscussion creates an inline discussion (diff-anchored comment) on a merge request.
func (c *Client) CreateDiscussion(ctx context.Context, projectID, mrIID string, req vcs.InlineCommentRequest) error {
	url := fmt.Sprintf("%s/projects/%s/merge_requests/%s/discussions", c.baseURL, url.PathEscape(projectID), mrIID)

	// Convert vcs.InlineCommentRequest to GitLab-specific API request.
	glReq := CreateDiscussionRequest{
		Body: req.Body + "\n" + botMarker,
	}
	if req.Position != nil {
		glReq.Position = &DiscussionPosition{
			PositionType: "text",
			BaseSHA:      req.Position.BaseSHA,
			HeadSHA:      req.Position.HeadSHA,
			StartSHA:     req.Position.StartSHA,
			OldPath:      req.Position.OldPath,
			NewPath:      req.Position.NewPath,
			OldLine:      req.Position.OldLine,
			NewLine:      req.Position.NewLine,
		}
	}

	if err := c.post(ctx, url, glReq, nil); err != nil {
		return fmt.Errorf("creating discussion: %w", err)
	}
	return nil
}

// ListBotNotes returns all notes on an MR that were posted by this tool.
// Follows Link header pagination to handle MRs with >100 notes.
func (c *Client) ListBotNotes(ctx context.Context, projectID, mrIID string) ([]vcs.Comment, error) {
	initialURL := fmt.Sprintf("%s/projects/%s/merge_requests/%s/notes?per_page=100&sort=asc",
		c.baseURL, url.PathEscape(projectID), mrIID)

	var allNotes []Note
	if err := c.getPaginated(ctx, initialURL, func(raw json.RawMessage) error {
		var page []Note
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

// DeleteNote removes a note from a merge request.
func (c *Client) DeleteNote(ctx context.Context, projectID, mrIID string, noteID int) error {
	url := fmt.Sprintf("%s/projects/%s/merge_requests/%s/notes/%d", c.baseURL, url.PathEscape(projectID), mrIID, noteID)
	if err := c.delete(ctx, url); err != nil {
		return fmt.Errorf("deleting note %d: %w", noteID, err)
	}
	return nil
}

// CleanPreviousReviews deletes all bot-tagged notes on an MR.
func (c *Client) CleanPreviousReviews(ctx context.Context, projectID, mrIID string) (int, error) {
	notes, err := c.ListBotNotes(ctx, projectID, mrIID)
	if err != nil {
		return 0, err
	}

	deleted := 0
	for _, n := range notes {
		if err := c.DeleteNote(ctx, projectID, mrIID, n.ID); err != nil {
			// Non-fatal: may not have permission to delete all notes.
			continue
		}
		deleted++
		time.Sleep(apiRateDelay)
	}
	return deleted, nil
}

// getPaginated fetches all pages of a paginated GitLab API response.
// The decode function is called with each page's raw JSON for type-safe decoding.
// Uses doRaw internally, so paginated requests get the same 429 retry handling.
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

		// Validate the next URL stays on the same host to prevent SSRF.
		nextURL = parseLinkNext(linkHeader)
		if nextURL != "" && !c.isSameOrigin(nextURL) {
			return fmt.Errorf("pagination URL %q has a different origin than the configured GitLab host; refusing to follow (possible SSRF)", nextURL)
		}
	}
	return nil
}

// doRaw performs a GET request with 429 retry and returns the raw response body and Link header.
func (c *Client) doRaw(ctx context.Context, method, url string) ([]byte, string, error) {
	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, url, nil)
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("PRIVATE-TOKEN", c.token)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, "", fmt.Errorf("HTTP request failed: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			_ = resp.Body.Close()
			if attempt >= maxRetries {
				return nil, "", fmt.Errorf("GitLab API rate limited after %d retries", maxRetries)
			}
			wait := c.retryDelay(resp.Header.Get("Retry-After"))
			slog.Warn("GitLab rate limited, retrying",
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
			return nil, "", fmt.Errorf("GitLab API error %d: %s", resp.StatusCode, string(body))
		}

		linkHeader := resp.Header.Get("Link")
		raw, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, "", fmt.Errorf("reading response: %w", err)
		}
		return raw, linkHeader, nil
	}
	return nil, "", fmt.Errorf("GitLab API rate limited after %d retries", maxRetries)
}

// isSameOrigin checks that a URL has the same scheme and host as the GitLab base URL.
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

// parseLinkNext extracts the "next" URL from a Link header.
// Format: <https://gitlab.example.com/api/v4/...?page=2>; rel="next"
func parseLinkNext(header string) string {
	if header == "" {
		return ""
	}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if strings.Contains(part, `rel="next"`) {
			// Extract URL between < and >.
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
	req.Header.Set("PRIVATE-TOKEN", c.token)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Clone the request for retry (body may have been consumed).
			req = req.Clone(req.Context())
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("HTTP request failed: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			_ = resp.Body.Close()
			if attempt >= maxRetries {
				return fmt.Errorf("GitLab API rate limited after %d retries", maxRetries)
			}
			wait := c.retryDelay(resp.Header.Get("Retry-After"))
			slog.Warn("GitLab rate limited, retrying",
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
			return fmt.Errorf("GitLab API error %d: %s", resp.StatusCode, string(body))
		}

		defer func() { _ = resp.Body.Close() }()

		if out != nil {
			if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
				return fmt.Errorf("decoding response: %w", err)
			}
		}

		return nil
	}

	return fmt.Errorf("GitLab API rate limited after %d retries", maxRetries)
}

// retryDelay returns the delay before a 429 retry. Uses Retry-After header if valid,
// otherwise falls back to c.retryBaseMs (or defaultRetryMs if unset).
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
		secs = 60 // Cap at 60s to avoid excessive waits.
	}
	return time.Duration(secs) * time.Second
}
