// Package config handles configuration loading from flags, environment
// variables, and .code-reviewer.yaml files. Priority: flags > env > yaml > defaults.
package config

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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

// CleanupMode determines how previous bot comments are handled.
type CleanupMode string

const (
	CleanupModeDelete  CleanupMode = "delete"  // Delete old comments (default).
	CleanupModeResolve CleanupMode = "resolve" // Resolve old discussions (GitLab only; GitHub falls back to delete).
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

	// Cache settings.
	CacheDir    string
	NoCache     bool
	CacheMaxAge time.Duration

	// Model settings.
	Model              string
	Models             []string // Multiple models for consensus mode.
	ConsensusThreshold int      // Min models that must agree on a finding (default: 2).
	GCPProject         string
	GCPLocation        string
	ChunkStrategy      ChunkStrategy
	APIURL             string // HTTP provider: OpenAI-compatible endpoint URL.
	APIKey             string // HTTP provider: API key (optional, e.g. for IAM/ADC auth).

	// Review settings.
	Focus        []string
	MinSeverity  Severity
	ExtraRules   string
	CustomPrompt string // Path to custom system prompt file.
	Incremental  bool   // Only review files changed in the latest push (CI mode).
	ReviewMD     string // Contents of REVIEW.md (repo-level review instructions).

	// Output settings.
	CommentMode CommentMode
	CleanupMode CleanupMode
	DryRun      bool
	OutputJSON  bool
	NoColor     bool   // Disable ANSI color output.
	SARIFOutput string // Path to write SARIF 2.1.0 output file.
	AuditLog    string // Path to write JSONL audit log.
	ProxyURL    string // Optional: LLM proxy URL for observability (e.g., Candela).
	UpdateDescription bool

	// GitLab settings.
	GitLabToken   string
	GitLabBaseURL string
	SkipDraftMRs  bool

	// GitHub settings.
	GitHubToken   string
	GitHubBaseURL string // Default: https://api.github.com

	// Platform detection: "gitlab", "github", or "" (auto-detect from CI env).
	Platform string

	// CI auto-detected.
	CIProjectID      string
	CIMergeRequestID string
	CIDiffBaseSHA      string // Reserved: loaded from CI_MERGE_REQUEST_DIFF_BASE_SHA for future incremental review.
	CICommitBeforeSHA  string // Loaded from CI env; reserved for future use.

	// Exclusions.
	ExcludedPatterns []string

	// Context discovery.
	DisableContext bool // Skip repo-aware context discovery (--no-context).

	// Summary mode.
	Summarize                bool // Generate MR summary instead of review.
	SummaryUpdateDescription bool // Update MR description with the generated summary.

	// Intent-aware review (two-pass).
	IntentReview   bool // Enable two-pass intent-aware review.
	NoIntentReview bool // Explicitly disable (overrides CI default).

	// Explain mode.
	Explain bool // Explain the diff instead of reviewing it.

	// Fix mode.
	Fix bool // Apply suggested fixes to the working tree.

	// Auto-approve.
	AutoApprove bool // Automatically approve MR/PR when review finds no issues.

	// Budget.
	MaxTokens int // Maximum total tokens per review (0 = unlimited).

	// Scope enforcement.
	MaxFiles    int
	ScopeAction string

	// Custom rules.
	Rules []Rule
}

// repoConfig represents the .code-reviewer.yaml file.
type repoConfig struct {
	Model            string   `yaml:"model"`
	Focus            []string `yaml:"focus"`
	MinSeverity      string   `yaml:"min_severity"`
	CommentMode      string   `yaml:"comment_mode"`
	CleanupMode      string   `yaml:"cleanup_mode"`
	ChunkStrategy    string   `yaml:"chunk_strategy"`
	CacheDir         string   `yaml:"cache_dir"`
	NoCache          *bool    `yaml:"no_cache"`
	CacheMaxAge      string   `yaml:"cache_max_age"` // e.g. "7d", "24h"
	ExcludedPatterns []string `yaml:"excluded_patterns"`
	ExtraRules       string   `yaml:"extra_rules"`
	OutputJSON       bool     `yaml:"output_json"`
	AuditLog                 string   `yaml:"audit_log"`
	CustomPrompt             string   `yaml:"custom_prompt"`
	ProxyURL                 string   `yaml:"proxy_url"`
	MaxTokens                int      `yaml:"max_tokens"`
	MaxFiles                 int      `yaml:"max_files"`
	ScopeAction              string   `yaml:"scope_action"`
	APIURL                   string   `yaml:"api_url"`
	Summarize                bool     `yaml:"summarize"`
	SummaryUpdateDescription bool     `yaml:"summary_update_description"`
	UpdateDescription        *bool    `yaml:"update_description"`
	IntentReview             *bool    `yaml:"intent_review"`
	AutoApprove              *bool    `yaml:"auto_approve"`
	Rules                    []Rule   `yaml:"rules"`
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
		CleanupMode:      CleanupModeDelete,
		ChunkStrategy:    ChunkStrategyFail,
		GitLabBaseURL:    "https://gitlab.com",
		GitHubBaseURL:    "https://api.github.com",
		SkipDraftMRs:     true,
		ExcludedPatterns: DefaultExcludedPatterns,
		ScopeAction:      "warn",
		CacheMaxAge:      7 * 24 * time.Hour,
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

	// Intent review: default on in CI, off in local.
	// Explicit --no-intent or REVIEW_INTENT=false overrides.
	if cfg.NoIntentReview {
		cfg.IntentReview = false
	} else if !cfg.IntentReview && cfg.CIMode {
		// Auto-enable in CI unless explicitly disabled.
		cfg.IntentReview = true
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) loadRepoConfig() error {
	// Walk up from cwd to find .code-reviewer.yaml and REVIEW.md.
	dir, err := os.Getwd()
	if err != nil {
		return nil // Non-fatal: skip yaml config.
	}

	var foundYAML, foundReviewMD bool
	for {
		// Check if we've reached a repo root (.git boundary).
		gitDir := filepath.Join(dir, ".git")
		_, gitErr := os.Stat(gitDir)
		atRepoRoot := gitErr == nil

		// Try to load .code-reviewer.yaml/.yml (stop walking after first match).
		if !foundYAML {
			path := filepath.Join(dir, ".code-reviewer.yaml")
			data, err := os.ReadFile(path)
			if err == nil {
				if err := c.applyRepoConfig(data); err != nil {
					return err
				}
				foundYAML = true
			} else {
				path = filepath.Join(dir, ".code-reviewer.yml")
				data, err = os.ReadFile(path)
				if err == nil {
					if err := c.applyRepoConfig(data); err != nil {
						return err
					}
					foundYAML = true
				}
			}
		}

		// Try to load REVIEW.md (only if not already found).
		if !foundReviewMD {
			path := filepath.Join(dir, "REVIEW.md")
			data, err := os.ReadFile(path)
			if err == nil {
				c.ReviewMD = strings.TrimSpace(string(data))
				foundReviewMD = true
			}
		}

		// Stop if both found, at repo root, or reached filesystem root.
		if foundYAML && foundReviewMD {
			break
		}
		if atRepoRoot {
			break // Don't walk past the repo boundary.
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
	if rc.CleanupMode != "" {
		c.CleanupMode = CleanupMode(rc.CleanupMode)
	}
	if rc.ChunkStrategy != "" {
		c.ChunkStrategy = ChunkStrategy(rc.ChunkStrategy)
	}
	if rc.CacheDir != "" {
		c.CacheDir = rc.CacheDir
	}
	if rc.NoCache != nil {
		c.NoCache = *rc.NoCache
	}
	if rc.CacheMaxAge != "" {
		d, err := parseDuration(rc.CacheMaxAge)
		if err != nil {
			return fmt.Errorf("invalid cache_max_age: %w", err)
		}
		c.CacheMaxAge = d
	}
	if len(rc.ExcludedPatterns) > 0 {
		c.ExcludedPatterns = rc.ExcludedPatterns
	}
	if rc.ExtraRules != "" {
		c.ExtraRules = rc.ExtraRules
	}
	if rc.OutputJSON {
		c.OutputJSON = true
	}
	if rc.AuditLog != "" {
		c.AuditLog = rc.AuditLog
	}
	if rc.CustomPrompt != "" {
		c.CustomPrompt = rc.CustomPrompt
	}
	if rc.ProxyURL != "" {
		c.ProxyURL = rc.ProxyURL
	}
	if rc.MaxTokens > 0 {
		c.MaxTokens = rc.MaxTokens
	}
	if rc.MaxFiles > 0 {
		c.MaxFiles = rc.MaxFiles
	}
	if rc.ScopeAction != "" {
		c.ScopeAction = rc.ScopeAction
	}
	if rc.APIURL != "" {
		c.APIURL = rc.APIURL
	}
	if rc.Summarize {
		c.Summarize = true
	}
	if rc.SummaryUpdateDescription {
		c.SummaryUpdateDescription = true
	}
	if rc.UpdateDescription != nil {
		c.UpdateDescription = *rc.UpdateDescription
	}
	if rc.IntentReview != nil {
		c.IntentReview = *rc.IntentReview
		if !*rc.IntentReview {
			c.NoIntentReview = true // Explicit false prevents CI auto-enable.
		}
	}
	if rc.AutoApprove != nil {
		c.AutoApprove = *rc.AutoApprove
	}
	if len(rc.Rules) > 0 {
		if err := ValidateRules(rc.Rules); err != nil {
			return fmt.Errorf("invalid config: %w", err)
		}
		c.Rules = rc.Rules
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
	if v := os.Getenv("REVIEW_CACHE_DIR"); v != "" {
		c.CacheDir = v
	}
	if v := os.Getenv("REVIEW_NO_CACHE"); strings.EqualFold(v, "true") {
		c.NoCache = true
	}
	if v := os.Getenv("REVIEW_COMMENT_MODE"); v != "" {
		c.CommentMode = CommentMode(v)
	}
	if v := os.Getenv("CODE_REVIEWER_CLEANUP_MODE"); v != "" {
		c.CleanupMode = CleanupMode(v)
	}
	if v := os.Getenv("CODE_REVIEWER_UPDATE_DESCRIPTION"); strings.EqualFold(v, "true") {
		c.UpdateDescription = true
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
	if v := os.Getenv("REVIEW_OUTPUT_JSON"); strings.EqualFold(v, "true") {
		c.OutputJSON = true
	}
	if v := os.Getenv("REVIEW_CUSTOM_PROMPT"); v != "" {
		c.CustomPrompt = v
	}
	// Respect the NO_COLOR standard (https://no-color.org/).
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		c.NoColor = true
	}
	if v := os.Getenv("SARIF_OUTPUT"); v != "" {
		c.SARIFOutput = v
	}
	if v := os.Getenv("REVIEW_AUDIT_LOG"); v != "" {
		c.AuditLog = v
	}
	if v := os.Getenv("REVIEW_MODELS"); v != "" {
		c.Models = splitAndTrim(v)
	}
	if v := os.Getenv("INCREMENTAL"); strings.EqualFold(v, "true") {
		c.Incremental = true
	}
	if v := os.Getenv("REVIEW_PROXY_URL"); v != "" {
		c.ProxyURL = v
	}
	if v := os.Getenv("REVIEW_MAX_TOKENS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			slog.Warn("ignoring invalid REVIEW_MAX_TOKENS", "value", v, "error", err)
		} else if n < 0 {
			slog.Warn("ignoring negative REVIEW_MAX_TOKENS", "value", n)
		} else {
			// 0 = unlimited (clears any YAML cap), >0 = token budget.
			c.MaxTokens = n
		}
	}
	if v := os.Getenv("REVIEW_MAX_FILES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			slog.Warn("ignoring invalid REVIEW_MAX_FILES", "value", v, "error", err)
		} else if n < 0 {
			slog.Warn("ignoring negative REVIEW_MAX_FILES", "value", n)
		} else {
			c.MaxFiles = n
		}
	}
	if v := os.Getenv("REVIEW_SCOPE_ACTION"); v != "" {
		c.ScopeAction = v
	}
	if v := os.Getenv("REVIEW_API_URL"); v != "" {
		c.APIURL = v
	}
	if v := os.Getenv("REVIEW_API_KEY"); v != "" {
		c.APIKey = v
	}
	if v := os.Getenv("REVIEW_INTENT"); v != "" {
		c.IntentReview = strings.EqualFold(v, "true") || v == "1"
		if strings.EqualFold(v, "false") || v == "0" {
			c.NoIntentReview = true
		}
	}
	if v := os.Getenv("REVIEW_AUTO_APPROVE"); v != "" {
		if strings.EqualFold(v, "true") || v == "1" {
			c.AutoApprove = true
		} else if strings.EqualFold(v, "false") || v == "0" {
			c.AutoApprove = false
		}
	}
}

func (c *Config) loadFlags() error {
	fs := flag.NewFlagSet("code-reviewer", flag.ContinueOnError)

	ci := fs.Bool("ci", false, "Run in GitLab CI mode (auto-detect MR from env vars)")
	diffFlag := fs.Bool("diff", false, "Review local git diff (default: against origin/HEAD)")
	files := fs.String("files", "", "Comma-separated list of files to review")

	cacheDir := fs.String("cache-dir", "", "Cache directory")
	noCache := fs.Bool("no-cache", false, "Disable caching")

	model := fs.String("model", "", "Vertex AI model ID (e.g., gemini-2.5-flash, claude-sonnet-4)")
	focus := fs.String("focus", "", "Review focus areas, comma-separated (bugs,security,performance,style,docs,all)")
	minSev := fs.String("min-severity", "", "Minimum severity to report (low, medium, high, critical)")
	commentMode := fs.String("comment-mode", "", "GitLab comment mode: notes (simple) or discussions (inline)")
	cleanupMode := fs.String("cleanup-mode", "", "How to handle previous bot comments (delete or resolve)")
	chunkStrategy := fs.String("chunk-strategy", "", "How to handle large diffs: fail (default) or split")
	extraRules := fs.String("extra-rules", "", "Additional review rules appended to prompt")
	dryRun := fs.Bool("dry-run", false, "Run analysis but don't post to GitLab")
	outputJSON := fs.Bool("json", false, "Output results as JSON to stdout")
	_ = fs.Bool("version", false, "Print version and exit") // Handled in main() before config.Load().
	customPrompt := fs.String("custom-prompt", "", "Path to a custom system prompt file")
	noColor := fs.Bool("no-color", false, "Disable ANSI color output")
	sarifOutput := fs.String("sarif", "", "Write SARIF 2.1.0 output to the given file path")
	auditLog := fs.String("audit-log", "", "Write structured JSONL audit log to file")
	models := fs.String("models", "", "Comma-separated list of models for consensus review")
	consensusThreshold := fs.Int("consensus-threshold", 0, "Min models that must agree on a finding (default: 2)")
	incremental := fs.Bool("incremental", false, "Only review files changed in the latest push (CI mode)")
	proxyURL := fs.String("proxy-url", "", "LLM proxy URL for observability (e.g., http://localhost:8181/proxy/google/)")
	noContext := fs.Bool("no-context", false, "Disable repo-aware context discovery")
	maxTokens := fs.Int("max-tokens", 0, "Maximum total tokens (input+output) per review (0 = unlimited)")
	maxFiles := fs.Int("max-files", 0, "Maximum files before scope warning (0 = unlimited)")
	scopeAction := fs.String("scope-action", "warn", "Action when scope exceeded: warn or fail")
	apiURL := fs.String("api-url", "", "OpenAI-compatible API endpoint (e.g., http://localhost:11434/v1)")
	apiKey := fs.String("api-key", "", "API key for HTTP provider (optional for IAM/ADC auth)")
	summarize := fs.Bool("summarize", false, "Generate MR summary instead of review")
	summaryUpdateDesc := fs.Bool("summary-update-description", false, "Update MR description with the generated summary")
	updateDesc := fs.Bool("update-description", false, "Inject review summary into MR/PR description")
	intentFlag := fs.Bool("intent", false, "Enable two-pass intent-aware review")
	noIntentFlag := fs.Bool("no-intent", false, "Disable intent-aware review (overrides CI default)")
	explain := fs.Bool("explain", false, "Explain the diff instead of reviewing it")
	fix := fs.Bool("fix", false, "Apply suggested fixes to the working tree after review")
	autoApprove := fs.Bool("auto-approve", false, "Automatically approve MR/PR when review finds no issues (CI mode only)")

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

	if *cacheDir != "" {
		c.CacheDir = *cacheDir
	}
	if *noCache {
		c.NoCache = true
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
	if *cleanupMode != "" {
		c.CleanupMode = CleanupMode(*cleanupMode)
	}
	if *chunkStrategy != "" {
		c.ChunkStrategy = ChunkStrategy(*chunkStrategy)
	}
	if *extraRules != "" {
		c.ExtraRules = *extraRules
	}
	if *outputJSON {
		c.OutputJSON = true
	}
	if *customPrompt != "" {
		c.CustomPrompt = *customPrompt
	}
	if *noColor {
		c.NoColor = true
	}
	if *sarifOutput != "" {
		c.SARIFOutput = *sarifOutput
	}
	if *auditLog != "" {
		c.AuditLog = *auditLog
	}
	if *models != "" {
		c.Models = splitAndTrim(*models)
	}
	if *consensusThreshold > 0 {
		c.ConsensusThreshold = *consensusThreshold
	}
	if *incremental {
		c.Incremental = true
	}
	if *proxyURL != "" {
		c.ProxyURL = *proxyURL
	}
	if *noContext {
		c.DisableContext = true
	}
	// Detect if --max-tokens was explicitly set (including to 0 for unlimited).
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "max-tokens" {
			if *maxTokens < 0 {
				slog.Warn("ignoring negative --max-tokens", "value", *maxTokens)
				return
			}
			c.MaxTokens = *maxTokens
		}
		if f.Name == "max-files" {
			if *maxFiles < 0 {
				slog.Warn("ignoring negative --max-files", "value", *maxFiles)
				return
			}
			c.MaxFiles = *maxFiles
		}
		if f.Name == "scope-action" {
			c.ScopeAction = *scopeAction
		}
	})
	if *apiURL != "" {
		c.APIURL = *apiURL
	}
	if *apiKey != "" {
		c.APIKey = *apiKey
	}
	if *summarize {
		c.Summarize = true
	}
	if *summaryUpdateDesc {
		c.SummaryUpdateDescription = true
	}
	if *updateDesc {
		c.UpdateDescription = true
	}
	if *intentFlag {
		c.IntentReview = true
	}
	if *noIntentFlag {
		c.NoIntentReview = true
	}
	if *explain {
		c.Explain = true
	}
	if *fix {
		c.Fix = true
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "auto-approve" {
			c.AutoApprove = *autoApprove
		}
	})
	return nil
}

func (c *Config) loadCIEnv() {
	// Auto-detect platform from CI environment unless explicitly configured.
	// Check GitLab first: CI_PROJECT_ID is specific to GitLab CI,
	// while GITHUB_ACTIONS is set for all GitHub Actions jobs (even
	// ones that test GitLab-targeting tools).
	switch {
	case c.Platform != "":
		// Platform was explicitly configured (e.g., via flag or yaml).
		switch c.Platform {
		case "github":
			c.loadGitHubCIEnv()
		case "gitlab":
			c.loadGitLabCIEnv()
		}
	case os.Getenv("CI_PROJECT_ID") != "":
		c.Platform = "gitlab"
		c.loadGitLabCIEnv()
	case os.Getenv("GITHUB_ACTIONS") == "true":
		c.Platform = "github"
		c.loadGitHubCIEnv()
	default:
		// Try GitLab env vars anyway for manual runs.
		c.loadGitLabCIEnv()
	}
}

func (c *Config) loadGitLabCIEnv() {
	c.CIProjectID = os.Getenv("CI_PROJECT_ID")
	c.CIMergeRequestID = os.Getenv("CI_MERGE_REQUEST_IID")
	c.CIDiffBaseSHA = os.Getenv("CI_MERGE_REQUEST_DIFF_BASE_SHA")
	c.CICommitBeforeSHA = os.Getenv("CI_COMMIT_BEFORE_SHA")
}

func (c *Config) loadGitHubCIEnv() {
	c.CIProjectID = os.Getenv("GITHUB_REPOSITORY") // "owner/repo"
	c.GitHubToken = os.Getenv("GITHUB_TOKEN")
	if v := os.Getenv("GITHUB_API_URL"); v != "" {
		c.GitHubBaseURL = v // GitHub Enterprise support.
	}

	// Parse PR number from GITHUB_REF (e.g., "refs/pull/42/merge").
	if ref := os.Getenv("GITHUB_REF"); ref != "" {
		parts := strings.Split(ref, "/")
		if len(parts) >= 3 && parts[1] == "pull" {
			c.CIMergeRequestID = parts[2]
		}
	}
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

	// CI mode requires MR/PR context.
	if c.CIMode {
		if c.CIProjectID == "" || c.CIMergeRequestID == "" {
			if c.Platform == "github" {
				return fmt.Errorf("CI mode requires GITHUB_REPOSITORY and a pull_request event\n\n" +
					"Ensure your workflow uses: on: pull_request\n" +
					"GITHUB_REF must match refs/pull/<number>/merge")
			}
			return fmt.Errorf("CI mode requires CI_PROJECT_ID and CI_MERGE_REQUEST_IID env vars\n\n" +
				"These are set automatically when running in a GitLab MR pipeline.\n" +
				"If running locally, use --diff instead of --ci")
		}

		switch c.Platform {
		case "github":
			if c.GitHubToken == "" && !c.DryRun {
				return fmt.Errorf("CI mode on GitHub requires GITHUB_TOKEN env var\n\n" +
					"Add 'permissions: pull-requests: write' to your workflow")
			}
		default: // gitlab
			// Validate GitLab URL scheme to prevent token leakage over plain HTTP.
			if !strings.HasPrefix(c.GitLabBaseURL, "https://") {
				if os.Getenv("CODE_REVIEWER_ALLOW_INSECURE") != "true" {
					return fmt.Errorf("GITLAB_BASE_URL must use HTTPS to protect tokens\n\n"+
						"Current URL: %s\n"+
						"Set CODE_REVIEWER_ALLOW_INSECURE=true to override (not recommended)",
						c.GitLabBaseURL)
				}
			}
			if c.GitLabToken == "" && !c.DryRun {
				return fmt.Errorf("CI mode requires GITLAB_TOKEN env var\n\n" +
					"Options:\n" +
					"  CI_JOB_TOKEN:  Add 'GITLAB_TOKEN: $CI_JOB_TOKEN' to your job variables\n" +
					"  Access Token:  Create a Project Access Token with api scope")
			}
		}
	}

	// GCP project required for Vertex AI model calls, but not for HTTP provider.
	if c.GCPProject == "" && c.APIURL == "" {
		return fmt.Errorf("GOOGLE_CLOUD_PROJECT is required for Vertex AI, or use --api-url for an OpenAI-compatible endpoint")
	}

	// Validate comment mode.
	if c.CommentMode != CommentModeNotes && c.CommentMode != CommentModeDiscussions {
		return fmt.Errorf("invalid comment-mode: %q (valid: notes, discussions)", c.CommentMode)
	}

	// Validate cleanup mode.
	if c.CleanupMode != CleanupModeDelete && c.CleanupMode != CleanupModeResolve {
		return fmt.Errorf("invalid cleanup-mode: %q (valid: delete, resolve)", c.CleanupMode)
	}

	// Validate chunk strategy.
	if c.ChunkStrategy != ChunkStrategyFail && c.ChunkStrategy != ChunkStrategySplit {
		return fmt.Errorf("invalid chunk-strategy: %q (valid: fail, split)", c.ChunkStrategy)
	}

	// Summarize mode is single-model only.
	if c.Summarize && len(c.Models) > 0 {
		return fmt.Errorf("--summarize cannot be used with --models (multi-model consensus); use --model instead")
	}

	if c.IntentReview && c.Summarize {
		return fmt.Errorf("--intent and --summarize are mutually exclusive; intent review includes summarization")
	}

	if c.Explain && c.Summarize {
		return fmt.Errorf("--explain and --summarize are mutually exclusive")
	}
	if c.Explain && c.IntentReview {
		return fmt.Errorf("--explain and --intent are mutually exclusive")
	}

	if c.Fix && !c.DiffMode {
		return fmt.Errorf("--fix requires --diff mode (cannot modify files from CI)")
	}
	if c.Fix && c.Explain {
		return fmt.Errorf("--fix and --explain are mutually exclusive")
	}
	if c.Fix && c.Summarize {
		return fmt.Errorf("--fix and --summarize are mutually exclusive")
	}
	if c.AutoApprove && !c.CIMode {
		return fmt.Errorf("--auto-approve requires --ci mode")
	}
	if c.AutoApprove && c.DryRun {
		return fmt.Errorf("--auto-approve cannot be used with --dry-run")
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

// splitAndTrim splits a comma-separated string, trims whitespace, and drops empty entries.
func splitAndTrim(s string) []string {
	var result []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func parseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		daysStr := strings.TrimSuffix(s, "d")
		days, err := strconv.Atoi(daysStr)
		if err != nil {
			return 0, fmt.Errorf("invalid days format: %s", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
