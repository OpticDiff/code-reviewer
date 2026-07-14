package context

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/OpticDiff/code-reviewer/internal/diff"
	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// SymbolExtractor parses diffs to find changed symbols.
type SymbolExtractor interface {
	Extract(diffs []diff.FileDiff, repoRoot string) ([]SymbolChange, error)
}

// TreeSitterExtractor uses gotreesitter to parse source files and extract
// symbols whose definitions overlap with changed lines in the diff.
type TreeSitterExtractor struct {
	// MinNameLength filters out short symbol names that would produce
	// noisy grep results (e.g., "ID", "Err"). Default: 4.
	MinNameLength int
}

// NewTreeSitterExtractor creates a new extractor with sensible defaults.
func NewTreeSitterExtractor() *TreeSitterExtractor {
	return &TreeSitterExtractor{
		MinNameLength: 4,
	}
}

// Extract parses each changed file in the diff, runs tree-sitter queries to
// find symbol definitions, and returns only those symbols whose definitions
// overlap with changed (added) lines.
func (e *TreeSitterExtractor) Extract(diffs []diff.FileDiff, repoRoot string) ([]SymbolChange, error) {
	var symbols []SymbolChange

	for _, fd := range diffs {
		path := fd.NewPath
		if fd.IsDelete {
			continue // Deleted files can't have callers to worry about.
		}

		lang := languageForFile(path)
		if lang == "" {
			continue // Unsupported language, skip gracefully.
		}

		query, ok := languageQueries[lang]
		if !ok {
			continue
		}

		// Read the current file from disk.
		fullPath := filepath.Join(repoRoot, path)
		source, err := os.ReadFile(fullPath)
		if err != nil {
			slog.Debug("context: could not read file, skipping",
				"path", path, "error", err)
			continue
		}

		// Parse with tree-sitter.
		fileSymbols, err := extractSymbols(source, lang, query)
		if err != nil {
			slog.Debug("context: tree-sitter parse failed, skipping",
				"path", path, "error", err)
			continue
		}

		// Filter to only symbols on changed lines.
		changedLines := extractedLines(fd)
		for _, sym := range fileSymbols {
			if !changedLines[sym.Line] {
				continue
			}
			if len(sym.Name) < e.MinNameLength {
				continue
			}
			sym.File = path
			sym.Language = lang
			symbols = append(symbols, sym)
		}
	}

	return symbols, nil
}

// extractSymbols runs tree-sitter queries on source code and returns all
// symbol definitions found.
func extractSymbols(source []byte, lang, queryStr string) ([]SymbolChange, error) {
	// Detect the grammar for this language.
	entry := grammars.DetectLanguage(dummyFilename(lang))
	if entry == nil {
		return nil, fmt.Errorf("no tree-sitter grammar for %q", lang)
	}

	tsLang := entry.Language()
	parser := gts.NewParser(tsLang)
	tree, err := parser.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("parsing source: %w", err)
	}

	root := tree.RootNode()

	// Compile and execute the query.
	q, err := gts.NewQuery(queryStr, tsLang)
	if err != nil {
		return nil, fmt.Errorf("compiling query for %q: %w", lang, err)
	}

	matches := q.ExecuteNode(root, tsLang, source)

	var symbols []SymbolChange
	for _, match := range matches {
		for _, capture := range match.Captures {
			node := capture.Node
			name := capture.Text(source)
			captureName := capture.Name

			// Parse kind from capture name (e.g., "symbol.function" → "function").
			kind := "unknown"
			if parts := strings.SplitN(captureName, ".", 2); len(parts) == 2 {
				kind = parts[1]
			}

			symbols = append(symbols, SymbolChange{
				Name: name,
				Kind: kind,
				Line: int(node.StartPoint().Row) + 1, // tree-sitter is 0-indexed
			})
		}
	}

	return symbols, nil
}

// dummyFilename returns a filename with the right extension for language detection.
func dummyFilename(lang string) string {
	switch lang {
	case "go":
		return "x.go"
	case "kotlin":
		return "x.kt"
	case "java":
		return "x.java"
	case "python":
		return "x.py"
	case "typescript":
		return "x.ts"
	default:
		return "x." + lang
	}
}
