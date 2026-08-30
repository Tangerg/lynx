# Eino：类型化 Runnable 与图组合

证据基线：`eino` 提交 `0e01b2a4e3050c4027bd61f2c2e2a519aa1e237c`。

## 框架层判断

Eino 的设计中心是把模型、工具、Lambda 和图节点统一成可组合的 `Runnable[I,O]`，再通过 Graph/Workflow 构建复杂执行。它更像类型化 AI 组件编排框架，而不是 Scope 式的 durable process kernel。

旧稿把“图节点直接产生副作用”简单判成缺陷，口径不公平。对于请求级的数据流，直接执行能显著降低实现成本；只有当目标是精确重放和跨进程恢复时，缺少统一副作用身份才成为结构限制。

## 可复核证据

- `compose/runnable.go`：`Runnable[I,O]` 统一 Invoke、Stream、Collect、Transform 四种调用形态。
- `compose`：Graph、Workflow、Lambda 和状态处理构成主要编排表面。
- checkpoint/state 相关实现：图执行可保存和恢复节点状态。
- callback 体系：组件和图运行可产生统一事件。
- `schema/message.go`：共同消息模型，并支持 AgenticMessage 等 Agent 场景表达。
- 模型和工具实现主要通过扩展模块或扩展仓库提供，核心组合层与具体 provider 有一定隔离。

## 八维对照

| 维度 | Eino 的实际取舍 | 与 Scope 的关键差异 |
| --- | --- | --- |
| 协议边界 | 共同 schema 与组件接口，provider 扩展相对分离 | 两者都重视隔离；Eino 以组件类型为中心 |
| 最小契约 | Runnable 统一四种同步/流式形态 | Scope 用 Definition/Execution 表达实例和恢复 |
| 状态所有权 | 图状态、节点状态和 context 协同 | Scope 更严格区分 Host 数据与 Execution 快照 |
| 副作用 | 节点或组件执行时直接发生 | Scope 的 Step 只描述 Effect |
| 编排 | 任意图与类型化组合能力强 | Scope Workflow 词汇闭合、只管理子 Process |
| 恢复 | 图 checkpoint 与 state 序列化 | Scope 进一步统一副作用结果和执行树恢复 |
| 扩展 | callback 贯穿组件和编排 | Scope 使用 middleware/listener 并隔离 OTel |
| 依赖 | 核心与 provider 扩展边界较自然 | Scope 叶子模块更细，治理成本也更高 |

## Scope 应该借鉴什么

1. **统一同步与流式组合的类型体验。** Eino 的 Runnable 让组件替换和图接线更自然。
2. **Graph 构建期校验。** 类型、边和节点约束越早失败，运行时恢复负担越小。
3. **普通数据流保持轻量。** Scope 将此职责交给独立 `flow` 是合理的，应保持互操作，而不是重新把 DAG 塞进受管 Workflow。
4. **扩展生态与内核解耦。** provider 和组件扩展不需要进入核心仓库的稳定面。

## Scope 不应照搬什么

- 不应把任意图状态直接等同于可恢复 Process 状态。
- 不应允许节点内不可识别的外部 I/O 绕过长期任务的 Effect 语义。
- 不应为了四种调用形态扩大 `agent.Execution`；普通调用应由更低层接口承担。

## 最终定位

若问题是“如何类型安全地组合 AI 组件和图”，Eino 比 Scope 的 managed Workflow 更自然。若问题是“如何恢复一个包含已完成外部工作的执行树”，Scope 的 Effect 和 Process 语义更完整。二者解决的是相邻但不同的问题。
