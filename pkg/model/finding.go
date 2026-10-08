package model

// FindingsMatch returns true if two findings refer to the same issue:
// same file, same category, and within 3 lines of each other.
// Used by both consensus mode (multi-model dedup) and chunk dedup.
func FindingsMatch(a, b Finding) bool {
	if a.File != b.File {
		return false
	}
	if a.Category != b.Category {
		return false
	}
	aRemoved, bRemoved := a.Line <= 0 && a.OldLine > 0, b.Line <= 0 && b.OldLine > 0
	if aRemoved != bRemoved {
		return false // Old-side and new-side numbers are not comparable.
	}
	lineDiff := a.Line - b.Line
	if aRemoved {
		lineDiff = a.OldLine - b.OldLine
	}
	if lineDiff < 0 {
		lineDiff = -lineDiff
	}
	return lineDiff <= 3
}
