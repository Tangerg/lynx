# 可观测性

> Lynx **直接使用 OpenTelemetry API**，不自造观测抽象。OTel 本身就是厂商中立层，再加一层是重复建设。
>
> **当前状态**：`otelbridge/slog` + `otelbridge/log` 两个 SpanExporter 已实现；**核心代码尚未挂上任何 `otel.Tracer(...)` 埋点**——即「接收器就绪、源头未启动」。

---

## 1. 设计决策

### 1.1 直接用 OTel，不再自造抽象

| 维度 | 判断 |
|-----|-----|
| OTel 自身定位 | **就是** vendor-neutral 抽象层，设计目标和「Lynx 自造一层」完全重合 |
| Go 生态现状 | 从一开始就是 OTel 独大，没有「Go 的 Micrometer」 |
| API 稳定性 | trace/metric v1.0 自 2022 年稳定，近 3 年无破坏性变更 |
| 依赖成本 | API 包（`go.opentelemetry.io/otel/trace`）只含接口 + noop，不拉 gRPC；SDK 包才是重的，但只在用户 app 引入 |
| 零配置行为 | 不调 `otel.SetTracerProvider(...)` = 自动 noop，真·零开销 |

**结论**：`core/` 直接 import OTel API 包，不建 `core/observation/` 自造抽象，不建 `observations/` 外部 module。

### 1.2 设计原则

| # | 原则 |
|---|-----|
| P1 | **直接用 OTel API**：核心代码就调 `otel.Tracer("lynx/...")`，不包装 |
| P2 | **严格遵循 OTel GenAI 语义规范**：所有属性名用官方 `gen_ai.*` / `db.*` 前缀 |
| P3 | **Context 传播**：span 通过 `context.Context` 自动串联 |
| P4 | **零配置即 noop**：默认 TracerProvider 不输出任何 span，无开销 |
| P5 | **观测是读路径**：不 mutate 业务数据 |
| P6 | **slog 作为开发态便利**：内置 `SpanExporter → slog` 桥接，方便本地看 span |

### 1.3 非目标

- ❌ 不做 APM 平台（交给后端厂商）
- ❌ 不内置 OTel SDK（用户在 app 层引入）
- ❌ 不做自研 metrics 聚合（走 OTel metric + Prometheus pull）
- ❌ 不做日志框架（业务日志归业务自己；这里只处理 span → slog）

---

## 2. 依赖成本

| 档位 | 装什么 | `go.sum` 影响 |
|-----|-------|-------------|
| 纯用 Lynx 库，不观测 | 只装 `core/` 等 | `go.opentelemetry.io/otel`（API 包，仅接口 + noop，~10KB）|
| 开发态看 span（slog）| + `otelbridge/slog` | 拉入 OTel SDK 依赖（仅此 module 的 go.sum）|
| 生产 OTel + Jaeger/Tempo | + `go.opentelemetry.io/otel/sdk` + OTLP exporter | OTel 全家桶（gRPC、protobuf）|

**关键**：
- `core/` 只 import OTel **API 包**（不装 SDK）——不主动配置观测的用户完全不感知 OTel 重依赖
- 所有需要 OTel SDK 的桥接实现都放在独立外部 module `otelbridge/`，严格不污染 core
- 与 `models/`、`vectorstores/` 同规格——重依赖走外挂

---

## 3. 语义规范（必须遵守）

所有埋点使用 OpenTelemetry GenAI 官方属性名。

### 3.1 GenAI Model 调用

| Key | 类型 | 说明 |
|-----|-----|-----|
| `gen_ai.system` | string | `openai` / `anthropic` / `google` / `ollama` |
| `gen_ai.operation.name` | string | `chat` / `embeddings` / `image` / ... |
| `gen_ai.request.model` | string | 请求模型 ID |
| `gen_ai.response.model` | string | 实际返回模型 ID |
| `gen_ai.request.max_tokens` | int | |
| `gen_ai.request.temperature` | float | |
| `gen_ai.request.top_p` | float | |
| `gen_ai.response.finish_reasons` | []string | |
| `gen_ai.response.id` | string | |
| `gen_ai.usage.input_tokens` | int | |
| `gen_ai.usage.output_tokens` | int | |
| `gen_ai.usage.total_tokens` | int | |

### 3.2 VectorStore（对齐 OTel DB 规范）

| Key | 类型 |
|-----|-----|
| `db.system` | `qdrant` / `milvus` / `chroma` / ... |
| `db.operation.name` | `create` / `retrieve` / `delete` |
| `db.vector.query.top_k` | int |
| `db.vector.query.similarity_threshold` | float |

### 3.3 Lynx 专有扩展（`lynx.*` 前缀）

| Key | 类型 |
|-----|-----|
| `lynx.rag.stage` | `transform` / `expand` / `retrieve` / `refine` / `augment` |
| `lynx.rag.query_count` | int |
| `lynx.rag.doc_count` | int |
| `lynx.tool.name` | string |
| `lynx.tool.recursion_depth` | int |
| `lynx.tool.is_error` | bool |
| `lynx.mcp.server` | string（MCP source name）|
| `lynx.mcp.tool.is_error` | bool |
| `lynx.agent.name` | string（agent 框架落地后启用）|
| `lynx.agent.action.name` | string |
| `lynx.agent.process_id` | string |
| `lynx.agent.plan.length` | int |

---

## 4. 埋点清单（待落地）

### 4.1 Chat / Embedding / Image / Audio Model

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/codes"
    "go.opentelemetry.io/otel/trace"
)

var chatTracer = otel.Tracer("lynx/chat")

func (c *ClientCaller) call(ctx context.Context, req *Request) (*Response, error) {
    ctx, span := chatTracer.Start(ctx, "gen_ai.chat",
        trace.WithAttributes(
            attribute.String("gen_ai.system", c.request.model.Info().Provider),
            attribute.String("gen_ai.operation.name", "chat"),
            attribute.String("gen_ai.request.model", req.Options.Model),
        ),
    )
    defer span.End()

    res, err := c.request.MiddlewareManager().BuildCallHandler(c.request.model).Call(ctx, req)
    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())
        return nil, err
    }

    if m := res.Metadata; m != nil && m.Usage != nil {
        span.SetAttributes(
            attribute.Int64("gen_ai.usage.input_tokens",  m.Usage.PromptTokens),
            attribute.Int64("gen_ai.usage.output_tokens", m.Usage.CompletionTokens),
        )
    }
    return res, nil
}
```

### 4.2 Stream 模式额外事件

```go
ctx, span := chatTracer.Start(ctx, "gen_ai.chat.stream", ...)
defer span.End()

firstChunk := true
for chunk, err := range streamHandler.Stream(ctx, req) {
    if firstChunk {
        span.AddEvent("first_token_received")
        firstChunk = false
    }
}
```

### 4.3 RAG Pipeline 五阶段

每阶段一个子 span，自然形成调用树：

```go
var ragTracer = otel.Tracer("lynx/rag")

func (p *Pipeline) Execute(ctx context.Context, q *Query) (*Query, error) {
    ctx, span := ragTracer.Start(ctx, "lynx.rag.pipeline")
    defer span.End()

    ctx1, s1 := ragTracer.Start(ctx, "lynx.rag.transform",
        trace.WithAttributes(attribute.String("lynx.rag.stage", "transform")))
    transformed, err := p.transformQuery(ctx1, q)
    s1.End()
    // ... 其余阶段同样模式
}
```

### 4.4 VectorStore

```go
var qdrantTracer = otel.Tracer("lynx/vectorstore/qdrant")

func (s *Store) Retrieve(ctx context.Context, req *RetrievalRequest) ([]*Document, error) {
    ctx, span := qdrantTracer.Start(ctx, "db.vector.retrieve",
        trace.WithAttributes(
            attribute.String("db.system", "qdrant"),
            attribute.String("db.operation.name", "retrieve"),
            attribute.Int("db.vector.query.top_k", req.TopK),
        ),
    )
    defer span.End()
    // ...
    span.SetAttributes(attribute.Int("lynx.rag.doc_count", len(docs)))
}
```

### 4.5 Tool Middleware

```go
var toolTracer = otel.Tracer("lynx/tool")

ctx, span := toolTracer.Start(ctx, "lynx.tool.invoke",
    trace.WithAttributes(
        attribute.String("lynx.tool.name", toolCall.Name),
        attribute.Int("lynx.tool.recursion_depth", depth),
    ),
)
result, err := tool.Call(ctx, toolCall.Arguments)
if err != nil {
    span.RecordError(err)
    span.SetAttributes(attribute.Bool("lynx.tool.is_error", true))
}
span.End()
```

### 4.6 MCP Tool（client 与 server 双向）

```go
var mcpTracer = otel.Tracer("lynx/mcp")

// Client 侧 mcp.Tool.Call
ctx, span := mcpTracer.Start(ctx, "lynx.mcp.tool.call",
    trace.WithAttributes(
        attribute.String("lynx.tool.name",  t.descriptor.Name),
        attribute.String("lynx.mcp.server", t.sourceName),
    ),
)
defer span.End()

// Server 侧 makeServerHandler
ctx, span := mcpTracer.Start(ctx, "lynx.mcp.tool.serve",
    trace.WithAttributes(attribute.String("lynx.tool.name", tool.Definition().Name)),
)
defer span.End()
```

### 4.7 Agent Tick / Action / Plan（agent 框架落地后）

```go
var agentTracer = otel.Tracer("lynx/agent")

func (p *AgentProcess) Tick(ctx context.Context) error {
    ctx, span := agentTracer.Start(ctx, "lynx.agent.tick",
        trace.WithAttributes(
            attribute.String("lynx.agent.name", p.Agent.Name),
            attribute.String("lynx.agent.process_id", p.ID),
        ),
    )
    defer span.End()
}
```

### 4.8 埋点粒度规则

**一个 span = 用户能直接理解的边界**：
- ✅ Chat 一次调用
- ✅ VectorStore 一次查询
- ✅ RAG 一个阶段
- ✅ Agent 一个 tick / action
- ✅ Tool 一次调用（含 MCP tool）
- ❌ 不对中间件链每环埋 span
- ❌ 不对每个 Document format 埋 span

---

## 5. Noop 行为（默认）

什么都不用写。OTel 的 `TracerProvider` 默认返回 Noop 实现：

```go
import "go.opentelemetry.io/otel"

func main() {
    client := chat.NewClient(...)
    client.ChatWithText("hello").Call().Response(ctx)
    // ↑ 这里面所有 otel.Tracer(...).Start(...) 返回 noop span，零开销
}
```

**OTel 官方保证**：noop `TracerProvider` 的 `Start()`、`span.End()`、`span.SetAttributes(...)` 都是 inline-able 的空函数，编译后几乎被优化掉。**实测零分配、零系统调用**。

---

## 6. otelbridge：自带的 SpanExporter

> 独立外部 module，不污染 core。两个 exporter 都已实现并有单测。

### 6.1 `otelbridge/slog`：开发态看 span

把 OTel span 写进 `log/slog`：

```go
import (
    stdslog "log/slog"
    "go.opentelemetry.io/otel"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    "github.com/Tangerg/lynx/otelbridge/slog"
)

func main() {
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithSyncer(slog.NewExporter(stdslog.Default())),
    )
    otel.SetTracerProvider(tp)
    defer tp.Shutdown(context.Background())

    runApp()
}
```

输出：

```
time=2026-04-30T... level=INFO msg=span trace_id=a1b2c3d4 span_id=aabb...
  name=gen_ai.chat duration=523ms
  gen_ai.system=openai gen_ai.request.model=gpt-4
  gen_ai.usage.input_tokens=120 gen_ai.usage.output_tokens=85

time=... level=INFO msg=span trace_id=a1b2c3d4 parent_span_id=aabb... span_id=ccdd...
  name=lynx.tool.invoke duration=45ms
  lynx.tool.name=web_search lynx.tool.recursion_depth=1

time=... level=ERROR msg="span (error): timeout after 30s" trace_id=...
  name=lynx.agent.action lynx.agent.action.name=classifyIntent duration=30s
```

父子 span 通过 `parent_span_id` 关联，重建调用链。

### 6.2 `otelbridge/log`：stdlib log 版

logfmt 风格单行输出，依赖更少（不需要 slog）。

### 6.3 用法选择

| 场景 | 推荐 |
|-----|-----|
| 单机开发、快速看调用 | `otelbridge/slog`（与业务日志同一路径）|
| 看官方格式、不与业务日志融合 | OTel 官方 `stdouttrace` |
| 生产保留 trace 结构 | OTLP → Jaeger / Tempo |
| 生产按指标监控 | Prometheus pull（metric exporter，不是 span）|

**不要同时挂多个 exporter**——SDK 支持多 processor，但 I/O 会重复。生产一般只挂一个 OTLP，本地开发挂 slog。

---

## 7. 生产接入 OTel（参考）

### 7.1 OTLP gRPC 到 Grafana Tempo / Jaeger

```go
import (
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

exporter, _ := otlptracegrpc.New(ctx,
    otlptracegrpc.WithEndpoint("tempo:4317"),
    otlptracegrpc.WithInsecure(),
)
tp := sdktrace.NewTracerProvider(
    sdktrace.WithBatcher(exporter),    // 生产用 batcher 减少 I/O
    sdktrace.WithResource(resource.NewWithAttributes(
        semconv.SchemaURL,
        semconv.ServiceName("my-lynx-app"),
    )),
)
otel.SetTracerProvider(tp)
```

### 7.2 Datadog / NewRelic / Honeycomb

各自有 OTLP 接收端或专用 exporter，配置一行 endpoint 即可。**Lynx 代码完全不动**——这是用 OTel 的根本好处。

---

## 8. 落地清单

### 8.1 已就绪（接收侧）

- [x] `otelbridge/slog/`：5 项单测
- [x] `otelbridge/log/`：6 项单测（logfmt 风格）

### 8.2 待动工（发射侧）

> 当前 `grep -r 'otel.Tracer' core/ models/ vectorstores/ mcp/` 零命中——核心埋点全部未挂。

- [ ] `core/model/chat/client.go` Call/Stream 加 OTel span
- [ ] `core/rag/pipeline.go` 五阶段加 span
- [ ] `vectorstores/{qdrant,milvus,pinecone,weaviate,chroma}` 统一加 `db.vector.*` 埋点
- [ ] `mcp/tool.go::Tool.Call` + `mcp/server.go::makeServerHandler` 加 span（与 v2 反向能力工作捆绑）
- [ ] `core/model/chat/tool_middleware.go` 加 `lynx.tool.invoke` span
- [ ] `doc/` 写一页「Lynx observability quickstart」示例

### 8.3 Agent 框架落地时

- [ ] `agent/runtime/` 所有 tick / action / plan 埋点
- [ ] Plan / Goal / Action 的事件 → span 映射

### 8.4 不做的事

- ❌ 不写 `observation.Registry` 接口
- ❌ 不建 `observations/` 外部 module
- ❌ 不自己写 `Convention` / `Adapter` 抽象
- ❌ 不提供 Prometheus metrics exporter（生产用户自己配 OTel metric SDK + prom exporter）

---

## 9. 关键取舍

| 问题 | 结论 |
|-----|-----|
| 是不是需要自造观测抽象？ | **不需要**。OTel 就是那层 |
| 依赖会不会爆？ | **不会**。API 包极轻，SDK 只在用户 app 引入 |
| 用户能不能「零依赖」？ | **能**。不调 `SetTracerProvider` 就是 noop，无分配 |
| 开发态如何看 span？ | **自带 slog / log 两个 exporter**，60-80 行桥接代码 |
| 生产如何接后端？ | **用户自配 OTel SDK + exporter**，Lynx 代码不变 |
| 为什么 Spring AI 用 Micrometer？ | 历史包袱。Go 世界从来没有这个问题 |

---

## 10. 一句话定档

> Go 世界里 OTel 已经是共识，再加一层是负资产。Lynx 的 observability 故事就是「core 直接 import OTel API 包 + otelbridge/ 提供 slog/log 两个开发态 exporter + 生产用户自配 OTel SDK」。当前接收侧已就绪，发射侧（核心代码挂 span）是下一阶段工作——埋点点位与语义已在本文 §3-§4 定下，落地是机械活。
