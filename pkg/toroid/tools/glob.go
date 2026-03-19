package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"charm.land/fantasy"
)

type GlobArgs struct {
	Pattern string `json:"pattern" jsonschema:"description=The glob pattern to match (e.g. '**/*.go')"`
}

func NewGlobTool(a Agent, desc string) *ToolDef {
	fTool := fantasy.NewAgentTool("glob", desc, func(ctx context.Context, args GlobArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
		cmd := exec.CommandContext(ctx, "find", ".", "-name", args.Pattern)
		cmd.Dir = a.WorkDir()
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fantasy.ToolResponse{Type: "text", Content: string(out) + "\nError: " + err.Error()}, nil
		}
		
		lines := strings.Split(string(out), "\n")
		var filtered []string
		for _, l := range lines {
			if l != "" {
				filtered = append(filtered, l)
			}
		}
		
		content := fmt.Sprintf("<matches>\n%s\n</matches>", strings.Join(filtered, "\n"))
		return fantasy.ToolResponse{Type: "text", Content: content}, nil
	})

	return &ToolDef{
		Name:        "glob",
		Description: desc,
		Template:    "glob.tool.tmpl",
		AgentTool:   fTool,
	}
}
