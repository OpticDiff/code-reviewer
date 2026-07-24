// Package hook provides git hook installation for code-reviewer.
package hook

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// managedSentinel is the exact marker that identifies hooks managed by code-reviewer.
// Ownership checks match this sentinel, not a loose substring, so a foreign hook
// that happens to mention "code-reviewer" in a comment is never overwritten.
const managedSentinel = "# managed-by: code-reviewer"

const preCommitHookContent = `#!/bin/sh
` + managedSentinel + `
# code-reviewer pre-push hook
# Installed by: code-reviewer hook install
# Remove with:  code-reviewer hook uninstall
#
# This hook reviews your changes before pushing.
# To skip: git push --no-verify

set -e

# Only run if there are commits to push.
if ! git diff --quiet @{push} 2>/dev/null; then
    echo "🔍 code-reviewer: reviewing changes before push..."
    code-reviewer --diff --min-severity high --no-color
fi
`

// Install writes the pre-push hook to the repository's hooks directory.
// If a hook already exists and wasn't installed by code-reviewer, it returns an error.
func Install() error {
	hooksDir, err := resolveHooksDir()
	if err != nil {
		return err
	}

	hookPath := filepath.Join(hooksDir, "pre-push")

	// Check for existing hook.
	if data, err := os.ReadFile(hookPath); err == nil {
		if !strings.Contains(string(data), managedSentinel) {
			return fmt.Errorf("pre-push hook already exists at %s\n\nTo overwrite, remove it first:\n  rm %s", hookPath, hookPath)
		}
		// Our hook — safe to overwrite.
	}

	// Ensure hooks directory exists.
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("creating hooks directory: %w", err)
	}

	if err := os.WriteFile(hookPath, []byte(preCommitHookContent), 0o755); err != nil {
		return fmt.Errorf("writing pre-push hook: %w", err)
	}

	fmt.Printf("✅ Installed pre-push hook at %s\n", hookPath)
	fmt.Println("   Reviews will run automatically on git push.")
	fmt.Println("   Skip with: git push --no-verify")
	return nil
}

// Uninstall removes the pre-push hook if it was installed by code-reviewer.
func Uninstall() error {
	hooksDir, err := resolveHooksDir()
	if err != nil {
		return err
	}

	hookPath := filepath.Join(hooksDir, "pre-push")

	data, err := os.ReadFile(hookPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No pre-push hook found.")
			return nil
		}
		return fmt.Errorf("reading hook: %w", err)
	}

	if !strings.Contains(string(data), managedSentinel) {
		return fmt.Errorf("pre-push hook at %s was not installed by code-reviewer; refusing to remove", hookPath)
	}

	if err := os.Remove(hookPath); err != nil {
		return fmt.Errorf("removing hook: %w", err)
	}

	fmt.Printf("✅ Removed pre-push hook from %s\n", hookPath)
	return nil
}

// resolveHooksDir returns the hooks directory for the current repository,
// honoring Git's core.hooksPath configuration if set.
func resolveHooksDir() (string, error) {
	// Try core.hooksPath first.
	cmd := exec.Command("git", "rev-parse", "--git-path", "hooks")
	out, err := cmd.Output()
	if err == nil {
		resolved := strings.TrimSpace(string(out))
		if resolved != "" {
			return resolved, nil
		}
	}

	// Fallback: <gitdir>/hooks.
	gitDir, err := findGitDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(gitDir, "hooks"), nil
}

// findGitDir locates the .git directory by running git rev-parse.
func findGitDir() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository (run this from inside a git repo): %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
