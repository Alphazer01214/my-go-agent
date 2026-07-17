package memory

import (
	"context"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/utils"
	"sync"
	"time"
)

// context key for session
type sessionKey struct{}

// WithSession 将 Session 注入 context
func WithSession(ctx context.Context, s *Session) context.Context {
	return context.WithValue(ctx, sessionKey{}, s)
}

// SessionFromContext 从 context 中获取 Session
func SessionFromContext(ctx context.Context) *Session {
	s, _ := ctx.Value(sessionKey{}).(*Session)
	return s
}

// Session 管理一次对话的完整状态：消息历史、执行轨迹、压缩策略、待办列表
type Session struct {
	mu        sync.RWMutex
	id        string
	parent    *Session
	agentName string
	// chat model settings
	model   string // 使用模型例如 deepseek mimo
	apiKey  string
	baseUrl string

	createdAt time.Time
	// memory
	messages   []llm.Message
	windowSize int
	assistant  llm.ChatModel // 辅助决策
	systemSet  bool          // system message 是否已添加
	trace      *Trace
	todoList   *TodoList
	project    *Project
	metadata   map[string]string

	// 消息压缩
	compressor    Compressor // 压缩器（由 Project 注入）
	summary       string     // 累积的摘要文本（压缩后注入 LLM 上下文）
	autoTruncate  int        // tool result 自动截断阈值（字符数），0 表示不截断
	compressCount int        // 已压缩的旧消息数量（Messages 返回时跳过前 N 条，但消息仍保留在 s.messages 中）
}

// defaultAutoTruncate 默认 tool result 截断阈值
const defaultAutoTruncate = 4096

// NewSession 创建新的 Session
// 自动识别 project 产生 project 配置
func NewSession(project *Project, agent string) *Session {
	agentName := agent
	if agentName == "" {
		agentName = utils.GenId("agent")
	}
	s := &Session{
		id:           utils.GenId("session"),
		agentName:    agentName,
		createdAt:    time.Now(),
		trace:        NewTrace(agentName),
		todoList:     NewTodoList(),
		project:      project,
		metadata:     make(map[string]string),
		autoTruncate: defaultAutoTruncate,
	}
	// 从 Project 继承压缩器
	if project != nil && project.compressor != nil {
		s.compressor = project.compressor
	}
	return s
}

func (s *Session) SetModel(model string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.model = model
}

func (s *Session) Model() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.model
}

// WithWindowSize WithMemoryAssistant 这两个用于记忆优化
func (s *Session) WithWindowSize(size int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.windowSize = size
}

func (s *Session) WithMemoryAssistant(model llm.ChatModel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assistant = model
}

// WithCompressor 设置消息压缩器
func (s *Session) WithCompressor(c Compressor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.compressor = c
}

// GetCompressor 返回当前压缩器（供外部检查是否已配置）
func (s *Session) GetCompressor() Compressor {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.compressor
}

// ID 返回 session 唯一标识
func (s *Session) ID() string { return s.id }

// AgentName 返回当前使用的 agent 名称
func (s *Session) AgentName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.agentName
}

// SetAgent 切换当前 agent
func (s *Session) SetAgent(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentName = name
}

// CreatedAt 返回创建时间
func (s *Session) CreatedAt() time.Time { return s.createdAt }

// Project 返回所属项目
func (s *Session) Project() *Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.project
}

// Trace 返回执行轨迹
func (s *Session) Trace() *Trace { return s.trace }

// Todo 返回待办列表
func (s *Session) Todo() *TodoList { return s.todoList }

// AddMessage 向 session 添加一条消息
// tool 结果消息会自动截断到 autoTruncate 阈值
func (s *Session) AddMessage(msg llm.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 截断过大的 tool 结果
	if msg.Role == llm.RoleTool && s.autoTruncate > 0 && len(msg.Content) > s.autoTruncate {
		msg.Content = msg.Content[:s.autoTruncate] + "\n... [truncated]"
	}
	s.messages = append(s.messages, msg)
}

// Messages 返回模型可见的消息列表（供 LLM 调用使用）
// 自动触发压缩，并在返回时注入累积的摘要上下文
// 压缩后的旧消息仍在 s.messages 中（通过 compressCount 跳过），支持 trace 回退恢复
func (s *Session) Messages(ctx context.Context) []llm.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 自动压缩（内部按需获取写锁）
	s.autoCompressInternal(ctx)

	// 构建返回列表：summary + 可见消息（跳过已压缩的旧消息）
	var result []llm.Message
	if s.summary != "" {
		result = append(result, llm.SystemMessage("[历史摘要]\n"+s.summary))
	}
	result = append(result, s.messages[s.compressCount:]...)
	return result
}

// RawMessages 返回原始消息列表（不压缩）
func (s *Session) RawMessages() []llm.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	msgs := make([]llm.Message, len(s.messages))
	copy(msgs, s.messages)
	return msgs
}

// MessageCount 返回消息数量
func (s *Session) MessageCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.messages)
}

// RemoveLastMessages 从末尾移除消息（用于回滚）
// 自动保证消息组完整性：不会留下孤立的 assistant(tool_calls) 或 tool 消息
func (s *Session) RemoveLastMessages(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n <= 0 || len(s.messages) == 0 {
		return
	}
	if n >= len(s.messages) {
		s.messages = nil
		return
	}

	cut := len(s.messages) - n

	// 确保不会切断 tool-call 消息组
	// 如果 cut 位置后面有 tool 消息，需要找到它们的父 assistant 消息
	for cut < len(s.messages) && s.messages[cut].Role == llm.RoleTool {
		// 找到这个 tool 消息的父 assistant 消息
		callID := s.messages[cut].ToolCallID
		for j := cut - 1; j >= 0; j-- {
			if s.messages[j].Role == llm.RoleAssistant && len(s.messages[j].ToolCalls) > 0 {
				for _, tc := range s.messages[j].ToolCalls {
					if tc.ID == callID {
						// 找到父消息，cut 移到它前面
						cut = j
						break
					}
				}
				break
			}
		}
		break // 只处理第一个孤立的 tool 消息
	}

	// 如果 cut 位置是 assistant(with tool_calls)，需要把它和所有 tool 子消息一起删除
	if cut < len(s.messages) && s.messages[cut].Role == llm.RoleAssistant &&
		len(s.messages[cut].ToolCalls) > 0 {
		// 收集这个 assistant 的所有 tool_call_id
		callIDs := make(map[string]bool)
		for _, tc := range s.messages[cut].ToolCalls {
			callIDs[tc.ID] = true
		}
		cut++
		// 跳过所有属于这个 assistant 的 tool 消息
		for cut < len(s.messages) && s.messages[cut].Role == llm.RoleTool &&
			callIDs[s.messages[cut].ToolCallID] {
			cut++
		}
	}

	s.messages = s.messages[:cut]

	// 回退后检查 system message 是否还在，更新 systemSet 状态
	hasSystem := false
	for _, msg := range s.messages {
		if msg.Role == llm.RoleSystem {
			hasSystem = true
			break
		}
	}
	s.systemSet = hasSystem
}

// SetMessages 直接设置消息列表（用于 trace Restore 恢复压缩快照）
func (s *Session) SetMessages(msgs []llm.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = msgs
	s.compressCount = 0
	// 检查 system message 状态
	hasSystem := false
	for _, msg := range s.messages {
		if msg.Role == llm.RoleSystem {
			hasSystem = true
			break
		}
	}
	s.systemSet = hasSystem
}

// SetSummary 直接设置摘要文本（用于 trace Restore 恢复压缩快照）
func (s *Session) SetSummary(summary string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.summary = summary
}

// RemoveMessagesByID 按 ID 精确移除消息（用于 trace 回滚）
// 只移除 s.messages 中（可见窗口内）匹配的消息
func (s *Session) RemoveMessagesByID(ids []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(ids) == 0 {
		return
	}
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	var kept []llm.Message
	for _, msg := range s.messages {
		if msg.ID != "" && idSet[msg.ID] {
			continue // 跳过要移除的消息
		}
		kept = append(kept, msg)
	}
	s.messages = kept
	// 更新 systemSet 状态
	hasSystem := false
	for _, msg := range s.messages {
		if msg.Role == llm.RoleSystem {
			hasSystem = true
			break
		}
	}
	s.systemSet = hasSystem
}

// RestoreMessagesByID 恢复被压缩隐藏的消息（按 ID 插入到 compressCount 之前）
// 将 removedMsgs 中 ID 匹配的消息从 compressCount 之前的隐藏区域移到可见区域
func (s *Session) RestoreMessagesByID(ids []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(ids) == 0 || s.compressCount == 0 {
		return
	}
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	// 将隐藏区域中匹配的消息移到可见区域
	var hidden []llm.Message
	var restored []llm.Message
	for _, msg := range s.messages[:s.compressCount] {
		if msg.ID != "" && idSet[msg.ID] {
			restored = append(restored, msg)
		} else {
			hidden = append(hidden, msg)
		}
	}
	if len(restored) == 0 {
		return
	}
	// 重建 messages：隐藏区域(去掉恢复的) + 恢复的消息 + 可见区域
	result := make([]llm.Message, 0, len(hidden)+len(restored)+len(s.messages[s.compressCount:]))
	result = append(result, hidden...)
	result = append(result, restored...)
	result = append(result, s.messages[s.compressCount:]...)
	s.messages = result
	s.compressCount = len(hidden)
}

// SystemSet 返回 system message 是否已设置
func (s *Session) SystemSet() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.systemSet
}

// MarkSystemSet 标记 system message 已设置
func (s *Session) MarkSystemSet() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.systemSet = true
}

// Compress 手动触发消息压缩（使用 LLM 生成摘要）
// 压缩前保存完整消息快照到 trace，支持回退恢复
func (s *Session) Compress(ctx context.Context, trace *Trace) error {
	s.mu.Lock()
	compressor := s.compressor
	oldSummary := s.summary
	oldCount := s.compressCount
	// 保存完整消息快照（含已压缩的旧消息）
	snapshot := make([]llm.Message, len(s.messages))
	copy(snapshot, s.messages)
	s.mu.Unlock()

	if compressor == nil {
		return nil
	}

	// 对可见消息进行压缩
	visible := snapshot[oldCount:]
	newMsgs, newSummary, err := compressor.Compress(ctx, visible, oldSummary)
	if err != nil {
		return err
	}

	// 收集被压缩移除的消息 ID
	var removedIDs []string
	if len(newMsgs) < len(visible) {
		removedCount := len(visible) - len(newMsgs)
		for _, msg := range visible[:removedCount] {
			if msg.ID != "" {
				removedIDs = append(removedIDs, msg.ID)
			}
		}
	}

	// 更新：保留完整历史 + 压缩后的可见消息
	s.mu.Lock()
	// 保留旧消息（前 compressCount 条），追加压缩后的可见消息
	s.messages = append(snapshot[:oldCount], newMsgs...)
	s.summary = newSummary
	s.compressCount = oldCount // 旧消息偏移不变
	s.mu.Unlock()

	// 记录 CompressEvent（含完整快照和被移除的消息 ID）
	trace.AddCompressEvent(snapshot, oldSummary, len(newMsgs), removedIDs)
	return nil
}

// autoCompressInternal 自动压缩：消息数过多时隐藏旧消息组（不生成摘要，不丢弃消息）
// 调用方需持有 s.mu 读锁；内部按需升级为写锁
func (s *Session) autoCompressInternal(ctx context.Context) {
	visible := s.messages[s.compressCount:]
	if len(visible) <= 100 {
		return
	}

	// 按消息组切分，保留最近 10 个组
	groups := MessageGroups(visible)
	if len(groups) <= 10 {
		return
	}

	keepN := 10
	cutIdx := len(groups) - keepN
	// 确保不会切断 tool-call 序列
	cut := 0
	for i := 0; i < cutIdx; i++ {
		cut += groups[i].Len()
	}
	cut = findCleanCutPoint(visible, cut)
	if cut <= 0 {
		return
	}

	// 升级为写锁完成修改（只移动 compressCount，不丢弃消息）
	s.mu.RUnlock()
	s.mu.Lock()
	s.compressCount += cut
	s.mu.Unlock()
	s.mu.RLock()
}

// RestoreToEvent 回滚到指定事件（硬回溯：不可逆事件跳过文件回滚，消息仍回滚）
func (s *Session) RestoreToEvent(eventId string) (*RestoreResult, error) {
	return s.trace.Restore(eventId, s)
}

// CanRestore 检查是否可以回滚到指定事件
// 返回 (ok, willSkip, description)
func (s *Session) CanRestore(eventId string) (bool, bool, string) {
	return s.trace.CanRestore(eventId)
}

// Reset 清空消息和 trace（重新开始会话，保留 project 引用）
func (s *Session) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = nil
	s.systemSet = false
	s.summary = ""
	s.compressCount = 0
	s.trace.Clear()
}

// Metadata 返回元数据值
func (s *Session) Metadata(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.metadata[key]
}

// SetMetadata 设置元数据
func (s *Session) SetMetadata(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metadata[key] = value
}

// EstimateTokens 粗略估算当前模型可见上下文的 token 数
func (s *Session) EstimateTokens() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	msgs := s.messages[s.compressCount:]
	if s.summary != "" {
		msgs = append([]llm.Message{llm.SystemMessage(s.summary)}, msgs...)
	}
	return EstimateTokens(msgs)
}

// TruncateToolResult 手动截断 tool 结果（外部调用，用于特殊场景）
func (s *Session) TruncateToolResult(content string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = s.autoTruncate
	}
	if maxLen > 0 && len(content) > maxLen {
		return content[:maxLen] + "\n... [truncated]"
	}
	return content
}
