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
	lineDiff := a.Line - b.Line
	if lineDiff < 0 {
		lineDiff = -lineDiff
	}
	return lineDiff <= 3
}
