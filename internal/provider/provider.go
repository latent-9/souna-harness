package provider

import (
	"context"
)

// Role is a conversation message role.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ToolCall is a complete tool call requested by the model.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// Message is one conversation turn.
type Message struct {
	Role       Role
	Content    string
	ToolCallID string
	ToolCalls  []ToolCall
}

// ToolDef describes a tool to the provider.
type ToolDef struct {
	Name        string
	Description string
	Schema      map[string]any
}

// StreamRequest is one completion request.
type StreamRequest struct {
	Model    string
	Messages []Message
	Tools    []ToolDef
}

// StreamChunk is one chunk of a streaming completion. Providers accumulate
// tool-call deltas internally and emit complete ToolCalls.
type StreamChunk struct {
	Token    string
	ToolCall *ToolCall
	Err      error
	Done     bool
}

// Provider streams completions from an LLM API.
type Provider interface {
	Name() string
	Models() []string
	Stream(ctx context.Context, req StreamRequest) (<-chan StreamChunk, error)
}
