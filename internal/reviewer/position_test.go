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
@@ -1,2 +1,2 @@
 ctx
-old
+new
@@ -40,2 +40,3 @@
 ctx
+later
 ctx
`,
			path: "conf.yaml",
			want: 2,
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
