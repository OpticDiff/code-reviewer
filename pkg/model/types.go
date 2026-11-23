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
//
// CachedInputTokens is a subset of InputTokens: the part of the prompt the
// provider served from a cache, which providers usually bill at a discount.
// InputTokens still counts every prompt token, so a consumer that ignores the
// field sees the same numbers as before.
//
// ThinkingTokens counts the reasoning ("thinking") tokens the provider
// reports. Where it sits in the other counts follows the transport: a Vertex
// AI response reports it apart from OutputTokens and includes it in
// TotalTokens, while an OpenAI-compatible response reports it as part of
// OutputTokens. Both fields are zero when the provider does not report them.
type TokenUsage struct {
	InputTokens       int64 `json:"input_tokens"`
	CachedInputTokens int64 `json:"cached_input_tokens"`
	OutputTokens      int64 `json:"output_tokens"`
	ThinkingTokens    int64 `json:"thinking_tokens"`
	TotalTokens       int64 `json:"total_tokens"`
}

// Add accumulates other into u. A nil other adds nothing.
func (u *TokenUsage) Add(other *TokenUsage) {
	if other == nil {
		return
	}
	u.InputTokens += other.InputTokens
	u.CachedInputTokens += other.CachedInputTokens
	u.OutputTokens += other.OutputTokens
	u.ThinkingTokens += other.ThinkingTokens
	u.TotalTokens += other.TotalTokens
}
