# lyra/doc — 文档索引

> lyra 模块的架构基准与配套说明。**模块级上下文（设计原则 / 分层 / Go idiom / 协议约定）以 [`../CLAUDE.md`](../CLAUDE.md) 为准**；本目录是它的展开与佐证。
>
> **组织约定**：平铺、不分子目录（互引用 `doc/XXX.md` / 裸 `XXX.md` 路径）。新增文档归类 + 在此加一行。

---

## A. 架构基准

| 文档 | 一句话 |
|---|---|
| [EXECUTION_CENTERED_ARCHITECTURE.md](EXECUTION_CENTERED_ARCHITECTURE.md) | ★ **唯一架构基准**：Run 生命周期中心的 Domain / Application / Adapter / Infra / Delivery / Bootstrap 边界，以及事件、事务、并发和完成定义。 |
| [EXTENSIBILITY.md](EXTENSIBILITY.md) | 当前可替换端口、内部具体类型与组合根注入规则。 |
| [ARCHITECTURE_HYGIENE_PLAN.md](ARCHITECTURE_HYGIENE_PLAN.md) | `app/runtime` 架构卫生治理的目标、批次、验收标准与进度台账。 |
| [TOOL_SYSTEM_VNEXT.md](TOOL_SYSTEM_VNEXT.md) | 新工具体系的唯一词汇、模型契约、删除范围与分批实施台账。 |
| [API.md](API.md) | Runtime Protocol 的语义与跨方法不变量；字段和方法目录由 `contract/` 生成物拥有。 |
| [TRANSPORT.md](TRANSPORT.md) | InProcess 与 streamable HTTP 的 binding、状态码、流、重放和安全边界。 |
| [AUX_API.md](AUX_API.md) | VCS、失效流、会话恢复、MCP 生命周期与审批等旁路能力语义。 |

---

> 竞品能力吸纳的源码级对比分析在 [`inspiration/`](inspiration/)（跨应用合并 backlog + 各应用一份）。

---

> 协议的人读规范由本目录的 `API.md` / `TRANSPORT.md` / `AUX_API.md` 拥有；字段、方法、错误、联合类型和示例的机器真相源是 [`../contract/`](../contract/)。客户端只能消费这些制品，不得反向成为 Runtime 契约作者。
