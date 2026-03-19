package toroid

import "time"

type EventKind string

const (
	EventSessionStart       EventKind = "SessionStart"
	EventUserPromptSubmit   EventKind = "UserPromptSubmit"
	EventToken              EventKind = "Token"              // each streamed text chunk
	EventPermissionRequest  EventKind = "PermissionRequest"  // before a tool is called, if permission is required
	EventPreToolUse         EventKind = "PreToolUse"         // before a tool is called
	EventPostToolUse        EventKind = "PostToolUse"        // after a tool call is completed
	EventPostToolUseFailure EventKind = "PostToolUseFailure" // after a tool call fails
	EventSubagentStart      EventKind = "SubagentStart"      // before the subagent is started
	EventSubagentStop       EventKind = "SubagentStop"       // before the subagent is stopped
	EventMasterIdle         EventKind = "MasterIdle"         // after the main agent is idle
	EventNotification       EventKind = "Notification"       // before the notification is sent
	EventTaskCompleted      EventKind = "TaskCompleted"      // before the task is completed
	EventTitle              EventKind = "Title"              // fired async when session title is ready
	EventReasoning          EventKind = "Reasoning"          // streamed reasoning/thinking tokens
	EventStop               EventKind = "Stop"               // when the agent is stopped
	EventPreCompact         EventKind = "PreCompact"         // before compacting the memory
	EventSessionEnd         EventKind = "SessionEnd"         // after the session ends
)

type Event struct {
	Kind      EventKind `json:"kind"`
	SessionID string    `json:"session_id"`
	Timestamp time.Time `json:"timestamp"`
	Payload   any       `json:"payload,omitempty"`
}

// fantasy event to Swarm Buddy event Map

// Payload types

type UserPromptPayload struct {
	Prompt string `json:"prompt"`
}

type TokenPayload struct {
	Text string `json:"text"`
}

type ReasoningPayload struct {
	Text string `json:"text"`
}

type TitlePayload struct {
	Title string `json:"title"`
}

type ToolUsePayload struct {
	Name string `json:"name"`
	Args string `json:"args"`
}

type ToolUseResultPayload struct {
	Name   string `json:"name,omitempty"`
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

type SubagentPayload struct {
	SessionID    string       `json:"session_id"`
	Prompt       string       `json:"prompt"`
	Output       string       `json:"output,omitempty"`
	UsagePayload UsagePayload `json:"usage,omitempty"`
}

type NotificationPayload struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

type CompactPayload struct {
	MessageCount int `json:"message_count"`
	TokenCount   int `json:"token_count"`
}

type StopPayload struct {
	Reason string `json:"reason"`
}

type PermissionPayload struct {
	ToolName string         `json:"tool_name"`
	Args     map[string]any `json:"args"`
	Verdict  string         `json:"verdict"` // "allow" | "deny"
}

type TaskPayload struct {
	TaskID string `json:"task_id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// UsagePayload is attached to EventStop and contains the total token usage
// across the session and all subagents it spawned, keyed by session ID.
type UsagePayload struct {
	Tokens map[string]Usage `json:"tokens"` // sessionID -> token breakdown
}
