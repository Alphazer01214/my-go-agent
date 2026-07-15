package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

type ListDir struct{}

type ListDirArgs struct {
	Path string `json:"path"`
}

func NewListDirTool() *ListDir {
	return &ListDir{}
}

func (l *ListDir) Name() string {
	return "list_dir"
}

func (l *ListDir) Description() string {
	return "List directory contents. Returns [DIR] for folders, [FILE] with size for files."
}

func (l *ListDir) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Relative directory path (e.g. \"internal/agent\" or \".\")",
			},
		},
		"required": []string{"path"},
	}
}

func (l *ListDir) Call(ctx context.Context, args string) (string, error) {
	var arg ListDirArgs
	if err := json.Unmarshal([]byte(args), &arg); err != nil {
		return "", err
	}
	if arg.Path == "" {
		return "", fmt.Errorf("[list_dir] path is empty")
	}
	entries, err := os.ReadDir(arg.Path)
	if err != nil {
		return "", fmt.Errorf("[list_dir] %w", err)
	}
	result := ""
	for _, entry := range entries {
		if entry.IsDir() {
			result += fmt.Sprintf("[DIR]  %s\n", entry.Name())
		} else {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			result += fmt.Sprintf("[FILE] %s (%d bytes)\n", entry.Name(), info.Size())
		}
	}
	if result == "" {
		return "(empty directory)", nil
	}
	return result, nil
}
