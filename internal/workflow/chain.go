package workflow

import (
	"context"
	"fmt"
	"my-go-agent/internal/agent"
	"time"
)

// ---------------------------------------------------------------------------
// ChainEventType：链式执行事件类型
// ---------------------------------------------------------------------------

// ChainEventType 表示链式执行中的事件类型
type ChainEventType int

const (
	ChainStepStarted   ChainEventType = iota // 步骤开始执行
	ChainStepCompleted                       // 步骤执行成功
	ChainStepFailed                          // 步骤执行失败
	ChainStepRetrying                        // 步骤正在重试
)

// String 返回事件类型的可读字符串
func (e ChainEventType) String() string {
	switch e {
	case ChainStepStarted:
		return "step_started"
	case ChainStepCompleted:
		return "step_completed"
	case ChainStepFailed:
		return "step_failed"
	case ChainStepRetrying:
		return "step_retrying"
	default:
		return "unknown"
	}
}

// ChainEvent 携带链式执行过程中的事件信息
type ChainEvent struct {
	Type      ChainEventType
	StepName  string
	StepIndex int
	Output    string
	Err       error
	RetryNum  int
	Timestamp time.Time
}

// ChainEventListener 链式执行事件监听器函数类型
type ChainEventListener func(event ChainEvent)

// ---------------------------------------------------------------------------
// ChainStep：链中的单个执行步骤
// ---------------------------------------------------------------------------

// ChainStep 表示链中的一个执行步骤，封装一个 agent 及其配置
type ChainStep struct {
	Agent        agent.Agent
	Name         string
	Description  string
	maxRetries   int
	timeout      time.Duration
	inputBuilder func(prevOutput string) string
}

// ChainStepOption 用于配置 ChainStep 的函数式选项
type ChainStepOption func(*ChainStep)

// WithStepRetry 设置步骤执行失败时的最大重试次数（默认 0）。
// 重试间隔采用指数退避：500ms, 1s, 2s, 4s ...
func WithStepRetry(maxRetries int) ChainStepOption {
	return func(s *ChainStep) {
		s.maxRetries = maxRetries
	}
}

// WithStepTimeout 设置步骤的单次执行超时时间。
// 超时后将取消执行并视为失败（可触发重试）。
func WithStepTimeout(timeout time.Duration) ChainStepOption {
	return func(s *ChainStep) {
		s.timeout = timeout
	}
}

// WithStepInputBuilder 设置步骤的输入构建函数。
// 该函数接收上一个步骤的输出，返回该步骤的输入 prompt。
// 默认行为是将上一个步骤的输出直接作为输入。
func WithStepInputBuilder(builder func(prevOutput string) string) ChainStepOption {
	return func(s *ChainStep) {
		s.inputBuilder = builder
	}
}

// WithStepDescription 设置步骤描述
func WithStepDescription(desc string) ChainStepOption {
	return func(s *ChainStep) {
		s.Description = desc
	}
}

// ---------------------------------------------------------------------------
// Chain：顺序执行引擎
// ---------------------------------------------------------------------------

// Chain 是一个顺序执行的 agent 编排器。
//
// Chain 将多个 agent 按顺序串联执行，每个 agent 的输出作为下一个 agent 的输入。
// 适用于以下场景：
//   - 需要按特定顺序执行多个任务
//   - 每个步骤依赖前一步的结果
//   - 流水线式的数据处理流程
//
// 增强特性：
//   - 步骤级别：重试、超时、自定义输入构建
//   - 事件监听：全局事件监听器，跟踪每个步骤的执行状态
//   - 容错控制：步骤失败时自动重试（可配置）
type Chain struct {
	steps     []*ChainStep
	listeners []ChainEventListener
}

// NewChain 创建一个新的空 Chain
func NewChain() *Chain {
	return &Chain{
		steps: make([]*ChainStep, 0),
	}
}

// OnEvent 注册全局事件监听器，所有步骤的事件都会触发此监听器
func (c *Chain) OnEvent(listener ChainEventListener) {
	c.listeners = append(c.listeners, listener)
}

// fireEvent 触发事件，通知所有注册的监听器
func (c *Chain) fireEvent(event ChainEvent) {
	for _, listener := range c.listeners {
		listener(event)
	}
}

// AddStep 向链末尾添加一个执行步骤。
// 支持传入任意数量的 ChainStepOption 进行配置。
func (c *Chain) AddStep(agent agent.Agent, name string, description string, opts ...ChainStepOption) *Chain {
	step := &ChainStep{
		Agent:       agent,
		Name:        name,
		Description: description,
	}
	for _, opt := range opts {
		opt(step)
	}
	c.steps = append(c.steps, step)
	return c
}

// AddSteps 批量添加多个步骤到链末尾
func (c *Chain) AddSteps(steps ...*ChainStep) *Chain {
	c.steps = append(c.steps, steps...)
	return c
}

// StepCount 返回链中的步骤数量
func (c *Chain) StepCount() int {
	return len(c.steps)
}

// Steps 返回链中的所有步骤副本
func (c *Chain) Steps() []*ChainStep {
	result := make([]*ChainStep, len(c.steps))
	copy(result, c.steps)
	return result
}

// StepAt 返回指定索引的步骤。如果索引越界，返回 nil。
func (c *Chain) StepAt(index int) *ChainStep {
	if index < 0 || index >= len(c.steps) {
		return nil
	}
	return c.steps[index]
}

// RemoveStepAt 移除指定索引的步骤。如果索引越界，返回错误。
func (c *Chain) RemoveStepAt(index int) error {
	if index < 0 || index >= len(c.steps) {
		return fmt.Errorf("step index %d out of range (0..%d)", index, len(c.steps)-1)
	}
	c.steps = append(c.steps[:index], c.steps[index+1:]...)
	return nil
}

// Clear 清空链中的所有步骤和状态
func (c *Chain) Clear() {
	c.steps = make([]*ChainStep, 0)
}

// ---------------------------------------------------------------------------
// Run：核心执行引擎
// ---------------------------------------------------------------------------

// Run 顺序执行链中的所有步骤。
//
// 执行策略：
//   - 严格按步骤添加顺序执行
//   - 每个步骤的输出默认作为下一个步骤的输入
//   - 支持步骤级别重试（指数退避）
//   - 支持步骤级别超时
//   - 支持自定义输入构建函数
//
// 参数:
//   - ctx: 上下文，用于取消或超时控制
//   - input: 初始输入，传递给第一个步骤
//
// 返回:
//   - string: 最后一个步骤的输出
//   - error: 如果任意步骤执行失败（且重试耗尽）返回错误
func (c *Chain) Run(ctx context.Context, input string) (string, error) {
	if len(c.steps) == 0 {
		return "", fmt.Errorf("chain has no steps")
	}

	currentInput := input

	for i, step := range c.steps {
		// 检查 context 是否已取消
		if ctx.Err() != nil {
			return "", fmt.Errorf("chain execution cancelled at step %q (index %d): %w",
				step.Name, i, ctx.Err())
		}

		// 触发步骤开始事件
		c.fireEvent(ChainEvent{
			Type:      ChainStepStarted,
			StepName:  step.Name,
			StepIndex: i,
			Timestamp: time.Now(),
		})

		var lastErr error
		var lastOutput string
		maxAttempts := 1 + step.maxRetries

		for attempt := 0; attempt < maxAttempts; attempt++ {
			// 检查 context 是否已取消
			if ctx.Err() != nil {
				return "", fmt.Errorf("chain execution cancelled at step %q (index %d): %w",
					step.Name, i, ctx.Err())
			}

			if attempt > 0 {
				// 指数退避：500ms, 1s, 2s, 4s ...
				backoff := time.Duration(500*(1<<(attempt-1))) * time.Millisecond
				select {
				case <-ctx.Done():
					return "", ctx.Err()
				case <-time.After(backoff):
				}

				// 触发重试事件
				c.fireEvent(ChainEvent{
					Type:      ChainStepRetrying,
					StepName:  step.Name,
					StepIndex: i,
					RetryNum:  attempt,
					Timestamp: time.Now(),
				})
			}

			// 构建步骤输入
			stepInput := currentInput
			if step.inputBuilder != nil && i > 0 {
				stepInput = step.inputBuilder(currentInput)
			}

			// 如有超时，创建子 context
			execCtx := ctx
			var execCancel context.CancelFunc
			if step.timeout > 0 {
				execCtx, execCancel = context.WithTimeout(ctx, step.timeout)
			}

			output, err := step.Agent.Run(execCtx, stepInput)

			if execCancel != nil {
				execCancel()
			}

			if err == nil {
				lastOutput = output
				currentInput = output

				// 触发步骤完成事件
				c.fireEvent(ChainEvent{
					Type:      ChainStepCompleted,
					StepName:  step.Name,
					StepIndex: i,
					Output:    output,
					Timestamp: time.Now(),
				})
				break
			}

			lastErr = err
		}

		// 如果所有重试都失败，触发失败事件并返回错误
		if lastErr != nil {
			c.fireEvent(ChainEvent{
				Type:      ChainStepFailed,
				StepName:  step.Name,
				StepIndex: i,
				Err:       lastErr,
				Timestamp: time.Now(),
			})
			return "", fmt.Errorf("chain step %q (index %d) failed after %d attempt(s): %w",
				step.Name, i, maxAttempts, lastErr)
		}
	}

	return currentInput, nil
}

// Reset 重置链的状态（清空步骤列表和事件监听器）
func (c *Chain) Reset() {
	c.steps = make([]*ChainStep, 0)
	c.listeners = make([]ChainEventListener, 0)
}
