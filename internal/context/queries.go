package context

import (
	"embed"
	"strings"
)

//go:embed queries/*.scm
var queryFS embed.FS

// languageQueries maps language names to their tree-sitter query strings.
// Loaded once from embedded .scm files.
var languageQueries map[string]string

func init() {
	languageQueries = make(map[string]string)
	entries, err := queryFS.ReadDir("queries")
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".scm") {
			continue
		}
		data, err := queryFS.ReadFile("queries/" + e.Name())
		if err != nil {
			continue
		}
		lang := strings.TrimSuffix(e.Name(), ".scm")
		languageQueries[lang] = string(data)
	}
}

// extensionToLanguage maps file extensions to language names used in queries.
var extensionToLanguage = map[string]string{
	".go":   "go",
	".kt":   "kotlin",
	".kts":  "kotlin",
	".java": "java",
	".py":   "python",
	".ts":   "typescript",
	".tsx":  "typescript",
}

// languageForFile returns the language name for a file path, or "" if unsupported.
func languageForFile(path string) string {
	for ext, lang := range extensionToLanguage {
		if strings.HasSuffix(path, ext) {
			return lang
		}
	}
	return ""
}
