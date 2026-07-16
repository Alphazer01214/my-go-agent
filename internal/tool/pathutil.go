package tool

import (
	"os"
	"path/filepath"
	"strings"
)

// NormalizePath 将文件路径规范化为相对路径
// 处理 LLM 常见的路径错误：
//   - 绝对路径 → 尝试转为相对路径
//   - Windows 反斜杠 → 统一为正斜杠
//   - 路径前后空格 → 去除
func NormalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}

	// 统一路径分隔符为正斜杠
	path = filepath.ToSlash(path)

	// 如果已经是相对路径且不以 / 开头，直接返回
	if !filepath.IsAbs(path) && !strings.HasPrefix(path, "/") {
		return path
	}

	// 绝对路径 → 尝试转为相对路径
	cwd, err := os.Getwd()
	if err != nil {
		return path
	}

	// 尝试直接转相对路径
	rel, err := filepath.Rel(cwd, path)
	if err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}

	// 如果绝对路径包含 cwd 作为前缀，截取后半部分
	cwdNorm := filepath.ToSlash(cwd)
	pathNorm := filepath.ToSlash(path)
	if strings.HasPrefix(pathNorm, cwdNorm+"/") {
		return strings.TrimPrefix(pathNorm, cwdNorm+"/")
	}

	// 尝试从路径末尾匹配项目结构
	// 例如 /Go/my-go-agent/internal/agent/agent.go → internal/agent/agent.go
	parts := strings.Split(pathNorm, "/")
	for i, part := range parts {
		// 查找常见的项目目录名
		if part == "internal" || part == "cmd" || part == "pkg" || part == "src" || part == "go.mod" {
			return strings.Join(parts[i:], "/")
		}
	}

	// 最后手段：返回原始路径
	return path
}

// ResolvePath 解析路径，返回绝对路径（用于实际文件操作）
// 如果是相对路径，基于 cwd 解析
func ResolvePath(path string) (string, error) {
	path = NormalizePath(path)
	if filepath.IsAbs(path) {
		return path, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return path, err
	}
	return filepath.Join(cwd, path), nil
}
