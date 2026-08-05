# Agent Framework 架构决策记录

> 状态：持续维护
> 建立日期：2026-08-06
> 最后更新：2026-08-06

本文只记录影响长期结构的架构决策及其理由，不复述目标架构，不记录任务进度。

- 目标设计见 [`ARCHITECTURE.md`](ARCHITECTURE.md)。
- 实施进度见 [`EXECUTION_PLAN.md`](EXECUTION_PLAN.md)。

改变已接受决策时，不直接改写历史结论。应追加新的 ADR，注明被取代的旧 ADR，并同步更新目标架构。

---

## ADR-A2-001：采用平行模块绿色重构

- 状态：已接受。
- 决策：在 `agent2` 独立实施，不在旧 `agent` 上继续根本性修补。
- 原因：隔离架构验证与应用消费迁移，消除历史模型对新执行窄腰的约束。
- 后果：开发期间允许有限的实现重复，但禁止新旧模块互相依赖。

## ADR-A2-002：`agent2` 是临时路径，不是永久版本概念

- 状态：已接受。
- 决策：最终删除旧模块，并把新模块目录和 module path 改回唯一的 `agent`。
- 原因：一个领域只保留一个术语和一套公共 API。

## ADR-A2-003：直接 AI 能力保持独立

- 状态：已接受。
- 决策：继续使用 `chatclient`、`embeddingclient` 和 `tool`，Agent 不复制 Client、Model、Message、Embedding 或 Tool 协议。
- 原因：渐进式集成的真实最小入口是基础库，不是所谓 Embedded Agent Mode。

## ADR-A2-004：共同 Process 与 Planning 解耦

- 状态：已接受。
- 决策：Goal、WorldState、Blackboard 和 Plan 不进入共同 Process/Snapshot。
- 原因：这些是 Planning 状态；放入共同抽象会迫使其他 Strategy 伪装。

## ADR-A2-005：Definition/Execution 是执行窄腰

- 状态：已接受，精确 Go 签名待 P1 验证。
- 决策：不可变 Definition 创建或恢复 per-Process Execution，Engine 只推进有界 Step。
- 原因：统一生命周期而不统一各 Strategy 的内部状态。

## ADR-A2-006：Execution Strategy 是一等替换点

- 状态：已接受。
- 决策：Interaction、Planning、Workflow 和 Supervisor 是平等 Definition，不是 Mode 枚举或 Extension。
- 原因：它们拥有不同状态推进语义，强行压入同一配置会形成 god abstraction。

## ADR-A2-007：Workflow 使用原生 Execution

- 状态：已接受。
- 决策：Workflow 不编译成 GOAP Agent。
- 原因：固定控制流与目标搜索是不同问题；原生 Workflow 才能准确表达 fork/join/gate/loop 和恢复。

## ADR-A2-008：GOAP 保留但不作为默认中心

- 状态：已接受。
- 决策：GOAP 是 Planning 下的 Planner，服务于可验证目标、多路线和动态重规划场景。
- 原因：保留 Embabel 最有价值的思想，同时不让它限制 ReAct、Workflow 和未来策略。

## ADR-A2-009：子 Agent 是 Process 关系，不是新类型

- 状态：已接受。
- 决策：统一使用 Child Process；同一 Definition 可以递归创建新的 Process。
- 原因：保持实体模型最小，并让所有 Strategy 共享组合能力。

## ADR-A2-010：递归由 Engine 调度和治理

- 状态：已接受。
- 决策：父 Process 以 waiting/resume 组合子 Process，不依赖递归 Go 调用栈；预算、权限、深度和取消由 Engine 强制。
- 原因：支持持久化恢复，并防止成本指数扩张、权限提升和失控递归。

## ADR-A2-011：Action 与 Tool 分离

- 状态：已接受。
- 决策：Action 是框架类型化操作，Tool 是模型可调用协议；通过 adapter 组合，不共享含混基类型。
- 原因：二者的消费者、描述要求和治理边界不同。

## ADR-A2-012：Framework snapshot 与 Host persistence 分层

- 状态：已接受。
- 决策：Agent 负责执行状态捕获、验证和恢复，Host 负责 Store、事务、CAS、lease 和产品 write-set。
- 原因：防止应用持久化抽象泄漏进 Framework，也防止 Host 解析 Strategy 内部状态。

## ADR-A2-013：Engine 与 Platform 分层

- 状态：已接受，精确 API 待实现验证。
- 决策：Engine 是最小 Process 执行内核；Platform 是可选的多 Deployment 目录、路由和治理容器。
- 原因：同一框架同时支持局部嵌入和完整 Agent 应用，又不把 Engine 做成 god object。

## ADR-A2-014：只保留一个主生命周期循环

- 状态：已接受。
- 决策：Engine 是唯一 Process loop；Strategy 只实现有界 Step，不各自复制 runtime、event bus 和恢复系统。
- 原因：统一生命周期是 Framework 成立的根本，重复循环会造成状态和恢复语义分叉。

## ADR-A2-015：不自动注册或默认选择执行策略

- 状态：已接受。
- 决策：Definition 显式装配所需 Strategy；Engine 不按名称自动注册 GOAP/ReAct/Workflow，也不设置普适默认 Planner。
- 原因：调用方必须清楚选择了何种控制流，避免方便性掩盖错误语义。

## ADR-A2-016：旧模块是并存期参考实现，不是新模块规范

- 状态：已接受。
- 决策：实施时直接参考旧 `agent` 的代码、测试和文档，但不建立 import、兼容或共享混合抽象。
- 原因：并存使经过生产验证的细节无需从 Git 历史恢复；同时必须避免旧结构反向决定新设计。
- 执行要求：每个能力阶段明确裁决旧实现是保留思想、重新实现还是移除，并用新合同重新验收。
