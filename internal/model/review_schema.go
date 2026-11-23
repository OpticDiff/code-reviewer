package model

import "google.golang.org/genai"

// ReviewSeverities are the severity values the review accepts, in the order
// the prompt lists them. They are the enum of the response schema's severity.
var ReviewSeverities = []string{"CRITICAL", "HIGH", "MEDIUM", "LOW"}

// reviewerSetFindingFields are the JSON fields of Finding that the reviewer
// fills in after the model answers. The model is not asked for them, so the
// response schema leaves them out.
var reviewerSetFindingFields = map[string]string{
	"rule_source":  "set from the rule that matched",
	"rule_file":    "set from the rule that matched",
	"rule_url":     "set from the rule that matched",
	"fingerprint":  "computed by the reviewer",
	"anchor":       "computed by the reviewer",
	"anchor_lines": "computed by the reviewer",
}

// ReviewResponseSchema describes the JSON object the built-in review prompt
// asks for (see OUTPUT FORMAT in the prompt) and parseReviewJSON reads: a
// summary and a list of findings.
//
// Required are the fields every finding needs. A finding carries "line" for an
// added line or "old_line" for a removed one, never both, so neither is
// required. "category" is a plain string: the prompt lists six values, but a
// rule may fix another category and the model must be able to use it.
//
// The schema fits the built-in prompt only. A custom prompt defines its own
// shape in prose, so the caller must not send this schema with one.
func ReviewResponseSchema() *genai.Schema {
	str := func(desc string) *genai.Schema { return &genai.Schema{Type: genai.TypeString, Description: desc} }
	num := func(desc string) *genai.Schema { return &genai.Schema{Type: genai.TypeInteger, Description: desc} }

	finding := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"file":          str("Path of the file, as shown in the diff."),
			"line":          num("new_line number of the added line; omit for a removed line."),
			"old_line":      num("Line number shown next to a removed line; omit for an added line."),
			"end_line":      num("Last line of the range; omit for a single line."),
			"severity":      {Type: genai.TypeString, Enum: ReviewSeverities},
			"category":      str("bug, security, performance, style, docs or scope, or the category a rule fixes."),
			"rule_name":     str("Exact name of the custom or platform rule violated, if any."),
			"title":         str("Single sentence summary of the issue."),
			"body":          str("Explanation of why this is an issue and its impact."),
			"suggestion":    str("Corrected code, as a drop-in replacement; omit if unsure."),
			"code_snippet":  str("Verbatim code the finding is about, if useful to locate it."),
			"existing_code": str("Verbatim current code the suggestion replaces."),
		},
		Required:         []string{"file", "severity", "category", "title", "body"},
		PropertyOrdering: []string{"file", "line", "old_line", "end_line", "severity", "category", "rule_name", "title", "body", "suggestion", "code_snippet", "existing_code"},
	}
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"summary":  str("A brief 1-2 sentence description of the overall change and its quality."),
			"findings": {Type: genai.TypeArray, Items: finding},
		},
		Required:         []string{"summary", "findings"},
		PropertyOrdering: []string{"summary", "findings"},
	}
}
