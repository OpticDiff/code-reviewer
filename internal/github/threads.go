package github

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

// currentUser returns the login of the account the client authenticates as.
// GitHub does not expose this for every token type (for example the Actions
// GITHUB_TOKEN is refused), in which case the error propagates and no thread
// is treated as dismissed.
func (c *Client) currentUser(ctx context.Context) (string, error) {
	var u User
	if err := c.get(ctx, c.baseURL+"/user", &u); err != nil {
		return "", fmt.Errorf("getting authenticated user: %w", err)
	}
	if u.Login == "" {
		return "", fmt.Errorf("getting authenticated user: empty login")
	}
	return u.Login, nil
}

// pullRequestAuthor returns the login of the pull request author.
func (c *Client) pullRequestAuthor(ctx context.Context, projectID, prNumber string) (string, error) {
	var pr struct {
		User User `json:"user"`
	}
	if err := c.get(ctx, fmt.Sprintf("%s/repos/%s/pulls/%s", c.baseURL, projectID, prNumber), &pr); err != nil {
		return "", fmt.Errorf("getting pull request author: %w", err)
	}
	return pr.User.Login, nil
}

// ListDismissedFindings returns this tool's inline review comments on a pull
// request that people other than the tool have replied to.
//
// A thread only counts when its root comment was written by the tool's own
// authenticated account: the fingerprint marker is plain text that any
// participant could forge or quote. Resolution of a review thread is only
// exposed by the GraphQL API, so only replies are considered. When every
// replier is the pull request author the finding is flagged AuthorOnly.
func (c *Client) ListDismissedFindings(ctx context.Context, projectID, prNumber string) ([]vcs.DismissedFinding, error) {
	botUser, err := c.currentUser(ctx)
	if err != nil {
		return nil, err
	}
	author, err := c.pullRequestAuthor(ctx, projectID, prNumber)
	if err != nil {
		return nil, err
	}

	initialURL := fmt.Sprintf("%s/repos/%s/pulls/%s/comments?per_page=100", c.baseURL, projectID, prNumber)
	var all []PullReviewComment
	if err := c.getPaginated(ctx, initialURL, func(raw json.RawMessage) error {
		var page []PullReviewComment
		if err := json.Unmarshal(raw, &page); err != nil {
			return err
		}
		all = append(all, page...)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("listing review comments: %w", err)
	}

	repliers := make(map[int]map[string]bool)
	for _, cm := range all {
		if cm.InReplyToID == nil || cm.User.Login == botUser {
			continue
		}
		if repliers[*cm.InReplyToID] == nil {
			repliers[*cm.InReplyToID] = map[string]bool{}
		}
		repliers[*cm.InReplyToID][cm.User.Login] = true
	}

	var out []vcs.DismissedFinding
	for _, cm := range all {
		fp, ok := vcs.ParseFingerprint(cm.Body)
		if cm.InReplyToID != nil || cm.User.Login != botUser || !ok || len(repliers[cm.ID]) == 0 {
			continue
		}
		who := repliers[cm.ID]
		out = append(out, vcs.DismissedFinding{
			Fingerprint: fp.Fingerprint,
			Anchor:      fp.Anchor,
			AnchorLines: fp.AnchorLines,
			Path:        cm.Path,
			Line:        cm.Line,
			AuthorOnly:  author == "" || (len(who) == 1 && who[author]),
		})
	}
	return out, nil
}
