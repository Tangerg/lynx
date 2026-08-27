# scope/mcp — 设计与使用

`scope/mcp` 不是第二套 MCP SDK。协议客户端、服务端、session、transport
全部直接使用官方 `github.com/modelcontextprotocol/go-sdk/mcp`；本模块只保留
scope 需要、而官方 SDK 不应该知道的薄适配。

这个取舍来自官方 Go SDK `design.md` 的包组织思想：

- 协议域优先放在一个核心包里，像 `net/http`、`net/rpc`、`grpc` 一样提高可发现性。
- 不人为拆出 client/server/transport/tool 小包；未来协议演进时，过早的包结构很容易变错。
- 非 MCP 能力才放到独立包；应用配置、OAuth、热重连、状态展示属于应用层。
- 传输、session、server/client 生命周期使用官方 SDK 原语，不重新包装一套。

## 包结构

```
mcp/
├── doc.go       // 包注释和 import alias 约定
├── meta.go      // context scoped _meta helpers
├── reverse.go   // active call context + progress / elicitation reverse helpers
├── descriptor.go // immutable remote descriptor snapshot + schema projection
├── tool.go       // remote MCP tool -> tool.Tool
├── result.go     // remote result -> tool result / error
├── tools.go      // list remote tools and wrap them as []tool.Tool
├── server.go     // tool.Tool -> MCP server tool
└── prompt.go     // MCP prompt messages -> []chat.Message
```

应用运行时自己的 `mcpServers` 配置、OAuth 登录、热重连和状态管理放在
`app/runtime/internal/infra/mcp`，不放在本模块根包。

## Import 约定

本模块和官方 SDK 都叫 `mcp`。调用方通常这样导入：

```go
import (
    scopemcp "github.com/Tangerg/scope/mcp"
    sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)
```

## 核心 API

| API | 说明 |
|---|---|
| `WithRequestMeta` / `RequestMetaFromContext` | 把请求级 `_meta` 放进 context，并由 `Tool` 透传到 `CallToolParams.Meta` |
| `ReportProgress` | 根据原始 progress token 发送 progress notification |
| `Elicit` | tool 执行中使用官方 `sdkmcp.ElicitParams` 向客户端发起 elicitation |
| `DiscoverTools(ctx, sources, config)` | 现场列出远端 MCP tools，并包装成 `[]tool.Tool` |
| `Register(server, tools...)` | 把 scope `tool.Tool` 暴露到 MCP server |
| `PromptMessagesToChat` | 把 MCP prompt messages 转成 `[]chat.Message` |

根包刻意不提供：

- `ServerConfig` / `Dial`：这是应用配置装配。
- `Provider` / cache：工具列表刷新策略由应用决定。
- transport wrappers：直接用官方 SDK 的 `CommandTransport`、
  `StreamableClientTransport`、`NewStreamableHTTPHandler`。

## 客户端示例

```go
ctx := context.Background()

client := sdkmcp.NewClient(&sdkmcp.Implementation{
    Name: "scope-app", Version: "v0.1.0",
}, nil)

session, err := client.Connect(ctx, &sdkmcp.CommandTransport{
    Command: exec.Command("./mcp-server"),
}, nil)
if err != nil {
    return err
}
defer session.Close()

tools, err := scopemcp.DiscoverTools(ctx,
    []scopemcp.ToolSource{{Name: "local", Session: session}},
    scopemcp.ToolDiscoveryConfig{RequestMeta: scopemcp.RequestMetaFromContext},
)
if err != nil {
    return err
}

ctx = scopemcp.WithRequestMeta(ctx, sdkmcp.Meta{"userId": "u-42"})
out, err := tools[0].Call(ctx, `{"name":"world"}`)
```

## 服务端示例

```go
server := sdkmcp.NewServer(&sdkmcp.Implementation{
    Name: "scope-bridge", Version: "v0.1.0",
}, nil)

if err := scopemcp.Register(server, myTools()...); err != nil {
    return err
}

return server.Run(context.Background(), &sdkmcp.StdioTransport{})
```

HTTP 部署直接使用官方 SDK handler：

```go
handler := sdkmcp.NewStreamableHTTPHandler(
    func(*http.Request) *sdkmcp.Server { return server },
    nil,
)
```

## 错误处理约定

MCP tool execution failure crosses the protocol as `IsError=true`; it is not a
JSON-RPC protocol error. `scope/mcp` preserves that distinction:

- 远端 `CallToolResult.IsError=true` → `*scopemcp.ToolCallError`
- 传输/协议错误 → 普通 wrapped Go error
- 本地 `tool.Tool` 返回 error → `CallToolResult{IsError:true}`

调用方用 `errors.AsType` 区分远端 tool 自身失败和基础设施失败。

## 测试模式

单测优先使用官方 SDK 的 `sdkmcp.NewInMemoryTransports()`：

```go
serverT, clientT := sdkmcp.NewInMemoryTransports()

server := sdkmcp.NewServer(impl, nil)
_ = scopemcp.Register(server, tool)
ss, _ := server.Connect(ctx, serverT, nil)
defer ss.Close()

client := sdkmcp.NewClient(impl, nil)
cs, _ := client.Connect(ctx, clientT, nil)
defer cs.Close()
```

这和官方 SDK design.md 保持一致：协议测试走 SDK transport；scope 只测试自己的
adapter 语义。
