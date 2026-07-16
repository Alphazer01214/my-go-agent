package sandbox

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// dangerPatterns 危险命令模式（正则）
// 匹配到的命令将被直接拒绝，不询问用户
var dangerPatterns = []struct {
	Pattern string
	Reason  string
}{
	// 文件系统破坏
	{`rm\s+(-[a-zA-Z]*r[a-zA-Z]*f|--recursive)\s+/`, "recursive delete from root"},
	{`rm\s+(-[a-zA-Z]*f[a-zA-Z]*r|--recursive)\s+/`, "recursive delete from root"},
	{`rm\s+-rf\s+\.`, "recursive delete current directory"},
	{`rmdir\s+/[sS]\s+/[qQ]`, "silent recursive directory removal"},
	{`mkfs\.`, "filesystem format"},
	{`format\s+[a-zA-Z]:`, "disk format"},
	// 磁盘直接写入
	{`dd\s+.*of=/dev/`, "direct disk write"},
	// 权限提升
	{`chmod\s+777\s+/`, "set 777 on root path"},
	{`chown\s+root\s+/`, "change owner to root on root path"},
	// 下载并执行
	{`curl\s.*\|\s*(sh|bash|zsh)`, "download and execute"},
	{`wget\s.*\|\s*(sh|bash|zsh)`, "download and execute"},
	{`curl\s.*&&\s*(sh|bash|zsh)`, "download and execute"},
	// 系统控制
	{`shutdown\s`, "system shutdown"},
	{`reboot\s`, "system reboot"},
	{`halt\s`, "system halt"},
	{`init\s+[06]`, "system shutdown/reboot"},
	// 进程杀手
	{`kill\s+-9\s+1\b`, "kill init process"},
	{`killall\s+-9`, "kill all processes"},
	// 危险重定向
	{`>\s*/dev/sd`, "write to block device"},
	{`>\s*/dev/nvme`, "write to NVMe device"},
	// Windows 高危命令
	{`del\s+/[sS]\s+/[qQ]\s+[a-zA-Z]:\\`, "silent recursive delete"},
	{`format\s+[a-zA-Z]:`, "disk format"},
}

// CheckDangerousCommand 检查 exec_command 的命令是否危险
// 返回：(safe bool, reason string)
func CheckDangerousCommand(argsStr string) (bool, string) {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
		return true, "" // JSON 解析失败，不拦截（让工具自己报错）
	}

	cmd := strings.TrimSpace(args.Command)
	if cmd == "" {
		return true, ""
	}

	// 检查危险模式
	for _, dp := range dangerPatterns {
		re, err := regexp.Compile(dp.Pattern)
		if err != nil {
			continue
		}
		if re.MatchString(cmd) {
			return false, fmt.Sprintf("dangerous command detected: %s (%s)", dp.Reason, cmd)
		}
	}

	// 检查 sudo + 危险命令
	if strings.HasPrefix(cmd, "sudo ") {
		inner := strings.TrimPrefix(cmd, "sudo ")
		for _, dp := range dangerPatterns {
			re, err := regexp.Compile(dp.Pattern)
			if err != nil {
				continue
			}
			if re.MatchString(inner) {
				return false, fmt.Sprintf("dangerous sudo command: %s (%s)", dp.Reason, cmd)
			}
		}
	}

	return true, ""
}
