package reviewer

import (
	"fmt"
	"strings"
)

func getMarkers(profile string) (string, string) {
	switch profile {
	case "platform":
		return "<!-- code-reviewer-platform-start -->", "<!-- code-reviewer-platform-end -->"
	case "product":
		return "<!-- code-reviewer-product-start -->", "<!-- code-reviewer-product-end -->"
	default:
		return "<!-- code-reviewer:start -->", "<!-- code-reviewer:end -->"
	}
}

// buildDescriptionSection wraps the review summary in HTML comment markers.
func buildDescriptionSection(summary, profile string) string {
	start, end := getMarkers(profile)
	return fmt.Sprintf("%s\n%s\n%s", start, summary, end)
}

// replaceDescriptionSection replaces or appends the review section in a description.
func replaceDescriptionSection(existing, section, profile string) string {
	startMarker, endMarker := getMarkers(profile)
	startIdx := strings.Index(existing, startMarker)
	endIdx := strings.Index(existing, endMarker)
	if startIdx >= 0 && endIdx >= 0 {
		endIdx += len(endMarker)
		return existing[:startIdx] + section + existing[endIdx:]
	}
	// Append with separator.
	if existing != "" {
		return existing + "\n\n---\n\n" + section
	}
	return section
}
