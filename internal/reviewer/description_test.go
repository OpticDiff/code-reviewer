package reviewer

import (
	"testing"
)

func TestBuildDescriptionSection(t *testing.T) {
	summary := "This is a summary."
	out := buildDescriptionSection(summary)
	want := "<!-- code-reviewer:start -->\nThis is a summary.\n<!-- code-reviewer:end -->"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestReplaceDescriptionSection(t *testing.T) {
	tests := []struct {
		name     string
		existing string
		section  string
		want     string
	}{
		{
			name:     "empty existing description",
			existing: "",
			section:  "SECTION",
			want:     "SECTION",
		},
		{
			name:     "existing description without markers",
			existing: "Original Description.",
			section:  "SECTION",
			want:     "Original Description.\n\n---\n\nSECTION",
		},
		{
			name:     "existing description with markers (replace)",
			existing: "Top\n<!-- code-reviewer:start -->\nOld\n<!-- code-reviewer:end -->\nBottom",
			section:  "<!-- code-reviewer:start -->\nNew\n<!-- code-reviewer:end -->",
			want:     "Top\n<!-- code-reviewer:start -->\nNew\n<!-- code-reviewer:end -->\nBottom",
		},
		{
			name:     "markers with content after them (preserve trailing)",
			existing: "Start\n<!-- code-reviewer:start -->Middle<!-- code-reviewer:end -->End",
			section:  "<!-- code-reviewer:start -->NewMiddle<!-- code-reviewer:end -->",
			want:     "Start\n<!-- code-reviewer:start -->NewMiddle<!-- code-reviewer:end -->End",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := replaceDescriptionSection(tt.existing, tt.section)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
