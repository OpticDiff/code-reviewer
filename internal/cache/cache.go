package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

const SchemaVersion = "v1"

// Entry is a cached review result for a single file diff.
type Entry struct {
	Version   string          `json:"version"`
	Key       string          `json:"key"`
	FilePath  string          `json:"file_path"`
	DiffHash  string          `json:"diff_hash"`
	Model     string          `json:"model"`
	CreatedAt time.Time       `json:"created_at"`
	Findings  []model.Finding `json:"findings"`
}

func DiffHash(d diff.FileDiff) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s:%s\n%s", d.OldPath, d.NewPath, d.RawText())
	return hex.EncodeToString(h.Sum(nil))
}

func PromptHash(customPrompt string, focus []string, extraRules string, formattedRules string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s:%s:%s:%s", customPrompt, strings.Join(focus, ","), extraRules, formattedRules)
	return hex.EncodeToString(h.Sum(nil))
}

func CacheKey(diffHash, model, promptHash string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s:%s:%s:%s", diffHash, model, promptHash, SchemaVersion)
	return hex.EncodeToString(h.Sum(nil))
}

type Cache struct {
	Dir    string
	MaxAge time.Duration
}

func New(dir string, maxAge time.Duration) (*Cache, error) {
	if dir == "" {
		userCacheDir, err := os.UserCacheDir()
		if err != nil {
			return nil, fmt.Errorf("getting user cache dir: %w", err)
		}
		dir = filepath.Join(userCacheDir, "code-reviewer")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating cache dir: %w", err)
	}
	return &Cache{Dir: dir, MaxAge: maxAge}, nil
}

func (c *Cache) Lookup(key string) (*Entry, bool) {
	if len(key) < 2 {
		return nil, false
	}
	path := filepath.Join(c.Dir, SchemaVersion, key[:2], key+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}

	if entry.Version != SchemaVersion {
		return nil, false
	}

	if c.MaxAge > 0 && time.Since(entry.CreatedAt) > c.MaxAge {
		return nil, false
	}

	return &entry, true
}

func (c *Cache) Store(key string, entry Entry) error {
	if len(key) < 2 {
		return fmt.Errorf("invalid cache key")
	}
	
	entry.Version = SchemaVersion
	entry.Key = key
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	dirPath := filepath.Join(c.Dir, SchemaVersion, key[:2])
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return err
	}

	tempFile, err := os.CreateTemp(dirPath, key+".*.tmp")
	if err != nil {
		return err
	}
	tempName := tempFile.Name()
	
	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close() //nolint:errcheck
		_ = os.Remove(tempName) //nolint:errcheck
		return err
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close() //nolint:errcheck
		_ = os.Remove(tempName) //nolint:errcheck
		return err
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempName) //nolint:errcheck
		return err
	}

	finalPath := filepath.Join(dirPath, key+".json")
	return os.Rename(tempName, finalPath)
}

func (c *Cache) Clear() error {
	return os.RemoveAll(filepath.Join(c.Dir, SchemaVersion))
}

func (c *Cache) Stats() (entries int, totalBytes int64, oldestTime time.Time, err error) {
	dir := filepath.Join(c.Dir, SchemaVersion)
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".json") {
			entries++
			totalBytes += info.Size()
			modTime := info.ModTime()
			if oldestTime.IsZero() || modTime.Before(oldestTime) {
				oldestTime = modTime
			}
		}
		return nil
	})
	return
}

func Partition(diffs []diff.FileDiff, c *Cache, model, promptHash string) (uncached []diff.FileDiff, cachedFindings []model.Finding, cacheKeys map[string]string) {
	cacheKeys = make(map[string]string)
	for _, d := range diffs {
		dh := DiffHash(d)
		key := CacheKey(dh, model, promptHash)
		cacheKeys[d.NewPath] = key

		if c != nil {
			if entry, ok := c.Lookup(key); ok {
				cachedFindings = append(cachedFindings, entry.Findings...)
				continue
			}
		}
		uncached = append(uncached, d)
	}
	return uncached, cachedFindings, cacheKeys
}
