package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"my-go-agent/internal/config"
	"os"
	"path/filepath"
	"strings"
)

type Glob struct {
	maxResults int
}

type GlobArgs struct {
	Pattern    string `json:"pattern"`     // glob 模式，如 "*.go" 或 "src/**/*.go"
	Root       string `json:"root"`        // 搜索根目录，默认当前目录
	MaxResults int    `json:"max_results"` // 最大结果数，默认50
}

func NewGlobTool() *Glob {
	return &Glob{
		maxResults: config.ToolGlobMaxResults,
	}
}

func (g *Glob) Name() string {
	return "glob"
}

func (g *Glob) Description() string {
	return "Find files by name pattern (e.g. *.go, **/*.txt). For searching text content inside files, use search_file."
}

func (g *Glob) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "Glob pattern like '*.go' or 'src/**/*.go'",
			},
			"root": map[string]interface{}{
				"type":        "string",
				"description": "Relative root directory (default: current dir, e.g. \"internal\")",
			},
			"max_results": map[string]interface{}{
				"type":        "integer",
				"description": "Max results (default 50)",
			},
		},
		"required": []string{"pattern"},
	}
}

func (g *Glob) Call(ctx context.Context, args string) (string, error) {
	var arg GlobArgs
	if err := json.Unmarshal([]byte(args), &arg); err != nil {
		return "", err
	}
	if arg.Pattern == "" {
		return "", fmt.Errorf("[glob] pattern is empty")
	}
	if arg.Root == "" {
		arg.Root = "."
	} else {
		arg.Root = NormalizePath(arg.Root)
	}
	if arg.MaxResults <= 0 {
		arg.MaxResults = g.maxResults
	}

	// 使用 filepath.Glob 处理简单模式
	// 对于 ** 模式需要特殊处理
	var results []string

	if strings.Contains(arg.Pattern, "**") {
		// 处理 ** 模式
		parts := strings.SplitN(arg.Pattern, "**", 2)
		baseDir := parts[0]
		suffix := ""
		if len(parts) > 1 {
			suffix = strings.TrimPrefix(parts[1], "/")
		}

		basePath := filepath.Join(arg.Root, baseDir)
		err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				return nil
			}
			if suffix != "" {
				matched, _ := filepath.Match(suffix, filepath.Base(path))
				if !matched {
					return nil
				}
			}
			relPath, _ := filepath.Rel(arg.Root, path)
			results = append(results, relPath)
			if len(results) >= arg.MaxResults {
				return filepath.SkipAll
			}
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("[glob] walk: %w", err)
		}
	} else {
		// 简单 glob 模式
		matches, err := filepath.Glob(filepath.Join(arg.Root, arg.Pattern))
		if err != nil {
			return "", fmt.Errorf("[glob] pattern error: %w", err)
		}
		for _, m := range matches {
			info, err := os.Stat(m)
			if err != nil || info.IsDir() {
				continue
			}
			relPath, _ := filepath.Rel(arg.Root, m)
			results = append(results, relPath)
			if len(results) >= arg.MaxResults {
				break
			}
		}
	}

	if len(results) == 0 {
		return "No files matched", nil
	}
	return strings.Join(results, "\n"), nil
}
