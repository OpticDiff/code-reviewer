package config

import (
	"os"
	"testing"
)

func TestConfig_ProfileValidation(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		wantErr bool
	}{
		{"platform", "platform", false},
		{"product", "product", false},
		{"all", "all", false},
		{"empty", "", false},
		{"invalid", "unknown", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{
				Profile:                tt.profile,
				DiffMode:               true, // Need an input mode
				PlatformAutoDiscovered: true, // Need platform governance config for platform profile
				APIURL:                 "http://localhost:8080",
				CommentMode:            "notes",
				CleanupMode:            "delete",
				ChunkStrategy:          ChunkStrategyFail,
			}
			if tt.profile == "platform" {
				c.PlatformRules = []Rule{{Name: "test-rule"}}
			}

			err := c.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPreDetectProfile(t *testing.T) {
	// Save original args
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	tests := []struct {
		name       string
		args       []string
		envName    string
		envValue   string
		wantResult string
	}{
		{
			name:       "CLI flag --profile platform",
			args:       []string{"cmd", "--profile", "platform"},
			wantResult: "platform",
		},
		{
			name:       "CLI flag --profile=product",
			args:       []string{"cmd", "--profile=product"},
			wantResult: "product",
		},
		{
			name:       "CODE_REVIEWER_PROFILE env var",
			args:       []string{"cmd"},
			envName:    "CODE_REVIEWER_PROFILE",
			envValue:   "platform",
			wantResult: "platform",
		},
		{
			name:       "CODE_REVIEW_PROFILE env var",
			args:       []string{"cmd"},
			envName:    "CODE_REVIEW_PROFILE",
			envValue:   "product",
			wantResult: "product",
		},
		{
			name:       "CLI flag overrides env var",
			args:       []string{"cmd", "--profile", "platform"},
			envName:    "CODE_REVIEWER_PROFILE",
			envValue:   "product",
			wantResult: "platform",
		},
		{
			name:       "No flag or env",
			args:       []string{"cmd"},
			wantResult: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Args = tt.args

			// Unset all relevant env vars first
			t.Setenv("CODE_REVIEWER_PROFILE", "")
			t.Setenv("CODE_REVIEW_PROFILE", "")

			if tt.envName != "" {
				t.Setenv(tt.envName, tt.envValue)
			}

			got := preDetectProfile()
			if got != tt.wantResult {
				t.Errorf("preDetectProfile() = %v, want %v", got, tt.wantResult)
			}
		})
	}
}

func TestConfig_ProfileNormalization(t *testing.T) {
	c := &Config{
		Profile:       "",
		DiffMode:      true,
		APIURL:        "http://localhost:8080",
		CommentMode:   "notes",
		CleanupMode:   "delete",
		ChunkStrategy: ChunkStrategyFail,
	}
	err := c.validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Profile != "all" {
		t.Errorf("expected profile to normalize to 'all', got %q", c.Profile)
	}
}

func TestEffectiveModel(t *testing.T) {
	tests := []struct {
		name          string
		profile       string
		model         string
		platformModel string
		productModel  string
		want          string
	}{
		{"all profile uses base model", "all", "gemini-2.5-flash", "gemini-2.5-pro", "qwen3:8b", "gemini-2.5-flash"},
		{"platform uses platform override", "platform", "gemini-2.5-flash", "gemini-2.5-pro", "", "gemini-2.5-pro"},
		{"product uses product override", "product", "gemini-2.5-flash", "", "qwen3:8b", "qwen3:8b"},
		{"platform falls back to base", "platform", "gemini-2.5-flash", "", "", "gemini-2.5-flash"},
		{"product falls back to base", "product", "gemini-2.5-flash", "", "", "gemini-2.5-flash"},
		{"empty profile uses base", "", "gemini-2.5-flash", "gemini-2.5-pro", "qwen3:8b", "gemini-2.5-flash"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{
				Profile:       tt.profile,
				Model:         tt.model,
				PlatformModel: tt.platformModel,
				ProductModel:  tt.productModel,
			}
			got := c.EffectiveModel()
			if got != tt.want {
				t.Errorf("EffectiveModel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConfig_PlatformVisibilityValidation(t *testing.T) {
	tests := []struct {
		name       string
		visibility string
		wantErr    bool
		wantValue  string
	}{
		{"public", "public", false, "public"},
		{"security-team-only", "security-team-only", false, "security-team-only"},
		{"empty defaults to public", "", false, "public"},
		{"invalid", "private", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{
				PlatformVisibility: tt.visibility,
				DiffMode:           true,
				APIURL:             "http://localhost:8080",
				CommentMode:        "notes",
				CleanupMode:        "delete",
				ChunkStrategy:      ChunkStrategyFail,
			}
			err := c.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && c.PlatformVisibility != tt.wantValue {
				t.Errorf("PlatformVisibility = %q, want %q", c.PlatformVisibility, tt.wantValue)
			}
		})
	}
}

func TestConfig_EffectiveModelAppliedInValidate(t *testing.T) {
	c := &Config{
		Profile:       "platform",
		Model:         "gemini-2.5-flash",
		PlatformModel: "gemini-2.5-pro",
		DiffMode:      true,
		APIURL:        "http://localhost:8080",
		CommentMode:   "notes",
		CleanupMode:   "delete",
		ChunkStrategy: ChunkStrategyFail,
		PlatformAutoDiscovered: true,
		PlatformRules:          []Rule{{Name: "test-rule"}},
	}
	err := c.validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Model != "gemini-2.5-pro" {
		t.Errorf("after validate(), Model = %q, want %q", c.Model, "gemini-2.5-pro")
	}
}
