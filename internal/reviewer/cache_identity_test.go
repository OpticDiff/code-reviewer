package reviewer

import (
	"context"
	"testing"
	"time"

	"github.com/OpticDiff/code-reviewer/internal/cache"
	"github.com/OpticDiff/code-reviewer/internal/config"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

func TestReviewCacheModelID(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Config
		want string
	}{
		{"model only", config.Config{Model: "m"}, "m"},
		{"unset thinking level is left out", config.Config{Model: "m", Profile: "security", Version: "1.0"}, "m:profile=security:version=1.0"},
		{"thinking level", config.Config{Model: "m", ThinkingLevel: "high"}, "m:thinking=high"},
		{"thinking level with profile and version", config.Config{Model: "m", Profile: "security", ThinkingLevel: "low", Version: "1.0"}, "m:profile=security:thinking=low:version=1.0"},
		{"consensus", config.Config{Models: []string{"a", "b"}, ConsensusThreshold: 2, ThinkingLevel: "medium"}, "consensus:a+b:threshold=2:thinking=medium"},
		{"api-url ignores the thinking level", config.Config{Model: "m", APIURL: "https://example.com/v1", ThinkingLevel: "high"}, "m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reviewCacheModelID(&tt.cfg); got != tt.want {
				t.Errorf("reviewCacheModelID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReviewCacheModelID_ThinkingLevelsDiffer(t *testing.T) {
	low := reviewCacheModelID(&config.Config{Model: "m", ThinkingLevel: "low"})
	high := reviewCacheModelID(&config.Config{Model: "m", ThinkingLevel: "high"})
	if low == high {
		t.Errorf("different thinking levels share cache identity %q", low)
	}
}

// A cached review is replayed only when it was produced under the same
// effective thinking level; otherwise the model is called again.
func TestRun_CacheReplayDependsOnEffectiveThinkingLevel(t *testing.T) {
	tests := []struct {
		name      string
		stored    config.Config // identity the cached entry was written under
		run       config.Config // settings of the re-run
		wantCalls int
	}{
		{"same level replays", config.Config{ThinkingLevel: "high"}, config.Config{ThinkingLevel: "high"}, 0},
		{"different level re-reviews", config.Config{ThinkingLevel: "low"}, config.Config{ThinkingLevel: "high"}, 1},
		{"level newly set re-reviews", config.Config{}, config.Config{ThinkingLevel: "high"}, 1},
		{"unset level keeps existing entries", config.Config{}, config.Config{}, 0},
		{"api-url ignores the level", config.Config{}, config.Config{ThinkingLevel: "high", APIURL: "https://example.com/v1"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diffs := makeTestDiffs("file1.go")
			cacheDir := t.TempDir()

			cfg := tt.run
			cfg.CacheDir = cacheDir
			cfg.CacheMaxAge = time.Hour
			cfg.Model = "test-model"
			cfg.ChunkStrategy = config.ChunkStrategyFail
			cfg.MinSeverity = config.SeverityLow

			stored := tt.stored
			stored.Model = cfg.Model
			storedID := reviewCacheModelID(&stored)

			c, err := cache.New(cacheDir, time.Hour)
			if err != nil {
				t.Fatalf("creating cache: %v", err)
			}
			promptHash := cache.PromptHash(cfg.CustomPrompt, "", "", cfg.Focus, cfg.ExtraRules, "")
			key := cache.CacheKey(cache.DiffHash(diffs[0]), storedID, promptHash)
			if err := c.Store(key, cache.Entry{FilePath: "file1.go", DiffHash: cache.DiffHash(diffs[0]), Model: storedID}); err != nil {
				t.Fatalf("storing cache entry: %v", err)
			}

			mm := &mockModel{result: &model.ReviewResult{Summary: "ok"}}
			r := NewWithDiffSource(&cfg, mm, &mockVCS{}, &mockDiffSource{diffs: diffs})
			if _, err := r.Run(context.Background()); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mm.calls != tt.wantCalls {
				t.Errorf("model calls = %d, want %d", mm.calls, tt.wantCalls)
			}
		})
	}
}
