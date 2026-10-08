package github

import "testing"

func TestPullFile_toVCSDiffEntry_TooLarge(t *testing.T) {
	tests := []struct {
		name string
		file PullFile
		want bool
	}{
		{"patch present", PullFile{Status: "modified", Patch: "@@ -1 +1 @@", Changes: 2}, false},
		{"omitted patch with changes (oversized)", PullFile{Status: "modified", Changes: 5000}, true},
		{"omitted patch, no changes (binary or mode-only)", PullFile{Status: "modified", Changes: 0}, false},
		{"pure rename", PullFile{Status: "renamed", Changes: 0}, false},
		{"renamed with omitted patch and changes", PullFile{Status: "renamed", Changes: 10}, false},
		{"added empty file", PullFile{Status: "added"}, false},
		{"removed file", PullFile{Status: "removed", Changes: 3}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.file.toVCSDiffEntry().TooLarge; got != tt.want {
				t.Errorf("TooLarge = %v, want %v", got, tt.want)
			}
		})
	}
}
