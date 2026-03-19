package tools

import (
	"context"
	"os/exec"
	"charm.land/fantasy"
)

type GrepArgs struct {
	Pattern string `json:"pattern" jsonschema:"description=The regular expression pattern to search for"`
	Path    string `json:"path" jsonschema:"description=The path to search in (defaults to current working directory),default=."`
}

func NewGrepTool(a Agent, desc string) *ToolDef {
	fTool := fantasy.NewAgentTool("grep", desc, func(ctx context.Context, args GrepArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
		path := args.Path
		if path == "" {
			path = "."
		}
		
		cmd := exec.CommandContext(ctx, "grep", "-r", "-n", "-C", "2", args.Pattern, path)
		cmd.Dir = a.WorkDir()
		out, err := cmd.CombinedOutput()
		if err != nil && len(out) == 0 {
			return fantasy.ToolResponse{Type: "text", Content: "No matches found."}, nil
		}
		
		return fantasy.ToolResponse{Type: "text", Content: string(out)}, nil
	})

	return &ToolDef{
		Name:        "grep",
		Description: desc,
		Template:    "grep.tool.tmpl",
		AgentTool:   fTool,
	}
}
