package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type WriteFile struct{}

type WriteFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Append  bool   `json:"append"` // 是否追加模式
}

func NewWriteFileTool() *WriteFile {
	return &WriteFile{}
}

func (w *WriteFile) Name() string {
	return "write_file"
}

func (w *WriteFile) Description() string {
	return "Create NEW file or completely REWRITE file. For editing parts of existing file, use edit_file instead."
}

func (w *WriteFile) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "File path to write",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Content to write",
			},
			"append": map[string]interface{}{
				"type":        "boolean",
				"description": "Append to file instead of overwrite (default false)",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (w *WriteFile) Call(ctx context.Context, args string) (string, error) {
	var arg WriteFileArgs
	if err := json.Unmarshal([]byte(args), &arg); err != nil {
		return "", err
	}
	if arg.Path == "" {
		return "", fmt.Errorf("[write_file] path is empty")
	}

	// 确保目录存在
	dir := filepath.Dir(arg.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("[write_file] create directory: %w", err)
	}

	// 写入文件
	flag := os.O_WRONLY | os.O_CREATE
	if arg.Append {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}

	f, err := os.OpenFile(arg.Path, flag, 0644)
	if err != nil {
		return "", fmt.Errorf("[write_file] open file: %w", err)
	}
	defer f.Close()

	n, err := f.WriteString(arg.Content)
	if err != nil {
		return "", fmt.Errorf("[write_file] write: %w", err)
	}

	return fmt.Sprintf("Written %d bytes to %s", n, arg.Path), nil
}
