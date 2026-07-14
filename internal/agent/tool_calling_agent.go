package agent

import (
	"context"
	"fmt"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/tool"
)

// ToolCallingAgent 实现 Agent 接口，执行思考-工具-观察循环
type ToolCallingAgent struct {
	*BaseAgent
}

func NewToolCallingAgent(model llm.ChatModel, tools *tool.Registry) *ToolCallingAgent {
	return &ToolCallingAgent{
		BaseAgent: NewBaseAgent("tool_calling", model, tools),
	}
}

func (a *ToolCallingAgent) Run(ctx context.Context, prompt string) (string, error) {
	a.Memory.Clear()
	a.steps = nil

	a.Memory.Add(ctx, llm.SystemMessage(a.BuildSysPrompt()))
	a.Memory.Add(ctx, llm.UserMessage(prompt))

	res := ""
	for step := 1; step <= a.MaxStep; step++ {
		messages := a.Memory.Messages(ctx)
		req := &llm.ChatRequest{
			Messages: messages,
			Tools:    a.Tools.ToLLMTool(),
		}

		resp, err := a.Model.StreamChat(ctx, req, func(chunk string) {
			fmt.Print(chunk)
		})
		if err != nil {
			return "", err
		}

		current := Step{Thought: resp.Content}

		if resp.Content != "" {
			res += resp.Content
			a.Memory.Add(ctx, llm.AssistantMessage(resp.Content))
		}

		// 没有 tool call，说明 agent 认为目标已完成
		if len(resp.ToolCalls) == 0 {
			a.steps = append(a.steps, current)
			if a.OnStep != nil {
				a.OnStep(step, current)
			}
			return res, nil
		}

		a.Memory.Add(ctx, llm.AssistantMessageWithToolCall(resp.Content, resp.ToolCalls))
		for _, call := range resp.ToolCalls {
			current.Action = fmt.Sprintf("%s(%s)", call.Function.Name, call.Function.Arguments)
			result := a.Tools.Execute(ctx, call.Function.Name, call.Function.Arguments)
			current.Observation = result
			a.Memory.Add(ctx, llm.ToolMessage(call.ID, result))
		}

		a.steps = append(a.steps, current)
		if a.OnStep != nil {
			a.OnStep(step, current)
		}

		if err := a.Memory.Compress(ctx); err != nil {
			return "", err
		}
	}
	return res, fmt.Errorf("[agent] reached max step %d", a.MaxStep)
}
