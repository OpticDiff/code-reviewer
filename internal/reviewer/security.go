package reviewer

import (
	"strings"

	"github.com/OpticDiff/code-reviewer/internal/model"
)

// DetectConfigPoisoning returns true if any of the diff files modify code reviewer configuration.
func DetectConfigPoisoning(files []string) bool {
	for _, file := range files {
		if strings.HasSuffix(file, ".code-reviewer.yaml") ||
			strings.HasSuffix(file, ".code-reviewer.yml") ||
			strings.HasSuffix(file, ".opticdiff.yaml") {
			return true
		}
	}
	return false
}

// ConfigPoisoningFinding returns a mandatory finding warning about configuration modification.
func ConfigPoisoningFinding() model.Finding {
	return model.Finding{
		File:        ".code-reviewer.yaml",
		Line:        1,
		Severity:    "CRITICAL",
		Category:    "Security",
		Title:       "Config File Modified",
		Body:        "Config file modified in this PR — review manually. Bot will not auto-approve. CODEOWNER approval required.",
	}
}
