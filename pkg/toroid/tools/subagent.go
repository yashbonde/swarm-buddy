package tools

import (
	"context"

	"charm.land/fantasy"
)

type SubagentArgs struct {
	Task string `json:"task" jsonschema:"description=Full description of the subtask for the subagent to handle"`
}

// NewSubagentTool runs a subagent synchronously and returns its output.
func NewSubagentTool(a Agent, desc string) *ToolDef {
	fTool := fantasy.NewAgentTool("subagent", desc, func(ctx context.Context, args SubagentArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
		output, err := a.RunSubagent(ctx, args.Task)
		if err != nil {
			return fantasy.ToolResponse{}, err
		}
		return fantasy.ToolResponse{Type: "text", Content: output}, nil
	})

	return &ToolDef{
		Name:        "subagent",
		Description: desc,
		Template:    "subagent.tool.tmpl",
		AgentTool:   fTool,
	}
}
