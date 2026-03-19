package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/fantasy"
)

type Edit struct {
	OldString  string `json:"oldString" jsonschema:"description=The exact string to replace"`
	NewString  string `json:"newString" jsonschema:"description=The string to replace with"`
	ReplaceAll bool   `json:"replaceAll,omitempty" jsonschema:"description=If true, replace all occurrences"`
}

type MultiEditArgs struct {
	FilePath string `json:"filePath" jsonschema:"description=The absolute path to the file to modify"`
	Edits    []Edit `json:"edits" jsonschema:"description=List of edits to apply"`
}

func NewMultiEditTool(a Agent, desc string) *ToolDef {
	fTool := fantasy.NewAgentTool("multiedit", desc, func(ctx context.Context, args MultiEditArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
		path := args.FilePath
		if !filepath.IsAbs(path) {
			path = filepath.Join(a.WorkDir(), path)
		}

		b, err := os.ReadFile(path)
		if err != nil {
			return fantasy.ToolResponse{Type: "text", Content: fmt.Sprintf("Error: %v", err)}, nil
		}
		content := string(b)

		for i, e := range args.Edits {
			oldStr := e.OldString
			newStr := e.NewString
			replaceAll := e.ReplaceAll

			count := strings.Count(content, oldStr)
			if count == 0 {
				return fantasy.ToolResponse{Type: "text", Content: fmt.Sprintf("Error: edit %d: oldString not found", i)}, nil
			}
			if !replaceAll && count > 1 {
				return fantasy.ToolResponse{Type: "text", Content: fmt.Sprintf("Error: edit %d: found multiple matches for oldString", i)}, nil
			}

			n := 1
			if replaceAll {
				n = -1
			}
			content = strings.Replace(content, oldStr, newStr, n)
		}

		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fantasy.ToolResponse{Type: "text", Content: fmt.Sprintf("Error: %v", err)}, nil
		}

		return fantasy.ToolResponse{Type: "text", Content: "Multiple edits applied successfully."}, nil
	})

	return &ToolDef{
		Name:        "multiedit",
		Description: desc,
		Template:    "multiedit.tool.tmpl",
		AgentTool:   fTool,
	}
}
