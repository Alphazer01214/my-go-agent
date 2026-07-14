package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"my-go-agent/internal/memory"
)

type UpdateMemory struct {
	projectCtx *memory.ProjectContext
}

type UpdateMemoryArgs struct {
	Action  string `json:"action"`
	Content string `json:"content"`
}

func NewUpdateMemoryTool(ctx *memory.ProjectContext) *UpdateMemory {
	return &UpdateMemory{projectCtx: ctx}
}

func (u *UpdateMemory) Name() string {
	return "update_memory"
}

func (u *UpdateMemory) Description() string {
	return `update long-term project memory (persisted across sessions). Use this to remember:
- Project structure, purpose, and tech stack
- Coding conventions and patterns
- Key file locations and their roles
- Build/test/lint commands
- User preferences and decisions
Action: "append" to add a section, "replace" to overwrite all, "clear" to reset.`
}

func (u *UpdateMemory) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"append", "replace", "clear"},
				"description": "append: add new section, replace: overwrite all, clear: reset memory",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "markdown content to write (not needed for 'clear')",
			},
		},
		"required": []string{"action"},
	}
}

func (u *UpdateMemory) Call(ctx context.Context, args string) (string, error) {
	var arg UpdateMemoryArgs
	if err := json.Unmarshal([]byte(args), &arg); err != nil {
		return "", fmt.Errorf("[update_memory] parse args: %w", err)
	}

	switch arg.Action {
	case "append":
		if arg.Content == "" {
			return "", fmt.Errorf("[update_memory] content is required for append")
		}
		if err := u.projectCtx.AppendSection(arg.Content); err != nil {
			return "", err
		}
		return "memory updated (appended)", nil
	case "replace":
		if arg.Content == "" {
			return "", fmt.Errorf("[update_memory] content is required for replace")
		}
		if err := u.projectCtx.SetContent(arg.Content); err != nil {
			return "", err
		}
		return "memory replaced", nil
	case "clear":
		if err := u.projectCtx.SetContent(""); err != nil {
			return "", err
		}
		return "memory cleared", nil
	default:
		return "", fmt.Errorf("[update_memory] unknown action: %s (use append/replace/clear)", arg.Action)
	}
}
