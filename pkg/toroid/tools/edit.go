package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"charm.land/fantasy"
)

type EditArgs struct {
	FilePath string `json:"filePath" jsonschema:"description=The absolute path to the file to edit"`
	OldText  string `json:"oldText" jsonschema:"description=The exact text to replace"`
	NewText  string `json:"newText" jsonschema:"description=The text to replace it with"`
}

func NewEditTool(a Agent, desc string) *ToolDef {
	fTool := fantasy.NewAgentTool("edit", desc, func(ctx context.Context, args EditArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
		path := args.FilePath
		if !filepath.IsAbs(path) {
			path = filepath.Join(a.WorkDir(), path)
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fantasy.ToolResponse{Type: "text", Content: fmt.Sprintf("Error: %v", err)}, nil
		}

		sContent := string(content)
		if !strings.Contains(sContent, args.OldText) {
			return fantasy.ToolResponse{Type: "text", Content: "Error: oldText not found in file"}, nil
		}

		if strings.Count(sContent, args.OldText) > 1 {
			return fantasy.ToolResponse{Type: "text", Content: "Error: oldText found multiple times, please be more specific"}, nil
		}

		newContent := strings.Replace(sContent, args.OldText, args.NewText, 1)
		if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
			return fantasy.ToolResponse{Type: "text", Content: fmt.Sprintf("Error: %v", err)}, nil
		}

		return fantasy.ToolResponse{Type: "text", Content: "File edited successfully."}, nil
	})

	return &ToolDef{
		Name:        "edit",
		Description: desc,
		Template:    "edit.tool.tmpl",
		AgentTool:   fTool,
	}
}
