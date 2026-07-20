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
	DryRun      bool
	OutputJSON  bool
	NoColor     bool   // Disable ANSI color output.
	SARIFOutput string // Path to write SARIF 2.1.0 output file.
	ProxyURL    string // Optional: LLM proxy URL for observability (e.g., Candela).

	// GitLab settings.
	GitLabToken   string
	GitLabBaseURL string
	SkipDraftMRs  bool

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

	// Budget.
	MaxTokens int // Maximum total tokens per review (0 = unlimited).
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
	OutputJSON       bool     `yaml:"output_json"`
	CustomPrompt             string   `yaml:"custom_prompt"`
	ProxyURL                 string   `yaml:"proxy_url"`
	MaxTokens                int      `yaml:"max_tokens"`
	APIURL                   string   `yaml:"api_url"`
	Summarize                bool     `yaml:"summarize"`
	SummaryUpdateDescription bool     `yaml:"summary_update_description"`
	IntentReview             *bool    `yaml:"intent_review"`
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
	if rc.ChunkStrategy != "" {
		c.ChunkStrategy = ChunkStrategy(rc.ChunkStrategy)
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
	if rc.CustomPrompt != "" {
		c.CustomPrompt = rc.CustomPrompt
	}
	if rc.ProxyURL != "" {
		c.ProxyURL = rc.ProxyURL
	}
	if rc.MaxTokens > 0 {
		c.MaxTokens = rc.MaxTokens
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
	if rc.IntentReview != nil {
		c.IntentReview = *rc.IntentReview
		if !*rc.IntentReview {
			c.NoIntentReview = true // Explicit false prevents CI auto-enable.
		}
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
	outputJSON := fs.Bool("json", false, "Output results as JSON to stdout")
	_ = fs.Bool("version", false, "Print version and exit") // Handled in main() before config.Load().
	customPrompt := fs.String("custom-prompt", "", "Path to a custom system prompt file")
	noColor := fs.Bool("no-color", false, "Disable ANSI color output")
	sarifOutput := fs.String("sarif", "", "Write SARIF 2.1.0 output to the given file path")
	models := fs.String("models", "", "Comma-separated list of models for consensus review")
	consensusThreshold := fs.Int("consensus-threshold", 0, "Min models that must agree on a finding (default: 2)")
	incremental := fs.Bool("incremental", false, "Only review files changed in the latest push (CI mode)")
	proxyURL := fs.String("proxy-url", "", "LLM proxy URL for observability (e.g., http://localhost:8181/proxy/google/)")
	noContext := fs.Bool("no-context", false, "Disable repo-aware context discovery")
	maxTokens := fs.Int("max-tokens", 0, "Maximum total tokens (input+output) per review (0 = unlimited)")
	apiURL := fs.String("api-url", "", "OpenAI-compatible API endpoint (e.g., http://localhost:11434/v1)")
	apiKey := fs.String("api-key", "", "API key for HTTP provider (optional for IAM/ADC auth)")
	summarize := fs.Bool("summarize", false, "Generate MR summary instead of review")
	summaryUpdateDesc := fs.Bool("summary-update-description", false, "Update MR description with the generated summary")
	intentFlag := fs.Bool("intent", false, "Enable two-pass intent-aware review")
	noIntentFlag := fs.Bool("no-intent", false, "Disable intent-aware review (overrides CI default)")

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
	if *intentFlag {
		c.IntentReview = true
	}
	if *noIntentFlag {
		c.NoIntentReview = true
	}
	return nil
}

func (c *Config) loadCIEnv() {
	c.CIProjectID = os.Getenv("CI_PROJECT_ID")
	c.CIMergeRequestID = os.Getenv("CI_MERGE_REQUEST_IID")
	c.CIDiffBaseSHA = os.Getenv("CI_MERGE_REQUEST_DIFF_BASE_SHA")
	c.CICommitBeforeSHA = os.Getenv("CI_COMMIT_BEFORE_SHA")
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
			return fmt.Errorf("CI mode requires CI_PROJECT_ID and CI_MERGE_REQUEST_IID env vars\n\n" +
				"These are set automatically when running in a GitLab MR pipeline.\n" +
				"If running locally, use --diff instead of --ci")
		}
		// Validate GitLab URL scheme to prevent token leakage over plain HTTP.
		if !strings.HasPrefix(c.GitLabBaseURL, "https://") {
			if os.Getenv("CODE_REVIEWER_ALLOW_INSECURE") != "true" {
				return fmt.Errorf("GITLAB_BASE_URL must use HTTPS to protect tokens\n\n"+
					"Current URL: %s\n"+
					"Set CODE_REVIEWER_ALLOW_INSECURE=true to override (not recommended)",
					c.GitLabBaseURL)
			}
		}
		if c.GitLabToken == "" && (!c.Summarize || !c.DryRun) {
			return fmt.Errorf("CI mode requires GITLAB_TOKEN env var\n\n" +
				"Options:\n" +
				"  CI_JOB_TOKEN:  Add 'GITLAB_TOKEN: $CI_JOB_TOKEN' to your job variables\n" +
				"  Access Token:  Create a Project Access Token with api scope")
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
