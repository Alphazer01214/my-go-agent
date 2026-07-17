package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"my-go-agent/internal/config"
	"net/http"
	"net/url"
	"strings"
)

type WebSearch struct {
	apiKey  string
	baseURL string
}

type WebSearchArgs struct {
	Query string `json:"query"`
	Count int    `json:"count"` // 结果数量，默认3
}

func NewWebSearchTool(apiKey, baseURL string) *WebSearch {
	return &WebSearch{
		apiKey:  apiKey,
		baseURL: baseURL,
	}
}

func (w *WebSearch) Name() string {
	return "web_search"
}

func (w *WebSearch) Description() string {
	return "Search the web for information. Returns titles, URLs, and snippets."
}

func (w *WebSearch) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query",
			},
			"count": map[string]interface{}{
				"type":        "integer",
				"description": "Number of results (default 3, max 10)",
			},
		},
		"required": []string{"query"},
	}
}

func (w *WebSearch) Call(ctx context.Context, args string) (string, error) {
	var arg WebSearchArgs
	if err := json.Unmarshal([]byte(args), &arg); err != nil {
		return "", err
	}
	if arg.Query == "" {
		return "", fmt.Errorf("[web_search] query is empty")
	}
	if arg.Count <= 0 {
		arg.Count = config.ToolWebSearchResults
	}
	if arg.Count > 10 {
		arg.Count = 10
	}

	// 使用 Serper API 或类似服务
	apiURL := fmt.Sprintf("%s/search?q=%s&num=%d", w.baseURL, url.QueryEscape(arg.Query), arg.Count)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("[web_search] create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+w.apiKey)

	client := &http.Client{Timeout: config.ToolWebSearchTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("[web_search] request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("[web_search] read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("[web_search] API error: %s", string(body))
	}

	// 解析并格式化结果
	var result struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Snippet string `json:"snippet"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return string(body), nil // 返回原始响应
	}

	var sb strings.Builder
	for i, r := range result.Results {
		sb.WriteString(fmt.Sprintf("%d. %s\n   %s\n   %s\n", i+1, r.Title, r.URL, r.Snippet))
	}
	if sb.Len() == 0 {
		return "No results found", nil
	}
	return sb.String(), nil
}
