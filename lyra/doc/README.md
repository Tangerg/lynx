# lyra/doc — 文档索引

> lyra 模块的设计 / 评审 / 对比文档总目录。**模块级上下文（设计原则 / 分层 / Go idiom / 协议约定）以 [`../CLAUDE.md`](../CLAUDE.md) 为准**；本目录是它的展开与佐证。
>
> **组织约定**：**平铺、不分子目录**。CLAUDE.md 与文档互引大量使用 `doc/XXX.md` / 裸 `XXX.md` 路径，挪进子目录会整体断链；分类靠本索引而非目录树。新增文档：归入下面某一类 + 在此加一行。
>
> **状态标注**：★ = 现行权威 · ⚠️ = 部分漂移（以标注的"以…为准"为准）。

---

## A. 现行架构基准（背景先读）

| 文档 | 一句话 | 状态 |
|---|---|---|
| [LAYERING.md](LAYERING.md) | 分层与单向依赖（delivery→engine→service→infra）+ 重构计划与进度 | ★ CLAUDE.md 重度引用，最权威 |
| [MICROKERNEL.md](MICROKERNEL.md) | 微内核架构：engine 作核 + 端口注入 | ★ |
| [EXTENSIBILITY.md](EXTENSIBILITY.md) | 可替换性边界：外部 SPI（memory/压缩/LLM/RAG…可换）vs 内部焊死（核心强耦合）+ 注入方式 | ★ 现行准则 |
| [ARCHITECTURE.md](ARCHITECTURE.md) | CS 架构总览 / transport-agnostic service 接口 | ⚠️ 部分漂移（仍写 gRPC/IPC transport、M0~M7；现状为 http+inprocess、v2 协议、rpc/ 重命名、单向分层 —— **以 LAYERING.md + ../CLAUDE.md 为准**） |
| [ROADMAP.md](ROADMAP.md) | 路线图 / milestone | ⚠️ 部分漂移（M0~M7 + transport phase 的 gRPC/MCP adapter 与现状不符 —— **以 ../CLAUDE.md「已做过的大重构」为准**） |

## B. 子系统设计基准

| 文档 | 一句话 |
|---|---|
| [FAT_SERVICES.md](FAT_SERVICES.md) | 领域模型改革：fat services, thin engine |
| [WORKSPACE_MODEL.md](WORKSPACE_MODEL.md) | 工程目录 / 会话的心智模型与设计基准 |
| [PACK_MODEL.md](PACK_MODEL.md) | 跨渠道的契约式扩展（Pack 模型） |
| [IM_GATEWAY.md](IM_GATEWAY.md) | 第三方 IM 接入（Slack/飞书/钉钉/微信/TG）的心智模型与基准 |

## C. 评审 / 时点快照

| 文档 | 一句话 |
|---|---|
| [STRUCTURE_REVIEW.md](STRUCTURE_REVIEW.md) | 向 DDD / 整洁架构演进的结构审视（依赖规则 / 限界上下文 / F1 用例归位）+ 执行进度 | ★ 现行结构基准 |
| [GREENFIELD_ARCHITECTURE.md](GREENFIELD_ARCHITECTURE.md) | "假设从零重写" 的架构师设计稿：单一心智模型（依赖向内）+ 微内核目录树 + 与现状的差异 | 评审稿（供作者评审，非权威） |
| [NAMING_REVIEW.md](NAMING_REVIEW.md) | `lyra/` 命名 review |
| [PROTOCOL_ALIGNMENT_REVIEW.md](PROTOCOL_ALIGNMENT_REVIEW.md) | 协议对齐审视（与前端 docs/API.md / TRANSPORT.md 对账） |

## D. agent 框架利用 + 外部启发

| 文档 | 一句话 |
|---|---|
| [AGENT_LEVERAGE.md](AGENT_LEVERAGE.md) | 对 `agent` 模块肌肉的利用率 + 自举（a+b=c）清单 |
| [AGENT_SDK_INSIGHTS.md](AGENT_SDK_INSIGHTS.md) | Claude Agent SDK (TS) 对 `agent` / `lyra` 的启发 |
| [AGENT_CAPABILITY_COMPARISON.md](AGENT_CAPABILITY_COMPARISON.md) | Agent 能力横向对比矩阵（claude_code/codex/opencode/cline/kimi-code/plandex/mimocode）★ 时点快照，落地后回来更新 |
| [PLANDEX_ARCHITECTURE_REVIEW.md](PLANDEX_ARCHITECTURE_REVIEW.md) | plandex 架构剖析 → 启发：Role 即配置（per-role model） |
| [MIMOCODE_ARCHITECTURE_REVIEW.md](MIMOCODE_ARCHITECTURE_REVIEW.md) | mimocode 架构剖析 → 启发：Goal + judge 停止闸（落 GoalApprover） |

---

> **变更记录**：2026-06-13 建索引；删除 `REVIEW.md`（描述 v2 迁移 / rpc 重命名 / 分层重构**之前**的死架构 —— `impl.go`/`agui`/`internal/storage`/`internal/transport`/M0~M7/pi-mono/Yolo 默认，全已不存在，留之误导）。
