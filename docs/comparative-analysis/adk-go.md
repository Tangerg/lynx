# Google ADK Go：会话驱动的 Agent 与 Workflow 图

证据基线：`adk-go` 提交 `0da17d5183cc7affd4bdb7b4075f9e264bb598be`。

## 框架层判断

当前 ADK Go 不能只概括为 Agent 树和 Sequential/Parallel/Loop。它同时拥有一套独立 `workflow` 图运行时：Node/Edge、静态与动态调度、并发控制、JSON Schema 校验、HITL、持久 RunState 和 resume 都已进入源码与测试。

因此，它的框架中心是两层并存：

- Agent、InvocationContext、Session 和事件流构成会话驱动的 Agent 运行时。
- Workflow scheduler 将 Agent、函数、工具、动态节点和子 Workflow 组织成可暂停、可恢复的图。

CLI、Web 界面和服务端外壳仍属于应用/开发工具层，不进入框架评价。

## 可复核证据

- `agent/agent.go`：Agent 由框架控制构造，通过 `iter.Seq2` 产生 Session Event。
- `internal/context/invocation_context.go`：InvocationContext 集中承载本次调用的 Agent、Session 和服务上下文。
- `workflow/workflow.go`：`Node`、`Edge`、Route、Workflow 和 scheduler 入口；节点输入输出可声明 JSON Schema。
- `workflow/scheduler.go`：单消费者拥有状态提交，节点任务可并发执行，并支持最大并发和 pending queue。
- `workflow/state.go`：RunState/NodeState 保存 status、input、output、branch、interrupt、attempt 和 resume inputs。
- `workflow/persistence.go`、`resume.go`：从 Session history 重建暂停状态，校验响应并恢复等待节点。
- `workflow/request_input.go`：HITL 请求使用稳定 InterruptID 和 response schema。
- `agent/workflowagents/`：Sequential、Parallel、Loop Agent 仍与新 Workflow 表面并存。
- `plugin/plugin.go`：run、message 和 event 等生命周期插件。

## 恢复能力的准确边界

ADK Workflow 的恢复能力是真实实现，不应降格为普通聊天记录：

- RunState 可写入 `session.State`。
- NodeRunning 在进程重启后可被重新调度。
- HITL pause 可通过 Session Event 历史重建。
- Resume 按 InterruptID 匹配响应，并避免重复消费已完成 handoff。
- 节点 input/output 和 response 可通过 schema 校验。

它与 Scope 仍有两个本质差异：

1. 节点、Agent 和 Tool 的外部调用直接发生，没有统一的 EffectID、pending/settled 边界和 replay contract。
2. `workflow.New` 当前源码明确保留 graph fingerprint TODO；同名 Workflow 在图演化后恢复，尚缺对历史 RunState 与当前拓扑的身份校验。

所以 ADK 已经具备图级暂停恢复，但不能直接推导出任意外部副作用都能精确重放。

## 八维对照

| 维度 | ADK Go 的实际取舍 | 与 Scope 的关键差异 |
| --- | --- | --- |
| 协议边界 | 统一 Agent/模型/工具/Session 表面，Google 生态集成自然 | Scope 更强调供应商中立的独立核心 |
| 最小契约 | 密封 Agent；Workflow Node 契约较宽并允许 schema | Scope Definition/Execution 更窄，但从入口冻结恢复协议 |
| 状态所有权 | Session、InvocationContext、Workflow RunState 协作 | Scope 分开 Host 产品数据、ExecutionState 和 tree state |
| 副作用 | Agent、节点和工具运行时直接调用 | Scope Step 不直接 I/O，Effect 有稳定身份 |
| 编排 | Workflow graph + legacy Workflow Agents | Scope Workflow 只表达有独立 Process 的闭合 Stage |
| 恢复 | Session 持久 RunState、历史重建、HITL resume | Scope 进一步保存 Effect phase、完整 Process tree 与 exact DeploymentRef |
| 扩展 | callback/plugin 和 Session Event | Scope 倾向中性 middleware/listener，OTel 为独立模块 |
| 依赖 | 框架、服务和 Google 协议协同较深 | Scope 叶子适配器隔离更彻底 |

## Scope 应该借鉴什么

1. **Workflow 构建期与运行期 schema 校验。** 输入输出契约不应只停留在文档。
2. **HITL 响应协议。** InterruptID、response schema、重复 resume 和 stale response 都有明确错误语义。
3. **并发状态单写者。** 节点并发执行、scheduler 单点提交状态，和 Scope tree owner 的原则相通。
4. **从事件历史重建暂停状态。** 即使 canonical snapshot 是恢复真相，事件仍可承担诊断和恢复交叉验证。
5. **图运行的人体工学。** 函数、工具、Agent 和子 Workflow 都能作为 Node，普通图组合比每节点 Process 更轻。

## Scope 不应照搬什么

- 不应让所有服务和应用状态重新集中到宽 InvocationContext。
- 不应并存两套语义重叠、长期都稳定的 Workflow API；ADK 当前并存本身也是迁移和认知成本。
- 不应允许 durable topology 缺少 exact definition/graph identity；Scope 的 DeploymentRef 与 snapshot 校验应继续保留。
- 不应把 Session event replay 替代 Effect 级 settlement 和幂等裁决。
- 不应为了生态便利让 Google 特定服务进入核心协议。

## 最终定位

ADK Go 已经是会话 Agent 运行时加图工作流框架，而不只是几个顺序/并行 Agent。它在图组合、HITL 和 Session 驱动 resume 上比旧稿描述得强得多；Scope 则在外部副作用身份、完整执行树和部署版本一致性上保持更严格的恢复语义。
