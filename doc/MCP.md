# Lynx MCP 桥接

> 在 Lynx 现有 `chat.Tool` / `ToolMiddleware` 体系之上接入 [Model Context Protocol](https://modelcontextprotocol.io/)，让 Lynx 既能消费外部 MCP server 暴露的工具，也能把本地工具反向暴露成 MCP server。
>
> **底层 SDK**：[`github.com/modelcontextprotocol/go-sdk@v1.5.0`](https://github.com/modelcontextprotocol/go-sdk)（已 GA）
>
> **状态**：v1 已落地（commit `5069ead`），单包 `mcp/` ~750 行 + 4 个测试文件。

---

## 1. 目标与非目标

### 目标

| # | 目标 |
|---|-----|
| G1 | **零侵入**：不改 `core/model/chat/tool.go` 任何一行；MCP 通过实现 `chat.Tool` 加入工具体系 |
| G2 | **双向**：消费（client）+ 暴露（server）两条路径都支持 |
| G3 | **依赖隔离**：MCP SDK 重依赖（`google/jsonschema-go`、`uritemplate`、`oauth2`...）走独立 module，不污染 `core/` |
| G4 | **协议透明**：上层（`chat.Client`、`ToolMiddleware`）感知不到 MCP 存在，外部 MCP 工具与本地工具无差异 |
| G5 | **生命周期托管**：`tools/list_changed` 通知自动刷新工具缓存 |
| G6 | **多 server 安全**：多个 MCP server 同名工具走前缀策略，重名时 fail-fast |

### 非目标

- ❌ **不重新实现 MCP 协议**：完全依赖 go-sdk
- ❌ **不抽象 Sync/Async 双 API**：Go 单一同步路径就够（go-sdk 内部本就异步）
- ❌ **不做 OAuth 工作流**：`auth` / `oauthex` 包按需直接暴露给用户
- ❌ **不内置 transport**：`stdio` / `streamable HTTP` / `SSE` 全部直接复用 go-sdk 类型
- ❌ **不做注解扫描**：Go 无 runtime annotation；显式注册更可控

---

## 2. 模块布局

```
mcp/                            # 独立顶层 module
├── go.mod                      # require core, modelcontextprotocol/go-sdk@v1.5.0
├── doc.go                      # 包级文档
├── tool.go                     # 单 tool 桥接：Tool / ToolConfig / NamingFunc / MetaFunc / WithMeta
├── provider.go                 # 批量发现 + 缓存：Provider / ProviderConfig / Source
├── server.go                   # 反向：RegisterTools(*sdkmcp.Server, ...chat.Tool)
├── errors.go                   # *ToolCallError 类型
├── tool_test.go
├── provider_test.go
└── server_test.go
```

**为什么独立 module**：与 `models/`、`vectorstores/` 同档——重依赖外置。go-sdk v1.5.0 拉 `google/jsonschema-go`、`segmentio/encoding`、`yosida95/uritemplate/v3`、`golang-jwt/jwt/v5`、`golang.org/x/oauth2`，全部进 `mcp/go.sum`，不污染 `core/`。

**为什么单包**：与 go-sdk 单包风格一致，5 个生产 .go 即可覆盖双向 hot path，client/server 关注点切割明确。

---

## 3. 公共 API 速览

### 3.1 消费侧（client）

```go
// 单个 MCP tool 的桥接：实现 chat.Tool
type Tool struct { /* unexported */ }

type ToolConfig struct {
    Session      *sdkmcp.ClientSession  // required
    Descriptor   *sdkmcp.Tool           // required（远端 tool 元数据）
    PrefixedName string                 // optional（默认走 NamingFunc）
    Metadata     chat.ToolMetadata
    MetaFunc     MetaFunc
}

func NewTool(cfg ToolConfig) (*Tool, error)
```

```go
// 多源批量发现 + 缓存
type Provider struct { /* unexported */ }

type ProviderConfig struct {
    Sources  []Source
    Naming   NamingFunc
    MetaFunc MetaFunc
}

type Source struct {
    Name    string                 // 用户给的连接名（前缀生成依据）
    Session *sdkmcp.ClientSession
}

type NamingFunc func(sourceName string, tool *sdkmcp.Tool) string

var DefaultNaming NamingFunc = func(name string, t *sdkmcp.Tool) string {
    if name == "" { return t.Name }
    return name + "_" + t.Name
}

func NewProvider(cfg ProviderConfig) (*Provider, error)
func (p *Provider) Tools(ctx context.Context) ([]chat.Tool, error)
func (p *Provider) Invalidate()
func (p *Provider) OnToolListChanged(context.Context, *sdkmcp.ToolListChangedRequest)
```

### 3.2 暴露侧（server）

```go
// 把 lynx Tool 注册到 mcp.Server
func RegisterTools(server *sdkmcp.Server, tools ...chat.Tool) error
```

### 3.3 元数据透传

```go
type MetaFunc func(ctx context.Context) sdkmcp.Meta

// 默认 opt-in：用户必须显式 WithMeta 才会透传
func WithMeta(ctx context.Context, meta sdkmcp.Meta) context.Context
func MetaFromContext(ctx context.Context) sdkmcp.Meta  // 兼容 MetaFunc 签名
```

### 3.4 错误类型

```go
type ToolCallError struct {
    ToolName string
    Message  string
}
func (e *ToolCallError) Error() string { ... }

// 用户判别
var tcErr *mcp.ToolCallError
if errors.As(err, &tcErr) {
    // 远端 tool 失败，message 可喂回 LLM 自我纠正
}
```

---

## 4. 一次远程工具调用的时序

```
[启动]
ChatClient            Provider             *mcp.ClientSession           MCP Server
    │                     │                       │                         │
    │                     │ NewClient + Connect   │                         │
    │                     ├──────────────────────►│  initialize             │
    │                     │                       ├────────────────────────►│
    │                     │                       │◄────────────────────────┤
    │                     │                       │  notifications/init...  │
    │                     │                       ├────────────────────────►│
    │                     │                       │                         │
[首次请求]
    │ Prompt().WithTools(p.Tools(ctx)...)         │                         │
    │                     │ refresh: Tools(ctx)   │                         │
    │                     ├──────────────────────►│  tools/list             │
    │                     │                       ├────────────────────────►│
    │                     │                       │◄────────────────────────┤
    │                     │◄──────────────────────┤  []*mcp.Tool            │
    │                     │ adapt + cache.Store   │                         │
    │◄────────────────────┤ []chat.Tool           │                         │
    │ Call(ctx)           │                       │                         │
    │  └ ToolMiddleware: LLM 选 toolX             │                         │
    │     └ Tool.Call(ctx, args)                  │                         │
    │                     │ session.CallTool      │                         │
    │                     │                       │  tools/call             │
    │                     │                       ├────────────────────────►│
    │                     │                       │◄────────────────────────┤
    │                     │                       │  CallToolResult         │
    │  └ flatten content → string → 回灌 LLM      │                         │
    │                                                                       │
[Server 端 tool 列表变更]
    │                     │ OnToolListChanged ◄───┤  notifications/...      │
    │                     │ cache.Store(nil)      │                         │
[下一次请求]                                                                  │
    │ ... cache 为 nil → 自动 refresh             │                         │
```

要点：
- `Provider.OnToolListChanged` 是直接绑到 `sdkmcp.ClientOptions.ToolListChangedHandler` 的 method value——比 Spring 的 ApplicationEvent 总线更轻
- `Tool.Call` 用**原始**远端 tool 名（不是前缀名），前缀名只对 LLM 可见

---

## 5. ChatClient 集成

`WithTools(...Tool)` 是值语义；MCP 工具列表是动态的，需要每次请求时调 `Provider.Tools(ctx)` 注入。两条路径：

```go
// 推荐：用户自调 Provider.Tools(ctx)
tools, _ := mcpProvider.Tools(ctx)
resp, _ := client.Prompt(prompt).WithTools(tools...).Call(ctx)

// 替代：手动单 tool 注入
tool, _ := mcp.NewTool(mcp.ToolConfig{Session: ss, Descriptor: t})
client.WithTools(tool)
```

> Spring AI 走 `ToolCallbackProvider` 接口由 ChatClient 拉取；Lynx 不在 `core/` 引入 `ToolProvider` 接口，**保持核心不动**——用户手动 `provider.Tools(ctx)` 一行。

---

## 6. 关键设计取舍

| 取舍 | 选 | 理由 |
|-----|----|-----|
| MCP SDK | 官方 `modelcontextprotocol/go-sdk` v1.5.0 | 官方维护、协议跟进及时、有 Streamable HTTP；mcp-go 社区时间长但官方 SDK 已 GA |
| 包合并 | 单包 `mcp/`，不再分 `client/` / `server/` 子包 | 与 go-sdk 单包风格一致；750 行可一目了然 |
| Provider 注入 | 用户手动 `provider.Tools(ctx)` 后 `WithTools` | 不动 `core/` |
| Schema 引擎 | Lynx 既有的 `invopop/jsonschema` **不动** | 两库都吐 draft 2020-12 兼容 JSON，字符串可互通；切到 google/jsonschema-go 收益小、成本大 |
| 错误传播 | 远端 `IsError=true` → `*ToolCallError`；其他 → `fmt.Errorf("...")` | type-safe error，符合 Go 风格（参考 `*net.OpError` / `*fs.PathError`）|
| Content 扁平化 | 单 `TextContent` 直传，多元素或非 text 走 JSON 序列化 | 99% 实际场景就是一段 text；JSON 化整个 Content 数组让 LLM 多解析一层 |
| 缓存刷新 | event-driven invalidate，下次 lazy refresh | 避免在 SDK 回调内做阻塞 IO |
| 命名前缀 | `<src>_<tool>`，不截断 | 现代模型 100+ 字符 tool 名无问题；用户可换 `NamingFunc` |
| Sync vs Async | 仅同步 | Go 无 reactive，多此一举 |
| 元数据透传 | `WithMeta` opt-in，默认不透传 | 安全默认；Spring AI 走全量倒 + 黑名单是 Java 历史包袱 |

---

## 7. Schema 桥

| 端 | Schema 形态 |
|---|-----------|
| Lynx `chat.ToolDefinition.InputSchema` | `string`（draft 2020-12 兼容 JSON Schema）|
| go-sdk `mcp.Tool.InputSchema` | `any`（可塞 `*jsonschema.Schema`、`map[string]any`、`json.RawMessage`）|
| 网络上传输 | 永远是 JSON 对象 |

**消费方向**（MCP Tool → chat.ToolDefinition）：

```go
switch s := t.InputSchema.(type) {
case string:                  schema = s
case json.RawMessage:         schema = string(s)
default:                      b, _ := json.Marshal(s); schema = string(b)
}
```

**暴露方向**（chat.ToolDefinition → mcp.Tool）：直接 `Tool.InputSchema = json.RawMessage(def.InputSchema)`。**必须走 `Server.AddTool` 低层方法**（不带泛型版），因为 lynx tool 已自带 schema 字符串，泛型 `mcp.AddTool[In, Out]` 会用 `In` 反射重新生成覆盖掉。

---

## 8. Content / Media 映射

go-sdk `CallToolResult.Content` 是 `[]mcp.Content`，元素可能是 `*TextContent` / `*ImageContent` / `*AudioContent` / `*EmbeddedResource` / `*ResourceLink`。`Tool.Call` 只能返回 `(string, error)`。

**默认扁平化**：
- 纯一段 `TextContent` → 原样返回（最常见，LLM 可直接读）
- 其它情况 → 整个 Content 数组 JSON 序列化（保留 type discriminator）

**富内容回灌（Image/Resource）**——v1 不做。如果上层希望把 ImageContent 真正注入到下一轮 LLM 输入而不是当字符串：在 `ToolReturn` / `ToolMessage` 增加 `Media []*media.Media` 字段（影响 `core/model/chat/message.go`），mcpClientTool 旁路返回，模型适配层按 MIME 序列化。等用户提需求再说。

---

## 9. 命名冲突与多 Server

```go
var DefaultNaming NamingFunc = func(name string, t *sdkmcp.Tool) string {
    if name == "" { return t.Name }
    return name + "_" + t.Name
}

// Provider.refresh 末尾 fail-fast
func validateUniqueNames(tools []chat.Tool) error {
    seen := map[string]bool{}
    for _, t := range tools {
        n := t.Definition().Name
        if seen[n] { return fmt.Errorf("duplicate tool name: %s", n) }
        seen[n] = true
    }
    return nil
}
```

与 Spring AI 一致：宁可启动失败也不静默覆盖。Spring AI 用 `alt_<n>_` 自动改写——哪种更安全各有道理。

---

## 10. 服务端：暴露本地 Tool

```go
srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "lynx-bridge", Version: "0.1.0"}, nil)

if err := mcp.RegisterTools(srv,
    fakeweatherquery.New(),
    /* 其他 lynx Tool */
); err != nil {
    log.Fatal(err)
}

// stdio
_ = srv.Run(ctx, &sdkmcp.StdioTransport{})

// 或 streamable HTTP
http.Handle("/mcp", sdkmcp.NewStreamableHTTPHandler(
    func(*http.Request) *sdkmcp.Server { return srv }, nil,
))
```

Handler 内部把 `tool.Call(ctx, args)` 的 error 转成 `IsError=true` + `TextContent`——**不**返回 Go `error`，否则在低层 `AddTool` 路径会被当成 protocol-level 错误（直接 JSON-RPC error），破坏 LLM 端语义。

---

## 11. 测试支持：`InMemoryTransports` 优势

go-sdk 提供 `mcp.NewInMemoryTransports()` 可造一对内存 client/server，端到端测试无需起进程或 HTTP server。`mcp/provider_test.go` / `tool_test.go` / `server_test.go` 全部用这套：

```go
serverT, clientT := sdkmcp.NewInMemoryTransports()

srv := sdkmcp.NewServer(impl, nil)
_ = mcp.RegisterTools(srv, myTools()...)
ss, _ := srv.Connect(ctx, serverT, nil)
defer ss.Close()

cli := sdkmcp.NewClient(impl, nil)
cs, _ := cli.Connect(ctx, clientT, nil)
defer cs.Close()
```

零外部进程、零端口监听。Spring AI 测 MCP 必须起进程或 Testcontainers——**Lynx 在这一项明确胜出**。

---

## 12. 与 Spring AI MCP 的对比

> 基线：Spring AI HEAD（依赖 `io.modelcontextprotocol:mcp:2.0.0-M2`，starters 已重组到 `starters/` 子目录）；Lynx HEAD `4da6a37`。

### 12.1 鸟瞰

| 维度 | Spring AI | Lynx |
|-----|----------|------|
| 模块拆分 | `mcp/{common,transport/{webmvc,webflux,stateless}}`、`mcp-annotations`、`auto-configurations/mcp-{client,server}-{common,*}`、`spring-ai-spring-boot-starters/spring-ai-starter-mcp-*` —— **~12 个 Maven module** | 单包 `mcp/`，5 个生产 .go |
| 代码量 | 估算 8-10k LoC（不含 SDK） | ~750 LoC |
| SDK 关系 | 依赖 `mcp:2.0.0-M2`（pre-release）；客户端/服务端再包一层 Spring Bean | 依赖 `go-sdk@v1.5.0`（已 GA）；薄壳 |
| 装配模型 | Spring Boot autoconfig + `@ConditionalOnProperty` + `application.yml` | 用户手工 `NewProvider(cfg)` / `RegisterTools(srv, tools...)`；纯库 |
| API 风格 | Builder（`SyncMcpToolCallback.builder()`）+ 注解（`@McpTool`）| Config struct + `Validate()`（`ToolConfig{...}` / `ProviderConfig{...}`）|
| 双轨模型 | Sync + Async（每组件并行两份类，Async 内部 `.block()`）| 仅 Sync |
| 错误传播 | unchecked `ToolExecutionException(toolDef, cause)` | `error` 返回值 + `*ToolCallError{ToolName, Message}` |
| 注解扫描 | 13 种注解（tool/resource/prompt/sampling/elicit/progress/logging/...）| 无 |
| 反向能力 | `ToolContext` 注入 `McpSyncServerExchange`，本地工具可反向 sampling/elicit/progress/ping | 不支持（v2 规划）|
| Customizer | `McpClientCustomizer<B>` / `McpServerCustomizer<B>` | 无（用户直接配 `*sdkmcp.ClientOptions`）|
| Transport | `WebMvcSse` / `WebFluxSse` / `WebMvcStreamable` / `WebFluxStreamable` / `Stateless` 五种 Spring 集成 | 透传 SDK 的 `stdio` / `SSEHandler` / `NewStreamableHTTPHandler` |
| 测试支持 | 无内建夹具，需起进程或 mock server | 直接 `NewInMemoryTransports()`，零外部进程 |
| 命名前缀 | `DefaultMcpToolNamePrefixGenerator`：缩写 + 字符过滤 + 64 字符截断 + 跨连接幂等去重 | `DefaultNaming`：`<src>_<tool>` 一行 |
| 元数据透传 | 默认全量倒 ToolContext（黑名单 `"exchange"`）| `WithMeta` opt-in（默认不透传）|
| 工具过滤 | `McpToolFilter` 接口 | 无（v2 候选）|

### 12.2 单 tool 桥接：核心调用流程并排

**Spring AI** (`SyncMcpToolCallback.java`)：

```java
Map<String, Object> arguments = ModelOptionsUtils.jsonToMap(toolCallInput);
try {
    var mcpMeta = toolContextToMcpMetaConverter.convert(toolContext);
    var request = CallToolRequest.builder()
        .name(this.tool.name()).arguments(arguments).meta(mcpMeta).build();
    response = this.mcpClient.callTool(request);
} catch (Exception ex) {
    throw new ToolExecutionException(this.getToolDefinition(), ex);
}
if (response.isError()) {
    throw new ToolExecutionException(...);
}
return ModelOptionsUtils.toJsonString(response.content());   // 一律 JSON 序列化
```

**Lynx** (`mcp/tool.go`)：

```go
args, err := decodeArguments(arguments)              // ""→{} 容错
params := &sdkmcp.CallToolParams{Name: t.descriptor.Name, Arguments: args}
if t.metaFunc != nil {
    if meta := t.metaFunc(ctx); len(meta) > 0 { params.Meta = meta }
}
result, err := t.session.CallTool(ctx, params)
if err != nil { return "", fmt.Errorf("call tool %q: %w", t.descriptor.Name, err) }
if result.IsError {
    return "", &ToolCallError{ToolName: t.descriptor.Name, Message: firstTextOrFallback(...)}
}
return flattenContent(result.Content)                // 单 TextContent 直传，多元素才 JSON
```

### 12.3 Provider 缓存

两侧都是双检锁 lazy fetch + 失效后下次再拉，原语不同（`volatile + ReentrantLock + volatile List` vs `atomic.Pointer + sync.Mutex`）但语义一致。两者都顺序遍历多 client/source——慢源会拖慢整体 fan-out，是双方共同潜在短板。

### 12.4 失效触发：Spring 事件总线 vs Go 方法值

**Spring AI**：`SyncMcpToolCallbackProvider implements ApplicationListener<McpToolsChangedEvent>`，配套 `McpAsyncToolsChangeEventEmmiter` 把 SDK 通知翻译成 Spring 事件发布。

**Lynx**：

```go
// mcp/provider.go
func (p *Provider) OnToolListChanged(context.Context, *sdkmcp.ToolListChangedRequest) {
    p.Invalidate()
}

// 用户接入：method value 直接赋值
sdkmcp.NewClient(impl, &sdkmcp.ClientOptions{
    ToolListChangedHandler: provider.OnToolListChanged,
})
```

省掉中间 publisher/listener，无 IoC 容器开销。代价：lynx 上层若有多个组件想关心 `tools/list_changed`，目前只能各自接 SDK 钩子。

### 12.5 服务端反向能力：Spring AI 真实差距

Spring AI 通过 `ToolContext` 注入 `McpSyncServerExchange`：

```java
@McpTool(name="research")
public String doResearch(String query, ToolContext ctx) {
    Optional<McpSyncServerExchange> exchange = McpToolUtils.getMcpExchange(ctx);
    var samplingResult = exchange.get().createMessage(...);  // 反向 LLM 调用
    return samplingResult.content();
}
```

通过 `exchange` 句柄，server 端工具可反向调用 client：

| 能力 | 方法 |
|-----|------|
| Sampling | `exchange.createMessage(req)` |
| Elicitation | `exchange.elicit(req)` |
| Progress | `exchange.progressNotification(...)` |
| Logging | `exchange.log(...)` |
| Ping | `exchange.ping()` |

**Lynx 现状**：`mcp/server.go` 的 handler 完全没注入 server 句柄。`chat.Tool.Call(ctx, args)` 签名固定，没有 server session 通道。

**v2 设计草图**（不需改 chat 抽象）：

```go
type serverSessionKey struct{}

// mcp/server.go handler 内部
ctx = context.WithValue(ctx, serverSessionKey{}, req.Session)

// 暴露读取函数
func ServerSessionFromContext(ctx context.Context) *sdkmcp.ServerSession { ... }

// 用户工具
func (t *MyTool) Call(ctx context.Context, args string) (string, error) {
    if sess := mcp.ServerSessionFromContext(ctx); sess != nil {
        result, _ := sess.CreateMessage(ctx, &sdkmcp.CreateMessageParams{...})
    }
}
```

与 `WithMeta` 风格一致。**v2 第一项优先做**。

### 12.6 注解扫描

Spring AI 13 种注解 + 12 个 Provider 扫描器：`@McpTool` / `@McpResource` / `@McpResourceTemplate` / `@McpPrompt` / `@McpComplete` / `@McpSampling` / `@McpElicitation` / `@McpProgress` / `@McpLogging` / `@McpToolListChanged` / `@McpResourceListChanged` / `@McpPromptListChanged` 等，加上参数级 `@McpToolParam` / `@McpProgressToken` / `@McpMeta`。

`McpJsonSchemaGenerator` 反射方法签名自动生成 draft-2020-12 schema，跳过基础设施类型（`McpSyncServerExchange` / `McpTransportContext` / `@McpProgressToken` 标注的参数等）。

**Lynx**：用户显式注册：

```go
tool, _ := chat.NewTool(chat.ToolDefinition{Name: "weather_today", ...}, ...)
mcp.RegisterTools(srv, tool)
```

不是「好坏」对比，是哲学差异：Spring AI 走声明式框架路线，Lynx 走显式编程式库路线。Go 也没有运行时 annotation——做类似效果必须 codegen 或 reflect-on-tags，复杂度激增、收益不大。

---

## 13. v2 待补能力清单（按优先级）

| 能力 | 接入点 | 优先级 |
|-----|-------|-------|
| **Server 端 `exchange` 反向调用**（sampling/elicit/progress/ping/logging）| `ctx` 上挂 `*sdkmcp.ServerSession` + `ServerSessionFromContext(ctx)` | 🔴 高 |
| `ToolFilter` | `ProviderConfig.Filter func(Source, *sdkmcp.Tool) bool` | 🟠 中 |
| OTel 埋点 | `Tool.Call` / `makeServerHandler` 包 span，对齐 OTel GenAI 规范 | 🟠 中 |
| 多源并行 fan-out | `ProviderConfig.ParallelFetch bool`，多 source 时用 `errgroup` | 🟡 低（看实测）|
| Schema 主动校验 | `stringSchemaToAny` 增强（当前仅 `json.Valid` 粗校验）| 🟡 低 |
| 富内容回灌（Image/Resource）| 扩展 `chat.ToolReturn` schema | 🟡 低 |
| Resource 订阅 | 不在 chat tool 范畴；需新抽象 | 🟡 低 |

### 故意不做的清单（保持克制）

| 特性 | 不做的理由 |
|-----|----------|
| Spring Boot autoconfig | Lynx 是库非框架；用户在自己的 main 里 5 行装配 |
| `@McpTool` 等注解扫描 | Go 无 runtime annotation；codegen 复杂度收益不平衡 |
| 双轨 Sync/Async | SDK 同步签名内部已异步；Go 无 reactor |
| 命名前缀的字符过滤+64 截断+跨连接幂等 | 现代 LLM 已不限 64 字符；行为更可预测 |
| `McpClientCustomizer` 链式接口 | 直接配 SDK `ClientOptions` 够清楚 |
| `ToolContextToMcpMetaConverter` 全量倒 + 黑名单 | 默认不外发更安全；用户用 `WithMeta` 显式注入 |

---

## 14. 一句话定档

> Lynx mcp 桥接 ~750 LoC 顶 Spring AI ~8-10k LoC 的关键能力，靠的是不假设 IoC 容器、不做注解魔法、不双轨 Sync/Async——三个负空间设计的复利。Spring AI 在 server 端反向能力（`exchange`）和注解化框架上整体仍更全；Lynx 在「消费 + 暴露 chat tool」两条 hot path 上的 API 表面、构造复杂度、测试便利性明显更轻。v2 第一项是补齐 `ServerSessionFromContext`，让本地工具能反向 LLM 调用——这是 agentic 编排的基石。

---

**版本基线**：lynx HEAD `4da6a37`（含 commit `5069ead`）/ spring-ai 当前主干（mcp 2.0.0-M2，starters 重组）/ go-sdk v1.5.0 GA。
