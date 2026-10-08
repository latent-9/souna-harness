package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// OpenAI is an OpenAI-compatible streaming client. It works with any
// server exposing /chat/completions: NVIDIA NIM, OpenRouter, Groq,
// ollama, and others.
type OpenAI struct {
	name    string
	baseURL string
	apiKey  string
	models  []string
	client  *http.Client
}

// NewOpenAI builds an OpenAI-compatible provider.
func NewOpenAI(name string, pc ProviderConfig) (Provider, error) {
	return &OpenAI{
		name:    name,
		baseURL: strings.TrimRight(pc.BaseURL, "/"),
		apiKey:  ResolveKey(pc.APIKey),
		models:  pc.Models,
		client:  &http.Client{},
	}, nil
}

func (o *OpenAI) Name() string     { return o.name }
func (o *OpenAI) Models() []string { return o.models }

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []wireMessage `json:"messages"`
	Tools    []wireTool    `json:"tools,omitempty"`
	Stream   bool          `json:"stream"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
}

type wireToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Index    int          `json:"index"`
	Function wireFunction `json:"function"`
}

type wireFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Arguments   string          `json:"arguments"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type wireTool struct {
	Type     string       `json:"type"`
	Function wireFunction `json:"function"`
}

type chatChunk struct {
	Choices []struct {
		Delta struct {
			Content   string         `json:"content"`
			ToolCalls []wireToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func (o *OpenAI) toWire(req StreamRequest) chatRequest {
	msgs := make([]wireMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		wm := wireMessage{Role: string(m.Role), Content: m.Content, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			wm.ToolCalls = append(wm.ToolCalls, wireToolCall{
				ID:       tc.ID,
				Type:     "function",
				Function: wireFunction{Name: tc.Name, Arguments: tc.Arguments},
			})
		}
		msgs = append(msgs, wm)
	}
	tools := make([]wireTool, 0, len(req.Tools))
	for _, t := range req.Tools {
		tools = append(tools, wireTool{
			Type:     "function",
			Function: wireFunction{Name: t.Name, Description: t.Description},
		})
	}
	// The schema field is dropped from wireFunction here; encode it separately.
	for i, t := range req.Tools {
		if schema, err := json.Marshal(t.Schema); err == nil {
			tools[i].Function.Parameters = json.RawMessage(schema)
		}
	}
	return chatRequest{Model: req.Model, Messages: msgs, Tools: tools, Stream: true}
}

// Stream issues a streaming completion and returns a channel of chunks.
// The channel closes when the stream ends.
func (o *OpenAI) Stream(ctx context.Context, req StreamRequest) (<-chan StreamChunk, error) {
	body, err := json.Marshal(o.toWire(req))
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(buf.String()))
	}

	out := make(chan StreamChunk)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		acc := newToolAccumulator()
		send := func(c StreamChunk) bool {
			select {
			case out <- c:
				return true
			case <-ctx.Done():
				return false
			}
		}
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}
			var chunk chatChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				if !send(StreamChunk{Err: fmt.Errorf("decode chunk: %w", err)}) {
					return
				}
				continue
			}
			for _, choice := range chunk.Choices {
				if choice.Delta.Content != "" && !send(StreamChunk{Token: choice.Delta.Content}) {
					return
				}
				for _, tc := range choice.Delta.ToolCalls {
					acc.add(tc)
				}
			}
		}
		for _, tc := range acc.finish() {
			if !send(StreamChunk{ToolCall: &tc}) {
				return
			}
		}
		send(StreamChunk{Done: true})
	}()
	return out, nil
}

// toolAccumulator merges index-keyed tool call deltas into complete calls.
type toolAccumulator struct {
	byIndex map[int]*wireToolCall
}

func newToolAccumulator() *toolAccumulator {
	return &toolAccumulator{byIndex: make(map[int]*wireToolCall)}
}

func (a *toolAccumulator) add(tc wireToolCall) {
	cur, ok := a.byIndex[tc.Index]
	if !ok {
		cur = &wireToolCall{ID: tc.ID, Type: tc.Type, Function: wireFunction{Name: tc.Function.Name}}
		a.byIndex[tc.Index] = cur
	}
	if tc.ID != "" {
		cur.ID = tc.ID
	}
	if tc.Function.Name != "" {
		cur.Function.Name = tc.Function.Name
	}
	cur.Function.Arguments += tc.Function.Arguments
}

func (a *toolAccumulator) finish() []ToolCall {
	indexes := make([]int, 0, len(a.byIndex))
	for idx := range a.byIndex {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)
	calls := make([]ToolCall, 0, len(indexes))
	for _, idx := range indexes {
		tc := a.byIndex[idx]
		if tc.Function.Name == "" {
			continue
		}
		args := tc.Function.Arguments
		if args == "" {
			args = "{}"
		}
		calls = append(calls, ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: args})
	}
	return calls
}
