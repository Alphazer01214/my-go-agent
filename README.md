# GoAgent

用 Go 实现的 LLM Agent 框架和交互式 CLI。项目通过 OpenAI 兼容的 Chat Completions API 与模型通信，并用 Function Calling 驱动“思考 -> 工具调用 -> 观察”循环。

仓库同时实现了会话持久化、上下文压缩、执行轨迹、回溯分支、工具沙箱和多 Agent 编排。

## 功能概览

- **Agent 循环**：流式调用 LLM，解析跨数据块的 `tool_calls`，执行工具并回传结果。
- **事件溯源会话**：以追加式事件日志作为唯一事实源，消息、轨迹和压缩视图都从日志派生。
- **上下文压缩**：上下文超过预算后生成检查点摘要，保留完整日志并支持撤销压缩。
- **回溯与分支**：支持硬回溯、快速撤销工具调用，以及从已完成轮次创建会话分支。
- **多 Agent 编排**：提供计划分解、专家路由、子 Agent 和基础 DAG 流程。
- **工具与沙箱**：内置文件、搜索、命令、待办和交互工具，并对高风险操作要求确认。
- **交互式 CLI**：支持单次运行、会话管理、轨迹查看、模型切换和多行粘贴。

## 快速开始

**环境要求**

- Go 1.26 或更高版本。
- 支持 `chat/completions`、Function Calling 和 SSE 流式输出的 OpenAI 兼容服务。
- Windows、Linux 或 macOS。命令工具会按平台选择 `cmd /C` 或 `sh -c`。

**配置**

在项目根目录创建 `.env`：

```dotenv
API_KEY=your-api-key
BASE_URL=https://api.deepseek.com
MODEL=deepseek-chat

# 可选。两项都存在时才注册 web_search。
SEARCH_API_KEY=your-search-api-key
SEARCH_BASE_URL=https://your-search-service.example
```

| 变量 | 必填 | 说明 |
| --- | --- | --- |
| `API_KEY` | 是 | LLM 服务的 API Key |
| `BASE_URL` | 是 | OpenAI 兼容 API 的基础地址 |
| `MODEL` | 是 | 模型名称，可在 REPL 中用 `/model` 切换 |
| `SEARCH_API_KEY` | 否 | 网络搜索服务的 Bearer Token |
| `SEARCH_BASE_URL` | 否 | 网络搜索服务地址，请求路径为 `/search` |

`.env` 已被 `.gitignore` 忽略。不要把真实密钥提交到仓库。

**运行**

交互式模式：

```bash
go run ./cmd/mygoagent
```

单次运行模式：

```bash
go run ./cmd/mygoagent "读取项目结构，并总结当前架构"
```

单次运行默认最多等待 300 秒。交互式模式直接输入消息，使用 `exit`、`quit`、`/exit` 或 `/quit` 退出。

Windows PowerShell 构建：

```powershell
go build -o bin\mygoagent.exe .\cmd\mygoagent
.\bin\mygoagent.exe
```

类 Unix 环境构建：

```bash
go build -o bin/mygoagent ./cmd/mygoagent
./bin/mygoagent
```

也可以使用 Makefile：

```bash
make run-mygo
make build
```

## CLI 使用

不带参数启动 REPL。带参数时，所有参数会合并为一次任务输入。

REPL 会显示当前消息数和待办进度。粘贴多行文本时，终端的 bracketed paste 内容会作为一个完整输入处理。

**命令**

| 命令 | 作用 |
| --- | --- |
| `/help`、`/h` | 显示命令列表 |
| `/new`、`/n` | 创建并切换到新会话 |
| `/fork [事件 ID 或 seq]` | 从指定锚点创建分支，无参数时从最新已完成轮次分支 |
| `/switch [id]` | 切换会话，无参数时列出会话树 |
| `/rename <标题>` | 重命名当前会话 |
| `/archive [id]` | 归档会话，不删除日志 |
| `/delete <id>` | 永久删除会话，操作不可逆 |
| `/reset` | 重置当前会话的消息和轨迹 |
| `/trace`、`/t` | 查看执行轨迹 |
| `/trace last` | 查看最后一个轨迹事件 |
| `/trace stats` | 查看轨迹统计 |
| `/trace clear` | 清空展示轨迹 |
| `/todo` | 查看待办列表 |
| `/restore [事件 ID 或 seq]` | 硬回溯到指定位置 |
| `/revert` | 回滚到上一个工具调用之前 |
| `/messages`、`/m` | 列出当前会话消息 |
| `/context`、`/c` | 查看上下文和 token 估算 |
| `/compress` | 手动创建压缩检查点 |
| `/sessions`、`/s` | 以树形结构列出会话 |
| `/info` | 查看当前会话信息 |
| `/pwd` | 查看工作目录和项目信息 |
| `/model [name]` | 查看或切换模型 |
| `/exit`、`/quit` | 保存并退出 |

`/restore` 会截断目标之后的会话日志，并按文件快照逆序恢复可逆的文件变更。`/delete` 和文件恢复都应谨慎使用。

## 内置工具

CLI 默认注册以下工具：

| 工具 | 用途 |
| --- | --- |
| `read_file` | 读取文件，默认最多返回 4096 字节 |
| `write_file` | 创建、覆盖或追加文件 |
| `edit_file` | 按精确文本替换编辑文件 |
| `list_dir` | 列出目录内容 |
| `glob` | 按文件名模式查找文件 |
| `search_file` | 搜索文件内容 |
| `exec_command` | 运行构建、测试、Git 或其他命令 |
| `make_dir` | 创建目录，可递归创建父目录 |
| `question_table` | 向用户提问并获取选择或文本 |
| `create_todo` | 创建待办事项 |
| `start_todo` | 将待办标记为进行中 |
| `complete_todo` | 将待办标记为已完成 |
| `list_todos` | 列出所有待办 |
| `web_search` | 查询网络搜索服务，仅在配置搜索变量后启用 |
| `spawn_subagent` | 创建子 Agent 处理独立分析任务 |

`internal/tool` 还提供 `update_memory`。自定义入口可以调用 `tool.NewUpdateMemoryTool(project)` 注册它，用于维护 `.memory/project.md`。

**工具缓存**

`read_file`、`list_dir`、`glob` 和 `search_file` 是只读工具。它们的结果会在进程内缓存。写文件、编辑文件或执行命令后，缓存会自动失效。

## 工具沙箱

工具按风险分级执行：

- 只读工具自动放行。
- 文件写入、编辑和未知工具需要用户确认。
- `exec_command` 属于高风险操作，并额外执行危险命令检查。
- 明确危险的命令会被直接拒绝，不再询问用户。

确认提示支持三种操作：

| 输入 | 行为 |
| --- | --- |
| `y` | 本次放行 |
| `a` | 当前 `Run` 内放行同名工具 |
| `d` | 拒绝执行 |

## 会话与记忆

**事件日志**

每个会话对应一个 JSONL 文件：

```text
.memory/sessions/<session-id>.jsonl
```

文件首行是会话 Header，后续每行是一个不可变事件。事件序号从 0 开始连续递增。日志包含用户消息、助手消息、工具结果、待办快照、轮次边界和压缩检查点。

模型可见消息是日志表面的折叠结果。完整人类记录通过 append-only 视图保留。崩溃恢复会截断不完整的尾行，并清理未闭合的轮次或步骤。

**压缩检查点**

模型可见上下文的估算 token 数超过 24,000 后，会话会触发摘要压缩。压缩保留最近 10 个消息组，并用一条 `<compacted-summary>` 消息替换旧上下文。

压缩不会删除原始事件。撤销检查点后，被遮蔽的消息会重新进入模型可见表面。

**执行轨迹**

Trace 是日志之上的展示视图。它记录用户输入、模型思考、工具调用、工具结果、沙箱拦截、压缩和恢复操作。

Trace 事件关联日志的 `seq` 区间。会话加载、分支或回溯后，可以依据日志重建轨迹。

**会话分支**

`/fork` 会复制父会话在锚点之前的日志前缀。分支边界必须落在完整轮次边界上。父会话不会被修改，子会话通过 `parent_session` 和 `seed_length` 保留血缘信息。

## 架构

```text
cmd/mygoagent
    |
    v
internal/agent  -----> internal/llm
    |                    OpenAI-compatible API
    |
    +------> internal/tool -------> internal/sandbox
    |
    +------> internal/memory
               |
               +------> .memory/project.md
               +------> .memory/sessions/*.jsonl

internal/workflow ------> internal/agent
internal/prompts
internal/config
internal/utils
```

| 目录 | 职责 |
| --- | --- |
| `cmd/mygoagent` | 主 CLI、REPL 和斜杠命令 |
| `cmd/agent` | 旧版多专家演示入口 |
| `internal/agent` | Agent 循环、计划、专家、审查和子 Agent |
| `internal/llm` | OpenAI 兼容 API、消息类型、SSE 与重试 |
| `internal/tool` | 工具接口、注册表和内置工具 |
| `internal/memory` | 事件日志、会话、压缩、轨迹、分支和持久化 |
| `internal/sandbox` | 工具风险检查和危险命令拦截 |
| `internal/workflow` | 基础 DAG 节点、依赖和事件监听 |
| `internal/prompts` | 系统提示词和模板 |
| `internal/config` | 默认配置与运行参数 |

`ToolCallingAgent` 是主执行引擎。`PlanAgent` 先生成 `goal` 和 `todos`，再顺序执行任务。
`ExpertManager` 把专家包装成工具，让模型按任务描述路由。
`SubagentTool` 创建独立会话的子 Agent。这样不会打乱父会话的 `tool_calls` 与 `tool` 消息顺序。

## 二次开发

核心扩展点如下：

| 接口或类型 | 扩展方式 |
| --- | --- |
| `agent.Agent` | 实现新的任务执行器 |
| `llm.ChatModel` | 接入其他 OpenAI 兼容服务或 Mock |
| `tool.Tool` | 增加新的模型可调用能力 |
| `sandbox.Sandbox` | 调整风险规则或确认逻辑 |
| `memory.Project` | 管理项目、长期记忆和会话 |
| `workflow.Graph` | 组合多个 Agent 并监听执行事件 |

## 测试

构建所有包：

```bash
go build ./...
```

运行不访问真实 LLM 服务的测试：

```bash
go test ./internal/agent ./internal/memory ./internal/middleware ./internal/utils ./internal/workflow
```

运行完整测试：

```bash
go test ./...
```

完整测试包含 `internal/llm.TestChat`。该测试会读取 `.env` 并调用配置的 LLM 服务，需要有效 API 凭据和网络连接。

单独验证真实 API：

```bash
go test -v ./internal/llm -run TestChat
```
