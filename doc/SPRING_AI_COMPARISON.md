# Lynx vs Spring AI 深度代码对比

> Lynx 最初仿照 Spring AI 架构设计。本文对比两者实际代码实现，识别 Lynx 忠实继承、Go 风格改进、意外缺失、和意外倒退之处。
> Spring AI 代码参考：`spring-projects/spring-ai` main 分支（1.0 GA 后）。
> Lynx 代码参考：`/Users/tangerg/Desktop/lynx`。

---

## 0. 核心结论先说

| 维度 | Lynx 对齐度 |
|-----|-----------|
| **核心心智模型**（Model → ChatClient → Advisor 链 → Tool → Memory） | **~80% 忠实** |
| **Go 惯用法改进**（流式、泛型、错误处理、ctx 传播） | **4 处明确胜出** |
| **必须回填的 Spring AI 关键概念** | **7 处缺失** |
| **设计倒退**（原本可以照抄却改坏了） | **6 处明确倒退** |

**一句话**：Lynx 抓住了 Spring AI 的「骨架」，但把 Spring 刻意**暴露给用户的扩展点**（Advisor Ordering、ToolCallingManager、AbstractFilterExpressionConverter、QueryRouter、DocumentJoiner）简化成了**硬编码的闭合逻辑**——这些是最值得回填的。

---

## 1. 抽象映射全表

### 1.1 Chat / Model / Client 层

| Spring AI | Lynx | 判定 |
|-----------|------|-----|
| `Model<TReq extends ModelRequest<?>, TRes extends ModelResponse<?>>` | `model.CallHandler[Req, Res any]`（`core/model/handler.go:22`） | ✅ Go 泛型改进（取消上界约束） |
| `StreamingModel<TReq, TResChunk>` + `Flux<TResChunk>` | `model.StreamHandler` + `iter.Seq2[Res, error]`（handler.go:59） | 🏆 **Go 胜**：pull-based 迭代器取代 Reactor |
| `ChatModel extends Model + StreamingModel` | `chat.Model`（组合两个 Handler）（model.go:56） | ✅ 忠实 |
| `ChatClient` 流式 API（`prompt().call()/stream()`） | `chat.Client → ClientRequest → ClientCaller/ClientStreamer`（client.go） | ✅ 忠实 |
| `Prompt`（messages + options） | `chat.Request`（Messages + Options + **Params**） | ⚠️ 偏离：多了 Params |
| `ChatOptions` 接口 + nullable getter + `copy()/mutate()` | `chat.Options` struct + `*float64` 指针 + `Clone()/MergeOptions()` | ✅ 忠实（Go 用指针表达 nullable） |
| `Message` 密封体系（User/System/Assistant/Tool） | 密封 interface + 未导出 `message()` 方法 | ✅ 忠实 |
| `ChatResponse`+`Generation` | `Response`+`Result` | ✅ 忠实 |
| `Advisor extends Ordered` + `getName() + getOrder()` | **无**；中间件 = `func(next H) H` | ❌ **倒退**（见 §2） |
| `CallAdvisor.adviseCall(req, chain)` | `CallMiddleware` + `MiddlewareManager` | ⚠️ 简化 |
| `AdvisorChain` + `ObservationRegistry` | 无显式链对象 | ❌ 缺失 |
| `ChatClientRequest.context: Map<String,Object>` + `ChatClientResponse.context` | `Request.Params` 单向 | ⚠️ 响应侧无 context |
| `ChatMemory.add/get/clear` | `memory.Store = Reader+Writer+Clearer` | 🏆 **Go 胜**：ISP 拆接口 |
| `MessageWindowChatMemory` | `MessageWindowStore` | ✅ 忠实（Lynx 多了 [10,100] 范围保护） |
| `MessageChatMemoryAdvisor` (order 常量) | `chatMemoryMiddleware` | ❌ **倒退**：order 常量丢失 |
| `ToolCallback`+`FunctionToolCallback` | `chat.Tool + CallableTool` | ⚠️ 仅 string→string |
| `ToolCallbackProvider` | **无** | ❌ 缺失 |
| `ToolCallingManager`（可注入） | `ToolMiddleware`（硬编码注入） | ❌ **倒退**（见 §3） |
| `ToolCallingChatOptions.isInternalToolExecutionEnabled` | **无** | ❌ 缺失 |
| `PromptTemplate`（StringTemplate） | `PromptTemplate`（text/template） | ✅ 忠实 |
| `StructuredOutputConverter<T>` | `StructuredParser[T]` | ✅ 忠实（Go 泛型） |
| `ObservationRegistry` / Micrometer | **无** | ❌ 缺失 |

### 1.2 RAG / VectorStore / Document / Filter 层

| Spring AI | Lynx | 判定 |
|-----------|------|-----|
| `Query` record（text + history + context） | `rag.Query`（Text + **Extra map[string]any**） | ⚠️ history 被 stringly-typed |
| `RetrievalAugmentationAdvisor` | `rag.Pipeline`+`pipelineMiddleware` | ❌ 闭合结构 vs 组合策略（见 §4） |
| `QueryTransformer` | `rag.QueryTransformer` | ✅ 忠实 |
| `QueryExpander` / `MultiQueryExpander` | 同名 | ✅ 忠实 |
| `DocumentRetriever` / `VectorStoreDocumentRetriever` | 同名 | ✅ 忠实 |
| `QueryRouter` / `AllRetrieversQueryRouter` | **无**（硬编码跑全部 retriever） | ❌ 缺失 |
| `DocumentJoiner` / `ConcatenationDocumentJoiner` | **无**（implicit append） | ❌ 缺失 |
| `DocumentPostProcessor` / `DocumentRanker` / `DocumentSelector` | `DocumentRefiner`（rank/dedupe） | ✅ 改名，角色一致 |
| `QueryAugmenter` / `ContextualQueryAugmenter` | 同名 | ✅ 忠实（prompt 几乎字面抄 Spring） |
| `Document`（Builder + Double Score） | `Document`（public fields + float64 Score） | ❌ **倒退**：无 builder、Score 无法表达「无分数」 |
| `DocumentReader extends Supplier<List<Document>>` | `Reader.Read(ctx) ([]*Document, error)` | 🏆 **Go 胜**：ctx + error |
| `DocumentTransformer extends Function` | `Transformer.Transform(ctx, docs) (..., error)` | 🏆 同上 |
| `DocumentWriter extends Consumer` | `Writer.Write(ctx, docs) error` | 🏆 同上 |
| `TokenCountBatchingStrategy` | `TokenCountBatcher` | ✅ 算法几乎完全照抄 |
| `IdGenerator.generateId(Object...)` | `id.Generator.Generate(ctx, ...any)` | ✅ 忠实 |
| `Document.Builder.idGenerator(IdGenerator)` | **未集成** | ❌ 缺失 |
| `VectorStore` | `VectorStore = Creator+Retriever+Deleter+Info` | 🏆 ISP 拆接口 |
| `SearchRequest` | `RetrievalRequest` | ✅ 忠实 |
| `Filter.Expression`（Tagged triple AST） | `ast.BinaryExpr/UnaryExpr/IndexExpr/...`（compiler-grade AST） | ⚠️ Lynx AST 更重 |
| `FilterExpressionBuilder` 流式 | `filter.Builder` | ✅ 忠实（Lynx 加了默认 AND 链式更友好） |
| `FilterExpressionConverter` + **`AbstractFilterExpressionConverter` 模板方法基类** | `ast.Visitor` 每 store 自己实现 | ❌ **严重倒退**（见 §5） |
| `FilterExpressionTextParser`（ANTLR4 `Filters.g4`） | 手写 lexer(478行) + parser(512行) | ⚠️ 重复造轮子（见 §5） |
| `isNull` / `isNotNull` | **无** | ❌ 缺失 |

---

## 2. 核心对比一：Advisor ↔ Middleware

### 2.1 Spring AI 的 Advisor 接口

```java
// Advisor.java
public interface Advisor extends Ordered {
    String getName();
    int DEFAULT_CHAT_MEMORY_PRECEDENCE_ORDER = Ordered.HIGHEST_PRECEDENCE + 1000;
}

// CallAdvisor.java
public interface CallAdvisor extends Advisor {
    ChatClientResponse adviseCall(ChatClientRequest req, CallAdvisorChain chain);
}
```

Spring 的每个 Advisor 都：
- **有名字**（`getName()`）→ 调试/日志/observability 友好
- **有顺序**（`getOrder()`）→ `DefaultAroundAdvisorChain.reOrder()` 用 `OrderComparator.sort` 排序
- **能访问 chain 对象**（`chain.nextCall(request)`）→ 可 introspect 其他 advisor，可复制子链重跑

### 2.2 Lynx 的 Middleware 做法

```go
// core/model/chat/client.go:17
type CallHandler    = model.CallHandler[*Request, *Response]
type CallMiddleware = model.CallMiddleware[*Request, *Response]

// middleware 是匿名闭包：func(next CallHandler) CallHandler
```

### 2.3 逐项对比

| 维度 | Spring AI | Lynx | 判决 |
|-----|-----------|------|-----|
| 顺序控制 | `getOrder()` + 常量 | 仅注册顺序 | Lynx 倒退 |
| 共享上下文 | request.context **+ response.context** | 仅 `Request.Params` | Lynx 单向 |
| chain 对象可见 | 是（可 introspect） | 否 | Lynx 更简单但封闭 |
| 流式 | `Flux<...>` + `contextView`/`ObservationThreadLocalAccessor` | `iter.Seq2` | 🏆 **Lynx 胜**：无 Reactor 负担 |
| observation 专用 hook | 有（`AroundCallObservation` 等） | 无 | Lynx 缺失 |

### 2.4 最值得回填的

**两行 Go 代码就能加上 ordering**：

```go
type OrderedMiddleware interface { Order() int }

// 在 MiddlewareManager.BuildCallHandler 中：
slices.SortStableFunc(m.callMiddlewares, func(a, b CallMiddleware) int {
    oa := orderOf(a)  // 接口断言或默认 0
    ob := orderOf(b)
    return oa - ob
})
```

让 `chatMemoryMiddleware` 默认 order = `DefaultChatMemoryPrecedenceOrder`，用户注册顺序再混乱也能保证 memory 在外层。

---

## 3. 核心对比二：ToolCallingManager vs ToolMiddleware

### 3.1 Spring AI：外置可替换

```java
// ToolCallingManager.java
public interface ToolCallingManager {
    List<ToolDefinition> resolveToolDefinitions(ToolCallingChatOptions chatOptions);
    ToolExecutionResult executeToolCalls(Prompt prompt, ChatResponse chatResponse);
}
```

`ChatModel` 实现（OpenAI、Anthropic…）**持有一个 `ToolCallingManager` 字段**，用户可注入自定义实现。关键开关：

```java
ToolCallingChatOptions.isInternalToolExecutionEnabled()  // 默认 true
// 设为 false → 工具调用原样返回给 caller，由上层决定如何执行
```

这是 **agentic 代理模式的基石**：允许应用层把 tool calls 转发给其他服务执行（例如 MCP server、外部 agent），而不是在进程内 loop。

### 3.2 Lynx：硬编码 + 无开关

```go
// core/model/chat/client.go:378-380
if len(req.Options.Tools) > 0 {
    callMW, _ := NewToolMiddleware()
    callHandler = callMW(callHandler)   // 强制最外层包裹
}
```

- `ToolMiddleware` 是空 struct，**无参构造器**，无法注入自定义 invoker/registry/返回策略
- `Options.Tools` 非空 ⇒ 工具**总是在进程内执行**
- `ClientCaller.call` 和 `ClientStreamer.stream` 里同样逻辑写两遍，暗示缺少合适的注入点

### 3.3 最小必要修复

```go
// 1. 把硬编码改成默认注册（但可替换）
type ToolExecutionMode int
const (
    ToolExecutionInternal ToolExecutionMode = iota  // 默认：进程内递归
    ToolExecutionExternal                            // 返回 raw tool calls
    ToolExecutionCustom                              // 调用用户提供的 ToolInvoker
)

type ToolInvoker interface {
    Invoke(ctx context.Context, call *ToolCall) (*ToolResult, error)
}

// 2. Options 加开关
type Options struct {
    Tools         []Tool
    ToolMode      ToolExecutionMode
    CustomInvoker ToolInvoker  // 当 ToolMode == Custom 时
}
```

这 10 行改动打开 agentic 场景。

---

## 4. 核心对比三：RAG = Advisor vs RAG = Pipeline

### 4.1 Spring AI 的组合式

```java
// RetrievalAugmentationAdvisor.before()
Query originalQuery = Query.builder().text(...).history(...).context(context).build();

Query transformedQuery = originalQuery;
for (var qt : this.queryTransformers) transformedQuery = qt.apply(transformedQuery);

List<Query> expanded = this.queryExpander != null 
    ? this.queryExpander.expand(transformedQuery) 
    : List.of(transformedQuery);

Map<Query,List<List<Document>>> perQuery = expanded.stream()
    .map(q -> CompletableFuture.supplyAsync(() -> getDocsFor(q), taskExecutor))
    .toList().stream().map(CompletableFuture::join).collect(...);

List<Document> docs = this.documentJoiner.join(perQuery);   // ← 可插拔
for (var pp : this.documentPostProcessors) docs = pp.process(originalQuery, docs);

Query augmented = this.queryAugmenter.augment(originalQuery, docs);
```

**每个节点都是注入的策略对象**：`queryTransformers`、`queryExpander`、`queryRouter`、`documentRetrievers`、`documentJoiner`、`documentPostProcessors`、`queryAugmenter`、**`taskExecutor`**、**`scheduler`**、**`observationRegistry`**。

### 4.2 Lynx 的闭合式

```go
// core/rag/pipeline.go:217-248
transformed, err := p.transformQuery(ctx, query)
expanded,   err := p.expandQuery(ctx, transformed)
retrieved,  err := p.retrieveByQueries(ctx, expanded)
refined,    err := p.refineDocuments(ctx, query, retrieved)
augmented,  err := p.augmentQuery(ctx, query, refined)
```

五段硬编码在 `Pipeline.Execute` 里。`retrieveByQueries` 内部用 `errgroup.WithContext` 并行跑所有 retriever，**无 QueryRouter、无 DocumentJoiner**。

### 4.3 Lynx 的两个亮点

1. **Lynx 的部分失败策略更实用**：`retrieveByQuery` 即使部分 retriever 失败，也返回已成功的文档；Spring 的 `CompletableFuture.join` 一个失败全盘崩溃。
2. **Lynx 用 `iter.Seq2` 做 stream 中间件比 Spring 的 Reactor 集成更轻**：`pipeline_middleware.go::executeStream` 无 `Flux.deferContextual` 之类的 ceremony。

### 4.4 Lynx 的四个缺失

1. **QueryRouter**：无法按 query 特性路由到不同 retriever（例如语义检索 vs BM25）
2. **DocumentJoiner**：无法自定义「多 retriever × 多 query」结果的合并策略（interleave、RRF 融合等）
3. **TaskExecutor 注入**：并发线程池不可配
4. **阶段插桩**：无法在两阶段间插 cache / tracing / 预过滤

---

## 5. 核心对比四：FilterExpressionConverter 架构（最大的倒退点）

### 5.1 Spring AI：模板方法基类

```java
// AbstractFilterExpressionConverter.java
public abstract class AbstractFilterExpressionConverter implements FilterExpressionConverter {
    public final String convertExpression(Filter.Expression e) {
        StringBuilder ctx = new StringBuilder();
        this.doExpression(e, ctx);    // 模板方法
        return ctx.toString();
    }
    protected abstract void doExpression(Expression e, StringBuilder ctx);
    protected abstract void doKey(String key, StringBuilder ctx);
    protected abstract void doSingleValue(Object v, StringBuilder ctx);
    protected void doNot(Expression e, StringBuilder ctx) { /* 默认实现 */ }
    protected void doStartGroup(...); protected void doEndGroup(...);
    // 静态工具：normalizeDateString, emitLuceneString, emitJsonValue...
}
```

每个具体 store（`QdrantFilterExpressionConverter`、`PineconeFilterExpressionConverter`、`MilvusFilterExpressionConverter`…）**只覆盖差异点**，**~100-200 LOC**。

### 5.2 Lynx：每 store 独立 Visitor

```go
// core/vectorstore/filter/ast/ast.go
type Visitor interface { Visit(expr Expr) Visitor }

// vectorstores/qdrant/visitor.go     → 798 行
// vectorstores/milvus/visitor.go     → ~700 行
// vectorstores/pinecone/visitor.go   → ~600 行
// vectorstores/weaviate/visitor.go   → ~600 行
// vectorstores/chroma/visitor.go     → ~600 行
```

每个 visitor **从零实现**：`visitBinaryExpr`、`visitUnaryExpr`、`visitIndexExpr`、`visitIdent`、`visitLiteral`、`visitListLiteral`、`visitLogicalExpr`、`visitEqualityExpr`、`visitOrderingExpr`、`visitInExpr`、`visitLikeExpr`，外加 `currentFieldKey` / `currentFieldValue` 状态管理。

### 5.3 量化差距

| 指标 | Spring AI | Lynx | 差距 |
|-----|-----------|------|-----|
| 基类模板 LOC | ~300 | 0 | — |
| 每 store LOC | 100-200 | 600-800 | **~4x** |
| 5 store 总 LOC | ~750 | ~3300 | **~4.4x** |
| 新增一个 store 成本 | ~150 行 | ~700 行 |  |

**这是整个 Lynx 架构上最明确的倒退**。修复方案：在 `core/vectorstore/filter/visitors/` 提供 `BaseVisitor`，embedding + 方法覆盖方式让 5 个 store 只写叶子翻译。**预计减少 ~2500 LOC**。

### 5.4 Parser 重复造轮子（次要但值得讨论）

Spring AI：`Filters.g4` ANTLR 语法文件 **~80 行** → 自动生成完整解析器。

Lynx：
- `filter/lexer/lexer.go` 478 行
- `filter/parser/parser.go` 512 行
- `filter.g4` + `filter.ebnf` 旁挂（但只用于文档）
- 合计 **~1000 行手写解析器**

Go 生态没有与 ANTLR 同级的成熟工具，但 [participle](https://github.com/alecthomas/participle) 或 goyacc 可以做到接近 Spring 的体验。**当前做法不是错的，但维护成本是 Spring 的 10 倍**。

**更重要的问题**：Spring AI 把字符串解析定位为**便利层**——`Filter.Expression` AST 是主入口。Lynx 的 `pipeline_middleware` 是把 filter **以字符串形式塞进 `Query.Extra[FilterExprKey]`**，然后 VectorStoreDocumentRetriever 再解析——这让 parser 成为热路径，**无法简单移除**。

---

## 6. Memory 子系统：Lynx 的「已保存」标记问题

### 6.1 Spring AI 如何避开此问题

`MessageChatMemoryAdvisor` 的 `before()` 和 `after()` 分工明确：
- `before()`：从 memory 取 history → 塞进 prompt
- `after()`：**只保存 `chatResponse().getResults()...getOutput()`**（模型输出）+ builder 在构造 prompt 时自动保存过一次的 user message

因为**两套消息永不混在一起**，不需要「已保存」标记。

### 6.2 Lynx 的实现

```go
// core/model/chat/memory/memory_middleware.go:14
const SavedMarkerKey = "lynx:ai:model:chat:memory:saved_marker"

// :111-114
for _, msg := range history {
    m.markMessageAsSaved(msg)   // 在 msg.Meta() 里写标记
}
```

写入**调用方持有的 Message.Meta map**，属于副作用泄漏：
- 同一个 Message 指针在多会话间共享时标记污染
- Message 不再是 immutable data

### 6.3 修复建议

方案 A（最小）：把 saved 状态放进 middleware 自己的 `map[msgPtr]struct{}`，不写入 Message。
方案 B（架构级）：效仿 Spring，把 memory advisor 拆成 before（加载历史、拼 prompt）和 after（只存响应），让新旧消息从流程上分开，消除标记需求。

---

## 7. Lynx 确实胜 Spring AI 的地方

### 7.1 🏆 iter.Seq2 取代 Flux
Go 1.23 pull-based 迭代器不需要 Reactor、schedulers、`publishOn`、`contextView` 这一套。Spring 的 `DefaultAroundAdvisorChain.adviseStream` 要处理 `Flux.deferContextual(contextView -> ...)` + `ObservationThreadLocalAccessor`，Lynx 的 stream middleware 就是普通迭代器。

### 7.2 🏆 context.Context + error 全程贯通
`Read/Transform/Write/Retrieve/Refine/Augment` 全都接 `ctx` 返 `error`。Spring 的 `Supplier/Function/Consumer` 链全是无检查异常 + 无取消语义。

### 7.3 🏆 Interface Segregation 拆接口
- `memory.Store = Reader + Writer + Clearer`
- `VectorStore = Creator + Retriever + Deleter + Info`

下游代码可以只依赖最窄的那个（例如 RAG retriever 只依赖 `vectorstore.Retriever`）。Java 单继承没这个便利。

### 7.4 🏆 Go 泛型把 Handler 变成代数基元
`CallHandler[Req, Res]` 不绑定 Chat。理论上 Embedding / Image / Audio middleware 可以共用同一套装饰器（虽然 Lynx 目前没利用这个能力——见 §8.2）。Spring 的 `ChatClient` 死锁在 `Prompt/ChatResponse` 上。

### 7.5 🏆 Splitter 解耦
Lynx 的 `Splitter` + `SplitFunc` 把「切分策略」和「切分机械」分开。Spring 的 `TokenTextSplitter` 是单体类。

### 7.6 🏆 Pipeline 部分失败策略
某个 retriever 失败时 Lynx 返回剩余文档；Spring `CompletableFuture.join` 一挂全挂。

### 7.7 🏆 RichAST：ast 节点带 Position
Lynx 的 ast 节点带 `token.Position Start/End` + `Precedence()` 分类器，报错能精确到字符位置。Spring 的 `Filter.Expression` 是 tagged triple，无 source location。

### 7.8 🏆 Role-interface 化的 ChatMemory
- Spring: 一个 `ChatMemory` 接口带三方法
- Lynx: 三个独立接口（可按 ISP 组合）

---

## 8. Lynx 相对 Spring AI 的倒退点（按影响排序）

### 8.1 🔴 ToolCallingManager 变成硬编码（§3）
**代价**：agentic 场景、自定义 tool 编排、external execution 全都堵死。
**修复**：10 行代码引入 `ToolInvoker` 接口 + `ToolExecutionMode` 枚举。

### 8.2 🔴 AbstractFilterExpressionConverter 基类缺失（§5）
**代价**：5 个 store 重复 ~2500 LOC，新增 store 成本 4 倍。
**修复**：`core/vectorstore/filter/visitors/BaseVisitor` 提供默认遍历 + hook 方法。

### 8.3 🔴 Advisor Ordering 丢失（§2）
**代价**：memory middleware 位置全靠用户记得，注册顺序错了产生 subtle bug。
**修复**：`OrderedMiddleware` interface + `MiddlewareManager` 排序。

### 8.4 🟠 RAG Pipeline 闭合化（§4）
**代价**：无 QueryRouter、无 DocumentJoiner、无阶段插桩、无 TaskExecutor 注入。
**修复**：引入 `Stage` 接口列表 + 可插拔默认；或至少补齐 Router/Joiner。

### 8.5 🟠 Memory 用 SavedMarker 副作用（§6）
**代价**：Message 变成可变共享状态，多会话污染风险。
**修复**：状态外置到 middleware map；或拆 before/after 消除标记需求。

### 8.6 🟠 `Query.history` 和 filter 都 stringly-typed
`ChatHistoryKey`、`FilterExprKey`、`DocumentContextKey`——都是 magic string + `Extra.Get()` + 类型断言。Spring 的 `Query` record 把 `history` 建模成 `List<Message>` 字段。
**修复**：把 `Query` 改成 `Text string; History []chat.Message; Context map[string]any`。

---

## 9. Lynx 缺失但**应该从 Spring AI 回填**的清单

按 ROI 排序，供后续 issue / milestone 规划：

| # | 概念 | Spring 文件 | 代价 | 收益 |
|---|-----|------------|-----|-----|
| 1 | `Ordered` / `getOrder()` | `Advisor.java` | XS（~20行） | 消除一类 subtle bug |
| 2 | `ToolCallingManager` 注入 | `ToolCallingManager.java` | S（~50行） | agentic 模式解锁 |
| 3 | `isInternalToolExecutionEnabled` | `ToolCallingChatOptions.java` | XS | 同上 |
| 4 | `AbstractFilterExpressionConverter` | 同名 | M（~300行） | 减 2500+ LOC |
| 5 | `QueryRouter` 注入点 | `AllRetrieversQueryRouter.java` | S | 多模态/路由检索 |
| 6 | `DocumentJoiner` 注入点 | `ConcatenationDocumentJoiner.java` | S | RRF 融合、交错合并 |
| 7 | `isNull` / `isNotNull` 算子 | `Filter.java` `ExpressionType` | XS | DSL 完备性 |
| 8 | `Document.Builder` + `IdGenerator` 集成 | `Document.java` | S | 去除「ID 必须调用方生成」的负担 |
| 9 | `Document.Score` 改 nullable（`*float64`） | 同上 | XS | 区分「0 分」和「无分数」 |
| 10 | `ToolCallbackProvider` | 同名 | S | 支持 MCP 这类动态 tool 源 |
| 11 | `ToolCallResultConverter` | `FunctionToolCallback.java` | S | tool 结果自动序列化 |
| 12 | `ObservationRegistry` | 同名（Micrometer） | M | tracing/metrics 故事 |
| 13 | `ResponseContext` | `ChatClientResponse.context` | XS | middleware 双向通信 |
| 14 | `AdvisorContext` 独立类型 | `ChatClientRequest.context` | S | 与 request body 解耦 |

---

## 10. 落地执行优先级

### 第一阶段：小手术，大收益（~2 周）
- #1 Middleware Ordering
- #2 + #3 ToolCallingManager + 执行模式开关
- #7 + #9 Filter 补 `isNull`、Document.Score 改指针

### 第二阶段：清理重复（~3-4 周）
- #4 `AbstractFilterExpressionConverter` → 重构 5 个 store visitor
- #8 Document.Builder + IdGenerator 集成
- §6 消除 SavedMarker 副作用

### 第三阶段：RAG 扩展性（~4-6 周）
- #5 + #6 QueryRouter + DocumentJoiner
- §8.6 把 `Query.History` 建模成字段
- Pipeline 引入 `Stage` 接口列表

### 第四阶段：可观测性（~视需求）
- #12 ObservationRegistry（Go 侧可用 OpenTelemetry API）
- #13 + #14 双向 AdvisorContext

---

## 11. 总评

**Lynx 最初仿 Spring AI** 这件事做得好——它抓住了 Spring AI 最有生命力的设计：分层（Model/Client/Advisor/Tool/Memory）、组合（策略注入）、双模式（call + stream）、结构化输出、RAG 抽象。80% 的**核心心智模型是忠实的**。

**但 Lynx 在「扩展点」上简化得太激进**：
- Advisor 变 middleware，丢了 ordering 和 getName
- ToolCallingManager 变硬编码，丢了可替换性
- AbstractFilterExpressionConverter 消失，每 store 从零写
- Pipeline 从可组合的 Advisor 退化成闭合 struct
- Query 的 history 从模型化字段退化成 Extra map 的 magic key

**这些不是 Go 惯用法的胜利**，是**懒抽象**——用 Spring AI 原本解决好的问题交换了更少的代码量。

**好消息是修复都很小**：Ordering 20 行，ToolInvoker 50 行，BaseVisitor 300 行。一个季度之内可以把整个倒退面抹平，同时保住 Lynx 在流式 / 泛型 / context / ISP 上真正赢 Spring 的部分。

**Go 风格胜出不该以放弃 Spring AI 的好抽象为代价**。这是本文的核心判断。

---

*（配套阅读：`IMPROVEMENTS.md` 战术清单、`ARCHITECTURE.md` 架构分析）*
