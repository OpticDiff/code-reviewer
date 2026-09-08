package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadPlatformConfig_MultiFileGlobs tests expanding multi-file glob patterns for platform configs and guidelines.
func TestLoadPlatformConfig_MultiFileGlobs(t *testing.T) {
	tmpDir := t.TempDir()
	rulesDir := filepath.Join(tmpDir, "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatal(err)
	}

	hipaaContent := `
min_severity: medium
rules:
  - name: mask-phi
    description: "Ensure all patient PHI variables are masked before logging"
    category: security
    severity: critical
    url: "https://docs.azra.internal/compliance/hipaa"
`
	tenancyContent := `
rules:
  - name: require-tenant-context
    description: "Direct SQL queries must filter by tenant_id"
    category: security
    severity: high
    paths:
      - "internal/db/**/*.go"
`
	if err := os.WriteFile(filepath.Join(rulesDir, "01-hipaa.yaml"), []byte(hipaaContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "02-tenancy.yaml"), []byte(tenancyContent), 0644); err != nil {
		t.Fatal(err)
	}

	guidelinesDir := filepath.Join(tmpDir, "guidelines")
	if err := os.MkdirAll(guidelinesDir, 0755); err != nil {
		t.Fatal(err)
	}
	mdContent := "# Platform Mandates\n\nAll services must isolate tenant databases."
	if err := os.WriteFile(filepath.Join(guidelinesDir, "security.md"), []byte(mdContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		MinSeverity:      SeverityLow,
		PlatformConfig:   filepath.Join(rulesDir, "*.yaml"),
		PlatformReviewMD: filepath.Join(guidelinesDir, "*.md"),
	}

	if err := cfg.loadPlatformConfig(); err != nil {
		t.Fatalf("loadPlatformConfig failed: %v", err)
	}

	if len(cfg.PlatformRules) != 2 {
		t.Fatalf("expected 2 platform rules, got %d", len(cfg.PlatformRules))
	}

	r0 := cfg.PlatformRules[0]
	if r0.Name != "mask-phi" || r0.Source != "platform" || r0.SourceFile != "01-hipaa.yaml" {
		t.Errorf("unexpected rule 0: %+v", r0)
	}
	if r0.URL != "https://docs.azra.internal/compliance/hipaa" {
		t.Errorf("expected URL to be populated, got %s", r0.URL)
	}

	r1 := cfg.PlatformRules[1]
	if r1.Name != "require-tenant-context" || r1.Source != "platform" || r1.SourceFile != "02-tenancy.yaml" {
		t.Errorf("unexpected rule 1: %+v", r1)
	}

	// Verify SHA-256 hashes recorded
	if len(cfg.PlatformRuleFileHashes) != 3 { // 2 yaml + 1 md
		t.Errorf("expected 3 file hashes, got %d: %+v", len(cfg.PlatformRuleFileHashes), cfg.PlatformRuleFileHashes)
	}
	if cfg.PlatformRuleFileHashes["01-hipaa.yaml"] == "" {
		t.Error("missing hash for 01-hipaa.yaml")
	}

	// Verify markdown content merged
	if cfg.PlatformReviewMDContent == "" {
		t.Error("expected non-empty PlatformReviewMDContent")
	}
}

// TestLoadPlatformConfig_CommaSeparated tests loading comma-separated platform config file paths.
func TestLoadPlatformConfig_CommaSeparated(t *testing.T) {
	tmpDir := t.TempDir()
	f1 := filepath.Join(tmpDir, "rule1.yaml")
	f2 := filepath.Join(tmpDir, "rule2.yaml")

	content1 := `
rules:
  - name: rule-one
    description: "Rule one description"
`
	content2 := `
rules:
  - name: rule-two
    description: "Rule two description"
`
	if err := os.WriteFile(f1, []byte(content1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte(content2), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		PlatformConfig: f1 + "," + f2,
	}

	if err := cfg.loadPlatformConfig(); err != nil {
		t.Fatalf("loadPlatformConfig failed: %v", err)
	}

	if len(cfg.PlatformRules) != 2 {
		t.Fatalf("expected 2 platform rules, got %d", len(cfg.PlatformRules))
	}
}

// TestLoadPlatformConfig_MonotonicSeverityFloor tests clamping repo min_severity to the platform floor.
func TestLoadPlatformConfig_MonotonicSeverityFloor(t *testing.T) {
	tmpDir := t.TempDir()
	pYAML := filepath.Join(tmpDir, "platform.yaml")
	content := `
min_severity: medium
rules:
  - name: rule-high
    description: "Mandatory check"
`
	if err := os.WriteFile(pYAML, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Repo requested SeverityHigh (looser than medium)
	cfg := &Config{
		MinSeverity:    SeverityHigh,
		PlatformConfig: pYAML,
	}

	if err := cfg.loadPlatformConfig(); err != nil {
		t.Fatalf("loadPlatformConfig failed: %v", err)
	}

	// Monotonic floor: MinSeverity must be clamped down to SeverityMedium
	if cfg.MinSeverity != SeverityMedium {
		t.Errorf("expected MinSeverity to be clamped to SeverityMedium (%v), got %v", SeverityMedium, cfg.MinSeverity)
	}
}

// TestLoadPlatformConfig_RepoRuleConflict tests that platform rules take precedence over repo rules with duplicate names.
func TestLoadPlatformConfig_RepoRuleConflict(t *testing.T) {
	tmpDir := t.TempDir()
	pYAML := filepath.Join(tmpDir, "platform.yaml")
	content := `
rules:
  - name: conflict-rule
    description: "Platform definition"
    severity: critical
`
	if err := os.WriteFile(pYAML, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		PlatformConfig: pYAML,
		Rules: []Rule{
			{
				Name:        "conflict-rule",
				Description: "Repo attempting to override platform rule",
				Severity:    "low",
			},
			{
				Name:        "team-custom-rule",
				Description: "Legitimate team rule",
				Severity:    "low",
			},
		},
	}

	if err := cfg.loadPlatformConfig(); err != nil {
		t.Fatalf("loadPlatformConfig failed: %v", err)
	}

	if len(cfg.Rules) != 2 {
		t.Fatalf("expected 2 rules total (1 platform, 1 repo), got %d", len(cfg.Rules))
	}

	// First rule must be the platform rule
	if cfg.Rules[0].Name != "conflict-rule" || cfg.Rules[0].Source != "platform" || cfg.Rules[0].Severity != "critical" {
		t.Errorf("expected platform rule to win, got: %+v", cfg.Rules[0])
	}
	if cfg.Rules[1].Name != "team-custom-rule" || cfg.Rules[1].Source != "repo" {
		t.Errorf("expected team rule to remain, got: %+v", cfg.Rules[1])
	}
}

// TestLoadPlatformConfig_EmptyGlobReturnsError tests that a glob pattern matching zero files returns an error.
func TestLoadPlatformConfig_EmptyGlobReturnsError(t *testing.T) {
	cfg := &Config{
		PlatformConfig: "/nonexistent-path-abc123xyz/*.yaml",
	}
	err := cfg.loadPlatformConfig()
	if err == nil {
		t.Fatal("expected error for glob matching no files, got nil")
	}
}
