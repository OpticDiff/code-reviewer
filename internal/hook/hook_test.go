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

func TestInstallAndUninstall(t *testing.T) {
	dir := initGitRepo(t)

	// cd into the repo so findGitDir works.
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	// Install.
	if err := Install(); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	hookPath := filepath.Join(dir, ".git", "hooks", "pre-push")
	data, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("hook file not found: %v", err)
	}
	if !strings.Contains(string(data), "code-reviewer") {
		t.Error("hook content should contain code-reviewer marker")
	}

	// Check executable permission.
	info, _ := os.Stat(hookPath)
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

func TestInstall_ExistingForeignHook(t *testing.T) {
	dir := initGitRepo(t)

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	// Create a foreign hook.
	hookDir := filepath.Join(dir, ".git", "hooks")
	os.MkdirAll(hookDir, 0o755)
	os.WriteFile(filepath.Join(hookDir, "pre-push"), []byte("#!/bin/sh\necho 'my custom hook'"), 0o755)

	// Install should fail — refuse to overwrite foreign hook.
	err := Install()
	if err == nil {
		t.Fatal("Install() should fail when foreign hook exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention 'already exists', got: %v", err)
	}
}

func TestUninstall_ForeignHook(t *testing.T) {
	dir := initGitRepo(t)

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	// Create a foreign hook.
	hookDir := filepath.Join(dir, ".git", "hooks")
	os.MkdirAll(hookDir, 0o755)
	os.WriteFile(filepath.Join(hookDir, "pre-push"), []byte("#!/bin/sh\necho 'foreign'"), 0o755)

	// Uninstall should refuse.
	err := Uninstall()
	if err == nil {
		t.Fatal("Uninstall() should fail for foreign hook")
	}
	if !strings.Contains(err.Error(), "not installed by code-reviewer") {
		t.Errorf("error should mention 'not installed by code-reviewer', got: %v", err)
	}
}

func TestUninstall_NoHook(t *testing.T) {
	dir := initGitRepo(t)

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	// Uninstall with no hook should succeed silently.
	if err := Uninstall(); err != nil {
		t.Fatalf("Uninstall() should succeed when no hook exists: %v", err)
	}
}
