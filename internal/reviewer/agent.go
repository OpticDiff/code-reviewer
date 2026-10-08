// Package reviewer provides the agent refinement loop for multi-turn code review.
package reviewer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/OpticDiff/code-reviewer/internal/retry"
	"github.com/OpticDiff/code-reviewer/pkg/model"
	"google.golang.org/genai"
)

// Chatter supports multi-turn conversation for the agent loop.
// This is separate from ModelReviewer (which is stateless sys+user)
// because the agent loop needs to maintain conversation history
// including tool call results across turns.
type Chatter interface {
	Chat(ctx context.Context, history []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error)
}

// GenaiChatter wraps a genai.Client to satisfy the Chatter interface.
type GenaiChatter struct {
	Client    *genai.Client
	ModelName string
}

// Chat sends a multi-turn conversation to the model.
func (g *GenaiChatter) Chat(ctx context.Context, history []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	var result *genai.GenerateContentResponse
	var genErr error

	retryOpts := retry.DefaultOptions()
	retryOpts.RetryIf = func(err error) bool {
		errStr := strings.ToLower(err.Error())
		return strings.Contains(errStr, "429") ||
			strings.Contains(errStr, "503") ||
			strings.Contains(errStr, "502") ||
			strings.Contains(errStr, "504") ||
			strings.Contains(errStr, "rate") ||
			strings.Contains(errStr, "unavailable") ||
			strings.Contains(errStr, "overloaded")
	}

	if err := retry.Do(ctx, "agent chat", func() error {
		result, genErr = g.Client.Models.GenerateContent(ctx, g.ModelName, history, config)
		return genErr
	}, retryOpts); err != nil {
		return nil, fmt.Errorf("agent chat: %w", err)
	}
	return result, nil
}

// AgentConfig configures the agent refinement loop.
type AgentConfig struct {
	MaxIterations   int           // Max refinement turns (default 3).
	RemainingBudget int64         // Token budget remaining from initial review.
	Timeout         time.Duration // Wall-clock timeout (default 3m).
	RepoRoot        string        // Repository root for tool access.
}

// AgentResult holds the output of the agent refinement loop.
type AgentResult struct {
	Findings   []model.Finding    `json:"findings"`
	Usage      model.TokenUsage   `json:"usage"`
	Iterations int                `json:"iterations"`
	ToolCalls  map[string]int     `json:"tool_calls"` // tool name -> call count
	StopReason string             `json:"stop_reason"` // "done", "budget", "iterations", "timeout", "error", "stable"
	// Verdicts records, for every finding the loop was given, whether it was
	// verified, dismissed, or left unverified because the loop did not finish.
	Verdicts []FindingVerdict `json:"verdicts,omitempty"`
}

// Agent loop verdict statuses.
const (
	// VerdictVerified marks a finding that survived a completed agent loop.
	VerdictVerified = "verified"
	// VerdictDismissed marks a finding the agent loop refuted and dropped.
	VerdictDismissed = "dismissed"
	// VerdictUnverified marks a finding the loop could not rule on, for
	// example because it hit its budget, timeout or an error first.
	VerdictUnverified = "unverified"
)

// Values accepted for Config.AgentScope.
const (
	// AgentScopeAll sends every finding through the agent loop.
	AgentScopeAll = "all"
	// AgentScopeHighSeverityOnly sends only HIGH and CRITICAL findings through the agent loop.
	AgentScopeHighSeverityOnly = "high_severity_only"
)

// FindingVerdict is the agent loop's outcome for a single finding, written to the audit log.
type FindingVerdict struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	OldLine  int    `json:"old_line,omitempty"` // Set instead of Line for a finding on a removed line.
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Status   string `json:"status"` // VerdictVerified, VerdictDismissed or VerdictUnverified.
}

// splitAgentCandidates partitions findings into those the agent loop should
// examine under the given scope and those it should leave untouched.
func splitAgentCandidates(findings []model.Finding, scope string) (candidates, rest []model.Finding) {
	if scope != AgentScopeHighSeverityOnly {
		return findings, nil
	}
	for _, f := range findings {
		if severityRank(f.Severity) >= severityRank("HIGH") {
			candidates = append(candidates, f)
		} else {
			rest = append(rest, f)
		}
	}
	return candidates, rest
}

// buildVerdicts labels each initial finding by whether it survives in the
// final findings. When the loop did not complete, nothing is ruled on.
func buildVerdicts(initial, final []model.Finding, completed bool) []FindingVerdict {
	type key struct {
		File    string
		Line    int
		OldLine int
	}
	retained := make(map[key]int, len(final))
	for _, f := range final {
		retained[key{f.File, f.Line, f.OldLine}]++
	}
	verdicts := make([]FindingVerdict, 0, len(initial))
	for _, f := range initial {
		k := key{f.File, f.Line, f.OldLine}
		status := VerdictUnverified
		switch {
		case !completed:
		case retained[k] > 0:
			retained[k]--
			status = VerdictVerified
		default:
			status = VerdictDismissed
		}
		verdicts = append(verdicts, FindingVerdict{File: f.File, Line: f.Line, OldLine: f.OldLine, Severity: f.Severity, Title: f.Title, Status: status})
	}
	return verdicts
}

// agentAction is the parsed model response format.
type agentAction struct {
	Action   string          `json:"action"`             // "tool", "findings", "done"
	Tool     string          `json:"tool,omitempty"`     // tool name (when action=tool)
	Args     map[string]any  `json:"args,omitempty"`     // tool arguments
	Findings json.RawMessage `json:"findings,omitempty"` // refined findings (when action=findings)
}

// RunAgentLoop executes the agent refinement loop. It takes initial findings,
// uses tool calls to gather additional context, and returns refined findings.
// If the loop fails, callers should fall back to the initial findings.
func RunAgentLoop(ctx context.Context, chatter Chatter, cfg AgentConfig,
	initialFindings []model.Finding,
) (*AgentResult, error) {
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = 3
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 3 * time.Minute
	}
	if cfg.RemainingBudget <= 0 {
		cfg.RemainingBudget = 50000
	}

	// Enforce wall-clock timeout (from reliability review).
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	tools := NewToolRegistry(cfg.RepoRoot)
	result := &AgentResult{
		Findings:   initialFindings,
		ToolCalls:  make(map[string]int),
		StopReason: "iterations",
	}

	// Build initial findings JSON for system prompt.
	findingsJSON, err := json.Marshal(initialFindings)
	if err != nil {
		return nil, fmt.Errorf("marshaling initial findings: %w", err)
	}

	// System instruction: pin initial findings (from ML review — don't rely on event history).
	systemPrompt := buildRefinementSystemPrompt(string(findingsJSON))
	genConfig := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(systemPrompt, genai.RoleUser),
		Temperature:       genai.Ptr(float32(0.0)), // Deterministic for refinement (ML review).
	}

	// Build conversation history.
	history := []*genai.Content{
		genai.NewContentFromText(
			"Review the initial findings above. Use the available tools to verify or dismiss each finding. "+
				"Only dismiss a finding if you can prove it is a false positive with concrete evidence from the code.",
			genai.RoleUser,
		),
	}

	var totalUsage model.TokenUsage
	consecutiveToolErrors := 0

	for i := 0; i < cfg.MaxIterations; i++ {
		// Check budget before calling model.
		if totalUsage.TotalTokens >= cfg.RemainingBudget {
			result.StopReason = "budget"
			slog.Info("agent loop: budget exhausted", "used", totalUsage.TotalTokens, "budget", cfg.RemainingBudget)
			break
		}

		// Check context (timeout).
		if ctx.Err() != nil {
			result.StopReason = "timeout"
			slog.Info("agent loop: timeout reached")
			break
		}

		resp, err := chatter.Chat(ctx, history, genConfig)
		if err != nil {
			result.StopReason = "error"
			result.Usage = totalUsage // Preserve usage accumulated so far.
			result.Verdicts = buildVerdicts(initialFindings, result.Findings, false)
			return result, fmt.Errorf("agent iteration %d: %w", i+1, err)
		}
		result.Iterations = i + 1

		// Track token usage.
		if resp.UsageMetadata != nil {
			totalUsage.InputTokens += int64(resp.UsageMetadata.PromptTokenCount)
			totalUsage.OutputTokens += int64(resp.UsageMetadata.CandidatesTokenCount)
			totalUsage.TotalTokens += int64(resp.UsageMetadata.TotalTokenCount)
		}

		// Extract model response text.
		modelText := extractResponseText(resp)
		if modelText == "" {
			slog.Warn("agent loop: empty model response", "iteration", i+1)
			result.StopReason = "error"
			break
		}

		// Append assistant turn to history.
		history = append(history, genai.NewContentFromText(modelText, genai.RoleModel))

		// Parse the model's action.
		var action agentAction
		if err := json.Unmarshal([]byte(modelText), &action); err != nil {
			slog.Warn("agent loop: unparseable response, stopping", "iteration", i+1, "error", err)
			result.StopReason = "error"
			break
		}

		switch action.Action {
		case "tool":
			if action.Tool == "" {
				slog.Warn("agent loop: tool action with empty tool name", "iteration", i+1)
				consecutiveToolErrors++
				history = append(history, genai.NewContentFromText(
					"Tool call error: the tool name is empty. Choose a valid tool or return findings.",
					genai.RoleUser,
				))
				if consecutiveToolErrors >= 2 {
					slog.Warn("agent loop: too many consecutive tool errors, stopping")
					result.StopReason = "error"
					goto done
				}
				continue
			}
			result.ToolCalls[action.Tool]++

			// Execute tool.
			toolResult := tools.Execute(ctx, action.Tool, action.Args)
			if toolResult.Error != "" {
				consecutiveToolErrors++
				if consecutiveToolErrors >= 2 {
					slog.Warn("agent loop: too many consecutive tool errors, stopping",
						"tool", action.Tool, "error", toolResult.Error)
					result.StopReason = "error"
					goto done
				}
			} else {
				consecutiveToolErrors = 0
			}

			// Append tool result to history as user message.
			toolJSON, _ := json.Marshal(toolResult) //nolint:errcheck
			history = append(history, genai.NewContentFromText(
				fmt.Sprintf("<tool_result>%s</tool_result>", string(toolJSON)),
				genai.RoleUser,
			))

		case "findings":
			// Parse refined findings.
			var refined []model.Finding
			if err := json.Unmarshal(action.Findings, &refined); err != nil {
				slog.Warn("agent loop: invalid findings JSON, keeping initial", "error", err)
				result.StopReason = "error"
				goto done
			}
			// Validate: refined findings must be a subset of initial (can't add new ones).
			validated := validateRefinedFindings(initialFindings, refined)
			result.Findings = validated
			result.StopReason = "done"
			goto done

		case "done":
			result.StopReason = "done"
			goto done

		default:
			slog.Warn("agent loop: unknown action", "action", action.Action, "iteration", i+1)
			result.StopReason = "error"
			goto done
		}
	}

done:
	result.Usage = totalUsage
	result.Verdicts = buildVerdicts(initialFindings, result.Findings, result.StopReason == "done")
	return result, nil
}

// runScopedAgentLoop runs the agent loop over the findings selected by scope
// and returns them merged with the findings the scope excluded. When nothing
// is selected the model is not called and the result is nil. On error the
// original findings are returned unchanged.
func runScopedAgentLoop(ctx context.Context, chatter Chatter, cfg AgentConfig,
	findings []model.Finding, scope string,
) ([]model.Finding, *AgentResult, error) {
	candidates, rest := splitAgentCandidates(findings, scope)
	if len(candidates) == 0 {
		return findings, nil, nil
	}
	result, err := RunAgentLoop(ctx, chatter, cfg, candidates)
	if err != nil {
		return findings, result, err
	}
	return append(append([]model.Finding{}, result.Findings...), rest...), result, nil
}

// buildRefinementSystemPrompt constructs the system prompt with pinned findings
// and tool schemas. Initial findings are in the system instruction so they
// survive memory compression (from ML Systems review).
func buildRefinementSystemPrompt(findingsJSON string) string {
	var sb strings.Builder
	sb.WriteString("You are a code review refinement agent. Your job is to verify or dismiss initial findings.\n\n")
	sb.WriteString("## Rules\n")
	sb.WriteString("- You may ONLY drop or modify initial findings. You MUST NOT add new findings.\n")
	sb.WriteString("- Only dismiss a finding if new context from tools PROVES it is a false positive.\n")
	sb.WriteString("- If you cannot verify a finding, keep it as-is.\n")
	sb.WriteString("- Use tools to read relevant source files and verify findings.\n\n")
	sb.WriteString("## Response Format\n")
	sb.WriteString("Respond with exactly ONE JSON object per turn:\n")
	sb.WriteString("- To call a tool: {\"action\": \"tool\", \"tool\": \"<name>\", \"args\": {<args>}}\n")
	sb.WriteString("- To return refined findings: {\"action\": \"findings\", \"findings\": [<findings>]}\n")
	sb.WriteString("- To finish without changes: {\"action\": \"done\"}\n\n")
	sb.WriteString("## ")
	sb.WriteString(ToolSchemas())
	sb.WriteString("\n\n")
	sb.WriteString("## Initial Findings\n")
	sb.WriteString("<initial_findings>\n")
	sb.WriteString(findingsJSON)
	sb.WriteString("\n</initial_findings>\n")
	return sb.String()
}

// validateRefinedFindings ensures refined findings are a subset of initial findings.
// Uses count-based matching to handle duplicate findings at the same file+line.
// Any finding in refined that doesn't match an initial finding is dropped.
func validateRefinedFindings(initial, refined []model.Finding) []model.Finding {
	// Build count of initial findings per location.
	type key struct {
		File    string
		Line    int
		OldLine int
	}
	initialCounts := make(map[key]int, len(initial))
	for _, f := range initial {
		initialCounts[key{File: f.File, Line: f.Line, OldLine: f.OldLine}]++
	}

	// Accept refined findings only while their location has remaining matches.
	validated := make([]model.Finding, 0, len(refined))
	for _, f := range refined {
		k := key{File: f.File, Line: f.Line, OldLine: f.OldLine}
		if initialCounts[k] > 0 {
			initialCounts[k]--
			validated = append(validated, f)
		} else {
			slog.Debug("agent loop: dropping non-initial finding",
				"file", f.File, "line", f.Line)
		}
	}
	return validated
}

// extractResponseText pulls text from a genai response.
func extractResponseText(resp *genai.GenerateContentResponse) string {
	if resp == nil || len(resp.Candidates) == 0 {
		return ""
	}
	candidate := resp.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			sb.WriteString(part.Text)
		}
	}
	return sb.String()
}
