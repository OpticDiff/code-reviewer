package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewClient_URLNormalization(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{
			name:    "no trailing slash",
			baseURL: "https://gitlab.example.com",
			want:    "https://gitlab.example.com/api/v4",
		},
		{
			name:    "trailing slash",
			baseURL: "https://gitlab.example.com/",
			want:    "https://gitlab.example.com/api/v4",
		},
		{
			name:    "multiple trailing slashes",
			baseURL: "https://gitlab.example.com///",
			want:    "https://gitlab.example.com/api/v4",
		},
		{
			name:    "subpath no slash",
			baseURL: "https://gitlab.example.com/gitlab",
			want:    "https://gitlab.example.com/gitlab/api/v4",
		},
		{
			name:    "subpath with slash",
			baseURL: "https://gitlab.example.com/gitlab/",
			want:    "https://gitlab.example.com/gitlab/api/v4",
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

func TestGetMRChanges_Success(t *testing.T) {
	want := MRChangesResponse{
		ID:          42,
		IID:         7,
		Title:       "Fix bug",
		Description: "Fixes the important bug",
		State:       "opened",
		Draft:       false,
		Changes: []DiffEntry{
			{
				OldPath:     "main.go",
				NewPath:     "main.go",
				Diff:        "@@ -1,3 +1,4 @@\n+// new line\n",
				NewFile:     false,
				RenamedFile: false,
				DeletedFile: false,
			},
			{
				OldPath:     "",
				NewPath:     "new_file.go",
				Diff:        "@@ -0,0 +1,5 @@\n+package main\n",
				NewFile:     true,
				RenamedFile: false,
				DeletedFile: false,
			},
		},
	}

	var gotPath string
	var gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.Header.Get("PRIVATE-TOKEN")
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(want); err != nil {
			t.Fatalf("encoding response: %v", err)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "my-token")
	got, err := client.GetMRChanges(context.Background(), "myproject", "7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/api/v4/projects/myproject/merge_requests/7/changes" {
		t.Errorf("request path = %q, want %q", gotPath, "/api/v4/projects/myproject/merge_requests/7/changes")
	}
	if gotToken != "my-token" {
		t.Errorf("PRIVATE-TOKEN = %q, want %q", gotToken, "my-token")
	}
	if got.ID != want.ID {
		t.Errorf("ID = %d, want %d", got.ID, want.ID)
	}
	if got.Title != want.Title {
		t.Errorf("Title = %q, want %q", got.Title, want.Title)
	}
	if len(got.Changes) != len(want.Changes) {
		t.Fatalf("len(Changes) = %d, want %d", len(got.Changes), len(want.Changes))
	}
	if got.Changes[0].OldPath != "main.go" {
		t.Errorf("Changes[0].OldPath = %q, want %q", got.Changes[0].OldPath, "main.go")
	}
	if !got.Changes[1].NewFile {
		t.Error("Changes[1].NewFile = false, want true")
	}
}

func TestGetMRChanges_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"message":"404 Not Found"}`)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	_, err := client.GetMRChanges(context.Background(), "proj", "1")
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention 404, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Not Found") {
		t.Errorf("error should include body content, got: %v", err)
	}
}

func TestPostNote_BotMarker(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		var req CreateNoteRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		gotBody = req.Body

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(Note{ID: 1, Body: req.Body}); err != nil {
			t.Fatalf("encoding response: %v", err)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	_, err := client.PostNote(context.Background(), "proj", "1", "Great code!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(gotBody, botMarker) {
		t.Errorf("body should contain bot marker %q, got: %q", botMarker, gotBody)
	}
	if !strings.HasSuffix(gotBody, botMarker) {
		t.Errorf("body should end with bot marker, got: %q", gotBody)
	}
	if !strings.HasPrefix(gotBody, "Great code!") {
		t.Errorf("body should start with original text, got: %q", gotBody)
	}
}

func TestCreateDiscussion_BotMarker(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		var req CreateDiscussionRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		gotBody = req.Body

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	req := CreateDiscussionRequest{
		Body: "Consider refactoring this",
	}
	err := client.CreateDiscussion(context.Background(), "proj", "1", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(gotBody, botMarker) {
		t.Errorf("body should contain bot marker %q, got: %q", botMarker, gotBody)
	}
	if !strings.HasSuffix(gotBody, botMarker) {
		t.Errorf("body should end with bot marker, got: %q", gotBody)
	}
	if !strings.HasPrefix(gotBody, "Consider refactoring this") {
		t.Errorf("body should start with original text, got: %q", gotBody)
	}
}

func TestListBotNotes_FiltersBotNotes(t *testing.T) {
	notes := []Note{
		{ID: 1, Body: "human comment"},
		{ID: 2, Body: "bot review\n" + botMarker},
		{ID: 3, Body: "another human comment"},
		{ID: 4, Body: "another bot review\n" + botMarker},
		{ID: 5, Body: "system note", System: true},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(notes); err != nil {
			t.Fatalf("encoding response: %v", err)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	got, err := client.ListBotNotes(context.Background(), "proj", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("len(botNotes) = %d, want 2", len(got))
	}
	if got[0].ID != 2 {
		t.Errorf("botNotes[0].ID = %d, want 2", got[0].ID)
	}
	if got[1].ID != 4 {
		t.Errorf("botNotes[1].ID = %d, want 4", got[1].ID)
	}
}

func TestListBotNotes_Pagination(t *testing.T) {
	page1Notes := []Note{
		{ID: 1, Body: "bot note page1\n" + botMarker},
		{ID: 2, Body: "human note page1"},
	}
	page2Notes := []Note{
		{ID: 3, Body: "human note page2"},
		{ID: 4, Body: "bot note page2\n" + botMarker},
	}

	var mu sync.Mutex
	requestCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		reqNum := requestCount
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		if reqNum == 1 {
			// First page — include Link header pointing to page 2.
			page2URL := fmt.Sprintf("http://%s%s?per_page=100&sort=asc&page=2", r.Host, r.URL.Path)
			w.Header().Set("Link", fmt.Sprintf("<%s>; rel=\"next\"", page2URL))
			if err := json.NewEncoder(w).Encode(page1Notes); err != nil {
				t.Errorf("encoding page1: %v", err)
			}
		} else {
			// Second page — no Link header.
			if err := json.NewEncoder(w).Encode(page2Notes); err != nil {
				t.Errorf("encoding page2: %v", err)
			}
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	got, err := client.ListBotNotes(context.Background(), "proj", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	totalRequests := requestCount
	mu.Unlock()

	if totalRequests != 2 {
		t.Errorf("expected 2 HTTP requests (2 pages), got %d", totalRequests)
	}

	if len(got) != 2 {
		t.Fatalf("len(botNotes) = %d, want 2", len(got))
	}
	if got[0].ID != 1 {
		t.Errorf("botNotes[0].ID = %d, want 1", got[0].ID)
	}
	if got[1].ID != 4 {
		t.Errorf("botNotes[1].ID = %d, want 4", got[1].ID)
	}
}

func TestCleanPreviousReviews_DeletesCorrectNotes(t *testing.T) {
	// Notes returned by list (some bot, some not).
	listNotes := []Note{
		{ID: 10, Body: "human"},
		{ID: 20, Body: "bot\n" + botMarker},
		{ID: 30, Body: "bot2\n" + botMarker},
		{ID: 40, Body: "human2"},
	}

	var mu sync.Mutex
	deletedIDs := make(map[string]bool)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(listNotes); err != nil {
				t.Errorf("encoding: %v", err)
			}
		case http.MethodDelete:
			mu.Lock()
			// Extract note ID from path: .../notes/<id>
			parts := strings.Split(r.URL.Path, "/")
			noteID := parts[len(parts)-1]
			deletedIDs[noteID] = true
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	count, err := client.CleanPreviousReviews(context.Background(), "proj", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if count != 2 {
		t.Errorf("deleted count = %d, want 2", count)
	}

	mu.Lock()
	defer mu.Unlock()

	if !deletedIDs["20"] {
		t.Error("expected note 20 to be deleted")
	}
	if !deletedIDs["30"] {
		t.Error("expected note 30 to be deleted")
	}
	if deletedIDs["10"] {
		t.Error("note 10 (non-bot) should not be deleted")
	}
	if deletedIDs["40"] {
		t.Error("note 40 (non-bot) should not be deleted")
	}
}

func TestParseLinkNext_Formats(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{
			name:   "empty header",
			header: "",
			want:   "",
		},
		{
			name:   "single next link",
			header: `<https://gitlab.example.com/api/v4/projects/1/merge_requests/2/notes?page=2>; rel="next"`,
			want:   "https://gitlab.example.com/api/v4/projects/1/merge_requests/2/notes?page=2",
		},
		{
			name:   "next and prev links",
			header: `<https://gitlab.example.com/api/v4/notes?page=1>; rel="prev", <https://gitlab.example.com/api/v4/notes?page=3>; rel="next"`,
			want:   "https://gitlab.example.com/api/v4/notes?page=3",
		},
		{
			name:   "only prev link no next",
			header: `<https://gitlab.example.com/api/v4/notes?page=1>; rel="prev"`,
			want:   "",
		},
		{
			name:   "next first then last",
			header: `<https://gitlab.example.com/api/v4/notes?page=2>; rel="next", <https://gitlab.example.com/api/v4/notes?page=5>; rel="last"`,
			want:   "https://gitlab.example.com/api/v4/notes?page=2",
		},
		{
			name:   "first prev and last only",
			header: `<https://gitlab.example.com/api/v4/notes?page=1>; rel="first", <https://gitlab.example.com/api/v4/notes?page=1>; rel="prev", <https://gitlab.example.com/api/v4/notes?page=3>; rel="last"`,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLinkNext(tt.header)
			if got != tt.want {
				t.Errorf("parseLinkNext(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

func TestURLEscape_NamespacedProject(t *testing.T) {
	var gotRequestURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestURI = r.RequestURI
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(MRChangesResponse{}); err != nil {
			t.Fatalf("encoding: %v", err)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	_, err := client.GetMRChanges(context.Background(), "my-group/my-project", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// url.PathEscape("my-group/my-project") => "my-group%2Fmy-project"
	// RequestURI preserves the percent-encoding unlike URL.Path which decodes it.
	expected := "/api/v4/projects/my-group%2Fmy-project/merge_requests/1/changes"
	if gotRequestURI != expected {
		t.Errorf("request URI = %q, want %q", gotRequestURI, expected)
	}
}

func TestCheckRedirect_CrossHost_StripsToken(t *testing.T) {
	// Second server (redirect target) — records the headers it receives.
	var gotHeaders http.Header
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(MRChangesResponse{}); err != nil {
			t.Fatalf("encoding: %v", err)
		}
	}))
	defer target.Close()

	// First server — redirects to the target.
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()

	client := NewClient(origin.URL, "secret-token")
	_, err := client.GetMRChanges(context.Background(), "proj", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if token := gotHeaders.Get("PRIVATE-TOKEN"); token != "" {
		t.Errorf("PRIVATE-TOKEN should be stripped on cross-host redirect, got %q", token)
	}
}

func TestCheckRedirect_SameHost_KeepsToken(t *testing.T) {
	callCount := 0
	var gotToken string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// Redirect to a different path on the same host.
			http.Redirect(w, r, r.URL.Path+"?redirected=true", http.StatusTemporaryRedirect)
			return
		}
		gotToken = r.Header.Get("PRIVATE-TOKEN")
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(MRChangesResponse{}); err != nil {
			t.Fatalf("encoding: %v", err)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "secret-token")
	_, err := client.GetMRChanges(context.Background(), "proj", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotToken != "secret-token" {
		t.Errorf("PRIVATE-TOKEN should be preserved on same-host redirect, got %q", gotToken)
	}
}

func TestDo_ErrorResponseBody(t *testing.T) {
	errorBody := `{"error":"internal server error","message":"something went terribly wrong"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, errorBody)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	_, err := client.GetMRChanges(context.Background(), "proj", "1")
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "500") {
		t.Errorf("error should mention status code 500, got: %v", errMsg)
	}
	if !strings.Contains(errMsg, "something went terribly wrong") {
		t.Errorf("error should include response body content, got: %v", errMsg)
	}
}

// ---------------------------------------------------------------------------
// Security: SSRF prevention in pagination
// ---------------------------------------------------------------------------

func TestListBotNotes_RejectsCrossHostPagination(t *testing.T) {
	// Attacker-controlled GitLab returns a Link header pointing to a different host.
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("SSRF: request was sent to attacker server — token exfiltrated!")
		w.WriteHeader(http.StatusOK)
	}))
	defer attacker.Close()

	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		if requestCount == 1 {
			// Return a Link header pointing to the attacker's server.
			w.Header().Set("Link", fmt.Sprintf(`<%s/steal?token=X>; rel="next"`, attacker.URL))
			_, _ = fmt.Fprint(w, `[{"id": 1, "body": "note <!-- code-reviewer -->"}]`)
		} else {
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "secret-token")
	_, err := client.ListBotNotes(context.Background(), "proj", "1")
	if err == nil {
		t.Fatal("expected SSRF error, got nil")
	}
	if !strings.Contains(err.Error(), "different origin") {
		t.Errorf("expected SSRF origin error, got: %v", err)
	}
}

func TestIsSameOrigin(t *testing.T) {
	client := NewClient("https://gitlab.example.com", "token")

	tests := []struct {
		name    string
		url     string
		allowed bool
	}{
		{"same host", "https://gitlab.example.com/api/v4/notes?page=2", true},
		{"different host", "https://attacker.com/steal", false},
		{"different scheme", "http://gitlab.example.com/api/v4/notes?page=2", false},
		{"subdomain", "https://evil.gitlab.example.com/steal", false},
		{"empty", "", false},
		{"invalid url", "://bad", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := client.isSameOrigin(tt.url)
			if got != tt.allowed {
				t.Errorf("isSameOrigin(%q) = %v, want %v", tt.url, got, tt.allowed)
			}
		})
	}
}

func TestRetryAfterDuration(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   time.Duration
	}{
		{"empty header", "", time.Duration(defaultRetryMs) * time.Millisecond},
		{"valid seconds", "2", 2 * time.Second},
		{"invalid string", "abc", time.Duration(defaultRetryMs) * time.Millisecond},
		{"zero", "0", time.Duration(defaultRetryMs) * time.Millisecond},
		{"negative", "-1", time.Duration(defaultRetryMs) * time.Millisecond},
		{"capped at 60", "120", 60 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := retryAfterDuration(tt.header)
			if got != tt.want {
				t.Errorf("retryAfterDuration(%q) = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}

func TestDo_Retries429WithRetryAfter(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.Header().Set("Retry-After", "0") // will use default (1s), but we override below
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id": 1}`))
	}))
	defer server.Close()

	// Override defaultRetryMs for fast test — use a client with short timeout.
	client := NewClient(server.URL, "test-token")
	client.httpClient.Timeout = 5 * time.Second

	ctx := context.Background()
	var result map[string]int
	err := client.get(ctx, server.URL+"/api/v4/test", &result)

	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestDo_429ExhaustsRetries(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	ctx := context.Background()
	err := client.get(ctx, server.URL+"/api/v4/test", nil)

	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("expected rate limited error, got: %v", err)
	}
	// maxRetries+1 attempts (0..maxRetries)
	if attempts != maxRetries+1 {
		t.Errorf("expected %d attempts, got %d", maxRetries+1, attempts)
	}
}

func TestCleanPreviousReviews_ContinuesOnDeleteError(t *testing.T) {
	// 3 bot notes; DELETE for note 2 returns 403, notes 1 and 3 return 204.
	listNotes := []Note{
		{ID: 1, Body: "bot1\n" + botMarker},
		{ID: 2, Body: "bot2\n" + botMarker},
		{ID: 3, Body: "bot3\n" + botMarker},
	}

	var mu sync.Mutex
	deleteAttempts := make(map[string]bool)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(listNotes); err != nil {
				t.Errorf("encoding: %v", err)
			}
		case http.MethodDelete:
			parts := strings.Split(r.URL.Path, "/")
			noteID := parts[len(parts)-1]

			mu.Lock()
			deleteAttempts[noteID] = true
			mu.Unlock()

			if noteID == "2" {
				w.WriteHeader(http.StatusForbidden)
				_, _ = fmt.Fprint(w, `{"message":"403 Forbidden"}`)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	count, err := client.CleanPreviousReviews(context.Background(), "proj", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only notes 1 and 3 should count as deleted.
	if count != 2 {
		t.Errorf("deleted count = %d, want 2", count)
	}

	mu.Lock()
	defer mu.Unlock()

	// All 3 deletes should have been attempted.
	for _, id := range []string{"1", "2", "3"} {
		if !deleteAttempts[id] {
			t.Errorf("expected DELETE attempt for note %s", id)
		}
	}
}

