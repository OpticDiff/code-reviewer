package diff

import (
	"testing"
)

func makeTestDiffs(count, linesPerFile int) []FileDiff {
	diffs := make([]FileDiff, count)
	for i := range diffs {
		lines := make([]DiffLine, linesPerFile)
		for j := range lines {
			lines[j] = DiffLine{
				Type:      LineAdded,
				Content:   "func example() { return nil }",
				NewLineNo: j + 1,
			}
		}
		diffs[i] = FileDiff{
			NewPath: "file_" + string(rune('a'+i)) + ".go",
			Hunks: []Hunk{{
				Header:   "@@ -0,0 +1," + string(rune('0'+linesPerFile)) + " @@",
				NewStart: 1,
				NewCount: linesPerFile,
				Lines:    lines,
			}},
		}
	}
	return diffs
}

func TestFailStrategy_WithinLimit(t *testing.T) {
	diffs := makeTestDiffs(3, 10)
	s := &FailStrategy{}
	chunks, err := s.Chunk(diffs, 100000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(chunks))
	}
	if len(chunks[0]) != 3 {
		t.Errorf("expected 3 files in chunk, got %d", len(chunks[0]))
	}
}

func TestFailStrategy_ExceedsLimit(t *testing.T) {
	diffs := makeTestDiffs(5, 100)
	s := &FailStrategy{}
	_, err := s.Chunk(diffs, 10) // Tiny limit.
	if err == nil {
		t.Fatal("expected error for oversized diff")
	}
	dte, ok := err.(*DiffTooLargeError)
	if !ok {
		t.Fatalf("expected DiffTooLargeError, got %T", err)
	}
	if dte.FileCount != 5 {
		t.Errorf("FileCount = %d, want 5", dte.FileCount)
	}
}

func TestSplitStrategy_SingleChunk(t *testing.T) {
	diffs := makeTestDiffs(3, 10)
	s := &SplitStrategy{}
	chunks, err := s.Chunk(diffs, 100000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk for small diff, got %d", len(chunks))
	}
}

func TestSplitStrategy_MultipleChunks(t *testing.T) {
	diffs := makeTestDiffs(10, 100)
	tokens := EstimateTokens(diffs)
	// Set limit to roughly half.
	limit := tokens / 2

	s := &SplitStrategy{}
	chunks, err := s.Chunk(diffs, limit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks, got %d", len(chunks))
	}

	// Verify all files are present.
	totalFiles := 0
	for _, chunk := range chunks {
		totalFiles += len(chunk)
	}
	if totalFiles != 10 {
		t.Errorf("expected 10 total files across chunks, got %d", totalFiles)
	}
}

func TestSplitStrategy_EmptyInput(t *testing.T) {
	s := &SplitStrategy{}
	chunks, err := s.Chunk(nil, 100000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chunks != nil {
		t.Errorf("expected nil chunks for empty input, got %v", chunks)
	}
}

func TestEstimateTokens(t *testing.T) {
	diffs := makeTestDiffs(1, 10)
	tokens := EstimateTokens(diffs)
	if tokens <= 0 {
		t.Errorf("expected positive token count, got %d", tokens)
	}
}

func TestTokenLimitForModel(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"gemini-2.5-flash", 1000000},
		{"claude-sonnet-4", 200000},
		{"unknown-model", 128000}, // Default.
	}
	for _, tt := range tests {
		got := TokenLimitForModel(tt.model)
		if got != tt.want {
			t.Errorf("TokenLimitForModel(%q) = %d, want %d", tt.model, got, tt.want)
		}
	}
}

func TestNewChunkStrategy(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"fail", false},
		{"split", false},
		{"", false},
		{"invalid", true},
	}
	for _, tt := range tests {
		_, err := NewChunkStrategy(tt.name)
		if (err != nil) != tt.wantErr {
			t.Errorf("NewChunkStrategy(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
	}
}
