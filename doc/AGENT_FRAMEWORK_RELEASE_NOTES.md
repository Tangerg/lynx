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
| exported declaration | 735 |
| root façade | 50 / 50 |
| exported JSON struct | 15 |
| wire fixture | 490 行 |

- API baseline SHA-256：`13705ed7568b34e170a1177f44b716539445b179d9cfd453fb2bdc8cb9299a70`
- wire fixture SHA-256：`581d7a1542353b8cc24fe492acc23b064227bdc148662e195c286e24366ec3fb`

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
- Engine 生命周期方法统一为 context-first 单入口；删除所有无 context / `FooContext` 双轨 API。
- `Start` 与 `ContinueAsync` 立即返回 admission error，完成 channel 只表示已启动的后台执行。
- 自定义事件不再通过 `ProcessContext.Emit(any)` 注入；Action 只能消费声明过的 `ActionTools`。
- Team 从 Runtime 特例收敛为 `workflow.Team` 定义组合器，结果仍是普通 Agent。
- `workflow.Team` 现在同时合并成员的 durable-state binding，组合结果不会在 snapshot/restore 时丢失 Loop/Repeat 等成员私有 schema；重复 binding 由普通 Agent 校验统一拒绝。
- `runtime.Config.Chat` 接受 `core.ChatCapability`，不要求具体 chat client。
- `hitl.Interrupt[R]` 返回 `(R, error)`，不返回冗余 resumed bool。
- `toolloop.Runner.Run(ctx, request, resolver)` 直接接收输入；删除 `Invocation` 中间 DTO、`NewInvocation` 和其序列化错误。
- `toolloop.RunnerConfig` → `toolloop.Config`；配置错误与运行输入错误分离为 `ErrInvalidConfig` / `ErrInvalidInput`。
- `utility.GoalFirstPlanner` → `utility.GoalFirst`，`routing.ModelRankerConfig` → `routing.ModelConfig`。
- tool policy 收敛为 `toolpolicy.Once`、`Gate`、`Condition`、`WithScope`、
  `ErrAlreadyCalled` 与 `ErrLocked`；删除 package qualification 后重复的 `Tool`、
  `Only`、`Unlocked` 命名。无 scope 的 once allowance 明确归 decorator 实例所有，
  不再错误声称具有 Process 生命周期。
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
- `StuckDecision` 补齐 `Valid` / `String`，runtime 拒绝未知决策并将策略 panic
  收敛为 Process failure。`StuckReplan` 对同一 WorldState 只允许一次恢复尝试，策略未产生
  可观察进展时确定性进入 Stuck，避免无界空转；`ProcessStuck` 事件新增可选 `reason`。
- 同一 tick 边界前到达的终止请求按 scope 合并：Agent 终止稳定高于 Action 重规划，
  同级保留首个原因，消除并发到达顺序造成的控制语义漂移。
- Deployment 改为“先冻结、后统一校验、再编码身份”：内建校验和 `AgentValidator` 都读取
  最终执行快照，消除 source metadata 在校验与 digest 之间变化的 TOCTOU seam；SPI metadata
  或 validator panic 被收敛为带归因的部署错误。
- Agent/DeploymentRef 的非空版本只接受规范 `MAJOR.MINOR.PATCH` SemVer；Name/Digest
  拒绝首尾空白，不再让语义等价但文本不同的身份进入 catalog/cache key。
- GOAP、Utility、跨 goal 的 Plan 排序和 Router 统一拒绝 NaN/Inf；planning cost 还必须非负。
  ScoreFunc panic 转为可追踪错误。`ModelRanker` 不再 clamp 越界 confidence，并拒绝重复或未知
  candidate ID；自定义 Ranker 同样必须返回 `[0,1]` 内的有限 confidence。
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
- child delegation 深度由 `Config.MaxChildDepth` 与 `DefaultMaxChildDepth` 明确定义，不再埋在运行路径中的包内数字。
- `interaction.Limits.MaxModelCalls` 以 Process 子树的累计 model call ledger 约束应用级 step budget，同时保留 `MaxSteps` 的单次 interaction 语义。
- `Engine.Kill` 会取消活动 Run / Continue context，并递归终止仍存活的 child Process，避免同步委派在 root kill 后成为 orphan。
- `Engine.Kill` 即使面对已终态父进程也会继续清扫存活子树，覆盖 `StartChild` 后父进程先完成的后台任务；`Engine.Remove` 现在只接受终态 Process，活动进程返回 `ErrProcessActive`，避免从注册表移除后失去取消与回收路径。
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
- ProcessStore 直接使用 `ProcessSnapshot.Revision` 作为 expected revision CAS；成功提交固定
  持久化为下一 revision，不再传递或返回重复版本值。
- Engine/Process 在构造边界把公开 Config/ProcessOptions 投影为私有快照，调用方后续修改
  Session identity、Extensions slice 或 Guardrails 不再改变运行中语义；typed-nil capability
  和负数工具轮数在边界返回归因错误，而不是执行期 panic/延迟失败。
- `RunInSession` 改为按值接收并 clone Session，消除同一调用方指针跨并发 turn 的共享写；派生 identity 和审计时间只写 runtime-owned 值与 SessionStore。
- ModelCall、EmbeddingCall、Budget 与 interaction Limits 暴露确定的 `Validate` 合同；usage 记录返回 error，非法/非有限/溢出值在修改账本和发布事件前失败。
- `storetest.MemoryProcessStore`、`storetest.MemorySessionStore` 提供测试实现；`core` 仅保留持久化端口。后者在 Save/Load
  两侧递归快照 JSON metadata，拒绝不可持久化值且不泄漏嵌套 map/slice 别名。
- Session 成为自校验 identity：`Validate` 固定 ID/lineage/audit 不变量，`BindAgent` 只允许
  未绑定→精确 Agent 或幂等重绑；冲突通过 `ErrInvalidSession` 分类。
- 相同 Session 的并发 turn 由进程内 FIFO 协调器稳定排序，取消 waiter 会从队列移除；无效
  Session 不再进入协调器，Host 实现的 acquire/release panic 会转为可归因错误。
- ScatterGather 的并发槽位获取现在可被首个分支错误立即取消，不再继续提交排队分支；快照
  保存、自动保存和恢复实现收拢到同一文件，零散 schema helper 归入其直接调用方。
- 自动快照使用独立于请求取消的有限时收尾上下文，`SnapshotFinalizeTimeout` 控制写入上限；取消状态也会持久化，且自动快照失败不再覆盖已经确定的 Completed、Killed 等终态。
- `SessionStore` 从 Save/Load/Delete/List 收窄为 Runtime 真正消费的 Save/Load；删除与列表
  分别由可选 `SessionDeleter` / `SessionLister` 表达。
- `runtime.Config.SessionStore` 与 `ChildSessionStore` 分别拥有 root multi-turn 和 delegated
  child 生命周期；产品 lineage adapter 不再伪造 root CRUD。
- `RunInSession` 支持按 Session.AgentName 调度已部署 Agent，拒绝 Agent identity 漂移；最终
  save 脱离 request cancellation，并同时保留 run/save 双错误。
- `storetest.TestProcessStore` 与 `storetest.TestSessionStore` 是公开外部实现 contract suite；
  `providertest` 已移除。

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
