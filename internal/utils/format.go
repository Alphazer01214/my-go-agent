package utils

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// ParseJSON 从 LLM 输出中提取并解析 JSON，支持以下格式：
// 1. 纯 JSON
// 2. ```json ``` 代码块包裹
// 3. 文本中嵌入的 JSON（提取第一个完整的 JSON 对象或数组）
func ParseJSON(content string, v interface{}) error {
	text := extractJSON(content)
	return json.Unmarshal([]byte(text), v)
}

func extractJSON(content string) string {
	text := strings.TrimSpace(content)

	// case 1: ```json ``` code block
	if idx := strings.Index(text, "```json"); idx != -1 {
		start := idx + len("```json")
		start = skipWhitespace(text, start)
		if end := strings.Index(text[start:], "```"); end != -1 {
			return strings.TrimSpace(text[start : start+end])
		}
	}

	// case 2: ``` ``` code block (no json label)
	if idx := strings.Index(text, "```"); idx != -1 {
		start := idx + len("```")
		start = skipNewline(text, start)
		if end := strings.Index(text[start:], "```"); end != -1 {
			candidate := strings.TrimSpace(text[start : start+end])
			if isJSON(candidate) {
				return candidate
			}
		}
	}

	// case 3: find first [...] or {...}
	if arr := findBalanced(text, '[', ']'); arr != "" {
		return arr
	}
	if obj := findBalanced(text, '{', '}'); obj != "" {
		return obj
	}

	return text
}

func skipWhitespace(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return i
}

func skipNewline(s string, i int) int {
	if i < len(s) && s[i] == '\r' {
		i++
	}
	if i < len(s) && s[i] == '\n' {
		i++
	}
	return i
}

func isJSON(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return false
	}
	return s[0] == '{' || s[0] == '['
}

// findBalanced 从文本中查找第一个完整配对的 open/close 包裹内容
func findBalanced(text string, open, close byte) string {
	start := strings.IndexByte(text, open)
	if start == -1 {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(text); i++ {
		c := text[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' {
			if inString {
				escaped = true
			}
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if c == open {
			depth++
		} else if c == close {
			depth--
			if depth == 0 {
				candidate := text[start : i+1]
				if json.Valid([]byte(candidate)) {
					return candidate
				}
				// 不合法则继续找下一个 open
				next := strings.IndexByte(text[i+1:], open)
				if next == -1 {
					return ""
				}
				return findBalanced(text[i+1+next:], open, close)
			}
		}
	}
	return ""
}

// NormalizeJSON 归一化 JSON 字符串，去除多余空格，用于重复调用检测
func NormalizeJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\t", "")
	s = strings.ReplaceAll(s, "  ", "")
	return s
}

// GenId 生成唯一 ID，格式为 name_timestamp_counter
var genIdCounter atomic.Uint64

func GenId(name string) string {
	n := genIdCounter.Add(1)
	return fmt.Sprintf("%s_%d_%d", name, time.Now().UnixMilli(), n)
}
