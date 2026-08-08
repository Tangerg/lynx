# Lyra

**Lyra Runtime — 产品级通用 agent 运行时后端（Go）。** 实现 Lyra Runtime Protocol（JSON-RPC 2.0，MCP-inspired），通过 streamable HTTP 服务桌面、Web 与独立进程客户端，并为同进程 CLI/TUI 提供 inprocess transport。

> 模块级上下文见 [`CLAUDE.md`](./CLAUDE.md)；重构目标架构见 [`doc/ARCHITECTURE.md`](./doc/ARCHITECTURE.md)；阶段、当前事实和全部文档入口见 [`doc/README.md`](./doc/README.md)。

---

## 这是什么

以 **Run 生命周期**（而非 agent loop）为中心的 Agent 应用后端。**协议层薄、业务层厚、传输层可换**：`internal/delivery` 是 wire 边界，`internal/application/*` 驱动 Run/Session/能力生命周期，`internal/adapter/agentexec` 隔离 Agent Framework，`internal/domain/*` 按限界上下文表达产品规则，`internal/infra/*` 提供技术机制。客户端独立消费 Runtime 发布的 JSON-RPC / contract 制品，不共享服务端实现类型。

当前生产代码仍消费旧 [`agent`](../../agent)，正在依据 [`agent2`](../../agent2) Baseline 9 设计原位重构；旧实现不是新 API 的兼容规范。

## 架构（Clean Arch 同心环，依赖向内，`internal/arch` 机器强制）

```
composition (internal/{bootstrap,config}, cmd)  唯一装配与 Host 生命周期 owner
delivery    (internal/delivery)      protocol / server / dispatch / transport
adapter     (internal/adapter/*)     应用能力与外部 SDK 的防腐/翻译
application (internal/application/*) Run / Session / capability use cases 与 consumer ports
infra       (internal/infra/*)       sqlite / git / lsp / mcp / a2a / exec 等技术 mechanism
domain      (internal/domain/*)      entity / value / aggregate behavior / pure domain policy
```

依赖一律向内（Domain 是核心）；Application 依赖 Domain 和消费方端口，Adapter/Infra 实现外部能力，Delivery 只驱动 Application，Bootstrap 是唯一组合根。详见 [`doc/ARCHITECTURE.md`](./doc/ARCHITECTURE.md)。

## 能力（现状）

Planner-driven Agent process tree · framework-managed interaction · nested child checkpoint 精确
pause/resume · HITL 审批/提问 · plan 模式 · LSP 代码智能 · read-before/stale 编辑保护 ·
worktree 与 Git checkpoint · MCP client/server bridge · A2A 远端 agent · Agent Skills ·
LYRA.md 长期知识与提取 · model-facing plan · per-run provider+model 显式选择 ·
token 触发上下文压缩 · OTel trace/metric/log → slog。

## 跑起来

```bash
cd app/runtime                                         # 从仓库根进入 runtime 模块
go build ./... && go vet ./... && go test ./...        # 全绿
ANTHROPIC_API_KEY=xxx ./lyra                           # 默认 127.0.0.1:17171（匹配前端默认 base），SQLite at $LYRA_HOME/lyra.db
```

## 不做（刻意）

不写 client（各客户端独立消费协议制品）· 不做 stdio/gRPC transport（streamable HTTP + inprocess）· 不做用户鉴权/多租户（协议层零 user 概念）· 不向 lynx 反向贡献抽象（除非沉淀过 3+ 用例）。
