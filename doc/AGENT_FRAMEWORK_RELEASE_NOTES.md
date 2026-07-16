# Agent Framework 开发版变更说明

> 状态：开发期候选基线；不是已发布 release
> Module：`github.com/Tangerg/lynx/agent`
> Go：1.26.5
> 已形成开发提交；未创建 tag、push 或 release

## 概要

本轮把 `agent` 从移植导向的执行库继续收敛为框架：`runtime.Engine` 成为唯一生命周期 owner，`core` 保持稳定协议与领域语言，策略、路由、tool-loop、workflow 和外部测试契约各自位于明确 owner package。

公共 API 不以兼容旧开发版本为目标。本轮优先减少概念数量、消除 Java/Embabel 痕迹、让零值安全，并让调用形态接近 Go 标准库和成熟第三方库。

## 当前公共基线

| 项目 | 当前值 |
|---|---:|
| public package | 16 |
| exported declaration | 628 |
| root façade | 46 / 50 |
| exported JSON struct | 14 |
| wire fixture | 456 行 |

- API baseline SHA-256：`b141e3b420575e9b5f3fd9c03a3539b5bbc55ff6754c5ed60a614ceafae24a39`
- wire fixture SHA-256：`324d63d70cc6f7bb613028f7c470ba089ecbec4fcd8b1ff04fe91bfd7bb3a5ea`

这些值用于审查开发期差异，不代表已经发布稳定承诺。

## 主要变化

### Framework lifecycle

- `Platform` 收敛为 `Engine`；`AgentProcess` 收敛为 `Process`。
- Agent 定义与运行身份分离；`Deploy` 返回不可变 `Deployment`。
- durable identity 使用 `DeploymentRef{Name, Version, Digest}`。
- 同名不同定义不再隐式覆盖；活动路由切换使用 `Replace`。
- child、workflow 和 agent-as-tool 捕获同一 Engine 的 exact Deployment。

### Domain language

- `Determination` → `Truth`。
- `IOBinding` → `Binding`。
- `PlanningSystem` → `planning.Domain`。
- `ConditionWorldState` → `planning.State`。
- `GoalExport` → `GoalTool`。
- `LLMInvocation` / `Invocation` 审计记录 → `ModelCall`、`EmbeddingCall`、`ActionRun`。
- `Services` / `ServiceProvider` → typed `Dependencies`。
- `autonomy` package → `routing`。

### Go API ergonomics

- 根入口为 `NewEngine`、`MustNewEngine`、`Engine.Run`、`Engine.Start`。
- `runtime.Config.Chat` 接受 `core.ChatCapability`，不要求具体 chat client。
- `hitl.Interrupt[R]` 返回 `(R, error)`，不返回冗余 resumed bool。
- `toolloop.Runner.Run(ctx, request, resolver)` 直接接收输入；删除 `Invocation` 中间 DTO、`NewInvocation` 和其序列化错误。
- `toolloop.RunnerConfig` → `toolloop.Config`；配置错误与运行输入错误分离为 `ErrInvalidConfig` / `ErrInvalidInput`。
- `utility.GoalFirstPlanner` → `utility.GoalFirst`，`routing.ModelRankerConfig` → `routing.ModelConfig`。
- `runtime.AgentGoalTools` → `runtime.Engine.GoalToolsFor`。
- Agent durable blackboard codec、planning templates、Domain prune 与 goal-tool fan-out 改为 owner method；删除以 Agent、Domain、Engine 为首参的自由函数。
- `ToolGroupRequirement` 自己校验声明并判断权限集合，`ToolGroupInfo` 自己校验 resolver 返回的普适合同。
- ScatterGather 的中间结果类型改为包内实现细节，不再扩大公共 API。

### Execution semantics

- Action 与 ActionMiddleware 显式传递 `(ActionStatus, error)`。
- `RetryPolicy{}` 只尝试一次；多次 retry 必须声明安全性。
- `StuckDecision` 的零值是停止，显式值 `StuckReplan` 才重新规划。
- Action 运行条件统一为 `ActionRunConditionPrefix` 与 `RunCondition()`。
- process-wide candidate-action 并发删除；并发只通过隔离 workflow branch 或 child Process。

### Interaction and persistence

- `ProcessContext.Prompt`、`PromptJSON` 与 `Interact` 统一进入 framework-managed interaction。
- Human/tool pause 共用 JSON-safe Suspension；`Resume` 与 `Continue` 分离。
- ProcessSnapshot 使用 strict decoder 与 exact DeploymentRef。
- ProcessStore 使用 expected revision CAS。
- `MemoryProcessStore`、`MemorySessionStore` 提供 reference implementation。
- `storetest.TestProcessStore` 是唯一公开外部实现 contract suite；`providertest` 已移除。

### Naming and files

- 文件按领域职责命名，例如 `process_interaction.go`、`agent_task_tools.go`、`goal_tools_for_test.go`、`memory_dispatcher.go`。
- 删除或改名泛化的 `platform`、`service_provider`、`invocation`、`inmemory`、`determination`、`io_binding` 文件。
- 变量按语义使用 `engine`、`process`、`deployment`、`request`、`input`；保留 `ctx`、`err`、`ok` 和短接收者等地道 Go 惯例。

## 消费者状态

Lyra App 已迁移到 Engine、Process、ChatCapability、`Engine.GoalToolsFor`、线性 HITL 与新的 ToolLoop API。Host 只保留 prompt、provider/model 选择、pricing、approval、stream/UI projection 和 transport mapping，不复制 Framework 的 Process 状态机、usage 或 checkpoint。

## Wire 边界

受管 wire 包括：

- `ProcessSnapshot` 及 `DeploymentRef`、`ActionRunSnapshot`、`ModelCall`、`EmbeddingCall`、`TaggedValue`；
- `Session`；
- Interaction Event / Resume / Suspension；
- ToolLoop Checkpoint / Event / Pause / Resume。

当前开发期不读取旧无版本 snapshot，也不加入 dual-read。正式稳定版本发布后才按 SemVer 管理兼容。

## 已知边界

- CAS 防 lost update，不提供分布式执行 owner；多节点需要 lease/fencing。
- 外部副作用 exactly-once 仍需业务幂等键、事务或补偿。
- Snapshot 不保存 transient runtime/client/function；恢复时由 Host 重新装配。
- ToolLoop checkpoint 只保证 Framework 已提交边界内不重跑，不能替代外部服务幂等。

## 发布前门槛

当前 receiver 精修基线已通过 Agent 全量 build、vet、test、race、lint、tidy、API/wire/architecture gate，以及 App 全量常规门禁和高风险 race。正式版本仍必须在内部依赖改为精确 tag 后，以 `GOWORK=off` 复验 module DAG、完整门禁、干净 consumer 和数据迁移。本轮开发 commit 已获授权；tag、push、数据库迁移或 release 仍需要维护者另行授权。

迁移见 [`AGENT_FRAMEWORK_MIGRATION.md`](./AGENT_FRAMEWORK_MIGRATION.md)，架构审查见 [`AGENT_FRAMEWORK_ARCHITECTURE_REVIEW.md`](./AGENT_FRAMEWORK_ARCHITECTURE_REVIEW.md)，执行进度见 [`AGENT_FRAMEWORK_ARCHITECTURE_EXECUTION_PLAN.md`](./AGENT_FRAMEWORK_ARCHITECTURE_EXECUTION_PLAN.md)。
