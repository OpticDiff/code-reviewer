package diff

import (
	"fmt"
	"reflect"
	"strings"
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
		{"gemini-3.8-flash", 1000000},
		{"gemini-2.5-flash", 1000000},
		{"claude-sonnet-5", 1000000},
		{"claude-haiku-4-5", 200000},
		{"claude-haiku-4-5@20251001", 200000}, // Vertex AI snapshot suffix.
		{"claude-sonnet-4", 200000},
		{"unknown-model", 128000},          // Default.
		{"unknown-model@20251001", 128000}, // Default, suffix or not.
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

// sizedDiff returns a one-line diff for path whose content makes it roughly
// `tokens` estimated tokens.
func sizedDiff(path string, tokens int) FileDiff {
	return FileDiff{
		NewPath: path,
		Hunks: []Hunk{{
			Lines: []DiffLine{{Type: LineAdded, Content: strings.Repeat("x", tokens*4), NewLineNo: 1}},
		}},
	}
}

func chunkPaths(chunks [][]FileDiff) [][]string {
	out := make([][]string, len(chunks))
	for i, c := range chunks {
		for _, d := range c {
			out[i] = append(out[i], d.NewPath)
		}
	}
	return out
}

func TestSplitStrategy_TargetTokensPacksToTarget(t *testing.T) {
	var diffs []FileDiff
	for i := range 10 {
		diffs = append(diffs, sizedDiff(fmt.Sprintf("f%02d.go", i), 100))
	}

	// Default: the 1M limit holds everything in one chunk.
	chunks, err := (&SplitStrategy{}).Chunk(diffs, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 1 {
		t.Fatalf("default packing: got %d chunks, want 1", len(chunks))
	}

	// A 350-token target holds three 100-token files per chunk.
	chunks, err = (&SplitStrategy{TargetTokens: 350}).Chunk(diffs, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 4 {
		t.Fatalf("got %d chunks, want 4 (3+3+3+1)", len(chunks))
	}
	for i, c := range chunks {
		if got := EstimateTokens(c); got > 350 {
			t.Errorf("chunk %d is ~%d tokens, over the 350 target", i, got)
		}
	}
}

func TestSplitStrategy_TargetTokensAboveDefaultIsCapped(t *testing.T) {
	var diffs []FileDiff
	for i := range 4 {
		diffs = append(diffs, sizedDiff(fmt.Sprintf("f%d.go", i), 300))
	}
	// Limit 1000 leaves 800 for diff content; a target of 5000 must not raise it.
	chunks, err := (&SplitStrategy{TargetTokens: 5000}).Chunk(diffs, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2 (the 80%% cap of 800 tokens holds two files)", len(chunks))
	}
}

func TestSplitStrategy_FileLargerThanTargetGetsOwnChunk(t *testing.T) {
	diffs := []FileDiff{
		sizedDiff("small1.go", 50),
		sizedDiff("huge.go", 900),
		sizedDiff("small2.go", 50),
	}
	chunks, err := (&SplitStrategy{TargetTokens: 200}).Chunk(diffs, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"huge.go"}, {"small1.go", "small2.go"}}
	if got := chunkPaths(chunks); !reflect.DeepEqual(got, want) {
		t.Errorf("chunks = %v, want %v", got, want)
	}
}

func TestSplitStrategy_DeterministicOrder(t *testing.T) {
	// Equal sizes: the order must follow the input, run after run.
	var diffs []FileDiff
	for i := range 12 {
		diffs = append(diffs, sizedDiff(fmt.Sprintf("f%02d.go", i), 100))
	}
	s := &SplitStrategy{TargetTokens: 250}
	first, err := s.Chunk(diffs, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"f00.go", "f01.go"}, {"f02.go", "f03.go"}, {"f04.go", "f05.go"},
		{"f06.go", "f07.go"}, {"f08.go", "f09.go"}, {"f10.go", "f11.go"},
	}
	if got := chunkPaths(first); !reflect.DeepEqual(got, want) {
		t.Fatalf("chunks = %v, want %v", got, want)
	}
	for range 20 {
		again, err := s.Chunk(diffs, 1_000_000)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(chunkPaths(again), chunkPaths(first)) {
			t.Fatal("chunking the same diff twice gave different chunks")
		}
	}
}

func TestSplitStrategy_ZeroTargetKeepsDefault(t *testing.T) {
	diffs := makeTestDiffs(10, 100)
	limit := EstimateTokens(diffs) / 2
	a, err := (&SplitStrategy{}).Chunk(diffs, limit)
	if err != nil {
		t.Fatal(err)
	}
	b, err := (&SplitStrategy{TargetTokens: 0}).Chunk(diffs, limit)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(chunkPaths(a), chunkPaths(b)) {
		t.Error("TargetTokens 0 changed the chunking")
	}
}

func TestNewChunkStrategyWithTarget(t *testing.T) {
	s, err := NewChunkStrategyWithTarget("split", 1234)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.(*SplitStrategy).TargetTokens; got != 1234 {
		t.Errorf("TargetTokens = %d, want 1234", got)
	}
	if _, err := NewChunkStrategyWithTarget("bogus", 1); err == nil {
		t.Error("expected an error for an unknown strategy")
	}
}
