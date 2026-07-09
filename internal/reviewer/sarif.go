package reviewer

import (
	"encoding/json"
	"fmt"
	"os"

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
	ID               string           `json:"id"`
	ShortDescription sarifMessage     `json:"shortDescription"`
	DefaultConfig    *sarifRuleConfig `json:"defaultConfiguration,omitempty"`
}

type sarifRuleConfig struct {
	Level string `json:"level"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations"`
}

type sarifMessage struct {
	Text string `json:"text"`
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
}

// WriteSARIF writes review results in SARIF 2.1.0 format to the given path.
func WriteSARIF(path string, result *model.ReviewResult) error {
	report := buildSARIF(result)

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling SARIF: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing SARIF file: %w", err)
	}
	return nil
}

func buildSARIF(result *model.ReviewResult) sarifReport {
	// Deduplicate rules by category.
	ruleMap := make(map[string]bool)
	var rules []sarifRule
	for _, f := range result.Findings {
		ruleID := f.Category
		if ruleID == "" {
			ruleID = "general"
		}
		if !ruleMap[ruleID] {
			ruleMap[ruleID] = true
			rules = append(rules, sarifRule{
				ID:               ruleID,
				ShortDescription: sarifMessage{Text: ruleID},
			})
		}
	}

	var results []sarifResult
	for _, f := range result.Findings {
		ruleID := f.Category
		if ruleID == "" {
			ruleID = "general"
		}
		line := f.Line
		if line <= 0 {
			line = 1
		}
		results = append(results, sarifResult{
			RuleID:  ruleID,
			Level:   sarifLevel(f.Severity),
			Message: sarifMessage{Text: f.Title + "\n\n" + f.Body},
			Locations: []sarifLocation{{
				PhysicalLocation: sarifPhysicalLocation{
					ArtifactLocation: sarifArtifactLocation{URI: f.File},
					Region:           sarifRegion{StartLine: line},
				},
			}},
		})
	}

	return sarifReport{
		Version: "2.1.0",
		Schema:  "https://docs.oasis-open.org/sarif/sarif/v2.1.0/errata01/os/schemas/sarif-schema-2.1.0.json",
		Runs: []sarifRun{{
			Tool: sarifTool{
				Driver: sarifDriver{
					Name:           "code-reviewer",
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
