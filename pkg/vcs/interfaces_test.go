package vcs_test

import (
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/github"
	"github.com/OpticDiff/code-reviewer/internal/gitlab"
	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

// Compile-time checks that implementations satisfy ISP interfaces.
var (
	_ vcs.DiffFetcher        = (*gitlab.Client)(nil)
	_ vcs.NotePoster         = (*gitlab.Client)(nil)
	_ vcs.Approver           = (*gitlab.Client)(nil)
	_ vcs.DescriptionUpdater = (*gitlab.Client)(nil)
	_ vcs.VCSProvider        = (*gitlab.Client)(nil)

	_ vcs.DiffFetcher        = (*github.Client)(nil)
	_ vcs.NotePoster         = (*github.Client)(nil)
	_ vcs.Approver           = (*github.Client)(nil)
	_ vcs.DescriptionUpdater = (*github.Client)(nil)
	_ vcs.VCSProvider        = (*github.Client)(nil)
)

func TestInterfaces(t *testing.T) {
	// This test simply ensures the compile-time checks above are compiled.
}
