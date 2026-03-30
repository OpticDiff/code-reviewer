package diff

import (
	"strings"
	"testing"
)

const testDiff = `diff --git a/internal/handler.go b/internal/handler.go
index abc1234..def5678 100644
--- a/internal/handler.go
+++ b/internal/handler.go
@@ -10,6 +10,8 @@ func HandleRequest(w http.ResponseWriter, r *http.Request) {
 	ctx := r.Context()
 	id := r.URL.Query().Get("id")
 
+	if id == "" {
+		http.Error(w, "missing id", http.StatusBadRequest)
+		return
+	}
+
 	result, err := service.Fetch(ctx, id)
 	if err != nil {
 		log.Printf("error: %v", err)
diff --git a/internal/service.go b/internal/service.go
new file mode 100644
--- /dev/null
+++ b/internal/service.go
@@ -0,0 +1,5 @@
+package internal
+
+func Fetch(ctx context.Context, id string) (*Result, error) {
+	return nil, nil
+}
`

func TestParse(t *testing.T) {
	diffs, err := Parse(strings.NewReader(testDiff))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(diffs) != 2 {
		t.Fatalf("expected 2 file diffs, got %d", len(diffs))
	}

	// First file: handler.go
	d := diffs[0]
	if d.OldPath != "internal/handler.go" {
		t.Errorf("OldPath = %q, want %q", d.OldPath, "internal/handler.go")
	}
	if d.NewPath != "internal/handler.go" {
		t.Errorf("NewPath = %q, want %q", d.NewPath, "internal/handler.go")
	}
	if len(d.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(d.Hunks))
	}

	h := d.Hunks[0]
	if h.OldStart != 10 || h.OldCount != 6 {
		t.Errorf("old range = %d,%d, want 10,6", h.OldStart, h.OldCount)
	}
	if h.NewStart != 10 || h.NewCount != 8 {
		t.Errorf("new range = %d,%d, want 10,8", h.NewStart, h.NewCount)
	}

	// Check that added lines have correct line numbers.
	addedLines := 0
	for _, l := range h.Lines {
		if l.Type == LineAdded {
			addedLines++
			if l.NewLineNo == 0 {
				t.Errorf("added line has NewLineNo=0: %q", l.Content)
			}
		}
	}
	if addedLines != 5 {
		t.Errorf("expected 5 added lines, got %d", addedLines)
	}

	// Second file: service.go (new file).
	d2 := diffs[1]
	if d2.NewPath != "internal/service.go" {
		t.Errorf("NewPath = %q, want %q", d2.NewPath, "internal/service.go")
	}
	if !d2.IsNew {
		t.Error("expected IsNew=true for new file")
	}
	if len(d2.Hunks) != 1 {
		t.Fatalf("expected 1 hunk for new file, got %d", len(d2.Hunks))
	}
	if len(d2.Hunks[0].Lines) != 5 {
		t.Errorf("expected 5 lines in new file, got %d", len(d2.Hunks[0].Lines))
	}
}

func TestParse_EmptyDiff(t *testing.T) {
	diffs, err := Parse(strings.NewReader(""))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(diffs) != 0 {
		t.Errorf("expected 0 diffs, got %d", len(diffs))
	}
}

func TestParse_BinaryFile(t *testing.T) {
	input := `diff --git a/image.png b/image.png
Binary files /dev/null and b/image.png differ
`
	diffs, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if !diffs[0].IsBinary {
		t.Error("expected IsBinary=true")
	}
}

func TestParse_DeletedFile(t *testing.T) {
	input := `diff --git a/old.go b/old.go
deleted file mode 100644
--- a/old.go
+++ /dev/null
@@ -1,3 +0,0 @@
-package old
-
-func Deprecated() {}
`
	diffs, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if !diffs[0].IsDelete {
		t.Error("expected IsDelete=true")
	}
}

func TestParse_RenamedFile(t *testing.T) {
	input := `diff --git a/old_name.go b/new_name.go
rename from old_name.go
rename to new_name.go
`
	diffs, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if !diffs[0].IsRename {
		t.Error("expected IsRename=true")
	}
}

func TestFileDiff_LineCount(t *testing.T) {
	diffs, _ := Parse(strings.NewReader(testDiff))
	d := diffs[0]
	count := d.LineCount()
	if count != 5 {
		t.Errorf("LineCount() = %d, want 5", count)
	}
}
