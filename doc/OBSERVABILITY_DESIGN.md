# Lynx 可观测性设计

> 对标 Spring AI 的 Micrometer Observation 设计，为 Lynx 设计一套**不锁定具体厂商**的可观测性体系。
> 核心原则：**依赖抽象而非具体**——核心代码只认 `observation.Registry` 接口，OTel / Prometheus / slog 都是可插拔适配器。

---

## 1. 设计目标 & 原则

### 1.1 目标
1. 为 Chat / Embedding / Image / Audio / RAG / VectorStore / Tool 提供统一的观测点
2. 输出**分布式追踪**（trace / span）、**指标**（counter / histogram）、**结构化事件**（event / log）
3. 遵循 OpenTelemetry GenAI 语义规范（`gen_ai.*`），与业界打通
4. 默认零开销——未配置后端时 **no-op**，不引入任何外部依赖

### 1.2 原则

| # | 原则 | 含义 |
|---|-----|------|
| P1 | **依赖抽象** | 核心代码只 import `core/observation`，不 import OTel/Prom |
| P2 | **core 只允许 stdlib 实现** | `core/` 可以有 Noop + slog（两者都只用标准库）；**任何引入第三方 SDK 的适配器**放独立顶层 module |
| P3 | **零配置默认** | 不注入 Registry 时 = noop，不改变行为、不增开销 |
| P4 | **显式注入** | Registry 作为 Client/Pipeline/VectorStore 的字段，不走全局变量 |
| P5 | **Context 传播** | Span 通过 `context.Context` 向下传递（Go 惯用法） |
| P6 | **语义对齐** | 属性键严格遵循 OTel GenAI 规范，跨平台可比 |
| P7 | **观测是读路径** | Observation 不应改变业务行为——只读采集 |
| P8 | **依赖隔离** | `core` 的 `go.mod` **不**新增 OTel/Prom 这类重依赖；下游用户只装自己需要的适配器 |

### 1.3 非目标
- ❌ **不**做 APM 平台——采集抽象而非后端
- ❌ **不**内置 OTel SDK——作为独立子包，按需引入
- ❌ **不**做日志框架——log 走 `slog`，本库只做 observation 事件发射
- ❌ **不**做性能分析（pprof、CPU profile）——那是 Go 运行时层面的事

---

## 2. Spring AI 可观测性快速回顾

Spring AI 的观测架构分三层：

```
┌──────────────────────────────────────────────────────────┐
│  Application                                              │
│   └─ Prometheus / Zipkin / Jaeger / OTel Collector 等    │
├──────────────────────────────────────────────────────────┤
│  Adapters                                                 │
│   └─ micrometer-tracing-bridge-otel                      │
│   └─ micrometer-registry-prometheus                      │
├──────────────────────────────────────────────────────────┤
│  Abstractions  ←  Spring AI 只依赖这一层                  │
│   └─ io.micrometer.observation.ObservationRegistry       │
│   └─ Observation / ObservationConvention / Context       │
├──────────────────────────────────────────────────────────┤
│  Spring AI 代码（仅依赖上面抽象层）                         │
│   ├─ ChatModelObservationConvention                      │
│   ├─ EmbeddingModelObservationConvention                 │
│   ├─ VectorStoreObservationConvention                    │
│   ├─ AdvisorObservationConvention                        │
│   └─ ToolCallingObservationConvention                    │
└──────────────────────────────────────────────────────────┘
```

**关键洞察**：Spring AI 不 import Prometheus / OTel 的具体类。它依赖的是 Micrometer 的 Observation API（一个抽象），Micrometer 负责桥接具体后端。Lynx 要做的就是在 Go 侧建立**同样的分层**。

### Spring AI 具体观测点

| 层 | Convention | 名称 | 关键属性 |
|---|-----------|------|---------|
| Chat Model | `ChatModelObservationConvention` | `gen_ai.client.operation` | `gen_ai.system`, `gen_ai.request.model`, `gen_ai.usage.*` |
| Embedding | `EmbeddingModelObservationConvention` | `gen_ai.client.operation` | 同上 + `gen_ai.request.embedding.dimensions` |
| VectorStore | `VectorStoreObservationConvention` | `db.vector.client.operation` | `db.system`, `db.operation.name`, `db.vector.query.*` |
| Advisor | `AdvisorObservationConvention` | `spring.ai.advisor` | `spring.ai.advisor.type`, `name` |
| Tool | `ToolCallingObservationConvention` | `spring.ai.tool` | `spring.ai.tool.call.arguments` |

---

## 3. Lynx 可观测性架构

### 3.1 层次图 & 模块拓扑

```
┌────────────────────────────────────────────────────────────┐
│  用户代码                                                    │
│   reg := otelobs.New(tracer, meter)                        │
│   client := chat.NewClient(m, chat.WithObservation(reg))   │
├────────────────────────────────────────────────────────────┤
│  外部 Module（独立 go.mod，按需引入——任何第三方 SDK）         │
│   observations/        ← 新增顶层 module（仿 models/ 规格）   │
│   ├─ otel/             → OpenTelemetry SDK                  │
│   ├─ prom/             → Prometheus client_golang           │
│   ├─ datadog/          → Datadog SDK（如将来有）             │
│   └─ ...               → 其他 SaaS / 第三方后端              │
├────────────────────────────────────────────────────────────┤
│  核心 Module core/observation/（仅 stdlib）                   │
│   ├─ observation.go    Registry / Observation / Attr        │
│   ├─ convention.go     Convention / ObservationContext      │
│   ├─ context.go        WithRegistry / FromContext           │
│   ├─ noop.go           零开销默认（无依赖）                  │
│   └─ slog.go           结构化日志实现（依赖 log/slog 标准库） │
├────────────────────────────────────────────────────────────┤
│  Instrumentation（埋点，分散在 core/ 各处）                   │
│   ├─ chat.Model / Client                                   │
│   ├─ embedding.Model                                       │
│   ├─ vectorstore.{Creator, Retriever, Deleter}             │
│   ├─ rag.Pipeline（五段式各自埋点）                          │
│   └─ ToolMiddleware                                        │
└────────────────────────────────────────────────────────────┘
```

**关键决策**：核心内的实现有**严格的准入规则**：

> **一个实现可以进 core，当且仅当它的 import 列表只包含 Go 标准库。**

Noop 什么都不 import，slog 只 import `log/slog`，两者都满足。OTel / Prom / Datadog 等带来第三方依赖的实现，**必须**放独立顶层 module `observations/`——与 `models/`、`vectorstores/` 同架构规格：

| Module | 定位 | 外部依赖 |
|--------|-----|---------|
| `core/` | 抽象 + Noop + slog | **仅标准库 + 少量轻量 pkg** |
| `models/` | Provider 适配 | OpenAI SDK / Anthropic SDK / Google genai |
| `vectorstores/` | VectorStore 适配 | Qdrant / Milvus / Pinecone / ... |
| `observations/` | 观测后端适配（SaaS/OSS APM） | OTel SDK / Prom client / Datadog / ... |
| `tools/` | 工具适配 | — |
| `pkg/` | 通用工具 | — |

这样 `core/go.mod` 不会多出 OTel 的 50+ 间接依赖——**开发态用 slog 看日志 / 生产态用 OTel 打 Jaeger**，两档切换零成本。

### 3.2 核心抽象

```go
// core/observation/observation.go
package observation

import "context"

// Registry 是观测系统的入口。核心代码只依赖此接口。
type Registry interface {
    // Start 开启一次观测，返回携带 span 的 ctx 和 Observation 句柄。
    // 实现必须保证：即使 ctx、attrs 为 nil 也不 panic。
    Start(ctx context.Context, name string, attrs ...Attr) (context.Context, Observation)
}

// Observation 代表一次观测的生命周期（一个 span + 可选的 metrics 事件）。
// 必须 End()，否则 span 不会上报。
type Observation interface {
    // SetAttr 追加属性（适合结果信息，如 token 数、finish_reason）。
    SetAttr(key string, value any)

    // AddEvent 附加结构化事件（如 "first_token_received"）。
    AddEvent(name string, attrs ...Attr)

    // SetError 标记观测为错误状态；后续 End 时会带错误信息。
    SetError(err error)

    // End 结束观测。重复调用无副作用。
    End()
}

// Attr 是最小化的键值对，避免直接依赖 otel.attribute.KeyValue。
type Attr struct {
    Key   string
    Value any
}

// 便捷构造函数
func String(k, v string) Attr       { return Attr{k, v} }
func Int64(k string, v int64) Attr  { return Attr{k, v} }
func Float64(k string, v float64) Attr { return Attr{k, v} }
func Bool(k string, v bool) Attr    { return Attr{k, v} }
func Strings(k string, v []string) Attr { return Attr{k, v} }
```

```go
// core/observation/convention.go
// Convention 定义一个观测点的命名和属性约定。
// 不同适配器可覆写 Convention 来改变落地效果。
type Convention interface {
    // Name 返回观测点名称（如 "gen_ai.chat"）。
    Name() string

    // LowCardinalityAttrs 返回适合用作指标 label 的属性（基数有限）。
    LowCardinalityAttrs(ctx ObservationContext) []Attr

    // HighCardinalityAttrs 返回适合附加到 span 但不宜作 metric label 的属性
    //（基数高，例如 prompt 内容、响应内容）。
    HighCardinalityAttrs(ctx ObservationContext) []Attr
}

// ObservationContext 是埋点处传入 Convention 的数据载体。
// 每个观测域（Chat/Embedding/VectorStore/...）有自己的结构体实现此接口。
type ObservationContext interface {
    OperationName() string  // "chat" | "embedding" | "retrieve" | ...
    System() string         // "openai" | "anthropic" | "qdrant" | ...
}
```

```go
// core/observation/context.go
// 通过 context.Context 传递 Registry，便于跨层调用。
// 如果 ctx 中无 Registry，FromContext 返回 noop 实例。

type ctxKey struct{}

func WithRegistry(ctx context.Context, r Registry) context.Context {
    return context.WithValue(ctx, ctxKey{}, r)
}

func FromContext(ctx context.Context) Registry {
    if r, ok := ctx.Value(ctxKey{}).(Registry); ok && r != nil {
        return r
    }
    return noopRegistry{}
}
```

---

## 4. 语义规范（Attribute Naming）

### 4.1 严格对齐 OpenTelemetry GenAI

所有模型层观测必须使用标准属性：

| Key | 类型 | 说明 |
|-----|-----|------|
| `gen_ai.system` | string | `openai` / `anthropic` / `google` / ... |
| `gen_ai.operation.name` | string | `chat` / `embeddings` / `image` / ... |
| `gen_ai.request.model` | string | 请求使用的模型 ID |
| `gen_ai.response.model` | string | 实际返回的模型 ID |
| `gen_ai.request.max_tokens` | int | |
| `gen_ai.request.temperature` | float | |
| `gen_ai.request.top_p` | float | |
| `gen_ai.response.finish_reasons` | []string | |
| `gen_ai.response.id` | string | |
| `gen_ai.usage.input_tokens` | int | |
| `gen_ai.usage.output_tokens` | int | |
| `gen_ai.usage.total_tokens` | int | |

### 4.2 VectorStore（参考 OTel DB 规范）

| Key | 类型 | 说明 |
|-----|-----|------|
| `db.system` | string | `qdrant` / `milvus` / `chroma` / ... |
| `db.operation.name` | string | `create` / `retrieve` / `delete` |
| `db.vector.query.top_k` | int | |
| `db.vector.query.similarity_threshold` | float | |

### 4.3 Lynx 专有扩展（`lynx.*` 前缀）

OTel 未覆盖的场景使用 `lynx.*` 前缀，避免污染标准命名空间：

| Key | 类型 | 说明 |
|-----|-----|------|
| `lynx.rag.stage` | string | `transform` / `expand` / `retrieve` / `refine` / `augment` |
| `lynx.rag.query_count` | int | 扩展后的 query 数 |
| `lynx.rag.doc_count` | int | 当前阶段的文档数 |
| `lynx.tool.name` | string | 工具名 |
| `lynx.tool.recursion_depth` | int | 工具递归深度 |
| `lynx.middleware.name` | string | 中间件名 |

---

## 5. 埋点清单（Instrumentation Points）

### 5.1 Chat / Embedding / Image / Audio Model

**埋点位置**：Model 调用的 Client 层（而非 Provider 内部）。

```go
// core/model/chat/client.go (伪代码，展示集成方式)
func (c *ClientCaller) call(ctx context.Context, req *Request) (*Response, error) {
    reg := observation.FromContext(ctx)  // 或从 client 字段取
    ctx, obs := reg.Start(ctx, "gen_ai.chat",
        observation.String("gen_ai.system", c.request.model.Info().Provider),
        observation.String("gen_ai.operation.name", "chat"),
        observation.String("gen_ai.request.model", req.Options.Model),
        observation.Int64("gen_ai.request.max_tokens", int64(deref(req.Options.MaxTokens))),
    )
    defer obs.End()

    res, err := c.request.MiddlewareManager().BuildCallHandler(c.request.model).Call(ctx, req)
    if err != nil {
        obs.SetError(err)
        return nil, err
    }

    if m := res.Metadata; m != nil && m.Usage != nil {
        obs.SetAttr("gen_ai.usage.input_tokens", m.Usage.InputTokens)
        obs.SetAttr("gen_ai.usage.output_tokens", m.Usage.OutputTokens)
    }
    if len(res.Results) > 0 && res.Results[0].Metadata != nil {
        obs.SetAttr("gen_ai.response.finish_reasons",
            []string{res.Results[0].Metadata.FinishReason})
    }
    return res, nil
}
```

### 5.2 Stream 模式

Stream 模式需要额外事件：

```go
ctx, obs := reg.Start(ctx, "gen_ai.chat.stream", ...)
defer obs.End()

firstChunk := true
for chunk, err := range streamHandler.Stream(ctx, req) {
    if firstChunk {
        obs.AddEvent("first_token_received")
        firstChunk = false
    }
    // yield to caller
}
obs.SetAttr("gen_ai.usage.output_tokens", accumulator.TotalTokens())
```

### 5.3 RAG Pipeline（五阶段各自埋点）

每个阶段都应该是一个子 span，作为 pipeline 整体 span 的子节点：

```go
func (p *Pipeline) Execute(ctx context.Context, q *Query) (*Query, error) {
    reg := observation.FromContext(ctx)
    ctx, obs := reg.Start(ctx, "lynx.rag.pipeline",
        observation.String("lynx.rag.stage", "pipeline"),
    )
    defer obs.End()

    ctx1, obs1 := reg.Start(ctx, "lynx.rag.transform",
        observation.String("lynx.rag.stage", "transform"))
    transformed, err := p.transformQuery(ctx1, q)
    obs1.End()
    if err != nil { obs.SetError(err); return nil, err }

    // ... expand / retrieve / refine / augment 同样模式
}
```

### 5.4 VectorStore

```go
// vectorstores/qdrant/store.go (伪代码)
func (s *Store) Retrieve(ctx context.Context, req *RetrievalRequest) ([]*Document, error) {
    reg := observation.FromContext(ctx)
    ctx, obs := reg.Start(ctx, "db.vector.retrieve",
        observation.String("db.system", "qdrant"),
        observation.String("db.operation.name", "retrieve"),
        observation.Int64("db.vector.query.top_k", int64(req.TopK)),
    )
    defer obs.End()

    docs, err := s.retrieve(ctx, req)
    if err != nil { obs.SetError(err); return nil, err }

    obs.SetAttr("lynx.rag.doc_count", int64(len(docs)))
    return docs, nil
}
```

### 5.5 Tool Middleware

工具调用递归时每次都是新 span：

```go
// 在 ToolMiddleware.executeCallRecursively 里
ctx, obs := reg.Start(ctx, "lynx.tool.invoke",
    observation.String("lynx.tool.name", toolCall.Name),
    observation.Int64("lynx.tool.recursion_depth", int64(depth)),
)
result, err := tool.Call(ctx, toolCall.Arguments)
if err != nil { obs.SetError(err) }
obs.End()
```

---

## 6. 适配器（Adapters）

**核心原则**：只用 stdlib 的实现（Noop、slog）留在 `core/observation`；**任何引入第三方 SDK 的适配器**放独立顶层 module `observations/`。

### 6.1 Noop（留在 core，默认启用）

```go
// core/observation/noop.go
package observation

import "context"

type noopRegistry struct{}
func (noopRegistry) Start(ctx context.Context, _ string, _ ...Attr) (context.Context, Observation) {
    return ctx, noopObservation{}
}

type noopObservation struct{}
func (noopObservation) SetAttr(string, any)       {}
func (noopObservation) AddEvent(string, ...Attr)  {}
func (noopObservation) SetError(error)            {}
func (noopObservation) End()                      {}
```

**零开销**：无堆分配、无锁、无系统调用。**无任何外部依赖**。

> 为什么 Noop 不另起一个 module？因为它是零依赖的默认实现，放 core 才能让 `FromContext` 在未注入时也能直接返回 noop，不引入循环依赖。

### 6.2 slog（留在 core，开发/轻量部署使用）

```go
// core/observation/slog.go
package observation

import (
    "context"
    "log/slog"
    "time"
)

type slogRegistry struct{ logger *slog.Logger }

// NewSlogRegistry 用标准库 log/slog 做观测后端，适合开发/轻量部署。
// 不引入任何第三方依赖。
func NewSlogRegistry(logger *slog.Logger) Registry {
    if logger == nil {
        logger = slog.Default()
    }
    return &slogRegistry{logger: logger}
}

func (r *slogRegistry) Start(ctx context.Context, name string, attrs ...Attr) (context.Context, Observation) {
    obs := &slogObs{
        logger: r.logger,
        name:   name,
        attrs:  attrsToSlogAny(attrs),
        start:  time.Now(),
    }
    r.logger.LogAttrs(ctx, slog.LevelDebug, "observation.start",
        slog.String("name", name),
        slog.Any("attrs", obs.attrs),
    )
    return ctx, obs
}

type slogObs struct {
    logger  *slog.Logger
    name    string
    attrs   map[string]any
    start   time.Time
    err     error
}

func (o *slogObs) SetAttr(k string, v any)             { o.attrs[k] = v }
func (o *slogObs) AddEvent(n string, a ...Attr)        { /* 立刻落 info 日志 */ }
func (o *slogObs) SetError(err error)                  { o.err = err }
func (o *slogObs) End() {
    level := slog.LevelInfo
    if o.err != nil { level = slog.LevelError }
    o.logger.LogAttrs(context.Background(), level, "observation.end",
        slog.String("name", o.name),
        slog.Duration("duration", time.Since(o.start)),
        slog.Any("attrs", o.attrs),
        slog.Any("error", o.err),
    )
}
```

**为什么留 core**：`log/slog` 是 Go 1.21+ 标准库，零第三方依赖。下游用户开发态想「看点日志」不用 `go get` 任何东西。

### 6.3 OTel 适配器（独立 module）

```
observations/
├── go.mod          → module github.com/Tangerg/lynx/observations
└── otel/
    └── otel.go
```

```go
// observations/otel/otel.go
package otel

import (
    "context"
    "go.opentelemetry.io/otel/trace"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/codes"
    "github.com/Tangerg/lynx/core/observation"
)

type Registry struct {
    tracer trace.Tracer
}

func New(tracer trace.Tracer) *Registry { return &Registry{tracer: tracer} }

func (r *Registry) Start(ctx context.Context, name string, attrs ...observation.Attr) (context.Context, observation.Observation) {
    ctx, span := r.tracer.Start(ctx, name, trace.WithAttributes(toOTelAttrs(attrs)...))
    return ctx, &obs{span: span}
}

type obs struct{ span trace.Span }
func (o *obs) SetAttr(k string, v any)                      { o.span.SetAttributes(toOTelAttr(k, v)) }
func (o *obs) AddEvent(n string, a ...observation.Attr)     { o.span.AddEvent(n, trace.WithAttributes(toOTelAttrs(a)...)) }
func (o *obs) SetError(err error)                           { o.span.RecordError(err); o.span.SetStatus(codes.Error, err.Error()) }
func (o *obs) End()                                         { o.span.End() }

func toOTelAttr(k string, v any) attribute.KeyValue { /* 按 Go 类型分发 */ }
```

**`observations/go.mod`** 只依赖 `core` 和 OTel：

```
module github.com/Tangerg/lynx/observations

require (
    github.com/Tangerg/lynx/core vX.Y.Z
    go.opentelemetry.io/otel v1.x
    go.opentelemetry.io/otel/trace v1.x
)
```

### 6.4 Prometheus 适配器（独立 module）

```
observations/
├── otel/
└── prom/
    └── prom.go    // 依赖 prometheus/client_golang
```

为每个观测点生成：调用次数 counter、时延 histogram、错误 counter。用 `Convention.LowCardinalityAttrs` 作为 labels。

### 6.5 依赖隔离对比

| 场景 | 装什么 | `go.sum` 膨胀 |
|-----|-------|-------------|
| 不需要观测 | 仅 `core/`（自动 noop） | **0 新依赖** |
| 开发态看日志 | 仅 `core/`（NewSlogRegistry） | **0 新依赖**（用标准库 log/slog） |
| 生产 OTel | `core/` + `observations/otel` | OTel 全家桶（50+ 间接） |
| Prom 指标 | `core/` + `observations/prom` | prom client_golang |
| Datadog / NewRelic / Jaeger 直连 | `core/` + `observations/<vendor>` | 对应厂商 SDK |

关键优势：**从 noop 升级到 slog 零依赖成本**——开发调试不用装东西、不用改 go.mod。只有真的上 OTel/Prom 这类 SaaS 级后端时，才会动 `go.sum`。

---

## 7. 集成方式

### 7.1 Client 注入（推荐）

每个 Client / Pipeline / VectorStore 接受一个可选的 Registry：

```go
// core/model/chat/client.go
type Client struct {
    defaultRequest *ClientRequest
    registry       observation.Registry  // 可选
}

func WithObservation(reg observation.Registry) ClientOption {
    return func(c *Client) { c.registry = reg }
}
```

在方法入口把 registry 放进 ctx，埋点处 `FromContext(ctx)` 取用。

### 7.2 Context 注入（灵活）

如果用户不想在每个 Client 上配置，可以把 Registry 放到 context 根节点：

```go
ctx = observation.WithRegistry(ctx, otelobs.New(tracer, meter))
// 所有 lynx 调用都会自动用上
client.Chat().Call().Response(ctx)
```

### 7.3 中间件注入（可选）

对 A 类（纯观察型）中间件，用户可以用自己的方式包一个 `LoggingMiddleware`，与 observation 并行使用——这是用户关注点，不是框架要解决的。

---

## 8. 用户使用示例

### 8.1 生产（OTel + Jaeger）

```go
import (
    "go.opentelemetry.io/otel"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    lynxotel "github.com/Tangerg/lynx/observations/otel"   // 独立 module
)

tp := sdktrace.NewTracerProvider(/* export to Jaeger/Tempo */)
otel.SetTracerProvider(tp)

reg := lynxotel.New(otel.Tracer("lynx"))

client, _ := chat.NewClientWithModel(openai.NewChatModel(...))
client = client.With(chat.WithObservation(reg))

resp, err := client.ChatWithText("hello").Call().Response(ctx)
// 自动产生 gen_ai.chat span，包含 usage、model、finish_reason
```

### 8.2 开发（slog 控制台，零新依赖）

```go
import (
    "log/slog"
    "github.com/Tangerg/lynx/core/observation"   // 就在 core 里
)

reg := observation.NewSlogRegistry(slog.Default())
client = client.With(chat.WithObservation(reg))
// 不用 go get 任何东西，直接看结构化日志
```

### 8.3 不配置 = 零开销

```go
// 什么都不做 → 自动 noop → 无分配、无锁、core 无外部依赖
client, _ := chat.NewClientWithModel(...)
```

---

## 9. 与 Spring AI 的对照

| 维度 | Spring AI | Lynx |
|-----|-----------|------|
| 抽象层 | Micrometer `ObservationRegistry` | `observation.Registry` |
| 观测对象 | `Observation` | `Observation` |
| 规约 | `ObservationConvention` | `Convention` |
| 传递机制 | ThreadLocal | `context.Context`（Go 惯用） |
| 默认实现 | `ObservationRegistry.NOOP` | `noop.Registry` |
| OTel 桥接 | `micrometer-tracing-bridge-otel` | `core/observation/otel` |
| Prom 桥接 | `micrometer-registry-prometheus` | `core/observation/prom` |
| 语义规范 | OTel GenAI | OTel GenAI（完全对齐） |

**设计一致**：都选「第三方抽象 → 多后端适配器」的双层结构。Lynx 用 Go 特色（context、子包）表达同一模式。

---

## 10. 工程拆解（落地阶段）

### 阶段 1：核心抽象 + Noop + slog（1 周）—— **在 `core/` 内**
- [ ] `core/observation/observation.go` — `Registry` / `Observation` / `Attr`
- [ ] `core/observation/convention.go` — `Convention` / `ObservationContext`
- [ ] `core/observation/context.go` — `WithRegistry` / `FromContext`
- [ ] `core/observation/noop.go` — zero-cost 默认实现
- [ ] `core/observation/slog.go` — 基于 log/slog 的实现（stdlib only）
- [ ] 单测覆盖 nil ctx / nil attrs 等边界
- [ ] **验收**：`core/go.mod` **无任何第三方依赖新增**（只能出现 stdlib import）

### 阶段 2：Chat 埋点（1 周）—— 仍在 `core/`
- [ ] `core/model/chat/client.go` — Call/Stream 两路埋点
- [ ] `chat.WithObservation(reg)` option
- [ ] 对齐所有 `gen_ai.*` 属性

### 阶段 3：RAG + VectorStore 埋点（1-2 周）—— 仍在 `core/` + `vectorstores/`
- [ ] `core/rag/pipeline.go` — 五阶段各自 span
- [ ] `core/vectorstore/` 埋点接口
- [ ] `vectorstores/*/store.go` 五个 store 实现中加入埋点（仅调用 `observation.FromContext`，不引入后端依赖）

### 阶段 4：第三方适配器（并行 2-3 周）—— **在新 module `observations/`**
- [ ] 新建顶层 module `observations/go.mod`
- [ ] `go.work` 里 `use ./observations`
- [ ] `observations/otel/` — OpenTelemetry 桥接
- [ ] （可选）`observations/prom/` — Prometheus 指标
- [ ] **验收**：在不 `go get observations/otel` 的项目中，`core/` 相关功能照常工作（noop + slog 仍可用）

> 注意：**slog 不放这里**——它只依赖标准库 `log/slog`，属于 core 的一等公民。

### 阶段 5：其他模态（视需求）
- [ ] Embedding / Image / Audio / Moderation 埋点
- [ ] ToolMiddleware 埋点

---

## 11. 取舍说明

### 11.1 为什么不直接用 OTel？

OTel Go SDK 是事实标准，但：
1. 让 `core/` 直接 `import go.opentelemetry.io/otel`，会让 **Lynx 的所有下游用户** 被动拉入 OTel 依赖（~50 个 go.mod 间接依赖）
2. OTel API 偶尔破坏性变更（trace v1 → v1.x 几次迁移），核心代码绑到具体 API 会把升级成本传导给所有用户
3. 小型项目（只想 slog）不应被迫装 OTel

**用薄抽象层隔离 = 依赖倒置**，这是 Spring AI 选 Micrometer 而非 Prometheus 的同样逻辑。

### 11.1.1 为什么第三方适配器必须另起 module

即使是子包，Go 的 module 机制也是**以 go.mod 为单位管理依赖的**——只要 `core/observation/otel/otel.go` 出现 `import go.opentelemetry.io/otel`，`core/go.mod` 就会写入该依赖，**所有引用 `core` 的模块都会间接拉入**。子包隔离不了依赖，只有独立 module 能。

### 11.1.2 为什么 slog 可以留 core、OTel 不行

判定标准很简单：**看 import 列表里有没有 `go.opentelemetry.io`、`github.com/prometheus`、`github.com/DataDog` 等第三方路径**。

- `log/slog` → 标准库路径，**留 core**
- `go.opentelemetry.io/otel` → 第三方，**必须 external module**
- `github.com/prometheus/client_golang` → 第三方，**必须 external module**

这个规则简单、可自动化检查（CI 加一条「core 模块禁止 import 非标准库路径（除 Tangerg/lynx/pkg 外）」），未来添加新实现时不会有歧义。

这与 Lynx 现有架构一致：
- `models/` 承载 Provider SDK（OpenAI、Anthropic、Google），不污染 core
- `vectorstores/` 承载向量数据库 SDK，不污染 core
- `observations/` 承载**第三方**观测后端 SDK，同样不污染 core；**stdlib 能解决的实现（Noop、slog）留 core**

**所有重依赖都在外部 module，`core/` 永远保持最小依赖面**——这是整个 Lynx 的第一架构原则。

### 11.2 为什么不走全局变量？

OTel 的 `otel.Tracer("name")` 隐式读全局 TracerProvider。方便，但：
- 测试隔离困难（多测试共享全局状态）
- 多租户场景无法区分 registry
- 违反「显式优于隐式」的 Go 风格

**显式注入 = Registry 在 Client 字段或 context**，调用路径清晰。

### 11.3 为什么 Metric 不做单独抽象？

Spring AI 的 Micrometer Observation 同时覆盖 trace 和 metric——一个 Observation 的生命周期自动产生一个 span + 一组计数/直方图。

Lynx 初版先做 trace，Metric 在 Convention.LowCardinalityAttrs 上靠适配器生成。后续若发现不够用，再引入独立的 `Meter` 抽象（参考 OTel 的 `metric.Meter`）。**YAGNI 优先**。

### 11.4 埋点粒度的取舍

太细（每个 for 循环一个 span）→ span 爆炸、信噪比低。
太粗（只埋 Call 外层）→ 看不到 RAG 内部瓶颈。

规则：**用户能直接理解的边界 = 一个 span**。
- ✅ Chat 一次调用
- ✅ VectorStore 一次查询
- ✅ RAG 一个阶段（transform / expand / retrieve / refine / augment）
- ✅ Tool 一次调用
- ❌ 不对中间件链的每一环埋 span（用户不关心）
- ❌ 不对每个 Document 的 format 埋 span（无意义）

---

## 12. 核心判断

1. **Lynx 的可观测性设计复刻 Spring AI 的「抽象+适配」双层结构**，只是在 Go 侧用 context 传播、独立 module 适配器实现。
2. **`observation.Registry` 是唯一的第一公民**——核心代码不依赖任何具体后端。
3. **准入规则极简**：只用 stdlib 的实现进 core（Noop、slog），引入第三方 SDK 的实现走 `observations/` 外部 module。
4. **语义规范锚定 OpenTelemetry GenAI**，保证跨语言、跨平台可对比。
5. **Noop 默认 + 显式注入**——零配置无开销，有配置无魔法。
6. **slog 作为开发/轻量部署的一等公民**，升级到它不增加一行 `go get`。
7. **初版只做 trace + 少量基础 metric**，待真实场景反馈再扩展。

这套设计的价值：
- **开发态**：直接用 `core/observation.NewSlogRegistry(...)` 看日志，零依赖
- **生产态**：`observations/otel` 接 Jaeger / Tempo / Datadog，业务代码零改动
- **不用观测的用户**：`go.sum` 一字不增

这就是「依赖抽象而非具体」+「stdlib 留 core、第三方走外部 module」的双重落地。

---

*（配套文档：`ARCHITECTURE.md` §7 Middleware、`SPRING_AI_COMPARISON.md` §G.12 ObservationRegistry、`MIDDLEWARE_DESIGN.md`）*
