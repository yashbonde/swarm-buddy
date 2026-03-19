package tools

import (
	"context"
	"fmt"

	"charm.land/fantasy"
)

type ToolDef struct {
	Name        string
	Description string // short (discovery) — shown to LLM in tool schema
	Template    string // name of the .tool.tmpl file (full documentation)
	AgentTool   fantasy.AgentTool
}

type Agent interface {
	WorkDir() string
	SessionID() string
	Fire(ctx context.Context, kind string, payload any) error
	RunSubagent(ctx context.Context, task string) (string, error)
}

type Registry struct {
	tools map[string]*ToolDef
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]*ToolDef)}
}

func (r *Registry) Register(t *ToolDef) {
	r.tools[t.Name] = t
}

func (r *Registry) Lookup(name string) (*ToolDef, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) Tools() map[string]*ToolDef {
	return r.tools
}

func (r *Registry) Execute(ctx context.Context, name string, args any) (any, error) {
	return nil, fmt.Errorf("direct execution not implemented for Fantasy tools")
}
