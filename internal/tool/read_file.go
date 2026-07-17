package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"my-go-agent/internal/config"
	"os"
	"strings"
)

// MaxReadSizeConst 绝对最大读取大小，使用 config 常量
const MaxReadSizeConst = config.ToolMaxReadSizeConst

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
				"description": "Relative file path from project root (e.g. \"internal/agent/agent.go\"). NEVER use absolute paths like \"/Go/...\" or \"E:\\...\"",
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
	if arg.MaxReadSize <= 0 {
		arg.MaxReadSize = config.ReadFileSize
	}
	if arg.MaxReadSize > MaxReadSizeConst {
		arg.MaxReadSize = MaxReadSizeConst
	}
	data, err := os.ReadFile(arg.Path)
	if err != nil {
		return "", pathError("read_file", arg.Path, err)
	}
	res := string(data)
	if len(res) > arg.MaxReadSize {
		res = res[:arg.MaxReadSize]
	}
	return res, nil
}

// pathError 生成带提示的路径错误信息
func pathError(tool, path string, err error) error {
	msg := fmt.Sprintf("[%s] %v", tool, err)
	if strings.HasPrefix(path, "/") || strings.Contains(path, ":\\") {
		msg += "\n⚠ You used an absolute path. Use RELATIVE paths from project root (e.g. \"internal/agent/agent.go\")"
	}
	msg += "\nTip: use list_dir to check available files"
	return fmt.Errorf("%s", msg)
}
