package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

func TestNewClient_URLNormalization(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{
			name:    "no trailing slash",
			baseURL: "https://api.github.com",
			want:    "https://api.github.com",
		},
		{
			name:    "trailing slash",
			baseURL: "https://api.github.com/",
			want:    "https://api.github.com",
		},
		{
			name:    "multiple trailing slashes",
			baseURL: "https://api.github.com///",
			want:    "https://api.github.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(tt.baseURL, "test-token")
			if c.baseURL != tt.want {
				t.Errorf("baseURL = %q, want %q", c.baseURL, tt.want)
			}
		})
	}
}

func TestGetPRChanges(t *testing.T) {
	pr := PullRequest{
		Number: 1,
		Title:  "Test PR",
		Body:   "PR body",
		State:  "open",
		Draft:  false,
	}

	files := []PullFile{
		{
			SHA:      "abcd",
			Filename: "file.go",
			Status:   "added",
			Patch:    "@@ -0,0 +1 @@\n+test",
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/files") {
			_ = json.NewEncoder(w).Encode(files)
		} else {
			_ = json.NewEncoder(w).Encode(pr)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	changes, err := client.GetMRChanges(context.Background(), "owner/repo", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if changes.ID != 1 {
		t.Errorf("ID = %d, want 1", changes.ID)
	}
	if len(changes.Changes) != 1 {
		t.Fatalf("expected 1 file, got %d", len(changes.Changes))
	}
	if !changes.Changes[0].NewFile {
		t.Errorf("expected file to be new")
	}
}

func TestGetPRVersions_SingleVersion(t *testing.T) {
	pr := PullRequest{
		Head: PRRef{SHA: "head_sha"},
		Base: PRRef{SHA: "base_sha"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pr)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	versions, err := client.GetMRVersions(context.Background(), "owner/repo", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(versions))
	}
	if versions[0].HeadSHA != "head_sha" {
		t.Errorf("HeadSHA = %q, want 'head_sha'", versions[0].HeadSHA)
	}
	if versions[0].BaseSHA != "base_sha" {
		t.Errorf("BaseSHA = %q, want 'base_sha'", versions[0].BaseSHA)
	}
}

func TestCompareCommits(t *testing.T) {
	resp := CompareResponse{
		Files: []PullFile{
			{Filename: "a.go"},
			{Filename: "b.go"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	files, err := client.CompareCommits(context.Background(), "owner/repo", "base", "head")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if files[0] != "a.go" {
		t.Errorf("files[0] = %q, want 'a.go'", files[0])
	}
}

func TestPostComment_BotMarker(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req CreateIssueCommentRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotBody = req.Body

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(IssueComment{ID: 1})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	_, err := client.PostNote(context.Background(), "owner/repo", "1", "Great code!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(gotBody, botMarker) {
		t.Errorf("expected bot marker in body, got: %q", gotBody)
	}
}

func TestCreateDiscussion_InlineComment(t *testing.T) {
	var req CreatePullCommentRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	line := 10
	inlineReq := vcs.InlineCommentRequest{
		Body: "Typo here",
		Position: &vcs.InlineCommentPosition{
			HeadSHA: "head_sha",
			NewPath: "file.go",
			NewLine: &line,
		},
	}
	err := client.CreateDiscussion(context.Background(), "owner/repo", "1", inlineReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.CommitID != "head_sha" {
		t.Errorf("CommitID = %q", req.CommitID)
	}
	if req.Path != "file.go" {
		t.Errorf("Path = %q", req.Path)
	}
	if req.Line != 10 {
		t.Errorf("Line = %d", req.Line)
	}
	if req.Side != "RIGHT" {
		t.Errorf("Side = %q", req.Side)
	}
}

func TestSubmitReview_BatchedComments(t *testing.T) {
	// First clean reviews (List bot comments) -> empty list
	// Then Submit Review
	var gotReq CreateReviewRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
			return
		}

		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	req := vcs.SubmitReviewRequest{
		Summary: "Summary",
		Comments: []vcs.ReviewComment{
			{Path: "a.go", Line: 1, Body: "msg1"},
			{Path: "b.go", Line: 2, Body: "msg2"},
		},
	}
	err := client.SubmitReview(context.Background(), "owner/repo", "1", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(gotReq.Comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(gotReq.Comments))
	}
	if gotReq.Event != "COMMENT" {
		t.Errorf("event = %q", gotReq.Event)
	}
}

func TestSubmitReview_SummaryOnly(t *testing.T) {
	var gotReq CreateReviewRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	req := vcs.SubmitReviewRequest{Summary: "Looks good"}
	err := client.SubmitReview(context.Background(), "owner/repo", "1", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotReq.Comments) != 0 {
		t.Errorf("expected 0 comments, got %d", len(gotReq.Comments))
	}
}

func TestListBotComments_FiltersByMarker(t *testing.T) {
	notes := []IssueComment{
		{ID: 1, Body: "human comment"},
		{ID: 2, Body: "bot review\n" + botMarker},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(notes)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	got, err := client.ListBotNotes(context.Background(), "proj", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 bot comment, got %d", len(got))
	}
	if got[0].ID != 2 {
		t.Errorf("bot comment ID = %d, want 2", got[0].ID)
	}
}

func TestListBotComments_Pagination(t *testing.T) {
	page1 := []IssueComment{{ID: 1, Body: botMarker}}
	page2 := []IssueComment{{ID: 2, Body: botMarker}}

	reqCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount++
		if reqCount == 1 {
			w.Header().Set("Link", fmt.Sprintf(`<%s/page2>; rel="next"`, "http://"+r.Host))
			_ = json.NewEncoder(w).Encode(page1)
		} else {
			_ = json.NewEncoder(w).Encode(page2)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	got, err := client.ListBotNotes(context.Background(), "proj", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(got))
	}
}

func TestDeleteComment(t *testing.T) {
	deleted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/issues/comments/42") {
			deleted = true
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	err := client.DeleteNote(context.Background(), "proj", "1", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Errorf("comment was not deleted")
	}
}

func TestCleanPreviousReviews_ContinuesOnDeleteError(t *testing.T) {
	notes := []IssueComment{
		{ID: 1, Body: botMarker},
		{ID: 2, Body: botMarker},
	}

	var mu sync.Mutex
	deletes := make(map[string]bool)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(notes)
			return
		}
		if r.Method == http.MethodDelete {
			parts := strings.Split(r.URL.Path, "/")
			id := parts[len(parts)-1]
			mu.Lock()
			deletes[id] = true
			mu.Unlock()
			if id == "1" {
				w.WriteHeader(http.StatusForbidden)
			}
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	count, err := client.CleanPreviousReviews(context.Background(), "proj", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 { // Only 1 succeeded
		t.Errorf("count = %d, want 1", count)
	}
	if !deletes["1"] || !deletes["2"] {
		t.Errorf("expected deletes for both")
	}
}

func TestRateLimit429_WithRetryAfter(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.Header().Set("Retry-After", "1")
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	client.retryBaseMs = 1
	var result []IssueComment
	err := client.get(context.Background(), srv.URL, &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestRateLimit429_ExhaustsRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	client.retryBaseMs = 1
	err := client.get(context.Background(), srv.URL, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestRejectsCrossHostPagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", `<http://attacker.com>; rel="next"`)
		_, _ = w.Write([]byte(`[{"id":1, "body":"<!-- code-reviewer -->"}]`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	_, err := client.ListBotNotes(context.Background(), "proj", "1")
	if err == nil || !strings.Contains(err.Error(), "different origin") {
		t.Fatalf("expected different origin error, got %v", err)
	}
}

func TestRedirect_StripsAuthCrossHost(t *testing.T) {
	var gotAuth string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer target.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()

	client := NewClient(origin.URL, "secret")
	_ = client.get(context.Background(), origin.URL, nil)

	if gotAuth != "" {
		t.Errorf("expected stripped auth, got %q", gotAuth)
	}
}
