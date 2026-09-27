package reviewer

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// ToolResult holds the output of a tool invocation.
type ToolResult struct {
	Tool   string `json:"tool"`
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

// ToolRegistry holds available agent tools scoped to a repo root.
type ToolRegistry struct {
	repoRoot string
}

// NewToolRegistry creates a tool registry scoped to the given repo root.
func NewToolRegistry(repoRoot string) *ToolRegistry {
	return &ToolRegistry{repoRoot: repoRoot}
}

// Execute runs a tool by name with the given arguments.
func (r *ToolRegistry) Execute(ctx context.Context, tool string, args map[string]any) ToolResult {
	switch tool {
	case "read_file":
		return r.readFile(ctx, args)
	case "find_definition":
		return r.findDefinition(ctx, args)
	case "list_directory":
		return r.listDirectory(ctx, args)
	default:
		return ToolResult{Tool: tool, Error: fmt.Sprintf("unknown tool: %s", tool)}
	}
}

// ToolSchemas returns the tool descriptions for the model prompt.
func ToolSchemas() string {
	return `Available tools (respond with JSON {"action": "tool", "tool": "<name>", "args": {<args>}}):

1. read_file: Read file contents from the repository.
   Args: {"path": "<relative path>", "start_line": <optional int>, "end_line": <optional int>}
   Max 32 KiB returned. Lines are 1-indexed.

2. find_definition: Find where a symbol is defined in the codebase.
   Args: {"symbol": "<symbol name>"}
   Returns up to 10 matches with surrounding context.

3. list_directory: List files in a directory.
   Args: {"path": "<relative path>"}
   Returns up to 200 entries.`
}

func (r *ToolRegistry) resolvePath(relPath string) (string, error) {
	absRepoRoot, err := filepath.Abs(r.repoRoot)
	if err != nil {
		return "", err
	}
	absRepoRoot, err = filepath.EvalSymlinks(absRepoRoot)
	if err != nil {
		return "", err
	}

	targetPath := filepath.Join(absRepoRoot, filepath.Clean(filepath.FromSlash(relPath)))

	resolved, err := filepath.EvalSymlinks(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			// If file doesn't exist, check base path
			if !strings.HasPrefix(targetPath, absRepoRoot+string(filepath.Separator)) && targetPath != absRepoRoot {
				return "", fmt.Errorf("path escapes repository root")
			}
			return targetPath, nil
		}
		return "", err
	}

	if !strings.HasPrefix(resolved, absRepoRoot+string(filepath.Separator)) && resolved != absRepoRoot {
		return "", fmt.Errorf("path escapes repository root")
	}

	return resolved, nil
}

func (r *ToolRegistry) readFile(ctx context.Context, args map[string]any) ToolResult {
	path, ok := args["path"].(string)
	if !ok {
		return ToolResult{Tool: "read_file", Error: "missing or invalid 'path' argument"}
	}

	resolved, err := r.resolvePath(path)
	if err != nil {
		return ToolResult{Tool: "read_file", Error: err.Error()}
	}

	content, err := os.ReadFile(resolved)
	if err != nil {
		return ToolResult{Tool: "read_file", Error: fmt.Sprintf("failed to read file: %v", err)}
	}

	if len(content) > 32768 {
		content = content[:32768]
	}

	lines := strings.Split(string(content), "\n")

	startLine := 1
	if sl, ok := args["start_line"].(float64); ok {
		startLine = int(sl)
	} else if sl, ok := args["start_line"].(int); ok {
		startLine = sl
	}

	endLine := len(lines)
	if el, ok := args["end_line"].(float64); ok {
		endLine = int(el)
	} else if el, ok := args["end_line"].(int); ok {
		endLine = el
	}

	if startLine < 1 {
		startLine = 1
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	if startLine > endLine {
		return ToolResult{Tool: "read_file", Error: "start_line is greater than end_line"}
	}

	selected := lines[startLine-1 : endLine]
	return ToolResult{Tool: "read_file", Output: strings.Join(selected, "\n")}
}

func isDefinition(line string, symbol string) bool {
	line = strings.TrimSpace(line)
	patterns := []string{
		"func " + symbol,
		"type " + symbol,
		"var " + symbol,
		"const " + symbol,
		"class " + symbol,
		"def " + symbol,
		"function " + symbol,
	}
	for _, p := range patterns {
		if strings.HasPrefix(line, p) || strings.Contains(line, " "+p) {
			return true
		}
	}
	if strings.HasPrefix(line, "func (") && strings.Contains(line, ") "+symbol) {
		return true
	}
	return false
}

func (r *ToolRegistry) findDefinition(ctx context.Context, args map[string]any) ToolResult {
	symbol, ok := args["symbol"].(string)
	if !ok {
		return ToolResult{Tool: "find_definition", Error: "missing or invalid 'symbol' argument"}
	}

	var matches []string
	filesScanned := 0

	absRoot, err := filepath.Abs(r.repoRoot)
	if err != nil {
		return ToolResult{Tool: "find_definition", Error: err.Error()}
	}
	absRoot, err = filepath.EvalSymlinks(absRoot)
	if err != nil {
		return ToolResult{Tool: "find_definition", Error: err.Error()}
	}

	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if filesScanned >= 5000 {
			return filepath.SkipAll
		}

		if len(matches) >= 10 {
			return filepath.SkipAll
		}

		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(path)
		switch ext {
		case ".go", ".py", ".js", ".ts", ".java":
		default:
			return nil
		}

		filesScanned++
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		if !utf8.Valid(content) {
			return nil
		}

		relPath, _ := filepath.Rel(absRoot, path)
		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			if isDefinition(line, symbol) {
				matches = append(matches, fmt.Sprintf("%s:%d: %s", relPath, i+1, strings.TrimSpace(line)))
				if len(matches) >= 10 {
					break
				}
			}
		}

		return nil
	})

	if err != nil {
		return ToolResult{Tool: "find_definition", Error: fmt.Sprintf("error walking directory: %v", err)}
	}

	if len(matches) == 0 {
		return ToolResult{Tool: "find_definition", Output: "No definitions found."}
	}

	return ToolResult{Tool: "find_definition", Output: strings.Join(matches, "\n")}
}

func (r *ToolRegistry) listDirectory(ctx context.Context, args map[string]any) ToolResult {
	path, ok := args["path"].(string)
	if !ok {
		return ToolResult{Tool: "list_directory", Error: "missing or invalid 'path' argument"}
	}

	resolved, err := r.resolvePath(path)
	if err != nil {
		return ToolResult{Tool: "list_directory", Error: err.Error()}
	}

	entries, err := os.ReadDir(resolved)
	if err != nil {
		return ToolResult{Tool: "list_directory", Error: fmt.Sprintf("failed to read directory: %v", err)}
	}

	var output []string
	for i, entry := range entries {
		if i >= 200 {
			output = append(output, "... (truncated after 200 entries)")
			break
		}
		indicator := ""
		if entry.IsDir() {
			indicator = "/"
		}
		output = append(output, entry.Name()+indicator)
	}

	return ToolResult{Tool: "list_directory", Output: strings.Join(output, "\n")}
}
