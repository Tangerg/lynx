# Lynx 可观测性设计

> **设计修订（2026-04）**：放弃自造 `observation.Registry` 抽象，**直接使用 OpenTelemetry API**。
> OTel 本身就是厂商中立层，再加一层是重复建设。
> 本文档保留「埋点清单」「语义规范」「slog 桥接」「生产接入」几部分——这些才是真正有价值的内容。

---

## 1. 设计决策

### 1.1 直接用 OTel，不再自造抽象

| 维度 | 判断 |
|-----|-----|
| OTel 自身定位 | **就是** vendor-neutral 抽象层，设计目标和 Lynx 原本要做的那层完全重合 |
| Go 生态现状 | 从一开始就是 OTel 独大，没有「Go 的 Micrometer」 |
| API 稳定性 | trace/metric v1.0 自 2022 年稳定，近 3 年无破坏性变更 |
| 依赖成本 | **API 包**（`go.opentelemetry.io/otel/trace`）只含接口 + noop，不拉 gRPC；**SDK 包**才是重的，但只在用户 app 引入 |
| 零配置行为 | 不调 `otel.SetTracerProvider(...)` = 自动 noop，真·零开销 |

**结论**：`core/` 直接 import OTel API 包，取消 `core/observation/` 自造抽象；`observations/` 外部 module 也不需要建立。

### 1.2 设计原则（精简版）

| # | 原则 |
|---|-----|
| P1 | **直接用 OTel API**：核心代码就调 `otel.Tracer("lynx/...")`，不包装 |
| P2 | **严格遵循 OTel GenAI 语义规范**：所有属性名用官方 `gen_ai.*` / `db.*` 前缀 |
| P3 | **Context 传播**：span 通过 `context.Context` 自动串联 |
| P4 | **零配置即 noop**：默认 TracerProvider 不输出任何 span，无开销 |
| P5 | **观测是读路径**：不 mutate 业务数据 |
| P6 | **slog 作为开发态便利**：提供一个内置 `SpanExporter → slog` 桥接，方便本地看 span |

### 1.3 非目标

- ❌ 不做 APM 平台（交给后端厂商）
- ❌ 不内置 OTel SDK（用户在 app 层引入）
- ❌ 不做自研 metrics 聚合（走 OTel metric + Prometheus pull）
- ❌ 不做日志框架（业务日志归业务自己；这里只处理 span → slog）

---

## 2. 依赖成本 —— 对不同部署档位的实际影响

| 档位 | 装什么 | `go.sum` 影响 |
|-----|-------|-------------|
| 纯用 Lynx 库，不观测 | 只装 `core/` 等 | `go.opentelemetry.io/otel`（API 包，仅接口 + noop，~10KB） |
| 开发看 span（slog） | + `otelbridge/slog` | 拉入 OTel SDK 依赖（仅此 module 的 go.sum） |
| 生产上 OTel + Jaeger/Tempo | + `go.opentelemetry.io/otel/sdk` 和 OTLP exporter | OTel 全家桶（gRPC、protobuf） |

**关键**：
- `core/` 只 import OTel **API 包**（不装 SDK），所以「不主动配置观测」的用户完全不感知 OTel 重依赖
- **所有需要 OTel SDK 的桥接实现**都放在 **独立外部 module `otelbridge/`**，严格不污染 core
- 与 `models/`、`vectorstores/` 同架构规格：重依赖走外挂，核心保持最小

---

## 3. 语义规范（必须遵守）

所有埋点使用 OpenTelemetry GenAI 官方属性名：

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

OTel 未覆盖的场景用 `lynx.*` 隔离命名空间：

| Key | 类型 |
|-----|-----|
| `lynx.rag.stage` | `transform` / `expand` / `retrieve` / `refine` / `augment` |
| `lynx.rag.query_count` | int |
| `lynx.rag.doc_count` | int |
| `lynx.tool.name` | string |
| `lynx.tool.recursion_depth` | int |
| `lynx.agent.name` | string |
| `lynx.agent.action.name` | string |
| `lynx.agent.process_id` | string |
| `lynx.agent.plan.length` | int |

---

## 4. 埋点清单

### 4.1 Chat / Embedding / Image / Audio Model（`core/model/chat/client.go` 等）

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/codes"
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
            attribute.Int("gen_ai.usage.input_tokens", m.Usage.InputTokens),
            attribute.Int("gen_ai.usage.output_tokens", m.Usage.OutputTokens),
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
    // yield to caller
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

### 4.4 VectorStore（`vectorstores/*/store.go`）

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

### 4.5 Agent Tick / Action / Plan（`agent/runtime/`）

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
    // ...
}

func (p *AgentProcess) executeAction(ctx context.Context, a Action) ... {
    ctx, span := agentTracer.Start(ctx, "lynx.agent.action",
        trace.WithAttributes(attribute.String("lynx.agent.action.name", a.Name())),
    )
    defer span.End()
    // ...
}
```

### 4.6 Tool Middleware

```go
var toolTracer = otel.Tracer("lynx/tool")

ctx, span := toolTracer.Start(ctx, "lynx.tool.invoke",
    trace.WithAttributes(
        attribute.String("lynx.tool.name", toolCall.Name),
        attribute.Int("lynx.tool.recursion_depth", depth),
    ),
)
result, err := tool.Call(ctx, toolCall.Arguments)
if err != nil { span.RecordError(err) }
span.End()
```

### 4.7 埋点粒度规则

**一个 span = 用户能直接理解的边界**：
- ✅ Chat 一次调用
- ✅ VectorStore 一次查询
- ✅ RAG 一个阶段
- ✅ Agent 一个 tick / action
- ✅ Tool 一次调用
- ❌ 不对中间件链每环埋 span
- ❌ 不对每个 Document format 埋 span

---

## 5. Noop 行为（默认）

**什么都不用写**。OTel 的 `TracerProvider` 默认返回 Noop 实现：

```go
import "go.opentelemetry.io/otel"

// 用户 main.go 里什么都不调
func main() {
    client := chat.NewClient(...)
    client.ChatWithText("hello").Call().Response(ctx)
    // ↑ 这里面所有 otel.Tracer(...).Start(...) 返回 noop span，零开销
}
```

**OTel 官方保证**：noop `TracerProvider` 的 `Start()`、`span.End()`、`span.SetAttributes(...)` 都是 inline-able 的空函数，编译后几乎被优化掉。**实测零分配、零系统调用**。

---

## 6. Slog 桥接 —— 开发态看 span

### 6.1 设计

OTel SDK 通过 **`SpanExporter` 接口**输出 span。我们实现一个把 span 写进 slog 的 exporter——这是标准 OTel 扩展点，跟写 stdouttrace、jaeger exporter 是同一个模式。

### 6.2 落位

独立外部 module 的子包：**`otelbridge/slog/`**（父包 `otelbridge` 已表达 "otel bridge" 含义，子包不再重复 `otel` 前缀）。

理由（取舍详见本节末）：
- 依赖 OTel SDK，不能放 `core/` 或 `pkg/`（会污染整个依赖生态）
- 与 `models/`、`vectorstores/` 同规格，符合 Lynx「重依赖走外挂」的架构原则
- 不用 slog exporter 的用户**完全零成本**

### 6.3 完整实现

见仓库 `otelbridge/slog/exporter.go`（已实现）。核心代码骨架：

```go
package slog

import (
    "context"
    stdslog "log/slog"

    "go.opentelemetry.io/otel/codes"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Exporter 把 OTel span 输出到 log/slog。
type Exporter struct {
    logger *slog.Logger
}

func NewExporter(logger *slog.Logger) *Exporter {
    if logger == nil {
        logger = slog.Default()
    }
    return &Exporter{logger: logger}
}

// ExportSpans 实现 sdktrace.SpanExporter 接口
func (e *Exporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
    for _, span := range spans {
        attrs := make([]slog.Attr, 0, 6+len(span.Attributes()))

        sc := span.SpanContext()
        attrs = append(attrs,
            slog.String("trace_id", sc.TraceID().String()),
            slog.String("span_id", sc.SpanID().String()),
            slog.String("name", span.Name()),
            slog.Duration("duration", span.EndTime().Sub(span.StartTime())),
        )

        if parent := span.Parent(); parent.HasSpanID() {
            attrs = append(attrs, slog.String("parent_span_id", parent.SpanID().String()))
        }

        // OTel 属性（gen_ai.* / db.* / lynx.* 等）→ slog 属性
        for _, kv := range span.Attributes() {
            attrs = append(attrs, slog.Any(string(kv.Key), kv.Value.AsInterface()))
        }

        if evs := span.Events(); len(evs) > 0 {
            names := make([]string, len(evs))
            for i, ev := range evs { names[i] = ev.Name }
            attrs = append(attrs, slog.Any("events", names))
        }

        level, msg := slog.LevelInfo, "span"
        if status := span.Status(); status.Code == codes.Error {
            level = slog.LevelError
            if status.Description != "" {
                msg = "span (error): " + status.Description
            } else {
                msg = "span (error)"
            }
        }

        e.logger.LogAttrs(ctx, level, msg, attrs...)
    }
    return nil
}

func (e *Exporter) Shutdown(ctx context.Context) error { return nil }
```

完整源码含：详细 godoc、处理空 status description、`exporter_test.go`（5 个测试覆盖 success / error / child-parent / nil logger / shutdown）。

### 6.4 用户接入（~10 行）

```go
import (
    stdslog "log/slog"                                    // stdlib（别名避免冲突）
    "go.opentelemetry.io/otel"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    "github.com/Tangerg/lynx/otelbridge/slog"              // 本包
)

func main() {
    // 开发态：slog 输出，同步模式看得快
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithSyncer(slog.NewExporter(stdslog.Default())),
    )
    otel.SetTracerProvider(tp)
    defer tp.Shutdown(context.Background())

    // 之后所有 Lynx 埋点都会在 slog 输出
    runApp()
}
```

### 6.5 示例输出

```
time=2026-04-20T... level=INFO msg=span trace_id=a1b2c3d4 span_id=aabb...
  name=gen_ai.chat duration=523ms
  gen_ai.system=openai gen_ai.request.model=gpt-4
  gen_ai.usage.input_tokens=120 gen_ai.usage.output_tokens=85

time=... level=INFO msg=span trace_id=a1b2c3d4 parent_span_id=aabb... span_id=ccdd...
  name=lynx.tool.invoke duration=45ms
  lynx.tool.name=web_search lynx.tool.recursion_depth=1

time=... level=ERROR msg="span (error): timeout after 30s" trace_id=...
  name=lynx.agent.action lynx.agent.action.name=classifyIntent duration=30s
```

父子 span 通过 `parent_span_id` 关联，就能重建整条调用链。

### 6.6 可选增强

- **仅打错误 span**：在 `ExportSpans` 里按 `span.Status().Code == codes.Error` 过滤
- **慢调用告警**：按 `duration > 1s` 升到 `Warn` 级
- **采样**：`sdktrace.WithSampler(sdktrace.TraceIDRatioBased(0.1))` 只采 10%
- **隐私过滤**：在 exporter 里删除 `gen_ai.prompt` 这类敏感属性

### 6.7 什么时候用 slog exporter，什么时候用别的

| 场景 | 推荐 |
|-----|-----|
| 单机开发、快速看调用 | **slog exporter**（和业务日志同一路径） |
| 看官方格式、不与业务日志融合 | `stdouttrace`（OTel 官方 stdout exporter） |
| 生产要保留 trace 结构 | OTLP → Jaeger / Tempo / Grafana Tempo |
| 生产要按指标监控 | Prometheus pull（metric exporter，不是 span） |

**不要同时挂多个 exporter**——SDK 支持多 processor，但 I/O 会重复。生产一般只挂一个 OTLP 就够了，本地开发挂 slog。

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
    sdktrace.WithBatcher(exporter),   // 生产用 batcher 减少 I/O
    sdktrace.WithResource(resource.NewWithAttributes(
        semconv.SchemaURL,
        semconv.ServiceName("my-lynx-app"),
    )),
)
otel.SetTracerProvider(tp)
```

### 7.2 Datadog / NewRelic / Honeycomb

各自官方有 OTLP 接收端或专用 exporter，配置一行 endpoint 即可。**Lynx 代码完全不动**——这是用 OTel 的根本好处。

---

## 8. 落地清单

> **2026-04-20 复核进度**（HEAD = `63e4bb2`）：exporter 两项落地；**核心埋点 0 条**——span 目前没有源头产生，只有接收器就绪。

### 8.1 立即可做
- [x] **slog exporter 已实现**：`otelbridge/slog/`（独立外部 module，5 项单测）
- [x] **stdlib log exporter 已实现**：`otelbridge/log/`（logfmt 风格单行输出，6 项单测）
- [ ] 把 `core/model/chat/client.go` 的 Call/Stream 加 OTel 埋点 ← **未动工**（代码中无 `otel.Tracer(...)` 调用）
- [ ] 把 `core/rag/pipeline.go` 五阶段加 span ← **未动工**
- [ ] 所有 vectorstore 实现统一加 `db.vector.*` 埋点 ← **未动工**（qdrant/milvus/pinecone/weaviate/chroma 无一挂 span）
- [ ] 在 `doc/` 写一页「Lynx observability quickstart」示例 ← **未动工**

### 8.2 M3 阶段（agent 落地时）
- [ ] `agent/runtime/` 所有 tick / action / plan 埋点
- [ ] Tool middleware 埋点

### 8.3 不做的事
- ❌ **不**写 `observation.Registry` 接口
- ❌ **不**建 `observations/` 外部 module
- ❌ **不**自己写 `Convention` / `Adapter` 抽象
- ❌ **不**提供 Prometheus metrics exporter（生产用户自己配 OTel metric SDK + prom exporter）

---

## 9. 关键取舍总结

| 问题 | 结论 |
|-----|-----|
| 是不是需要自造观测抽象？ | **不需要**。OTel 就是那层 |
| 依赖会不会爆？ | **不会**。API 包极轻，SDK 只在用户 app 引入 |
| 用户能不能 "零依赖"？ | **能**。不调 SetTracerProvider 就是 noop，无分配 |
| 开发态如何看 span？ | **自带 slog exporter**，60 行桥接代码 |
| 生产如何接后端？ | **用户自配 OTel SDK + exporter**，Lynx 代码不变 |
| 为什么 Spring AI 用 Micrometer？ | 历史包袱。Go 世界从来没有这个问题 |

---

## 10. 与原方案的 diff

| 维度 | 原方案 | 新方案 |
|-----|-------|-------|
| 核心抽象 | `observation.Registry` 接口 | **删除**——直接用 OTel API |
| Convention 概念 | `observation.Convention` | **删除**——属性名走 OTel GenAI 规范 |
| Noop 实现 | `core/observation/noop.go` | **OTel 自带**，零代码 |
| slog 实现 | `core/observation/slog.go` 按自造接口 | **`SpanExporter` 实现**，60 行 |
| OTel 适配器 | `observations/otel/` 外部 module | **不需要**——用户直接用 OTel SDK |
| Prometheus 适配器 | `observations/prom/` 外部 module | **不需要**——用户自配 OTel metric + prom |
| 代码总量 | ~400 行抽象层 + 适配器 | ~60 行 slog exporter |

---

**最终判断**：原方案是「用 Java 思路解 Go 问题」的错位设计。Go 世界里 OTel 已经是共识，再加一层是负资产。新方案更轻、更标准、更容易被社区接受。
