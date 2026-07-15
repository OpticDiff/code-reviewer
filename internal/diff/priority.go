package diff

import (
	"path/filepath"
	"sort"
	"strings"
)

// securityPatterns are path substrings that indicate security-sensitive files.
var securityPatterns = []string{
	"auth", "session", "token", "secret", "secrets", "password", "credential",
	"crypto", "security", "permission", "rbac", "acl", "oauth",
	"saml", "jwt", "cert", "tls", "ssl", "key",
}

// securityFiles are exact filenames or extensions that are security-sensitive.
var securityFiles = []string{
	".env", "Dockerfile", "docker-compose",
	"config.yaml", "config.yml", "config.json",
}

// generatedPatterns are path substrings that indicate generated code.
var generatedPatterns = []string{
	".pb.go", ".pb.gw.go", "_generated", "_gen.go",
	"generated.go", "mock_", "mocks/", "zz_generated",
	"vendor/", "node_modules/", "dist/",
}

// FilePriority assigns a review priority score to a file diff.
// Higher score = review first. Used to decide which files to skip
// when a token budget is exceeded.
func FilePriority(fd FileDiff) int {
	score := 0
	path := strings.ToLower(fd.NewPath)

	// Security-sensitive files get highest priority.
	if isSecuritySensitive(path) {
		score += 100
	}

	// More changed lines = higher priority.
	score += fd.LineCount()

	// New files get a boost — no prior review coverage.
	if fd.IsNew {
		score += 30
	}

	// Penalize low-value files.
	if isGenerated(path) {
		score -= 50
	}
	if isTestFile(path) {
		score -= 10 // Still review tests, but prioritize prod code.
	}
	if fd.IsRename && fd.LineCount() == 0 {
		score -= 40 // Pure rename with no content changes.
	}

	return score
}

// SortByPriority sorts diffs highest-priority first.
func SortByPriority(diffs []FileDiff) {
	sort.SliceStable(diffs, func(i, j int) bool {
		return FilePriority(diffs[i]) > FilePriority(diffs[j])
	})
}

func isSecuritySensitive(path string) bool {
	// Split into path components and check each against security patterns.
	parts := strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == '\\' || r == '_' || r == '-' || r == '.'
	})
	for _, part := range parts {
		part = strings.ToLower(part)
		for _, p := range securityPatterns {
			if part == p {
				return true
			}
		}
	}
	base := filepath.Base(path)
	for _, f := range securityFiles {
		if base == f || strings.HasPrefix(base, f) {
			return true
		}
	}
	return false
}

func isGenerated(path string) bool {
	for _, p := range generatedPatterns {
		if strings.Contains(path, p) {
			return true
		}
	}
	return false
}

func isTestFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, "_test.go") ||
		strings.HasSuffix(base, ".test.ts") ||
		strings.HasSuffix(base, ".test.js") ||
		strings.HasSuffix(base, ".spec.ts") ||
		strings.HasSuffix(base, ".spec.js") ||
		strings.HasSuffix(base, "Test.java") ||
		strings.HasSuffix(base, "Test.kt") ||
		strings.HasPrefix(base, "test_") ||
		strings.Contains(path, "/test/") ||
		strings.Contains(path, "/tests/")
}
