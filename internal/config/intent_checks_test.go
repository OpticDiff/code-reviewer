package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyRepoConfig_IntentChecks(t *testing.T) {
	c := &Config{}
	err := c.applyRepoConfig([]byte(`
intent_checks:
  stack_aware: true
  missing_tests: { severity: medium }
  breaking_changes: { severity: low, documented_in: ["CHANGELOG.md", "MR_DESCRIPTION"] }
  scope_creep: { severity: off }
`))
	if err != nil {
		t.Fatalf("applyRepoConfig: %v", err)
	}
	ic := c.IntentChecks
	if ic.MissingTests.Severity != "medium" || ic.ScopeCreep.Severity != "off" || !ic.StackAware {
		t.Errorf("unexpected intent checks: %+v", ic)
	}
	if got := ic.BreakingChanges.DocumentedIn; len(got) != 2 || got[1] != "MR_DESCRIPTION" {
		t.Errorf("documented_in = %v", got)
	}
}

func TestApplyRepoConfig_IntentChecksInvalidSeverity(t *testing.T) {
	c := &Config{}
	if err := c.applyRepoConfig([]byte("intent_checks:\n  missing_tests: { severity: urgent }\n")); err == nil {
		t.Fatal("expected error for invalid severity")
	}
}

func TestLoadGitLabCIEnv_Stacked(t *testing.T) {
	tests := []struct {
		name, target, def string
		want              bool
	}{
		{"default branch target", "main", "main", false},
		{"stacked", "feature/base", "main", true},
		{"unknown default", "feature/base", "", false},
		{"not a merge request", "", "main", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CI_MERGE_REQUEST_TARGET_BRANCH_NAME", tt.target)
			t.Setenv("CI_DEFAULT_BRANCH", tt.def)
			c := &Config{}
			c.loadGitLabCIEnv()
			if c.IntentChecks.Stacked != tt.want {
				t.Errorf("Stacked = %v, want %v", c.IntentChecks.Stacked, tt.want)
			}
		})
	}
}

func TestLoadGitHubCIEnv_Stacked(t *testing.T) {
	event := filepath.Join(t.TempDir(), "event.json")
	if err := os.WriteFile(event, []byte(`{"repository":{"default_branch":"main"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_EVENT_PATH", event)
	t.Setenv("CI_COMMIT_MESSAGE", "")

	for base, want := range map[string]bool{"main": false, "feature/base": true, "": false} {
		t.Setenv("GITHUB_BASE_REF", base)
		c := &Config{}
		c.loadGitHubCIEnv()
		if c.IntentChecks.Stacked != want {
			t.Errorf("base %q: Stacked = %v, want %v", base, c.IntentChecks.Stacked, want)
		}
	}
}
