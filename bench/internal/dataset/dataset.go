package dataset

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type Case struct {
	Name       string
	Dir        string
	Category   string
	Language   string
	Difficulty string
	Tags       []string
	DiffText   string
	Expected   ExpectedResult
}

type ExpectedResult struct {
	Findings       []ExpectedFinding `json:"findings"`
	FalsePositives []FalsePositive   `json:"false_positives"`
	MinFindings    int               `json:"min_findings"`
	MaxFindings    int               `json:"max_findings"`
}

type ExpectedFinding struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	Severity  string `json:"severity"`
	Category  string `json:"category"`
	Title     string `json:"title"`
	MustMatch bool   `json:"must_match"`
}

type FalsePositive struct {
	File        string `json:"file"`
	Line        int    `json:"line"`
	Category    string `json:"category"`
	Description string `json:"description"`
}

type CaseMeta struct {
	Language   string   `json:"language"`
	Difficulty string   `json:"difficulty"`
	Tags       []string `json:"tags"`
}

func LoadCases(dir string) ([]Case, error) {
	var cases []Case

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			metaPath := filepath.Join(path, "meta.json")
			if _, err := os.Stat(metaPath); err == nil {
				c, err := LoadCase(path)
				if err != nil {
					return fmt.Errorf("loading case %s: %w", path, err)
				}
				cases = append(cases, *c)
			}
		}
		return nil
	})

	return cases, err
}

func LoadCase(dir string) (*Case, error) {
	metaData, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return nil, fmt.Errorf("reading meta.json: %w", err)
	}
	var meta CaseMeta
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return nil, fmt.Errorf("parsing meta.json: %w", err)
	}

	expectedData, err := os.ReadFile(filepath.Join(dir, "expected.json"))
	if err != nil {
		return nil, fmt.Errorf("reading expected.json: %w", err)
	}
	var expected ExpectedResult
	if err := json.Unmarshal(expectedData, &expected); err != nil {
		return nil, fmt.Errorf("parsing expected.json: %w", err)
	}

	diffText, err := os.ReadFile(filepath.Join(dir, "diff.patch"))
	if err != nil {
		return nil, fmt.Errorf("reading diff.patch: %w", err)
	}

	category := filepath.Base(filepath.Dir(dir))
	name := filepath.Base(dir)

	return &Case{
		Name:       name,
		Dir:        dir,
		Category:   category,
		Language:   meta.Language,
		Difficulty: meta.Difficulty,
		Tags:       meta.Tags,
		DiffText:   string(diffText),
		Expected:   expected,
	}, nil
}

func FilterCases(cases []Case, language, category string) []Case {
	var filtered []Case
	for _, c := range cases {
		if language != "" && c.Language != language {
			continue
		}
		if category != "" && c.Category != category {
			continue
		}
		filtered = append(filtered, c)
	}
	return filtered
}
