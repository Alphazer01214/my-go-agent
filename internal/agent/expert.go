package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/tool"
)

// Expert 包装一个 Agent，提供名称和描述用于路由
type Expert struct {
	Name        string
	Description string
	Agent       Agent
}

// ExpertAsTool 将 Expert 包装为 tool.Tool，使其可被 ToolCallingAgent 调用
type ExpertAsTool struct {
	expert *Expert
}

func NewExpertAsTool(expert *Expert) *ExpertAsTool {
	return &ExpertAsTool{expert: expert}
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
	return e.expert.Agent.Run(ctx, param.Task)
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
