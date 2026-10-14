package repair

import (
	"encoding/json"
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

	for range 4 {
		next := repairOnce(repaired)
		if next == repaired {
			break
		}
		repaired = next
	}

	return repaired, original != repaired
}

func repairOnce(s string) string {
	repaired := stripMarkdownFences(s)
	repaired = unwrapEncodedJSON(repaired)

	trimmed := strings.TrimSpace(repaired)
	if trimmed != "" && json.Valid([]byte(trimmed)) {
		return repaired
	}

	return fixJSONStrings(repaired)
}

func stripMarkdownFences(s string) string {
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, `"`) {
		return s
	}
	openIdx := strings.Index(s, "```")
	if openIdx < 0 {
		return s
	}
	afterOpen := s[openIdx+3:]
	if len(afterOpen) >= 4 && strings.EqualFold(afterOpen[:4], "json") {
		if len(afterOpen) == 4 || afterOpen[4] == '\n' || afterOpen[4] == '\r' || afterOpen[4] == ' ' || afterOpen[4] == '\t' {
			afterOpen = afterOpen[4:]
		}
	}
	closeIdx := strings.LastIndex(afterOpen, "```")
	if closeIdx < 0 {
		return s
	}
	return strings.TrimSpace(afterOpen[:closeIdx])
}

func unwrapEncodedJSON(s string) string {
	curr := s
	for range 5 {
		trimmed := strings.TrimSpace(curr)
		if !strings.HasPrefix(trimmed, `"`) || !strings.HasSuffix(trimmed, `"`) {
			break
		}
		var unquoted string
		if err := json.Unmarshal([]byte(trimmed), &unquoted); err != nil {
			break
		}
		uTrim := strings.TrimSpace(unquoted)
		if strings.HasPrefix(uTrim, "{") || strings.HasPrefix(uTrim, "[") {
			return unquoted
		}
		if strings.HasPrefix(uTrim, `"`) && strings.HasSuffix(uTrim, `"`) {
			curr = unquoted
			continue
		}
		break
	}
	return s
}

func isEscaped(s string, i int) bool {
	backslashes := 0
	for j := i - 1; j >= 0 && s[j] == '\\'; j-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func fixJSONStrings(s string) string {
	var sb strings.Builder
	inString := false

	// Iterate bytes and copy them verbatim: converting a byte to a rune would
	// re-encode every byte of a multi-byte UTF-8 sequence as Latin-1.
	for i := 0; i < len(s); i++ {
		r := rune(s[i])
		if r == '"' && !isEscaped(s, i) {
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
				sb.WriteByte(s[i])
			}
		} else {
			sb.WriteByte(s[i])
		}
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
