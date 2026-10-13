package reviewer

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/OpticDiff/code-reviewer/internal/config"
	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
	"github.com/OpticDiff/code-reviewer/internal/vcs"
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

	lines := bytes.Count(data, []byte("\n"))

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

	entry := buildAuditEntry(cfg, diffs, skipped, findings, 0, 0, usage, duration)

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

	entry := buildAuditEntry(cfg, nil, nil, findings, 0, 0, nil, 0)

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

func TestAuditEntry_IncludesProfile(t *testing.T) {
	cfg := &config.Config{
		Profile: "platform",
	}

	entry := buildAuditEntry(cfg, nil, nil, nil, 0, 0, nil, 0)

	if entry.Profile != "platform" {
		t.Errorf("buildAuditEntry() profile = %q, want %q", entry.Profile, "platform")
	}
}

func TestAuditEntry_SuggestionsDropped(t *testing.T) {
	data, err := json.Marshal(AuditEntry{SuggestionsDropped: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"suggestions_dropped":2`)) {
		t.Errorf("audit entry missing suggestions_dropped: %s", data)
	}
}

func TestAuditSuppressed_RecordsEachSuppressedFinding(t *testing.T) {
	high := model.Finding{File: "main.go", Line: 2, Severity: "HIGH", Category: "security", RuleName: "no-eval", Fingerprint: "aa"}
	removed := model.Finding{File: "old.go", OldLine: 7, Severity: "LOW", Category: "bug", Fingerprint: "bb"}
	dismissed := []vcs.DismissedFinding{
		{Fingerprint: "aa", NoteID: 41},
		{Fingerprint: "bb", NoteID: 42, AuthorOnly: true},
	}

	got, counts := auditSuppressed([]model.Finding{high, removed}, dismissed)

	want := []SuppressedEntry{
		{Fingerprint: "aa", Severity: "high", Category: "security", File: "main.go", Line: 2, RuleName: "no-eval", DismissedNoteID: 41},
		{Fingerprint: "bb", Severity: "low", Category: "bug", File: "old.go", OldLine: 7, DismissedNoteID: 42, AuthorOnly: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("suppressed = %+v, want %+v", got, want)
	}
	if !reflect.DeepEqual(counts, map[string]int{"HIGH": 1, "LOW": 1}) {
		t.Errorf("suppressed_severity_counts = %v, want HIGH:1 LOW:1 keyed like severity_counts", counts)
	}
}

func TestAuditSuppressed_NothingSuppressed(t *testing.T) {
	got, counts := auditSuppressed(nil, []vcs.DismissedFinding{{Fingerprint: "aa", NoteID: 1}})
	if got != nil || counts != nil {
		t.Errorf("got %v, %v; want nil so the fields are omitted", got, counts)
	}
}

func TestAuditSuppressed_UsesTheStrongerDismissalWhenThreadsShareAFingerprint(t *testing.T) {
	f := model.Finding{File: "main.go", Line: 2, Severity: "HIGH", Category: "bug", Fingerprint: "aa"}
	dismissed := []vcs.DismissedFinding{
		{Fingerprint: "aa", NoteID: 10, AuthorOnly: true},
		{Fingerprint: "aa", NoteID: 11},
	}
	got, _ := auditSuppressed([]model.Finding{f}, dismissed)
	if len(got) != 1 || got[0].DismissedNoteID != 11 || got[0].AuthorOnly {
		t.Errorf("got %+v, want the non-author thread 11", got)
	}
}

func TestAuditEntry_SuppressedFieldsAreOmittedWhenEmptyAndSerializedWhenSet(t *testing.T) {
	var entry AuditEntry
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("suppressed")) {
		t.Errorf("empty entry must omit suppressed fields: %s", data)
	}

	entry.Suppressed, entry.SuppressedSeverityCounts = auditSuppressed(
		[]model.Finding{{File: "main.go", Line: 2, Severity: "CRITICAL", Category: "bug", Fingerprint: "aa"}},
		[]vcs.DismissedFinding{{Fingerprint: "aa", NoteID: 5}})
	data, err = json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	items, _ := raw["suppressed"].([]any)
	if len(items) != 1 {
		t.Fatalf("suppressed = %v", raw["suppressed"])
	}
	item := items[0].(map[string]any)
	for _, key := range []string{"fingerprint", "severity", "category", "file", "line", "dismissed_note_id", "author_only"} {
		if _, ok := item[key]; !ok {
			t.Errorf("suppressed item lacks %q: %v", key, item)
		}
	}
	for _, key := range []string{"old_line", "rule_name"} {
		if _, ok := item[key]; ok {
			t.Errorf("suppressed item must omit empty %q: %v", key, item)
		}
	}
	if counts, _ := raw["suppressed_severity_counts"].(map[string]any); counts["CRITICAL"] != float64(1) {
		t.Errorf("suppressed_severity_counts = %v", raw["suppressed_severity_counts"])
	}
}

func TestRun_AuditLogRecordsSuppressedHighAndKeepsSeverityCountsToPostedFindings(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "audit.jsonl")
	finding := model.Finding{File: "main.go", Line: 2, Severity: "HIGH", Category: "bug", Title: "Unchecked value", Body: "details"}
	posted := fingerprinted(finding, dismissedTestDiffs("x := compute()"))
	cfg := dismissedRunCfg()
	cfg.AuditLog = logPath
	client := &threadVCS{
		mockVCS:   &mockVCS{mrVersions: []vcs.DiffVersion{{ID: 1, HeadSHA: "a", BaseSHA: "b", StartSHA: "c"}}},
		dismissed: []vcs.DismissedFinding{{Fingerprint: posted.Fingerprint, Anchor: posted.Anchor, AnchorLines: posted.AnchorLines, Path: "main.go", Line: 2, NoteID: 77}},
	}
	mm := &mockModel{result: &model.ReviewResult{Summary: "s", Findings: []model.Finding{finding}}}
	r := NewWithDiffSource(cfg, mm, client, &mockDiffSource{diffs: dismissedTestDiffs("x := compute()")})

	count, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want the suppressed HIGH to keep failing the gate", count)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	var entry AuditEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatal(err)
	}
	if entry.SeverityCounts["HIGH"] != 0 || entry.FindingsCount != 0 {
		t.Errorf("severity_counts = %v findings_count = %d; want posted findings only", entry.SeverityCounts, entry.FindingsCount)
	}
	if len(entry.Suppressed) != 1 || entry.Suppressed[0].DismissedNoteID != 77 || entry.Suppressed[0].Severity != "high" || entry.Suppressed[0].Fingerprint != posted.Fingerprint {
		t.Errorf("suppressed = %+v", entry.Suppressed)
	}
	if entry.SuppressedSeverityCounts["HIGH"] != 1 {
		t.Errorf("suppressed_severity_counts = %v", entry.SuppressedSeverityCounts)
	}
}
