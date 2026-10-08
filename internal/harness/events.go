package harness

// Event is the seam between the harness and any UI. The harness emits
// events; consumers render them. The harness never imports a UI package.
type Event interface {
	eventKind() string
}

// MessageStart signals a new assistant message beginning.
type MessageStart struct {
	SessionID string
	Agent     string
}

// TokenDelta carries an incremental chunk of assistant text.
type TokenDelta struct {
	SessionID string
	Text      string
}

// MessageComplete carries the final text of an assistant message.
type MessageComplete struct {
	SessionID string
	Text      string
}

// ToolUseStart signals a tool call beginning.
type ToolUseStart struct {
	SessionID string
	Tool      string
	CallID    string
}

// ToolOutput carries a tool's result.
type ToolOutput struct {
	SessionID string
	Tool      string
	CallID    string
	Output    string
	IsError   bool
}

// PermissionRequest asks the consumer to allow or deny a tool call.
// The consumer replies through Harness.Decide with the same ID.
type PermissionRequest struct {
	ID        string
	SessionID string
	Tool      string
	Pattern   string
}

// PermissionDecided echoes a resolved permission request.
type PermissionDecided struct {
	ID      string
	Allowed bool
}

// AgentSpawned signals a subagent loop starting.
type AgentSpawned struct {
	ParentID string
	AgentID  string
	Name     string
}

// AgentProgress carries incremental output from a subagent.
type AgentProgress struct {
	AgentID string
	Text    string
}

// AgentCompleted carries a subagent's final result.
type AgentCompleted struct {
	AgentID string
	Result  string
}

// Error reports a harness failure for a session.
type Error struct {
	SessionID string
	Err       error
}

// TurnComplete signals the end of one agent turn.
type TurnComplete struct {
	SessionID string
}

func (MessageStart) eventKind() string      { return "message_start" }
func (TokenDelta) eventKind() string        { return "token_delta" }
func (MessageComplete) eventKind() string   { return "message_complete" }
func (ToolUseStart) eventKind() string      { return "tool_use_start" }
func (ToolOutput) eventKind() string        { return "tool_output" }
func (PermissionRequest) eventKind() string { return "permission_request" }
func (PermissionDecided) eventKind() string { return "permission_decided" }
func (AgentSpawned) eventKind() string      { return "agent_spawned" }
func (AgentProgress) eventKind() string     { return "agent_progress" }
func (AgentCompleted) eventKind() string    { return "agent_completed" }
func (Error) eventKind() string             { return "error" }
func (TurnComplete) eventKind() string      { return "turn_complete" }
