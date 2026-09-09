//go:build eval
// +build eval

// Package eval provides persona evaluation tests for the dual-review
// profile system. These tests require a real LLM backend (set
// GOOGLE_CLOUD_PROJECT) and are gated behind the "eval" build tag.
//
// Run: nix develop -c go test -tags=eval ./eval/ -v -timeout=600s
package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// finding mirrors the JSON output from code-reviewer --json.
type finding struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	Severity   string `json:"severity"`
	Category   string `json:"category"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	RuleName   string `json:"rule_name"`
	RuleSource string `json:"rule_source"`
}

type reviewOutput struct {
	Summary  string    `json:"summary"`
	Findings []finding `json:"findings"`
}

// diffCase describes a test diff and what we expect from each profile.
type diffCase struct {
	Name string
	File string // relative path under testdata/diffs/

	// Platform expectations.
	PlatformShouldFind    bool     // True if platform profile should emit findings.
	PlatformCategories    []string // Expected categories if found (e.g. "security", "bug").
	PlatformMinFindings   int      // Minimum expected findings.

	// Product expectations.
	ProductShouldFind     bool // True if product profile should emit findings.
	ProductMinFindings    int
}

var cases = []diffCase{
	// === Security/Platform diffs ===
	{
		Name: "sql-injection",
		File: "sql-injection.diff",
		PlatformShouldFind:  true,
		PlatformCategories:  []string{"security", "bug"},
		PlatformMinFindings: 1,
		ProductShouldFind:   false, // Pure security issue
	},
	{
		Name: "hardcoded-secret",
		File: "hardcoded-secret.diff",
		PlatformShouldFind:  true,
		PlatformCategories:  []string{"security"},
		PlatformMinFindings: 1,
		ProductShouldFind:   false,
	},
	{
		Name: "missing-auth-check",
		File: "missing-auth-check.diff",
		PlatformShouldFind:  true,
		PlatformCategories:  []string{"security"},
		PlatformMinFindings: 1,
		ProductShouldFind:   false,
	},
	{
		Name: "insecure-tls",
		File: "insecure-tls.diff",
		PlatformShouldFind:  true,
		PlatformCategories:  []string{"security"},
		PlatformMinFindings: 1,
		ProductShouldFind:   false,
	},
	{
		Name: "path-traversal",
		File: "path-traversal.diff",
		PlatformShouldFind:  true,
		PlatformCategories:  []string{"security"},
		PlatformMinFindings: 1,
		ProductShouldFind:   false,
	},
	{
		Name: "xss-vulnerability",
		File: "xss-vulnerability.diff",
		PlatformShouldFind:  true,
		PlatformCategories:  []string{"security"},
		PlatformMinFindings: 1,
		ProductShouldFind:   false,
	},
	{
		Name: "race-condition",
		File: "race-condition.diff",
		PlatformShouldFind:  true,
		PlatformCategories:  []string{"security", "bug"},
		PlatformMinFindings: 1,
		ProductShouldFind:   false,
	},

	// === Code Quality/Product diffs ===
	{
		Name: "error-not-checked",
		File: "error-not-checked.diff",
		PlatformShouldFind: false, // Not a security issue
		ProductShouldFind:  true,
		ProductMinFindings: 1,
	},
	{
		Name: "naming-convention",
		File: "naming-convention.diff",
		PlatformShouldFind: false,
		ProductShouldFind:  true,
		ProductMinFindings: 1,
	},
	{
		Name: "dead-code",
		File: "dead-code.diff",
		PlatformShouldFind: false,
		ProductShouldFind:  true,
		ProductMinFindings: 1,
	},
	{
		Name: "magic-numbers",
		File: "magic-numbers.diff",
		PlatformShouldFind: false,
		ProductShouldFind:  true,
		ProductMinFindings: 1,
	},
	{
		Name: "missing-test",
		File: "missing-test.diff",
		PlatformShouldFind: false,
		ProductShouldFind:  true,
		ProductMinFindings: 1,
	},
	{
		Name: "long-function",
		File: "long-function.diff",
		PlatformShouldFind: false,
		ProductShouldFind:  true,
		ProductMinFindings: 1,
	},
	{
		Name: "missing-docs",
		File: "missing-docs.diff",
		PlatformShouldFind: false,
		ProductShouldFind:  true,
		ProductMinFindings: 1,
	},

	// === Mixed ===
	{
		Name: "mixed-security-style",
		File: "mixed-security-style.diff",
		PlatformShouldFind:  true,
		PlatformCategories:  []string{"security", "bug"},
		PlatformMinFindings: 1,
		ProductShouldFind:   true,
		ProductMinFindings:  1,
	},
}

func TestPersonaIsolation(t *testing.T) {
	if os.Getenv("GOOGLE_CLOUD_PROJECT") == "" {
		t.Skip("GOOGLE_CLOUD_PROJECT not set; skipping eval tests (requires LLM backend)")
	}

	binary := findBinary(t)
	evalDir := findEvalDir(t)
	diffsDir := filepath.Join(evalDir, "testdata", "diffs")
	rulesFile := filepath.Join(evalDir, "platform-rules.yaml")
	guidelinesFile := filepath.Join(evalDir, "platform-guidelines.md")

	for _, tc := range cases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			diffFile := filepath.Join(diffsDir, tc.File)
			if _, err := os.Stat(diffFile); os.IsNotExist(err) {
				t.Skipf("diff file %s not found", tc.File)
			}

			// Run platform profile.
			t.Run("platform", func(t *testing.T) {
				findings := runReview(t, binary, diffFile, "platform", rulesFile, guidelinesFile)
				if tc.PlatformShouldFind {
					if len(findings) < tc.PlatformMinFindings {
						t.Errorf("platform: expected >= %d findings, got %d", tc.PlatformMinFindings, len(findings))
					}
					if len(tc.PlatformCategories) > 0 {
						assertCategoriesMatch(t, findings, tc.PlatformCategories)
					}
				} else {
					// Platform should NOT find product-only issues.
					for _, f := range findings {
						t.Errorf("platform: unexpected finding on product-only diff: [%s] %s — %s",
							f.Category, f.Severity, f.Title)
					}
				}
			})

			// Run product profile.
			t.Run("product", func(t *testing.T) {
				findings := runReview(t, binary, diffFile, "product", rulesFile, guidelinesFile)
				if tc.ProductShouldFind {
					if len(findings) < tc.ProductMinFindings {
						t.Errorf("product: expected >= %d findings, got %d", tc.ProductMinFindings, len(findings))
					}
					// Product findings must not reference platform rules.
					for _, f := range findings {
						if f.RuleSource == "platform" {
							t.Errorf("product: finding has platform rule source: %s", f.RuleName)
						}
					}
				} else {
					for _, f := range findings {
						t.Errorf("product: unexpected finding on security-only diff: [%s] %s — %s",
							f.Category, f.Severity, f.Title)
					}
				}
			})
		})
	}
}

func TestProfileCountInvariant(t *testing.T) {
	if os.Getenv("GOOGLE_CLOUD_PROJECT") == "" {
		t.Skip("GOOGLE_CLOUD_PROJECT not set")
	}

	binary := findBinary(t)
	evalDir := findEvalDir(t)
	diffsDir := filepath.Join(evalDir, "testdata", "diffs")
	rulesFile := filepath.Join(evalDir, "platform-rules.yaml")
	guidelinesFile := filepath.Join(evalDir, "platform-guidelines.md")

	// Pick the mixed diff — it should produce findings in all profiles.
	diffFile := filepath.Join(diffsDir, "mixed-security-style.diff")
	if _, err := os.Stat(diffFile); os.IsNotExist(err) {
		t.Skip("mixed diff not found")
	}

	all := runReview(t, binary, diffFile, "all", rulesFile, guidelinesFile)
	platform := runReview(t, binary, diffFile, "platform", rulesFile, guidelinesFile)
	product := runReview(t, binary, diffFile, "product", rulesFile, guidelinesFile)

	t.Logf("all=%d, platform=%d, product=%d", len(all), len(platform), len(product))

	// The combined profile-filtered findings should be <= all findings,
	// because filtering can only reduce, never add.
	if len(platform)+len(product) > len(all) {
		t.Errorf("profile isolation violation: platform(%d) + product(%d) > all(%d)",
			len(platform), len(product), len(all))
	}
}

// runReview executes code-reviewer with the given profile and returns findings.
func runReview(t *testing.T, binary, diffFile, profile, rulesFile, guidelinesFile string) []finding {
	t.Helper()

	args := []string{
		"--diff",
		"--json",
		"--profile", profile,
		"--platform-config", rulesFile,
		"--platform-review-md", guidelinesFile,
		"--no-context",
	}

	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("CODE_REVIEW_DIFF_FILE=%s", diffFile),
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		// Exit code 1 means findings were found — that's expected.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// Expected: findings found.
		} else {
			t.Logf("command output: %s", string(out))
			t.Fatalf("code-reviewer failed (exit %v): %v", err, string(out))
		}
	}

	var result reviewOutput
	if err := json.Unmarshal(out, &result); err != nil {
		// Try to find JSON in mixed output (slog lines + JSON).
		jsonStart := strings.Index(string(out), "{")
		if jsonStart >= 0 {
			if err := json.Unmarshal(out[jsonStart:], &result); err != nil {
				t.Logf("raw output: %s", string(out))
				t.Fatalf("failed to parse JSON output: %v", err)
			}
		} else {
			t.Logf("raw output: %s", string(out))
			t.Fatalf("no JSON in output: %v", err)
		}
	}

	return result.Findings
}

func assertCategoriesMatch(t *testing.T, findings []finding, expected []string) {
	t.Helper()
	allowed := make(map[string]bool)
	for _, c := range expected {
		allowed[strings.ToLower(c)] = true
	}
	for _, f := range findings {
		cat := strings.ToLower(f.Category)
		if cat != "" && !allowed[cat] {
			t.Errorf("finding category %q not in expected %v (title: %s)", f.Category, expected, f.Title)
		}
	}
}

func findBinary(t *testing.T) string {
	t.Helper()
	// Look for compiled binary or use `go run`.
	binary, err := exec.LookPath("code-reviewer")
	if err == nil {
		return binary
	}
	// Fall back to building.
	t.Log("code-reviewer not in PATH, building...")
	tmpBin := filepath.Join(t.TempDir(), "code-reviewer")
	cmd := exec.Command("go", "build", "-o", tmpBin, "./cmd/code-reviewer")
	cmd.Dir = findRepoRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build code-reviewer: %v\n%s", err, out)
	}
	return tmpBin
}

func findEvalDir(t *testing.T) string {
	t.Helper()
	root := findRepoRoot(t)
	return filepath.Join(root, "eval")
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	// Walk up from the test file's directory to find the repo root.
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (no go.mod found)")
		}
		dir = parent
	}
}
