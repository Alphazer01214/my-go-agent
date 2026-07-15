package agent

import (
	"context"
	"my-go-agent/internal/config"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/memory"
	"my-go-agent/internal/prompts"
	"my-go-agent/internal/tool"
)

// Step 记录 agent 单步执行的 thought/action/observation
type Step struct {
	Thought     string
	Action      string
	Observation string
}

// StepCallback 每步执行后的回调，用于展示中间过程
type StepCallback func(step int, data Step)

// Agent 通用 agent 接口
type Agent interface {
	Run(ctx context.Context, prompt string) (string, error)
	Name() string
	Steps() []Step
}

// BaseAgent 包含 agent 的通用配置和工具方法，不实现 Run
type BaseAgent struct {
	name       string
	Model      llm.ChatModel
	Tools      *tool.Registry
	Memory     memory.Memory
	ProjectCtx *memory.ProjectContext
	SysPrompt  string
	OnStep     StepCallback
	MaxStep    int
	steps      []Step
}

func NewBaseAgent(name string, model llm.ChatModel, tools *tool.Registry) *BaseAgent {
	return &BaseAgent{
		name:      name,
		Model:     model,
		Tools:     tools,
		Memory:    memory.NewSlideWindow(config.MemorySize),
		SysPrompt: prompts.DefaultSystemPrompt,
		MaxStep:   config.MaxSteps,
	}
}

func (a *BaseAgent) Name() string    { return a.name }
func (a *BaseAgent) Steps() []Step   { return a.steps }
func (a *BaseAgent) Plan() *Plan     { return nil }

func (a *BaseAgent) SetSysPrompt(prompt string) {
	a.SysPrompt = prompt
}

func (a *BaseAgent) AppendSysPrompt(extra string) {
	a.SysPrompt += "\n" + extra
}

func (a *BaseAgent) SetMemory(mem memory.Memory) {
	a.Memory = mem
}

func (a *BaseAgent) SetMaxStep(n int) {
	a.MaxStep = n
}

func (a *BaseAgent) SetOnStep(cb StepCallback) {
	a.OnStep = cb
}

func (a *BaseAgent) SetProjectContext(ctx *memory.ProjectContext) {
	a.ProjectCtx = ctx
	a.Tools.Register(tool.NewUpdateMemoryTool(ctx))
}

func (a *BaseAgent) AddTools(tools ...tool.Tool) {
	for _, t := range tools {
		a.Tools.Register(t)
	}
}

func (a *BaseAgent) BuildSysPrompt() string {
	p := a.SysPrompt
	if a.ProjectCtx != nil && !a.ProjectCtx.IsEmpty() {
		p += "\n\n## Project Context (from long-term memory)\n" + a.ProjectCtx.Content()
	}
	return p
}
