package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Read reads a file, one line number per line prefix.
type Read struct{}

func (Read) Name() string { return "read" }

func (Read) Description() string {
	return "Read the contents of a file. Each line is prefixed by its 1-based line number."
}

func (Read) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":   map[string]any{"type": "string", "description": "File to read"},
			"offset": map[string]any{"type": "integer", "description": "Line to start reading from (1-based)"},
			"limit":  map[string]any{"type": "integer", "description": "Maximum number of lines to read"},
		},
		"required": []string{"path"},
	}
}

func (Read) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("decode args: %w", err)
	}
	data, err := os.ReadFile(in.Path)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	start := in.Offset
	if start < 1 {
		start = 1
	}
	if start > len(lines) {
		return "", fmt.Errorf("offset %d beyond end of file (%d lines)", start, len(lines))
	}
	end := len(lines)
	if in.Limit > 0 && start-1+in.Limit < end {
		end = start - 1 + in.Limit
	}
	var b strings.Builder
	for i := start; i <= end; i++ {
		fmt.Fprintf(&b, "%d: %s\n", i, lines[i-1])
	}
	return b.String(), nil
}
