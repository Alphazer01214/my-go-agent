package middleware_test

import (
	"my-go-agent/internal/llm"
	"my-go-agent/internal/middleware"
)

// 示例：ChatModel 日志中间件
func Example_chatLog() {
	model := llm.NewOpenAIChatModel("http://localhost:8080", "key", "gpt-4")

	// 添加日志
	loggedModel := middleware.WithChatLog(nil)(model)
	_ = loggedModel

	// 组合多个中间件
	composed := middleware.ComposeChatMiddlewares(
		middleware.WithChatLog(nil),
	)
	_ = composed(model)
}
