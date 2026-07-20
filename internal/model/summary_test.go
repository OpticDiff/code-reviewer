package model

import (
	"strings"
	"testing"
)

func TestBuildSummaryPrompt(t *testing.T) {
	p := BuildSummaryPrompt()
	if p == "" {
		t.Fatal("summary prompt should not be empty")
	}
	if !strings.Contains(p, "PERSONA") {
		t.Error("summary prompt should contain PERSONA section")
	}
	if !strings.Contains(p, "classification") {
		t.Error("summary prompt should mention classification")
	}
	if !strings.Contains(p, "ADVERSARIAL") {
		t.Error("summary prompt should contain adversarial content warning")
	}
}

func TestBuildSummaryUserPrompt(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		desc     string
		diff     string
		wantSubs []string
	}{
		{
			name:     "full metadata",
			title:    "fix: null pointer in auth",
			desc:     "Fixes NPE when token is expired",
			diff:     "+ return nil, err",
			wantSubs: []string{"fix: null pointer", "Fixes NPE", "return nil, err"},
		},
		{
			name:     "no metadata",
			title:    "",
			desc:     "",
			diff:     "+ fmt.Println()",
			wantSubs: []string{"Code Changes", "fmt.Println"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildSummaryUserPrompt(tt.title, tt.desc, tt.diff)
			for _, sub := range tt.wantSubs {
				if !strings.Contains(result, sub) {
					t.Errorf("expected prompt to contain %q", sub)
				}
			}
		})
	}
}
