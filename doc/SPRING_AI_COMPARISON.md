# 多角度对比：lynx vs spring-ai

> **基线**
> - lynx HEAD `c8797cf`（branch `feat/core-spring-ai-comparison`，2026-05-14）；go.work 编入 7 个 module（`core` / `models` / `vectorstores` / `agent` / `rag` / `tools` / `mcp` / `otel` / `pkg`），Go 1.26.3
> - spring-ai HEAD `2d3807526`（main，2026-05-14）；2177 个 `.java` 文件，整体 ~90k LOC，按"四层 + 多 sibling module"组织
>
> 本文取代原 `SPRING_AI_COMPARISON.md` 与 `SPRING_AI_GAPS.md` 两个旧版本，按 9 个角度重组：哲学 / 语言范式 / API 形态 / 生态广度 / 中间件 / 子系统深入 / 集成方式 / 可观测与重试 / 战略 gap。

---

## 0. TL;DR

**两者覆盖同一份心智模型**：ChatModel + Embedding + Image + Audio + Moderation + Document + VectorStore + RAG + MCP + Tool calling + Memory + Evaluation。9 大概念**在两边都能一一对上**，差异不在"有没有"，而在"怎么做"。

**根本分歧一句话**：spring-ai 是 *batteries-included framework*（autoconfigure / starter / 23 个 vector store + 15 个 model vendor / 完整 Advisor 体系 / Micrometer 抽象），lynx 是 *thin library*（核心抽象 + 显式构造 + 6 个 vector store + 3 个 model vendor / 2 个 middleware / 原生 OTel）。

**lynx 当前在 5 条线反超 spring-ai 主干**：

| # | 反超点 | 简述 |
|---|---|---|
| 1 | **Reasoning 一等公民** | `AssistantMessage.Reasoning string` + `Usage.ReasoningTokens *int64`，spring-ai 至今没有相应字段 |
| 2 | **chat 包零 provider 知识** | provider-specific metadata key 下沉到 `models/<provider>/`，core 包 grep 不到任何 vendor 字符串；spring-ai `MessageAggregator` 仍硬编码识别 Google `"isThought"` |
| 3 | **`iter.Seq2` 流式** | Go 1.23 内置迭代器即 streaming，调用方 `for r, err := range stream {...}` 自然就能 cancel；spring-ai 仍是 Reactor `Flux` + `contextView` ceremony |
| 4 | **ISP 拆接口** | `vectorstore.Store = Creator + Retriever + Deleter`、`memory.Store = Reader + Writer + Clearer`，spring-ai 单接口仍是巨型 interface |
| 5 | **Cache tokens 类型化** | `Usage.CacheReadInputTokens / CacheWriteInputTokens *int64` 三态语义清晰；spring-ai 2.0 的 `default Long getCacheReadInputTokens() { return null; }` 是默认 null 的弱形态 |

**lynx 在生态广度上的关键 gap**（按 ROI 顺序）：

| # | Gap | 简述 |
|---|---|---|
| 1 | Ollama adapter | 本地模型头号生产入口，OpenAI-compat 走不通（丢 think option / pull-model） |
| 2 | 持久化 Memory 后端 | spring-ai 有 6 个（JDBC / Redis / Mongo / Neo4j / Cassandra / CosmosDB），lynx 仅 in-memory |
| 3 | PDF / Markdown reader | spring-ai 出货 4 种 reader，lynx 仅 text / JSON |
| 4 | Structured output converter | spring-ai 整套 `BeanOutputConverter` / `ListOutputConverter` / `MapOutputConverter` + markdown/thinking cleaner，lynx 仅 `parser.go` 一个 ListParser |
| 5 | Retry 错误分类 | spring-ai `Transient` vs `NonTransient` 双轨；lynx 默认全部重试 |

---

## 1. 哲学 / 定位

两边的**根本立场**决定了后面所有差异。

| 维度 | lynx | spring-ai |
|---|---|---|
| **角色** | 基础库——给应用层 / agent runtime 用 | 应用 framework——给 Spring Boot 业务应用用 |
| **打包思路** | go.work 多 module，按需 import | Maven artifact 矩阵 + Spring Boot starter |
| **加 vendor 的代价** | 新建 sibling module（`models/<x>` 或 `vectorstores/<x>`），不影响 core | 新建 `models/spring-ai-<x>` + `auto-configurations/models/<x>` + `starters/spring-ai-starter-<x>`，三件套 |
| **依赖注入** | 没有（显式 `NewClient(model)` / `NewStore(config)`） | Spring DI 容器一等公民，所有 Bean 都接 `@Autowired` |
| **配置方式** | 构造函数 + Option struct | `application.yml` + `@ConfigurationProperties` + autoconfigure |
| **开箱即用程度** | 最小可工作集合需要用户手动拼 model + middleware | starter 引入即生效，properties 配几行就跑 |
| **依赖体积** | 单 module 内可以零外部依赖（pkg + core） | 即便最小用例也带 Spring Framework + Boot + Micrometer + Reactor |

**lynx 的赌注**：AI 是基础库，工程化（DI / 配置 / 生命周期 / 监控）由 host app 负责。这种立场和 Go 生态长期一致——`net/http` 不带 DI，`database/sql` 不带 ORM，`log/slog` 不带 appender 自动装配。

**spring-ai 的赌注**：AI 是 Spring 应用里的一等组件，所以走完整的 Spring 套路——autoconfigure 让 `spring-ai-starter-openai` 一引入就拿到 `ChatClient` Bean，business code 只见 `@Autowired ChatClient` 不见构造细节。

**这不是优劣问题，是 host 生态决定的**。如果 lynx 也长出"`lynx-fx-openai` 自动装配"那种东西，反而会和 Go 生态语感冲突；如果 spring-ai 抛弃 autoconfigure 让用户自己 `new OpenAiChatModel(...)`，也违反了 Spring 用户的预期。

---

## 2. 语言范式

不只是 Go vs Java，更深的是**两边语言生态里"现代写法"的具体形态**。

### 2.1 流式

```go
// lynx —— iter.Seq2[*Response, error]
for resp, err := range client.Chat().WithUserPrompt("hi").Stream(ctx) {
    if err != nil { return err }
    fmt.Print(resp.Result.Output.Text)
}
```

```java
// spring-ai —— Reactor Flux<ChatResponse>
chatClient.prompt("hi")
    .stream()
    .chatResponse()
    .doOnNext(r -> System.out.print(r.getResult().getOutput().getText()))
    .blockLast();
```

- **lynx**：`iter.Seq2` 是标准库 1.23+ 的内置类型，`for ... range` 直接消费，`return / break` 自然 cancel 上游（runtime 调 stop func），不需要任何 reactive 概念
- **spring-ai**：Reactor `Flux` 是第三方库——长处是组合性强（`map / filter / merge / contextView`），代价是用户必须懂 backpressure / hot vs cold / `subscribeOn` 等概念

中间件层差异更明显：

```go
// lynx StreamMiddleware
type StreamMiddleware func(next StreamHandler) StreamHandler
// 就是普通 Go 1.23 装饰器
```

```java
// spring-ai 流式 Advisor 仍走 Reactor，contextView ceremony
public Flux<ChatClientResponse> adviseStream(ChatClientRequest req, StreamAdvisorChain chain) {
    return chain.nextStream(req).contextWrite(ctx -> ...);
}
```

### 2.2 类型层级

| | lynx | spring-ai |
|---|---|---|
| Message 多态 | 4 个独立 struct（`SystemMessage` / `UserMessage` / `AssistantMessage` / `ToolResultMessage`）共同实现 `Message` interface | `AbstractMessage` 抽象基类 + 4 子类继承 |
| Provider Options | 每个 vendor 自己的 `Options` struct + 共享 `chat.Options` | `ChatOptions` interface + `DefaultChatOptions` + 每 vendor `OpenAiChatOptions extends DefaultChatOptions` |
| 错误 | sentinel error + `errors.Is` / `errors.As` | checked vs unchecked exception 二选一；多用 `RuntimeException` 派生 |
| 泛型用法 | 严格双参 `[Req, Resp]`，跨 6 个模态复用同一 `MiddlewareManager[Req, Resp]` | 通配符 `<T>` + bounded `<T extends X>`，类型擦除限制深 |

### 2.3 反射 vs 显式 schema

工具 schema 生成 ——

```go
// lynx：generic + JSON schema 从 Go struct 自动出
type WeatherIn struct {
    City string `json:"city" jsonschema:"description=城市名"`
}
tool := core.TypedActionFunc[WeatherIn, WeatherOut](...)
```

```java
// spring-ai：反射读取 @Tool / @ToolParam 注解 + Spring bean 注入
@Tool(description = "获取天气")
public WeatherOut getWeather(@ToolParam(description = "城市名") String city) { ... }
```

**取舍**：spring-ai 走的是 Spring 生态多年的"注解 + 反射 + Spring bean DI"路线，能用 `@Autowired` 在 tool 方法里拿其他服务；lynx 走的是"显式构造 + 泛型 + JSON tag"路线，没有 DI 容器、没有运行时注解扫描，类型在编译期就锁死。

### 2.4 并发模型

| | lynx | spring-ai |
|---|---|---|
| 并发原语 | goroutine + channel + `context.Context` | `CompletableFuture` / Reactor / Spring `@Async` |
| 取消 | `ctx.Done()` 沿调用链向下 | `Subscription.cancel()` + Reactor `contextView` |
| 超时 | `context.WithTimeout` | `Mono.timeout(Duration)` |

`context.Context` 在 lynx 里是**第一参数强约定**，Spring 里没有等价的、统一的传递机制（用 Reactor `contextView` 或 Spring `RequestContext`，但不普遍）。

---

## 3. 抽象形态 / API surface

**两层模型完全一致**：底层 SPI（vendor 实现）+ 高层 facade（用户用）。

### 3.1 底层 SPI

```go
// lynx
type Chat.Model interface {
    Call(ctx, *Request) (*Response, error)
    Stream(ctx, *Request) iter.Seq2[*Response, error]
    DefaultOptions() *Options
    Info() ModelInfo
}
```

```java
// spring-ai
public interface ChatModel extends Model<Prompt, ChatResponse>, StreamingChatModel {
    ChatResponse call(Prompt prompt);
    @Override Flux<ChatResponse> stream(Prompt prompt);
}
```

接口本质等价：`call` + `stream`。

### 3.2 高层 facade — fluent 形态对比

```java
// spring-ai：嵌套 Spec 类型，编译期分阶段
chatClient.prompt()
    .system("you are an assistant")
    .user("hi")
    .advisors(advisor1, advisor2)
    .call()
    .chatResponse();
// 每一步返回不同 Spec 类型：PromptSystemSpec → PromptUserSpec → AdvisorSpec → CallResponseSpec
```

```go
// lynx：单一 ClientRequest 类型，方法都能链
client.Chat().
    WithSystemPrompt("you are an assistant").
    WithUserPrompt("hi").
    WithMiddlewares(mw1, mw2).
    Call(ctx)
// 全是 *ClientRequest 上的 method
```

**差别**：
- spring-ai 的 Spec 类型分阶段是**编译期约束**——`CallResponseSpec` 上没有 `user()` 方法，写错了编译不过
- lynx 一个类型上挂所有方法——更松，调用顺序错了运行时不一定立刻报错；但 API 表面积更小，learning curve 短

### 3.3 接口 ISP 粒度

**VectorStore：**

```go
// lynx —— 4 个细接口
type Creator interface { Create(ctx, *CreateRequest) error }
type Retriever interface { Retrieve(ctx, *RetrievalRequest) ([]*Document, error) }
type Deleter interface { Delete(ctx, *DeleteRequest) error }
type Store interface { Creator; Retriever; Deleter; Info() StoreInfo }
```

```java
// spring-ai —— 2 个接口
public interface VectorStore extends DocumentWriter, VectorStoreRetriever {
    void add(List<Document> documents);
    void delete(List<String> idList);
    void delete(Filter.Expression filterExpression);
    void delete(SearchRequest searchRequest);
    Optional<Boolean> delete(...);
    List<Document> similaritySearch(SearchRequest request);
}
```

**Memory：**

```go
// lynx
type Store interface { Reader; Writer; Clearer }
```

```java
// spring-ai
public interface ChatMemory {
    void add(String conversationId, Message message);
    void add(String conversationId, List<Message> messages);
    List<Message> get(String conversationId);
    void clear(String conversationId);
}
```

**取舍**：lynx 的 ISP 拆分让 mock / partial impl 更容易（写一个只读 vector store 不需要实现 `Create / Delete`）；spring-ai 的合并接口让 vendor 更"必须实现所有方法"——文档清晰但灵活度低。

### 3.4 模态对称性

两边都有 5 大模态 + Document。

| 模态 | lynx | spring-ai |
|---|---|---|
| Chat | `chat.Model` + `chat.Client` | `ChatModel` + `ChatClient` |
| Embedding | `embedding.Model` + `embedding.Client` | `EmbeddingModel` |
| Image | `image.Model` + `image.Client` | `ImageModel` |
| Audio TTS | `audio/tts.Model` | `TextToSpeechModel` |
| Audio STT | `audio/transcription.Model` | `TranscriptionModel` |
| Moderation | `moderation.Model` + `moderation.Client` | `ModerationModel` |

**lynx 多一层 `Client` 一致性**——所有 6 个模态都用同款 `NewClient(model)` 构造，链式 API 形态对称（spring-ai 只有 Chat 有 `ChatClient` 这种 fluent 层，其他模态直接调 `ImageModel.call(...)`）。

---

## 4. 生态广度（vendor 矩阵）

这是 lynx 最显著的"广度差距"——但也是**故意的**：spring-ai 是 framework 必须覆盖全市场，lynx 是 library 按需扩。

### 4.1 模型 vendor

| 类别 | spring-ai 已有 | lynx 已有 | lynx 缺 |
|---|---|---|---|
| **闭源云模型** | anthropic / openai / google-genai / minimax / deepseek / mistral-ai | anthropic / openai / google | minimax / deepseek / mistral-ai |
| **本地 / 自托管** | ollama / transformers / postgresml | — | ollama（生产硬刚需）|
| **AWS** | bedrock / bedrock-converse | — | bedrock-converse |
| **Google** | vertex-ai-embedding | — | — |
| **专业图像 / 语音** | stability-ai / elevenlabs | — | stability-ai / elevenlabs |

**总计：spring-ai 15 个独立 model module，lynx 3 个**（注意 lynx 的 openai module 内部覆盖 chat / embedding / image / audio/transcription / audio/tts / moderation 6 个模态，spring-ai 拆得更细）。

> **OpenAI-compat 通道**：DeepSeek / vLLM / Together / Anyscale / Groq / Fireworks 这些可以通过 `option.WithBaseURL` 直接走 lynx 的 OpenAI adapter，不需要独立包。MiniMax 同理。这条路在 lynx 比在 spring-ai 更顺——spring-ai 的 `OpenAiChatModel` 接 third-party endpoint 需要绕过 `@ConditionalOnProperty` 配置。

### 4.2 Vector store vendor

| spring-ai（23 个）| lynx（6 个）|
|---|---|
| azure / azure-cosmos-db / bedrock-knowledgebase / cassandra / chroma / coherence / couchbase / elasticsearch / gemfire / mariadb / milvus / mongodb-atlas / neo4j / opensearch / oracle / pgvector / pinecone / qdrant / redis / redis-semantic-cache / s3 / typesense / weaviate | chroma / milvus / pinecone / qdrant / weaviate / inmemory |

**lynx 选了 5 个最常用的云 vendor + 1 个 in-memory**（用于测试 / 演示）。spring-ai 的 23 个里很多是 enterprise 场景驱动的（gemfire / coherence / oracle / mariadb），未必是新项目首选。

### 4.3 Document reader

| spring-ai（4 个）| lynx（2 个）|
|---|---|
| pdf（PagePdf + ParagraphPdf）/ markdown / tika / jsoup | text / JSON |

**真 gap**：PDF / Markdown 在 RAG 场景里几乎必需，lynx 当前需要用户自己包 reader。

### 4.4 Memory 后端

| spring-ai（6 个）| lynx（1 个）|
|---|---|
| jdbc / redis / mongodb / neo4j / cassandra / cosmos-db | in-memory + message-window |

**真 gap**：生产环境必然需要持久化 memory，lynx 当前只有 in-memory。

### 4.5 Tool 实现

| 类别 | spring-ai | lynx |
|---|---|---|
| 内置 tool 实现 | 仅提供 `ToolCallback` SPI，无开箱即用工具 | 出货 `tools/{bash, fs, websearch, webfetch}` 4 大类 |
| Websearch backend | — | 7 个（brave / exa / firecrawl / jina / perplexity / serper / tavily）|
| Webfetch backend | — | 4 个 |

**lynx 反向超**：spring-ai 把 tool 实现完全留给用户，lynx 出货可直接用的 4 大类常用工具，且 backend 走 SPI 可换。

### 4.6 MCP 模块化

| | spring-ai | lynx |
|---|---|---|
| 模块数 | ~12 个 Maven module（common + annotations + transport/webmvc + transport/webflux + 5 个 starter + autoconfigure 2 个）| 1 个 Go package |
| 估算 LOC | 8k–10k | ~750 |
| Sync/Async | 双轨（每组件 sync/async 两份）| 仅 sync + `iter.Seq2` 流式 |
| Server transport | WebMvc / WebFlux / stateless 三种 | InProc + stdio + HTTP SSE（走 mcp-go SDK） |

---

## 5. 中间件 / Advisor 体系

### 5.1 lynx middleware 清单

| 名称 | 位置 | 作用 |
|---|---|---|
| `chat.NewToolMiddleware` | `core/model/chat/tool_middleware.go` | self-driving tool loop（接到 LLM 返回的 tool call → 调用 → 把结果塞回去 → 继续 LLM）|
| `chat/memory.NewMiddleware` | `core/model/chat/memory/middleware.go` | 多轮会话记忆（prepend 历史 + append 新 message）|
| `rag.NewMiddleware` | `rag/pipeline_middleware.go` | 把 RAG pipeline 包成 chat middleware：增强 user 最后一条 message + 附 retrieved docs 到 metadata |

**共 3 个 middleware**，全部走统一的 `CallMiddleware` / `StreamMiddleware` 类型签名。用户写自定义中间件就是普通 Go 函数装饰器。

### 5.2 spring-ai advisor 清单

| 名称 | 位置 | 作用 |
|---|---|---|
| `ChatModelCallAdvisor` | client-chat | 链尾真正调用 ChatModel |
| `ChatModelStreamAdvisor` | client-chat | 流式链尾 |
| `ToolCallAdvisor` | client-chat | tool loop |
| `SimpleLoggerAdvisor` | client-chat | logging |
| `SafeGuardAdvisor` | client-chat | 敏感词 / 安全过滤 |
| `MessageChatMemoryAdvisor` | client-chat | 普通 message-window memory |
| `StructuredOutputValidationAdvisor` | client-chat | 结构化输出 schema 校验 |
| `QuestionAnswerAdvisor` | advisors/vector-store | vector store RAG（query → retrieve → augment）|
| `VectorStoreChatMemoryAdvisor` | advisors/vector-store | 语义检索式长期 memory |
| `RetrievalAugmentationAdvisor` | spring-ai-rag | 完整 RAG pipeline 协调 |

**共 ~10 个 advisor**，全部基于 `Advisor` 接口 + `Ordered`（spring 的 priority），通过 `chatClient.advisors(...)` 注册。

### 5.3 概念差异

| 概念 | lynx middleware | spring-ai Advisor |
|---|---|---|
| 抽象本质 | 普通装饰器（`func(next Handler) Handler`）| around 链 + `Ordered` 优先级 |
| Call / Stream 分离 | 两套 type（`CallMiddleware` + `StreamMiddleware`）| 一个 `Advisor` interface 但 call/stream 是两个方法（`adviseCall` / `adviseStream`）|
| 注册方式 | `WithMiddlewares(mw1, mw2)` 按位置 | `.advisors(a, b)` + Ordered annotation 综合排序 |
| 内置数量 | 3 | ~10 |
| 用户写自定义 | 写一个 closure | 实现 `CallAdvisor` 或 `StreamAdvisor` 接口 |

**两者本质等价**（都是装饰器 + around chain），差别在出货数量和 ceremony 程度。

### 5.4 lynx 真缺的 advisor

按使用频率：

1. **`SafeGuardMiddleware`**（敏感词 / prompt injection 简单防护）—— 适合作为单独 middleware 出货
2. **`LoggerMiddleware`**（请求 / 响应自动 log）—— 30 行可写
3. **`StructuredOutputValidationMiddleware`**（结构化输出校验失败时把错误回填给 LLM）—— 配合 §6.2 的 Structured Output 一起做

---

## 6. 关键子系统深入对比

挑 5 个最有代表性的子系统做深入对比。

### 6.1 Chat client 形态

详见 §3.2。补充几个细节：

**ChatOptions / Provider Options 关系：**

| | lynx | spring-ai |
|---|---|---|
| 共享字段定义在 | `core/model/chat.Options`（Model / Temperature / TopP / TopK / MaxTokens / Stop / Penalties / Tools）| `ChatOptions` interface + `DefaultChatOptions` impl |
| Vendor 扩展字段 | `models/<vendor>/options.go` 内 struct embed `chat.Options`，加 vendor-only 字段 | `OpenAiChatOptions extends DefaultChatOptions`，加 vendor-only 字段 |
| 透传未知字段 | `JSON.ExtraFields` map（用户能自由塞 OpenAI-compat endpoint 的非标参数）| 必须改 spring-ai source code 加 setter（受限） |

**lynx 反向超**：`JSON.ExtraFields` 让用户能直接接 DeepSeek-R1（OpenAI-compat 但带 `reasoning_content` 扩展字段）这种 third-party endpoint，不需要 patch 库。spring-ai 走"每个字段都得在 Options 上预定义"的强类型路线。

### 6.2 Tool / function calling

**spring-ai `tool/` 子系统**：

```
spring-ai-model/.../tool/
├── definition/    — ToolDefinition + DefaultToolDefinition
├── execution/     — ToolCallResultConverter / ToolExecutionException / ToolExecutionExceptionProcessor
├── resolution/    — ToolCallbackResolver SPI + SpringBean/Static/Delegating 三种 resolver
├── function/      — FunctionToolCallback（Java function → tool）
├── method/        — MethodToolCallback（反射 Java method → tool）
├── annotation/    — @Tool / @ToolParam
├── augment/       — ToolInputSchemaAugmenter / AugmentedToolCallback
└── support/       — ToolUtils
```

**估算 LOC：~3000**（不含 vendor 适配的 tool serialization）。

**lynx tool 抽象**：

```go
// core/model/chat/tool.go
type Tool interface { Definition() ToolDefinition }
type CallableTool interface {
    Tool
    Metadata() ToolMetadata
    Call(ctx, arguments string) (string, error)
}

// pkg/json
// 从 Go struct 用 tag + 反射出 JSON schema，~500 LOC
```

加上 agent 层的 `core.TypedActionFunc[In, Out]` 把任意 typed action 转成 `CallableTool`。

**估算 LOC：~500（core 内）+ 300（pkg/json schema）**。

**差异：**

| 维度 | spring-ai | lynx |
|---|---|---|
| Schema 生成 | 反射 `@Tool` / `@ToolParam` 注解 | 反射 Go struct + JSON tag + jsonschema tag |
| 调度 | `ToolCallbackResolver`（按 name 查 Bean） | 直接 `[]chat.Tool` 数组持有 |
| 执行异常处理 | `ToolExecutionException` + `ExceptionProcessor`（用户可自定义如何把异常转成 LLM 能看的 string）| `ToolCallError` + `errors.As` 类型安全；tool 自己负责返回 string |
| Result 转换 | `ToolCallResultConverter` 把任意 Java return → LLM string | tool 自己返回 string（约束更紧）|
| Bean 集成 | `SpringBeanToolCallbackResolver` 自动从 Spring DI 找 tool | 无 DI，构造时显式传入 `[]Tool` |
| 反向暴露给 MCP | `SyncMcpToolCallbackProvider`（lazy）| `mcp.RegisterTools(tools, server)`（eager）|

**关键差距**：spring-ai 的 tool 子系统更"厚"——支持注解、Spring Bean、resolver、exception processor、result converter 一应俱全。lynx 走"显式构造 + 泛型 + json schema"路线，整体 ceremony 少很多。

### 6.3 RAG pipeline

**spring-ai：** 4 阶段抽象 + 1 个总 advisor

```
rag/preretrieval/
├── query/transformation/  — CompressionQueryTransformer / RewriteQueryTransformer / TranslationQueryTransformer
└── query/expansion/        — MultiQueryExpander
rag/retrieval/
├── search/                 — VectorStoreDocumentRetriever / DocumentRetriever
└── join/                   — ConcatenationDocumentJoiner / DocumentJoiner
rag/postretrieval/
└── document/               — DocumentPostProcessor
rag/generation/
└── augmentation/           — ContextualQueryAugmenter / QueryAugmenter

advisor/RetrievalAugmentationAdvisor  ← 接入 ChatClient 的入口
```

**lynx：** 扁平组件 + 1 个 pipeline middleware

```
rag/
├── query_transformer_compression.go
├── query_transformer_rewrite.go
├── query_transformer_translation.go
├── query_expander_multi.go
├── query_augmenter_contextual.go
├── document_retriever_vectorstore.go
├── document_refiner_dedup.go    ← lynx 独有
├── document_refiner_rank.go     ← lynx 独有
├── pipeline.go
└── pipeline_middleware.go        ← 接入 chat client 的入口
```

**对照：**
- 数量上 lynx ~9 个组件 vs spring-ai ~10 个（含 4 阶段层级）
- spring-ai 4 阶段（preretrieval / retrieval / postretrieval / generation）层级更明显；lynx 是扁平 + 自由组合
- spring-ai 走 `RetrievalAugmentationAdvisor`；lynx 走 `rag.NewMiddleware`
- **lynx 多 `DeduplicationRefiner` / `RankRefiner`**；spring-ai 没有专门的 refiner 概念（用 `DocumentPostProcessor` 通用接口）
- **spring-ai 多 `DocumentJoiner`**（多路检索合并）；lynx 当前 pipeline 直接拼

### 6.4 VectorStore filter mini-language

两边都有 SQL-like 过滤表达式（`type == 'comedy' AND year >= 2020`）。

| 维度 | spring-ai | lynx |
|---|---|---|
| 语法 | SQL-like | 同款 |
| Lexer/Parser 实现 | **ANTLR4 generated**（grammar 在 `vectorstore/filter/antlr4/`）| **手写** lexer + recursive descent parser，~500 LOC |
| AST visitor | 内置 visitor pattern | 同款 |
| 转 vendor query | `AbstractFilterExpressionConverter` 模板方法基类 | 每个 vendor 独立写 visitor（chroma / milvus / pinecone / qdrant / weaviate / inmemory） |
| Runtime 依赖 | ANTLR4 runtime（~500KB JAR） | 零运行时依赖 |
| 总代码量（含 vendor） | ~750 LOC（基类省代码）| ~3300 LOC（各 vendor 各 ~600–800） |

**取舍：**
- ANTLR4：改 grammar 只动一处，运行时多带 runtime；基类省代码
- 手写 parser：零运行时依赖；但每个 vendor visitor 从 0 写
- **lynx 当前的架构倒退**：没有 `AbstractVisitor` 基类，每个 vendor 600–800 LOC 重复样板——一个未来要补的事

### 6.5 MCP 桥接

| 维度 | spring-ai | lynx |
|---|---|---|
| 模块数 | ~12 个 Maven module（common + annotations + 2 transport + 5 starter + 2 autoconfigure）| 1 个 Go package |
| 估算 LOC | 8k–10k | ~750 |
| 客户端方向（MCP server → 本地 tool）| `SyncMcpToolCallback` / `AsyncMcpToolCallback`（双轨）| `mcp.Tool` 包 `chat.CallableTool` |
| 服务端方向（本地 tool → MCP server）| `SyncMcpToolCallbackProvider` 等 | `mcp.RegisterTools(tools, server)` |
| 注解化 | `@McpTool` / `@McpToolParam` / `@McpPrompt` / `@McpResource` / `@McpSampling` / `@McpElicitation` / `@McpLogging` / `@McpProgress` / `@McpMeta` 9 个 | 无（Go 无 runtime annotation；走构造 `ToolConfig{...}` / `ProviderConfig{...}`）|
| Sync/Async 双轨 | 必须（每组件 sync/async 两份类）| 单同步路径 |
| Transport | WebMvc / WebFlux / stateless 三种实现 | 走 mcp-go SDK（InProc / stdio / HTTP SSE / Streamable HTTP）|
| 测试夹具 | 无内建（Testcontainers 或起 server 进程）| `mcp.NewInMemoryTransports()` 内置 |
| 错误判别 | unchecked `ToolExecutionException` + cause 链 | type-safe `*ToolCallError` + `errors.As` |
| 命名前缀 | 缩写 + 字符过滤 + 64 截断 + 跨连接幂等去重 | `<src>_<tool>` + fail-fast 重名校验 |

**lynx 反向超**：MCP 桥接的"克制设计"是 lynx 最自信的点之一——单包 ~750 LoC 覆盖了双向 hot path，对比 spring-ai 估算 8k–10k 的 module 树。代价是 lynx 没出货 sync/async 双轨、没出货 transport-level autoconfigure、没出货注解化 server tool 声明。

---

## 7. 打包 / 集成 / 部署

### 7.1 引入路径

```yaml
# spring-ai：pom.xml
<dependency>
    <groupId>org.springframework.ai</groupId>
    <artifactId>spring-ai-starter-openai</artifactId>
</dependency>
```

```yaml
# application.yml
spring.ai.openai.api-key: ${OPENAI_API_KEY}
spring.ai.openai.chat.options.model: gpt-4o
```

```java
// business code
@Autowired ChatClient chatClient;
// 用 chatClient.prompt(...).call().chatResponse() 即可
```

```go
// lynx：go.mod
require github.com/tangerg/lynx/core v0.x
require github.com/tangerg/lynx/models/openai v0.x

// business code
model, _ := openai.NewChatModel(openai.Config{APIKey: os.Getenv("OPENAI_API_KEY"), Model: "gpt-4o"})
client, _ := chat.NewClient(model)
resp, _ := client.Chat().WithUserPrompt("hi").Call(ctx)
```

**差异**：
- spring-ai 用户**完全不接触**模型构造代码，配 yml 引入 starter 即可拿 `ChatClient` Bean
- lynx 用户**显式构造**——但好处是 import 路径就是依赖图，不存在"为什么 ClassPath 上有 anthropic 但我没引入"的疑惑

### 7.2 BOM / 版本管理

| | spring-ai | lynx |
|---|---|---|
| 集中版本管理 | `spring-ai-bom`（一个 pom 锁所有模块版本） | go.work 在 lynx workspace 内自动同步；外部使用者用 `go.mod` 每个 module 独立版本 |
| 跨 vendor 升级 | 升 BOM 版本就升全部 | 各 module 各自 `go get`（更细粒度，也更繁琐） |

### 7.3 Spring Boot autoconfigure 链

spring-ai 一个 starter 背后通常是：

```
spring-ai-starter-openai
  └── 依赖 spring-ai-autoconfigure-model-openai
        └── 依赖 spring-ai-openai
              └── 依赖 spring-ai-model + spring-ai-commons
        └── 依赖 spring-ai-autoconfigure-model-chat
        └── 依赖 spring-ai-autoconfigure-tool
        └── 依赖 spring-ai-autoconfigure-retry
```

`@AutoConfiguration` 类按 `@Conditional` 决定是否生效（如果有 `spring.ai.openai.api-key`，注册 `OpenAiChatModel` Bean）。

**lynx 完全没这层**——没有 autoconfigure / starter / conditional / properties binding。代价是用户多写几行构造代码，好处是不会出现"为什么不 work，是 condition 没满足吗"这种 Spring 经典调试问题。

### 7.4 整体复杂度

| | spring-ai | lynx |
|---|---|---|
| 最小可工作引入 | 1 个 starter | 2 个 module（core + 1 vendor）|
| 配置 | yml + properties + autoconfigure | Go 构造函数 + Option struct |
| 调试 启动失败 | "ClassNotFoundException / NoSuchBeanDefinition / @Conditional missing" 链路 | "import 路径错 / option 字段错" 编译期就报 |

---

## 8. 可观测性 / 错误处理 / Retry

### 8.1 可观测性

**spring-ai**：深度整合 Micrometer Observation 抽象

```java
// 每个 chat call 出 ChatModelObservationConvention + ChatModelObservationContext
// ChatClient 出 ChatClientObservationConvention
// Advisor 链每一步出 AdvisorObservationContext
// VectorStore 出 VectorStoreObservationConvention
// 这些都通过 ObservationRegistry 接 Micrometer，进而到 OTel / Prometheus / Zipkin
```

**lynx**：直接用 OTel API + 一层 exporter bridge

```go
// otel/log + otel/slog —— SpanExporter 把 span 转写到标准库 logger
// core 包通过 otel.Tracer(...) 直接埋点（注意：当前 core 部分 path 尚未完成埋点，OBSERVABILITY.md §8 有清单）
```

| 维度 | spring-ai | lynx |
|---|---|---|
| 抽象层 | Micrometer Observation（厂商无关）→ OTel / Prometheus | 直接 OTel（OTel 已是事实标准）|
| 命名规范 | gen_ai semantic conventions（OTel 半官方）| 同款 |
| 埋点覆盖 | Chat / Embedding / Image / VectorStore / Advisor 全覆盖 | 计划全覆盖；当前 core hot path 待补埋点（OBSERVABILITY.md §4 列表）|
| Exporter | 通过 ObservationRegistry 配 Prometheus / OTel SDK / Brave | OTel SDK 直配 / 用户用 `otel/slog` 把 span 写到 slog logger |

**为什么 lynx 不走 Micrometer-like 抽象**：Go 世界从来没有 Micrometer 这种"多 backend 切换 metric 框架"的历史包袱——OTel 已经是事实标准，再套一层抽象没意义。

### 8.2 Retry

**spring-ai**：`spring-ai-retry/` 独立 module

```java
// 错误分类
public class TransientAiException extends RuntimeException { ... }  // 可重试（429 / 503）
public class NonTransientAiException extends RuntimeException { ... }  // 不该重试（400 / 401 / 404）

// RetryUtils 包装 Spring Retry，自动按 Transient/NonTransient 决定
```

**lynx**：`pkg/retry` workspace module

```go
// 默认所有 error 都重试，用户传 RetryOn(fn) 自定义判别
retry.Do(ctx, fn, retry.WithMaxAttempts(3), retry.WithBackoff(...), retry.RetryOn(func(err error) bool { ... }))
```

**关键差距**：spring-ai 的**显式错误分类**对 LLM 集成更友好——429 / 503 自动重试，401 / 400 立刻报错。lynx 当前需要用户在每个 adapter 里自己判别。**这是一个真 gap**，闭合大概要：
1. `pkg/retry` 加 `Transient` / `NonTransient` sentinel 类型
2. 各 vendor adapter（anthropic / openai / google）把 HTTP status code 分类抛错
3. 默认 `RetryOn` 改成识别 Transient 类型

**估算工作量**：30 LOC retry 类型 + 各 adapter ~20 LOC 分类逻辑。

### 8.3 错误处理总体

| | spring-ai | lynx |
|---|---|---|
| 默认风格 | unchecked exception（`RuntimeException` 派生）| sentinel error + `errors.Is/As` |
| 错误传递 | throws + try/catch | return error + caller 判断 |
| 类型识别 | `instanceof XException` 或 `getCause()` 链 | `errors.As(err, &target)` |
| 工具调用错误 | `ToolExecutionException`（unchecked + cause 链）| `*ToolCallError`（type-safe）|

**lynx 反向超**：Go 的 sentinel + `errors.As` 让"分类错误然后决定怎么处理"在 callsite 是显式的，比 Java unchecked exception 在 callsite 是不可见的 default-pass 模式更稳。

---

## 9. 战略 gap 清单 + ROI 路线图

按 ROI 排序，每项明确"为什么不抄"或"该不该抄"。

### 9.1 P0 — 该补，工作量小性价比高

| # | 项 | 工作量 | 价值 |
|---|---|---|---|
| 1 | **Retry `Transient` / `NonTransient` 分类** | 低（pkg/retry ~30 LOC + 各 vendor adapter ~20 LOC）| LLM 集成最常见痛点：429 / 503 自动重试，401 立刻报错 |
| 2 | **Anthropic Extra 通道保护**（让用户预填的 `params.System` / `params.Messages` / `params.Tools` 不被 `buildApiChatRequest` overwrite，启用 prompt caching）| 极低（~30 LOC）| Anthropic prompt caching 1.25× 写入 / 0.1× 读取，省钱关键 |
| 3 | **OTel hot path 埋点**（OBSERVABILITY.md §4 清单）| 低（每个 callsite ~10 LOC）| exporter 已就绪，缺埋点 |

### 9.2 P1 — 生产硬刚需

| # | 项 | 工作量 | 价值 |
|---|---|---|---|
| 4 | **Ollama adapter** | 中（不能走 OpenAI-compat，需独立 client + think option + pull-model 管理）| 本地 LLM 头号生产入口，CLI / 桌面 / 内网 agent 必备 |
| 5 | **持久化 Memory 后端**（Redis + Postgres 先做）| 中（基于 ISP `memory.Store` 接口）| 任何多用户场景必需 |
| 6 | **PDF + Markdown document reader**（放 `document-readers/` sibling module）| 中（PDF 依赖 UniPDF 或 pdfcpu，~200 LOC + 测试）| RAG 场景最常见输入 |

### 9.3 P2 — 闭合架构倒退或 spring-ai 已有的设计

| # | 项 | 工作量 | 价值 |
|---|---|---|---|
| 7 | **Structured output converter**（`core/model/chat/converter/`：JSONParser + MapParser + Markdown / thinking-tag cleaner + 解析失败回填 retry）| 低（~150 LOC + 测试）| LLM → typed Go struct 的开箱即用通道 |
| 8 | **FilterExpression `BaseVisitor` 基类**（`core/vectorstore/filter/visitors.BaseVisitor`）| 低（~150 LOC，重构各 vendor visitor）| 减 ~60% vendor 代码量（~2000 LOC） |
| 9 | **`SafeGuardMiddleware` + `LoggerMiddleware`**（套件式开箱即用 middleware）| 低（~100 LOC 共） | 弥补 advisor 数量差距 |
| 10 | **`DocumentJoiner`（多路检索合并）** + **`QueryRouter`** | 低（~80 LOC）| spring-ai RAG 4 阶段里 lynx 缺的那一块 |

### 9.4 P3 — 长尾 / 大依赖

| # | 项 | 工作量 | 价值 |
|---|---|---|---|
| 11 | Bedrock Converse adapter | 中 | AWS 用户 |
| 12 | MCP v2 反向能力（`ServerSessionFromContext` / sampling / elicit / progress / ping / logging）| 中 | agentic 编排基石 |
| 13 | Vector-store backed chat memory（语义检索式长期记忆）| 中 | 复用 vectorstore.Store + 现有 memory middleware |
| 14 | OpenAI Responses API | 中 | o1/o3/gpt-5 reasoning 解锁 |
| 15 | Anthropic Web Search Tool + Citations | 中 | Anthropic 特有能力 |
| 16 | Anthropic Skills + Files API | 高 | 生成 Excel / PPT / PDF 报告 |
| 17 | Google Gemini 高级特性（CachedContent API / ThinkingLevel / thoughtSignatures）| 中 | Google 用户 |
| 18 | OpenAI Audio Output（chat 主路径同时返回 text + audio）| 低 | gpt-4o-audio-preview |
| 19 | 更多 vendor vector-store（按真实需求扩） | 中-高 | 不紧急 |
| 20 | 更多 OpenAI-compat README 示例（DeepSeek / vLLM / Groq）| 极低 | 防止用户重复提"加独立包" |

### 9.5 故意不做（"为什么不抄"）

| # | spring-ai 有但 lynx 不该抄 | 原因 |
|---|---|---|
| A | Spring Boot autoconfigure / starter | lynx 是 library 不是 framework；Go 生态没有"DI 容器自动装配"传统 |
| B | Micrometer Observation 抽象层 | Go 世界 OTel 已是事实标准，再套一层无意义 |
| C | `@Tool` / `@McpTool` 注解 | Go 无 runtime annotation；上 codegen 性价比低，构造函数路线已足够清晰 |
| D | `ToolCallbackResolver` Spring Bean 解析 | lynx 无 DI 容器；agent 层的 `core.ToolGroupResolver` 已覆盖"按 role 解析工具组"需求 |
| E | sync/async 双轨（每组件两份类）| Go goroutine + `iter.Seq2` 单一路径已经覆盖 sync/async 两种用法 |
| F | `StringTemplate` 完整模板引擎（spring-ai-template-st）| 复杂场景用户挂 `text/template` 即可；lynx 简单 `{key}` placeholder 够用 |
| G | 多套 vendor model module 完整拆分（每模态独立 module）| Go module 多 vendor 适配走 sibling 即可；lynx 一个 vendor module 内可覆盖多模态（如 openai 含 chat + embedding + image + audio + moderation） |

---

## 10. 一句话定档

对照 spring-ai 时**不照抄，照搬克制原则做薄壳**——然后在文档层面把"为什么不抄"讲清楚。这套打法在 reasoning（不抄 P0-1 的 record 重设计）、MCP（不抄全家桶 12 module 拆分）、observability（不抄 Micrometer 抽象层）、cache tokens（不抄 default null Long）四条线上都已经验证成立。

**下一阶段最高 ROI**：Retry 错误分类 + Anthropic Extra 通道保护 + Ollama adapter。三件做完，lynx 在"生产可用度"上就能补齐与 spring-ai 的硬差距，同时保留 thin library 哲学不动摇。

---

*对比结束。双方 HEAD 截至 2026-05-14。*
