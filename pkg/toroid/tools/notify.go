package tools

import (
	"context"
	"fmt"
	"os/exec"

	"charm.land/fantasy"
)

type NotifyArgs struct {
	Title   string `json:"title" jsonschema:"description=Notification title"`
	Message string `json:"message" jsonschema:"description=Notification message"`
}

func NewNotifyTool(a Agent, desc string) *ToolDef {
	fTool := fantasy.NewAgentTool("notify", desc, func(ctx context.Context, args NotifyArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
		title := args.Title
		message := args.Message

		_ = a.Fire(ctx, "Notification", map[string]any{
			"title":   title,
			"message": message,
		})

		script := fmt.Sprintf(`display notification %q with title %q`, message, title)
		err := exec.CommandContext(ctx, "osascript", "-e", script).Run()
		if err != nil {
			return fantasy.ToolResponse{}, err
		}
		return fantasy.ToolResponse{Type: "text", Content: "ok"}, nil
	})

	return &ToolDef{
		Name:        "notify",
		Description: desc,
		Template:    "notify.tool.tmpl",
		AgentTool:   fTool,
	}
}
