// openai.go 实现OpenAI的ChatModel接口

package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type openaiChoice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"` // non stream
	Delta        Message `json:"delta"`   // stream
	FinishReason string  `json:"finish_reason"`
}

type openaiToolCallBuilder struct {
	ID       string
	Name     string
	Argument strings.Builder
}

type openaiUsage struct {
	InputToken  int `json:"input_token"`
	OutputToken int `json:"output_token"`
	TotalToken  int `json:"total_token"`
}
type openaiRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []Tool    `json:"tools,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

type openaiResponse struct {
	Choices []openaiChoice `json:"choices"`
	Usage   openaiUsage    `json:"usage"`
}

type OpenAIChatModel struct {
	baseUrl string
	apiKey  string
	model   string
	http    *http.Client
}

func NewOpenAIChatModel(baseUrl string, apiKey string, model string) *OpenAIChatModel {
	return &OpenAIChatModel{
		baseUrl: baseUrl,
		apiKey:  apiKey,
		model:   model,
		http:    &http.Client{Timeout: 66 * time.Second},
	}
}

func (m *OpenAIChatModel) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	reqBody, _ := json.Marshal(m.toOpenAIRequest(req))
	resp, err := m.doRequest(ctx, m.baseUrl, reqBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var res openaiResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	if len(res.Choices) == 0 {
		return nil, errors.New("no choices found")
	}
	choice := res.Choices[0]
	return &ChatResponse{
		Content: choice.Message.Content,
		Usage: Usage{
			InputTokens:  res.Usage.InputToken,
			OutputTokens: res.Usage.OutputToken,
			TotalTokens:  res.Usage.TotalToken,
		},
	}, nil
}

// StreamChat openai uses SSE
// data: {"choices": [{"delta": {"content": "hello"}}] }
func (m *OpenAIChatModel) StreamChat(ctx context.Context, req *ChatRequest, callback func(string)) (*ChatResponse, error) {
	oaiReq := m.toOpenAIRequest(req)
	oaiReq.Stream = true
	reqBody, _ := json.Marshal(oaiReq)
	resp, err := m.doRequest(ctx, m.baseUrl, reqBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	//var oresp openaiResponse   // final response
	var content strings.Builder
	var finRes string
	//var usage Usage
	toolCalls := map[int]*openaiToolCallBuilder{}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var chunk openaiResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) > 0 {
			finishReason := chunk.Choices[0].FinishReason
			if finishReason != "" {
				finRes = finishReason
				continue
			}
			delta := chunk.Choices[0].Delta
			text := delta.Content
			tcs := delta.ToolCalls
			for _, call := range tcs {
				builder := toolCalls[call.Index]
				if builder == nil {
					builder = &openaiToolCallBuilder{}
					toolCalls[call.Index] = builder
				}
				if call.ID != "" {
					builder.ID = call.ID
				}
				if call.Function.Name != "" {
					builder.Name = call.Function.Name
				}
				if call.Function.Arguments != "" {
					builder.Argument.WriteString(call.Function.Arguments)
				}
			}
			callback(text)
			content.WriteString(text)
		}
	}
	return &ChatResponse{
		Content:      content.String(),
		ToolCalls:    m.toToolCalls(toolCalls),
		FinishReason: finRes,
	}, nil
}

func (m *OpenAIChatModel) GetModel() string {
	return m.model
}

func (m *OpenAIChatModel) GetBaseUrl() string {
	return m.baseUrl
}

func (m *OpenAIChatModel) GetApiKey() string {
	return m.apiKey
}

func (m *OpenAIChatModel) SetModel(model string) {
	m.model = model
}

func (m *OpenAIChatModel) SetBaseUrl(baseUrl string) {

	m.baseUrl = baseUrl
}

func (m *OpenAIChatModel) SetApiKey(apiKey string) {
	m.apiKey = apiKey
}

func (m *OpenAIChatModel) toOpenAIRequest(req *ChatRequest) openaiRequest {
	model := req.Model
	if model == "" {
		model = m.model
	}
	return openaiRequest{
		Model:       model,
		Messages:    req.Messages,
		Tools:       req.Tools,
		Temperature: req.Options.Temperature,
		MaxTokens:   req.Options.MaxTokens,
	}
}

// doRequest openai发送POST请求
func (m *OpenAIChatModel) doRequest(ctx context.Context, baseUrl string, body []byte) (*http.Response, error) {
	var lastErr error
	// retry
	for i := 0; i < 6; i++ {
		// break
		if i > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(i - 1)):

			}
		}
		// baseurl+/chat/completions
		reqUrl, _ := url.JoinPath(baseUrl, "chat", "completions")
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqUrl, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		// build header
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+m.apiKey)

		resp, err := m.http.Do(req)
		if err != nil {
			// retry
			lastErr = fmt.Errorf("%s: %w", reqUrl, err)
			continue

		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		// error occurred
		errBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		lastErr = fmt.Errorf("[Open AI Request Error] %s: %s", reqUrl, string(errBody))
		if isRetryable(resp.StatusCode) {
			continue
		}
		return nil, lastErr

	}

	return nil, fmt.Errorf("[Open AI Request Error] max retry times reached, last error: %v", lastErr)
}

func (m *OpenAIChatModel) toToolCalls(builders map[int]*openaiToolCallBuilder) []ToolCall {
	res := make([]ToolCall, 0)
	for idx, call := range builders {
		res = append(res, ToolCall{
			ID:    call.ID,
			Index: idx,
			Type:  "function",
			Function: ToolCallFunction{
				Name:      call.Name,
				Arguments: call.Argument.String(),
			},
		})
	}
	return res
}
