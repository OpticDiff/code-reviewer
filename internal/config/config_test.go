package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParseSeverity_AllLevels is a table-driven test covering all valid
// severity strings (including case variations) plus invalid input.
func TestParseSeverity_AllLevels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    Severity
		wantErr bool
	}{
		{name: "low", input: "low", want: SeverityLow},
		{name: "LOW", input: "LOW", want: SeverityLow},
		{name: "Low", input: "Low", want: SeverityLow},
		{name: "medium", input: "medium", want: SeverityMedium},
		{name: "MEDIUM", input: "MEDIUM", want: SeverityMedium},
		{name: "high", input: "high", want: SeverityHigh},
		{name: "HIGH", input: "HIGH", want: SeverityHigh},
		{name: "critical", input: "critical", want: SeverityCritical},
		{name: "CRITICAL", input: "CRITICAL", want: SeverityCritical},
		{name: "with_whitespace", input: "  high  ", want: SeverityHigh},
		{name: "invalid", input: "urgent", want: SeverityLow, wantErr: true},
		{name: "empty", input: "", want: SeverityLow, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseSeverity(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseSeverity(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseSeverity(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestSeverity_String verifies String() for every Severity constant and an
// unknown value.
func TestSeverity_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		sev  Severity
		want string
	}{
		{SeverityLow, "LOW"},
		{SeverityMedium, "MEDIUM"},
		{SeverityHigh, "HIGH"},
		{SeverityCritical, "CRITICAL"},
		{Severity(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			if got := tt.sev.String(); got != tt.want {
				t.Errorf("Severity(%d).String() = %q, want %q", tt.sev, got, tt.want)
			}
		})
	}
}

// TestLoad_Defaults verifies that Load() with minimal env (GOOGLE_CLOUD_PROJECT
// set and --diff flag) produces the expected default configuration.
func TestLoad_Defaults(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"code-reviewer", "--diff"}

	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")

	// Clear env vars that could interfere.
	t.Setenv("REVIEW_MODEL", "")
	t.Setenv("REVIEW_FOCUS", "")
	t.Setenv("REVIEW_MIN_SEVERITY", "")
	t.Setenv("REVIEW_COMMENT_MODE", "")
	t.Setenv("REVIEW_CHUNK_STRATEGY", "")
	t.Setenv("REVIEW_OUTPUT_JSON", "")
	t.Setenv("GITLAB_BASE_URL", "")
	t.Setenv("SKIP_DRAFT_MRS", "")
	t.Setenv("EXCLUDED_PATTERNS", "")
	t.Setenv("REVIEW_EXTRA_RULES", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Model != "gemini-2.5-flash" {
		t.Errorf("Model = %q, want %q", cfg.Model, "gemini-2.5-flash")
	}
	if cfg.GCPLocation != "us-central1" {
		t.Errorf("GCPLocation = %q, want %q", cfg.GCPLocation, "us-central1")
	}
	if len(cfg.Focus) != 1 || cfg.Focus[0] != "all" {
		t.Errorf("Focus = %v, want [all]", cfg.Focus)
	}
	if cfg.MinSeverity != SeverityLow {
		t.Errorf("MinSeverity = %v, want SeverityLow", cfg.MinSeverity)
	}
	if cfg.CommentMode != CommentModeNotes {
		t.Errorf("CommentMode = %q, want %q", cfg.CommentMode, CommentModeNotes)
	}
	if cfg.ChunkStrategy != ChunkStrategyFail {
		t.Errorf("ChunkStrategy = %q, want %q", cfg.ChunkStrategy, ChunkStrategyFail)
	}
	if !cfg.SkipDraftMRs {
		t.Error("SkipDraftMRs = false, want true")
	}
	if cfg.GitLabBaseURL != "https://gitlab.com" {
		t.Errorf("GitLabBaseURL = %q, want %q", cfg.GitLabBaseURL, "https://gitlab.com")
	}
	if !cfg.DiffMode {
		t.Error("DiffMode = false, want true")
	}
	if cfg.GCPProject != "test-project" {
		t.Errorf("GCPProject = %q, want %q", cfg.GCPProject, "test-project")
	}
}

// TestLoad_EnvOverrides sets REVIEW_* env vars and verifies they override
// defaults.
func TestLoad_EnvOverrides(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"code-reviewer", "--diff"}

	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	t.Setenv("REVIEW_MODEL", "claude-sonnet-4")
	t.Setenv("REVIEW_FOCUS", "bugs,security")
	t.Setenv("REVIEW_MIN_SEVERITY", "high")
	t.Setenv("REVIEW_OUTPUT_JSON", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Model != "claude-sonnet-4" {
		t.Errorf("Model = %q, want %q", cfg.Model, "claude-sonnet-4")
	}
	if len(cfg.Focus) != 2 || cfg.Focus[0] != "bugs" || cfg.Focus[1] != "security" {
		t.Errorf("Focus = %v, want [bugs security]", cfg.Focus)
	}
	if cfg.MinSeverity != SeverityHigh {
		t.Errorf("MinSeverity = %v, want SeverityHigh", cfg.MinSeverity)
	}
	if !cfg.OutputJSON {
		t.Error("OutputJSON = false, want true")
	}
}

// TestLoad_FlagOverrides verifies that CLI flags override both defaults and env
// vars.
func TestLoad_FlagOverrides(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{
		"code-reviewer",
		"--diff",
		"--model", "gemini-2.5-pro",
		"--focus", "performance,docs",
		"--min-severity", "critical",
		"--json",
	}

	// Set env vars that should be overridden by flags.
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	t.Setenv("REVIEW_MODEL", "env-model-should-be-overridden")
	t.Setenv("REVIEW_FOCUS", "env-focus-should-be-overridden")
	t.Setenv("REVIEW_MIN_SEVERITY", "low")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Model != "gemini-2.5-pro" {
		t.Errorf("Model = %q, want %q (flag should override env)", cfg.Model, "gemini-2.5-pro")
	}
	if len(cfg.Focus) != 2 || cfg.Focus[0] != "performance" || cfg.Focus[1] != "docs" {
		t.Errorf("Focus = %v, want [performance docs]", cfg.Focus)
	}
	if cfg.MinSeverity != SeverityCritical {
		t.Errorf("MinSeverity = %v, want SeverityCritical", cfg.MinSeverity)
	}
	if !cfg.OutputJSON {
		t.Error("OutputJSON = false, want true")
	}
}

// TestLoad_ValidationErrors is a table-driven test for all validation error
// scenarios.
func TestLoad_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		env     map[string]string
		wantSub string // substring expected in error message
	}{
		{
			name:    "no_input_mode",
			args:    []string{"code-reviewer"},
			env:     map[string]string{"GOOGLE_CLOUD_PROJECT": "p"},
			wantSub: "must specify one of",
		},
		{
			name:    "conflicting_modes_ci_and_diff",
			args:    []string{"code-reviewer", "--ci", "--diff"},
			env:     map[string]string{"GOOGLE_CLOUD_PROJECT": "p", "CI_PROJECT_ID": "1", "CI_MERGE_REQUEST_IID": "10", "GITLAB_TOKEN": "tok"},
			wantSub: "only one input mode allowed",
		},
		{
			name:    "ci_without_project_id",
			args:    []string{"code-reviewer", "--ci"},
			env:     map[string]string{"GOOGLE_CLOUD_PROJECT": "p", "CI_PROJECT_ID": "", "CI_MERGE_REQUEST_IID": "", "GITLAB_TOKEN": "tok", "GITHUB_ACTIONS": ""},
			wantSub: "CI_PROJECT_ID",
		},
		{
			name:    "ci_without_gitlab_token",
			args:    []string{"code-reviewer", "--ci"},
			env:     map[string]string{"GOOGLE_CLOUD_PROJECT": "p", "CI_PROJECT_ID": "1", "CI_MERGE_REQUEST_IID": "10", "GITLAB_TOKEN": ""},
			wantSub: "GITLAB_TOKEN",
		},
		{
			name:    "missing_gcp_project",
			args:    []string{"code-reviewer", "--diff"},
			env:     map[string]string{"GOOGLE_CLOUD_PROJECT": ""},
			wantSub: "GOOGLE_CLOUD_PROJECT",
		},
		{
			name:    "invalid_comment_mode",
			args:    []string{"code-reviewer", "--diff", "--comment-mode", "invalid"},
			env:     map[string]string{"GOOGLE_CLOUD_PROJECT": "test-project"},
			wantSub: "invalid comment-mode",
		},
		{
			name:    "invalid_chunk_strategy",
			args:    []string{"code-reviewer", "--diff", "--chunk-strategy", "bogus"},
			env:     map[string]string{"GOOGLE_CLOUD_PROJECT": "test-project"},
			wantSub: "invalid chunk-strategy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldArgs := os.Args
			defer func() { os.Args = oldArgs }()
			os.Args = tt.args

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			_, err := Load()
			if err == nil {
				t.Fatal("Load() expected error, got nil")
			}
			if got := err.Error(); !contains(got, tt.wantSub) {
				t.Errorf("error = %q, want substring %q", got, tt.wantSub)
			}
		})
	}
}

// TestLoad_HTTPSValidation ensures CI mode rejects http:// URLs and that
// CODE_REVIEWER_ALLOW_INSECURE=true overrides this.
func TestLoad_HTTPSValidation(t *testing.T) {
	t.Run("http_rejected", func(t *testing.T) {
		oldArgs := os.Args
		defer func() { os.Args = oldArgs }()
		os.Args = []string{"code-reviewer", "--ci"}

		t.Setenv("GOOGLE_CLOUD_PROJECT", "p")
		t.Setenv("CI_PROJECT_ID", "1")
		t.Setenv("CI_MERGE_REQUEST_IID", "10")
		t.Setenv("GITLAB_TOKEN", "tok")
		t.Setenv("GITLAB_BASE_URL", "http://gitlab.internal")
		t.Setenv("CODE_REVIEWER_ALLOW_INSECURE", "")

		_, err := Load()
		if err == nil {
			t.Fatal("Load() expected HTTPS error, got nil")
		}
		if got := err.Error(); !contains(got, "HTTPS") {
			t.Errorf("error = %q, want substring %q", got, "HTTPS")
		}
	})

	t.Run("http_allowed_with_insecure_flag", func(t *testing.T) {
		oldArgs := os.Args
		defer func() { os.Args = oldArgs }()
		os.Args = []string{"code-reviewer", "--ci"}

		t.Setenv("GOOGLE_CLOUD_PROJECT", "p")
		t.Setenv("CI_PROJECT_ID", "1")
		t.Setenv("CI_MERGE_REQUEST_IID", "10")
		t.Setenv("GITLAB_TOKEN", "tok")
		t.Setenv("GITLAB_BASE_URL", "http://gitlab.internal")
		t.Setenv("CODE_REVIEWER_ALLOW_INSECURE", "true")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() unexpected error: %v", err)
		}
		if cfg.GitLabBaseURL != "http://gitlab.internal" {
			t.Errorf("GitLabBaseURL = %q, want %q", cfg.GitLabBaseURL, "http://gitlab.internal")
		}
	})
}

// TestLoad_YAMLConfig creates a temp .code-reviewer.yaml, changes into its
// directory, and verifies the YAML values are loaded.
func TestLoad_YAMLConfig(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"code-reviewer", "--diff"}

	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	// Clear env vars that could override yaml values.
	t.Setenv("REVIEW_MODEL", "")
	t.Setenv("REVIEW_FOCUS", "")
	t.Setenv("REVIEW_MIN_SEVERITY", "")
	t.Setenv("REVIEW_EXTRA_RULES", "")
	t.Setenv("REVIEW_PROXY_URL", "")

	tmpDir := t.TempDir()
	yamlContent := []byte(`model: yaml-model
focus:
  - security
  - bugs
min_severity: medium
extra_rules: "always check for nil"
output_json: true
proxy_url: http://yaml-proxy:8181/proxy/google/
`)
	if err := os.WriteFile(filepath.Join(tmpDir, ".code-reviewer.yaml"), yamlContent, 0o644); err != nil {
		t.Fatalf("writing yaml: %v", err)
	}

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer os.Chdir(oldDir) //nolint:errcheck
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Model != "yaml-model" {
		t.Errorf("Model = %q, want %q", cfg.Model, "yaml-model")
	}
	if len(cfg.Focus) != 2 || cfg.Focus[0] != "security" || cfg.Focus[1] != "bugs" {
		t.Errorf("Focus = %v, want [security bugs]", cfg.Focus)
	}
	if cfg.MinSeverity != SeverityMedium {
		t.Errorf("MinSeverity = %v, want SeverityMedium", cfg.MinSeverity)
	}
	if cfg.ExtraRules != "always check for nil" {
		t.Errorf("ExtraRules = %q, want %q", cfg.ExtraRules, "always check for nil")
	}
	if !cfg.OutputJSON {
		t.Error("OutputJSON = false, want true")
	}
	if cfg.ProxyURL != "http://yaml-proxy:8181/proxy/google/" {
		t.Errorf("ProxyURL = %q, want %q", cfg.ProxyURL, "http://yaml-proxy:8181/proxy/google/")
	}
}

// TestLoad_VersionFlag verifies that --version is accepted as a known flag and
// does not cause a parsing error (it's handled in main() before Load()).
func TestLoad_VersionFlag(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"code-reviewer", "--version", "--diff"}

	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")

	_, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error with --version: %v", err)
	}
}

// TestLoad_JSONFlag verifies that the --json flag sets OutputJSON.
func TestLoad_JSONFlag(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"code-reviewer", "--diff", "--json"}

	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	t.Setenv("REVIEW_OUTPUT_JSON", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if !cfg.OutputJSON {
		t.Error("OutputJSON = false, want true (--json flag should set it)")
	}
}

// TestLoad_ProxyURLFlag verifies that the --proxy-url flag sets ProxyURL.
func TestLoad_ProxyURLFlag(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"code-reviewer", "--diff", "--proxy-url", "http://localhost:8181/proxy/google/"}

	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	t.Setenv("REVIEW_PROXY_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.ProxyURL != "http://localhost:8181/proxy/google/" {
		t.Errorf("ProxyURL = %q, want %q", cfg.ProxyURL, "http://localhost:8181/proxy/google/")
	}
}

// TestLoad_ProxyURLEnv verifies that REVIEW_PROXY_URL env var sets ProxyURL.
func TestLoad_ProxyURLEnv(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"code-reviewer", "--diff"}

	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	t.Setenv("REVIEW_PROXY_URL", "https://candela.example.com/proxy/google/")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.ProxyURL != "https://candela.example.com/proxy/google/" {
		t.Errorf("ProxyURL = %q, want %q", cfg.ProxyURL, "https://candela.example.com/proxy/google/")
	}
}

// TestLoad_ProxyURLFlagOverridesEnv verifies that --proxy-url flag overrides
// REVIEW_PROXY_URL env var.
func TestLoad_ProxyURLFlagOverridesEnv(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"code-reviewer", "--diff", "--proxy-url", "http://flag-proxy/"}

	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	t.Setenv("REVIEW_PROXY_URL", "http://env-proxy/")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.ProxyURL != "http://flag-proxy/" {
		t.Errorf("ProxyURL = %q, want %q (flag should override env)", cfg.ProxyURL, "http://flag-proxy/")
	}
}

// TestLoad_FullPrecedenceChain verifies that all three config layers are
// applied in the correct order: yaml < env < flag.
func TestLoad_FullPrecedenceChain(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{
		"code-reviewer",
		"--diff",
		"--model", "flag-model",
		"--min-severity", "critical",
	}

	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	t.Setenv("REVIEW_MODEL", "env-model-should-lose-to-flag")
	t.Setenv("REVIEW_MIN_SEVERITY", "medium")
	t.Setenv("REVIEW_FOCUS", "")
	t.Setenv("REVIEW_EXTRA_RULES", "")
	t.Setenv("REVIEW_PROXY_URL", "")

	// Write YAML with different values for everything.
	tmpDir := t.TempDir()
	yamlContent := []byte(`model: yaml-model-should-lose
focus:
  - docs
min_severity: low
extra_rules: "yaml-rule"
`)
	if err := os.WriteFile(filepath.Join(tmpDir, ".code-reviewer.yaml"), yamlContent, 0o644); err != nil {
		t.Fatalf("writing yaml: %v", err)
	}

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer os.Chdir(oldDir) //nolint:errcheck
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	// Flag wins over env and yaml.
	if cfg.Model != "flag-model" {
		t.Errorf("Model = %q, want %q (flag should win)", cfg.Model, "flag-model")
	}
	// Flag wins over env.
	if cfg.MinSeverity != SeverityCritical {
		t.Errorf("MinSeverity = %v, want SeverityCritical (flag should win over env)", cfg.MinSeverity)
	}
	// YAML wins when no env or flag set.
	if len(cfg.Focus) != 1 || cfg.Focus[0] != "docs" {
		t.Errorf("Focus = %v, want [docs] (yaml should apply when no env/flag)", cfg.Focus)
	}
	if cfg.ExtraRules != "yaml-rule" {
		t.Errorf("ExtraRules = %q, want %q (yaml should apply when no env/flag)", cfg.ExtraRules, "yaml-rule")
	}
}

// TestLoad_EnvOverridesYAML verifies that env vars override YAML config when
// no flags are set.
func TestLoad_EnvOverridesYAML(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"code-reviewer", "--diff"}

	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	t.Setenv("REVIEW_MODEL", "env-model")
	t.Setenv("REVIEW_FOCUS", "security,bugs")
	t.Setenv("REVIEW_EXTRA_RULES", "env-rule")
	t.Setenv("REVIEW_PROXY_URL", "")

	tmpDir := t.TempDir()
	yamlContent := []byte(`model: yaml-model-should-lose
focus:
  - docs
extra_rules: "yaml-rule-should-lose"
`)
	if err := os.WriteFile(filepath.Join(tmpDir, ".code-reviewer.yaml"), yamlContent, 0o644); err != nil {
		t.Fatalf("writing yaml: %v", err)
	}

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer os.Chdir(oldDir) //nolint:errcheck
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Model != "env-model" {
		t.Errorf("Model = %q, want %q (env should override yaml)", cfg.Model, "env-model")
	}
	if len(cfg.Focus) != 2 || cfg.Focus[0] != "security" || cfg.Focus[1] != "bugs" {
		t.Errorf("Focus = %v, want [security bugs] (env should override yaml)", cfg.Focus)
	}
	if cfg.ExtraRules != "env-rule" {
		t.Errorf("ExtraRules = %q, want %q (env should override yaml)", cfg.ExtraRules, "env-rule")
	}
}

// TestLoad_FlagAndEnvOptions is a table-driven test consolidating all
// individual flag and env-var option tests.
func TestLoad_FlagAndEnvOptions(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		env    map[string]string
		assert func(t *testing.T, cfg *Config)
	}{
		{
			name: "incremental_flag",
			args: []string{"code-reviewer", "--diff", "--incremental"},
			env:  map[string]string{"GOOGLE_CLOUD_PROJECT": "test-project", "INCREMENTAL": ""},
			assert: func(t *testing.T, cfg *Config) {
				if !cfg.Incremental {
					t.Error("Incremental = false, want true")
				}
			},
		},
		{
			name: "incremental_env",
			args: []string{"code-reviewer", "--diff"},
			env:  map[string]string{"GOOGLE_CLOUD_PROJECT": "test-project", "INCREMENTAL": "true"},
			assert: func(t *testing.T, cfg *Config) {
				if !cfg.Incremental {
					t.Error("Incremental = false, want true")
				}
			},
		},
		{
			name: "sarif_flag",
			args: []string{"code-reviewer", "--diff", "--sarif", "results.sarif"},
			env:  map[string]string{"GOOGLE_CLOUD_PROJECT": "test-project", "SARIF_OUTPUT": ""},
			assert: func(t *testing.T, cfg *Config) {
				if cfg.SARIFOutput != "results.sarif" {
					t.Errorf("SARIFOutput = %q, want %q", cfg.SARIFOutput, "results.sarif")
				}
			},
		},
		{
			name: "sarif_env",
			args: []string{"code-reviewer", "--diff"},
			env:  map[string]string{"GOOGLE_CLOUD_PROJECT": "test-project", "SARIF_OUTPUT": "env.sarif"},
			assert: func(t *testing.T, cfg *Config) {
				if cfg.SARIFOutput != "env.sarif" {
					t.Errorf("SARIFOutput = %q, want %q", cfg.SARIFOutput, "env.sarif")
				}
			},
		},
		{
			name: "models_consensus",
			args: []string{
				"code-reviewer", "--diff",
				"--models", "gemini-2.5-flash,claude-sonnet-4",
				"--consensus-threshold", "2",
			},
			env: map[string]string{"GOOGLE_CLOUD_PROJECT": "test-project", "REVIEW_MODELS": ""},
			assert: func(t *testing.T, cfg *Config) {
				if len(cfg.Models) != 2 || cfg.Models[0] != "gemini-2.5-flash" || cfg.Models[1] != "claude-sonnet-4" {
					t.Errorf("Models = %v, want [gemini-2.5-flash claude-sonnet-4]", cfg.Models)
				}
				if cfg.ConsensusThreshold != 2 {
					t.Errorf("ConsensusThreshold = %d, want 2", cfg.ConsensusThreshold)
				}
			},
		},
		{
			name: "no_color_flag",
			args: []string{"code-reviewer", "--diff", "--no-color"},
			env:  map[string]string{"GOOGLE_CLOUD_PROJECT": "test-project"},
			assert: func(t *testing.T, cfg *Config) {
				if !cfg.NoColor {
					t.Error("NoColor = false, want true")
				}
			},
		},
		{
			name: "no_color_env",
			args: []string{"code-reviewer", "--diff"},
			env:  map[string]string{"GOOGLE_CLOUD_PROJECT": "test-project", "NO_COLOR": "1"},
			assert: func(t *testing.T, cfg *Config) {
				if !cfg.NoColor {
					t.Error("NoColor = false, want true (NO_COLOR env should set it)")
				}
			},
		},
		{
			name: "custom_prompt_flag",
			args: []string{"code-reviewer", "--diff", "--custom-prompt", "my-prompt.md"},
			env:  map[string]string{"GOOGLE_CLOUD_PROJECT": "test-project", "REVIEW_CUSTOM_PROMPT": ""},
			assert: func(t *testing.T, cfg *Config) {
				if cfg.CustomPrompt != "my-prompt.md" {
					t.Errorf("CustomPrompt = %q, want %q", cfg.CustomPrompt, "my-prompt.md")
				}
			},
		},
		{
			name: "diff_ref",
			args: []string{"code-reviewer", "--diff", "origin/develop"},
			env:  map[string]string{"GOOGLE_CLOUD_PROJECT": "test-project"},
			assert: func(t *testing.T, cfg *Config) {
				if cfg.DiffRef != "origin/develop" {
					t.Errorf("DiffRef = %q, want %q", cfg.DiffRef, "origin/develop")
				}
			},
		},
		{
			name: "files_mode",
			args: []string{"code-reviewer", "--files", "main.go,utils.go"},
			env:  map[string]string{"GOOGLE_CLOUD_PROJECT": "test-project"},
			assert: func(t *testing.T, cfg *Config) {
				if cfg.Mode() != "files" {
					t.Errorf("Mode() = %q, want %q", cfg.Mode(), "files")
				}
				if len(cfg.Files) != 2 || cfg.Files[0] != "main.go" || cfg.Files[1] != "utils.go" {
					t.Errorf("Files = %v, want [main.go utils.go]", cfg.Files)
				}
			},
		},
		{
			name: "cleanup_mode_flag",
			args: []string{"code-reviewer", "--diff", "--cleanup-mode", "resolve"},
			env:  map[string]string{"GOOGLE_CLOUD_PROJECT": "test-project"},
			assert: func(t *testing.T, cfg *Config) {
				if cfg.CleanupMode != CleanupModeResolve {
					t.Errorf("CleanupMode = %q, want %q", cfg.CleanupMode, CleanupModeResolve)
				}
			},
		},
		{
			name: "cleanup_mode_env",
			args: []string{"code-reviewer", "--diff"},
			env:  map[string]string{"GOOGLE_CLOUD_PROJECT": "test-project", "CODE_REVIEWER_CLEANUP_MODE": "resolve"},
			assert: func(t *testing.T, cfg *Config) {
				if cfg.CleanupMode != CleanupModeResolve {
					t.Errorf("CleanupMode = %q, want %q", cfg.CleanupMode, CleanupModeResolve)
				}
			},
		},
		{
			name: "cleanup_mode_flag_over_env",
			args: []string{"code-reviewer", "--diff", "--cleanup-mode", "delete"},
			env:  map[string]string{"GOOGLE_CLOUD_PROJECT": "test-project", "CODE_REVIEWER_CLEANUP_MODE": "resolve"},
			assert: func(t *testing.T, cfg *Config) {
				if cfg.CleanupMode != CleanupModeDelete {
					t.Errorf("CleanupMode = %q, want %q (flag should override env)", cfg.CleanupMode, CleanupModeDelete)
				}
			},
		},
		{
			name: "update_description_flag",
			args: []string{"code-reviewer", "--diff", "--update-description"},
			env:  map[string]string{"GOOGLE_CLOUD_PROJECT": "test-project"},
			assert: func(t *testing.T, cfg *Config) {
				if !cfg.UpdateDescription {
					t.Errorf("UpdateDescription = false, want true")
				}
			},
		},
		{
			name: "update_description_env",
			args: []string{"code-reviewer", "--diff"},
			env:  map[string]string{"GOOGLE_CLOUD_PROJECT": "test-project", "CODE_REVIEWER_UPDATE_DESCRIPTION": "true"},
			assert: func(t *testing.T, cfg *Config) {
				if !cfg.UpdateDescription {
					t.Errorf("UpdateDescription = false, want true")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldArgs := os.Args
			defer func() { os.Args = oldArgs }()
			os.Args = tt.args

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
			tt.assert(t, cfg)
		})
	}
}

// contains is a helper to check if s contains substr.
func contains(s, substr string) bool {
	return len(substr) > 0 && len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestLoad_APIURLBypassesGCPProject(t *testing.T) {
	// When api_url is set, GOOGLE_CLOUD_PROJECT should NOT be required.
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"code-reviewer", "--diff", "--api-url", "https://candela.example.com/v1"}

	// Clear GOOGLE_CLOUD_PROJECT to prove it's not needed.
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("REVIEW_API_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v — expected no error when api_url bypasses GCP project", err)
	}
	if cfg.APIURL != "https://candela.example.com/v1" {
		t.Errorf("APIURL = %q, want https://candela.example.com/v1", cfg.APIURL)
	}
	if cfg.GCPProject != "" {
		t.Errorf("GCPProject = %q, want empty", cfg.GCPProject)
	}
}

func TestLoad_NoAPIURLRequiresGCPProject(t *testing.T) {
	// Without api_url, GOOGLE_CLOUD_PROJECT should still be required.
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"code-reviewer", "--diff"}

	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("REVIEW_API_URL", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when no GCP project and no api_url")
	}
	errStr := err.Error()
	if !containsStr(errStr, "GOOGLE_CLOUD_PROJECT") {
		t.Errorf("error = %q, want mention of GOOGLE_CLOUD_PROJECT", errStr)
	}
	if !containsStr(errStr, "api-url") {
		t.Errorf("error = %q, want mention of api-url alternative", errStr)
	}
}

// TestIntentReview_DefaultOnInCI verifies intent is auto-enabled in CI mode.
func TestIntentReview_DefaultOnInCI(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"code-reviewer", "--ci"}

	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	t.Setenv("CI_PROJECT_ID", "123")
	t.Setenv("CI_MERGE_REQUEST_IID", "456")
	t.Setenv("GITLAB_TOKEN", "test-token")
	t.Setenv("REVIEW_INTENT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if !cfg.IntentReview {
		t.Error("IntentReview should be true in CI mode by default")
	}
}

// TestIntentReview_YAMLFalseOverridesCIDefault verifies YAML intent_review: false
// prevents CI auto-enable.
func TestIntentReview_YAMLFalseOverridesCIDefault(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"code-reviewer", "--ci"}

	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	t.Setenv("CI_PROJECT_ID", "123")
	t.Setenv("CI_MERGE_REQUEST_IID", "456")
	t.Setenv("GITLAB_TOKEN", "test-token")
	t.Setenv("REVIEW_INTENT", "")

	tmpDir := t.TempDir()
	yamlContent := []byte("intent_review: false\n")
	if err := os.WriteFile(filepath.Join(tmpDir, ".code-reviewer.yaml"), yamlContent, 0o644); err != nil {
		t.Fatalf("writing yaml: %v", err)
	}

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer os.Chdir(oldDir) //nolint:errcheck
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.IntentReview {
		t.Error("IntentReview should be false when YAML sets intent_review: false, even in CI")
	}
}

// TestIntentReview_EnvFalseOverridesCIDefault verifies REVIEW_INTENT=false
// prevents CI auto-enable.
func TestIntentReview_EnvFalseOverridesCIDefault(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"code-reviewer", "--ci"}

	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	t.Setenv("CI_PROJECT_ID", "123")
	t.Setenv("CI_MERGE_REQUEST_IID", "456")
	t.Setenv("GITLAB_TOKEN", "test-token")
	t.Setenv("REVIEW_INTENT", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.IntentReview {
		t.Error("IntentReview should be false when REVIEW_INTENT=false, even in CI")
	}
}

// TestIntentReview_DefaultOffLocal verifies intent is off in local/diff mode.
func TestIntentReview_DefaultOffLocal(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"code-reviewer", "--diff"}

	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	t.Setenv("REVIEW_INTENT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.IntentReview {
		t.Error("IntentReview should be false in diff mode by default")
	}
}

// TestIntentReview_MutuallyExclusiveWithSummarize verifies that --intent and
// --summarize cannot be used together.
func TestIntentReview_MutuallyExclusiveWithSummarize(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"code-reviewer", "--diff", "--intent", "--summarize"}

	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	t.Setenv("REVIEW_INTENT", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for --intent + --summarize")
	}
	if !containsStr(err.Error(), "mutually exclusive") {
		t.Errorf("error = %q, want mention of 'mutually exclusive'", err.Error())
	}
}

func TestLoad_InvalidCleanupMode(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"code-reviewer", "--diff", "--cleanup-mode", "archive"}

	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid cleanup-mode")
	}
	if !containsStr(err.Error(), "invalid cleanup-mode") {
		t.Errorf("error = %q, want mention of 'invalid cleanup-mode'", err.Error())
	}
}
