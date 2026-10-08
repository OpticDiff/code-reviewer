package reviewer

import "github.com/OpticDiff/code-reviewer/internal/diff"

// configAnchorLine returns the new-side line a finding about the changed file
// at path should be anchored to. It prefers the first added line, which is a
// real change and a valid diff position. When the file has no added lines it
// falls back to the first line that exists on the new side (an unchanged
// context line), and finally to line 1 when there is none (for example a
// deleted file) or the file is not in the diff.
func configAnchorLine(diffs []diff.FileDiff, path string) int {
	for _, d := range diffs {
		if d.NewPath != path && d.OldPath != path {
			continue
		}
		firstNew := 0
		for _, h := range d.Hunks {
			for _, l := range h.Lines {
				if l.NewLineNo == 0 {
					continue
				}
				if l.Type == diff.LineAdded {
					return l.NewLineNo
				}
				if firstNew == 0 {
					firstNew = l.NewLineNo
				}
			}
		}
		if firstNew > 0 {
			return firstNew
		}
		break
	}
	return 1
}

// fileDiffFor returns the diff of the file at path, or nil. A match on the new
// path wins over a match on the old path, so that a file renamed away from path
// is not confused with a different file added at path.
func fileDiffFor(diffs []diff.FileDiff, path string) *diff.FileDiff {
	var byOldPath *diff.FileDiff
	for i := range diffs {
		d := &diffs[i]
		if d.NewPath == path {
			return d
		}
		if byOldPath == nil && d.OldPath == path {
			byOldPath = d
		}
	}
	return byOldPath
}

// oldPathFor returns the pre-change path of the file at path when it differs
// (a rename), and "" otherwise. GitLab pairs the old and new paths to locate a
// position in a renamed file.
func oldPathFor(diffs []diff.FileDiff, path string) string {
	d := fileDiffFor(diffs, path)
	if d == nil || d.OldPath == "" || d.OldPath == path {
		return ""
	}
	return d.OldPath
}

// oldLineFor returns the pre-change line number of the unchanged (context)
// line at new-side line number line in the file at path. It returns 0 when the
// line is not an unchanged line of that file's diff, for example an added line.
func oldLineFor(diffs []diff.FileDiff, path string, line int) int {
	d := fileDiffFor(diffs, path)
	if d == nil {
		return 0
	}
	for _, h := range d.Hunks {
		for _, l := range h.Lines {
			if l.Type == diff.LineContext && l.NewLineNo == line {
				return l.OldLineNo
			}
		}
	}
	return 0
}
