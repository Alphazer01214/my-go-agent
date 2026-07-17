package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/memory"
	"my-go-agent/internal/tool"
	"my-go-agent/internal/utils"
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
	mode      Mode           // 子 agent 运行模式
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

	child := e.createChildAgent(utils.GenId("subagent"))
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
func (e *ExpertAsTool) createChildAgent(name string) *ToolCallingAgent {
	base := e.expert.Agent.Base()
	child := NewToolCallingAgent(name, base.model, base.tools)
	child.sysPrompt = base.sysPrompt
	child.maxStep = base.maxStep
	child.mode = e.mode
	child.sharedCtx = e.sharedCtx
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
		ToolCallingAgent: NewToolCallingAgent(utils.GenId("agent"), model, tools),
	}
}

// ==================== SubagentTool ====================

// SubagentTool 让主 Agent 动态创建子 Agent 处理子任务
// 子 Agent 继承父 Agent 的 model 和 tools，maxStep 限制为 10
// 适用于需要独立上下文的调查、分析任务
type SubagentTool struct {
	parent *BaseAgent
}

func NewSubagentTool(parent *BaseAgent) *SubagentTool {
	return &SubagentTool{parent: parent}
}

func (s *SubagentTool) Name() string { return "spawn_subagent" }
func (s *SubagentTool) Description() string {
	return `创建一个专注的子 Agent 处理子任务。子 Agent 继承父 Agent 的所有工具，maxStep=10。
适用场景：
- 需要独立上下文的调查任务（避免污染主对话）
- 可并行执行的独立子任务
- 需要多次工具调用但不需要修改文件的分析任务
注意：子 Agent 仅用于读取和分析，不要让它修改文件。`
}

func (s *SubagentTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task": map[string]interface{}{
				"type":        "string",
				"description": "子 Agent 的任务描述（清晰、具体，包含预期输出格式）",
			},
		},
		"required": []string{"task"},
	}
}

func (s *SubagentTool) Call(ctx context.Context, args string) (string, error) {
	var param struct {
		Task string `json:"task"`
	}
	if err := json.Unmarshal([]byte(args), &param); err != nil {
		return "", fmt.Errorf("[spawn_subagent] invalid input: %w", err)
	}

	session := SessionFromContext(ctx)
	if session == nil {
		return "", fmt.Errorf("[spawn_subagent] no session in context")
	}

	// 创建子 Agent（继承父 Agent 的 model 和 tools，maxStep=10）
	child := s.parent.NewSubagent(utils.GenId("subagent"), 10)
	raw, err := child.Run(ctx, session, param.Task)
	if err != nil {
		return "", fmt.Errorf("[spawn_subagent] error: %w", err)
	}

	return formatStructuredToolResult(raw), nil
}
