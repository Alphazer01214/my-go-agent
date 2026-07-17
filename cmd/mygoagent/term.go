package main

import "bytes"

// ANSI 颜色
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
)

// bracketed paste 终端协议标记
var (
	pasteStart = []byte("\x1b[200~")
	pasteEnd   = []byte("\x1b[201~")
)

// scanLinesOrPaste 自定义 scanner split 函数
// 支持两种模式：
//   - 普通输入：按行分割（与 ScanLines 相同）
//   - bracketed paste：检测到 \x1b[200~ 后，一直读到 \x1b[201~，整段作为一个 token
//     终端在粘贴时会自动加上这些转义序列，粘贴内容中的换行不会被截断
var inPaste bool

func scanLinesOrPaste(data []byte, atEOF bool) (advance int, token []byte, err error) {
	// 正在粘贴模式中：寻找 pasteEnd
	if inPaste {
		if idx := bytes.Index(data, pasteEnd); idx != -1 {
			inPaste = false
			// 返回 pasteStart 和 pasteEnd 之间的内容
			content := data[:idx]
			content = bytes.TrimRight(content, "\r\n")
			return idx + len(pasteEnd), content, nil
		}
		// 还没收到 pasteEnd，等更多数据
		if atEOF && len(data) > 0 {
			// EOF 但没收到结束标记，按普通行处理
			inPaste = false
			return len(data), bytes.TrimRight(data, "\r\n"), nil
		}
		return 0, nil, nil
	}

	// 检测 pasteStart
	if idx := bytes.Index(data, pasteStart); idx != -1 {
		// pasteStart 之前有普通内容，先返回它
		if idx > 0 {
			before := data[:idx]
			line := bytes.TrimRight(before, "\r\n")
			if len(line) > 0 {
				return idx, line, nil
			}
		}
		// 进入粘贴模式，跳过 pasteStart，继续读
		inPaste = true
		return idx + len(pasteStart), nil, nil
	}

	// 普通行模式：找换行符
	if idx := bytes.IndexByte(data, '\n'); idx != -1 {
		line := data[:idx]
		// 处理 \r\n
		if idx > 0 && line[idx-1] == '\r' {
			line = line[:idx-1]
		}
		return idx + 1, line, nil
	}

	// atEOF 且有剩余数据
	if atEOF && len(data) > 0 {
		return len(data), bytes.TrimRight(data, "\r\n"), nil
	}

	return 0, nil, nil
}

// truncateDisplay 截断字符串用于显示
func truncateDisplay(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
