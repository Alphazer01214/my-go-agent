# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run Commands

```bash
make run          # Run main.go (legacy hello-world entrypoint)
make run-mygo     # Run the actual agent from cmd/mygoagent/main.go
make test         # Run all tests: go test -v ./...
make build        # Build binary to bin/mygoagent
```

Run a single test:
```bash
go test -v ./internal/llm -run TestChat
```

## Configuration

The agent requires a `.env` file with:
- `API_KEY` — LLM API key
- `BASE_URL` — OpenAI-compatible API base URL
- `MODEL` — model name

Long-term project memory is stored in `.memory/context.md` (auto-created, gitignored).

## Architecture

This is an LLM agent framework written in Go. The core loop is a **thought → tool call → observation** cycle powered by an OpenAI-compatible chat API with function calling.

### Agent Layer (`internal/agent/`)

- **`Agent` interface** — `Run(ctx, prompt) (string, error)` + `Name()` + `Steps()`
- **`BaseAgent`** — shared config (model, tools, memory, system prompt, step callback, max steps). All agent types embed this.
- **`ToolCallingAgent`** — the core agent. Runs a streaming loop: send messages + tool definitions to LLM → if LLM returns tool calls, execute them and feed results back → repeat until LLM responds without tool calls or max steps reached.
- **`PlanAgent`** — decomposes a user request into a JSON plan (`goal` + `todos`) via LLM, then executes each todo sequentially. Each todo can specify an `expert` name to route to, otherwise falls back to a fresh `ToolCallingAgent`.
- **`Expert`** — wraps any `Agent` with a name/description for routing. `ExpertAsTool` adapts an Expert into a `tool.Tool` so it can be called by a `ToolCallingAgent`.
- **`ExpertManager`** — a `ToolCallingAgent` whose tools are all `ExpertAsTool` wrappers, letting the LLM decide which expert to invoke.
- **`ReviewAgent`** — specialized agent for code review. Uses an embedded Go template (`template/code_review.tmpl`) and returns structured JSON with issues/severity.

### LLM Layer (`internal/llm/`)

- **`ChatModel` interface** — `Chat()` (non-streaming) and `StreamChat()` (SSE streaming with callback)
- **`OpenAIChatModel`** — implements `ChatModel` against any OpenAI-compatible `/chat/completions` endpoint. Handles SSE parsing, tool call accumulation across stream chunks, and exponential-backoff retries (up to 6 attempts, only on retryable HTTP status codes).
- Message types: `SystemMessage`, `UserMessage`, `AssistantMessage`, `AssistantMessageWithToolCall`, `ToolMessage`

### Tool Layer (`internal/tool/`)

- **`Tool` interface** — `Name()`, `Description()`, `Parameters()` (JSON Schema), `Call(ctx, args) (string, error)`
- **`Registry`** — maps tool names to implementations. `ToLLMTool()` converts all registered tools to the LLM function-calling format.
- Built-in tools: `read_file`, `list_dir`, `exec_command`, `update_memory`, `question_table`

### Memory Layer (`internal/memory/`)

- **`Memory` interface** — `Add`, `Messages`, `Compress`, `Clear`
- **`SlideWindow`** — keeps the last N messages (default 20), always preserves the system message. `Compress()` trims old messages but avoids cutting mid-tool-call sequence.
- **`SummaryMemory`** — when messages exceed a threshold, uses the LLM to summarize old messages into a single summary.
- **`ProjectContext`** — file-backed long-term memory (`.memory/context.md`). Persisted to disk, thread-safe. Injected into the system prompt automatically.

### Workflow Layer (`internal/workflow/`)

Currently skeletal — `Chain` and `Node` types are defined but not yet implemented.

### Utilities (`internal/utils/`)

- **`ParseJSON`** — robust JSON extraction from LLM output. Handles raw JSON, ```json code blocks, and embedded JSON objects/arrays in text.

## Code Conventions

- Comments are in Chinese (Mandarin) — match existing style when adding comments.
- System prompts are defined as constants in `internal/prompts/prompt.go`.
- Tool arguments are always JSON strings; each tool unmarshals internally.
- The `exec_command` tool uses `cmd /C` on Windows, `sh -c` on other platforms.
