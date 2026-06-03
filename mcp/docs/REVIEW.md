# `mcp/` — Review 阅读顺序

`mcp/` 是 lynx 与 Model Context Protocol (modelcontextprotocol/go-sdk)
的桥：把外部 MCP 工具吸纳为 `core/model/chat.Tool`，把 lynx 工具暴露为
MCP server，并在两侧维护元数据 / 通知 / 采样 / 提示模板等扩展。

## 阅读顺序

1. `DESIGN.md` **[必读]** — 设计文档，先看，背景全在这里。
2. `doc.go` — 包说明。
3. `errors.go` — 错误集合（先看错误能感知整个模块的失败面）。
4. `transport.go` **[精读]** — Streamable HTTP transport（客户端 +
   服务端处理器）；Lyra MCP 客户端 (`lyra/internal/engine/mcp.go`)
   通过 `DialStreamableHTTP` 接入。
5. `session.go` — 客户端 session 管理（连接 / 重连 / 心跳）。
6. `meta.go` — `MetaFunc` 元数据钩子，决定每个工具调用如何把上下文
   塞给外部 server。
7. `tool.go` **[精读]** — `NewTool` 把 `*sdkmcp.Tool` 包成
   `chat.Tool`，注意：
   - schema 直通（不要让 SDK 反射覆盖手写 schema）
   - 错误如何 → `IsError` + TextContent
8. `provider.go` **[精读]** — `Provider` 聚合多 `Source`、做命名前缀
   去冲突、缓存 + invalidation。Lyra 的工具合并路径走这里。
9. `server.go` — `RegisterTools` 把 lynx tools 暴露成 MCP server。
10. `content.go` — 内容类型映射（text / image / audio）。
11. `prompt.go` — MCP prompt template 处理。
12. `sampling.go` — `sampling/createMessage` 协议方法的桥接。
13. `notify.go` — 服务端通知（tools/list_changed 等）的派发。
14. `tracing.go` — span 包装。

## 关注点

- **Naming 函数**：`NamingFunc` 是不是确定性的？两次调用要稳定，否则缓存
  失效会乱。
- **schema 完整性**：MCP server 给的 `InputSchema` 形态 (map[string]any)
  在 lynx tool 层的兼容性。
- **错误归一**：MCP 协议错误 ≠ tool 业务错误（参见 `tool.go` 的
  `IsError`）。
- **OTel**：所有 RPC 是否经过 `tracing.go`。

## 跨模块提醒

- Lyra 端：`lyra/internal/engine/mcp.go` 用 `Provider` + 多 Source。
- agent 端：`agent/runtime/mcp.go` 提供 platform-level 集成。

## 体检命令

- `go test ./mcp/...`
- 跑 `provider_test.go` 看 in-memory transport 的契约用法。
