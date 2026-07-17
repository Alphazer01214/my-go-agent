package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/memory"
	"my-go-agent/internal/tool"
	"my-go-agent/internal/utils"
	"os"
	"strings"
)

// ToolCallingAgent 实现 Agent 接口，执行思考-工具-观察循环
// 作为基础 agent
type ToolCallingAgent struct {
	*BaseAgent
}

func NewToolCallingAgent(name string, model llm.ChatModel, tools *tool.Registry) *ToolCallingAgent {
	return &ToolCallingAgent{
		BaseAgent: NewBaseAgent(name, model, tools),
	}
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
	trace.AddUserEvent(0, prompt)

	res := ""
	calledTools := make(map[string]int)

	// 一般一个 step 就是一次 thought + tool call + response
	for step := 1; step <= a.maxStep; step++ {
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		default:
		}

		messages := session.Messages(ctx)
		req := &llm.ChatRequest{
			Messages: messages,
			Tools:    a.tools.ToLLMTool(),
		}

		var streamBuf strings.Builder
		resp, err := a.model.StreamChat(ctx, req, func(chunk string) {
			if chunk != "" {
				streamBuf.WriteString(chunk)
				fmt.Print(chunk)
			}
		})
		if err != nil {
			return "", err
		}
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
				trace.AddResponseEvent(step, llmOutput)
			}
			return res, nil
		}

		// 有 tool call：content + tool_calls 合并为一条消息
		if llmOutput != "" {
			res += llmOutput
		}
		assistantMsg := llm.AssistantMessageWithToolCall(llmOutput, resp.ToolCalls)
		session.AddMessage(assistantMsg)

		// 这个用来警告重复调用
		//var duplicateWarnings []string
		var toolRecords []memory.ToolCall
		var allFileChanges []memory.FileChange
		msgsAddedThisStep := 1 // assistant 消息

		for _, call := range resp.ToolCalls {
			callKey := call.Function.Name + "|" + utils.NormalizeJSON(call.Function.Arguments)
			calledTools[callKey]++

			//if calledTools[callKey] > 1 {
			//	duplicateWarnings = append(duplicateWarnings,
			//		fmt.Sprintf("[警告] 工具 %s 已被调用过 %d 次（参数: %s），请勿重复调用",
			//			call.Function.Name, calledTools[callKey]-1, call.Function.Arguments))
			//}

			// 工具调用可视化提示
			fmt.Printf("%s(%s)\n", call.Function.Name, truncateArgs(call.Function.Arguments, 80))

			// 在执行前捕获文件 before-state
			fileChangesBefore := detectFileChangesBefore(call.Function.Name, call.Function.Arguments)

			// 注入 session 到 context，供 ExpertAsTool 等需要 session 的工具使用
			toolCtx := WithSession(ctx, session)
			result := a.tools.Execute(toolCtx, call.Function.Name, call.Function.Arguments)

			// 检测沙箱拦截
			if strings.HasPrefix(result, "[SANDBOX]") {
				reason := strings.TrimPrefix(result, "[SANDBOX] ")
				trace.AddSandboxEvent(step, call.Function.Name, reason)
				continue
			}

			// 在执行后补全文件 after-state
			fileChanges := detectFileChangesAfter(fileChangesBefore, call.Function.Name, call.Function.Arguments)
			allFileChanges = append(allFileChanges, fileChanges...)

			toolMsg := llm.ToolMessage(call.ID, result)
			session.AddMessage(toolMsg)
			msgsAddedThisStep++ // tool result 消息

			toolRecords = append(toolRecords, memory.ToolCall{
				Name:   call.Function.Name,
				Args:   call.Function.Arguments,
				Result: result,
				CallID: call.ID,
			})
		}

		// 记录工具调用事件（含 assistant + tool results 的消息数）
		trace.AddToolCallEvent(step, toolRecords, allFileChanges, msgsAddedThisStep)

		//if len(duplicateWarnings) > 0 {
		//	warningMsg := strings.Join(duplicateWarnings, "\n")
		//	session.AddMessage(llm.SystemMessage(warningMsg))
		//}
	}
	return res, fmt.Errorf("[agent] reached max step %d", a.maxStep)
}

// ==================== File Change Detection ====================

// fileChangeBefore 工具执行前的文件状态快照
type fileChangeBefore struct {
	path   string
	op     memory.FileOp
	before string
	// 保存工具参数，用于 after 检测
	toolName string
	args     string
}

// detectFileChangesBefore 在工具执行前检测文件变更，捕获 before-state
func detectFileChangesBefore(toolName string, argsStr string) []fileChangeBefore {
	switch toolName {
	case "write_file":
		return detectWriteFileBefore(argsStr)
	case "edit_file":
		return detectEditFileBefore(argsStr)
	case "make_dir":
		return detectMakeDirBefore(argsStr)
	default:
		return nil
	}
}

// detectFileChangesAfter 在工具执行后补全 after-state，生成完整的 FileChange
func detectFileChangesAfter(befores []fileChangeBefore, toolName string, argsStr string) []memory.FileChange {
	if len(befores) == 0 {
		return nil
	}

	var changes []memory.FileChange
	for _, b := range befores {
		fc := memory.FileChange{
			Path:   b.path,
			Op:     b.op,
			Before: b.before,
		}

		if b.op == memory.DirCreate {
			// 目录创建：检查目录是否存在
			info, err := os.Stat(b.path)
			if err != nil || !info.IsDir() {
				// 创建失败，跳过
				continue
			}
			fc.After = b.path
		} else {
			// 文件操作：读取工具执行后的文件内容
			afterData, err := os.ReadFile(b.path)
			if err != nil {
				// 文件可能被删除或不可读
				if b.op == memory.FileCreate {
					// 创建失败，跳过
					continue
				}
				// 修改失败，After 为空
				fc.After = ""
			} else {
				fc.After = string(afterData)
			}

			// 如果 before 和 after 相同，说明没有实际变更
			if fc.Before == fc.After && b.op != memory.FileCreate {
				continue
			}
		}

		changes = append(changes, fc)
	}
	return changes
}

// detectWriteFileBefore 检测 write_file 的 before-state
func detectWriteFileBefore(argsStr string) []fileChangeBefore {
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

	return []fileChangeBefore{{
		path:   args.Path,
		op:     op,
		before: beforeStr,
	}}
}

// detectEditFileBefore 检测 edit_file 的 before-state
func detectEditFileBefore(argsStr string) []fileChangeBefore {
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

	return []fileChangeBefore{{
		path:   args.Path,
		op:     memory.FileModify,
		before: string(before),
	}}
}

// detectMakeDirBefore 检测 make_dir 的 before-state
func detectMakeDirBefore(argsStr string) []fileChangeBefore {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
		return nil
	}

	// 检查目录是否已存在
	info, err := os.Stat(args.Path)
	if err == nil && info.IsDir() {
		// 目录已存在，无变更
		return nil
	}

	return []fileChangeBefore{{
		path:   args.Path,
		op:     memory.DirCreate,
		before: "",
	}}
}
