package llm

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
)

func TestChat(t *testing.T) {
	err := godotenv.Load("../../.env")
	if err != nil {
		panic("Failed to load .env file: " + err.Error())
	}
	apiKey := os.Getenv("API_KEY")
	baseUrl := os.Getenv("BASE_URL")
	model := os.Getenv("MODEL")

	chatModel := NewOpenAIChatModel(baseUrl, apiKey, model)

	req := &ChatRequest{
		Messages: []Message{
			{
				Role:    RoleSystem,
				Content: "You are a blockchain engineer.",
			},
			{
				Role:    RoleUser,
				Content: "What is blockchain?",
			},
		},
		Options: ChatOptions{
			Temperature: 0.5,
			MaxTokens:   2048,
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	fmt.Println("----test chat----")
	resp, err := chatModel.Chat(ctx, req)
	if err != nil {
		panic("Failed to chat: " + err.Error())
	}

	fmt.Println(resp.Content)
	fmt.Println(resp.Usage)

	fmt.Println("----test stream---")
	// test stream requires go test -v -run TestChat
	_, err = chatModel.StreamChat(ctx, req, func(s string) {
		fmt.Printf("%s", s)
	})
	if err != nil {
		panic("Failed to stream chat: " + err.Error())
	}
}

//func TestStream(t *testing.T) {
//
//}
