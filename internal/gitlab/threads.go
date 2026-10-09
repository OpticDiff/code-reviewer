package gitlab

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

// dismissedThread pairs a dismissed finding with the bot note that opened it.
type dismissedThread struct {
	noteID  int
	finding vcs.DismissedFinding
}

// dismissedThreads returns the inline threads opened by this tool that other
// people have engaged with, by replying or by resolving the thread.
//
// A thread only counts when its first note was written by botUser, the tool's
// own authenticated account: the markers it carries are plain text that any
// participant could forge. Engagement by botUser itself (for example resolving
// its own old threads in cleanup_mode "resolve") is ignored. When every
// participant is mrAuthor the finding is flagged AuthorOnly.
func dismissedThreads(discussions []Discussion, botMarker, botUser, mrAuthor string) []dismissedThread {
	if botUser == "" {
		return nil
	}
	var out []dismissedThread
	for _, d := range discussions {
		if len(d.Notes) == 0 {
			continue
		}
		first := d.Notes[0]
		fp, ok := vcs.ParseFingerprint(first.Body)
		if first.System || first.Author.Username != botUser || !strings.Contains(first.Body, botMarker) || first.Position == nil || !ok {
			continue
		}
		participants := map[string]bool{}
		for _, n := range d.Notes[1:] {
			if !n.System && n.Author.Username != botUser {
				participants[n.Author.Username] = true
			}
		}
		if first.Resolved && first.ResolvedBy != nil && first.ResolvedBy.Username != botUser {
			participants[first.ResolvedBy.Username] = true
		}
		if len(participants) == 0 {
			continue
		}
		authorOnly := mrAuthor != "" && len(participants) == 1 && participants[mrAuthor]
		line := 0
		if first.Position.NewLine != nil {
			line = *first.Position.NewLine
		} else if first.Position.OldLine != nil {
			line = *first.Position.OldLine
		}
		path := first.Position.NewPath
		if path == "" {
			path = first.Position.OldPath
		}
		out = append(out, dismissedThread{
			noteID: first.ID,
			finding: vcs.DismissedFinding{
				Fingerprint: fp.Fingerprint,
				Anchor:      fp.Anchor,
				AnchorLines: fp.AnchorLines,
				Path:        path,
				Line:        line,
				AuthorOnly:  authorOnly || mrAuthor == "",
			},
		})
	}
	return out
}

// currentUser returns the username of the account the client authenticates as.
func (c *Client) currentUser(ctx context.Context) (string, error) {
	var u struct {
		Username string `json:"username"`
	}
	if err := c.get(ctx, c.baseURL+"/user", &u); err != nil {
		return "", fmt.Errorf("getting authenticated user: %w", err)
	}
	if u.Username == "" {
		return "", fmt.Errorf("getting authenticated user: empty username")
	}
	return u.Username, nil
}

// mergeRequestAuthor returns the username of the merge request author.
func (c *Client) mergeRequestAuthor(ctx context.Context, projectID, mrIID string) (string, error) {
	var mr struct {
		Author Author `json:"author"`
	}
	apiURL := fmt.Sprintf("%s/projects/%s/merge_requests/%s", c.baseURL, url.PathEscape(projectID), mrIID)
	if err := c.get(ctx, apiURL, &mr); err != nil {
		return "", fmt.Errorf("getting merge request author: %w", err)
	}
	return mr.Author.Username, nil
}

// threads loads discussions together with the identities needed to classify
// them. It fails closed: any lookup error is returned and nothing is classified.
func (c *Client) threads(ctx context.Context, projectID, mrIID string) ([]dismissedThread, error) {
	botUser, err := c.currentUser(ctx)
	if err != nil {
		return nil, err
	}
	author, err := c.mergeRequestAuthor(ctx, projectID, mrIID)
	if err != nil {
		return nil, err
	}
	discussions, err := c.ListDiscussions(ctx, projectID, mrIID)
	if err != nil {
		return nil, fmt.Errorf("listing discussions: %w", err)
	}
	return dismissedThreads(discussions, c.botMarker, botUser, author), nil
}

// ListDismissedFindings returns the tool's inline threads on a merge request
// that people other than the tool have resolved or replied to.
func (c *Client) ListDismissedFindings(ctx context.Context, projectID, mrIID string) ([]vcs.DismissedFinding, error) {
	threads, err := c.threads(ctx, projectID, mrIID)
	if err != nil {
		return nil, err
	}
	out := make([]vcs.DismissedFinding, len(threads))
	for i, t := range threads {
		out[i] = t.finding
	}
	return out, nil
}

// keptNoteIDs returns the IDs of bot notes that cleanup must not delete:
// dismissed threads whose fingerprint is in keepFingerprints. A failed lookup
// is logged and keeps nothing.
func (c *Client) keptNoteIDs(ctx context.Context, projectID, mrIID string, keepFingerprints []string) map[int]bool {
	if len(keepFingerprints) == 0 {
		return nil
	}
	want := make(map[string]bool, len(keepFingerprints))
	for _, fp := range keepFingerprints {
		want[fp] = true
	}
	threads, err := c.threads(ctx, projectID, mrIID)
	if err != nil {
		slog.Warn("could not read threads to preserve dismissed findings", "error", err)
		return nil
	}
	keep := make(map[int]bool)
	for _, t := range threads {
		if want[t.finding.Fingerprint] {
			keep[t.noteID] = true
		}
	}
	return keep
}
