package hook

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initGitRepo creates a temporary git repo and returns its path.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	return dir
}

// chdirRepo changes into dir and restores the original working directory on cleanup.
func chdirRepo(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(orig); err != nil {
			t.Errorf("restoring working directory: %v", err)
		}
	})
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("os.Chdir(%s): %v", dir, err)
	}
}

// writeForeignHook creates a non-code-reviewer hook at .git/hooks/pre-push.
func writeForeignHook(t *testing.T, dir string) {
	t.Helper()
	hookDir := filepath.Join(dir, ".git", "hooks")
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hookDir, "pre-push"), []byte("#!/bin/sh\necho 'foreign'"), 0o755); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
}

// TestInstallAndUninstall verifies the full lifecycle: install, re-install (idempotent), uninstall.
func TestInstallAndUninstall(t *testing.T) {
	dir := initGitRepo(t)
	chdirRepo(t, dir)

	// Install.
	if err := Install(); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	hookPath := filepath.Join(dir, ".git", "hooks", "pre-push")
	data, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("hook file not found: %v", err)
	}
	if !strings.Contains(string(data), managedSentinel) {
		t.Error("hook content should contain managed sentinel")
	}

	// Check executable permission.
	info, err := os.Stat(hookPath)
	if err != nil {
		t.Fatalf("os.Stat: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Error("hook file should be executable")
	}

	// Re-install should succeed (overwrite our own hook).
	if err := Install(); err != nil {
		t.Fatalf("re-Install() error: %v", err)
	}

	// Uninstall.
	if err := Uninstall(); err != nil {
		t.Fatalf("Uninstall() error: %v", err)
	}

	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Error("hook file should be removed after uninstall")
	}
}

// TestHookPolicy covers table-driven cases for hook protection behavior.
func TestHookPolicy(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, dir string) // optional pre-test setup
		op        func() error                    // operation under test
		wantErr   bool
		errSubstr string
	}{
		{
			name:  "install refuses to overwrite foreign hook",
			setup: writeForeignHook,
			op:    Install,
			wantErr:   true,
			errSubstr: "already exists",
		},
		{
			name:  "uninstall refuses to remove foreign hook",
			setup: writeForeignHook,
			op:    Uninstall,
			wantErr:   true,
			errSubstr: "not installed by code-reviewer",
		},
		{
			name:    "uninstall succeeds when no hook exists",
			op:      Uninstall,
			wantErr: false,
		},
		{
			name: "install preserves foreign hook that mentions code-reviewer in comment",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				hookDir := filepath.Join(dir, ".git", "hooks")
				if err := os.MkdirAll(hookDir, 0o755); err != nil {
					t.Fatalf("os.MkdirAll: %v", err)
				}
				// Foreign hook that happens to mention code-reviewer but lacks the sentinel.
				content := "#!/bin/sh\n# Run code-reviewer manually if needed\necho 'my hook'"
				if err := os.WriteFile(filepath.Join(hookDir, "pre-push"), []byte(content), 0o755); err != nil {
					t.Fatalf("os.WriteFile: %v", err)
				}
			},
			op:        Install,
			wantErr:   true,
			errSubstr: "already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := initGitRepo(t)
			chdirRepo(t, dir)

			if tt.setup != nil {
				tt.setup(t, dir)
			}

			err := tt.op()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error should contain %q, got: %v", tt.errSubstr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}
