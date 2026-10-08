package model

import "testing"

func TestFindingsMatch_RemovedLines(t *testing.T) {
	removed := func(old int) Finding {
		return Finding{File: "a.go", OldLine: old, Category: "bug"}
	}
	added := Finding{File: "a.go", Line: 21, Category: "bug"}

	if !FindingsMatch(removed(21), removed(23)) {
		t.Error("removed-line findings 2 lines apart should match")
	}
	if FindingsMatch(removed(21), removed(40)) {
		t.Error("removed-line findings 19 lines apart should not match")
	}
	if FindingsMatch(removed(21), added) {
		t.Error("a removed-line finding must not match a new-side finding with the same number")
	}
}
