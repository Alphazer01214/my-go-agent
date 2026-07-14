package tool

import (
	"context"
	"encoding/json"
	"fmt"
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
	return "read file"
}

func (r *ReadFile) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "relative path to file",
			},
			"max_read_size": map[string]interface{}{
				"type":        "integer",
				"description": "max read size",
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
	if arg.MaxReadSize > MaxReadSizeConst {
		arg.MaxReadSize = MaxReadSizeConst
	}
	if arg.Path == "" {
		return "", fmt.Errorf("[read_file] path is empty")
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
