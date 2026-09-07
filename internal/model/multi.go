package model

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// ReviewProvider is a single model that can review code. Extracted as an
// interface so MultiProvider can be tested with mocks.
type ReviewProvider interface {
	Review(ctx context.Context, systemPrompt, userPrompt string) (*ReviewResult, error)
	Close()
}

// MultiProvider runs multiple models in parallel and deduplicates findings
// using a consensus threshold. Only findings that appear in >= threshold
// model results are kept, reducing false positives.
type MultiProvider struct {
	providers []ReviewProvider
	threshold int
}

// NewMultiProvider creates a provider that runs multiple models concurrently.
// The threshold controls how many models must agree on a finding for it to be
// included (default: 2, minimum: 1).
// If proxyURL is non-empty, all model calls are routed through that URL.
func NewMultiProvider(ctx context.Context, project, location string, models []string, threshold int, proxyURL string) (*MultiProvider, error) {
	if len(models) == 0 {
		return nil, fmt.Errorf("at least one model is required")
	}
	if threshold < 1 {
		threshold = 1
	}
	if threshold > len(models) {
		threshold = len(models)
	}

	providers := make([]ReviewProvider, 0, len(models))
	for _, m := range models {
		p, err := NewProvider(ctx, project, location, m, proxyURL)
		if err != nil {
			// Close already-created providers on failure.
			for _, existing := range providers {
				existing.Close()
			}
			return nil, fmt.Errorf("creating provider for model %q: %w", m, err)
		}
		providers = append(providers, p)
	}

	return &MultiProvider{
		providers: providers,
		threshold: threshold,
	}, nil
}

// NewMultiProviderFromReviewers creates a MultiProvider from pre-built
// ReviewProvider instances. Useful for testing with mocks.
func NewMultiProviderFromReviewers(providers []ReviewProvider, threshold int) *MultiProvider {
	if threshold < 1 {
		threshold = 1
	}
	return &MultiProvider{
		providers: providers,
		threshold: threshold,
	}
}

// Review runs all models concurrently, collects findings, deduplicates them
// by file+line proximity+category, and returns only findings that meet the
// consensus threshold. If individual providers fail, the review succeeds
// as long as at least threshold providers succeeded.
func (m *MultiProvider) Review(ctx context.Context, systemPrompt, userPrompt string) (*ReviewResult, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex

	successfulResults := make([]*ReviewResult, 0, len(m.providers))
	var errors []error

	for i, p := range m.providers {
		wg.Add(1)
		go func(idx int, prov ReviewProvider) {
			defer wg.Done()
			result, err := prov.Review(ctx, systemPrompt, userPrompt)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errors = append(errors, fmt.Errorf("model provider %d: %w", idx, err))
			} else {
				successfulResults = append(successfulResults, result)
			}
		}(i, p)
	}

	wg.Wait()

	if len(successfulResults) < m.threshold {
		var errMsgs []string
		for _, err := range errors {
			errMsgs = append(errMsgs, err.Error())
		}
		return nil, fmt.Errorf("consensus threshold not met (%d/%d successful, needed %d): %s",
			len(successfulResults), len(m.providers), m.threshold, strings.Join(errMsgs, "; "))
	}

	if len(errors) > 0 {
		slog.Warn("some consensus providers failed, proceeding with successful models",
			"successful", len(successfulResults),
			"failed", len(errors),
			"threshold", m.threshold)
	}

	return mergeResults(successfulResults, m.threshold), nil
}

// Close releases resources for all providers.
func (m *MultiProvider) Close() {
	for _, p := range m.providers {
		p.Close()
	}
}

// mergeResults deduplicates findings across model results and applies the
// consensus threshold.
func mergeResults(results []*ReviewResult, threshold int) *ReviewResult {
	type findingGroup struct {
		anchor    Finding // Fixed comparison point — never changes after group creation.
		canonical Finding // Best finding to display (longest body).
		count     int
	}

	var groups []*findingGroup

	for _, r := range results {
		if r == nil {
			continue
		}
		for _, f := range r.Findings {
			// Check if we have an existing group this finding matches.
			matched := false
			for _, g := range groups {
				if findingsMatch(f, g.anchor) {
					g.count++
					// Keep the finding with the longest body as canonical.
					if len(f.Body) > len(g.canonical.Body) {
						g.canonical = f
					}
					matched = true
					break
				}
			}
			if !matched {
				groups = append(groups, &findingGroup{
					anchor:    f,
					canonical: f,
					count:     1,
				})
			}
		}
	}

	// Apply threshold filter.
	var findings []Finding
	for _, g := range groups {
		if g.count >= threshold {
			findings = append(findings, g.canonical)
		}
	}

	// Merge summaries (deduplicated).
	var summaries []string
	seen := make(map[string]bool)
	for _, r := range results {
		if r != nil && r.Summary != "" && !seen[r.Summary] {
			summaries = append(summaries, r.Summary)
			seen[r.Summary] = true
		}
	}

	// Aggregate token usage across all models.
	var totalUsage TokenUsage
	for _, r := range results {
		if r != nil && r.Usage != nil {
			totalUsage.InputTokens += r.Usage.InputTokens
			totalUsage.OutputTokens += r.Usage.OutputTokens
			totalUsage.TotalTokens += r.Usage.TotalTokens
		}
	}

	merged := &ReviewResult{
		Summary:  strings.Join(summaries, " "),
		Findings: findings,
	}
	if totalUsage.TotalTokens > 0 {
		merged.Usage = &totalUsage
	}
	return merged
}

// findingsMatch is a local alias for the shared FindingsMatch function.
// Kept for backward compatibility within this file.
var findingsMatch = FindingsMatch
