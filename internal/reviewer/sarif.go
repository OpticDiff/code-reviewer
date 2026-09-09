package reviewer

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
	Tool                      sarifTool                       `json:"tool"`
	Results                   []sarifResult                   `json:"results"`
	VersionControlProvenance  []sarifVersionControlProvenance `json:"versionControlProvenance,omitempty"`
}

type sarifVersionControlProvenance struct {
	RepositoryURI string `json:"repositoryUri"`
	RevisionID    string `json:"revisionId"`
	Branch        string `json:"branch,omitempty"`
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
	HelpURI          string                 `json:"helpUri,omitempty"`
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
func WriteSARIF(path string, result *model.ReviewResult, version, profile string) error {
	report := buildSARIF(result, version, profile)

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

func buildSARIF(result *model.ReviewResult, version, profile string) sarifReport {
	ruleMap := make(map[string]int)
	ruleMaxSev := make(map[string]string)
	var rules []sarifRule
	var results []sarifResult

	for _, f := range result.Findings {
		ruleID := f.Category
		if ruleID == "" {
			ruleID = "general"
		}

		level := sarifLevel(f.Severity)

		ruleIdx, ok := ruleMap[ruleID]
		if !ok {
			ruleIdx = len(rules)
			ruleMap[ruleID] = ruleIdx
			ruleMaxSev[ruleID] = f.Severity

			props := make(map[string]interface{})
			if secSev := getSecuritySeverity(f.Severity); secSev != "" {
				props["security-severity"] = secSev
			}
			props["tags"] = []string{ruleID}

			rule := sarifRule{
				ID:               ruleID,
				Name:             titleCase(ruleID),
				ShortDescription: sarifMessage{Text: ruleID},
				FullDescription:  &sarifMessage{Text: titleCase(ruleID) + " issue"},
				Help:             &sarifMessage{Text: "Please review the finding details."},
				DefaultConfig:    &sarifRuleConfig{Level: level},
				Properties:       props,
			}
			if f.RuleURL != "" {
				rule.HelpURI = f.RuleURL
			}
			rules = append(rules, rule)
		} else {
			// SARIF §3.19.3 requires driver.rules to have unique IDs.
			// When multiple findings share a category, update the rule's
			// security-severity and default level to the maximum seen.
			if severityRank(f.Severity) > severityRank(ruleMaxSev[ruleID]) {
				ruleMaxSev[ruleID] = f.Severity
				rules[ruleIdx].DefaultConfig.Level = level
				if secSev := getSecuritySeverity(f.Severity); secSev != "" {
					rules[ruleIdx].Properties["security-severity"] = secSev
				}
			}
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

		// Fingerprint uniquely identifies this finding by location and issue to prevent
		// distinct findings in the same file and category from colliding in GitHub Code Scanning.
		hashInput := fmt.Sprintf("%s:%s:%d:%s", f.File, ruleID, line, f.Title)
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

	driverName := "code-reviewer"
	switch profile {
	case "platform":
		driverName = "code-reviewer/platform"
	case "product":
		driverName = "code-reviewer/product"
	}

	return sarifReport{
		Version: "2.1.0",
		Schema:  "https://docs.oasis-open.org/sarif/sarif/v2.1.0/errata01/os/schemas/sarif-schema-2.1.0.json",
		Runs: []sarifRun{{
			Tool: sarifTool{
				Driver: sarifDriver{
					Name:           driverName,
					Version:        version,
					InformationURI: "https://github.com/OpticDiff/code-reviewer",
					Rules:          rules,
				},
			},
			Results:                  results,
			VersionControlProvenance: buildVCSProvenance(),
		}},
	}
}

// buildVCSProvenance constructs versionControlProvenance from CI environment
// variables (GitHub Actions, GitLab CI) or falls back to local git info.
func buildVCSProvenance() []sarifVersionControlProvenance {
	var repoURI, revisionID, branch string

	// GitHub Actions.
	if repo := os.Getenv("GITHUB_REPOSITORY"); repo != "" {
		serverURL := os.Getenv("GITHUB_SERVER_URL")
		if serverURL == "" {
			serverURL = "https://github.com"
		}
		repoURI = serverURL + "/" + repo
		revisionID = os.Getenv("GITHUB_SHA")
		branch = os.Getenv("GITHUB_REF_NAME")
	} else if projectURL := os.Getenv("CI_PROJECT_URL"); projectURL != "" {
		// GitLab CI.
		repoURI = projectURL
		revisionID = os.Getenv("CI_COMMIT_SHA")
		branch = os.Getenv("CI_COMMIT_REF_NAME")
	} else {
		// Local git fallback.
		if out, err := exec.Command("git", "remote", "get-url", "origin").Output(); err == nil {
			repoURI = strings.TrimSpace(string(out))
		}
		if out, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
			revisionID = strings.TrimSpace(string(out))
		}
		if out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
			branch = strings.TrimSpace(string(out))
		}
	}

	if repoURI == "" && revisionID == "" {
		return nil
	}

	return []sarifVersionControlProvenance{{
		RepositoryURI: repoURI,
		RevisionID:    revisionID,
		Branch:        branch,
	}}
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
