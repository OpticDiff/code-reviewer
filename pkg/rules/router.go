package rules

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// RuleRouter maps file paths to rule checklist files using glob patterns.
type RuleRouter struct {
	DefaultRule string            `json:"default_rule"`
	PathRuleMap map[string]string `json:"path_rule_map"`
}

// LoadRuleRouter reads a system_rules.json file and returns a RuleRouter.
// The path must be an absolute path to the rules directory root.
func LoadRuleRouter(rulesDir string) (*RuleRouter, error) {
	root, err := os.OpenRoot(rulesDir)
	if err != nil {
		return nil, fmt.Errorf("opening rules root %s: %w", rulesDir, err)
	}
	defer root.Close() //nolint:errcheck // best-effort cleanup of read-only root

	f, err := root.Open("system_rules.json")
	if err != nil {
		return nil, fmt.Errorf("reading rule router: %w", err)
	}
	defer f.Close() //nolint:errcheck // best-effort cleanup of read-only file

	var router RuleRouter
	if err := json.NewDecoder(f).Decode(&router); err != nil {
		return nil, fmt.Errorf("parsing rule router: %w", err)
	}
	return &router, nil
}

// Match returns the rule file path that applies to a given source file path.
// Returns the default rule if no glob pattern matches.
func (r *RuleRouter) Match(filePath string) string {
	for pattern, ruleFile := range r.PathRuleMap {
		if globMatch(pattern, filePath) {
			return ruleFile
		}
	}
	return r.DefaultRule
}

// MatchAll returns the deduplicated set of rule files that apply to a list of
// source file paths. Each rule file appears at most once.
func (r *RuleRouter) MatchAll(filePaths []string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, fp := range filePaths {
		rule := r.Match(fp)
		if _, ok := seen[rule]; !ok {
			seen[rule] = struct{}{}
			result = append(result, rule)
		}
	}
	return result
}

// globMatch matches a file path against a glob pattern.
// Supports extended brace expansion for patterns like "**/*.{go,proto}".
func globMatch(pattern, filePath string) bool {
	// Handle brace expansion: **/*.{go,proto} -> try each alternative
	if idx := strings.Index(pattern, "{"); idx >= 0 {
		end := strings.Index(pattern[idx:], "}")
		if end > 0 {
			prefix := pattern[:idx]
			suffix := pattern[idx+end+1:]
			alternatives := strings.Split(pattern[idx+1:idx+end], ",")
			for _, alt := range alternatives {
				if globMatch(prefix+strings.TrimSpace(alt)+suffix, filePath) {
					return true
				}
			}
			return false
		}
	}

	// Use filepath.Match for standard glob matching
	// Handle ** (match any number of path segments)
	if strings.Contains(pattern, "**") {
		return matchDoubleGlob(pattern, filePath)
	}

	matched, _ := filepath.Match(pattern, filePath)
	if matched {
		return true
	}

	// Try matching just the filename (for patterns like "Dockerfile*")
	matched, _ = filepath.Match(pattern, filepath.Base(filePath))
	return matched
}

// matchDoubleGlob handles ** patterns that match any number of path segments.
func matchDoubleGlob(pattern, filePath string) bool {
	parts := strings.SplitN(pattern, "**", 2)
	if len(parts) != 2 {
		return false
	}
	prefix := parts[0]
	suffix := strings.TrimPrefix(parts[1], "/")

	// If there's a prefix, the path must start with it
	if prefix != "" && !strings.HasPrefix(filePath, prefix) {
		return false
	}

	// If suffix is empty, ** matches everything
	if suffix == "" {
		return true
	}

	// Try matching the suffix against every possible sub-path
	segments := strings.Split(filePath, "/")
	for i := 0; i < len(segments); i++ {
		remainder := strings.Join(segments[i:], "/")
		if globMatch(suffix, remainder) {
			return true
		}
		// Also try matching just the last segment for basename patterns
		if i == len(segments)-1 {
			if matched, _ := filepath.Match(suffix, segments[i]); matched {
				return true
			}
		}
	}
	return false
}

// RuleLoader loads Markdown rule checklists from the filesystem.
// File reads are scoped under rulesDir using os.Root to prevent traversal.
type RuleLoader struct {
	rulesDir string
	root     *os.Root
	cache    map[string]string
	mu       sync.RWMutex
}

// NewRuleLoader creates a new RuleLoader rooted at the given directory.
// All file reads are scoped under this directory.
func NewRuleLoader(rulesDir string) (*RuleLoader, error) {
	root, err := os.OpenRoot(rulesDir)
	if err != nil {
		return nil, fmt.Errorf("opening rules root %s: %w", rulesDir, err)
	}
	return &RuleLoader{
		rulesDir: rulesDir,
		root:     root,
		cache:    make(map[string]string),
	}, nil
}

// Close releases the root directory handle.
func (l *RuleLoader) Close() error {
	return l.root.Close()
}

// Load reads a rule checklist file and returns its Markdown content.
// Results are cached after the first read.
func (l *RuleLoader) Load(ruleFile string) (string, error) {
	l.mu.RLock()
	if content, ok := l.cache[ruleFile]; ok {
		l.mu.RUnlock()
		return content, nil
	}
	l.mu.RUnlock()

	f, err := l.root.Open(ruleFile)
	if err != nil {
		return "", fmt.Errorf("loading rule %s: %w", ruleFile, err)
	}
	defer f.Close() //nolint:errcheck // best-effort cleanup of read-only file

	const maxRuleBytes = 1 << 20 // 1 MiB — rules should be ~50 lines
	data, err := io.ReadAll(io.LimitReader(f, maxRuleBytes))
	if err != nil {
		return "", fmt.Errorf("reading rule %s: %w", ruleFile, err)
	}
	content := string(data)

	l.mu.Lock()
	l.cache[ruleFile] = content
	l.mu.Unlock()

	return content, nil
}

// LoadAll reads multiple rule files and concatenates their content.
// Each rule file's content is separated by a horizontal rule.
func (l *RuleLoader) LoadAll(ruleFiles []string) (string, error) {
	var sections []string
	for _, rf := range ruleFiles {
		content, err := l.Load(rf)
		if err != nil {
			return "", err
		}
		sections = append(sections, content)
	}
	return strings.Join(sections, "\n---\n\n"), nil
}

// LoadForFiles resolves which rule checklists apply to a set of source file
// paths using the given router, then loads and concatenates their content.
func (l *RuleLoader) LoadForFiles(router *RuleRouter, filePaths []string) (string, error) {
	ruleFiles := router.MatchAll(filePaths)
	return l.LoadAll(ruleFiles)
}
