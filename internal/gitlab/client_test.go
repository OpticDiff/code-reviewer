package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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

// mrServer serves a merge request, its paginated /diffs (2 entries per page)
// and its raw_diffs, recording every request path.
type mrServer struct {
	meta     string
	diffs    []string // JSON objects, one per file
	raw      string
	rawCalls int
	paths    []string
}

func (m *mrServer) handler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		m.paths = append(m.paths, r.URL.Path)
		switch {
		case strings.HasSuffix(r.URL.Path, "/raw_diffs"):
			m.rawCalls++
			_, _ = fmt.Fprint(w, m.raw)
		case strings.HasSuffix(r.URL.Path, "/diffs"):
			page := 1
			if p := r.URL.Query().Get("page"); p != "" {
				page, _ = strconv.Atoi(p)
			}
			const perPage = 2
			lo := (page - 1) * perPage
			hi := min(lo+perPage, len(m.diffs))
			if hi < len(m.diffs) {
				next := fmt.Sprintf("http://%s%s?page=%d&per_page=%d", r.Host, r.URL.Path, page+1, perPage)
				w.Header().Set("Link", "<"+next+`>; rel="next"`)
			}
			_, _ = fmt.Fprintf(w, "[%s]", strings.Join(m.diffs[lo:hi], ","))
		default:
			_, _ = fmt.Fprint(w, m.meta)
		}
	}
}

// writeEmptyMR answers the merge request and its /diffs with empty payloads.
func writeEmptyMR(w http.ResponseWriter, r *http.Request) error {
	body := "{}"
	if strings.HasSuffix(r.URL.Path, "/diffs") {
		body = "[]"
	}
	_, err := w.Write([]byte(body))
	return err
}

func diffJSON(path, diff string, flags string) string {
	b, _ := json.Marshal(diff)
	return fmt.Sprintf(`{"old_path":%q,"new_path":%q,"diff":%s%s}`, path, path, b, flags)
}

func TestGetMRChanges_Success(t *testing.T) {
	m := &mrServer{
		meta: `{"id":42,"iid":7,"title":"Fix bug","description":"Fixes it","state":"opened","draft":true,"changes_count":"3"}`,
		diffs: []string{
			diffJSON("a.go", "@@ -1 +1 @@\n-a\n+b\n", ""),
			diffJSON("b.go", "@@ -1 +1 @@\n-c\n+d\n", ""),
			`{"old_path":"","new_path":"new_file.go","diff":"@@ -0,0 +1 @@\n+x\n","new_file":true}`,
		},
	}
	var gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("PRIVATE-TOKEN")
		m.handler(t)(w, r)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "my-token")
	got, err := client.GetMRChanges(context.Background(), "myproject", "7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotToken != "my-token" {
		t.Errorf("PRIVATE-TOKEN = %q, want %q", gotToken, "my-token")
	}
	for _, p := range m.paths {
		if strings.HasSuffix(p, "/changes") {
			t.Errorf("deprecated /changes endpoint requested: %s", p)
		}
	}
	if got.ID != 42 || got.IID != 7 || got.Title != "Fix bug" || !got.Draft || got.State != "opened" {
		t.Errorf("metadata not populated: %+v", got)
	}
	if len(got.Changes) != 3 {
		t.Fatalf("len(Changes) = %d, want 3 (all pages)", len(got.Changes))
	}
	if !got.Changes[2].NewFile || got.Changes[2].NewPath != "new_file.go" {
		t.Errorf("Changes[2] = %+v", got.Changes[2])
	}
	if m.rawCalls != 0 {
		t.Errorf("raw_diffs fetched %d times with no collapsed entries", m.rawCalls)
	}
}

func TestGetMRChanges_RecoversCollapsedFromRawDiffs(t *testing.T) {
	raw := "diff --git a/big.go b/big.go\nindex 111..222 100644\n--- a/big.go\n+++ b/big.go\n@@ -1 +1 @@\n-old\n+new\n" +
		"diff --git a/ok.go b/ok.go\nindex 333..444 100644\n--- a/ok.go\n+++ b/ok.go\n@@ -1 +1 @@\n-x\n+y\n"
	m := &mrServer{
		meta: `{"id":1,"iid":1,"changes_count":"2"}`,
		diffs: []string{
			diffJSON("big.go", "", `,"collapsed":true`),
			diffJSON("ok.go", "@@ -1 +1 @@\n-x\n+y\n", ""),
		},
		raw: raw,
	}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()

	got, err := NewClient(srv.URL, "t").GetMRChanges(context.Background(), "p", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.rawCalls != 1 {
		t.Errorf("raw_diffs calls = %d, want 1", m.rawCalls)
	}
	if want := "@@ -1 +1 @@\n-old\n+new\n"; got.Changes[0].Diff != want {
		t.Errorf("recovered diff = %q, want %q", got.Changes[0].Diff, want)
	}
	if got.Changes[0].Incomplete {
		t.Error("recovered entry still flagged Incomplete")
	}
}

func TestGetMRChanges_TooLargeStaysIncomplete(t *testing.T) {
	m := &mrServer{
		meta:  `{"id":1,"iid":1,"changes_count":"1"}`,
		diffs: []string{diffJSON("go.sum", "", `,"too_large":true`)},
		raw:   "", // GitLab could not produce a patch either
	}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()

	got, err := NewClient(srv.URL, "t").GetMRChanges(context.Background(), "p", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Changes) != 1 || !got.Changes[0].Incomplete {
		t.Errorf("too_large entry must be kept and flagged Incomplete, got %+v", got.Changes)
	}
}

func TestGetMRChanges_ChangesCountMismatchWarns(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	m := &mrServer{
		meta:  `{"id":1,"iid":1,"changes_count":"5"}`,
		diffs: []string{diffJSON("a.go", "@@ -1 +1 @@\n-a\n+b\n", "")},
	}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()

	if _, err := NewClient(srv.URL, "t").GetMRChanges(context.Background(), "p", "1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "changes_count") {
		t.Errorf("expected a changes_count mismatch warning, log was: %s", buf.String())
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

	if !strings.Contains(gotBody, "<!-- code-reviewer -->") {
		t.Errorf("body should contain bot marker %q, got: %q", "<!-- code-reviewer -->", gotBody)
	}
	if !strings.HasSuffix(gotBody, "<!-- code-reviewer -->") {
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
	req := vcs.InlineCommentRequest{
		Body: "Consider refactoring this",
	}
	err := client.CreateDiscussion(context.Background(), "proj", "1", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(gotBody, "<!-- code-reviewer -->") {
		t.Errorf("body should contain bot marker %q, got: %q", "<!-- code-reviewer -->", gotBody)
	}
	if !strings.HasSuffix(gotBody, "<!-- code-reviewer -->") {
		t.Errorf("body should end with bot marker, got: %q", gotBody)
	}
	if !strings.HasPrefix(gotBody, "Consider refactoring this") {
		t.Errorf("body should start with original text, got: %q", gotBody)
	}
}

func TestListBotNotes_FiltersBotNotes(t *testing.T) {
	notes := []Note{
		{ID: 1, Body: "human comment"},
		{ID: 2, Body: "bot review\n" + "<!-- code-reviewer -->"},
		{ID: 3, Body: "another human comment"},
		{ID: 4, Body: "another bot review\n" + "<!-- code-reviewer -->"},
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
		{ID: 1, Body: "bot note page1\n" + "<!-- code-reviewer -->"},
		{ID: 2, Body: "human note page1"},
	}
	page2Notes := []Note{
		{ID: 3, Body: "human note page2"},
		{ID: 4, Body: "bot note page2\n" + "<!-- code-reviewer -->"},
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
		{ID: 20, Body: "bot\n" + "<!-- code-reviewer -->"},
		{ID: 30, Body: "bot2\n" + "<!-- code-reviewer -->"},
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
	count, err := client.CleanPreviousReviews(context.Background(), "proj", "1", nil)
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
		if gotRequestURI == "" {
			gotRequestURI = r.RequestURI
		}
		w.Header().Set("Content-Type", "application/json")
		if err := writeEmptyMR(w, r); err != nil {
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
	expected := "/api/v4/projects/my-group%2Fmy-project/merge_requests/1"
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
		if err := writeEmptyMR(w, r); err != nil {
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
		if err := writeEmptyMR(w, r); err != nil {
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

func TestRetryDelay(t *testing.T) {
	client := NewClient("https://gitlab.example.com", "test")

	tests := []struct {
		name      string
		retryBase int
		header    string
		want      time.Duration
	}{
		{"empty header, default base", 0, "", time.Duration(defaultRetryMs) * time.Millisecond},
		{"empty header, custom base", 50, "", 50 * time.Millisecond},
		{"valid seconds", 0, "2", 2 * time.Second},
		{"invalid string", 0, "abc", time.Duration(defaultRetryMs) * time.Millisecond},
		{"zero", 0, "0", time.Duration(defaultRetryMs) * time.Millisecond},
		{"negative", 0, "-1", time.Duration(defaultRetryMs) * time.Millisecond},
		{"capped at 60", 0, "120", 60 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client.retryBaseMs = tt.retryBase
			got := client.retryDelay(tt.header)
			if got != tt.want {
				t.Errorf("retryDelay(%q) = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}

func TestDo_Retries429WithRetryAfter(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id": 1}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	client.retryBaseMs = 1 // 1ms for fast test

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
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	client.retryBaseMs = 1 // 1ms for fast test

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
		{ID: 1, Body: "bot1\n" + "<!-- code-reviewer -->"},
		{ID: 2, Body: "bot2\n" + "<!-- code-reviewer -->"},
		{ID: 3, Body: "bot3\n" + "<!-- code-reviewer -->"},
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
	count, err := client.CleanPreviousReviews(context.Background(), "proj", "1", nil)
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

func TestCompareCommits_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"compare_timeout": true, "diffs": []}`)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	_, err := client.CompareCommits(context.Background(), "proj", "abc123", "def456")
	if err == nil {
		t.Fatal("expected error for compare timeout, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected error containing 'timed out', got: %v", err)
	}
}

func TestCompareCommits_Success(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"compare_timeout": false, "diffs": [{"new_path": "a.go"}, {"new_path": "b.go"}]}`)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	files, err := client.CompareCommits(context.Background(), "proj", "abc123", "def456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := gotQuery.Get("straight"); got != "true" {
		t.Errorf("straight = %q, want %q", got, "true")
	}
	if got := gotQuery.Get("from"); got != "abc123" {
		t.Errorf("from = %q, want %q", got, "abc123")
	}
	if got := gotQuery.Get("to"); got != "def456" {
		t.Errorf("to = %q, want %q", got, "def456")
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if files[0] != "a.go" {
		t.Errorf("files[0] = %q, want %q", files[0], "a.go")
	}
	if files[1] != "b.go" {
		t.Errorf("files[1] = %q, want %q", files[1], "b.go")
	}
}

func TestCompareCommits_CollapsedDiff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"compare_timeout": false, "diffs": [{"new_path": "a.go"}, {"new_path": "big.go", "collapsed": true}]}`)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	_, err := client.CompareCommits(context.Background(), "proj", "abc", "def")
	if err == nil {
		t.Fatal("expected error for collapsed diff, got nil")
	}
	if !strings.Contains(err.Error(), "collapsed/too_large") {
		t.Errorf("expected error containing 'collapsed/too_large', got: %v", err)
	}
}

func TestCompareCommits_TooLargeDiff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"compare_timeout": false, "diffs": [{"new_path": "a.go", "too_large": true}]}`)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	_, err := client.CompareCommits(context.Background(), "proj", "abc", "def")
	if err == nil {
		t.Fatal("expected error for too_large diff, got nil")
	}
	if !strings.Contains(err.Error(), "collapsed/too_large") {
		t.Errorf("expected error containing 'collapsed/too_large', got: %v", err)
	}
}

// --- Draft Notes tests ---

func TestSubmitReview_DraftNotes_HappyPath(t *testing.T) {
	// Draft notes path: create summary draft + inline drafts → bulk_publish.
	var draftPosts int
	var publishCalled bool
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		switch {
		// ListBotNotes (for CleanPreviousReviews)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/notes"):
			_, _ = w.Write([]byte(`[]`))

		// List draft notes (for deleteAllDraftNotes cleanup)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/draft_notes"):
			_, _ = w.Write([]byte(`[]`))

		// Create draft note
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/draft_notes") && !strings.Contains(r.URL.Path, "bulk_publish"):
			draftPosts++
			_, _ = fmt.Fprintf(w, `{"id":%d,"note":"draft"}`, draftPosts)

		// Publish drafts
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/bulk_publish"):
			publishCalled = true
			w.WriteHeader(http.StatusOK)

		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	req := vcs.SubmitReviewRequest{
		Summary: "Review summary",
		Version: &vcs.DiffVersion{
			HeadSHA:  "head",
			BaseSHA:  "base",
			StartSHA: "start",
		},
		Comments: []vcs.ReviewComment{
			{Path: "a.go", Line: 10, Body: "fix this"},
			{Path: "b.go", Line: 20, Body: "and this"},
		},
	}

	if err := client.SubmitReview(context.Background(), "mygroup/myproject", "1", req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	// 1 summary draft + 2 inline drafts = 3 total
	if draftPosts != 3 {
		t.Errorf("expected 3 draft POSTs, got %d", draftPosts)
	}
	if !publishCalled {
		t.Error("expected bulk_publish to be called")
	}
}

func TestSubmitReview_DraftNotes_FallbackOn404(t *testing.T) {
	// Draft Notes API returns 404 → falls back to individual comments.
	var notePosts int
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		switch {
		// ListBotNotes
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/notes"):
			_, _ = w.Write([]byte(`[]`))

		// Draft notes API → 404 (unavailable)
		case strings.Contains(r.URL.Path, "/draft_notes"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))

		// PostNote (fallback path)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/notes"):
			notePosts++
			_, _ = fmt.Fprintf(w, `{"id":%d,"body":"note"}`, notePosts)

		// CreateDiscussion (fallback path)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/discussions"):
			w.WriteHeader(http.StatusOK)

		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	req := vcs.SubmitReviewRequest{
		Summary: "Review",
	}

	if err := client.SubmitReview(context.Background(), "proj", "1", req); err != nil {
		t.Fatalf("expected fallback success, got: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if notePosts < 1 {
		t.Errorf("expected at least 1 PostNote call in fallback, got %d", notePosts)
	}
}

func TestSubmitReview_DraftNotes_PublishFailure(t *testing.T) {
	// Drafts created successfully but publish fails → error returned.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/notes"):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/draft_notes"):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/bulk_publish"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"Internal Server Error"}`))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/draft_notes"):
			_, _ = w.Write([]byte(`{"id":1,"note":"draft"}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	req := vcs.SubmitReviewRequest{Summary: "Review"}

	err := client.SubmitReview(context.Background(), "proj", "1", req)
	if err == nil {
		t.Fatal("expected error on publish failure")
	}
	if !strings.Contains(err.Error(), "publishing draft notes") {
		t.Errorf("error = %q, want 'publishing draft notes'", err.Error())
	}
}

func TestSubmitReview_DraftNotes_CleansStaleDrafts(t *testing.T) {
	// Existing stale drafts are deleted before new ones are created.
	var deleteCalled bool
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/notes"):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/draft_notes"):
			// Return a stale draft note.
			_, _ = w.Write([]byte(`[{"id":99,"note":"stale draft"}]`))
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/draft_notes/99"):
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/bulk_publish"):
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/draft_notes"):
			_, _ = w.Write([]byte(`{"id":1,"note":"new"}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	req := vcs.SubmitReviewRequest{Summary: "Review"}

	if err := client.SubmitReview(context.Background(), "proj", "1", req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if !deleteCalled {
		t.Error("expected stale draft note 99 to be deleted")
	}
}

func TestSubmitReview_DraftNotes_DropsInvalidComments(t *testing.T) {
	var draftPosts int
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/notes"):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/draft_notes"):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/bulk_publish"):
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/draft_notes"):
			draftPosts++
			_, _ = fmt.Fprintf(w, `{"id":%d,"note":"draft"}`, draftPosts)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	req := vcs.SubmitReviewRequest{
		Summary: "Review",
		Version: &vcs.DiffVersion{HeadSHA: "h", BaseSHA: "b", StartSHA: "s"},
		Comments: []vcs.ReviewComment{
			{Path: "ok.go", Line: 5, Body: "valid"},
			{Path: "", Line: 10, Body: "empty path"},     // dropped
			{Path: "bad.go", Line: 0, Body: "zero line"}, // dropped
		},
	}

	if err := client.SubmitReview(context.Background(), "proj", "1", req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	// 1 summary + 1 valid inline = 2
	if draftPosts != 2 {
		t.Errorf("expected 2 draft POSTs (1 summary + 1 valid), got %d", draftPosts)
	}
}

func TestSubmitReview_DraftNotes_SummaryOnly(t *testing.T) {
	// No version → only summary draft + publish, no inline drafts.
	var draftPosts int
	var publishCalled bool
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/notes"):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/draft_notes"):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/bulk_publish"):
			publishCalled = true
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/draft_notes"):
			draftPosts++
			_, _ = fmt.Fprintf(w, `{"id":%d,"note":"draft"}`, draftPosts)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	req := vcs.SubmitReviewRequest{
		Summary: "Clean review, no findings",
	}

	if err := client.SubmitReview(context.Background(), "proj", "1", req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if draftPosts != 1 {
		t.Errorf("expected 1 draft POST (summary only), got %d", draftPosts)
	}
	if !publishCalled {
		t.Error("expected bulk_publish to be called")
	}
}

func TestIsDraftNotesUnavailable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"typed 404", &APIError{StatusCode: 404, Body: "Not Found"}, true},
		{"wrapped typed 404", fmt.Errorf("creating draft note: %w", &APIError{StatusCode: 404, Body: "Not Found"}), true},
		{"typed 500", &APIError{StatusCode: 500, Body: "Internal Server Error"}, false},
		{"typed 422", &APIError{StatusCode: 422, Body: "Validation Failed"}, false},
		{"string 404 Not Found", fmt.Errorf("GitLab API error 404: Not Found"), true},
		{"string 404 only (no Not Found)", fmt.Errorf("something 404"), false},
		{"generic error", fmt.Errorf("connection refused"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDraftNotesUnavailable(tt.err)
			if got != tt.want {
				t.Errorf("isDraftNotesUnavailable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestSubmitReview_DraftNotes_InlineDraftFailsFallsBackToNote(t *testing.T) {
	// When createDraftNote fails for an inline comment, it should fall back to PostNote.
	var notePosts int
	var draftPosts int
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/notes"):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/draft_notes"):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/bulk_publish"):
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/draft_notes"):
			draftPosts++
			if draftPosts == 2 {
				// Second draft (first inline) fails.
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"message":"Validation failed"}`))
				return
			}
			_, _ = fmt.Fprintf(w, `{"id":%d,"note":"draft"}`, draftPosts)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/notes"):
			notePosts++
			_, _ = fmt.Fprintf(w, `{"id":%d,"body":"fallback"}`, 100+notePosts)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	req := vcs.SubmitReviewRequest{
		Summary: "Review",
		Version: &vcs.DiffVersion{HeadSHA: "h", BaseSHA: "b", StartSHA: "s"},
		Comments: []vcs.ReviewComment{
			{Path: "a.go", Line: 10, Body: "problem here"},
		},
	}

	if err := client.SubmitReview(context.Background(), "proj", "1", req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if notePosts != 1 {
		t.Errorf("expected 1 PostNote fallback call, got %d", notePosts)
	}
}

func TestSubmitReview_DraftNotes_StaleDraftCleanupFailure(t *testing.T) {
	// If stale drafts can't be deleted AND remain after cleanup,
	// submitViaDraftNotes should propagate the error to prevent republishing stale content.
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/notes"):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/draft_notes"):
			// Always returns a stale draft (can't be deleted).
			_, _ = w.Write([]byte(`[{"id":99,"note":"stale draft that won't go away"}]`))
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/draft_notes/99"):
			// Delete fails.
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"403 Forbidden"}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	req := vcs.SubmitReviewRequest{Summary: "Review"}

	err := client.SubmitReview(context.Background(), "proj", "1", req)
	if err == nil {
		t.Fatal("expected error when stale drafts remain")
	}
	if !strings.Contains(err.Error(), "stale draft note(s) remain") {
		t.Errorf("error = %q, want contains 'stale draft note(s) remain'", err.Error())
	}
}

func TestGetDescription_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"description": "existing description"}`)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	desc, err := client.GetDescription(context.Background(), "proj", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if desc != "existing description" {
		t.Errorf("GetDescription() = %q, want %q", desc, "existing description")
	}
}

func TestSetDescription_Success(t *testing.T) {
	var gotMethod string
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	err := client.SetDescription(context.Background(), "proj", "1", "new desc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want %q", gotMethod, http.MethodPut)
	}
	if !strings.Contains(gotBody, `"description":"new desc"`) {
		t.Errorf("body = %q, want it to contain %q", gotBody, `"description":"new desc"`)
	}
}

func TestGetMRVersions_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[
			{"id": 1, "head_commit_sha": "abc", "base_commit_sha": "def", "start_commit_sha": "ghi"},
			{"id": 2, "head_commit_sha": "jkl", "base_commit_sha": "mno", "start_commit_sha": "pqr"}
		]`)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	versions, err := client.GetMRVersions(context.Background(), "proj", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("got %d versions, want 2", len(versions))
	}
	if versions[0].HeadSHA != "abc" || versions[0].BaseSHA != "def" || versions[0].StartSHA != "ghi" {
		t.Errorf("version 0 mapping incorrect: %+v", versions[0])
	}
	if versions[1].HeadSHA != "jkl" {
		t.Errorf("version 1 mapping incorrect: %+v", versions[1])
	}
}

func TestResolveDiscussion_Success(t *testing.T) {
	var gotMethod string
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	err := client.ResolveDiscussion(context.Background(), "proj", "1", "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want %q", gotMethod, http.MethodPut)
	}
	if !strings.Contains(gotBody, `"resolved":true`) {
		t.Errorf("body = %q, want it to contain resolved:true", gotBody)
	}
}

func TestResolvePreviousReviews(t *testing.T) {
	var mu sync.Mutex
	resolvedIDs := make(map[string]bool)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = fmt.Fprint(w, `[
				{"id": "d1", "notes": [{"id": 1, "body": "bot comment\n<!-- code-reviewer -->"}]},
				{"id": "d2", "notes": [{"id": 2, "body": "human comment, no marker"}]},
				{"id": "d3", "notes": [{"id": 3, "body": "another bot\n<!-- code-reviewer -->"}]}
			]`)
			return
		}
		if r.Method == http.MethodPut {
			parts := strings.Split(r.URL.Path, "/")
			id := parts[len(parts)-1]
			mu.Lock()
			resolvedIDs[id] = true
			mu.Unlock()
			_, _ = fmt.Fprint(w, `{}`)
			return
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	count, err := client.ResolvePreviousReviews(context.Background(), "proj", "1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("resolved count = %d, want 2", count)
	}
	mu.Lock()
	defer mu.Unlock()
	if !resolvedIDs["d1"] {
		t.Errorf("d1 was not resolved")
	}
	if !resolvedIDs["d3"] {
		t.Errorf("d3 was not resolved")
	}
	if resolvedIDs["d2"] {
		t.Errorf("d2 should not be resolved")
	}
}

func TestListDiscussions_Pagination(t *testing.T) {
	var mu sync.Mutex
	requestCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		reqNum := requestCount
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		if reqNum == 1 {
			page2URL := fmt.Sprintf("http://%s%s?per_page=100&sort=asc&page=2", r.Host, r.URL.Path)
			w.Header().Set("Link", fmt.Sprintf("<%s>; rel=\"next\"", page2URL))
			_, _ = fmt.Fprint(w, `[{"id": "d1"}, {"id": "d2"}]`)
		} else {
			_, _ = fmt.Fprint(w, `[{"id": "d3"}]`)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	got, err := client.ListDiscussions(context.Background(), "proj", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	totalRequests := requestCount
	mu.Unlock()

	if totalRequests != 2 {
		t.Errorf("expected 2 HTTP requests, got %d", totalRequests)
	}

	if len(got) != 3 {
		t.Fatalf("got %d discussions, want 3", len(got))
	}
}

func ptr[T any](v T) *T {
	return &v
}

func TestCreateDiscussion_MultiLine(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	req := vcs.InlineCommentRequest{
		Body: "multi-line",
		Position: &vcs.InlineCommentPosition{
			NewLine: ptr(10),
			EndLine: ptr(15),
		},
	}
	err := client.CreateDiscussion(context.Background(), "proj", "1", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(gotBody, `"line_range"`) {
		t.Errorf("body missing line_range: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"new_line":10`) || !strings.Contains(gotBody, `"new_line":15`) {
		t.Errorf("body missing start or end line: %s", gotBody)
	}
}

func TestSubmitReview_MultiLineDraftNote(t *testing.T) {
	var gotDraft string
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/notes"):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/draft_notes"):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/bulk_publish"):
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/draft_notes"):
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), "suggestion") {
				gotDraft = string(body)
			}
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	req := vcs.SubmitReviewRequest{
		Summary: "Review",
		Version: &vcs.DiffVersion{HeadSHA: "h", BaseSHA: "b", StartSHA: "s"},
		Comments: []vcs.ReviewComment{
			{Path: "a.go", Line: 10, EndLine: 15, Body: "fix", Suggestion: "better code"},
		},
	}
	if err := client.SubmitReview(context.Background(), "proj", "1", req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(gotDraft, `"line_range"`) {
		t.Errorf("draft missing line_range: %s", gotDraft)
	}
	if !strings.Contains(gotDraft, "suggestion:-5+0") {
		t.Errorf("draft missing suggestion format: %s", gotDraft)
	}
}

func TestSubmitReview_SingleLineSuggestion(t *testing.T) {
	var gotDraft string
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/notes"):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/draft_notes"):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/bulk_publish"):
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/draft_notes"):
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), "suggestion") {
				gotDraft = string(body)
			}
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	req := vcs.SubmitReviewRequest{
		Summary: "Review",
		Version: &vcs.DiffVersion{HeadSHA: "h", BaseSHA: "b", StartSHA: "s"},
		Comments: []vcs.ReviewComment{
			{Path: "a.go", Line: 10, Body: "fix", Suggestion: "code"},
		},
	}
	if err := client.SubmitReview(context.Background(), "proj", "1", req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(gotDraft, "suggestion:-0+0") {
		t.Errorf("draft missing single line suggestion format: %s", gotDraft)
	}
}

// positionOf decodes the "position" object of a draft note or discussion body.
func positionOf(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var payload struct {
		Position map[string]any `json:"position"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decoding request body %s: %v", body, err)
	}
	return payload.Position
}

func TestSubmitReview_InlinePositionLineNumbers(t *testing.T) {
	// GitLab requires both old_line and new_line for an unchanged line,
	// new_line only for an added line.
	tests := []struct {
		name        string
		comment     vcs.ReviewComment
		wantNewLine float64
		wantOldLine float64 // 0 means the key must be absent.
		wantOldPath string
	}{
		{
			name:        "added line sends new_line only",
			comment:     vcs.ReviewComment{Path: "a.go", Line: 28, Body: "x"},
			wantNewLine: 28,
			wantOldPath: "a.go",
		},
		{
			name:        "unchanged line sends old_line and new_line",
			comment:     vcs.ReviewComment{Path: "a.go", Line: 27, OldLine: 24, Body: "x"},
			wantNewLine: 27,
			wantOldLine: 24,
			wantOldPath: "a.go",
		},
		{
			name:        "renamed file sends the pre-rename path as old_path",
			comment:     vcs.ReviewComment{Path: "new.go", OldPath: "old.go", Line: 27, OldLine: 24, Body: "x"},
			wantNewLine: 27,
			wantOldLine: 24,
			wantOldPath: "old.go",
		},
	}

	for _, tt := range tests {
		for _, path := range []string{"draft_notes", "individual_comments"} {
			t.Run(tt.name+"/"+path, func(t *testing.T) {
				var got map[string]any
				var mu sync.Mutex

				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					defer mu.Unlock()
					w.Header().Set("Content-Type", "application/json")
					body, _ := io.ReadAll(r.Body)
					switch {
					case r.Method == http.MethodGet:
						_, _ = w.Write([]byte(`[]`))
					case strings.Contains(r.URL.Path, "/draft_notes") && !strings.Contains(r.URL.Path, "bulk_publish"):
						if path == "individual_comments" {
							w.WriteHeader(http.StatusNotFound)
							return
						}
						if pos := positionOf(t, body); pos != nil {
							got = pos
						}
						_, _ = w.Write([]byte(`{"id":1}`))
					case strings.Contains(r.URL.Path, "/discussions"):
						got = positionOf(t, body)
						_, _ = w.Write([]byte(`{}`))
					default:
						_, _ = w.Write([]byte(`{"id":1}`))
					}
				}))
				defer srv.Close()

				client := NewClient(srv.URL, "token")
				req := vcs.SubmitReviewRequest{
					Summary:  "Review",
					Version:  &vcs.DiffVersion{HeadSHA: "h", BaseSHA: "b", StartSHA: "s"},
					Comments: []vcs.ReviewComment{tt.comment},
				}
				if err := client.SubmitReview(context.Background(), "proj", "1", req); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				mu.Lock()
				defer mu.Unlock()
				if got == nil {
					t.Fatal("no inline position was sent")
				}
				if got["new_line"] != tt.wantNewLine {
					t.Errorf("new_line = %v, want %v", got["new_line"], tt.wantNewLine)
				}
				oldLine, hasOld := got["old_line"]
				if tt.wantOldLine == 0 && hasOld {
					t.Errorf("old_line = %v, want it omitted", oldLine)
				}
				if tt.wantOldLine != 0 && oldLine != tt.wantOldLine {
					t.Errorf("old_line = %v, want %v", oldLine, tt.wantOldLine)
				}
				if got["old_path"] != tt.wantOldPath {
					t.Errorf("old_path = %v, want %v", got["old_path"], tt.wantOldPath)
				}
			})
		}
	}
}

func TestSplitRawDiff(t *testing.T) {
	const hunk = "@@ -1 +1 @@\n-a\n+b\n"
	raw := "diff --git a/plain.go b/plain.go\nindex 1..2 100644\n--- a/plain.go\n+++ b/plain.go\n" + hunk +
		"diff --git \"a/caf\\303\\251.go\" \"b/caf\\303\\251.go\"\nindex 1..2 100644\n--- \"a/caf\\303\\251.go\"\n+++ \"b/caf\\303\\251.go\"\n@@ -1 +1 @@\n-q\n+r\n" +
		"diff --git a/with space.go b/with space.go\nindex 1..2 100644\n--- a/with space.go\n+++ b/with space.go\n@@ -1 +1 @@\n-s\n+t\n" +
		"diff --git a/x b/y b/x b/y b/z.go\nsimilarity index 100%\nrename from x b/y\nrename to x b/y b/z.go\n" +
		"diff --git a/old.go b/new.go\nsimilarity index 100%\nrename from old.go\nrename to new.go\n" +
		"diff --git a/img.png b/img.png\nindex 1..2 100644\nBinary files a/img.png and b/img.png differ\n" +
		"diff --git a/last.go b/last.go\nindex 1..2 100644\n--- a/last.go\n+++ b/last.go\n@@ -1 +1 @@\n-l\n+m\n"

	got := splitRawDiff(raw)

	tests := []struct {
		key  string
		want string
	}{
		{"plain.go\x00plain.go", hunk},
		{"café.go\x00café.go", "@@ -1 +1 @@\n-q\n+r\n"},
		{"with space.go\x00with space.go", "@@ -1 +1 @@\n-s\n+t\n"},
		{"x b/y\x00x b/y b/z.go", ""}, // path containing " b/", rename with no hunks
		{"old.go\x00new.go", ""},
		{"img.png\x00img.png", ""},
		{"last.go\x00last.go", "@@ -1 +1 @@\n-l\n+m\n"},
	}
	for _, tt := range tests {
		v, ok := got[tt.key]
		if !ok {
			t.Errorf("%q not listed; keys: %q", tt.key, got)
			continue
		}
		if v != tt.want {
			t.Errorf("patch for %q = %q, want %q", tt.key, v, tt.want)
		}
	}
	// The quoted file's hunks must not be glued onto the file before it.
	if strings.Contains(got["plain.go\x00plain.go"], "-q") {
		t.Error("quoted file's hunk leaked into the previous file")
	}
}

func TestGetMRChanges_QuotedPathDoesNotMarkPreviousFileRecovered(t *testing.T) {
	raw := "diff --git a/first.go b/first.go\n--- a/first.go\n+++ b/first.go\n@@ -1 +1 @@\n-a\n+b\n" +
		"diff --git \"a/caf\\303\\251.go\" \"b/caf\\303\\251.go\"\n--- \"a/caf\\303\\251.go\"\n+++ \"b/caf\\303\\251.go\"\n@@ -1 +1 @@\n-c\n+d\n"
	m := &mrServer{
		meta: `{"id":1,"iid":1,"changes_count":"2"}`,
		diffs: []string{
			diffJSON("first.go", "", `,"collapsed":true`),
			diffJSON("café.go", "", `,"collapsed":true`),
		},
		raw: raw,
	}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()

	got, err := NewClient(srv.URL, "t").GetMRChanges(context.Background(), "p", "1")
	if err != nil {
		t.Fatal(err)
	}
	if want := "@@ -1 +1 @@\n-a\n+b\n"; got.Changes[0].Diff != want {
		t.Errorf("first.go diff = %q, want %q", got.Changes[0].Diff, want)
	}
	if want := "@@ -1 +1 @@\n-c\n+d\n"; got.Changes[1].Diff != want || got.Changes[1].Incomplete {
		t.Errorf("café.go = %+v, want recovered %q", got.Changes[1], want)
	}
}

func TestGetMRChanges_TruncatedRawDiffsKeepsEntriesIncomplete(t *testing.T) {
	prev := maxResponseBytes
	maxResponseBytes = 400
	defer func() { maxResponseBytes = prev }()

	m := &mrServer{
		meta:  `{"id":1,"iid":1,"changes_count":"1"}`,
		diffs: []string{diffJSON("big.go", "", `,"collapsed":true`)},
		raw:   "diff --git a/big.go b/big.go\n--- a/big.go\n+++ b/big.go\n@@ -1 +1 @@\n" + strings.Repeat("+line\n", 200),
	}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()

	got, err := NewClient(srv.URL, "t").GetMRChanges(context.Background(), "p", "1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Changes[0].Incomplete || got.Changes[0].Diff != "" {
		t.Errorf("truncated raw_diffs must not recover anything, got %+v", got.Changes[0])
	}
}

func TestGetMRChanges_MissingFiles(t *testing.T) {
	tests := []struct {
		count string
		want  int
	}{
		{"5", 4},
		{"1", 0},
		{"", 0},
		{"1000+", 0},
	}
	for _, tt := range tests {
		t.Run("count="+tt.count, func(t *testing.T) {
			m := &mrServer{
				meta:  fmt.Sprintf(`{"id":1,"iid":1,"changes_count":%q}`, tt.count),
				diffs: []string{diffJSON("a.go", "@@ -1 +1 @@\n-a\n+b\n", "")},
			}
			srv := httptest.NewServer(m.handler(t))
			defer srv.Close()
			got, err := NewClient(srv.URL, "t").GetMRChanges(context.Background(), "p", "1")
			if err != nil {
				t.Fatal(err)
			}
			if got.MissingFiles != tt.want {
				t.Errorf("MissingFiles = %d, want %d", got.MissingFiles, tt.want)
			}
		})
	}
}

// legacyDiff builds an entry as served by GitLab < 18.4: modes present, no
// collapsed/too_large fields.
func legacyDiff(path, diff, extra string) string {
	b, _ := json.Marshal(diff)
	return fmt.Sprintf(`{"old_path":%q,"new_path":%q,"a_mode":"100644","b_mode":"100644","diff":%s%s}`, path, path, b, extra)
}

func TestGetMRChanges_LegacyGitLabEmptyDiffs(t *testing.T) {
	raw := "diff --git a/big.go b/big.go\n--- a/big.go\n+++ b/big.go\n@@ -1 +1 @@\n-o\n+n\n" +
		"diff --git a/img.png b/img.png\nindex 1..2 100644\nBinary files a/img.png and b/img.png differ\n"
	m := &mrServer{
		meta: `{"id":1,"iid":1,"changes_count":"6"}`,
		diffs: []string{
			legacyDiff("big.go", "", ""),                 // blanked by the size limit: recoverable
			legacyDiff("img.png", "", ""),                // binary: legitimately empty
			legacyDiff("gone.go", "", ""),                // blanked and absent from raw_diffs: unreviewed
			legacyDiff("new.go", "", `,"new_file":true`), // empty added file
			`{"old_path":"a.go","new_path":"b.go","a_mode":"100644","b_mode":"100644","diff":"","renamed_file":true}`,
			legacyDiff("ok.go", "@@ -1 +1 @@\n-x\n+y\n", ""),
		},
		raw: raw,
	}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()

	got, err := NewClient(srv.URL, "t").GetMRChanges(context.Background(), "p", "1")
	if err != nil {
		t.Fatal(err)
	}
	wantIncomplete := []bool{false, false, true, false, false, false}
	for i, w := range wantIncomplete {
		if got.Changes[i].Incomplete != w {
			t.Errorf("%s: Incomplete = %v, want %v", got.Changes[i].NewPath, got.Changes[i].Incomplete, w)
		}
	}
	if got.Changes[0].Diff == "" {
		t.Error("big.go should have been recovered from raw_diffs")
	}
}

func TestGetMRChanges_NoSuspicionWhenFlagsPresent(t *testing.T) {
	m := &mrServer{
		meta:  `{"id":1,"iid":1,"changes_count":"1"}`,
		diffs: []string{legacyDiff("img.png", "", `,"collapsed":false,"too_large":false`)},
	}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()

	got, err := NewClient(srv.URL, "t").GetMRChanges(context.Background(), "p", "1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Changes[0].Incomplete || m.rawCalls != 0 {
		t.Errorf("flags say the empty diff is real; Incomplete=%v rawCalls=%d", got.Changes[0].Incomplete, m.rawCalls)
	}
}
