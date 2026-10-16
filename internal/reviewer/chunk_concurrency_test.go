package reviewer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

// funcModel answers each review with fn and tracks how many calls overlap. It
// is safe for concurrent use.
type funcModel struct {
	fn func(prompt string) (*model.ReviewResult, error)

	mu          sync.Mutex
	calls       int
	inFlight    int
	maxInFlight int
}

func (m *funcModel) Review(_ context.Context, _, userPrompt string) (*model.ReviewResult, error) {
	m.mu.Lock()
	m.calls++
	m.inFlight++
	m.maxInFlight = max(m.maxInFlight, m.inFlight)
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.inFlight--
		m.mu.Unlock()
	}()
	return m.fn(userPrompt)
}

func (m *funcModel) Close() {}

// markedDiffs returns equal-size diffs whose body carries "marker:<path>", so a
// model can tell which chunk a prompt belongs to. With chunkTestLimit each
// file is its own chunk; callers use paths of equal length so the sizes tie and
// the chunks keep the input order.
func markedDiffs(files ...string) []diff.FileDiff {
	ds := bigDiffs(files...)
	for i, f := range files {
		ds[i].Hunks[0].Lines[0].Content = "marker:" + f + strings.Repeat("x", 4000-len("marker:"+f))
	}
	return ds
}

// chunkOf returns the path whose marker is in the prompt.
func chunkOf(prompt string, files []string) string {
	for _, f := range files {
		if strings.Contains(prompt, "marker:"+f) {
			return f
		}
	}
	return ""
}

func TestReviewWave_ResultsInChunkOrderWhateverFinishesFirst(t *testing.T) {
	files := []string{"a.go", "b.go", "c.go", "d.go"}
	delay := map[string]time.Duration{"a.go": 80 * time.Millisecond, "b.go": 60 * time.Millisecond, "c.go": 30 * time.Millisecond, "d.go": 0}
	m := &funcModel{fn: func(prompt string) (*model.ReviewResult, error) {
		f := chunkOf(prompt, files)
		time.Sleep(delay[f])
		return &model.ReviewResult{Summary: f}, nil
	}}
	r := &Reviewer{provider: m}

	prompts := make([]string, len(files))
	for i, f := range files {
		prompts[i] = "marker:" + f
	}
	results, err := r.reviewWave(context.Background(), "sys", prompts, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i, f := range files {
		if results[i].Summary != f {
			t.Errorf("result %d = %q, want %q", i, results[i].Summary, f)
		}
	}
	if m.maxInFlight < 2 {
		t.Errorf("max calls in flight = %d, want the calls to overlap", m.maxInFlight)
	}
}

func TestReviewWave_ReportsLowestNumberedFailure(t *testing.T) {
	errSecond := errors.New("second failed")
	errFourth := errors.New("fourth failed")
	m := &funcModel{fn: func(prompt string) (*model.ReviewResult, error) {
		switch {
		case strings.Contains(prompt, "p2"):
			time.Sleep(50 * time.Millisecond) // Finishes after chunk 4's failure.
			return nil, errSecond
		case strings.Contains(prompt, "p4"):
			return nil, errFourth
		}
		return &model.ReviewResult{}, nil
	}}
	r := &Reviewer{provider: m}

	// The wave starts at chunk 5, so the labels are 6..9 and the failure is chunk 7.
	_, err := r.reviewWave(context.Background(), "sys", []string{"p1", "p2", "p3", "p4"}, 5)
	if !errors.Is(err, errSecond) {
		t.Fatalf("err = %v, want the second call's error", err)
	}
	if !strings.Contains(err.Error(), "model review (chunk 7)") {
		t.Errorf("err = %q, want it to name chunk 7", err)
	}
}

func TestRun_ConcurrentSummaryOrderIsChunkOrder(t *testing.T) {
	files := []string{"a/one.go", "b/two.go", "c/tri.go"}
	m := &funcModel{fn: func(prompt string) (*model.ReviewResult, error) {
		f := chunkOf(prompt, files)
		// Later chunks answer first.
		time.Sleep(time.Duration(len(files)-indexOf(files, f)) * 40 * time.Millisecond)
		return &model.ReviewResult{Summary: "summary of " + f}, nil
	}}
	cfg := chunkCfg(t)
	cfg.ChunkConcurrency = 3
	vc := &mockVCS{}
	if _, err := NewWithDiffSource(cfg, m, vc, &mockDiffSource{diffs: markedDiffs(files...)}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.calls != 3 || m.maxInFlight < 2 {
		t.Fatalf("calls = %d, max in flight = %d; expected 3 calls running concurrently", m.calls, m.maxInFlight)
	}
	want := "summary of a/one.go\n\nsummary of b/two.go\n\nsummary of c/tri.go"
	if !strings.Contains(vc.submitReviewReq.Summary, want) {
		t.Errorf("summary = %q, want chunks in order: %q", vc.submitReviewReq.Summary, want)
	}
}

func indexOf(ss []string, s string) int {
	for i, v := range ss {
		if v == s {
			return i
		}
	}
	return -1
}

func TestRun_ConcurrencyIsCappedAtK(t *testing.T) {
	files := []string{"a.go", "b.go", "c.go", "d.go", "e.go"}
	m := &funcModel{fn: func(string) (*model.ReviewResult, error) {
		time.Sleep(30 * time.Millisecond)
		return &model.ReviewResult{}, nil
	}}
	cfg := chunkCfg(t)
	cfg.ChunkConcurrency = 2
	if _, err := NewWithDiffSource(cfg, m, &mockVCS{}, &mockDiffSource{diffs: markedDiffs(files...)}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.calls != 5 {
		t.Errorf("calls = %d, want 5", m.calls)
	}
	if m.maxInFlight != 2 {
		t.Errorf("max in flight = %d, want 2", m.maxInFlight)
	}
}

func TestRun_DefaultConcurrencyIsSequential(t *testing.T) {
	files := []string{"a.go", "b.go", "c.go"}
	for _, k := range []int{0, 1} {
		var order []string
		m := &funcModel{fn: func(prompt string) (*model.ReviewResult, error) {
			order = append(order, chunkOf(prompt, files)) // No lock: calls must not overlap.
			time.Sleep(10 * time.Millisecond)
			return &model.ReviewResult{}, nil
		}}
		cfg := chunkCfg(t)
		cfg.ChunkConcurrency = k
		if _, err := NewWithDiffSource(cfg, m, &mockVCS{}, &mockDiffSource{diffs: markedDiffs(files...)}).Run(context.Background()); err != nil {
			t.Fatal(err)
		}
		if m.maxInFlight != 1 {
			t.Errorf("K=%d: max in flight = %d, want 1", k, m.maxInFlight)
		}
		if strings.Join(order, ",") != "a.go,b.go,c.go" {
			t.Errorf("K=%d: call order = %v, want a.go,b.go,c.go", k, order)
		}
	}
}

func TestRun_ConcurrentErrorIsReported(t *testing.T) {
	files := []string{"a.go", "b.go", "c.go"}
	boom := errors.New("model unavailable")
	m := &funcModel{fn: func(prompt string) (*model.ReviewResult, error) {
		if chunkOf(prompt, files) == "b.go" {
			return nil, boom
		}
		return &model.ReviewResult{}, nil
	}}
	cfg := chunkCfg(t)
	cfg.ChunkConcurrency = 3
	_, err := NewWithDiffSource(cfg, m, &mockVCS{}, &mockDiffSource{diffs: markedDiffs(files...)}).Run(context.Background())
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "model review (chunk 2)") {
		t.Fatalf("err = %v, want chunk 2's error", err)
	}
}

// With a runtime token budget, the check runs between waves: no new wave
// starts once the budget is spent, and K=1 stops exactly as before.
func TestRun_TokenBudgetStopsBetweenWaves(t *testing.T) {
	files := []string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go"}
	for _, tc := range []struct {
		k         int
		wantCalls int
	}{
		{k: 1, wantCalls: 2}, // 6000 after one chunk, 12000 after two.
		{k: 2, wantCalls: 2}, // One wave of two spends the budget.
		{k: 3, wantCalls: 3}, // The first wave of three overshoots; the second never starts.
	} {
		m := &funcModel{fn: func(string) (*model.ReviewResult, error) {
			return &model.ReviewResult{Usage: &model.TokenUsage{TotalTokens: 6000}}, nil
		}}
		cfg := chunkCfg(t)
		cfg.ChunkConcurrency = tc.k
		cfg.MaxTokens = 10000
		if _, err := NewWithDiffSource(cfg, m, &mockVCS{}, &mockDiffSource{diffs: markedDiffs(files...)}).Run(context.Background()); err != nil {
			t.Fatalf("K=%d: %v", tc.k, err)
		}
		if m.calls != tc.wantCalls {
			t.Errorf("K=%d: %d model calls, want %d", tc.k, m.calls, tc.wantCalls)
		}
	}
}

// A rate-limited call is retried by the provider, per call, so a concurrent
// run recovers from a 429 without the reviewer doing anything.
func TestRun_ConcurrentChunksRecoverFromRateLimit(t *testing.T) {
	files := []string{"a.go", "b.go", "c.go"}
	var requests, limited atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if limited.Add(1) == 1 { // Reject the first request only.
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		review, _ := json.Marshal(model.ReviewResult{Summary: "ok"})
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": string(review)}}},
		})
	}))
	defer srv.Close()

	provider, err := model.NewHTTPProvider(srv.URL+"/v1", "key", "test-model")
	if err != nil {
		t.Fatal(err)
	}
	cfg := chunkCfg(t)
	cfg.ChunkConcurrency = 3
	if _, err := NewWithDiffSource(cfg, provider, &mockVCS{}, &mockDiffSource{diffs: markedDiffs(files...)}).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := requests.Load(); got != 4 {
		t.Errorf("requests = %d, want 4 (3 chunks + 1 retry)", got)
	}
}

func TestRun_ChunkTargetTokensSplitsBelowTheContextLimit(t *testing.T) {
	files := []string{"a.go", "b.go", "c.go"}
	for _, tc := range []struct {
		target     int
		wantChunks int
	}{
		{target: 0, wantChunks: 1},    // 80% of a 100000-token limit holds all three.
		{target: 1600, wantChunks: 3}, // ~1000 tokens per file: one per chunk.
	} {
		cfg := chunkCfg(t)
		diff.ModelTokenLimits["test-chunk"] = 100000
		cfg.ChunkTargetTokens = tc.target
		m := &funcModel{fn: func(string) (*model.ReviewResult, error) { return &model.ReviewResult{}, nil }}
		if _, err := NewWithDiffSource(cfg, m, &mockVCS{}, &mockDiffSource{diffs: markedDiffs(files...)}).Run(context.Background()); err != nil {
			t.Fatalf("target %d: %v", tc.target, err)
		}
		if m.calls != tc.wantChunks {
			t.Errorf("target %d: %d chunks reviewed, want %d", tc.target, m.calls, tc.wantChunks)
		}
	}
}
