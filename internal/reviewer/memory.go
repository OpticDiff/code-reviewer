package reviewer

import (
	"fmt"
	"strings"
)

// Turn represents a single conversation turn in the agent loop.
type Turn struct {
	Role    string `json:"role"` // "system", "user", "assistant", "tool"
	Content string `json:"content"`
}

// Memory tracks conversation turns with compression to stay within token limits.
type Memory struct {
	turns       []Turn
	maxTokens   int
	compressed  bool
}

// NewMemory creates a new Memory with the given approximate token budget.
func NewMemory(maxTokens int) *Memory {
	return &Memory{
		turns:      make([]Turn, 0),
		maxTokens:  maxTokens,
		compressed: false,
	}
}

// Add appends a turn to memory.
func (m *Memory) Add(role, content string) {
	m.turns = append(m.turns, Turn{Role: role, Content: content})
}

// Turns returns all turns (compressed + recent).
func (m *Memory) Turns() []Turn {
	return m.turns
}

// EstimateTokens returns an approximate token count (chars / 4).
func (m *Memory) EstimateTokens() int {
	totalChars := 0
	for _, t := range m.turns {
		totalChars += len(t.Content)
	}
	return totalChars / 4
}

// ShouldCompress returns true if memory is getting large enough to warrant compression.
func (m *Memory) ShouldCompress() bool {
	return float64(m.EstimateTokens()) > float64(m.maxTokens)*0.6
}

// Compress squeezes older turns into a summary, keeping the last keepTurns
// turns verbatim. The summary replaces all earlier turns with a single
// "system" turn containing a condensed representation.
func (m *Memory) Compress(keepTurns int) {
	if len(m.turns) <= keepTurns {
		return
	}

	var newTurns []Turn
	systemTurns := 0

	// Pin all initial system turns
	for _, t := range m.turns {
		if t.Role == "system" && !strings.HasPrefix(t.Content, "[Compressed Context]") {
			newTurns = append(newTurns, t)
			systemTurns++
		} else {
			break
		}
	}

	remainingTurns := len(m.turns) - systemTurns
	if remainingTurns <= keepTurns {
		return
	}

	compressCount := remainingTurns - keepTurns
	toCompress := m.turns[systemTurns : systemTurns+compressCount]
	recentTurns := m.turns[systemTurns+compressCount:]

	var summaryLines []string
	summaryLines = append(summaryLines, "[Compressed Context]")
	for _, t := range toCompress {
		if t.Role == "system" && strings.HasPrefix(t.Content, "[Compressed Context]") {
			summaryLines = append(summaryLines, t.Content)
			continue
		}
		
		firstLine := t.Content
		if idx := strings.Index(t.Content, "\n"); idx != -1 {
			firstLine = strings.TrimSpace(t.Content[:idx])
		}
		
		summaryLines = append(summaryLines, fmt.Sprintf("- %s: %s", t.Role, firstLine))
	}

	summaryContent := strings.Join(summaryLines, "\n")
	newTurns = append(newTurns, Turn{
		Role:    "system",
		Content: summaryContent,
	})

	newTurns = append(newTurns, recentTurns...)

	m.turns = newTurns
	m.compressed = true
}

// BuildPrompt renders all turns into a single string suitable for the model.
func (m *Memory) BuildPrompt() string {
	var sb strings.Builder

	turnCount := 1
	for i, t := range m.turns {
		if t.Role == "system" {
			sb.WriteString(t.Content)
			sb.WriteString("\n\n")
			continue
		}

		fmt.Fprintf(&sb, "[Turn %d]\n%s: %s\n", turnCount, titleCase(t.Role), t.Content)
		turnCount++
		
		if i < len(m.turns)-1 {
			sb.WriteString("\n")
		}
	}

	return strings.TrimSpace(sb.String())
}
