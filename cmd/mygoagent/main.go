package main

import (
	"context"
	"fmt"
	"my-go-agent/internal/agent"
	"my-go-agent/internal/config"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/memory"
	"my-go-agent/internal/sandbox"
	"my-go-agent/internal/tool"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()
	apiKey := os.Getenv("API_KEY")
	model := os.Getenv("MODEL")
	baseUrl := os.Getenv("BASE_URL")

	if apiKey == "" || baseUrl == "" {
		fmt.Fprintf(os.Stderr, "%s[config]%s API_KEY and BASE_URL must be set in .env\n", colorRed, colorReset)
		os.Exit(1)
	}

	// 创建 LLM
	var cm llm.ChatModel = llm.NewOpenAIChatModel(baseUrl, apiKey, model)

	// 注册工具
	tools := registerTools()

	// 创建沙箱并注入 Registry
	sb := sandbox.New()
	tools.SetSandbox(sb)

	// 创建 Project（用当前目录作为项目路径）
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s[project]%s get cwd: %v\n", colorRed, colorReset, err)
		os.Exit(1)
	}
	projectName := filepath.Base(dir)
	project, err := memory.NewProject(memory.ProjectOpts{
		Name:        projectName,
		Path:        dir,
		ContextPath: config.DefaultContextFile,
		Compressor:  memory.NewSummaryCompressor(cm, config.MemorySize),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s[project]%s init: %v\n", colorRed, colorReset, err)
		os.Exit(1)
	}

	// 切换工作目录到项目路径，确保工具的相对路径解析正确
	if err := os.Chdir(project.Path()); err != nil {
		fmt.Fprintf(os.Stderr, "%s[project]%s chdir: %v\n", colorRed, colorReset, err)
		os.Exit(1)
	}

	// 创建 agent
	a := agent.NewToolCallingAgent(fmt.Sprintf("%v_tool_calling_agent", projectName), cm, tools)
	a.SetMaxStep(config.MaxSteps)

	// 注册子 Agent 工具（需要 agent 实例）
	tools.Register(agent.NewSubagentTool(a.Base()))

	// 创建 Session
	session := project.NewSession(a.Name())

	if len(os.Args) > 1 {
		runSingle(a, session, tools, strings.Join(os.Args[1:], " "))
	} else {
		runInteractive(a, session, project, tools, cm)
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
	r.Register(tool.NewMakeDirTool())
	r.Register(tool.NewQuestionTableTool())
	r.Register(tool.NewCreateTodoTool())
	r.Register(tool.NewStartTodoTool())
	r.Register(tool.NewCompleteTodoTool())
	r.Register(tool.NewListTodosTool())

	searchAPIKey := os.Getenv("SEARCH_API_KEY")
	searchBaseURL := os.Getenv("SEARCH_BASE_URL")
	if searchAPIKey != "" && searchBaseURL != "" {
		r.Register(tool.NewWebSearchTool(searchAPIKey, searchBaseURL))
	}

	return r
}

// ==================== Single Run ====================

func runSingle(a agent.Agent, session *memory.Session, tools *tool.Registry, prompt string) {
	ctx, cancel := context.WithTimeout(context.Background(), config.DefaultSingleRunTimeout)
	defer cancel()

	tools.ResetSandbox()

	result, err := a.Run(ctx, session, prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%s[agent]%s %v\n", colorRed, colorReset, err)
		os.Exit(1)
	}
	fmt.Println(result)

	if session.Trace().EventCnt() > 0 {
		fmt.Println("\n" + session.Trace().Format())
	}
}
