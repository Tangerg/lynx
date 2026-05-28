# CLAUDE.md — otel module

> Lightweight OpenTelemetry span exporters for dev/debug — no OTLP backend needed.
> 项目级约定见 `../lyra/CLAUDE.md`。

---

## 一句话定位

把 OTel span 直接吐到 `*log.Logger` 或 `*slog.Logger`，logfmt 单行格式。本机开发 / 排查跑 trace 用，**不是生产 exporter**（生产用 OTLP / Jaeger / Tempo）。

## 技术栈

- Go 1.26.3
- `go.opentelemetry.io/otel` 1.43 + `otel/sdk/trace`
- 零外部依赖（除了 OTel 本身）
- ~270 LOC，4 个 .go 文件

## 核心架构

两个并列子包，各实现一份 `sdktrace.SpanExporter`：

- **`log/`** — 写 stdlib `*log.Logger`（log 包；老代码路径）
- **`slog/`** — 写 stdlib `*slog.Logger`（结构化，**新代码优先用这个**）

接口是 OTel 的，不是我们的——`exporter.ExportSpans(ctx, spans)` + `Shutdown(ctx)`。

## 关键接口/类型

- `sdktrace.SpanExporter` — OTel SDK 标准接口，两个 exporter 都实现
- 单行输出字段：`trace_id` / `span_id` / `parent_span_id`（可选）/ `name` / `duration` / `attrs` / `status`

## 强约定

- **永远返回 nil** ——  `ExportSpans` 哪怕底层 log 写失败也不上报，本机调试场景不让 export 错误污染业务流
- **error span 升级**：span.Status.Code == ERROR → slog 走 ErrorLevel + message 前缀 `"span (error): <desc>"`
- **attribute 原样转**：`gen_ai.*` / `lynx.*` 这些 attr key 直接当 slog attrs 输出，不做映射
- **同步 flush**：不批量缓冲，每个 span 当场写出（dev 速度优先）
- **新代码用 slog/，不用 log/**

## 关键目录

```
otel/
├── log/         stdlib *log.Logger exporter（保留兼容）
└── slog/        *slog.Logger exporter（推荐）
```

## 常用命令

```bash
go build ./...
go test ./...
```

## 修改任何东西之前

- 想加 OTLP / Jaeger / Zipkin exporter？**不在这个模块做** —— 那是生产 exporter，直接用 OTel 官方 contrib。这个包的定位就是"本机一行一个 span 看着方便"
