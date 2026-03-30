package diff

import (
	"testing"
)

func TestFilter(t *testing.T) {
	diffs := []FileDiff{
		{NewPath: "internal/handler.go"},
		{NewPath: "go.sum"},
		{NewPath: "vendor/lib/foo.go"},
		{NewPath: "package-lock.json"},
		{NewPath: "internal/service.go"},
		{NewPath: "image.png", IsBinary: true},
	}

	patterns := []string{"go.sum", "*.lock", "package-lock.json", "vendor/*"}
	result := Filter(diffs, patterns)

	if len(result) != 2 {
		t.Fatalf("expected 2 files after filter, got %d", len(result))
	}
	if result[0].NewPath != "internal/handler.go" {
		t.Errorf("expected handler.go, got %s", result[0].NewPath)
	}
	if result[1].NewPath != "internal/service.go" {
		t.Errorf("expected service.go, got %s", result[1].NewPath)
	}
}

func TestFilter_NoPatterns(t *testing.T) {
	diffs := []FileDiff{
		{NewPath: "foo.go"},
		{NewPath: "bar.go"},
	}
	result := Filter(diffs, nil)
	if len(result) != 2 {
		t.Errorf("expected 2 files with no patterns, got %d", len(result))
	}
}

func TestFilter_AllExcluded(t *testing.T) {
	diffs := []FileDiff{
		{NewPath: "go.sum"},
		{NewPath: "yarn.lock"},
	}
	result := Filter(diffs, []string{"go.sum", "*.lock"})
	if len(result) != 0 {
		t.Errorf("expected 0 files, got %d", len(result))
	}
}

func TestShouldExclude(t *testing.T) {
	tests := []struct {
		path     string
		patterns []string
		want     bool
	}{
		{"go.sum", []string{"go.sum"}, true},
		{"vendor/lib/foo.go", []string{"vendor/*"}, true},
		{"yarn.lock", []string{"*.lock"}, true},
		{"internal/foo.go", []string{"go.sum", "*.lock"}, false},
		{"node_modules/pkg/index.js", []string{"node_modules/*"}, true},
	}

	for _, tt := range tests {
		got := shouldExclude(tt.path, tt.patterns)
		if got != tt.want {
			t.Errorf("shouldExclude(%q, %v) = %v, want %v", tt.path, tt.patterns, got, tt.want)
		}
	}
}
