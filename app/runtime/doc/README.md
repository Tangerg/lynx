# lyra/doc — 文档索引

> lyra 模块的架构基准与配套说明。**模块级上下文（设计原则 / 分层 / Go idiom / 协议约定）以 [`../CLAUDE.md`](../CLAUDE.md) 为准**；本目录是它的展开与佐证。
>
> **组织约定**：平铺、不分子目录（互引用 `doc/XXX.md` / 裸 `XXX.md` 路径）。新增文档归类 + 在此加一行。

---

## A. 架构基准

| 文档 | 一句话 |
|---|---|
| [EXECUTION_CENTERED_ARCHITECTURE.md](EXECUTION_CENTERED_ARCHITECTURE.md) | ★ **唯一架构基准**：以 Run 生命周期（而非 agent loop）为中心的 Clean Arch —— Domain / Application / Adapter / Delivery / Bootstrap 五环 + 事件三层 + 事务 / 并发 / 生命周期语义 + 完成判据。主体已由 8 批重写落地，依赖规则由 `internal/arch` 机器强制，剩余语义收敛由执行计划跟踪。 |
| [EXECUTION_ARCHITECTURE_CONVERGENCE_PLAN.md](EXECUTION_ARCHITECTURE_CONVERGENCE_PLAN.md) | **当前执行控制面**：记录 Execution-centered 架构剩余差异、四批收敛顺序、进度、门禁、风险与决策日志，防止长任务偏离目标。 |
| [EXTENSIBILITY.md](EXTENSIBILITY.md) | 可替换性边界：外部 SPI（memory / 压缩 / LLM / RAG…可换）vs 内部焊死（核心强耦合）+ nil-default 注入方式 |

## B. 外部对照

| 文档 | 一句话 |
|---|---|
| [PRIOR_ART.md](PRIOR_ART.md) | 业界横向体检：lyra vs 16 个流行 AI agent 的上下文 / 沙箱 / 工具 / 多代理做法对照 + backlog（该做 / 取向-等触发 / 别做-仪式） |

---

> 协议契约（wire shape）在前端模块：[`../../desktop/docs/protocol/`](../../desktop/docs/protocol/)（API / TRANSPORT / AUX_API）。改协议契约先在那里对一遍。
