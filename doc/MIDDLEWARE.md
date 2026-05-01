# 中间件设计

> Call/Stream 类型不对称带来的「双写」问题、`MiddlewareManager` 的简化路径、以及与 Spring AI / Embabel 的对照。
>
> **当前状态**：中间件框架已用，`AroundMiddleware` 是提案、未实现。

---

## 1. 当前形态

```go
// core/model/handler.go
type CallHandler[Req, Res any] interface {
    Call(ctx context.Context, req Req) (Res, error)
}
type StreamHandler[Req, Res any] interface {
    Stream(ctx context.Context, req Req) iter.Seq2[Res, error]
}

// core/model/middleware.go
type CallMiddleware[Req, Res any]   func(next CallHandler[Req, Res])   CallHandler[Req, Res]
type StreamMiddleware[Req, Res any] func(next StreamHandler[Req, Res]) StreamHandler[Req, Res]

// 4 个泛型参数（Call 与 Stream 的 Req/Res 各一对）
type MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse any] struct {
    mu                sync.RWMutex
    callMiddlewares   []CallMiddleware[CallRequest, CallResponse]
    streamMiddlewares []StreamMiddleware[StreamRequest, StreamResponse]
}
```

`chat.MiddlewareManager` 把 4 参数都填同一对：

```go
type MiddlewareManager = model.MiddlewareManager[*Request, *Response, *Request, *Response]
```

`embedding/image/audio/moderation` 5 个模态完全同形——4 参数泛型在实际代码里**永远** Call 和 Stream 用同一对 Req/Res，没有任何使用点用到不同类型。

---

## 2. 痛点：Call/Stream 双写

### 2.1 类型不对称是不可调和的

| 方法 | 返回类型 |
|-----|---------|
| `Call` | `Res` |
| `Stream` | `iter.Seq2[Res, error]` |

没法设计一个函数签名让两者共用同一个实现体——**这是 Go 类型系统决定的硬约束**，而非抽象能力不够。

### 2.2 中间件分两类

#### A 类：模式无关（可合并）

关心「请求→响应」生命周期事件，不关心响应是一次性还是流式：
- Logging：开始/结束记日志
- Metrics：耗时、错误率
- Auth / Rate-limit：请求前校验
- Tracing：一对开启/关闭 span
- Retry（call only）
- Cache（call only）

**Call 和 Stream 里 95% 雷同**。

#### B 类：模式相关（必须分写）

执行语义本质不同：
- **ToolMiddleware**：
  - Call：等 `next.Call` 返回完整 response，检查 tool calls，执行，递归
  - Stream：边收 chunk 边 yield 给上游，同时累积成完整 response 判断 tool calls，最后**重启流**递归
- **MemoryMiddleware**：对 stream 要等完整累积才能写入 memory

这类**无法合并**。强行抽象反而引入错误的中间层。

### 2.3 当前 `NewToolMiddleware` 的双值返回

```go
func NewToolMiddleware() (CallMiddleware, StreamMiddleware) {
    mw := &ToolMiddleware{}
    return mw.wrapCallHandler, mw.wrapStreamHandler
}

// 用户使用
callMW, streamMW := chat.NewToolMiddleware()
client.WithMiddlewares(callMW, streamMW)
```

每次都要解构双值——即使该用户根本不用 streaming。

### 2.4 `UseMiddlewares(...any)` 反 Go 风格

```go
func (m *MiddlewareManager[...]).UseMiddlewares(middlewares ...any) {
    for _, mw := range middlewares {
        if cm, ok := mw.(CallMiddleware[...]); ok { /* append */ }
        if sm, ok := mw.(StreamMiddleware[...]); ok { /* append */ }
    }
}
```

- 编译期不报错——传错类型（如 `func()`）静默丢弃
- 与 `UseCallMiddlewares` / `UseStreamMiddlewares` 类型安全版本完全重叠

---

## 3. 设计方向

### 3.1 短期：纯瘦身（无 API 破坏）

**4 → 2 泛型参数**：

```go
type MiddlewareManager[Req any, Res any] struct {
    mu                sync.RWMutex
    callMiddlewares   []CallMiddleware[Req, Res]
    streamMiddlewares []StreamMiddleware[Req, Res]
}
```

`CallMiddlewareManager` / `StreamMiddlewareManager` 两个 wrapper 删除——用户用 `MiddlewareManager` 直接，只想注册一边就调 `UseCallMiddlewares` 或 `UseStreamMiddlewares`。

**收益**：删 ~80 行重复代码、类型签名简洁、为后续合并铺路。
**风险**：影响所有 6 个模态的 client.go alias，但 alias 类型对外 API 透明，调用方代码无需改。
**实施成本**：约 1 小时。

### 3.2 中期：删 `UseMiddlewares(...any)`

- §3.1 落地后，`UseMiddlewares(...any)` 与类型安全版完全重叠
- 弃用，告诉用户用 `UseCallMiddlewares` / `UseStreamMiddlewares`，下个版本删除

### 3.3 长期：`AroundMiddleware`（A 类便利层）

参考 Spring AI `BaseAdvisor`，**保留底层** `CallMiddleware` / `StreamMiddleware`（B 类继续用），增加高层 struct 让 A 类只写一份：

```go
// core/model/around.go (proposed, ~80 行)
type AroundMiddleware[Req, Res any] struct {
    Name  string
    Order int

    // 都可 nil；nil 即 no-op
    Before      func(ctx context.Context, req Req) (context.Context, Req, error)
    AfterCall   func(ctx context.Context, req Req, res Res, err error) (Res, error)
    AfterStream func(ctx context.Context, req Req, in iter.Seq2[Res, error]) iter.Seq2[Res, error]
}

// 展开到 Call/Stream 两边
func (a *AroundMiddleware[Req, Res]) Call() CallMiddleware[Req, Res]   { ... }
func (a *AroundMiddleware[Req, Res]) Stream() StreamMiddleware[Req, Res] { ... }
```

`MiddlewareManager.UseMiddlewares` 识别 `*AroundMiddleware` 自动展开成 Call + Stream。

**好处**：
- B 类零改动（ToolMiddleware / MemoryMiddleware 不动）
- A 类大瘦身（logging / auth / retry 从 2×wrapper 降到 1 个 struct）
- 向后兼容（AroundMiddleware 是新增选项，老代码不受影响）
- `Order` / `Name` 字段为 ordering / observability 预留空间

### 3.4 v1.0 ABI 重塑：单一 Middleware 接口

破坏性 API 改动，留到 v1.0 一起做：

```go
type Middleware[Req any, Res any] interface {
    WrapCall(next CallHandler[Req, Res]) CallHandler[Req, Res]
    WrapStream(next StreamHandler[Req, Res]) StreamHandler[Req, Res]
}

// 默认透传 helper（嵌入用）
type PassthroughCall[Req, Res any]   struct{}
type PassthroughStream[Req, Res any] struct{}

func (PassthroughCall[Req, Res]) WrapCall(next CallHandler[Req, Res]) CallHandler[Req, Res]       { return next }
func (PassthroughStream[Req, Res]) WrapStream(next StreamHandler[Req, Res]) StreamHandler[Req, Res] { return next }
```

模式无关中间件嵌入 `Passthrough*`，只实现关心的一边：

```go
type LoggingMiddleware struct {
    PassthroughStream[*Request, *Response]
}
func (m *LoggingMiddleware) WrapCall(next CallHandler) CallHandler { ... }

// 模式相关中间件实现两个方法
type ToolMiddleware struct{}
func (m *ToolMiddleware) WrapCall(next CallHandler)     CallHandler   { ... }
func (m *ToolMiddleware) WrapStream(next StreamHandler) StreamHandler { ... }
```

`NewToolMiddleware()` 改返单值：

```go
client.WithMiddleware(chat.NewToolMiddleware())
client.WithMiddlewares(toolMW, memoryMW)  // 不再返双值
```

**收益**：用户心智模型从「两个对偶 middleware」变成「一个对象」；`WithMiddlewares(...any)` 不再需要 type assertion；模式无关中间件少写一半代码。

**风险**：所有现有 middleware 实现都要改。Lynx 当前只有 2 个内置（Tool / Memory），影响可控；外部用户中间件需要迁移。

**实施成本**：约 4-6 小时（含 chat.Client / 6 个模态 alias / 2 个内置 middleware / 测试）。

---

## 4. 统一 Middleware Manager 框架（架构级）

更彻底的方向——把 chat / embedding / image / audio / moderation / RAG 全部基于同一套 `Manager[CallHandler[Req,Res]]` 实例化：

```go
// core/middleware/middleware.go (proposed)
type Middleware[H any] func(next H) H

type Manager[H any] struct { middlewares []Middleware[H] }
func (m *Manager[H]) Build(endpoint H) H { /* reverse wrap */ }
```

这样 RAG、Tool、Memory、Cache、RateLimit 都能跨模态组合。

**当前症结**：Lynx 有「装饰器」的实现，没有「Middleware」的接口。三类中间件（ChatMiddleware / RAGPipelineMiddleware / ToolMiddleware）是三条独立进化线。Embedding/Image/Audio/Moderation 完全没有中间件故事——无法加 logging / caching / rate-limit，除非用户从 Handler 层手写装饰器。

**何时做**：与 §3.4 同节奏（v1.0 ABI 重塑）。

---

## 5. 对照 Spring AI 与 Embabel

### 5.1 Spring AI 2.0：`BaseAdvisor` 已成一等接口

`spring-ai-client-chat/.../advisor/api/BaseAdvisor.java`：把原本只是「默认方法集合」的 BaseAdvisor 抬升为具名接口，同时实现 `CallAdvisor` 和 `StreamAdvisor`、提供 `before()/after()` 模板方法，已被 `RetrievalAugmentationAdvisor` 采用。这进一步验证了 §3.3 的方向。

| 维度 | Spring AI 2.0 | Lynx（当前 / 提案） |
|-----|--------------|----------------|
| 底层接口 | `CallAdvisor` / `StreamAdvisor` | `CallMiddleware` / `StreamMiddleware`（保留）|
| 高层抽象 | `BaseAdvisor` 默认 `adviseCall/adviseStream` 委托给 `before/after` | `AroundMiddleware`（提案，用 struct + 可选函数字段）|
| Ordering | `Ordered.getOrder()` | `AroundMiddleware.Order`（待 `MiddlewareManager` 消费）|
| Name | `Advisor.getName()` | `AroundMiddleware.Name` |
| 共享 context | `ChatClientRequest.context` + `ChatClientResponse.context` | `Request.Params`（单向）|

Lynx 用 struct + 可选函数字段代替 Java 的抽象类 + default methods，语义等价。

### 5.2 Embabel：`ToolCallInspector` SPI（最近落地）

Embabel 0.4.0-SNAPSHOT 引入 `ToolCallInspector`：拦截/观察 tool 执行流，配合 `StreamingLlmOperationsFactory` 与 `LlmMessageStreamer` 流式重构。

**对 Lynx 的启示**：
- Lynx 的 `iter.Seq2` 流式天然就是这一层——`StreamMiddleware` + `ToolMiddleware.WrapStream` 已经能做拦截
- 没有强烈需要把「Inspector」抽成独立 SPI；现有 middleware 接口已能胜任
- 但对照参考是有价值的：当 Lynx agent 框架（`agent/`）落地时，`ToolCallInspector` 的语义可以映射到 agent runtime 的事件总线 + middleware 链组合

---

## 6. 不做的事 / 已拒绝的方案

| 方案 | 拒绝理由 |
|-----|---------|
| 删除 `CallHandler`，只留 `StreamHandler`，`Call = Stream + 聚合` | OpenAI/Anthropic 原生 Call 和 Stream 是两套 API，统一后反而绕路；ResponseAccumulator 语义并非所有模态通用（音频拼 bytes、embedding 无 stream）|
| Observer 模式独立接口 | 不能修改 request/response，只适合日志/指标；A 类除 logging 外的需求都兜不住——`AroundMiddleware` 能覆盖 Observer 能力 |
| 把 ToolMiddleware 自动注入回去 | 已修复为 opt-in（commit `8e58479`），保留显式注册 |

---

## 7. 一句话定档

> Call/Stream 在 Go 类型系统下不可强行合并，但 90% 中间件根本不需要真合并，只需要「少写一份」。短期清掉 4 参数泛型与 `UseMiddlewares(...any)` 的反 Go API；中期加 `AroundMiddleware` 让 A 类（logging/metrics/auth）从 2×wrapper 降到 1 个 struct；底层 `CallMiddleware`/`StreamMiddleware` 保留作为 B 类（Tool/Memory）的 escape hatch。这是承认类型不对称、然后用便利层消解痛点的工程解法，不是架构革命。
