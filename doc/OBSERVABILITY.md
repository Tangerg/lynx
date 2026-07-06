# 可观测性

> Lynx **直接使用 OpenTelemetry API**，不自造观测抽象。OTel 本身就是厂商中立层，再加一层是重复建设。
>
> **当前状态（2026-06-05 更新）**：可观测性三驾马车（**Traces / Metrics / Logs**）都是 OTel 信号，dev 统一 sink 到 `log/slog`、生产换 OTLP（vendor-neutral）。`otel/slog` 含三个 exporter——`NewSpanExporter`(span) + `NewMetricExporter`(metric) + `NewLogExporter`(OTel log record→slog)；log 的 `slog.Handler` 用 contrib `otelslog` bridge（→ LoggerProvider → LogExporter）。lyra startup（`cmd/lyra/observability.go::setupObservability`）一次性绑全局 provider。**埋点原则：观测靠 span + metric,不在业务代码撒 `slog`**——span 经 sink 渲染即"日志"。**全模块埋点已落地**——chat（`Call`+`Stream`）、embedding、tool、RAG 五阶段、MCP client+server、agent runtime（tick/action/plan metrics + planner span）、24 个 vectorstore、6 个 chathistory provider 按 GenAI/DB semconv;lyra 业务层 chat turn `invoke_agent` span + `run.*` metrics + 各层 span（session/tool/mcp 经 RPC/dial span 覆盖）。**所有 attr key 已去品牌**（见 §3）。

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

**结论**：`core/` 直接 import OTel API 包、不建 `core/observation/` 自造抽象；需要 OTel SDK 的 dev-sink（span/metric/log → slog）外挂在独立 `otel/` module，不污染 core。

### 1.2 设计原则

| # | 原则 |
|---|-----|
| P1 | **直接用 OTel API**：核心代码就调 `otel.Tracer("lynx/...")` / `otel.Meter("lynx/...")`，不包装、零 DI |
| P2 | **去品牌、严格 semconv**：attr key 有 semconv 就用（`gen_ai.*` / `db.*` / `rpc.method`），否则裸 domain（`run.*` / `agent.*` / `rag.*`）。**不带 `lynx.*` / `lyra.*` 前缀**（scope 名 `lynx/...` 是库标识，例外） |
| P3 | **Context 传播 + 全链路**：span 通过 `context.Context` 自动串联；trace_id 在入口生成，脱钩 goroutine 用 `context.WithoutCancel` 保 span |
| P4 | **零配置即 noop**：默认 provider 不输出，无开销 |
| P5 | **观测是读路径**：不 mutate 业务数据 |
| P6 | **观测靠 span + metric,不撒 slog**：一个事件该被观测就开 span / 记 metric,不加 `slog.InfoContext` 行;span 经 sink 渲染即"日志"。Logs 管线（bridge → LoggerProvider → LogExporter）仍在,作 vendor-neutral sink + 兜 stdlib log,不是邀请到处写 slog |

### 1.3 非目标

- ❌ 不做 APM 平台（交给后端厂商）
- ❌ 不内置 OTel SDK（用户在 app 层引入；lyra 这个具体 app 才装 SDK）
- ❌ 不自造观测抽象（`Registry` / `Convention` / `Adapter` 一概不写——OTel 就是那层）
- ✅ **但**：发 metric instrument（不只是 span）、Logs 经 `otelslog` bridge → `NewLogExporter` 成一等 OTel 信号——三驾马车都要齐且都可一键换 OTLP（旧版"不做 metrics 聚合 / 不做日志框架"的措辞已废：我们不写聚合/框架，但**三个信号都发 + sink 到 slog**）

---

## 2. 依赖成本

| 档位 | 装什么 | `go.sum` 影响 |
|-----|-------|-------------|
| 纯用 Lynx 库，不观测 | 只装 `core/` 等 | `go.opentelemetry.io/otel`（API 包，仅接口 + noop，~10KB）|
| 开发态看 span（slog）| + `otel/slog` | 拉入 OTel SDK 依赖（仅此 module 的 go.sum）|
| 生产 OTel + Jaeger/Tempo | + `go.opentelemetry.io/otel/sdk` + OTLP exporter | OTel 全家桶（gRPC、protobuf）|

**关键**：
- `core/` 只 import OTel **API 包**（不装 SDK）——不主动配置观测的用户完全不感知 OTel 重依赖
- 所有需要 OTel SDK 的桥接实现都放在独立外部 module `otel/`，严格不污染 core
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
            attribute.String("gen_ai.system", c.request.model.Metadata().Provider),
            attribute.String("gen_ai.operation.name", "chat"),
            attribute.String("gen_ai.request.model", req.Options.Model),
        ),
    )
    defer span.End()

    res, err := c.request.MiddlewareChain().BuildCallHandler(c.request.model).Call(ctx, req)
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
    span.SetAttributes(attribute.Int("rag.doc_count", len(docs)))
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

func (p *AgentProcess) Tick(ctx context.Context) error {
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

## 6. `otel/`：自带的 dev-sink（span / metric / log → slog）

> 独立外部 module，不污染 core。span/metric exporter + 关联 log handler 都已实现并有单测。

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
  gen_ai.system=openai gen_ai.request.model=gpt-4
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
- [x] `lyra/cmd/lyra/observability.go::setupObservability`：startup 一次性绑全局 TracerProvider + MeterProvider + LoggerProvider + W3C propagator

### 8.2 已就绪（发射侧）

- [x] `core/model/chat/client.go` Call/Stream 加 OTel span（GenAI semconv 全套属性 + `gen_ai.stream.first_token_received` 事件 + 错误状态）
- [x] `core/model/chat/tool.go::invokeOne` 单次 tool 调用加 `execute_tool <name>` span（`gen_ai.tool.*` + span status=Error）
- [x] `core/model/embedding/client.go` Response 加 OTel span（GenAI semconv 子集 + `embeddings.input.count` 扩展）
- [x] `rag/pipeline.go` 五阶段加 span（`rag.pipeline` 父 span + Query/Retrieve/Augment/Generate/Stream 子 span）
- [x] `vectorstores/{pgvector,qdrant,milvus,pinecone,weaviate,chroma,redis,mongodb,cassandra,neo4j,couchbase,typesense,vespa,vectara,bedrockkb,s3vectors,azureaisearch,azurecosmos,mariadb,oracle,tidb,clickhouse,opensearch,elasticsearch,inmemory}` 共 24 个 provider 统一加 `db.vector.*` 埋点（cockroachdb/supabase 通过 pgvector 类型别名继承）
- [x] `mcp/tool.go::Tool.Call` + `mcp/server.go::makeServerHandler` 加 `mcp.tool.call` / `mcp.tool.serve` span
- [x] `agent/runtime/` tick / action / plan 全套埋点：span（含 HTN / Reactive / GOAP planner）+ metrics（`agent.ticks` / `agent.action.executions` / `agent.action.duration` / `agent.plan.duration` / `agent.process.exits`）
- [x] `chathistory/{postgres,redis,mongodb,cassandra,neo4j,cosmosdb}` 6 个 provider Read/Write/Clear 加 DB-semconv span
- [x] **lyra 业务层**：chat turn `invoke_agent <model>` span（全链路父 span）+ `run.duration` / `run.interrupts` metrics；MCP dial（`mcp.dial_servers`）、直调 tool（`execute_tool`）span；session/run 生命周期由 RPC server span + turn span 覆盖（**不撒 slog**——见 §1.2 P6）

### 8.3 待动工

- [ ] vectorstore + chathistory 的端到端 span 行为单测（chat tracing_test + lyra chat tracing_test 已覆盖各自一例）

### 8.4 不做的事

- ❌ 不写 `observation.Registry` 接口
- ❌ 不自己写 `Convention` / `Adapter` 抽象
- ❌ 不提供 Prometheus metrics exporter（生产用户自己配 OTel metric SDK + prom exporter）
- ⚠️ `otel/` 外部 module **是有意保留的**（dev 态 span/metric/log → slog 的 sink，重 SDK 依赖外挂、不污染 `core/`）——别和"不在 core 自造抽象"搞混

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

> Go 世界里 OTel 已经是共识，再加一层是负资产。Lynx 的 observability 故事就是「core/库 直接 import OTel API 包发 Traces+Metrics + `otel/` module 把三驾马车 sink 到 slog（含关联 log handler）+ lyra startup 一次绑全局 provider、业务事件直接 `slog.InfoContext` + 生产用户自配 OTel SDK」。接收侧与发射侧均已就绪、attr key 全部去品牌（semconv 优先），trace_id 从 HTTP 入口生成、全链路串联。
