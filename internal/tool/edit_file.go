package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type EditFile struct{}

type EditFileArgs struct {
	Path      string `json:"path"`       // 文件路径
	OldString string `json:"old_string"` // 要替换的字符串
	NewString string `json:"new_string"` // 新字符串
	All       bool   `json:"all"`        // 是否替换所有，默认只替换第一个
}

func NewEditFileTool() *EditFile {
	return &EditFile{}
}

func (e *EditFile) Name() string {
	return "edit_file"
}

func (e *EditFile) Description() string {
	return "Edit EXISTING file by replacing specific text. Use this to modify parts of a file without rewriting the whole file. old_string must match exactly."
}

func (e *EditFile) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Relative file path from project root (e.g. \"internal/agent/agent.go\"). NEVER use absolute paths",
			},
			"old_string": map[string]interface{}{
				"type":        "string",
				"description": "Exact text to replace",
			},
			"new_string": map[string]interface{}{
				"type":        "string",
				"description": "New text to insert",
			},
			"all": map[string]interface{}{
				"type":        "boolean",
				"description": "Replace all occurrences (default false)",
			},
		},
		"required": []string{"path", "old_string", "new_string"},
	}
}

func (e *EditFile) Call(ctx context.Context, args string) (string, error) {
	var arg EditFileArgs
	if err := json.Unmarshal([]byte(args), &arg); err != nil {
		return "", err
	}
	if arg.Path == "" {
		return "", fmt.Errorf("[edit_file] path is empty")
	}
	arg.Path = NormalizePath(arg.Path)
	if arg.OldString == "" {
		return "", fmt.Errorf("[edit_file] old_string is empty")
	}

	// 读取文件
	data, err := os.ReadFile(arg.Path)
	if err != nil {
		return "", pathError("edit_file", arg.Path, err)
	}

	content := string(data)
	if !strings.Contains(content, arg.OldString) {
		return "", fmt.Errorf("[edit_file] old_string not found in file")
	}

	// 替换字符串
	var newContent string
	if arg.All {
		newContent = strings.ReplaceAll(content, arg.OldString, arg.NewString)
	} else {
		if strings.Count(content, arg.OldString) > 1 {
			return "", fmt.Errorf("[edit_file] old_string found multiple times in file")
		}
		newContent = strings.Replace(content, arg.OldString, arg.NewString, 1)
	}

	// 写回文件
	if err := os.WriteFile(arg.Path, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("[edit_file] write file: %w", err)
	}

	return fmt.Sprintf("Edited %s successfully", arg.Path), nil
}
