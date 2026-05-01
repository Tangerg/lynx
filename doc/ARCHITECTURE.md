# Lynx 系统架构

> 模块拓扑、核心抽象、分层关系、可扩展性评价以及当前的已知技术债。
>
> **基线**：HEAD `4da6a37`（main）。本文合并了原 `ARCHITECTURE.md` / `IMPROVEMENTS.md` / `CORE_AUDIT_2026-04-30.md` 的有效内容。

---

## 1. 模块拓扑

```
┌─────────────────────────────────────────────────────────────┐
│                        上层应用                              │
└────────────┬────────────────────────────────────────────────┘
             │
   ┌─────────┼──────────┬─────────────┬────────────┬───────────┐
   │         │          │             │            │           │
┌──▼──┐ ┌────▼────┐ ┌───▼────┐ ┌──────▼─────┐ ┌────▼────┐ ┌────▼───┐
│tools│ │ models  │ │  mcp   │ │vectorstores│ │otelbridge│ │ agent  │
│     │ │openai   │ │go-sdk  │ │qdrant      │ │slog/log  │ │（未落） │
│     │ │anthropic│ │bridge  │ │milvus      │ │exporter  │ │        │
│     │ │google   │ │        │ │pinecone    │ │          │ │        │
│     │ │         │ │        │ │weaviate    │ │          │ │        │
│     │ │         │ │        │ │chroma      │ │          │ │        │
└──┬──┘ └────┬────┘ └───┬────┘ └──────┬─────┘ └─────┬────┘ └────┬───┘
   │         │          │             │             │           │
   └─────────┴──────────┼─────────────┴─────────────┴───────────┘
                        │
                  ┌─────▼─────┐         ┌─────────┐
                  │   core/   │ ◀────── │  pkg/   │
                  │ (抽象+RAG) │         │（通用工具）│
                  └───────────┘         └─────────┘
```

`go.work` 把 7 个子模块拉进同一个 workspace；对外以独立 Go module 发布。

**核心假设**：`core/` 定义抽象与协议；`models/ / vectorstores/ / tools/ / mcp/` 只做适配实现；`pkg/` 是底层工具库——外部不应直接依赖。

**评价**：
- ✅ 依赖方向单向（外层 → core → pkg），无循环
- ✅ 重依赖（provider SDK、vector DB driver、OTel SDK、MCP SDK）全部隔离在外挂 module
- ⚠️ 跨模块 core 版本漂移（`models/go.mod` vs `vectorstores/go.mod` pin 不同 commit），go.work 下看不出来，对外发布会让下游踩坑
- ⚠️ `pkg/` 的 23 个子包结构扁平（`maps / sets / slices / stream / json / xml / text / strings / math / random / ptr / cast …`），缺乏分层；`result / retry / sync / safe / stream` 这类通用工具若渗透进 core 公开 API 会造成依赖扩散

---

## 2. 核心抽象：`Handler[Req, Res]` 代数基元

```go
// core/model/handler.go
type CallHandler[Req, Res any] interface {
    Call(ctx context.Context, req Req) (Res, error)
}
type StreamHandler[Req, Res any] interface {
    Stream(ctx context.Context, req Req) iter.Seq2[Res, error]
}
```

整个库的 Chat、Embedding、Image、Audio、Moderation 都建立在这一对最小代数算子上。

**与 Spring AI 的对比**：
- Spring AI 走继承：`ChatModel extends StreamingChatModel extends Model<Req, Res>`
- Lynx 走组合：`chat.Model = CallHandler[*Request, *Response] + StreamHandler[*Request, *Response] + Options 方法`

组合胜出之处：测试 mock 容易、类型参数显式、能用 Go 1.23 的 `iter.Seq2` 做流式而不发明私有的 Mono/Flux。

**隐藏代价**：`chat/client.go` 用类型别名 `type CallHandler = model.CallHandler[*Request, *Response]` 把 `*Request/*Response` 钉死。下游若想扩展 Request 结构（加字段、替换 Message 类型），只能 fork 整个 chat 包。Spring AI 通过继承可以自然扩展——这是 Lynx 的有意取舍：用 `Request.Params map[string]any` 与 `Options.Extra map[string]any` 通道兜住扩展需求，避免顶层 API 全泛型化。

---

## 3. 四层洋葱：Model → MiddlewareManager → Client → 用户 API

```
┌────────────────────────────────────────────────────┐
│  Layer 4: 用户 API                                  │
│    chat.Client（fluent builder）                    │
│    ClientRequest → ClientCaller / ClientStreamer    │
├────────────────────────────────────────────────────┤
│  Layer 3: MiddlewareManager                         │
│    用户注册的 Middleware（按逆序包装，洋葱模型）     │
│    Tool/Memory middleware 现在均为 opt-in           │
├────────────────────────────────────────────────────┤
│  Layer 2: Handler 抽象                              │
│    CallHandler / StreamHandler（纯函数链）           │
├────────────────────────────────────────────────────┤
│  Layer 1: Model 实现                                │
│    openai / anthropic / google                      │
└────────────────────────────────────────────────────┘
```

### 3.1 洋葱执行顺序

`MiddlewareManager.BuildCallHandler` 按**注册逆序**包装：第一个注册的中间件位于最外层，最后一个最贴近 Model。这是经典写法，与 `net/http` 的 `mux.Use()` 一致。

### 3.2 ToolMiddleware：opt-in，不再硬编码

旧版本在 `ClientCaller.call()` 与 `ClientStreamer.stream()` 里硬编码注入 `ToolMiddleware`，commit `8e58479` 已修复。现在两条路径明确声明：

```
// Tool execution is NOT injected automatically; register ToolMiddleware explicitly
// via WithMiddlewares if tool calls need to be handled.
```

`NewToolMiddleware()` 对外返回 `(CallMiddleware, StreamMiddleware)`，与其他中间件平等。这条修复让用户能：
- 用自定义 Tool 编排替换（并行 / retry / 超时）
- 控制 ToolMiddleware 与其他中间件的相对位置
- 完全不挂（Options.Tools 非空时也不强制生效）

### 3.3 Memory Middleware 的「原地变更」问题（待修）

`core/model/chat/memory/memory_middleware.go` 通过在 `Message.Meta()[SavedMarkerKey]` 上打标记表达「已存盘」。这是有状态副作用：
- 同一个 Message 被多 Client 复用时会互相污染
- 多 Memory Middleware 串联时标记冲突
- 值语义消失——Message 不再是 immutable

**修法**：把 saved 状态放在 middleware 的本地 map，按 Message 身份哈希去重；或让 Message 是 immutable、通过新对象承载「已保存」语义。

### 3.4 Middleware 框架问题（详见 `MIDDLEWARE.md`）

- `MiddlewareManager` 当前 4 个泛型参数（CallReq/CallRes/StreamReq/StreamRes），所有使用点都填同样的 Req/Res，可瘦身到 2 参数
- `CallMiddleware` 与 `StreamMiddleware` 是两个独立类型，模式无关中间件（logging / metrics / auth）必须写两份
- `UseMiddlewares(...any)` 反 Go 风格——传错类型静默丢弃；与 `UseCallMiddlewares` / `UseStreamMiddlewares` 重叠

---

## 4. RAG：固定五阶段流水线

```
 Query
   │
   ▼
 [1] Transform      ── seq:      QueryTransformer[]
   │
   ▼
 [2] Expand         ── 1→N:      QueryExpander
   │
   ▼
 [3] Retrieve       ── parallel: DocumentRetriever[]（errgroup）
   │
   ▼
 [4] Refine         ── seq:      DocumentRefiner[]
   │
   ▼
 [5] Augment        ── (Query + Docs) → 新 Query
   │
   ▼
 augmented Query
```

`core/rag/pipeline.go::Execute` 把阶段顺序硬编码。

**评价**：
- ✅ 直观——五段对应标准 RAG 论文的 pre-retrieval / retrieval / post-retrieval
- ✅ Retrieve 并行——多路召回用 `errgroup`，符合工业实践
- ❌ 阶段不可重排——想在 Refine 和 Augment 之间插 dedup？要 fork
- ❌ 阶段间无条件跳转——「top-1 得分 > 0.95 就跳过 refine」做不到
- ❌ 与 chat Middleware 的二元割裂——RAG 有自己的 `PipelineMiddleware`，与 `CallMiddleware` 类型不通

**演进路线**（非紧急）：把 Pipeline 改为 `Stage` 接口的有序列表，默认 5 个 stage，用户可插/删/换，保留并行 hint。

---

## 5. Document Pipeline：架构最干净的子系统

```
Reader  →  Transformer*  →  Batcher  →  Writer
(源头)     (切分/富化)      (按 token)   (落盘/入库)
```

- 所有阶段都是 `[]*Document → []*Document` 的纯函数
- `Document` 内嵌 `Formatter`，支持 `MetadataModeAll / Embed / Inference / None`——晚绑定的格式化，不需要拷贝 Document
- `TokenCountBatcher` 按 token 限额打包，单条超限直接报错（不静默丢弃）

每个抽象只干一件事，阶段之间靠数据结构连通，无隐藏状态。**这套设计美学应该扩散到 RAG Pipeline**。

**唯一架构瑕疵**：`Reader.Read()` 返回 `[]*Document`（全量）而非 `iter.Seq2[*Document, error]`（流式）。大数据集下会 OOM。Go 1.23 已有 `iter`，应该统一到迭代器。

---

## 6. VectorStore + Filter DSL

```
core/vectorstore/
├── vector_store.go             # ISP 三件套：Creator / Retriever / Deleter / Info
└── filter/
    ├── lexer/                  # 词法分析
    ├── parser/                 # 语法分析
    ├── token/                  # token 类型
    ├── ast/                    # AST 节点：Ident/Literal/BinaryExpr/UnaryExpr/IndexExpr/ListLiteral
    └── visitors/               # AST 遍历框架

vectorstores/{qdrant,milvus,weaviate,pinecone,chroma}/
└── visitor.go                  # 每个 store 实现自己的 Visitor，AST → 该 store 原生 Filter
```

**这是一门真正的 DSL**——不是键值 map，包含逻辑算子（`AND/OR/NOT`）、比较算子、`IN`、`LIKE`、嵌套表达式。

### 6.1 抽象层次评价

- ✅ Visitor 模式天然适合 AST→目标方言翻译，添加新 store 只需写一个 visitor
- ✅ Filter 在 `RetrievalRequest` / `DeleteRequest` 中传递 AST（非字符串），统一了「应用层构造查询」与「DSL 字符串解析」两种入口

### 6.2 架构级问题

1. **能力矩阵不统一**：Pinecone 不支持 `LIKE`、Milvus 对 nested field 有限制，但 DSL 允许构造任意表达式，错误只在运行时暴露。**应在接口层提供 `Capabilities()` 能力声明**，让上层能提前检查或降级。

2. **5 套 visitor 重复建设**：每个 store 的 visitor 都要处理 AND/OR/NOT 的布尔代数、字面量类型映射、NULL 处理。**核心层应提供 `BaseVisitor` 模板（模板方法模式）**，子类只实现叶子节点翻译。当前 5 个文件各写 500-800 行，重复度极高（详见 §10.4）。

### 6.3 是否过度工程

- 对「只要按 user_id 过滤」的初学者：是
- 对「多租户 + 多条件 + 嵌套逻辑」的生产系统：不是

**保留 DSL，补一个流式 builder**：

```go
filter.Eq("user_id", 123).And(filter.In("category", "tech", "news"))
```

让 80% 的简单场景不需要学词法/语法规则。

### 6.4 安全

`core/vectorstore/filter/parser` / `lexer` 当前对 token 数量、嵌套深度均无上限。恶意/畸形输入可触发 stack overflow / OOM / ReDoS。**应加 `MaxTokens`、`MaxDepth` 常量**——过滤 DSL 是 API 边界，必须视为不可信输入。

---

## 7. 多模态 Client：四份 boilerplate

`chat.Client` / `embedding.Client` / `image.Client` / `audio.Client` 都做同一件事：
1. 持有 Model
2. 提供 fluent builder
3. 通过 `Call() / Stream()` 触发 Handler（**仅 chat 全家桶有 Stream，其余只 Call**）

**DRY 违反度 ≈ 80%**。每加一个模态要重写 ~200 行 boilerplate。

**改造方向**（参见 `MIDDLEWARE.md` §统一 Manager 部分）：
- 抽 `CallBaseClient[Req, Res]` 与 `StreamBaseClient[Req, Res]`
- chat 同时继承两者；embedding/image/audio 只继承 Call 基类
- 保留两个独立 Manager（Call/Stream 类型不能合并，见下文）

> **注意**：这是 v1.0 ABI 重塑级别工程，影响 6 个模态 client，优先级低。先做 §10 的 quick wins。

---

## 8. Middleware：三种实例，没有共同祖先

| 维度 | ChatMiddleware | RAGPipelineMiddleware | ToolMiddleware |
|-----|---------------|----------------------|----------------|
| 签名 | `(CallHandler) → CallHandler` | 包裹整个 Pipeline.Execute | `(CallMiddleware, StreamMiddleware)` 对 |
| 数据 | `*chat.Request → *chat.Response` | `*Query → (Query, []Document)` | 同 ChatMiddleware |
| 注入 | 用户注册 | 通过 ChatMiddleware 桥接 | 用户注册（已修复 opt-in）|

**症结**：Lynx 有「装饰器」的实现，没有「Middleware」的接口。三类中间件是三条独立进化线。Embedding/Image/Audio/Moderation 甚至**完全没有中间件故事**——无法加 logging / caching / rate-limit，除非用户从 Handler 层手写装饰器。

**架构级改进**：

```go
// core/middleware/middleware.go
type Middleware[H any] func(next H) H

type Manager[H any] struct { middlewares []Middleware[H] }
func (m *Manager[H]) Build(endpoint H) H { /* reverse wrap */ }
```

Chat / Embedding / Image / Audio / Moderation / RAG 全部基于同一套 `Manager[CallHandler[Req,Res]]` 实例化。RAG、Tool、Memory、Cache、RateLimit 就都能组合。

详细讨论见 [`MIDDLEWARE.md`](./MIDDLEWARE.md)。

---

## 9. 可扩展性打分

| 扩展方向 | 打分 | 说明 |
|---------|-----|------|
| 添加 Model Provider | 9/10 | 实现 `chat.Model` 即可 |
| 添加 VectorStore | 8/10 | 实现三件套 + visitor；visitor 有重复 |
| 添加 Chat 中间件 | 8/10 | 装饰器模式，清晰 |
| 添加其他模态中间件 | **3/10** | **无现成故事**，要从 Handler 层手写 |
| 添加 RAG Stage | **4/10** | 阶段固定，要 fork |
| 添加 Filter 操作符 | **3/10** | 要改 lexer/parser/所有 visitor |
| 自定义 Tool 编排策略 | 7/10 | ToolMiddleware 已 opt-in；缺 ExecutionMode 枚举 |
| 自定义 Document Transformer | 9/10 | 实现接口即可 |
| 替换 Prompt 模板引擎 | 6/10 | 接口在，default impl 假设 Go text/template |
| 自定义 Memory 存储 | 8/10 | `memory.Store` 清晰 |

短板集中在：**非 chat 模态的中间件、RAG 阶段、Filter 语法扩展**。

---

## 10. 已知技术债（按 ROI 排序）

> 改动量、风险、优先级三档。第 1-3 项是可立即执行的 quick wins，第 4-7 项是中期重构，第 8 项是 v1.0 ABI 重塑级别。

| # | 动作 | 改动量 | 风险 | 优先级 |
|---|-----|--------|-----|-------|
| 1 | `MiddlewareManager` 4 → 2 参数瘦身 | -80 行 | 中 | 🔴 |
| 2 | 删 `UseMiddlewares(...any)` 反 Go API（type-safe 版替代） | -25 行 | 小 | 🟠 |
| 3 | 安全：filter parser 加 `MaxTokens / MaxDepth` 限制 | +30 行 | 极小 | 🔴 |
| 4 | VectorStore `BaseVisitor` 抽象 | -60% visitor 代码 | 中 | 🟠 |
| 5 | 跨模块 core 版本对齐（CI 校验 + tag 化） | – | 小（运维） | 🔴 |
| 6 | Document `Reader` 流式化 | 接口改动 | 中 | 🟡 |
| 7 | RAG Pipeline 阶段化（`Stage` 接口） | 中等重构 | 中 | 🟠 |
| 8 | Call/Stream Middleware 合并为单一类型（破坏性 API） | -150 行 | 中-大 | 🟠（v1.0 ABI）|

### 10.1 PANIC / 错误处理待修

- `core/model/chat/message.go::FilterMessages` 在 nil predicate 时 `panic("...")`——库代码不应对调用方输入 panic，应返回 `([]Message, error)` 或退化为 identity
- `core/tokenizer/tiktoken.go::NewTiktokenWithCL100KBase` 构造时 panic——破坏 Go 「返回 error」契约，改为 `(*Tiktoken, error)` 或提供 `Must...` 包装
- `core/model/chat/client.go::Clone()` 对 `middlewareManager / options / userPromptTemplate / systemPromptTemplate` 直接调 `Clone()`，未做 nil 防御。`NewClientRequest` 已 eager 初始化规避大部分场景，但内部仍未防御
- `core/model/chat/response_accumulator.go::AddChunk` 无锁；多 goroutine 并发 push 会 race。**最少要在文档声明非线程安全**

### 10.2 测试覆盖

- 整个 `core/` 16k+ 行代码无 `_test.go`，最值得补：
  - `core/model/chat/client_test.go`（Clone、WithMessages 语义）
  - `core/model/chat/message_test.go`（MergeMessages / FilterMessages / MergeAdjacent）
  - `core/rag/pipeline_test.go`（带 `-race`）
  - `core/document/transformer_splitter_test.go`（切分边界）
- `models/`、`vectorstores/`、`tools/` 三模块合计 0 个 `_test.go`——每个 provider/store 至少加最小契约测试（mock SDK）
- `mcp/` 已有 4 个测试文件，是当前测试覆盖最好的模块

### 10.3 错误处理 / API 一致性

- 缺包级 sentinel error。`message.go` 里的 `"at least one tool return required"` 是字符串字面量，调用方无法 `errors.Is`。补：

```go
var (
    ErrEmptyToolMessage = errors.New("chat: tool message requires at least one return")
    ErrEmptyDocument    = errors.New("document: must contain text or media")
    ErrNilRequest       = errors.New("request is nil")
)
```

- Streaming 返回类型不统一：Anthropic `*ssestream.Stream`、OpenAI `(stream, error)`、Google `iter.Seq2`。统一到 `iter.Seq2[T, error]`
- `chat.Client.Structured()` 接 `StructuredParser[any]`，泛型参数被吃掉，丢失类型安全。改为顶层函数 `Structured[T](c *Client, parser StructuredParser[T])`
- VectorStore ID 语义不统一：Qdrant/Chroma 内部 `uuid.NewString()` 覆盖 `doc.ID`，导致 upsert 身份丢失；其他 store 又各有策略。统一为「优先用 `doc.ID`，空则生成」，暴露 `IDGenerator` 接口

### 10.4 VectorStore Visitor 重复

5 个 store 的 visitor.go 共 ~3300 行，相比 Spring AI 的 `AbstractFilterExpressionConverter` 模板基类（~750 行）。`core/vectorstore/filter/visitors.BaseVisitor` 实现布尔代数骨架后，子类只写叶子节点翻译——预计减少 ~60% 代码量。

### 10.5 文档与 ergonomics

- 缺 `doc.go` 包级注释：`core/`、`core/model/chat/`、`core/document/`、`core/tokenizer/`
- 缺 `examples/` 子目录：建议 5 个最小 quickstart（`chat_basic` / `chat_streaming` / `chat_with_tools` / `chat_with_memory` / `chat_structured`）
- 导出类型缺 godoc：`Response` / `Result` / `ResultMetadata` / `Options` 各字段 / `Document` 等

### 10.6 性能（机会而非债）

- `defaultRequest.Clone()` 每次调用都全量深拷贝；高 QPS 下分配压力大。可考虑 copy-on-write 或 lazy clone
- `ResponseAccumulator.EnsureIndex` 多次重分配 Results 切片；按预估容量预分配
- `MergeOptions` 先 `ptr.Clone` 整个 Options，再覆盖字段；只拷贝被 override 的字段更高效

---

## 11. 明确不应改的（避免反复讨论）

下面这些是品味偏好性质的判断，避免被反复争论：

| # | 不动项 | 理由 |
|---|-------|------|
| 1 | 链式 fluent API（`.WithTools(...).Call().Text(ctx)`）| OpenAI Go SDK / Anthropic Go SDK / grpc-go / AWS SDK Go v2 都是这风格，是 Go 现代库惯例 |
| 2 | `Client / ClientRequest / Request / Model` 4 角色分层 | 各司其职：工厂 / builder / 不可变快照 / provider 适配器；解耦 default 与 single-call 修改 |
| 3 | `chat` fluent vs `mcp` config 风格分裂 | 分工合理：chat 是上层用户面 API，mcp 是低层集成 API（多源装配，多字段）。Spring AI 也是 ChatClient 用 builder / autoconfig 用 properties |
| 4 | 终端方法 `.Call().Text(ctx)` 两段式 | 让 `Call()` 返回的 `ClientCaller` 能选择 3 种终端方法（Text/Response/Structured）|
| 5 | 三件套返回 `(string, *Response, error)` | `*Response` 让用户拿到 token usage / finish_reason / metadata 而无需再跑一遍 |
| 6 | 类型别名钉死 `*Request, *Response` | `Request.Params` / `Options.Extra` 通道已能传任意数据；强行参数化让所有上层 API 签名爆炸 |
| 7 | 缺乏「配方层」（RAG/Agent/MemoryChat 一键装配）| 克制小库定位；用户手动拼装 + 复制示例，长期心智成本反而更低 |
| 8 | `Client` + `NewClientWithModel` 双构造 | Go 惯例（`http.NewServeMux` vs `DefaultServeMux`、`time.NewTicker` vs `time.Tick`）|

---

## 12. 总评

**定位**：Lynx 是 「Go idiom 友好 + Spring AI 功能对标」的 LLM 框架。设计品味清晰——不是 Python 框架的直译，也不是重复造轮子。

**值得保留的设计**：
- `CallHandler[Req, Res]` 代数基元
- 原生 `iter.Seq2` 流式
- Document pipeline 四段式
- VectorStore 用 Filter DSL 做 lingua franca
- 显式 Middleware 洋葱
- `AssistantMessage.Reasoning` 一等公民（详见 `REASONING.md`）
- MCP 桥接的克制设计（详见 `MCP.md`）
- Usage cache tokens（`CacheReadInputTokens` / `CacheWriteInputTokens`）一等公民

**仍要偿还的代价**：
- 多模态 Client 大量重复
- 中间件框架未统一到抽象层
- RAG 五段式硬编码
- 能力矩阵（VectorStore operator support）未显式化
- 零测试覆盖让所有重构有风险

**结论**：架构骨相正确。问题在于**抽象还没提升到与骨相同等的高度**。再做一轮「找重复 → 抽基类 / 抽接口 → 补缺口」的重构，就能达到生产级质量。短期改动的 ROI 排序见 §10。
