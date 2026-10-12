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
		first, fp, ok := botThreadRoot(d, botMarker, botUser)
		if !ok {
			continue
		}
		participants := threadParticipants(d, botUser)
		if len(participants) == 0 {
			continue
		}
		authorOnly := mrAuthor != "" && len(participants) == 1 && participants[mrAuthor]
		path, line, oldSide := threadLocation(first.Position)
		out = append(out, dismissedThread{
			noteID: first.ID,
			finding: vcs.DismissedFinding{
				Fingerprint: fp.Fingerprint,
				Anchor:      fp.Anchor,
				AnchorLines: fp.AnchorLines,
				Path:        path,
				Line:        line,
				OldSide:     oldSide,
				AuthorOnly:  authorOnly || mrAuthor == "",
			},
		})
	}
	return out
}

// openThread pairs an open finding with the discussion and bot note that hold it.
type openThread struct {
	discussionID string
	noteID       int
	finding      vcs.OpenFinding
}

// openThreads returns the inline threads opened by this tool that are still
// unresolved and that nobody but the tool has touched. The same authorship
// rules as dismissedThreads apply. A thread people replied to belongs to
// dismissedThreads, so the two never overlap.
func openThreads(discussions []Discussion, botMarker, botUser string) []openThread {
	if botUser == "" {
		return nil
	}
	var out []openThread
	for _, d := range discussions {
		first, fp, ok := botThreadRoot(d, botMarker, botUser)
		if !ok || first.Resolved || len(threadParticipants(d, botUser)) > 0 {
			continue
		}
		path, line, oldSide := threadLocation(first.Position)
		out = append(out, openThread{
			discussionID: d.ID,
			noteID:       first.ID,
			finding: vcs.OpenFinding{
				FingerprintInfo: fp,
				Path:            path,
				Line:            line,
				OldSide:         oldSide,
			},
		})
	}
	return out
}

// botThreadRoot returns the first note of d and its fingerprint when the thread
// is an inline finding written by botUser. The marker is plain text that any
// participant could forge, so the note's author must be the tool's account.
func botThreadRoot(d Discussion, botMarker, botUser string) (Note, vcs.FingerprintInfo, bool) {
	if len(d.Notes) == 0 {
		return Note{}, vcs.FingerprintInfo{}, false
	}
	first := d.Notes[0]
	fp, ok := vcs.ParseFingerprint(first.Body)
	if first.System || first.Author.Username != botUser || !strings.Contains(first.Body, botMarker) || first.Position == nil || !ok {
		return Note{}, vcs.FingerprintInfo{}, false
	}
	return first, fp, true
}

// threadParticipants returns the people other than botUser who replied to the
// thread or resolved it. System notes do not count.
func threadParticipants(d Discussion, botUser string) map[string]bool {
	participants := map[string]bool{}
	for _, n := range d.Notes[1:] {
		if !n.System && n.Author.Username != botUser {
			participants[n.Author.Username] = true
		}
	}
	if first := d.Notes[0]; first.Resolved && first.ResolvedBy != nil && first.ResolvedBy.Username != botUser {
		participants[first.ResolvedBy.Username] = true
	}
	return participants
}

// threadLocation returns the file, line and diff side a thread is anchored to.
func threadLocation(pos *DiscussionPosition) (path string, line int, oldSide bool) {
	if pos.NewLine != nil {
		line = *pos.NewLine
	} else if pos.OldLine != nil {
		line = *pos.OldLine
		oldSide = true
	}
	path = pos.NewPath
	if path == "" {
		path = pos.OldPath
	}
	return path, line, oldSide
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

// openThreadsOf lists the tool's open inline threads on a merge request. Like
// threads, it fails closed when the tool's own account cannot be determined.
func (c *Client) openThreadsOf(ctx context.Context, projectID, mrIID string) ([]openThread, error) {
	botUser, err := c.currentUser(ctx)
	if err != nil {
		return nil, err
	}
	discussions, err := c.ListDiscussions(ctx, projectID, mrIID)
	if err != nil {
		return nil, fmt.Errorf("listing discussions: %w", err)
	}
	return openThreads(discussions, c.botMarker, botUser), nil
}

// ListOpenFindings returns the tool's unresolved inline threads on a merge
// request that nobody else has replied to or resolved.
func (c *Client) ListOpenFindings(ctx context.Context, projectID, mrIID string) ([]vcs.OpenFinding, error) {
	threads, err := c.openThreadsOf(ctx, projectID, mrIID)
	if err != nil {
		return nil, err
	}
	out := make([]vcs.OpenFinding, len(threads))
	for i, t := range threads {
		out[i] = t.finding
	}
	return out, nil
}

// preserved names the threads cleanup must leave in place.
type preserved struct {
	noteIDs         map[int]bool    // Bot notes that must not be deleted.
	openDiscussions map[string]bool // Open discussions that must not be resolved.
}

// preservedThreads resolves the fingerprints cleanup must keep to concrete
// threads: dismissed threads whose fingerprint is in dismissedFingerprints, and
// open threads for openFingerprints, which lists one entry per thread to keep
// so a duplicate beyond the listed count is cleaned. A failed lookup is logged
// and keeps nothing.
func (c *Client) preservedThreads(ctx context.Context, projectID, mrIID string, dismissedFingerprints, openFingerprints []string) preserved {
	var keep preserved
	if len(dismissedFingerprints) > 0 {
		want := make(map[string]bool, len(dismissedFingerprints))
		for _, fp := range dismissedFingerprints {
			want[fp] = true
		}
		threads, err := c.threads(ctx, projectID, mrIID)
		if err != nil {
			slog.Warn("could not read threads to preserve dismissed findings", "error", err)
		}
		keep.noteIDs = make(map[int]bool)
		for _, t := range threads {
			if want[t.finding.Fingerprint] {
				keep.noteIDs[t.noteID] = true
			}
		}
	}
	if len(openFingerprints) > 0 {
		threads, err := c.openThreadsOf(ctx, projectID, mrIID)
		if err != nil {
			slog.Warn("could not read threads to preserve open findings", "error", err)
		}
		left := make(map[string]int, len(openFingerprints))
		for _, fp := range openFingerprints {
			left[fp]++
		}
		if keep.noteIDs == nil {
			keep.noteIDs = make(map[int]bool)
		}
		keep.openDiscussions = make(map[string]bool)
		for _, t := range threads {
			if fp := t.finding.Fingerprint; left[fp] > 0 {
				left[fp]--
				keep.noteIDs[t.noteID] = true
				keep.openDiscussions[t.discussionID] = true
			}
		}
	}
	return keep
}
