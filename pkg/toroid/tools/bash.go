package tools

import (
	"context"
	"os/exec"
	"charm.land/fantasy"
)

type BashArgs struct {
	Command string `json:"command" jsonschema:"description=The bash command to execute"`
}

func NewBashTool(a Agent, desc string) *ToolDef {
	fTool := fantasy.NewAgentTool("bash", desc, func(ctx context.Context, args BashArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
		cmd := exec.CommandContext(ctx, "bash", "-c", args.Command)
		cmd.Dir = a.WorkDir()
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fantasy.ToolResponse{Type: "text", Content: string(out) + "\nError: " + err.Error()}, nil
		}
		return fantasy.ToolResponse{Type: "text", Content: string(out)}, nil
	})

	return &ToolDef{
		Name:        "bash",
		Description: desc,
		Template:    "bash.tool.tmpl",
		AgentTool:   fTool,
	}
}
