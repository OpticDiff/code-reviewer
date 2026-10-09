package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

func intPtr(v int) *int { return &v }

func commentBody(fp string) string {
	return "🟠 **[HIGH]** Missing nil check\n\ndetails\n" +
		vcs.FingerprintMarker(vcs.FingerprintInfo{Fingerprint: fp, Anchor: "a" + fp, AnchorLines: 2})
}

func threadServer(t *testing.T, comments []PullReviewComment, userStatus int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/user":
			if userStatus != 0 {
				w.WriteHeader(userStatus)
				return
			}
			_, _ = w.Write([]byte(`{"login":"review-bot"}`))
		case strings.HasSuffix(r.URL.Path, "/repos/acme/app/pulls/7"):
			_, _ = w.Write([]byte(`{"user":{"login":"pr-author"}}`))
		case strings.HasSuffix(r.URL.Path, "/repos/acme/app/pulls/7/comments"):
			_ = json.NewEncoder(w).Encode(comments)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
}

func TestListDismissedFindings(t *testing.T) {
	bot := User{Login: "review-bot"}
	comments := []PullReviewComment{
		{ID: 1, Path: "main.go", Line: 10, Body: commentBody("aa"), User: bot},
		{ID: 2, Path: "main.go", Line: 11, Body: commentBody("bb"), User: bot},
		{ID: 3, Path: "main.go", Line: 10, Body: "this is intended", InReplyToID: intPtr(1), User: User{Login: "reviewer"}},
		// A quote-reply that copies the marker is still just a person's reply.
		{ID: 4, Path: "main.go", Line: 11, Body: "> quoted\n" + commentBody("bb"), InReplyToID: intPtr(2), User: bot},
		// Forged: a participant writes the marker themselves and gets a reply.
		{ID: 5, Path: "main.go", Line: 20, Body: commentBody("99"), User: User{Login: "pr-author"}},
		{ID: 6, Path: "main.go", Line: 20, Body: "ok", InReplyToID: intPtr(5), User: User{Login: "friend"}},
		// Author-only reply.
		{ID: 7, Path: "main.go", Line: 30, Body: commentBody("cc"), User: bot},
		{ID: 8, Path: "main.go", Line: 30, Body: "nah", InReplyToID: intPtr(7), User: User{Login: "pr-author"}},
	}
	srv := threadServer(t, comments, 0)
	defer srv.Close()

	got, err := NewClient(srv.URL, "tok").ListDismissedFindings(context.Background(), "acme/app", "7")
	if err != nil {
		t.Fatalf("ListDismissedFindings: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %+v, want threads aa and cc only", got)
	}
	want := vcs.DismissedFinding{Fingerprint: "aa", Anchor: "aaa", AnchorLines: 2, Path: "main.go", Line: 10}
	if got[0] != want {
		t.Errorf("got %+v, want %+v", got[0], want)
	}
	if got[1].Fingerprint != "cc" || !got[1].AuthorOnly {
		t.Errorf("got %+v, want cc flagged AuthorOnly", got[1])
	}
}

func TestListDismissedFindings_FailsClosedWithoutIdentity(t *testing.T) {
	srv := threadServer(t, nil, http.StatusForbidden)
	defer srv.Close()
	if _, err := NewClient(srv.URL, "tok").ListDismissedFindings(context.Background(), "acme/app", "7"); err == nil {
		t.Error("want an error when the authenticated user cannot be determined")
	}
}

func TestListDismissedFindings_RemovedLine(t *testing.T) {
	bot := User{Login: "review-bot"}
	comments := []PullReviewComment{
		{ID: 1, Path: "deleted.go", Line: 42, Side: "LEFT", Body: commentBody("dd"), User: bot},
		{ID: 2, Path: "deleted.go", Line: 42, Body: "noted", InReplyToID: intPtr(1), User: User{Login: "reviewer"}},
	}
	srv := threadServer(t, comments, 0)
	defer srv.Close()

	got, err := NewClient(srv.URL, "tok").ListDismissedFindings(context.Background(), "acme/app", "7")
	if err != nil {
		t.Fatalf("ListDismissedFindings: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	if !got[0].OldSide || got[0].Line != 42 || got[0].Path != "deleted.go" {
		t.Errorf("expected OldSide=true line=42 on deleted.go, got %+v", got[0])
	}
}

