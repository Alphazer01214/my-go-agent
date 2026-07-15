package middleware_test

import (
	"fmt"
	"my-go-agent/internal/agent"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/middleware"
	"my-go-agent/internal/tool"
)

// 示例：ChatModel 中间件
func Example_chatModel() {
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
	_ = tracedModel
}

// 示例：Agent 中间件
func Example_agent() {
	model := llm.NewOpenAIChatModel("http://localhost:8080", "key", "gpt-4")
	registry := tool.NewRegistry()
	baseAgent := agent.NewToolCallingAgent(model, registry)

	// 添加日志
	loggedAgent := middleware.WithAgentLog(nil)(baseAgent)

	// 添加追踪（记录每一步的 thought/action/observation）
	tracedAgent := middleware.WithAgentTrace(nil)(loggedAgent)

	// 或者组合多个中间件
	composed := middleware.ComposeAgentMiddlewares(
		middleware.WithAgentLog(nil),
		middleware.WithAgentTrace(nil),
	)
	_ = composed(baseAgent)
	_ = tracedAgent
}

// 示例：自定义追踪处理器
func Example_customTraceHandler() {
	// 自定义 agent 追踪处理器
	agentHandler := middleware.WithAgentTrace(func(entry middleware.TraceEntry) {
		fmt.Printf("Agent %s step %d: %s\n", entry.AgentName, entry.Step, entry.Data.Action)
	})

	// 自定义 LLM 追踪处理器
	llmHandler := middleware.WithChatTrace(func(entry middleware.ChatTraceEntry) {
		fmt.Printf("LLM %s: %v\n", entry.Method, entry.Duration)
	})

	model := llm.NewOpenAIChatModel("http://localhost:8080", "key", "gpt-4")
	loggedModel := llmHandler(model)

	registry := tool.NewRegistry()
	baseAgent := agent.NewToolCallingAgent(loggedModel, registry)
	tracedAgent := agentHandler(baseAgent)

	fmt.Println(tracedAgent.Name())
}
