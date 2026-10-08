package harness

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/latent-9/souna-harness/internal/provider"
	"github.com/latent-9/souna-harness/internal/tools"
)

// Spawn runs a child agent loop in a goroutine with its own session.
// The child inherits the parent's provider and model. Child events are
// surfaced as AgentSpawned, AgentProgress, and AgentCompleted.
func (h *Harness) Spawn(ctx context.Context, parentID, agentName, prompt string, events chan<- Event) (string, error) {
	parent, err := h.sessions.Get(parentID)
	if err != nil {
		return "", err
	}
	depth, err := h.sessions.Depth(parentID)
	if err != nil {
		return "", err
	}
	if depth >= h.maxSubagentDepth {
		return "", fmt.Errorf("subagent depth limit reached (%d). Solve the task yourself.", h.maxSubagentDepth)
	}
	child, err := h.sessions.Create(parent.Title+" @"+agentName+" subagent", parent.Provider, parent.Model, parentID)
	if err != nil {
		return "", err
	}
	if err := h.sessions.Append(child.ID, provider.Message{Role: provider.RoleUser, Content: prompt}); err != nil {
		return "", err
	}
	if !emit(ctx, events, AgentSpawned{ParentID: parentID, AgentID: child.ID, Name: agentName}) {
		return "", ctx.Err()
	}

	go func() {
		childEvents := make(chan Event, 64)
		done := make(chan error, 1)
		go func() { done <- h.Run(ctx, child.ID, childEvents) }()

		var result string
		forward := func(ev Event) {
			switch e := ev.(type) {
			case TokenDelta:
				emit(ctx, events, AgentProgress{AgentID: child.ID, Text: e.Text})
			case MessageComplete:
				result = e.Text
			}
		}
		for {
			select {
			case ev := <-childEvents:
				forward(ev)
				continue
			case err := <-done:
				for {
					select {
					case ev := <-childEvents:
						forward(ev)
						continue
					default:
					}
					break
				}
				if err != nil {
					emit(ctx, events, Error{SessionID: child.ID, Err: err})
				}
				emit(ctx, events, AgentCompleted{AgentID: child.ID, Result: result})
				return
			}
		}
	}()
	return child.ID, nil
}

// taskTool is the model-facing subagent tool. It runs a child agent loop to
// completion and returns the child's final text as the tool result.
type taskTool struct {
	h *Harness
}

func (h *Harness) TaskTool() tools.Tool { return taskTool{h: h} }

func (taskTool) Name() string { return "task" }

func (taskTool) Description() string {
	return "Spawn a subagent to research or execute a task. Give detailed instructions; " +
		"the subagent returns a single message back to you."
}

func (taskTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prompt":        map[string]any{"type": "string", "description": "The task for the subagent"},
			"subagent_name": map[string]any{"type": "string", "description": "Name for the subagent session"},
		},
		"required": []string{"prompt"},
	}
}

func (t taskTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Prompt       string `json:"prompt"`
		SubagentName string `json:"subagent_name"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("decode args: %w", err)
	}
	if in.Prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}
	// The caller's session ID is carried on ctx by the TUI; subagents spawned
	// by the model nest under the session running the loop.
	sessionID, ok := ctx.Value(sessionIDKey{}).(string)
	if !ok || sessionID == "" {
		return "", fmt.Errorf("no session in context")
	}

	parent, err := t.h.sessions.Get(sessionID)
	if err != nil {
		return "", err
	}
	depth, err := t.h.sessions.Depth(sessionID)
	if err != nil {
		return "", err
	}
	if depth >= t.h.maxSubagentDepth {
		return "", fmt.Errorf("subagent depth limit reached (%d). Solve the task yourself.", t.h.maxSubagentDepth)
	}
	child, err := t.h.sessions.Create(parent.Title+" @"+in.SubagentName+" subagent", parent.Provider, parent.Model, sessionID)
	if err != nil {
		return "", err
	}
	if err := t.h.sessions.Append(child.ID, provider.Message{Role: provider.RoleUser, Content: in.Prompt}); err != nil {
		return "", err
	}

	childEvents := make(chan Event, 64)
	done := make(chan error, 1)
	go func() { done <- t.h.Run(ctx, child.ID, childEvents) }()

	var result string
	for {
		select {
		case ev := <-childEvents:
			if mc, ok := ev.(MessageComplete); ok {
				result = mc.Text
			}
			continue
		case err := <-done:
			for {
				select {
				case ev := <-childEvents:
					if mc, ok := ev.(MessageComplete); ok {
						result = mc.Text
					}
					continue
				default:
				}
				break
			}
			if err != nil {
				return "", err
			}
			return result, nil
		}
	}
}

// sessionIDKey carries the session ID on tool execution contexts.
type sessionIDKey struct{}

// WithSessionID returns a context carrying the session ID for tool execution.
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDKey{}, sessionID)
}
