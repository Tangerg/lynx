# Lynx MCP 集成设计

> **目标**：在 lynx 现有 `chat.Tool` / `ToolMiddleware` 体系之上接入 [Model Context Protocol](https://modelcontextprotocol.io/)，
> 让 lynx 既能消费外部 MCP server 暴露的工具/资源/提示词，也能把 lynx 的本地工具反向暴露成 MCP server。
> 底层 SDK 选用官方 [`github.com/modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk) `v1.5.0`（2026-04 已 GA）。
>
> 关联文档：`ARCHITECTURE.md` §工具子系统、`MIDDLEWARE_DESIGN.md` §B 类中间件、`SPRING_AI_COMPARISON.md`。

---

## 1. 设计目标与非目标

### 1.1 目标

| # | 目标 |
|---|-----|
| G1 | **零侵入**：不改 `core/model/chat/tool.go` 任何一行；MCP 通过实现 `chat.CallableTool` 加入工具体系 |
| G2 | **双向**：消费 (Client) + 暴露 (Server) 两条路径都支持 |
| G3 | **依赖隔离**：MCP SDK 重依赖（`google/jsonschema-go`、`uritemplate`、`oauth2`...）走独立 module，不污染 `core/` |
| G4 | **协议透明**：lynx 上层（`chat.Client`、`ToolMiddleware`）感知不到 MCP 存在，外部 MCP 工具与本地工具无差异 |
| G5 | **生命周期托管**：list_changed 通知自动刷新工具缓存，无需用户手动 refresh |
| G6 | **多 server 安全**：多个 MCP server 同名工具走前缀策略，重名时 fail-fast |

### 1.2 非目标

- ❌ **不重新实现 MCP 协议**：完全依赖 `go-sdk`，不自己写 JSON-RPC、SSE、Streamable HTTP
- ❌ **不抽象 Sync/Async 双 API**：Go 不像 Java 需要 `Mono` 镜像；`CallableTool.Call` 一条同步 API 已足够（go-sdk 内部本就异步）
- ❌ **不做 OAuth 工作流**：`auth` / `oauthex` 包按需直接暴露给用户；lynx 不再加一层
- ❌ **不内置 transport**：`stdio` / `streamable HTTP` / `SSE` 全部直接复用 go-sdk 的 `mcp.*Transport`

---

## 2. Spring AI MCP 设计速读（用于借鉴）

### 2.1 分层

```
┌────────────────────────────────────────────────────────┐
│ ChatClient / ChatModel  (Spring AI 上层)               │
└──────────────▲─────────────────────────────────────────┘
               │ 调用 ToolCallback.call(json) -> string
┌──────────────┴─────────────────────────────────────────┐
│ ToolCallback (Spring AI Tool 抽象)                     │
│   - getToolDefinition() : ToolDefinition (含 schema)   │
│   - call(input, ctx)    : String                       │
└──────────────▲─────────────────────────────────────────┘
               │ implements
┌──────────────┴─────────────────────────────────────────┐
│ SyncMcpToolCallback / AsyncMcpToolCallback             │  ← 适配层（薄）
│ SyncMcpToolCallbackProvider                            │
│   - 启动时 listTools()，构建 ToolCallback[]            │
│   - onApplicationEvent(McpToolsChangedEvent) 失效缓存  │
└──────────────▲─────────────────────────────────────────┘
               │ 复用官方 SDK
┌──────────────┴─────────────────────────────────────────┐
│ io.modelcontextprotocol.sdk (协议 + Transport)         │
└────────────────────────────────────────────────────────┘
```

### 2.2 关键决策（值得抄）

| 决策 | Spring AI 怎么做 | lynx 是否照搬 |
|-----|----------------|-------------|
| **桥接基本单元** | `McpToolCallback` 把单个 MCP tool 包成 `ToolCallback` | ✅ 抄：`mcpClientTool` 实现 `chat.CallableTool` |
| **批量发现** | `McpToolCallbackProvider` 统一从一/多个 client 拉 tool 列表 | ✅ 抄：`mcp/client.Provider`，但接口更小 |
| **缓存 + 事件刷新** | `volatile` 缓存 + `McpToolsChangedEvent` invalidate | ✅ 抄：`atomic.Pointer[[]chat.Tool]` + go-sdk 的 `ToolListChangedHandler` |
| **命名冲突** | `McpToolNamePrefixGenerator`（缩写客户端名 + 连接名 + tool 名，截 64 字符） | ✅ 抄思路，但默认更激进：`<conn>_<tool>` 全长，不截断；用户可注入策略 |
| **错误处理** | `CallToolResult.isError=true` → 抛 `ToolExecutionException` | ⚠ 改造：lynx 没有专门 ToolExecutionException 类型，直接 `return "", err`；`isError` 的 content text 包成 `error.Error()` |
| **Schema 编码** | 用字符串 JSON Schema（`ToolDefinition.inputSchema()` 返回 String） | ✅ 同：lynx 本就是 `string` |
| **ToolContext 透传** | `ToolContext` 中除 `mcpExchange` 外的 entries → `CallToolParams.Meta` | ✅ 抄：`Request.Params` → `_meta`（保留 key 黑名单） |
| **Sync vs Async 双类** | 两个独立类，无共享父类 | ❌ 不抄：Go 单一同步路径就够 |
| **反向：本地工具 → MCP Server** | `McpToolUtils.toSyncToolSpecification(toolCallback)` | ✅ 抄：`mcp/server.RegisterTools(s, lynxTools...)` |
| **反向能力（sampling/elicitation）** | `@McpSampling` / `@McpElicit` 注解 | ⏸ 暂不做（v1）；放到第二阶段 |

### 2.3 一次远程工具调用的时序（对照学习）

```
LLM 决定调用 weather_query_api(...)
        │
        ▼
chat.ToolMiddleware.executeCall
        │  invoker.Find("weather_query_api") -> mcpClientTool
        │
        ▼
mcpClientTool.Call(ctx, argsJSON)
        │  json.Unmarshal(argsJSON) -> map[string]any
        │  build CallToolParams{Name, Arguments, _meta=ctxMeta}
        ▼
session.CallTool(ctx, &params)        ────► [MCP Server 执行]
        ◄──── CallToolResult{Content: [...], IsError: false}
        │
        │  result.IsError ? -> return "", errors.New(text)
        │  flatten Content -> string  (TextContent 直接拼，其它走 Media)
        ▼
返回 string 给 ToolMiddleware → 塞回 ToolMessage → 下一轮 LLM
```

---

## 3. lynx 现状对照

### 3.1 核心抽象（已就位）

| 抽象 | 文件 | 行为 |
|-----|------|------|
| `chat.Tool` | `core/model/chat/tool.go:56-64` | `Definition()` + `Metadata()`，纯定义 |
| `chat.CallableTool` | `core/model/chat/tool.go:69-82` | 在 Tool 之上加 `Call(ctx, args string) (string, error)` |
| `chat.ToolDefinition` | `core/model/chat/tool.go:20-29` | `Name` / `Description` / `InputSchema string` |
| `chat.ToolMetadata` | `core/model/chat/tool.go:33-38` | 仅 `ReturnDirect bool` |
| `chat.ToolRegistry` | `core/model/chat/tool.go:150-272` | 线程安全只读注册表 |
| `chat.ToolMiddleware` | `core/model/chat/tool_middleware.go` | LLM ↔ Tool 递归循环（Call + Stream 双版本） |
| `chat.Request.Options.Tools` | `core/model/chat/request.go:35` | 每次请求注入工具列表 |
| `chat.Request.Params` | `core/model/chat/request.go:182` | `map[string]any`，可作 MCP `_meta` 通道 |
| `core/media.Media` | `core/media/media.go:12-18` | 通用富内容载体，可承接 MCP 的 Image/Audio/Resource |
| `pkg/json.StringDefSchemaOf` | `pkg/json/schema.go:53` | invopop/jsonschema 反射出 schema 字符串 |

### 3.2 关键观察

1. **`CallableTool.Call` 签名与 MCP `tools/call` 的语义完美匹配**：都是 `(jsonString) -> jsonString`。MCP tool → CallableTool 是一对一映射，不需要桥转两次。
2. **`ToolMiddleware` 已经处理了循环 / `ReturnDirect` / 流式累积**——MCP 工具加进 `Options.Tools` 立刻可用，无需新中间件。
3. **lynx 走 invopop/jsonschema，go-sdk 走 google/jsonschema-go**——都吐 draft 2020-12 兼容 JSON。**字符串形态在两库间互通**，所以 lynx 不需要把 schema 引擎切换到 go-sdk 那一套；具体桥接见 §6。

---

## 4. 模块布局

参照 `models/`、`vectorstores/`、`tools/` 的拆 module 惯例，新增独立顶层 module：

```
lynx/
├── core/                       # 不动
├── tools/
│   ├── fakeweatherquery/       # 既有
│   └── ...
├── mcp/                        # ★ 新增 module，独立 go.mod
│   ├── go.mod                  #   require core, modelcontextprotocol/go-sdk v1.5.0
│   ├── client/                 # ─ 消费侧：MCP server tool → lynx Tool
│   │   ├── tool.go             #   mcpClientTool implements chat.CallableTool
│   │   ├── provider.go         #   Provider：拉列表 + 缓存 + list_changed 刷新
│   │   ├── naming.go           #   NamingStrategy 接口与默认实现
│   │   ├── content.go          #   MCP Content[] -> string / *media.Media 的扁平化
│   │   └── meta.go             #   chat.Request.Params <-> _meta 转换
│   ├── server/                 # ─ 暴露侧：lynx Tool → MCP server
│   │   ├── adapter.go          #   RegisterTools(*mcp.Server, ...chat.Tool)
│   │   └── handler.go          #   Tool handler 闭包构造
│   └── go.work / examples/     #   端到端样例
└── go.work                     # 把 mcp 加入 workspace
```

### 4.1 为什么独立 module？

- 与 `models/anthropic`、`vectorstores/qdrant` 同档：**重依赖外置**。
- go-sdk v1.5.0 会拉 `google/jsonschema-go`、`segmentio/encoding`、`yosida95/uritemplate/v3`、`golang-jwt/jwt/v5`、`golang.org/x/oauth2`，全部进 `mcp/go.sum`，不污染 `core/`、`pkg/`。
- 用户不用 MCP 时零成本。

### 4.2 为什么不再分 `mcp/transport/`？

go-sdk 的 transport 全部写在 `mcp` 包里（`StdioTransport` / `CommandTransport` / `StreamableHTTPHandler` / `SSEHandler`），不是独立子包。lynx 没必要再加一层包装。

---

## 5. Client 侧：消费外部 MCP

### 5.1 单工具桥接

```go
// mcp/client/tool.go (sketch)

package client

import (
    "context"
    "encoding/json"
    "errors"

    "github.com/Tangerg/lynx/core/model/chat"
    "github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpClientTool struct {
    session    *mcp.ClientSession
    raw        *mcp.Tool                  // 原始 MCP tool 元数据
    definition chat.ToolDefinition        // 已生成（含前缀名 + schema string）
    metadata   chat.ToolMetadata
    metaConv   MetaConverter              // chat.Request.Params -> _meta（见 §9）
}

func (t *mcpClientTool) Definition() chat.ToolDefinition { return t.definition }
func (t *mcpClientTool) Metadata() chat.ToolMetadata     { return t.metadata }

func (t *mcpClientTool) Call(ctx context.Context, args string) (string, error) {
    var arguments any
    if args == "" {
        arguments = map[string]any{}
    } else if err := json.Unmarshal([]byte(args), &arguments); err != nil {
        return "", err
    }

    params := &mcp.CallToolParams{
        Name:      t.raw.Name,                       // ★ 用原始名，不是前缀名
        Arguments: arguments,
        Meta:      t.metaConv.FromContext(ctx),      // 透传 ctx 上的扩展元信息
    }

    res, err := t.session.CallTool(ctx, params)
    if err != nil {
        return "", err
    }
    if res.IsError {
        return "", errors.New(flattenContentText(res.Content))
    }
    return flattenContent(res.Content)               // 见 §7
}
```

要点：
- `Definition().Name` 是**前缀化后的对外名**（避免多 server 撞名），但请求 MCP server 时用 `t.raw.Name` 原始名。
- `IsError == true` 翻译成 Go `error`（与 lynx 现有 ToolMiddleware 错误传播链一致）。
- `Arguments` 字段类型是 `any`，可以直接传 `map[string]any`。

### 5.2 Provider：批量发现 + 缓存

```go
// mcp/client/provider.go (sketch)

type Provider struct {
    sources []Source         // 每个 Source 持一条 ClientSession
    naming  NamingStrategy
    metaConv MetaConverter

    cache atomic.Pointer[[]chat.Tool]
    once  sync.Once
}

type Source struct {
    Name    string             // 用户给的连接名（前缀生成依据）
    Session *mcp.ClientSession
}

// Tools 返回当前缓存；首次调用时 lazy 拉取。
func (p *Provider) Tools(ctx context.Context) ([]chat.Tool, error) {
    if cached := p.cache.Load(); cached != nil {
        return *cached, nil
    }
    return p.refresh(ctx)
}

// refresh 强制重新拉取所有 source 的 tool 列表。
func (p *Provider) refresh(ctx context.Context) ([]chat.Tool, error) {
    var all []chat.Tool
    for _, src := range p.sources {
        for tool, err := range src.Session.Tools(ctx, nil) { // iter.Seq2 自动翻页
            if err != nil {
                return nil, err
            }
            all = append(all, p.adapt(src, tool))
        }
    }
    if err := validateUniqueNames(all); err != nil {
        return nil, err
    }
    p.cache.Store(&all)
    return all, nil
}
```

**connect 时挂 list_changed 钩子**：

```go
client := mcp.NewClient(impl, &mcp.ClientOptions{
    ToolListChangedHandler: func(ctx context.Context, req *mcp.ToolListChangedRequest) {
        provider.Invalidate()        // 异步刷新，下次 Tools() 重拉
    },
})
session, _ := client.Connect(ctx, transport, nil)
```

`provider.Invalidate()` 只把 cache 置 nil，不主动重拉——避免在通知 handler 内部阻塞回调线程。

### 5.3 ChatClient 集成

`ChatClient` 当前的 `WithTools(...Tool)` 是**值语义**的；MCP 工具列表是动态的，需要每次请求时调 `Provider.Tools(ctx)` 注入。两条接入方式：

**方案 A（推荐，最小侵入）**：用户自己调 `provider.Tools(ctx)` 后 `WithTools(tools...)`。

```go
tools, _ := mcpProvider.Tools(ctx)
resp, _ := client.Prompt(prompt).WithTools(tools...).Call(ctx)
```

**方案 B（可选语法糖）**：在 `ClientRequest` 上加 `WithToolProvider(p ToolProvider)`，构建请求时调用。需要给 `chat` 包加一个新接口：

```go
// core/model/chat/tool.go 新增
type ToolProvider interface {
    Tools(ctx context.Context) ([]Tool, error)
}
```

**v1 阶段先走 A**——保持 `core/` 完全不动；如果用户反馈强烈，v2 再考虑 B。

### 5.4 完整的发现/调用时序

```
[启动]
ChatClient                Provider               *mcp.ClientSession           MCP Server
    │                         │                            │                       │
    │                         │ NewClient + Connect(ctx)   │                       │
    │                         ├───────────────────────────►│                       │
    │                         │                            │  initialize           │
    │                         │                            ├──────────────────────►│
    │                         │                            │◄──────────────────────┤
    │                         │                            │  notifications/init.. │
    │                         │                            ├──────────────────────►│
    │                         │                            │                       │
[首次请求]                                                                          │
    │ Prompt().WithTools(p.Tools(ctx)...)                  │                       │
    │                         │ refresh: Tools(ctx, nil)   │                       │
    │                         ├───────────────────────────►│  tools/list           │
    │                         │                            ├──────────────────────►│
    │                         │                            │◄──────────────────────┤
    │                         │◄───────────────────────────┤  []*mcp.Tool          │
    │                         │ adapt + cache.Store        │                       │
    │◄────────────────────────┤ []chat.Tool                │                       │
    │ Call(ctx)                                            │                       │
    │  └─ ToolMiddleware ─ LLM 选 toolX                    │                       │
    │     └─ mcpClientTool.Call(ctx, args)                 │                       │
    │                         │ session.CallTool(ctx,...)  │                       │
    │                         │                            │  tools/call           │
    │                         │                            ├──────────────────────►│
    │                         │                            │◄──────────────────────┤
    │                         │                            │  CallToolResult       │
    │  └─ flatten content → string → 回灌 LLM              │                       │
    │                                                                              │
[Server 端 tool 列表变更]                                                          │
    │                         │  ToolListChangedHandler ◄──┤  notifications/...    │
    │                         │  cache.Store(nil)          │                       │
    │                         │                            │                       │
[下一次请求]                                                                       │
    │ ... cache 为 nil → 自动 refresh                      │                       │
```

---

## 6. Schema 桥

### 6.1 现状

| 端 | Schema 形态 |
|---|-----------|
| lynx `chat.ToolDefinition.InputSchema` | `string`（draft 2020-12 兼容 JSON Schema） |
| go-sdk `mcp.Tool.InputSchema` | `any`（可塞 `*jsonschema.Schema`、`map[string]any`、`json.RawMessage`） |
| 网络上传输 | 永远是 JSON 对象 |

### 6.2 转换规则

**消费方向（MCP Tool → chat.ToolDefinition）**：

```go
func toLynxDefinition(t *mcp.Tool, prefixedName string) (chat.ToolDefinition, error) {
    var schema string
    switch s := t.InputSchema.(type) {
    case string:
        schema = s
    case json.RawMessage:
        schema = string(s)
    default:
        b, err := json.Marshal(s)
        if err != nil {
            return chat.ToolDefinition{}, err
        }
        schema = string(b)
    }
    return chat.ToolDefinition{
        Name:        prefixedName,
        Description: t.Description,
        InputSchema: schema,
    }, nil
}
```

**暴露方向（chat.ToolDefinition → mcp.Tool）**：直接 `Tool.InputSchema = json.RawMessage(def.InputSchema)`。go-sdk 在 `Server.AddTool`（低层版）下要求 `InputSchema != nil` 且 schema `"type":"object"`，lynx 现有工具（如 `fakeweatherquery`）已满足。

### 6.3 关键约束：必须用 `Server.AddTool` 低层方法注册

lynx 的 Tool 拿到的 schema 是**手写/反射出来的成品字符串**——不能用 go-sdk 的泛型 `mcp.AddTool[In, Out]`，因为后者会用 `In` 类型反射重新生成 schema，覆盖掉 lynx 已有的。所以反向适配必须走 `*mcp.Server.AddTool(*Tool, ToolHandler)` 这条不带泛型的低层 API。

---

## 7. Content / Media 映射

go-sdk `CallToolResult.Content` 是 `[]mcp.Content`，元素可能是 `*TextContent` / `*ImageContent` / `*AudioContent` / `*EmbeddedResource` / `*ResourceLink`。lynx `CallableTool.Call` 只能返回 `(string, error)`。

### 7.1 默认扁平化策略

```go
// mcp/client/content.go (sketch)

func flattenContent(items []mcp.Content) (string, error) {
    if len(items) == 0 {
        return "", nil
    }
    if len(items) == 1 {
        if t, ok := items[0].(*mcp.TextContent); ok {
            return t.Text, nil
        }
    }
    // 多元素或非纯文本：序列化为 JSON 数组，由 LLM 端理解
    b, err := json.Marshal(items)
    return string(b), err
}
```

**约定**：
- 纯一段 `TextContent` → 原样返回（最常见，LLM 可直接读）
- 其它情况 → 整个 Content 数组 JSON 序列化（保留 type discriminator）

### 7.2 富内容回灌：扩展点（v2 再做）

如果上层希望把 ImageContent 真正注入到下一轮 LLM 输入而不是当字符串：
- 在 `ToolReturn` / `ToolMessage` 增加 `Media []*media.Media` 字段（影响 `core/model/chat/message.go`），
- `mcpClientTool.Call` 用 `*ToolReturnExtra` 旁路返回，
- 模型适配层（`models/anthropic/chat.go` 等）按 MIME 序列化。

**v1 不做**——保持 `core/` 不动。等用户提需求再说。

### 7.3 错误内容文本

`IsError=true` 时同样 flatten：

```go
func flattenContentText(items []mcp.Content) string {
    for _, c := range items {
        if t, ok := c.(*mcp.TextContent); ok {
            return t.Text
        }
    }
    return "MCP tool returned isError=true"
}
```

---

## 8. 命名冲突与多 Server

### 8.1 默认策略

```go
type NamingStrategy interface {
    PrefixedName(source Source, tool *mcp.Tool) string
}

type defaultNaming struct{}
func (defaultNaming) PrefixedName(s Source, t *mcp.Tool) string {
    if s.Name == "" { return t.Name }
    return s.Name + "_" + t.Name
}
```

- 默认：`<source.Name>_<tool.Name>`，源名为空时退化为原名。
- **不截断长度**——LLM 实测能吃 100+ 字符的 tool 名，Spring AI 的 64 字符截断是历史遗留。
- 用户可注入自定义 `NamingStrategy`（例如 hash 后缀、白名单跳过前缀）。

### 8.2 Fail-Fast 重名校验

`Provider.refresh` 末尾：

```go
func validateUniqueNames(tools []chat.Tool) error {
    seen := map[string]bool{}
    for _, t := range tools {
        n := t.Definition().Name
        if seen[n] {
            return fmt.Errorf("duplicate tool name after prefixing: %s", n)
        }
        seen[n] = true
    }
    return nil
}
```

与 Spring AI 一致：宁可启动失败也不静默覆盖。

---

## 9. ToolContext / Meta 透传

### 9.1 lynx 端

`chat.Request.Params map[string]any` 是约定的"穿透"通道（`request.go:175-184`）。中间件不会动它，但能读取（已用于传 userId、sessionId、A2A correlation 等场景）。

### 9.2 MCP 端

`mcp.CallToolParams.Meta` 是协议级 `_meta` 字段，go-sdk 接受 `map[string]any`。

### 9.3 转换

```go
type MetaConverter interface {
    FromContext(ctx context.Context) mcp.Meta  // 调 MCP 时
}

// 默认实现：从 ctx value 读 chat.Request.Params 同对象，黑名单过滤后透传
type defaultMetaConverter struct {
    blockedKeys []string  // 默认含 "lynx.internal.*" 等敏感前缀
}
```

**注意**：lynx 当前没有把 `Request.Params` 自动注入 ctx 的机制——需要 ToolMiddleware 之前的某个中间件先 `ctx = context.WithValue(ctx, paramsKey, req.Params)`，或者在 mcpClientTool 内部约定 caller 自己塞 ctx。v1 推荐前者，由 MCP module 提供一个轻量 `WithRequestParams` middleware。

---

## 10. Server 侧：暴露 lynx Tool

### 10.1 接口

```go
// mcp/server/adapter.go (sketch)

func RegisterTools(s *mcp.Server, tools ...chat.Tool) error {
    for _, t := range tools {
        callable, ok := t.(chat.CallableTool)
        if !ok {
            return fmt.Errorf("tool %s is not callable, cannot expose as MCP tool", t.Definition().Name)
        }
        if err := registerOne(s, callable); err != nil {
            return err
        }
    }
    return nil
}

func registerOne(s *mcp.Server, t chat.CallableTool) error {
    def := t.Definition()
    mcpTool := &mcp.Tool{
        Name:        def.Name,
        Description: def.Description,
        InputSchema: json.RawMessage(def.InputSchema),
    }
    s.AddTool(mcpTool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        argsBytes, _ := json.Marshal(req.Params.Arguments) // Arguments 是 json.RawMessage
        out, err := t.Call(ctx, string(argsBytes))
        if err != nil {
            return &mcp.CallToolResult{
                Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
                IsError: true,
            }, nil
        }
        return &mcp.CallToolResult{
            Content: []mcp.Content{&mcp.TextContent{Text: out}},
        }, nil
    })
    return nil
}
```

### 10.2 用户侧使用

```go
srv := mcp.NewServer(&mcp.Implementation{Name: "lynx-bridge", Version: "0.1.0"}, nil)

err := mcpserver.RegisterTools(srv,
    fakeweatherquery.New(),
    /* 其它 lynx CallableTool */
)
if err != nil { log.Fatal(err) }

// stdio
_ = srv.Run(ctx, &mcp.StdioTransport{})

// 或 streamable HTTP
http.Handle("/mcp", mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil))
```

**关键**：错误用 `IsError=true` + `TextContent` 上报，**不**返回 Go `error` ——后者在低层 `AddTool` 路径下会被当成 protocol-level 错误（直接 JSON-RPC error），破坏 LLM 端语义。

---

## 11. 反向能力（v2 规划，v1 不实现）

| MCP 反向能力 | 用例 | lynx 接入构想 |
|------------|-----|-------------|
| Sampling (`CreateMessage`) | MCP server 让 client 端 LLM 帮它生成内容 | 在 `mcp/server` 注入一个 `chat.Model`，handler 内部转调 |
| Elicitation | server 询问用户结构化字段 | 走 lynx Agent / UI 集成（参考 `doc/agent`） |
| Roots | client 告诉 server 文件系统根目录 | 用 lynx 安全沙箱配置项注入 |
| Logging | server 推日志给 client | 桥到 `slog` |
| Progress | 长任务进度 | 桥到 lynx observation span events |

这些都通过 `mcp.ServerOptions` / `mcp.ClientOptions` 的 handler 字段实现，**不走 Tool 抽象**，所以与 §3-§10 完全正交。等 v1 集成跑稳后再做。

---

## 12. 关键取舍

| 取舍 | 选 | 理由 |
|-----|----|-----|
| MCP SDK | 官方 `modelcontextprotocol/go-sdk` | 官方维护、协议跟进及时、有 streamable HTTP；mcp-go 的优势是社区时间长，但官方 SDK 已 GA |
| 包合并 | `mcp/client/` + `mcp/server/` 两子包 | 与 go-sdk 单包风格不冲突；client/server 关注点切割明确 |
| Provider 注入 | 用户手动 `provider.Tools(ctx)` 后 `WithTools` | 不动 `core/`；`ChatClient` 加 `WithToolProvider` 是后续事 |
| Schema 引擎 | lynx 既有的 invopop/jsonschema **不动** | 两库都吐 draft 2020-12 兼容 JSON，字符串可互通；切到 google/jsonschema-go 收益小、成本大 |
| 错误传播 | `IsError=true` → `error` | 一致于 lynx 风格（错误就是 `error`），`ToolMiddleware` 不需特判 |
| Content 扁平化 | 单 TextContent 直传，其他 JSON 序列化 | 简单可读；富内容延后 |
| 缓存刷新 | event-driven invalidate，下次 lazy refresh | 避免在 SDK 回调内做阻塞 IO |
| 命名前缀 | `<src>_<tool>`，不截断 | 简单；用户可换 |
| Sync vs Async | 仅同步 | Go 无 reactive，多此一举 |

---

## 13. 依赖成本

| 档位 | 装什么 | 影响 |
|-----|-------|-----|
| 不用 MCP | 只装 `core/`、`models/`... | `go.sum` 完全无 MCP 痕迹 |
| 用 MCP client | + `mcp/client` | `mcp/go.sum` 拉 go-sdk + jsonschema-go + uritemplate；不影响 core |
| 用 MCP server | + `mcp/server` | 同上；额外用 streamable HTTP 不需新依赖（`net/http` 即可） |
| 用 OAuth client | + 直接 import `go-sdk/auth` | 拉 oauth2、jwt；按需 |

go-sdk v1.5.0 要 **Go 1.25+**——lynx 当前 `go.mod` 已声明 `go 1.25.0`（`core/go.mod:3` / `pkg/go.mod:3`），匹配。

---

## 14. 落地路线图

| 里程碑 | 内容 | 验收 |
|-------|-----|------|
| **M1：骨架** | 新建 `mcp/` module；`mcp/client/tool.go` + 单 server 适配；最小 stdio demo（连 `examples/server/hello`） | 用 lynx ChatClient 跑通一次远程 tool 调用 |
| **M2：Provider** | `Provider` + 缓存 + list_changed handler；多 source 命名前缀；fail-fast 重名校验 | 双 server 双工具不冲突；server 增删工具自动刷新 |
| **M3：Server 侧** | `mcp/server/RegisterTools`；stdio + Streamable HTTP 双 transport demo | `fakeweatherquery` 暴露成 MCP server，被 Claude Desktop 调到 |
| **M4：Meta + Content** | `MetaConverter` + content flatten + 黑名单 key | `Request.Params` 透传到 server；多元素 content 可读 |
| **M5：可观测** | 在 `mcpClientTool.Call` 加 OTel span（`lynx.tool.name`、`mcp.server`、`mcp.tool.is_error`），与 `OBSERVABILITY_DESIGN.md` 一致 | trace 中能看到一次 tool 调用的 MCP 段 |
| **M6（延后）：富内容/反向能力** | Image/Resource 回灌；sampling、elicitation、roots、logging | 由具体业务需求驱动 |

---

## 15. 待决问题

1. **`ToolMessage` 是否需要扩展富内容字段？** 影响 §7.2。短期不动，先用 JSON 字符串。
2. **Provider 是否要做 health-check 机制？** 一个 source down 是否拖垮整个 `Tools()` 调用？建议：单 source 失败仅 warn + 跳过，不阻塞其它 source（M2 实现）。
3. **`WithToolProvider` 该不该进 `core/model/chat/client.go`？** 目前倾向不进。等 MCP 用户多了再回头看。
4. **Server 侧能否复用 lynx 的 `ToolRegistry`？** 即 `RegisterAllFromRegistry(s *mcp.Server, r *chat.ToolRegistry)` 一行接入——可加，是 M3 的便捷 API。
5. **测试策略**：go-sdk 提供 `mcp.NewInMemoryTransports()` 可造一对内存 client/server，端到端测试无需起进程或 HTTP server——`mcp/client` 和 `mcp/server` 的所有单测都走这个模式。
