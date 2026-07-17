package memory

//
//import (
//	"fmt"
//	"my-go-agent/internal/llm"
//	"os"
//	"strings"
//	"sync"
//	"time"
//)
//
//// EventType trace 事件类型
//type EventType string
//
//const (
//	EventUser         EventType = "user"         // 用户输入
//	EventThought      EventType = "thought"      // LLM 推理/中间响应
//	EventToolCall     EventType = "tool_call"    // 工具调用
//	EventResponse     EventType = "response"     // 最终响应（agent 完成）
//	EventCheckpoint   EventType = "checkpoint"   // 手动检查点
//	EventRestore      EventType = "restore"      // 检查点恢复（不可逆）
//	EventSandboxDeny  EventType = "sandbox_deny" // 沙箱拦截（未执行）
//	EventPathFix      EventType = "path_fix"     // 路径自动修正
//)
//
//// FileOp 文件操作类型
//type FileOp string
//
//const (
//	FileCreate FileOp = "create" // 创建新文件
//	FileModify FileOp = "modify" // 修改已有文件
//	FileDelete FileOp = "delete" // 删除文件（不可逆）
//)
//
//// FileChange 文件变更记录
//type FileChange struct {
//	Path    string // 文件路径
//	Op      FileOp // 操作类型
//	Before  string // 操作前内容（create 时为空）
//	After   string // 操作后内容（delete 时为空）
//}
//
//// ToolCallRecord 工具调用记录
//type ToolCallRecord struct {
//	Name   string // 工具名称
//	Args   string // 调用参数
//	Result string // 执行结果
//	CallID string // 调用 ID
//}
//
//// Event 单个 trace 事件，通过 ID/ParentID 形成链式结构
//type Event struct {
//	ID        string          // 唯一标识
//	ParentID  string          // 父事件 ID（空 = 链头）
//	Step      int
//	Timestamp time.Time
//	Type      EventType
//	Response  string          // 本步骤的 LLM 输出内容
//	MsgsAdded []llm.Message   // 本步新增到 Session 的消息
//	ToolCalls []ToolCallRecord // 本步执行的工具调用
//	FileDiffs []FileChange    // 本步的文件变更（正向）
//	Reverse   *ReverseDiff    // 反向差分（nil = 不可逆）
//}
//
//// IsReversible 返回此事件是否可逆
//func (e *Event) IsReversible() bool {
//	return e.Reverse != nil
//}
//
//// ReverseDiff 反向差分，用于还原事件造成的影响
//type ReverseDiff struct {
//	MsgsToRemove  int           // 需要移除的消息数量（从尾部）
//	FilesToRevert []FileChange  // 需要还原的文件变更
//}
//
//// Trace 记录执行轨迹，事件通过 ID/ParentID 形成链式结构
//type Trace struct {
//	mu        sync.Mutex
//	id        string
//	agentName string
//	startTime time.Time
//	events    []Event
//	lastID    string // 上一个事件的 ID（空 = 无）
//	seq       int    // 事件序号计数器
//	response  string
//}
//
//// NewTrace 创建新的 Trace 实例
//func NewTrace(agentName string) *Trace {
//	return &Trace{
//		id:        fmt.Sprintf("trace_%d", time.Now().UnixMilli()),
//		agentName: agentName,
//		startTime: time.Now(),
//	}
//}
//
//// appendEvent 追加事件并维护 ID/ParentID 链
//func (t *Trace) appendEvent(e Event) {
//	t.seq++
//	e.ID = fmt.Sprintf("e%d", t.seq)
//	e.ParentID = t.lastID
//	e.Timestamp = time.Now()
//	t.events = append(t.events, e)
//	t.lastID = e.ID
//	if e.Response != "" {
//		t.response = e.Response
//	}
//}
//
//// RecordUser 记录用户输入事件
//func (t *Trace) RecordUser(step int, content string) {
//	t.mu.Lock()
//	defer t.mu.Unlock()
//	t.appendEvent(Event{
//		Step:     step,
//		Type:     EventUser,
//		Response: content,
//		Reverse:  &ReverseDiff{}, // 用户输入可逆（从消息中移除）
//	})
//}
//
//// RecordThought 记录 LLM 推理事件
//func (t *Trace) RecordThought(step int, msg llm.Message, stepContent string) {
//	t.mu.Lock()
//	defer t.mu.Unlock()
//	t.appendEvent(Event{
//		Step:      step,
//		Type:      EventThought,
//		Response:  stepContent,
//		MsgsAdded: []llm.Message{msg},
//		Reverse:   &ReverseDiff{MsgsToRemove: 1},
//	})
//}
//
//// RecordToolCall 记录工具调用事件
//// fileChanges: 本次工具调用产生的文件变更（正向）
//// reversible: 文件操作是否全部可逆
//func (t *Trace) RecordToolCall(step int, msgs []llm.Message, calls []ToolCallRecord, fileChanges []FileChange) {
//	reversible := true
//	var reverseFiles []FileChange
//
//	// 构建反向文件差分
//	for i := len(fileChanges) - 1; i >= 0; i-- {
//		fc := fileChanges[i]
//		switch fc.Op {
//		case FileCreate:
//			// 还原创建 = 删除文件
//			reverseFiles = append(reverseFiles, FileChange{
//				Path: fc.Path, Op: FileDelete, Before: fc.After,
//			})
//		case FileModify:
//			// 还原修改 = 恢复旧内容
//			reverseFiles = append(reverseFiles, FileChange{
//				Path: fc.Path, Op: FileModify, Before: fc.After, After: fc.Before,
//			})
//		case FileDelete:
//			// 删除不可逆
//			reversible = false
//		}
//	}
//
//	var rev *ReverseDiff
//	if reversible {
//		rev = &ReverseDiff{
//			MsgsToRemove:  len(msgs),
//			FilesToRevert: reverseFiles,
//		}
//	}
//
//	t.mu.Lock()
//	defer t.mu.Unlock()
//	t.appendEvent(Event{
//		Step:      step,
//		Type:      EventToolCall,
//		MsgsAdded: msgs,
//		ToolCalls: calls,
//		FileDiffs: fileChanges,
//		Reverse:   rev,
//	})
//}
//
//// RecordResponse 记录最终响应事件
//func (t *Trace) RecordResponse(step int, content string) {
//	t.mu.Lock()
//	defer t.mu.Unlock()
//	t.appendEvent(Event{
//		Step:     step,
//		Type:     EventResponse,
//		Response: content,
//		Reverse:  &ReverseDiff{MsgsToRemove: 1},
//	})
//}
//
//// RecordCheckpoint 记录手动检查点（可逆，但恢复操作不可逆）
//func (t *Trace) RecordCheckpoint(step int, name string) {
//	t.mu.Lock()
//	defer t.mu.Unlock()
//	t.appendEvent(Event{
//		Step:     step,
//		Type:     EventCheckpoint,
//		Response: name,
//		Reverse:  &ReverseDiff{},
//	})
//}
//
//// RecordSandboxDeny 记录沙箱拦截事件（未执行，无状态变更，可恢复）
//func (t *Trace) RecordSandboxDeny(step int, toolName, reason string) {
//	t.mu.Lock()
//	defer t.mu.Unlock()
//	t.appendEvent(Event{
//		Step:     step,
//		Type:     EventSandboxDeny,
//		Response: fmt.Sprintf("%s: %s", toolName, reason),
//		Reverse:  &ReverseDiff{}, // 未执行，无状态变更，可恢复
//	})
//}
//
//// RecordPathFix 记录路径修正事件（信息性，不影响回滚）
//func (t *Trace) RecordPathFix(step int, original, fixed string) {
//	t.mu.Lock()
//	defer t.mu.Unlock()
//	t.appendEvent(Event{
//		Step:     step,
//		Type:     EventPathFix,
//		Response: fmt.Sprintf("%s → %s", original, fixed),
//		Reverse:  &ReverseDiff{}, // 信息性事件，可逆（无实际操作）
//	})
//}
//
//// Restore 还原到指定检查点
//// 从最后一个事件向前遍历，逐个应用反向差分
//// 如果遇到不可逆操作，返回错误
//// 恢复操作本身是不可逆的
//func (t *Trace) Restore(checkpointIdx int) error {
//	t.mu.Lock()
//	defer t.mu.Unlock()
//
//	if checkpointIdx < 0 || checkpointIdx >= len(t.events) {
//		return fmt.Errorf("invalid checkpoint index: %d", checkpointIdx)
//	}
//
//	cp := t.events[checkpointIdx]
//	if cp.Type != EventCheckpoint {
//		return fmt.Errorf("event %d is not a checkpoint (type: %s)", checkpointIdx, cp.Type)
//	}
//
//	// 检查从 checkpoint 到末尾的所有事件是否可逆
//	for i := checkpointIdx + 1; i < len(t.events); i++ {
//		if !t.events[i].IsReversible() {
//			return fmt.Errorf("cannot restore: event %d (type: %s) is irreversible",
//				i, t.events[i].Type)
//		}
//	}
//
//	// 从后向前应用反向差分
//	for i := len(t.events) - 1; i > checkpointIdx; i-- {
//		e := &t.events[i]
//		if e.Reverse == nil {
//			continue
//		}
//
//		// 还原文件变更
//		for _, fc := range e.Reverse.FilesToRevert {
//			if err := revertFile(fc); err != nil {
//				return fmt.Errorf("revert file %s: %w", fc.Path, err)
//			}
//		}
//	}
//
//	// 截断事件列表（保留 checkpoint 及之前的所有事件）
//	t.events = t.events[:checkpointIdx+1]
//	t.lastID = t.events[checkpointIdx].ID
//
//	// 记录恢复事件（不可逆）
//	t.appendEvent(Event{
//		Step:     cp.Step,
//		Type:     EventRestore,
//		Response: fmt.Sprintf("Restored to checkpoint %d: %s", checkpointIdx, cp.Response),
//		Reverse:  nil, // 恢复操作不可逆
//	})
//
//	return nil
//}
//
//// revertFile 还原单个文件变更
//func revertFile(fc FileChange) error {
//	switch fc.Op {
//	case FileModify:
//		// 恢复文件内容
//		return os.WriteFile(fc.Path, []byte(fc.Before), 0644)
//	case FileCreate:
//		// 还原创建 = 删除文件
//		return os.Remove(fc.Path)
//	case FileDelete:
//		// 还原删除 = 创建文件（如果 Before 不为空）
//		if fc.Before != "" {
//			return os.WriteFile(fc.Path, []byte(fc.Before), 0644)
//		}
//		return nil
//	default:
//		return fmt.Errorf("unknown file op: %s", fc.Op)
//	}
//}
//
//// Events 返回所有事件的快照（线程安全）
//func (t *Trace) Events() []Event {
//	t.mu.Lock()
//	defer t.mu.Unlock()
//	dst := make([]Event, len(t.events))
//	copy(dst, t.events)
//	return dst
//}
//
//// Last 返回最后一个事件，无事件返回 nil
//func (t *Trace) Last() *Event {
//	t.mu.Lock()
//	defer t.mu.Unlock()
//	if len(t.events) == 0 {
//		return nil
//	}
//	return &t.events[len(t.events)-1]
//}
//
//// Len 返回事件数量
//func (t *Trace) Len() int {
//	t.mu.Lock()
//	defer t.mu.Unlock()
//	return len(t.events)
//}
//
//// Clear 清空 trace
//func (t *Trace) Clear() {
//	t.mu.Lock()
//	defer t.mu.Unlock()
//	t.events = nil
//	t.lastID = ""
//	t.seq = 0
//	t.response = ""
//	t.startTime = time.Now()
//}
//
//// ==================== 格式化输出 ====================
//
//// Format 格式化完整 trace 输出
//func (t *Trace) Format() string {
//	events := t.Events()
//	if len(events) == 0 {
//		return "(no trace events)"
//	}
//
//	var sb strings.Builder
//	sb.WriteString(fmt.Sprintf("═══ Trace: %s ═══\n", t.agentName))
//	sb.WriteString(fmt.Sprintf("Started: %s | Events: %d\n",
//		t.startTime.Format("15:04:05"), len(events)))
//
//	for _, e := range events {
//		sb.WriteString(formatEventBlock(e))
//	}
//	return sb.String()
//}
//
//// FormatEvent 格式化单个事件为一行（表格格式）
//func (t *Trace) FormatEvent(e Event) string {
//	return FormatEvent(e)
//}
//
//// FormatEvent 格式化单个事件为一行
//func FormatEvent(e Event) string {
//	ts := e.Timestamp.Format("15:04:05.000")
//	rev := "✓"
//	if !e.IsReversible() {
//		rev = "✗"
//	}
//	summary := summarizeEvent(e)
//	return fmt.Sprintf(" %2d  %-12s  %-10s  %-50s  %s",
//		e.Step, ts, e.Type, summary, rev)
//}
//
//// formatEventBlock 格式化单个事件为块状显示（仿 OpenCode 风格）
//func formatEventBlock(e Event) string {
//	ts := e.Timestamp.Format("15:04:05.000")
//	var sb strings.Builder
//
//	switch e.Type {
//	case EventUser:
//		sb.WriteString(fmt.Sprintf("\n [%s] %s  user\n", e.ID, ts))
//		sb.WriteString(fmt.Sprintf("  %s\n", truncateStr(e.Response, 200)))
//
//	case EventThought:
//		sb.WriteString(fmt.Sprintf("\n [%s] %s  step %d  thought\n", e.ID, ts, e.Step))
//		content := e.Response
//		if content == "" {
//			content = "(empty)"
//		}
//		sb.WriteString(fmt.Sprintf("  %s\n", truncateStr(content, 200)))
//
//	case EventToolCall:
//		rev := "✓"
//		if !e.IsReversible() {
//			rev = "✗"
//		}
//		sb.WriteString(fmt.Sprintf("\n [%s] %s  step %d  tool_call %s\n", e.ID, ts, e.Step, rev))
//		for _, tc := range e.ToolCalls {
//			sb.WriteString(fmt.Sprintf("  ⏺ %s(%s)\n", tc.Name, truncateStr(tc.Args, 60)))
//			sb.WriteString(fmt.Sprintf("    └─ %s\n", truncateStr(tc.Result, 100)))
//		}
//		for _, fc := range e.FileDiffs {
//			sb.WriteString(fmt.Sprintf("  📄 %s [%s]\n", fc.Path, fc.Op))
//			if fc.Op != FileDelete && fc.Before != "" {
//				sb.WriteString(fmt.Sprintf("    before: %s\n", truncateStr(fc.Before, 80)))
//			}
//			if fc.Op != FileDelete && fc.After != "" {
//				sb.WriteString(fmt.Sprintf("    after:  %s\n", truncateStr(fc.After, 80)))
//			}
//		}
//
//	case EventResponse:
//		sb.WriteString(fmt.Sprintf("\n [%s] %s  step %d  response ✓\n", e.ID, ts, e.Step))
//		content := e.Response
//		if content == "" {
//			content = "(empty)"
//		}
//		sb.WriteString(fmt.Sprintf("  %s\n", truncateStr(content, 300)))
//
//	case EventCheckpoint:
//		sb.WriteString(fmt.Sprintf("\n [%s] %s  step %d  checkpoint ✓\n", e.ID, ts, e.Step))
//		if e.Response != "" {
//			sb.WriteString(fmt.Sprintf("  %s\n", truncateStr(e.Response, 100)))
//		}
//
//	case EventRestore:
//		sb.WriteString(fmt.Sprintf("\n [%s] %s  step %d  restore ✗ (irreversible)\n", e.ID, ts, e.Step))
//		sb.WriteString(fmt.Sprintf("  %s\n", truncateStr(e.Response, 100)))
//
//	case EventSandboxDeny:
//		sb.WriteString(fmt.Sprintf("\n [%s] %s  step %d  sandbox ✗\n", e.ID, ts, e.Step))
//		sb.WriteString(fmt.Sprintf("  %s\n", truncateStr(e.Response, 200)))
//
//	case EventPathFix:
//		sb.WriteString(fmt.Sprintf("\n [%s] %s  step %d  path_fix\n", e.ID, ts, e.Step))
//		sb.WriteString(fmt.Sprintf("  %s\n", truncateStr(e.Response, 200)))
//
//	default:
//		sb.WriteString(fmt.Sprintf("\n [%s] %s  step %d  %s\n", e.ID, ts, e.Step, e.Type))
//	}
//
//	return sb.String()
//}
//
//// summarizeEvent 生成事件摘要
//func summarizeEvent(e Event) string {
//	switch e.Type {
//	case EventUser:
//		return truncateStr(e.Response, 80)
//
//	case EventThought:
//		content := e.Response
//		if content == "" {
//			return "(empty)"
//		}
//		return truncateStr(content, 80)
//
//	case EventToolCall:
//		if len(e.ToolCalls) == 0 {
//			return "tool_call (no details)"
//		}
//		parts := make([]string, 0, len(e.ToolCalls))
//		for _, tc := range e.ToolCalls {
//			parts = append(parts, fmt.Sprintf("%s(%s)", tc.Name, truncateStr(tc.Args, 30)))
//		}
//		s := strings.Join(parts, "; ")
//		// 附加文件变更信息
//		if len(e.FileDiffs) > 0 {
//			s += fmt.Sprintf(" [%d file changes]", len(e.FileDiffs))
//		}
//		return truncateStr(s, 100)
//
//	case EventResponse:
//		return truncateStr(e.Response, 120)
//
//	case EventCheckpoint:
//		return truncateStr(e.Response, 60)
//
//	case EventRestore:
//		return truncateStr(e.Response, 80)
//
//	case EventSandboxDeny:
//		return truncateStr(e.Response, 100)
//
//	case EventPathFix:
//		return truncateStr(e.Response, 80)
//
//	default:
//		return string(e.Type)
//	}
//}
//
//func truncateStr(s string, maxLen int) string {
//	s = flattenText(s)
//	if len(s) <= maxLen {
//		return s
//	}
//	return s[:maxLen] + "..."
//}
//
//// flattenText 将换行替换为空格，压缩连续空格
//func flattenText(s string) string {
//	s = strings.ReplaceAll(s, "\n", " ")
//	s = strings.ReplaceAll(s, "\r", "")
//	s = strings.ReplaceAll(s, "\t", " ")
//	// 压缩连续空格
//	for strings.Contains(s, "  ") {
//		s = strings.ReplaceAll(s, "  ", " ")
//	}
//	return strings.TrimSpace(s)
//}
