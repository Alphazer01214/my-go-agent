package llm

import (
	"context"
)

type ChatModel interface {
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	StreamChat(ctx context.Context, req *ChatRequest, callback func(string)) (*ChatResponse, error)
	GetModel() string
	SetModel(model string)
}
