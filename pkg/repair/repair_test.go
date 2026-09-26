package repair

import (
	"testing"
)

func TestRepairJSON(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		madeRepairs bool
	}{
		{
			name:        "valid json passes through",
			input:       `{"key": "value"}`,
			expected:    `{"key": "value"}`,
			madeRepairs: false,
		},
		{
			name:        "markdown code fence stripping",
			input:       "```json\n{\"key\": \"value\"}\n```",
			expected:    `{"key": "value"}`,
			madeRepairs: true,
		},
		{
			name:        "double-encoded JSON repair",
			input:       `"{\n  \"key\": \"value\"\n}"`,
			expected:    "{\n  \"key\": \"value\"\n}",
			madeRepairs: true,
		},
		{
			name:        "control character escaping",
			input:       "{\"key\": \"value\nwith\nnewlines\"}",
			expected:    "{\"key\": \"value\\nwith\\nnewlines\"}",
			madeRepairs: true,
		},
		{
			name:        "unescaped internal double quotes",
			input:       `{"message": "Here is a "quote" inside"}`,
			expected:    `{"message": "Here is a \"quote\" inside"}`,
			madeRepairs: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repaired, madeRepairs := RepairJSON(tt.input)
			if repaired != tt.expected {
				t.Errorf("RepairJSON() repaired = %v, expected %v", repaired, tt.expected)
			}
			if madeRepairs != tt.madeRepairs {
				t.Errorf("RepairJSON() madeRepairs = %v, expected %v", madeRepairs, tt.madeRepairs)
			}
		})
	}
}

func TestRepairedAcceptable(t *testing.T) {
	tests := []struct {
		name     string
		original string
		repaired string
		expected bool
	}{
		{
			name:     "valid pass through",
			original: `{"key": "value"}`,
			repaired: `{"key": "value"}`,
			expected: true,
		},
		{
			name:     "fused objects rejected",
			original: `{"a": 1} {"b": 2}`,
			repaired: `{"a": 1, "b": 2}`,
			expected: false,
		},
		{
			name:     "truncated rejected",
			original: `{"very_long_key_name_that_gets_truncated": "value"}`,
			repaired: `{"very_long_key": "v"}`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RepairedAcceptable(tt.original, tt.repaired); got != tt.expected {
				t.Errorf("RepairedAcceptable() = %v, expected %v", got, tt.expected)
			}
		})
	}
}

func TestRepairJSON_NestedBraces(t *testing.T) {
	input := `{"a": {"b": "here is a "quote" in a string"}}`
	expected := `{"a": {"b": "here is a \"quote\" in a string"}}`
	repaired, _ := RepairJSON(input)
	if repaired != expected {
		t.Errorf("got %q, want %q", repaired, expected)
	}
}

func TestRepairJSON_EmptyInput(t *testing.T) {
	repaired, _ := RepairJSON("")
	if repaired != "" {
		t.Errorf("got %q, want empty string", repaired)
	}
}

func TestRepairJSON_OnlyFences(t *testing.T) {
	repaired, _ := RepairJSON("```json\n```")
	if repaired != "" {
		t.Errorf("got %q, want empty string", repaired)
	}
}

func TestRepairJSON_MixedControlChars(t *testing.T) {
	input := "{\"a\": \"b\tc\nd\re\"}"
	expected := "{\"a\": \"b\\tc\\nd\\re\"}"
	repaired, _ := RepairJSON(input)
	if repaired != expected {
		t.Errorf("got %q, want %q", repaired, expected)
	}
}

func TestRepairedAcceptable_EmptyFindings(t *testing.T) {
	original := `{"findings": []}`
	if !RepairedAcceptable(original, original) {
		t.Errorf("expected empty findings to be acceptable")
	}
}

func TestRepairedAcceptable_TruncatedJSON(t *testing.T) {
	original := `{"findings": [{"file": "main.go"}]}`
	repaired := `{"find`
	if RepairedAcceptable(original, repaired) {
		t.Errorf("expected truncated JSON to be unacceptable")
	}
}

func TestRepairJSON_TripleEncoded(t *testing.T) {
	input := `"\"{\\n  \\\"key\\\": \\\"value\\\"\\n}\""`
	expected := "{\n  \"key\": \"value\"\n}"
	repaired, _ := RepairJSON(input)
	if repaired != expected {
		t.Errorf("got %q, want %q", repaired, expected)
	}
}
