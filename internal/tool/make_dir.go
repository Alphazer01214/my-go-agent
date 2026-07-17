package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

type MakeDir struct{}

type MakeDirArgs struct {
	Path    string `json:"path"`              // 目录路径
	Parents bool   `json:"parents,omitempty"` // 是否创建父目录（类似 mkdir -p）
}

func NewMakeDirTool() *MakeDir {
	return &MakeDir{}
}

func (m *MakeDir) Name() string {
	return "make_dir"
}

func (m *MakeDir) Description() string {
	return "Create a new directory. Use parents=true to create parent directories as needed (like mkdir -p)."
}

func (m *MakeDir) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Relative directory path from project root (e.g. \"internal/newpkg\")",
			},
			"parents": map[string]interface{}{
				"type":        "boolean",
				"description": "Create parent directories if they don't exist (default false)",
			},
		},
		"required": []string{"path"},
	}
}

func (m *MakeDir) Call(ctx context.Context, args string) (string, error) {
	var arg MakeDirArgs
	if err := json.Unmarshal([]byte(args), &arg); err != nil {
		return "", err
	}
	if arg.Path == "" {
		return "", fmt.Errorf("[make_dir] path is empty")
	}
	arg.Path = NormalizePath(arg.Path)

	// 检查是否已存在
	info, err := os.Stat(arg.Path)
	if err == nil {
		if info.IsDir() {
			return fmt.Sprintf("Directory %s already exists", arg.Path), nil
		}
		return "", fmt.Errorf("[make_dir] %s already exists and is not a directory", arg.Path)
	}

	if arg.Parents {
		if err := os.MkdirAll(arg.Path, 0755); err != nil {
			return "", fmt.Errorf("[make_dir] create directory: %w", err)
		}
	} else {
		if err := os.Mkdir(arg.Path, 0755); err != nil {
			return "", fmt.Errorf("[make_dir] create directory: %w", err)
		}
	}

	return fmt.Sprintf("Created directory %s", arg.Path), nil
}
