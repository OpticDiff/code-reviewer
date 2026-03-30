package diff

import (
	"path/filepath"
	"strings"
)

// Filter removes file diffs that match exclusion patterns.
func Filter(diffs []FileDiff, patterns []string) []FileDiff {
	if len(patterns) == 0 {
		return diffs
	}

	var result []FileDiff
	for _, d := range diffs {
		if d.IsBinary {
			continue
		}

		path := d.NewPath
		if path == "" {
			path = d.OldPath
		}

		if shouldExclude(path, patterns) {
			continue
		}

		result = append(result, d)
	}
	return result
}

func shouldExclude(path string, patterns []string) bool {
	base := filepath.Base(path)

	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}

		// Try matching against full path.
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
		// Try matching against basename.
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
		// Handle directory globs like "vendor/*".
		if strings.HasSuffix(pattern, "/*") {
			dir := strings.TrimSuffix(pattern, "/*")
			if strings.HasPrefix(path, dir+"/") {
				return true
			}
		}
	}
	return false
}
