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
)

// ==================== Command Dispatch ====================

func handleCommand(input string, session *memory.Session, project *memory.Project, a agent.Agent, tools *tool.Registry, cm llm.ChatModel) string {
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
	case "/context", "/c":
		handleContext(session)
	case "/compress":
		handleCompress(session)
	case "/pwd":
		handlePwd(session, project)
	case "/model":
		handleModel(cm, parts)
	default:
		fmt.Printf("%s未知命令: %s%s (输入 /help 查看命令列表)\n", colorRed, cmd, colorReset)
	}
	return ""
}

// ==================== Help ====================

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
	fmt.Printf("  %s/c, /context%s      显示当前上下文 token 数和消息组统计\n", colorGreen, colorReset)
	fmt.Printf("  %s/compress%s         手动触发历史消息压缩（LLM 摘要）\n", colorGreen, colorReset)
	fmt.Printf("  %s/pwd%s              显示当前工作目录和项目信息\n", colorGreen, colorReset)
	fmt.Printf("  %s/s, /sessions%s    列出所有会话\n", colorGreen, colorReset)
	fmt.Printf("  %s/info%s             显示当前会话信息\n", colorGreen, colorReset)
	fmt.Printf("  %s/model [name]%s    查看或切换 LLM 模型（如 deepseek-chat, qwen-turbo）\n", colorGreen, colorReset)
	fmt.Printf("  %s/exit, /quit%s     退出程序\n", colorGreen, colorReset)
	fmt.Println()
}

// ==================== Sessions ====================

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

// ==================== Messages ====================

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

// ==================== Context ====================

func handleContext(session *memory.Session) {
	msgs := session.RawMessages()
	tokens := session.EstimateTokens()
	groups := memory.MessageGroups(msgs)

	// 统计消息类型
	var userCnt, assistantCnt, toolCnt, systemCnt int
	for _, msg := range msgs {
		switch msg.Role {
		case llm.RoleSystem:
			systemCnt++
		case llm.RoleUser:
			userCnt++
		case llm.RoleAssistant:
			assistantCnt++
		case llm.RoleTool:
			toolCnt++
		}
	}

	fmt.Printf("\n%s%s上下文信息%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("  %s总消息数:%s      %d (含已压缩的旧消息)\n", colorGray, colorReset, len(msgs))
	fmt.Printf("  %s模型可见:%s      %d\n", colorGray, colorReset, len(msgs)) // visible = total - compressCount
	fmt.Printf("  %s消息组数:%s      %d\n", colorGray, colorReset, len(groups))
	fmt.Printf("  %s估算 tokens:%s   %d (仅模型可见部分)\n", colorGray, colorReset, tokens)
	fmt.Printf("  %s消息分布:%s      system=%d  user=%d  assistant=%d  tool=%d\n",
		colorGray, colorReset, systemCnt, userCnt, assistantCnt, toolCnt)
	fmt.Printf("  %s压缩器:%s       ", colorGray, colorReset)
	if session.GetCompressor() != nil {
		fmt.Printf("已配置\n")
	} else {
		fmt.Printf("未配置\n")
	}
	fmt.Println()
}

// ==================== Compress ====================

func handleCompress(session *memory.Session) {
	ctx := context.Background()
	before := session.EstimateTokens()
	beforeCount := session.MessageCount()

	if session.GetCompressor() == nil {
		fmt.Printf("%s未配置压缩器，无法执行 LLM 摘要压缩%s\n", colorYellow, colorReset)
		return
	}

	fmt.Printf("%s正在压缩...%s\n", colorDim, colorReset)
	if err := session.Compress(ctx, session.Trace()); err != nil {
		fmt.Printf("%s压缩失败: %v%s\n", colorRed, err, colorReset)
		return
	}

	after := session.EstimateTokens()
	afterCount := session.MessageCount()
	fmt.Printf("%s✓ 压缩完成%s\n", colorGreen, colorReset)
	fmt.Printf("  %s消息:%s %d → %d\n", colorGray, colorReset, beforeCount, afterCount)
	fmt.Printf("  %sTokens:%s %d → %d (节省 %d)\n", colorGray, colorReset, before, after, before-after)
}

// ==================== Model ====================

func handleModel(cm llm.ChatModel, parts []string) {
	if len(parts) > 1 {
		newModel := parts[1]
		oldModel := cm.GetModel()
		cm.SetModel(newModel)
		fmt.Printf("%s✓ 模型已切换：%s %s -> %s\n", colorGreen, colorReset, oldModel, newModel)
	} else {
		fmt.Printf("当前模型: %s\n", cm.GetModel())
		fmt.Printf("用法: /model <model_name>  切换模型（如 deepseek-chat, qwen-turbo）\n")
	}
}

// ==================== Pwd ====================

func handlePwd(session *memory.Session, project *memory.Project) {
	fmt.Printf("\n%s%s项目信息%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("  %s工作目录:%s %s\n", colorGray, colorReset, project.Path())
	fmt.Printf("  %s项目名称:%s %s\n", colorGray, colorReset, project.Name())
	fmt.Printf("  %s会话 ID:%s  %s\n", colorGray, colorReset, session.ID())
	fmt.Printf("  %sAgent:%s    %s\n", colorGray, colorReset, session.AgentName())
	fmt.Println()
}

// ==================== Trace ====================

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

// ==================== Restore ====================

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
