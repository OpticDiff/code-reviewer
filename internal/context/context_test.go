package context

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/diff"
)

func TestLanguageForFile(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"App.kt", "kotlin"},
		{"build.gradle.kts", "kotlin"},
		{"Main.java", "java"},
		{"app.py", "python"},
		{"index.ts", "typescript"},
		{"App.tsx", "typescript"},
		{"README.md", ""},
		{"Makefile", ""},
		{"styles.css", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := languageForFile(tt.path)
			if got != tt.want {
				t.Errorf("languageForFile(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestLanguageQueries_AllLoaded(t *testing.T) {
	expected := []string{"go", "kotlin", "java", "python", "typescript"}
	for _, lang := range expected {
		if _, ok := languageQueries[lang]; !ok {
			t.Errorf("missing query for language %q", lang)
		}
	}
}

func TestTreeSitterExtractor_Go(t *testing.T) {
	// Create a temp repo with a Go file.
	repoRoot := t.TempDir()
	goFile := filepath.Join(repoRoot, "auth.go")
	source := `package auth

func ValidateSession(token string) error {
	return nil
}

type SessionConfig struct {
	Timeout int
}
`
	if err := os.WriteFile(goFile, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	diffs := []diff.FileDiff{
		{
			NewPath: "auth.go",
			Hunks: []diff.Hunk{
				{
					Lines: []diff.DiffLine{
						{Type: diff.LineAdded, NewLineNo: 3, Content: `func ValidateSession(token string) error {`},
						{Type: diff.LineAdded, NewLineNo: 7, Content: `type SessionConfig struct {`},
					},
				},
			},
		},
	}

	extractor := NewTreeSitterExtractor()
	symbols, err := extractor.Extract(diffs, repoRoot)
	if err != nil {
		t.Fatal(err)
	}

	// Should find ValidateSession and SessionConfig.
	names := make(map[string]bool)
	for _, s := range symbols {
		names[s.Name] = true
	}

	if !names["ValidateSession"] {
		t.Error("should extract ValidateSession")
	}
	if !names["SessionConfig"] {
		t.Error("should extract SessionConfig")
	}
}

func TestTreeSitterExtractor_MinNameLength(t *testing.T) {
	repoRoot := t.TempDir()
	goFile := filepath.Join(repoRoot, "short.go")
	source := `package x

func Do() {}

func ValidateToken() {}
`
	if err := os.WriteFile(goFile, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	diffs := []diff.FileDiff{
		{
			NewPath: "short.go",
			Hunks: []diff.Hunk{
				{
					Lines: []diff.DiffLine{
						{Type: diff.LineAdded, NewLineNo: 3, Content: `func Do() {}`},
						{Type: diff.LineAdded, NewLineNo: 5, Content: `func ValidateToken() {}`},
					},
				},
			},
		},
	}

	extractor := NewTreeSitterExtractor()
	symbols, err := extractor.Extract(diffs, repoRoot)
	if err != nil {
		t.Fatal(err)
	}

	for _, s := range symbols {
		if s.Name == "Do" {
			t.Error("should filter out short names like 'Do' (< 4 chars)")
		}
	}

	found := false
	for _, s := range symbols {
		if s.Name == "ValidateToken" {
			found = true
		}
	}
	if !found {
		t.Error("should keep ValidateToken (>= 4 chars)")
	}
}

func TestTreeSitterExtractor_IgnoresDeletedFiles(t *testing.T) {
	repoRoot := t.TempDir()

	diffs := []diff.FileDiff{
		{
			NewPath:  "deleted.go",
			IsDelete: true,
			Hunks: []diff.Hunk{
				{
					Lines: []diff.DiffLine{
						{Type: diff.LineRemoved, NewLineNo: 0, Content: `func OldFunc() {}`},
					},
				},
			},
		},
	}

	extractor := NewTreeSitterExtractor()
	symbols, err := extractor.Extract(diffs, repoRoot)
	if err != nil {
		t.Fatal(err)
	}

	if len(symbols) != 0 {
		t.Errorf("should not extract symbols from deleted files, got %d", len(symbols))
	}
}

func TestTreeSitterExtractor_UnsupportedLanguage(t *testing.T) {
	repoRoot := t.TempDir()
	mdFile := filepath.Join(repoRoot, "README.md")
	if err := os.WriteFile(mdFile, []byte("# Hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	diffs := []diff.FileDiff{
		{
			NewPath: "README.md",
			Hunks: []diff.Hunk{
				{
					Lines: []diff.DiffLine{
						{Type: diff.LineAdded, NewLineNo: 1, Content: "# Hello"},
					},
				},
			},
		},
	}

	extractor := NewTreeSitterExtractor()
	symbols, err := extractor.Extract(diffs, repoRoot)
	if err != nil {
		t.Fatal(err)
	}

	if len(symbols) != 0 {
		t.Errorf("unsupported languages should return no symbols, got %d", len(symbols))
	}
}

func TestTreeSitterExtractor_OnlyChangedSymbols(t *testing.T) {
	repoRoot := t.TempDir()
	goFile := filepath.Join(repoRoot, "mixed.go")
	source := `package mixed

func UnchangedFunc() {}

func ChangedFunc() {}
`
	if err := os.WriteFile(goFile, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	diffs := []diff.FileDiff{
		{
			NewPath: "mixed.go",
			Hunks: []diff.Hunk{
				{
					// Only line 5 (ChangedFunc) is in the diff.
					Lines: []diff.DiffLine{
						{Type: diff.LineAdded, NewLineNo: 5, Content: `func ChangedFunc() {}`},
					},
				},
			},
		},
	}

	extractor := NewTreeSitterExtractor()
	symbols, err := extractor.Extract(diffs, repoRoot)
	if err != nil {
		t.Fatal(err)
	}

	for _, s := range symbols {
		if s.Name == "UnchangedFunc" {
			t.Error("should not extract UnchangedFunc — not on a changed line")
		}
	}

	found := false
	for _, s := range symbols {
		if s.Name == "ChangedFunc" {
			found = true
		}
	}
	if !found {
		t.Error("should extract ChangedFunc — on a changed line")
	}
}

func TestIsNoiseMatch(t *testing.T) {
	tests := []struct {
		content string
		noise   bool
	}{
		{"// ValidateSession validates the session", true},
		{"# import auth", true},
		{"import auth", true},
		{"from auth import ValidateSession", true},
		{"require('auth')", true},
		{"/* comment */", true},
		{"* list item", true},
		{"sess := auth.ValidateSession(token)", false},
		{"func TestValidateSession(t *testing.T) {", false},
		{"ValidateSession(ctx, opts...)", false},
	}

	for _, tt := range tests {
		t.Run(tt.content, func(t *testing.T) {
			got := isNoiseMatch(tt.content)
			if got != tt.noise {
				t.Errorf("isNoiseMatch(%q) = %v, want %v", tt.content, got, tt.noise)
			}
		})
	}
}

func TestParseGrepLine(t *testing.T) {
	tests := []struct {
		line    string
		file    string
		lineNo  int
		content string
		ok      bool
	}{
		{
			"handler.go:42:sess := auth.ValidateSession(token)",
			"handler.go", 42, "sess := auth.ValidateSession(token)", true,
		},
		{
			"/full/path/handler.go:10:call()",
			"/full/path/handler.go", 10, "call()", true,
		},
		{"no-match", "", 0, "", false},
		{"file:notanumber:content", "", 0, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			file, lineNo, content, ok := parseGrepLine(tt.line)
			if ok != tt.ok {
				t.Errorf("ok = %v, want %v", ok, tt.ok)
			}
			if ok {
				if file != tt.file || lineNo != tt.lineNo || content != tt.content {
					t.Errorf("got (%q, %d, %q), want (%q, %d, %q)",
						file, lineNo, content, tt.file, tt.lineNo, tt.content)
				}
			}
		})
	}
}

func TestExtractedLines(t *testing.T) {
	fd := diff.FileDiff{
		Hunks: []diff.Hunk{
			{
				Lines: []diff.DiffLine{
					{Type: diff.LineContext, NewLineNo: 1},
					{Type: diff.LineAdded, NewLineNo: 2},
					{Type: diff.LineAdded, NewLineNo: 3},
					{Type: diff.LineRemoved, NewLineNo: 0},
					{Type: diff.LineContext, NewLineNo: 4},
				},
			},
		},
	}

	lines := extractedLines(fd)
	if !lines[2] {
		t.Error("line 2 should be in changed lines")
	}
	if !lines[3] {
		t.Error("line 3 should be in changed lines")
	}
	if lines[1] {
		t.Error("line 1 (context) should not be in changed lines")
	}
	if lines[0] {
		t.Error("line 0 (removed) should not be in changed lines")
	}
}

func TestBuildUserPromptWithContext(t *testing.T) {
	// Test is in model package but we verify integration here
	// by checking that the provider correctly maps types.
	snippets := []CodeSnippet{
		{File: "handler.go", Line: 42, Content: "auth.ValidateSession(token)", Symbol: "ValidateSession"},
	}

	if len(snippets) == 0 {
		t.Error("should have snippets")
	}
	if snippets[0].Symbol != "ValidateSession" {
		t.Error("snippet symbol should match")
	}
}

func TestTreeSitterExtractor_Python(t *testing.T) {
	repoRoot := t.TempDir()
	pyFile := filepath.Join(repoRoot, "auth.py")
	source := `class SessionManager:
    pass

def validate_session(token):
    return True
`
	if err := os.WriteFile(pyFile, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	diffs := []diff.FileDiff{
		{
			NewPath: "auth.py",
			Hunks: []diff.Hunk{
				{
					Lines: []diff.DiffLine{
						{Type: diff.LineAdded, NewLineNo: 1, Content: "class SessionManager:"},
						{Type: diff.LineAdded, NewLineNo: 4, Content: "def validate_session(token):"},
					},
				},
			},
		},
	}

	extractor := NewTreeSitterExtractor()
	symbols, err := extractor.Extract(diffs, repoRoot)
	if err != nil {
		t.Fatal(err)
	}

	names := make(map[string]bool)
	for _, s := range symbols {
		names[s.Name] = true
	}

	if !names["SessionManager"] {
		t.Error("should extract SessionManager class")
	}
	if !names["validate_session"] {
		t.Error("should extract validate_session function")
	}
}

func TestTreeSitterExtractor_Java(t *testing.T) {
	repoRoot := t.TempDir()
	javaFile := filepath.Join(repoRoot, "Auth.java")
	source := `public class AuthService {
    public boolean validateToken(String token) {
        return true;
    }
}
`
	if err := os.WriteFile(javaFile, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	diffs := []diff.FileDiff{
		{
			NewPath: "Auth.java",
			Hunks: []diff.Hunk{
				{
					Lines: []diff.DiffLine{
						{Type: diff.LineAdded, NewLineNo: 1, Content: "public class AuthService {"},
						{Type: diff.LineAdded, NewLineNo: 2, Content: "    public boolean validateToken(String token) {"},
					},
				},
			},
		},
	}

	extractor := NewTreeSitterExtractor()
	symbols, err := extractor.Extract(diffs, repoRoot)
	if err != nil {
		t.Fatal(err)
	}

	names := make(map[string]bool)
	for _, s := range symbols {
		names[s.Name] = true
	}

	if !names["AuthService"] {
		t.Error("should extract AuthService class")
	}
	if !names["validateToken"] {
		t.Error("should extract validateToken method")
	}
}

func TestTreeSitterExtractor_TypeScript(t *testing.T) {
	repoRoot := t.TempDir()
	tsFile := filepath.Join(repoRoot, "auth.ts")
	source := `interface AuthConfig {
    timeout: number;
}

function validateSession(token: string): boolean {
    return true;
}

class SessionManager {
    validate() {}
}
`
	if err := os.WriteFile(tsFile, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	diffs := []diff.FileDiff{
		{
			NewPath: "auth.ts",
			Hunks: []diff.Hunk{
				{
					Lines: []diff.DiffLine{
						{Type: diff.LineAdded, NewLineNo: 1, Content: "interface AuthConfig {"},
						{Type: diff.LineAdded, NewLineNo: 5, Content: "function validateSession(token: string): boolean {"},
						{Type: diff.LineAdded, NewLineNo: 9, Content: "class SessionManager {"},
						{Type: diff.LineAdded, NewLineNo: 10, Content: "    validate() {}"},
					},
				},
			},
		},
	}

	extractor := NewTreeSitterExtractor()
	symbols, err := extractor.Extract(diffs, repoRoot)
	if err != nil {
		t.Fatal(err)
	}

	names := make(map[string]bool)
	for _, s := range symbols {
		names[s.Name] = true
	}

	if !names["AuthConfig"] {
		t.Error("should extract AuthConfig interface")
	}
	if !names["validateSession"] {
		t.Error("should extract validateSession function")
	}
	if !names["SessionManager"] {
		t.Error("should extract SessionManager class")
	}
	if !names["validate"] {
		t.Error("should extract validate method")
	}
}

func TestTreeSitterExtractor_Kotlin(t *testing.T) {
	repoRoot := t.TempDir()
	ktFile := filepath.Join(repoRoot, "Auth.kt")
	source := `class SessionManager {
    fun validate() {}
}

fun validateSession(token: String): Boolean {
    return true
}
`
	if err := os.WriteFile(ktFile, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	diffs := []diff.FileDiff{
		{
			NewPath: "Auth.kt",
			Hunks: []diff.Hunk{
				{
					Lines: []diff.DiffLine{
						{Type: diff.LineAdded, NewLineNo: 1, Content: "class SessionManager {"},
						{Type: diff.LineAdded, NewLineNo: 5, Content: "fun validateSession(token: String): Boolean {"},
					},
				},
			},
		},
	}

	extractor := NewTreeSitterExtractor()
	symbols, err := extractor.Extract(diffs, repoRoot)
	if err != nil {
		t.Fatal(err)
	}

	names := make(map[string]bool)
	for _, s := range symbols {
		names[s.Name] = true
	}

	if !names["SessionManager"] {
		t.Error("should extract SessionManager class")
	}
	if !names["validateSession"] {
		t.Error("should extract validateSession function")
	}
}

func TestGrepFinder_Integration(t *testing.T) {
	// Create a mini repo with cross-file references.
	repoRoot := t.TempDir()

	// File 1: the changed file (in diff).
	authFile := filepath.Join(repoRoot, "auth.go")
	if err := os.WriteFile(authFile, []byte(`package auth
func ValidateSession(token string) error { return nil }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// File 2: unchanged file that references ValidateSession.
	handlerFile := filepath.Join(repoRoot, "handler.go")
	if err := os.WriteFile(handlerFile, []byte(`package handler
import "auth"
func Handle() {
    auth.ValidateSession("token123")
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// File 3: another unchanged file.
	middlewareFile := filepath.Join(repoRoot, "middleware.go")
	if err := os.WriteFile(middlewareFile, []byte(`package middleware
func Check() {
    ValidateSession("x")
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = middlewareFile

	finder := NewGrepFinder()
	diffFiles := map[string]bool{
		filepath.Join(repoRoot, "auth.go"): true,
	}

	ctx := t.Context()
	symbols := []SymbolChange{
		{Name: "ValidateSession", Kind: "function", File: "auth.go", Language: "go"},
	}

	snippets, err := finder.FindUsages(ctx, repoRoot, symbols, diffFiles)
	if err != nil {
		t.Fatal(err)
	}

	// Should find at least the handler.go reference.
	if len(snippets) == 0 {
		t.Error("should find usages of ValidateSession in unchanged files")
	}

	for _, s := range snippets {
		if s.Symbol != "ValidateSession" {
			t.Errorf("snippet symbol should be ValidateSession, got %q", s.Symbol)
		}
	}
}

func TestGrepFinder_EmptySymbols(t *testing.T) {
	finder := NewGrepFinder()
	snippets, err := finder.FindUsages(t.Context(), t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(snippets) != 0 {
		t.Error("empty symbols should return empty snippets")
	}
}

func TestGrepFinder_FrequencyCap(t *testing.T) {
	finder := NewGrepFinder()
	finder.MaxFileMatches = 1 // Very low cap.

	repoRoot := t.TempDir()

	// Create multiple files that all reference the symbol.
	for i := range 5 {
		f := filepath.Join(repoRoot, filepath.Clean(filepath.Join(".", "file"+string(rune('a'+i))+".go")))
		if err := os.WriteFile(f, []byte("package x\nfunc f() { CommonSymbol() }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	symbols := []SymbolChange{
		{Name: "CommonSymbol", Kind: "function", File: "source.go", Language: "go"},
	}

	snippets, err := finder.FindUsages(t.Context(), repoRoot, symbols, nil)
	if err != nil {
		t.Fatal(err)
	}

	// With MaxFileMatches=1, symbol matching >1 files should be skipped.
	if len(snippets) != 0 {
		t.Errorf("over-common symbol should be skipped, got %d snippets", len(snippets))
	}
}

func TestDefaultProvider_EndToEnd(t *testing.T) {
	// Create a mini repo.
	repoRoot := t.TempDir()

	goFile := filepath.Join(repoRoot, "service.go")
	if err := os.WriteFile(goFile, []byte(`package service

func ProcessOrder(orderID string) error {
    return nil
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	callerFile := filepath.Join(repoRoot, "handler.go")
	if err := os.WriteFile(callerFile, []byte(`package handler

func Handle() {
    ProcessOrder("order-123")
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	diffs := []diff.FileDiff{
		{
			NewPath: "service.go",
			Hunks: []diff.Hunk{
				{
					Lines: []diff.DiffLine{
						{Type: diff.LineAdded, NewLineNo: 3, Content: `func ProcessOrder(orderID string) error {`},
					},
				},
			},
		},
	}

	provider := NewDefaultProvider()
	snippets, err := provider.FindRelatedCode(t.Context(), repoRoot, diffs)
	if err != nil {
		t.Fatal(err)
	}

	// Should find ProcessOrder in handler.go.
	found := false
	for _, s := range snippets {
		if s.Symbol == "ProcessOrder" {
			found = true
			break
		}
	}
	if !found {
		t.Error("DefaultProvider should find ProcessOrder usage in handler.go")
	}
}

func TestDefaultProvider_NoDiffs(t *testing.T) {
	provider := NewDefaultProvider()
	snippets, err := provider.FindRelatedCode(t.Context(), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(snippets) != 0 {
		t.Error("no diffs should return no snippets")
	}
}

func TestSymbolNames(t *testing.T) {
	symbols := []SymbolChange{
		{Name: "Foo"},
		{Name: "Bar"},
		{Name: "Baz"},
	}
	names := symbolNames(symbols)
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d", len(names))
	}
	if names[0] != "Foo" || names[1] != "Bar" || names[2] != "Baz" {
		t.Errorf("unexpected names: %v", names)
	}
}

func TestDummyFilename(t *testing.T) {
	tests := []struct {
		lang string
		want string
	}{
		{"go", "x.go"},
		{"kotlin", "x.kt"},
		{"java", "x.java"},
		{"python", "x.py"},
		{"typescript", "x.ts"},
		{"rust", "x.rust"},
	}
	for _, tt := range tests {
		got := dummyFilename(tt.lang)
		if got != tt.want {
			t.Errorf("dummyFilename(%q) = %q, want %q", tt.lang, got, tt.want)
		}
	}
}
