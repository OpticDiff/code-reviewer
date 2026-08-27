// Package initcmd implements the `code-reviewer init` interactive config generator.
package initcmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

const configFileName = ".code-reviewer.yaml"

// Options controls the init command behavior.
type Options struct {
	Force bool // Overwrite existing config file.
	Yes   bool // Non-interactive mode with all defaults.
}

// Config holds the user's answers for YAML generation.
type Config struct {
	Model            string
	Focus            []string
	MinSeverity      string
	ChunkStrategy    string
	MaxFiles         int
	ExcludedPatterns []string
	ExtraRules       string
	CommentMode      string
	Platform         string // "github", "gitlab", or ""
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Model:         "gemini-2.5-flash",
		Focus:         []string{"all"},
		MinSeverity:   "low",
		ChunkStrategy: "split",
		MaxFiles:      0,
		ExcludedPatterns: []string{
			"go.sum", "*.lock", "vendor/*", "node_modules/*",
		},
		CommentMode: "notes",
	}
}

// Run executes the init command.
func Run(opts Options) error {
	if !opts.Force {
		if _, err := os.Stat(configFileName); err == nil {
			return fmt.Errorf("%s already exists (use --force to overwrite)", configFileName)
		}
	}

	cfg := DefaultConfig()
	cfg.Platform = detectPlatform()

	// Apply platform-specific defaults before prompts.
	if cfg.Platform == "gitlab" {
		cfg.CommentMode = "discussions"
	}

	if !opts.Yes {
		reader := bufio.NewReader(os.Stdin)
		fmt.Println("🔧 code-reviewer configuration generator")
		fmt.Println()

		cfg.Model = prompt(reader, "Model", cfg.Model)

		focusDefault := strings.Join(cfg.Focus, ",")
		focusInput := prompt(reader, "Focus areas (bugs,security,performance,style,docs,all)", focusDefault)
		cfg.Focus = strings.Split(focusInput, ",")
		for i := range cfg.Focus {
			cfg.Focus[i] = strings.TrimSpace(cfg.Focus[i])
		}

		cfg.MinSeverity = prompt(reader, "Minimum severity (low,medium,high,critical)", cfg.MinSeverity)
		cfg.ChunkStrategy = prompt(reader, "Chunk strategy (fail,split)", cfg.ChunkStrategy)

		maxFilesStr := prompt(reader, "Max files before scope warning (0=unlimited)", "0")
		if _, err := fmt.Sscanf(maxFilesStr, "%d", &cfg.MaxFiles); err != nil {
			cfg.MaxFiles = 0
		}

		excludeDefault := strings.Join(cfg.ExcludedPatterns, ",")
		excludeInput := prompt(reader, "Excluded patterns", excludeDefault)
		cfg.ExcludedPatterns = strings.Split(excludeInput, ",")
		for i := range cfg.ExcludedPatterns {
			cfg.ExcludedPatterns[i] = strings.TrimSpace(cfg.ExcludedPatterns[i])
		}

		if cfg.Platform == "gitlab" {
			cfg.CommentMode = prompt(reader, "GitLab comment mode (notes,discussions)", cfg.CommentMode)
		}

		cfg.ExtraRules = prompt(reader, "Custom review rules (or press Enter to skip)", "")

		fmt.Println()
	}

	if err := writeConfig(cfg); err != nil {
		return err
	}

	fmt.Println("✅ Created " + configFileName)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  code-reviewer --diff          # review uncommitted changes")
	fmt.Println("  code-reviewer hook install     # add pre-push hook")
	if cfg.Platform == "" {
		fmt.Println("  See docs/CI-SETUP.md           # CI/CD integration")
	}

	return nil
}

// prompt displays a question with a default value and reads user input.
// Empty input returns the default.
func prompt(reader *bufio.Reader, question, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", question, defaultVal)
	} else {
		fmt.Printf("%s: ", question)
	}

	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)

	if line == "" {
		return defaultVal
	}
	return line
}

// detectPlatform checks for CI platform indicators in the current directory.
func detectPlatform() string {
	if _, err := os.Stat(".gitlab-ci.yml"); err == nil {
		return "gitlab"
	}
	if _, err := os.Stat(filepath.Join(".github", "workflows")); err == nil {
		return "github"
	}
	return ""
}

var configTemplate = template.Must(template.New("config").Parse(`# .code-reviewer.yaml — per-repo configuration
# Docs: https://github.com/OpticDiff/code-reviewer

# Model for review.
# Options: gemini-2.5-flash (recommended), gemini-2.5-pro, claude-sonnet-4,
#          or any OpenAI-compatible model via api_url.
model: {{ .Model }}

# Focus areas: bugs, security, performance, style, docs, all
focus:
{{- range .Focus }}
  - {{ . }}
{{- end }}

# Minimum severity to report: low, medium, high, critical
min_severity: {{ .MinSeverity }}

# How to handle large diffs: fail (error) or split (chunk into multiple reviews)
chunk_strategy: {{ .ChunkStrategy }}
{{ if gt .MaxFiles 0 }}
# Max files before scope warning (0 = unlimited)
max_files: {{ .MaxFiles }}
{{ end }}
# Glob patterns to exclude from review
excluded_patterns:
{{- range .ExcludedPatterns }}
  - "{{ . }}"
{{- end }}
{{ if eq .Platform "gitlab" }}
# GitLab comment mode: notes (summary) or discussions (inline diff-anchored)
comment_mode: {{ .CommentMode }}
{{ end }}
{{ if .ExtraRules }}
# Custom review rules appended to the AI prompt
extra_rules: |
  {{ .ExtraRules }}
{{ end }}
# Additional options (uncomment to enable):
# api_url: http://localhost:11434/v1    # OpenAI-compatible endpoint (Ollama, vLLM)
# custom_prompt: .review-prompts/team.md # Custom system prompt file
# audit_log: reviews.jsonl              # Structured audit trail
# intent_review: true                    # Two-pass intent-aware review
# auto_approve: false                    # Auto-approve clean MRs (CI only)
`))

// writeConfig renders the template to a temp file and renames it into place.
func writeConfig(cfg Config) (retErr error) {
	dir := filepath.Dir(configFileName)
	tmp, err := os.CreateTemp(dir, ".code-reviewer.yaml.tmp.*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	closed := false

	defer func() {
		if !closed {
			_ = tmp.Close() //nolint:errcheck // best-effort cleanup
		}
		if retErr != nil {
			_ = os.Remove(tmpName) //nolint:errcheck // best-effort cleanup
		}
	}()

	if err := configTemplate.Execute(tmp, cfg); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	closed = true
	if err := os.Rename(tmpName, configFileName); err != nil {
		return fmt.Errorf("renaming config: %w", err)
	}
	return nil
}
