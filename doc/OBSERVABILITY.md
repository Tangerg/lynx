# 可观测性

> Lynx 在应用、integration 和独立 `otel` module 中**直接使用 OpenTelemetry API**，不自造观测抽象。Core 不 import OTel；`otel` wrapper 从外层包装 Core 协议调用。
>
> **当前状态（2026-07-14 更新）**：Traces / Metrics / Logs 都是 OTel 信号，dev 统一 sink 到 `log/slog`、生产换 OTLP。`otel.ChatMiddleware`、`otel/slog` 的三个 exporter 与 `app/runtime` startup 组合根已经可用；RAG、MCP、Agent、VectorStore、ChatHistory 的外圈埋点继续直接使用官方 API。Core 的生产代码和 module graph 不依赖 OTel；Chat/Embedding 等能力在独立 `otel` module 或消费方边界包装。

---

## 1. 设计决策

### 1.1 直接用 OTel，但保持正确的依赖方向

| 维度 | 判断 |
|-----|-----|
| OTel 自身定位 | **就是** vendor-neutral 抽象层，设计目标和「Lynx 自造一层」完全重合 |
| Go 生态现状 | 从一开始就是 OTel 独大，没有「Go 的 Micrometer」 |
| API 稳定性 | trace/metric v1.0 自 2022 年稳定，近 3 年无破坏性变更 |
| 依赖成本 | API 包只含接口与 noop；SDK 才包含 provider/exporter 实现。Core 两者都不依赖 |
| 零配置行为 | 不设置全局 provider 时信号不导出；wrapper 仍有计时、属性读取和流聚合成本 |

**结论**：`otel`、应用和 integration 直接 import 官方 OTel API，不建 `core/observation` 或自造 tracer/meter 接口。`otel` 从外层 import Core 并提供普通 decorator；Core 只传播 `context.Context` 和协议值，不 import OTel。需要 SDK 的 dev-sink 也位于 `otel` module。

### 1.2 设计原则

| # | 原则 |
|---|-----|
| P1 | **直接用官方 OTel API**：应用/integration/`otel` wrapper 直接调 `otel.Tracer("lynx/...")` / `otel.Meter("lynx/...")`；不包装官方 API，但用普通 decorator 保持 Core 的依赖方向 |
| P2 | **去品牌、严格 semconv**：attr key 有 semconv 就用（`gen_ai.*` / `db.*` / `rpc.method`），否则裸 domain（`run.*` / `agent.*` / `rag.*`）。**不带 `lynx.*` / `lyra.*` 前缀**（scope 名 `lynx/...` 是库标识，例外） |
| P3 | **Context 传播 + 全链路**：span 通过 `context.Context` 自动串联；trace_id 在入口生成，脱钩 goroutine 用 `context.WithoutCancel` 保 span |
| P4 | **零配置不输出**：默认官方 provider 为 noop；不宣称 wrapper 自身零开销 |
| P5 | **观测是读路径**：不 mutate 业务数据 |
| P6 | **观测靠 span + metric,不撒 slog**：一个事件该被观测就开 span / 记 metric,不加 `slog.InfoContext` 行;span 经 sink 渲染即"日志"。Logs 管线（bridge → LoggerProvider → LogExporter）仍在,作 vendor-neutral sink + 兜 stdlib log,不是邀请到处写 slog |

### 1.3 非目标

- ❌ 不做 APM 平台（交给后端厂商）
- ❌ Core 不内置 OTel API/SDK；SDK 只由 `otel` dev-sink 或具体应用引入
- ❌ 不自造观测抽象（`Registry` / `Convention` / `Adapter` 一概不写——OTel 就是那层）
- ✅ **但**：发 metric instrument（不只是 span）、Logs 经 `otelslog` bridge → `NewLogExporter` 成一等 OTel 信号——三驾马车都要齐且都可一键换 OTLP（旧版"不做 metrics 聚合 / 不做日志框架"的措辞已废：我们不写聚合/框架，但**三个信号都发 + sink 到 slog**）

---

## 2. 依赖成本

| 档位 | 装什么 | `go.sum` 影响 |
|-----|-------|-------------|
| 纯用 Core 协议，不观测 | 只装 `core/` | 不增加任何 OTel 依赖 |
| 为 Core 调用加观测 | + `github.com/Tangerg/lynx/otel` wrapper | 根包生产代码使用 OTel API；同 module 的 dev sink/test 使 module graph 包含官方 SDK |
| 开发态看 span（slog）| + `otel/slog` | 使用同 module 内的 OTel SDK exporter 实现 |
| 生产 OTel + Jaeger/Tempo | + `go.opentelemetry.io/otel/sdk` + OTLP exporter | OTel 全家桶（gRPC、protobuf）|

**关键**：
- `core/` 不 import OTel API 或 SDK；只用协议的用户不承担任何 OTel 依赖
- Core 调用的 span/metric 由独立 `otel` module 的 decorator 发射；它直接使用官方 API，不增加自造抽象
- 所有需要 OTel SDK 的桥接实现都放在 `otel`/应用组合根，严格不污染 Core
- 与 `models/`、`vectorstores/` 同规格——重依赖走外挂

---

## 3. 语义规范（必须遵守）

所有埋点使用 OpenTelemetry GenAI 官方属性名。

### 3.1 GenAI Model 调用

| Key | 类型 | 说明 |
|-----|-----|-----|
| `gen_ai.provider.name` | string | `openai` / `anthropic` / `gcp.gemini` / `aws.bedrock` 等当前 semconv provider ID |
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

### 3.2 VectorStore（对齐 OTel DB 规范）

| Key | 类型 |
|-----|-----|
| `db.system` | `qdrant` / `milvus` / `chroma` / ... |
| `db.operation.name` | `create` / `retrieve` / `delete` |
| `db.vector.query.top_k` | int |
| `db.vector.query.similarity_threshold` | float |

### 3.3 域扩展键（去品牌：semconv 优先，否则裸 domain）

semconv 已有的概念用 semconv key；没有的用裸 domain（无 `lynx.`/`lyra.` 前缀）。工具失败一律走 **span status=Error + RecordError**，不再用 `is_error` bool。

| Key | 类型 | 备注 |
|-----|-----|-----|
| `gen_ai.tool.name` | string | tool / MCP tool 名（semconv） |
| `gen_ai.tool.call.id` | string | tool call id（semconv） |
| `gen_ai.agent.name` | string | agent 名（semconv，`invoke_agent` 操作） |
| `gen_ai.conversation.id` | string | session / 会话 id（semconv） |
| `gen_ai.operation.name` | string | `chat` / `embeddings` / `invoke_agent` |
| `rpc.method` | string | JSON-RPC 方法（semconv） |
| `agent.process.id` | string | agent process id |
| `agent.action.name` | string | action 名 |
| `agent.process.status` | string | tick/exit 的进程状态 |
| `rag.stage` | string | `transform` / `expand` / `retrieve` / `refine` / `augment` |
| `rag.query_count` / `rag.doc_count` | int | |
| `mcp.server.name` / `mcp.server.count` / `mcp.tool.count` | string/int | MCP dial |
| `run.id` | string | lyra run / turn id |
| `run.outcome` | string | turn 结局（completed / canceled / errored / budget_exceeded） |
| `run.event.id` | string | run 事件序号 |
| `run.interrupt.kind` | string | HITL 中断类型（approval / question） |
| `embeddings.input.count` | int | |

---

## 4. 埋点清单（参考实现）

### 4.1 Chat Model

Model 埋点属于 `otel` wrapper，不属于 Core Client。provider identity 等观测属性在构造 wrapper 时显式传入，不通过 Core Model 强制 `Metadata()`：

```go
instrumentation, err := lynxotel.NewChat(lynxotel.ChatConfig{
	Provider: "openai",
})
if err != nil {
	return err
}

model := chat.Wrap(providerModel, instrumentation.Call)
streamer := chat.WrapStream(providerStreamer, instrumentation.Stream)
```

真实实现位于 `otel/chat.go`：Call/Stream 能力保持分离，构造时可显式注入
`trace.TracerProvider` / `metric.MeterProvider`，并发安全。Embedding 使用当前
`core/embedding` 最小能力；需要埋点时同样在 `otel` 或消费方 decorator 包装，Core
不定义观测接口。

### 4.2 Stream 模式额外事件

```go
ctx, span := m.tracer.Start(ctx, "gen_ai.chat.stream", ...)
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
    ctx, span := ragTracer.Start(ctx, "rag.pipeline")
    defer span.End()

    ctx1, s1 := ragTracer.Start(ctx, "rag.transform",
        trace.WithAttributes(attribute.String("rag.stage", "transform")))
    transformed, err := p.transformQuery(ctx1, q)
    s1.End()
    // ... 其余阶段同样模式
}
```

### 4.4 VectorStore

```go
var qdrantTracer = otel.Tracer("lynx/vectorstore/qdrant")

func (s *Store) Search(ctx context.Context, req vectorstore.SearchRequest) ([]vectorstore.Match, error) {
    ctx, span := qdrantTracer.Start(ctx, "db.vector.search",
        trace.WithAttributes(
            attribute.String("db.system", "qdrant"),
            attribute.String("db.operation.name", "search"),
            attribute.Int("db.vector.query.top_k", req.TopK),
        ),
    )
    defer span.End()
    // ...
    span.SetAttributes(attribute.Int("rag.doc_count", len(matches)))
}
```

### 4.5 Tool Middleware

```go
var toolTracer = otel.Tracer("lynx/tool")

ctx, span := toolTracer.Start(ctx, "execute_tool "+toolCall.Name,
    trace.WithAttributes(
        attribute.String("gen_ai.tool.name", toolCall.Name),
        attribute.String("gen_ai.tool.call.id", toolCall.ID),
    ),
)
result, err := tool.Call(ctx, toolCall.Arguments)
if err != nil {
    // Failures surface via span status, not an is_error bool.
    span.RecordError(err)
    span.SetStatus(codes.Error, err.Error())
}
span.End()
```

### 4.6 MCP Tool（client 与 server 双向）

```go
var mcpTracer = otel.Tracer("lynx/mcp")

// Client 侧 mcp.Tool.Call
ctx, span := mcpTracer.Start(ctx, "execute_tool "+t.descriptor.Name,
    trace.WithAttributes(
        attribute.String("gen_ai.tool.name", t.descriptor.Name),
        attribute.String("mcp.server.name",  t.sourceName),
    ),
)
defer span.End()

// Server 侧 makeServerHandler
ctx, span := mcpTracer.Start(ctx, "execute_tool "+tool.Definition().Name,
    trace.WithAttributes(attribute.String("gen_ai.tool.name", tool.Definition().Name)),
)
defer span.End()
```

### 4.7 Agent Tick / Action / Plan（agent 框架落地后）

```go
var agentTracer = otel.Tracer("lynx/agent")

func (p *Process) Tick(ctx context.Context) error {
    ctx, span := agentTracer.Start(ctx, "agent.tick",
        trace.WithAttributes(
            attribute.String("gen_ai.agent.name", p.Agent.Name),
            attribute.String("agent.process.id", p.ID),
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

Core 本身不含埋点。调用方不安装 wrapper 时就是直接协议调用，不增加 OTel 依赖；安装 `otel` wrapper 但不配置 OTel provider 时，官方全局 provider 为 noop：

```go
func main() {
	instrumentation, _ := lynxotel.NewChat(lynxotel.ChatConfig{Provider: "openai"})
	model := chat.Wrap(providerModel, instrumentation.Call)
	model.Call(ctx, req)
	// 没有 SDK provider 时不导出 span/metric。
}
```

noop 只表示信号不导出。`ChatMiddleware` 仍执行计时、属性投影以及 stream
response 聚合；需要评估热路径成本时必须用 benchmark/pprof 测量，不能把
“provider noop”推导成“wrapper 零成本”。

---

## 6. `otel/`：自带的 dev-sink（span / metric / log → slog）

> 独立外部 module，从外层包装 Core，不污染 Core。Chat wrapper 与 span/metric/log exporter 均已实现。

### 6.1 `otel/slog`：开发态看 span

把 OTel span 写进 `log/slog`：

```go
import (
    stdslog "log/slog"
    "go.opentelemetry.io/otel"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    "github.com/Tangerg/lynx/otel/slog"
)

func main() {
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithSyncer(slog.NewSpanExporter(stdslog.Default())),
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
  gen_ai.provider.name=openai gen_ai.request.model=gpt-4
  gen_ai.usage.input_tokens=120 gen_ai.usage.output_tokens=85

time=... level=INFO msg=span trace_id=a1b2c3d4 parent_span_id=aabb... span_id=ccdd...
  name="execute_tool web_search" duration=45ms
  gen_ai.tool.name=web_search gen_ai.tool.call.id=call_abc

time=... level=ERROR msg="span (error): timeout after 30s" trace_id=...
  name="agent.action" agent.action.name=classifyIntent duration=30s
```

父子 span 通过 `parent_span_id` 关联，重建调用链。

### 6.1a 三驾马车一次性绑全（后端 startup 范式）

单进程后端（lyra）要的是 **Traces + Metrics + Logs 三个 OTel 信号全 sink 到 slog**，startup 一次绑好后业务侧零 DI（`otel.Tracer` / `otel.Meter`,日志经 bridge）。Logs 走 OTel `LoggerProvider`（不直接写 slog）正是为了可替换性——生产把这三个 exporter 换成 OTLP 即全导到云,本代码不动：

```go
import (
    stdslog "log/slog"
    slogbridge "go.opentelemetry.io/contrib/bridges/otelslog"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/propagation"
    sdklog "go.opentelemetry.io/otel/sdk/log"
    sdkmetric "go.opentelemetry.io/otel/sdk/metric"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    otelslog "github.com/Tangerg/lynx/otel/slog"
)

func setupObservability() func(context.Context) {
    base := stdslog.Default() // 实际 stderr sink；三个 exporter 都写它

    // 1) Traces → slog
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithSampler(sdktrace.AlwaysSample()),
        sdktrace.WithSyncer(otelslog.NewSpanExporter(base)),
    )
    otel.SetTracerProvider(tp)

    // 2) Metrics → slog（PeriodicReader 周期吐）
    mp := sdkmetric.NewMeterProvider(
        sdkmetric.WithReader(sdkmetric.NewPeriodicReader(otelslog.NewMetricExporter(base))),
    )
    otel.SetMeterProvider(mp)

    // 3) Logs：LoggerProvider → LogExporter；slog.Default 用 contrib bridge 喂它
    //    （trace_id/span_id 由 OTel log record 原生带，不用手动盖）
    lp := sdklog.NewLoggerProvider(
        sdklog.WithProcessor(sdklog.NewSimpleProcessor(otelslog.NewLogExporter(base))))
    stdslog.SetDefault(stdslog.New(slogbridge.NewHandler("lyra", slogbridge.WithLoggerProvider(lp))))

    // 4) 全链路：W3C traceparent 传播
    otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
        propagation.TraceContext{}, propagation.Baggage{}))

    return func(ctx context.Context) { _ = tp.Shutdown(ctx); _ = mp.Shutdown(ctx); _ = lp.Shutdown(ctx) }
}
```

（生产版：把 `NewSpanExporter`/`NewMetricExporter`/`NewLogExporter` 换成对应的 OTLP exporter,endpoint 指向 collector/Datadog,业务零改。）

**全链路要点**：HTTP transport 入口 `otel.GetTextMapPropagator().Extract` 提客户端 traceparent + 开 server span（trace_id 从入口生成）；脱钩后台 goroutine（run 必须 outlive 请求）用 `context.WithoutCancel(reqCtx)`——保住 span 让子 span 同 trace，只断 cancel，**不要 `context.Background()`**（那会另起一棵 trace）。

### 6.3 用法选择

| 场景 | 推荐 |
|-----|-----|
| 单机开发、快速看调用 | `otel/slog`（与业务日志同一路径）|
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

### 8.1 已就绪（接收侧 / sink）

- [x] `otel/slog/NewSpanExporter`：span → slog（单测）
- [x] `otel/slog/NewMetricExporter`：metric → slog（单测）
- [x] `otel/slog/NewLogExporter`：OTel log record → slog（单测）；log 的 `slog.Handler` 用 contrib `otelslog` bridge
- [x] `app/runtime/cmd/lyra/observability.go::setupObservability`：startup 一次性绑全局 TracerProvider + MeterProvider + LoggerProvider + W3C propagator

### 8.2 已就绪（外圈发射侧）

- [x] `rag/pipeline.go` 五阶段加 span（`rag.pipeline` 父 span + Query/Retrieve/Augment/Generate/Stream 子 span）
- [x] `vectorstores/{pgvector,qdrant,milvus,pinecone,weaviate,chroma,redis,mongodb,cassandra,neo4j,couchbase,typesense,vespa,vectara,bedrockkb,s3vectors,azureaisearch,azurecosmos,mariadb,oracle,tidb,clickhouse,opensearch,elasticsearch,inmemory}` 共 25 个实现统一加 `db.vector.*` 埋点（cockroachdb/supabase 复用 pgvector 实现）
- [x] `mcp/tool.go::Tool.Call` + `mcp/server.go::makeServerHandler` 加 `mcp.tool.call` / `mcp.tool.serve` span
- [x] `agent/runtime/` tick / action / plan 全套埋点：span（含 HTN / Reactive / GOAP planner）+ metrics（`agent.ticks` / `agent.action.executions` / `agent.action.duration` / `agent.plan.duration` / `agent.process.exits`）
- [x] `chathistory/{postgres,redis,mongodb,cassandra,neo4j,cosmosdb}` 6 个 provider Read/Write/Clear 加 DB-semconv span
- [x] **lyra 业务层**：chat turn `invoke_agent <model>` span（全链路父 span）+ `run.duration` / `run.interrupts` metrics；MCP dial（`mcp.dial_servers`）、直调 tool（`execute_tool`）span；session/run 生命周期由 RPC server span + turn span 覆盖（**不撒 slog**——见 §1.2 P6）

### 8.3 Core 外移状态

- [x] P3-06：新增 `otel.ChatMiddleware`，以独立 Call/Stream middleware 包装目标 `core/chat` 能力
- [x] P3-06：删除 `core/model/chat`、`core/model/embedding` 旧 tracing 与 Core 通用 metrics，不建立旧 API adapter
- [x] P5-01：建立 `core/embedding` 最小能力，Core 不复制旧 wrapper；Embedding 埋点归外圈 decorator
- [x] P3-07/P3-08：tool/tool-loop 观测归 `tools`、`agent`、MCP/A2A adapter 或外圈 decorator
- [x] 删除 Core 对 OTel API/SDK 的依赖并收紧 `internal/arch` 依赖预算
- [ ] vectorstore + chathistory 的端到端 span 行为单测（chat tracing_test 与 runtime tracing 测试已覆盖各自一例）

### 8.4 不做的事

- ❌ 不写 `observation.Registry` 接口
- ❌ 不自己写 `Convention` / `Adapter` 抽象
- ❌ 不提供 Prometheus metrics exporter（生产用户自己配 OTel metric SDK + prom exporter）
- ⚠️ `otel/` 外部 module **是有意保留的**（Core wrapper + dev 态 span/metric/log → slog sink）——外挂改变依赖方向，不等于自造观测抽象

---

## 9. 关键取舍

| 问题 | 结论 |
|-----|-----|
| 是不是需要自造观测抽象？ | **不需要**。OTel 就是那层 |
| 依赖会不会爆？ | Core **零 OTel 依赖**；选择 `otel` module 才进入 OTel module graph，SDK 由同 module 的 dev-sink/test 与具体应用使用 |
| 用户能不能不引入 OTel？ | **能**。不使用 wrapper 时 Core 的 module graph 中没有 OTel；使用 wrapper 但不设 OTel provider 时信号不导出 |
| 开发态如何看 span？ | `otel/slog` 自带 span/metric/log 三个 exporter |
| 生产如何接后端？ | **用户自配 OTel SDK + exporter**，Lynx 代码不变 |
| 为什么 Spring AI 用 Micrometer？ | 历史包袱。Go 世界从来没有这个问题 |

---

## 10. 一句话定档

> Go 世界里 OTel 已经是共识，再加一套观测接口是负资产。Lynx 的边界是「Core 只定义协议且不承担 OTel 依赖；`otel`/integration 直接使用官方 OTel API，从外层以 decorator 发 Traces+Metrics；`otel/slog` 把三驾马车 sink 到 slog；应用 startup 一次绑定全局 provider，生产用户可换官方 exporter」。attr key 遵循 semconv，trace_id 从入口生成并沿 context 全链路传播。
