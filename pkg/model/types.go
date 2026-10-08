package model

// ReviewResult holds the parsed model response.
type ReviewResult struct {
	Summary   string      `json:"summary"`
	Findings  []Finding   `json:"findings"`
	Usage     *TokenUsage `json:"usage,omitempty"`
	Truncated bool        `json:"truncated,omitempty"`
}

// Finding is a single review comment from the model.
type Finding struct {
	File         string `json:"file"`
	Line         int    `json:"line"`
	OldLine      int    `json:"old_line,omitempty"` // Pre-change line number of a removed line; set only when Line is 0.
	EndLine      int    `json:"end_line,omitempty"`
	Severity     string `json:"severity"`
	Category     string `json:"category"`
	Title        string `json:"title"`
	Body         string `json:"body"`
	Suggestion   string `json:"suggestion,omitempty"`
	CodeSnippet  string `json:"code_snippet,omitempty"`
	ExistingCode string `json:"existing_code,omitempty"`
	RuleName     string `json:"rule_name,omitempty"`
	RuleSource   string `json:"rule_source,omitempty"`  // "platform" or "repo"
	RuleFile     string `json:"rule_file,omitempty"`    // e.g. "hipaa-phi.yaml"
	RuleURL      string `json:"rule_url,omitempty"`     // documentation/runbook link
	Fingerprint  string `json:"fingerprint,omitempty"`  // set by the reviewer; see reviewer.Fingerprint
	Anchor       string `json:"anchor,omitempty"`       // set by the reviewer; hash of the anchored lines
	AnchorLines  int    `json:"anchor_lines,omitempty"` // set by the reviewer; lines the anchor spans
}

// TokenUsage tracks token consumption across model calls.
type TokenUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
}
