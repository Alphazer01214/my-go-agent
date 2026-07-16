package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"my-go-agent/internal/memory"
)

// ==================== CreateTodo ====================

type CreateTodo struct{}

func NewCreateTodoTool() *CreateTodo { return &CreateTodo{} }

func (t *CreateTodo) Name() string        { return "create_todo" }
func (t *CreateTodo) Description() string { return "Create a todo item to track task progress. Use this to break down complex tasks into steps." }
func (t *CreateTodo) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"description": map[string]interface{}{
				"type":        "string",
				"description": "Description of the todo item",
			},
		},
		"required": []string{"description"},
	}
}

func (t *CreateTodo) Call(ctx context.Context, args string) (string, error) {
	var arg struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(args), &arg); err != nil {
		return "", err
	}
	if arg.Description == "" {
		return "", fmt.Errorf("[create_todo] description is empty")
	}

	session := memory.SessionFromContext(ctx)
	if session == nil {
		return "", fmt.Errorf("[create_todo] no session in context")
	}

	id := session.Todo().Add(arg.Description)
	return fmt.Sprintf(`{"id": "%s", "description": "%s", "status": "pending"}`, id, arg.Description), nil
}

// ==================== StartTodo ====================

type StartTodo struct{}

func NewStartTodoTool() *StartTodo { return &StartTodo{} }

func (t *StartTodo) Name() string        { return "start_todo" }
func (t *StartTodo) Description() string { return "Mark a todo item as in_progress. Call this before starting work on a todo." }
func (t *StartTodo) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "ID of the todo item to start",
			},
		},
		"required": []string{"id"},
	}
}

func (t *StartTodo) Call(ctx context.Context, args string) (string, error) {
	var arg struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(args), &arg); err != nil {
		return "", err
	}
	if arg.ID == "" {
		return "", fmt.Errorf("[start_todo] id is empty")
	}

	session := memory.SessionFromContext(ctx)
	if session == nil {
		return "", fmt.Errorf("[start_todo] no session in context")
	}

	if !session.Todo().Start(arg.ID) {
		return "", fmt.Errorf("[start_todo] todo %s not found", arg.ID)
	}
	return fmt.Sprintf(`{"id": "%s", "status": "in_progress"}`, arg.ID), nil
}

// ==================== CompleteTodo ====================

type CompleteTodo struct{}

func NewCompleteTodoTool() *CompleteTodo { return &CompleteTodo{} }

func (t *CompleteTodo) Name() string        { return "complete_todo" }
func (t *CompleteTodo) Description() string { return "Mark a todo item as completed. Call this after finishing work on a todo." }
func (t *CompleteTodo) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "ID of the todo item to complete",
			},
		},
		"required": []string{"id"},
	}
}

func (t *CompleteTodo) Call(ctx context.Context, args string) (string, error) {
	var arg struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(args), &arg); err != nil {
		return "", err
	}
	if arg.ID == "" {
		return "", fmt.Errorf("[complete_todo] id is empty")
	}

	session := memory.SessionFromContext(ctx)
	if session == nil {
		return "", fmt.Errorf("[complete_todo] no session in context")
	}

	if !session.Todo().Complete(arg.ID) {
		return "", fmt.Errorf("[complete_todo] todo %s not found", arg.ID)
	}
	return fmt.Sprintf(`{"id": "%s", "status": "completed"}`, arg.ID), nil
}

// ==================== ListTodos ====================

type ListTodos struct{}

func NewListTodosTool() *ListTodos { return &ListTodos{} }

func (t *ListTodos) Name() string        { return "list_todos" }
func (t *ListTodos) Description() string { return "List all todo items and their status. Use this to check progress." }
func (t *ListTodos) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (t *ListTodos) Call(ctx context.Context, args string) (string, error) {
	session := memory.SessionFromContext(ctx)
	if session == nil {
		return "", fmt.Errorf("[list_todos] no session in context")
	}

	todos := session.Todo().Items()
	if len(todos) == 0 {
		return `{"todos": [], "progress": "0/0"}`, nil
	}

	done, total := session.Todo().Progress()
	result := map[string]interface{}{
		"todos":    todos,
		"progress": fmt.Sprintf("%d/%d", done, total),
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}
