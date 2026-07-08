# CLAUDE.md — a2a module

> lynx 对 Agent-to-Agent (A2A) 协议的薄适配。协议 wire、AgentCard、transport 和 task 生命周期由官方 `github.com/a2aproject/a2a-go/v2` 承担。
> 项目级约定见 `../CLAUDE.md`。

## 一句话定位

根包 `a2a` 放 A2A 协议 helper 和 chat tool adapter：resolve AgentCard、打开 client、文本内容投影、server executor、远端 agent → `chat.Tool`。不再拆 `a2a/chattool`。

## 技术栈

- Go 1.26.4
- `github.com/a2aproject/a2a-go/v2`
- SDK alias：核心类型 `sdka2a`，client `a2aclient`，server `a2asrv`
- `core/model/chat`
- `go.opentelemetry.io/otel` 1.43

## 核心架构

- `Tools(ctx, endpoints...)`：批量 resolve AgentCard，包装成 tools，返回 close 函数
- `content.go`：内部 A2A 内容到 text-first lynx 语义投影
- `Agent` / `NewHTTPHandler`：把 lynx 文本流能力暴露成 A2A endpoint

## 强约定

- **单包优先，少暴露**：同一 A2A 适配域先放在 `a2a` 根包里；远端 agent 只通过 `Tools` 暴露为 `[]chat.Tool`，不公开具体 wrapper/config。
- **不重写协议状态**：不要自己写 JSON-RPC envelope、SSE、AgentCard schema。
- **A2A tool 输入统一为 `{"message": string}`**：A2A 是消息协议，不是 typed function call。
- **executor 事件序列必须合法**：submitted → working → artifact deltas → completed/failed/canceled。
- **空 chunk 跳过**：SDK task update manager 会拒绝空 artifact。

## 常用命令

```bash
go build ./...
go test ./...
```

## 修改任何东西之前

- **先看官方 a2a-go 接口形状**：本包只做薄适配。
- **不要新增 Provider/cache/Registry**：批量连接就是普通函数 `Tools`，生命周期由返回的 close 函数表达，不暴露 SDK client。
- **多轮 input-required/auth-required** 会改变 `Agent` 形状，必须单独设计。
