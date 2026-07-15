package memory

import (
	"context"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/prompts"
	"sync"
)

// Compressor 消息压缩策略接口
type Compressor interface {
	// Compress 对消息列表进行压缩，返回压缩后的消息列表
	Compress(ctx context.Context, messages []llm.Message) ([]llm.Message, error)
}

// WindowCompressor 滑动窗口压缩：保留最近 N 条消息
type WindowCompressor struct {
	windowSize int
}

func NewWindowCompressor(windowSize int) *WindowCompressor {
	return &WindowCompressor{windowSize: windowSize}
}

func (c *WindowCompressor) Compress(_ context.Context, messages []llm.Message) ([]llm.Message, error) {
	if len(messages) <= c.windowSize {
		return messages, nil
	}
	start := len(messages) - c.windowSize
	start = findCleanCutPoint(messages, start)
	return messages[start:], nil
}

// SummaryCompressor 摘要压缩：超出阈值时用 LLM 总结旧消息
type SummaryCompressor struct {
	mu             sync.Mutex
	threshold      int
	summarizer     llm.ChatModel
	summary        string // 累积摘要
	keepRecentTools int   // 保留最近的工具结果数量
}

func NewSummaryCompressor(summarizer llm.ChatModel, threshold int) *SummaryCompressor {
	return &SummaryCompressor{
		summarizer:      summarizer,
		threshold:       threshold,
		keepRecentTools: 6,
	}
}

// Summary 返回当前累积摘要
func (c *SummaryCompressor) Summary() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.summary
}

// SetSummary 设置摘要（用于从外部恢复状态）
func (c *SummaryCompressor) SetSummary(s string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.summary = s
}

func (c *SummaryCompressor) Compress(ctx context.Context, messages []llm.Message) ([]llm.Message, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(messages) <= c.threshold {
		return messages, nil
	}

	// 找到需要保留的最近消息的起始位置
	keepStart := len(messages)
	toolCount := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.RoleTool {
			toolCount++
			if toolCount > c.keepRecentTools {
				keepStart = i + 1
				break
			}
		}
		if messages[i].Role == llm.RoleAssistant && i == len(messages)-1 {
			keepStart = i
		}
	}

	if keepStart >= len(messages) {
		keepStart = len(messages) / 2
	}

	keepStart = findCleanCutPoint(messages, keepStart)

	// 对较早的消息进行摘要
	oldMessages := messages[:keepStart]
	if len(oldMessages) > 0 {
		summary, err := c.summarize(ctx, oldMessages)
		if err != nil {
			return nil, err
		}
		if c.summary != "" {
			c.summary = c.summary + "\n\n" + summary
		} else {
			c.summary = summary
		}
	}

	// 返回：摘要消息 + 最近消息
	result := make([]llm.Message, 0, 1+len(messages)-keepStart)
	if c.summary != "" {
		result = append(result, llm.SystemMessage(c.summary))
	}
	result = append(result, messages[keepStart:]...)
	return result, nil
}

func (c *SummaryCompressor) summarize(ctx context.Context, messages []llm.Message) (string, error) {
	req := &llm.ChatRequest{
		Messages: []llm.Message{
			llm.SystemMessage(prompts.SummaryMemorySystemPrompt),
			llm.UserMessage(llm.MessagesToString(messages)),
		},
	}
	resp, err := c.summarizer.Chat(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}
