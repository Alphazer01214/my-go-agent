package tool

import (
	"context"
	"fmt"
	"my-go-agent/internal/llm"
)

type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]interface{}
	// Call return string, unmarshal to struct{} if need
	Call(ctx context.Context, args string) (string, error)
}

type Registry struct {
	tools map[string]Tool // tool name -> tool
}

func NewRegistry(tools ...Tool) *Registry {
	ts := make(map[string]Tool)
	for _, t := range tools {
		ts[t.Name()] = t
	}
	return &Registry{
		tools: ts,
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
	tool, err := r.Get(name)
	if err != nil {
		return err.Error()
	}
	res, err := tool.Call(ctx, args)
	if err != nil {
		return err.Error()
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
	return NewRegistry(NewListDirTool(), NewReadFileTool(1024))
}
