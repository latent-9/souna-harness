package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// Bash runs a shell command.
type Bash struct {
	// Timeout caps each command's runtime. Zero means 30 seconds.
	Timeout time.Duration
}

func (Bash) Name() string { return "bash" }

func (Bash) Description() string {
	return "Run a shell command and return its combined output. " +
		"Prefer dedicated tools (read, edit, grep, glob) when they can do the job."
}

func (Bash) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command":     map[string]any{"type": "string", "description": "The shell command to execute"},
			"workdir":     map[string]any{"type": "string", "description": "Working directory (optional)"},
			"timeout_sec": map[string]any{"type": "integer", "description": "Timeout in seconds (optional)"},
		},
		"required": []string{"command"},
	}
}

func (b Bash) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Command    string `json:"command"`
		Workdir    string `json:"workdir"`
		TimeoutSec int    `json:"timeout_sec"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("decode args: %w", err)
	}
	if in.Command == "" {
		return "", fmt.Errorf("command is required")
	}
	timeout := b.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	if in.TimeoutSec > 0 {
		timeout = time.Duration(in.TimeoutSec) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", in.Command)
	if in.Workdir != "" {
		cmd.Dir = in.Workdir
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	result := out.String()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return result, fmt.Errorf("command timed out after %s", timeout)
		}
		return result, fmt.Errorf("exit: %w", err)
	}
	return result, nil
}
