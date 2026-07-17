package main

import (
	"bufio"
	"bytes"
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
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

// bracketed paste 终端协议标记
var (
	pasteStart = []byte("\x1b[200~")
	pasteEnd   = []byte("\x1b[201~")
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
	// 用当前目录作为 project
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s[project]%s get cwd: %v\n", colorRed, colorReset, err)
		os.Exit(1)
	}
	projectName := filepath.Base(dir)
	project, err := memory.NewProject(memory.ProjectOpts{
		Name:        projectName,
		ContextPath: ".memory/context.md",
		Compressor:  memory.NewSummaryCompressor(cm, config.MemorySize),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s[project]%s init: %v\n", colorRed, colorReset, err)
		os.Exit(1)
	}

	// 创建 agent
	a := agent.NewToolCallingAgent(fmt.Sprintf("%v_tool_calling_agent", projectName), cm, tools)
	a.SetMaxStep(config.MaxSteps)

	// 创建 Session
	session := project.NewSession(a.Name())

	if len(os.Args) > 1 {
		runSingle(a, session, tools, strings.Join(os.Args[1:], " "))
	} else {
		runInteractive(a, session, project, tools, model)
	}
}

// scanLinesOrPaste 自定义 scanner split 函数
// 支持两种模式：
//   - 普通输入：按行分割（与 ScanLines 相同）
//   - bracketed paste：检测到 \x1b[200~ 后，一直读到 \x1b[201~，整段作为一个 token
//     终端在粘贴时会自动加上这些转义序列，粘贴内容中的换行不会被截断
var inPaste bool

func scanLinesOrPaste(data []byte, atEOF bool) (advance int, token []byte, err error) {
	// 正在粘贴模式中：寻找 pasteEnd
	if inPaste {
		if idx := bytes.Index(data, pasteEnd); idx != -1 {
			inPaste = false
			// 返回 pasteStart 和 pasteEnd 之间的内容
			content := data[:idx]
			content = bytes.TrimRight(content, "\r\n")
			return idx + len(pasteEnd), content, nil
		}
		// 还没收到 pasteEnd，等更多数据
		if atEOF && len(data) > 0 {
			// EOF 但没收到结束标记，按普通行处理
			inPaste = false
			return len(data), bytes.TrimRight(data, "\r\n"), nil
		}
		return 0, nil, nil
	}

	// 检测 pasteStart
	if idx := bytes.Index(data, pasteStart); idx != -1 {
		// pasteStart 之前有普通内容，先返回它
		if idx > 0 {
			before := data[:idx]
			line := bytes.TrimRight(before, "\r\n")
			if len(line) > 0 {
				return idx, line, nil
			}
		}
		// 进入粘贴模式，跳过 pasteStart，继续读
		inPaste = true
		return idx + len(pasteStart), nil, nil
	}

	// 普通行模式：找换行符
	if idx := bytes.IndexByte(data, '\n'); idx != -1 {
		line := data[:idx]
		// 处理 \r\n
		if idx > 0 && line[idx-1] == '\r' {
			line = line[:idx-1]
		}
		return idx + 1, line, nil
	}

	// atEOF 且有剩余数据
	if atEOF && len(data) > 0 {
		return len(data), bytes.TrimRight(data, "\r\n"), nil
	}

	return 0, nil, nil
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
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
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

// ==================== Interactive Run ====================

func runInteractive(a agent.Agent, session *memory.Session, project *memory.Project, tools *tool.Registry, model string) {
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
	scanner.Split(scanLinesOrPaste)
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
			cmd := handleCommand(input, session, project, a, tools)
			if cmd == "new_session" {
				session = project.NewSession(a.Name())
				fmt.Printf("%s✓ 新会话已创建%s\n", colorGreen, colorReset)
			}
			continue
		}

		// 执行 agent
		ctx, cancel := context.WithCancel(context.Background())
		tools.ResetSandbox()
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
	fmt.Printf("%s%s║        my-go-agent                   ║%s\n", colorBold, colorCyan, colorReset)
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

func handleCommand(input string, session *memory.Session, project *memory.Project, a agent.Agent, tools *tool.Registry) string {
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
		eventId := ""
		if len(parts) > 1 {
			eventId = parts[1]
		}
		handleRestore(session, eventId)
	case "/revert":
		// 快捷回滚到上一个可恢复事件
		handleQuickRevert(session)
	case "/messages", "/m":
		handleMessages(session)
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
	fmt.Printf("  %s/t stats%s          查看 trace 统计\n", colorGreen, colorReset)
	fmt.Printf("  %s/t clear%s          清空 trace\n", colorGreen, colorReset)
	fmt.Printf("  %s/todo%s             查看待办列表\n", colorGreen, colorReset)
	fmt.Printf("  %s/restore <id>%s     硬回溯到指定事件\n", colorGreen, colorReset)
	fmt.Printf("  %s/revert%s           快速回滚到上一个工具调用前\n", colorGreen, colorReset)
	fmt.Printf("  %s/m, /messages%s     列出当前会话所有消息\n", colorGreen, colorReset)
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
		fmt.Printf("  %s%s  %sagent=%s%s  msgs=%d  events=%d  todos=%d  %screated=%s%s\n",
			marker,
			s.ID(),
			colorGray, s.AgentName(), colorReset,
			s.MessageCount(),
			s.Trace().EventCnt(),
			todoTotal,
			colorGray, s.CreatedAt().Format("15:04:05"), colorReset,
		)
	}
	fmt.Println()
}

func printSessionInfo(session *memory.Session) {
	done, total := session.Todo().Progress()
	trace := session.Trace()
	events := trace.Events()

	recoverable := 0
	for _, e := range events {
		if e.IsRecoverable() {
			recoverable++
		}
	}

	fmt.Printf("\n%s%s会话信息%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("  %sID:%s       %s\n", colorGray, colorReset, session.ID())
	fmt.Printf("  %sAgent:%s    %s\n", colorGray, colorReset, session.AgentName())
	fmt.Printf("  %s消息数:%s   %d\n", colorGray, colorReset, session.MessageCount())
	fmt.Printf("  %sTrace:%s    %d events (%d recoverable, %d irreversible)\n",
		colorGray, colorReset, len(events), recoverable, len(events)-recoverable)
	fmt.Printf("  %sTodo:%s     %d/%d completed\n", colorGray, colorReset, done, total)
	fmt.Printf("  %s创建时间:%s %s\n", colorGray, colorReset, session.CreatedAt().Format("2006-01-02 15:04:05"))
	fmt.Println()
}

// ==================== Messages Command ====================

func handleMessages(session *memory.Session) {
	msgs := session.RawMessages()
	if len(msgs) == 0 {
		fmt.Println("(no messages)")
		return
	}

	fmt.Printf("\n%s%s消息列表 (%d)%s\n", colorBold, colorCyan, len(msgs), colorReset)
	for i, msg := range msgs {
		idx := i + 1
		ts := ""
		if msg.Timestamp > 0 {
			ts = time.UnixMilli(msg.Timestamp).Format("15:04:05.000")
		}

		switch msg.Role {
		case llm.RoleSystem:
			fmt.Printf("  %s[%2d]%s %s %s%s%s %s\n",
				colorGray, idx, colorReset, ts,
				colorDim, msg.Role, colorReset,
				truncateDisplay(msg.Content, 100))
		case llm.RoleUser:
			fmt.Printf("  %s[%2d]%s %s %s%s%s %s\n",
				colorGreen, idx, colorReset, ts,
				colorBold, msg.Role, colorReset,
				truncateDisplay(msg.Content, 100))
		case llm.RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				fmt.Printf("  %s[%2d]%s %s %s%s%s %s tool_calls=[%d]\n",
					colorBlue, idx, colorReset, ts,
					colorBold, msg.Role, colorReset,
					truncateDisplay(msg.Content, 80),
					len(msg.ToolCalls))
				for _, tc := range msg.ToolCalls {
					fmt.Printf("       %s→ %s(%s)%s\n",
						colorCyan, tc.Function.Name,
						truncateDisplay(tc.Function.Arguments, 60), colorReset)
				}
			} else {
				fmt.Printf("  %s[%2d]%s %s %s%s%s %s\n",
					colorBlue, idx, colorReset, ts,
					colorBold, msg.Role, colorReset,
					truncateDisplay(msg.Content, 100))
			}
		case llm.RoleTool:
			fmt.Printf("  %s[%2d]%s %s %s%s%s call=%s %s\n",
				colorYellow, idx, colorReset, ts,
				colorDim, msg.Role, colorReset,
				msg.ToolCallID[:min(12, len(msg.ToolCallID))],
				truncateDisplay(msg.Content, 80))
		default:
			fmt.Printf("  %s[%2d]%s %s %s %s\n",
				colorGray, idx, colorReset, ts, msg.Role,
				truncateDisplay(msg.Content, 100))
		}
	}
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
		recoverable := 0
		for _, e := range events {
			types[string(e.Type)]++
			if e.IsRecoverable() {
				recoverable++
			}
		}
		fmt.Printf("\n%s%sTrace 统计%s\n", colorBold, colorCyan, colorReset)
		fmt.Printf("  %s总事件:%s      %d\n", colorGray, colorReset, len(events))
		fmt.Printf("  %s可恢复:%s      %d\n", colorGray, colorReset, recoverable)
		fmt.Printf("  %s不可恢复:%s    %d\n", colorGray, colorReset, len(events)-recoverable)
		for typ, count := range types {
			fmt.Printf("  %s%s:%s %d\n", colorGray, typ, colorReset, count)
		}
		fmt.Println()
	case "":
		if trace.EventCnt() == 0 {
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

// handleRestore 硬回溯到指定事件
// 如果未指定 eventId，显示事件列表让用户选择
func handleRestore(session *memory.Session, eventId string) {
	trace := session.Trace()
	events := trace.Events()

	if len(events) == 0 {
		fmt.Printf("%s没有可回滚的事件%s\n", colorYellow, colorReset)
		return
	}

	// 如果未指定 eventId，显示事件列表
	if eventId == "" {
		eventId = selectEventFromList(events)
		if eventId == "" {
			return
		}
	}

	// 检查是否可以回滚
	ok, willSkip, reason := trace.CanRestore(eventId)
	if !ok {
		fmt.Printf("%s无法回滚: %s%s\n", colorRed, reason, colorReset)
		return
	}

	if willSkip {
		fmt.Printf("%s⚠ 存在不可逆事件，文件回滚将被跳过（消息仍会回滚）%s\n", colorYellow, colorReset)
		fmt.Printf("  %s继续? (y/N):%s ", colorDim, colorReset)
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() || strings.TrimSpace(strings.ToLower(scanner.Text())) != "y" {
			fmt.Println("已取消")
			return
		}
	}

	// 执行硬回溯
	result, err := session.RestoreToEvent(eventId)
	if err != nil {
		fmt.Printf("%s回滚失败: %v%s\n", colorRed, err, colorReset)
		return
	}

	// 显示回滚结果
	fmt.Printf("%s✓ 回滚完成%s\n", colorGreen, colorReset)
	fmt.Printf("  %s回滚事件:%s  %d\n", colorGray, colorReset, result.RolledBack)
	if result.Skipped > 0 {
		fmt.Printf("  %s跳过文件:%s  %d (不可逆)\n", colorYellow, colorReset, result.Skipped)
		for _, s := range result.SkippedEvents {
			fmt.Printf("    %s⚠ %s%s\n", colorYellow, s, colorReset)
		}
	}
	if result.FilesReverted > 0 {
		fmt.Printf("  %s恢复文件:%s  %d\n", colorGray, colorReset, result.FilesReverted)
	}
}

// handleQuickRevert 快速回滚到最近的 tool_call 事件之前
func handleQuickRevert(session *memory.Session) {
	trace := session.Trace()
	events := trace.Events()

	if len(events) == 0 {
		fmt.Printf("%s没有可回滚的事件%s\n", colorYellow, colorReset)
		return
	}

	// 从后往前找最近的 tool_call 事件
	targetIdx := -1
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == memory.EventToolCall {
			targetIdx = i
			break
		}
	}

	if targetIdx <= 0 {
		fmt.Printf("%s没有找到可回滚的工具调用事件%s\n", colorYellow, colorReset)
		return
	}

	// 回滚到 tool_call 之前的那个事件
	targetId := events[targetIdx-1].Id
	handleRestore(session, targetId)
}

// selectEventFromList 显示事件列表，让用户选择回滚目标
func selectEventFromList(events []memory.Event) string {
	fmt.Printf("\n%s%s选择回滚目标%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("  %s输入事件编号 (1-%d)，或 0 取消：%s\n\n", colorDim, len(events), colorReset)

	// 显示事件列表（最多显示最近 20 个）
	start := 0
	if len(events) > 20 {
		start = len(events) - 20
		fmt.Printf("  %s... 省略前 %d 个事件 ...%s\n\n", colorDim, start, colorReset)
	}

	for i := start; i < len(events); i++ {
		e := events[i]
		ts := e.Timestamp.Format("15:04:05")
		rev := colorGreen + "✓" + colorReset
		if !e.IsRecoverable() {
			rev = colorYellow + "⚠" + colorReset
		}
		idx := i + 1
		fmt.Printf("  %s[%2d]%s %s %s %-10s %s\n",
			colorGreen, idx, colorReset, ts, rev, e.Type,
			truncateDisplay(e.Subject, 60))
	}
	fmt.Println()

	fmt.Print("  > ")
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return ""
	}
	input := strings.TrimSpace(scanner.Text())

	var choice int
	if _, err := fmt.Sscanf(input, "%d", &choice); err != nil || choice < 0 || choice > len(events) {
		fmt.Printf("%s无效输入%s\n", colorRed, colorReset)
		return ""
	}
	if choice == 0 {
		fmt.Println("已取消")
		return ""
	}

	return events[choice-1].Id
}

// truncateDisplay 截断字符串用于显示
func truncateDisplay(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
