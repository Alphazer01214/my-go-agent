package memory

import (
	"context"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/prompts"
	"sync"
)

type Memory interface {
	Add(ctx context.Context, msg llm.Message)
	Messages(ctx context.Context) []llm.Message
	Compress(ctx context.Context) error
	Clear()
}

type BaseMemory struct {
	mu        sync.RWMutex
	systemMsg *llm.Message
	messages  []llm.Message
	// toolCache Name + Arguments -> Result
	//toolCache map[string]string // 缓存 tool-call 结果
}

func (s *BaseMemory) Add(_ context.Context, msg llm.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if msg.Role == llm.RoleSystem && s.systemMsg == nil {
		s.systemMsg = &msg
		return
	}
	s.messages = append(s.messages, msg)
}

//
//func (s *BaseMemory) GetToolCallCache(name string, args string) (string, bool) {
//	s.mu.RLock()
//	defer s.mu.RUnlock()
//	result, ok := s.toolCache[name+args]
//	return result, ok
//}
//
//func (s *BaseMemory) SetToolCallCache(name string, args string, result string) {
//	s.mu.Lock()
//	defer s.mu.Unlock()
//	s.toolCache[name+args] = result
//}

// SlideWindow 滑动窗口记忆，保留最近 windowSize 条消息
type SlideWindow struct {
	BaseMemory
	windowSize int
}

func NewSlideWindow(windowSize int) *SlideWindow {
	return &SlideWindow{
		windowSize: windowSize,
	}
}

func (s *SlideWindow) Messages(_ context.Context) []llm.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	msgs := s.messages
	if len(msgs) > s.windowSize {
		start := len(msgs) - s.windowSize
		// 跳过窗口开头孤立的 tool 消息（其父 assistant 消息被截断）
		start = findCleanCutPoint(msgs, start)
		msgs = msgs[start:]
	}
	if s.systemMsg != nil {
		result := make([]llm.Message, 0, 1+len(msgs))
		result = append(result, *s.systemMsg)
		result = append(result, msgs...)
		return result
	}
	return msgs
}

func (s *SlideWindow) Compress(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.messages) <= s.windowSize {
		return nil
	}
	cut := len(s.messages) - s.windowSize
	cut = findCleanCutPoint(s.messages, cut)
	s.messages = s.messages[cut:]
	return nil
}

// findCleanCutPoint 从 cut 位置向后跳过孤立的 tool 消息，确保不会截断 tool-call 序列。
// 如果 cut 落在 tool 消息上，说明该 tool 的父 assistant 消息已被丢弃，需要跳过整个 tool 块。
func findCleanCutPoint(msgs []llm.Message, cut int) int {
	for cut < len(msgs) && msgs[cut].Role == llm.RoleTool {
		cut++
	}
	return cut
}

func (s *SlideWindow) Clear() {
	s.systemMsg = nil
	s.messages = nil
}

// SummaryMemory 摘要记忆：消息超过阈值时用 LLM 总结旧消息
type SummaryMemory struct {
	BaseMemory
	threshold       int
	summary         string
	summarizer      llm.ChatModel
	keepRecentTools int // 保留最近的工具结果数量
}

func NewSummaryMemory(summarizer llm.ChatModel, threshold int) *SummaryMemory {
	return &SummaryMemory{
		summarizer:      summarizer,
		threshold:       threshold,
		keepRecentTools: 6, // 默认保留最近 6 条工具结果
	}
}

func (s *SummaryMemory) Messages(_ context.Context) []llm.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	msgs := make([]llm.Message, 0, 2+len(s.messages))
	if s.systemMsg != nil {
		msgs = append(msgs, *s.systemMsg)
	}
	if s.summary != "" {
		msgs = append(msgs, llm.SystemMessage(s.summary))
	}
	msgs = append(msgs, s.messages...)
	return msgs
}

func (s *SummaryMemory) Compress(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.messages) <= s.threshold {
		return nil
	}

	// 找到需要保留的最近消息的起始位置
	// 保留最近的工具结果和对应的 assistant 消息
	keepStart := len(s.messages)
	toolCount := 0
	for i := len(s.messages) - 1; i >= 0; i-- {
		if s.messages[i].Role == llm.RoleTool {
			toolCount++
			if toolCount > s.keepRecentTools {
				keepStart = i + 1
				break
			}
		}
		// 如果遇到 assistant 消息且后面没有更多 tool，也保留
		if s.messages[i].Role == llm.RoleAssistant && i == len(s.messages)-1 {
			keepStart = i
		}
	}

	// 如果没有找到足够的工具消息，保留后半部分
	if keepStart >= len(s.messages) {
		keepStart = len(s.messages) / 2
	}

	// 确保 keepStart 不会截断 tool-call 序列
	keepStart = findCleanCutPoint(s.messages, keepStart)

	// 对较早的消息进行摘要
	oldMessages := s.messages[:keepStart]
	if len(oldMessages) > 0 {
		summary, err := s.summarize(ctx, oldMessages)
		if err != nil {
			return err
		}
		// 如果已有摘要，合并新旧摘要
		if s.summary != "" {
			s.summary = s.summary + "\n\n" + summary
		} else {
			s.summary = summary
		}
	}

	// 只保留最近的消息
	s.messages = s.messages[keepStart:]
	return nil
}

func (s *SummaryMemory) Clear() {
	s.systemMsg = nil
	s.messages = nil
	s.summary = ""
}

func (s *SummaryMemory) summarize(ctx context.Context, messages []llm.Message) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	req := &llm.ChatRequest{
		Messages: []llm.Message{
			llm.SystemMessage(prompts.SummaryMemorySystemPrompt),
			llm.UserMessage(llm.MessagesToString(messages)),
		},
	}
	resp, err := s.summarizer.Chat(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// Compressor 消息压缩器接口，用于 Session 级别的历史消息压缩
type Compressor interface {
	Compress(ctx context.Context, msgs []llm.Message, summary string) (newMsgs []llm.Message, newSummary string, err error)
}

// SummaryCompressor 使用 LLM 对旧消息进行摘要式压缩
type SummaryCompressor struct {
	summarizer llm.ChatModel
	threshold  int
}

func NewSummaryCompressor(summarizer llm.ChatModel, threshold int) *SummaryCompressor {
	return &SummaryCompressor{
		summarizer: summarizer,
		threshold:  threshold,
	}
}

// Compress 当消息数超过阈值时，对旧消息进行摘要压缩
// 返回压缩后的消息列表和累积的摘要文本
func (sc *SummaryCompressor) Compress(ctx context.Context, msgs []llm.Message, summary string) ([]llm.Message, string, error) {
	if len(msgs) <= sc.threshold {
		return msgs, summary, nil
	}

	// 按消息组切分，保留最近的组
	groups := MessageGroups(msgs)
	if len(groups) <= 2 {
		return msgs, summary, nil
	}

	// 保留最后 3 个组（最近的一轮对话），其余进行摘要
	keepGroups := 3
	if keepGroups > len(groups) {
		keepGroups = len(groups)
	}
	oldEnd := len(groups) - keepGroups
	if oldEnd <= 0 {
		return msgs, summary, nil
	}

	oldMsgs := groups[:oldEnd].AllMessages()
	recentMsgs := groups[oldEnd:].AllMessages()

	// 对旧消息调用 LLM 生成摘要
	newSummary, err := sc.summarizeOld(ctx, oldMsgs)
	if err != nil {
		return msgs, summary, err
	}

	// 合并摘要
	if summary != "" {
		summary = summary + "\n\n" + newSummary
	} else {
		summary = newSummary
	}

	return recentMsgs, summary, nil
}

// summarizeOld 使用 LLM 对旧消息生成摘要
func (sc *SummaryCompressor) summarizeOld(ctx context.Context, msgs []llm.Message) (string, error) {
	req := &llm.ChatRequest{
		Messages: []llm.Message{
			llm.SystemMessage(prompts.SummaryMemorySystemPrompt),
			llm.UserMessage(llm.MessagesToString(msgs)),
		},
	}
	resp, err := sc.summarizer.Chat(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// EstimateTokens 粗略估算消息列表的 token 数
// 中文约 1.5 字/token，英文约 4 字符/token，简单取 len/3 作为折中
func EstimateTokens(msgs []llm.Message) int {
	total := 0
	for _, msg := range msgs {
		// 每条消息固定开销（role、结构等）
		total += 2
		total += len(msg.Content) / 3
		for _, tc := range msg.ToolCalls {
			total += 10 // id、type、function.name 等固定字段
			total += len(tc.Function.Arguments) / 3
		}
	}
	return total
}

// MessageGroup 一条 user 消息及其引发的所有 assistant+tool 消息
// 用于按对话轮次管理消息，确保 tool-call 序列的完整性
type MessageGroup struct {
	UserMsg     *llm.Message  // 触发本轮的 user 消息（可能是 nil，如 system 消息开头）
	Messages    []llm.Message // 本轮所有消息（assistant + tool）
	HasToolCall bool          // 是否包含 tool call
}

// Len 返回该组包含的消息总数
func (g *MessageGroup) Len() int {
	n := len(g.Messages)
	if g.UserMsg != nil {
		n++
	}
	return n
}

// Groups 消息组切片类型
type Groups []MessageGroup

// AllMessages 将所有组展开为消息列表
func (gs Groups) AllMessages() []llm.Message {
	var result []llm.Message
	for _, g := range gs {
		if g.UserMsg != nil {
			result = append(result, *g.UserMsg)
		}
		result = append(result, g.Messages...)
	}
	return result
}

// MessageGroups 将消息按对话轮次分组
// 每个 user 消息开始一个新组，后续的 assistant+tool 消息属于同一组
func MessageGroups(msgs []llm.Message) Groups {
	var groups Groups
	var current *MessageGroup

	for _, msg := range msgs {
		switch msg.Role {
		case llm.RoleSystem:
			// system 消息作为独立组（不合并到后续组）
			groups = append(groups, MessageGroup{
				Messages: []llm.Message{msg},
			})
		case llm.RoleUser:
			// user 消息开始新组
			if current != nil {
				groups = append(groups, *current)
			}
			current = &MessageGroup{
				UserMsg:  &msg,
				Messages: []llm.Message{},
			}
		case llm.RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				if current == nil {
					current = &MessageGroup{Messages: []llm.Message{}}
				}
				current.Messages = append(current.Messages, msg)
				current.HasToolCall = true
			} else {
				if current == nil {
					current = &MessageGroup{Messages: []llm.Message{}}
				}
				current.Messages = append(current.Messages, msg)
				// 无 tool call 的 assistant 消息结束当前组
				groups = append(groups, *current)
				current = nil
			}
		case llm.RoleTool:
			if current == nil {
				current = &MessageGroup{Messages: []llm.Message{}}
			}
			current.Messages = append(current.Messages, msg)
		}
	}
	if current != nil {
		groups = append(groups, *current)
	}
	return groups
}
