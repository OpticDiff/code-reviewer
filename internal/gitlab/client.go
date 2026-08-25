package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// GetDescription returns the current description of a merge request.
func (c *Client) GetDescription(ctx context.Context, projectID, mrIID string) (string, error) {
	apiURL := fmt.Sprintf("%s/projects/%s/merge_requests/%s", c.baseURL, url.PathEscape(projectID), mrIID)
	var mr struct {
		Description string `json:"description"`
	}
	if err := c.get(ctx, apiURL, &mr); err != nil {
		return "", fmt.Errorf("getting MR description: %w", err)
	}
	return mr.Description, nil
}

// SetDescription updates the description of a merge request.
func (c *Client) SetDescription(ctx context.Context, projectID, mrIID, description string) error {
	apiURL := fmt.Sprintf("%s/projects/%s/merge_requests/%s", c.baseURL, url.PathEscape(projectID), mrIID)
	type descReq struct {
		Description string `json:"description"`
	}
	return c.put(ctx, apiURL, descReq{Description: description}, nil)
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
		if req.Position.EndLine != nil && req.Position.NewLine != nil {
			glReq.Position.LineRange = &DiscussionLineRange{
				Start: DiscussionLineRef{NewLine: *req.Position.NewLine, Type: "new"},
				End:   DiscussionLineRef{NewLine: *req.Position.EndLine, Type: "new"},
			}
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

type Discussion struct {
	ID    string `json:"id"`
	Notes []Note `json:"notes"`
}

// ListDiscussions returns all discussions on a merge request.
func (c *Client) ListDiscussions(ctx context.Context, projectID, mrIID string) ([]Discussion, error) {
	url := fmt.Sprintf("%s/projects/%s/merge_requests/%s/discussions", c.baseURL, url.PathEscape(projectID), mrIID)
	var discussions []Discussion
	err := c.getPaginated(ctx, url, func(page json.RawMessage) error {
		var pageDiscussions []Discussion
		if err := json.Unmarshal(page, &pageDiscussions); err != nil {
			return err
		}
		discussions = append(discussions, pageDiscussions...)
		return nil
	})
	return discussions, err
}

// ResolveDiscussion marks a discussion as resolved on a merge request.
func (c *Client) ResolveDiscussion(ctx context.Context, projectID, mrIID string, discussionID string) error {
	apiURL := fmt.Sprintf("%s/projects/%s/merge_requests/%s/discussions/%s", c.baseURL, url.PathEscape(projectID), mrIID, discussionID)
	type resolveReq struct {
		Resolved bool `json:"resolved"`
	}
	return c.put(ctx, apiURL, resolveReq{Resolved: true}, nil)
}

// DeleteNote removes a note from a merge request.
func (c *Client) DeleteNote(ctx context.Context, projectID, mrIID string, noteID int) error {
	url := fmt.Sprintf("%s/projects/%s/merge_requests/%s/notes/%d", c.baseURL, url.PathEscape(projectID), mrIID, noteID)
	if err := c.delete(ctx, url); err != nil {
		return fmt.Errorf("deleting note %d: %w", noteID, err)
	}
	return nil
}

// CleanPreviousReviews deletes bot-tagged notes on an MR.
// If changedFiles is non-empty, only notes referencing those files are deleted;
// the summary note is always deleted so it can be replaced with an updated one.
func (c *Client) CleanPreviousReviews(ctx context.Context, projectID, mrIID string, changedFiles []string) (int, error) {
	notes, err := c.ListBotNotes(ctx, projectID, mrIID)
	if err != nil {
		return 0, err
	}

	changedSet := make(map[string]bool, len(changedFiles))
	for _, f := range changedFiles {
		changedSet[f] = true
	}

	deleted := 0
	for _, n := range notes {
		if len(changedSet) > 0 && !noteReferencesFiles(n.Body, changedSet) {
			continue // Preserve findings for unchanged files.
		}
		if err := c.DeleteNote(ctx, projectID, mrIID, n.ID); err != nil {
			// Non-fatal: may not have permission to delete all notes.
			continue
		}
		deleted++
		time.Sleep(apiRateDelay)
	}
	return deleted, nil
}

// noteReferencesFiles returns true if the note body references any file in the set,
// or if the note is a summary note (which should always be replaced).
func noteReferencesFiles(body string, files map[string]bool) bool {
	// Summary notes always get replaced.
	if strings.Contains(body, "## 📋 Code Review Summary") {
		return true
	}
	// Check if the note body mentions any changed file path.
	for f := range files {
		if strings.Contains(body, f) {
			return true
		}
	}
	return false
}

// ResolvePreviousReviews resolves all bot-tagged discussions on an MR.
func (c *Client) ResolvePreviousReviews(ctx context.Context, projectID, mrIID string) (int, error) {
	discussions, err := c.ListDiscussions(ctx, projectID, mrIID)
	if err != nil {
		return 0, err
	}

	resolved := 0
	for _, d := range discussions {
		isBotDiscussion := false
		for _, n := range d.Notes {
			if strings.Contains(n.Body, botMarker) {
				isBotDiscussion = true
				break
			}
		}
		if isBotDiscussion {
			if err := c.ResolveDiscussion(ctx, projectID, mrIID, d.ID); err != nil {
				continue
			}
			resolved++
			select {
			case <-time.After(apiRateDelay):
			case <-ctx.Done():
				return resolved, ctx.Err()
			}
		}
	}
	return resolved, nil
}

// SubmitReview posts a complete review to the merge request using draft notes
// for a single-notification experience. Falls back to individual discussions
// if the Draft Notes API is unavailable (GitLab < 15.11 or CE without the feature).
func (c *Client) SubmitReview(ctx context.Context, projectID, mrIID string, req vcs.SubmitReviewRequest) error {
	// 1. Clean previous bot comments.
	if req.CleanupMode == "resolve" {
		resolved, err := c.ResolvePreviousReviews(ctx, projectID, mrIID)
		if err != nil {
			slog.Warn("failed to resolve previous reviews", "error", err)
		} else if resolved > 0 {
			slog.Info(fmt.Sprintf("resolved %d previous bot discussion(s)", resolved))
		}
	} else {
		deleted, err := c.CleanPreviousReviews(ctx, projectID, mrIID, req.ChangedFiles)
		if err != nil {
			slog.Warn("failed to clean previous reviews", "error", err)
		} else if deleted > 0 {
			slog.Info(fmt.Sprintf("cleaned %d previous bot comment(s)", deleted))
		}
	}

	// 2. Try draft notes path (single notification).
	if err := c.submitViaDraftNotes(ctx, projectID, mrIID, req); err != nil {
		if !isDraftNotesUnavailable(err) {
			return err
		}

		// Draft Notes API unavailable — fall back to individual comments.
		slog.Warn("Draft Notes API unavailable, falling back to individual comments", "error", err)
		return c.submitViaIndividualComments(ctx, projectID, mrIID, req)
	}
	return nil
}

// submitViaDraftNotes creates all comments as unpublished drafts, then publishes
// them in a single batch. This produces exactly 1 notification to the MR author.
func (c *Client) submitViaDraftNotes(ctx context.Context, projectID, mrIID string, req vcs.SubmitReviewRequest) error {
	// Clean any stale unpublished drafts from a previous failed attempt.
	if err := c.deleteAllDraftNotes(ctx, projectID, mrIID); err != nil {
		// If stale drafts remain, we must not proceed — bulk_publish would republish them.
		if strings.Contains(err.Error(), "stale draft note(s) remain") {
			return fmt.Errorf("aborting draft review: %w", err)
		}
		slog.Warn("failed to clean stale draft notes", "error", err)
	}

	// Create summary draft note.
	summaryReq := CreateDraftNoteRequest{
		Note: req.Summary + "\n" + botMarker,
	}
	if _, err := c.createDraftNote(ctx, projectID, mrIID, summaryReq); err != nil {
		return fmt.Errorf("creating summary draft: %w", err)
	}

	// Create inline draft notes.
	var draftsFailed int
	if req.Version != nil && len(req.Comments) > 0 {
		for _, comment := range req.Comments {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("context cancelled during draft creation: %w", err)
			}
			if comment.Path == "" || comment.Line <= 0 {
				slog.Warn("dropping invalid review comment",
					"path", comment.Path,
					"line", comment.Line,
				)
				continue
			}

			newLine := comment.Line
			noteBody := comment.Body
			if comment.Suggestion != "" {
				if comment.EndLine > comment.Line {
					offset := comment.EndLine - comment.Line
					noteBody += fmt.Sprintf("\n\n```suggestion:-%d+0\n%s\n```", offset, comment.Suggestion)
				} else {
					noteBody += fmt.Sprintf("\n\n```suggestion:-0+0\n%s\n```", comment.Suggestion)
				}
			}
			draftReq := CreateDraftNoteRequest{
				Note: noteBody + "\n" + botMarker,
				Position: &DiscussionPosition{
					PositionType: "text",
					BaseSHA:      req.Version.BaseSHA,
					HeadSHA:      req.Version.HeadSHA,
					StartSHA:     req.Version.StartSHA,
					NewPath:      comment.Path,
					OldPath:      comment.Path,
					NewLine:      &newLine,
				},
			}
			if comment.EndLine > comment.Line {
				draftReq.Position.LineRange = &DiscussionLineRange{
					Start: DiscussionLineRef{NewLine: comment.Line, Type: "new"},
					End:   DiscussionLineRef{NewLine: comment.EndLine, Type: "new"},
				}
			}
			if _, err := c.createDraftNote(ctx, projectID, mrIID, draftReq); err != nil {
				// Fallback: post as a regular note so feedback is not lost.
				slog.Warn("draft creation failed, posting as note fallback",
					"file", comment.Path,
					"line", comment.Line,
					"error", err,
				)
				noteBodyStr := fmt.Sprintf("**%s:%d** — %s", comment.Path, comment.Line, noteBody)
				if _, noteErr := c.PostNote(ctx, projectID, mrIID, noteBodyStr); noteErr != nil {
					slog.Error("note fallback also failed", "error", noteErr)
				}
				draftsFailed++
			}
			time.Sleep(apiRateDelay) // Rate limit between draft note creations.
		}
	}

	// Publish all drafts in one batch (1 notification).
	if err := c.publishDraftNotes(ctx, projectID, mrIID); err != nil {
		return fmt.Errorf("publishing draft notes: %w", err)
	}

	slog.Info("posted GitLab review via draft notes",
		"comments", len(req.Comments),
		"drafts_failed", draftsFailed,
	)
	return nil
}

// submitViaIndividualComments is the legacy path: PostNote + N×CreateDiscussion.
// Used as fallback when Draft Notes API is unavailable.
func (c *Client) submitViaIndividualComments(ctx context.Context, projectID, mrIID string, req vcs.SubmitReviewRequest) error {
	// Post summary note.
	if _, err := c.PostNote(ctx, projectID, mrIID, req.Summary); err != nil {
		return fmt.Errorf("posting summary: %w", err)
	}
	slog.Info("posted review summary")

	// Post inline comments if there's a diff version.
	if req.Version != nil && len(req.Comments) > 0 {
		inlinePosted := 0
		fallbackPosted := 0

		for _, comment := range req.Comments {
			if err := ctx.Err(); err != nil {
				slog.Warn("context canceled, stopping inline comment posting", "error", err)
				break
			}
			newLine := comment.Line
			noteBody := comment.Body
			if comment.Suggestion != "" {
				if comment.EndLine > comment.Line {
					offset := comment.EndLine - comment.Line
					noteBody += fmt.Sprintf("\n\n```suggestion:-%d+0\n%s\n```", offset, comment.Suggestion)
				} else {
					noteBody += fmt.Sprintf("\n\n```suggestion:-0+0\n%s\n```", comment.Suggestion)
				}
			}
			inlineReq := vcs.InlineCommentRequest{
				Body: noteBody,
				Position: &vcs.InlineCommentPosition{
					BaseSHA:  req.Version.BaseSHA,
					HeadSHA:  req.Version.HeadSHA,
					StartSHA: req.Version.StartSHA,
					NewPath:  comment.Path,
					OldPath:  comment.Path,
					NewLine:  &newLine,
				},
			}
			if comment.EndLine > comment.Line {
				endLine := comment.EndLine
				inlineReq.Position.EndLine = &endLine
			}

			if err := c.CreateDiscussion(ctx, projectID, mrIID, inlineReq); err != nil {
				slog.Warn("failed to create inline discussion, posting as note instead",
					"file", comment.Path,
					"line", comment.Line,
					"error", err,
				)
				// Fallback: post as a regular note.
				noteBodyStr := fmt.Sprintf("**%s:%d** — %s", comment.Path, comment.Line, noteBody)
				if _, err := c.PostNote(ctx, projectID, mrIID, noteBodyStr); err != nil {
					slog.Error("failed to post fallback note", "error", err)
				} else {
					fallbackPosted++
				}
			} else {
				inlinePosted++
			}

			time.Sleep(apiRateDelay) // Rate limit.
		}
		slog.Info("posted inline comments",
			"discussions", inlinePosted,
			"fallback_notes", fallbackPosted,
			"total_findings", len(req.Comments),
		)
	}

	return nil
}

// --- Draft Notes API methods ---

// createDraftNote creates an unpublished draft note on an MR.
func (c *Client) createDraftNote(ctx context.Context, projectID, mrIID string, req CreateDraftNoteRequest) (*DraftNote, error) {
	apiURL := fmt.Sprintf("%s/projects/%s/merge_requests/%s/draft_notes",
		c.baseURL, url.PathEscape(projectID), mrIID)

	var note DraftNote
	if err := c.post(ctx, apiURL, req, &note); err != nil {
		return nil, fmt.Errorf("creating draft note: %w", err)
	}
	return &note, nil
}

// listDraftNotes returns all unpublished draft notes on an MR.
// Follows pagination to handle large numbers of stale drafts.
func (c *Client) listDraftNotes(ctx context.Context, projectID, mrIID string) ([]DraftNote, error) {
	initialURL := fmt.Sprintf("%s/projects/%s/merge_requests/%s/draft_notes?per_page=100",
		c.baseURL, url.PathEscape(projectID), mrIID)

	var allNotes []DraftNote
	if err := c.getPaginated(ctx, initialURL, func(raw json.RawMessage) error {
		var page []DraftNote
		if err := json.Unmarshal(raw, &page); err != nil {
			return err
		}
		allNotes = append(allNotes, page...)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("listing draft notes: %w", err)
	}
	return allNotes, nil
}

// publishDraftNotes publishes all pending draft notes on an MR in a single batch.
// This results in exactly one notification to the MR author.
func (c *Client) publishDraftNotes(ctx context.Context, projectID, mrIID string) error {
	apiURL := fmt.Sprintf("%s/projects/%s/merge_requests/%s/draft_notes/bulk_publish",
		c.baseURL, url.PathEscape(projectID), mrIID)

	if err := c.post(ctx, apiURL, nil, nil); err != nil {
		return fmt.Errorf("publishing draft notes: %w", err)
	}
	return nil
}

// deleteAllDraftNotes deletes all unpublished draft notes on an MR.
// Used to clean up stale drafts from a previous failed review attempt.
// Returns an error if any drafts remain after the delete pass.
func (c *Client) deleteAllDraftNotes(ctx context.Context, projectID, mrIID string) error {
	notes, err := c.listDraftNotes(ctx, projectID, mrIID)
	if err != nil {
		return err
	}
	if len(notes) == 0 {
		return nil
	}

	var deleteFailed int
	for _, n := range notes {
		dURL := fmt.Sprintf("%s/projects/%s/merge_requests/%s/draft_notes/%d",
			c.baseURL, url.PathEscape(projectID), mrIID, n.ID)
		if err := c.delete(ctx, dURL); err != nil {
			slog.Warn("failed to delete stale draft note", "id", n.ID, "error", err)
			deleteFailed++
		}
	}

	// Verify no drafts remain — bulk_publish would republish them.
	if deleteFailed > 0 {
		remaining, err := c.listDraftNotes(ctx, projectID, mrIID)
		if err != nil {
			return fmt.Errorf("verifying draft cleanup: %w", err)
		}
		if len(remaining) > 0 {
			return fmt.Errorf("%d stale draft note(s) remain after cleanup; refusing to publish (would republish stale content)", len(remaining))
		}
	}
	return nil
}

// APIError represents an HTTP error response from the GitLab API.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("GitLab API error %d: %s", e.StatusCode, e.Body)
}

// isDraftNotesUnavailable checks if the error indicates the Draft Notes API
// is not available (GitLab < 15.11 or the endpoint doesn't exist).
func isDraftNotesUnavailable(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusNotFound
	}
	// Fallback: check message for wrapped errors that lost type info.
	msg := err.Error()
	return strings.Contains(msg, "404") && strings.Contains(msg, "Not Found")
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
			return nil, "", &APIError{StatusCode: resp.StatusCode, Body: string(body)}
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

func (c *Client) put(ctx context.Context, url string, body interface{}, out interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
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
			return &APIError{StatusCode: resp.StatusCode, Body: string(body)}
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
