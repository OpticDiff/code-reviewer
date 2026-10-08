package model

import (
	"context"
	"fmt"

	pkgmodel "github.com/OpticDiff/code-reviewer/pkg/model"
)

// Type aliases re-exported from pkg/model for backward compatibility.
type ReviewProvider = pkgmodel.ReviewProvider
type MultiProvider = pkgmodel.MultiProvider

// NewMultiProviderFromReviewers is re-exported from pkg/model.
var NewMultiProviderFromReviewers = pkgmodel.NewMultiProviderFromReviewers

// NewMultiProvider creates a provider that runs multiple models concurrently.
// The threshold controls how many models must agree on a finding for it to be
// included (default: 2, minimum: 1).
// If proxyURL is non-empty, all model calls are routed through that URL.
// Each opt is applied to every per-model provider after it is created.
func NewMultiProvider(ctx context.Context, project, location string, models []string, threshold int, proxyURL string, opts ...func(*Provider)) (*MultiProvider, error) {
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
			return nil, fmt.Errorf("creating provider for %s: %w", m, err)
		}
		for _, opt := range opts {
			opt(p)
		}
		providers = append(providers, p)
	}

	return pkgmodel.NewMultiProviderFromReviewers(providers, threshold), nil
}
