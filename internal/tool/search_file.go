package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"my-go-agent/internal/config"
	"os"
	"path/filepath"
	"strings"
)

type SearchFile struct {
	maxResults int
}

type SearchFileArgs struct {
	Path       string `json:"path"`        // 搜索目录
	Pattern    string `json:"pattern"`     // 搜索的字符串
	FileFilter string `json:"file_filter"` // 文件过滤，如 "*.go"
	MaxResults int    `json:"max_results"` // 最大结果数，默认20
}

func NewSearchFileTool() *SearchFile {
	return &SearchFile{
		maxResults: config.ToolSearchMaxResults,
	}
}

func (s *SearchFile) Name() string {
	return "search_file"
}

func (s *SearchFile) Description() string {
	return "Search for specific text/keyword in files. Use this to find where code or text appears. For finding files by name, use glob."
}

func (s *SearchFile) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Relative directory path to search in (e.g. \"internal/agent\")",
			},
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "Text to search for",
			},
			"file_filter": map[string]interface{}{
				"type":        "string",
				"description": "File filter like '*.go' or '*.txt'",
			},
			"max_results": map[string]interface{}{
				"type":        "integer",
				"description": "Max results (default 20)",
			},
		},
		"required": []string{"path", "pattern"},
	}
}

func (s *SearchFile) Call(ctx context.Context, args string) (string, error) {
	var arg SearchFileArgs
	if err := json.Unmarshal([]byte(args), &arg); err != nil {
		return "", err
	}
	if arg.Path == "" {
		return "", fmt.Errorf("[search_file] path is empty")
	}
	if arg.Pattern == "" {
		return "", fmt.Errorf("[search_file] pattern is empty")
	}
	if arg.MaxResults <= 0 {
		arg.MaxResults = s.maxResults
	}

	var results []string
	count := 0

	err := filepath.Walk(arg.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 跳过错误
		}
		if info.IsDir() {
			return nil
		}
		// 检查文件过滤
		if arg.FileFilter != "" {
			matched, _ := filepath.Match(arg.FileFilter, info.Name())
			if !matched {
				return nil
			}
		}
		// 跳过二进制文件
		if info.Size() > 1024*1024 {
			return nil
		}

		// 搜索文件内容
		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if strings.Contains(line, arg.Pattern) {
				results = append(results, fmt.Sprintf("%s:%d: %s", path, lineNum, strings.TrimSpace(line)))
				count++
				if count >= arg.MaxResults {
					return filepath.SkipAll
				}
			}
		}
		return nil
	})

	if err != nil {
		return "", fmt.Errorf("[search_file] %w", err)
	}

	if len(results) == 0 {
		return "No matches found", nil
	}
	return strings.Join(results, "\n"), nil
}
