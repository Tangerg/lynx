# lyra/doc — 文档索引

> lyra 模块的设计 / 评审 / 对比文档总目录。**模块级上下文（设计原则 / 分层 / Go idiom / 协议约定）以 [`../CLAUDE.md`](../CLAUDE.md) 为准**；本目录是它的展开与佐证。
>
> **组织约定**：**平铺、不分子目录**（互引大量用 `doc/XXX.md` / 裸 `XXX.md` 路径，挪进子目录会整体断链；分类靠本索引而非目录树）。新增文档：归入下面某一类 + 在此加一行。
>
> **2026-06-14 大清理**：架构演进切片（`ARCHITECTURE` / `ROADMAP` / `LAYERING` / `MICROKERNEL` / `STRUCTURE_REVIEW`）**折叠进** [`GREENFIELD_ARCHITECTURE.md`](GREENFIELD_ARCHITECTURE.md)（升格为唯一架构基准）;`FAT_SERVICES`（微内核已实现）+ `NAMING_REVIEW`（改名已完成）+ `PLANDEX/MIMOCODE_ARCHITECTURE_REVIEW`（并入能力对比）**已删**;`CLAUDE.md` 尾部 changelog 抽成 [`REFACTORING_LOG.md`](REFACTORING_LOG.md)。历史见 git。

---

## A. 架构基准

| 文档 | 一句话 |
|---|---|
| [GREENFIELD_ARCHITECTURE.md](GREENFIELD_ARCHITECTURE.md) | ★ **唯一架构基准**：依赖向内（Clean Arch）+ 微内核（kernel 定义 port、runtime 注入）+ 限界上下文（domain）+ SPI/焊死判据 + 不做清单 + §9 执行记录（kernel/domain/delivery/turn 重命名） |
| [ARCHITECTURE_REVIEW.md](ARCHITECTURE_REVIEW.md) | 现状体检（B+→A-）：资深架构师视角逐条裁决与教科书 Clean Arch/DDD 的偏差 + DDD 该做 vs 仪式裁决 + 唯一真债（`delivery/server` 的 pump/rollback 编排应回 kernel）+ P0/P1 落地清单 |
| [EXTENSIBILITY.md](EXTENSIBILITY.md) | 可替换性边界：外部 SPI（memory / 压缩 / LLM / RAG…可换）vs 内部焊死（核心强耦合）+ nil-default 注入方式 |

## B. 子系统设计基准

| 文档 | 一句话 |
|---|---|
| [WORKSPACE_MODEL.md](WORKSPACE_MODEL.md) | 工程目录 / 会话的心智模型与设计基准 |
| [PACK_MODEL.md](PACK_MODEL.md) | 跨渠道的契约式扩展（Pack 模型） |
| [IM_GATEWAY.md](IM_GATEWAY.md) | 第三方 IM 接入（Slack/飞书/钉钉/微信/TG）的心智模型与基准（对应能力缺口：远程触达） |

## C. API / 协议

| 文档 | 一句话 |
|---|---|
| [FRONTEND_API_REVIEW.md](FRONTEND_API_REVIEW.md) | ★ 前端 API 体验评审：交互参数 vs Stripe 范式记分卡 + 同类对照 + 能力缺口 + 全协议 typed 枚举落地（§2.5） |
| [PROTOCOL_ALIGNMENT_REVIEW.md](PROTOCOL_ALIGNMENT_REVIEW.md) | 协议对齐审视（与前端 docs/API.md / TRANSPORT.md 对账；wire-shape 债修复账，已基本完成） |

## D. 能力对比 + agent 框架利用 + 外部启发

| 文档 | 一句话 |
|---|---|
| [AGENT_CAPABILITY_COMPARISON.md](AGENT_CAPABILITY_COMPARISON.md) | ★ Agent 能力横向对比（全部桌面 AI agent 应用，含 Proma / AionUi）+ lyra 缺口（**自主性 / 触达 / 多 agent** 为主线）+ 落地优先级 |
| [AGENT_LEVERAGE.md](AGENT_LEVERAGE.md) | 对 `agent` 模块肌肉的利用率 + 自举（a+b=c）清单 |
| [AGENT_SDK_INSIGHTS.md](AGENT_SDK_INSIGHTS.md) | Claude Agent SDK (TS) 对 `agent` / `lyra` 的启发 |

## E. 历史

| 文档 | 一句话 |
|---|---|
| [REFACTORING_LOG.md](REFACTORING_LOG.md) | 已落地的大重构流水账（避免重复讨论；保留各条目当时的路径名，勿据以定位当前代码） |

---

> **变更记录**：2026-06-18 新增 [`ARCHITECTURE_REVIEW.md`](ARCHITECTURE_REVIEW.md)（现状体检，A 类）。2026-06-14 大清理（折叠架构演进切片 → GREENFIELD、删 FAT_SERVICES/NAMING_REVIEW/PLANDEX/MIMOCODE_REVIEW、抽 REFACTORING_LOG、重写 AGENT_CAPABILITY_COMPARISON）。2026-06-13 建索引。
