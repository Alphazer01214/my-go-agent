package main

import (
	"bufio"
	"context"
	"fmt"
	"my-go-agent/internal/agent"
	"my-go-agent/internal/config"
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

	// 创建 LLM
	var cm llm.ChatModel = llm.NewOpenAIChatModel(baseUrl, apiKey, model)

	// 注册工具
	tools := registerTools()

	// 创建 Project
	project, err := memory.NewProject(memory.ProjectOpts{
		Name:        "mygoagent",
		ContextPath: ".memory/context.md",
		Compressor:  memory.NewSummaryCompressor(cm, config.MemorySize),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[project] init: %v\n", err)
		os.Exit(1)
	}

	// 创建 agent
	a := agent.NewToolCallingAgent(cm, tools)
	a.SetMaxStep(config.MaxSteps)

	// 创建 Session
	session := project.NewSession(a.Name())

	if len(os.Args) > 1 {
		runSingle(a, session, strings.Join(os.Args[1:], " "))
	} else {
		runInteractive(a, session, project)
	}
}

func registerTools() *tool.Registry {
	r := tool.NewRegistry()

	r.Register(tool.NewReadFileTool(config.ReadFileSize))
	r.Register(tool.NewWriteFileTool())
	r.Register(tool.NewEditFileTool())
	r.Register(tool.NewListDirTool())
	r.Register(tool.NewGlobTool())
	r.Register(tool.NewSearchFileTool())
	r.Register(tool.NewExecCommandTool())

	searchAPIKey := os.Getenv("SEARCH_API_KEY")
	searchBaseURL := os.Getenv("SEARCH_BASE_URL")
	if searchAPIKey != "" && searchBaseURL != "" {
		r.Register(tool.NewWebSearchTool(searchAPIKey, searchBaseURL))
	}

	return r
}

func runSingle(a agent.Agent, session *memory.Session, prompt string) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	result, err := a.Run(ctx, session, prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n[agent] %v\n", err)
		os.Exit(1)
	}
	fmt.Println(result)

	if session.Trace().Len() > 0 {
		fmt.Println("\n" + session.Trace().Format())
	}
}

func runInteractive(a agent.Agent, session *memory.Session, project *memory.Project) {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("my-go-agent (type 'exit' to quit)")
	fmt.Println("Commands: /trace, /trace last, /trace clear, /new, /reset")
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

		// 命令处理
		if strings.HasPrefix(input, "/") {
			cmd := handleCommand(input, session, project)
			if cmd == "new_session" {
				session = project.NewSession(a.Name())
				fmt.Println("New session created.")
			}
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		result, err := a.Run(ctx, session, input)
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

func handleCommand(input string, session *memory.Session, project *memory.Project) string {
	parts := strings.Fields(input)
	cmd := parts[0]

	switch cmd {
	case "/trace":
		arg := ""
		if len(parts) > 1 {
			arg = parts[1]
		}
		handleTraceCommand(session.Trace(), arg)
	case "/new":
		return "new_session"
	case "/reset":
		session.Reset()
		fmt.Println("Session reset.")
	case "/sessions":
		for _, s := range project.Sessions() {
			fmt.Printf("  %s  agent=%s  messages=%d  created=%s\n",
				s.ID(), s.AgentName(), s.MessageCount(), s.CreatedAt().Format("15:04:05"))
		}
	default:
		fmt.Printf("Unknown command: %s\n", cmd)
	}
	return ""
}

func handleTraceCommand(trace *memory.Trace, arg string) {
	switch arg {
	case "clear":
		trace.Clear()
		fmt.Println("Trace cleared.")
	case "last":
		e := trace.Last()
		if e == nil {
			fmt.Println("(no trace events)")
		} else {
			fmt.Println(trace.FormatEvent(*e))
		}
	case "":
		if trace.Len() == 0 {
			fmt.Println("(no trace events)")
		} else {
			fmt.Println(trace.Format())
		}
	default:
		fmt.Printf("Unknown trace command: %s\n", arg)
		fmt.Println("Usage: /trace [clear|last]")
	}
}
