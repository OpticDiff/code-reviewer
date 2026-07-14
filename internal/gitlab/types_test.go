package gitlab

import (
	"testing"
	"time"

	"github.com/OpticDiff/code-reviewer/internal/vcs"
	"github.com/google/go-cmp/cmp"
)

func TestMRChangesResponse_toVCS(t *testing.T) {
	tests := []struct {
		name string
		resp *MRChangesResponse
		want *vcs.MRChanges
	}{
		{
			name: "renamed and new files",
			resp: &MRChangesResponse{
				ID: 42, IID: 7, Title: "Fix bug", Description: "Important fix",
				State: "opened", Draft: true,
				Changes: []DiffEntry{
					{OldPath: "old.go", NewPath: "new.go", Diff: "@@ -1 +1 @@\n-old\n+new", RenamedFile: true},
					{NewPath: "added.go", Diff: "@@ -0,0 +1 @@\n+new file", NewFile: true},
				},
			},
			want: &vcs.MRChanges{
				ID: 42, IID: 7, Title: "Fix bug", Description: "Important fix",
				State: "opened", Draft: true,
				Changes: []vcs.DiffEntry{
					{OldPath: "old.go", NewPath: "new.go", Diff: "@@ -1 +1 @@\n-old\n+new", RenamedFile: true},
					{NewPath: "added.go", Diff: "@@ -0,0 +1 @@\n+new file", NewFile: true},
				},
			},
		},
		{
			name: "empty response",
			resp: &MRChangesResponse{},
			want: &vcs.MRChanges{Changes: []vcs.DiffEntry{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.resp.toVCS()
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("toVCS() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDiffVersion_toVCS(t *testing.T) {
	ts := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	tests := []struct {
		name string
		v    *DiffVersion
		want vcs.DiffVersion
	}{
		{
			name: "all fields",
			v:    &DiffVersion{ID: 5, HeadSHA: "head-abc", BaseSHA: "base-def", StartSHA: "start-ghi", CreatedAt: ts},
			want: vcs.DiffVersion{ID: 5, HeadSHA: "head-abc", BaseSHA: "base-def", StartSHA: "start-ghi", CreatedAt: ts},
		},
		{
			name: "zero value",
			v:    &DiffVersion{},
			want: vcs.DiffVersion{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.v.toVCS()
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("toVCS() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNote_toVCS(t *testing.T) {
	ts := time.Date(2025, 6, 15, 11, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		note *Note
		want *vcs.Comment
	}{
		{
			name: "all fields mapped",
			note: &Note{
				ID: 99, Body: "review comment",
				Author:    Author{ID: 1, Username: "reviewer-bot", Name: "Reviewer Bot"},
				System:    false,
				CreatedAt: ts,
			},
			want: &vcs.Comment{
				ID: 99, Body: "review comment", Author: "reviewer-bot",
				System: false, CreatedAt: ts,
			},
		},
		{
			name: "system note",
			note: &Note{
				ID: 100, Body: "merged", System: true,
				Author: Author{Username: "gitlab"},
			},
			want: &vcs.Comment{
				ID: 100, Body: "merged", Author: "gitlab", System: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.note.toVCS()
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("toVCS() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
