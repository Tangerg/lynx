# lyra/doc — 文档索引

> lyra 模块的架构基准与配套说明。**模块级上下文（设计原则 / 分层 / Go idiom / 协议约定）以 [`../CLAUDE.md`](../CLAUDE.md) 为准**；本目录是它的展开与佐证。
>
> **组织约定**：平铺、不分子目录（互引用 `doc/XXX.md` / 裸 `XXX.md` 路径）。新增文档归类 + 在此加一行。

---

## A. 架构基准

| 文档 | 一句话 |
|---|---|
| [EXECUTION_CENTERED_ARCHITECTURE.md](EXECUTION_CENTERED_ARCHITECTURE.md) | ★ **唯一架构基准**：Run 生命周期中心的 Domain / Application / Adapter / Infra / Delivery / Bootstrap 边界，以及事件、事务、并发和完成定义。 |
| [AGENT_CORE_ALIGNMENT_EXECUTION_PLAN.md](AGENT_CORE_ALIGNMENT_EXECUTION_PLAN.md) | ★ **当前执行控制面**：Agent/Core 演进后的 Runtime 对齐目标、批次、门禁、进度、风险与决策记录。 |
| [EXTENSIBILITY.md](EXTENSIBILITY.md) | 当前可替换端口、内部具体类型与组合根注入规则。 |
| [EXECUTION_ARCHITECTURE_CONVERGENCE_PLAN.md](EXECUTION_ARCHITECTURE_CONVERGENCE_PLAN.md) | 已完成的历史收敛计划，仅保留批次、决策和验收证据。 |

---

## B. 能力吸纳 Backlog

| 文档 | 一句话 |
|---|---|
| [AGENTSCOPE_INSPIRED_BACKLOG.md](AGENTSCOPE_INSPIRED_BACKLOG.md) | 从 AgentScope Java 对比分析筛出的增量能力提案(压缩不拆 tool 对 / tool-result eviction / 分级压缩 / 受治理技能自著述 / bypass-immune 自否决 / 超时收养 / 沙箱 SPI 参考),含为什么·目标·落点·计划·进度与刻意不吸清单。全部提案态,待满血上下文实现。 |

---

> 协议契约（wire shape）在前端模块：[`../../desktop/docs/protocol/`](../../desktop/docs/protocol/)（API / TRANSPORT / AUX_API）。改协议契约先在那里对一遍。
