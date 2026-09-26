package repair

import (
	"encoding/json"
	"regexp"
	"strings"
)

// RepairJSON attempts to fix common LLM JSON output issues:
// - Strips markdown code fences (```json ... ```)
// - Fixes double-encoded JSON strings
// - Escapes bare control characters in string values
// - Fixes unescaped internal double quotes
// Returns the repaired string and whether any repairs were made.
func RepairJSON(raw string) (string, bool) {
	original := raw
	repaired := raw

	// 1. Strip markdown code fences
	if strings.Contains(repaired, "```") {
		re := regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)\\s*```")
		if match := re.FindStringSubmatch(repaired); len(match) > 1 {
			repaired = match[1]
		}
	}

	// 2. Fix double-encoded (or triple-encoded) JSON strings
	for {
		trimmed := strings.TrimSpace(repaired)
		if strings.HasPrefix(trimmed, `"`) && strings.HasSuffix(trimmed, `"`) {
			var unquoted string
			if err := json.Unmarshal([]byte(trimmed), &unquoted); err == nil {
				repaired = unquoted
				if strings.HasPrefix(strings.TrimSpace(unquoted), "{") || strings.HasPrefix(strings.TrimSpace(unquoted), "[") {
					break
				}
				continue
			}
		}
		break
	}

	// 3 & 4. Escape bare control characters & fix unescaped internal double quotes
	repaired = fixJSONStrings(repaired)

	return repaired, original != repaired
}

func fixJSONStrings(s string) string {
	var sb strings.Builder
	inString := false
	var prev rune

	for i := 0; i < len(s); i++ {
		r := rune(s[i])
		if r == '"' && prev != '\\' {
			var isBoundary bool
			if inString {
				isBoundary = false
				for j := i + 1; j < len(s); j++ {
					c := s[j]
					if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
						continue
					}
					if c == ',' || c == ':' || c == '}' || c == ']' {
						isBoundary = true
					}
					break
				}
				if i+1 == len(s) {
					isBoundary = true
				}
			} else {
				isBoundary = false
				for j := i - 1; j >= 0; j-- {
					c := s[j]
					if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
						continue
					}
					if c == '{' || c == '[' || c == ',' || c == ':' {
						isBoundary = true
					}
					break
				}
				if i == 0 {
					isBoundary = true
				}
			}

			if !isBoundary {
				sb.WriteString(`\"`)
				prev = '"'
				continue
			} else {
				inString = !inString
			}
		}

		if inString {
			switch r {
			case '\n':
				sb.WriteString(`\n`)
			case '\r':
				sb.WriteString(`\r`)
			case '\t':
				sb.WriteString(`\t`)
			default:
				sb.WriteRune(r)
			}
		} else {
			sb.WriteRune(r)
		}
		prev = r
	}

	return sb.String()
}

// RepairedAcceptable validates that a JSON repair didn't corrupt the data.
// Checks: didn't fuse separate objects, didn't truncate strings.
func RepairedAcceptable(original, repaired string) bool {
	origObjCount := strings.Count(original, "{")
	repObjCount := strings.Count(repaired, "{")
	if origObjCount > 0 && repObjCount < origObjCount {
		return false
	}

	if len(repaired) < len(original)/2 {
		return false
	}

	return true
}
