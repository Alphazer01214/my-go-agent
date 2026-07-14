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

// AgentTraceEvent 追踪事件类型
type AgentTraceEvent string

const (
	AgentTraceStart AgentTraceEvent = "start"
	AgentTraceStep  AgentTraceEvent = "step"
	AgentTraceEnd   AgentTraceEvent = "end"
	AgentTraceError AgentTraceEvent = "error"
)

// AgentTraceEntry Agent 追踪记录
type AgentTraceEntry struct {
	Timestamp time.Time
	Event     AgentTraceEvent
	AgentName string
	Step      int
	TotalSteps int
	Data      agent.Step
	Duration  time.Duration
	Error     error
}

// AgentTraceHandler 处理 Agent 追踪事件
type AgentTraceHandler func(entry AgentTraceEntry)

// AgentWithTrace 带追踪的 Agent 包装器
type AgentWithTrace struct {
	agent   agent.Agent
	handler AgentTraceHandler
}

// WithAgentTrace 创建 Agent 追踪中间件，handler 为 nil 时使用默认处理器
func WithAgentTrace(handler AgentTraceHandler) AgentMiddleware {
	if handler == nil {
		handler = DefaultAgentTraceHandler
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
	start := time.Now()

	// 追踪开始
	a.handler(AgentTraceEntry{
		Timestamp: start,
		Event:     AgentTraceStart,
		AgentName: a.agent.Name(),
		Data:      agent.Step{Thought: prompt},
	})

	// 包装 OnStep 回调（如果 agent 支持）
	type stepSetter interface {
		SetOnStep(agent.StepCallback)
	}
	if setter, ok := a.agent.(stepSetter); ok {
		setter.SetOnStep(func(step int, data agent.Step) {
			a.handler(AgentTraceEntry{
				Timestamp: time.Now(),
				Event:     AgentTraceStep,
				AgentName: a.agent.Name(),
				Step:      step,
				Data:      data,
			})
		})
	}

	// 执行 agent
	result, err := a.agent.Run(ctx, prompt)
	duration := time.Since(start)
	steps := a.agent.Steps()

	// 追踪结束
	if err != nil {
		a.handler(AgentTraceEntry{
			Timestamp:  time.Now(),
			Event:      AgentTraceError,
			AgentName:  a.agent.Name(),
			TotalSteps: len(steps),
			Duration:   duration,
			Error:      err,
		})
	} else {
		a.handler(AgentTraceEntry{
			Timestamp:  time.Now(),
			Event:      AgentTraceEnd,
			AgentName:  a.agent.Name(),
			TotalSteps: len(steps),
			Duration:   duration,
		})
	}

	return result, err
}

// DefaultAgentTraceHandler 默认的 Agent 追踪处理器
func DefaultAgentTraceHandler(entry AgentTraceEntry) {
	ts := entry.Timestamp.Format("15:04:05.000")

	switch entry.Event {
	case AgentTraceStart:
		fmt.Printf("[%s] ▶ [%s] start\n", ts, entry.AgentName)
	case AgentTraceStep:
		step := entry.Data
		action := step.Action
		if action == "" {
			action = "(thinking)"
		}
		fmt.Printf("[%s] → [%s] step %d | %s\n", ts, entry.AgentName, entry.Step, action)
	case AgentTraceEnd:
		fmt.Printf("[%s] ✓ [%s] done | duration=%v steps=%d\n",
			ts, entry.AgentName, entry.Duration, entry.TotalSteps)
	case AgentTraceError:
		fmt.Printf("[%s] ✗ [%s] error | duration=%v steps=%d err=%v\n",
			ts, entry.AgentName, entry.Duration, entry.TotalSteps, entry.Error)
	}
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
