# Microsoft Agent Framework Go：流式 Agent 与工作流执行器

证据基线：`agent-framework-go` 提交 `6aabdc7ea2d2af7ac5f673dd693a01d5e61bed35`。

## 框架层判断

Microsoft Agent Framework Go 同时提供轻量的流式 Agent 调用和更显式的 Workflow/Executor。它没有要求普通 Agent 一开始就进入重型执行模型，这一点对 Scope 的“双路径”设计很有参考价值。

本轮只比较框架包，不以示例、服务外壳或预置 Agent 数量评价内核。

## 可复核证据

- `RunFunc` 以 Go 迭代器返回流式结果，构成轻量 Agent 执行面。
- 具体 Agent 组合 client、tools、instructions 和 middleware。
- Workflow/Executor 使用节点、边、事件和运行状态组织更复杂流程。
- Checkpoint Store 提供工作流状态保存接口。
- `RequestPort` 把人机或外部请求暂停表达为工作流协议的一部分。
- `PortableValue` 等类型服务于跨执行边界的数据表示。
- middleware、context provider 与 trace context 提供扩展和上下文传播。

## 八维对照

| 维度 | Microsoft AF Go 的实际取舍 | 与 Scope 的关键差异 |
| --- | --- | --- |
| 协议边界 | 统一 Agent/client 内容模型，提供商实现同框架协作 | Scope 的 provider 叶子隔离更细 |
| 最小契约 | `RunFunc` 提供很轻的流式入口 | Scope 受管入口要求 Definition/Execution；另有直接 client 路径 |
| 状态所有权 | Agent 会话与 Workflow 状态分层 | Scope 进一步把产品状态明确留给 Host |
| 副作用 | Agent/Executor 中直接发生模型和工具调用 | Scope 通过 Effect 统一外部工作身份 |
| 编排 | Executor、节点、边和 RequestPort | Scope Workflow 聚焦受管子 Process |
| 恢复 | Checkpoint Store 与工作流状态 | Agent 内部任意 I/O 不自动获得重放语义 |
| 扩展 | middleware、context provider、trace context | Scope 同样偏中间件，但 OTel 保持独立模块 |
| 依赖 | 一套框架同时承载 Agent 与 Workflow | Scope 模块更碎、边界更纯、治理更重 |

## Scope 应该借鉴什么

1. **轻调用与工作流分层。** `RunFunc` 说明普通调用无需背负工作流概念；Scope 应继续强化 direct `chatclient` 路径。
2. **RequestPort。** 把人工输入或外部决策表达为明确暂停点，比把等待埋在任意 callback 中更可恢复。
3. **可移植值边界。** 在 checkpoint 或跨执行传值前明确检查可序列化性，能把错误提前。
4. **Go 原生流式 API。** 使用 `iter.Seq2` 表达值与错误，调用体验值得持续观察。

## Scope 不应照搬什么

- 不应让普通 Agent 直接副作用侵蚀受管 Execution 的 Effect 规则。
- 不应将 Workflow Executor 和现有 Process Runtime 合并成职责宽泛的统一容器。
- 不应假设 checkpoint store 自动解决模型、工具和外部系统的幂等提交。

## 最终定位

Microsoft Agent Framework Go 在“轻量 Agent + 显式 Workflow”之间取得了很有启发的分层。Scope 的恢复语义更严格，但需要让轻路径同样清楚；否则框架使用者会把受管执行误认为唯一入口。
