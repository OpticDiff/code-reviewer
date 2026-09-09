//go:build eval
// +build eval

// Package eval provides persona evaluation tests for the dual-review
// profile system. These tests validate that platform and product personas
// correctly isolate their findings — platform surfaces security/compliance
// issues while product surfaces code quality issues.
//
// Tests require a real LLM backend (set GOOGLE_CLOUD_PROJECT).
//
// Run: nix develop -c go test -tags=eval ./eval/ -v -timeout=600s
package eval
