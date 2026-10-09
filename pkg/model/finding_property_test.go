package model

import (
	"testing"
	"testing/quick"
)

func TestProperty_FindingsMatch_Reflexivity(t *testing.T) {
	// Property: Every finding matches itself (reflexivity).
	property := func(file string, line, oldLine int, cat string) bool {
		f := Finding{
			File:     file,
			Line:     line,
			OldLine:  oldLine,
			Category: cat,
		}
		return FindingsMatch(f, f)
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatalf("Property TestProperty_FindingsMatch_Reflexivity failed: %v", err)
	}
}

func TestProperty_FindingsMatch_Symmetry(t *testing.T) {
	// Property: FindingsMatch is symmetric: FindingsMatch(a, b) == FindingsMatch(b, a).
	property := func(fileA, fileB string, lineA, lineB, oldLineA, oldLineB int, catA, catB string) bool {
		a := Finding{File: fileA, Line: lineA, OldLine: oldLineA, Category: catA}
		b := Finding{File: fileB, Line: lineB, OldLine: oldLineB, Category: catB}
		return FindingsMatch(a, b) == FindingsMatch(b, a)
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatalf("Property TestProperty_FindingsMatch_Symmetry failed: %v", err)
	}
}

func TestProperty_FindingsMatch_OldVsNewNeverMatch(t *testing.T) {
	// Property: A removed-line finding (Line<=0, OldLine>0) never matches
	// a new-side finding (Line>0), even if file, category, and line numbers match.
	property := func(file, cat string, lineNum uint16) bool {
		if lineNum == 0 {
			lineNum = 1
		}
		num := int(lineNum)
		removed := Finding{File: file, Line: 0, OldLine: num, Category: cat}
		added := Finding{File: file, Line: num, OldLine: 0, Category: cat}

		return !FindingsMatch(removed, added) && !FindingsMatch(added, removed)
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatalf("Property TestProperty_FindingsMatch_OldVsNewNeverMatch failed: %v", err)
	}
}

func TestProperty_FindingsMatch_DistanceEquivalence(t *testing.T) {
	// Property: For same file and category on new-side, findings match
	// if and only if |lineA - lineB| <= 3.
	property := func(file, cat string, lineA, lineB int) bool {
		if lineA <= 0 || lineB <= 0 {
			return true // skip non-positive new-side lines
		}
		a := Finding{File: file, Line: lineA, Category: cat}
		b := Finding{File: file, Line: lineB, Category: cat}

		diff := lineA - lineB
		if diff < 0 {
			diff = -diff
		}
		expected := diff <= 3
		return FindingsMatch(a, b) == expected
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatalf("Property TestProperty_FindingsMatch_DistanceEquivalence failed: %v", err)
	}
}
