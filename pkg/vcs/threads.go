package vcs

import (
	"context"
	"crypto/sha256"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// DismissedFinding describes a finding posted by this tool whose review
// thread other people have resolved or replied to.
type DismissedFinding struct {
	NoteID      int    // ID of the thread's first (tool-written) note or comment.
	Fingerprint string // Finding fingerprint embedded in the original comment.
	Anchor      string // Hash of the anchored lines (and neighbours) at post time.
	AnchorLines int    // Number of lines the anchor spans.
	Path        string // File the thread is anchored to.
	Line        int    // Line the thread is anchored to (0 if unknown).
	OldSide     bool   // True when anchored to a removed line on the old side of the diff.

	// AuthorOnly is true when the only people who engaged with the thread are
	// the merge/pull request author. Such a dismissal must not silence
	// high-severity findings, since the author is the party being reviewed.
	AuthorOnly bool

	// Resolved is true when the thread was resolved by someone other than the
	// tool and the merge/pull request author, as opposed to merely replied to.
	// Clients that cannot read thread resolution leave it false.
	Resolved bool
}

// SilencesHighSeverity reports whether the dismissal may suppress a HIGH or
// CRITICAL finding. A dismissal by the author alone never may; with
// resolveOnly, a reply is not enough either.
func (d DismissedFinding) SilencesHighSeverity(resolveOnly bool) bool {
	return !d.AuthorOnly && (!resolveOnly || d.Resolved)
}

// ThreadReader is implemented by clients that can report which of the tool's
// own inline review threads other people have resolved or replied to.
// Implementations must only consider threads opened by the tool's own
// authenticated account and must fail closed (return an error) when that
// account cannot be determined.
type ThreadReader interface {
	ListDismissedFindings(ctx context.Context, projectID, mrIID string) ([]DismissedFinding, error)
}

// OpenFinding describes an unresolved inline thread the tool opened earlier
// and nobody has engaged with since. A later run that reports the same finding
// leaves it open instead of posting a second thread.
type OpenFinding struct {
	FingerprintInfo
	Path    string // File the thread is anchored to.
	Line    int    // Line the thread is anchored to (0 if unknown).
	OldSide bool   // True when anchored to a removed line on the old side of the diff.

	// Severity and RuleAttributed come from the header of the posted comment
	// (see ParseFindingHeader). Severity is empty when the header is unreadable.
	Severity       string
	RuleAttributed bool
}

// OpenThreadReader is implemented by clients that can report the tool's own
// unresolved, unanswered inline threads. Implementations must apply the same
// author check as ThreadReader and fail closed when the account is unknown.
type OpenThreadReader interface {
	ListOpenFindings(ctx context.Context, projectID, mrIID string) ([]OpenFinding, error)
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

// FindingHeader is what the first line of an inline finding comment says about
// the finding.
type FindingHeader struct {
	Severity       string // As written, e.g. "HIGH".
	RuleAttributed bool   // The finding was reported for a rule rather than by the model alone.
}

// findingHeaderRe matches the first line the tool writes for a finding:
// "<emoji> **[SEVERITY]** [<rule badge>] <title>". The badge is the one part
// that tells a rule finding from a plain one. formatInlineComment in
// internal/reviewer writes it; a test there keeps the two in step.
var findingHeaderRe = regexp.MustCompile(`^\S+ \*\*\[([A-Za-z]+)\]\*\* (📋 \*\*\[Rule: |🛡️ \*\*\[Platform Policy: )?`)

// ParseFindingHeader reads the severity and rule attribution back from the
// text of an inline comment written by the tool. Like ParseFingerprint it
// trusts nothing about the author; callers verify that first.
func ParseFindingHeader(body string) (FindingHeader, bool) {
	m := findingHeaderRe.FindStringSubmatch(body)
	if m == nil {
		return FindingHeader{}, false
	}
	return FindingHeader{Severity: m[1], RuleAttributed: m[2] != ""}, true
}

// AnchorHash hashes the file, the lines start..end and the lines directly
// around them. Including the neighbours keeps identical one-line anchors in
// different places from sharing a hash. It returns false when the first
// anchored line is not part of the diff.
func AnchorHash(file string, lines map[int]string, start, end int) (string, bool) {
	if _, ok := lines[start]; !ok {
		return "", false
	}
	var body []string
	for n := start; n <= end; n++ {
		body = append(body, lines[n])
	}
	sum := sha256.Sum256([]byte(file + "\x00" + lines[start-1] + "\x1f" + strings.Join(body, "\n") + "\x1f" + lines[end+1]))
	return fmt.Sprintf("%x", sum)[:16], true
}

// FindingSpan returns the finding's line range clamped to the lines the diff
// shows for the file and to MaxAnchorLines, so a model-supplied end_line
// cannot make hashing unbounded. It avoids iterating over the lines map so
// callers inside deterministic workflow engines (e.g. Temporal workflowcheck)
// pass static determinism analysis.
func FindingSpan(line, endLine int, lines map[int]string) (start, end int) {
	end = endLine
	if end < line {
		end = line
	}
	if end-line+1 > MaxAnchorLines {
		end = line + MaxAnchorLines - 1
	}
	for end > line {
		if _, ok := lines[end]; ok {
			break
		}
		end--
	}
	return line, end
}

// FingerprintFor combines an anchor hash with the finding's category. It lets
// a reader test whether a new finding of some category belongs to a thread
// whose anchor and fingerprint are known, without knowing the thread's category.
func FingerprintFor(anchor, category string) string {
	cat := strings.ToLower(strings.TrimSpace(category))
	sum := sha256.Sum256([]byte(anchor + "\x00" + cat))
	return fmt.Sprintf("%x", sum)[:16]
}

// ComputeFingerprint computes the anchor hash and fingerprint for a finding
// anchored to lines of a diff (keyed by line number, trimmed of whitespace).
func ComputeFingerprint(file, category string, line, endLine int, lines map[int]string) (FingerprintInfo, bool) {
	start, end := FindingSpan(line, endLine, lines)
	anchor, ok := AnchorHash(file, lines, start, end)
	if !ok {
		return FingerprintInfo{}, false
	}
	return FingerprintInfo{
		Fingerprint: FingerprintFor(anchor, category),
		Anchor:      anchor,
		AnchorLines: end - start + 1,
	}, true
}

