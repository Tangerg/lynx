# Core v1 破坏性变更迁移指南

> 目标版本：`github.com/Tangerg/lynx/core` v1.0.0
> 适用范围：从 2026-07-14 以前的 Spring AI 移植形态或任意旧 pseudo-version 迁移
> 迁移策略：一次性直接切换；没有旧包、type alias、deprecated wrapper、双读写或旧 wire 解码

Core v1 不是对旧 API 的重新命名，而是对职责边界的重新定档。旧 Core 同时承担协议、Client framework、history、tool execution、agent control flow、观测、provider catalog 和 tokenizer；v1 把 Core 收缩为跨 provider 的稳定协议、最小调用 SPI 与纯组合算法，并把运行时策略放到独立模块。

这次设计保留了 Spring AI 中“按 modality 建模、provider-neutral protocol、adapter 隔离”的思想，但不复制 Java framework 的层级、继承体系、builder 和自动装配方式。Go 用户面以普通值、小接口、显式构造和函数组合为主。

## 1. 升级前必须知道的事

1. 旧 import path 已物理删除，不能先升级依赖再逐步消除编译错误。
2. 持久化的旧 Chat/History wire 不能由新库直接读取。必须在部署新二进制前完成一次性数据迁移，或者明确清空旧数据。
3. `Model` 不再隐含 streaming、默认参数或 provider identity；调用方只依赖自己实际需要的能力。
4. Core DTO 进入 provider、存储或网络边界前必须经过构造器或 `Validate`。
5. 可执行工具、registry、history、重试、HITL 和 tracing 都不属于 Core protocol。
6. Core v1 要求 Go 1.26.5 或更高的兼容版本。
7. 所有 modality 的 Request-level `Params` 和 `model.Usage.OriginalUsage` 均已删除；没有兼容字段或旧 wire 双读。

建议在独立分支完成整次迁移，不发布“半新半旧”的中间构建。

## 2. Package 路径迁移

| 旧路径/职责 | v1 路径/处理 | 迁移动作 |
|---|---|---|
| `core/model/chat` 协议 | `core/chat` | 改用 tagged `Message`/`Part`、纯 `Request`、多 `Choice` `Response` 与最小 `Model`/`Streamer` |
| `core/model/chat` Client/builder/template/structured output | `chatclient` module | 用 `chatclient.New` 和显式 options/middleware 组合 |
| `core/model/chat/history` 与 history middleware | `chathistory` module | 选择窄 `Reader`/`Writer`/`Store` 能力及对应 middleware |
| `core/model/chat/middleware/safeguard` | `chatclient/middleware/safeguard` | 显式包装 Client 调用边界 |
| `core/model/chat/middleware/logger` | 删除 | 业务审计由应用建模；调用遥测使用 `otel` wrapper |
| `core/model/embedding` 协议 | `core/embedding` | 使用单方法 `Model`；维度查询只依赖可选 `Dimensioner` 或显式 probe helper |
| `core/embedding.Client` 向量便利层 | `embeddingclient` module | 用 `embeddingclient.New` 包装 `embedding.Model`；只需要原始 metadata/usage 时直接调用 Model |
| `core/model/image` | `core/image` | 使用单方法 `Model` 和普通 Request/Options |
| `core/model/audio/transcription` | `core/transcription` | 音频通过 `media.Media` 显式传入 |
| `core/model/audio/tts` | `core/speech` | 同步 `Model` 与流式 `Streamer` 为独立能力 |
| `core/model/moderation` | `core/moderation` | 使用单方法 `Model` 和显式分类结果 |
| `core/tokenizer` | `tokenizer` module | 按需依赖 `TextEstimator`、`Encoder`、`Decoder` 或 `Tokenizer`；tiktoken 在实现子包 |
| `core/evaluation` | `rag/evaluation` | 把事实性/相关性评估留在 RAG 语义层 |
| `core/document/id` 与 formatter/transformer/batcher | `documentpipeline` module | Core `Document` 只保留数据与最小 Reader/Writer 词汇 |
| `core/vectorstore/filter/{ast,lexer,parser,token,visitors}` | `core/vectorstore/filter` | 根输入使用 `Predicate`，路径使用 `Selector`，compiler 实现 `Visitor`；递归下降前端不可导出且只构造一份 AST |
| Core tool schema/executor | `tools` module | 用 `tools.New`/`Registry` 创建并执行 typed tool |
| Core tool loop/Halt/control flow | `agent/toolloop` | 用 `Invocation`、`Runner`、`Event` 和显式 checkpoint/resume |
| Core Chat/Embedding tracing 与 metrics | `otel` module | 在 Core 外显式包装 model/client |
| Core model catalog/pricing/capability | `models/catalog` | reference data 随 provider 模块发布，不进入稳定协议 |
| `core/model.APIKey` | `models/<provider>` config 字符串或 provider 专用认证 | secret 存储、刷新、脱敏由应用负责 |
| `core/model.Usage` / `core/model.RateLimit` | `embedding.Usage` / 删除 | embedding 用量改用 `embedding.Usage{InputTokens}`；chat 用量一直是 `chat.Usage`；RateLimit 零消费直接删除 |

`core/model` 包已整体删除。它声称的"跨模态共享 Usage/RateLimit"与事实不符：Usage 只被 embedding 消费且只有输入 token 有填充者，RateLimit 零生产零消费。embedding 自有 input-only 的 `embedding.Usage{InputTokens}`；chat 保留自己的 `chat.Usage`（input/output 词汇）；RateLimit 无替代（需要时由 provider adapter 按真实语义建模）。

## 3. Chat 迁移

### 3.1 Model 能力

旧复合模型要求每个 provider 同时实现同步、流式、默认参数和 metadata。v1 将能力拆开：

```go
type Model interface {
    Call(context.Context, *chat.Request) (*chat.Response, error)
}

type Streamer interface {
    Stream(context.Context, *chat.Request) iter.Seq2[*chat.Response, error]
}
```

迁移规则：

- 只做同步调用的函数接收 `chat.Model`。
- 只在确实使用 streaming 的位置额外接收 `chat.Streamer`。
- provider 默认值放在 provider 构造配置中；应用级默认值通过 `chatclient.WithDefaults` 注入。
- provider/model 观测属性在构造 `otel` wrapper 时显式传入，不从 Model 反向探测。

### 3.2 Message、Part 与 Request

旧 message interface 和多个具体 message 类型改为普通 tagged value：

```go
request, err := chat.NewRequest(
    chat.NewSystemMessage("Answer briefly."),
    chat.NewUserMessage(chat.NewTextPart("What is a goroutine?")),
)
if err != nil {
    return err
}
request.Options = chat.Options{Model: "provider-model"}
```

迁移时必须同时处理以下语义变化：

- `Role` 与 `Part.Kind` 是 wire discriminator，未知值会被拒绝。
- `Request.Tools` 只保存可序列化 `ToolDefinition`，不能保存 executor、registry 或闭包。
- 不再使用任意 `Params map[string]any` 承载 provider 扩展。Chat provider 扩展使用 namespaced `Extensions`，通过 `SetExtension` 编码为 JSON。
- 构造器只验证初始值；修改 exported field 后，在 I/O 边界再次调用 `Validate`。

### 3.3 Response

旧的单 Result 响应改为保留全部 provider choices：

```go
response, err := model.Call(ctx, request)
if err != nil {
    return err
}
first := response.First()
text := response.Text()
```

不要把 tool result、pause/resume 或 round boundary 合成 Chat Response。这些属于 `agent/toolloop.Event`。流式聚合使用 `chat.ResponseAccumulator`，不能通过一次同步 Call 伪造 stream。

### 3.4 高层 Client

需要默认参数、middleware、模板或结构化输出时，显式依赖 `chatclient`：

```go
client, err := chatclient.New(model,
    chatclient.WithDefaults(chat.Options{Temperature: &temperature}),
)
if err != nil {
    return err
}
response, err := client.Call(ctx, request)
```

这不是 Core Model 的组成部分。库代码如果只需要一次 `Call`，应继续接收 `chat.Model`，不要扩大到 `*chatclient.Client`。

## 4. Tool 与 Agent 运行时迁移

工具现在有两份相邻但不互相嵌套的状态：

- `chat.ToolDefinition`：模型可见、可序列化的名称/描述/input schema。
- `tools.Tool`/`tools.Registry`：进程内可执行对象。

```go
tool, err := tools.New(tools.Config{
    Name: "lookup",
    Description: "look up a record",
}, func(ctx context.Context, input lookupInput) (lookupOutput, error) {
    return lookup(ctx, input)
})
registry, err := tools.NewRegistry(tool)

request.Tools = registry.Definitions()
invocation, err := toolloop.NewInvocation(request, registry)
runner, err := toolloop.NewRunner(model, toolloop.RunnerConfig{})
events := runner.Run(ctx, invocation)
```

迁移时不要重新建立一个同时持有 wire schema 和 executor 的 Core Tool。需要 HITL 时，工具在副作用前返回 `toolloop.PauseError`；应用持久化 `Checkpoint`，再通过 `Runner.Resume` 恢复。普通 tool error 回传给模型，context error 与 `toolloop.AbortError` 终止运行；Runner 不隐式重试。

## 5. Metadata 与 Media 迁移

### 5.1 JSON-safe metadata

跨边界 metadata 使用 `metadata.Map`，值在写入时即编码：

```go
values := metadata.New()
if err := values.Set("tenant", tenantID); err != nil {
    return err
}
tenant, ok, err := metadata.Decode[string](values, "tenant")
```

合并两个 metadata 值时使用 receiver 方法；它会先校验双方、深拷贝 source，并按末值覆盖语义更新 target：

```go
if err := values.Merge(providerValues); err != nil {
    return err
}
```

不要直接塞入函数、reader、SDK client 或其他运行时对象。直接 map 赋值只接受 `json.RawMessage`，并仍需通过 `Validate`。

Embedding、Image、Moderation、Speech 和 Transcription 的 `Options.Extra`、`ResultMetadata.Extra`、`ResponseMetadata.Extra` 也统一为 `metadata.Map`。写入必须处理 `Set` 返回的错误；读取使用 `metadata.Decode[T]`：

```go
opts := &embedding.Options{}
if err := opts.Set("provider/dimensions_mode", "truncate"); err != nil {
    return err
}
mode, ok, err := metadata.Decode[string](opts.Extra, "provider/dimensions_mode")
```

这些 modality 不再提供 Request-level `Params`。provider 请求参数先进入对应的 typed `Options`；只有 Core 尚未稳定建模、且确实需要透传的 JSON 数据才进入 `Options.Extra`。不要在调用方重建 `map[string]any` 参数袋。

每个 modality Request 都公开 `Validate()`。所有官方 provider adapter 会在 `Call`/`Stream` 边界先验证整个请求（包括嵌套 Media 与 Options metadata），再读取字段或映射 SDK 参数。自定义 Model 实现也必须遵循同一顺序：

```go
func (m *Model) Call(ctx context.Context, req *embedding.Request) (*embedding.Response, error) {
    if err := req.Validate(); err != nil {
        return nil, err
    }
    // translate only after validation
}
```

官方 provider 的原生 options key 已统一为 `<provider>/options`（例如 `openai/options`），不再接受移植期的 `lynx:ai:model:*` key。Google transcription prompt 直接使用 typed `transcription.Options.Prompt`，不再另建 Extra key。

Core 的 usage 类型（`chat.Usage` / `embedding.Usage`）只保留跨 provider 可比较的规范化计数。provider SDK 原始 usage 如需诊断，由 adapter 自己记录，或在明确命名并转为 JSON 后写入 response metadata；不能把 SDK 对象放回 Core Usage。

### 5.2 Tagged media

旧 `Media.Data any` 已删除。调用方必须在进入协议前把数据转换为唯一来源：

```go
inline, err := media.NewBytes("image/png", pngBytes)
remote, err := media.NewURI("image/png", "https://example.test/image.png")
native, err := media.NewReference("image/png", providerFileID)
```

`Source.Kind` 必须与 Bytes、URI、Ref 中恰好一个字段匹配。文件句柄和 `io.Reader` 的生命周期由调用方处理，不能进入 DTO。

## 6. Document 与 VectorStore 迁移

`document.Document` 只保存内容、ID 和 JSON-safe metadata。旧 `Document.Score` 移到搜索结果 `vectorstore.Match.Score`；formatter、splitter、batcher、ID generator 迁到 `documentpipeline`。

旧胖 `Store`/`Creator`/`Retriever`/`Deleter` 被四个独立能力替代：

```go
type Indexer interface { Add(context.Context, []*document.Document) error }
type Searcher interface { Search(context.Context, vectorstore.SearchRequest) ([]vectorstore.Match, error) }
type IDDeleter interface { DeleteIDs(context.Context, []string) error }
type FilterDeleter interface { DeleteWhere(context.Context, filter.Predicate) error }
```

消费方接口只组合实际调用的能力。搜索请求是普通值：

```go
predicate, err := filter.Parse(`kind == 'guide' AND year >= 2025`)
if err != nil {
    return err
}
request := vectorstore.SearchRequest{
    Query: "Go API design",
    TopK: 10,
    MinScore: 0.7,
    Filter: predicate,
}
if err := request.Validate(); err != nil {
    return err
}
matches, err := searcher.Search(ctx, request)
```

旧 `AtomicExpr`/`ComputedExpr`/`ExprBuilder` 与 precedence API 已删除。`Expr` 仍是所有节点的封闭根，但只有 `Predicate` 能作为 Parse、Search 或 DeleteWhere 的输入；这会让裸 identifier/literal 和残缺逻辑树在编译期或 `Validate` 阶段失败。`profile['name']` 始终表示完整路径 `profile → name`，不会再丢弃 base identifier。`Parse` 在严格校验后会规范化双重 NOT、同运算符重复项、吸收结构和可提取的公共因子；比较、IN、LIKE、IS 等非逻辑叶子的顺序和值保持不变。直接调用 `Validate` 或 `Visit` 不会重写调用方持有的树。

不要重建 fluent `With*` 链或 `NativeClient any` 探测面。provider 专用能力直接由具体 backend 类型公开。

`vectorstores/inmemory.StoreConfig` 与其他 embedding-backed backend 一样直接接收 Core Model；调用方不再预先构造便利 Client：

```go
store, err := inmemory.NewStore(inmemory.StoreConfig{
    EmbeddingModel: model,
})
```

旧 `EmbeddingClient` 字段、`ErrMissingEmbeddingClient` 和不可达的 `ErrNilConfig` 已删除；缺少模型统一返回 `ErrMissingEmbeddingModel`。

## 7. 其他 modality 迁移

Embedding、Image、Transcription、Moderation 都只要求一个 `Call`；Speech 将 `Call` 与 `Stream` 拆成独立能力。旧 ClientCaller/Handler/Middleware/Chain/ModelMetadata framework 已删除。

迁移方式统一为：

1. 在 `models/<provider>` 构造具体 adapter。
2. 让业务函数只接收对应包的最小 `Model` 或可选 `Streamer`。
3. 用 `NewRequest`/`NewOptions` 构造输入，按需调用 `base.Merged(overrides...)` 获取合并后的独立 Options；该方法不修改 receiver。
4. provider 特有 options 用 `Options.Set` 写入 JSON-safe `Extra`，key 使用 `<provider>/options`，并显式处理错误；Request 不再有 `Params`。
5. 在 Model `Call`/`Stream` 读取请求前调用 `Request.Validate`，并保留规范化 metadata/usage；原始 SDK usage 不进入 Core DTO。
6. Embedding dimensions 需要时先断言 `embedding.Dimensioner`，否则显式调用 `ProbeDimensions`；没有全局缓存。

Embedding 与 Moderation 的多结果响应使用 `response.First()` 取得首项。Speech 的音频容器字段为 `Options.OutputFormat`，生成结果字节为 `Result.Audio`；对应 JSON 字段是 `output_format` 与 `audio`，不再使用移植期的 `response_format` 与 `speech`。

需要把文本或 Document 批量转换为向量时，使用 Core 之外的窄便利层：

```go
client, err := embeddingclient.New(model)
if err != nil {
    return err
}
vectors, err := client.EmbedTexts(ctx, texts)
```

`embeddingclient.Client` 只公开 `EmbedText`、`EmbedTexts` 与 `EmbedDocuments`，返回与 provider Response 不共享底层数组的向量。旧 Client 的 `Call` 和额外 `*embedding.Response` 返回值没有迁入新模块：需要 response metadata/usage 的调用方应直接构造 `embedding.Request` 并调用 `embedding.Model.Call`，避免两个调用入口产生语义漂移。

## 8. 持久化数据的一次性迁移

新库不会读取旧 type-tagged Chat/History wire。存在历史数据时，按以下顺序升级：

1. 冻结旧系统写入，记录行数、会话数和校验摘要。
2. 使用旧版本二进制导出旧数据；不要让新 Core import 旧类型。
3. 在一次性迁移程序中映射 role、part、tool call/result、media 和 metadata，构造当前 `chat.Message`。
4. 对每条新值执行 `Validate`，写入新表/新 bucket/新 object prefix。
5. 抽样回放完整 tool-call round，核对消息顺序、内容、usage 和附件引用。
6. 原子切换读写目标后再部署新二进制；保留旧数据只用于回滚，不让生产代码双读。
7. 观察期结束后删除旧存储和迁移程序。

无法可靠映射的历史记录必须进入隔离/人工处理清单，不能把旧 decoder 加回库中。

## 9. 推荐迁移顺序

1. 把工具链升级到 Go 1.26.5。
2. 先迁纯 DTO：metadata、media、document、chat message/request/response。
3. 再迁 provider adapter 到最小 modality SPI。
4. 迁 ChatClient、history、tools、tool loop、OTel 和 document pipeline 等外圈职责。
5. 执行一次性历史数据迁移。
6. 删除业务仓库内所有旧 import、自定义 bridge 和旧 wire decoder。
7. 更新 `go.mod` 到 Core v1 和按发布顺序已发布的 dependent modules。
8. 运行全量 build/vet/test/lint/race、provider/backend conformance 与序列化回归。

## 10. 验收清单

- [ ] `rg 'github.com/Tangerg/lynx/core/model/(chat|embedding|image|audio|moderation)'` 无结果。
- [ ] `rg 'github.com/Tangerg/lynx/core/(tokenizer|evaluation|document/id)'` 无结果。
- [ ] 没有本地 alias、shim、deprecated forwarding constructor 或双 wire decoder。
- [ ] Model 消费接口没有强制不使用的 streaming/default/metadata 能力。
- [ ] Chat Request 不持有 executor、registry、闭包或 SDK client。
- [ ] 所有 modality Request 均无 `Params` 参数袋，所有 `Extra` 均通过 `metadata.Map` 写入时编码。
- [ ] 每个自定义 Model 在读取请求前调用 `Request.Validate`；官方 Models 架构门禁已覆盖所有 Call/Stream。
- [ ] provider 原生 options 只使用 `<provider>/options` key，没有 `lynx:ai:model:*` 历史 key。
- [ ] Core usage 类型（`chat.Usage` / `embedding.Usage`）不持有 provider 原始 SDK usage 对象。
- [ ] 所有持久化/网络/provider 边界都验证当前 DTO。
- [ ] 历史数据已经一次性迁移或明确清空，生产路径只读写当前格式。
- [ ] `go mod tidy -diff` 为空。
- [ ] 相关 module 的 build、vet、test、lint、race 与 conformance 全部通过。

当前 API 的最小可运行入口见 [`CORE_GETTING_STARTED.md`](./CORE_GETTING_STARTED.md)，完整设计决策和证据见 [`CORE_ARCHITECTURE_EXECUTION_PLAN.md`](./CORE_ARCHITECTURE_EXECUTION_PLAN.md)。
