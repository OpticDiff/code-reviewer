package reviewer

import (
	"path/filepath"
	"strings"

	"github.com/OpticDiff/code-reviewer/internal/model"
)

// configFileNames lists the config file basenames that trigger poisoning detection.
var configFileNames = []string{
	".code-reviewer.yaml",
	".code-reviewer.yml",
	".opticdiff.yaml",
}

// isConfigFile returns true if the path refers to a code reviewer configuration file.
func isConfigFile(path string) bool {
	base := filepath.Base(path)
	for _, name := range configFileNames {
		if strings.EqualFold(base, name) {
			return true
		}
	}
	return false
}

// DetectConfigPoisoning returns the list of config files found in the diff.
// Both old and new paths should be included to catch renames.
func DetectConfigPoisoning(files []string) []string {
	var found []string
	seen := make(map[string]bool)
	for _, file := range files {
		if file != "" && isConfigFile(file) && !seen[file] {
			found = append(found, file)
			seen[file] = true
		}
	}
	return found
}

// ConfigPoisoningFinding returns a mandatory finding for the given config file path and line.
func ConfigPoisoningFinding(filePath string, line int) model.Finding {
	if line < 1 {
		line = 1
	}
	return model.Finding{
		File:     filePath,
		Line:     line,
		Severity: "CRITICAL",
		Category: "Security",
		Title:    "Config File Modified",
		Body:     "Config file modified in this PR — review manually. Bot will not auto-approve. CODEOWNER approval required.",
	}
}
