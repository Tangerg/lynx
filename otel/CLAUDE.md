# CLAUDE.md — otel module

> Lightweight OpenTelemetry span exporters for dev/debug — no OTLP backend needed.
> 项目级约定见 `../CLAUDE.md`。

---

## 一句话定位

把 OTel **三驾马车（Traces / Metrics / Logs）统一吐到 `log/slog`**：span 走 `SpanExporter`、metric 走 `MetricExporter`、应用日志走一个**上下文关联 `slog.Handler`**（从 ctx 的活跃 span 盖上 `trace_id` / `span_id`）。本机开发 / 排查 + 单进程后端（lyra startup 即把全局 OTel provider 绑到这套 sink）用，**不是生产 exporter**（生产用 OTLP / Jaeger / Tempo）。`log/` 子包是 `*log.Logger` 版的 span exporter（保留兼容）。

## 技术栈

- Go 1.26.3
- `go.opentelemetry.io/otel` 1.43 + `otel/sdk/trace`
- 零外部依赖（除了 OTel 本身）
- ~270 LOC，4 个 .go 文件

## 核心架构

- **`slog/`** — `log/slog` 三件套（**新代码 / 单进程后端用这个**）：
  - `NewExporter(logger)` → `sdktrace.SpanExporter`：每个完成的 span 一条 slog record。
  - `NewMetricExporter(logger)` → `sdkmetric.Exporter`：每个 metric 一条 slog record（配 `PeriodicReader` 用）。
  - `NewHandler(inner)` → `slog.Handler`：包一层底层 handler，从 ctx 的活跃 span 盖 `trace_id` / `span_id`——这是让**应用日志**和 span 串到同一 trace 的关键件。
- **`log/`** — 写 stdlib `*log.Logger` 的 `SpanExporter`（logfmt 单行；保留兼容）。

接口都是 OTel / stdlib 的，不是我们自造的——`ExportSpans(ctx, spans)` / `Export(ctx, metrics)` / `Handle(ctx, record)`。

**绑法**（见 `lyra/cmd/lyra/observability.go::setupObservability`）：startup 一次性把全局 `TracerProvider`（`WithSyncer(NewExporter)`）+ `MeterProvider`（`NewPeriodicReader(NewMetricExporter)`）+ `slog.SetDefault(slog.New(NewHandler(base)))` + W3C propagator 全设好；之后业务里 `otel.Tracer(...)` / `otel.Meter(...)` / `slog.InfoContext(ctx, ...)` 直接用，零 DI。

## 关键接口/类型

- `sdktrace.SpanExporter` / `sdkmetric.Exporter` / `slog.Handler` — 三个标准接口，slog/ 各实现一份
- span 输出字段：`trace_id` / `span_id` / `parent_span_id`（可选）/ `name` / `duration` / `attrs` / `status`
- 日志关联字段：`trace_id` / `span_id`（Handler 从 ctx 活跃 span 自动盖）

## 强约定

- **永远返回 nil** ——  `ExportSpans` 哪怕底层 log 写失败也不上报，本机调试场景不让 export 错误污染业务流
- **error span 升级**：span.Status.Code == ERROR → slog 走 ErrorLevel + message 前缀 `"span (error): <desc>"`
- **attribute 原样转**：attr key 直接当 slog attrs 输出，不做映射。**key 本身去品牌**——semconv 有就用（`gen_ai.*` / `db.*` / `rpc.method`），否则裸 domain（`run.*` / `agent.*` / `rag.*`），不带 `lynx.*` / `lyra.*` 前缀
- **同步 flush**：不批量缓冲，每个 span 当场写出（dev 速度优先）
- **新代码用 slog/，不用 log/**

## 关键目录

```
otel/
├── log/         stdlib *log.Logger span exporter（保留兼容）
└── slog/        三件套（推荐）：span exporter + metric exporter + 上下文关联 log handler
```

## 常用命令

```bash
go build ./...
go test ./...
```

## 修改任何东西之前

- 想加 OTLP / Jaeger / Zipkin exporter？**不在这个模块做** —— 那是生产 exporter，直接用 OTel 官方 contrib。这个包的定位就是"本机一行一个 span 看着方便"
