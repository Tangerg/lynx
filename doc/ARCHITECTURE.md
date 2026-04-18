# Lynx 系统架构分析

> 从架构层面审视 `Tangerg/lynx` 的分层、抽象、可扩展性与设计张力。
> 本文档与 `IMPROVEMENTS.md` 互补：后者聚焦战术问题，本文聚焦战略问题。

---

## 1. 模块拓扑

```
┌─────────────────────────────────────────────────────────────┐
│                        上层应用                              │
└────────────┬────────────────────────────────────────────────┘
             │
      ┌──────▼───────┐       ┌──────────────┐   ┌────────────┐
      │  tools/      │       │ models/      │   │vectorstores│
      │ (ToolBinding)│       │ openai       │   │ qdrant     │
      │              │       │ anthropic    │   │ milvus     │
      │              │       │ google       │   │ pinecone   │
      │              │       │              │   │ weaviate   │
      │              │       │              │   │ chroma     │
      └──────┬───────┘       └──────┬───────┘   └─────┬──────┘
             │                      │                 │
             │  all depend on       │                 │
             └───────┬──────────────┴────────┬────────┘
                     │                       │
              ┌──────▼──────┐         ┌──────▼───────┐
              │   core/     │◀────────│   pkg/       │
              │ (契约 + RAG) │         │ (通用工具)    │
              └─────────────┘         └──────────────┘
```

`go.work` 使 5 个子模块共开发；对外以独立 Go module 发布。**核心假设**：`core` 定义抽象与协议，`models / vectorstores / tools` 只做适配实现，`pkg` 是底层工具库——外部不应直接依赖。

**拓扑评价**：
- ✅ **分层清晰**：依赖方向单向（外层 → core → pkg），无循环。
- ⚠️ **版本漂移**：见 IMPROVEMENTS.md H-1，`models` 与 `vectorstores` 各自 pin 不同 commit 的 core，在单仓 go.work 下看不出来，但对外发布时会让下游用户踩坑。
- ⚠️ **pkg 边界模糊**：25 个子包没有分层，`result / retry / sync / safe / stream` 这类通用工具若渗透进 core 的公开 API 会造成依赖扩散。

---

## 2. 核心抽象：`Handler[Req, Res]` 作为「代数基元」

```go
// core/model/handler.go
type CallHandler[Req, Res any] interface {
    Call(ctx context.Context, req Req) (Res, error)
}
type StreamHandler[Req, Res any] interface {
    Stream(ctx context.Context, req Req) iter.Seq2[Res, error]
}
```

**架构意义**：整个库的 Chat、Embedding、Image、Audio、Moderation 统一在 `(Req → Res)` 这一对语义最小的泛型上。这是整个库最优雅的决策——把「调用」与「流式」两种交互抽象成语义无关的代数算子，让中间件可以通用化。

**与 Spring AI 的对比**：
- Spring AI 走继承：`ChatModel extends StreamingChatModel extends Model<Req, Res>`。
- Lynx 走组合：`chat.Model = CallHandler[*Request, *Response] + StreamHandler[*Request, *Response] + Options 方法`。

组合胜出之处：测试 mock 更容易、类型参数显式、能用 `iter.Seq2`（Go 1.23 迭代器）做流式而不发明私有的 Mono/Flux。

**隐藏代价**：
- `chat/client.go:13-19` 里用类型别名 `type CallHandler = model.CallHandler[*Request, *Response]` 把 `*Request/*Response` 钉死；如果下游希望扩展 Request 结构（加字段、替换 Message 类型），需要 fork 整个 chat 包。Spring AI 通过继承可以自然扩展——Lynx 牺牲了这条路径。

---

## 3. 四层洋葱：Model → Client → Middleware → Tool/Memory/Parser

```
┌────────────────────────────────────────────────────┐
│  Layer 4: 用户 API                                  │
│   • chat.Client（fluent builder）                   │
│   • ClientRequest → ClientCaller / ClientStreamer   │
├────────────────────────────────────────────────────┤
│  Layer 3: MiddlewareManager                         │
│   • 用户注册的 Middleware（按逆序包装，洋葱模型）    │
│   • ⚠️ ToolMiddleware 在此层被硬编码注入            │
├────────────────────────────────────────────────────┤
│  Layer 2: Handler 抽象                              │
│   • CallHandler / StreamHandler（纯函数链）          │
├────────────────────────────────────────────────────┤
│  Layer 1: Model 实现                                │
│   • openai / anthropic / google                     │
└────────────────────────────────────────────────────┘
```

#### 3.1 洋葱模型执行顺序

`MiddlewareManager.BuildCallHandler` 按**注册逆序**包装：第一个注册的中间件位于最外层，最后一个最贴近 Model。这是经典写法，与 `net/http` 的 `mux.Use()` 一致，Go 开发者零学习成本。

#### 3.2 ToolMiddleware 硬编码问题（架构级瑕疵）

`core/model/chat/client.go:378-380` 的模式：

```go
if len(req.Options.Tools) > 0 {
    callMW, _ := NewToolMiddleware()
    callHandler = callMW(callHandler)
}
```

**为什么是架构瑕疵**：
1. 破坏「中间件是一等公民」的抽象：其他中间件由用户注册、可配置顺序、可替换；ToolMiddleware 却由框架隐式注入。
2. 用户无法：
   - 换掉工具编排逻辑（例如改用并行执行、带 retry/超时的执行）
   - 控制 ToolMiddleware 与其他中间件的相对位置（例如让 caching 在 tool 外层还是内层）
   - 禁用 tool（即使传了 Tools，也总会生效）
3. 实现代码同一逻辑在 `ClientCaller.call()` 与 `ClientStreamer.stream()` 重复一次，暗示这是「缺少合适抽象」的补丁。

**架构级修法**：提供默认 middleware 列表，用户可 replace：
```go
NewClient(model, WithDefaultMiddlewares(ToolMiddleware{}, MemoryMiddleware{}))
// 用户可以：WithMiddlewares(myCustomToolOrchestrator)
```

#### 3.3 Memory Middleware 的「原地变更」问题

`core/model/chat/memory/memory_middleware.go` 通过在 `Message.Meta()[SavedMarkerKey]` 上打标记表达「已存盘」。这是**有状态副作用**：
- 同一个 Message 被多 Client 复用时会互相污染；
- 多 Memory Middleware 串联时标记冲突；
- 值语义消失——Message 不再是 immutable data。

**架构级修法**：把 saved 状态放在 middleware 的本地 map，用 Message 的身份哈希去重；或让 Message 是 immutable、通过新对象承载「已保存」语义。

---

## 4. RAG：固定五阶段流水线 vs DAG

```
 Query
   ↓
 [1] Transform      ── seq: QueryTransformer[]
   ↓
 [2] Expand         ── 1→N: QueryExpander
   ↓
 [3] Retrieve       ── parallel: DocumentRetriever[] (errgroup)
   ↓
 [4] Refine         ── seq: DocumentRefiner[]
   ↓
 [5] Augment        ── (Query + Docs) → 新 Query
   ↓
 augmented Query
```

**`core/rag/pipeline.go:217-248` 的五段式**：阶段顺序硬编码在 `Execute()`。

**架构评价**：
- ✅ **直观**：五段对应标准 RAG 论文的 pre-retrieval / retrieval / post-retrieval。新手上手快。
- ✅ **Retrieve 并行**：多路召回用 `errgroup` 并发执行，符合工业实践。
- ❌ **阶段不可重排**：想在 Refine 和 Augment 之间插入 dedup？要 fork。
- ❌ **阶段间无条件跳转**：无法「top-1 得分 > 0.95 就跳过 refine」。
- ❌ **无阶段内旁路/并行分支**：「语义检索 + BM25 并行**再融合**」做不到；只能写一个 DocumentRetriever 内部自己实现。
- ❌ **与 chat Middleware 的二元割裂**：RAG 有自己的 `PipelineMiddleware`（包裹 `Pipeline.Execute`），chat 有 `CallMiddleware`。两套语义相近但不兼容。

**建议的架构演进**：
- **方案 A（温和）**：在五阶段外加 `BeforeStage / AfterStage` 钩子（类似 Rails callback），允许插入但不破坏现有 API。
- **方案 B（激进）**：把 Pipeline 改为 `Stage` 接口的有序列表，默认 5 个 stage，用户可插/删/换，保留并行 hint。
- **方案 C（终极）**：DAG 执行器（节点 = Handler，边 = 依赖）。这是 LangChain LCEL 的路线，代价是 API 复杂度指数上升，对 90% 用户是负资产。

**取舍**：方案 B 最平衡。

---

## 5. Document Pipeline：架构最漂亮的子系统

```
Reader  →  Transformer*  →  Batcher  →  Writer
(源头)     (切分/富化)      (按 token)   (落盘/入库)
```

- 所有阶段都是 `[]*Document → []*Document` 的纯函数。
- `Document` 内嵌 `Formatter`，支持 `MetadataModeAll/Embed/Inference/None`——**晚绑定的格式化**，不需要拷贝 Document。
- `TokenCountBatcher` 按 token 限额打包，单条超限直接报错（不静默丢弃）。

**评价**：这是整个库最干净的子系统，每个抽象都只干一件事，阶段之间靠数据结构连通，无隐藏状态。**这里的设计美学应该扩散到 RAG Pipeline**。

**唯一的架构瑕疵**：`Reader.Read()` 返回 `[]*Document`（全量）而非 `iter.Seq2[*Document, error]`（流式）。大数据集下会 OOM。考虑到 Go 1.23 已有 `iter`，应该统一到迭代器。

---

## 6. VectorStore + Filter DSL：过度工程还是必要抽象？

```
core/vectorstore/
├── vector_store.go              # 三件套接口：Creator / Retriever / Deleter
└── filter/
    ├── lexer/                   # 词法分析
    ├── parser/                  # 语法分析
    ├── token/                   # token 类型
    ├── ast/                     # AST 节点：Ident/Literal/BinaryExpr/UnaryExpr/IndexExpr/ListLiteral
    └── visitors/                # AST 遍历框架
                                 
vectorstores/{qdrant,milvus,...}/
└── visitor.go                   # 每个 store 实现自己的 Visitor
                                 # AST → 该 store 原生 Filter
```

#### 6.1 这是一门真·DSL

不是键值 map，而是：
- 逻辑算子 `AND / OR / NOT`
- 比较算子 `== != < <= > >=`
- 集合算子 `IN`
- 模式算子 `LIKE`
- 嵌套表达式 `(a > 1 AND b IN [x,y]) OR c == "z"`

#### 6.2 抽象层次：对

- ✅ **Visitor 模式天然适合 AST→目标方言的翻译**，添加新 store 只需写一个 visitor，不污染 core。
- ✅ **Filter 在 `RetrievalRequest` / `DeleteRequest` 中传递 AST（非字符串）**：统一了「应用层构造查询」与「DSL 字符串解析」两种入口。

#### 6.3 但有两个架构级问题

1. **能力矩阵不统一**：Pinecone 不支持 `LIKE`、Milvus 对 nested field 有限制，但 DSL 允许你构造任意表达式，错误只在运行时暴露。**应该在接口层提供 `SupportedOperators() set[Op]` 能力声明**，让上层能提前检查或降级。

2. **五套 visitor 重复建设**：每个 store 的 visitor 都要处理 AND/OR/NOT 的布尔代数、字面量类型映射、NULL 处理。**核心层应提供 `BaseVisitor` 模板**（模板方法模式），子类只需实现叶子节点翻译；目前五个文件各写 500-800 行，重复度极高。

#### 6.4 是否过度工程？

对「只要按 user_id 过滤」的初学者：是。对「多租户 + 多条件 + 嵌套逻辑」的生产系统：不是。**建议保留 DSL，但补一个流式 builder**：

```go
filter.Eq("user_id", 123).And(filter.In("category", "tech", "news"))
```

让 80% 的简单场景不需要学词法/语法规则。

---

## 7. Middleware：三种实例，没有共同祖先

| 维度 | ChatMiddleware | RAGPipelineMiddleware | ToolMiddleware |
|-----|---------------|----------------------|----------------|
| 签名 | `(CallHandler) → CallHandler` | 包裹整个 Pipeline.Execute | 硬编码进 Client |
| 数据 | `*chat.Request → *chat.Response` | `*Query → (Query, []Document)` | 同 ChatMiddleware |
| 注入 | 用户注册 | 通过 ChatMiddleware 桥接 | **自动注入** |

**症结**：Lynx 有「装饰器」的**实现**，没有「Middleware」的**接口**。三类中间件是三条独立进化线。Embedding/Image/Audio/Moderation 甚至**完全没有中间件故事**——无法加 logging / caching / rate-limit，除非用户自己从 Handler 层手写装饰器。

**架构级改进**：
```go
// core/middleware/middleware.go
type Middleware[H any] func(next H) H

// 统一的 Manager
type Manager[H any] struct { middlewares []Middleware[H] }
func (m *Manager[H]) Build(endpoint H) H { /* reverse wrap */ }
```

`chat`、`embedding`、`image` 全部基于同一套 `Manager[CallHandler[Req,Res]]` 实例化。这样 RAG、Tool、Memory、Cache、RateLimit 就都能组合。

---

## 8. 多模态：四个 Client 各自复制

`chat.Client`、`embedding.Client`、`image.Client`、`audio.Client` 都在做同一件事：
1. 持有一个 Model
2. 提供 fluent builder
3. 通过 `Call()` / `Stream()` 触发 Handler

**DRY 违反度**：≈ 80%。

**架构级改进**：
```go
// core/model/client.go
type BaseClient[Req, Res any] struct {
    model   Model[Req, Res]
    manager *middleware.Manager[CallHandler[Req, Res]]
    defaults Req
}

// chat/client.go
type Client struct { *BaseClient[*Request, *Response] }
// 每个模态只加自己的 fluent 方法
```

当前结构每加一个模态要重写 ~200 行 boilerplate。

---

## 9. 可扩展性打分表

| 扩展方向 | 打分 | 说明 |
|---------|-----|------|
| 添加 Model Provider | 9/10 | 实现 `chat.Model` 即可；model/内 provider 均照此模式 |
| 添加 VectorStore | 8/10 | 实现三件套 + visitor；visitor 有重复 |
| 添加自定义 Middleware（Chat） | 8/10 | 装饰器模式，清晰 |
| 添加自定义 Middleware（其他模态） | **3/10** | **无现成故事**，要从 Handler 层手写 |
| 添加 RAG Stage | **4/10** | 阶段数量固定，要 fork |
| 添加 Filter 操作符 | **3/10** | 要改 lexer/parser/所有 visitor |
| 自定义 Tool 编排策略 | **4/10** | ToolMiddleware 硬编码 |
| 自定义 Document Transformer | 9/10 | 实现接口即可 |
| 替换 Prompt 模板引擎 | 6/10 | `PromptTemplate` 是接口，但 default impl 假设 Go text/template |
| 自定义 Memory 存储 | 8/10 | `memory.Store` 清晰 |

**短板集中在**：非 chat 模态的中间件、RAG 阶段、Tool 编排、Filter 语法扩展。

---

## 10. 架构级改进建议（战略清单）

> 这些不是小修小补，是要涉及跨文件的重构，建议按季度规划。

### 10.1 统一 Middleware 框架 🔴 高价值
- 抽出 `core/middleware/Manager[H]`
- Chat / Embedding / Image / Audio / Moderation / RAG 全部切换到统一 Manager
- 把 ToolMiddleware 从「硬编码注入」变成「默认注册、可替换」

### 10.2 BaseClient[Req, Res] 泛型基类 🔴 高价值
- 消除 4 份 Client boilerplate
- 未来加 Video / 3D / 新模态时零成本

### 10.3 RAG Pipeline 插槽化 🟠 中价值
- 保留五阶段默认结构，但允许 `Before/After` 钩子与阶段替换
- 为条件跳转预留 `ShouldSkip(ctx, stageInput)` 钩子

### 10.4 VectorStore BaseVisitor 模板 🟠 中价值
- `core/vectorstore/filter/visitors.BaseVisitor` 实现布尔代数骨架
- 5 个 store 的 visitor 变成只写叶子节点翻译
- 预计减少 ~60% 代码量

### 10.5 Filter DSL 能力声明 🟠 中价值
- `VectorStore.Capabilities() Capabilities`
- RetrievalRequest 构造时做静态验证，避免运行时才报「不支持 LIKE」

### 10.6 Document Reader 流式化 🟡 低价值
- `Read()` 返回 `iter.Seq2[*Document, error]`
- 大数据集 RAG ingest 无需全量驻留

### 10.7 Message immutable 化 + MemoryMiddleware 状态外置 🟠 中价值
- Message 变成值类型 / 只读接口
- Saved 状态走 middleware 内部 map

### 10.8 跨模块 core 版本对齐 🔴 高价值（运维）
- go.work 下层 pin 到同一 core commit
- CI 校验 `models/go.mod` 与 `vectorstores/go.mod` 的 `require core ...` 一致

---

## 11. 总评

**定位**：Lynx 是「**Go idiom 友好 + Spring AI 功能对标**」的 LLM 框架雏形。这是一个非常清晰的技术品味——不是 Python 框架的直译，也不是重复造轮子。

**设计品味**（✅ 值得保留）：
- 以 `CallHandler[Req, Res]` 为代数基元
- 原生 `iter.Seq2` 做流式
- Document pipeline 的四段式
- VectorStore 用 Filter DSL 做 lingua franca
- 显式的 Middleware 洋葱

**品味的代价**（⚠️ 需要偿还）：
- 具体 Request/Response 被类型别名钉死
- 中间件框架没有统一到抽象层
- RAG 五段式有硬编码味道
- Tool 编排逻辑不可替换
- 多模态 Client 大量重复
- 能力矩阵（operator support）未显式化
- 零测试覆盖使所有重构都有风险

**综合结论**：架构的**骨相**正确，问题在于**抽象还没提升到与骨相同等的高度**。再做一轮「找重复 → 抽基类 / 抽接口 → 补缺口」的重构，就能达到生产级质量。短期改动的 ROI 排序见 §10。

---

*（与战术清单配套阅读：`IMPROVEMENTS.md`）*
