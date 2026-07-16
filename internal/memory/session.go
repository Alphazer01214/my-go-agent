package memory

import (
	"context"
	"fmt"
	"my-go-agent/internal/llm"
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
	mu         sync.RWMutex
	id         string
	agentName  string
	createdAt  time.Time
	messages   []llm.Message
	systemSet  bool      // system message 是否已添加
	trace      *Trace
	todoList   *TodoList
	compressor Compressor
	project    *Project
	metadata   map[string]string
}

// SessionOpts 创建 Session 的选项
type SessionOpts struct {
	AgentName  string
	Compressor Compressor
	Project    *Project
}

// NewSession 创建新的 Session
func NewSession(opts SessionOpts) *Session {
	agentName := opts.AgentName
	if agentName == "" {
		agentName = "default"
	}
	return &Session{
		id:         fmt.Sprintf("session_%d", time.Now().UnixMilli()),
		agentName:  agentName,
		createdAt:  time.Now(),
		trace:      NewTrace(agentName),
		todoList:   NewTodoList(),
		compressor: opts.Compressor,
		project:    opts.Project,
		metadata:   make(map[string]string),
	}
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

// ProjectContext 返回项目的长期记忆（便捷方法）
func (s *Session) ProjectContext() *ProjectContext {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.project == nil {
		return nil
	}
	return s.project.Context()
}

// Trace 返回执行轨迹
func (s *Session) Trace() *Trace { return s.trace }

// Todo 返回待办列表
func (s *Session) Todo() *TodoList { return s.todoList }

// AddMessage 向 session 添加一条消息
func (s *Session) AddMessage(msg llm.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, msg)
}

// Messages 返回压缩后的消息列表（供 LLM 调用使用）
// 如果设置了 Compressor，会对消息进行压缩
func (s *Session) Messages(ctx context.Context) []llm.Message {
	s.mu.RLock()
	msgs := make([]llm.Message, len(s.messages))
	copy(msgs, s.messages)
	s.mu.RUnlock()

	if s.compressor == nil {
		return msgs
	}
	compressed, err := s.compressor.Compress(ctx, msgs)
	if err != nil {
		// 压缩失败时返回原始消息
		return msgs
	}
	return compressed
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

// Compress 手动触发消息压缩
func (s *Session) Compress(ctx context.Context) error {
	if s.compressor == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	compressed, err := s.compressor.Compress(ctx, s.messages)
	if err != nil {
		return err
	}
	s.messages = compressed
	return nil
}

// Reset 清空消息和 trace（重新开始会话，保留 project 引用）
func (s *Session) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = nil
	s.systemSet = false
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
