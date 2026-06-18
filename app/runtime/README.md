# Lyra

**Lyra Runtime — 产品级通用 agent 运行时后端（Go）。** 实现 Lyra Runtime Protocol（JSON-RPC 2.0，MCP-inspired），经 HTTP+SSE / inprocess 两种 transport 给独立前端（Wails / Web，仓在 `/Users/tangerg/Desktop/lyra/`）用。

> 模块级上下文（设计原则 / 分层 / Go idiom / 协议约定）见 [`CLAUDE.md`](./CLAUDE.md)；架构基准见 [`doc/GREENFIELD_ARCHITECTURE.md`](./doc/GREENFIELD_ARCHITECTURE.md)；文档总目录见 [`doc/README.md`](./doc/README.md)。

---

## 这是什么

基于 [lynx-agent](../agent) 框架的 agent 服务运行时。**协议层薄、业务层厚、传输层可换**：`internal/delivery` 是 wire 契约 + 传输，`internal/kernel` 是驱动 agent loop 的微内核，`internal/domain/*` 按限界上下文切业务，`internal/infra/*` 是技术设施。客户端（前端）在独立仓,经 JSON-RPC over HTTP/SSE 消费。

## 架构（Clean Arch 同心环，依赖向内，`internal/arch` 机器强制）

```
delivery (internal/delivery)  协议契约 + HTTP+SSE / inprocess 传输 + dispatch
   ↓
kernel   (internal/kernel)    微内核：定义窄 port、驱动 agent loop、装配工具集（含 turn 用例）
   ↓
domain   (internal/domain/*)  限界上下文：session / transcript / knowledge / maintenance / codeintel / workspace / …
   ↓
infra    (internal/infra/*)   sqlite / git / lsp / mcp / a2a / exec / checkpoint
```

详见 [`doc/GREENFIELD_ARCHITECTURE.md`](./doc/GREENFIELD_ARCHITECTURE.md)。

## 能力（现状）

agent loop + 并行工具循环 · **HITL R 模型**（park-on-interrupt + resume）· plan 模式 · **LSP 代码智能**（6 操作）· 编辑安全（read-before + stale）· **fork + 影子 git 文件 checkpoint + export/import** · MCP client（+ auth 基座）· **A2A** 跨 runtime · Agent Skills · LYRA.md 长期记忆 + 提取 · model-facing todo · **多 provider × 多 model（38 provider，显式配对）** · token 触发上下文压缩 · loop detection · OTel 三驾马车 → slog。

> 与同类（claude_code / codex / OpenHands / Proma / AionUi …）的能力对比 + 缺口见 [`doc/AGENT_CAPABILITY_COMPARISON.md`](./doc/AGENT_CAPABILITY_COMPARISON.md)。

## 跑起来

```bash
cd /Users/tangerg/Desktop/lynx/lyra
go build ./... && go vet ./... && go test ./...        # 全绿
ANTHROPIC_API_KEY=xxx ./lyra serve                     # 默认 127.0.0.1:17171（匹配前端默认 base），SQLite at $LYRA_HOME/lyra.db
./lyra agents --show                                   # 看本会话能读到哪些 AGENTS.md
```

## 不做（刻意）

不写 client（前端独立仓）· 不做 stdio/gRPC transport（只 HTTP+SSE + inprocess）· 不做用户鉴权/多租户（协议层零 user 概念）· 不向 lynx 反向贡献抽象（除非沉淀过 3+ 用例）。
