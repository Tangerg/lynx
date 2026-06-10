# CLAUDE.md — mcp module

> Bridge between Model Context Protocol (MCP) and lynx `chat.Tool` system — both client (consume remote MCP tools) and server (expose lynx tools as MCP).
> 项目级约定见 `../CLAUDE.md`。

---

## 一句话定位

`modelcontextprotocol/go-sdk` 的 lynx 适配层：把远端 MCP server 的 tools 包装成 `core/chat.Tool` 给 agent 用，也把 lynx 端的 tool 暴露成 MCP server。**协议层不重新发明**，靠官方 SDK。

**有意聚焦 Tools 这一条 primitive**（连同 sampling / elicitation / progress / log 等反向辅助特性）。MCP 的另外两个 server primitive（Resources / Prompts）+ client primitive（Roots）的取舍见下「## MCP primitive 取舍」。

## 技术栈

- Go 1.26.3
- `github.com/modelcontextprotocol/go-sdk` v1.6+（包内别名 `sdkmcp`，避免和我们包名冲突）
- `go.opentelemetry.io/otel` 1.43
- `core/model/chat` —— 复用 `chat.Tool` 接口
- ~1.3k LOC，~13 个 .go 文件

## 核心架构

- **`provider.go`** —— 客户端聚合：`Provider.Tools(ctx)` 拉取并缓存 MCP server 的 tools，`tools/list_changed` notification 触发 invalidate
- **`tool.go`** —— `Tool` 包装器，远端 MCP tool → lynx `chat.Tool`（call → MCP RPC → 拿结果）
- **`server.go`** —— 服务端：`RegisterTools(server, lynxTools, ...)` 把 lynx `chat.Tool` 注册成 MCP `CallTool` handler
- **`transport.go` / `session.go`** —— 生命周期 + context 注入
- **`meta.go` / `prompt.go` / `reverse.go` / `sampling.go`** —— 自定义 `_meta` map / prompt 消息→`chat.Message` 转换（仅消费端 helper，见下取舍）/ 反向能力（progress·elicit·log）/ sampling

## 关键接口/类型

- `Provider` —— `Tools(ctx) ([]chat.Tool, error)` + `Invalidate()`
- `Tool` —— 包装 `sdkmcp.Tool`，实现 `chat.Tool`
- `Source` —— `{Name, Session}` 二元组（一个 MCP server 一份）
- `NamingFunc` —— tool 名 de-conflict（默认 `DefaultNaming` = `"<source>_<tool>"`）
- `MetaFunc` —— 注入自定义 `_meta` map

## 强约定

- **缓存 + double-checked locking**：`Tools()` 首次拉远端，之后命中缓存，直到 `tools/list_changed` notify 触发 invalidate
- **命名 deterministic**：默认 `<sourceName>_<toolName>`，多 server 时调用方自己换 `NamingFunc` de-conflict
- **错误分流**：`ToolCallError` 区分远端 `IsError`（业务错误，给 LLM 看）vs transport 失败（重试）
- **包名 alias**：包内统一 `import sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"`，避免和本包 `mcp` 冲突

## 关键目录

```
mcp/
├── provider.go      客户端 tool aggregator + cache
├── tool.go          远端 tool → chat.Tool 包装
├── server.go        服务端 RegisterTools
├── session.go       session lifecycle
├── transport.go     transport hooks
├── reverse.go       反向能力 helper（progress / elicit / log）
├── meta.go          _meta map 处理
├── prompt.go        prompts/get 结果 → []chat.Message（仅消费端）
└── sampling.go      sampling 请求
```

## MCP primitive 取舍

MCP 官方定义 server 有三大 primitive（[modelcontextprotocol.io](https://modelcontextprotocol.io/docs/learn/server-concepts)）+ client 侧若干能力。本模块**有意只完整覆盖 Tools**，其余按真实消费需求决定，不为不存在的需求建抽象（YAGNI）：

| Primitive | 控制方 | 本模块 | 取舍理由 |
|---|---|---|---|
| **Tools** | model | ✅ 完整（client `Provider` 发现 + server `RegisterTools`） | 主线 —— lynx 是 agent/RAG 基础设施，model-controlled 的 tool 调用是核心 |
| **Sampling / Elicitation / Progress / Log** | — | ✅ `sampling.go` / `reverse.go` | tool 执行期间真实需要的反向辅助特性 |
| **Resources** | application | ❌ 暂不做（`content.go` 仅把 embedded resource 当内容类型，`PromptMessagesToChat` 丢弃之） | **零消费者** —— lyra 没有任何代码消费 MCP resource；且"应用挑数据喂模型"这条线 lynx 已由 **RAG pipeline（vectorstores）+ documentreaders + LYRA.md/AGENTS.md 级联** 覆盖。**触发条件**：当真出现需要消费的 resource-only MCP server（且 `task` 子 agent 要用其数据）时，按 thin-wrapper 给 `Provider` 加 `resources/list` + `resources/read`，把 resource 拉成 `document.Document` 接 rag 那条线 |
| **Prompts**（server 侧提供） | user | ❌ 不做（仅有消费端 `PromptMessagesToChat`） | prompt 靠前端 slash-command / 命令面板触发，lyra 是后端、**无 prompt-picker 消费面**。给不存在的 UI 暴露 prompt 是推测性 hook |
| **Roots** | client→server | ❌ 不做 | 用途是 client 向远端广播文件系统边界；lynx 用 mcp 主要消费远端 tool，无此场景 |

> 加任何一项前先在官方 go-sdk 看接口形状，本包是 thin wrapper，不维护自己的协议状态。Resources 是其中唯一有现实理由的（野生 resource-only server 存在），但需 **需求驱动**——出现具体消费者再接。

## 常用命令

```bash
go build ./...
go test ./...
```

## 修改任何东西之前

- **改 provider 缓存逻辑**：跑 `provider_test.go`，并发 + invalidate 路径都测
- **加新 MCP primitive**（resources / server-side prompts / roots）：先看「## MCP primitive 取舍」——它们是有意暂缓而非遗漏；确认有真实消费者后再按 thin-wrapper 接，先在官方 go-sdk 看接口形状，不维护自己的协议状态
- **不要自己写 JSON-RPC envelope** —— 用 sdkmcp 的；同 lyra 模块约定一致
