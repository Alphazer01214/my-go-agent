package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const contextHeader = `# Project Context
> Auto-maintained by agent. Do not edit manually.
> Last updated: %s

`

type ProjectContext struct {
	mu      sync.RWMutex
	path    string
	content string
}

func NewProjectContext(path string) (*ProjectContext, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("[memory] mkdir: %w", err)
	}
	pc := &ProjectContext{path: path}
	if err := pc.load(); err != nil {
		return nil, err
	}
	return pc, nil
}

func (pc *ProjectContext) load() error {
	data, err := os.ReadFile(pc.path)
	if err != nil {
		if os.IsNotExist(err) {
			pc.content = ""
			return nil
		}
		return fmt.Errorf("[memory] load context: %w", err)
	}
	pc.content = string(data)
	return nil
}

func (pc *ProjectContext) Content() string {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.content
}

func (pc *ProjectContext) SetContent(content string) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	header := fmt.Sprintf(contextHeader, time.Now().Format("2006-01-02 15:04:05"))
	pc.content = header + strings.TrimSpace(content) + "\n"
	return pc.save()
}

func (pc *ProjectContext) AppendSection(section string) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.content += "\n" + strings.TrimSpace(section) + "\n"
	return pc.save()
}

func (pc *ProjectContext) IsEmpty() bool {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return strings.TrimSpace(pc.content) == ""
}

func (pc *ProjectContext) save() error {
	return os.WriteFile(pc.path, []byte(pc.content), 0644)
}
