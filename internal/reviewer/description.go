package reviewer

import (
	"fmt"
	"strings"
)

const (
	descMarkerStart = "<!-- code-reviewer:start -->"
	descMarkerEnd   = "<!-- code-reviewer:end -->"
)

// buildDescriptionSection wraps the review summary in HTML comment markers.
func buildDescriptionSection(summary string) string {
	return fmt.Sprintf("%s\n%s\n%s", descMarkerStart, summary, descMarkerEnd)
}

// replaceDescriptionSection replaces or appends the review section in a description.
func replaceDescriptionSection(existing, section string) string {
	startIdx := strings.Index(existing, descMarkerStart)
	endIdx := strings.Index(existing, descMarkerEnd)
	if startIdx >= 0 && endIdx >= 0 {
		endIdx += len(descMarkerEnd)
		return existing[:startIdx] + section + existing[endIdx:]
	}
	// Append with separator.
	if existing != "" {
		return existing + "\n\n---\n\n" + section
	}
	return section
}
