package reviewer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OpticDiff/code-reviewer/internal/config"
	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

func TestWriteAuditLog_CreatesFile(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "audit.jsonl")

	entry := AuditEntry{
		Timestamp:  time.Now().UTC(),
		DurationMs: 100,
		Model:      "test-model",
	}

	err := WriteAuditLog(logPath, entry)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	var readEntry AuditEntry
	if err := json.Unmarshal(data, &readEntry); err != nil {
		t.Fatalf("failed to unmarshal log line: %v", err)
	}

	if readEntry.Model != "test-model" {
		t.Errorf("expected model 'test-model', got '%s'", readEntry.Model)
	}
}

func TestWriteAuditLog_Appends(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "audit.jsonl")

	entry1 := AuditEntry{Model: "model-1"}
	entry2 := AuditEntry{Model: "model-2"}

	if err := WriteAuditLog(logPath, entry1); err != nil {
		t.Fatalf("failed to write first entry: %v", err)
	}
	if err := WriteAuditLog(logPath, entry2); err != nil {
		t.Fatalf("failed to write second entry: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	// Read both lines (might contain an extra empty line)
	lines := 0
	for i, ch := range string(data) {
		if ch == '\n' {
			lines++
			if i == len(string(data))-1 {
				// Don't count trailing newline as a distinct line for data
			}
		}
	}
	
	if lines != 2 {
		t.Errorf("expected 2 lines, got %d lines", lines)
	}
}

func TestBuildAuditEntry(t *testing.T) {
	cfg := &config.Config{
		DiffMode:         true,
		Model:            "gemini-test",
		Incremental:      true,
		CIMode:           true,
		CIProjectID:      "123",
		CIMergeRequestID: "456",
	}

	diffs := []diff.FileDiff{
		{NewPath: "main.go"},
		{NewPath: "utils.go"},
	}
	skipped := []string{"big_file.go"}
	
	findings := []model.Finding{
		{Severity: "HIGH"},
		{Severity: "LOW"},
	}

	usage := &model.TokenUsage{TotalTokens: 1500}
	duration := 500 * time.Millisecond

	entry := buildAuditEntry(cfg, diffs, skipped, findings, usage, duration)

	if entry.Model != "gemini-test" {
		t.Errorf("expected model gemini-test, got %s", entry.Model)
	}
	if entry.DurationMs != 500 {
		t.Errorf("expected duration 500ms, got %dms", entry.DurationMs)
	}
	if entry.ProjectID != "123" {
		t.Errorf("expected project ID 123, got %s", entry.ProjectID)
	}
	if entry.MRID != "456" {
		t.Errorf("expected MR ID 456, got %s", entry.MRID)
	}
	if len(entry.FilesReviewed) != 2 {
		t.Errorf("expected 2 files reviewed, got %d", len(entry.FilesReviewed))
	}
	if len(entry.FilesSkipped) != 1 {
		t.Errorf("expected 1 file skipped, got %d", len(entry.FilesSkipped))
	}
	if entry.FindingsCount != 2 {
		t.Errorf("expected 2 findings, got %d", entry.FindingsCount)
	}
	if !entry.Incremental {
		t.Error("expected incremental to be true")
	}
	if entry.Usage == nil || entry.Usage.TotalTokens != 1500 {
		t.Error("expected usage total tokens 1500")
	}
}

func TestBuildAuditEntry_SeverityCounts(t *testing.T) {
	cfg := &config.Config{}
	findings := []model.Finding{
		{Severity: "HIGH"},
		{Severity: "HIGH"},
		{Severity: "MEDIUM"},
		{Severity: "LOW"},
		{Severity: "LOW"},
		{Severity: "LOW"},
	}

	entry := buildAuditEntry(cfg, nil, nil, findings, nil, 0)

	if entry.SeverityCounts["HIGH"] != 2 {
		t.Errorf("expected 2 HIGH, got %d", entry.SeverityCounts["HIGH"])
	}
	if entry.SeverityCounts["MEDIUM"] != 1 {
		t.Errorf("expected 1 MEDIUM, got %d", entry.SeverityCounts["MEDIUM"])
	}
	if entry.SeverityCounts["LOW"] != 3 {
		t.Errorf("expected 3 LOW, got %d", entry.SeverityCounts["LOW"])
	}
}
