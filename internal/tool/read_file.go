package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"my-go-agent/internal/config"
	"os"
)

const MaxReadSizeConst = 1024 * 1024

type ReadFile struct {
	MaxMaxReadSize int
}

type ReadFileArgs struct {
	MaxReadSize int    `json:"max_read_size"`
	Path        string `json:"path"`
}

func NewReadFileTool(maxReadSize int) *ReadFile {
	return &ReadFile{
		MaxMaxReadSize: maxReadSize,
	}
}

func (r *ReadFile) Name() string {
	return "read_file"
}

func (r *ReadFile) Description() string {
	return "Read file content. Always use max_read_size to limit output and save tokens."
}

func (r *ReadFile) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Relative file path (e.g. \"internal/agent/agent.go\")",
			},
			"max_read_size": map[string]interface{}{
				"type":        "integer",
				"description": "Max bytes to read (default 4096, max 1MB)",
			},
		},
		"required": []string{"path"},
	}
}

func (r *ReadFile) Call(ctx context.Context, args string) (string, error) {
	var arg ReadFileArgs
	if err := json.Unmarshal([]byte(args), &arg); err != nil {
		return "", err
	}
	if arg.Path == "" {
		return "", fmt.Errorf("[read_file] path is empty")
	}
	arg.Path = NormalizePath(arg.Path)
	// 默认限制 4KB，避免 token 浪费
	if arg.MaxReadSize <= 0 {
		arg.MaxReadSize = config.ReadFileSize
	}
	if arg.MaxReadSize > MaxReadSizeConst {
		arg.MaxReadSize = MaxReadSizeConst
	}
	data, err := os.ReadFile(arg.Path)
	if err != nil {
		return "", fmt.Errorf("[read_file] %w", err)
	}
	res := string(data)
	if len(res) > arg.MaxReadSize {
		res = res[:arg.MaxReadSize]
	}
	return res, nil
}
