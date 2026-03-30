// Package config handles configuration loading from flags, environment
// variables, and .code-reviewer.yaml files. Priority: flags > env > yaml > defaults.
package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Severity levels for filtering findings.
type Severity int

const (
	SeverityLow Severity = iota
	SeverityMedium
	SeverityHigh
	SeverityCritical
)

// String returns the string representation of a Severity.
func (s Severity) String() string {
	switch s {
	case SeverityLow:
		return "LOW"
	case SeverityMedium:
		return "MEDIUM"
	case SeverityHigh:
		return "HIGH"
	case SeverityCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// ParseSeverity converts a string to a Severity level.
func ParseSeverity(s string) (Severity, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "LOW":
		return SeverityLow, nil
	case "MEDIUM":
		return SeverityMedium, nil
	case "HIGH":
		return SeverityHigh, nil
	case "CRITICAL":
		return SeverityCritical, nil
	default:
		return SeverityLow, fmt.Errorf("unknown severity: %q (valid: low, medium, high, critical)", s)
	}
}

// CommentMode determines how findings are posted to GitLab.
type CommentMode string

const (
	CommentModeNotes       CommentMode = "notes"
	CommentModeDiscussions CommentMode = "discussions"
)

// ChunkStrategy determines how large diffs are handled.
type ChunkStrategy string

const (
	ChunkStrategyFail  ChunkStrategy = "fail"
	ChunkStrategySplit ChunkStrategy = "split"
)

// Config holds all configuration for a code-reviewer run.
type Config struct {
	// Input mode (exactly one should be set).
	CIMode   bool
	DiffRef  string // empty means origin/HEAD
	Files    []string
	DiffMode bool // true if --diff was passed

	// Model settings.
	Model          string
	GCPProject     string
	GCPLocation    string
	ChunkStrategy  ChunkStrategy

	// Review settings.
	Focus       []string
	MinSeverity Severity
	ExtraRules  string

	// Output settings.
	CommentMode CommentMode
	DryRun      bool
	OutputJSON  bool

	// GitLab settings.
	GitLabToken   string
	GitLabBaseURL string
	SkipDraftMRs  bool

	// CI auto-detected.
	CIProjectID      string
	CIMergeRequestID string
	CIDiffBaseSHA    string

	// Exclusions.
	ExcludedPatterns []string
}

// repoConfig represents the .code-reviewer.yaml file.
type repoConfig struct {
	Model            string   `yaml:"model"`
	Focus            []string `yaml:"focus"`
	MinSeverity      string   `yaml:"min_severity"`
	CommentMode      string   `yaml:"comment_mode"`
	ChunkStrategy    string   `yaml:"chunk_strategy"`
	ExcludedPatterns []string `yaml:"excluded_patterns"`
	ExtraRules       string   `yaml:"extra_rules"`
}

// DefaultExcludedPatterns are file patterns excluded by default.
var DefaultExcludedPatterns = []string{
	"go.sum",
	"*.lock",
	"package-lock.json",
	"yarn.lock",
	"vendor/*",
	"node_modules/*",
}

// Load builds a Config by merging defaults, .code-reviewer.yaml, env vars, and flags.
// Priority: flags > env > yaml > defaults.
func Load() (*Config, error) {
	cfg := &Config{
		Model:            "gemini-2.5-flash",
		GCPLocation:      "us-central1",
		Focus:            []string{"all"},
		MinSeverity:      SeverityLow,
		CommentMode:      CommentModeNotes,
		ChunkStrategy:    ChunkStrategyFail,
		GitLabBaseURL:    "https://gitlab.com",
		SkipDraftMRs:     true,
		ExcludedPatterns: DefaultExcludedPatterns,
	}

	// Layer 1: .code-reviewer.yaml (if exists).
	if err := cfg.loadRepoConfig(); err != nil {
		return nil, fmt.Errorf("loading .code-reviewer.yaml: %w", err)
	}

	// Layer 2: Environment variables.
	cfg.loadEnv()

	// Layer 3: Flags (highest priority).
	if err := cfg.loadFlags(); err != nil {
		return nil, fmt.Errorf("parsing flags: %w", err)
	}

	// Auto-detect CI environment.
	cfg.loadCIEnv()

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) loadRepoConfig() error {
	// Walk up from cwd to find .code-reviewer.yaml.
	dir, err := os.Getwd()
	if err != nil {
		return nil // Non-fatal: skip yaml config.
	}

	for {
		path := filepath.Join(dir, ".code-reviewer.yaml")
		data, err := os.ReadFile(path)
		if err == nil {
			return c.applyRepoConfig(data)
		}
		// Also check .yml extension.
		path = filepath.Join(dir, ".code-reviewer.yml")
		data, err = os.ReadFile(path)
		if err == nil {
			return c.applyRepoConfig(data)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break // Reached filesystem root.
		}
		dir = parent
	}
	return nil
}

func (c *Config) applyRepoConfig(data []byte) error {
	var rc repoConfig
	if err := yaml.Unmarshal(data, &rc); err != nil {
		return err
	}

	if rc.Model != "" {
		c.Model = rc.Model
	}
	if len(rc.Focus) > 0 {
		c.Focus = rc.Focus
	}
	if rc.MinSeverity != "" {
		sev, err := ParseSeverity(rc.MinSeverity)
		if err != nil {
			return err
		}
		c.MinSeverity = sev
	}
	if rc.CommentMode != "" {
		c.CommentMode = CommentMode(rc.CommentMode)
	}
	if rc.ChunkStrategy != "" {
		c.ChunkStrategy = ChunkStrategy(rc.ChunkStrategy)
	}
	if len(rc.ExcludedPatterns) > 0 {
		c.ExcludedPatterns = rc.ExcludedPatterns
	}
	if rc.ExtraRules != "" {
		c.ExtraRules = rc.ExtraRules
	}
	return nil
}

func (c *Config) loadEnv() {
	if v := os.Getenv("REVIEW_MODEL"); v != "" {
		c.Model = v
	}
	if v := os.Getenv("REVIEW_FOCUS"); v != "" {
		c.Focus = strings.Split(v, ",")
	}
	if v := os.Getenv("REVIEW_MIN_SEVERITY"); v != "" {
		if sev, err := ParseSeverity(v); err == nil {
			c.MinSeverity = sev
		}
	}
	if v := os.Getenv("REVIEW_COMMENT_MODE"); v != "" {
		c.CommentMode = CommentMode(v)
	}
	if v := os.Getenv("REVIEW_CHUNK_STRATEGY"); v != "" {
		c.ChunkStrategy = ChunkStrategy(v)
	}
	if v := os.Getenv("GOOGLE_CLOUD_PROJECT"); v != "" {
		c.GCPProject = v
	}
	if v := os.Getenv("GOOGLE_CLOUD_LOCATION"); v != "" {
		c.GCPLocation = v
	}
	if v := os.Getenv("GITLAB_TOKEN"); v != "" {
		c.GitLabToken = v
	}
	if v := os.Getenv("GITLAB_BASE_URL"); v != "" {
		c.GitLabBaseURL = v
	}
	if v := os.Getenv("EXCLUDED_PATTERNS"); v != "" {
		c.ExcludedPatterns = strings.Split(v, ",")
	}
	if v := os.Getenv("REVIEW_EXTRA_RULES"); v != "" {
		c.ExtraRules = v
	}
	if v := os.Getenv("SKIP_DRAFT_MRS"); strings.EqualFold(v, "false") {
		c.SkipDraftMRs = false
	}
}

func (c *Config) loadFlags() error {
	fs := flag.NewFlagSet("code-reviewer", flag.ContinueOnError)

	ci := fs.Bool("ci", false, "Run in GitLab CI mode (auto-detect MR from env vars)")
	diffFlag := fs.Bool("diff", false, "Review local git diff (default: against origin/HEAD)")
	files := fs.String("files", "", "Comma-separated list of files to review")

	model := fs.String("model", "", "Vertex AI model ID (e.g., gemini-2.5-flash, claude-sonnet-4)")
	focus := fs.String("focus", "", "Review focus areas, comma-separated (bugs,security,performance,style,docs,all)")
	minSev := fs.String("min-severity", "", "Minimum severity to report (low, medium, high, critical)")
	commentMode := fs.String("comment-mode", "", "GitLab comment mode: notes (simple) or discussions (inline)")
	chunkStrategy := fs.String("chunk-strategy", "", "How to handle large diffs: fail (default) or split")
	extraRules := fs.String("extra-rules", "", "Additional review rules appended to prompt")
	dryRun := fs.Bool("dry-run", false, "Run analysis but don't post to GitLab")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	// Apply flags (only if explicitly set).
	c.CIMode = *ci
	c.DiffMode = *diffFlag
	c.DryRun = *dryRun

	if *files != "" {
		c.Files = strings.Split(*files, ",")
	}

	// Remaining args after --diff are the ref.
	if c.DiffMode && fs.NArg() > 0 {
		c.DiffRef = fs.Arg(0)
	}

	if *model != "" {
		c.Model = *model
	}
	if *focus != "" {
		c.Focus = strings.Split(*focus, ",")
	}
	if *minSev != "" {
		sev, err := ParseSeverity(*minSev)
		if err != nil {
			return err
		}
		c.MinSeverity = sev
	}
	if *commentMode != "" {
		c.CommentMode = CommentMode(*commentMode)
	}
	if *chunkStrategy != "" {
		c.ChunkStrategy = ChunkStrategy(*chunkStrategy)
	}
	if *extraRules != "" {
		c.ExtraRules = *extraRules
	}

	return nil
}

func (c *Config) loadCIEnv() {
	c.CIProjectID = os.Getenv("CI_PROJECT_ID")
	c.CIMergeRequestID = os.Getenv("CI_MERGE_REQUEST_IID")
	c.CIDiffBaseSHA = os.Getenv("CI_MERGE_REQUEST_DIFF_BASE_SHA")
}

func (c *Config) validate() error {
	// Must specify exactly one input mode.
	modes := 0
	if c.CIMode {
		modes++
	}
	if c.DiffMode {
		modes++
	}
	if len(c.Files) > 0 {
		modes++
	}
	if modes == 0 {
		return fmt.Errorf("must specify one of: --ci, --diff, or --files")
	}
	if modes > 1 {
		return fmt.Errorf("only one input mode allowed (--ci, --diff, or --files)")
	}

	// CI mode requires MR context.
	if c.CIMode {
		if c.CIProjectID == "" || c.CIMergeRequestID == "" {
			return fmt.Errorf("CI mode requires CI_PROJECT_ID and CI_MERGE_REQUEST_IID env vars")
		}
		if c.GitLabToken == "" {
			return fmt.Errorf("CI mode requires GITLAB_TOKEN env var")
		}
	}

	// GCP project required for model calls.
	if c.GCPProject == "" {
		return fmt.Errorf("GOOGLE_CLOUD_PROJECT env var is required")
	}

	// Validate comment mode.
	if c.CommentMode != CommentModeNotes && c.CommentMode != CommentModeDiscussions {
		return fmt.Errorf("invalid comment-mode: %q (valid: notes, discussions)", c.CommentMode)
	}

	// Validate chunk strategy.
	if c.ChunkStrategy != ChunkStrategyFail && c.ChunkStrategy != ChunkStrategySplit {
		return fmt.Errorf("invalid chunk-strategy: %q (valid: fail, split)", c.ChunkStrategy)
	}

	return nil
}

// Mode returns a human-readable string describing the input mode.
func (c *Config) Mode() string {
	switch {
	case c.CIMode:
		return "ci"
	case c.DiffMode:
		return "diff"
	case len(c.Files) > 0:
		return "files"
	default:
		return "unknown"
	}
}
