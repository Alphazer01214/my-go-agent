package main

import (
	"bufio"
	"context"
	"fmt"
	"my-go-agent/internal/agent"
	"my-go-agent/internal/config"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/memory"
	"my-go-agent/internal/sandbox"
	"my-go-agent/internal/tool"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

// ANSI 颜色
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
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

	// 创建 Project
	project, err := memory.NewProject(memory.ProjectOpts{
		Name:        "mygoagent",
		ContextPath: ".memory/context.md",
		Compressor:  memory.NewSummaryCompressor(cm, config.MemorySize),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s[project]%s init: %v\n", colorRed, colorReset, err)
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
		runInteractive(a, session, project, model)
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

func runSingle(a agent.Agent, session *memory.Session, prompt string) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	a.Base().Tools.ResetSandbox()

	result, err := a.Run(ctx, session, prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%s[agent]%s %v\n", colorRed, colorReset, err)
		os.Exit(1)
	}
	fmt.Println(result)

	if session.Trace().Len() > 0 {
		fmt.Println("\n" + session.Trace().Format())
	}
}

// ==================== Interactive Run ====================

func runInteractive(a agent.Agent, session *memory.Session, project *memory.Project, model string) {
	// 信号处理：Ctrl+C 优雅退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Printf("\n%s再见！%s\n", colorDim, colorReset)
		os.Exit(0)
	}()

	printWelcome(model, session)

	scanner := bufio.NewScanner(os.Stdin)
	for {
		// 动态提示符：显示 session 消息数
		msgCount := session.MessageCount()
		todoProgress := session.Todo().FormatProgress()
		prompt := buildPrompt(msgCount, todoProgress)

		fmt.Print(prompt)
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			fmt.Printf("%s再见！%s\n", colorDim, colorReset)
			break
		}

		// 命令处理
		if strings.HasPrefix(input, "/") {
			cmd := handleCommand(input, session, project, a)
			if cmd == "new_session" {
				session = project.NewSession(a.Name())
				fmt.Printf("%s✓ 新会话已创建%s\n", colorGreen, colorReset)
			}
			continue
		}

		// 执行 agent（交互模式不设超时，MaxStep 已限制循环次数，沙箱确认需要无限等待）
		ctx, cancel := context.WithCancel(context.Background())
		a.Base().Tools.ResetSandbox()
		result, err := a.Run(ctx, session, input)
		cancel()

		if err != nil {
			fmt.Fprintf(os.Stderr, "\n%s[error]%s %v\n", colorRed, colorReset, err)
		}
		if result != "" {
			fmt.Println(result)
		}

		// 显示 todo 进度
		if progress := session.Todo().FormatProgress(); progress != "" {
			fmt.Printf("\n  %s%s%s\n", colorCyan, progress, colorReset)
		}
		fmt.Println()
	}
}

// printWelcome 打印欢迎信息
func printWelcome(model string, session *memory.Session) {
	fmt.Printf("\n%s%s╔══════════════════════════════════════╗%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%s%s║        my-go-agent  v0.1             ║%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%s%s╚══════════════════════════════════════╝%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("\n  %sModel:%s   %s\n", colorGray, colorReset, model)
	fmt.Printf("  %sSession:%s %s\n", colorGray, colorReset, session.ID())
	fmt.Printf("  %sProject:%s %s\n", colorGray, colorReset, session.Project().Name())
	fmt.Printf("\n  %s输入消息开始对话，/help 查看命令列表%s\n\n", colorDim, colorReset)
}

// buildPrompt 构建动态提示符
func buildPrompt(msgCount int, todoProgress string) string {
	var sb strings.Builder
	sb.WriteString(colorBold)
	sb.WriteString(colorCyan)
	if todoProgress != "" {
		sb.WriteString(fmt.Sprintf("[%d|%s]", msgCount, todoProgress))
	} else {
		sb.WriteString(fmt.Sprintf("[%d]", msgCount))
	}
	sb.WriteString(colorReset)
	sb.WriteString(" > ")
	return sb.String()
}

// ==================== Commands ====================

func handleCommand(input string, session *memory.Session, project *memory.Project, a agent.Agent) string {
	parts := strings.Fields(input)
	cmd := parts[0]

	switch cmd {
	case "/help", "/h":
		printHelp()
	case "/trace", "/t":
		arg := ""
		if len(parts) > 1 {
			arg = parts[1]
		}
		handleTraceCommand(session.Trace(), arg)
	case "/new", "/n":
		return "new_session"
	case "/reset":
		session.Reset()
		fmt.Printf("%s✓ 会话已重置%s\n", colorGreen, colorReset)
	case "/todo":
		fmt.Println(session.Todo().Format())
	case "/sessions", "/s":
		printSessions(project, session)
	case "/info":
		printSessionInfo(session)
	case "/restore":
		handleRestore(session)
	case "/model":
		if len(parts) > 1 {
			fmt.Printf("%s提示：%s模型切换需要重启程序%s\n", colorYellow, colorDim, colorReset)
		} else {
			fmt.Printf("当前会话 agent: %s\n", session.AgentName())
		}
	default:
		fmt.Printf("%s未知命令: %s%s (输入 /help 查看命令列表)\n", colorRed, cmd, colorReset)
	}
	return ""
}

func printHelp() {
	fmt.Printf("\n%s%s命令列表%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("  %s/h, /help%s        显示此帮助\n", colorGreen, colorReset)
	fmt.Printf("  %s/n, /new%s         创建新会话\n", colorGreen, colorReset)
	fmt.Printf("  %s/reset%s           重置当前会话（清空消息和 trace）\n", colorGreen, colorReset)
	fmt.Printf("  %s/t, /trace%s       查看执行轨迹\n", colorGreen, colorReset)
	fmt.Printf("  %s/t last%s           查看最后一个 trace 事件\n", colorGreen, colorReset)
	fmt.Printf("  %s/t clear%s          清空 trace\n", colorGreen, colorReset)
	fmt.Printf("  %s/todo%s             查看待办列表\n", colorGreen, colorReset)
	fmt.Printf("  %s/restore%s          回滚到指定检查点\n", colorGreen, colorReset)
	fmt.Printf("  %s/s, /sessions%s    列出所有会话\n", colorGreen, colorReset)
	fmt.Printf("  %s/info%s             显示当前会话信息\n", colorGreen, colorReset)
	fmt.Printf("  %s/exit, /quit%s     退出程序\n", colorGreen, colorReset)
	fmt.Println()
}

func printSessions(project *memory.Project, current *memory.Session) {
	sessions := project.Sessions()
	if len(sessions) == 0 {
		fmt.Println("(no sessions)")
		return
	}
	fmt.Printf("\n%s%s会话列表%s\n", colorBold, colorCyan, colorReset)
	for _, s := range sessions {
		marker := "  "
		if s.ID() == current.ID() {
			marker = fmt.Sprintf("%s→%s ", colorGreen, colorReset)
		}
		_, todoTotal := s.Todo().Progress()
		fmt.Printf("  %s%s  %sagent=%s%s  msgs=%d  todos=%d  %screated=%s%s\n",
			marker,
			s.ID(),
			colorGray, s.AgentName(), colorReset,
			s.MessageCount(),
			todoTotal,
			colorGray, s.CreatedAt().Format("15:04:05"), colorReset,
		)
	}
	fmt.Println()
}

func printSessionInfo(session *memory.Session) {
	done, total := session.Todo().Progress()
	fmt.Printf("\n%s%s会话信息%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("  %sID:%s       %s\n", colorGray, colorReset, session.ID())
	fmt.Printf("  %sAgent:%s    %s\n", colorGray, colorReset, session.AgentName())
	fmt.Printf("  %s消息数:%s   %d\n", colorGray, colorReset, session.MessageCount())
	fmt.Printf("  %sTrace:%s    %d events\n", colorGray, colorReset, session.Trace().Len())
	fmt.Printf("  %sTodo:%s     %d/%d completed\n", colorGray, colorReset, done, total)
	fmt.Printf("  %s创建时间:%s %s\n", colorGray, colorReset, session.CreatedAt().Format("2006-01-02 15:04:05"))
	fmt.Println()
}

// ==================== Trace Command ====================

func handleTraceCommand(trace *memory.Trace, arg string) {
	switch arg {
	case "clear", "c":
		trace.Clear()
		fmt.Printf("%s✓ Trace 已清空%s\n", colorGreen, colorReset)
	case "last", "l":
		e := trace.Last()
		if e == nil {
			fmt.Println("(no trace events)")
		} else {
			fmt.Println(trace.FormatEvent(*e))
		}
	case "stats":
		events := trace.Events()
		if len(events) == 0 {
			fmt.Println("(no trace events)")
			return
		}
		types := make(map[string]int)
		reversible := 0
		for _, e := range events {
			types[string(e.Type)]++
			if e.IsReversible() {
				reversible++
			}
		}
		fmt.Printf("\n%s%sTrace 统计%s\n", colorBold, colorCyan, colorReset)
		fmt.Printf("  %s总事件:%s    %d\n", colorGray, colorReset, len(events))
		fmt.Printf("  %s可恢复:%s    %d\n", colorGray, colorReset, reversible)
		fmt.Printf("  %s不可恢复:%s  %d\n", colorGray, colorReset, len(events)-reversible)
		for typ, count := range types {
			fmt.Printf("  %s%s:%s %d\n", colorGray, typ, colorReset, count)
		}
		fmt.Println()
	case "":
		if trace.Len() == 0 {
			fmt.Println("(no trace events)")
		} else {
			fmt.Println(trace.Format())
		}
	default:
		fmt.Printf("%s未知 trace 命令: %s%s\n", colorRed, arg, colorReset)
		fmt.Println("Usage: /trace [clear|last|stats]")
	}
}

// ==================== Restore Command ====================

func handleRestore(session *memory.Session) {
	trace := session.Trace()
	events := trace.Events()

	// 收集所有 checkpoint 事件
	type checkpointInfo struct {
		index int
		event memory.Event
	}
	var checkpoints []checkpointInfo
	for i, e := range events {
		if e.Type == memory.EventCheckpoint {
			checkpoints = append(checkpoints, checkpointInfo{index: i, event: e})
		}
	}

	if len(checkpoints) == 0 {
		fmt.Printf("%s没有可用的检查点%s\n", colorYellow, colorReset)
		return
	}

	// 显示检查点列表
	fmt.Printf("\n%s%s可用检查点%s\n", colorBold, colorCyan, colorReset)
	for i, cp := range checkpoints {
		ts := cp.event.Timestamp.Format("15:04:05")
		reversible := "✓"
		// 检查该检查点之后是否有不可逆事件
		for j := cp.index + 1; j < len(events); j++ {
			if !events[j].IsReversible() {
				reversible = "✗"
				break
			}
		}
		fmt.Printf("  %s[%d]%s %s %s  %s\n",
			colorGreen, i+1, colorReset, ts, reversible, cp.event.Response)
	}
	fmt.Printf("\n  %s输入检查点编号（1-%d），或 0 取消：%s\n", colorDim, len(checkpoints), colorReset)
	fmt.Print("  > ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return
	}
	input := strings.TrimSpace(scanner.Text())

	var choice int
	if _, err := fmt.Sscanf(input, "%d", &choice); err != nil || choice < 0 || choice > len(checkpoints) {
		fmt.Printf("%s无效输入%s\n", colorRed, colorReset)
		return
	}
	if choice == 0 {
		fmt.Println("已取消")
		return
	}

	selected := checkpoints[choice-1]

	// 计算需要移除的消息数（checkpoint 之后所有事件的 MsgsAdded 总和）
	msgsToRemove := 0
	for i := selected.index + 1; i < len(events); i++ {
		msgsToRemove += len(events[i].MsgsAdded)
	}

	// 执行回滚
	if err := trace.Restore(selected.index); err != nil {
		fmt.Printf("%s回滚失败: %v%s\n", colorRed, err, colorReset)
		return
	}

	// 从 session 中移除消息
	if msgsToRemove > 0 {
		session.RemoveLastMessages(msgsToRemove)
	}

	fmt.Printf("%s✓ 已回滚到检查点: %s%s\n", colorGreen, selected.event.Response, colorReset)
}
