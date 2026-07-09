package reviewer

import (
	"fmt"
	"os"
	"strings"

	"github.com/OpticDiff/code-reviewer/internal/model"
	"golang.org/x/term"
)

// ANSI color codes.
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiBlue   = "\033[34m"
	ansiOrange = "\033[38;5;208m"
	ansiCyan   = "\033[36m"
	ansiWhite  = "\033[37m"
)

// isTTY returns true if stdout is a terminal (not piped).
func isTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// ColorTerminalOutput formats findings with ANSI colors for terminal display.
func ColorTerminalOutput(result *model.ReviewResult, useColor bool) string {
	if !useColor {
		return TerminalOutput(result)
	}

	var sb strings.Builder

	// Count by severity.
	counts := map[string]int{}
	for _, f := range result.Findings {
		counts[strings.ToUpper(f.Severity)]++
	}

	// Summary header box.
	sb.WriteString("\n")
	sb.WriteString(ansiBold + ansiCyan + "┌─────────────────────────────────────────────────────┐" + ansiReset + "\n")

	if len(result.Findings) == 0 {
		sb.WriteString(ansiBold + ansiCyan + "│" + ansiReset)
		sb.WriteString(ansiBold + ansiGreen + "  ✅ No issues found. Code looks clean!" + ansiReset)
		sb.WriteString(strings.Repeat(" ", 15))
		sb.WriteString(ansiBold + ansiCyan + "│" + ansiReset + "\n")
	} else {
		line := fmt.Sprintf("  Code Review: %d finding(s)", len(result.Findings))
		padded := line + strings.Repeat(" ", 53-len(line))
		sb.WriteString(ansiBold + ansiCyan + "│" + ansiReset + ansiBold + padded + ansiCyan + "│" + ansiReset + "\n")

		// Severity counts line.
		var parts []string
		if c := counts["CRITICAL"]; c > 0 {
			parts = append(parts, fmt.Sprintf("🔴 %d critical", c))
		}
		if c := counts["HIGH"]; c > 0 {
			parts = append(parts, fmt.Sprintf("🟠 %d high", c))
		}
		if c := counts["MEDIUM"]; c > 0 {
			parts = append(parts, fmt.Sprintf("🟡 %d medium", c))
		}
		if c := counts["LOW"]; c > 0 {
			parts = append(parts, fmt.Sprintf("🔵 %d low", c))
		}
		countsLine := "  " + strings.Join(parts, "  ")
		// Pad to box width (accounting for emoji width).
		padLen := 53 - len(countsLine) + (len(parts) * 2) // emojis take extra space
		if padLen < 0 {
			padLen = 0
		}
		countsPadded := countsLine + strings.Repeat(" ", padLen)
		sb.WriteString(ansiBold + ansiCyan + "│" + ansiReset + ansiDim + countsPadded + ansiBold + ansiCyan + "│" + ansiReset + "\n")
	}

	sb.WriteString(ansiBold + ansiCyan + "└─────────────────────────────────────────────────────┘" + ansiReset + "\n")

	// Summary text.
	if result.Summary != "" {
		sb.WriteString("\n" + ansiDim + result.Summary + ansiReset + "\n")
	}

	if len(result.Findings) == 0 {
		return sb.String()
	}

	// Group by file.
	byFile := make(map[string][]model.Finding)
	var fileOrder []string
	for _, f := range result.Findings {
		if _, seen := byFile[f.File]; !seen {
			fileOrder = append(fileOrder, f.File)
		}
		byFile[f.File] = append(byFile[f.File], f)
	}

	for _, file := range fileOrder {
		sb.WriteString("\n" + ansiBold + ansiWhite + "── " + file + " " + strings.Repeat("─", max(1, 50-len(file))) + ansiReset + "\n")

		for _, f := range byFile[file] {
			sevColor := severityColor(f.Severity)
			sevLabel := strings.ToUpper(f.Severity)
			fmt.Fprintf(&sb, "\n  L%-4d %s%s%-8s%s  %s%s%s\n",
				f.Line,
				sevColor, ansiBold, sevLabel, ansiReset,
				ansiBold, f.Title, ansiReset,
			)

			// Body — indented with dim pipe.
			for _, line := range strings.Split(f.Body, "\n") {
				sb.WriteString(ansiDim + "  │ " + ansiReset + line + "\n")
			}

			// Suggestion.
			if f.Suggestion != "" {
				sb.WriteString(ansiDim + "  │" + ansiReset + "\n")
				sb.WriteString(ansiDim + "  │ " + ansiGreen + "Suggestion:" + ansiReset + "\n")
				for _, line := range strings.Split(f.Suggestion, "\n") {
					sb.WriteString(ansiDim + "  │   " + ansiGreen + line + ansiReset + "\n")
				}
			}
		}
	}

	sb.WriteString("\n")
	return sb.String()
}

func severityColor(severity string) string {
	switch strings.ToUpper(severity) {
	case "CRITICAL":
		return ansiRed
	case "HIGH":
		return ansiOrange
	case "MEDIUM":
		return ansiYellow
	case "LOW":
		return ansiBlue
	default:
		return ansiWhite
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
