package main

import (
	"context"
	"my-go-agent/internal/agent"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/tool"
	"os"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()
	apiKey := os.Getenv("API_KEY")
	model := os.Getenv("MODEL")
	baseUrl := os.Getenv("BASE_URL")
	cm := llm.NewOpenAIChatModel(baseUrl, apiKey, model)
	tools := tool.NewRegistry()
	readFile := tool.NewReadFileTool(1024 * 1024)
	listDir := tool.NewListDirTool()
	execCmd := tool.NewExecCommandTool()
	tools.Register(readFile)
	tools.Register(listDir)
	tools.Register(execCmd)
	a := agent.NewToolCallingAgent(cm, tools)
	ctx, cancel := context.WithTimeout(context.Background(), 1145*time.Second)
	defer cancel()
	resp, err := a.Run(ctx, "分析这个项目")
	if err != nil {
		panic(err)
	}
	println(resp)
}
