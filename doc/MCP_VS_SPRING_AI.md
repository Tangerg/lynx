# lynx/mcp vs Spring AI MCP 深度对比

> 目标：把 lynx 当前 `mcp/` 包（合并到 main 后的最终状态）和 Spring AI MCP 全套（`mcp/` + `mcp-annotations/` + `auto-configurations/mcp/`）逐项对照，挖出**做了什么、没做什么、为什么、什么时候补**。
>
> 关联：`mcp/DESIGN.md`（lynx 实现+使用）、`doc/MCP_DESIGN.md`（设计稿）。
>
> 范围：
> - lynx：`mcp/`（5 个生产 .go + 4 个测试，约 750/720 行；依赖 `modelcontextprotocol/go-sdk@v1.5.0`）
> - Spring AI：`spring-ai/mcp/{common,transport,mcp-annotations}` + `spring-ai/auto-configurations/mcp/*`（依赖 `io.modelcontextprotocol:mcp:2.0.0-M2`）

代码引用：
- Spring AI 路径以仓库根为基准（`spring-ai/...`）
- lynx 路径以仓库根为基准（`mcp/...`）

---

## 0. 鸟瞰

| 维度 | Spring AI | lynx |
|-----|----------|------|
| 模块拆分 | `mcp/{common,transport/{webmvc,webflux,stateless}}`、`mcp/mcp-annotations`、`auto-configurations/mcp-{client,server}-{common,*}`、`spring-ai-spring-boot-starters/spring-ai-starter-mcp-*`，**~12 个 Maven module** | 单包 `mcp/`，5 个生产 .go |
| 代码量 | 估算 8-10k LoC（不含 SDK） | 753 LoC（含 errors.go + DESIGN.md 之外的 5 个 .go）|
| SDK 关系 | 依赖 `mcp:2.0.0-M2`；客户端/服务端再包一层 Spring Bean | 依赖 `go-sdk@v1.5.0`；薄到不能再薄的适配 |
| 装配模型 | Spring Boot autoconfig + `@ConditionalOnProperty` + `application.yml` | 用户手工 `NewProvider(cfg)` / `RegisterTools(srv, tools...)`；纯库 |
| API 风格 | Builder（`SyncMcpToolCallback.builder()`）+ 注解（`@McpTool`） | Config struct + `Validate()`（`ToolConfig{...}` / `ProviderConfig{...}`）|
| 双轨模型 | Sync + Async（每组件并行两份类，Async 内部 `.block()`） | 仅 Sync（SDK 已经是异步实现，再包反应式无收益）|
| 错误传播 | unchecked `ToolExecutionException(toolDef, cause)` | `error` 返回值 + `*ToolCallError{ToolName, Message}` |
| 工具发现 | `SyncMcpToolCallbackProvider`（多 client + filter + 命名生成器 + meta 转换器）| `Provider`（多 source + naming + metaFunc）|
| 命名前缀 | `DefaultMcpToolNamePrefixGenerator`：缩写 + 字符过滤 + 64 字符截断 + 跨连接幂等去重 | `DefaultNaming`：`<src>_<tool>` 一行；fail-fast 重名校验 |
| 工具过滤 | `McpToolFilter` 接口（`(ConnInfo, Tool)→bool`）| 无（v1）|
| 元数据透传 | `ToolContext` 全量倒（默认排除 `"exchange"` key） | `WithMeta` opt-in（默认不透传）|
| 注解扫描 | 13 种注解（tool/resource/prompt/sampling/elicit/progress/logging/...） | 无 |
| 反向能力 | `ToolContext` 中注入 `McpSyncServerExchange`，本地工具可反向 sampling / elicit / progress / ping | 不支持（v2 规划）|
| Customizer | `McpClientCustomizer<B>` / `McpServerCustomizer<B>` 接口，链式装配 | 无（用户直接配 `*sdkmcp.ClientOptions`）|
| Transport | `WebMvcSse` / `WebFluxSse` / `WebMvcStreamable` / `WebFluxStreamable` / `Stateless` 五种 Spring 集成 | 透传 SDK 的 `stdio` / `SSEHandler` / `NewStreamableHTTPHandler` |
| 测试支持 | 无内建夹具，需起进程或 mock server | 直接用 SDK `NewInMemoryTransports()`，单元测试零外部进程 |
| Schema 引擎 | `McpJsonSchemaGenerator`（基于 victools/jsonschema-generator + `SpringAiSchemaModule`） | lynx 工具自带字符串 schema；无生成器，仅做 `json.Valid` 粗校验 |

---

## 1. 客户端：单 tool 包装

### 1.1 类型映射

| 角色 | Spring AI | lynx |
|-----|----------|------|
| 单 tool wrapper | `SyncMcpToolCallback implements ToolCallback`<br>`spring-ai/mcp/common/.../SyncMcpToolCallback.java:46` | `Tool`（实现 `chat.CallableTool`）<br>`mcp/tool.go:156` |
| 异步版本 | `AsyncMcpToolCallback`（独立类，~95% 重复） | — |
| 错误结构化 | `ToolExecutionException(ToolDefinition, Throwable cause)` | `*ToolCallError{ToolName, Message}` |
| 配置入口 | `SyncMcpToolCallback.Builder` | `ToolConfig`（值类型）+ `Validate()` 就地填默认 |

### 1.2 调用流程并排

**Spring AI** (`SyncMcpToolCallback.java:103-146`)：

```java
if (!StringUtils.hasText(toolCallInput)) {
    toolCallInput = "{}";
}
Map<String, Object> arguments = ModelOptionsUtils.jsonToMap(toolCallInput);

try {
    var mcpMeta = toolContext != null
        ? this.toolContextToMcpMetaConverter.convert(toolContext) : null;
    var request = CallToolRequest.builder()
        .name(this.tool.name())                      // 原始名
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
return ModelOptionsUtils.toJsonString(response.content());   // 一律 JSON 序列化
```

**lynx** (`mcp/tool.go:255`)：

```go
args, err := decodeArguments(arguments)              // ""→{} 容错
if err != nil {
    return "", fmt.Errorf("decode arguments for tool %q: %w", t.descriptor.Name, err)
}

params := &sdkmcp.CallToolParams{
    Name:      t.descriptor.Name,                    // 原始名（与 Spring AI 一致）
    Arguments: args,
}
if t.metaFunc != nil {
    if meta := t.metaFunc(ctx); len(meta) > 0 {
        params.Meta = meta
    }
}

result, err := t.session.CallTool(ctx, params)
if err != nil {
    return "", fmt.Errorf("call tool %q: %w", t.descriptor.Name, err)
}
if result.IsError {
    return "", &ToolCallError{
        ToolName: t.descriptor.Name,
        Message:  firstTextOrFallback(result.Content, "..."),
    }
}
return flattenContent(result.Content)                // 单 TextContent 直传，多元素才 JSON
```

### 1.3 关键差异

| 点 | Spring AI | lynx |
|---|----------|------|
| 错误的语言机制 | unchecked `ToolExecutionException` | `error` 返回值 |
| 远端 IsError 程序化判别 | `catch ToolExecutionException` 后看 cause | `errors.As(err, &tcErr)`（同时判别 + 取字段）|
| 错误结构化数据 | 异常对象上挂 `getToolDefinition()` | `ToolCallError{ToolName, Message}` 字段 |
| 多 Content 处理 | 一律 JSON 序列化整个数组 | 单 `TextContent` 直传，多元素才 JSON |
| 元数据来源 | `ToolContext` 全量 → `meta`（converter 会过滤掉 `exchange` key） | `ctx` 上 `WithMeta` opt-in；用户必须显式注入 |
| 空入参 | log warn 后 `"{}"` | 静默 `"{}"` |

**为什么 lynx 单 TextContent 直传**：99% 实际场景就是一段 text；JSON 序列化整个 `[{"type":"text","text":"..."}]` 让 LLM 多解析一层无意义模板，浪费 token。Spring AI 统一 JSON 是为了 ImageContent / EmbeddedResource 等富内容用例的一致性。

**为什么 lynx 错误用 `*ToolCallError` 而不是 sentinel**：`errors.As` 一步同时完成判别和取字段，比 `errors.Is + errors.As` 双步更紧凑（详见 `mcp/errors.go:17-30`）。Spring AI 把信息挂在异常对象 cause 链上，依赖 `instanceof` 判断不如 type-safe。

---

## 2. 客户端：批量发现 (Provider)

### 2.1 类型映射

| 角色 | Spring AI | lynx |
|-----|----------|------|
| 多源聚合 | `SyncMcpToolCallbackProvider`<br>`spring-ai/mcp/common/.../SyncMcpToolCallbackProvider.java:43` | `Provider`<br>`mcp/provider.go:66-79` |
| 异步版本 | `AsyncMcpToolCallbackProvider`（独立类）| — |
| 单源容器 | `McpSyncClient`（直接持 client）| `Source{Name, Session}`（命名 + session 配对）|
| 工具过滤 | `McpToolFilter`（接口）| 无 |
| 命名策略 | `McpToolNamePrefixGenerator`（接口）| `NamingFunc`（函数类型）|
| 元数据转换 | `ToolContextToMcpMetaConverter`（接口）| `MetaFunc`（函数类型）|

### 2.2 缓存原语并排

**Spring AI** (`SyncMcpToolCallbackProvider.java:53-155`)：

```java
private volatile boolean invalidateCache = true;
private volatile List<ToolCallback> cachedToolCallbacks = List.of();
private final Lock lock = new ReentrantLock();

public ToolCallback[] getToolCallbacks() {
    if (this.invalidateCache) {                            // 无锁读 volatile
        this.lock.lock();
        try {
            if (this.invalidateCache) {                    // double-check
                this.cachedToolCallbacks = this.mcpClients.stream()
                    .flatMap(mcpClient -> mcpClient.listTools().tools().stream()
                        .filter(tool -> this.toolFilter.test(connectionInfo(mcpClient), tool))
                        .map(tool -> SyncMcpToolCallback.builder()
                            .mcpClient(mcpClient).tool(tool)
                            .prefixedToolName(this.toolNamePrefixGenerator.prefixedToolName(...))
                            .toolContextToMcpMetaConverter(this.toolContextToMcpMetaConverter)
                            .build()))
                    .toList();
                this.validateToolCallbacks(this.cachedToolCallbacks);
                this.invalidateCache = false;
            }
        } finally { this.lock.unlock(); }
    }
    return this.cachedToolCallbacks.toArray(new ToolCallback[0]);
}
```

**lynx** (`mcp/provider.go:121-149`)：

```go
type Provider struct {
    cfg       ProviderConfig
    cache     atomic.Pointer[[]chat.Tool]
    refreshMu sync.Mutex
}

func (p *Provider) Tools(ctx context.Context) ([]chat.Tool, error) {
    if cached := p.cache.Load(); cached != nil {
        return *cached, nil
    }
    return p.refresh(ctx)
}

func (p *Provider) refresh(ctx context.Context) ([]chat.Tool, error) {
    p.refreshMu.Lock()
    defer p.refreshMu.Unlock()
    if cached := p.cache.Load(); cached != nil {           // double-check
        return *cached, nil
    }
    all := make([]chat.Tool, 0)
    for _, src := range p.cfg.Sources {                    // 顺序遍历（同 Spring AI）
        wrapped, err := p.wrapToolsFromSource(ctx, src)
        if err != nil { return nil, err }
        all = append(all, wrapped...)
    }
    if err := validateUniqueNames(all); err != nil { return nil, err }
    p.cache.Store(&all)
    return all, nil
}
```

**等价性**：双方都是双检锁 lazy fetch + 失效后下次再拉。原语不同（`volatile + ReentrantLock + volatile List` vs `atomic.Pointer + sync.Mutex`）但语义一致。两者都**顺序遍历多 client/source** —— 当某个慢 server 阻塞 listTools 时，整体 fan-out 会被拖慢。这是双方共同的潜在性能短板（见 §10）。

### 2.3 失效触发：Spring 事件总线 vs Go 方法值

**Spring AI**（`SyncMcpToolCallbackProvider.java:43, 165-167`）：

```java
public class SyncMcpToolCallbackProvider implements
        ToolCallbackProvider, ApplicationListener<McpToolsChangedEvent> {
    @Override
    public void onApplicationEvent(McpToolsChangedEvent event) {
        this.invalidateCache();
    }
}
```

配套：`McpAsyncToolsChangeEventEmmiter` 把 SDK 的 `toolsChangeConsumer(...)` 翻译成 `McpToolsChangedEvent` 发布到 `ApplicationEventPublisher`。Provider 订阅事件后翻 `invalidateCache=true`。

**lynx**（`mcp/provider.go:117-119`）：

```go
func (p *Provider) OnToolListChanged(context.Context, *sdkmcp.ToolListChangedRequest) {
    p.Invalidate()
}
```

用户直接做方法值赋值：

```go
sdkmcp.NewClient(impl, &sdkmcp.ClientOptions{
    ToolListChangedHandler: provider.OnToolListChanged,    // method value
})
```

**取舍**：
- Spring：多组件可同时订阅同一事件（Spring 事件天然多订阅）
- lynx：直接回调，省掉中间 publisher/listener，无 IoC 容器开销
- 对应到 lynx 上层若有多个组件想关心 `tools/list_changed`，目前只能各自接 SDK 钩子

### 2.4 命名前缀：复杂 vs 极简

**Spring AI** (`McpToolUtils.java:87-130` + `DefaultMcpToolNamePrefixGenerator.java:49-82`)：

```java
// McpToolUtils.prefixedToolName
String input = shorten(format(prefix));                   // 客户端名 -> 缩写
if (!StringUtils.isEmpty(title)) {
    input = input + "_" + format(title);                  // 连接名（不缩写，仅过滤字符）
}
input = input + "_" + format(toolName);
if (input.length() > 64) {                                // 64 字符上限（OpenAI 历史限制）
    input = input.substring(input.length() - 64);
}
return input;

// format: 删除非 [\p{IsHan}\p{InCJK_*}a-zA-Z0-9_-] 字符；- 改 _
// shorten: 取每个 _ 段的首字母小写

// DefaultMcpToolNamePrefixGenerator: 还做"跨连接幂等"
// 同一 (clientInfo, serverInfo, tool) 三元组保证名字稳定，
// 重复时用 alt_<n>_<name> 自动改写
```

举例：`prefix="my_app", title="Weather Service", toolName="getForecast"`
- `shorten(format("my_app"))` → `"m_a"`
- 拼上 title `"WeatherService"`（被过滤掉空格）→ `"m_a_WeatherService"`
- 拼上 tool → `"m_a_WeatherService_getForecast"`
- 长度 > 64 时截尾

**lynx** (`mcp/tool.go:30-36`)：

```go
var DefaultNaming NamingFunc = func(sourceName string, tool *sdkmcp.Tool) string {
    if sourceName == "" {
        return tool.Name
    }
    return sourceName + "_" + tool.Name
}
```

**为什么 lynx 这么简单**：
- 64 字符截断是 OpenAI 早期 tool name 上限，新模型 (Anthropic、OpenAI 新规) 100+ 已无问题；不主动截断更可预测。
- 字符过滤会丢中文/特殊符号；lynx 默认信任用户给的 sourceName 是合法的。
- 跨连接幂等去重是为了"同一 server 重连后名字不变"，但 lynx 根本不假设 connection 会重连——session 重建就重 wrap。
- 用户随时可换一个 `NamingFunc`，需要复杂规则自己实现。

冲突时 lynx 直接 `validateUniqueNames` fail-fast (`mcp/provider.go:174-185`)；Spring AI 用 `alt_<n>_` 自动改写：哪种更安全各有道理。

### 2.5 ToolFilter：Spring 有，lynx 没有

**Spring AI** (`McpToolFilter.java:30`)：

```java
public interface McpToolFilter extends BiPredicate<McpConnectionInfo, McpSchema.Tool> { }

// McpConnectionInfo.java:33
public record McpConnectionInfo(
    McpSchema.ClientCapabilities clientCapabilities,
    McpSchema.Implementation clientInfo,
    McpSchema.@Nullable InitializeResult initializeResult
) { }
```

调用时机（`SyncMcpToolCallbackProvider.java:135`）：在 `listTools` 之后、wrap 之前**客户端侧**过滤，不会影响协议（远端始终能列出全工具）。

典型用例：

```java
@Bean
McpToolFilter toolFilter() {
    return (connInfo, tool) ->
        "trusted-server".equals(connInfo.initializeResult().serverInfo().name());
}
```

**lynx 缺**：当前没有 `Filter` 钩子。临时变通：在 `NamingFunc` 内返回 `""` 或特殊前缀名后由调用方过滤（不优雅）。

**评价**：filter 是真实需求（信任域过滤、按 capability 决定是否暴露给 LLM）。建议进 v2 规划：在 `ProviderConfig` 加 `Filter func(src Source, tool *sdkmcp.Tool) bool` 字段。

---

## 3. 元数据透传

### 3.1 Spring AI: ToolContext 全量 + exchange 黑名单

`ToolContext`（一个 `Map<String, Object>`）由 ChatModel 一路传递到 `ToolCallback.call(input, toolContext)`。`ToolContextToMcpMetaConverter.defaultConverter()` (`spring-ai/mcp/common/.../ToolContextToMcpMetaConverter.java:46-59`)：

```java
return toolContext -> {
    if (toolContext == null || CollectionUtils.isEmpty(toolContext.getContext())) {
        return Map.of();
    }
    return toolContext.getContext()
        .entrySet().stream()
        .filter(e -> !"exchange".equals(e.getKey()) && e.getValue() != null)
        .collect(Collectors.toMap(Map.Entry::getKey, Map.Entry::getValue));
};
```

特殊 key `"exchange"`（`McpToolUtils.TOOL_CONTEXT_MCP_EXCHANGE_KEY`）被排除——server 端会把 `McpSyncServerExchange` 塞到同一个 ToolContext 里供本地工具反向调用 client（见 §6），但客户端发送时必须过滤掉，避免把 server 句柄序列化外发。这个细节非常关键，体现了"全量倒"模式下需要黑名单兜底。

### 3.2 lynx: WithMeta opt-in

`mcp/tool.go:53-71`：

```go
type metaContextKey struct{}

func WithMeta(ctx context.Context, meta sdkmcp.Meta) context.Context {
    if len(meta) == 0 { return ctx }
    return context.WithValue(ctx, metaContextKey{}, meta)
}

func MetaFromContext(ctx context.Context) sdkmcp.Meta {
    meta, _ := ctx.Value(metaContextKey{}).(sdkmcp.Meta)
    return meta
}
```

默认 `MetaFunc=nil` —— 不透传。要启用就显式：

```go
&lynxmcp.ProviderConfig{MetaFunc: lynxmcp.MetaFromContext}
```

### 3.3 取舍

| 维度 | Spring AI | lynx |
|-----|----------|------|
| 默认行为 | 全量拷贝（黑名单 `"exchange"` + null） | 不拷贝（必须显式启用）|
| 数据源 | `ToolContext.getContext()` 整个 Map | `ctx.Value(metaContextKey{})` 单维度 |
| 安全风险 | 较高：业务往 ToolContext 塞敏感字段会被静默外发 | 较低：opt-in |
| 复杂度 | 黑名单 + null 过滤 + exchange 兜底 | 调用方自决 |

lynx 的保守默认是有意为之 —— Go 没有 ToolContext 的"约定 map"，把 `chat.Request.Params` 全量倒给远端 server 不是合理默认。

---

## 4. 服务端桥接

### 4.1 类型映射

| 角色 | Spring AI | lynx |
|-----|----------|------|
| 注册入口 | `McpToolUtils.toSyncToolSpecification(toolCallback)` 等多个静态工厂 | `RegisterTools(server, tools...)` |
| Server 装配 | `McpServerAutoConfiguration` 自动 + Bean | 用户自己 `sdkmcp.NewServer(impl, opts)` |
| Handler 闭包 | `BiFunction<exchange, request, CallToolResult>` | `func(ctx, *CallToolRequest) (*CallToolResult, error)` |

### 4.2 Handler 行为对比

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

**lynx** (`mcp/server.go:74-86`)：

```go
func makeServerHandler(tool chat.CallableTool) sdkmcp.ToolHandler {
    return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
        args := string(req.Params.Arguments)
        if args == "" { args = "{}" }

        out, err := tool.Call(ctx, args)
        if err != nil {
            return errorResult(err), nil                   // IsError=true + TextContent
        }
        return textResult(out), nil
    }
}
```

### 4.3 关键差异

| 点 | Spring AI | lynx |
|---|----------|------|
| MIME 判定 | 支持 `image/*` 直接产 ImageContent | 永远走 TextContent；富内容延后 v2 |
| Server-side context 注入 | `new ToolContext(Map.of("exchange", exchange))` | **未注入** —— 工具拿不到 server 句柄 |
| 错误转换 | catch Exception → IsError=true | err != nil → IsError=true（语义一致）|

第二行就是 §5 要展开的重大功能差距。

### 4.4 Schema 处理

| 阶段 | Spring AI | lynx |
|-----|----------|------|
| ToolDefinition 生成 schema | `JsonSchemaUtils.ensureValidInputSchema(...)` 校验+修正 | 用户自己保证；`stringSchemaToAny` 仅 `json.Valid` 粗校验 |
| client→server 传 schema | `ModelOptionsUtils.jsonToObject(string, JsonSchema.class)` 反序列化为 SDK 类型 | `json.RawMessage(string)` 直接传，不解析 |
| Server 端 schema 一致性校验 | 用 SDK `JsonSchemaUtils` 主动验证 | 仅 `json.Valid` |

lynx 偏向"传字节、不解释"，Spring AI 偏向"反序列化 + 主动校验"。合规 schema 下行为一致；非合规 schema 时 lynx 报错时机更晚（实际 client 调用时 SDK 才发现）。这是 lynx **已知薄弱点**，需要时再补。

---

## 5. 服务端反向能力（Spring AI 真实差距）

> 这是上一版对比文档没充分挖的核心差距。lynx 在 v1 完全不支持，Spring AI 已成体系。

### 5.1 机制：ToolContext 注入 McpSyncServerExchange

`spring-ai/mcp/common/.../McpToolUtils.java:75`：

```java
public static final String TOOL_CONTEXT_MCP_EXCHANGE_KEY = "exchange";
```

每次工具被调用时，handler 闭包把 `McpSyncServerExchange`（server 端 session 句柄）塞进 `ToolContext`：

```java
new ToolContext(Map.of(TOOL_CONTEXT_MCP_EXCHANGE_KEY, exchangeOrContext))
```

用户写本地工具时可以这样反向访问 server session：

```java
@McpTool(name="research")
public String doResearch(String query, ToolContext ctx) {
    Optional<McpSyncServerExchange> exchange = McpToolUtils.getMcpExchange(ctx);
    if (exchange.isPresent()) {
        // 反向 sampling：让客户端的 LLM 帮我做摘要
        var samplingResult = exchange.get().createMessage(
            CreateMessageRequest.builder()
                .messages(List.of(new SamplingMessage(...)))
                .build()
        );
        return samplingResult.content();
    }
    return defaultBehavior(query);
}
```

### 5.2 通过 exchange 可发起的反向能力

通过 `McpSyncServerExchange` 句柄，server 端工具可以反向调用 client：

| 能力 | 方法 | 用例 |
|-----|------|------|
| Sampling (`createMessage`) | `exchange.createMessage(req)` | 工具内部需要 LLM 推理时让客户端帮忙 |
| Elicitation | `exchange.elicit(req)` | 工具询问用户结构化字段（缺失参数时）|
| Progress | `exchange.progressNotification(...)` | 长任务进度回报，配 `@McpProgress` 注解 |
| Logging | `exchange.log(...)` | 把日志推给 client 端 SLF4J 桥 |
| Ping | `exchange.ping()` | 心跳保活 |
| Roots list 通知 | `exchange.rootsListChanged()` | 文件系统根集变更 |

注意 §3 里 `ToolContextToMcpMetaConverter` 默认实现要把 `"exchange"` 黑名单——这就是为了避免 client 调用本地工具时把 exchange 句柄序列化外发。

### 5.3 lynx 现状

`mcp/server.go:74-86` 的 handler 完全没注入 server 句柄：

```go
return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
    args := string(req.Params.Arguments)
    if args == "" { args = "{}" }
    out, err := tool.Call(ctx, args)                       // 只传 args
    ...
}
```

`chat.CallableTool.Call(ctx, args string) (string, error)` 签名固定，没有 server session 通道。

### 5.4 v2 设计草图

要补齐，最小侵入做法：

1. 在 `mcp/server.go` 的 handler 内把 `req.Session` (`*sdkmcp.ServerSession`) 通过 ctx 传下去：
   ```go
   type serverSessionKey struct{}
   ctx = context.WithValue(ctx, serverSessionKey{}, req.Session)
   ```
2. 暴露读取函数 `ServerSessionFromContext(ctx) *sdkmcp.ServerSession` 让用户工具按需取出。
3. 用户工具实现内：
   ```go
   func (t *MyTool) Call(ctx context.Context, args string) (string, error) {
       if sess := lynxmcp.ServerSessionFromContext(ctx); sess != nil {
           result, _ := sess.CreateMessage(ctx, &sdkmcp.CreateMessageParams{...})
           ...
       }
   }
   ```

不需改 `chat.CallableTool` 签名，只是加一个 ctx 通道；与 `WithMeta` 风格一致。**优先级建议：v2 第一项**。

---

## 6. 注解扫描机制（Spring AI 框架化）

> Spring AI 的注解模块是完整的反射框架，13 种注解 + 12 个 Provider 扫描器。lynx 完全不做。

### 6.1 注解总览

`spring-ai/mcp/mcp-annotations/src/main/java/org/springframework/ai/mcp/annotation/` 下的方法级注解：

| 注解 | 用途 | 关联 Provider |
|------|------|--------------|
| `@McpTool` | 暴露方法为 MCP tool | `Sync/AsyncMcpToolProvider` |
| `@McpResource` / `@McpResourceTemplate` | 暴露方法为 resource / 模板 | `Sync/AsyncMcpResourceProvider` |
| `@McpPrompt` | 暴露方法为 prompt | `Sync/AsyncMcpPromptProvider` |
| `@McpComplete` | 关联 prompt/resource 的 completion | `Sync/AsyncMcpCompleteProvider` |
| `@McpSampling(clients)` | 处理 server 发起的 sampling 请求 | `Sync/AsyncMcpSamplingProvider` |
| `@McpElicitation(clients)` | 处理 elicitation 请求 | `McpElicitationProvider` |
| `@McpProgress(clients)` | 消费 progress 通知 | `Sync/AsyncMcpProgressProvider` |
| `@McpLogging(clients)` | 消费 logging 消息 | `Sync/AsyncMcpLoggingProvider` |
| `@McpToolListChanged(clients)` | 监听 server 的 tool list 变更 | `Sync/AsyncMcpToolListChangedProvider` |
| `@McpResourceListChanged` / `@McpPromptListChanged` | 类似 | 类似 |

参数级别还有 `@McpToolParam`（schema 注解）、`@McpProgressToken`（标记进度令牌入参）、`@McpMeta`（注入 meta 参数）等。

### 6.2 扫描器工作流

以 `SyncMcpToolProvider.java:48-157` 为例：

```
遍历所有 Spring Bean（toolObjects）
├─ getDeclaredMethods()
├─ method.isAnnotationPresent(McpTool.class) ?
├─ 排除 reactive 返回类型（Mono/Flux）
├─ 从 @McpTool 读 name/description/title/annotations/metaProvider/generateOutputSchema
├─ McpJsonSchemaGenerator.generateForMethodInput(method)
│  └─ 反射方法签名 → JSON Schema 字符串（draft-2020-12）
├─ 若 generateOutputSchema=true → generateFromType(returnType)
├─ 构造 SyncToolSpecification(tool, SyncMcpToolMethodCallback)
└─ 注册到 McpSyncServer
```

### 6.3 McpJsonSchemaGenerator：反射→Schema

`spring-ai/mcp/mcp-annotations/.../McpJsonSchemaGenerator.java:60-105`：

```java
private static final SchemaGenerator SUBTYPE_SCHEMA_GENERATOR;
static {
    SchemaGeneratorConfig subtypeConfig = new SchemaGeneratorConfigBuilder(
            SchemaVersion.DRAFT_2020_12, OptionPreset.PLAIN_JSON)
        .with(new JacksonModule(JacksonOption.RESPECT_JSONPROPERTY_REQUIRED))
        .with(new Swagger2Module())
        .with(new SpringAiSchemaModule())                  // 识别 @McpToolParam
        .with(Option.STANDARD_FORMATS)
        .without(Option.SCHEMA_VERSION_INDICATOR)
        .build();
    SUBTYPE_SCHEMA_GENERATOR = new SchemaGenerator(subtypeConfig);
}

private static final Map<Method, String> methodSchemaCache =
    new ConcurrentReferenceHashMap<>(256);                 // 弱引用缓存
```

特殊处理：
- 若方法签名带 `CallToolRequest` 入参且没有其他业务参，返回最小空对象 schema
- 自动跳过基础设施类型（`McpSyncServerExchange` / `McpAsyncServerExchange` / `McpSyncRequestContext` / `McpTransportContext` / `McpMeta` / `@McpProgressToken` 标注的参数）
- `@McpToolParam` / `@JsonProperty` / `@Schema` 提供 description / required 元数据

### 6.4 lynx：用户显式注册

```go
tool, _ := chat.NewTool(
    chat.ToolDefinition{
        Name:        "weather_today",
        Description: "...",
        InputSchema: `{"type":"object",...}`,              // 用户手写或用 invopop/jsonschema 反射
    },
    chat.ToolMetadata{},
    func(ctx context.Context, args string) (string, error) { ... },
)
mcp.RegisterTools(srv, tool)
```

### 6.5 评价

| 维度 | Spring AI | lynx |
|-----|----------|------|
| 注册方式 | `@McpTool` + 包扫描 + autoconfig 自动汇聚 | 显式 `RegisterTools(...)` |
| Schema 生成 | 反射方法签名自动生成（draft-2020-12）| 用户手写或自带 codegen |
| 学习曲线 | 必须懂 Spring + 多种注解语义 + schema 生成器配置 | 写一个 Go 函数即可 |
| 重命名/重构友好 | 注解依赖运行时反射，IDE 可识别 | 字符串 schema，需手动同步 |
| 适合场景 | 大型 Spring Boot 应用，工具集多且常增减 | CLI / 脚本 / 内部工具 / 测试可控 |

不是"好坏"对比，而是哲学差异：Spring AI 走声明式框架路线，lynx 走显式编程式库路线。Go 也没有运行时 annotation——要做类似效果必须 codegen 或 reflect-on-tags，复杂度激增、收益不大。

---

## 7. Auto-configuration 装配链路

### 7.1 Spring AI 的自动装配树

`spring-ai/auto-configurations/mcp/` 下的 AutoConfig 类：

```
spring-ai-autoconfigure-mcp-client-common/
├─ McpClientAutoConfiguration                    # 主装配（client 端）
│  ├─ @ConditionalOnProperty(prefix=spring.ai.mcp.client, name=enabled)
│  ├─ McpSyncClient[] / McpAsyncClient[]         # 多 client 实例
│  ├─ McpSyncClientConfigurer                    # 聚合 customizer
│  └─ CloseableMcpSyncClients                    # lifecycle 管理
├─ McpClientAnnotationScannerAutoConfiguration   # 扫描 @McpSampling/@McpProgress 等
├─ McpToolCallbackAutoConfiguration              # 创建 ToolCallbackProvider
└─ McpAsyncToolsChangeEventEmmiter               # SDK 通知 → Spring 事件

spring-ai-autoconfigure-mcp-client-httpclient/
└─ SseHttpClientTransportAutoConfiguration       # HttpClient 版 SSE transport

spring-ai-autoconfigure-mcp-client-webflux/
└─ SseWebFluxTransportAutoConfiguration          # WebFlux 版 SSE transport

spring-ai-autoconfigure-mcp-server-common/
├─ McpServerAutoConfiguration                    # 主装配（server 端）
│  ├─ @ConditionalOnProperty(prefix=spring.ai.mcp.server, name=enabled)
│  ├─ stdioServerTransport()
│  ├─ capabilitiesBuilder()
│  └─ mcpSyncServer / mcpAsyncServer
├─ McpServerAnnotationScannerAutoConfiguration   # 扫描 @McpTool/@McpResource/@McpPrompt
├─ ToolCallbackConverterAutoConfiguration        # 把 ToolCallback bean 转成 ToolSpecification
└─ McpServerStatelessAutoConfiguration           # stateless HTTP server

spring-ai-autoconfigure-mcp-server-webmvc/
├─ McpServerSseWebMvcAutoConfiguration           # WebMvc SSE endpoint 注册
└─ McpServerStreamableHttpWebMvcAutoConfiguration

spring-ai-autoconfigure-mcp-server-webflux/
├─ McpServerSseWebFluxAutoConfiguration
└─ McpServerStreamableHttpWebFluxAutoConfiguration
```

加上 starters（`spring-ai-spring-boot-starters/spring-ai-starter-mcp-{client,server}-*`）：用户在 `pom.xml` 里加一个依赖就能自动启用。

### 7.2 关键 Properties

`McpClientCommonProperties`：
- `enabled` (default: true)
- `type` (SYNC | ASYNC)
- `name` / `version`
- `request-timeout` (Duration)
- `initialized` (启动时是否自动 initialize)
- 嵌套 `stdio.*` / `sse.*` / `streamable-http.*`

`McpServerProperties`：
- `enabled` (default: true)
- `serverInfo.name` / `version`
- `capabilities.*`
- `sse.keep-alive` (心跳间隔)
- `streamable.max-size` (单帧大小上限)
- `annotation-scanner.enabled` / `base-packages`

### 7.3 Customizer 体系

`spring-ai/mcp/common/.../customizer/McpClientCustomizer.java:36-47`：

```java
public interface McpClientCustomizer<B> {
    void customize(String name, B componentBuilder);
}
```

装配时（`McpClientAutoConfiguration.java:220-227`）：

```java
@Bean
McpSyncClientConfigurer mcpSyncClientConfigurer(
        ObjectProvider<McpClientCustomizer<McpClient.SyncSpec>> customizerProvider) {
    return new McpSyncClientConfigurer(customizerProvider.orderedStream().toList());
}
```

用户：

```java
@Bean
McpClientCustomizer<McpClient.SyncSpec> myCustomizer() {
    return (name, spec) -> {
        if ("weather".equals(name)) {
            spec.requestTimeout(Duration.ofSeconds(30));
        }
    };
}
```

多个 customizer 可独立声明，按 `@Order` 链式执行。

### 7.4 lynx：纯库，无 IoC 容器

```go
// 用户全权组装
client := sdkmcp.NewClient(impl, &sdkmcp.ClientOptions{
    KeepaliveInterval:      30 * time.Second,
    ToolListChangedHandler: provider.OnToolListChanged,
})
session, _ := client.Connect(ctx, transport, nil)

provider, _ := lynxmcp.NewProvider(lynxmcp.ProviderConfig{
    Sources: []lynxmcp.Source{{Name: "weather", Session: session}},
})
```

### 7.5 取舍

| 维度 | Spring AI | lynx |
|-----|----------|------|
| 上手成本 | 加 starter + 写 yaml 即可 | 写 5-10 行 Go 装配 |
| 灵活度 | 受 properties + customizer 约束 | 完全自由（直接配 SDK options）|
| 可测试性 | 需 Spring Test + mock 进程 | `NewInMemoryTransports()` 单测自给自足 |
| 多 client 管理 | autoconfig 自动装配数组 | 用户自己 `[]Source` |

Spring AI 的开箱即用对 Spring Boot 用户有显著加速；非 Spring 环境用户引入这套有相当负担。lynx 不假设 IoC 容器存在，所有装配显式可见。

---

## 8. Sync vs Async：双轨 vs 单轨

**Spring AI**：每个核心组件都有两份独立类：

```
SyncMcpToolCallback                  ↔  AsyncMcpToolCallback
SyncMcpToolCallbackProvider          ↔  AsyncMcpToolCallbackProvider
McpToolUtils.toSyncToolSpecification ↔  toAsyncToolSpecification
```

`AsyncMcpToolCallback.call(...)` 内部用 `mcpClient.callTool(req).block()` —— 把 reactive 转回阻塞 —— 然后 `contextWrite(...)` 透传 reactor context。`toAsyncToolSpecification` 复用 sync 版的 handler 然后包成 `Mono.fromCallable(...).subscribeOn(Schedulers.boundedElastic())`。

为什么必须两份：`McpAsyncClient` 和 `McpSyncClient` 是 SDK 层的不同类型，泛型抽象不出来。

**lynx**：单轨同步。SDK 的 `*sdkmcp.ClientSession.CallTool` 本身就同步签名（`(*CallToolResult, error)`），内部已是异步实现。lynx 上层若需并发，goroutine 自起。

---

## 9. Transport 实现层次

### 9.1 Spring AI 的五种 server transport

| 类 | 协议 | 模式 |
|---|------|-----|
| `StdioServerTransportProvider` | Stdio | 单进程 |
| `WebMvcSseServerTransportProvider` | HTTP + SSE | Servlet 多 session，内部 `Emitter` 池 |
| `WebFluxSseServerTransportProvider` | HTTP + SSE | Reactive，基于 `Flux<ServerSentEvent>` |
| `WebMvcStreamableServerTransportProvider` | HTTP streaming | 一请一响，长连接 |
| `WebMvcStatelessServerTransportProvider` | HTTP streaming | 强制无状态，FaaS 友好 |

加上 `WebFluxStreamableServerTransportProvider`，共六种。每种都有一份 `McpServer*AutoConfiguration` 配套，自动注册 endpoint（默认 `/mcp/sse` / `/mcp`）。

### 9.2 SSE 心跳

`McpServerSseProperties.keepAlive` 默认 20s，防代理超时；client 端 `HttpClientSseClientTransport` 自动重连。

### 9.3 lynx：透传 SDK，零包装

`mcp/DESIGN.md` 第 5 章直接示范用 SDK 的 `mcp.StdioTransport{}` / `mcp.NewStreamableHTTPHandler(...)` / `mcp.SSEHandler` 注册到 `http.Handler`。本包不再加一层。

```go
// stdio
srv.Run(ctx, &sdkmcp.StdioTransport{})

// streamable HTTP
http.Handle("/mcp", sdkmcp.NewStreamableHTTPHandler(
    func(*http.Request) *sdkmcp.Server { return srv }, nil,
))
```

### 9.4 取舍

Spring AI 的多种 transport autoconfig 在 Spring Boot 项目里很丝滑，但**所有 transport 实现都有对应的 SDK 类**——Spring 这层主要是 endpoint 自动注册和 properties 绑定。lynx 不在 IoC 环境里，没必要再做。

---

## 10. 错误体系全景

### 10.1 Spring AI

异常层级：

```
java.lang.RuntimeException
└─ org.springframework.ai.tool.execution.ToolExecutionException
   ├─ field: ToolDefinition toolDefinition
   ├─ method: getToolDefinition()
   └─ 用途：包装所有 tool 执行异常
```

使用模式（`SyncMcpToolCallback.java:135-144`）：

```java
catch (Exception ex) {
    throw new ToolExecutionException(this.getToolDefinition(), ex);
}
if (response.isError() != null && response.isError()) {
    throw new ToolExecutionException(this.getToolDefinition(),
        new IllegalStateException("Error calling tool: " + response.content()));
}
```

用户捕获（典型 Spring MVC 模式）：

```java
@ControllerAdvice
class GlobalExceptionHandler {
    @ExceptionHandler(ToolExecutionException.class)
    ResponseEntity<?> handle(ToolExecutionException e) {
        return ResponseEntity.status(500).body(Map.of(
            "tool", e.getToolDefinition().name(),
            "error", e.getCause().getMessage()
        ));
    }
}
```

### 10.2 lynx

`mcp/errors.go:17-30`：

```go
type ToolCallError struct {
    ToolName string
    Message  string
}

func (e *ToolCallError) Error() string {
    return fmt.Sprintf("mcp tool %q failed: %s", e.ToolName, e.Message)
}
```

`Tool.Call` 三类错误（`mcp/tool.go:255-282`）：

| 场景 | 错误形式 |
|-----|---------|
| 远端 IsError=true | `*ToolCallError{ToolName, Message}` |
| 协议/传输错误 | `fmt.Errorf("call tool %q: %w", name, err)` |
| 参数 unmarshal 失败 | `fmt.Errorf("decode arguments for tool %q: %w", name, err)` |

用户判别：

```go
out, err := tool.Call(ctx, args)
var tcErr *lynxmcp.ToolCallError
switch {
case errors.As(err, &tcErr):
    // 远端 tool 失败 → 把 message 喂回 LLM 自我纠正
case err != nil:
    // 传输/协议错误 → 重试或告警
}
```

### 10.3 取舍

- Spring AI：unchecked 异常 + cause 链 + 框架级 ExceptionHandler，符合 Spring MVC 用户习惯
- lynx：type-safe error + `errors.As`，符合 Go 标准库习惯（参考 `*net.OpError` / `*fs.PathError`）
- 信息密度等价；编程模型不同

---

## 11. 协议版本与 SDK 版本

| 维度 | Spring AI | lynx |
|-----|----------|------|
| MCP SDK | `io.modelcontextprotocol:mcp:2.0.0-M2`（pre-release） | `github.com/modelcontextprotocol/go-sdk@v1.5.0`（已 GA）|
| MCP spec 版本 | 跟随 2.0.0-M2 支持的最新 spec（含 sampling-with-tools） | 跟随 go-sdk v1.5.0：支持 2025-11-25 / 2025-06-18 / 2025-03-26 / 2024-11-05 |
| 协议协商 | SDK 内部 `Initialize` 握手 + capabilities 交换 | 同上 |
| 反向能力 spec 支持 | 完整 | SDK 完整支持，但 lynx 桥接层未暴露 |

go-sdk 已 GA，spring-ai-mcp 仍在 milestone（M2）；接口可能继续变。lynx 因为薄壳，跟着 SDK 升级几乎无成本；Spring AI 升 SDK 时多个 autoconfig / properties / customizer 都要联动改。

---

## 12. 未支持能力清单（按优先级）

### 12.1 lynx 完全不支持的 MCP 能力（v2 候选）

| 能力 | Spring AI 怎么做 | lynx 应做的接入点 | 优先级 |
|-----|-----------------|-----------------|-------|
| Server-side `exchange` 反向调用（sampling/elicit/progress/ping）| `ToolContext` 注入 `McpSyncServerExchange` + `@McpProgress` 等注解 | `ctx` 上挂 `*sdkmcp.ServerSession`，加 `ServerSessionFromContext(ctx)` | 高 |
| Cancellation 通知 | SDK 自动；用户可挂 handler | SDK ctx cancel 已传递，lynx 无包装 | 中（SDK 自动可用）|
| Resource 订阅 | `@McpResource` + 手动 SDK 接入 | 不在 chat tool 范畴；需要新抽象 | 低 |
| Output Schema 自动生成 | `@McpTool(generateOutputSchema=true)` 反射 | Go 无运行时 reflection-friendly schema 生成 | 低 |
| Progress 注解 | `@McpProgress(clients)` | 需 §12.1 第 1 行先到位 | 中 |
| Logging 注解 | `@McpLogging(clients)` | 同上 | 中 |
| ToolFilter | `McpToolFilter` 接口 | `ProviderConfig.Filter func(Source, *Tool) bool` | 中 |
| Customizer 链 | `McpClientCustomizer<B>` | 用户自配 SDK options 已够 | 低 |
| Auto-configuration | Spring Boot autoconfig | lynx 是库非框架，不做 | — |

### 12.2 lynx 故意不做（带理由）

| 能力 | 理由 |
|-----|------|
| 注解扫描（`@McpTool` 等）| Go 无 runtime annotation；codegen 复杂度激增收益小；显式注册更可控 |
| 双轨 Sync/Async | Go 不需要 reactor 镜像；SDK 已是异步实现 |
| 富内容自动 ImageContent | 99% 场景 TextContent；富内容延后 v2 |
| `DefaultMcpToolNamePrefixGenerator` 的字符过滤+截断+幂等 | 现代模型 100+ 字符 tool 名无问题；用户可自定义 |
| Spring 事件总线驱动 | 直接方法值更轻；多订阅当前无需求 |
| `McpClientCustomizer` | SDK 的 `ClientOptions` 已足够直接 |

---

## 13. 测试支持

**Spring AI**：无内建轻量夹具。社区做法是
1. `@SpringBootTest` + 起真实 server 进程
2. 用 Testcontainers 拉镜像
3. WebMvc 测 SSE 用 `MockMvc + RestAssured`

**lynx**：直接用 SDK 的 `mcp.NewInMemoryTransports()`：

```go
serverT, clientT := sdkmcp.NewInMemoryTransports()

srv := sdkmcp.NewServer(impl, nil)
_ = lynxmcp.RegisterTools(srv, myTools()...)
ss, _ := srv.Connect(ctx, serverT, nil)
defer ss.Close()

cli := sdkmcp.NewClient(impl, nil)
cs, _ := cli.Connect(ctx, clientT, nil)
defer cs.Close()
```

`mcp/provider_test.go` / `tool_test.go` / `server_test.go` 全部用这套，零外部进程、零端口监听。lynx 在这一项明确胜出。

---

## 14. 性能与并发

### 14.1 多 source / 多 client 遍历模式

| 维度 | Spring AI | lynx |
|-----|----------|------|
| listTools fan-out | `mcpClients.stream().flatMap(...)` 顺序流 | `for _, src := range p.cfg.Sources` 顺序循环 |
| 慢源阻塞影响 | 阻塞整个 fan-out | 同上 |
| 改并行成本 | `parallelStream()` 一行 | `errgroup` 一段（约 15 行）|

两者都未做并行 —— 保守设计避免资源耗尽。多源场景 N 大时这是共同的潜在短板。建议 v2 在 lynx 加 `ProviderConfig.ParallelFetch bool`，按需切并行。

### 14.2 Tool.Call 并发安全

| 维度 | Spring AI | lynx |
|-----|----------|------|
| `McpSyncClient` 并发安全 | 是（SDK 层）| 同上（go-sdk session 并发安全）|
| Provider 缓存读写 | volatile + 双检锁 | atomic.Pointer + 双检锁 |
| `Tool` / `ToolCallback` 实例 | 不可变 | 不可变 |

---

## 15. 总结表

### 15.1 lynx 故意不做（保持克制）

| 特性 | 不做的理由 |
|-----|----------|
| Spring Boot autoconfig | lynx 是库非框架；用户可在自己的 main 里 5 行装配 |
| `@McpTool` 等注解扫描 | Go 无 runtime annotation；codegen 复杂度收益不平衡 |
| 双轨 Sync/Async | SDK 同步签名内部已异步；Go 无 reactor |
| 命名前缀的字符过滤+64 截断+跨连接幂等 | 现代 LLM 已不限 64 字符；行为更可预测 |
| `McpClientCustomizer` 链式接口 | 直接配 SDK `ClientOptions` 够清楚 |
| `ToolContextToMcpMetaConverter` 全量倒 + 黑名单 | 默认不外发更安全；用户用 `WithMeta` 显式注入 |
| `RegisterRegistry` 静默跳过非 callable | 与 `RegisterTools` fail-fast 语义不一致；删 |
| Progress / Logging / Sampling / Elicitation 框架 | 无 server-side `exchange` 注入前不能做；先做 §12.1 第 1 行 |

### 15.2 lynx 应该做但还没做（v2 路线）

| 特性 | Spring AI 做法 | lynx 接入点 | 优先级 |
|-----|---------------|-----------|-------|
| Server 端反向 `exchange` | `ToolContext` 注入 `McpSyncServerExchange` | `ctx` 上挂 `*sdkmcp.ServerSession` + `ServerSessionFromContext(ctx)` | 高 |
| `ToolFilter` | `McpToolFilter` 接口 | `ProviderConfig.Filter func(Source, *Tool) bool` | 中 |
| Progress 通知 | `@McpProgress` + `exchange.progressNotification` | 高位实现需 §12.1 第 1 行 | 中 |
| OTel 埋点 | Spring observation API | `Tool.Call` / `makeServerHandler` 包 span | 中 |
| Schema 主动校验 | `JsonSchemaUtils.ensureValidInputSchema` | `stringSchemaToAny` 增强 | 低 |
| 多源并行 fan-out | `parallelStream()`（Spring AI 也未做）| `errgroup` + `ProviderConfig.ParallelFetch` | 低（看实测）|
| 富内容回灌（Image/Resource） | `ImageContent` 在 handler 里直接产出 | 扩展 `chat.ToolReturn` schema | 低 |

### 15.3 lynx 有，Spring AI 没有/做得差

| 特性 | lynx 做法 | Spring AI 缺陷 |
|-----|---------|--------------|
| InMemory 测试夹具 | 直接 `NewInMemoryTransports()` | 无轻量夹具，需起进程或 Testcontainers |
| API 表面 | `NewProvider(cfg)` / `NewTool(cfg)` / `RegisterTools(srv, tools...)` 三件套 | 需熟悉 12+ AutoConfig + 20+ properties + 10+ Bean 类型 |
| 显式错误处理 | `errors.As(&ToolCallError)` type-safe | unchecked 异常 + cause 链反射判断 |
| 包结构紧凑 | 单包 `mcp/`，5 个生产 .go ~750 行 | 12 个 module 跨度大 |
| Config 表达力 | 值类型 + `Validate()` 就地填默认 + 单参构造 | Builder + 多参 + 多 Bean 协作 |
| SDK 跟新成本 | 改一两行 import 即可（薄壳） | 多个 autoconfig / properties / customizer 联动改 |
| 协议版本 | go-sdk v1.5.0 已 GA | mcp 2.0.0-M2 仍 milestone |

---

## 16. 选型建议

**用 Spring AI 的场景**：
- 项目已经是 Spring Boot 全栈
- 需要开箱即用的 yaml 配置驱动
- 工具集大、增减频繁、要靠注解管理
- 需要服务端工具反向 sampling / elicitation 等高级能力（直接可用）
- 团队熟悉 Spring 生态

**用 lynx 的场景**：
- Go 后端、CLI、内部脚本工具
- 工具数量少而稳定，显式注册可接受
- 重视测试便利性（InMemory transport）
- 不想引入 IoC 容器和注解魔法
- 协议跟新敏感（lynx 跟 SDK 几乎零差）

两者解决同一类问题但走完全不同的路线 —— 框架文化 vs stdlib 文化的典型例子。lynx 在 v1 阶段有意把"反向能力 / 注解 / 自动配置 / 富内容"留给 v2，先把 hot path（消费 + 暴露 chat tool）做到极致克制；Spring AI 一开始就是"全功能框架"定位。

---

**版本基线**：lynx mcp 模块（合并到 main 后）/ spring-ai 当前主干（mcp 2.0.0-M2）。后续若 spring-ai 升级到 2.0 GA 或 lynx 进入 v2，本文档需对应更新。
