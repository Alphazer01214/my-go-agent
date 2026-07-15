package middleware

import (
	"context"
	"fmt"
	"my-go-agent/internal/agent"
	"time"
)

// AgentMiddleware Agent 中间件类型
type AgentMiddleware func(agent.Agent) agent.Agent

// ==================== Log ====================

// AgentWithLog 带日志的 Agent 包装器
type AgentWithLog struct {
	agent   agent.Agent
	logFunc func(string)
}

// WithAgentLog 创建 Agent 日志中间件，logFunc 为 nil 时使用默认 fmt.Println
func WithAgentLog(logFunc func(string)) AgentMiddleware {
	if logFunc == nil {
		logFunc = func(msg string) { fmt.Println(msg) }
	}
	return func(a agent.Agent) agent.Agent {
		return &AgentWithLog{agent: a, logFunc: logFunc}
	}
}

func (a *AgentWithLog) Name() string {
	return a.agent.Name()
}

func (a *AgentWithLog) Steps() []agent.Step {
	return a.agent.Steps()
}

func (a *AgentWithLog) Run(ctx context.Context, prompt string) (string, error) {
	start := time.Now()
	a.logFunc(fmt.Sprintf("[%s] Run start", a.agent.Name()))

	result, err := a.agent.Run(ctx, prompt)
	duration := time.Since(start)
	steps := a.agent.Steps()

	if err != nil {
		a.logFunc(fmt.Sprintf("[%s] Run error | duration=%v steps=%d err=%v",
			a.agent.Name(), duration, len(steps), err))
		return "", err
	}
	a.logFunc(fmt.Sprintf("[%s] Run done | duration=%v steps=%d",
		a.agent.Name(), duration, len(steps)))
	return result, nil
}

// ==================== Trace ====================

// TraceEntry 追踪记录
type TraceEntry struct {
	Timestamp time.Time
	AgentName string
	Step      int
	Data      agent.Step
}

// TraceHandler 处理追踪事件
type TraceHandler func(entry TraceEntry)

// AgentWithTrace 带追踪的 Agent 包装器，忠实记录每一步的 thought/action/observation
type AgentWithTrace struct {
	agent   agent.Agent
	handler TraceHandler
}

// WithAgentTrace 创建 Agent 追踪中间件，记录每一步的执行详情
func WithAgentTrace(handler TraceHandler) AgentMiddleware {
	if handler == nil {
		handler = DefaultTraceHandler
	}
	return func(a agent.Agent) agent.Agent {
		return &AgentWithTrace{agent: a, handler: handler}
	}
}

func (a *AgentWithTrace) Name() string {
	return a.agent.Name()
}

func (a *AgentWithTrace) Steps() []agent.Step {
	return a.agent.Steps()
}

func (a *AgentWithTrace) Run(ctx context.Context, prompt string) (string, error) {
	// 包装 OnStep 回调，记录每一步
	type stepSetter interface {
		SetOnStep(agent.StepCallback)
	}
	if setter, ok := a.agent.(stepSetter); ok {
		setter.SetOnStep(func(step int, data agent.Step) {
			a.handler(TraceEntry{
				Timestamp: time.Now(),
				AgentName: a.agent.Name(),
				Step:      step,
				Data:      data,
			})
		})
	}

	return a.agent.Run(ctx, prompt)
}

// ComposeAgentMiddlewares 组合多个 Agent 中间件，从左到右执行
func ComposeAgentMiddlewares(middlewares ...AgentMiddleware) AgentMiddleware {
	return func(a agent.Agent) agent.Agent {
		for i := len(middlewares) - 1; i >= 0; i-- {
			a = middlewares[i](a)
		}
		return a
	}
}

// truncate 截断字符串到指定长度
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// DefaultTraceHandler 默认的追踪处理器
func DefaultTraceHandler(entry TraceEntry) {
	ts := entry.Timestamp.Format("15:04:05.000")
	step := entry.Data

	// 输出 thought
	if step.Thought != "" {
		fmt.Printf("[%s] [%s] step %d thought: %s\n", ts, entry.AgentName, entry.Step, step.Thought)
	}

	// 输出 action
	if step.Action != "" {
		fmt.Printf("[%s] [%s] step %d action: %s\n", ts, entry.AgentName, entry.Step, step.Action)
	}

	// 输出 observation
	if step.Observation != "" {
		fmt.Printf("[%s] [%s] step %d observation: %s\n", ts, entry.AgentName, entry.Step, truncate(step.Observation, 100))
	}
}
