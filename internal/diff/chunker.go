package diff

import (
	"fmt"
	"sort"
)

// ChunkStrategy defines how large diffs are split to fit within model context windows.
// This interface is intentionally minimal to allow complex future implementations.
type ChunkStrategy interface {
	// Chunk splits a set of file diffs into groups that each fit within tokenLimit.
	// Returns an error if the strategy cannot produce valid chunks (e.g., FailStrategy
	// when the diff is too large).
	Chunk(diffs []FileDiff, tokenLimit int) ([][]FileDiff, error)
}

// FailStrategy checks whether the diff fits and returns an error if it doesn't.
// This is the default, safe strategy that forces teams to scope MRs properly.
type FailStrategy struct{}

// Chunk returns all diffs in a single chunk if they fit, or an error if they don't.
func (s *FailStrategy) Chunk(diffs []FileDiff, tokenLimit int) ([][]FileDiff, error) {
	total := EstimateTokens(diffs)
	if total > tokenLimit {
		return nil, &DiffTooLargeError{
			EstimatedTokens: total,
			TokenLimit:      tokenLimit,
			FileCount:       len(diffs),
		}
	}
	return [][]FileDiff{diffs}, nil
}

// SplitStrategy splits diffs into groups that each fit within the token limit.
// Files are sorted by size (largest first) and bin-packed into groups.
type SplitStrategy struct{}

// Chunk splits diffs into groups that fit within tokenLimit.
func (s *SplitStrategy) Chunk(diffs []FileDiff, tokenLimit int) ([][]FileDiff, error) {
	if len(diffs) == 0 {
		return nil, nil
	}

	// Reserve ~20% of the limit for the system prompt and response.
	effectiveLimit := int(float64(tokenLimit) * 0.80)
	if effectiveLimit <= 0 {
		effectiveLimit = tokenLimit
	}

	// Sort by estimated tokens descending (review largest files first).
	type indexedDiff struct {
		diff   FileDiff
		tokens int
	}
	indexed := make([]indexedDiff, len(diffs))
	for i, d := range diffs {
		indexed[i] = indexedDiff{diff: d, tokens: estimateFileDiffTokens(&d)}
	}
	sort.Slice(indexed, func(i, j int) bool {
		return indexed[i].tokens > indexed[j].tokens
	})

	// Note: if a single file exceeds the limit, we still include it.
	// The model will do its best with a truncated context.

	// Bin-pack into groups.
	var chunks [][]FileDiff
	var currentChunk []FileDiff
	currentTokens := 0

	for _, id := range indexed {
		if currentTokens+id.tokens > effectiveLimit && len(currentChunk) > 0 {
			chunks = append(chunks, currentChunk)
			currentChunk = nil
			currentTokens = 0
		}
		currentChunk = append(currentChunk, id.diff)
		currentTokens += id.tokens
	}
	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
	}

	return chunks, nil
}

// DiffTooLargeError is returned by FailStrategy when the diff exceeds the model's context window.
type DiffTooLargeError struct {
	EstimatedTokens int
	TokenLimit      int
	FileCount       int
}

func (e *DiffTooLargeError) Error() string {
	return fmt.Sprintf(
		"MR diff too large for model context window\n"+
			"  Estimated: ~%d tokens\n"+
			"  Model limit: %d tokens\n"+
			"  Files: %d\n\n"+
			"Options:\n"+
			"  --chunk-strategy split     Auto-split into review chunks\n"+
			"  --model gemini-2.5-flash   Use a model with larger context (1M tokens)\n"+
			"  --excluded-patterns \"...\"  Exclude generated/vendored files",
		e.EstimatedTokens, e.TokenLimit, e.FileCount,
	)
}

// EstimateTokens returns a rough token count for a set of file diffs.
// Uses the approximation of 1 token ≈ 4 characters.
func EstimateTokens(diffs []FileDiff) int {
	total := 0
	for _, d := range diffs {
		total += estimateFileDiffTokens(&d)
	}
	return total
}

func estimateFileDiffTokens(d *FileDiff) int {
	chars := len(d.OldPath) + len(d.NewPath)
	for _, h := range d.Hunks {
		chars += len(h.Header)
		for _, l := range h.Lines {
			chars += len(l.Content) + 2 // +2 for prefix and newline
		}
	}
	return chars / 4 // ~4 chars per token
}

// ModelTokenLimits maps known model names to their approximate context window sizes.
// These are conservative estimates leaving room for the system prompt and response.
var ModelTokenLimits = map[string]int{
	"gemini-2.5-flash":  1000000,
	"gemini-2.5-pro":    1000000,
	"gemini-2.0-flash":  1000000,
	"claude-sonnet-4":   200000,
	"claude-sonnet-4.5": 200000,
	"claude-opus-4":     200000,
	"claude-haiku-4.5":  200000,
	"mistral-medium-3":  128000,
}

// TokenLimitForModel returns the token limit for a model, or a conservative default.
func TokenLimitForModel(model string) int {
	if limit, ok := ModelTokenLimits[model]; ok {
		return limit
	}
	// Conservative default for unknown models.
	return 128000
}

// NewChunkStrategy creates a ChunkStrategy from a strategy name.
func NewChunkStrategy(name string) (ChunkStrategy, error) {
	switch name {
	case "fail", "":
		return &FailStrategy{}, nil
	case "split":
		return &SplitStrategy{}, nil
	default:
		return nil, fmt.Errorf("unknown chunk strategy: %q (valid: fail, split)", name)
	}
}
