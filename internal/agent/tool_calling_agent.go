package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/memory"
	"my-go-agent/internal/tool"
	"os"
	"strings"
)

// ToolCallingAgent 实现 Agent 接口，执行思考-工具-观察循环
type ToolCallingAgent struct {
	*BaseAgent
}

func NewToolCallingAgent(model llm.ChatModel, tools *tool.Registry) *ToolCallingAgent {
	return &ToolCallingAgent{
		BaseAgent: NewBaseAgent("tool_calling", model, tools),
	}
}

// normalizeJSON 归一化 JSON 字符串，去除多余空格，用于重复调用检测
func normalizeJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\t", "")
	s = strings.ReplaceAll(s, "  ", "")
	return s
}

// extractPath 从工具参数 JSON 中提取 path 字段
func extractPath(args string) string {
	var arg struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(args), &arg); err != nil {
		return ""
	}
	return arg.Path
}

// normalizePath 规范化路径（与 tool.NormalizePath 逻辑一致）
func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	path = strings.ReplaceAll(path, "\\", "/")
	if !strings.HasPrefix(path, "/") {
		return path
	}
	// 绝对路径 → 尝试提取相对部分
	cwd, err := os.Getwd()
	if err != nil {
		return path
	}
	cwdNorm := strings.ReplaceAll(cwd, "\\", "/")
	pathNorm := strings.ReplaceAll(path, "\\", "/")
	if strings.HasPrefix(pathNorm, cwdNorm+"/") {
		return strings.TrimPrefix(pathNorm, cwdNorm+"/")
	}
	// 查找常见项目目录
	parts := strings.Split(pathNorm, "/")
	for i, part := range parts {
		if part == "internal" || part == "cmd" || part == "pkg" || part == "src" {
			return strings.Join(parts[i:], "/")
		}
	}
	return path
}

// truncateArgs 截断参数显示，将换行替换为空格
func truncateArgs(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\t", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

func (a *ToolCallingAgent) Run(ctx context.Context, session *memory.Session, prompt string) (string, error) {
	// 首次调用时添加 system message
	if !session.SystemSet() {
		session.AddMessage(llm.SystemMessage(a.BuildSysPrompt(session)))
		session.MarkSystemSet()
	}
	session.AddMessage(llm.UserMessage(prompt))

	trace := session.Trace()
	trace.RecordUser(0, prompt)
	// 自动创建检查点，便于回滚
	cpName := prompt
	if len(cpName) > 60 {
		cpName = cpName[:60] + "..."
	}
	trace.RecordCheckpoint(0, cpName)
	res := ""
	calledTools := make(map[string]int)

	for step := 1; step <= a.MaxStep; step++ {
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		default:
		}

		messages := session.Messages(ctx)
		req := &llm.ChatRequest{
			Messages: messages,
			Tools:    a.Tools.ToLLMTool(),
		}

		// 流式输出：累积 chunk 用于 trace 记录
		var streamBuf strings.Builder
		resp, err := a.Model.StreamChat(ctx, req, func(chunk string) {
			if chunk != "" {
				streamBuf.WriteString(chunk)
				fmt.Print(chunk)
			}
		})
		if err != nil {
			return "", err
		}
		// 模型输出完成后换行
		fmt.Println()

		// 优先使用流式累积内容，fallback 到 resp.Content
		llmOutput := streamBuf.String()
		if llmOutput == "" {
			llmOutput = resp.Content
		}

		// 没有 tool call，说明 agent 认为目标已完成
		if len(resp.ToolCalls) == 0 {
			if llmOutput != "" {
				res += llmOutput
				session.AddMessage(llm.AssistantMessage(llmOutput))
			}
			trace.RecordResponse(step, llmOutput)
			return res, nil
		}

		// 有 tool call：content + tool_calls 合并为一条消息
		if llmOutput != "" {
			res += llmOutput
		}
		assistantMsg := llm.AssistantMessageWithToolCall(llmOutput, resp.ToolCalls)
		session.AddMessage(assistantMsg)
		// RecordThought 只记录展示内容，不记录 MsgsAdded（由 RecordToolCall 统一记录）
		trace.RecordThought(step, llm.Message{}, llmOutput)

		var duplicateWarnings []string
		var toolMsgs []llm.Message
		var toolRecords []memory.ToolCallRecord
		var allFileChanges []memory.FileChange

		for _, call := range resp.ToolCalls {
			callKey := call.Function.Name + "|" + normalizeJSON(call.Function.Arguments)
			calledTools[callKey]++

			if calledTools[callKey] > 1 {
				duplicateWarnings = append(duplicateWarnings,
					fmt.Sprintf("[警告] 工具 %s 已被调用过 %d 次（参数: %s），请勿重复调用",
						call.Function.Name, calledTools[callKey]-1, call.Function.Arguments))
			}

			// 工具调用可视化提示
			fmt.Printf("  ⏺ %s(%s)\n", call.Function.Name, truncateArgs(call.Function.Arguments, 80))

			// 检测路径修正
			originalPath := extractPath(call.Function.Arguments)
			if originalPath != "" {
				normalizedPath := normalizePath(originalPath)
				if normalizedPath != originalPath {
					trace.RecordPathFix(step, originalPath, normalizedPath)
				}
			}

			// 在执行前检测文件变更（捕获 before 状态）
			fileChanges := detectFileChanges(call.Function.Name, call.Function.Arguments)
			allFileChanges = append(allFileChanges, fileChanges...)

			// 注入 session 到 context，供 ExpertAsTool 等需要 session 的工具使用
			toolCtx := WithSession(ctx, session)
			result := a.Tools.Execute(toolCtx, call.Function.Name, call.Function.Arguments)

			// 检测沙箱拦截
			if strings.HasPrefix(result, "[SANDBOX]") {
				reason := strings.TrimPrefix(result, "[SANDBOX] ")
				trace.RecordSandboxDeny(step, call.Function.Name, reason)
			}

			toolMsg := llm.ToolMessage(call.ID, result)
			session.AddMessage(toolMsg)
			toolMsgs = append(toolMsgs, toolMsg)
			toolRecords = append(toolRecords, memory.ToolCallRecord{
				Name:   call.Function.Name,
				Args:   call.Function.Arguments,
				Result: result,
				CallID: call.ID,
			})
		}

		// MsgsAdded = assistant(with tool_calls) + 所有 tool results
		// 确保回滚时删除完整的消息组，不留下孤立的 assistant 消息
		allMsgs := append([]llm.Message{assistantMsg}, toolMsgs...)
		trace.RecordToolCall(step, allMsgs, toolRecords, allFileChanges)

		if len(duplicateWarnings) > 0 {
			warningMsg := strings.Join(duplicateWarnings, "\n")
			session.AddMessage(llm.SystemMessage(warningMsg))
		}
	}
	return res, fmt.Errorf("[agent] reached max step %d", a.MaxStep)
}

// detectFileChanges 在工具执行前检测文件变更，捕获 before 状态
func detectFileChanges(toolName string, argsStr string) []memory.FileChange {
	switch toolName {
	case "write_file":
		return detectWriteFileChanges(argsStr)
	case "edit_file":
		return detectEditFileChanges(argsStr)
	default:
		return nil
	}
}

// detectWriteFileChanges 检测 write_file 的文件变更
func detectWriteFileChanges(argsStr string) []memory.FileChange {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Append  bool   `json:"append"`
	}
	if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
		return nil
	}

	before, _ := os.ReadFile(args.Path)
	beforeStr := string(before)

	op := memory.FileCreate
	if len(before) > 0 {
		op = memory.FileModify
	}

	after := args.Content
	if args.Append {
		after = beforeStr + args.Content
	}

	return []memory.FileChange{{
		Path:   args.Path,
		Op:     op,
		Before: beforeStr,
		After:  after,
	}}
}

// detectEditFileChanges 检测 edit_file 的文件变更
func detectEditFileChanges(argsStr string) []memory.FileChange {
	var args struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
		All       bool   `json:"all"`
	}
	if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
		return nil
	}

	before, err := os.ReadFile(args.Path)
	if err != nil {
		return nil
	}
	beforeStr := string(before)

	// 预估 after 内容
	after := beforeStr
	if args.All {
		after = strings.ReplaceAll(after, args.OldString, args.NewString)
	} else {
		after = strings.Replace(after, args.OldString, args.NewString, 1)
	}

	return []memory.FileChange{{
		Path:   args.Path,
		Op:     memory.FileModify,
		Before: beforeStr,
		After:  after,
	}}
}
