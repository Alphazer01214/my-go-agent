package memory

import (
	"context"
	"fmt"
	"sync"
)

// Agent 最小化 agent 接口，避免循环依赖（完整接口在 agent 包）
type Agent interface {
	Run(ctx context.Context, session *Session, prompt string) (string, error)
	Name() string
}

// Project 管理项目级别的资源：ProjectContext、Sessions
type Project struct {
	mu          sync.RWMutex
	name        string
	projectCtx  *ProjectContext
	sessions    map[string]*Session
	compressor  Compressor
}

// ProjectOpts 创建 Project 的选项
type ProjectOpts struct {
	Name        string
	ContextPath string   // ProjectContext 文件路径，如 ".memory/context.md"
	Compressor  Compressor
}

// NewProject 创建新的 Project
func NewProject(opts ProjectOpts) (*Project, error) {
	p := &Project{
		name:       opts.Name,
		sessions:   make(map[string]*Session),
		compressor: opts.Compressor,
	}

	if opts.ContextPath != "" {
		pc, err := NewProjectContext(opts.ContextPath)
		if err != nil {
			return nil, fmt.Errorf("[project] load context: %w", err)
		}
		p.projectCtx = pc
	}

	return p, nil
}

// Name 返回项目名称
func (p *Project) Name() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.name
}

// Context 返回项目长期记忆
func (p *Project) Context() *ProjectContext {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.projectCtx
}

// NewSession 创建新的 Session 并注册到 Project
func (p *Project) NewSession(agentName string) *Session {
	p.mu.Lock()
	defer p.mu.Unlock()

	s := NewSession(SessionOpts{
		AgentName:  agentName,
		Compressor: p.compressor,
		Project:    p,
	})
	p.sessions[s.ID()] = s
	return s
}

// GetSession 根据 ID 获取 Session
func (p *Project) GetSession(id string) *Session {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sessions[id]
}

// Sessions 返回所有 Session
func (p *Project) Sessions() []*Session {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]*Session, 0, len(p.sessions))
	for _, s := range p.sessions {
		result = append(result, s)
	}
	return result
}

// DeleteSession 删除 Session
func (p *Project) DeleteSession(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.sessions, id)
}

// Run 在指定 Session 中执行 agent
// 这是 Project 级别的入口，CLI 使用它来运行 agent
func (p *Project) Run(ctx context.Context, agent Agent, session *Session, prompt string) (string, error) {
	return agent.Run(ctx, session, prompt)
}
