package reviewer

import (
	"strings"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

func mustParseDiff(t *testing.T, raw string) []diff.FileDiff {
	t.Helper()
	diffs, err := diff.Parse(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parsing diff: %v", err)
	}
	return diffs
}

// A hunk whose first line is unchanged context, followed by additions and a
// removal, so old line 29 is new line 30.
const contextFirstDiff = `diff --git a/conf.yaml b/conf.yaml
index 1111111..2222222 100644
--- a/conf.yaml
+++ b/conf.yaml
@@ -27,4 +27,5 @@ section
 unchanged-a
+added-b
+added-c
-removed-d
 unchanged-e
 unchanged-f
`

func TestConfigAnchorLine(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		path string
		want int
	}{
		{
			name: "first added line, not the leading context line",
			raw:  contextFirstDiff,
			path: "conf.yaml",
			want: 28,
		},
		{
			name: "removal-only change falls back to first context line",
			raw: `diff --git a/conf.yaml b/conf.yaml
--- a/conf.yaml
+++ b/conf.yaml
@@ -10,3 +10,2 @@
 keep
-drop
 keep-too
`,
			path: "conf.yaml",
			want: 10,
		},
		{
			name: "deleted file has no new-side line and falls back to 1",
			raw: `diff --git a/conf.yaml b/conf.yaml
deleted file mode 100644
--- a/conf.yaml
+++ /dev/null
@@ -1,2 +0,0 @@
-one
-two
`,
			path: "conf.yaml",
			want: 1,
		},
		{
			name: "path not in diff falls back to 1",
			raw:  contextFirstDiff,
			path: "other.yaml",
			want: 1,
		},
		{
			name: "added line in a later hunk is preferred over earlier context",
			raw: `diff --git a/conf.yaml b/conf.yaml
--- a/conf.yaml
+++ b/conf.yaml
@@ -1,3 +1,2 @@
 ctx
-old
 ctx2
@@ -40,2 +39,3 @@
 ctx
+later
 ctx
`,
			path: "conf.yaml",
			want: 40,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diffs := mustParseDiff(t, tt.raw)
			if got := configAnchorLine(diffs, tt.path); got != tt.want {
				t.Errorf("configAnchorLine() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestConfigAnchorLine_IsValidatedFindingLine(t *testing.T) {
	diffs := mustParseDiff(t, contextFirstDiff)
	line := configAnchorLine(diffs, "conf.yaml")
	f := ConfigPoisoningFinding("conf.yaml", line)
	got := ValidateFindings([]model.Finding{f}, diffs)
	if len(got) != 1 || got[0].Line != 28 {
		t.Fatalf("expected the finding to survive validation on line 28, got %+v", got)
	}
}

func TestOldLineFor(t *testing.T) {
	diffs := mustParseDiff(t, contextFirstDiff)
	tests := []struct {
		name string
		path string
		line int
		want int
	}{
		{"unchanged line before the edits maps to itself", "conf.yaml", 27, 27},
		{"added line has no old counterpart", "conf.yaml", 28, 0},
		{"unchanged line after an addition and a removal is offset", "conf.yaml", 30, 29},
		{"line outside every hunk", "conf.yaml", 99, 0},
		{"file not in diff", "other.yaml", 27, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := oldLineFor(diffs, tt.path, tt.line); got != tt.want {
				t.Errorf("oldLineFor(%q, %d) = %d, want %d", tt.path, tt.line, got, tt.want)
			}
		})
	}
}

func TestOldLineFor_MultipleHunksWithDifferentOffsets(t *testing.T) {
	diffs := mustParseDiff(t, `diff --git a/f.go b/f.go
--- a/f.go
+++ b/f.go
@@ -1,3 +1,4 @@
 a
+b
 c
 d
@@ -20,3 +21,3 @@
 e
-f
+g
 h
`)
	tests := []struct {
		line int
		want int
	}{
		{1, 1},   // first hunk, before the addition
		{4, 3},   // first hunk, after one added line
		{21, 20}, // second hunk, shifted by the first hunk's addition
		{23, 22}, // second hunk, after the replaced line
		{22, 0},  // added line
	}
	for _, tt := range tests {
		if got := oldLineFor(diffs, "f.go", tt.line); got != tt.want {
			t.Errorf("oldLineFor(f.go, %d) = %d, want %d", tt.line, got, tt.want)
		}
	}
}

const renameDiff = `diff --git a/old.yaml b/new.yaml
similarity index 80%
rename from old.yaml
rename to new.yaml
--- a/old.yaml
+++ b/new.yaml
@@ -5,3 +5,4 @@
 keep
+added
 keep-too
 tail
`

func TestOldPathAndOldLineFor_Rename(t *testing.T) {
	diffs := mustParseDiff(t, renameDiff)
	if got := oldPathFor(diffs, "new.yaml"); got != "old.yaml" {
		t.Errorf("oldPathFor(new.yaml) = %q, want old.yaml", got)
	}
	if got := oldLineFor(diffs, "new.yaml", 5); got != 5 {
		t.Errorf("oldLineFor(new.yaml, 5) = %d, want 5", got)
	}
	if got := oldPathFor(mustParseDiff(t, contextFirstDiff), "conf.yaml"); got != "" {
		t.Errorf("oldPathFor for an unrenamed file = %q, want empty", got)
	}
}

func TestOldLineFor_PrefersNewPathOverOldPath(t *testing.T) {
	// a.txt is renamed to b.txt, and a new a.txt is added.
	diffs := mustParseDiff(t, `diff --git a/a.txt b/b.txt
similarity index 80%
rename from a.txt
rename to b.txt
--- a/a.txt
+++ b/b.txt
@@ -10,2 +11,3 @@
 x
+y
 z
diff --git a/a.txt b/a.txt
new file mode 100644
--- /dev/null
+++ b/a.txt
@@ -0,0 +1,2 @@
+one
+two
`)
	if got := oldLineFor(diffs, "a.txt", 1); got != 0 {
		t.Errorf("oldLineFor(a.txt, 1) = %d, want 0 (added line of the new file)", got)
	}
	if got := oldPathFor(diffs, "a.txt"); got != "" {
		t.Errorf("oldPathFor(a.txt) = %q, want empty", got)
	}
}
