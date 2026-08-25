package reviewer

import (
	"encoding/json"
	"os"
	"time"

	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/config"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

// AuditEntry is a single structured log record written after each review run.
type AuditEntry struct {
	Timestamp      time.Time          `json:"timestamp"`
	DurationMs     int64              `json:"duration_ms"`
	Version        string             `json:"version"`
	Mode           string             `json:"mode"`
	Model          string             `json:"model"`
	ProjectID      string             `json:"project_id,omitempty"`
	MRID           string             `json:"mr_id,omitempty"`
	FilesReviewed  []string           `json:"files_reviewed"`
	FilesSkipped   []string           `json:"files_skipped,omitempty"`
	FindingsCount  int                `json:"findings_count"`
	SeverityCounts map[string]int     `json:"severity_counts"`
	Usage          *model.TokenUsage  `json:"usage,omitempty"`
	Incremental    bool               `json:"incremental,omitempty"`
}

// buildAuditEntry constructs an AuditEntry from the review run data.
func buildAuditEntry(cfg *config.Config, diffs []diff.FileDiff, skippedFiles []string, findings []model.Finding, usage *model.TokenUsage, duration time.Duration) AuditEntry {
	files := make([]string, 0, len(diffs))
	for _, d := range diffs {
		path := d.NewPath
		if path == "" {
			path = d.OldPath
		}
		files = append(files, path)
	}

	severityCounts := make(map[string]int)
	for _, f := range findings {
		severityCounts[f.Severity]++
	}

	entry := AuditEntry{
		Timestamp:      time.Now().UTC(),
		DurationMs:     duration.Milliseconds(),
		Mode:           cfg.Mode(),
		Model:          cfg.Model,
		FilesReviewed:  files,
		FilesSkipped:   skippedFiles,
		FindingsCount:  len(findings),
		SeverityCounts: severityCounts,
		Incremental:    cfg.Incremental,
	}

	if cfg.CIMode {
		entry.ProjectID = cfg.CIProjectID
		entry.MRID = cfg.CIMergeRequestID
	}

	if usage != nil && usage.TotalTokens > 0 {
		entry.Usage = usage
	}

	return entry
}

// WriteAuditLog appends a single JSON line to the audit log file.
func WriteAuditLog(path string, entry AuditEntry) (retErr error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := f.Close(); retErr == nil {
			retErr = closeErr
		}
	}()

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = f.Write(data)
	return err
}
