package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Edit replaces an exact string in a file. The old string must occur
// exactly once unless replace_all is set.
type Edit struct{}

func (Edit) Name() string { return "edit" }

func (Edit) Description() string {
	return "Edit a file by replacing an exact string. The old string must occur exactly " +
		"once unless replace_all is true."
}

func (Edit) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":        map[string]any{"type": "string", "description": "File to edit"},
			"old_string":  map[string]any{"type": "string", "description": "Exact text to find"},
			"new_string":  map[string]any{"type": "string", "description": "Replacement text"},
			"replace_all": map[string]any{"type": "boolean", "description": "Replace every occurrence"},
		},
		"required": []string{"path", "old_string", "new_string"},
	}
}

func (Edit) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Path       string `json:"path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("decode args: %w", err)
	}
	if in.OldString == in.NewString {
		return "", fmt.Errorf("old_string and new_string are identical")
	}
	data, err := os.ReadFile(in.Path)
	if err != nil {
		return "", err
	}
	content := string(data)
	count := strings.Count(content, in.OldString)
	if count == 0 {
		return "", fmt.Errorf("old_string not found in %s", in.Path)
	}
	if count > 1 && !in.ReplaceAll {
		return "", fmt.Errorf("old_string occurs %d times; include more context or set replace_all", count)
	}
	if in.ReplaceAll {
		content = strings.ReplaceAll(content, in.OldString, in.NewString)
	} else {
		content = strings.Replace(content, in.OldString, in.NewString, 1)
	}
	if err := os.WriteFile(in.Path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("edited %s", in.Path), nil
}
