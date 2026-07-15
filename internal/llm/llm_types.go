package llm

import (
	"fmt"
	"strings"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role      Role   `json:"role"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp,omitempty"`

	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ChatOptions struct {
	Temperature    float64        `json:"temperature"`
	MaxTokens      int            `json:"max_tokens"`
	ResponseFormat ResponseFormat `json:"response_format,omitempty"`
}

type ResponseFormat struct {
	Type string `json:"type"` // json object
}
type ChatRequest struct {
	Model    string
	Messages []Message
	Tools    []Tool
	Options  ChatOptions
}

type ChatResponse struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
	Usage        Usage
}

// Usage 用于统计LLM用量等
type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

type Tool struct {
	Type     string       `json:"type"` // function
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}
type ToolCall struct {
	ID       string           `json:"id"`
	Index    int              `json:"index"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction 传入的工具函数参数和参数，并作为 memory 的 cache 实现
type ToolCallFunction struct {
	Name string `json:"name"`
	//Arguments map[string]interface{} `json:"arguments"`
	Arguments string `json:"arguments"`
}

func SystemMessage(systemPrompt string) Message {
	return Message{
		Role:    RoleSystem,
		Content: systemPrompt,
	}
}

func UserMessage(userPrompt string) Message {
	return Message{
		Role:    RoleUser,
		Content: userPrompt,
	}
}

func AssistantMessage(modelResponse string) Message {
	return Message{
		Role:    RoleAssistant,
		Content: modelResponse,
	}
}

func AssistantMessageWithToolCall(modelResponse string, toolCalls []ToolCall) Message {
	return Message{
		Role:      RoleAssistant,
		ToolCalls: toolCalls,
		Content:   modelResponse,
	}
}

func ToolMessage(callId string, callResult string) Message {
	return Message{
		Role:       RoleTool,
		Content:    callResult,
		ToolCallID: callId,
	}
}

func MessagesToString(messages []Message) string {
	var sb strings.Builder
	for _, msg := range messages {
		if msg.Role == RoleTool {
			continue
		}
		// [assistant] xxx
		// [user] xxx
		sb.WriteString(fmt.Sprintf("[%v] %v\n", msg.Role, msg.Content))
	}
	return sb.String()
}
