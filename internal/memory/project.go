package memory

import (
	"fmt"
	"my-go-agent/internal/utils"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const contextHeader = `# Project Document
> Auto-maintained by agent. Do not edit manually.
> Last updated: %s

`

// ProjectOpts Project 创建选项
type ProjectOpts struct {
	Name        string      // 项目名称
	Path        string      // 项目工作路径（绝对路径），为空时使用当前目录
	ContextPath string      // 长期记忆文件路径（相对于 memPath），默认 "project.md"
	Compressor  Compressor  // 消息压缩器（可选，注入到 Session）
}

// Project 管理项目级别的资源：ProjectContext、Sessions
type Project struct {
	mu         sync.RWMutex
	name       string
	path       string // 绝对路径，后续执行都是基于这个路径的相对路径，如果跑出路径要用户确认
	memPath    string // 项目记忆路径，默认 path/.memory
	sessions   map[string]*Session
	compressor Compressor // 消息压缩器，注入到新创建的 Session

	contextFile string // 长期记忆文件名，默认 "project.md"
	pjmd        string // project.md content
}

// NewProject 创建新的 Project
func NewProject(opts ProjectOpts) (*Project, error) {
	workDir := opts.Path
	if workDir == "" {
		workDir = utils.CurrentDir()
	}

	p := &Project{
		name:        opts.Name,
		sessions:    make(map[string]*Session),
		path:        workDir,
		memPath:     filepath.Join(workDir, ".memory"),
		compressor:  opts.Compressor,
		contextFile: opts.ContextPath,
	}
	if p.contextFile == "" {
		p.contextFile = "project.md"
	}
	if err := p.initMemory(); err != nil {
		return nil, fmt.Errorf("[project] init memory: %w", err)
	}

	return p, nil
}

func (p *Project) initMemory() error {
	// 创建 .memory 目录（已存在则忽略）
	if err := os.MkdirAll(p.memPath, 0755); err != nil {
		return err
	}

	contextPath := filepath.Join(p.memPath, p.contextFile)

	// 如果文件已存在，加载内容；否则创建新文件
	if data, err := os.ReadFile(contextPath); err == nil {
		p.pjmd = string(data)
	} else {
		header := fmt.Sprintf(contextHeader, time.Now().Format("2006-01-02 15:04:05"))
		if err := os.WriteFile(contextPath, []byte(header), 0644); err != nil {
			return err
		}
		p.pjmd = header
	}

	return nil
}

// Name 返回项目名称
func (p *Project) Name() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.name
}

// Path 返回项目工作路径
func (p *Project) Path() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.path
}

// GetPjmd 返回项目长期记忆内容
func (p *Project) GetPjmd() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.pjmd
}

// UpdatePjmd 更新项目长期记忆并持久化到文件
func (p *Project) UpdatePjmd(content string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pjmd = content
	return os.WriteFile(filepath.Join(p.memPath, p.contextFile), []byte(content), 0644)
}

// NewSession 创建新的 Session 并注册到 Project
func (p *Project) NewSession(agentName string) *Session {
	p.mu.Lock()
	defer p.mu.Unlock()

	s := NewSession(p, agentName)
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
