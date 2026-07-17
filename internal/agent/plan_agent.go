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
	base.sysPrompt = prompts.PlanAgentSystemPrompt
	return &PlanAgent{
		BaseAgent:  base,
		experts:    experts,
		planPrompt: prompts.PlanAgentSystemPrompt,
	}
}

func (a *PlanAgent) Plan() *Plan { return a.plan }

func (a *PlanAgent) Run(ctx context.Context, session *memory.Session, prompt string) (string, error) {
	a.plan = nil
	// trace := session.Trace()

	plan, err := a.createPlan(ctx, prompt)
	if err != nil {
		return "", err
	}
	a.plan = plan

	// trace: 记录计划创建检查点
	// trace.RecordCheckpoint(0, fmt.Sprintf("Plan: %s (%d tasks)", plan.Goal, len(plan.Todos)))

	shared := &SharedContext{
		Background: fmt.Sprintf("Overall goal: %s", plan.Goal),
	}

	result := fmt.Sprintf("Goal: %s\n", plan.Goal)
	for i := range plan.Todos {
		todo := &plan.Todos[i]

		// trace: 记录任务开始检查点
		// trace.RecordCheckpoint(i+1, fmt.Sprintf("Task %d/%d: %s", i+1, len(plan.Todos), todo.Description))

		executor := a.resolveExecutor(todo, shared)
		subPrompt := fmt.Sprintf("Your task: %s\nComplete this task.", todo.Description)

		resp, err := executor.Run(ctx, session, subPrompt)
		if err != nil {
			todo.IsSkipped = true
			todo.Result = err.Error()
			result += fmt.Sprintf("\n[Skipped] %s: %s", todo.Description, err)
			continue
		}

		todo.IsDone = true
		todo.Result = resp
		result += fmt.Sprintf("\n[Done] %s:\n%s", todo.Description, resp)

		shared.Findings = append(shared.Findings,
			fmt.Sprintf("Task '%s' completed: %s", todo.Description, summarizeResult(resp)))
	}

	return result, nil
}

// resolveExecutor 根据 todo.Expert 字段选择执行者，并注入共享上下文
func (a *PlanAgent) resolveExecutor(todo *Todo, shared *SharedContext) Agent {
	if todo.Expert != "" {
		if expert := FindExpert(a.experts, todo.Expert); expert != nil {
			child := a.createChildFromExpert(expert, shared)
			return child
		}
	}
	executor := NewToolCallingAgent("plan_executor", a.model, a.tools)
	executor.SetSysPrompt(a.sysPrompt)
	executor.SetMaxStep(a.maxStep)
	executor.SetSharedContext(shared)
	return executor
}

// createChildFromExpert 基于 expert 创建子 agent，注入共享上下文
func (a *PlanAgent) createChildFromExpert(expert *Expert, shared *SharedContext) Agent {
	base := expert.Agent.Base()
	child := NewToolCallingAgent("expert_child", base.model, base.tools)
	child.sysPrompt = base.sysPrompt
	child.maxStep = base.maxStep
	child.SetSharedContext(shared)
	return child
}

// summarizeResult 从子 agent 结果中提取简短摘要
func summarizeResult(resp string) string {
	answer := ExtractAnswer(resp)
	const maxLen = 200
	runes := []rune(answer)
	if len(runes) > maxLen {
		return string(runes[:maxLen]) + "..."
	}
	return answer
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
	resp, err := a.model.Chat(ctx, req)
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
