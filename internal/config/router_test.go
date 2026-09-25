package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestLoadRuleRouter(t *testing.T) {
	dir := t.TempDir()
	jsonContent := `{
		"default_rule": "default.md",
		"path_rule_map": {
			"**/*.go": "go.md"
		}
	}`
	err := os.WriteFile(filepath.Join(dir, "system_rules.json"), []byte(jsonContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	router, err := LoadRuleRouter(dir)
	if err != nil {
		t.Fatal(err)
	}

	if router.DefaultRule != "default.md" {
		t.Errorf("expected default.md, got %s", router.DefaultRule)
	}
}

func TestLoadRuleRouter_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "system_rules.json"), []byte("{invalid"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = LoadRuleRouter(dir)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadRuleRouter_MissingDir(t *testing.T) {
	_, err := LoadRuleRouter("/does/not/exist")
	if err == nil {
		t.Error("expected error for missing dir")
	}
}

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		pattern  string
		filePath string
		want     bool
	}{
		{"**/*.go", "main.go", true},
		{"**/*.go", "a/b/c/main.go", true},
		{"**/*.go", "main.txt", false},
		{"src/**/*.{go,proto}", "src/api.proto", true},
		{"src/**/*.{go,proto}", "src/main.go", true},
		{"src/**/*.{go,proto}", "src/main.txt", false},
		{"Dockerfile*", "Dockerfile", true},
		{"Dockerfile*", "Dockerfile.dev", true},
		{"Dockerfile*", "a/b/Dockerfile", true},
		{"Dockerfile*", "a/b/Dockerfile.dev", true},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.filePath, func(t *testing.T) {
			got := globMatch(tt.pattern, tt.filePath)
			if got != tt.want {
				t.Errorf("globMatch(%q, %q) = %v, want %v", tt.pattern, tt.filePath, got, tt.want)
			}
		})
	}
}

func TestRuleLoader_Load(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "rule.md"), []byte("content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	loader, err := NewRuleLoader(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()

	content, err := loader.Load("rule.md")
	if err != nil {
		t.Fatal(err)
	}
	if content != "content" {
		t.Errorf("expected 'content', got %q", content)
	}

	// Cache hit
	err = os.Remove(filepath.Join(dir, "rule.md"))
	if err != nil {
		t.Fatal(err)
	}

	content2, err := loader.Load("rule.md")
	if err != nil {
		t.Fatal(err)
	}
	if content2 != "content" {
		t.Errorf("expected 'content' from cache, got %q", content2)
	}
}

func TestRuleLoader_Load_MissingFile(t *testing.T) {
	dir := t.TempDir()
	loader, err := NewRuleLoader(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()

	_, err = loader.Load("missing.md")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestRuleLoader_BoundedRead(t *testing.T) {
	dir := t.TempDir()
	largeContent := make([]byte, (1<<20)+100) // 1 MiB + 100 bytes
	err := os.WriteFile(filepath.Join(dir, "large.md"), largeContent, 0644)
	if err != nil {
		t.Fatal(err)
	}

	loader, err := NewRuleLoader(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()

	content, err := loader.Load("large.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(content) != 1<<20 {
		t.Errorf("expected exactly 1 MiB, got %d bytes", len(content))
	}
}

func TestRuleLoader_ConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "rule.md"), []byte("content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	loader, err := NewRuleLoader(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = loader.Load("rule.md")
		}()
	}
	wg.Wait()
}
