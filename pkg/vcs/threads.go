package vcs

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
)

// DismissedFinding describes a finding posted by this tool whose review
// thread other people have resolved or replied to.
type DismissedFinding struct {
	Fingerprint string // Finding fingerprint embedded in the original comment.
	Anchor      string // Hash of the anchored lines (and neighbours) at post time.
	AnchorLines int    // Number of lines the anchor spans.
	Path        string // File the thread is anchored to.
	Line        int    // Line the thread is anchored to (0 if unknown).

	// AuthorOnly is true when the only people who engaged with the thread are
	// the merge/pull request author. Such a dismissal must not silence
	// high-severity findings, since the author is the party being reviewed.
	AuthorOnly bool
}

// ThreadReader is implemented by clients that can report which of the tool's
// own inline review threads other people have resolved or replied to.
// Implementations must only consider threads opened by the tool's own
// authenticated account and must fail closed (return an error) when that
// account cannot be determined.
type ThreadReader interface {
	ListDismissedFindings(ctx context.Context, projectID, mrIID string) ([]DismissedFinding, error)
}

// FingerprintInfo is the payload embedded in an inline comment.
type FingerprintInfo struct {
	Fingerprint string
	Anchor      string
	AnchorLines int
}

const fingerprintPrefix = "<!-- code-reviewer-fp:"

// MaxAnchorLines caps how many lines a fingerprint anchor may span. It bounds
// the work done per finding and rejects absurd values read back from comments.
const MaxAnchorLines = 200

var fingerprintRe = regexp.MustCompile(`<!-- code-reviewer-fp:([0-9a-f]+):([0-9a-f]+):([0-9]+) -->`)

// FingerprintMarker returns the hidden marker that embeds fi in a comment body.
func FingerprintMarker(fi FingerprintInfo) string {
	return fmt.Sprintf("%s%s:%s:%d -->", fingerprintPrefix, fi.Fingerprint, fi.Anchor, fi.AnchorLines)
}

// ParseFingerprint extracts the payload embedded by FingerprintMarker. The
// marker is plain text anyone can write, so callers must also verify the
// comment's author before trusting it.
func ParseFingerprint(body string) (FingerprintInfo, bool) {
	// The tool appends its own marker after all model-written text, so the
	// last match is authoritative and an earlier forged one is ignored.
	all := fingerprintRe.FindAllStringSubmatch(body, -1)
	if all == nil {
		return FingerprintInfo{}, false
	}
	m := all[len(all)-1]
	n, err := strconv.Atoi(m[3])
	if err != nil || n < 1 || n > MaxAnchorLines {
		return FingerprintInfo{}, false
	}
	return FingerprintInfo{Fingerprint: m[1], Anchor: m[2], AnchorLines: n}, true
}
