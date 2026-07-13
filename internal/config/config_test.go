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
			env:     map[string]string{"GOOGLE_CLOUD_PROJECT": "p", "CI_PROJECT_ID": "", "CI_MERGE_REQUEST_IID": "", "GITLAB_TOKEN": "tok"},
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

	tmpDir := t.TempDir()
	yamlContent := []byte(`model: yaml-model
focus:
  - security
  - bugs
min_severity: medium
extra_rules: "always check for nil"
output_json: true
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
