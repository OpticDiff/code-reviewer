package context

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// UsageFinder searches for usages of symbols across the repository.
type UsageFinder interface {
	FindUsages(ctx context.Context, repoRoot string,
		symbols []SymbolChange, diffFiles map[string]bool) ([]CodeSnippet, error)
}

// GrepFinder uses grep (or ripgrep if available) to find symbol usages
// in unchanged files.
type GrepFinder struct {
	// MaxSnippetsPerSymbol limits results per symbol. Default: 10.
	MaxSnippetsPerSymbol int
	// MaxTotalSnippets limits total results across all symbols. Default: 50.
	MaxTotalSnippets int
	// MaxFileMatches skips symbols that match more than this many files
	// (too common to be useful). Default: 20.
	MaxFileMatches int
}

// NewGrepFinder creates a GrepFinder with sensible defaults.
func NewGrepFinder() *GrepFinder {
	return &GrepFinder{
		MaxSnippetsPerSymbol: 10,
		MaxTotalSnippets:     50,
		MaxFileMatches:       20,
	}
}

// FindUsages searches for symbol usages in the repo, excluding files
// already in the diff and common noise directories.
func (g *GrepFinder) FindUsages(ctx context.Context, repoRoot string,
	symbols []SymbolChange, diffFiles map[string]bool) ([]CodeSnippet, error) {

	if len(symbols) == 0 {
		return nil, nil
	}

	var allSnippets []CodeSnippet

	for _, sym := range symbols {
		if len(allSnippets) >= g.MaxTotalSnippets {
			break
		}

		snippets, err := g.grepSymbol(ctx, repoRoot, sym, diffFiles)
		if err != nil {
			slog.Debug("context: grep failed for symbol",
				"symbol", sym.Name, "error", err)
			continue
		}

		remaining := g.MaxTotalSnippets - len(allSnippets)
		if len(snippets) > remaining {
			snippets = snippets[:remaining]
		}
		allSnippets = append(allSnippets, snippets...)
	}

	return allSnippets, nil
}

// grepSymbol searches for a single symbol in the repo.
func (g *GrepFinder) grepSymbol(ctx context.Context, repoRoot string,
	sym SymbolChange, diffFiles map[string]bool) ([]CodeSnippet, error) {

	// Build the grep command. Prefer ripgrep if available.
	grepCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	args := g.buildGrepArgs(sym.Name, repoRoot)
	cmd := exec.CommandContext(grepCtx, args[0], args[1:]...)
	cmd.Dir = repoRoot

	output, err := cmd.Output()
	if err != nil {
		// grep returns exit code 1 for no matches — that's not an error.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		// Timeout or other errors — skip this symbol.
		return nil, fmt.Errorf("grep for %q: %w", sym.Name, err)
	}

	return g.parseGrepOutput(string(output), sym, diffFiles, repoRoot)
}

// buildGrepArgs constructs the grep command line.
func (g *GrepFinder) buildGrepArgs(symbol, repoRoot string) []string {
	// Try ripgrep first.
	if _, err := exec.LookPath("rg"); err == nil {
		return []string{
			"rg", "--no-heading", "--line-number", "--color", "never",
			"--glob", "!vendor/",
			"--glob", "!node_modules/",
			"--glob", "!.git/",
			"--glob", "!build/",
			"--glob", "!dist/",
			"--glob", "!*.min.js",
			"--glob", "!*.pb.go",
			"--glob", "!go.sum",
			"--word-regexp",
			"--",
			symbol,
			repoRoot,
		}
	}

	// Fall back to grep.
	return []string{
		"grep", "-rn", "--word-regexp",
		"--exclude-dir=vendor",
		"--exclude-dir=node_modules",
		"--exclude-dir=.git",
		"--exclude-dir=build",
		"--exclude-dir=dist",
		"--",
		symbol,
		repoRoot,
	}
}

// parseGrepOutput converts grep output lines into CodeSnippets, filtering
// out diff files, imports, and comments.
func (g *GrepFinder) parseGrepOutput(output string, sym SymbolChange,
	diffFiles map[string]bool, repoRoot string) ([]CodeSnippet, error) {

	// First pass: count unique files to enforce frequency cap.
	// This must scan ALL lines, not just the first MaxSnippetsPerSymbol,
	// so the "too common" check works even when MaxFileMatches > MaxSnippetsPerSymbol.
	type parsedLine struct {
		relPath string
		lineNo  int
		content string
	}
	var parsed []parsedLine
	uniqueFiles := make(map[string]bool)

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()

		file, lineNo, content, ok := parseGrepLine(line)
		if !ok {
			continue
		}

		// Normalize to relative path for matching against diffFiles.
		relPath := file
		if filepath.IsAbs(file) {
			if rel, err := filepath.Rel(filepath.Clean(repoRoot), file); err == nil {
				relPath = rel
			}
		}

		// Skip files already in the diff.
		if diffFiles[relPath] {
			continue
		}

		// Skip import/comment lines (simple heuristics).
		if isNoiseMatch(content) {
			continue
		}

		uniqueFiles[relPath] = true
		parsed = append(parsed, parsedLine{relPath, lineNo, content})
	}

	// If symbol matches too many files, it's too common — skip entirely.
	if len(uniqueFiles) > g.MaxFileMatches {
		slog.Debug("context: symbol too common, skipping",
			"symbol", sym.Name, "file_matches", len(uniqueFiles))
		return nil, nil
	}

	// Second pass: collect snippets up to the per-symbol limit.
	var snippets []CodeSnippet
	for _, p := range parsed {
		if len(snippets) >= g.MaxSnippetsPerSymbol {
			break
		}
		snippets = append(snippets, CodeSnippet{
			File:    p.relPath,
			Line:    p.lineNo,
			Content: strings.TrimSpace(p.content),
			Symbol:  sym.Name,
		})
	}

	return snippets, nil
}

// parseGrepLine parses a "file:line:content" grep output line.
func parseGrepLine(line string) (file string, lineNo int, content string, ok bool) {
	// Split on first two colons: file:line:content
	// Handle Windows paths (C:\...) by looking for ":digit:"
	parts := strings.SplitN(line, ":", 3)
	if len(parts) < 3 {
		return "", 0, "", false
	}

	n, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, "", false
	}

	return parts[0], n, parts[2], true
}

// isNoiseMatch returns true if the line is an import statement or comment
// that isn't useful as context.
func isNoiseMatch(content string) bool {
	trimmed := strings.TrimSpace(content)

	// Comments.
	if strings.HasPrefix(trimmed, "//") ||
		strings.HasPrefix(trimmed, "#") ||
		strings.HasPrefix(trimmed, "/*") ||
		strings.HasPrefix(trimmed, "*") {
		return true
	}

	// Import statements.
	if strings.HasPrefix(trimmed, "import ") ||
		strings.HasPrefix(trimmed, "from ") ||
		strings.HasPrefix(trimmed, "require(") {
		return true
	}

	return false
}
