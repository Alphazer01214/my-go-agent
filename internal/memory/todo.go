package memory

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// TodoStatus 待办事项状态
type TodoStatus string

const (
	TodoPending    TodoStatus = "pending"
	TodoInProgress TodoStatus = "in_progress"
	TodoCompleted  TodoStatus = "completed"
)

// TodoItem 单个待办事项
type TodoItem struct {
	ID          string     `json:"id"`
	Description string     `json:"description"`
	Status      TodoStatus `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// TodoList 待办事项列表，用于跟踪复杂任务的执行进度
type TodoList struct {
	mu    sync.Mutex
	items []TodoItem
	seq   int
}

// NewTodoList 创建新的待办列表
func NewTodoList() *TodoList {
	return &TodoList{}
}

// Add 添加待办事项，返回 ID
func (tl *TodoList) Add(description string) string {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.seq++
	id := fmt.Sprintf("t%d", tl.seq)
	tl.items = append(tl.items, TodoItem{
		ID:          id,
		Description: description,
		Status:      TodoPending,
		CreatedAt:   time.Now(),
	})
	return id
}

// Start 将待办事项标记为进行中
func (tl *TodoList) Start(id string) bool {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	for i := range tl.items {
		if tl.items[i].ID == id {
			tl.items[i].Status = TodoInProgress
			return true
		}
	}
	return false
}

// Complete 将待办事项标记为完成
func (tl *TodoList) Complete(id string) bool {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	now := time.Now()
	for i := range tl.items {
		if tl.items[i].ID == id {
			tl.items[i].Status = TodoCompleted
			tl.items[i].CompletedAt = &now
			return true
		}
	}
	return false
}

// Items 返回所有待办事项的副本
func (tl *TodoList) Items() []TodoItem {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	dst := make([]TodoItem, len(tl.items))
	copy(dst, tl.items)
	return dst
}

// Current 返回当前进行中的事项，没有则返回 nil
func (tl *TodoList) Current() *TodoItem {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	for i := range tl.items {
		if tl.items[i].Status == TodoInProgress {
			return &tl.items[i]
		}
	}
	return nil
}

// IsEmpty 返回列表是否为空
func (tl *TodoList) IsEmpty() bool {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	return len(tl.items) == 0
}

// Progress 返回 (已完成数, 总数)
func (tl *TodoList) Progress() (int, int) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	done := 0
	for _, item := range tl.items {
		if item.Status == TodoCompleted {
			done++
		}
	}
	return done, len(tl.items)
}

// Clear 清空待办列表
func (tl *TodoList) Clear() {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.items = nil
	tl.seq = 0
}

// Format 格式化待办列表为可读字符串
func (tl *TodoList) Format() string {
	items := tl.Items()
	if len(items) == 0 {
		return "(no todos)"
	}

	done, total := tl.Progress()
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Todos (%d/%d)\n", done, total))

	for _, item := range items {
		var icon string
		switch item.Status {
		case TodoPending:
			icon = "○"
		case TodoInProgress:
			icon = "◐"
		case TodoCompleted:
			icon = "●"
		}
		sb.WriteString(fmt.Sprintf("  %s [%s] %s\n", icon, item.ID, item.Description))
	}
	return sb.String()
}

// FormatProgress 返回简短的进度条
func (tl *TodoList) FormatProgress() string {
	items := tl.Items()
	if len(items) == 0 {
		return ""
	}

	var sb strings.Builder
	done, total := tl.Progress()
	sb.WriteString(fmt.Sprintf("[%d/%d] ", done, total))

	for _, item := range items {
		switch item.Status {
		case TodoPending:
			sb.WriteString("○")
		case TodoInProgress:
			sb.WriteString("◐")
		case TodoCompleted:
			sb.WriteString("●")
		}
	}
	return sb.String()
}
