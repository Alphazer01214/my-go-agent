# 项目简历：my-go-agent — 基于 LLM Function Calling 的智能 Agent 框架

## 项目概述
**my-go-agent** 是一个用 **Go 语言** 实现的大语言模型（LLM）Agent 框架，底层基于 **OpenAI 兼容 API 的 Function Calling** 机制，完整实现了 **"思考(Think)→工具调用(Tool Call)→观察(Observe)"** 的核心循环。该项目采用模块化、接口驱动的架构设计，支持多 Agent 协作、专家路由、记忆管理、中间件插拔等高级特性。

---

## 技术栈
| 维度 | 技术选型 |
|------|---------|
| **编程语言** | Go 1.26 |
| **LLM API** | OpenAI 兼容接口 (Chat Completions + Function Calling) |
| **通信协议** | SSE (Server-Sent Events) 流式响应 |
| **外部依赖** | 仅 github.com/joho/godotenv（.env 配置加载） |
| **配置管理** | .env 文件（API_KEY, BASE_URL, MODEL） |

---

## 核心架构与模块

### 1. Agent 层 (`internal/agent/`) — 多策略智能体实现
| 组件 | 功能描述 |
|------|---------|
| **Agent 接口** | 定义 `Run(ctx, prompt)`, `Name()`, `Steps()` 通用契约 |
| **BaseAgent** | 封装 Model、Tools、Memory、SystemPrompt、StepCallback、MaxStep 等公共配置 |
| **ToolCallingAgent** | **核心循环实现**：流式调用 LLM → 解析 tool_calls → 执行工具 → 回传结果，直至 LLM 无工具调用或达到最大步数 |
| **PlanAgent** | 先让 LLM 将用户需求拆解为 JSON 计划（goal + todos），再依次执行每个子任务，支持按 expert 名称路由 |
| **ReviewAgent** | 专用代码审查 Agent，使用 Go template 渲染审查提示词，输出结构化 JSON 审查结果（支持 bug/security/performance/style 等多维度检测） |
| **Expert / ExpertAsTool / ExpertManager** | 多专家模式。将任意 Agent 包装为 Tool，实现 LLM 驱动的任务路由分发 |
| **中间件系统** | `AgentWithLog`（日志记录）、`AgentWithTrace`（步骤级追踪，记录每步 thought/action/observation） |

### 2. LLM 层 (`internal/llm/`) — 统一的大语言模型抽象
| 组件 | 功能描述 |
|------|---------|
| **ChatModel 接口** | `Chat()` (非流式同步调用) + `StreamChat()` (SSE 流式回调) |
| **OpenAIChatModel** | 完整实现：SSE 流解析、tool call 跨 chunk 累积、**指数退避重试**（最多 6 次，仅对可重试 HTTP 状态码） |
| **消息类型体系** | SystemMessage / UserMessage / AssistantMessage / AssistantMessageWithToolCall / ToolMessage |
| **Token 用量统计** | 记录每次请求的 input/output/total token 消耗 |
| **中间件系统** | `ChatModelWithLog`（LLM 调用日志）、`ChatModelWithTrace`（请求/响应级追踪） |

### 3. 工具层 (`internal/tool/`) — 可插拔工具生态系统
| 组件 | 功能描述 |
|------|---------|
| **Tool 接口** | Name / Description / Parameters (JSON Schema) / Call(ctx, args) |
| **Registry 注册中心** | 工具注册、查找、批量转换为 LLM function calling 格式 |
| **内置文件工具** | read_file, write_file, edit_file, list_dir, glob, search_file |
| **Shell 执行** | exec_command（安全执行系统命令） |
| **网络搜索** | web_search（可选，支持 Serper API） |
| **记忆管理** | update_memory（操作长期记忆文件） |
| **专家包装** | Expert-As-Tool 模式，将 Agent 无缝嵌入工具链 |

### 4. 记忆层 (`internal/memory/`) — 多策略上下文管理
| 组件 | 功能描述 |
|------|---------|
| **Memory 接口** | Add / Messages / Compress / Clear 统一契约 |
| **SlideWindow** | 滑动窗口策略，保留最近 N 条消息，压缩时智能避免切断 tool call 序列 |
| **SummaryMemory** | 超阈值时用 LLM 自动总结旧消息为摘要，保留长对话能力 |
| **ProjectContext** | **文件持久化长期记忆**（`.memory/context.md`），线程安全（`sync.RWMutex`），自动注入 system prompt |

### 5. 工作流层 (`internal/workflow/`) — Agent 编排引擎
| 组件 | 功能描述 |
|------|---------|
| **Chain** | 链式 Agent 编排基础架构 |
| **Graph** | 有向图编排引擎，支持 `AddNodes`、`AddEdge`、`SetRoot`、`RenameNode` 等操作，支持复杂多 Agent 工作流 |

### 6. 工具函数 (`internal/utils/`)
| 功能 | 描述 |
|------|------|
| **ParseJSON** | 鲁棒的 JSON 提取，支持纯 JSON、\`\`\`json 代码块、文本中嵌入的 JSON 多种格式 |

---

## 技术亮点

1. **纯接口驱动架构** — 所有核心模块（Agent、LLM、Tool、Memory）均面向接口编程，高度可测试、可扩展
2. **零重依赖** — 外部依赖仅 godotenv，其余全部手写实现（包括 SSE 流解析、模板引擎、JSON Schema 工具描述等）
3. **优雅的错误处理** — 指数退避 + 随机抖动（jitter）的重试策略，避免惊群效应
4. **中间件插拔模式** — 借鉴装饰器模式，支持日志、追踪等横切关注点的灵活组合
5. **流式体验** — 完整 SSE 流式响应，实时展示 Agent 思考过程
6. **长期记忆持久化** — 支持文件持久化的项目上下文，使 Agent 具备跨会话记忆能力
7. **多 Agent 协作** — PlanAgent + Expert 模式实现复杂任务拆解与分配，ReviewAgent 专注代码质量审查

---

## 构建与运行
```bash
make build        # 编译到 bin/mygoagent
make run          # 运行旧版 main.go
make run-mygo     # 运行实际 agent（cmd/mygoagent/main.go）
make test         # 运行全部单元测试 (go test -v ./...)
```

## 配置文件 (.env)
```
API_KEY=your_api_key
BASE_URL=https://api.openai.com/v1/chat/completions
MODEL=gpt-4
SEARCH_API_KEY=optional
SEARCH_BASE_URL=optional
```
