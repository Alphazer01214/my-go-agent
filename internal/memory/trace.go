package memory

import (
	"fmt"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/utils"
	"os"
	"strings"
	"sync"
	"time"
)

// trace 记录事件，与 session 一一对应
// event 记录状态
// diff 记录差分

type EventType string

const (
	EventUser      EventType = "user"       // 用户输入
	EventThought   EventType = "thought"    // LLM 推理/中间响应
	EventToolCall  EventType = "tool_call"  // 工具调用
	EventResponse  EventType = "response"   // 最终响应（agent 完成）
	EventSandbox   EventType = "sandbox"    // 沙箱拦截
	EventRestore   EventType = "restore"    // 检查点恢复（不可逆）
	EventCompress  EventType = "compress"   // 消息压缩（可恢复：通过快照恢复）
)

// FileOp 文件操作类型，执行 tool 的时候记录，这些都是用于 agent 层面记录的
type FileOp string

const (
	FileCreate FileOp = "create"
	FileModify FileOp = "modify"
	FileDelete FileOp = "delete" // 不可逆
	DirCreate  FileOp = "mkdir"  // 创建目录
)

// FileChange 文件变更记录（用于 trace 展示和回滚）
type FileChange struct {
	Path   string `json:"path"`
	Op     FileOp `json:"op"`
	Before string `json:"before,omitempty"` // 操作前内容（create 时为空）
	After  string `json:"after,omitempty"`  // 操作后内容（delete 时为空）
}

// Diff 记录单个事件的状态差分
type Diff struct {
	eventId     string
	msgAdded    int          // 本事件新增的消息数量（兼容旧逻辑）
	fileChanges []FileChange // 本事件的文件变更（Before/After 完整内容）
	// 消息 ID 差分（精确回滚）
	addedMsgs   []string     // 本事件新增的消息 ID（回滚时移除）
	removedMsgs []string     // 因压缩而移除的消息 ID（回滚时恢复）
	// 压缩事件专用：保存压缩前的完整消息和摘要快照
	messages []llm.Message // 压缩前的全部消息（用于回退恢复）
	summary  string        // 压缩前的摘要文本
}

// Reverse 返回反向差分（用于回滚）
// 如果包含 FileDelete 操作，返回 nil（不可逆）
func (d *Diff) Reverse() *Diff {
	r := &Diff{eventId: d.eventId, msgAdded: d.msgAdded}
	for _, fc := range d.fileChanges {
		rfc := FileChange{Path: fc.Path}
		switch fc.Op {
		case FileCreate:
			// 回滚创建 = 删除文件
			rfc.Op = FileDelete
			rfc.Before = fc.After
		case FileModify:
			// 回滚修改 = 恢复旧内容
			rfc.Op = FileModify
			rfc.Before = fc.After
			rfc.After = fc.Before
		case FileDelete:
			// 删除不可逆
			return nil
		case DirCreate:
			// 回滚目录创建 = 尝试删除空目录（非阻塞）
			rfc.Op = DirCreate
			rfc.After = "revert"
		}
		r.fileChanges = append(r.fileChanges, rfc)
	}
	return r
}

// Event 精确到 tool call 级别
// json 导出供前端查看
type Event struct {
	Id string `json:"id"`
	// ParentId 一般是 agent 相关事件追溯用户事件
	ParentId string    `json:"parent_id,omitempty"`
	Type     EventType `json:"type"`
	// Recoverable 设为 false 则回溯过程跳过该事件
	Recoverable bool `json:"recoverable"`

	// 状态信息
	// messages 记录该 event 产生的消息
	Messages    []llm.Message `json:"messages"`
	AgentName   string        `json:"agent_name"`
	Subject     string        `json:"subject"` // user or model
	Timestamp   time.Time     `json:"timestamp"`
	DiffSummary string        `json:"diff_summary"`

	diff *Diff
}

// newEvent 创建新事件，自动生成 ID
func newEvent() *Event {
	return &Event{
		Id: utils.GenId("event"),
	}
}

// IsRecoverable 返回此事件是否可恢复
func (e *Event) IsRecoverable() bool {
	return e.Recoverable
}

// ToolCall 工具调用记录
// args 需要通过 unmarshal
type ToolCall struct {
	Name   string `json:"name"`
	Args   string `json:"args"`
	Result string `json:"result"`
	CallID string `json:"call_id"`
}

// Trace 记录一个 Agent 的执行轨迹
// 每个 Agent 拥有独立的 Trace，通过 parent 形成树结构
type Trace struct {
	mu        sync.Mutex
	agentName string
	parent    *Trace   // 父 Agent 的 Trace（nil = 根）
	children  []*Trace // 子 Agent 的 Trace
	events    []Event  // 本 Agent 的事件

	latestEventId string
	messageIDs    []string // 记录用户 message 的 id 用于用户会话回滚
}

func NewTrace(agentName string) *Trace {
	return &Trace{
		agentName: agentName,
	}
}

func (t *Trace) NewChildTrace(childAgentName string) *Trace {
	child := &Trace{
		agentName: childAgentName,
		parent:    t,
	}
	t.mu.Lock()
	t.children = append(t.children, child)
	t.mu.Unlock()
	return child
}

func (t *Trace) Events() []Event {
	t.mu.Lock()
	defer t.mu.Unlock()
	dst := make([]Event, len(t.events))
	copy(dst, t.events)
	return dst
}

// AllEvents 返回本 Agent 及所有子 Agent 的事件（扁平化）
func (t *Trace) AllEvents() []Event {
	t.mu.Lock()
	result := make([]Event, len(t.events))
	copy(result, t.events)
	children := make([]*Trace, len(t.children))
	copy(children, t.children)
	t.mu.Unlock()

	for _, child := range children {
		result = append(result, child.AllEvents()...)
	}
	return result
}

// AddEvent 添加事件
func (t *Trace) AddEvent(e Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if e.Id == "" {
		e.Id = utils.GenId("evt")
	}
	e.Timestamp = time.Now()
	t.events = append(t.events, e)
	t.latestEventId = e.Id
}

// AddUserMessageId 记录用户 message 的 id
func (t *Trace) AddUserMessageId(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.messageIDs = append(t.messageIDs, id)
}

// UserMessageIDs 返回用户 message 的 id 列表
func (t *Trace) UserMessageIDs() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	dst := make([]string, len(t.messageIDs))
	copy(dst, t.messageIDs)
	return dst
}

func (t *Trace) AgentName() string {
	return t.agentName
}

func (t *Trace) Parent() *Trace {
	return t.parent
}

func (t *Trace) Children() []*Trace {
	t.mu.Lock()
	defer t.mu.Unlock()
	dst := make([]*Trace, len(t.children))
	copy(dst, t.children)
	return dst
}

// EventCnt 返回事件数量
func (t *Trace) EventCnt() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.events)
}

// Clear 清空 trace
func (t *Trace) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = nil
	t.latestEventId = ""
	t.messageIDs = nil
}

// AddUserEvent 记录用户输入事件
// 用户事件本身可回滚（从消息中移除），但通常不需要
func (t *Trace) AddUserEvent(step int, content string, addedMsgs []string) {
	t.AddEvent(Event{
		Type:        EventUser,
		Subject:     truncate(content, 80),
		Recoverable: true,
		diff:        &Diff{addedMsgs: addedMsgs},
	})
}

// AddThoughtEvent 记录 LLM 推理事件
// 思考事件可回滚（移除 assistant 消息）
func (t *Trace) AddThoughtEvent(step int, msg llm.Message, stepContent string, addedMsgs []string) {
	t.AddEvent(Event{
		Type:        EventThought,
		Messages:    []llm.Message{msg},
		Subject:     truncate(stepContent, 80),
		Recoverable: true,
		diff:        &Diff{msgAdded: 1, addedMsgs: addedMsgs},
	})
}

// AddToolCallEvent 记录工具调用事件
// calls: 本步执行的工具调用记录
// fileChanges: 本步的文件变更（正向，含 Before/After 完整内容）
// msgsAdded: 本步新增到 Session 的消息数量（assistant + tool results）
// toolNames: 本步调用的工具名称列表（用于判断可回溯性）
// addedMsgs: 本步新增的消息 ID 列表（用于精确回滚）
func (t *Trace) AddToolCallEvent(step int, calls []ToolCall, fileChanges []FileChange, msgsAdded int, toolNames []string, addedMsgs []string) {
	// 判断是否可恢复：
	// - 有文件变更时：FileDelete 不可逆，其余可逆
	// - 无文件变更时：只读工具（read_file/list_dir/glob/search_file）可回溯，
	//   exec_command 等副作用不可确定的工具不可回溯
	recoverable := true
	if len(fileChanges) > 0 {
		for _, fc := range fileChanges {
			if fc.Op == FileDelete {
				recoverable = false
				break
			}
		}
	} else {
		// 无文件变更：只读工具可回溯，其余不可回溯
		readOnlyTools := map[string]bool{
			"read_file": true, "list_dir": true,
			"glob": true, "search_file": true,
		}
		allReadOnly := true
		for _, name := range toolNames {
			if !readOnlyTools[name] {
				allReadOnly = false
				break
			}
		}
		if !allReadOnly {
			recoverable = false
		}
	}

	// 格式化变更摘要
	summary := formatFileChanges(fileChanges)
	if len(calls) > 0 {
		parts := make([]string, 0, len(calls))
		for _, c := range calls {
			parts = append(parts, truncate(c.Name+"("+c.Args+")", 60))
		}
		callSummary := strings.Join(parts, "; ")
		if summary != "" {
			summary = callSummary + " [" + summary + "]"
		} else {
			summary = callSummary
		}
	}

	t.AddEvent(Event{
		Type:        EventToolCall,
		Recoverable: recoverable,
		diff:        &Diff{msgAdded: msgsAdded, fileChanges: fileChanges, addedMsgs: addedMsgs},
		DiffSummary: summary,
	})
}

// AddResponseEvent 记录最终响应事件
// 响应事件可回滚（只是移除消息），Recoverable=true
func (t *Trace) AddResponseEvent(step int, content string, addedMsgs []string) {
	t.AddEvent(Event{
		Type:        EventResponse,
		Subject:     truncate(content, 120),
		Recoverable: true,
		diff:        &Diff{msgAdded: 1, addedMsgs: addedMsgs},
	})
}

// AddSandboxEvent 记录沙箱拦截
func (t *Trace) AddSandboxEvent(step int, toolName, reason string) {
	t.AddEvent(Event{
		Type:        EventSandbox,
		Subject:     fmt.Sprintf("%s: %s", toolName, reason),
		Recoverable: false,
		diff:        &Diff{},
	})
}

// AddRestoreEvent 记录恢复操作（不可逆）
func (t *Trace) AddRestoreEvent(targetId, description string) {
	t.AddEvent(Event{
		Type:        EventRestore,
		Subject:     description,
		Recoverable: false,
		diff:        &Diff{},
	})
}

// AddCompressEvent 记录压缩操作（可恢复：通过快照恢复压缩前的消息）
// messages: 压缩前的全部消息快照
// summary: 压缩前的摘要文本
// msgsAfter: 压缩后的可见消息数量
// removedMsgs: 因压缩而隐藏的消息 ID 列表
func (t *Trace) AddCompressEvent(messages []llm.Message, summary string, msgsAfter int, removedMsgs []string) {
	// 计算被压缩的消息数（压缩前总数 - 压缩后可见数）
	msgsRemoved := len(messages) - msgsAfter
	summaryText := fmt.Sprintf("compressed %d messages → %d visible", len(messages), msgsAfter)
	if summary != "" {
		summaryText += " (had prior summary)"
	}
	t.AddEvent(Event{
		Type:        EventCompress,
		Subject:     summaryText,
		Recoverable: true,
		diff: &Diff{
			eventId:     "", // AddEvent 会自动生成 ID
			msgAdded:    msgsRemoved,
			removedMsgs: removedMsgs,
			messages:    messages,
			summary:     summary,
		},
		DiffSummary: summaryText,
	})
}

// RestoreResult 回滚结果
type RestoreResult struct {
	RolledBack    int      // 成功回滚的事件数
	Skipped       int      // 因不可逆而跳过文件回滚的事件数
	FilesReverted int      // 成功回滚的文件数
	SkippedEvents []string // 被跳过的事件摘要（type + subject）
}

// Restore 回滚到指定事件（eventId 对应的事件保留，之后的事件全部回滚）
// 硬回溯模式：
//   - 消息回滚：所有事件的消息都可回滚
//   - 文件回滚：Recoverable=true 的事件正常回滚文件，Recoverable=false 的跳过文件回滚
//   - 如果文件回滚过程中发生 IO 错误，返回 error（此时已部分回滚）
func (t *Trace) Restore(eventId string, session *Session) (*RestoreResult, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// 1. 找到目标事件
	targetIdx := -1
	for i, e := range t.events {
		if e.Id == eventId {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		return nil, fmt.Errorf("event not found: %s", eventId)
	}
	if targetIdx == len(t.events)-1 {
		return nil, fmt.Errorf("already at target event, nothing to restore")
	}

	result := &RestoreResult{}

	// 2. 逆序回滚（硬回溯：遇到不可逆事件跳过文件回滚，但继续回滚消息）
	for i := len(t.events) - 1; i > targetIdx; i-- {
		evt := &t.events[i]

		// 压缩事件：恢复被压缩隐藏的消息
		if evt.Type == EventCompress && evt.diff != nil {
			// 优先用 removedMsgs 精确恢复，fallback 到快照
			if len(evt.diff.removedMsgs) > 0 {
				session.RestoreMessagesByID(evt.diff.removedMsgs)
			} else if evt.diff.messages != nil {
				session.SetMessages(evt.diff.messages)
			}
			session.SetSummary(evt.diff.summary)
			result.RolledBack++
			continue
		}

		if evt.diff == nil {
			result.RolledBack++
			continue
		}

		if evt.Recoverable {
			// 可恢复：回滚文件 + 消息
			for j := len(evt.diff.fileChanges) - 1; j >= 0; j-- {
				fc := evt.diff.fileChanges[j]
				if err := revertFile(fc); err != nil {
					// 文件回滚失败，记录已回滚状态后返回 error
					return result, fmt.Errorf("revert file %s at event %d (%s): %w",
						fc.Path, i, evt.Type, err)
				}
				result.FilesReverted++
			}
			// 优先用 addedMsgs 精确移除，fallback 到计数
			if len(evt.diff.addedMsgs) > 0 {
				session.RemoveMessagesByID(evt.diff.addedMsgs)
			} else if evt.diff.msgAdded > 0 {
				session.RemoveLastMessages(evt.diff.msgAdded)
			}
			result.RolledBack++
		} else {
			// 不可恢复：跳过文件回滚，但仍回滚消息
			if len(evt.diff.addedMsgs) > 0 {
				session.RemoveMessagesByID(evt.diff.addedMsgs)
			} else if evt.diff.msgAdded > 0 {
				session.RemoveLastMessages(evt.diff.msgAdded)
			}
			result.Skipped++
			result.SkippedEvents = append(result.SkippedEvents,
				fmt.Sprintf("[%s] %s", evt.Type, truncate(evt.Subject, 60)))
		}
	}

	// 3. 截断事件列表（保留 target 及之前的所有事件）
	t.events = t.events[:targetIdx+1]
	t.latestEventId = t.events[targetIdx].Id

	// 4. 记录恢复事件（不可逆）
	desc := fmt.Sprintf("Restored to %s (rolled_back=%d, skipped=%d, files=%d)",
		eventId, result.RolledBack, result.Skipped, result.FilesReverted)
	if len(result.SkippedEvents) > 0 {
		desc += "\n  Skipped file rollback: " + strings.Join(result.SkippedEvents, ", ")
	}
	restoreEvt := Event{
		Id:          utils.GenId("evt"),
		Type:        EventRestore,
		Subject:     desc,
		Recoverable: false,
		Timestamp:   time.Now(),
		diff:        &Diff{},
	}
	t.events = append(t.events, restoreEvt)
	t.latestEventId = restoreEvt.Id

	return result, nil
}

// CanRestore 检查是否可以回滚到指定事件
// 硬回溯模式下，只要有后续事件就可以尝试回滚
// 返回 (ok, willSkip, description)
func (t *Trace) CanRestore(eventId string) (bool, bool, string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	targetIdx := -1
	for i, e := range t.events {
		if e.Id == eventId {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		return false, false, "event not found"
	}
	if targetIdx == len(t.events)-1 {
		return false, false, "already at target event"
	}

	// 检查是否有不可逆事件（会跳过文件回滚）
	willSkip := false
	for i := targetIdx + 1; i < len(t.events); i++ {
		if !t.events[i].Recoverable {
			willSkip = true
			break
		}
	}

	if willSkip {
		return true, true, "some events have irreversible file changes (messages will still be rolled back)"
	}
	return true, false, ""
}

// revertFile 回滚单个文件/目录变更
func revertFile(fc FileChange) error {
	switch fc.Op {
	case FileModify:
		// 恢复文件内容
		return os.WriteFile(fc.Path, []byte(fc.Before), 0644)
	case FileCreate:
		// 回滚创建 = 删除文件
		return os.Remove(fc.Path)
	case FileDelete:
		// 删除不可逆（应该在预检查中就被拦截）
		return fmt.Errorf("cannot revert file deletion: %s", fc.Path)
	case DirCreate:
		// 回滚目录创建 = 尝试删除空目录
		// 如果目录非空（后续有文件写入），静默跳过
		if err := os.Remove(fc.Path); err != nil {
			// 目录非空或其他错误，跳过（不视为回滚失败）
			return nil
		}
		return nil
	default:
		return fmt.Errorf("unknown file op: %s", fc.Op)
	}
}

// Format 格式化完整 trace 输出
func (t *Trace) Format() string {
	events := t.Events()
	if len(events) == 0 {
		return "(no trace events)"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("═══ Trace: %s ═══\n", t.agentName))
	sb.WriteString(fmt.Sprintf("Events: %d\n", len(events)))

	for _, e := range events {
		sb.WriteString(t.FormatEvent(e))
	}
	return sb.String()
}

// FormatEvent 格式化单个事件
func (t *Trace) FormatEvent(e Event) string {
	ts := e.Timestamp.Format("15:04:05.000")
	rev := "✓"
	if !e.Recoverable {
		rev = "✗"
	}

	switch e.Type {
	case EventUser:
		return fmt.Sprintf("\n[%s] %s  user\n  %s\n", e.Id, ts, e.Subject)
	case EventThought:
		return fmt.Sprintf("\n[%s] %s  thought\n  %s\n", e.Id, ts, e.Subject)
	case EventToolCall:
		sb := fmt.Sprintf("\n[%s] %s  tool_call %s\n", e.Id, ts, rev)
		if e.DiffSummary != "" {
			sb += fmt.Sprintf("%s\n", e.DiffSummary)
		}
		if e.diff != nil {
			for _, fc := range e.diff.fileChanges {
				sb += formatFileChangeDetail(fc)
			}
		}
		return sb
	case EventResponse:
		return fmt.Sprintf("\n[%s] %s  response\n  %s\n", e.Id, ts, e.Subject)
	case EventSandbox:
		return fmt.Sprintf("\n[%s] %s  sandbox\n  %s\n", e.Id, ts, e.Subject)
	case EventCompress:
		return fmt.Sprintf("\n[%s] %s  compress (recoverable)\n  %s\n", e.Id, ts, e.Subject)
	case EventRestore:
		return fmt.Sprintf("\n[%s] %s  restore (irreversible)\n  %s\n", e.Id, ts, e.Subject)
	default:
		return fmt.Sprintf("\n[%s] %s  %s\n", e.Id, ts, e.Type)
	}
}

// Last 返回最后一个事件
func (t *Trace) Last() *Event {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.events) == 0 {
		return nil
	}
	return &t.events[len(t.events)-1]
}

// formatFileChanges 格式化文件变更摘要（单行）
func formatFileChanges(changes []FileChange) string {
	if len(changes) == 0 {
		return ""
	}
	parts := make([]string, 0, len(changes))
	for _, fc := range changes {
		parts = append(parts, fmt.Sprintf("%s[%s]", fc.Path, fc.Op))
	}
	return strings.Join(parts, ", ")
}

// formatFileChangeDetail 格式化单个文件变更详情（多行）
func formatFileChangeDetail(fc FileChange) string {
	sb := fmt.Sprintf("  📄 %s [%s]\n", fc.Path, fc.Op)
	if fc.Op != FileDelete && fc.Before != "" {
		sb += fmt.Sprintf("    before: %s\n", truncate(fc.Before, 80))
	}
	if fc.Op != FileDelete && fc.After != "" {
		sb += fmt.Sprintf("    after:  %s\n", truncate(fc.After, 80))
	}
	return sb
}

// truncate 截断字符串，将换行替换为空格
func truncate(s string, maxLen int) string {
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
