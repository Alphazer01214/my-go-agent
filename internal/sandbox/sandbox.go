package sandbox

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
)

// RiskLevel 风险等级
type RiskLevel string

const (
	RiskSafe     RiskLevel = "safe"     // 只读，自动放行
	RiskHigh     RiskLevel = "high"     // 写操作，需确认
	RiskCritical RiskLevel = "critical" // 高危操作，需确认 + 参数校验
)

// Action 执行动作
type Action string

const (
	ActionAllow    Action = "allow"     // 本次放行
	ActionApprove  Action = "approve"   // 批量放行（本次 Run 内该工具全部放行）
	ActionDeny     Action = "deny"      // 拒绝
)

// Rule 用户自定义规则
type Rule struct {
	Tool    string // 工具名，"*" 匹配所有
	Pattern string // 参数匹配正则，空 = 匹配该工具所有调用
	Action  Action
}

// ConfirmFunc 确认回调：返回执行动作
type ConfirmFunc func(tool, args string) Action

// Sandbox 沙箱：拦截高危工具调用，征求用户确认
type Sandbox struct {
	mu           sync.Mutex
	defaultRisk  map[string]RiskLevel // 内置风险等级
	rules        []Rule               // 用户自定义规则
	approved     map[string]bool      // 本次 Run() 已批量放行的工具
	confirmFunc  ConfirmFunc          // 确认回调
}

// New 创建 Sandbox，使用默认终端确认交互
func New() *Sandbox {
	return &Sandbox{
		defaultRisk: defaultRiskLevels(),
		approved:    make(map[string]bool),
		confirmFunc: terminalConfirm,
	}
}

// NewWithConfirm 创建 Sandbox，使用自定义确认回调
func NewWithConfirm(confirm ConfirmFunc) *Sandbox {
	return &Sandbox{
		defaultRisk: defaultRiskLevels(),
		approved:    make(map[string]bool),
		confirmFunc: confirm,
	}
}

// AddRule 添加用户自定义规则
func (s *Sandbox) AddRule(rule Rule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rules = append(s.rules, rule)
}

// Reset 重置批量放行状态（每次 Run() 开始时调用）
func (s *Sandbox) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.approved = make(map[string]bool)
}

// Check 检查工具调用是否允许执行
// 返回：(allowed bool, reason string)
func (s *Sandbox) Check(name, args string) (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. 检查是否已批量放行
	if s.approved[name] {
		return true, ""
	}

	// 2. 检查用户自定义规则
	for _, rule := range s.rules {
		if rule.Tool != name && rule.Tool != "*" {
			continue
		}
		if rule.Pattern != "" {
			re, err := regexp.Compile(rule.Pattern)
			if err != nil {
				continue
			}
			if !re.MatchString(args) {
				continue
			}
		}
		switch rule.Action {
		case ActionAllow, ActionApprove:
			return true, ""
		case ActionDeny:
			return false, fmt.Sprintf("denied by rule: tool=%s pattern=%s", rule.Tool, rule.Pattern)
		}
	}

	// 3. exec_command 危险命令检测（先于确认，直接拒绝）
	if name == "exec_command" {
		if safe, reason := CheckDangerousCommand(args); !safe {
			return false, reason
		}
	}

	// 4. 检查内置风险等级
	risk := s.defaultRisk[name]
	switch risk {
	case RiskSafe:
		return true, ""
	case RiskHigh, RiskCritical:
		if s.confirmFunc == nil {
			return false, "no confirm function configured"
		}
		action := s.confirmFunc(name, args)
		switch action {
		case ActionAllow:
			return true, ""
		case ActionApprove:
			s.approved[name] = true
			return true, ""
		default:
			return false, "denied by user"
		}
	default:
		// 未知工具，默认需要确认
		if s.confirmFunc == nil {
			return false, "unknown tool and no confirm function"
		}
		action := s.confirmFunc(name, args)
		switch action {
		case ActionAllow:
			return true, ""
		case ActionApprove:
			s.approved[name] = true
			return true, ""
		default:
			return false, "denied by user"
		}
	}
}

// defaultRiskLevels 返回内置风险等级
func defaultRiskLevels() map[string]RiskLevel {
	return map[string]RiskLevel{
		"read_file":      RiskSafe,
		"list_dir":       RiskSafe,
		"glob":           RiskSafe,
		"search_file":    RiskSafe,
		"web_search":     RiskSafe,
		"question_table": RiskSafe,
		"create_todo":    RiskSafe,
		"start_todo":     RiskSafe,
		"complete_todo":  RiskSafe,
		"list_todos":     RiskSafe,
		"write_file":     RiskHigh,
		"edit_file":      RiskHigh,
		"exec_command":   RiskCritical,
	}
}

// terminalConfirm 终端确认交互
func terminalConfirm(tool, args string) Action {
	fmt.Printf("\n  ⚠  Sandbox: %s\n", tool)

	summary := args
	if len(summary) > 200 {
		summary = summary[:200] + "..."
	}
	summary = strings.ReplaceAll(summary, "\n", " ")
	fmt.Printf("    args: %s\n", summary)

	fmt.Print("  [y] Approve once  [a] Approve all  [d] Deny\n  > ")

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	switch input {
	case "y", "yes":
		return ActionAllow
	case "a", "all":
		return ActionApprove
	default:
		return ActionDeny
	}
}
