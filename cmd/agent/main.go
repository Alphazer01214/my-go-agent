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

	// 创建共享 LLM
	var cm llm.ChatModel = llm.NewOpenAIChatModel(baseUrl, apiKey, model)
	fileTrace := middleware.NewFileChatTraceHandler("logs", "multi-agent")
	cm = middleware.WithChatTrace(fileTrace)(cm)

	// 加载项目上下文
	projectCtx, err := memory.NewProjectContext(".memory/context.md")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[memory] load context: %v\n", err)
		os.Exit(1)
	}

	// ========== 创建各领域专家 ==========

	// 1. code_writer: 写代码专家
	codeWriterTools := tool.NewRegistry()
	codeWriterTools.Register(tool.NewWriteFileTool())
	codeWriterTools.Register(tool.NewReadFileTool(config.ReadFileSize))
	codeWriterTools.Register(tool.NewEditFileTool())
	codeWriterTools.Register(tool.NewExecCommandTool())

	codeWriter := agent.NewToolCallingAgent(cm, codeWriterTools)
	codeWriter.SetSysPrompt(`你是一个代码编写专家。
你的职责是：根据用户需求编写高质量的代码。
你可以：
- 使用 write_file 创建新文件
- 使用 edit_file 修改已有文件
- 使用 read_file 读取已有文件
- 使用 exec_command 运行命令（如 go build、go test）
请始终输出完整的、可运行的代码。`)
	codeWriter.SetMaxStep(10)
	codeWriter.SetMemory(memory.NewSlideWindow(10))
	codeWriter.SetProjectContext(projectCtx)

	// 2. file_explorer: 文件探索专家
	exploreTools := tool.NewRegistry()
	exploreTools.Register(tool.NewListDirTool())
	exploreTools.Register(tool.NewReadFileTool(config.ReadFileSize))
	exploreTools.Register(tool.NewGlobTool())
	exploreTools.Register(tool.NewSearchFileTool())

	fileExplorer := agent.NewToolCallingAgent(cm, exploreTools)
	fileExplorer.SetSysPrompt(`你是一个文件探索专家。
你的职责是：帮助用户了解项目结构和文件内容。
你可以：
- 使用 list_dir 查看目录
- 使用 read_file 读取文件
- 使用 glob 查找文件
- 使用 search_file 搜索内容
请清晰地向用户汇报你发现的信息。`)
	fileExplorer.SetMaxStep(10)
	fileExplorer.SetMemory(memory.NewSlideWindow(10))
	fileExplorer.SetProjectContext(projectCtx)

	// 3. code_reviewer: 代码审查专家
	codeReviewer := agent.NewReviewAgent(cm)
	codeReviewer.SetSysPrompt(`你是一个代码审查专家。
请严格检查代码的质量、安全性、性能和可维护性。
用中文输出结构化审查结果。`)
	codeReviewer.SetMaxStep(1)
	codeReviewer.SetMemory(memory.NewSlideWindow(5))

	// 包装为 Expert
	experts := []*agent.Expert{
		{Name: "code_writer", Description: "用于编写代码、创建文件、修改文件。适合实现新功能、修复 bug、创建代码文件。", Agent: codeWriter},
		{Name: "file_explorer", Description: "用于探索项目目录结构、读取文件内容、搜索文件和目录。适合了解项目架构和代码逻辑。", Agent: fileExplorer},
		{Name: "code_reviewer", Description: "用于审查代码质量，检查 bug 和安全隐患，给出改进建议。适合 review 已有代码。", Agent: codeReviewer},
	}

	// ========== 创建总管 Agent ==========
	// 方案一：ExpertManager — LLM 自动路由到合适的专家
	manager := agent.NewExpertManager(cm, experts)
	manager.SetMaxStep(15)
	manager.SetProjectContext(projectCtx)
	manager.SetSysPrompt(`你是一个多智能体系统的总管协调员。
你有多个专家可以调用：
- code_writer: 写代码、改代码
- file_explorer: 探索文件、读取代码
- code_reviewer: 代码审查

请先分析用户需求，然后选择合适的专家来处理。
如果任务复杂，可以分步调用多个专家协作完成。
每次调用专家时，用清晰的任务描述告诉专家需要做什么。
专家完成后，将结果整理后回复给用户。`)

	// 添加追踪中间件
	traceHandler := middleware.WithAgentTrace(func(entry middleware.TraceEntry) {
		step := entry.Data
		if step.Thought != "" {
			fmt.Printf("\n  [step %d] 🤔 %s\n", entry.Step, step.Thought)
		}
		if step.Action != "" {
			fmt.Printf("  [step %d] 🛠️  %s\n", entry.Step, step.Action)
		}
	})
	tracedManager := traceHandler(manager)

	// ========== 运行 ==========
	if len(os.Args) > 1 {
		runSingle(tracedManager, strings.Join(os.Args[1:], " "))
	} else {
		runInteractive(tracedManager)
	}
}

func runSingle(a agent.Agent, prompt string) {
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	fmt.Printf("🤖 Multi-Agent 开始处理: %s\n\n", prompt)
	result, err := a.Run(ctx, prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n[agent] %v\n", err)
		os.Exit(1)
	}
	fmt.Println(result)
}

func runInteractive(a agent.Agent) {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("🤖 Multi-Agent System (type 'exit' to quit)")
	fmt.Println("  我有 code_writer / file_explorer / code_reviewer 三个专家可供调度！")
	fmt.Println()
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

		ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
		result, err := a.Run(ctx, input)
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n❌ [agent] %v\n", err)
		}
		if result != "" {
			fmt.Println(result)
		}
		fmt.Println()
	}
}
