package agent

import (
	"context"
	"fmt"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/memory"
	"my-go-agent/internal/prompts"
	"my-go-agent/internal/tool"
	"my-go-agent/internal/utils"
)

type Plan struct {
	Goal  string `json:"goal"`
	Todos []Todo `json:"todos"`
}

type Todo struct {
	Index       int    `json:"index"`
	Description string `json:"description"`
	Expert      string `json:"expert,omitempty"`
	IsDone      bool   `json:"is_done,omitempty"`
	IsSkipped   bool   `json:"is_skipped,omitempty"`
	Result      string `json:"result,omitempty"`
}

// PlanAgent 分析需求、制定计划、将子任务委派给合适的 Expert 或 ToolCallingAgent
type PlanAgent struct {
	*BaseAgent
	experts    []*Expert
	planPrompt string
	plan       *Plan
}

func NewPlanAgent(model llm.ChatModel, tools *tool.Registry, experts ...*Expert) *PlanAgent {
	base := NewBaseAgent("plan_agent", model, tools)
	base.SysPrompt = prompts.PlanAgentSystemPrompt
	return &PlanAgent{
		BaseAgent:  base,
		experts:    experts,
		planPrompt: prompts.PlanAgentSystemPrompt,
	}
}

func (a *PlanAgent) Plan() *Plan { return a.plan }

func (a *PlanAgent) Run(ctx context.Context, prompt string) (string, error) {
	a.steps = nil
	a.plan = nil

	plan, err := a.createPlan(ctx, prompt)
	if err != nil {
		return "", err
	}
	a.plan = plan

	if a.OnStep != nil {
		a.OnStep(0, Step{Thought: fmt.Sprintf("Plan: %s (%d tasks)", plan.Goal, len(plan.Todos))})
	}

	result := fmt.Sprintf("Goal: %s\n", plan.Goal)
	for i := range plan.Todos {
		todo := &plan.Todos[i]

		if a.OnStep != nil {
			a.OnStep(i+1, Step{Thought: fmt.Sprintf("Task %d/%d: %s", i+1, len(plan.Todos), todo.Description)})
		}

		executor := a.resolveExecutor(todo)
		subPrompt := fmt.Sprintf("Overall goal: %s\nYour task: %s\nComplete this task.", plan.Goal, todo.Description)

		resp, err := executor.Run(ctx, subPrompt)
		if err != nil {
			todo.IsSkipped = true
			todo.Result = err.Error()
			result += fmt.Sprintf("\n[Skipped] %s: %s", todo.Description, err)
			continue
		}

		todo.IsDone = true
		todo.Result = resp
		result += fmt.Sprintf("\n[Done] %s:\n%s", todo.Description, resp)
	}

	return result, nil
}

// resolveExecutor 根据 todo.Expert 字段选择执行者
func (a *PlanAgent) resolveExecutor(todo *Todo) Agent {
	if todo.Expert != "" {
		if expert := FindExpert(a.experts, todo.Expert); expert != nil {
			return expert.Agent
		}
	}
	// fallback: 普通 ToolCallingAgent
	executor := NewToolCallingAgent(a.Model, a.Tools)
	executor.SetSysPrompt(a.SysPrompt)
	executor.SetMaxStep(a.MaxStep)
	executor.SetOnStep(a.OnStep)
	executor.SetMemory(memory.NewSlideWindow(a.MaxStep))
	return executor
}

func (a *PlanAgent) createPlan(ctx context.Context, userPrompt string) (*Plan, error) {
	prompt := a.planPrompt
	if len(a.experts) > 0 {
		prompt += "\n\n## Available Experts\n"
		prompt += "For each task, specify which expert should handle it (use the expert's name).\n"
		prompt += "If no expert matches, leave expert empty.\n\n"
		for _, e := range a.experts {
			prompt += fmt.Sprintf("- **%s**: %s\n", e.Name, e.Description)
		}
		prompt += "\nThe JSON todos should include an optional \"expert\" field with the expert's name."
	}

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			llm.SystemMessage(prompt),
			llm.UserMessage(userPrompt),
		},
		Options: llm.ChatOptions{
			ResponseFormat: llm.ResponseFormat{Type: "json_object"},
		},
	}
	resp, err := a.Model.Chat(ctx, req)
	if err != nil {
		return nil, err
	}
	var plan Plan
	if err := utils.ParseJSON(resp.Content, &plan); err != nil {
		return nil, fmt.Errorf("[plan_agent] parse plan JSON: %w (raw: %s)", err, resp.Content)
	}
	if len(plan.Todos) == 0 {
		return nil, fmt.Errorf("[plan_agent] plan has no todos (raw: %s)", resp.Content)
	}
	return &plan, nil
}
