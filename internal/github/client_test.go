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
	"time"

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

// --- New hardening tests ---

func TestSubmitReview_PinsCommitID(t *testing.T) {
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
	req := vcs.SubmitReviewRequest{
		Summary: "pinned review",
		Version: &vcs.DiffVersion{HeadSHA: "abc123def"},
		Comments: []vcs.ReviewComment{
			{Path: "main.go", Line: 10, Body: "fix"},
		},
	}
	if err := client.SubmitReview(context.Background(), "o/r", "1", req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotReq.CommitID != "abc123def" {
		t.Errorf("commit_id = %q, want %q", gotReq.CommitID, "abc123def")
	}
}

func TestSubmitReview_DropsInvalidComments(t *testing.T) {
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
	req := vcs.SubmitReviewRequest{
		Summary: "filtered",
		Comments: []vcs.ReviewComment{
			{Path: "ok.go", Line: 5, Body: "valid"},
			{Path: "", Line: 10, Body: "empty path"},      // dropped
			{Path: "bad.go", Line: 0, Body: "zero line"},   // dropped
			{Path: "bad.go", Line: -1, Body: "neg line"},   // dropped
			{Path: "ok2.go", Line: 20, Body: "also valid"},
		},
	}
	if err := client.SubmitReview(context.Background(), "o/r", "1", req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotReq.Comments) != 2 {
		t.Fatalf("expected 2 valid comments, got %d", len(gotReq.Comments))
	}
	if gotReq.Comments[0].Path != "ok.go" {
		t.Errorf("first comment path = %q, want ok.go", gotReq.Comments[0].Path)
	}
	if gotReq.Comments[1].Path != "ok2.go" {
		t.Errorf("second comment path = %q, want ok2.go", gotReq.Comments[1].Path)
	}
}

func TestSubmitReview_422FallsBackToSummary(t *testing.T) {
	// First POST (with comments) returns 422.
	// Second POST (summary-only) succeeds.
	reviewPosts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		reviewPosts++
		if reviewPosts == 1 {
			// First attempt: reject with 422
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"message":"Validation Failed","errors":[{"code":"invalid","field":"line"}]}`))
			return
		}
		// Second attempt (summary-only): succeed
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	req := vcs.SubmitReviewRequest{
		Summary: "my review",
		Comments: []vcs.ReviewComment{
			{Path: "a.go", Line: 999, Body: "bad line"},
		},
	}
	if err := client.SubmitReview(context.Background(), "o/r", "1", req); err != nil {
		t.Fatalf("expected graceful degradation, got error: %v", err)
	}
	if reviewPosts != 2 {
		t.Errorf("expected 2 review POSTs (1 rejected + 1 summary-only), got %d", reviewPosts)
	}
}

func TestSubmitReview_422SummaryAlsoFails_FallsBackToPostNote(t *testing.T) {
	// Both review POSTs return 422 (stale commit_id).
	// Falls back to PostNote (issue comment).
	reviewPosts := 0
	issuePosts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		// Distinguish review POST from issue comment POST by URL.
		if strings.Contains(r.URL.Path, "/reviews") {
			reviewPosts++
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"message":"Validation Failed"}`))
			return
		}
		if strings.Contains(r.URL.Path, "/comments") {
			issuePosts++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"body":"test"}`))
			return
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	req := vcs.SubmitReviewRequest{
		Summary: "my review",
		Comments: []vcs.ReviewComment{
			{Path: "a.go", Line: 5, Body: "comment"},
		},
	}
	if err := client.SubmitReview(context.Background(), "o/r", "1", req); err != nil {
		t.Fatalf("expected PostNote fallback, got error: %v", err)
	}
	if reviewPosts != 2 {
		t.Errorf("expected 2 review POSTs (both 422), got %d", reviewPosts)
	}
	if issuePosts != 1 {
		t.Errorf("expected 1 PostNote fallback, got %d", issuePosts)
	}
}

func TestSubmitReview_422AllFallbacksFail(t *testing.T) {
	// Both review POSTs return 422. PostNote also fails.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		if strings.Contains(r.URL.Path, "/reviews") {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"message":"Validation Failed"}`))
			return
		}
		// PostNote also fails.
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"Internal Server Error"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	req := vcs.SubmitReviewRequest{
		Summary:  "review",
		Comments: []vcs.ReviewComment{{Path: "a.go", Line: 5, Body: "c"}},
	}
	err := client.SubmitReview(context.Background(), "o/r", "1", req)
	if err == nil {
		t.Fatal("expected error when all fallbacks fail")
	}
	if !strings.Contains(err.Error(), "posting summary as comment") {
		t.Errorf("error = %q, want 'posting summary as comment'", err.Error())
	}
}

func TestSubmitReview_CleanupFailureContinues(t *testing.T) {
	// ListBotNotes returns notes, but deletion fails for all.
	// Review should still be posted.
	var gotReviewReq CreateReviewRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/comments"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":1,"body":"old <!-- code-reviewer -->"}]`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusInternalServerError)
		case r.Method == http.MethodPost:
			_ = json.NewDecoder(r.Body).Decode(&gotReviewReq)
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	req := vcs.SubmitReviewRequest{
		Summary: "new review",
	}
	if err := client.SubmitReview(context.Background(), "o/r", "1", req); err != nil {
		t.Fatalf("cleanup failure should not block review: %v", err)
	}
	if !strings.Contains(gotReviewReq.Body, "new review") {
		t.Errorf("review body = %q, want 'new review'", gotReviewReq.Body)
	}
}

func TestSubmitReview_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		// Block long enough for context to be cancelled.
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req := vcs.SubmitReviewRequest{Summary: "review"}
	err := client.SubmitReview(ctx, "o/r", "1", req)
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestDoRaw_429Retry(t *testing.T) {
	var mu sync.Mutex
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		a := attempts
		mu.Unlock()
		if a <= 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		// Return paginated response.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"body":"<!-- code-reviewer -->"}]`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	client.retryBaseMs = 1
	notes, err := client.ListBotNotes(context.Background(), "proj", "1")
	if err != nil {
		t.Fatalf("expected retry success, got: %v", err)
	}
	if len(notes) != 1 {
		t.Errorf("expected 1 note, got %d", len(notes))
	}
}

func Test403SecondaryRateLimit_Retried(t *testing.T) {
	var mu sync.Mutex
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		a := attempts
		mu.Unlock()
		if a <= 1 {
			// Secondary rate limit: 403 with abuse body, NO X-RateLimit-Remaining header.
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"You have exceeded a secondary rate limit. Please wait."}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	client.retryBaseMs = 1
	err := client.get(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("expected retry on secondary rate limit, got: %v", err)
	}
	mu.Lock()
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
	mu.Unlock()
}

func Test403PermissionDenied_NotRetried(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		// Regular 403: no rate limit headers, no abuse keywords.
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Resource not accessible by integration"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	client.retryBaseMs = 1
	err := client.get(context.Background(), srv.URL, nil)
	if err == nil {
		t.Fatal("expected permission error")
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry), got %d", attempts)
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %q, want 403", err.Error())
	}
}

func TestGetPRChanges_RenamedFile(t *testing.T) {
	pr := PullRequest{Number: 1, Title: "rename", State: "open"}
	files := []PullFile{
		{SHA: "a", Filename: "new_name.go", Status: "renamed", PreviousFilename: "old_name.go", Patch: "@@ -1 +1 @@\n-old\n+new"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/files") {
			_ = json.NewEncoder(w).Encode(files)
		} else {
			_ = json.NewEncoder(w).Encode(pr)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	changes, err := client.GetMRChanges(context.Background(), "o/r", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changes.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes.Changes))
	}
	c := changes.Changes[0]
	if c.OldPath != "old_name.go" {
		t.Errorf("OldPath = %q, want old_name.go", c.OldPath)
	}
	if c.NewPath != "new_name.go" {
		t.Errorf("NewPath = %q, want new_name.go", c.NewPath)
	}
	if !c.RenamedFile {
		t.Error("expected RenamedFile = true")
	}
}

func TestGetPRChanges_PaginatedFiles(t *testing.T) {
	pr := PullRequest{Number: 1, Title: "big PR", State: "open"}

	page := 0
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !strings.HasSuffix(r.URL.Path, "/files") {
			_ = json.NewEncoder(w).Encode(pr)
			return
		}
		page++
		if page == 1 {
			// First page: Link to page 2.
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/pulls/1/files?page=2>; rel="next"`, srvURL))
			_ = json.NewEncoder(w).Encode([]PullFile{
				{SHA: "a", Filename: "file1.go", Status: "modified", Patch: "p"},
			})
		} else {
			// Second page: no Link header.
			_ = json.NewEncoder(w).Encode([]PullFile{
				{SHA: "b", Filename: "file2.go", Status: "added", Patch: "p"},
			})
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	client := NewClient(srv.URL, "token")
	changes, err := client.GetMRChanges(context.Background(), "o/r", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changes.Changes) != 2 {
		t.Fatalf("expected 2 files across 2 pages, got %d", len(changes.Changes))
	}
}

