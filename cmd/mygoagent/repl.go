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
	"os/signal"
	"strings"
	"syscall"
)

// ==================== Interactive Run ====================

func runInteractive(a agent.Agent, session *memory.Session, project *memory.Project, tools *tool.Registry, cm llm.ChatModel) {
	// 信号处理：Ctrl+C 优雅退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Printf("\n%s再见！%s\n", colorDim, colorReset)
		os.Exit(0)
	}()

	printWelcome(cm.GetModel(), session)

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
			cmd := handleCommand(input, session, project, a, tools, cm)
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
