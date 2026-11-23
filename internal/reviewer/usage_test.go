package reviewer

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/config"
	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
	"google.golang.org/genai"
)

// usageChatter answers each turn with the next reply and usage metadata.
type usageChatter struct {
	replies []string
	usages  []*genai.GenerateContentResponseUsageMetadata
	calls   int
}

func (c *usageChatter) Chat(context.Context, []*genai.Content, *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	i := c.calls
	c.calls++
	return &genai.GenerateContentResponse{
		Candidates:    []*genai.Candidate{{Content: genai.NewContentFromText(c.replies[i], genai.RoleModel)}},
		UsageMetadata: c.usages[i],
	}, nil
}

// TestRunAgentLoop_SumsCachedAndThinkingTokens checks that every turn of the
// loop adds its cached and thinking counts, including turns without metadata.
func TestRunAgentLoop_SumsCachedAndThinkingTokens(t *testing.T) {
	initial := []model.Finding{{File: "a.go", Line: 1, Severity: "HIGH", Title: "kept"}}
	refined, _ := json.Marshal(initial)
	chatter := &usageChatter{
		replies: []string{
			`{"action":"tool","tool":"read_file","args":{"path":"present.txt"}}`,
			`{"action":"tool","tool":"read_file","args":{"path":"present.txt"}}`,
			`{"action":"findings","findings":` + string(refined) + `}`,
		},
		usages: []*genai.GenerateContentResponseUsageMetadata{
			{PromptTokenCount: 1000, CachedContentTokenCount: 0, CandidatesTokenCount: 20, ThoughtsTokenCount: 100, TotalTokenCount: 1120},
			nil,
			{PromptTokenCount: 1500, CachedContentTokenCount: 1000, CandidatesTokenCount: 30, ThoughtsTokenCount: 50, TotalTokenCount: 1580},
		},
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "present.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := RunAgentLoop(context.Background(), chatter, AgentConfig{RepoRoot: root, MaxIterations: 5, RemainingBudget: 1 << 30}, initial)
	if err != nil {
		t.Fatalf("RunAgentLoop: %v", err)
	}
	want := model.TokenUsage{InputTokens: 2500, CachedInputTokens: 1000, OutputTokens: 50, ThinkingTokens: 150, TotalTokens: 2700}
	if res.Usage != want {
		t.Errorf("usage = %+v, want %+v", res.Usage, want)
	}
}

// TestRun_ReportsCachedAndThinkingTokens runs two chunks and checks that the
// JSON report and the audit log carry the summed counts.
func TestRun_ReportsCachedAndThinkingTokens(t *testing.T) {
	diff.ModelTokenLimits["test-tiny-usage"] = 15
	defer delete(diff.ModelTokenLimits, "test-tiny-usage")

	auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
	cfg := &config.Config{
		NoCache:       true,
		Model:         "test-tiny-usage",
		ChunkStrategy: config.ChunkStrategySplit,
		MinSeverity:   config.SeverityLow,
		DryRun:        true,
		OutputJSON:    true,
		AuditLog:      auditPath,
	}
	mm := &mockModel{result: &model.ReviewResult{
		Summary: "s",
		Usage:   &model.TokenUsage{InputTokens: 1000, CachedInputTokens: 600, OutputTokens: 40, ThinkingTokens: 25, TotalTokens: 1065},
	}}
	r := NewWithDiffSource(cfg, mm, &mockVCS{}, &mockDiffSource{diffs: makeTestDiffs("file1.go", "file2.go")})

	stdout := captureStdout(t, func() {
		if _, err := r.Run(context.Background()); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
	if mm.calls != 2 {
		t.Fatalf("model calls = %d, want 2 (one per chunk)", mm.calls)
	}

	want := model.TokenUsage{InputTokens: 2000, CachedInputTokens: 1200, OutputTokens: 80, ThinkingTokens: 50, TotalTokens: 2130}

	var report struct {
		Usage map[string]int64 `json:"usage"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("report is not JSON: %v\n%s", err, stdout)
	}
	wantKeys := map[string]int64{
		"input_tokens": want.InputTokens, "cached_input_tokens": want.CachedInputTokens,
		"output_tokens": want.OutputTokens, "thinking_tokens": want.ThinkingTokens, "total_tokens": want.TotalTokens,
	}
	if len(report.Usage) != len(wantKeys) {
		t.Errorf("report usage = %v, want exactly %v", report.Usage, wantKeys)
	}
	for k, v := range wantKeys {
		if report.Usage[k] != v {
			t.Errorf("report usage.%s = %d, want %d", k, report.Usage[k], v)
		}
	}

	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	var entry struct {
		Usage map[string]int64 `json:"usage"`
	}
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatal(err)
	}
	for k, v := range wantKeys {
		if entry.Usage[k] != v {
			t.Errorf("audit usage.%s = %d, want %d", k, entry.Usage[k], v)
		}
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = wr
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(rd)
		done <- string(b)
	}()
	defer func() { os.Stdout = old }()
	fn()
	_ = wr.Close()
	os.Stdout = old
	return <-done
}
