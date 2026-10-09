package diff

import (
	"fmt"
	"strings"
	"testing"
	"testing/quick"
)

func TestProperty_Filter_Idempotent(t *testing.T) {
	// Property: Filter is idempotent: Filter(Filter(diffs, p), p) == Filter(diffs, p).
	patterns := []string{"*.gen.go", "vendor/*", "dist/**", "package-lock.json"}

	property := func(fileSeeds []uint8) bool {
		var diffs []FileDiff
		for i, s := range fileSeeds {
			var path string
			switch s % 5 {
			case 0:
				path = fmt.Sprintf("src/file%d.gen.go", i)
			case 1:
				path = fmt.Sprintf("vendor/pkg%d/file.go", i)
			case 2:
				path = fmt.Sprintf("dist/bundle%d.js", i)
			case 3:
				path = "package-lock.json"
			default:
				path = fmt.Sprintf("pkg/service/logic%d.go", i)
			}
			diffs = append(diffs, FileDiff{NewPath: path})
		}

		filtered1 := Filter(diffs, patterns)
		filtered2 := Filter(filtered1, patterns)

		if len(filtered1) != len(filtered2) {
			return false
		}
		for i := range filtered1 {
			if filtered1[i].NewPath != filtered2[i].NewPath {
				return false
			}
		}
		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatalf("Property TestProperty_Filter_Idempotent failed: %v", err)
	}
}

func TestProperty_Filter_ExcludesAllMatching(t *testing.T) {
	// Property: None of the retained diffs in Filter match any exclusion pattern.
	patterns := []string{"*.min.js", "docs/*", "build/*"}

	property := func(fileSeeds []uint8) bool {
		var diffs []FileDiff
		for i, s := range fileSeeds {
			var path string
			switch s % 4 {
			case 0:
				path = fmt.Sprintf("assets/app%d.min.js", i)
			case 1:
				path = fmt.Sprintf("docs/page%d.md", i)
			case 2:
				path = fmt.Sprintf("build/output%d.bin", i)
			default:
				path = fmt.Sprintf("src/handler%d.go", i)
			}
			diffs = append(diffs, FileDiff{NewPath: path})
		}

		filtered := Filter(diffs, patterns)
		for _, d := range filtered {
			if IsExcluded(d.NewPath, patterns) {
				return false // Excluded file was leaked through filter!
			}
		}
		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatalf("Property TestProperty_Filter_ExcludesAllMatching failed: %v", err)
	}
}

func TestProperty_SplitStrategy_FileCompletenessAndUniqueness(t *testing.T) {
	// Property: For any set of file diffs and token limit, SplitStrategy.Chunk
	// preserves every file exactly once across all chunks (no loss, no duplication).
	strategy := &SplitStrategy{}

	property := func(fileCount uint8, limit uint16) bool {
		count := int(fileCount%30) + 1 // 1..30 files
		tokenLimit := int(limit%5000) + 100

		var diffs []FileDiff
		expectedPaths := make(map[string]int)
		for i := 0; i < count; i++ {
			p := fmt.Sprintf("pkg/file%d.go", i)
			expectedPaths[p]++
			diffs = append(diffs, FileDiff{
				NewPath: p,
				Hunks: []Hunk{{
					Lines: []DiffLine{
						{Type: LineAdded, Content: fmt.Sprintf("code line for file %d", i)},
					},
				}},
			})
		}

		chunks, err := strategy.Chunk(diffs, tokenLimit)
		if err != nil {
			return false
		}
		if len(chunks) == 0 {
			return false // non-empty input must produce >= 1 chunk
		}

		seenPaths := make(map[string]int)
		for _, chunk := range chunks {
			if len(chunk) == 0 {
				return false // chunks should not be empty
			}
			for _, d := range chunk {
				seenPaths[d.NewPath]++
			}
		}

		if len(seenPaths) != len(expectedPaths) {
			return false
		}
		for path, expCount := range expectedPaths {
			if seenPaths[path] != expCount {
				return false
			}
		}
		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatalf("Property TestProperty_SplitStrategy_FileCompletenessAndUniqueness failed: %v", err)
	}
}

func TestProperty_ParseUnifiedDiff_NoPanicOnArbitraryStrings(t *testing.T) {
	// Property: Parse never panics on arbitrary string inputs.
	property := func(raw string) bool {
		_, _ = Parse(strings.NewReader(raw))
		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 500}); err != nil {
		t.Fatalf("Property TestProperty_ParseUnifiedDiff_NoPanicOnArbitraryStrings failed: %v", err)
	}
}

func TestProperty_EstimateTokens_Monotonicity(t *testing.T) {
	// Property: Adding hunks or lines to FileDiff never decreases the estimated token count.
	property := func(line1, line2 string) bool {
		base := []FileDiff{{
			NewPath: "main.go",
			Hunks: []Hunk{{
				Lines: []DiffLine{{Type: LineContext, Content: line1}},
			}},
		}}
		expanded := []FileDiff{{
			NewPath: "main.go",
			Hunks: []Hunk{
				{Lines: []DiffLine{
					{Type: LineContext, Content: line1},
					{Type: LineAdded, Content: line2},
				}},
			},
		}}

		t1 := EstimateTokens(base)
		t2 := EstimateTokens(expanded)
		return t2 >= t1
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatalf("Property TestProperty_EstimateTokens_Monotonicity failed: %v", err)
	}
}

func TestProperty_ParseUnifiedDiff_HunkLineInvariants(t *testing.T) {
	// Property: For validly formatted unified diffs, parsed line numbers are
	// monotonic within each hunk.
	property := func(start uint16, addedCount uint8) bool {
		s := int(start%1000) + 1
		c := int(addedCount%20) + 1

		var sb strings.Builder
		sb.WriteString("diff --git a/test.go b/test.go\n")
		sb.WriteString("--- a/test.go\n")
		sb.WriteString("+++ b/test.go\n")
		fmt.Fprintf(&sb, "@@ -%d,1 +%d,%d @@\n", s, s, c)
		sb.WriteString(" context line\n")
		for i := 1; i < c; i++ {
			fmt.Fprintf(&sb, "+added line %d\n", i)
		}

		diffs, err := Parse(strings.NewReader(sb.String()))
		if err != nil || len(diffs) != 1 || len(diffs[0].Hunks) != 1 {
			return false
		}
		hunk := diffs[0].Hunks[0]
		prevNew := 0
		for _, l := range hunk.Lines {
			if l.NewLineNo > 0 {
				if l.NewLineNo <= prevNew {
					return false // must be strictly increasing
				}
				prevNew = l.NewLineNo
			}
		}
		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatalf("Property TestProperty_ParseUnifiedDiff_HunkLineInvariants failed: %v", err)
	}
}
