package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	botMarker    = "<!-- code-reviewer -->"
	apiRateDelay = 100 * time.Millisecond
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
	if err := c.getPaginated(ctx, initialURL, &allNotes); err != nil {
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
func (c *Client) getPaginated(ctx context.Context, initialURL string, out *[]Note) error {
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
			resp.Body.Close()
			return fmt.Errorf("GitLab API error %d: %s", resp.StatusCode, string(body))
		}

		var page []Note
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			resp.Body.Close()
			return fmt.Errorf("decoding response: %w", err)
		}
		resp.Body.Close()

		*out = append(*out, page...)
		nextURL = parseLinkNext(resp.Header.Get("Link"))
	}
	return nil
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("GitLab API error %d: %s", resp.StatusCode, string(body))
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}

	return nil
}
