package reviewer

import (
	"reflect"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/config"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

func TestFilterPlatformFindings(t *testing.T) {
	r := &Reviewer{
		cfg: &config.Config{
			Profile: "platform",
			PlatformRules: []config.Rule{
				{Name: "platform-rule"},
			},
		},
	}

	findings := []model.Finding{
		{Category: "security", Title: "sec issue"},
		{Category: "bug", Title: "bug issue"},
		{Category: "style", RuleName: "platform-rule", Title: "platform style issue"},
		{Category: "style", Title: "other style issue"},
		{Category: "docs", Title: "docs issue"},
		{Category: "performance", Title: "perf issue"},
	}

	got := r.filterFindingsByProfile(findings)
	want := []model.Finding{
		{Category: "security", Title: "sec issue"},
		{Category: "bug", Title: "bug issue"},
		{Category: "style", RuleName: "platform-rule", Title: "platform style issue"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("filterFindingsByProfile() = %v, want %v", got, want)
	}
}

func TestFilterProductFindings(t *testing.T) {
	r := &Reviewer{
		cfg: &config.Config{
			Profile: "product",
			PlatformRules: []config.Rule{
				{Name: "platform-rule"},
			},
		},
	}

	findings := []model.Finding{
		{Category: "security", Title: "sec issue"},
		{Category: "style", RuleName: "platform-rule", Title: "platform style issue"},
		{Category: "style", Title: "product style issue"},
	}

	got := r.filterFindingsByProfile(findings)
	want := []model.Finding{
		{Category: "security", Title: "sec issue"},
		{Category: "style", Title: "product style issue"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("filterFindingsByProfile() = %v, want %v", got, want)
	}
}

func TestFilterAllProfile(t *testing.T) {
	r := &Reviewer{
		cfg: &config.Config{
			Profile: "all",
			PlatformRules: []config.Rule{
				{Name: "platform-rule"},
			},
		},
	}

	findings := []model.Finding{
		{Category: "security", Title: "sec issue"},
		{Category: "style", RuleName: "platform-rule", Title: "platform style issue"},
		{Category: "style", Title: "product style issue"},
	}

	got := r.filterFindingsByProfile(findings)
	if !reflect.DeepEqual(got, findings) {
		t.Errorf("filterFindingsByProfile() = %v, want %v", got, findings)
	}
}
