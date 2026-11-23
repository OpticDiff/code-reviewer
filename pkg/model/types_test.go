package model

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

func TestTokenUsage_Add(t *testing.T) {
	var total TokenUsage
	total.Add(nil)
	total.Add(&TokenUsage{InputTokens: 100, CachedInputTokens: 60, OutputTokens: 10, ThinkingTokens: 5, TotalTokens: 115})
	total.Add(&TokenUsage{InputTokens: 40, OutputTokens: 20, TotalTokens: 60})
	total.Add(&TokenUsage{InputTokens: 10, CachedInputTokens: 8, ThinkingTokens: 2, TotalTokens: 12})

	want := TokenUsage{InputTokens: 150, CachedInputTokens: 68, OutputTokens: 30, ThinkingTokens: 7, TotalTokens: 187}
	if total != want {
		t.Errorf("sum = %+v, want %+v", total, want)
	}
}

// TestTokenUsage_JSONKeys pins the keys of the usage object that appears in
// the JSON report and the audit log. Every key is always present, so a
// consumer can read a zero instead of testing for absence.
func TestTokenUsage_JSONKeys(t *testing.T) {
	data, err := json.Marshal(ReviewResult{Usage: &TokenUsage{InputTokens: 1}})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Usage map[string]int64 `json:"usage"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range out.Usage {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := []string{"cached_input_tokens", "input_tokens", "output_tokens", "thinking_tokens", "total_tokens"}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("usage keys = %v, want %v", keys, want)
	}
}

type usageProvider struct{ usage *TokenUsage }

func (p usageProvider) Review(context.Context, string, string) (*ReviewResult, error) {
	return &ReviewResult{Summary: "s", Usage: p.usage}, nil
}
func (usageProvider) Close() {}

func TestMultiProvider_SumsCachedAndThinkingTokens(t *testing.T) {
	m := NewMultiProviderFromReviewers([]ReviewProvider{
		usageProvider{&TokenUsage{InputTokens: 100, CachedInputTokens: 50, OutputTokens: 10, ThinkingTokens: 4, TotalTokens: 114}},
		usageProvider{&TokenUsage{InputTokens: 200, CachedInputTokens: 0, OutputTokens: 20, ThinkingTokens: 6, TotalTokens: 226}},
		usageProvider{},
	}, 1)
	res, err := m.Review(context.Background(), "sys", "user")
	if err != nil {
		t.Fatal(err)
	}
	want := TokenUsage{InputTokens: 300, CachedInputTokens: 50, OutputTokens: 30, ThinkingTokens: 10, TotalTokens: 340}
	if res.Usage == nil || *res.Usage != want {
		t.Errorf("usage = %+v, want %+v", res.Usage, want)
	}
}
