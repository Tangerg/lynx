# Agent Framework 开发版变更说明

> 状态：开发期候选基线；不是已发布 release
> Module：`github.com/Tangerg/lynx/agent`
> Go：1.26.5
> 已形成开发提交；未创建 tag 或 release

## 概要

本轮把 `agent` 从移植导向的执行库继续收敛为框架：`runtime.Engine` 成为唯一生命周期 owner，`core` 保持稳定协议与领域语言，策略、路由、tool-loop、workflow 和外部测试契约各自位于明确 owner package。

公共 API 不以兼容旧开发版本为目标。本轮优先减少概念数量、消除 Java/Embabel 痕迹、让零值安全，并让调用形态接近 Go 标准库和成熟第三方库。

## 当前公共基线

| 项目 | 当前值 |
|---|---:|
| public package | 16 |
| exported declaration | 645 |
| root façade | 47 / 50 |
| exported JSON struct | 16 |
| wire fixture | 490 行 |

- API baseline SHA-256：`5d7c9ac2eed7f546642b65f433c6a653160e90d0e1389752e2194b02269f97c4`
- wire fixture SHA-256：`6e6ba3b76c9f4c06093984d8c897585de95e2e19b550690ec349ddd6c18b793b`

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
- 私有 owner 继续收敛：`FuncAction` 读取 typed input，`DependencyKey` 校验 typed key，`Process` 调度 middleware/chat/interaction/child listener，`runnerState` 管理 input/resume/continuation，agent tool/task tool 自己编码结果。
- Deployment snapshot、canonical definition 与 digest 归入持有 `buildID` 的私有 `deploymentCompiler`；文件改为 `deployment_compiler.go`，不增加公共 service/interface。
- `ToolGroupRequirement` 自己校验声明并判断权限集合，`ToolGroupInfo` 自己校验 resolver 返回的普适合同。
- ScatterGather 的中间结果类型改为包内实现细节，不再扩大公共 API。
- `event.ProcessSnapshotFailed` 补齐 `json.Marshaler`，现在与其余 lifecycle event 一样输出
  `{kind,timestamp,process_id,payload}` envelope；payload 明确携带 `policy` 与错误文本。

### Execution semantics

- Action 与 ActionMiddleware 显式传递 `(ActionStatus, error)`。
- `RetryPolicy{}` 只尝试一次；多次 retry 必须声明安全性。
- `StuckDecision` 的零值是停止，显式值 `StuckReplan` 才重新规划。
- Action 运行条件统一为 `ActionRunConditionPrefix` 与 `RunCondition()`。
- process-wide candidate-action 并发删除；Process tick 始终稳定执行计划首步。业务 fan-out
  通过隔离 workflow branch 或 child Process。
- ToolLoop 恢复显式、保守的并发能力：工具默认独占；实现 `ConcurrentTool` 后可按
  resource key 有界并发。`Config.MaxConcurrentCalls` 和
  `interaction.Limits.MaxConcurrentToolCalls` 控制宽度。
- 工具可以并发完成，但 ToolResult event、continuation message、checkpoint cursor
  始终按模型原始 tool-call 顺序提交，保证 history、cache key 与重放轨迹确定。
- 工具 `Call` 的 panic 在 Runner 扩展边界被收敛为 recoverable error ToolResult；
  并发 sibling 继续完成，插件 goroutine 不能击穿 Host 进程。
- `ProcessOptions.ChildOptions` 提供 Host 显式的逐 child 配置通路，并默认覆盖完整委派树；未配置时不改变 Agent 的最小继承策略。
- `interaction.Limits.MaxModelCalls` 以 Process 子树的累计 model call ledger 约束应用级 step budget，同时保留 `MaxSteps` 的单次 interaction 语义。
- `Engine.Kill` 会取消活动 Run / Continue context，并递归终止仍存活的 child Process，避免同步委派在 root kill 后成为 orphan。
- 同步 `NewAgentTool` 的每个调用拥有隔离 child Process，可在同一 model round 并发启动。
  Runtime 用 exact `ToolCall.ID` 关联同名同参数调用；多个 waiting child 按 tool-call
  顺序持久化为 child forest，恢复不会重跑已完成的 parent model/tool 边界。
  Standalone/background waiting JSON 语义保持独立。
- GOAP 删除了“未满足条件数必然 admissible”的错误假设，改为确定性 uniform-cost
  search。动态 cost 在候选 transition 的源 WorldState 上求值，保留 state-dependent
  cost 语义；ScoreFunc 必须纯且确定。为保持该合同，删除无法感知 cost 依赖的
  relevance pruning 与搜索后 action removal，原样返回最小成本 predecessor path。
  负数、NaN、Inf 以 `goap.ErrInvalidActionCost` 失败。有限非负 edge cost 下保证
  返回最小成本 plan。

### Interaction and persistence

- `ProcessContext.Prompt`、`PromptJSON` 与 `Interact` 统一进入 framework-managed interaction。
- Human/tool pause 共用 JSON-safe Suspension；`Resume` 与 `Continue` 分离。
- `Engine.Resume(parent)` 会沿 durable relation 响应当前顺序位置的 waiting child；
  `RestoreResumable` 递归恢复有序 child forest，未激活 sibling 保持 parked。
- ProcessSnapshot schema v2 使用 strict decoder 与 exact DeploymentRef，并把
  `OwnCost`、`OwnTokens`、`OwnModelCalls`、`OwnEmbeddingCalls` 定义为当前 Process
  的直接 ledger。聚合值通过恢复后的父子链接计算，不再做 child usage 后缀减法。
- ToolLoop Checkpoint schema v2 使用逐 call 的 `CallStates` 与 `NextResult`，可持久化
  同一并发批次中多个 completed/paused call，并只暴露顺序上最早的 pause；
  `MaxConcurrentCalls` 也进入 checkpoint，恢复时拒绝与原执行宽度不同的 Runner。
- durable Host 使用 `Engine.Resumable` 判断 stored continuation，并通过
  `Engine.RestoreResumable` 获得统一的 `ErrResumableSnapshotLost` 分类；Host
  不再读取或解释 `ProcessSnapshot`。
- ProcessStore 使用 expected revision CAS。
- `MemoryProcessStore`、`MemorySessionStore` 提供 reference implementation。
- `storetest.TestProcessStore` 是唯一公开外部实现 contract suite；`providertest` 已移除。

### Naming and files

- 文件按领域职责命名，例如 `process_interaction.go`、`agent_task_tools.go`、`deployment_compiler.go`、`goal_tools_for_test.go`、`memory_dispatcher.go`。
- 删除或改名泛化的 `platform`、`service_provider`、`invocation`、`inmemory`、`determination`、`io_binding` 文件。
- 变量按语义使用 `engine`、`process`、`deployment`、`request`、`input`；保留 `ctx`、`err`、`ok` 和短接收者等地道 Go 惯例。

## 消费者状态

Lyra App 已迁移到 Engine、Process、ChatCapability、`Engine.GoalToolsFor`、线性 HITL 与新的 ToolLoop API。Host 只保留 prompt、provider/model 选择、pricing、approval、stream/UI projection 和 transport mapping，不复制 Framework 的 Process 状态机、usage 或 checkpoint。

## Wire 边界

受管 wire 包括：

- `ProcessSnapshot` 及 `DeploymentRef`、`ActionRunSnapshot`、`ModelCall`、`EmbeddingCall`、`TaggedValue`；
- `Session`；
- Interaction Event / Resume / Suspension；
- ToolLoop Checkpoint / CallCheckpoint / PendingCall / Event / Pause / Resume。

当前开发期不读取 ProcessSnapshot v1、ToolLoop Checkpoint v1 或旧 private nested-child
payload，也不加入 dual-read。正式稳定版本发布后才按 SemVer 管理兼容。

## 已知边界

- CAS 防 lost update，不提供分布式执行 owner；多节点需要 lease/fencing。
- 外部副作用 exactly-once 仍需业务幂等键、事务或补偿。
- Snapshot 不保存 transient runtime/client/function；恢复时由 Host 重新装配。
- ToolLoop checkpoint 只保证 Framework 已提交边界内不重跑，不能替代外部服务幂等。
- 有序提交保证框架 history/cache 输入稳定，但并发工具对外部系统的副作用先后仍取决于
  实际执行；存在顺序依赖的工具必须保持独占或使用同一 resource key。

## 发布前门槛

本轮包含明确的开发期 breaking API/wire：ProcessSnapshot v2、ToolLoop Checkpoint v2、
`ConcurrentTool`、并发上限和 GOAP invalid-cost error。API/wire golden 只会在语义、
迁移说明和消费者测试全部审查后刷新。正式版本仍必须在内部依赖改为精确 tag 后，以
`GOWORK=off` 复验 module DAG、完整门禁、干净 consumer 和数据迁移。开发提交与 push
已获维护者授权；tag、数据库迁移或 release 仍需要维护者另行授权。

迁移见 [`AGENT_FRAMEWORK_MIGRATION.md`](./AGENT_FRAMEWORK_MIGRATION.md)，架构审查见 [`AGENT_FRAMEWORK_ARCHITECTURE_REVIEW.md`](./AGENT_FRAMEWORK_ARCHITECTURE_REVIEW.md)，执行进度见 [`AGENT_FRAMEWORK_ARCHITECTURE_EXECUTION_PLAN.md`](./AGENT_FRAMEWORK_ARCHITECTURE_EXECUTION_PLAN.md)。
