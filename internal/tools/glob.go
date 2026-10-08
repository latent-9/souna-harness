package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// Glob matches file paths against a shell glob pattern.
type Glob struct{}

func (Glob) Name() string { return "glob" }

func (Glob) Description() string {
	return "Match file paths against a glob pattern (e.g. src/**/*.go)."
}

func (Glob) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{"type": "string", "description": "Glob pattern"},
			"path":    map[string]any{"type": "string", "description": "Directory to search in"},
		},
		"required": []string{"pattern"},
	}
}

func (Glob) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("decode args: %w", err)
	}
	if in.Path == "" {
		in.Path = "."
	}
	matches, err := filepath.Glob(filepath.Join(in.Path, in.Pattern))
	if err != nil {
		return "", fmt.Errorf("glob: %w", err)
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return "no matches", nil
	}
	var b strings.Builder
	for _, m := range matches {
		b.WriteString(m + "\n")
	}
	return b.String(), nil
}
