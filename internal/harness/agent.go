package harness

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/latent-9/souna-harness/internal/provider"
	"github.com/latent-9/souna-harness/internal/session"
	"github.com/latent-9/souna-harness/internal/tools"
)

// Harness drives the agent loop: provider call, tool execution, repeat.
// It emits events to a caller-owned channel and never imports a UI package.
type Harness struct {
	providers *provider.Registry
	tools     *tools.Registry
	sessions  *session.Store
	system    string

	// maxSubagentDepth caps subagent nesting.
	maxSubagentDepth int

	mu      sync.Mutex
	pending map[string]chan bool
}

// New builds a harness. system overrides the default system prompt.
func New(providers *provider.Registry, toolReg *tools.Registry, sessions *session.Store, system string) *Harness {
	if system == "" {
		system = defaultSystemPrompt
	}
	return &Harness{
		providers:        providers,
		tools:            toolReg,
		sessions:         sessions,
		system:           system,
		maxSubagentDepth: 2,
		pending:          make(map[string]chan bool),
	}
}

// defaultAllow are the tools that never require a permission prompt.
var defaultAllow = map[string]bool{
	"read": true,
	"grep": true,
	"glob": true,
}

const defaultSystemPrompt = `You are a coding assistant. Help the user accomplish their task ` +
	`using the available tools. Read files before editing them. Keep responses clear and concise. ` +
	`Prefer dedicated tools (read, edit, grep, glob) over shell commands when possible.`

// Run drives one agent turn for a session: it loops provider calls and tool
// executions until the model stops calling tools. The caller appends the
// user message to the session before calling Run. Run blocks until the turn
// completes or ctx is cancelled; it does not close the events channel.
func (h *Harness) Run(ctx context.Context, sessionID string, events chan<- Event) error {
	sess, err := h.sessions.Get(sessionID)
	if err != nil {
		return err
	}
	for {
		_, toolCalls, err := h.turn(ctx, sess, events)
		if err != nil {
			emit(ctx, events, Error{SessionID: sessionID, Err: err})
			return err
		}
		if len(toolCalls) == 0 {
			emit(ctx, events, TurnComplete{SessionID: sessionID})
			return nil
		}
	}
}

// turn runs one provider call plus the tool calls it produces.
func (h *Harness) turn(ctx context.Context, sess *session.Session, events chan<- Event) (string, []provider.ToolCall, error) {
	if !emit(ctx, events, MessageStart{SessionID: sess.ID, Agent: "main"}) {
		return "", nil, ctx.Err()
	}

	prov, err := h.providers.Get(sess.Provider)
	if err != nil {
		return "", nil, err
	}
	req := provider.StreamRequest{
		Model:    sess.Model,
		Messages: h.conversation(sess),
		Tools:    h.tools.Defs(),
	}
	chunks, err := prov.Stream(ctx, req)
	if err != nil {
		return "", nil, err
	}

	var text strings.Builder
	var calls []provider.ToolCall
	for chunk := range chunks {
		if chunk.Err != nil {
			return "", nil, chunk.Err
		}
		if chunk.Token != "" {
			text.WriteString(chunk.Token)
			if !emit(ctx, events, TokenDelta{SessionID: sess.ID, Text: chunk.Token}) {
				return "", nil, ctx.Err()
			}
		}
		if chunk.ToolCall != nil {
			calls = append(calls, *chunk.ToolCall)
		}
	}

	assistantMsg := provider.Message{Role: provider.RoleAssistant, Content: text.String(), ToolCalls: calls}
	if err := h.sessions.Append(sess.ID, assistantMsg); err != nil {
		return "", nil, err
	}
	if !emit(ctx, events, MessageComplete{SessionID: sess.ID, Text: text.String()}) {
		return "", nil, ctx.Err()
	}

	for _, call := range calls {
		output, execErr := h.execute(ctx, sess, call, events)
		toolMsg := provider.Message{Role: provider.RoleTool, Content: output, ToolCallID: call.ID}
		if execErr != nil {
			toolMsg.Content = fmt.Sprintf("error: %v\n%s", execErr, output)
		}
		if err := h.sessions.Append(sess.ID, toolMsg); err != nil {
			return "", nil, err
		}
		if !emit(ctx, events, ToolOutput{SessionID: sess.ID, Tool: call.Name, CallID: call.ID, Output: toolMsg.Content, IsError: execErr != nil}) {
			return "", nil, ctx.Err()
		}
	}
	return text.String(), calls, nil
}

// conversation builds the wire messages: system prompt plus history.
func (h *Harness) conversation(sess *session.Session) []provider.Message {
	msgs := make([]provider.Message, 0, len(sess.Messages)+1)
	msgs = append(msgs, provider.Message{Role: provider.RoleSystem, Content: h.system})
	msgs = append(msgs, sess.Messages...)
	return msgs
}

// execute runs one tool call after its permission check.
func (h *Harness) execute(ctx context.Context, sess *session.Session, call provider.ToolCall, events chan<- Event) (string, error) {
	tool, ok := h.tools.Get(call.Name)
	if !ok {
		return "", fmt.Errorf("unknown tool %q", call.Name)
	}
	if !defaultAllow[call.Name] {
		allowed, err := h.ask(ctx, sess.ID, call.Name, events)
		if err != nil {
			return "", err
		}
		if !allowed {
			return "", fmt.Errorf("permission denied for %q", call.Name)
		}
	}
	if !emit(ctx, events, ToolUseStart{SessionID: sess.ID, Tool: call.Name, CallID: call.ID}) {
		return "", ctx.Err()
	}
	return tool.Execute(ctx, json.RawMessage(call.Arguments))
}

// ask emits a permission request and blocks until Decide resolves it.
func (h *Harness) ask(ctx context.Context, sessionID, toolName string, events chan<- Event) (bool, error) {
	id, err := newRequestID()
	if err != nil {
		return false, err
	}
	ch := make(chan bool, 1)
	h.mu.Lock()
	h.pending[id] = ch
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.pending, id)
		h.mu.Unlock()
	}()

	if !emit(ctx, events, PermissionRequest{ID: id, SessionID: sessionID, Tool: toolName, Pattern: toolName}) {
		return false, ctx.Err()
	}

	select {
	case allowed := <-ch:
		emit(ctx, events, PermissionDecided{ID: id, Allowed: allowed})
		return allowed, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

// Decide resolves a pending permission request with the given ID.
func (h *Harness) Decide(reqID string, allowed bool) error {
	h.mu.Lock()
	ch, ok := h.pending[reqID]
	h.mu.Unlock()
	if !ok {
		return fmt.Errorf("no pending permission request %q", reqID)
	}
	ch <- allowed
	return nil
}

// emit sends an event, returning false when ctx is cancelled. It never
// drops events: the consumer must read the channel until Run returns.
func emit(ctx context.Context, events chan<- Event, e Event) bool {
	select {
	case events <- e:
		return true
	case <-ctx.Done():
		return false
	}
}

func newRequestID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
