# Lyra

**A general-purpose AI coding agent — product-grade, built on the lynx-agent framework.**

> Status: **skeleton (M0)**. See [`doc/ARCHITECTURE.md`](./doc/ARCHITECTURE.md) for the design and [`doc/ROADMAP.md`](./doc/ROADMAP.md) for the milestone plan.

---

## 这是什么

Lyra 是基于 [lynx-agent](../agent) framework 构建的**通用编码 agent 产品**，对标 Claude Code / Codex / pi-coding-agent。

- ✅ **产品**：下载二进制就用，不需要写 Go 代码
- ✅ **opinionated**：内置 system prompt、tool 集、sandbox、permission、memory、UI
- ✅ **基于 lynx**：所有 framework 能力来自 lynx，Lyra 自己只做产品级集成
- ❌ **不是 framework**：用户不 `import "github.com/Tangerg/lynx/lyra"`
- ❌ **不是 SDK**：业务代码全在 `internal/`，外部不可见

## 跟 lynx 的关系

```
Lyra (product)        ── 你正在看的这个 module
   ↓
lynx-agent (framework) ── 通用 agent runtime
   ↓
lynx-core (foundation) ── chat / tool / vector / RAG / MCP / OTel
```

Lyra **只消费** lynx 的能力，不向 lynx 反向贡献抽象（除非沉淀过 3+ 用例）。

## 当前状态

- `cmd/lyra/main.go` — skeleton，打印一句话
- `doc/ARCHITECTURE.md` — 完整产品架构 + 模块拆分 + 关键决策
- `doc/ROADMAP.md` — 10 个 milestone 路线图（M0 → M10 v0.1 发布）

## Quick check

```bash
cd /Users/tangerg/Desktop/lynx
go build -o /tmp/lyra-bin ./lyra/cmd/lyra && /tmp/lyra-bin
# → lyra: skeleton — see lyra/doc/ARCHITECTURE.md
```

## 下一步

按 [`doc/ROADMAP.md`](./doc/ROADMAP.md) 进 **M1 Walking Skeleton**：单轮 chat。
