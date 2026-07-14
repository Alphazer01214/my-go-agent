package main

import (
	"bufio"
	"context"
	"fmt"
	"my-go-agent/internal/agent"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/memory"
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

	cm := llm.NewOpenAIChatModel(baseUrl, apiKey, model)
	tools := registerTools()

	projectCtx, err := memory.NewProjectContext(".memory/context.md")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[memory] load context: %v\n", err)
		os.Exit(1)
	}

	a := agent.NewToolCallingAgent(cm, tools)
	a.SetMemory(memory.NewSlideWindow(20))
	a.SetMaxStep(50)
	a.SetProjectContext(projectCtx)
	a.SetOnStep(func(step int, data agent.Step) {
		if data.Action != "" {
			fmt.Printf("\n  [step %d] %s\n", step, data.Action)
		}
	})

	if len(os.Args) > 1 {
		runSingle(a, strings.Join(os.Args[1:], " "))
	} else {
		runInteractive(a)
	}
}

func registerTools() *tool.Registry {
	r := tool.NewRegistry()
	r.Register(tool.NewReadFileTool(1024 * 1024))
	r.Register(tool.NewListDirTool())
	r.Register(tool.NewExecCommandTool())
	return r
}

func runSingle(a *agent.ToolCallingAgent, prompt string) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	_, err := a.Run(ctx, prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n[agent] %v\n", err)
		os.Exit(1)
	}
}

func runInteractive(a *agent.ToolCallingAgent) {
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
		_, err := a.Run(ctx, input)
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n[agent] %v\n", err)
		}
		fmt.Println()
	}
}
