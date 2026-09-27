package reviewer

import (
	"strings"
	"testing"
)

func TestMemory_AddAndRetrieve(t *testing.T) {
	m := NewMemory(1000)
	m.Add("system", "Init")
	m.Add("user", "Hello")

	turns := m.Turns()
	if len(turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(turns))
	}
	if turns[0].Role != "system" || turns[1].Role != "user" {
		t.Errorf("unexpected roles")
	}
}

func TestMemory_EstimateTokens(t *testing.T) {
	m := NewMemory(1000)
	m.Add("user", "1234") // 4 chars
	m.Add("assistant", "5678") // 4 chars
	// Total 8 chars = 2 tokens

	if tokens := m.EstimateTokens(); tokens != 2 {
		t.Errorf("expected 2 tokens, got %d", tokens)
	}
}

func TestMemory_ShouldCompress_Threshold(t *testing.T) {
	m := NewMemory(10) // 10 tokens max -> 60% = 6 tokens -> 24 chars
	
	// Add 20 chars (5 tokens), 5 < 6 -> false
	m.Add("user", "12345678901234567890")
	if m.ShouldCompress() {
		t.Errorf("should not compress yet")
	}

	// Add 8 chars (2 tokens), total 7 tokens, 7 > 6 -> true
	m.Add("assistant", "12345678")
	if !m.ShouldCompress() {
		t.Errorf("should compress now")
	}
}

func TestMemory_Compress_KeepsRecentTurns(t *testing.T) {
	m := NewMemory(1000)
	m.Add("user", "t1")
	m.Add("assistant", "t2")
	m.Add("user", "t3")
	m.Add("assistant", "t4")
	m.Add("user", "t5")
	m.Add("assistant", "t6")

	m.Compress(2)

	turns := m.Turns()
	if len(turns) != 3 { // 1 compressed + 2 recent
		t.Fatalf("expected 3 turns, got %d", len(turns))
	}

	if turns[0].Role != "system" {
		t.Errorf("expected compressed turn to be system")
	}
	if turns[1].Content != "t5" || turns[2].Content != "t6" {
		t.Errorf("expected recent turns to be kept")
	}
}

func TestMemory_Compress_SummarizesOldTurns(t *testing.T) {
	m := NewMemory(1000)
	m.Add("user", "t1")
	m.Add("assistant", "t2")
	m.Add("user", "t3")

	m.Compress(1)
	
	turns := m.Turns()
	summary := turns[0].Content
	
	if !strings.Contains(summary, "[Compressed Context]") {
		t.Errorf("missing compressed context header")
	}
	if !strings.Contains(summary, "- user: t1") || !strings.Contains(summary, "- assistant: t2") {
		t.Errorf("missing summarized old turns")
	}
}

func TestMemory_Compress_PreservesSystemTurn(t *testing.T) {
	m := NewMemory(1000)
	m.Add("system", "Sys Context")
	m.Add("user", "t1")
	m.Add("assistant", "t2")
	m.Add("user", "t3")
	
	m.Compress(1)

	turns := m.Turns()
	if turns[0].Role != "system" || turns[0].Content != "Sys Context" {
		t.Errorf("system turn not preserved verbatim")
	}
	if turns[1].Role != "system" || !strings.Contains(turns[1].Content, "[Compressed Context]") {
		t.Errorf("compressed summary not created correctly")
	}
	if turns[2].Content != "t3" {
		t.Errorf("recent turn not kept")
	}
}

func TestMemory_BuildPrompt_Format(t *testing.T) {
	m := NewMemory(1000)
	m.Add("system", "Sys Context")
	m.Add("user", "Hello")
	m.Add("assistant", "Hi there")
	
	prompt := m.BuildPrompt()
	if !strings.Contains(prompt, "Sys Context") {
		t.Errorf("missing system context")
	}
	if !strings.Contains(prompt, "[Turn 1]\nUser: Hello") {
		t.Errorf("missing turn 1")
	}
	if !strings.Contains(prompt, "[Turn 2]\nAssistant: Hi there") {
		t.Errorf("missing turn 2")
	}
}

func TestMemory_EmptyMemory(t *testing.T) {
	m := NewMemory(1000)
	m.Compress(2) // should not panic
	
	if len(m.Turns()) != 0 {
		t.Errorf("expected 0 turns")
	}
	if m.EstimateTokens() != 0 {
		t.Errorf("expected 0 tokens")
	}
	if m.ShouldCompress() {
		t.Errorf("should not compress empty")
	}
	if m.BuildPrompt() != "" {
		t.Errorf("expected empty string")
	}
}
