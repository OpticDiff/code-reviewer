package reviewer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFile_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	reg := NewToolRegistry(tmp)
	err := os.WriteFile(filepath.Join(tmp, "test.txt"), []byte("hello world"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	res := reg.Execute(context.Background(), "read_file", map[string]any{"path": "test.txt"})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if res.Output != "hello world" {
		t.Errorf("got %q, want 'hello world'", res.Output)
	}
}

func TestReadFile_PathTraversal(t *testing.T) {
	tmp := t.TempDir()
	reg := NewToolRegistry(tmp)
	
	res := reg.Execute(context.Background(), "read_file", map[string]any{"path": "../../../etc/passwd"})
	if res.Error == "" || !strings.Contains(res.Error, "escapes repository root") {
		t.Errorf("expected path traversal error, got %v", res.Error)
	}
}

func TestReadFile_SymlinkTraversal(t *testing.T) {
	tmp := t.TempDir()
	reg := NewToolRegistry(tmp)
	
	targetFile := filepath.Join(t.TempDir(), "secret.txt")
	err := os.WriteFile(targetFile, []byte("secret"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	
	symlinkPath := filepath.Join(tmp, "link.txt")
	err = os.Symlink(targetFile, symlinkPath)
	if err != nil {
		t.Fatal(err)
	}
	
	res := reg.Execute(context.Background(), "read_file", map[string]any{"path": "link.txt"})
	if res.Error == "" || !strings.Contains(res.Error, "escapes repository root") {
		t.Errorf("expected path traversal error from symlink, got %v", res.Error)
	}
}

func TestReadFile_BoundedRead(t *testing.T) {
	tmp := t.TempDir()
	reg := NewToolRegistry(tmp)
	
	content := strings.Repeat("A", 40000)
	err := os.WriteFile(filepath.Join(tmp, "large.txt"), []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}
	
	res := reg.Execute(context.Background(), "read_file", map[string]any{"path": "large.txt"})
	if res.Error != "" {
		t.Fatal(res.Error)
	}
	if len(res.Output) > 32768 {
		t.Errorf("output length %d exceeds max 32768", len(res.Output))
	}
}

func TestReadFile_LineSlicing(t *testing.T) {
	tmp := t.TempDir()
	reg := NewToolRegistry(tmp)
	
	content := "line1\nline2\nline3\nline4\nline5"
	err := os.WriteFile(filepath.Join(tmp, "lines.txt"), []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}
	
	res := reg.Execute(context.Background(), "read_file", map[string]any{
		"path": "lines.txt",
		"start_line": float64(2),
		"end_line": float64(4),
	})
	if res.Error != "" {
		t.Fatal(res.Error)
	}
	expected := "line2\nline3\nline4"
	if res.Output != expected {
		t.Errorf("got %q, want %q", res.Output, expected)
	}
}

func TestReadFile_MissingFile(t *testing.T) {
	tmp := t.TempDir()
	reg := NewToolRegistry(tmp)
	
	res := reg.Execute(context.Background(), "read_file", map[string]any{"path": "missing.txt"})
	if res.Error == "" || !strings.Contains(res.Error, "failed to read file") && !strings.Contains(res.Error, "no such file or directory") {
		t.Errorf("expected missing file error, got %v", res.Error)
	}
}

func TestFindDefinition_GoFunc(t *testing.T) {
	tmp := t.TempDir()
	reg := NewToolRegistry(tmp)
	
	err := os.WriteFile(filepath.Join(tmp, "main.go"), []byte("package main\n\nfunc HelloWorld() {}\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	
	res := reg.Execute(context.Background(), "find_definition", map[string]any{"symbol": "HelloWorld"})
	if res.Error != "" {
		t.Fatal(res.Error)
	}
	if !strings.Contains(res.Output, "func HelloWorld()") {
		t.Errorf("expected to find func HelloWorld(), got %v", res.Output)
	}
}

func TestFindDefinition_MaxMatches(t *testing.T) {
	tmp := t.TempDir()
	reg := NewToolRegistry(tmp)
	
	var sb strings.Builder
	for i := 0; i < 15; i++ {
		sb.WriteString("func Common() {}\n")
	}
	err := os.WriteFile(filepath.Join(tmp, "common.go"), []byte(sb.String()), 0644)
	if err != nil {
		t.Fatal(err)
	}
	
	res := reg.Execute(context.Background(), "find_definition", map[string]any{"symbol": "Common"})
	if res.Error != "" {
		t.Fatal(res.Error)
	}
	matches := strings.Split(strings.TrimSpace(res.Output), "\n")
	if len(matches) != 10 {
		t.Errorf("expected 10 matches, got %d", len(matches))
	}
}

func TestFindDefinition_SkipsVendor(t *testing.T) {
	tmp := t.TempDir()
	reg := NewToolRegistry(tmp)
	
	vendorDir := filepath.Join(tmp, "vendor")
	err := os.MkdirAll(vendorDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	
	err = os.WriteFile(filepath.Join(vendorDir, "main.go"), []byte("func VendorFunc() {}\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	
	res := reg.Execute(context.Background(), "find_definition", map[string]any{"symbol": "VendorFunc"})
	if res.Error != "" {
		t.Fatal(res.Error)
	}
	if !strings.Contains(res.Output, "No definitions found") {
		t.Errorf("expected no definitions found, got %v", res.Output)
	}
}

func TestListDirectory_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	reg := NewToolRegistry(tmp)
	
	err := os.Mkdir(filepath.Join(tmp, "subdir"), 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("a"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	
	res := reg.Execute(context.Background(), "list_directory", map[string]any{"path": "."})
	if res.Error != "" {
		t.Fatal(res.Error)
	}
	if !strings.Contains(res.Output, "subdir/") {
		t.Errorf("expected subdir/ in output, got %v", res.Output)
	}
	if !strings.Contains(res.Output, "a.txt") {
		t.Errorf("expected a.txt in output, got %v", res.Output)
	}
}

func TestListDirectory_MaxEntries(t *testing.T) {
	tmp := t.TempDir()
	reg := NewToolRegistry(tmp)
	
	for i := 0; i < 205; i++ {
		// Use fmt.Sprintf for file names to avoid weird characters
		// or just use integer string
		name := "file_" + string(rune('a'+(i%26))) + string(rune('A'+(i/26))) + ".txt"
		err := os.WriteFile(filepath.Join(tmp, name), []byte(""), 0644)
		if err != nil {
			t.Fatal(err)
		}
	}
	
	res := reg.Execute(context.Background(), "list_directory", map[string]any{"path": "."})
	if res.Error != "" {
		t.Fatal(res.Error)
	}
	
	lines := strings.Split(strings.TrimSpace(res.Output), "\n")
	if len(lines) != 201 { // 200 items + 1 truncation message
		t.Errorf("expected 201 lines, got %d", len(lines))
	}
	if !strings.Contains(res.Output, "truncated after 200 entries") {
		t.Errorf("expected truncation message, got %v", res.Output)
	}
}

func TestExecute_UnknownTool(t *testing.T) {
	tmp := t.TempDir()
	reg := NewToolRegistry(tmp)
	
	res := reg.Execute(context.Background(), "unknown_tool", map[string]any{})
	if res.Error == "" || !strings.Contains(res.Error, "unknown tool") {
		t.Errorf("expected unknown tool error, got %v", res.Error)
	}
}
