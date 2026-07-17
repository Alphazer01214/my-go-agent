package agent

import (
	"context"
	"my-go-agent/internal/config"
	"my-go-agent/internal/llm"
	"my-go-agent/internal/memory"
	"my-go-agent/internal/prompts"
	"my-go-agent/internal/tool"
)

// Agent 通用 agent 接口
type Agent interface {
	Run(ctx context.Context, session *memory.Session, prompt string) (string, error)
	Name() string
	Base() *BaseAgent // 返回基础 agent 配置，用于子 agent 创建时继承设置
}

type BaseAgent struct {
	name      string
	model     llm.ChatModel
	tools     *tool.Registry
	sysPrompt string
	maxStep   int
	// mode 决定模型 token 的多少，用于本地模型的优化，也许不能放在 agent，后面再设计
	mode      Mode
	sharedCtx *SharedContext // 父 agent 传入的共享上下文
}

func NewBaseAgent(name string, model llm.ChatModel, tools *tool.Registry) *BaseAgent {
	return &BaseAgent{
		name:      name,
		model:     model,
		tools:     tools,
		sysPrompt: prompts.DefaultSystemPrompt,
		maxStep:   config.MaxSteps,
	}
}

func (a *BaseAgent) Name() string     { return a.name }
func (a *BaseAgent) Base() *BaseAgent { return a }
func (a *BaseAgent) Plan() *Plan      { return nil }

func (a *BaseAgent) SetSysPrompt(prompt string) {
	a.sysPrompt = prompt
}

// AppendSysPrompt 追加系统提示
func (a *BaseAgent) AppendSysPrompt(extra string) {
	a.sysPrompt += "\n" + extra
}

func (a *BaseAgent) SetMaxStep(n int) {
	a.maxStep = n
}

func (a *BaseAgent) SetMode(m Mode) {
	a.mode = m
}

func (a *BaseAgent) SetSharedContext(ctx *SharedContext) {
	a.sharedCtx = ctx
}

func (a *BaseAgent) AddTools(tools ...tool.Tool) {
	for _, t := range tools {
		a.tools.Register(t)
	}
}

// NewSubagent 创建一个子 Agent，继承父 Agent 的 model、tools、sysPrompt
func (a *BaseAgent) NewSubagent(name string, maxStep int) *ToolCallingAgent {
	child := NewToolCallingAgent(name, a.model, a.tools)
	child.sysPrompt = a.sysPrompt
	child.maxStep = maxStep
	child.sharedCtx = a.sharedCtx
	return child
}

// BuildSysPrompt 从 session 获取 ProjectContext，构建完整的系统提示
func (a *BaseAgent) BuildSysPrompt(session *memory.Session) string {
	p := a.sysPrompt
	// 注入项目长期记忆
	if session != nil {
		if proj := session.Project(); proj != nil {
			if pjmd := proj.GetPjmd(); pjmd != "" {
				p += "\n\n## Project Context (from long-term memory)\n" + pjmd
			}
		}
	}
	// 注入共享上下文（父 agent → 子 agent）
	if a.sharedCtx != nil {
		if extra := a.sharedCtx.Format(); extra != "" {
			p += "\n\n" + extra
		}
	}
	// 注入子 agent 模式指令
	if a.mode == ModeLightweight {
		p += "\n\n" + SubagentLightweightPrompt
	} else if a.sharedCtx != nil {
		// 有共享上下文时自动启用结构化回传
		p += "\n\n" + SubagentStructuredPrompt
	}
	return p
}
