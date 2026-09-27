package model

// test helpers that expose pkg/model internals for existing tests.
// These are test-only (file ends in _test.go) so they don't leak.

import (
	"context"

	pkgmodel "github.com/OpticDiff/code-reviewer/pkg/model"
)

// findingsMatch delegates to the exported function for backward compat in tests.
var findingsMatch = pkgmodel.FindingsMatch

// mergeResults recreates the merge behavior for tests that called the old
// unexported function directly. We add padding providers to ensure the
// threshold is not capped below the requested value.
func mergeResults(results []*ReviewResult, threshold int) *ReviewResult {
	// Build enough providers so the threshold won't be capped.
	n := len(results)
	if threshold > n {
		n = threshold
	}
	providers := make([]ReviewProvider, n)
	for i := range n {
		if i < len(results) {
			providers[i] = &cannedProvider{result: results[i]}
		} else {
			// Padding provider that returns empty result (not error).
			providers[i] = &cannedProvider{result: &ReviewResult{}}
		}
	}
	mp := pkgmodel.NewMultiProviderFromReviewers(providers, threshold)
	result, err := mp.Review(context.Background(), "", "")
	if err != nil {
		// Threshold not met — return empty result like the original mergeResults.
		return &ReviewResult{}
	}
	return result
}

type cannedProvider struct {
	result *ReviewResult
}

func (c *cannedProvider) Review(_ context.Context, _, _ string) (*ReviewResult, error) {
	if c.result == nil {
		return &ReviewResult{}, nil
	}
	return c.result, nil
}

func (c *cannedProvider) Close() {}
