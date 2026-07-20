package reviewer

import (
	"context"
	"strings"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/config"
	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

// explainMockModel implements ModelReviewer and ExplainProvider.
type explainMockModel struct {
	explanation    string
	usage          *model.TokenUsage
	err            error
	explainCalled  bool
}

func (m *explainMockModel) Review(_ context.Context, _, _ string) (*model.ReviewResult, error) {
	return &model.ReviewResult{Summary: "OK"}, nil
}

func (m *explainMockModel) Close() {}

func (m *explainMockModel) Explain(_ context.Context, _, _ string) (string, *model.TokenUsage, error) {
	m.explainCalled = true
	return m.explanation, m.usage, m.err
}

func TestRunExplain_ProducesOutput(t *testing.T) {
	mock := &explainMockModel{
		explanation: "This diff adds a login handler that validates JWT tokens.",
		usage:       &model.TokenUsage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150},
	}
	cfg := &config.Config{
		Explain:       true,
		DryRun:        true,
		Focus:         []string{"bugs"},
		ChunkStrategy: config.ChunkStrategySplit,
		MinSeverity:   config.SeverityLow,
	}
	diffs := []diff.FileDiff{{
		NewPath: "auth/handler.go",
		Hunks: []diff.Hunk{{
			OldStart: 1, OldCount: 0, NewStart: 1, NewCount: 2,
			Lines: []diff.DiffLine{
				{Type: diff.LineAdded, NewLineNo: 1, Content: "func Login() {}"},
				{Type: diff.LineAdded, NewLineNo: 2, Content: "// validates JWT"},
			},
		}},
	}}
	rev := NewWithDiffSource(cfg, mock, nil, &intentMockDiffSource{
		diffs: diffs, title: "Add login", desc: "",
	})
	exitCode, err := rev.RunExplain(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !mock.explainCalled {
		t.Error("expected Explain to be called")
	}
}

func TestRunExplain_ProviderNotSupported(t *testing.T) {
	// Use a mock that does NOT implement ExplainProvider.
	mock := &intentMockModel{
		reviewResult: &model.ReviewResult{Summary: "OK"},
	}
	cfg := &config.Config{
		Explain:       true,
		DryRun:        true,
		Focus:         []string{"bugs"},
		ChunkStrategy: config.ChunkStrategySplit,
		MinSeverity:   config.SeverityLow,
	}
	diffs := []diff.FileDiff{{
		NewPath: "test.go",
		Hunks: []diff.Hunk{{
			OldStart: 1, OldCount: 0, NewStart: 1, NewCount: 1,
			Lines: []diff.DiffLine{
				{Type: diff.LineAdded, NewLineNo: 1, Content: "// test"},
			},
		}},
	}}
	rev := NewWithDiffSource(cfg, mock, nil, &intentMockDiffSource{
		diffs: diffs, title: "test", desc: "",
	})
	_, err := rev.RunExplain(context.Background())
	if err == nil {
		t.Fatal("expected error when provider doesn't implement ExplainProvider")
	}
	if !strings.Contains(err.Error(), "does not support explain") {
		t.Errorf("error = %q, want mention of 'does not support explain'", err.Error())
	}
}

func TestFormatExplainTerminal(t *testing.T) {
	explanation := "This adds a login endpoint."
	result := formatExplainTerminal(explanation, false)
	if !strings.Contains(result, "🔍 Explanation") {
		t.Error("expected header")
	}
	if !strings.Contains(result, "login endpoint") {
		t.Error("expected explanation content")
	}
}

func TestFormatExplainMarkdown(t *testing.T) {
	explanation := "This refactors the auth module."
	result := formatExplainMarkdown(explanation)
	if !strings.Contains(result, "## 🔍 Diff Explanation") {
		t.Error("expected markdown header")
	}
	if !strings.Contains(result, "auth module") {
		t.Error("expected explanation content")
	}
}

func TestStripControlChars_PreservesNormal(t *testing.T) {
	input := "Hello\nWorld\t!"
	result := stripControlChars(input)
	if result != input {
		t.Errorf("expected %q, got %q", input, result)
	}
}

func TestStripControlChars_RemovesESC(t *testing.T) {
	// ESC[ is CSI (Control Sequence Introducer) used for terminal manipulation.
	input := "before\x1b[31mred\x1b[0mafter"
	result := stripControlChars(input)
	if strings.Contains(result, "\x1b") {
		t.Errorf("expected ESC sequences removed, got %q", result)
	}
	if !strings.Contains(result, "before") || !strings.Contains(result, "after") {
		t.Error("expected normal text preserved")
	}
}

func TestStripControlChars_RemovesOSC(t *testing.T) {
	// OSC (Operating System Command) can change terminal title.
	input := "normal\x1b]0;malicious title\x07rest"
	result := stripControlChars(input)
	if strings.Contains(result, "\x1b") || strings.Contains(result, "\x07") {
		t.Errorf("expected control chars removed, got %q", result)
	}
}

func TestFormatExplainTerminal_SanitizesControlChars(t *testing.T) {
	// Model output with embedded CSI sequence.
	explanation := "This is safe\x1b[2J\x1b[H but this clears screen"
	result := formatExplainTerminal(explanation, false)
	if strings.Contains(result, "\x1b") {
		t.Error("expected control sequences stripped from terminal output")
	}
	if !strings.Contains(result, "This is safe") {
		t.Error("expected safe content preserved")
	}
}
