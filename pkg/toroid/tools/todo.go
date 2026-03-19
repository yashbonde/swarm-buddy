package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"charm.land/fantasy"
)

type Task struct {
	ID      string `json:"id" jsonschema:"description=Unique task ID"`
	Title   string `json:"title,omitempty" jsonschema:"description=Task title"`
	Status  string `json:"status" jsonschema:"description=pending | in_progress | completed | cancelled"`
	Details string `json:"details,omitempty" jsonschema:"description=Task details"`
}

type TodoWriteArgs struct {
	Tasks []Task `json:"tasks" jsonschema:"description=List of tasks to update or create"`
}

type TodoReadArgs struct{}

func NewTodoWriteTool(a Agent, desc string) *ToolDef {
	fTool := fantasy.NewAgentTool("todo_write", desc, func(ctx context.Context, args TodoWriteArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
		path := todoPath(a.SessionID())
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return fantasy.ToolResponse{}, err
		}

		existing := map[string]Task{}
		order := []string{}
		if b, err := os.ReadFile(path); err == nil {
			var list []Task
			if err := json.Unmarshal(b, &list); err == nil {
				for _, t := range list {
					existing[t.ID] = t
					order = append(order, t.ID)
				}
			}
		}

		for _, tm := range args.Tasks {
			id := tm.ID
			if id == "" {
				continue
			}
			cur, exists := existing[id]
			if !exists {
				order = append(order, id)
				cur = Task{ID: id}
			}
			if tm.Title != "" {
				cur.Title = tm.Title
			}
			if tm.Status != "" {
				cur.Status = tm.Status
			}
			if tm.Details != "" {
				cur.Details = tm.Details
			}
			existing[id] = cur

			if cur.Status == "completed" {
				_ = a.Fire(ctx, "TaskCompleted", map[string]any{
					"task_id": cur.ID,
					"title":   cur.Title,
					"status":  cur.Status,
				})
			}
		}

		out := make([]Task, 0, len(order))
		for _, id := range order {
			out = append(out, existing[id])
		}
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return fantasy.ToolResponse{}, err
		}
		if err := os.WriteFile(path, b, 0644); err != nil {
			return fantasy.ToolResponse{}, err
		}
		return fantasy.ToolResponse{Type: "text", Content: "ok"}, nil
	})

	return &ToolDef{
		Name:        "todo_write",
		Description: desc,
		Template:    "todowrite.tool.tmpl",
		AgentTool:   fTool,
	}
}

func NewTodoReadTool(a Agent, desc string) *ToolDef {
	fTool := fantasy.NewAgentTool("todo_read", desc, func(ctx context.Context, args TodoReadArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
		path := todoPath(a.SessionID())
		b, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			return fantasy.ToolResponse{Type: "text", Content: "[]"}, nil
		}
		if err != nil {
			return fantasy.ToolResponse{}, err
		}
		return fantasy.ToolResponse{Type: "text", Content: string(b)}, nil
	})

	return &ToolDef{
		Name:        "todo_read",
		Description: desc,
		Template:    "todoread.tool.tmpl",
		AgentTool:   fTool,
	}
}

func todoPath(sessionID string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".toroid", "tasks", sessionID+".json")
}
