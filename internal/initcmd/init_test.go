package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRun_Defaults(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	err := Run(Options{Yes: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(configFileName)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	content := string(data)

	// Verify it's valid YAML.
	var parsed map[string]interface{}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("generated YAML is invalid: %v", err)
	}

	// Verify key fields are present.
	if !strings.Contains(content, "model: gemini-2.5-flash") {
		t.Error("expected default model in output")
	}
	if !strings.Contains(content, "min_severity: low") {
		t.Error("expected default min_severity in output")
	}
	if !strings.Contains(content, "chunk_strategy: split") {
		t.Error("expected default chunk_strategy in output")
	}
	if !strings.Contains(content, "- all") {
		t.Error("expected default focus 'all' in output")
	}
}

func TestRun_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	// Create existing file.
	os.WriteFile(configFileName, []byte("existing"), 0644)

	err := Run(Options{Yes: true})
	if err == nil {
		t.Fatal("expected error when file exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' error, got: %v", err)
	}
}

func TestRun_Force(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	// Create existing file.
	os.WriteFile(configFileName, []byte("old"), 0644)

	err := Run(Options{Yes: true, Force: true})
	if err != nil {
		t.Fatalf("unexpected error with --force: %v", err)
	}

	data, err := os.ReadFile(configFileName)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	if strings.Contains(string(data), "old") {
		t.Error("expected file to be overwritten")
	}
}

func TestWriteConfig_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	cfg := Config{
		Model:            "gemini-2.5-pro",
		Focus:            []string{"security", "bugs"},
		MinSeverity:      "medium",
		ChunkStrategy:    "split",
		MaxFiles:         30,
		ExcludedPatterns: []string{"go.sum", "vendor/*"},
		ExtraRules:       "Always check error handling.",
		CommentMode:      "discussions",
		Platform:         "gitlab",
	}

	if err := writeConfig(cfg); err != nil {
		t.Fatalf("writeConfig failed: %v", err)
	}

	data, err := os.ReadFile(configFileName)
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}

	// Must be valid YAML.
	var parsed map[string]interface{}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid YAML: %v\n---\n%s", err, string(data))
	}

	content := string(data)
	if !strings.Contains(content, "model: gemini-2.5-pro") {
		t.Error("expected model gemini-2.5-pro")
	}
	if !strings.Contains(content, "- security") {
		t.Error("expected focus security")
	}
	if !strings.Contains(content, "max_files: 30") {
		t.Error("expected max_files 30")
	}
	if !strings.Contains(content, "comment_mode: discussions") {
		t.Error("expected comment_mode discussions")
	}
	if !strings.Contains(content, "Always check error handling.") {
		t.Error("expected extra_rules content")
	}
}

func TestWriteConfig_NoMaxFiles(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	cfg := DefaultConfig()
	if err := writeConfig(cfg); err != nil {
		t.Fatalf("writeConfig failed: %v", err)
	}

	data, _ := os.ReadFile(configFileName)
	if strings.Contains(string(data), "max_files") {
		t.Error("max_files should be omitted when 0")
	}
}

func TestDetectPlatform_GitHub(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	os.MkdirAll(filepath.Join(".github", "workflows"), 0755)
	if p := detectPlatform(); p != "github" {
		t.Errorf("expected github, got %q", p)
	}
}

func TestDetectPlatform_GitLab(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	os.WriteFile(".gitlab-ci.yml", []byte("stages:"), 0644)
	if p := detectPlatform(); p != "gitlab" {
		t.Errorf("expected gitlab, got %q", p)
	}
}

func TestDetectPlatform_None(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	if p := detectPlatform(); p != "" {
		t.Errorf("expected empty, got %q", p)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Model != "gemini-2.5-flash" {
		t.Errorf("expected gemini-2.5-flash, got %s", cfg.Model)
	}
	if len(cfg.Focus) != 1 || cfg.Focus[0] != "all" {
		t.Errorf("expected [all], got %v", cfg.Focus)
	}
	if cfg.ChunkStrategy != "split" {
		t.Errorf("expected split, got %s", cfg.ChunkStrategy)
	}
}
