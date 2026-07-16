package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// QuestionTable 交互式问答工具
// Agent 通过此工具向用户提问，获取用户选择或输入
type QuestionTable struct {
	reader io.Reader
	writer io.Writer
}

func NewQuestionTableTool() *QuestionTable {
	return &QuestionTable{
		reader: os.Stdin,
		writer: os.Stdout,
	}
}

// NewQuestionTableWithIO 创建自定义输入输出的 QuestionTable（用于测试）
func NewQuestionTableWithIO(reader io.Reader, writer io.Writer) *QuestionTable {
	return &QuestionTable{
		reader: reader,
		writer: writer,
	}
}

func (q *QuestionTable) Name() string {
	return "question_table"
}

func (q *QuestionTable) Description() string {
	return "Ask the user a question and get their response. Use this when you need user input, want them to choose between options, or need confirmation before proceeding."
}

func (q *QuestionTable) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"question": map[string]interface{}{
				"type":        "string",
				"description": "The question to ask the user",
			},
			"options": map[string]interface{}{
				"type":        "array",
				"description": "List of options for the user to choose from. If empty, user can type freely.",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"label": map[string]interface{}{
							"type":        "string",
							"description": "Short display label for the option",
						},
						"description": map[string]interface{}{
							"type":        "string",
							"description": "Detailed description of what this option means",
						},
					},
					"required": []string{"label"},
				},
			},
		},
		"required": []string{"question"},
	}
}

type questionArgs struct {
	Question string         `json:"question"`
	Options  []optionEntry  `json:"options,omitempty"`
}

type optionEntry struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

func (q *QuestionTable) Call(ctx context.Context, args string) (string, error) {
	var arg questionArgs
	if err := json.Unmarshal([]byte(args), &arg); err != nil {
		return "", fmt.Errorf("[question_table] invalid args: %w", err)
	}

	if arg.Question == "" {
		return "", fmt.Errorf("[question_table] question is required")
	}

	// 检查 context 是否已取消
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	// 显示问题
	fmt.Fprintf(q.writer, "\n  ❓ %s\n", arg.Question)

	if len(arg.Options) > 0 {
		return q.handleOptions(arg.Options)
	}
	return q.handleFreeInput()
}

// handleOptions 处理有选项的提问
func (q *QuestionTable) handleOptions(options []optionEntry) (string, error) {
	// 显示选项
	for i, opt := range options {
		if opt.Description != "" {
			fmt.Fprintf(q.writer, "  [%d] %s — %s\n", i+1, opt.Label, opt.Description)
		} else {
			fmt.Fprintf(q.writer, "  [%d] %s\n", i+1, opt.Label)
		}
	}
	fmt.Fprintf(q.writer, "  [0] Other (type your own answer)\n")
	fmt.Fprintf(q.writer, "  > ")

	scanner := bufio.NewScanner(q.reader)
	if !scanner.Scan() {
		return "", fmt.Errorf("no input")
	}
	input := strings.TrimSpace(scanner.Text())

	// 尝试解析为数字选择
	if input != "" && input != "0" {
		for i, opt := range options {
			// 匹配数字
			if fmt.Sprintf("%d", i+1) == input {
				result := map[string]interface{}{
					"choice": opt.Label,
					"index":  i + 1,
				}
				out, _ := json.Marshal(result)
				return string(out), nil
			}
			// 匹配标签（大小写不敏感）
			if strings.EqualFold(opt.Label, input) {
				result := map[string]interface{}{
					"choice": opt.Label,
					"index":  i + 1,
				}
				out, _ := json.Marshal(result)
				return string(out), nil
			}
		}
	}

	// 用户输入了自定义内容
	if input == "0" {
		fmt.Fprintf(q.writer, "  Your answer: ")
		if !scanner.Scan() {
			return "", fmt.Errorf("no input")
		}
		input = strings.TrimSpace(scanner.Text())
	}

	if input == "" {
		return "", fmt.Errorf("no input provided")
	}

	result := map[string]interface{}{
		"choice":  "custom",
		"answer":  input,
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}

// handleFreeInput 处理自由输入的提问
func (q *QuestionTable) handleFreeInput() (string, error) {
	fmt.Fprintf(q.writer, "  Your answer: ")

	scanner := bufio.NewScanner(q.reader)
	if !scanner.Scan() {
		return "", fmt.Errorf("no input")
	}
	input := strings.TrimSpace(scanner.Text())

	if input == "" {
		return "", fmt.Errorf("no input provided")
	}

	result := map[string]interface{}{
		"answer": input,
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}
