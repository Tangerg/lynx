# lyra/doc — 文档索引

> Runtime 模块的目标架构、实施治理和协议规范。模块级强约束以 [`../CLAUDE.md`](../CLAUDE.md) 为入口。
>
> 组织约定：文档平铺在本目录；每份文档只有一个事实 owner。禁止把目标架构复制到进度台账、把进度复制到能力台账，或把 generated contract catalog 复制到人读规范。

## A. 重构必读基线

设计、实现、评审或重构 `app/runtime` 前，按顺序完整阅读：

| 顺序 | 文档 | 唯一职责 |
|---:|---|---|
| 1 | [ARCHITECTURE.md](ARCHITECTURE.md) | 已接受的目标架构、统一语言、所有权和依赖方向 |
| 2 | [DECISIONS.md](DECISIONS.md) | 架构裁决、理由、后果和取代关系 |
| 3 | [ENGINEERING_STANDARDS.md](ENGINEERING_STANDARDS.md) | 强制实施、Go API、测试和完成标准 |
| 4 | [EXECUTION_PLAN.md](EXECUTION_PLAN.md) | 授权范围、阶段、依赖、进度和当前下一步 |
| 5 | [CAPABILITY_LEDGER.md](CAPABILITY_LEDGER.md) | 当前能力事实、retain/refactor/rewrite/remove verdict 与验收证据 |
| 6 | [CONTRACT_BASELINE.md](CONTRACT_BASELINE.md) | Protocol、storage、Agent Framework consumer 与架构边界的可比较基线 |

六份文档的 owner 不可互换。改变已接受结论必须追加或取代 ADR；完成一个批次必须更新 Execution Plan 和 Capability Ledger；contract/wire/version 变化必须更新 Contract Baseline。

## B. 协议规范

| 文档 | 唯一职责 |
|---|---|
| [API.md](API.md) | Runtime Protocol 业务语义和跨方法不变量 |
| [TRANSPORT.md](TRANSPORT.md) | HTTP/SSE 与 in-process binding、流、重放和安全边界 |
| [AUX_API.md](AUX_API.md) | VCS、MCP、审批等旁路能力语义 |
| [CONSUMER_HANDOFF.md](CONSUMER_HANDOFF.md) | 服务端 breaking contract 版本与前端/TUI/CLI 后续接线清单 |

字段、方法、错误、union 和示例的机器真相源是 [`../contract/`](../contract/)。客户端只能消费这些制品，不能反向成为 Runtime 契约作者。

## C. 子系统规范

| 文档 | 地位 |
|---|---|
| [TOOL_SYSTEM.md](TOOL_SYSTEM.md) | 当前工具体系的唯一模型工具词汇、schema、能力删除和工具专项实施事实；架构文档只引用其 Tool/Agent 边界，不复制工具目录 |

## D. 外部参考

[`inspiration/`](inspiration/) 是有日期的同类产品源码对比和能力证据，不是实施合同。外部证据只有通过新 ADR 和 Capability Ledger verdict 才能改变 Runtime 设计。
