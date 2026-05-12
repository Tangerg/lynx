# 深度对比：lynx/core vs spring-ai（2026Q2）

> 基线：spring-ai HEAD `29103b681`（2026-05-11，~40k LOC Java 在 core 模块 + 22 个 vendor vector-store + 15 个 vendor model module）vs lynx HEAD `feat/core-spring-ai-comparison`（基于 main `fbf4673`，2026-05-12），`core/` 模块约 10k Go LOC。
>
> 这份文档只比 **lynx/core/**（基础库层：chat / embedding / image / audio / moderation / document / tokenizer / evaluation / vectorstore / media）vs **spring-ai 的等价模块**（`spring-ai-commons` / `spring-ai-model` / `spring-ai-client-chat` / `spring-ai-vector-store` / `spring-ai-rag` / `spring-ai-retry` / `mcp/`）。**Agent runtime 层（lynx/agent）已经在 [`agent/docs/EMBABEL_DEEP_COMPARISON.md`](../agent/docs/EMBABEL_DEEP_COMPARISON.md) 与 embabel-agent 对比过，不在本文范围**。
>
> 跟 embabel 对比类似地按"架构赌注"组织，但 Spring AI 比 embabel-agent 更接近 lynx/core（都是基础库层），所以这里的对比更聚焦于具体抽象的取舍而不是范式之争。

---

## 1. 范围与定位

| | lynx/core | spring-ai |
|---|---|---|
| **角色** | lynx workspace 里 `core/` 模块 —— 基础库，给 `lynx/agent` / `lynx/mcp` / `lynx/rag` 等模块用 | Spring 生态里的 AI 抽象层 —— 给业务应用用 |
| **语言/平台** | Go 1.26 | Java 17 + Spring Framework |
| **分发** | Go 模块 import | Maven artifact / Spring Boot starter |
| **抽象层数** | 一层：interface + struct + factory func | 多层：interface / abstract class / builder / autoconfigure / starter |
| **多 vendor 内置** | 仓库分 sibling 模块：`models/{anthropic,google,openai}` + `vectorstores/{chroma,milvus,pinecone,qdrant,weaviate}` | 一等公民：22 个 vector-store + 15 个 model vendor module |
| **整体 LOC** | ~10k Go LOC | ~40k Java LOC 仅核心 + 数十 k 在 vendor 集成 |

**底层立场差异**：lynx/core 是"恰好够用的薄抽象 + 显式 vendor 适配"；spring-ai 是"丰富的中间层 + 内置大量 vendor + Boot autoconfigure"。

---

## 2. 模块拓扑对照

| 概念 | lynx/core | spring-ai |
|---|---|---|
| Chat 抽象 | `core/model/chat/` | `spring-ai-model/.../chat/` |
| Chat Client（高层 facade） | `core/model/chat/Client` | `spring-ai-client-chat/.../ChatClient` |
| Chat memory | `core/model/chat/memory/` 一个 middleware | `spring-ai-model/.../chat/memory/` 独立子系统 + `memory/` sibling module |
| Chat advisor / middleware | `core/model/chat/tool_middleware.go` + memory middleware | `spring-ai-client-chat/.../advisor/` 一整套 Advisor pattern |
| Embedding | `core/model/embedding/` | `spring-ai-model/.../embedding/` |
| Image / Audio / Moderation | `core/model/{image,audio,moderation}/` | `spring-ai-model/.../{image,audio,moderation}/` |
| Document | `core/document/` | `spring-ai-commons/.../document/` |
| Document readers | `core/document/{reader_text,reader_json}.go` | `document-readers/` sibling module（PDF / Markdown / Tika / 其他） |
| Token / Tokenizer | `core/tokenizer/Tiktoken` | `spring-ai-commons/.../tokenizer/` |
| Evaluation | `core/evaluation/` | `spring-ai-commons/.../evaluation/` + `spring-ai-client-chat/.../evaluation/` |
| Vector store 抽象 | `core/vectorstore/Store` + `Creator/Retriever/Deleter` | `spring-ai-vector-store/.../VectorStore` |
| Vector store filter 语言 | `core/vectorstore/filter/` 手写 lexer/parser/ast/visitors | `spring-ai-vector-store/.../filter/` ANTLR4 generated parser |
| Vector store vendors | `vectorstores/{chroma,milvus,pinecone,qdrant,weaviate}` sibling module | `vector-stores/` 22 个 vendor module |
| RAG | `rag/` sibling module | `spring-ai-rag/` |
| MCP client | `mcp/` sibling module | `mcp/common/` + `mcp/mcp-annotations/` + `mcp/transport/` |
| Structured output / parser | `core/model/chat/parser.go`（List + 简单解析） | `spring-ai-model/.../converter/` 整套 `StructuredOutputConverter` + `BeanOutputConverter` / `ListOutputConverter` / `MapOutputConverter` |
| Prompt template | `core/model/chat/PromptTemplate` 轻量 | `chat/prompt/PromptTemplate` + `ChatPromptTemplate` + `SystemPromptTemplate` 系统 |
| Retry | `pkg/retry`（lynx workspace 下另一个 module） | `spring-ai-retry/` 独立 module |
| Vendor models | `models/{anthropic,google,openai}` 3 个 | `models/` 15 个 |
| Observability | `otel/{log,slog}` exporter + 直接 OTel API | Micrometer Observation pattern 深度整合 |
| Autoconfigure / Boot starter | ❌ 不做 | `auto-configurations/` + `starters/` 整套 |

---

## 3. Chat 抽象 — 接口形态

### 3.1 ChatModel / Client 层

**spring-ai：** 两层接口

```java
// 底层 SPI（vendor 实现这个）
public interface ChatModel extends Model<Prompt, ChatResponse>, StreamingChatModel {
    ChatResponse call(Prompt prompt);
    @Override Flux<ChatResponse> stream(Prompt prompt);
}

// 高层 facade（用户用这个）
public interface ChatClient {
    ChatClientRequestSpec prompt();
    ChatClientRequestSpec prompt(String content);
    ChatClientRequestSpec prompt(Prompt prompt);
    Builder mutate();
    // ... 嵌套很多 Spec 类型（PromptUserSpec / PromptSystemSpec / AdvisorSpec / CallResponseSpec / StreamResponseSpec）
}
```

ChatClient 是 fluent builder：`client.prompt("hi").user(...).system(...).advisors(...).call().chatResponse()` 这样写。

**lynx：** 也是两层，但是更扁

```go
// 底层 SPI
type Model interface {
    Call(ctx, *Request) (*Response, error)
    Stream(ctx, *Request) iter.Seq2[*Response, error]
    DefaultOptions() *Options
    Info() ModelInfo
}

// 高层 facade
type Client struct { /* ... */ }
func NewClient(model Model) (*Client, error)

func (c *Client) Chat() *ClientRequest
// ClientRequest 上链式：WithSystemPrompt / WithUserPrompt / WithMessages / WithTools / WithMiddlewares / Call() / Stream()
```

**差别：**
- spring-ai 用嵌套 Spec 类型分阶段（`PromptUserSpec` / `PromptSystemSpec` / `CallResponseSpec`）—— 每个阶段编译期限定能调什么
- lynx 一个 `ClientRequest` 类型上所有方法都能链 —— 形态更松，但少了类型安全

### 3.2 中间件模型 — Advisor vs Middleware

**spring-ai 的 Advisor**（around chain）：

```java
public interface Advisor extends Ordered {
    // ... CallAdvisor / StreamAdvisor / BaseAdvisor 派生
}

public interface CallAdvisor extends Advisor {
    ChatClientResponse adviseCall(ChatClientRequest request, CallAdvisorChain chain);
}
```

内置 advisor：`SafeGuardAdvisor`、`ToolCallAdvisor`、`SimpleLoggerAdvisor`、`MessageChatMemoryAdvisor`、`VectorStoreChatMemoryAdvisor`、`QuestionAnswerAdvisor`、`StructuredOutputValidationAdvisor`、`LastMaxTokenSizeContentPurger`、`RetrievalAugmentationAdvisor`（RAG）等。

**lynx 的 Middleware**：

```go
type CallMiddleware func(next CallHandler) CallHandler
type StreamMiddleware func(next StreamHandler) StreamHandler
```

内置：`chat.NewToolMiddleware`（self-driving tool loop）+ `chat/memory.NewMiddleware`（会话记忆）。

**两者本质等价**（都是装饰器模式 + around 链），差别：
- spring-ai 出货 ~10 个内置 advisor，覆盖 logging / safety / memory / RAG / validation / token-purge 各场景
- lynx 只有 2 个 middleware（tool loop + memory），其他场景靠用户自己写或用 `agent/toolpolicy` 的工具级装饰器
- Advisor 显式有 `Ordered`（spring 的 priority），lynx middleware 用 `WithMiddlewares(...)` 顺序就是注册顺序

### 3.3 ChatOptions

两边都很类似 —— 都有 `Model / Temperature / TopP / TopK / MaxTokens / Stop / FrequencyPenalty / PresencePenalty / Tools`。lynx 多 `Tools` 在 Options 上是 hook（让 tool middleware 看到 tools），spring-ai 通过 advisor 路径接 tools。

---

## 4. 多模态：embedding / image / audio / moderation

两边都覆盖这 4 个模态 + chat 共 5 种。形态对称：

| 模态 | spring-ai | lynx/core |
|---|---|---|
| Embedding | `EmbeddingModel.call(EmbeddingRequest) → EmbeddingResponse`；`Embedding` 值 + `EmbeddingOptions` | `embedding.Client.Embed()` chain + `Request` / `Response` / `Options` |
| Image | `ImageModel.call(ImagePrompt) → ImageResponse` | `image.Client` 同样 chain 风格 |
| Audio TTS | `audio/tts/` 子包 | `audio/tts/` 子包 |
| Audio STT | `audio/transcription/` | `audio/transcription/` |
| Moderation | `ModerationModel + ModerationPrompt + Moderation + Categories/CategoryScores` | `moderation.Client + Request/Response + Moderation result` |

**差别：**
- spring-ai 的 embedding 有 `BatchingStrategy` 抽象（把 documents 按 batch 切给 embedding model），lynx 把这个能力放在 `document.TokenCountBatcher`
- spring-ai 的 `DocumentEmbeddingModel` 是高层封装（输入 List<Document>，输出 embeddings 并写回 metadata），lynx 没单独抽这层 —— 用户自己接

---

## 5. Tool / function calling

### 5.1 抽象形态

**spring-ai 的 `ToolCallback`：**

```java
public interface ToolCallback {
    ToolDefinition getToolDefinition();
    default ToolMetadata getToolMetadata() { ... }
    String call(String toolInput);
    default String call(String toolInput, ToolContext toolContext) { ... }
}
```

加一整套配套：
- `tool/definition/` —— `ToolDefinition` 描述
- `tool/execution/` —— `ToolCallResultConverter` / `ToolExecutionException` / `ToolExecutionExceptionProcessor`
- `tool/resolution/` —— `ToolCallbackResolver` SPI（按 name 查 ToolCallback）+ `SpringBeanToolCallbackResolver` / `StaticToolCallbackResolver` / `DelegatingToolCallbackResolver`
- `tool/function/` —— 把 Java 方法变成 ToolCallback
- `tool/method/` —— 把任意 method（含注解）变成 ToolCallback
- `tool/annotation/` —— `@Tool` 等注解
- `tool/augment/` —— 增强（如自动加描述）

**lynx 的 `chat.Tool` / `CallableTool`：**

```go
type Tool interface { Definition() ToolDefinition }
type CallableTool interface {
    Tool
    Metadata() ToolMetadata
    Call(ctx, arguments string) (string, error)
}
```

加 `pkg/json` 做 schema 生成（从 Go struct 自动出 JSON schema），加 `mcp.Tool` 做 MCP 工具包装。**没有 ToolCallbackResolver 抽象**（lynx/agent 层的 `core.ToolGroupResolver` 是类似概念，但在 agent 层不在 core 层）。

**关键差异：**
1. spring-ai 的 `tool/` 子系统非常厚 —— 反射 Java method、Spring bean、注解、resolver、execution converter、exception processor 一应俱全
2. lynx 用 typed action（`core.TypedActionFunc[In, Out]`）+ `chat.NewToolMiddleware` 解决同样的需求，但要少很多代码 —— Go 反射 + 泛型 + json schema 直接可工作
3. spring-ai 的 `ToolCallResultConverter` 把任意 Java return 值变成 String 给 LLM；lynx 让 tool 自己负责返回 String

---

## 6. Memory / 会话记忆

### 6.1 spring-ai：3 层抽象

```java
public interface ChatMemory {
    String CONVERSATION_ID = "chat_memory_conversation_id";
    void add(String conversationId, Message message);
    void add(String conversationId, List<Message> messages);
    List<Message> get(String conversationId);
    void clear(String conversationId);
}

// 实现：MessageWindowChatMemory（按数量保留窗口）

public interface ChatMemoryRepository {
    // 持久化 store；ChatMemory 包一层这个用
}

// 实现：InMemoryChatMemoryRepository
// memory/ sibling module 里还有 jdbc / redis / cassandra 等实现
```

Advisor 侧用 `MessageChatMemoryAdvisor`（普通 message window）或 `VectorStoreChatMemoryAdvisor`（语义检索式 long-term memory）拼接到 ChatClient。

### 6.2 lynx：一个 middleware

```go
// core/model/chat/memory/
func NewMiddleware(store Store) (CallMiddleware, StreamMiddleware, error)
type Store interface { ... }
// 实现：NewInMemoryStore + MessageWindow store
```

Middleware 自己处理"prepend 历史 + append 新 message" 逻辑，store 只负责"按 conversation ID 增 / 查 / 删"。

**差别：**
- spring-ai 抽得更细 —— `ChatMemory` 是逻辑层（窗口策略），`ChatMemoryRepository` 是存储层
- spring-ai 已经有 vector-store backed memory（语义检索），lynx 没有内置
- spring-ai 的 memory 模块（sibling）已经实现 JDBC / Redis / Cassandra 几种 repository；lynx 只有 in-memory

---

## 7. Document / RAG pipeline

### 7.1 Document model

两边都很对称：

| | spring-ai | lynx/core |
|---|---|---|
| 核心类型 | `Document(id, text/media, metadata)` | `Document(ID, Text, Media, Metadata)` |
| Reader | `DocumentReader: Supplier<List<Document>>` | `Reader interface { Read(ctx) ([]*Document, error) }` |
| Writer | `DocumentWriter: Consumer<List<Document>>` | `Writer interface { Write(ctx, []*Document) error }` |
| Transformer | `DocumentTransformer` | `Transformer` |
| Formatter | `ContentFormatter` + `DefaultContentFormatter` | `Formatter` + `SimpleFormatter` |
| Batcher | embedding 模块里的 `BatchingStrategy` | `Batcher` interface + `TokenCountBatcher` 实现 |
| ID 生成 | `IdGenerator` SPI（`RandomIdGenerator` 内置） | `IDGenerator` interface（`UUIDGenerator` 内置） |
| Metadata mode | `MetadataMode.{ALL,EMBED,INFERENCE,NONE}` | 同款 |

**核心差别：document readers**

spring-ai 出货 PDF reader（`document-readers/pdf-reader/`），还有几个其他 reader 在 sibling module 里（Markdown / JSON / Tika）。lynx/core 只有 `TextReader` + `JSONReader`。

### 7.2 RAG pipeline

**spring-ai：** RAG 是完整子系统，分 4 阶段

```
preretrieval/
├── query/transformation/  — CompressionQueryTransformer / RewriteQueryTransformer / TranslationQueryTransformer
└── query/expansion/        — MultiQueryExpander
retrieval/
├── search/                 — VectorStoreDocumentRetriever / DocumentRetriever
└── join/                   — ConcatenationDocumentJoiner / DocumentJoiner
postretrieval/
└── document/               — DocumentPostProcessor
generation/
└── augmentation/           — ContextualQueryAugmenter / QueryAugmenter

advisor/RetrievalAugmentationAdvisor   ← 接入 ChatClient 的入口
```

加上 `Query` 抽象（带 history / context / metadata）。

**lynx：** `rag/` sibling module，覆盖大致相同的能力但更平

```
query_transformer_{compression,rewrite,translation}.go
query_expander_multi.go
query_augmenter_contextual.go
document_retriever_vectorstore.go
document_refiner_{dedup,rank}.go     ← lynx 多了 refiner 概念（spring-ai 的 DocumentPostProcessor 类似）
pipeline.go + pipeline_middleware.go ← 接入 chat client 的入口
```

**对照来看：**
- 数量上 lynx ~15 个 RAG 组件 vs spring-ai ~20 个（含 4 阶段层级）
- spring-ai 的 4 阶段抽象更明确（preretrieval / retrieval / postretrieval / generation），lynx 是扁平 + 自由组合
- spring-ai 走 Advisor 接到 ChatClient；lynx 走 middleware 接到 chat client
- spring-ai 没有 lynx 的 `DeduplicationRefiner` / `RankRefiner`（lynx 多两个）
- spring-ai 没有专门的 `DocumentJoiner`（lynx 的 pipeline 直接拼）

---

## 8. Vector store

### 8.1 接口

**spring-ai：**
```java
public interface VectorStore extends DocumentWriter, VectorStoreRetriever {
    void add(List<Document> documents);
    void delete(List<String> idList);
    void delete(Filter.Expression filterExpression);
    // ...
}

public interface VectorStoreRetriever {
    List<Document> similaritySearch(SearchRequest request);
    // ...
}
```

`SearchRequest` 包：`query / topK / similarityThreshold / filterExpression`。

**lynx：** 三接口 ISP 拆分

```go
type Creator interface { Create(ctx, *CreateRequest) error }
type Retriever interface { Retrieve(ctx, *RetrievalRequest) ([]*Document, error) }
type Deleter interface { Delete(ctx, *DeleteRequest) error }
type Store interface {  // 整合
    Extension
    Creator; Retriever; Deleter
    Info() StoreInfo
}
```

**差别：**
- lynx ISP 拆 3 个细接口 + 1 个组合接口；spring-ai 是 `VectorStore` + `VectorStoreRetriever`（2 个）
- spring-ai 多了 `AbstractVectorStoreBuilder` + 一堆 observation 类（每个 vector store 都有 trace span / metric）
- spring-ai 出货 `SimpleVectorStore`（in-memory 实现）+ `SimpleVectorStoreFilterExpressionEvaluator`；lynx/core 不出货任何 vector store 实现（用户挂 sibling `vectorstores/*`）

### 8.2 Filter mini-language

两边都有过滤表达式语言。差别在实现：

| | spring-ai | lynx |
|---|---|---|
| 语法 | SQL-like：`type == 'comedy' AND year >= 2020` | 同款 |
| 解析 | **ANTLR4 generated** parser（grammar 在 `vectorstore/filter/antlr4/`） | **手写** lexer + recursive descent parser |
| AST | 内置 visitor pattern | 同款 |
| 转换到 vendor 格式 | `FilterExpressionConverter` 接口，每个 vendor 实现 | `Visitor` 接口 + 每个 vendor 实现（chroma / milvus / pinecone / qdrant / weaviate） |
| 文本解析 API | `FilterExpressionTextParser.parse(...)` | `filter.Parse(...)` / `filter.ParseAndAnalyze(...)` |

**取舍：**
- ANTLR4 自动生成 lexer/parser → 改语法只动 grammar 文件；但运行时多带 ANTLR runtime
- 手写 parser → 没有运行时依赖，但每次改语法都要动 lexer + parser
- 两边的 visitor / converter 形态本质一样

### 8.3 Vendor 集成

**spring-ai：22 个 vector store vendor module**
azure-cosmos-db / azure / bedrock-knowledgebase / cassandra / chroma / coherence / couchbase / elasticsearch / gemfire / mariadb / milvus / mongodb-atlas / neo4j / opensearch / oracle / pgvector / pinecone / qdrant / redis / s3 / typesense / weaviate

**lynx：5 个** chroma / milvus / pinecone / qdrant / weaviate

**这是 lynx 最大的"广度差距"**。spring-ai 几乎覆盖了主流的全部，lynx 选了 5 个最常用的。

---

## 9. MCP 集成

**spring-ai：** 3 个 module
- `mcp/common/` — 把 MCP tool 包成 `ToolCallback`（`SyncMcpToolCallback` / `AsyncMcpToolCallback`），把 `ToolCallback` 暴露成 MCP tool（`SyncMcpToolCallbackProvider`）
- `mcp/mcp-annotations/` — `@McpTool` / `@McpToolParam` 注解
- `mcp/transport/` — 传输层

**lynx：** `mcp/` sibling module
- `Provider / Source / NamingFunc` — 把 MCP server 的 tool 列表暴露成 `[]chat.Tool`
- `Tool / ToolCallError` — 把单个 MCP tool 包成 `chat.CallableTool`
- `SamplingHandler / SamplingViaChatClient` — 让 MCP server 反向调 lynx 的 chat client（sampling）
- `RegisterTools / WithMeta / MetaFromContext` — 把 lynx tools 注册到 MCP server / 元数据透传

**差别：**
- spring-ai 走注解（`@McpTool`），lynx 走构造（`NewTool(ToolConfig{...})`）
- spring-ai 有 sync / async 两套；lynx 只有 sync + Go-style iter.Seq 流式
- 两边都覆盖 client 和 server 两个方向

---

## 10. Structured output / 类型化输出

**spring-ai：完整子系统**
```java
package org.springframework.ai.converter;
// StructuredOutputConverter<T> 接口
// 实现：BeanOutputConverter (用 Jackson 反序列化 Java POJO)
//      ListOutputConverter (CSV / 列表)
//      MapOutputConverter (JSON object → Map)
//      AbstractMessageOutputConverter (基类)
//      MarkdownCodeBlockCleaner / ThinkingTagCleaner / WhitespaceCleaner — 预处理 LLM 输出
```

ChatClient 上 `.entity(MyBean.class)` 直接出 typed 对象。

**lynx：** 只有 `core/model/chat/parser.go` 一个轻量 `ListParser`，处理简单列表场景。

**差距：** lynx 没有 spring-ai 那种"LLM 输出 → typed Java/Go struct"的开箱即用通道。lynx 的等价用法是 action body 里手动 `json.Unmarshal([]byte(text), &result)`。**这是个真 gap**，闭合大概要：
1. `StructuredOutputParser[T any]` 接口
2. `JSONParser[T]` —— 用 `pkg/json` 出 schema + `json.Unmarshal` 反序列化
3. `ChatClientRequest.Entity[T]() (T, error)` 上层 API
4. 一个 `MarkdownStripParser` decorator 剥 ``` 围栏
5. 可能加 retry：解析失败时把错误回填给 LLM 让它修正

---

## 11. Prompt template

**spring-ai：** 完整模板系统
- `PromptTemplate` —— StringTemplate 引擎（`spring-ai-template-st/` 独立 module）
- `ChatPromptTemplate` —— 整个 prompt 模板化
- `SystemPromptTemplate` / `AssistantPromptTemplate` —— 按 message 类型模板化
- `PromptTemplateActions` / `PromptTemplateChatActions` / 等等 —— 多层 actions

**lynx：** `core/model/chat/PromptTemplate` 单一类型，`{key}` 占位符替换。

**差别：** spring-ai 用 StringTemplate（Java 模板引擎），支持复杂控制结构；lynx 是简单字符串 placeholder。一般 prompt 场景 lynx 够用；复杂模板（条件、循环、include）就得用户自己拼。

---

## 12. Observability

**spring-ai：** 深度整合 Micrometer Observation
- 每个 chat call 出 `ChatModelObservationConvention` + `ChatModelObservationContext`
- ChatClient 出 `ChatClientObservationConvention`
- Advisor 链每一步出 `AdvisorObservationContext`
- VectorStore 出 `VectorStoreObservationConvention`
- 这些都通过 ObservationRegistry 接 Micrometer，进而到 OTel / Prometheus

**lynx：** 直接用 OTel API
- `otel/log` + `otel/slog` 出 span 给标准库 logger
- chat / planner / action 直接打 OTel span 和 attribute
- 没有 advisor / per-stage 那种细粒度切面

**差别：** spring-ai 走 Micrometer 抽象（用户配 ObservationRegistry），lynx 走原生 OTel（用户配 TracerProvider）。**两边覆盖度差不多**，但 spring-ai 的 metric / event / span 三件套用 Micrometer 统一更清晰。

---

## 13. Retry

**spring-ai：** `spring-ai-retry/`（独立 module）
- `TransientAiException` / `NonTransientAiException` —— 错误分类
- `RetryUtils` —— 基于 Spring Retry 的工具方法

**lynx：** `pkg/retry`（workspace 另一个 module）
- 提供退避 + 重试 + jitter + ctx 取消

**差别：** spring-ai 显式分类 `Transient` vs `NonTransient`（前者可重试，后者直接报错）；lynx 的 retry 默认所有错误都重试，可以传 `RetryOn(fn)` 自定义判断。spring-ai 这套**显式错误分类**对 LLM 集成更友好（404 / 401 不该重试，429 / 503 该重试）—— 是 lynx 可以借鉴的设计。

---

## 14. spring-ai 有 / lynx/core 没有

按必要性排序：

### 14.1 强相关（值得考虑加）

1. **Structured output converter** (`spring-ai-model/converter/`)
   - LLM 输出 → typed struct，配套预处理（剥 markdown、剥 thinking tag、空格清理）
   - 真 gap；动手写约 100 LOC + 测试
2. **`Transient/NonTransient` 错误分类 in retry**
   - 让 retry 智能区分 429 vs 401
   - 30 LOC + 在各 vendor adapter 里分类抛错
3. **`SimpleVectorStore`（in-memory vector store）**
   - 单元测试 / demo 场景非常有用；现在用户得挂 chroma 等才能玩 vector search
   - 100~150 LOC
4. **PDF document reader**
   - 现在 lynx 只有 text / JSON reader；PDF 是 RAG 最常见输入格式
   - 但要带 PDFBox 等大依赖，可放在 `core/document` 外的 sibling module

### 14.2 弱相关（lynx 哲学不做）

5. **大量 vendor vector-store**（22 vs 5）
   - 看用户实际需要；可按需扩
6. **大量 vendor model**（15 vs 3）
   - 同上
7. **`spring-ai-template-st`** StringTemplate 完整模板引擎
   - 复杂度有限，用户需要可自己挂 `text/template`
8. **Advisor 体系的 `SafeGuardAdvisor` / `StructuredOutputValidationAdvisor` 等内置 advisor**
   - 内置 advisor 多覆盖几个常见场景，但每个都几十行；lynx 走"用户写 middleware"的路线一致就好
9. **`ToolCallbackResolver` SPI**
   - spring-ai 这个是给"按 name 解析 Spring bean 工具"用的，lynx 没有 Spring DI 不需要；agent 层的 `ToolGroupResolver` 已经覆盖了"按 role 解析工具组"
10. **Boot autoconfigure / starter**
    - lynx 不做 Boot，user code 直接 `chat.NewClient(model)` 这种显式构造已经够清晰
11. **Memory `ChatMemoryRepository` 的 JDBC / Redis / Cassandra 实现**
    - lynx 让用户自己实现 `Store` 接口；不内置 RDBMS 集成

---

## 15. lynx/core 有 / spring-ai 没有

1. **HTN planner** —— 在 agent 层不在 core 层；但 lynx 的整体能力比 spring-ai 多一个规划范式
2. **Document.Bind dual-binding** —— lynx blackboard 的特性，spring-ai 没有 typed 类似物
3. **Go 1.26 idiom** —— `iter.Seq2` 流式（spring-ai 用 Reactor `Flux`）、`errors.AsType[T]`、`new(value)` 等现代 Go
4. **Filter mini-language 不依赖 antlr 运行时** —— 手写 parser ~500 LOC，零运行时依赖；spring-ai 要带 ANTLR4 runtime
5. **ISP 拆分严格** —— `vectorstore.Store` 拆成 `Creator/Retriever/Deleter`，spring-ai 是 `VectorStore extends DocumentWriter, VectorStoreRetriever` 两接口
6. **Token middleware 自驱动** —— `chat.ToolMiddleware` 默认就在 ChatClient 上，spring-ai 要显式注册 `ToolCallAdvisor`
7. **lynx 的 RAG 多 `Refiner`（rank / deduplication）** —— spring-ai 没有专门的 refiner 概念

---

## 16. Gap 闭合清单（lynx 视角）

按 ROI 排序：

| # | 项 | 工作量 | 评论 |
|---|---|---|---|
| 1 | **Structured output converter**（`core/model/chat/converter/`）| 低（~150 LOC + 测试）| 最高 ROI；用户场景非常普遍 |
| 2 | **`SimpleVectorStore` in-memory 实现** | 低（~120 LOC） | demo / test 场景；类比 spring-ai 的 SimpleVectorStore |
| 3 | **Retry 错误分类**（`pkg/retry` 加 `Transient/NonTransient` 区分） | 低（~30 LOC + 各 vendor adapter 接口） | LLM rate-limit / auth-error 智能重试 |
| 4 | **PDF reader**（在 `core/document` 外加 `document-readers/pdf` sibling） | 中（PDFBox 依赖 + 大约 200 LOC） | RAG 场景常用 |
| 5 | **Markdown code-block cleaner / thinking-tag cleaner** | 低（~50 LOC） | 配 #1 用，剥 LLM 输出包装层 |
| 6 | **Vector-store backed chat memory** | 中（基于现有 memory middleware 扩） | 语义检索式长期记忆 |
| 7 | **更多 vendor vector-store**（按真实使用情况扩） | 中-高 | 不紧急；用户报需求再补 |
| 8 | **更多 vendor model**（DeepSeek / Mistral / Ollama 等） | 中 | 同上 |

---

## 17. 战略定位总结

**spring-ai 的赌注：** "AI 是 Spring 应用的一等组件" —— autoconfigure / starter / 多 vendor / 大量内置 advisor，做 batteries-included framework。代价：依赖庞大、抽象层多、上手要学 Spring Boot + Advisor pattern。

**lynx/core 的赌注：** "AI 是基础库，工程化由 host app 负责" —— 薄抽象 / vendor 分到 sibling module / observation 走原生 OTel / 没有 DI 容器。代价：开箱集成少（PDF / 大量 vector store / structured output 都缺），用户得自己拼。

两者在**底层抽象上是同构的** —— ChatModel / Embedding / Document / VectorStore / RAG / MCP 概念都对得上，且 lynx 的 ISP 拆分和 Go-idiom 化形态在某些地方反而更清爽。**真正的差距在"丰富度"**：spring-ai 出货 22 个 vector-store + 15 个 vendor model + 完整的 structured output 体系 + advisor 生态；lynx 选了核心 5+3+少量 advisor 出货。

如果要让 lynx/core 更适合"开箱即用"，#1（structured output）+ #2（SimpleVectorStore）+ #3（retry classification）三项是性价比最高的闭环。其余的 vendor 集成应该按真实需求驱动，不必为了对齐 spring-ai 而扩张。

---

*对比结束。双方 HEAD 截至 2026-05-12。*
