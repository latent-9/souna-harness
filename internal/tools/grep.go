package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Grep searches file contents with a regular expression.
type Grep struct{}

func (Grep) Name() string { return "grep" }

func (Grep) Description() string {
	return "Search file contents with a regular expression. Returns matching file paths, " +
		"line numbers, and line previews."
}

func (Grep) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{"type": "string", "description": "Regular expression to match"},
			"path":    map[string]any{"type": "string", "description": "File or directory to search"},
			"include": map[string]any{"type": "string", "description": "Glob filter for files (e.g. *.go)"},
		},
		"required": []string{"pattern"},
	}
}

func (Grep) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
		Include string `json:"include"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("decode args: %w", err)
	}
	if in.Path == "" {
		in.Path = "."
	}
	re, err := regexp.Compile(in.Pattern)
	if err != nil {
		return "", fmt.Errorf("compile pattern: %w", err)
	}

	info, err := os.Stat(in.Path)
	if err != nil {
		return "", err
	}
	var files []string
	if !info.IsDir() {
		files = []string{in.Path}
	} else {
		err = filepath.WalkDir(in.Path, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if name := d.Name(); name == ".git" || name == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if in.Include != "" {
				ok, err := filepath.Match(in.Include, d.Name())
				if err != nil || !ok {
					return nil
				}
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			return "", err
		}
		sort.Strings(files)
	}

	var b strings.Builder
	matches := 0
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			if re.Match(scanner.Bytes()) {
				matches++
				fmt.Fprintf(&b, "%s:%d: %s\n", file, lineNo, scanner.Text())
			}
		}
		f.Close()
	}
	if matches == 0 {
		return "no matches", nil
	}
	return b.String(), nil
}
