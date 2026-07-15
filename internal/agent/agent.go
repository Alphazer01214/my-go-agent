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

// BaseAgent 包含 agent 的通用配置，不持有会话状态
type BaseAgent struct {
	name      string
	Model     llm.ChatModel
	Tools     *tool.Registry
	SysPrompt string
	MaxStep   int
	Mode      Mode          // 运行模式：默认结构化 / 轻量
	SharedCtx *SharedContext // 父 agent 传入的共享上下文
}

func NewBaseAgent(name string, model llm.ChatModel, tools *tool.Registry) *BaseAgent {
	return &BaseAgent{
		name:      name,
		Model:     model,
		Tools:     tools,
		SysPrompt: prompts.DefaultSystemPrompt,
		MaxStep:   config.MaxSteps,
	}
}

func (a *BaseAgent) Name() string     { return a.name }
func (a *BaseAgent) Base() *BaseAgent { return a }
func (a *BaseAgent) Plan() *Plan      { return nil }

func (a *BaseAgent) SetSysPrompt(prompt string) {
	a.SysPrompt = prompt
}

func (a *BaseAgent) AppendSysPrompt(extra string) {
	a.SysPrompt += "\n" + extra
}

func (a *BaseAgent) SetMaxStep(n int) {
	a.MaxStep = n
}

func (a *BaseAgent) SetMode(m Mode) {
	a.Mode = m
}

func (a *BaseAgent) SetSharedContext(ctx *SharedContext) {
	a.SharedCtx = ctx
}

func (a *BaseAgent) AddTools(tools ...tool.Tool) {
	for _, t := range tools {
		a.Tools.Register(t)
	}
}

// BuildSysPrompt 从 session 获取 ProjectContext，构建完整的系统提示
func (a *BaseAgent) BuildSysPrompt(session *memory.Session) string {
	p := a.SysPrompt
	// 注入项目长期记忆
	if session != nil {
		if pc := session.ProjectContext(); pc != nil && !pc.IsEmpty() {
			p += "\n\n## Project Context (from long-term memory)\n" + pc.Content()
		}
	}
	// 注入共享上下文（父 agent → 子 agent）
	if a.SharedCtx != nil {
		if extra := a.SharedCtx.Format(); extra != "" {
			p += "\n\n" + extra
		}
	}
	// 注入子 agent 模式指令
	if a.Mode == ModeLightweight {
		p += "\n\n" + SubagentLightweightPrompt
	} else if a.SharedCtx != nil {
		// 有共享上下文时自动启用结构化回传
		p += "\n\n" + SubagentStructuredPrompt
	}
	return p
}
