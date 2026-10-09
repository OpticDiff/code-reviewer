package vcs

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"testing/quick"
)

// hexHash generates a valid hex string from an integer for quick property testing.
func hexHash(n uint64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d", n)))
	return fmt.Sprintf("%x", sum)[:16]
}

func TestProperty_FingerprintMarker_RoundTrip(t *testing.T) {
	// Property: A valid FingerprintInfo embedded in a comment body (without forged earlier markers)
	// can always be round-tripped identically.
	property := func(fpSeed, anchorSeed uint64, lines uint8, prefix string) bool {
		// Clean prefix to ensure no collision with the marker format
		if strings.Contains(prefix, "<!-- code-reviewer-fp:") {
			return true
		}
		span := int(lines%MaxAnchorLines) + 1 // [1, MaxAnchorLines]
		want := FingerprintInfo{
			Fingerprint: hexHash(fpSeed),
			Anchor:      hexHash(anchorSeed),
			AnchorLines: span,
		}

		body := prefix + "\n" + FingerprintMarker(want)
		got, ok := ParseFingerprint(body)
		if !ok {
			return false
		}
		return got.Fingerprint == want.Fingerprint &&
			got.Anchor == want.Anchor &&
			got.AnchorLines == want.AnchorLines
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatalf("Property TestProperty_FingerprintMarker_RoundTrip failed: %v", err)
	}
}

func TestProperty_ParseFingerprint_LastMarkerWins(t *testing.T) {
	// Property: If an attacker injects a forged marker into the body, the tool's
	// trailing marker always takes precedence.
	property := func(seed1, seed2 uint64, span1, span2 uint8, middle string) bool {
		s1 := int(span1%MaxAnchorLines) + 1
		s2 := int(span2%MaxAnchorLines) + 1

		forged := FingerprintInfo{Fingerprint: hexHash(seed1), Anchor: hexHash(seed1), AnchorLines: s1}
		authoritative := FingerprintInfo{Fingerprint: hexHash(seed2), Anchor: hexHash(seed2), AnchorLines: s2}

		body := FingerprintMarker(forged) + "\n" + middle + "\n" + FingerprintMarker(authoritative)
		got, ok := ParseFingerprint(body)
		if !ok {
			return false
		}
		return got.Fingerprint == authoritative.Fingerprint &&
			got.Anchor == authoritative.Anchor &&
			got.AnchorLines == authoritative.AnchorLines
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatalf("Property TestProperty_ParseFingerprint_LastMarkerWins failed: %v", err)
	}
}

func TestProperty_ParseFingerprint_BoundsEnforced(t *testing.T) {
	// Property: AnchorLines <= 0 or > MaxAnchorLines must always fail validation.
	property := func(seed uint64, span uint16) bool {
		fp := hexHash(seed)
		// Intentionally test spans outside [1, MaxAnchorLines]
		var invalidSpan int
		if span%2 == 0 {
			invalidSpan = int(span) + MaxAnchorLines + 1 // > 200
		} else {
			invalidSpan = 0 // <= 0
		}

		marker := fmt.Sprintf("<!-- code-reviewer-fp:%s:%s:%d -->", fp, fp, invalidSpan)
		_, ok := ParseFingerprint(marker)
		return !ok // must be rejected
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatalf("Property TestProperty_ParseFingerprint_BoundsEnforced failed: %v", err)
	}
}

func TestProperty_ParseFingerprint_NoPanicOnArbitraryStrings(t *testing.T) {
	// Property: ParseFingerprint never panics on arbitrary string inputs.
	property := func(s string) bool {
		_, _ = ParseFingerprint(s)
		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 500}); err != nil {
		t.Fatalf("Property TestProperty_ParseFingerprint_NoPanicOnArbitraryStrings failed: %v", err)
	}
}
