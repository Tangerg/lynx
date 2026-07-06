# lynx/mcp — 设计与使用

> 把 [Model Context Protocol](https://modelcontextprotocol.io/) 桥接到 lynx 的
> `chat.Tool` 体系。底层使用官方 [`modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk) `v1.5.0+`。
>
> 本文是实现层 + 使用文档；架构层取舍在 `doc/MCP_DESIGN.md`，本文不再重复。

---

## 1. 关键决策

| # | 决策 |
|---|-----|
| D1 | **单包合并**：客户端、服务端两侧都在 `package mcp` 下，沿用 SDK 与 `net/http` 的"按概念分文件、不分包"风格 |
| D2 | **不引入新协议层**：直接复用 SDK 的 `*sdkmcp.ClientSession` / `*sdkmcp.Server`，本包只做 `chat.Tool` ↔ MCP tool 的双向适配 |
| D3 | **Config 结构体**取代 functional options：`*ToolConfig` / `*ProviderConfig`，`nil` 等价 `&Config{}`，零值字段走包默认 |
| D4 | **同步 API**：`chat.Tool.Call` 与 SDK `ClientSession.CallTool` 都是同步签名，无需 sync/async 双套接口 |
| D5 | **错误语义双向反转**：
| | • Client 侧：MCP `IsError=true` → `*ToolCallError`（用 `errors.As(err, &tcErr)` 判别），区分远端工具失败 vs 协议/传输错误 |
| | • Server 侧：lynx tool 的 Go `error` → `IsError=true` + TextContent（避免错误被升格为 JSON-RPC 协议错误） |
| D6 | **缓存 + 事件驱动刷新**：Provider 用 `atomic.Pointer` 缓存工具列表，`tools/list_changed` 通知通过 `Provider.OnToolListChanged` 失效缓存，下次拉取时按需刷新 |
| D7 | **命名 fail-fast**：多 server 同名工具默认走 `<source>_<tool>` 前缀，仍冲突直接报错而非静默覆盖 |

---

## 2. 包结构

```
mcp/
├── doc.go        // 包注释 + import alias 约定
├── errors.go     // ToolCallError
├── meta.go       // MetaFunc + WithMeta + MetaFromContext
├── content.go    // 包内私有 helper：textOfContent / flattenContent /
│                 //   schemaToString / decodeArguments / firstTextOrFallback /
│                 //   stringSchemaToAny / emptyObjectSchema
├── tool.go       // Tool + ToolConfig + NewTool
├── provider.go   // Provider + Source + ProviderConfig + NewProvider
│                 //   + NamingFunc + DefaultNaming
├── prompt.go     // PromptMessagesToChat — MCP PromptMessage → chat.Message
├── sampling.go   // SamplingHandler + SamplingViaChatClient
└── server.go     // RegisterTools
```

切分原则：一文件 = 一概念。Meta / Naming / Content 三个独立的小子系统各占一个 file；`tool.go` 仅描述 `Tool` 本身；`prompt.go` / `sampling.go` 各只暴露一个公共函数，不引入 wrapper 类型，直接吃 SDK 原型 → 吐 lynx 类型。

### Import 约定

本包名为 `mcp`，与官方 SDK 同名。源码内一律：

```go
import sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
```

外部使用者推荐：

```go
import (
    lynxmcp "github.com/Tangerg/lynx/mcp"
    "github.com/modelcontextprotocol/go-sdk/mcp"
)
```

下文示例采用后者：`mcp.*` 指 SDK，`lynxmcp.*` 指本包。

---

## 3. API 速查

### 类型

| 类型 | 用途 |
|-----|-----|
| `lynxmcp.Tool` | 把单个远端 MCP tool 包装成 `chat.Tool` |
| `lynxmcp.ToolConfig` | `Tool` 配置（PrefixedName / Metadata / MetaFunc） |
| `lynxmcp.Source` | 一条命名的 `*mcp.ClientSession`，喂给 Provider |
| `lynxmcp.Provider` | 多源工具发现与缓存；产出 `[]chat.Tool` |
| `lynxmcp.ProviderConfig` | Provider 配置（Naming / MetaFunc） |
| `lynxmcp.NamingFunc` | 函数类型；定制工具公开名 |
| `lynxmcp.MetaFunc` | 函数类型；从 `context.Context` 抽取 `_meta` 透传给远端 |
| `lynxmcp.ToolCallError` | 结构化错误；`errors.As(err, &tcErr)` 同时判别远端工具失败并取出 `ToolName` / `Message` |
| `lynxmcp.SamplingHandler` | `mcp.ClientOptions.CreateMessageHandler` 的别名；让 server 端发起 LLM 调用 |

### 函数

| 函数 | 说明 |
|-----|-----|
| `NewTool(ToolConfig)` | 单 tool 包装（一般由 Provider 内部调用）；config 校验在内部完成，不外暴 `Validate` |
| `NewProvider(ProviderConfig)` | 创建 Provider；sources 是 config 的字段 |
| `DefaultNaming(src, tool)` | 默认命名策略（普通 func，不可重新赋值）；Naming 留 nil 时使用 |
| `(*Provider).Tools(ctx)` | 取缓存的工具列表（首次或失效后会拉取） |
| `(*Provider).Invalidate()` | 标记缓存过期；下次 `Tools` 自动重拉 |
| `(*Provider).OnToolListChanged(ctx, req)` | 用作 `mcp.ClientOptions.ToolListChangedHandler` |
| `RegisterTools(server, tools...)` | 把若干 `chat.Tool` 暴露到 MCP server |
| `WithMeta(ctx, meta)` | 把 `mcp.Meta` 塞进 ctx，配合 `MetaFromContext` 使用 |
| `MetaFromContext(ctx)` | 从 ctx 读取由 `WithMeta` 注入的元数据；签名匹配 `MetaFunc` |
| `PromptMessagesToChat(msgs)` | 把 `*mcp.GetPromptResult.Messages` 转成 `[]chat.Message`，方便喂给 `chat.Client.ChatWith*` |
| `SamplingViaChatClient(c)` | 用 `*chat.Client` 实现的 `SamplingHandler`，装到 `mcp.ClientOptions.CreateMessageHandler` 让 server 端可以"借"本地 LLM |

---

## 4. 客户端示例

### 4.1 基础：连接 stdio MCP server，列工具并直接调一个

适合用 SDK 现成的 `examples/server/hello` 这种 stdio 子进程。

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os/exec"

    "github.com/modelcontextprotocol/go-sdk/mcp"

    "github.com/Tangerg/lynx/core/model/chat"
    lynxmcp "github.com/Tangerg/lynx/mcp"
)

func main() {
    ctx := context.Background()

    // 1. 启动 MCP server 子进程并初始化会话。
    cli := mcp.NewClient(&mcp.Implementation{
        Name:    "lynx-app",
        Version: "v0.1.0",
    }, nil)

    transport := &mcp.CommandTransport{Command: exec.Command("./mcp-hello-server")}
    session, err := cli.Connect(ctx, transport, nil)
    if err != nil {
        log.Fatalf("connect: %v", err)
    }
    defer session.Close()

    // 2. 用 Provider 把远端工具映射成 lynx Tool。
    provider, err := lynxmcp.NewProvider(lynxmcp.ProviderConfig{
        Sources: []lynxmcp.Source{{Name: "hello", Session: session}},
        // Naming 留空 -> DefaultNaming；MetaFunc 留空 -> 不透传 _meta
    })
    if err != nil {
        log.Fatal(err)
    }

    tools, err := provider.Tools(ctx)
    if err != nil {
        log.Fatal(err)
    }

    // 3. 直接驱动其中一个工具(绕过 LLM)。
    for _, tool := range tools {
        fmt.Printf("- %s: %s\n", tool.Definition().Name, tool.Definition().Description)
    }

    if len(tools) > 0 {
        callable := tools[0].(chat.Tool)
        out, err := callable.Call(ctx, `{"name":"world"}`)
        if err != nil {
            log.Fatal(err)
        }
        fmt.Println("result:", out)
    }
}
```

### 4.2 与 `chat.Client` 集成：让 LLM 自由调用远端工具

`chat.ToolMiddleware` 自动驱动「LLM ↔ tool」循环，把 MCP 工具喂进去就行。

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os/exec"

    "github.com/modelcontextprotocol/go-sdk/mcp"

    "github.com/Tangerg/lynx/core/model/chat"
    lynxmcp "github.com/Tangerg/lynx/mcp"
    // 你自己的 chat.Model 实现
    "example.com/myapp/myllm"
)

func main() {
    ctx := context.Background()

    // 一、连 MCP server 拿工具。
    cli := mcp.NewClient(&mcp.Implementation{Name: "lynx-app", Version: "v0.1.0"}, nil)
    session, err := cli.Connect(ctx, &mcp.CommandTransport{
        Command: exec.Command("./mcp-weather-server"),
    }, nil)
    if err != nil {
        log.Fatal(err)
    }
    defer session.Close()

    provider, err := lynxmcp.NewProvider(lynxmcp.ProviderConfig{
        Sources: []lynxmcp.Source{{Name: "weather", Session: session}},
    })
    if err != nil {
        log.Fatal(err)
    }
    tools, err := provider.Tools(ctx)
    if err != nil {
        log.Fatal(err)
    }

    // 二、装 ChatClient + ToolMiddleware。
    chatClient, err := chat.NewClientWithModel(myllm.New())
    if err != nil {
        log.Fatal(err)
    }
    callMW, streamMW := chat.NewToolMiddleware()

    // 三、问一个会触发工具调用的问题。
    text, _, err := chatClient.
        ChatWithPrompt("What's the weather like in Tokyo today?").
        WithTools(tools...).
        WithCallMiddlewares(callMW).
        WithStreamMiddlewares(streamMW).
        Call().
        Text(ctx)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(text)
}
```

### 4.3 多 server 聚合 + 自定义命名

```go
session1, _ := cli.Connect(ctx, weatherTransport, nil)
session2, _ := cli.Connect(ctx, calendarTransport, nil)

provider, _ := lynxmcp.NewProvider(lynxmcp.ProviderConfig{
    Sources: []lynxmcp.Source{
        {Name: "weather", Session: session1},
        {Name: "calendar", Session: session2},
    },
    // 自定义命名：仅在工具名前加 "ext_"
    Naming: func(_ string, t *mcp.Tool) string {
        return "ext_" + t.Name
    },
})
```

> 默认命名是 `<sourceName>_<toolName>`；不指定 `Naming` 即可。
> 同名冲突时 `Tools(ctx)` 返回 `error`，启动期就能发现问题。

### 4.4 订阅 list_changed 通知，工具列表变更时自动刷新

```go
// 闭包延迟绑定：Provider 在 Connect 之后才能创建，
// 但 ClientOptions 必须在 NewClient 时就给出。
var provider *lynxmcp.Provider

cli := mcp.NewClient(
    &mcp.Implementation{Name: "lynx-app", Version: "v0.1.0"},
    &mcp.ClientOptions{
        ToolListChangedHandler: func(ctx context.Context, req *mcp.ToolListChangedRequest) {
            if provider != nil {
                provider.OnToolListChanged(ctx, req)
            }
        },
    },
)
session, _ := cli.Connect(ctx, transport, nil)

provider, _ = lynxmcp.NewProvider(lynxmcp.ProviderConfig{
    Sources: []lynxmcp.Source{{Name: "main", Session: session}},
})

// 之后远端任何 AddTool/RemoveTool 都会触发 provider.Invalidate()，
// 下一次 provider.Tools(ctx) 自动重拉。
```

### 4.5 透传请求级元数据（userId、traceId 等）

```go
provider, _ := lynxmcp.NewProvider(lynxmcp.ProviderConfig{
    Sources:  sources,
    MetaFunc: lynxmcp.MetaFromContext, // 函数值直接赋给 MetaFunc 字段
})

// 请求路径上某一层（HTTP middleware / agent runtime）注入元数据：
ctx = lynxmcp.WithMeta(ctx, mcp.Meta{
    "userId":  "u-42",
    "traceId": "tx-abc",
})

// 后续每一次 Tool.Call 都会把这份 meta 写到 CallToolParams._meta，
// MCP server 端通过 req.Params.Meta 读取。
```

如需更复杂的逻辑（按角色过滤、字段加工），写一个 `func(ctx) sdkmcp.Meta` 直接赋值即可——`MetaFunc` 就是函数类型，没有 wrapper 层。

---

## 5. 服务端示例

### 5.1 基础：把 lynx 工具暴露成 stdio MCP server

```go
package main

import (
    "context"
    "log"

    "github.com/modelcontextprotocol/go-sdk/mcp"

    lynxmcp "github.com/Tangerg/lynx/mcp"
    // lynx 自带的样例工具
    "github.com/Tangerg/lynx/tools/fakeweatherquery"
)

func main() {
    server := mcp.NewServer(&mcp.Implementation{
        Name:    "lynx-bridge",
        Version: "v0.1.0",
    }, nil)

    if err := lynxmcp.RegisterTools(server,
        fakeweatherquery.New(/* args */),
    ); err != nil {
        log.Fatal(err)
    }

    // stdio transport 阻塞直到对端断开。
    if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
        log.Fatal(err)
    }
}
```

把可执行文件拷进 Claude Desktop / Cursor 等 MCP host 的配置即可被发现。

### 5.2 通过 `*chat.ToolRegistry` 批量暴露

`RegisterTools` 接受可变参数，搭配 `(*chat.ToolRegistry).All()` 即可批量暴露：

```go
support := chat.NewToolSupport()
support.RegisterTools(myTool1, myTool2 /* ... */)

server := mcp.NewServer(impl, nil)
if err := lynxmcp.RegisterTools(server, support.Registry().All()...); err != nil {
    log.Fatal(err)
}
```

如果 registry 中混杂了非 callable 的外部委托工具，`RegisterTools` 会 fail-fast。需要静默跳过这些条目时，调用方自己过滤一遍即可。

### 5.3 Streamable HTTP transport（Web 部署）

```go
package main

import (
    "log"
    "net/http"

    "github.com/modelcontextprotocol/go-sdk/mcp"

    lynxmcp "github.com/Tangerg/lynx/mcp"
)

func main() {
    server := mcp.NewServer(&mcp.Implementation{
        Name:    "lynx-bridge",
        Version: "v0.1.0",
    }, nil)
    if err := lynxmcp.RegisterTools(server, myTools()...); err != nil {
        log.Fatal(err)
    }

    // 所有 session 复用同一个 *mcp.Server（stateless 部署）。
    handler := mcp.NewStreamableHTTPHandler(
        func(*http.Request) *mcp.Server { return server },
        nil,
    )

    log.Fatal(http.ListenAndServe(":8080", handler))
}
```

如需 per-session 状态，把工厂函数改成"每次 new 一个新 Server"即可。

---

## 6. 错误处理约定

| 路径 | 上游表现 | 本包动作 | 下游观察 |
|-----|---------|---------|---------|
| Client → 远端 tool 失败 | `CallToolResult.IsError=true` | 返回 `*ToolCallError`（用 `errors.As` 判别） | `chat.ToolMiddleware` 把 error 上抛 |
| Client → 协议/传输错误 | `session.CallTool` 返回 Go error | 套上 `"call tool %q: %w"` 上下文 | 同上 |
| Client → 参数 unmarshal 失败 | `json.Unmarshal` 失败 | 套上 `"decode arguments for tool %q: %w"` | 同上 |
| Server → lynx tool 返回 error | `chat.Tool.Call` 返回 error | 转成 `CallToolResult{IsError:true, Content:[TextContent{err.Error()}]}` | LLM 端能"看见"错误并自我纠正 |
| Server → 协议级故障 | （无对应路径，本包不主动产生协议错误） | — | — |

**核心约定**：tool 失败永远以 `IsError=true` 形式跨过协议，不要触发 JSON-RPC `error` 字段——后者会被 SDK 当作"server 不支持这个工具"，破坏 LLM 端语义。

### 客户端错误判别

```go
out, err := tool.Call(ctx, args)
var tcErr *lynxmcp.ToolCallError
switch {
case err == nil:
    // success
case errors.As(err, &tcErr):
    // 远端 tool 自身失败：可考虑把 message 直接展示给用户/LLM 而不是重试
    log.Printf("tool %s reported failure: %s", tcErr.ToolName, tcErr.Message)
default:
    // 传输/参数错误：通常应当重试或上报基础设施告警
    log.Printf("tool call infrastructure error: %v", err)
}
```

---

## 7. 测试模式

go-sdk 提供 `mcp.NewInMemoryTransports()`，返回一对相互连通的内存 transport。本包所有单测都基于这一对，不起进程、不开端口。最小骨架：

```go
serverT, clientT := mcp.NewInMemoryTransports()

// server 必须先 Connect。
srv := mcp.NewServer(impl, nil)
_ = lynxmcp.RegisterTools(srv, myTools()...)
ss, _ := srv.Connect(ctx, serverT, nil)
defer ss.Close()

// client 端再 Connect，会自动 initialize 并就绪。
cli := mcp.NewClient(impl, nil)
cs, _ := cli.Connect(ctx, clientT, nil)
defer cs.Close()

// 之后用 cs / Tool / Provider 做断言。
```

参见 `provider_test.go`、`tool_test.go`、`server_test.go` 中的 `startServerWithEcho` / `connectPair` 等 helper。

---

## 8. 路线图（未做）

- **富内容回灌**：当前 `Tool.Call` 把 image/audio/embedded resource 序列化为 JSON 字符串。若要把 ImageContent 真正注入下一轮 LLM 输入，需要扩展 `chat.ToolReturn` / `chat.ToolMessage` 的 schema（影响 `core/`），延后到 v2。
- **其余反向能力**：`elicitation`、`roots`、`logging`、`progress` 这几条 server-to-client 通道仍未抽象——单独一条 `SamplingViaChatClient` 已落地（chat 集成够用），其余等真有用例再做。
- **Resources / Prompts / Complete 客户端 wrapper**：刻意不做。SDK 直接调（`session.Resources` / `session.GetPrompt` / `session.Complete`）已足够，再包一层只是协议层重复。需要把 prompt 喂给 chat 时用 `PromptMessagesToChat` 即可。
- **OTel 埋点**：在 `Tool.Call` 与 `serverHandler` 包一层 span，关联到 `doc/OBSERVABILITY_DESIGN.md` 的 GenAI 语义规范。
- **`chat.Client.WithToolProvider(p)`**：给 `core/model/chat` 加一个动态拉取的语法糖，目前需要用户手动 `provider.Tools(ctx)` 后 `WithTools(...)`。等 MCP 生态在 lynx 内有更多用户后再决定。

---

## 9. 相关文档

- `doc/MCP_DESIGN.md` — 架构层取舍、Spring AI 对照、设计稿
- [`go-sdk` 文档](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp) — SDK 类型与方法的权威说明
- [MCP 规范](https://modelcontextprotocol.io/specification) — 协议本身
