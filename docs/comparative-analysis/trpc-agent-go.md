# tRPC-Agent-Go：宽表面的 Agent 运行时

证据基线：`trpc-agent-go` 提交 `91bde85eb243333b2b33fe89061f2218ede00c99`。

## 框架层判断

tRPC-Agent-Go 同时覆盖 Agent、Runner、Invocation、Session、Graph、模型、工具和多种扩展机制。它追求较完整的一站式 Go Agent 开发面，设计中心比 Scope 更宽。

旧稿把 OpenClaw 和内置应用能力纳入优劣判断，混淆了框架与产品。本轮只看框架契约；OpenClaw、命令行和预置工具不计分。

## 可复核证据

- `agent/invocation.go`：`Invocation` 聚合运行身份、Agent、Session、模型和运行选项。
- Agent/Runner 路径：Agent 通过 Invocation 与事件流执行。
- Graph：使用通用 state，支持节点、边和 checkpoint 相关能力。
- checkpoint 模型包含父 checkpoint 身份，可表达图历史关系。
- callback、hook、plugin 或 runner 扩展存在多条接入路径。
- 模型、工具、知识和会话能力分布在主模块与子模块中。

## 八维对照

| 维度 | tRPC-Agent-Go 的实际取舍 | 与 Scope 的关键差异 |
| --- | --- | --- |
| 协议边界 | 提供统一模型/消息/工具面，运行时覆盖较宽 | Scope 核心更小，provider 和产品能力隔离更细 |
| 最小契约 | Agent + Invocation + Runner 协作 | Scope Definition/Execution 更窄但恢复要求更强 |
| 状态所有权 | Invocation、Session、Graph State 共同持有 | Scope 的 Host/Execution 所有权更明确 |
| 副作用 | Agent、Runner 或图节点直接执行 | Scope 用 Effect 形成统一外部工作边界 |
| 编排 | Graph 与 Agent 模式并存 | Scope Workflow 只表示受管子执行 |
| 恢复 | Session 和 Graph checkpoint 较丰富 | 不代表所有 Agent 外部调用都可幂等重放 |
| 扩展 | 多套 callback/hook/plugin 表面 | 能力广，但生命周期心智模型更分散 |
| 依赖 | 一站式能力多，最小依赖面相对更宽 | Scope 可精细按叶子模块选择，维护成本更高 |

## Scope 应该借鉴什么

1. **图历史和父 checkpoint 表达。** 对分支、回溯和调试有直接价值。
2. **面向开发者的集成路径。** 宽框架能降低首次接入成本，Scope 应在不污染内核的前提下改善组合文档和发行体验。
3. **Session/Runner 使用范式。** 即使 Scope 把产品会话留给 Host，也可以让边界协作更容易被发现。

## Scope 不应照搬什么

- 不应把模型、会话、知识、工具、Runner 和产品能力重新聚合进宽 Invocation。
- 不应并存多套无法说明优先级的扩展生命周期。
- 不应把 Graph checkpoint 宣传成任意外部副作用的恢复保证。
- 不应因为对方内置应用较多，就把应用层能力搬回 Scope 仓库。

## 最终定位

tRPC-Agent-Go 更适合需要宽功能面和一站式组合的 Go Agent 项目。Scope 更适合将执行语义做成独立、可恢复内核，并让 Flame 这类应用自行组装产品能力。其差异主要是边界宽度，而不是功能清单多少。
