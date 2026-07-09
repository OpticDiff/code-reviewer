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

// GetMRChanges fetches the file changes for a merge request.
func (c *Client) GetMRChanges(ctx context.Context, projectID, mrIID string) (*MRChangesResponse, error) {
	url := fmt.Sprintf("%s/projects/%s/merge_requests/%s/changes", c.baseURL, url.PathEscape(projectID), mrIID)
	var resp MRChangesResponse
	if err := c.get(ctx, url, &resp); err != nil {
		return nil, fmt.Errorf("fetching MR changes: %w", err)
	}
	return &resp, nil
}

// GetMRVersions fetches the diff versions for a merge request.
// Returns versions sorted by creation time (most recent first).
func (c *Client) GetMRVersions(ctx context.Context, projectID, mrIID string) ([]DiffVersion, error) {
	url := fmt.Sprintf("%s/projects/%s/merge_requests/%s/versions", c.baseURL, url.PathEscape(projectID), mrIID)
	var versions []DiffVersion
	if err := c.get(ctx, url, &versions); err != nil {
		return nil, fmt.Errorf("fetching MR versions: %w", err)
	}
	return versions, nil
}

// PostNote creates a simple note (comment) on a merge request.
func (c *Client) PostNote(ctx context.Context, projectID, mrIID, body string) (*Note, error) {
	url := fmt.Sprintf("%s/projects/%s/merge_requests/%s/notes", c.baseURL, url.PathEscape(projectID), mrIID)
	req := CreateNoteRequest{Body: body + "\n" + botMarker}

	var note Note
	if err := c.post(ctx, url, req, &note); err != nil {
		return nil, fmt.Errorf("posting note: %w", err)
	}
	return &note, nil
}

// CreateDiscussion creates an inline discussion (diff-anchored comment) on a merge request.
func (c *Client) CreateDiscussion(ctx context.Context, projectID, mrIID string, req CreateDiscussionRequest) error {
	url := fmt.Sprintf("%s/projects/%s/merge_requests/%s/discussions", c.baseURL, url.PathEscape(projectID), mrIID)
	req.Body = req.Body + "\n" + botMarker

	if err := c.post(ctx, url, req, nil); err != nil {
		return fmt.Errorf("creating discussion: %w", err)
	}
	return nil
}

// ListBotNotes returns all notes on an MR that were posted by this tool.
// Follows Link header pagination to handle MRs with >100 notes.
func (c *Client) ListBotNotes(ctx context.Context, projectID, mrIID string) ([]Note, error) {
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

	var botNotes []Note
	for _, n := range allNotes {
		if strings.Contains(n.Body, botMarker) {
			botNotes = append(botNotes, n)
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
func (c *Client) getPaginated(ctx context.Context, initialURL string, decode func(json.RawMessage) error) error {
	nextURL := initialURL
	for nextURL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, nextURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("PRIVATE-TOKEN", c.token)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("HTTP request failed: %w", err)
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			_ = resp.Body.Close()
			return fmt.Errorf("GitLab API error %d: %s", resp.StatusCode, string(body))
		}

		raw, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("reading response: %w", err)
		}

		if err := decode(raw); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}

		// Validate the next URL stays on the same host to prevent SSRF.
		// A malicious GitLab instance could return a Link header pointing to
		// an attacker-controlled server, exfiltrating the PRIVATE-TOKEN.
		nextURL = parseLinkNext(resp.Header.Get("Link"))
		if nextURL != "" && !c.isSameOrigin(nextURL) {
			return fmt.Errorf("pagination URL %q has a different origin than the configured GitLab host; refusing to follow (possible SSRF)", nextURL)
		}
	}
	return nil
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
			wait := retryAfterDuration(resp.Header.Get("Retry-After"))
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

// retryAfterDuration parses a Retry-After header value (seconds).
func retryAfterDuration(header string) time.Duration {
	if header == "" {
		return time.Duration(defaultRetryMs) * time.Millisecond
	}
	secs, err := strconv.Atoi(header)
	if err != nil || secs <= 0 {
		return time.Duration(defaultRetryMs) * time.Millisecond
	}
	if secs > 60 {
		secs = 60 // Cap at 60s to avoid excessive waits.
	}
	return time.Duration(secs) * time.Second
}
