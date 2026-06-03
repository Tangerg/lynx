# `otel/` — Review 阅读顺序

`otel/` 是 OpenTelemetry 适配桥：把标准库 `log/slog` 与 lynx 内的
span/metric 输出对齐。模块极小（4 个 .go 文件），可一次扫完。

## 阅读顺序

1. `README.md` — 模块定位 + 集成方式，先看。
2. `slog/` — `slog` → OTel logs 桥。
3. `log/` — 通用 log helpers。
4. 上层调用方：
   - `core/model/chat/tracing.go` 使用的 attribute 常量
   - `agent/runtime/process_invocation_test.go` 验证 span 形状

## 关注点

- **不要在这里加 SDK retry / backoff**：单纯映射，不做策略。
- 与上层模块的耦合点只有 attribute 名（`lynx.tool.*` /
  `lynx.chat.*`），它们是 `/doc/OBSERVABILITY.md` §4 的稳定契约。
- 检查 logger 是否带 ctx，避免在 hot path 丢 ctx 信息。

## 体检命令

- `go test ./otel/...`
- `grep -n "lynx\." /Users/tangerg/Desktop/lynx/doc/OBSERVABILITY.md`
  对照属性命名约定。
