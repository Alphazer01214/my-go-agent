package middleware_test

import (
	"fmt"
	"my-go-agent/internal/agent"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/middleware"
	"my-go-agent/internal/tool"
)

// 示例：ChatModel 中间件使用
func Example_chatModel() {
	// 创建原始 model
	model := llm.NewOpenAIChatModel("http://localhost:8080", "key", "gpt-4")

	// 添加日志
	loggedModel := middleware.WithChatLog(nil)(model)

	// 添加追踪
	tracedModel := middleware.WithChatTrace(nil)(loggedModel)

	// 或者组合多个中间件
	composed := middleware.ComposeChatMiddlewares(
		middleware.WithChatLog(nil),
		middleware.WithChatTrace(nil),
	)
	_ = composed(model)

	// 使用 tracedModel...
	_ = tracedModel
}

// 示例：Agent 中间件使用
func Example_agent() {
	// 创建原始 agent
	model := llm.NewOpenAIChatModel("http://localhost:8080", "key", "gpt-4")
	registry := tool.NewRegistry()
	baseAgent := agent.NewToolCallingAgent(model, registry)

	// 添加日志
	loggedAgent := middleware.WithAgentLog(nil)(baseAgent)

	// 添加追踪
	tracedAgent := middleware.WithAgentTrace(nil)(loggedAgent)

	// 或者组合多个中间件
	composed := middleware.ComposeAgentMiddlewares(
		middleware.WithAgentLog(nil),
		middleware.WithAgentTrace(nil),
	)
	_ = composed(baseAgent)

	// 使用 tracedAgent...
	_ = tracedAgent
}

// 示例：自定义追踪处理器
func Example_customTraceHandler() {
	// 收集所有追踪条目
	var entries []middleware.AgentTraceEntry

	handler := middleware.WithAgentTrace(func(entry middleware.AgentTraceEntry) {
		entries = append(entries, entry)
		// 可以发送到 Jaeger、Zipkin 等追踪系统
	})

	model := llm.NewOpenAIChatModel("http://localhost:8080", "key", "gpt-4")
	registry := tool.NewRegistry()
	baseAgent := agent.NewToolCallingAgent(model, registry)

	tracedAgent := handler(baseAgent)
	fmt.Println(tracedAgent.Name())
}
