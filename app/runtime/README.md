# Lyra

**Lyra Runtime — 产品级通用 agent 运行时后端（Go）。** 实现 Lyra Runtime Protocol（JSON-RPC 2.0，MCP-inspired），经 HTTP+SSE 给前端（Wails 桌面壳 / Web，同仓独立模块 [`../desktop`](../desktop)）用；保留 inprocess transport 给未来独立 CLI/TUI 复用。

> 模块级上下文（设计原则 / 分层 / Go idiom / 协议约定）见 [`CLAUDE.md`](./CLAUDE.md)；架构基准见 [`doc/EXECUTION_CENTERED_ARCHITECTURE.md`](./doc/EXECUTION_CENTERED_ARCHITECTURE.md)；文档总目录见 [`doc/README.md`](./doc/README.md)。

---

## 这是什么

基于 [lynx-agent](../agent) 框架的 agent 服务运行时，以 **Run 生命周期**（而非 agent loop）为中心。**协议层薄、业务层厚、传输层可换**：`internal/delivery` 是 wire 契约 + 传输，`internal/application/*` 是驱动 Run/Session/能力生命周期的用例协调器，`internal/adapter/*`（含 `agentexec`）适配外部能力与 agent SDK，`internal/domain/*` 按限界上下文切业务，`internal/infra/*` 是技术设施。客户端（前端）是同仓独立模块 [`../desktop`](../desktop)（自带 go.work，不共享代码），经 JSON-RPC over HTTP/SSE 消费。

## 架构（Clean Arch 同心环，依赖向内，`internal/arch` 机器强制）

```
composition (internal/{runtime,bootstrap,config}, cmd)  装配 + host 生命周期；wires 每一环，无环 import 它
delivery    (internal/delivery)      协议契约 + HTTP+SSE / inprocess 传输 + dispatch
adapter     (internal/adapter/*)     能力适配器（含 agentexec：驱动 agent loop 的 ACL over agent SDK）
application (internal/application/*) 用例协调器：runs / sessions / capabilities / workspace / schedules
infra       (internal/infra/*)       driven adapter：sqlite / git / lsp / mcp / a2a / exec / checkpoint
domain      (internal/domain/*)      限界上下文：entities + repo ports + domain services
```

依赖一律向内（domain 是核心）；application 只依赖 domain，adapter 实现 application/domain port，delivery 驱动协调器。详见 [`doc/EXECUTION_CENTERED_ARCHITECTURE.md`](./doc/EXECUTION_CENTERED_ARCHITECTURE.md)。

## 能力（现状）

agent loop + 并行工具循环 · **HITL R 模型**（park-on-interrupt + resume）· plan 模式 · **LSP 代码智能**（6 操作）· 编辑安全（read-before + stale）· **fork + 影子 git 文件 checkpoint + export/import** · MCP client（+ auth 基座）· **A2A** 跨 runtime · Agent Skills · LYRA.md 长期记忆 + 提取 · model-facing todo · **多 provider × 多 model（38 provider，显式配对）** · token 触发上下文压缩 · loop detection · OTel 三驾马车 → slog。

## 跑起来

```bash
cd app/runtime                                         # 从仓库根进入 runtime 模块
go build ./... && go vet ./... && go test ./...        # 全绿
ANTHROPIC_API_KEY=xxx ./lyra                           # 默认 127.0.0.1:17171（匹配前端默认 base），SQLite at $LYRA_HOME/lyra.db
```

## 不做（刻意）

不写 client（前端是同仓独立模块 `../desktop`，只共仓不共代码；未来 CLI/TUI 也独立做）· 不做 stdio/gRPC transport（HTTP+SSE + inprocess）· 不做用户鉴权/多租户（协议层零 user 概念）· 不向 lynx 反向贡献抽象（除非沉淀过 3+ 用例）。
