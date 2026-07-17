package middleware

import (
	"context"
	"fmt"
	"my-go-agent/internal/llm"
	"time"
)

// defaultLogFunc 默认日志输出函数
func defaultLogFunc(msg string) { fmt.Println(msg) }

// AgentMiddleware Agent 中间件类型
type AgentMiddleware func(Agent) Agent

// Agent 最小化 agent 接口，避免直接依赖 agent 包
type Agent interface {
	Run(ctx context.Context, prompt string) (string, error)
	Name() string
}

// AgentWithLog 带日志的 Agent 包装器
type AgentWithLog struct {
	agent   Agent
	logFunc func(string)
}

// WithAgentLog 创建 Agent 日志中间件，logFunc 为 nil 时使用默认 fmt.Println
func WithAgentLog(logFunc func(string)) AgentMiddleware {
	if logFunc == nil {
		logFunc = defaultLogFunc
	}
	return func(a Agent) Agent {
		return &AgentWithLog{agent: a, logFunc: logFunc}
	}
}

func (a *AgentWithLog) Name() string { return a.agent.Name() }

func (a *AgentWithLog) Run(ctx context.Context, prompt string) (string, error) {
	start := time.Now()
	a.logFunc(fmt.Sprintf("[%s] Run start", a.agent.Name()))

	result, err := a.agent.Run(ctx, prompt)
	duration := time.Since(start)

	if err != nil {
		a.logFunc(fmt.Sprintf("[%s] Run error | duration=%v err=%v",
			a.agent.Name(), duration, err))
		return "", err
	}
	a.logFunc(fmt.Sprintf("[%s] Run done | duration=%v",
		a.agent.Name(), duration))
	return result, nil
}

// ChatModelMiddleware ChatModel 中间件类型
type ChatModelMiddleware func(llm.ChatModel) llm.ChatModel

// ChatModelWithLog 带日志的 ChatModel 包装器
type ChatModelWithLog struct {
	model   llm.ChatModel
	logFunc func(string)
}

// WithChatLog 创建 ChatModel 日志中间件，logFunc 为 nil 时使用默认 fmt.Println
func WithChatLog(logFunc func(string)) ChatModelMiddleware {
	if logFunc == nil {
		logFunc = defaultLogFunc
	}
	return func(m llm.ChatModel) llm.ChatModel {
		return &ChatModelWithLog{model: m, logFunc: logFunc}
	}
}

func (c *ChatModelWithLog) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	start := time.Now()
	c.logFunc(fmt.Sprintf("[LLM] Chat start | messages=%d tools=%d", len(req.Messages), len(req.Tools)))

	resp, err := c.model.Chat(ctx, req)
	duration := time.Since(start)

	if err != nil {
		c.logFunc(fmt.Sprintf("[LLM] Chat error | duration=%v error=%v", duration, err))
		return nil, err
	}
	c.logFunc(fmt.Sprintf("[LLM] Chat done | duration=%v tokens=(in:%d out:%d total:%d)",
		duration, resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.TotalTokens))
	return resp, nil
}

func (c *ChatModelWithLog) StreamChat(ctx context.Context, req *llm.ChatRequest, callback func(string)) (*llm.ChatResponse, error) {
	start := time.Now()
	c.logFunc(fmt.Sprintf("[LLM] StreamChat start | messages=%d tools=%d", len(req.Messages), len(req.Tools)))

	resp, err := c.model.StreamChat(ctx, req, callback)
	duration := time.Since(start)

	if err != nil {
		c.logFunc(fmt.Sprintf("[LLM] StreamChat error | duration=%v error=%v", duration, err))
		return nil, err
	}
	c.logFunc(fmt.Sprintf("[LLM] StreamChat done | duration=%v content_len=%d tool_calls=%d",
		duration, len(resp.Content), len(resp.ToolCalls)))
	return resp, nil
}

// ComposeChatMiddlewares 组合多个 ChatModel 中间件，从左到右执行
func ComposeChatMiddlewares(middlewares ...ChatModelMiddleware) ChatModelMiddleware {
	return func(m llm.ChatModel) llm.ChatModel {
		for i := len(middlewares) - 1; i >= 0; i-- {
			m = middlewares[i](m)
		}
		return m
	}
}
