// Package diff provides parsing, filtering, and chunking of unified diffs.
package diff

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// FileDiff represents the diff for a single file.
type FileDiff struct {
	OldPath string
	NewPath string
	Hunks   []Hunk
	IsBinary bool
	IsNew    bool
	IsDelete bool
	IsRename bool
}

// LineCount returns the total number of added+removed lines in this file diff.
func (fd *FileDiff) LineCount() int {
	count := 0
	for _, h := range fd.Hunks {
		for _, l := range h.Lines {
			if l.Type == LineAdded || l.Type == LineRemoved {
				count++
			}
		}
	}
	return count
}

// RawText returns the unified diff text for this file.
func (fd *FileDiff) RawText() string {
	var sb strings.Builder
	for _, h := range fd.Hunks {
		sb.WriteString(h.Header)
		sb.WriteByte('\n')
		for _, l := range h.Lines {
			switch l.Type {
			case LineAdded:
				sb.WriteByte('+')
			case LineRemoved:
				sb.WriteByte('-')
			case LineContext:
				sb.WriteByte(' ')
			}
			sb.WriteString(l.Content)
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// Hunk represents a contiguous section of changes within a file diff.
type Hunk struct {
	Header       string // e.g., @@ -10,5 +10,7 @@ func Foo()
	OldStart     int
	OldCount     int
	NewStart     int
	NewCount     int
	Lines        []DiffLine
}

// LineType indicates whether a diff line is added, removed, or context.
type LineType int

const (
	LineContext LineType = iota
	LineAdded
	LineRemoved
)

// DiffLine is a single line within a diff hunk.
type DiffLine struct {
	Type      LineType
	Content   string
	OldLineNo int // 0 if not applicable (added lines).
	NewLineNo int // 0 if not applicable (removed lines).
}

// Parse reads unified diff output and returns a slice of FileDiffs.
func Parse(r io.Reader) ([]FileDiff, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line.

	var diffs []FileDiff
	var current *FileDiff

	for scanner.Scan() {
		line := scanner.Text()

		// New file diff starts with "diff --git".
		if strings.HasPrefix(line, "diff --git ") {
			if current != nil {
				diffs = append(diffs, *current)
			}
			current = &FileDiff{}
			parts := strings.SplitN(line, " b/", 2)
			if len(parts) == 2 {
				current.NewPath = parts[1]
			}
			aParts := strings.SplitN(parts[0], " a/", 2)
			if len(aParts) == 2 {
				current.OldPath = aParts[1]
			}
			continue
		}

		if current == nil {
			continue
		}

		// Parse diff metadata lines.
		switch {
		case strings.HasPrefix(line, "Binary files"):
			current.IsBinary = true
		case strings.HasPrefix(line, "new file mode"):
			current.IsNew = true
		case strings.HasPrefix(line, "deleted file mode"):
			current.IsDelete = true
		case strings.HasPrefix(line, "rename from"):
			current.IsRename = true
		case strings.HasPrefix(line, "--- "):
			if current.OldPath == "" {
				path := strings.TrimPrefix(line, "--- a/")
				if path != "/dev/null" {
					current.OldPath = path
				}
			}
		case strings.HasPrefix(line, "+++ "):
			if current.NewPath == "" {
				path := strings.TrimPrefix(line, "+++ b/")
				if path != "/dev/null" {
					current.NewPath = path
				}
			}
		case strings.HasPrefix(line, "@@"):
			hunk, err := parseHunkHeader(line)
			if err != nil {
				return nil, fmt.Errorf("parsing hunk header %q: %w", line, err)
			}
			current.Hunks = append(current.Hunks, *hunk)
		case len(current.Hunks) > 0:
			h := &current.Hunks[len(current.Hunks)-1]
			dl := parseDiffLine(line, h)
			h.Lines = append(h.Lines, dl)
		}
	}

	if current != nil {
		diffs = append(diffs, *current)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning diff: %w", err)
	}

	return diffs, nil
}

// parseHunkHeader parses "@@ -old,count +new,count @@ optional context".
func parseHunkHeader(line string) (*Hunk, error) {
	h := &Hunk{Header: line}

	// Strip @@ markers.
	line = strings.TrimPrefix(line, "@@")
	idx := strings.Index(line[1:], "@@")
	if idx < 0 {
		return nil, fmt.Errorf("malformed hunk header")
	}
	ranges := strings.TrimSpace(line[:idx+1])

	parts := strings.Fields(ranges)
	if len(parts) < 2 {
		return nil, fmt.Errorf("expected 2 range specs, got %d", len(parts))
	}

	oldRange := strings.TrimPrefix(parts[0], "-")
	newRange := strings.TrimPrefix(parts[1], "+")

	var err error
	h.OldStart, h.OldCount, err = parseRange(oldRange)
	if err != nil {
		return nil, fmt.Errorf("old range: %w", err)
	}
	h.NewStart, h.NewCount, err = parseRange(newRange)
	if err != nil {
		return nil, fmt.Errorf("new range: %w", err)
	}

	return h, nil
}

func parseRange(s string) (start, count int, err error) {
	parts := strings.SplitN(s, ",", 2)
	start, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}
	if len(parts) == 2 {
		count, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, err
		}
	} else {
		count = 1
	}
	return start, count, nil
}

func parseDiffLine(line string, h *Hunk) DiffLine {
	dl := DiffLine{}

	// Track current position in hunk.
	oldLine := h.OldStart
	newLine := h.NewStart
	for _, existing := range h.Lines {
		switch existing.Type {
		case LineContext:
			oldLine++
			newLine++
		case LineAdded:
			newLine++
		case LineRemoved:
			oldLine++
		}
	}

	if len(line) == 0 {
		// Empty context line.
		dl.Type = LineContext
		dl.OldLineNo = oldLine
		dl.NewLineNo = newLine
		return dl
	}

	switch line[0] {
	case '+':
		dl.Type = LineAdded
		dl.Content = line[1:]
		dl.NewLineNo = newLine
	case '-':
		dl.Type = LineRemoved
		dl.Content = line[1:]
		dl.OldLineNo = oldLine
	default:
		dl.Type = LineContext
		if len(line) > 0 && line[0] == ' ' {
			dl.Content = line[1:]
		} else {
			dl.Content = line
		}
		dl.OldLineNo = oldLine
		dl.NewLineNo = newLine
	}

	return dl
}
