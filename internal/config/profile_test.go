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
