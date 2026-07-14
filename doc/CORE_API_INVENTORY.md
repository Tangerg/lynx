# Core 公共 API 与消费清单

> 基线日期：2026-07-14
> 代码基线：`5a4e828d3`
> Go：`go1.26.4 darwin/arm64`
> 对应计划：[`CORE_ARCHITECTURE_EXECUTION_PLAN.md`](./CORE_ARCHITECTURE_EXECUTION_PLAN.md) P0-03

本文记录 Core 重构前的编译器可见公共面、workspace 直接消费关系和后续迁移批次。它解决“改什么、谁会受影响、何时删除”的问题；它不是永久兼容承诺。P7 建立机械 API diff baseline 后，以工具输出判断签名兼容性。

执行状态：P4、P5-01、P5-02、P5-03、P5-04 与 P5-05 已于 2026-07-14 完成。五个旧 `core/model/<modality>` 路径已无兼容层地直接移动到 Core 顶层，五个 modality SPI 也已完成最小化；provider reference data 已归入公开 `models/catalog`，credential 已回归各 provider 配置，tokenizer 已成为独立 module。下文数量表和未特别标注的声明列表仍是重构前基线，已迁 package 会同时标明当前路径。

## 1. 口径与结论

扫描口径：

- 包范围：`core` module 下 24 个非 `internal` package；`core/internal/arch` 不属于公共面。
- 标识符：exported package declarations，以及 exported type 的字段、接口方法和具体方法。
- 消费关系：workspace 中 `.go` 文件对 Core package 的直接 import；生产文件与 `_test.go` 分开计数。
- 模块：按最近的 workspace module root 归属；vendor、生成缓存和外部仓库不计入。
- 版本：仓库当前没有 Git tag；workspace module 通过 `v0.0.0-*` pseudo-version 声明 Core，再由 `go.work` 绑定本地源码。因此本轮可以做 v0 破坏，但仍必须记录迁移和发布顺序。

基线结论：

| 指标 | 数量 |
|---|---:|
| Core 公共 package | 24 |
| Exported package-level declarations | 472 |
| Exported fields/interface methods/concrete methods | 733 |
| Exported identifiers 合计 | 1,205 |
| Workspace Core direct-import 关系 | 830（516 生产 / 314 测试） |
| 含 Core direct import 的唯一文件 | 501（308 生产 / 193 测试） |
| 直接消费旧 `core/model/*` 的 workspace module | 9 |
| 需要迁移的 model provider 目录 | 38 |
| 需要验证的 vectorstore backend 目录 | 27（25 个实现 + 2 个 alias） |

这不是一次简单的 rename：`core/model/chat` 一项就有 297 个 exported identifiers，并被 173 个生产文件、115 个测试文件直接 import。全 Core 共涉及 501 个唯一文件和 830 条 package import 关系；所有迁移必须按计划中的“新路径限时并存”执行，不能在单个提交里盲目全局替换。

## 2. Package 公共面与消费热度

“顶层/成员”分别表示 package-level declaration 数，以及 exported type 所拥有的 exported 字段/方法数。消费计数只统计 Core 以外的 workspace 文件；同一文件 import 多个 Core package 时会在多行出现，因此各行之和是 830 条关系，不是唯一文件数。

| Package | 顶层 | 成员 | 生产/测试 import 文件 | 生产消费模块 | 计划归属 |
|---|---:|---:|---:|---|---|
| `document` | 42 | 64 | 38 / 6 | documentreaders/*, rag, vectorstores | P4 同路径纵向切片 |
| `document/id` | 5 | 3 | 0 / 0 | — | P4 → documentpipeline |
| `evaluation` | 13 | 21 | 0 / 0 | — | P3 → rag/evaluation |
| `media` | 2 | 9 | 10 / 12 | app/runtime, models | P1 同路径纵向切片 |
| `model` | 21 | 35 | 94 / 92 | agent, app/runtime, models | P2/P3/P5/P6 拆分后删除旧职责 |
| `model/audio/transcription` | 24 | 40 | 10 / 9 | models | P5 → transcription |
| `model/audio/tts` | 28 | 43 | 9 / 8 | models | P5 → speech |
| `model/chat` | 92 | 205 | 173 / 115 | a2a, agent, app/runtime, chathistory, mcp, models, rag, tools | P1–P3 → chat/chatclient/tools，P6 删除旧包 |
| `model/chat/conversation` | 3 | 0 | 4 / 2 | agent | P3 → agent/chatclient context |
| `model/chat/history` | 14 | 16 | 19 / 10 | agent, app/runtime, chathistory | P3 → chathistory |
| `model/chat/middleware/history` | 1 | 0 | 3 / 1 | agent | P3 → chathistory integration |
| `model/chat/middleware/logger` | 7 | 3 | 0 / 0 | — | P3 删除 |
| `model/chat/middleware/safeguard` | 10 | 5 | 0 / 0 | — | P3 → chatclient/middleware/safeguard |
| `model/embedding` | 33 | 52 | 39 / 23 | app/runtime, models, vectorstores | P5 → embedding |
| `model/image` | 29 | 48 | 10 / 10 | models | P5 → image |
| `model/moderation` | 26 | 62 | 2 / 2 | models | P5 → moderation |
| `tokenizer` | 9 | 8 | 2 / 0 | models | P5 → tokenizer module |
| `vectorstore` | 20 | 19 | 28 / 2 | rag, vectorstores | P4 同路径纵向切片 |
| `vectorstore/filter` | 22 | 12 | 1 / 19 | rag | P4 保留公共门面 |
| `vectorstore/filter/ast` | 10 | 48 | 47 / 2 | rag, vectorstores | P4 收敛到 filter 公共语义面 |
| `vectorstore/filter/lexer` | 2 | 4 | 0 / 0 | — | P4 → internal |
| `vectorstore/filter/parser` | 4 | 4 | 0 / 0 | — | P4 → internal |
| `vectorstore/filter/token` | 49 | 28 | 27 / 1 | vectorstores | P4 → internal；先迁 backend visitors |
| `vectorstore/filter/visitors` | 6 | 4 | 0 / 0 | — | P4 → internal/provider side |

## 3. Exported package-level identifiers

下面列出全部 472 个 package-level exported declaration。字段、接口方法和具体方法随其 owner type 作为一个迁移单元；第 4 节列出会发生结构性变化的高风险成员。精确签名可用 `go doc -all <package>` 从本基线 commit 复现。

### document（P4-01 当前公共面）

```text
Document, Reader, Writer, NewDocument
```

P4-01 已将所有 formatter/transformer/batcher/ID generator 运行时策略迁入 `documentpipeline`，将 Text/JSON reader 迁入 `documentreaders`；`core/document/id` 已删除。

### documentpipeline（P4-01 新 module）

```text
Batcher, BoundFormatter, FileWriter, FileWriterConfig, Formatter, FormatterFunc,
IDAssigner, IDAssignerConfig, MetadataMode, Nop, SimpleFormatter,
SimpleFormatterConfig, Splitter, SplitterConfig, TextSplitter,
TextSplitterConfig, TokenCountBatcher, TokenCountBatcherConfig, TokenSplitter,
TokenSplitterConfig, Transformer, NewFileWriter, NewIDAssigner, NewNop,
NewSimpleFormatter, NewSplitter, NewTextSplitter, NewTokenCountBatcher,
NewTokenSplitter

documentpipeline/id: Generator, Sha256Generator, UUIDGenerator,
NewSha256Generator, NewUUIDGenerator

documentreaders: JSONReader, TextReader, NewJSONReader, NewTextReader
```

### evaluation

```text
DefaultPassThreshold, ErrNilRequest, CompositeEvaluator, Evaluator,
FactCheckingEvaluator, FactCheckingEvaluatorConfig, RelevancyEvaluator,
RelevancyEvaluatorConfig, Request, Response, NewCompositeEvaluator,
NewFactCheckingEvaluator, NewRelevancyEvaluator
```

### media

```text
Media, NewMedia
```

### model

```text
MetricGenAIClientOperationDuration, MetricGenAIClientTokenUsage, APIKey,
CallHandler, CallHandlerFunc, CallMiddleware, ControlFlowError, Halt,
MiddlewareChain, Model, OperationMetrics, RateLimit, StreamHandler,
StreamHandlerFunc, StreamMiddleware, StreamingModel, Usage,
IsControlFlowError, NewAPIKey, NewMiddlewareChain, RecordOperationMetrics
```

P5-03 已删除 `APIKey`/`NewAPIKey`；Core 不再统一 provider credential 或 secret
展示。静态 key 是各 provider config 的字符串，特殊认证由 provider adapter 建模，
应用 runtime 独立负责持久化和脱敏。其余冻结泛型 Model/middleware 表面仍按 P6 删除。

### model/audio/transcription

```text
Client, ClientCaller, ClientRequest, Handler, HandlerFunc, Middleware,
MiddlewareChain, Model, ModelMetadata, Options, Request, Response,
ResponseMetadata, Result, ResultMetadata, MergeOptions, NewClient,
NewClientFromRequest, NewClientRequest, NewMiddlewareChain, NewOptions,
NewRequest, NewResponse, NewResult
```

P5-02 后当前路径为 `core/transcription`，公共面为：

```text
Model, ModelFunc, Options, Request, Response, ResponseMetadata, Result,
ResultMetadata, MergeOptions, NewOptions, NewRequest, NewResponse, NewResult
```

### model/audio/tts

```text
CallHandler, CallHandlerFunc, CallMiddleware, Client, ClientCaller,
ClientRequest, ClientStreamer, MiddlewareChain, Model, ModelMetadata, Options,
Request, Response, ResponseMetadata, Result, ResultMetadata, StreamHandler,
StreamHandlerFunc, StreamMiddleware, MergeOptions, NewClient,
NewClientFromRequest, NewClientRequest, NewMiddlewareChain, NewOptions,
NewRequest, NewResponse, NewResult
```

P5-02 后当前路径为 `core/speech`，公共面为：

```text
Model, ModelFunc, Streamer, StreamFunc, Options, Request, Response,
ResponseMetadata, Result, ResultMetadata, MergeOptions, NewOptions,
NewRequest, NewResponse, NewResult
```

同步 Model 与 Streamer 互不嵌入，provider 只实现真实能力。

### model/chat

```text
FinishReasonContentFilter, FinishReasonLength, FinishReasonNull,
FinishReasonOther, FinishReasonStop, FinishReasonToolCalls,
MessageTypeAssistant, MessageTypeSystem, MessageTypeTool, MessageTypeUser,
ModalityAudio, ModalityImage, ModalityPDF, ModalityText, ModalityVideo,
PartKindReasoning, PartKindText, PartKindToolCall
AnyParser, AssistantMessage, CallHandler, CallHandlerFunc, CallMiddleware,
Client, ClientCaller, ClientRequest, ClientStreamer, FinishReason, JSONParser,
Limits, ListParser, MapParser, Message, MessageList, MessageParams, MessageType,
MiddlewareChain, Modalities, Modality, Model, ModelInfo, ModelMetadata, Options,
OutputPart, PartKind, Pricing, PromptTemplate, RateLimit, Reasoning,
ReasoningPart, Request, Response, ResponseAccumulator, ResponseMetadata,
Result, ResultMetadata, StreamHandler, StreamHandlerFunc, StreamMiddleware,
StructuredParser, SystemMessage, TextPart, Tool, ToolCallPart, ToolDefinition,
ToolMessage, ToolReturn, Usage, UserMessage
CostOf, MergeOptions, NewAssistantMessage, NewClient, NewClientFromRequest,
NewClientRequest, NewJSONParser, NewListParser, NewMapParser, NewMessage,
NewMiddlewareChain, NewOptions, NewPromptTemplate, NewRequest, NewResponse,
NewResponseAccumulator, NewResult, NewSystemMessage, NewTool, NewToolMessage,
NewUserMessage, UnmarshalMessage, WrapParserAsAny
```

P5-03 已从该冻结包删除 `ModelInfo`、`Pricing`、`Reasoning`、`Limits`、
`Modality`/`Modalities` 与 `CostOf`；`ModelMetadata` 暂仅保留 provider identity，
随旧 Chat 包在 P6 删除。当前 reference data 公共面位于 `models/catalog`：

```text
Model, Provider, Pricing, Usage, Reasoning, Limits, Modality, Modalities,
Lookup, Models, Get, CostOf
```

### model/chat/conversation

```text
IDKey, ID, Stamp
```

### model/chat/history

```text
ErrListingUnsupported, Clearer, Counter, InMemoryStore, Lister,
MessageWindowStore, Reader, Replacer, Store, Writer, Count, NewInMemoryStore,
NewMessageWindowStore, Replace
```

### model/chat/middleware/history

```text
NewMiddleware
```

### model/chat/middleware/logger

```text
Logger, SlogLoggerOption, NewMiddleware, NewSlogLogger, WithSlogErrorLevel,
WithSlogRequestLevel, WithSlogResponseLevel
```

### model/chat/middleware/safeguard

```text
ErrUnsafeContent, ScopeBoth, ScopeInput, ScopeOutput, Matcher, Options, Scope,
SubstringMatcherOptions, NewMiddleware, NewSubstringMatcher
```

### model/embedding

```text
Audio, EncodingFormatBase64, EncodingFormatFloat, Image, Text, Video,
Client, ClientCaller, ClientRequest, EncodingFormat, Handler, HandlerFunc,
Middleware, MiddlewareChain, ModalityType, Model, ModelMetadata, Options,
Request, Response, ResponseMetadata, Result, ResultMetadata, GetDimensions,
MergeOptions, NewClient, NewClientFromRequest, NewClientRequest,
NewMiddlewareChain, NewOptions, NewRequest, NewResponse, NewResult
```

上框为重构前基线。P5-01 后当前路径为 `core/embedding`，公共面为：

```text
Client, DimensionFunc, Dimensioner, EncodingFormat, ModalityType, Model,
ModelFunc, Options, Request, Response, ResponseMetadata, Result,
ResultMetadata, MergeOptions, NewClient, NewOptions, NewRequest, NewResponse,
NewResult, ProbeDimensions, ResolveDimensions
```

`Model` 只含 `Call`；`Dimensioner` 独立且显式返回错误。旧 `ClientRequest`、
`ClientCaller`、handler/middleware/chain、`ModelMetadata`、`GetDimensions` 和
全局维度 cache 已物理删除，所有真实 provider/vectorstore/runtime 消费点已切换，
未保留 alias、bridge 或 deprecated wrapper。证据：`7cd3865c3`。

### model/image

```text
ResponseFormatB64JSON, ResponseFormatURL, Client, ClientCaller, ClientRequest,
Handler, HandlerFunc, Image, Middleware, MiddlewareChain, Model, ModelMetadata,
Options, Request, Response, ResponseFormat, ResponseMetadata, Result,
ResultMetadata, MergeOptions, NewClient, NewClientFromRequest,
NewClientRequest, NewImage, NewMiddlewareChain, NewOptions, NewRequest,
NewResponse, NewResult
```

P5-02 后当前路径为 `core/image`，公共面为：

```text
Image, Model, ModelFunc, Options, Request, Response, ResponseFormat,
ResponseMetadata, Result, ResultMetadata, MergeOptions, NewImage, NewOptions,
NewRequest, NewResponse, NewResult
```

### model/moderation

```text
Categories, Client, ClientCaller, ClientRequest, Handler, HandlerFunc,
Middleware, MiddlewareChain, Model, ModelMetadata, Options, Request, Response,
ResponseMetadata, Result, ResultMetadata, Verdict, MergeOptions, NewClient,
NewClientFromRequest, NewClientRequest, NewMiddlewareChain, NewOptions,
NewRequest, NewResponse, NewResult
```

P5-02 后当前路径为 `core/moderation`，公共面为：

```text
Categories, Model, ModelFunc, Options, Request, Response, ResponseMetadata,
Result, ResultMetadata, Verdict, MergeOptions, NewOptions, NewRequest,
NewResponse, NewResult
```

四个 modality 原有 Client/caller/request/streamer builder、handler/middleware/
chain 和 `ModelMetadata` 已直接删除；25 个具体 provider 与 6 个 facade 已迁移，
无 alias、bridge 或 deprecated wrapper。证据：`c27886f59`。

### tokenizer

```text
Decoder, Encoder, Estimator, MediaEstimator, TextEstimator, Tiktoken,
Tokenizer, NewDefaultTiktoken, NewTiktoken
```

P5-04 后 `core/tokenizer` 已删除。当前独立 module 公共面为：

```text
tokenizer: Decoder, Encoder, TextEstimator, Tokenizer
tokenizer/tiktoken: Tokenizer, New, NewDefault
```

无生产消费者的旧 `Estimator`/`MediaEstimator` 与不准确的媒体字节估算直接删除；
DocumentPipeline 只接收实际调用的 `TextEstimator`，tiktoken 第三方依赖仅存在于
实现 module。证据：`687df9b60`、`6953b45da`、`c0b679029`。

### vectorstore（P4-02 至 P4-06 当前公共面）

```text
AcceptAllScores, DefaultTopK, MaxSimilarityScore, MinSimilarityScore,
ErrEmptyDocuments, ErrMissingFilter, FilterDeleter, IDDeleter, Indexer, Match,
Searcher, SearchRequest, NewDocumentWriter
```

`Match` 只包含 `Document *document.Document` 与 `Score float64`。`Document.Score` 已删除，RAG 在自身边界使用 `Candidate` 并显式完成映射。旧 `Store`、`Creator`、`Retriever`、`Deleter`、request wrapper、metadata/native client 探测面均已删除；能力由四个单方法接口独立表达，`SearchRequest` 是带 `Validate` 的普通值。

### vectorstore/filter（P4-07 当前公共面）

```text
OpEqual, OpNotEqual, OpLess, OpLessEqual, OpGreater, OpGreaterEqual, OpAnd,
OpOr, OpNot, OpIn, OpLike, OpIs, LiteralString, LiteralNumber, LiteralBool,
LiteralNull, AtomicExpr, BinaryExpr, ComputedExpr, Expr, ExprBuilder, Ident,
IdentifierValue, IndexExpr, ListLiteral, ListValue, Literal, LiteralKind,
LiteralValue, Number, Operator, Position, UnaryExpr, And, EQ, GE, GT, In,
Index, IsNull, IsNotNull, LE, LT, Like, NE, NewExprBuilder, NewIdent,
NewListLiteral, NewLiteral, NewLiterals, Not, Or, Parse, Validate
```

公开树只包含 token-free 语义节点。`Parse` 负责 parse + validate + simplify，手工构造的树通过 `Validate` 校验；残缺或 typed-nil 节点稳定返回错误。

### vectorstore/filter/internal/*（P4-07 后非公共实现）

```text
ast, lexer, parser, token, visitors
```

原 `filter/{ast,lexer,parser,token,visitors}` import path 已物理删除，编译器实现只允许由根 package 使用；provider adapter 只依赖根 `filter.Expr`、语义节点和 `Operator`。

## 4. 高风险 type 成员

以下成员不是完整 733 项抄录，而是本轮会改变 shape、方法集或序列化行为的类型。未列出的成员仍随第 3 节 owner type 一起迁移，不能视为兼容承诺。

| Type | 当前高风险成员 | 目标影响 |
|---|---|---|
| `media.Media` | `Data any`, `MimeType *mime.MIME`, `MarshalJSON`, `UnmarshalJSON` | P1 改为 tagged source；全 workspace 同阶段切换 |
| `document.Document` | ~~`Score`, `Formatter`, `EnsureID`, `Format*`~~（P4-01 已删除） | 当前仅有 `ID/Text/Media/Metadata` 与 `Validate`；实现已移到 documentpipeline/vectorstore.Match |
| `model.Model` | generic `Call` + `DefaultOptions` | P2/P5 删除名义泛型层次 |
| `model.StreamingModel` | generic Call/Stream 复合能力 | P2 拆成独立 Streamer |
| `model.MiddlewareChain` | `Clone`, `WithCall`, `WithStream`, `Build*` | 只保留必要的函数式组合算法 |
| `chat.Message` | sealed 多态方法集 | P1 改为 tagged value 与明确 discriminator |
| `chat.Request` | `Messages`, `Options`, `Params`, `Tools`, `Get/Set` | 移除运行时 Tool 和任意 Params；只留 JSON-safe extensions |
| `chat.Response`/`Result` | 单 Result、ToolMessage synthetic result | P1 保留多 Choice；P3 tool-loop Event 外移 |
| `chat.Model` | `DefaultOptions`, `Metadata` + Call/Stream | P2 只强制单方法 Call |
| `chat.Tool` | `Definition`, `Call` | P1/P3 可执行契约迁到 tools；Core 只留 wire 词汇 |
| `embedding.Model` | ~~Call、Dimensions、DefaultOptions、Metadata~~（P5-01 已删除复合方法集） | 当前只含 `Call`；独立 `Dimensioner` 返回 `(int, error)`，探测 helper 无缓存 |
| `vectorstore.Store` | ~~Metadata/NativeClient，与 Creator/Retriever 等组合使用~~（P4-03/P4-05 已删除） | 当前由 `Indexer`/`Searcher`/`IDDeleter`/`FilterDeleter` 独立表达能力 |
| `vectorstore.RetrievalRequest` | ~~fluent `WithFilter/WithMinScore/WithTopK`~~（P4-06 已删除） | 当前 `SearchRequest` 为普通 struct + `Validate` |
| `filter/ast.*` 与 `filter/token.*` | ~~公开 AST 字段、token、visitor~~（P4-07 已删除） | 当前只保留 token-free `Expr`、稳定语义节点、构造函数、`Parse`/`Validate` |

## 5. 关键调用点

下表给出每个高热 package 的首批生产调用点，用于开始迁移时定位真实消费语义。完整文件集合以本节统计命令和 `rg` 为准。

| Package | 关键调用点 |
|---|---|
| `document` | `documentreaders/{html,markdown,pdf}/reader.go`; `rag/chat_middleware.go`; `rag/document_refiner_deduplication.go`; 各 `vectorstores/*/store.go` |
| `media` | `app/runtime/internal/adapter/agentexec/agent.go`; `turn/request.go`; `turnloop.go`; `models/{openai,google,azureopenai,...}` |
| `model` | `agent/hitl/interrupt.go`; `agent/toolloop/halt.go`; `app/runtime/internal/domain/{provider,mcpserver}/registry.go`; provider adapters |
| `model/chat` | `agent/core/chat.go`; `agent/core/guardrails.go`; `a2a/tool.go`; `mcp/*`; `rag/*`; `tools/*`; 绝大多数 chat providers |
| `model/chat/history` | `agent/runtime/guardrails.go`; `agent/workflow/supervisor.go`; `app/runtime/internal/adapter/agentexec/chat_pipeline.go`; chathistory providers |
| `model/embedding` | `app/runtime/internal/adapter/modelclient/embedding.go`; `app/runtime/internal/infra/llm/embedding.go`; embedding providers；vectorstore adapters |
| `vectorstore` | `rag/document_retriever_vectorstore.go`; 各 `vectorstores/*/store.go` |
| `vectorstore/filter` | 各 `vectorstores/*/visitor.go`; `rag/document_retriever_vectorstore.go`；均只消费根语义门面 |

零 workspace 消费并不代表可以无记录删除：`evaluation`、logger/safeguard、公开 lexer/parser/visitors 仍可能被仓库外部用户 import。它们属于 v0 破坏范围，必须进入 release notes。

## 6. P4 迁移子清单

### 6.1 Document/documentpipeline 消费方

- [x] `documentreaders/html`
- [x] `documentreaders/markdown`
- [x] `documentreaders/pdf`
- [x] `rag`
- [x] `vectorstores/internal/tracing`
- [x] 25 个实际 vectorstore 实现（Document/Match 纵向切片与 P4-08 conformance 均完成）

### 6.2 VectorStore backend

Reference batch：

- [x] `vectorstores/inmemory`
- [x] `vectorstores/pgvector`
- [x] `vectorstores/mongodb`
- [x] `vectorstores/qdrant`

Remaining implementation batch：

- [x] `vectorstores/azureaisearch`
- [x] `vectorstores/azurecosmos`
- [x] `vectorstores/bedrockkb`
- [x] `vectorstores/cassandra`
- [x] `vectorstores/chroma`
- [x] `vectorstores/clickhouse`
- [x] `vectorstores/couchbase`
- [x] `vectorstores/elasticsearch`
- [x] `vectorstores/mariadb`
- [x] `vectorstores/milvus`
- [x] `vectorstores/neo4j`
- [x] `vectorstores/opensearch`
- [x] `vectorstores/oracle`
- [x] `vectorstores/pinecone`
- [x] `vectorstores/redis`
- [x] `vectorstores/s3vectors`
- [x] `vectorstores/tidb`
- [x] `vectorstores/typesense`
- [x] `vectorstores/vectara`
- [x] `vectorstores/vespa`
- [x] `vectorstores/weaviate`

Alias verification batch：

- [x] `vectorstores/cockroachdb`（复用 pgvector）
- [x] `vectorstores/supabase`（复用 pgvector）

每个 backend 子项只有在 compile-time assertion、统一 conformance suite、原 backend 测试和 filter visitor 测试全部通过后才可勾选。

P4-08 证据：`c970a3343`。`vectorstores/internal/conformance` 对每个实现验证精确能力集合，并执行必须在外部 I/O 前完成的 Add/Search/Delete 输入契约；27 个目录的原测试、visitor 测试以及 build/vet/lint/race 全绿。

## 7. P6 Provider 迁移子清单

括号内是当前直接依赖的 Core 领域；它决定迁移批次，不表示 provider 必须实现所有能力。

- [ ] `models/alibaba`（chat, embedding）
- [ ] `models/anthropic`（chat, tokenizer）
- [ ] `models/assemblyai`（transcription, media）
- [ ] `models/azureopenai`（chat, embedding, image, transcription, speech, media）
- [ ] `models/bedrock`（chat, embedding, media）
- [ ] `models/blackforestlabs`（image）
- [ ] `models/cohere`（chat, embedding）
- [ ] `models/deepgram`（transcription, speech, media）
- [ ] `models/deepseek`（chat）
- [ ] `models/elevenlabs`（transcription, speech, media）
- [ ] `models/fireworks`（chat）
- [ ] `models/gladia`（transcription, media）
- [ ] `models/google`（chat, embedding, image, transcription, speech, tokenizer, media）
- [ ] `models/groq`（chat）
- [ ] `models/huggingface`（chat）
- [ ] `models/hume`（speech）
- [ ] `models/jina`（chat, embedding）
- [ ] `models/lmnt`（speech）
- [ ] `models/luma`（image）
- [ ] `models/midjourney`（image）
- [ ] `models/minimax`（chat）
- [ ] `models/mistral`（chat, embedding, moderation）
- [ ] `models/moonshot`（chat）
- [ ] `models/nomic`（chat, embedding）
- [ ] `models/ollama`（chat, embedding, media）
- [ ] `models/openai`（chat, embedding, image, moderation, transcription, speech, media）
- [ ] `models/openrouter`（chat）
- [ ] `models/perplexity`（chat）
- [ ] `models/prodia`（image）
- [ ] `models/replicate`（image, speech）
- [ ] `models/revai`（transcription, media）
- [ ] `models/stability`（image）
- [ ] `models/together`（chat）
- [ ] `models/vertexai`（chat, embedding, image, transcription, speech）
- [ ] `models/voyage`（chat, embedding）
- [ ] `models/xai`（chat）
- [ ] `models/xiaomi`（chat）
- [ ] `models/zhipu`（chat, embedding）

`models/catalog` 已在 P5-03 迁为公开 reference-data package；`models/internal` 是测试/共享实现，不计入 38 个 provider，但必须随 reference conformance harness 一起迁移。

## 8. P6 Workspace 消费模块

仍直接 import `core/model/*` 的模块：

- [ ] `a2a`
- [ ] `agent`
- [ ] `app/runtime`
- [ ] `chathistory`
- [ ] `mcp`
- [ ] `models`
- [ ] `rag`
- [ ] `tools`
- [ ] `vectorstores`

另外，`documentreaders/{html,markdown,pdf}` 在 P4 完成同路径 Document 迁移；`otel` 根据 ADR-006 处理；`pkg` 和 `skills` 当前没有直接 import `core/model/*`。

## 9. 更新和验收规则

开始 P1–P6 的任何相关任务前：

1. 将对应子项标为进行中，并记录 commit/batch。
2. 若新增 provider/backend/package，先补入本清单再实现。
3. 若发现仓库外消费证据，补充到影响面和 release notes，不用兼容 shim 隐藏。
4. 新路径迁移以 P6 删除旧 import 为终点；同路径纵向切片以所属阶段删除临时兼容面为终点。
5. 完成后重新统计 package/export/import 数并记录差异；不能只凭编译通过勾选。

复现 direct-import 统计的基本命令：

```bash
rg -n -o '"github\.com/Tangerg/lynx/core[^"]*"' \
  --glob '*.go' --glob '!core/**' .
```

复现单 package 公共面：

```bash
(cd core && go doc -all github.com/Tangerg/lynx/core/model/chat)
```
