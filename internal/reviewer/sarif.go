package reviewer

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/OpticDiff/code-reviewer/internal/model"
)

// SARIF 2.1.0 types
type sarifReport struct {
	Version string     `json:"version"`
	Schema  string     `json:"$schema"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version,omitempty"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules,omitempty"`
}

type sarifRule struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name,omitempty"`
	ShortDescription sarifMessage           `json:"shortDescription"`
	FullDescription  *sarifMessage          `json:"fullDescription,omitempty"`
	Help             *sarifMessage          `json:"help,omitempty"`
	DefaultConfig    *sarifRuleConfig       `json:"defaultConfiguration,omitempty"`
	Properties       map[string]interface{} `json:"properties,omitempty"`
}

type sarifRuleConfig struct {
	Level string `json:"level"`
}

type sarifResult struct {
	RuleID              string            `json:"ruleId"`
	RuleIndex           int               `json:"ruleIndex"`
	Level               string            `json:"level"`
	Message             sarifMessage      `json:"message"`
	Locations           []sarifLocation   `json:"locations"`
	PartialFingerprints map[string]string `json:"partialFingerprints,omitempty"`
}

type sarifMessage struct {
	Text     string `json:"text"`
	Markdown string `json:"markdown,omitempty"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           sarifRegion           `json:"region"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
	EndLine   int `json:"endLine,omitempty"`
}

// WriteSARIF writes review results in SARIF 2.1.0 format to the given path.
func WriteSARIF(path string, result *model.ReviewResult, version string) error {
	report := buildSARIF(result, version)

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling SARIF: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing SARIF file: %w", err)
	}
	return nil
}

func titleCase(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func getSecuritySeverity(severity string) string {
	switch severity {
	case "CRITICAL":
		return "9.5"
	case "HIGH":
		return "8.0"
	case "MEDIUM":
		return "5.0"
	case "LOW":
		return "2.0"
	default:
		return ""
	}
}

func buildSARIF(result *model.ReviewResult, version string) sarifReport {
	ruleMap := make(map[string]int)
	var rules []sarifRule
	var results []sarifResult

	for _, f := range result.Findings {
		ruleID := f.Category
		if ruleID == "" {
			ruleID = "general"
		}

		// Dedup key includes severity level so that findings with the same
		// category but different severities get separate rule entries with
		// correct security-severity scores.
		level := sarifLevel(f.Severity)
		ruleKey := ruleID + ":" + level

		ruleIdx, ok := ruleMap[ruleKey]
		if !ok {
			ruleIdx = len(rules)
			ruleMap[ruleKey] = ruleIdx

			props := make(map[string]interface{})
			if secSev := getSecuritySeverity(f.Severity); secSev != "" {
				props["security-severity"] = secSev
			}
			props["tags"] = []string{ruleID}

			rules = append(rules, sarifRule{
				ID:               ruleID,
				Name:             titleCase(ruleID),
				ShortDescription: sarifMessage{Text: ruleID},
				FullDescription:  &sarifMessage{Text: titleCase(ruleID) + " issue"},
				Help:             &sarifMessage{Text: "Please review the finding details."},
				DefaultConfig:    &sarifRuleConfig{Level: level},
				Properties:       props,
			})
		}

		line := f.Line
		if line <= 0 {
			line = 1
		}

		region := sarifRegion{StartLine: line}
		if f.EndLine > 0 && f.EndLine >= line {
			region.EndLine = f.EndLine
		}

		// SARIF requires message.text to be plain text.
		// Markdown formatting goes only in message.markdown.
		plainText := f.Title + "\n\n" + f.Body
		if f.Suggestion != "" {
			plainText += "\n\nSuggested fix:\n" + f.Suggestion
		}
		var markdown string
		if f.Suggestion != "" {
			markdown = f.Title + "\n\n" + f.Body +
				fmt.Sprintf("\n\n**Suggested fix:**\n```suggestion\n%s\n```", f.Suggestion)
		}

		// Fingerprint uses only stable data: file path and ruleID.
		// Excludes line number (shifts on unrelated edits) and model-generated
		// title (wording can change between runs).
		hashInput := fmt.Sprintf("%s:%s", f.File, ruleID)
		hashBytes := sha256.Sum256([]byte(hashInput))
		hashHex := fmt.Sprintf("%x", hashBytes)[:16]

		results = append(results, sarifResult{
			RuleID:    ruleID,
			RuleIndex: ruleIdx,
			Level:     level,
			Message:   sarifMessage{Text: plainText, Markdown: markdown},
			Locations: []sarifLocation{{
				PhysicalLocation: sarifPhysicalLocation{
					ArtifactLocation: sarifArtifactLocation{URI: f.File},
					Region:           region,
				},
			}},
			PartialFingerprints: map[string]string{
				"primaryLocationLineHash": hashHex,
			},
		})
	}

	return sarifReport{
		Version: "2.1.0",
		Schema:  "https://docs.oasis-open.org/sarif/sarif/v2.1.0/errata01/os/schemas/sarif-schema-2.1.0.json",
		Runs: []sarifRun{{
			Tool: sarifTool{
				Driver: sarifDriver{
					Name:           "code-reviewer",
					Version:        version,
					InformationURI: "https://github.com/OpticDiff/code-reviewer",
					Rules:          rules,
				},
			},
			Results: results,
		}},
	}
}

func sarifLevel(severity string) string {
	switch severity {
	case "CRITICAL", "HIGH":
		return "error"
	case "MEDIUM":
		return "warning"
	case "LOW":
		return "note"
	default:
		return "none"
	}
}
