package model

import (
	"strings"
	"testing"
)

func TestParseSummaryJSON(t *testing.T) {
	tests := []struct {
		name               string
		input              string
		wantErr            string
		wantTitle          string
		wantClassification string
		wantRiskLevel      string
	}{
		{
			name:               "valid_json",
			input:              `{"title":"Fix auth","description":"...","intent":"...","classification":"fix","scope_areas":["auth"],"breaking_changes":[],"risk_level":"low","confidence":0.9}`,
			wantTitle:          "Fix auth",
			wantClassification: "fix",
			wantRiskLevel:      "low",
		},
		{
			name:               "code_fenced",
			input:              "```json\n" + `{"title":"Fix auth","description":"...","intent":"...","classification":"fix","scope_areas":["auth"],"breaking_changes":[],"risk_level":"low","confidence":0.9}` + "\n```",
			wantTitle:          "Fix auth",
			wantClassification: "fix",
			wantRiskLevel:      "low",
		},
		{
			name:    "missing_title",
			input:   `{"description":"...","intent":"...","classification":"fix","scope_areas":["auth"],"breaking_changes":[],"risk_level":"low","confidence":0.9}`,
			wantErr: "title",
		},
		{
			name:    "missing_classification",
			input:   `{"title":"Fix auth","description":"...","intent":"...","scope_areas":["auth"],"breaking_changes":[],"risk_level":"low","confidence":0.9}`,
			wantErr: "classification",
		},
		{
			name:    "missing_risk_level",
			input:   `{"title":"Fix auth","description":"...","intent":"...","classification":"fix","scope_areas":["auth"],"breaking_changes":[],"confidence":0.9}`,
			wantErr: "risk_level",
		},
		{
			name:    "malformed_json",
			input:   `{"title": "broken`,
			wantErr: "JSON",
		},
		{
			name:               "prose_wrapped",
			input:              "Here is the summary:\n" + `{"title":"Fix","description":"...","intent":"...","classification":"fix","scope_areas":["auth"],"breaking_changes":[],"risk_level":"low","confidence":0.9}` + "\nHope that helps!",
			wantTitle:          "Fix",
			wantClassification: "fix",
			wantRiskLevel:      "low",
		},
		{
			name:               "extra_fields",
			input:              `{"title":"Fix auth","description":"...","intent":"...","classification":"fix","scope_areas":["auth"],"breaking_changes":[],"risk_level":"low","confidence":0.9, "unknown_field": "test"}`,
			wantTitle:          "Fix auth",
			wantClassification: "fix",
			wantRiskLevel:      "low",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := parseSummaryJSON(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res == nil {
				t.Fatal("expected non-nil result")
			}
			if tt.wantTitle != "" && res.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", res.Title, tt.wantTitle)
			}
			if tt.wantClassification != "" && res.Classification != tt.wantClassification {
				t.Errorf("Classification = %q, want %q", res.Classification, tt.wantClassification)
			}
			if tt.wantRiskLevel != "" && res.RiskLevel != tt.wantRiskLevel {
				t.Errorf("RiskLevel = %q, want %q", res.RiskLevel, tt.wantRiskLevel)
			}
		})
	}
}

func TestValidateSummary(t *testing.T) {
	tests := []struct {
		name    string
		input   *SummaryResult
		wantErr bool
	}{
		{
			name: "valid",
			input: &SummaryResult{
				Title:          "Fix auth",
				Classification: "fix",
				RiskLevel:      "low",
			},
			wantErr: false,
		},
		{
			name: "missing_title",
			input: &SummaryResult{
				Classification: "fix",
				RiskLevel:      "low",
			},
			wantErr: true,
		},
		{
			name: "missing_classification",
			input: &SummaryResult{
				Title:     "Fix auth",
				RiskLevel: "low",
			},
			wantErr: true,
		},
		{
			name: "missing_risk_level",
			input: &SummaryResult{
				Title:          "Fix auth",
				Classification: "fix",
			},
			wantErr: true,
		},
		{
			name: "only_required",
			input: &SummaryResult{
				Title:          "Fix auth",
				Classification: "fix",
				RiskLevel:      "low",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSummary(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSummary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
