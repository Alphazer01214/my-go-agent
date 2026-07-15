package agent

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/memory"
	"my-go-agent/internal/utils"
	"path/filepath"
	"strings"
)

//go:embed template/code_review.tmpl
var codeReviewTmpl string

type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

type Issue struct {
	File       string      `json:"file"`
	Line       json.Number `json:"line,omitempty"`
	Message    string      `json:"message"`
	Suggestion string      `json:"suggestion"`
	Severity   Severity    `json:"severity"`
	Category   string      `json:"category"`
}

type ReviewResult struct {
	Summary string  `json:"summary"`
	Issues  []Issue `json:"issues"`
}

// ReviewAgent 代码审查 agent，重写 Run 实现专用审查逻辑
type ReviewAgent struct {
	*BaseAgent
}

func NewReviewAgent(model llm.ChatModel) *ReviewAgent {
	base := NewBaseAgent("code_review", model, nil)
	base.SysPrompt = "你是一个专业的代码审查助手，负责审查代码并提供反馈，只能输出JSON格式的反馈"
	return &ReviewAgent{BaseAgent: base}
}

func (a *ReviewAgent) Run(ctx context.Context, session *memory.Session, prompt string) (string, error) {
	filename, code := parseReviewInput(prompt)

	tmpl, err := newTemplate("code-review", codeReviewTmpl)
	if err != nil {
		return "", fmt.Errorf("[review_agent] template: %w", err)
	}

	promptText, err := tmpl.Render(map[string]string{
		"Filename": filename,
		"Lang":     detectLang(filename),
		"Code":     code,
	})
	if err != nil {
		return "", fmt.Errorf("[review_agent] render: %w", err)
	}

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			llm.SystemMessage(a.SysPrompt),
			llm.UserMessage(promptText),
		},
	}
	resp, err := a.Model.Chat(ctx, req)
	if err != nil {
		return "", fmt.Errorf("[review_agent] chat: %w", err)
	}

	var result ReviewResult
	if err := utils.ParseJSON(resp.Content, &result); err != nil {
		return "", fmt.Errorf("[review_agent] parse: %w", err)
	}

	return formatReviewResult(&result), nil
}

// AsExpert 将 ReviewAgent 包装为 Expert，可接入 PlanAgent 或 ExpertManager
func (a *ReviewAgent) AsExpert() *Expert {
	return &Expert{
		Name:        "code_review",
		Description: "审查代码质量，发现 bug、安全漏洞、性能问题、风格和可维护性问题。输入格式：文件路径和代码内容。",
		Agent:       a,
	}
}

func parseReviewInput(input string) (filename, code string) {
	lines := strings.SplitN(input, "\n", 2)
	if len(lines) == 2 && !strings.HasPrefix(lines[0], "package ") && !strings.HasPrefix(lines[0], "import ") {
		return lines[0], lines[1]
	}
	return "unknown", input
}

func formatReviewResult(r *ReviewResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Summary: %s\n", r.Summary))
	for _, issue := range r.Issues {
		sb.WriteString(fmt.Sprintf("\n[%s] %s line %s\n  %s\n  Suggestion: %s\n  Category: %s",
			issue.Severity, issue.File, issue.Line, issue.Message, issue.Suggestion, issue.Category))
	}
	return sb.String()
}

func detectLang(filename string) string {
	ext := strings.TrimPrefix(filepath.Ext(filename), ".")
	if ext == "" {
		return "txt"
	}
	return ext
}

// 简单内联 template 实现，避免引入外部依赖

type simpleTemplate struct {
	name string
	text string
}

func newTemplate(name, text string) (*simpleTemplate, error) {
	return &simpleTemplate{name: name, text: text}, nil
}

func (t *simpleTemplate) Render(data map[string]string) (string, error) {
	result := t.text
	for k, v := range data {
		result = strings.ReplaceAll(result, "{{."+k+"}}", v)
	}
	return result, nil
}
