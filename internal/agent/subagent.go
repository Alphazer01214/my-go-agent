package agent

import (
	"strings"
)

// subagent 的设计：仅调查而不改文件，改动操作交给 parent agent 实现（因为要实现 trace restore）

// Mode agent 运行模式，影响 system prompt 和返回格式
type Mode string

const (
	// ModeDefault 默认模式：结构化回传，子 agent 用标记包裹回答
	ModeDefault Mode = ""
	// ModeLightweight 轻量模式：最小化 token，纯文本回答，适合小规模模型
	ModeLightweight Mode = "lightweight"
)

// Result  子 agent 结构化返回结果
type Result struct {
	Answer      string   // 最终回答（必填）
	KeyFindings []string // 关键发现（可选，结构化模式下 LLM 填充）
	Files       []string // 涉及的文件路径（可选）
	TokensUsed  int      // 粗略 token 估算
}

// Format 将 AgentResult 格式化为可读字符串，供父 agent 的 tool result 使用
func (r *Result) Format() string {
	var sb strings.Builder
	sb.WriteString(r.Answer)
	if len(r.KeyFindings) > 0 {
		sb.WriteString("\n\n[Key Findings]")
		for _, f := range r.KeyFindings {
			sb.WriteString("\n- ")
			sb.WriteString(f)
		}
	}
	if len(r.Files) > 0 {
		sb.WriteString("\n\n[Files Touched]")
		for _, f := range r.Files {
			sb.WriteString("\n- ")
			sb.WriteString(f)
		}
	}
	return sb.String()
}

// SharedContext 父 agent → 子 agent 的共享上下文
type SharedContext struct {
	Background  string   // 背景信息（用户需求、项目描述等）
	Constraints []string // 约束条件
	Findings    []string // 已累积的关键发现（来自其他子 agent 的结果）
}

// Format 将 SharedContext 格式化为 system prompt 注入段
func (sc *SharedContext) Format() string {
	if sc == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Shared Context (from parent agent)")
	if sc.Background != "" {
		sb.WriteString("\n\n### Background\n")
		sb.WriteString(sc.Background)
	}
	if len(sc.Constraints) > 0 {
		sb.WriteString("\n\n### Constraints")
		for _, c := range sc.Constraints {
			sb.WriteString("\n- ")
			sb.WriteString(c)
		}
	}
	if len(sc.Findings) > 0 {
		sb.WriteString("\n\n### Findings from Previous Tasks")
		for _, f := range sc.Findings {
			sb.WriteString("\n- ")
			sb.WriteString(f)
		}
	}
	return sb.String()
}

// EstimateTokens 粗略估算文本的 token 数（中文约 1.5 字/token，英文约 4 字符/token）
// 简单取 len/3 作为折中估算，不引入外部 tokenizer 依赖
func EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return len(text)/3 + 1
}

// ExtractAnswer 从子 agent 的结构化输出中提取 ---ANSWER--- 与 ---END--- 之间的内容
// 找不到标记时返回原始字符串（兼容非结构化输出）
func ExtractAnswer(raw string) string {
	const (
		startTag = "---ANSWER---"
		endTag   = "---END---"
	)
	startIdx := strings.Index(raw, startTag)
	if startIdx == -1 {
		return strings.TrimSpace(raw)
	}
	startIdx += len(startTag)
	endIdx := strings.Index(raw[startIdx:], endTag)
	if endIdx == -1 {
		// 没有结束标记，取到末尾
		return strings.TrimSpace(raw[startIdx:])
	}
	return strings.TrimSpace(raw[startIdx : startIdx+endIdx])
}

// parseKeyFindings 从结构化输出中提取 ---KEY--- 后的行
func parseKeyFindings(raw string) []string {
	const tag = "---KEY---"
	idx := strings.Index(raw, tag)
	if idx == -1 {
		return nil
	}
	lines := strings.Split(raw[idx+len(tag):], "\n")
	var findings []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 遇到下一个 --- 标签则停止
		if strings.HasPrefix(line, "---") {
			break
		}
		// 去掉行首的 "- " 前缀
		line = strings.TrimPrefix(line, "- ")
		findings = append(findings, line)
	}
	return findings
}

// parseFiles 从结构化输出中提取 ---FILE--- 后的行
func parseFiles(raw string) []string {
	const tag = "---FILE---"
	idx := strings.Index(raw, tag)
	if idx == -1 {
		return nil
	}
	lines := strings.Split(raw[idx+len(tag):], "\n")
	var files []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "---") {
			break
		}
		line = strings.TrimPrefix(line, "- ")
		files = append(files, line)
	}
	return files
}

// ParseAgentResult 从子 agent 的原始输出中解析结构化结果
func ParseAgentResult(raw string) *Result {
	return &Result{
		Answer:      ExtractAnswer(raw),
		KeyFindings: parseKeyFindings(raw),
		Files:       parseFiles(raw),
		TokensUsed:  EstimateTokens(raw),
	}
}

// formatStructuredToolResult 将子 agent 输出格式化为父 agent 的 tool result
// 提取 answer 作为主要内容，附加 findings 和 files 摘要
func formatStructuredToolResult(raw string) string {
	result := ParseAgentResult(raw)
	return result.Format()
}

// ==================== Subagent Prompt Templates ====================

// SubagentStructuredPrompt 结构化模式的子 agent 指令
// 要求 LLM 用标记包裹回答，便于父 agent 提取
const SubagentStructuredPrompt = `## Output Format
When you finish your task, wrap your final answer in the following format:

---ANSWER---
[your answer here]
---END---

Optionally, list key findings and files you accessed:

---KEY---
- finding 1
- finding 2
---FILE---
- path/to/file1
- path/to/file2

Rules:
- The ---ANSWER--- block is REQUIRED and must contain your complete answer
- The ---KEY--- and ---FILE--- blocks are OPTIONAL
- Keep findings concise (one line each)
- Only list files you actually read or modified
- Do not include these markers in intermediate tool-call steps, only in your final response`

// SubagentLightweightPrompt 轻量模式的子 agent 指令
// 不要求结构化标记，只要求简洁回答，适合小规模模型
const SubagentLightweightPrompt = `## Response Guidelines
- Be concise and direct. No preamble or filler.
- Answer the task in the fewest words possible.
- If you used tools, summarize what you found — do not repeat raw tool output.
- Stop as soon as you have enough information to answer.`
