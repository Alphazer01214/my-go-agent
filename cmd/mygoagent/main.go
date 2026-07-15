package main

import (
	"bufio"
	"context"
	"fmt"
	"my-go-agent/internal/agent"
	"my-go-agent/internal/config"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/memory"
	"my-go-agent/internal/middleware"
	"my-go-agent/internal/tool"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()
	apiKey := os.Getenv("API_KEY")
	model := os.Getenv("MODEL")
	baseUrl := os.Getenv("BASE_URL")

	if apiKey == "" || baseUrl == "" {
		fmt.Fprintln(os.Stderr, "[config] API_KEY and BASE_URL must be set in .env")
		os.Exit(1)
	}

	// 创建 LLM 并添加日志中间件
	var cm llm.ChatModel = llm.NewOpenAIChatModel(baseUrl, apiKey, model)
	cm = middleware.WithChatLog(nil)(cm)

	// 注册工具
	tools := registerTools()

	// 加载项目上下文
	projectCtx, err := memory.NewProjectContext(".memory/context.md")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[memory] load context: %v\n", err)
		os.Exit(1)
	}

	// 创建 agent
	a := agent.NewToolCallingAgent(cm, tools)
	a.SetMemory(memory.NewSummaryMemory(cm, config.MemorySize))
	a.SetMaxStep(config.MaxSteps)
	a.SetProjectContext(projectCtx)

	// 添加追踪中间件，记录每一步的 thought/action/observation
	traceHandler := middleware.WithAgentTrace(func(entry middleware.TraceEntry) {
		step := entry.Data
		if step.Action != "" {
			fmt.Printf("\n  [step %d] %s\n", entry.Step, step.Action)
		}
	})
	tracedAgent := traceHandler(a)

	if len(os.Args) > 1 {
		runSingle(tracedAgent, strings.Join(os.Args[1:], " "))
	} else {
		runInteractive(tracedAgent)
	}
}

func registerTools() *tool.Registry {
	r := tool.NewRegistry()

	// 文件操作工具
	r.Register(tool.NewReadFileTool(config.ReadFileSize))
	r.Register(tool.NewWriteFileTool())
	r.Register(tool.NewEditFileTool())

	// 目录和文件查找工具
	r.Register(tool.NewListDirTool())
	r.Register(tool.NewGlobTool())
	r.Register(tool.NewSearchFileTool())

	// Shell 命令（作为后备）
	r.Register(tool.NewExecCommandTool())

	// 网络搜索（可选，需要配置 API）
	searchAPIKey := os.Getenv("SEARCH_API_KEY")
	searchBaseURL := os.Getenv("SEARCH_BASE_URL")
	if searchAPIKey != "" && searchBaseURL != "" {
		r.Register(tool.NewWebSearchTool(searchAPIKey, searchBaseURL))
	}

	return r
}

func runSingle(a agent.Agent, prompt string) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	result, err := a.Run(ctx, prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n[agent] %v\n", err)
		os.Exit(1)
	}
	fmt.Println(result)
}

func runInteractive(a agent.Agent) {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("my-go-agent (type 'exit' to quit)")
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			break
		}

		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		result, err := a.Run(ctx, input)
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n[agent] %v\n", err)
		}
		if result != "" {
			fmt.Println(result)
		}
		fmt.Println()
	}
}
