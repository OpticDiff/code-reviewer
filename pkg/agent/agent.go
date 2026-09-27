// Package agent exports the agent refinement loop for use by external consumers
// (e.g., the code-review-worker). This is a thin re-export layer over
// internal/reviewer to keep the public API surface minimal.
package agent

import (
	"context"

	"github.com/OpticDiff/code-reviewer/internal/reviewer"
	"github.com/OpticDiff/code-reviewer/pkg/model"
	"google.golang.org/genai"
)

// Chatter supports multi-turn conversation for the agent loop.
// Implement this interface to bridge to your LLM provider.
type Chatter = reviewer.Chatter

// GenaiChatter wraps a genai.Client to satisfy the Chatter interface.
type GenaiChatter = reviewer.GenaiChatter

// Config configures the agent refinement loop.
type Config = reviewer.AgentConfig

// Result holds the output of the agent refinement loop.
type Result = reviewer.AgentResult

// ToolResult holds the output of a tool invocation.
type ToolResult = reviewer.ToolResult

// ToolRegistry holds available agent tools scoped to a repo root.
type ToolRegistry = reviewer.ToolRegistry

// NewToolRegistry creates a tool registry scoped to the given repo root.
var NewToolRegistry = reviewer.NewToolRegistry

// ToolSchemas returns the tool descriptions for the model prompt.
var ToolSchemas = reviewer.ToolSchemas

// RunLoop executes the agent refinement loop. It takes initial findings,
// uses tool calls to gather additional context, and returns refined findings.
// If the loop fails, callers should fall back to the initial findings.
func RunLoop(ctx context.Context, chatter Chatter, cfg Config, initialFindings []model.Finding) (*Result, error) {
	return reviewer.RunAgentLoop(ctx, chatter, cfg, initialFindings)
}

// NewGenaiChatter creates a Chatter backed by a genai.Client.
func NewGenaiChatter(client *genai.Client, modelName string) *GenaiChatter {
	return &GenaiChatter{
		Client:    client,
		ModelName: modelName,
	}
}
