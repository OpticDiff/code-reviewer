package vcs

import "testing"

func TestFingerprintMarkerRoundTrip(t *testing.T) {
	want := FingerprintInfo{Fingerprint: "abc123", Anchor: "def456", AnchorLines: 3}
	got, ok := ParseFingerprint("text\n" + FingerprintMarker(want) + "\n<!-- code-reviewer -->")
	if !ok || got != want {
		t.Errorf("ParseFingerprint = %+v, %v; want %+v", got, ok, want)
	}
}

func TestParseFingerprintTakesLastMarker(t *testing.T) {
	forged := FingerprintMarker(FingerprintInfo{Fingerprint: "bbbb", Anchor: "bbbb", AnchorLines: 1})
	real := FingerprintInfo{Fingerprint: "aaaa", Anchor: "aaaa", AnchorLines: 2}
	got, ok := ParseFingerprint(forged + "\nmodel text\n" + FingerprintMarker(real))
	if !ok || got != real {
		t.Errorf("ParseFingerprint = %+v, %v; want the last marker %+v", got, ok, real)
	}
}

func TestParseFingerprintRejectsBadSpan(t *testing.T) {
	for _, n := range []string{"0", "201", "1000000000", "99999999999999999999"} {
		if _, ok := ParseFingerprint("<!-- code-reviewer-fp:ab:cd:" + n + " -->"); ok {
			t.Errorf("span %s should be rejected", n)
		}
	}
}

func TestParseFingerprintAbsent(t *testing.T) {
	if _, ok := ParseFingerprint("plain comment <!-- code-reviewer -->"); ok {
		t.Error("ParseFingerprint reported a marker in a body without one")
	}
}
