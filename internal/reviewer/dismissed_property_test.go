package reviewer

import (
	"fmt"
	"strings"
	"testing"
	"testing/quick"

	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

func pbtTestDiffs(content string) []diff.FileDiff {
	return []diff.FileDiff{{
		NewPath: "main.go",
		OldPath: "main.go",
		Hunks: []diff.Hunk{{
			Header: "@@ -1,5 +1,6 @@", NewStart: 1, NewCount: 6, OldStart: 1, OldCount: 5,
			Lines: []diff.DiffLine{
				{Type: diff.LineContext, Content: "package main", OldLineNo: 1, NewLineNo: 1},
				{Type: diff.LineContext, Content: "import \"fmt\"", OldLineNo: 2, NewLineNo: 2},
				{Type: diff.LineRemoved, Content: "oldCode()", OldLineNo: 3},
				{Type: diff.LineAdded, Content: content, NewLineNo: 3},
				{Type: diff.LineContext, Content: "func main() {", OldLineNo: 4, NewLineNo: 4},
				{Type: diff.LineContext, Content: "}", OldLineNo: 5, NewLineNo: 5},
			},
		}},
	}}
}

func TestProperty_AssignFingerprints_Determinism(t *testing.T) {
	// Property: AssignFingerprints is deterministic.
	diffs := pbtTestDiffs("newCode()")

	property := func(lineNum uint8, oldSide bool, cat string) bool {
		line := int(lineNum%5) + 1
		f1 := model.Finding{File: "main.go", Category: cat}
		f2 := model.Finding{File: "main.go", Category: cat}
		if oldSide {
			f1.OldLine = line
			f2.OldLine = line
		} else {
			f1.Line = line
			f2.Line = line
		}

		fs1 := []model.Finding{f1}
		fs2 := []model.Finding{f2}
		AssignFingerprints(fs1, diffs)
		AssignFingerprints(fs2, diffs)

		return fs1[0].Fingerprint == fs2[0].Fingerprint &&
			fs1[0].Anchor == fs2[0].Anchor &&
			fs1[0].AnchorLines == fs2[0].AnchorLines
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatalf("Property TestProperty_AssignFingerprints_Determinism failed: %v", err)
	}
}

func TestProperty_AssignFingerprints_CategoryInsensitive(t *testing.T) {
	// Property: Casing and whitespace of Category do not change the fingerprint.
	diffs := pbtTestDiffs("newCode()")

	property := func(cat string) bool {
		trimmed := strings.TrimSpace(cat)
		if trimmed == "" {
			return true
		}
		fLower := model.Finding{File: "main.go", Line: 3, Category: strings.ToLower(trimmed)}
		fUpper := model.Finding{File: "main.go", Line: 3, Category: "  " + strings.ToUpper(trimmed) + "  "}

		fs1 := []model.Finding{fLower}
		fs2 := []model.Finding{fUpper}
		AssignFingerprints(fs1, diffs)
		AssignFingerprints(fs2, diffs)

		if fs1[0].Fingerprint == "" {
			return false
		}
		return fs1[0].Fingerprint == fs2[0].Fingerprint
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatalf("Property TestProperty_AssignFingerprints_CategoryInsensitive failed: %v", err)
	}
}

func TestProperty_AssignFingerprints_MetadataInvariance(t *testing.T) {
	// Property: Changing title, body, severity, or rule details does not alter
	// the computed fingerprint or anchor.
	diffs := pbtTestDiffs("newCode()")

	property := func(title1, title2, body1, body2, sev1, sev2, rule1, rule2 string) bool {
		f1 := model.Finding{
			File: "main.go", Line: 3, Category: "security",
			Title: title1, Body: body1, Severity: sev1, RuleName: rule1,
		}
		f2 := model.Finding{
			File: "main.go", Line: 3, Category: "security",
			Title: title2, Body: body2, Severity: sev2, RuleName: rule2,
		}

		fs1 := []model.Finding{f1}
		fs2 := []model.Finding{f2}
		AssignFingerprints(fs1, diffs)
		AssignFingerprints(fs2, diffs)

		return fs1[0].Fingerprint == fs2[0].Fingerprint &&
			fs1[0].Anchor == fs2[0].Anchor &&
			fs1[0].AnchorLines == fs2[0].AnchorLines
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatalf("Property TestProperty_AssignFingerprints_MetadataInvariance failed: %v", err)
	}
}

func TestProperty_FilterDismissed_Partitioning(t *testing.T) {
	// Property: FilterDismissed always partitions input findings into kept and suppressed.
	// len(kept) + len(suppressed) == len(findings).
	// Un-fingerprinted findings are always kept.
	property := func(findingsCount uint8, dismissedSeeds []uint8) bool {
		n := int(findingsCount%20) + 1
		var findings []model.Finding
		for i := 0; i < n; i++ {
			var fp string
			if i%3 != 0 {
				fp = fmt.Sprintf("fp%d", i)
			}
			findings = append(findings, model.Finding{
				File:        "main.go",
				Line:        i + 1,
				Severity:    "LOW",
				Fingerprint: fp,
			})
		}

		var dismissed []vcs.DismissedFinding
		for _, seed := range dismissedSeeds {
			dismissed = append(dismissed, vcs.DismissedFinding{
				Fingerprint: fmt.Sprintf("fp%d", int(seed%20)),
				AuthorOnly:  false,
			})
		}

		kept, suppressed := FilterDismissed(findings, dismissed)
		if len(kept)+len(suppressed) != len(findings) {
			return false
		}

		// Ensure unfingerprinted findings were kept
		for _, f := range findings {
			if f.Fingerprint == "" {
				foundInKept := false
				for _, k := range kept {
					if k.Line == f.Line {
						foundInKept = true
						break
					}
				}
				if !foundInKept {
					return false
				}
			}
		}
		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatalf("Property TestProperty_FilterDismissed_Partitioning failed: %v", err)
	}
}

func TestProperty_BlockingCount_Monotonicity(t *testing.T) {
	// Property: BlockingCount counts HIGH and CRITICAL findings only.
	// Adding LOW or MEDIUM findings never changes the blocking count.
	property := func(highCount, critCount, lowCount, medCount uint8) bool {
		var findings []model.Finding
		for i := uint8(0); i < highCount%10; i++ {
			findings = append(findings, model.Finding{Severity: "HIGH"})
		}
		for i := uint8(0); i < critCount%10; i++ {
			findings = append(findings, model.Finding{Severity: "CRITICAL"})
		}
		count1 := BlockingCount(findings)

		for i := uint8(0); i < lowCount%10; i++ {
			findings = append(findings, model.Finding{Severity: "LOW"})
		}
		for i := uint8(0); i < medCount%10; i++ {
			findings = append(findings, model.Finding{Severity: "MEDIUM"})
		}
		count2 := BlockingCount(findings)

		return count1 == count2 && count1 == int(highCount%10+critCount%10)
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatalf("Property TestProperty_BlockingCount_Monotonicity failed: %v", err)
	}
}

func TestProperty_AnchorUnchanged_OversizedSpanRejected(t *testing.T) {
	// Property: Any anchor span <= 0 or > MaxAnchorLines returns false without scanning.
	diffs := pbtTestDiffs("x := 1")

	property := func(span uint16) bool {
		var invalidSpan int
		if span%2 == 0 {
			invalidSpan = int(span) + vcs.MaxAnchorLines + 1
		} else {
			invalidSpan = -int(span)
		}
		d := vcs.DismissedFinding{
			Path:        "main.go",
			Anchor:      "abcdef",
			AnchorLines: invalidSpan,
		}
		return !anchorUnchanged(d, diffs)
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatalf("Property TestProperty_AnchorUnchanged_OversizedSpanRejected failed: %v", err)
	}
}
