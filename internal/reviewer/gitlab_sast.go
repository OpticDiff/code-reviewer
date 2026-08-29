package reviewer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/OpticDiff/code-reviewer/internal/model"
)

type gitlabSASTReport struct {
	Version         string                `json:"version"`
	Scan            gitlabScan            `json:"scan"`
	Vulnerabilities []gitlabVulnerability `json:"vulnerabilities"`
}

type gitlabScan struct {
	Scanner   gitlabScanner `json:"scanner"`
	StartTime string        `json:"start_time"`
	EndTime   string        `json:"end_time"`
	Status    string        `json:"status"`
	Type      string        `json:"type"`
}

type gitlabScanner struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type gitlabVulnerability struct {
	ID          string             `json:"id"`
	Category    string             `json:"category"`
	Name        string             `json:"name"`
	Message     string             `json:"message"`
	Description string             `json:"description"`
	Severity    string             `json:"severity"`
	Scanner     gitlabScanner      `json:"scanner"`
	Location    gitlabLocation     `json:"location"`
	Identifiers []gitlabIdentifier `json:"identifiers"`
}

type gitlabLocation struct {
	File      string `json:"file"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line,omitempty"`
}

type gitlabIdentifier struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// WriteGitLabSAST writes the review result to a file in GitLab SAST format.
func WriteGitLabSAST(path, toolVersion string, result *model.ReviewResult) error {
	now := time.Now().UTC().Format("2006-01-02T15:04:05")

	report := gitlabSASTReport{
		Version: "15.0.0",
		Scan: gitlabScan{
			Scanner: gitlabScanner{
				ID:      "code-reviewer",
				Name:    "code-reviewer",
				Version: toolVersion,
			},
			StartTime: now,
			EndTime:   now,
			Status:    "success",
			Type:      "sast",
		},
		Vulnerabilities: []gitlabVulnerability{},
	}

	for _, finding := range result.Findings {
		// id = hex(SHA256(`file:line:category:title`))[:32]
		idStr := fmt.Sprintf("%s:%d:%s:%s", finding.File, finding.Line, finding.Category, finding.Title)
		hash := sha256.Sum256([]byte(idStr))
		id := hex.EncodeToString(hash[:])[:32]

		severity := "Info"
		switch strings.ToUpper(finding.Severity) {
		case "CRITICAL":
			severity = "Critical"
		case "HIGH":
			severity = "High"
		case "MEDIUM":
			severity = "Medium"
		case "LOW":
			severity = "Low"
		}

		desc := finding.Body
		if finding.Suggestion != "" {
			desc += "\n\nSuggestion:\n" + finding.Suggestion
		}

		endLine := finding.Line
		if finding.EndLine > finding.Line {
			endLine = finding.EndLine
		}

		vuln := gitlabVulnerability{
			ID:          id,
			Category:    "sast",
			Name:        finding.Title,
			Message:     finding.Title,
			Description: desc,
			Severity:    severity,
			Scanner: gitlabScanner{
				ID:   "code-reviewer",
				Name: "code-reviewer",
			},
			Location: gitlabLocation{
				File:      finding.File,
				StartLine: finding.Line,
				EndLine:   endLine,
			},
			Identifiers: []gitlabIdentifier{
				{
					Type:  "code_reviewer_category",
					Name:  finding.Category,
					Value: finding.Category,
				},
			},
		}

		report.Vulnerabilities = append(report.Vulnerabilities, vuln)
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling GitLab SAST: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing GitLab SAST to %s: %w", path, err)
	}

	return nil
}
