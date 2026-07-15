package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"my-go-agent/internal/config"
	"os/exec"
	"runtime"
)

const defaultTimeout = config.ToolExecTimeout

type ExecCommand struct{}

type ExecCommandArgs struct {
	Command string `json:"command"`
}

func NewExecCommandTool() *ExecCommand {
	return &ExecCommand{}
}

func (e *ExecCommand) Name() string {
	return "exec_command"
}

func (e *ExecCommand) Description() string {
	return "Execute shell command. ONLY use when no other tool works. For files: read_file/write_file/edit_file. For search: search_file/glob. For dirs: list_dir."
}

func (e *ExecCommand) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Shell command to execute",
			},
		},
		"required": []string{"command"},
	}
}

func (e *ExecCommand) Call(ctx context.Context, args string) (string, error) {
	var arg ExecCommandArgs
	if err := json.Unmarshal([]byte(args), &arg); err != nil {
		return "", err
	}
	if arg.Command == "" {
		return "", fmt.Errorf("[exec_command] command is empty")
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", arg.Command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", arg.Command)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("[exec_command] %w", err)
	}
	return string(output), nil
}
