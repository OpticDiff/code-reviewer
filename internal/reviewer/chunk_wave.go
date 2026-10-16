package reviewer

import (
	"context"
	"fmt"
	"sync"

	"github.com/OpticDiff/code-reviewer/internal/model"
)

// reviewWave sends one prompt per chunk to the model and returns the results
// in prompt order, whatever order the calls finish in. firstChunk is the
// zero-based index of the first chunk in the wave and only labels errors.
//
// A wave of one is a plain call. Larger waves run all calls at once and wait
// for every call to finish; if any failed, the error of the lowest-numbered
// chunk is returned, which is the chunk a sequential run would have reported.
// Rate limiting is handled by the provider's own retry, per call.
func (r *Reviewer) reviewWave(ctx context.Context, systemPrompt string, userPrompts []string, firstChunk int) ([]*model.ReviewResult, error) {
	results := make([]*model.ReviewResult, len(userPrompts))
	errs := make([]error, len(userPrompts))

	if len(userPrompts) == 1 {
		results[0], errs[0] = r.provider.Review(ctx, systemPrompt, userPrompts[0])
	} else {
		var wg sync.WaitGroup
		for i, prompt := range userPrompts {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results[i], errs[i] = r.provider.Review(ctx, systemPrompt, prompt)
			}()
		}
		wg.Wait()
	}

	for i, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("model review (chunk %d): %w", firstChunk+i+1, err)
		}
	}
	return results, nil
}
