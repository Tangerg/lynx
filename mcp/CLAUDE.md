# CLAUDE.md — mcp module

> Bridge between Model Context Protocol (MCP) and lynx `chat.Tool` system — both client (consume remote MCP tools) and server (expose lynx tools as MCP).
> 项目级约定见 `../lyra/CLAUDE.md`。

---

## 一句话定位

`modelcontextprotocol/go-sdk` 的 lynx 适配层：把远端 MCP server 的 tools 包装成 `core/chat.Tool` 给 agent 用，也把 lynx 端的 tool 暴露成 MCP server。**协议层不重新发明**，靠官方 SDK。

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
- **`transport.go` / `session.go`** —— 生命周期 + notification 分发
- **`meta.go` / `prompt.go` / `notify.go` / `sampling.go`** —— OTel attrs / 自定义 `_meta` map / sampling

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
├── notify.go        notification 分发
├── meta.go          _meta map 处理
├── prompt.go        prompt 协议（如果用到）
└── sampling.go      sampling 请求
```

## 常用命令

```bash
go build ./...
go test ./...
```

## 修改任何东西之前

- **改 provider 缓存逻辑**：跑 `provider_test.go`，并发 + invalidate 路径都测
- **加新 MCP 能力**（resources / sampling 等）：先在官方 go-sdk 看接口形状，本包是 thin wrapper，不维护自己的协议状态
- **不要自己写 JSON-RPC envelope** —— 用 sdkmcp 的；同 lyra 模块约定一致
