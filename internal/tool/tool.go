package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/sandbox"
	"sync"
)

type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]interface{}
	// Call return string, unmarshal to struct{} if need
	Call(ctx context.Context, args string) (string, error)
}

type Registry struct {
	mu      sync.RWMutex
	tools   map[string]Tool // tool name -> tool
	cache   map[string]string
	sandbox *sandbox.Sandbox
}

// SetSandbox 设置沙箱（用于拦截高危操作）
func (r *Registry) SetSandbox(s *sandbox.Sandbox) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sandbox = s
}

// ResetSandbox 重置沙箱批量放行状态（每次 Run() 开始时调用）
func (r *Registry) ResetSandbox() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.sandbox != nil {
		r.sandbox.Reset()
	}
}

// 缓存白名单：只缓存无副作用的只读工具
var cacheableTools = map[string]bool{
	"read_file":   true,
	"list_dir":    true,
	"glob":        true,
	"search_file": true,
}

// normalizeArgs 归一化 JSON 参数，确保相同的参数生成相同的缓存键
func normalizeArgs(args string) string {
	var parsed interface{}
	if err := json.Unmarshal([]byte(args), &parsed); err != nil {
		// JSON 解析失败，返回原始字符串
		return args
	}
	// 重新序列化为标准化的 JSON
	normalized, err := json.Marshal(parsed)
	if err != nil {
		return args
	}
	return string(normalized)
}

func NewRegistry(tools ...Tool) *Registry {
	ts := make(map[string]Tool)
	for _, t := range tools {
		ts[t.Name()] = t
	}
	return &Registry{
		tools: ts,
		cache: make(map[string]string),
	}
}

func (r *Registry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

func (r *Registry) Get(name string) (Tool, error) {
	tool, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("[tool] not found: %s", name)
	}
	return tool, nil
}

func (r *Registry) Execute(ctx context.Context, name string, args string) string {
	// 沙箱检查
	if r.sandbox != nil {
		allowed, reason := r.sandbox.Check(name, args)
		if !allowed {
			return fmt.Sprintf("[SANDBOX] %s", reason)
		}
	}

	// 写操作清除缓存（数据可能变化）
	if name == "write_file" || name == "edit_file" || name == "exec_command" {
		r.mu.Lock()
		r.cache = make(map[string]string)
		r.mu.Unlock()
	}

	// 只读工具走缓存
	if cacheableTools[name] {
		normalizedArgs := normalizeArgs(args)
		key := name + "|" + normalizedArgs

		r.mu.RLock()
		cached, ok := r.cache[key]
		r.mu.RUnlock()
		if ok {
			return cached
		}

		tool, err := r.Get(name)
		if err != nil {
			return fmt.Sprintf("[ERROR] %s", err.Error())
		}
		res, err := tool.Call(ctx, args)
		if err != nil {
			return fmt.Sprintf("[ERROR] %s", err.Error())
		}

		r.mu.Lock()
		r.cache[key] = res
		r.mu.Unlock()
		return res
	}

	tool, err := r.Get(name)
	if err != nil {
		return fmt.Sprintf("[ERROR] %s", err.Error())
	}
	res, err := tool.Call(ctx, args)
	if err != nil {
		return fmt.Sprintf("[ERROR] %s", err.Error())
	}
	return res
}

// ToLLMTool convert tool registry to llm tool
func (r *Registry) ToLLMTool() []llm.Tool {
	tools := make([]llm.Tool, 0)
	for name, tool := range r.tools {
		tools = append(tools, llm.Tool{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        name,
				Description: tool.Description(),
				Parameters:  tool.Parameters(),
			},
		})
	}
	return tools
}

func DefaultRegistry() *Registry {
	return NewRegistry(
		NewListDirTool(),
		NewReadFileTool(1024),
		NewExecCommandTool(),
		NewWriteFileTool(),
		NewEditFileTool(),
		NewMakeDirTool(),
		NewSearchFileTool(),
		NewGlobTool(),
	)
}
