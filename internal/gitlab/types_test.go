package gitlab

import (
	"testing"
	"time"
)

func TestMRChangesResponse_toVCS(t *testing.T) {
	resp := &MRChangesResponse{
		ID:          42,
		IID:         7,
		Title:       "Fix bug",
		Description: "Important fix",
		State:       "opened",
		Draft:       true,
		Changes: []DiffEntry{
			{
				OldPath:     "old.go",
				NewPath:     "new.go",
				Diff:        "@@ -1 +1 @@\n-old\n+new",
				NewFile:     false,
				RenamedFile: true,
				DeletedFile: false,
			},
			{
				OldPath:     "",
				NewPath:     "added.go",
				Diff:        "@@ -0,0 +1 @@\n+new file",
				NewFile:     true,
				RenamedFile: false,
				DeletedFile: false,
			},
		},
	}

	got := resp.toVCS()

	if got.ID != 42 {
		t.Errorf("ID = %d, want 42", got.ID)
	}
	if got.IID != 7 {
		t.Errorf("IID = %d, want 7", got.IID)
	}
	if got.Title != "Fix bug" {
		t.Errorf("Title = %q, want %q", got.Title, "Fix bug")
	}
	if got.Description != "Important fix" {
		t.Errorf("Description = %q, want %q", got.Description, "Important fix")
	}
	if got.State != "opened" {
		t.Errorf("State = %q, want %q", got.State, "opened")
	}
	if !got.Draft {
		t.Error("Draft = false, want true")
	}
	if len(got.Changes) != 2 {
		t.Fatalf("len(Changes) = %d, want 2", len(got.Changes))
	}

	// First change: renamed file.
	c := got.Changes[0]
	if c.OldPath != "old.go" {
		t.Errorf("Changes[0].OldPath = %q, want %q", c.OldPath, "old.go")
	}
	if c.NewPath != "new.go" {
		t.Errorf("Changes[0].NewPath = %q, want %q", c.NewPath, "new.go")
	}
	if !c.RenamedFile {
		t.Error("Changes[0].RenamedFile = false, want true")
	}

	// Second change: new file.
	c = got.Changes[1]
	if !c.NewFile {
		t.Error("Changes[1].NewFile = false, want true")
	}
	if c.Diff != "@@ -0,0 +1 @@\n+new file" {
		t.Errorf("Changes[1].Diff = %q, want diff content", c.Diff)
	}
}

func TestDiffVersion_toVCS(t *testing.T) {
	ts := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	v := &DiffVersion{
		ID:        5,
		HeadSHA:   "head-abc",
		BaseSHA:   "base-def",
		StartSHA:  "start-ghi",
		CreatedAt: ts,
	}

	got := v.toVCS()

	if got.ID != 5 {
		t.Errorf("ID = %d, want 5", got.ID)
	}
	if got.HeadSHA != "head-abc" {
		t.Errorf("HeadSHA = %q, want %q", got.HeadSHA, "head-abc")
	}
	if got.BaseSHA != "base-def" {
		t.Errorf("BaseSHA = %q, want %q", got.BaseSHA, "base-def")
	}
	if got.StartSHA != "start-ghi" {
		t.Errorf("StartSHA = %q, want %q", got.StartSHA, "start-ghi")
	}
	if !got.CreatedAt.Equal(ts) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, ts)
	}
}

func TestNote_toVCS(t *testing.T) {
	ts := time.Date(2025, 6, 15, 11, 0, 0, 0, time.UTC)
	n := &Note{
		ID:   99,
		Body: "review comment",
		Author: Author{
			ID:       1,
			Username: "reviewer-bot",
			Name:     "Reviewer Bot",
		},
		System:    false,
		CreatedAt: ts,
	}

	got := n.toVCS()

	if got.ID != 99 {
		t.Errorf("ID = %d, want 99", got.ID)
	}
	if got.Body != "review comment" {
		t.Errorf("Body = %q, want %q", got.Body, "review comment")
	}
	if got.Author != "reviewer-bot" {
		t.Errorf("Author = %q, want %q (username)", got.Author, "reviewer-bot")
	}
	if got.System {
		t.Error("System = true, want false")
	}
	if !got.CreatedAt.Equal(ts) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, ts)
	}
}

func TestMRChangesResponse_toVCS_Empty(t *testing.T) {
	resp := &MRChangesResponse{}
	got := resp.toVCS()

	if got.ID != 0 {
		t.Errorf("ID = %d, want 0", got.ID)
	}
	if len(got.Changes) != 0 {
		t.Errorf("len(Changes) = %d, want 0", len(got.Changes))
	}
}
