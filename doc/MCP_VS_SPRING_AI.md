# lynx/mcp vs Spring AI MCP 实现差异

> 目标：把 lynx 当前 `mcp/` 包的实现和 Spring AI 同名模块逐项对照，给出**做了什么、没做什么、为什么**。
> 关联文档：`doc/MCP_DESIGN.md`（设计稿）、`mcp/DESIGN.md`（lynx 实现+使用）、`doc/SPRING_AI_COMPARISON.md`（lynx 整体对标 Spring AI 的总文档）。

代码引用：
- Spring AI 路径以仓库根为基准 (`spring-ai/mcp/...`)
- lynx 路径以仓库根为基准 (`mcp/...`)

---

## 1. 鸟瞰对比

| 维度 | Spring AI | lynx |
|-----|----------|------|
| 包组织 | `mcp/common`、`mcp/transport/{webmvc,webflux}`、`mcp/mcp-annotations`、`auto-configurations/mcp-{client,server}-*`，**多模块** | `mcp/`（单包） |
| SDK 关系 | 依赖 `io.modelcontextprotocol:mcp:2.0.0-M2`，包装层中等厚 | 依赖 `github.com/modelcontextprotocol/go-sdk@v1.5.0`，包装层薄 |
| API 风格 | Builder 模式（`SyncMcpToolCallback.builder()` 等） | Config struct（`*ToolConfig` / `*ProviderConfig`） |
| Sync/Async | 双套并行类（SyncXxx + AsyncXxx） | 仅同步 |
| 错误传播 | unchecked exception (`ToolExecutionException`) | `error` 返回值 |
| 缓存原语 | `volatile boolean` + `ReentrantLock` + `volatile List` | `atomic.Pointer[[]T]` + `sync.Mutex` |
| 命名前缀 | 复杂（缩写 + 字符过滤 + 64 字符截断） | 极简（`<src>_<tool>`） |
| 工具过滤 | `McpToolFilter` 接口 | 无（v1 暂不做） |
| 注解扫描 | `@McpTool` / `@McpResource` / `@McpPrompt` + autoconfig | 无 |
| 反向能力 | sampling/elicitation/roots 注解化抽象 | 无（v2 规划） |
| Transport | webmvc / webflux 两套 Spring 集成 | 直接复用 SDK 的 stdio / SSE / streamable HTTP，零包装 |

接下来按桥接组件分维度展开。

---

## 2. 客户端桥接：单 tool 包装

### 2.1 类型映射

| 角色 | Spring AI | lynx |
|-----|----------|------|
| 单 tool wrapper | `SyncMcpToolCallback implements ToolCallback`<br>`mcp/common/src/main/java/org/springframework/ai/mcp/SyncMcpToolCallback.java:46` | `Tool` (实现 `chat.CallableTool`)<br>`mcp/tool.go` |
| 异步版本 | `AsyncMcpToolCallback`（独立类，几乎全文复制） | — |

### 2.2 调用流程对比

**Spring AI (`SyncMcpToolCallback.call`)**, 行 109-146：

```java
if (!StringUtils.hasText(toolCallInput)) {
    toolCallInput = "{}"; // 容错
}
Map<String, Object> arguments = ModelOptionsUtils.jsonToMap(toolCallInput);

try {
    var mcpMeta = toolContext != null
        ? this.toolContextToMcpMetaConverter.convert(toolContext) : null;
    var request = CallToolRequest.builder()
        .name(this.tool.name())          // 用原始名而非 prefixedToolName
        .arguments(arguments)
        .meta(mcpMeta)
        .build();
    response = this.mcpClient.callTool(request);
} catch (Exception ex) {
    throw new ToolExecutionException(this.getToolDefinition(), ex);
}

if (response.isError() != null && response.isError()) {
    throw new ToolExecutionException(this.getToolDefinition(),
        new IllegalStateException("Error calling tool: " + response.content()));
}
return ModelOptionsUtils.toJsonString(response.content());  // 返回 JSON 数组
```

**lynx (`Tool.Call`)**, `mcp/tool.go`：

```go
args, err := decodeArguments(arguments)        // ""→{} 容错 + json.Unmarshal
if err != nil {
    return "", fmt.Errorf("decode arguments for tool %q: %w", t.descriptor.Name, err)
}

params := &sdkmcp.CallToolParams{
    Name:      t.descriptor.Name,              // 用原始名,与 Spring AI 一致
    Arguments: args,
}
if meta := t.metaConverter.FromContext(ctx); len(meta) > 0 {
    params.Meta = meta
}

result, err := t.session.CallTool(ctx, params)
if err != nil {
    return "", fmt.Errorf("call tool %q: %w", t.descriptor.Name, err)
}
if result.IsError {
    text := firstTextOrFallback(result.Content, "...")
    return "", fmt.Errorf("tool %q failed: %s", t.descriptor.Name, text)
}
return flattenContent(result.Content)          // 单 TextContent 直传，否则 JSON
```

**几个关键差异**

| 点 | Spring AI | lynx |
|---|----------|------|
| 错误的语言机制 | throw `ToolExecutionException`（unchecked） | return `error` |
| 远端 IsError 的程序化判别 | `catch ToolExecutionException` + cause 判断 | `errors.As(err, &tcErr)`（同时判别 + 取字段） |
| 错误结构化数据 | 异常上挂 `getToolDefinition()` | `ToolCallError{ToolName, Message}` 字段 |
| 多 Content 处理 | 一律 `toJsonString(response.content())` —— 始终 JSON | 单 `TextContent` 透传，多元素或非文本才 JSON |
| 元数据来源 | 调用方传 `ToolContext` 由 converter 抽取 | 调用方在 `ctx` 上 `WithMeta`，`MetaFromContext` 读出 |
| 空入参 | 日志 warn 后 `"{}"` | 静默 `"{}"` |

**为什么 lynx 不抛错而是返回 error**：Go 没有 unchecked exception，且 `chat.ToolMiddleware` 已经按 `error` 协议处理传播，沿用即可，不需要新建 `ToolExecutionException` 类型。

**为什么 lynx 单 TextContent 直传**：`TextContent` 是 99% 的实际场景；直接 JSON 序列化整个数组会让 LLM 多解析一层 `[{"type":"text","text":"..."}]` 模板，浪费 token。Spring AI 的统一 JSON 是为方便（也为 ImageContent 等其他类型）。

### 2.3 错误信息可读性与可分支处理

| 场景 | Spring AI 异常 | lynx 错误 |
|-----|---------------|----------|
| 远端 IsError | `ToolExecutionException("Error calling tool: " + content)` | `*ToolCallError{ToolName, Message}`（用 `errors.As` 判别+取字段） |
| RPC 失败 | `ToolExecutionException` cause = original ex | `fmt.Errorf("call tool %q: %w", name, err)` |
| 参数 unmarshal 失败 | （Spring 在更外层处理） | `fmt.Errorf("decode arguments for tool %q: %w", name, err)` |

lynx 在每条错误上都强制带工具名（方便 grep），并且**对最常见的"远端工具自身失败"提供哨兵 + 结构化错误**，让上层可以这样区分处理：

```go
out, err := tool.Call(ctx, args)
var tcErr *lynxmcp.ToolCallError
switch {
case errors.As(err, &tcErr):
    // 远端 tool 自己拒绝；可访问 tcErr.ToolName / tcErr.Message
case err != nil:
    // 传输/参数错误；通常应重试或上报基础设施告警
}
```

Spring AI 统一抛 `ToolExecutionException`，依赖 cause 链判断；lynx 用 `errors.As` 解出 `*ToolCallError` 是 Go 标准库习惯用法（参考 `*net.OpError`、`*fs.PathError`）。

---

## 3. 客户端桥接：批量发现（Provider）

### 3.1 类型映射

| 角色 | Spring AI | lynx |
|-----|----------|------|
| 多源聚合 | `SyncMcpToolCallbackProvider`<br>`mcp/common/.../SyncMcpToolCallbackProvider.java:43` | `Provider`<br>`mcp/provider.go` |
| 异步版本 | `AsyncMcpToolCallbackProvider`（独立类） | — |
| 多 client 容器 | `List<McpSyncClient>` | `[]Source` (含 Name + Session) |
| 工具过滤 | `McpToolFilter`（`(connectionInfo, tool) → boolean`） | 无 |
| 命名策略 | `McpToolNamePrefixGenerator` 接口 + `DefaultMcpToolNamePrefixGenerator` | `NamingFunc` 函数类型 |

### 3.2 缓存与刷新原语

**Spring AI** (`SyncMcpToolCallbackProvider.java:53-155`)：
```java
private volatile boolean invalidateCache = true;
private volatile List<ToolCallback> cachedToolCallbacks = List.of();
private final Lock lock = new ReentrantLock();

public ToolCallback[] getToolCallbacks() {
    if (this.invalidateCache) {
        this.lock.lock();
        try {
            if (this.invalidateCache) {                    // 双检
                this.cachedToolCallbacks = ...;            // 拉一遍
                this.validateToolCallbacks(...);
                this.invalidateCache = false;
            }
        } finally { this.lock.unlock(); }
    }
    return this.cachedToolCallbacks.toArray(new ToolCallback[0]);
}
```

**lynx** (`mcp/provider.go`)：
```go
cache     atomic.Pointer[[]chat.Tool] // nil = empty/stale
refreshMu sync.Mutex                  // serializes refresh

func (p *Provider) Tools(ctx context.Context) ([]chat.Tool, error) {
    if cached := p.cache.Load(); cached != nil {
        return *cached, nil
    }
    return p.refresh(ctx)
}

func (p *Provider) refresh(ctx context.Context) ([]chat.Tool, error) {
    p.refreshMu.Lock()
    defer p.refreshMu.Unlock()
    if cached := p.cache.Load(); cached != nil { return *cached, nil }  // 双检
    // 拉一遍 + validateUniqueNames + cache.Store
}
```

**等价性**：两者都做了 double-checked locking + 失效后 lazy 拉取 + fail-fast 重名校验。原语不同但语义一致。

### 3.3 失效触发：事件 vs 方法值

**Spring AI**：通过 Spring 事件总线
```java
public class SyncMcpToolCallbackProvider implements
        ToolCallbackProvider, ApplicationListener<McpToolsChangedEvent> {
    @Override
    public void onApplicationEvent(McpToolsChangedEvent event) {
        this.invalidateCache();
    }
}
```
配套 `McpAsyncToolsChangeEventEmmiter` 把 SDK 的 `toolsChangeConsumer(...)` 翻译成 `McpToolsChangedEvent`。需要 Spring 的 `ApplicationEventPublisher`，是典型 Spring-only 设计。

**lynx**：直接用 SDK 的 `ClientOptions.ToolListChangedHandler` 字段，方法值即 handler：
```go
sdkmcp.NewClient(impl, &sdkmcp.ClientOptions{
    ToolListChangedHandler: provider.OnToolListChanged,  // method value
})
```
无中间事件总线。`OnToolListChanged` 内只 `Invalidate()`，不阻塞 SDK 的 dispatcher 协程。

**取舍**：Spring AI 的方案能让多个组件共同响应同一个事件（Spring 事件多订阅天然支持）；lynx 的方案是单一回调，但因为只有 Provider 关心这个事件，没有损失。

### 3.4 命名前缀复杂度

**Spring AI** (`McpToolUtils.java:87-130`)：
```java
public static String prefixedToolName(String prefix, @Nullable String title, String toolName) {
    String input = shorten(format(prefix));      // 客户端名 -> 缩写
    if (!StringUtils.isEmpty(title)) {
        input = input + "_" + format(title);     // 连接名 -> 字符过滤
    }
    input = input + "_" + format(toolName);
    if (input.length() > 64) {                   // 截尾
        input = input.substring(input.length() - 64);
    }
    return input;
}

// format: 删除非字母数字/_/-，把 - 改成 _
// shorten: 取每个 _ 分段的首字母小写
```

举例：`prefix="my_app", title="Weather Service", toolName="getForecast"`
- `shorten(format("my_app"))` = `"m_a"`
- 拼上 title: `"m_a_WeatherService"`
- 拼上 tool: `"m_a_WeatherService_getForecast"`

**lynx** (`mcp/naming.go`)：
```go
type NamingFunc func(sourceName string, tool *sdkmcp.Tool) string

var DefaultNaming NamingFunc = func(sourceName string, tool *sdkmcp.Tool) string {
    if sourceName == "" {
        return tool.Name
    }
    return sourceName + "_" + tool.Name
}
```

举例：`sourceName="weather", tool.Name="getForecast"` → `"weather_getForecast"`，就这一行。

**为什么 lynx 这么简单**：
- 64 字符限制是 OpenAI tool name 历史上限，新模型 (Anthropic、OpenAI 新规) 100+ 没问题；不主动截断更安全。
- 字符过滤会丢失中文/特殊符号；lynx 默认信任用户给的 sourceName 是合法的。
- 用户随时可换一个 `NamingFunc`，需要复杂规则自己实现，不强加默认。

---

## 4. 元数据透传

### 4.1 Spring AI

`ToolContext`（一个 `Map<String, Object>`）由 ChatModel 一路传递到 `ToolCallback.call(input, toolContext)`。`ToolContextToMcpMetaConverter.defaultConverter()` 默认实现 (`ToolContextToMcpMetaConverter.java:46-60`):

```java
return toolContext.getContext()
    .entrySet().stream()
    .filter(e -> !"exchange".equals(e.getKey()) && e.getValue() != null)
    .collect(toMap(...));
```

特殊 key `"exchange"`（= `TOOL_CONTEXT_MCP_EXCHANGE_KEY`）被排除，因为 server 端把 `McpSyncServerExchange` 塞进同一个 ToolContext 用来反向访问 session。

### 4.2 lynx

走 `context.Context` 而不是单独的 `ToolContext` 参数（Go 风格）：

```go
// 注入
ctx = lynxmcp.WithMeta(ctx, sdkmcp.Meta{"userId": "u-42"})

// 抽取（在 Tool.Call 内）
if meta := t.metaConverter.FromContext(ctx); len(meta) > 0 {
    params.Meta = meta
}
```

默认 `MetaFunc=nil` —— 不透传。要启用就把 `MetaFromContext` 直接当函数值赋上：

```go
&lynxmcp.ProviderConfig{MetaFunc: lynxmcp.MetaFromContext}
```

**对比**：

| 点 | Spring AI | lynx |
|---|----------|------|
| 数据源 | `ToolContext.getContext()` 整个 Map | `ctx.Value(metaContextKey{})` 单独维度 |
| 默认行为 | 全量拷贝（去掉 "exchange" + null） | 不拷贝（必须显式 `WithMeta` + 显式启用 converter） |
| 安全风险 | 高：业务可能往 ToolContext 塞敏感字段，被静默发到 MCP server | 低：透传是 opt-in 的 |
| 复杂度 | 一层默认黑名单 + null 过滤 | 调用方自己决定 |

lynx 的默认更保守是有意为之 —— Go 没有 ToolContext 的等价"约定 map"，把 `chat.Request.Params` 全量倒给远端 server 不是合理默认。

---

## 5. 服务端桥接：`ToolCallback` → MCP server

### 5.1 类型映射

| 角色 | Spring AI | lynx |
|-----|----------|------|
| 注册入口 | `McpToolUtils.toSyncToolSpecification(toolCallback)` 等多个静态方法 | `RegisterTools(server, tools...)`（搭配 `registry.All()...` 即可批量） |
| Server 实例 | 用户自己 `McpSyncServer` 装配 + Spring autoconfig | 用户自己 `*sdkmcp.Server` 装配（`mcp.NewServer`） |
| Handler 闭包 | `BiFunction<exchange, request, CallToolResult>` | `func(ctx, *CallToolRequest) (*CallToolResult, error)` |

### 5.2 Handler 行为对比

**Spring AI** (`McpToolUtils.java:246-285`)：
```java
return new SharedSyncToolSpecification(tool, (exchangeOrContext, request) -> {
    try {
        String callResult = toolCallback.call(
            ModelOptionsUtils.toJsonString(request.arguments()),
            new ToolContext(Map.of(TOOL_CONTEXT_MCP_EXCHANGE_KEY, exchangeOrContext)));
        if (mimeType != null && mimeType.toString().startsWith("image")) {
            return CallToolResult.builder()
                .content(List.of(new ImageContent(annotations, callResult, mimeType.toString())))
                .isError(false).build();
        }
        return CallToolResult.builder()
            .content(List.of(new TextContent(callResult)))
            .isError(false).build();
    } catch (Exception e) {
        return CallToolResult.builder()
            .content(List.of(new TextContent(e.getMessage())))
            .isError(true).build();
    }
});
```

**lynx** (`mcp/server.go`)：
```go
func makeServerHandler(tool chat.CallableTool) sdkmcp.ToolHandler {
    return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
        args := string(req.Params.Arguments)
        if args == "" { args = emptyObjectArguments }

        out, err := tool.Call(ctx, args)
        if err != nil {
            return errorResult(err), nil          // IsError=true + TextContent
        }
        return textResult(out), nil               // 单 TextContent
    }
}
```

**差异**：

| 点 | Spring AI | lynx |
|---|----------|------|
| MIME 判定 | 支持 `image/*` 直接产 ImageContent | 永远走 TextContent，富内容延后到 v2 |
| Server-side context 注入 | `new ToolContext(Map.of("exchange", exchange))` —— 让本地 `ToolCallback` 可反向回 server session 发 sampling/elicitation | 没有等价机制 |
| 错误转换 | `catch Exception` → `IsError=true` | `if err != nil` → `IsError=true`（语义相同） |

**`exchange` 的实际用途**：Spring AI 的 `ToolCallback` 实现可以从 `ToolContext` 取出 `McpSyncServerExchange`，调用 `exchange.createMessage(...)` 让远端 client 帮自己做 LLM 推理（sampling），或 `exchange.elicit(...)` 询问用户输入。lynx 当前没暴露这条反向通道。

### 5.3 Schema 处理

| 阶段 | Spring AI | lynx |
|-----|----------|------|
| ToolDefinition 生成 schema | `JsonSchemaUtils.ensureValidInputSchema(...)` —— 校验 + 修正 | 直接传字符串，校验只在 `RegisterTools` 路径上做 `json.Valid` |
| client → server 传 schema | `ModelOptionsUtils.jsonToObject(string, JsonSchema.class)` —— 反序列化成 SDK 类型 | `json.RawMessage(string)` —— 不反序列化，让 SDK 自己识别 |
| Server 端 schema 一致性校验 | 用 SDK 的 `JsonSchemaUtils` | 仅 `json.Valid()` 粗校验 |

lynx 偏向"传字节、不解释"，Spring AI 偏向"反序列化 + 主动校验"。两条路径在合规 JSON Schema 下行为一致；非合规 schema 时 lynx 会更晚才报错（client 调用时 SDK 才发现不合法）。这是 lynx 一个**已知薄弱点**，需要时再补。

---

## 6. 配置与扩展机制

| 维度 | Spring AI | lynx |
|-----|----------|------|
| 注入方式 | Spring Bean + `@ConditionalOnProperty` + autoconfig 自动装配 | 用户手工 `lynxmcp.NewProvider(...)` |
| 配置载体 | `application.yml` (`spring.ai.mcp.client.*`) | Go struct (`*ProviderConfig`) |
| 扩展点 | `McpToolFilter`、`McpToolNamePrefixGenerator`、`ToolContextToMcpMetaConverter`、`McpClientCustomizer` | `NamingFunc`、`MetaFunc`（皆为函数类型，无接口/适配器层） |
| Tool filter | 有：`(connectionInfo, tool) → boolean` 在 listTools 时挑选 | 无 |
| 注解扫描 | 有：`@McpTool` 等 + 包扫描 + bean 注册 | 无 |

**lynx 暂未做 `McpToolFilter`** 的考虑：
- Go 没有 Spring 的 bean 自动汇聚；filter 通常意味着"在配置文件里写规则"，但 lynx 没有标准化的配置体系
- 用户想过滤可在 `NamingFunc` 内返回 `""` 然后在 `Provider` 之外手动剔除（待补）—— 或者直接在他们自己的 `chat.ToolRegistry` 上做白/黑名单

---

## 7. Sync vs Async

**Spring AI**：

每个核心组件都有两份并行实现：
- `SyncMcpToolCallback` ↔ `AsyncMcpToolCallback`
- `SyncMcpToolCallbackProvider` ↔ `AsyncMcpToolCallbackProvider`
- `McpToolUtils.toSyncToolSpecification` ↔ `toAsyncToolSpecification`

`AsyncMcpToolCallback.call(...)` 内部用 `mcpClient.callTool(req).block()` —— 即把 reactive 转回阻塞 —— 然后 `contextWrite(...)` 把 reactor context 透传。`toAsyncToolSpecification` 直接复用 sync 版的 handler 然后包成 `Mono.fromCallable(...).subscribeOn(Schedulers.boundedElastic())`。

实质上 async 实现是 sync 实现的薄封装，但因为 `McpAsyncClient` vs `McpSyncClient` 类型不同，必须独立类。

**lynx**：

只有同步路径。SDK 的 `*sdkmcp.ClientSession.CallTool` 本身就同步签名（`(*CallToolResult, error)`），内部已是异步实现；lynx 不需要再做一层。如果用户希望并发调用多个 tool，goroutine 自己起即可。

**对比影响**：

| 维度 | Spring AI Sync | Spring AI Async | lynx |
|-----|---------------|----------------|------|
| 接口数量 | 1×3 = 3 个（callback + provider + utils） | 同上独立 3 个 | 1×3 = 3 个，无重复 |
| 用户心智 | 启动期决定一次 | 同上，不能混用 | 单一路径 |
| 反应式语境 | — | 透传 reactor context | — |

---

## 8. 反向能力（Server → Client 调用）

| 能力 | Spring AI | lynx |
|-----|----------|------|
| Sampling (`createMessage`) | `@McpSampling` 注解 + `SyncMcpSamplingProvider` 扫描；本地 ToolCallback 通过 `ToolContext` 拿到 `McpSyncServerExchange` 后 `exchange.createMessage(...)` | 无 |
| Elicitation | `@McpElicit` 注解 + `McpElicitationProvider` | 无 |
| Roots | `@McpRoots` + customizer | 无 |
| Logging | `@McpLogging` 桥接到 SLF4J | 无 |
| Progress | `exchange.notifyProgress(...)` | 无 |
| List changed (server 主动) | `@McpToolListChanged` 等 changed-provider | 无 |

**lynx 现状**：这些能力 SDK 层都已经支持（通过 `ServerOptions.XxxHandler` / `*ServerSession` 上的方法），用户可以**直接用 SDK API**，不必通过 `lynx/mcp` 包；`lynx/mcp` 只在"把 lynx tool 暴露成 MCP server tool"这一条 hot path 上做适配。

如果未来要把这些能力也桥接进 lynx 的 agent / chat 体系（例如让 server 端的本地工具反过来发起 LLM 调用），需要扩展 `chat.CallableTool.Call` 的签名或在 `ctx` 上挂 server session —— 等 v2 业务驱动。

---

## 9. Transport

| 维度 | Spring AI | lynx |
|-----|----------|------|
| 自带 transport | `mcp/transport/mcp-spring-webmvc`（`WebMvcSseServerTransportProvider` 等）、`mcp/transport/mcp-spring-webflux` | 无（一律复用 SDK 的 `mcp.StdioTransport` / `mcp.SSEHandler` / `mcp.NewStreamableHTTPHandler`） |
| 自定义 transport | 实现 `McpServerTransportProvider` + Spring Bean | 实现 SDK 的 `mcp.Transport` 接口 |
| Stdio | SDK 默认提供 | SDK 默认提供 |

go-sdk 的 transport 都在 `mcp` 包内，不需要再做 Spring 那种"按 web stack 划分子模块"的工作。lynx 这层完全透明。

---

## 10. 注解扫描 / Auto-configuration

Spring AI `mcp-annotations` 模块对每种 MCP 能力有专门注解：`@McpTool`、`@McpResource`、`@McpResourceTemplate`、`@McpPrompt`、`@McpComplete`、`@McpSampling`、`@McpElicit`、`@McpToolListChanged`、`@McpResourceListChanged` 等。配套 `McpServerAnnotationScannerAutoConfiguration` 扫描所有 Spring Bean、收集被注解的方法、按种类生成 `McpServerFeatures.SyncToolSpecification` 等结构、注入 `McpSyncServer`。

**lynx 不做注解扫描**：
- Go 没有运行时 annotation；要做类似效果必须 codegen 或 reflect-on-tags
- lynx 现有 `chat.NewTool(definition, metadata, execFunc)` 就是显式注册，简单可控
- 用户希望批量暴露时直接 `RegisterTools(server, registry.All()...)` 一行搞定 —— 与 Spring AI 注解扫描的最终效果等价，只是写法不同

---

## 11. 独有能力小结

### Spring AI 有，lynx 没有

| 能力 | 用例 | lynx 替代方案 |
|-----|-----|--------------|
| `McpToolFilter` | 启动期按 client 信息屏蔽部分 tool | 暂未做；用户可在 `NamingFunc` 里做协议外过滤，或自己 wrap |
| 注解 `@McpTool` 等 | 注解化暴露本地工具 | `chat.NewTool(...)` 显式 + `RegisterTools(...)` |
| Sampling/Elicitation 抽象 | 服务端反向 LLM 调用 | 直接用 SDK API |
| `ToolExecutionException` 携带 ToolDefinition | 异常里挂结构化信息 | error 文本带工具名（粒度更粗） |
| ImageContent 自动编码 | 文件型工具直接出图 | 走 TextContent，业务层自己拼 |
| Spring Boot autoconfig | `application.yml` 一键启用 | 用户手工写 5 行 Go 代码（更可控） |

### lynx 有，Spring AI 没有（或简化掉）

| 能力 | 价值 |
|-----|------|
| 单包合并（与 SDK 风格一致） | 减少多模块/多包导航成本，符合 `net/http`、SDK 自身的组织风格 |
| Config struct 替代 Builder | Go 风格更地道，零值即默认，nil 即缺省 |
| `OnToolListChanged` 直接做方法值 | 不需要事件总线；适合无 IoC 容器的环境 |
| 极简命名前缀 | 不主动截断、不主动过滤字符 —— 行为更可预测 |
| 元数据 opt-in | 默认不外发任何 ctx 数据，安全默认 |
| `flattenContent` 单 Text 直传 | 节省 LLM token，常见路径更便宜 |

---

## 12. 一句话总结

Spring AI 的 MCP 模块是**重度集成 + 注解 + 双轨 sync/async** 的"框架风"实现，在 Spring 生态里非常顺滑，但跨进程/跨语言时要消化大量 Spring 概念。

lynx 的 `mcp/` 模块是**单包 + 显式装配 + 仅同步**的"库风"实现，故意省掉所有非必要扩展点（filter、autoconfig、注解、富内容、反向通道），只把"消费 MCP tool"和"暴露 lynx tool"两条 hot path 做成最薄的桥。需要更复杂能力时直接用 SDK API，避免在 lynx 这层堆砌抽象。

两者的差异基本就是 **Spring 框架文化** vs **Go stdlib 文化**的落差。
