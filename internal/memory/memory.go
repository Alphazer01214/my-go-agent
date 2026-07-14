package memory

import (
	"context"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/prompts"
)

type Memory interface {
	Add(ctx context.Context, msg llm.Message)
	Messages(ctx context.Context) []llm.Message
	Compress(ctx context.Context) error
	Clear()
}

// SlideWindow 滑动窗口记忆，保留最近 windowSize 条消息
type SlideWindow struct {
	systemMsg  *llm.Message
	messages   []llm.Message
	windowSize int
}

func NewSlideWindow(windowSize int) *SlideWindow {
	return &SlideWindow{
		windowSize: windowSize,
	}
}

func (s *SlideWindow) Add(_ context.Context, msg llm.Message) {
	if msg.Role == llm.RoleSystem && s.systemMsg == nil {
		s.systemMsg = &msg
		return
	}
	s.messages = append(s.messages, msg)
}

func (s *SlideWindow) Messages(_ context.Context) []llm.Message {
	msgs := s.messages
	if len(msgs) > s.windowSize {
		msgs = msgs[len(msgs)-s.windowSize:]
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
	if len(s.messages) <= s.windowSize {
		return nil
	}
	cut := len(s.messages) - s.windowSize
	for cut < len(s.messages) && s.messages[cut].Role == llm.RoleTool {
		cut++
	}
	s.messages = s.messages[cut:]
	return nil
}

func (s *SlideWindow) Clear() {
	s.systemMsg = nil
	s.messages = nil
}

// SummaryMemory 摘要记忆：消息超过阈值时用 LLM 总结旧消息
type SummaryMemory struct {
	systemMsg  *llm.Message
	messages   []llm.Message
	threshold  int
	summary    string
	summarizer llm.ChatModel
}

func NewSummaryMemory(summarizer llm.ChatModel, threshold int) *SummaryMemory {
	return &SummaryMemory{
		summarizer: summarizer,
		threshold:  threshold,
	}
}

func (s *SummaryMemory) Add(_ context.Context, msg llm.Message) {
	if msg.Role == llm.RoleSystem && s.systemMsg == nil {
		s.systemMsg = &msg
		return
	}
	s.messages = append(s.messages, msg)
}

func (s *SummaryMemory) Messages(_ context.Context) []llm.Message {
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
	if len(s.messages) <= s.threshold {
		return nil
	}
	summary, err := s.summarize(ctx)
	if err != nil {
		return err
	}
	s.summary = summary
	s.messages = nil
	return nil
}

func (s *SummaryMemory) Clear() {
	s.systemMsg = nil
	s.messages = nil
	s.summary = ""
}

func (s *SummaryMemory) summarize(ctx context.Context) (string, error) {
	req := &llm.ChatRequest{
		Messages: []llm.Message{
			llm.SystemMessage(prompts.SummaryMemorySystemPrompt),
			llm.UserMessage(llm.MessagesToString(s.messages)),
		},
	}
	resp, err := s.summarizer.Chat(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}
