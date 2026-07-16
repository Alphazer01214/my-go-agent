package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/memory"
	"my-go-agent/internal/tool"
)

// WithSession 将 Session 注入 context（代理到 memory 包）
func WithSession(ctx context.Context, s *memory.Session) context.Context {
	return memory.WithSession(ctx, s)
}

// SessionFromContext 从 context 中获取 Session（代理到 memory 包）
func SessionFromContext(ctx context.Context) *memory.Session {
	return memory.SessionFromContext(ctx)
}

// Expert 包装一个 Agent，提供名称和描述用于路由
type Expert struct {
	Name        string
	Description string
	Agent       Agent
}

// ExpertAsTool 将 Expert 包装为 tool.Tool，使其可被 ToolCallingAgent 调用
type ExpertAsTool struct {
	expert    *Expert
	mode      Mode          // 子 agent 运行模式
	sharedCtx *SharedContext // 传递给子 agent 的共享上下文
}

func NewExpertAsTool(expert *Expert) *ExpertAsTool {
	return &ExpertAsTool{expert: expert}
}

func NewExpertAsToolWithContext(expert *Expert, mode Mode, sharedCtx *SharedContext) *ExpertAsTool {
	return &ExpertAsTool{
		expert:    expert,
		mode:      mode,
		sharedCtx: sharedCtx,
	}
}

func (e *ExpertAsTool) Name() string        { return e.expert.Name }
func (e *ExpertAsTool) Description() string { return e.expert.Description }

func (e *ExpertAsTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task": map[string]interface{}{
				"type":        "string",
				"description": "task description for this expert",
			},
		},
		"required": []string{"task"},
	}
}

func (e *ExpertAsTool) Call(ctx context.Context, args string) (string, error) {
	var param struct {
		Task string `json:"task"`
	}
	if err := json.Unmarshal([]byte(args), &param); err != nil {
		return "", fmt.Errorf("[expert] invalid input: %w", err)
	}

	session := SessionFromContext(ctx)
	if session == nil {
		return "", fmt.Errorf("[expert] no session in context")
	}

	child := e.createChildAgent()
	raw, err := child.Run(ctx, session, param.Task)
	if err != nil {
		return "", err
	}

	if e.mode == ModeLightweight {
		return ExtractAnswer(raw), nil
	}
	return formatStructuredToolResult(raw), nil
}

// createChildAgent 基于 expert 的 agent 创建子 agent
func (e *ExpertAsTool) createChildAgent() *ToolCallingAgent {
	base := e.expert.Agent.Base()
	child := NewToolCallingAgent(base.Model, base.Tools)
	child.SysPrompt = base.SysPrompt
	child.MaxStep = base.MaxStep
	child.Mode = e.mode
	child.SharedCtx = e.sharedCtx
	return child
}

// FindExpert 在列表中按名称查找 Expert
func FindExpert(experts []*Expert, name string) *Expert {
	for _, e := range experts {
		if e.Name == name {
			return e
		}
	}
	return nil
}

// ExpertManager 管理多个专家，根据任务描述路由到合适的专家处理
type ExpertManager struct {
	*ToolCallingAgent
}

func NewExpertManager(model llm.ChatModel, experts []*Expert) *ExpertManager {
	tools := tool.NewRegistry()
	for _, expert := range experts {
		tools.Register(NewExpertAsTool(expert))
	}
	return &ExpertManager{
		ToolCallingAgent: NewToolCallingAgent(model, tools),
	}
}
