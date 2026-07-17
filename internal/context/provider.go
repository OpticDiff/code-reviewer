package context

import (
	"context"
	"log/slog"

	"github.com/OpticDiff/code-reviewer/internal/diff"
)

// Provider finds code related to a diff's changes.
type Provider interface {
	FindRelatedCode(ctx context.Context, repoRoot string,
		diffs []diff.FileDiff) ([]CodeSnippet, error)
}

// DefaultProvider chains SymbolExtractor → UsageFinder to discover
// unchanged code that references symbols modified in the diff.
type DefaultProvider struct {
	extractor SymbolExtractor
	finder    UsageFinder
}

// NewDefaultProvider creates a provider with tree-sitter extraction
// and grep-based usage finding.
func NewDefaultProvider() *DefaultProvider {
	return &DefaultProvider{
		extractor: NewTreeSitterExtractor(),
		finder:    NewGrepFinder(),
	}
}

// NewProvider creates a provider with custom extractor and finder.
// Useful for testing with mocks.
func NewProvider(extractor SymbolExtractor, finder UsageFinder) *DefaultProvider {
	return &DefaultProvider{
		extractor: extractor,
		finder:    finder,
	}
}

// FindRelatedCode extracts changed symbols from the diffs, then searches
// the repo for usages of those symbols in unchanged files.
func (p *DefaultProvider) FindRelatedCode(ctx context.Context, repoRoot string,
	diffs []diff.FileDiff) ([]CodeSnippet, error) {

	// Step 1: Extract changed symbols.
	symbols, err := p.extractor.Extract(diffs, repoRoot)
	if err != nil {
		return nil, err
	}

	if len(symbols) == 0 {
		slog.Debug("context: no changed symbols found")
		return nil, nil
	}

	slog.Info("context: extracted changed symbols",
		"count", len(symbols),
		"symbols", symbolNames(symbols))

	// Build set of diff files to exclude from usage search.
	diffFiles := make(map[string]bool, len(diffs))
	for _, d := range diffs {
		diffFiles[d.NewPath] = true
		if d.OldPath != "" && d.OldPath != d.NewPath {
			diffFiles[d.OldPath] = true
		}
	}

	// Step 2: Find usages.
	snippets, err := p.finder.FindUsages(ctx, repoRoot, symbols, diffFiles)
	if err != nil {
		return nil, err
	}

	slog.Info("context: found related code",
		"snippets", len(snippets))

	return snippets, nil
}

// symbolNames returns a slice of symbol names for logging.
func symbolNames(symbols []SymbolChange) []string {
	names := make([]string, len(symbols))
	for i, s := range symbols {
		names[i] = s.Name
	}
	return names
}
