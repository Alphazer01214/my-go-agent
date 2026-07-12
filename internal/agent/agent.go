package agent

import (
	"context"
	"fmt"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/memory"
	"my-go-agent/internal/prompts"
	"my-go-agent/internal/tool"
)

type ReactAgent interface {
	Run(ctx context.Context, prompt string) (string, error)
}
type Step struct {
	Thought     string
	Action      string
	Observation string
}

type Agent struct {
	Name   string
	Goal   string
	Memory []string
	Steps  []Step
}

type Trace struct {
	Step           int
	ModelOutput    string
	ToolCallResult []string
}

type ToolCallingAgent struct {
	model   llm.ChatModel
	tools   *tool.Registry
	memory  memory.Memory
	trace   []Trace
	maxStep int
}

func NewToolCallingAgent(model llm.ChatModel, tools *tool.Registry) *ToolCallingAgent {
	return &ToolCallingAgent{
		model:   model,
		tools:   tools,
		maxStep: 114,
	}
}
func (a *ToolCallingAgent) Run(ctx context.Context, prompt string) (string, error) {
	res := ""
	messages := []llm.Message{
		llm.SystemMessage(prompts.DefaultSystemPrompt),
		llm.UserMessage(prompt),
	}
	for i := 1; i <= a.maxStep; i++ {
		req := &llm.ChatRequest{
			Messages: messages,
			Tools:    a.tools.ToLLMTool(),
		}
		resp, err := a.model.StreamChat(ctx, req, func(chunk string) {
			fmt.Printf("%v", chunk)
		})
		if err != nil {
			return "", err
		}
		if resp.Content != "" {
			res += resp.Content
			messages = append(messages, llm.AssistantMessage(resp.Content))
		}

		if len(resp.ToolCalls) == 0 {
			return res, nil
		}
		messages = append(messages, llm.AssistantMessageWithToolCall(resp.Content, resp.ToolCalls))
		for _, call := range resp.ToolCalls {
			res += fmt.Sprintf("\nUse tool %s with arguments %s", call.Function.Name, call.Function.Arguments)
			result := a.tools.Execute(ctx, call.Function.Name, call.Function.Arguments)
			messages = append(messages, llm.ToolMessage(call.ID, result))
		}
	}
	res += fmt.Sprintf("\nReached max step %d", a.maxStep)
	return res, nil
}
