# CLAUDE.md — otel module

> Lightweight OpenTelemetry span exporters for dev/debug — no OTLP backend needed.
> 项目级约定见 `../CLAUDE.md`。

---

## 一句话定位

把 OTel **三驾马车（Traces / Metrics / Logs）的 dev sink 统一成 `log/slog`**：三个 exporter，每个信号一份——`SpanExporter`（span→slog）、`MetricExporter`（metric→slog）、`LogExporter`（OTel log record→slog）。关键点：**Logs 也是一等 OTel 信号**（应用经 contrib `otelslog` bridge 把 `slog` 喂进 `LoggerProvider`，再由 `LogExporter` 落地），所以 dev 用 slog、生产把每个 exporter 换成 OTLP（→ Datadog / Cloud Logging / Tempo）即可，**业务代码零改**——这就是走 OTel 而非直接写 slog 的理由（vendor-neutral 可替换）。本机开发 / 单进程后端（lyra startup 绑全局 provider）用，不是生产 exporter。

## 技术栈

- Go 1.26.3
- `go.opentelemetry.io/otel` 1.43 + `otel/sdk/{trace,metric,log}`（log SDK v0.19，配套 core 1.43）
- 零外部依赖（除了 OTel 本身）；contrib `otelslog` bridge 由消费方（lyra）引入，不在本模块

## 核心架构

单一子包 **`slog/`** —— 三个 exporter，每信号一份：

- `NewSpanExporter(logger)` → `sdktrace.SpanExporter`：每个完成的 span 一条 slog record。
- `NewMetricExporter(logger)` → `sdkmetric.Exporter`：每个 metric 一条 slog record（配 `PeriodicReader`）。
- `NewLogExporter(logger)` → `sdklog.Exporter`：每条 OTel log record 一条 slog record（body→msg、severity→level、`trace_id`/`span_id` 从 record 自带的 trace context 取，SDK 原生填，无需手动盖）。

接口都是 OTel SDK 的，不是我们自造的——`ExportSpans` / `Export(metrics)` / `Export(logs)`。**Log handler 不在本模块**：用 contrib 的 `go.opentelemetry.io/contrib/bridges/otelslog`（`slog.Handler` → `LoggerProvider`），本模块只提供它下游的 `LogExporter`。

**绑法**（见 `lyra/cmd/lyra/observability.go::setupObservability`）：startup 一次性设全局 `TracerProvider`（`WithSyncer(NewSpanExporter)`）+ `MeterProvider`（`NewPeriodicReader(NewMetricExporter)`）+ `LoggerProvider`（`NewSimpleProcessor(NewLogExporter)`）+ `slog.SetDefault(slog.New(contrib/bridges/otelslog.NewHandler(name, WithLoggerProvider(lp))))` + W3C propagator；之后 `otel.Tracer` / `otel.Meter` 直接用，零 DI。

## 关键接口/类型

- `sdktrace.SpanExporter` / `sdkmetric.Exporter` / `sdklog.Exporter` — 三个标准接口，slog/ 各实现一份
- span 输出字段：`trace_id` / `span_id` / `parent_span_id`（可选）/ `name` / `duration` / `attrs` / `status`
- log 输出字段：`trace_id` / `span_id`（record 自带）/ `scope` / body 作 msg / attrs

## 强约定

- **永远返回 nil** ——  `ExportSpans` 哪怕底层 log 写失败也不上报，本机调试场景不让 export 错误污染业务流
- **error span 升级**：span.Status.Code == ERROR → slog 走 ErrorLevel + message 前缀 `"span (error): <desc>"`
- **attribute 原样转**：attr key 直接当 slog attrs 输出，不做映射。**key 本身去品牌**——semconv 有就用（`gen_ai.*` / `db.*` / `rpc.method`），否则裸 domain（`run.*` / `agent.*` / `rag.*`），不带 `lynx.*` / `lyra.*` 前缀
- **同步 flush**：不批量缓冲，每个 span 当场写出（dev 速度优先）

## 关键目录

```
otel/
└── slog/        三个 exporter：span（exporter.go）+ metric（metric.go）+ log（logexporter.go）
```

（原 `log/`（stdlib `*log.Logger` span exporter）已删——所有信号统一走 slog。）

## 常用命令

```bash
go build ./...
go test ./...
```

## 修改任何东西之前

- 想加 OTLP / Jaeger / Zipkin exporter？**不在这个模块做** —— 那是生产 exporter，直接用 OTel 官方 contrib。这个包的定位就是"本机一行一个 span 看着方便"
