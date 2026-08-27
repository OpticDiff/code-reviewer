package cache_test

import (
	"testing"
	"time"

	"github.com/OpticDiff/code-reviewer/internal/cache"
	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

func TestDiffHash(t *testing.T) {
	d1 := diff.FileDiff{OldPath: "a.go", NewPath: "b.go"}
	d1.Hunks = append(d1.Hunks, diff.Hunk{Lines: []diff.DiffLine{{Type: diff.LineAdded, Content: "add"}}})

	d2 := diff.FileDiff{OldPath: "a.go", NewPath: "b.go"}
	d2.Hunks = append(d2.Hunks, diff.Hunk{Lines: []diff.DiffLine{{Type: diff.LineAdded, Content: "add"}}})

	d3 := diff.FileDiff{OldPath: "a.go", NewPath: "b.go"}
	d3.Hunks = append(d3.Hunks, diff.Hunk{Lines: []diff.DiffLine{{Type: diff.LineRemoved, Content: "del"}}})

	h1 := cache.DiffHash(d1)
	h2 := cache.DiffHash(d2)
	h3 := cache.DiffHash(d3)

	if h1 != h2 {
		t.Errorf("expected h1 == h2, got %s and %s", h1, h2)
	}
	if h1 == h3 {
		t.Errorf("expected h1 != h3, got same hash %s", h1)
	}
}

func TestCacheKey(t *testing.T) {
	k1 := cache.CacheKey("dh1", "model1", "ph1")
	k2 := cache.CacheKey("dh1", "model1", "ph1")
	k3 := cache.CacheKey("dh2", "model1", "ph1")

	if k1 != k2 {
		t.Errorf("expected k1 == k2, got %s and %s", k1, k2)
	}
	if k1 == k3 {
		t.Errorf("expected k1 != k3, got same hash")
	}
}

func TestStoreAndLookup(t *testing.T) {
	dir := t.TempDir()
	c, err := cache.New(dir, time.Hour)
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	key := cache.CacheKey("dh1", "model1", "ph1")
	entry := cache.Entry{
		FilePath: "test.go",
		Findings: []model.Finding{{Title: "bug"}},
	}

	if err := c.Store(key, entry); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Lookup hit
	loaded, ok := c.Lookup(key)
	if !ok {
		t.Fatalf("Lookup failed for existing key")
	}
	if loaded.FilePath != "test.go" || len(loaded.Findings) != 1 || loaded.Findings[0].Title != "bug" {
		t.Errorf("Loaded entry mismatch: %+v", loaded)
	}

	// Lookup miss
	if _, ok := c.Lookup("nonexistentkeythatislongenough"); ok {
		t.Errorf("Expected miss for nonexistent key")
	}
}

func TestExpiredLookup(t *testing.T) {
	dir := t.TempDir()
	c, err := cache.New(dir, time.Millisecond*10)
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	key := "testkey12345"
	entry := cache.Entry{FilePath: "test.go"}
	
	if err := c.Store(key, entry); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	time.Sleep(time.Millisecond * 15) // Wait for expiration

	if _, ok := c.Lookup(key); ok {
		t.Errorf("Expected miss for expired entry")
	}
}

func TestClearAndStats(t *testing.T) {
	dir := t.TempDir()
	c, err := cache.New(dir, time.Hour)
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	// Stats empty
	entries, _, _, err := c.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if entries != 0 {
		t.Errorf("Expected 0 entries, got %d", entries)
	}

	c.Store("key111", cache.Entry{})
	c.Store("key222", cache.Entry{})

	// Stats populated
	entries, _, _, err = c.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if entries != 2 {
		t.Errorf("Expected 2 entries, got %d", entries)
	}

	// Clear
	if err := c.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	// Stats empty again
	entries, _, _, err = c.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if entries != 0 {
		t.Errorf("Expected 0 entries after Clear, got %d", entries)
	}
}

func TestPartition(t *testing.T) {
	dir := t.TempDir()
	c, err := cache.New(dir, time.Hour)
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	d1 := diff.FileDiff{NewPath: "hit.go"}
	d1.Hunks = append(d1.Hunks, diff.Hunk{Lines: []diff.DiffLine{{Type: diff.LineAdded, Content: "hit"}}})
	d2 := diff.FileDiff{NewPath: "miss.go"}
	d2.Hunks = append(d2.Hunks, diff.Hunk{Lines: []diff.DiffLine{{Type: diff.LineAdded, Content: "miss"}}})

	dh1 := cache.DiffHash(d1)
	key1 := cache.CacheKey(dh1, "model", "ph")
	c.Store(key1, cache.Entry{Findings: []model.Finding{{Title: "cached finding"}}})

	diffs := []diff.FileDiff{d1, d2}
	uncached, cachedFindings, cacheKeys := cache.Partition(diffs, c, "model", "ph")

	if len(uncached) != 1 || uncached[0].NewPath != "miss.go" {
		t.Errorf("Expected 1 uncached (miss.go), got %v", uncached)
	}
	if len(cachedFindings) != 1 || cachedFindings[0].Title != "cached finding" {
		t.Errorf("Expected 1 cached finding, got %v", cachedFindings)
	}
	if len(cacheKeys) != 2 {
		t.Errorf("Expected 2 cache keys, got %d", len(cacheKeys))
	}
}
