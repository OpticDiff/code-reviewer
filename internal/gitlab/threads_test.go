package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

const testMarker = "<!-- code-reviewer -->"

func intPtr(v int) *int { return &v }

func marker(fp string) string {
	return vcs.FingerprintMarker(vcs.FingerprintInfo{Fingerprint: fp, Anchor: "a" + fp, AnchorLines: 1})
}

func noteBy(user string, id int, fp string, line int) Note {
	body := "🟠 **[HIGH]** Missing nil check\n\ndetails\n" + marker(fp) + "\n" + testMarker
	return Note{
		ID: id, Body: body, Author: Author{Username: user},
		Position: &DiscussionPosition{NewPath: "main.go", NewLine: intPtr(line)},
	}
}

func botNote(id int, fp string, line int) Note { return noteBy("review-bot", id, fp, line) }

func reply(user string, id int) Note {
	return Note{ID: id, Body: "this is intended", Author: Author{Username: user}}
}

func TestDismissedThreads(t *testing.T) {
	system := Note{ID: 91, Body: "changed this line", System: true, Author: Author{Username: "dev"}}

	resolvedByReviewer := botNote(3, "cc", 12)
	resolvedByReviewer.Resolved = true
	resolvedByReviewer.ResolvedBy = &Author{Username: "reviewer"}

	resolvedByBot := botNote(4, "dd", 13)
	resolvedByBot.Resolved = true
	resolvedByBot.ResolvedBy = &Author{Username: "review-bot"}

	discussions := []Discussion{
		{ID: "reviewer-replied", Notes: []Note{botNote(1, "aa", 10), reply("reviewer", 90)}},
		{ID: "untouched", Notes: []Note{botNote(2, "bb", 11)}},
		{ID: "resolved-reviewer", Notes: []Note{resolvedByReviewer}},
		{ID: "resolved-bot", Notes: []Note{resolvedByBot}},
		{ID: "system-only", Notes: []Note{botNote(5, "ee", 14), system}},
		{ID: "author-only", Notes: []Note{botNote(6, "ff", 15), reply("mr-author", 92)}},
		{ID: "forged", Notes: []Note{noteBy("mr-author", 7, "99", 16), reply("someone", 93)}},
	}

	got := dismissedThreads(discussions, testMarker, "review-bot", "mr-author")
	byFP := map[string]vcs.DismissedFinding{}
	for _, d := range got {
		byFP[d.finding.Fingerprint] = d.finding
	}
	if len(byFP) != 3 {
		t.Fatalf("dismissed fingerprints = %v, want aa, cc and ff only", byFP)
	}
	if _, forged := byFP["99"]; forged {
		t.Error("a marker forged by a non-bot account must be ignored")
	}
	if byFP["aa"].AuthorOnly || byFP["cc"].AuthorOnly {
		t.Error("engagement by a non-author must not be flagged AuthorOnly")
	}
	if !byFP["ff"].AuthorOnly {
		t.Error("engagement by the MR author alone must be flagged AuthorOnly")
	}
	if byFP["aa"].NoteID != 1 || byFP["cc"].NoteID != 3 || byFP["ff"].NoteID != 6 {
		t.Errorf("NoteID must be the first note of the thread: aa=%d cc=%d ff=%d", byFP["aa"].NoteID, byFP["cc"].NoteID, byFP["ff"].NoteID)
	}
	if a := byFP["aa"]; a.Path != "main.go" || a.Line != 10 || a.Anchor != "aaa" || a.AnchorLines != 1 {
		t.Errorf("unexpected finding: %+v", a)
	}
}

func TestDismissedThreads_NoBotIdentityFailsClosed(t *testing.T) {
	d := []Discussion{{ID: "x", Notes: []Note{botNote(1, "aa", 10), reply("reviewer", 2)}}}
	if got := dismissedThreads(d, testMarker, "", "mr-author"); len(got) != 0 {
		t.Errorf("got %v, want nothing without a known bot account", got)
	}
}

// threadServer serves the endpoints the thread logic needs.
func threadServer(t *testing.T, discussions []Discussion, notes []Note, deleted *[]int, userStatus int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case r.Method == http.MethodDelete:
			var id int
			_, _ = fmt.Sscanf(p[strings.LastIndex(p, "/")+1:], "%d", &id)
			*deleted = append(*deleted, id)
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(p, "/api/v4/user"):
			if userStatus != 0 {
				w.WriteHeader(userStatus)
				return
			}
			_, _ = w.Write([]byte(`{"username":"review-bot"}`))
		case strings.HasSuffix(p, "/discussions"):
			_ = json.NewEncoder(w).Encode(discussions)
		case strings.HasSuffix(p, "/notes"):
			_ = json.NewEncoder(w).Encode(notes)
		case strings.Contains(p, "/merge_requests/7"):
			_, _ = w.Write([]byte(`{"author":{"username":"mr-author"}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, p)
		}
	}))
}

func TestListDismissedFindings(t *testing.T) {
	discussions := []Discussion{{ID: "d", Notes: []Note{botNote(1, "aa", 10), reply("reviewer", 2)}}}
	var deleted []int
	srv := threadServer(t, discussions, nil, &deleted, 0)
	defer srv.Close()

	got, err := NewClient(srv.URL, "tok").ListDismissedFindings(context.Background(), "proj", "7")
	if err != nil {
		t.Fatalf("ListDismissedFindings: %v", err)
	}
	if len(got) != 1 || got[0].Fingerprint != "aa" || got[0].AuthorOnly {
		t.Errorf("got %+v, want one finding with fingerprint aa", got)
	}
}

func TestListDismissedFindings_FailsClosedWithoutIdentity(t *testing.T) {
	var deleted []int
	srv := threadServer(t, nil, nil, &deleted, http.StatusForbidden)
	defer srv.Close()
	if _, err := NewClient(srv.URL, "tok").ListDismissedFindings(context.Background(), "proj", "7"); err == nil {
		t.Error("want an error when the authenticated user cannot be determined")
	}
}

func TestCleanPreviousReviews_KeepsOnlyRequestedThreads(t *testing.T) {
	stale := botNote(1, "aa", 10)
	current := botNote(2, "bb", 11)
	untouched := botNote(3, "cc", 12)
	discussions := []Discussion{
		{ID: "d1", Notes: []Note{stale, reply("reviewer", 9)}},
		{ID: "d2", Notes: []Note{current, reply("reviewer", 10)}},
		{ID: "d3", Notes: []Note{untouched}},
	}
	notes := []Note{stale, current, untouched}

	var deleted []int
	srv := threadServer(t, discussions, notes, &deleted, 0)
	defer srv.Close()
	c := NewClient(srv.URL, "tok")

	for _, changed := range [][]string{nil, {"main.go"}} {
		deleted = nil
		// Only "bb" is still anchored to unchanged code; "aa" is stale.
		if _, err := c.cleanPreviousReviews(context.Background(), "proj", "7", changed, []string{"bb"}); err != nil {
			t.Fatalf("cleanPreviousReviews(%v): %v", changed, err)
		}
		got := fmt.Sprint(deleted)
		if strings.Contains(got, "2") || !strings.Contains(got, "1") || !strings.Contains(got, "3") {
			t.Errorf("changedFiles=%v: deleted %v, want 1 and 3 deleted but 2 kept", changed, deleted)
		}
	}
}

func TestDismissedThreads_RemovedLine(t *testing.T) {
	note := Note{
		ID: 10, Body: "🟠 **[HIGH]** Missing check\n\ndetails\n" + marker("ab12") + "\n" + testMarker,
		Author: Author{Username: "review-bot"},
		Position: &DiscussionPosition{OldPath: "deleted.go", OldLine: intPtr(42)},
	}
	discussions := []Discussion{
		{ID: "removed-replied", Notes: []Note{note, reply("reviewer", 95)}},
	}
	got := dismissedThreads(discussions, testMarker, "review-bot", "mr-author")
	if len(got) != 1 {
		t.Fatalf("expected 1 dismissed thread, got %d", len(got))
	}
	f := got[0].finding
	if f.Path != "deleted.go" || f.Line != 42 || f.Fingerprint != "ab12" || !f.OldSide {
		t.Errorf("unexpected finding for removed line: %+v", f)
	}
}


func TestResolvePreviousReviews_LeavesKeptDismissedThreadsAlone(t *testing.T) {
	dismissed := botNote(1, "aa", 10)
	stale := botNote(2, "bb", 11)
	untouched := botNote(3, "cc", 12)
	discussions := []Discussion{
		{ID: "d1", Notes: []Note{dismissed, reply("reviewer", 9)}},
		{ID: "d2", Notes: []Note{stale, reply("reviewer", 10)}},
		{ID: "d3", Notes: []Note{untouched}},
	}

	for _, changed := range [][]string{nil, {"main.go"}} {
		var resolved []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := r.URL.Path
			switch {
			case r.Method == http.MethodPut:
				resolved = append(resolved, p[strings.LastIndex(p, "/")+1:])
				_, _ = w.Write([]byte(`{}`))
			case strings.HasSuffix(p, "/api/v4/user"):
				_, _ = w.Write([]byte(`{"username":"review-bot"}`))
			case strings.HasSuffix(p, "/discussions"):
				_ = json.NewEncoder(w).Encode(discussions)
			case strings.Contains(p, "/merge_requests/7"):
				_, _ = w.Write([]byte(`{"author":{"username":"mr-author"}}`))
			default:
				t.Errorf("unexpected request %s %s", r.Method, p)
			}
		}))
		c := NewClient(srv.URL, "tok")

		// Only "aa" is still anchored to unchanged code and matched this run.
		if _, err := c.resolvePreviousReviews(context.Background(), "proj", "7", changed, []string{"aa"}); err != nil {
			t.Fatalf("resolvePreviousReviews(%v): %v", changed, err)
		}
		srv.Close()
		if got := fmt.Sprint(resolved); got != "[d2 d3]" {
			t.Errorf("changedFiles=%v: resolved %v, want d2 and d3 only; the kept thread must stay as the person left it", changed, resolved)
		}
	}
}

func TestSubmitReview_ResolveModePassesKeepFingerprints(t *testing.T) {
	dismissed := botNote(1, "aa", 10)
	discussions := []Discussion{{ID: "d1", Notes: []Note{dismissed, reply("reviewer", 9)}}}
	var resolved []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case r.Method == http.MethodPut && strings.Contains(p, "/discussions/"):
			resolved = append(resolved, p)
			_, _ = w.Write([]byte(`{}`))
		case strings.HasSuffix(p, "/api/v4/user"):
			_, _ = w.Write([]byte(`{"username":"review-bot"}`))
		case strings.HasSuffix(p, "/discussions"):
			_ = json.NewEncoder(w).Encode(discussions)
		case strings.HasSuffix(p, "/draft_notes") || strings.Contains(p, "/draft_notes/"):
			_, _ = w.Write([]byte(`[]`))
		case strings.HasSuffix(p, "/notes") && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{}`))
		case strings.Contains(p, "/merge_requests/7"):
			_, _ = w.Write([]byte(`{"author":{"username":"mr-author"}}`))
		}
	}))
	defer srv.Close()

	req := vcs.SubmitReviewRequest{Summary: "s", CleanupMode: "resolve", KeepFingerprints: []string{"aa"}}
	_ = NewClient(srv.URL, "tok").SubmitReview(context.Background(), "proj", "7", req)
	if len(resolved) != 0 {
		t.Errorf("resolve mode resolved %v, want the kept dismissed thread left alone", resolved)
	}
}
