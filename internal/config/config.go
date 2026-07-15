package config

import (
	"time"

	"github.com/joho/godotenv"
)

func LoadDotEnv() {
	godotenv.Load()
}

// Agent 配置常量
const (
	DefaultMaxSteps     = 30   // 默认最大步数（作为安全阀，主要依赖 LLM 自行判断停止）
	DefaultMemorySize   = 50   // 默认记忆窗口大小
	DefaultReadFileSize = 4096 // 默认读取文件大小（字节）
)

// LLM 配置常量
const (
	DefaultLLMTimeout     = 66 * time.Second // LLM 请求超时
	DefaultLLMMaxRetries  = 6                // LLM 最大重试次数
	DefaultLLMRetryBaseMs = 100              // 重试基础延迟（毫秒）
	DefaultLLMRetryMaxSec = 10               // 重试最大延迟（秒）
)

// Tool 配置常量
const (
	ToolSearchMaxResults = 20               // search_file 最大结果数
	ToolGlobMaxResults   = 50               // glob 最大结果数
	ToolWebSearchResults = 3                // web_search 默认结果数
	ToolExecTimeout      = 60 * time.Second // exec_command 超时
)

// 变量（可通过环境变量或函数修改）
var (
	MaxSteps     = DefaultMaxSteps
	MemorySize   = DefaultMemorySize
	ReadFileSize = DefaultReadFileSize
)
