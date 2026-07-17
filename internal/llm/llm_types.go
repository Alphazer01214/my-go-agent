package llm

import (
	"fmt"
	"my-go-agent/internal/utils"
	"strings"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type ModelEffort string

const (
	ModelEffortLow    ModelEffort = "low"
	ModelEffortMedium ModelEffort = "medium"
	ModelEffortHigh   ModelEffort = "high"
)

type ChatModelOption struct {
	Model   string
	ApiKey  string
	BaseUrl string
	Effort  ModelEffort
}

type Message struct {
	ID       string `json:"id,omitempty"`        // 消息唯一 ID（用户消息生成，assistant/tool 继承）
	ParentID string `json:"parent_id,omitempty"` // 指向所属用户消息的 ID

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

// ToolFunction openai 格式
type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ToolCall openai 格式
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
		ID:      utils.GenId("msg"),
		Role:    RoleSystem,
		Content: systemPrompt,
	}
}

func UserMessage(userPrompt string) Message {
	return Message{
		ID:      utils.GenId("msg"),
		Role:    RoleUser,
		Content: userPrompt,
	}
}

func AssistantMessage(modelResponse string) Message {
	return Message{
		ID:      utils.GenId("msg"),
		Role:    RoleAssistant,
		Content: modelResponse,
	}
}

func AssistantMessageWithToolCall(modelResponse string, toolCalls []ToolCall) Message {
	return Message{
		ID:        utils.GenId("msg"),
		Role:      RoleAssistant,
		ToolCalls: toolCalls,
		Content:   modelResponse,
	}
}

func ToolMessage(callId string, callResult string) Message {
	return Message{
		ID:         utils.GenId("msg"),
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
