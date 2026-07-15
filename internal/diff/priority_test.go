package diff

import "testing"

func TestFilePriority_SecurityFiles(t *testing.T) {
	tests := []struct {
		path string
		want bool // should score > 100
	}{
		{"internal/auth/handler.go", true},
		{"pkg/crypto/encrypt.go", true},
		{"config/secrets.yaml", true},
		{".env", true},
		{"internal/handler/list.go", false},
		{"README.md", false},
		// Negative cases — should NOT match despite containing security substrings.
		{"pkg/monkey.go", false},        // contains "key" but is not security-related
		{"docs/authors.md", false},      // contains "auth" but is not security-related
		{"internal/tokenizer.go", false}, // contains "token" but is not security-related
	}

	for _, tt := range tests {
		fd := FileDiff{NewPath: tt.path, Hunks: []Hunk{{Lines: []DiffLine{{Type: LineAdded}}}}}
		score := FilePriority(fd)
		if tt.want && score < 100 {
			t.Errorf("FilePriority(%q) = %d, expected >= 100 (security-sensitive)", tt.path, score)
		}
		if !tt.want && score >= 100 {
			t.Errorf("FilePriority(%q) = %d, expected < 100 (not security-sensitive)", tt.path, score)
		}
	}
}

func TestFilePriority_NewVsModified(t *testing.T) {
	newFile := FileDiff{
		NewPath: "pkg/handler.go",
		IsNew:   true,
		Hunks:   []Hunk{{Lines: []DiffLine{{Type: LineAdded}, {Type: LineAdded}}}},
	}
	modFile := FileDiff{
		NewPath: "pkg/handler.go",
		Hunks:   []Hunk{{Lines: []DiffLine{{Type: LineAdded}, {Type: LineAdded}}}},
	}

	newScore := FilePriority(newFile)
	modScore := FilePriority(modFile)

	if newScore <= modScore {
		t.Errorf("new file (%d) should score higher than modified file (%d)", newScore, modScore)
	}
}

func TestFilePriority_Generated(t *testing.T) {
	gen := FileDiff{
		NewPath: "api/service.pb.go",
		Hunks:   []Hunk{{Lines: []DiffLine{{Type: LineAdded}}}},
	}
	normal := FileDiff{
		NewPath: "api/service.go",
		Hunks:   []Hunk{{Lines: []DiffLine{{Type: LineAdded}}}},
	}

	if FilePriority(gen) >= FilePriority(normal) {
		t.Errorf("generated file should score lower than normal file")
	}
}

func TestFilePriority_RenameOnly(t *testing.T) {
	rename := FileDiff{
		NewPath:  "pkg/new_name.go",
		OldPath:  "pkg/old_name.go",
		IsRename: true,
		// No hunks = no changed lines.
	}
	modified := FileDiff{
		NewPath: "pkg/handler.go",
		Hunks:   []Hunk{{Lines: []DiffLine{{Type: LineAdded}, {Type: LineAdded}, {Type: LineAdded}}}},
	}

	if FilePriority(rename) >= FilePriority(modified) {
		t.Errorf("pure rename (%d) should score lower than modified file (%d)",
			FilePriority(rename), FilePriority(modified))
	}
}

func TestFilePriority_TestFile(t *testing.T) {
	test := FileDiff{
		NewPath: "pkg/handler_test.go",
		Hunks:   []Hunk{{Lines: []DiffLine{{Type: LineAdded}, {Type: LineAdded}}}},
	}
	prod := FileDiff{
		NewPath: "pkg/handler.go",
		Hunks:   []Hunk{{Lines: []DiffLine{{Type: LineAdded}, {Type: LineAdded}}}},
	}

	if FilePriority(test) >= FilePriority(prod) {
		t.Errorf("test file (%d) should score lower than prod file (%d)",
			FilePriority(test), FilePriority(prod))
	}
}

func TestSortByPriority(t *testing.T) {
	diffs := []FileDiff{
		{NewPath: "README.md"},                               // low priority
		{NewPath: "internal/auth/handler.go", IsNew: true,    // high priority
			Hunks: []Hunk{{Lines: []DiffLine{{Type: LineAdded}, {Type: LineAdded}}}}},
		{NewPath: "vendor/lib.go"},                            // generated = very low
		{NewPath: "pkg/server.go",
			Hunks: []Hunk{{Lines: []DiffLine{{Type: LineAdded}}}}}, // medium
	}

	SortByPriority(diffs)

	// Auth file should be first.
	if diffs[0].NewPath != "internal/auth/handler.go" {
		t.Errorf("expected auth file first, got %q", diffs[0].NewPath)
	}
	// Vendor file should be last.
	if diffs[len(diffs)-1].NewPath != "vendor/lib.go" {
		t.Errorf("expected vendor file last, got %q", diffs[len(diffs)-1].NewPath)
	}
}
