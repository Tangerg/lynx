# Lyra

**A general-purpose agent runtime — product-grade, transport-agnostic, deployable as either a local process or a remote service.**

> Status: **skeleton (M0)**. See [`doc/ARCHITECTURE.md`](./doc/ARCHITECTURE.md) for the CS-architecture design and [`doc/ROADMAP.md`](./doc/ROADMAP.md) for milestones.

---

## 这是什么

Lyra 是基于 [lynx-agent](../agent) framework 构建的**通用 agent 服务运行时**，采用 **client-server 架构**。Server 端跑业务，client（TUI / Web / Desktop）在独立 repo 实现，通过多 transport 接入。

```
┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐
│   TUI    │  │   Web    │  │ Desktop  │  │   MCP    │   ← clients
└────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘    (separate
     │ IPC         │ HTTP+SSE   │ gRPC        │ MCP        repos)
     └─────────────┴─────────────┴────────────┘
                         │
                         ▼
              ┌──────────────────┐
              │   Lyra Server    │   ← this repo
              │   (this module)  │
              └──────────────────┘
                         │ depends on
                         ▼
              lynx-agent + lynx-core
```

## 设计哲学

- ✅ **产品级**：不是 framework，不是 SDK，是 long-running server runtime
- ✅ **transport-agnostic**：HTTP+SSE / gRPC / IPC stdio / MCP 四个 transport 共享同一份 service 接口
- ✅ **可本地可远端**：embedded（stdio）/ local daemon / network service 三种部署模式
- ✅ **基于 lynx**：所有 framework 能力来自 lynx-agent，Lyra 只做产品级集成
- ❌ **不写 client**：TUI / Web / Desktop 在独立 repo，通过 proto IDL 消费
- ❌ **不是 Go SDK**：业务代码全在 `internal/`，外部不可见
- ❌ **不向 lynx 反向贡献抽象**（除非沉淀过 3+ 用例）

## 当前状态（M0）

- `cmd/lyra/main.go` — skeleton 入口
- `doc/ARCHITECTURE.md` — CS 架构完整设计
- `doc/ROADMAP.md` — 12 个 milestone（M0→M12，~4.5 个月 v0.1）

## Quick check

```bash
cd /Users/tangerg/Desktop/lynx
go build -o /tmp/lyra-bin ./lyra/cmd/lyra && /tmp/lyra-bin
# → lyra: skeleton — see lyra/doc/ARCHITECTURE.md
```

## 下一步

按 [`doc/ROADMAP.md`](./doc/ROADMAP.md) 进 **M1 Protocol Contract**：先冻结 6 个 service 的 proto，让前端 repo 能并行开发。
