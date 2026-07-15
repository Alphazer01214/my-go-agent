package middleware

import (
	"context"
	"fmt"
	"my-go-agent/internal/llm"
	"time"
)

// ChatModelMiddleware ChatModel 中间件类型
type ChatModelMiddleware func(llm.ChatModel) llm.ChatModel

// ==================== Log ====================

// ChatModelWithLog 带日志的 ChatModel 包装器
type ChatModelWithLog struct {
	model   llm.ChatModel
	logFunc func(string)
}

// WithChatLog 创建 ChatModel 日志中间件，logFunc 为 nil 时使用默认 fmt.Println
func WithChatLog(logFunc func(string)) ChatModelMiddleware {
	if logFunc == nil {
		logFunc = func(msg string) { fmt.Println(msg) }
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

// ==================== Trace ====================

// ChatTraceEntry ChatModel 追踪记录
type ChatTraceEntry struct {
	Timestamp time.Time
	Method    string // "Chat" or "StreamChat"
	Request   *llm.ChatRequest
	Response  *llm.ChatResponse
	Duration  time.Duration
	Error     error
}

// ChatTraceHandler 处理 ChatModel 追踪事件
type ChatTraceHandler func(entry ChatTraceEntry)

// ChatModelWithTrace 带追踪的 ChatModel 包装器
type ChatModelWithTrace struct {
	model   llm.ChatModel
	handler ChatTraceHandler
}

// WithChatTrace 创建 ChatModel 追踪中间件，记录请求/响应详情
func WithChatTrace(handler ChatTraceHandler) ChatModelMiddleware {
	if handler == nil {
		handler = DefaultChatTraceHandler
	}
	return func(m llm.ChatModel) llm.ChatModel {
		return &ChatModelWithTrace{model: m, handler: handler}
	}
}

func (c *ChatModelWithTrace) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	start := time.Now()
	resp, err := c.model.Chat(ctx, req)

	c.handler(ChatTraceEntry{
		Timestamp: start,
		Method:    "Chat",
		Request:   req,
		Response:  resp,
		Duration:  time.Since(start),
		Error:     err,
	})
	return resp, err
}

func (c *ChatModelWithTrace) StreamChat(ctx context.Context, req *llm.ChatRequest, callback func(string)) (*llm.ChatResponse, error) {
	start := time.Now()
	resp, err := c.model.StreamChat(ctx, req, callback)

	c.handler(ChatTraceEntry{
		Timestamp: start,
		Method:    "StreamChat",
		Request:   req,
		Response:  resp,
		Duration:  time.Since(start),
		Error:     err,
	})
	return resp, err
}

// DefaultChatTraceHandler 默认的 ChatModel 追踪处理器
func DefaultChatTraceHandler(entry ChatTraceEntry) {
	ts := entry.Timestamp.Format("15:04:05.000")
	status := "✓"
	if entry.Error != nil {
		status = "✗"
	}

	msgCount := 0
	toolCount := 0
	if entry.Request != nil {
		msgCount = len(entry.Request.Messages)
		toolCount = len(entry.Request.Tools)
	}

	if entry.Error != nil {
		fmt.Printf("[%s] %s [LLM] %s | messages=%d tools=%d duration=%v err=%v\n",
			ts, status, entry.Method, msgCount, toolCount, entry.Duration, entry.Error)
	} else {
		fmt.Printf("[%s] %s [LLM] %s | messages=%d tools=%d duration=%v\n",
			ts, status, entry.Method, msgCount, toolCount, entry.Duration)
	}
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
