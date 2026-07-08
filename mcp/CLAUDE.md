# CLAUDE.md — mcp module

> lynx 对 Model Context Protocol 的薄适配。协议 client/server/session/transport 直接使用官方 `github.com/modelcontextprotocol/go-sdk/mcp`；本模块不做第二套 SDK。
> 项目级约定见 `../CLAUDE.md`。

## 一句话定位

根包 `mcp` 放 lynx 自己需要的 MCP 周边能力：context metadata、server-to-client 反向能力 helper、`chat.Tool` 和 MCP tool 的双向适配、sampling/prompt 转换。应用层配置、OAuth 登录、热重连、状态展示属于 `app/runtime/internal/infra/mcp`。

## 技术栈

- Go 1.26.4
- `github.com/modelcontextprotocol/go-sdk` v1.6+
- `core/model/chat`
- `go.opentelemetry.io/otel` 1.43

## 核心架构

- `meta.go`：`WithMeta` / `MetaFromContext`
- `session.go`：`WithToolCall` / `ServerSessionFromContext`
- `reverse.go`：`ReportProgress` / `ElicitFromClient` / `LogToClient`
- `tools.go`：`ToolSource` / `ToolOptions` / `Tools`
- `tool.go`：内部 wrapper，远端 MCP tool → `chat.Tool`
- `server.go`：`Register(server, tools...)`，lynx `chat.Tool` → MCP server tool
- `sampling.go` / `prompt.go`：sampling 和 prompt 的 chat 适配

## 强约定

- **单包优先，少暴露**：同一 MCP 适配域先放在 `mcp` 根包里；远端工具只通过 `Tools` 暴露为 `[]chat.Tool`，不公开具体 wrapper/config。
- **不包装官方 SDK transport/session**：连接 stdio/HTTP 直接用 `sdkmcp.CommandTransport`、`sdkmcp.StreamableClientTransport`、`sdkmcp.NewStreamableHTTPHandler`。
- **无 Provider/cache 层**：工具列表刷新策略由应用层决定；收到 `tools/list_changed` 后重新调用 `Tools`。
- **协议错误和 tool 错误分开**：远端 `IsError=true` → `*ToolCallError`；传输/协议问题保持 wrapped Go error。
- **tool 失败不升格 JSON-RPC error**：server 侧 `chat.Tool` error 转成 `CallToolResult{IsError:true}`。
- **包名 alias**：包内统一 `sdkmcp` 指官方 SDK；外部建议 `lynxmcp` 指本包。

## 常用命令

```bash
go build ./...
go test ./...
```

## 修改任何东西之前

- **先看官方 go-sdk 接口形状**：本模块是 thin adapter，不维护自己的协议状态。
- **不要加配置注册中心**：`mcpServers`、OAuth handler、headers、reconnect 都在 app/runtime infra。
- **不要恢复 Provider/cache**：除非有多个真实调用方证明应用层刷新不能满足。
- **加新 MCP primitive**：优先直接暴露 SDK 类型或写一个小函数；不要包成框架。
