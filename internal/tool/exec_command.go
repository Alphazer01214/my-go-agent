package tool

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"runtime"
	"time"
)

const defaultTimeout = 60 * time.Second

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
	return "execute a shell command"
}

func (e *ExecCommand) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "shell command to execute, DO NOT USE THIS TOOL IF OTHER TOOL CAN DO IT",
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
		return "command is empty", errors.New("command is empty")
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", arg.Command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", arg.Command)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return string(output), nil
}
