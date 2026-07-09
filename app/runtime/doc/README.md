# lyra/doc — 文档索引

> lyra 模块的架构基准与现状体检。**模块级上下文（设计原则 / 分层 / Go idiom / 协议约定）以 [`../CLAUDE.md`](../CLAUDE.md) 为准**；本目录是它的展开与佐证。
>
> **组织约定**：平铺、不分子目录（互引用 `doc/XXX.md` / 裸 `XXX.md` 路径）。新增文档归类 + 在此加一行。

---

## A. 架构基准

| 文档 | 一句话 |
|---|---|
| [GREENFIELD_ARCHITECTURE.md](GREENFIELD_ARCHITECTURE.md) | ★ **唯一架构基准**：依赖向内（Clean Arch）+ 微内核（kernel 定义 port、runtime 注入）+ 限界上下文（domain）+ SPI/焊死判据 + 不做清单 + 执行记录（kernel/domain/delivery/turn 重命名） |
| [EXTENSIBILITY.md](EXTENSIBILITY.md) | 可替换性边界：外部 SPI（memory / 压缩 / LLM / RAG…可换）vs 内部焊死（核心强耦合）+ nil-default 注入方式 |

## B. 现状评审（2026-06-18，资深架构师视角）

| 文档 | 一句话 |
|---|---|
| [ARCHITECTURE_REVIEW.md](ARCHITECTURE_REVIEW.md) | 现状体检（B+→A-）：逐条裁决与教科书 Clean Arch/DDD 的偏差 + DDD 该做 vs 仪式 + 唯一真债（`delivery/server` 的 pump/rollback 编排）+ P0/P1 清单 |
| [GREENFIELD_DESIGN.md](GREENFIELD_DESIGN.md) | greenfield 重审：如果从零写 lyra 的应然设计 + 跨模块接缝（lyra 定义 `agentRuntime` 窄接口消费 agent / tool loop 归属 / 事件桥 / 持久化分工）。与 agent 侧 [`GREENFIELD_DESIGN.md`](../../../agent/docs/GREENFIELD_DESIGN.md) 配套 |
| [SECOND_BATCH_RUNTIME_WORKFLOW.md](SECOND_BATCH_RUNTIME_WORKFLOW.md) | 第二批 runtime 工作流可靠性改造的目标、非目标、计划、进度和完成判据 |

---

> 协议契约（wire shape）在前端模块：[`../../desktop/docs/protocol/`](../../desktop/docs/protocol/)（API / TRANSPORT / AUX_API）。改协议契约先在那里对一遍。
