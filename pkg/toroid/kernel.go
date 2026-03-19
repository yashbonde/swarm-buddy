package toroid

import (
	"context"
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"

	"swarm_buddy/pkg/toroid/tools"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/google"
)

//go:embed prompts/*.tmpl prompts/*.txt
var promptFS embed.FS

// readPrompt loads a prompt file from ~/.swb/prompts/<name> if present,
// falling back to the embedded copy. This allows prompt updates without recompiling.
func readPrompt(name string) ([]byte, error) {
	home, err := os.UserHomeDir()
	if err == nil {
		if b, err := os.ReadFile(filepath.Join(home, ".swb", "prompts", name)); err == nil {
			return b, nil
		}
	}
	return promptFS.ReadFile("prompts/" + name)
}

// Thinking controls the model's thinking budget.
type Thinking string

const (
	ThinkingNone Thinking = "none" // disable thinking (budget=0)
	ThinkingLow  Thinking = "low"  // ~1k tokens
	ThinkingHigh Thinking = "high" // ~8k tokens
)

// Kernel is the agentic orchestrator powered by Fantasy.
type Kernel struct {
	cfg              Config
	provider         fantasy.Provider
	model            fantasy.LanguageModel
	hooks            *HookRegistry
	tools            *tools.Registry
	memory           *MemoryStore
	systemPrompt     string
	title            string
	history          []fantasy.Message
	usage            map[string]Usage // sessionID -> total tokens used (self + subagents)
	usageMu          sync.Mutex
	fantasyAgentOpts []fantasy.AgentOption
	currentTokens    int
}

// Config holds all options for creating a Kernel.
type Config struct {
	Provider  fantasy.Provider `json:"provider,omitempty" description:"llm provider"`
	Model     string           `json:"model" description:"llm model name" default:"gemini-3-flash-preview"`
	APIKey    string           `json:"api_key,omitempty" description:"API key for the provider"`
	SessionID string           `json:"session_id,omitempty" description:"unique identifier for the session"`
	WorkDir   string           `json:"work_dir" description:"working directory" default:"current directory"`
	MaxIter   int              `json:"max_iter" description:"max tool-call iterations" default:"50"`
	Thinking  Thinking         `json:"thinking" description:"thinking budget: none | low | high" default:"none"`
	ThinkingWriter io.Writer   `json:"-"`
	Resume    bool             `json:"resume" description:"if true, load existing session history and continue" default:"false"`

	// compaction
	CompactionBufferSize int `json:"compaction_buffer_size" description:"buffer size for history compaction" default:"30000"`
	ToolCallPrune        int `json:"tool_call_prune" description:"token limit for tool call pruning" default:"40000"`
	TotalContextSize     int `json:"total_context_size" description:"total context window size" default:"300000"`

	// logging flags
	AttachLoggerHooks *bool `json:"attach_logger_hooks,omitempty" description:"automatically attach logger hooks" default:"false"`
	ShowHistory       *bool `json:"show_history" description:"print history" default:"false"`
}

type Usage struct {
	Output     int32
	Input      int32
	Reasoning  int32
	CacheRead  int32
	CacheWrite int32
	Cost       float64
}

// Implement tools.Agent interface
func (k *Kernel) WorkDir() string   { return k.cfg.WorkDir }
func (k *Kernel) SessionID() string { return k.cfg.SessionID }
func (k *Kernel) Model() string     { return k.cfg.Model }

func (k *Kernel) Fire(ctx context.Context, kind string, payload any) error {
	return k.hooks.Fire(ctx, Event{
		Kind:      EventKind(kind),
		SessionID: k.cfg.SessionID,
		Timestamp: time.Now(),
		Payload:   payload,
	})
}

// NewKernel creates and wires up a new Kernel.
func NewKernel(ctx context.Context, cfg Config) (*Kernel, error) {
	// priority cfg defaults
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("GEMINI_TOKEN")
	}
	if cfg.WorkDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		cfg.WorkDir = wd
	}
	if cfg.SessionID == "" {
		cfg.SessionID = NewSessionID()
	}

	ApplyDefaults(&cfg) // cfg OR default_cfg

	// Kernel object
	k := &Kernel{
		cfg:           cfg,
		hooks:         &HookRegistry{},
		usage:         map[string]Usage{},
		currentTokens: 0,
	}
	k.tools = initTools(k)

	// Initialize Session Storage
	mem, err := newMemoryStore(cfg.SessionID)
	if err != nil {
		return nil, err
	}
	k.memory = mem

	// Load system prompt
	systemPrompt, err := buildSystemPrompt(cfg.WorkDir)
	if err != nil {
		return nil, err
	}
	k.systemPrompt = systemPrompt

	// Load the model
	if cfg.Provider == nil {
		p, err := google.New(google.WithGeminiAPIKey(cfg.APIKey))
		if err != nil {
			return nil, fmt.Errorf("failed to initialize default google provider: %w", err)
		}
		cfg.Provider = p
	}
	model, err := cfg.Provider.LanguageModel(ctx, cfg.Model)
	if err != nil {
		return nil, err
	}
	k.model = model

	// Build Fantasy Tools
	var fTools []fantasy.AgentTool
	for _, t := range k.tools.Tools() {
		fTools = append(fTools, t.AgentTool)
	}

	// Default AttachLoggerHooks if nil
	if cfg.AttachLoggerHooks != nil && *cfg.AttachLoggerHooks {
		k.OnAll(func(ctx context.Context, e Event) error {
			if e.Kind == EventToken || e.Kind == EventReasoning {
				return nil
			}
			LogInfo(string(e.Kind) + " " + fmt.Sprintf("%v", e.Payload))
			return nil
		})
	}

	// Initialize Fantasy Agent
	opts := []fantasy.AgentOption{
		fantasy.WithSystemPrompt(systemPrompt),
		fantasy.WithTools(fTools...),
		fantasy.WithMaxRetries(5),
	}

	// Handle thinking
	if cfg.Thinking != ThinkingNone {
		if cfg.ThinkingWriter != nil {
			k.On(EventReasoning, func(_ context.Context, e Event) error {
				if p, ok := e.Payload.(*ReasoningPayload); ok {
					_, err := fmt.Fprint(cfg.ThinkingWriter, p.Text)
					return err
				}
				return nil
			})
		}
		budget := int64(1024)
		if cfg.Thinking == ThinkingHigh {
			budget = 8192
		}

		config := &google.ThinkingConfig{
			IncludeThoughts: fantasy.Opt(true),
		}

		if strings.Contains(cfg.Model, "gemini-3") {
			level := google.ThinkingLevelLow
			if cfg.Thinking == ThinkingHigh {
				level = google.ThinkingLevelHigh
			}
			config.ThinkingLevel = fantasy.Opt(level)
		} else {
			config.ThinkingBudget = fantasy.Opt(budget)
		}

		opts = append(opts, fantasy.WithProviderOptions(fantasy.ProviderOptions{
			google.Name: &google.ProviderOptions{
				ThinkingConfig: config,
			},
		}))
	}
	k.fantasyAgentOpts = opts

	return k, nil
}

func initTools(ag tools.Agent) *tools.Registry {
	r := tools.NewRegistry()

	getDescription := func(name string) string {
		b, _ := readPrompt(name + ".tool.tmpl")
		lines := strings.Split(string(b), "\n")
		if len(lines) > 1 {
			return lines[1]
		}
		return "Tool " + name
	}

	r.Register(tools.NewReadTool(ag, getDescription("read")))
	r.Register(tools.NewWriteTool(ag, getDescription("write")))
	r.Register(tools.NewLsTool(ag, getDescription("ls")))
	r.Register(tools.NewBashTool(ag, getDescription("bash")))
	r.Register(tools.NewEditTool(ag, getDescription("edit")))
	r.Register(tools.NewGlobTool(ag, getDescription("glob")))
	r.Register(tools.NewGrepTool(ag, getDescription("grep")))
	r.Register(tools.NewMultiEditTool(ag, getDescription("multiedit")))
	r.Register(tools.NewNotifyTool(ag, getDescription("notify")))
	r.Register(tools.NewSubagentTool(ag, getDescription("subagent")))
	r.Register(tools.NewTodoWriteTool(ag, getDescription("todowrite")))
	r.Register(tools.NewTodoReadTool(ag, getDescription("todoread")))
	return r
}

func buildSystemPrompt(workDir string) (string, error) {
	raw, err := readPrompt("system.tmpl")
	if err != nil {
		return "", err
	}
	tmpl, err := template.New("system").Parse(string(raw))
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	err = tmpl.Execute(&buf, map[string]any{
		"WorkDir": workDir,
		"Date":    time.Now().Format("2006-01-02 15:04:05"),
	})
	return buf.String(), err
}

// On registers a hook for an event kind.
func (k *Kernel) On(kind EventKind, fn HookFn) {
	k.hooks.On(kind, fn)
}

func (k *Kernel) OnAll(fn HookFn) {
	for _, kind := range []EventKind{
		EventSessionStart,
		EventUserPromptSubmit,
		EventToken,
		EventPermissionRequest,
		EventPreToolUse,
		EventPostToolUse,
		EventPostToolUseFailure,
		EventSubagentStart,
		EventSubagentStop,
		EventMasterIdle,
		EventNotification,
		EventTaskCompleted,
		EventTitle,
		EventReasoning,
		EventStop,
		EventPreCompact,
		EventSessionEnd,
	} {
		k.hooks.On(kind, fn)
	}
}

// Run runs the agent loop and returns the final text response.
func (k *Kernel) Run(ctx context.Context, prompt string) (string, UsagePayload, error) {
	var buf strings.Builder
	var usage UsagePayload
	k.On(EventStop, func(ctx context.Context, e Event) error {
		usage = *e.Payload.(*UsagePayload)
		return nil
	})
	err := k.Stream(ctx, prompt, &buf)
	return buf.String(), usage, err
}

func (k *Kernel) Stream(ctx context.Context, prompt string, w io.Writer) error {
	// Fire session start only once
	if len(k.history) == 0 {
		_ = k.Fire(ctx, string(EventSessionStart), nil)
		if k.systemPrompt != "" {
			k.history = append(k.history, fantasy.NewSystemMessage(k.systemPrompt))
		}
	}

	// Auto-compact if approaching context limit
	if k.currentTokens > 0 && k.currentTokens >= k.cfg.TotalContextSize-k.cfg.CompactionBufferSize {
		LogInfo("auto-compacting: currentTokens=%d threshold=%d", k.currentTokens, k.cfg.TotalContextSize-k.cfg.CompactionBufferSize)
		if err := k.Compact(ctx); err != nil {
			return err
		}
	}

	// append user message
	k.history = append(k.history, fantasy.NewUserMessage(prompt))

	// history validation
	if len(k.history) > 0 {
		if k.systemPrompt != "" && k.history[0].Role != fantasy.MessageRoleSystem {
			LogError("Kernel provided with system prompt. SysPrompt should be first item in history, found: '%s'", k.history[len(k.history)-1].Role)
			panic("Kernel provided with system prompt. SysPrompt should be first item in history")
		}
		if k.history[len(k.history)-1].Role != fantasy.MessageRoleUser {
			LogError("Last item (%d) is 'user' message. Got: '%s'", len(k.history)-1, k.history[len(k.history)-1].Role)
			panic("Last item is 'user' message.")
		}
	}

	// Trim tool results for messages older than 10 items to reduce token usage
	if len(k.history) > 4 {
		cutoff := len(k.history) - 4
		for i := 0; i < cutoff; i++ {
			if k.history[i].Role == fantasy.MessageRoleTool {
				for _, c := range k.history[i].Content {
					if p, ok := c.(fantasy.ToolResultOutputContent); ok {
						// p.Text = "[tool result trimmed]"
						p.GetType()
					}
				}
			}
		}
	}

	// Build Agent and handle streaming and events
	agent := fantasy.NewAgent(k.model, k.fantasyAgentOpts...)
	var runningCostUSD float64
	result, err := agent.Stream(ctx, fantasy.AgentStreamCall{
		Prompt:   prompt,
		Messages: k.history,
		OnStepFinish: func(step fantasy.StepResult) error {
			u := Usage{
				Input:      int32(step.Usage.InputTokens),
				CacheWrite: int32(step.Usage.CacheCreationTokens),
				CacheRead:  int32(step.Usage.CacheReadTokens),
				Reasoning:  int32(step.Usage.ReasoningTokens),
				Output:     int32(step.Usage.OutputTokens),
			}
			u.Cost = CalculateCost(k.cfg.Model, u)
			runningCostUSD += u.Cost
			turnPaise := int64(u.Cost * 94.0 * 100)
			totalPaise := int64(runningCostUSD * 94.0 * 100)
			_ = k.memory.AppendTurnCost(turnPaise, totalPaise)
			_ = k.Fire(ctx, string(EventTurnCost), &TurnCostPayload{
				TurnUsage:    u,
				TurnCostUSD:  u.Cost,
				TotalCostUSD: runningCostUSD,
			})
			return nil
		},
	})
	if err != nil {
		return err
	}

	// Then add all steps from the generation
	for _, step := range result.Steps {
		// Reasoning: Shoot out the eventhook and leave
		reasoning := step.Response.Content.Reasoning()
		if len(reasoning) > 0 {
			reasoningTrace := ""
			for _, r := range reasoning {
				reasoningTrace += "\n" + r.Text
			}
			if strings.TrimSpace(reasoningTrace) != "" {
				_ = k.Fire(ctx, string(EventReasoning), &ReasoningPayload{Text: reasoningTrace})
			}
		}

		// Text: Save in the history and shoot out the eventhook
		text := step.Response.Content.Text()
		if len(text) > 0 {
			// message := fantasy.NewUserMessage(text) // hack: no direct create assistant message presents
			// message.Role = fantasy.MessageRoleAssistant
			if strings.TrimSpace(text) != "" {
				// k.history = append(k.history, message)
				_ = k.Fire(ctx, string(EventToken), &TokenPayload{Text: text})
			}
		}

		// Tool Calls: Save in history and shoot out the eventhook
		toolCalls := step.Response.Content.ToolCalls()
		if len(toolCalls) > 0 {
			for _, tc := range toolCalls {
				_ = k.Fire(ctx, string(EventPreToolUse), &ToolUsePayload{
					Name: tc.ToolName,
					Args: tc.Input,
				})
			}
		}

		// Post Tool Results: Save in history and shoot out the eventhook
		toolResults := step.Response.Content.ToolResults()
		if len(toolResults) > 0 {
			for _, tr := range toolResults {
				resStr := fmt.Sprintf("%v", tr.Result)
				payload := &ToolUseResultPayload{
					Name:   tr.ToolName,
					Result: resStr,
				}
				if strings.HasPrefix(resStr, "Error:") {
					payload.Error = resStr
					_ = k.Fire(ctx, string(EventPostToolUseFailure), payload)
				} else {
					_ = k.Fire(ctx, string(EventPostToolUse), payload)
				}
			}
		}

		k.history = append(k.history, step.Messages...)
	}

	// Update Usage (Per-Turn)
	k.usageMu.Lock()
	u := Usage{
		Input:      int32(result.TotalUsage.InputTokens),
		CacheWrite: int32(result.TotalUsage.CacheCreationTokens),
		CacheRead:  int32(result.TotalUsage.CacheReadTokens),
		Reasoning:  int32(result.TotalUsage.ReasoningTokens),
		Output:     int32(result.TotalUsage.OutputTokens),
	}
	u.Cost = CalculateCost(k.cfg.Model, u)
	k.usage[k.cfg.SessionID] = u
	k.currentTokens = int(u.Input + u.Output + u.CacheRead + u.CacheWrite)
	k.usageMu.Unlock()

	if k.cfg.ShowHistory != nil && *k.cfg.ShowHistory {
		// Print history (after usage update so currentTokens is accurate)
		PrettyPrintHistory(k)
	}

	// Fire stop with usage
	usageSnapshot := make(map[string]Usage)
	k.usageMu.Lock()
	for k, v := range k.usage {
		usageSnapshot[k] = v
	}
	k.usageMu.Unlock()

	_ = k.Fire(ctx, string(EventStop), &UsagePayload{Tokens: usageSnapshot})

	// write response and exit
	w.Write([]byte(result.Response.Content.Text()))
	return nil
}

// Compact summarizes the current history and resets it.
func (k *Kernel) Compact(ctx context.Context) error {
	if len(k.history) == 0 {
		return nil
	}

	_ = k.Fire(ctx, string(EventPreCompact), &CompactPayload{
		MessageCount: len(k.history),
	})

	prompt, err := readPrompt("compact.kernel.tmpl")
	if err != nil {
		return err
	}

	// 1. Generate summary by calling the LLM
	agent := fantasy.NewAgent(k.model, fantasy.WithMaxRetries(5))
	result, err := agent.Generate(ctx, fantasy.AgentCall{
		Prompt:   string(prompt),
		Messages: k.history,
	})
	if err != nil {
		return err
	}
	summary := result.Response.Content.Text()

	// 2. Reset history
	if k.systemPrompt != "" {
		k.history = []fantasy.Message{fantasy.NewSystemMessage(k.systemPrompt)}
	} else {
		k.history = []fantasy.Message{}
	}
	k.history = append(k.history, fantasy.NewUserMessage(
		"Tell me the summary of our conversation.",
	))
	msg := fantasy.NewUserMessage(
		"Here is a summary of our previous interaction for your reference:\n\n" + summary,
	)
	msg.Role = fantasy.MessageRoleAssistant
	k.history = append(k.history, msg)

	return nil
}

// RunSubagent runs a subagent synchronously and returns its output.
func (k *Kernel) RunSubagent(ctx context.Context, task string) (string, error) {
	// Inherit provider, model, and key from parent, but give it a fresh session ID.
	subCfg := k.cfg
	subCfg.SessionID = NewSessionID()

	// Create an independent Kernel instance for the subagent
	subKernel, err := NewKernel(ctx, subCfg)
	if err != nil {
		return "", fmt.Errorf("failed to initialize subagent: %w", err)
	}

	// Fire an event to let the system know a subagent is starting
	_ = k.Fire(ctx, string(EventSubagentStart), &SubagentPayload{
		SessionID: subKernel.cfg.SessionID,
		Prompt:    task,
	})

	// Run the subagent on the task
	output, usage, err := subKernel.Run(ctx, task)

	// Fire stop event for the subagent
	_ = k.Fire(ctx, string(EventSubagentStop), &SubagentPayload{
		SessionID:    subKernel.cfg.SessionID,
		Prompt:       task,
		Output:       output,
		UsagePayload: usage,
	})

	if err != nil {
		return "", fmt.Errorf("subagent failed: %w", err)
	}

	return fmt.Sprintf("Subagent completed task. Output:\n%s", output), nil
}
