package review

import (
	"context"
	"my-go-agent/internal/llm"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
)

const sampleCode = `
package review

import (
	"context"
	_ "embed"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/prompts"
	"my-go-agent/internal/utils"
	"path/filepath"
	"strings"
)

//go:embed template/code_review.tmpl
var codeReviewPrompt string

var codeReviewSystemPrompt string = "你是一个专业的代码审查助手，负责审查代码并提供反馈，只能输出JSON格式的反馈"

type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

type Issue struct {
	File       string   
	Line       string   
	Message    string  
	Suggestion string  
	Severity   Severity 
	Category   string  // 问题分类，例如："style", "performance", "error"
}

type Result struct {
	Summary string 
	Issues  []Issue 
}

type Reviewer struct {
	model llm.ChatModel
	tmpl  *prompts.Template
}

type reviewInput struct {
	Filename string
	Code     string
	Lang     string
}

func NewReviewer(model llm.ChatModel) *Reviewer {
	tmpl, err := prompts.NewTemplate("code-review", codeReviewPrompt)
	if err != nil {
		return nil
	}

	return &Reviewer{
		model: model,
		tmpl:  tmpl,
	}
}

func (r *Reviewer) Review(ctx context.Context, filename string, code string) (*Result, error) {
	prompt, err := r.tmpl.Render(reviewInput{
		Filename: filename,
		Code:     code,
		Lang:     detectLang(filename),
	})
	if err != nil {
		return nil, err
	}
	req := llm.ChatRequest{
		Messages: []llm.Message{
			llm.SystemMessage(codeReviewSystemPrompt),
			llm.UserMessage(prompt),
		},
	}
	resp, err := r.model.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	var res Result
	if err := utils.ParseJSON(resp.Content, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func detectLang(filename string) string {
	ext := strings.TrimPrefix(filepath.Ext(filename), ".")
	if ext == "" {
		return "txt"
	}
	return ext
}

`

func TestReview(t *testing.T) {
	godotenv.Load("../../.env")
	apiKey := os.Getenv("API_KEY")
	model := os.Getenv("MODEL")
	baseUrl := os.Getenv("BASE_URL")
	cm := llm.NewOpenAIChatModel(baseUrl, apiKey, model)
	r := NewReviewer(cm)
	ctx, cancel := context.WithTimeout(context.Background(), 114*time.Second)
	defer cancel()
	res, err := r.Review(ctx, "test.go", sampleCode)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("res: %v", res)
}
