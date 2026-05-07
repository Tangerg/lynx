# agent 包优化建议

> 分析时间：2026-05-06  
> 范围：`agent/` 下核心代码、运行时、规划器、事件、DSL、示例与测试。  
> 验证：`go test ./agent/...`、`go test -race ./agent/...` 均通过。

## 1. 总体判断

`agent` 包已经具备清晰的 GOAP + Blackboard + OODA 主干，并且近期已经落地了多项结构性重构：

- `AgentProcess` 已从早期 god struct 拆出 `processState`、`processBudget`、`processSignals`。
- `Platform` 已拆出内部 `agentRegistry` 与 `processRegistry`。
- `core.Blackboard` 已拆出 `BlackboardReader` / `BlackboardWriter`。
- `event` 已按主题拆分，不再是单个大文件。
- `ProcessOptions` 与 `ActionConfig` 对暂未通电字段已补充 `TODO(future)` 说明。

因此，下一阶段的优化重点不应再是大规模拆文件，而应转向 **语义闭环、行为一致性、测试覆盖、并发安全边界、API 可用性**。

## 2. 当前度量

| 指标 | 现状 | 结论 |
|---|---:|---|
| 主代码行数 | 6473 LOC | 规模已接近需要系统测试保护 |
| 测试代码行数 | 254 LOC | 测试明显不足 |
| 测试文件 | 4 个 | 主要覆盖 core 基础类型与 runtime happy path |
| 最长文件 | `planner/goap/astar.go` 491 LOC | 算法内聚，可接受，但需要测试 |
| 运行时最大文件 | `runtime/platform.go` 403 LOC | 仍可继续拆职责，但不是最高风险 |
| race 测试 | 通过 | 现有测试样本下未发现数据竞争 |

## 3. 高优先级建议

### P0-1：补齐 HITL resume 执行闭环

**问题**

`ResumeProcess` 会将等待中的 process 状态改回 `StatusRunning`，但目前没有一个公开 API 能继续驱动同一个 process：

- `Platform.RunAgent` / `StartAgent` 总是通过 `createProcess` 创建新 process。
- `AgentProcess.run` 开头调用 `makeRunning`，只允许 `StatusNotStarted -> StatusRunning`。如果 process 已经是 `StatusRunning`，`run` 直接返回。

这会让 HITL 语义停在“响应已投递”，但缺少“继续执行原流程”的闭环。相关位置：

- `agent/runtime/platform.go`：`ResumeProcess` 注释建议重新运行，但公开方法会创建新 process。
- `agent/runtime/run.go`：`run` 对非 `StatusNotStarted` 直接返回。
- `agent/runtime/process_state.go`：`makeRunning` 只接受 `StatusNotStarted`。

**建议**

新增显式 API，而不是复用 `RunAgent`：

```go
func (p *Platform) ContinueProcess(ctx context.Context, id string) error
func (p *Platform) ContinueProcessAsync(ctx context.Context, id string) <-chan error
```

并将状态转换规则收敛为：

- `NotStarted` 可以启动。
- `Waiting` 经 `ResumeProcess` 投递响应后进入 `Running`。
- `Running` 的既有 process 可以被 `ContinueProcess` 继续驱动。
- terminal 状态拒绝继续执行并返回明确错误。

**验收标准**

- 新增 HITL 端到端测试：action 调用 `pc.AwaitInput`，process 进入 `StatusWaiting`，`ResumeProcess` 写黑板，`ContinueProcess` 完成目标。
- 并发 resume 测试：两个 goroutine 同时 resume，只有一个成功。

### P0-2：让 Budget 默认真正生效，或改名避免误导

**问题**

`ProcessOptions.ApplyDefaults` 会设置 `DefaultBudget()`，注释也说 runtime 会在 tick 边界检查 Budget。但实际 `checkEarlyTermination` 只读取 `ProcessControl.EarlyTerminationPolicy`，不会自动使用 `ProcessOptions.Budget`。

结果是：用户传零值 options 时虽然拿到了默认 budget，但除非额外配置 `BudgetPolicy`，否则 cost/token/action 限制不会生效。

**建议**

二选一：

1. 推荐：在 `ApplyDefaults` 或 runtime 组装阶段自动安装默认组合策略。

```go
if o.ProcessControl.EarlyTerminationPolicy == nil {
    o.ProcessControl.EarlyTerminationPolicy = BudgetPolicy{Budget: o.Budget}
}
```

2. 如果希望 Budget 只是配置数据，则删除“runtime checks Budget”的注释，把字段改名或文档明确为“供策略读取，不自动生效”。

**验收标准**

- `ActionLimit: 1` 的两步 agent 在第二个 tick 前被终止。
- 默认 `ProcessOptions{}` 下 action 数达到 50 会终止。
- 自定义 `EarlyTerminationPolicy` 与 `BudgetPolicy` 的组合行为有测试。

### P0-3：补齐 planner / runtime / event / HITL 测试矩阵

**问题**

当前 `go test -race ./agent/...` 通过，但测试覆盖面很窄：

- `planner/goap` 没有直接测试。
- `event` 没有 listener 顺序、panic 隔离、remove 行为测试。
- `runtime/concurrent.go` 没有并发 action、replan 合并、waiting/paused/failure 优先级测试。
- HITL 没有端到端测试。
- retry / panic recovery 没有 runtime 层测试。

**建议的最小测试集**

| 包 | 建议测试 |
|---|---|
| `planner/goap` | 最短路径、cost 优先、不可达目标、excluded actions、后处理去冗余、ctx cancel、max iterations |
| `runtime` | retry 次数、panic -> failed、replan 排除动作、concurrent mergeStatuses、child budget rollup、kill/cancel |
| `event` | listener 顺序、RemoveListener、单 listener panic 不影响其他 listener |
| `hitl` | AwaitInput -> Resume -> ContinueProcess 完整链路 |
| `core` | ActionConfig 多输入、output binding、ClearBlackboard 当前不生效的显式约束 |

**目标**

先把测试从 254 LOC 提到 1200-1800 LOC，不追求覆盖率数字，而是覆盖状态机和规划器的关键分支。

### P0-4：对“已暴露但未通电”的 API 做收敛决策

**问题**

当前有不少字段已出现在公开 API，但只作为 metadata 保存：

- `ProcessOptions.Prune`
- `Planner.Prune`
- `PlannerUtility`
- `ActionConfig.ReadOnly`
- `ActionConfig.ToolGroups`
- `ActionConfig.Trigger`
- `ActionConfig.ClearBlackboard`
- `ProcessControl.ToolDelay` / `OperationDelay`
- `Verbosity`
- `Identities`

虽然注释已有 `TODO(future)`，但这些字段出现在正式 API 中，用户会自然假设“配置后生效”。

**建议**

按字段分三类处理：

| 类别 | 字段 | 处理 |
|---|---|---|
| 近期落地 | `Prune`, `ClearBlackboard`, `ToolGroups` | 补行为和测试 |
| 明确保留 | `Verbosity`, `Identities` | 继续 metadata，但事件里透传 |
| 暂缓隐藏 | `PlannerUtility`, `Trigger`, delays | 如果未准备实现，考虑移到 experimental 或 internal config |

短期最值得落地的是：

- `Options.Prune == true` 时在 `createProcess` 调用 `planner.Prune(system)`。
- `ClearBlackboard == true` 的 action 成功后调用 `Blackboard.Clear()`，再写入输出和 `hasRun` 条件，或明确它当前不可用。
- `ActionMetadata.ToolGroups` 合并进 `ProcessContext.ResolveTools` 的默认角色集合，减少 action body 手写角色名。

## 4. 中优先级建议

### P1-1：优化 world-state determination 的性能与边界

当前 `blackboardDeterminer` 每个 tick 遍历 `KnownConditions()`，并对 type-binding / named condition / plain condition 做分支判断。这个设计清楚，但随着 action 和 condition 增多，每个 tick 会重复字符串解析。

建议在 `newBlackboardDeterminer` 阶段预编译 condition resolver：

```go
type conditionResolver func(context.Context, *core.OperationContext) core.Determination
```

将 `strings.Contains`、`ParseIOBinding`、`HasPrefix`、named condition lookup 提前做掉。收益是：

- tick 阶段少做字符串解析。
- 便于对 condition evaluation 加 timeout / tracing / panic guard。
- 更容易测试每类 condition 的行为。

### P1-2：给 Condition.Evaluate 加 panic guard 与超时策略

Action 执行已有 `ExecuteSafely` panic guard，但 Condition.Evaluate 由 observe 阶段直接调用。用户自定义 condition 如果 panic，会打断整个 tick；如果阻塞，会卡住规划。

建议：

- 在 determiner 中包装 condition evaluation。
- panic 转为 `Unknown` 并发布事件或 trace error。
- 可选支持 condition-level timeout，默认继承 tick context。

### P1-3：收紧并发 action 的黑板写入语义

`ProcessConcurrent` 会并发执行所有当前 precondition 满足的 action。`inMemoryBlackboard` 本身是并发安全的，但业务语义仍可能出现“两个 action 同时写同一 binding，最新结果由调度顺序决定”的问题。

建议增加一层轻量约束：

- 并发执行前检测同 tick action 的输出 binding 冲突。
- 默认遇到冲突时降级为 sequential，或按 plan 顺序分批执行。
- 在事件里记录降级原因，便于调试。

### P1-4：改进 planner 的可解释性

A* 规划器实现比较完整，但外部很难知道“为什么没有 plan”。

建议新增 debug result：

```go
type PlanDiagnostics struct {
    GoalName string
    Reachable bool
    MissingConditions []string
    Iterations int
    ExcludedActions []string
}
```

可以先通过 event 或 trace attributes 暴露，不一定进入核心 API。

### P1-5：治理 process registry 的生命周期

`processRegistry` 当前只注册，不清理。长生命周期服务中，完成、失败、终止的 process 会持续留在内存。

建议：

- 增加 `ProcessRetentionPolicy`：按状态、数量、TTL 清理。
- 提供 `RemoveProcess(id)` 或 `PruneProcesses(predicate)`。
- `ActiveProcesses` 可以只返回非 terminal，另提供 `Processes()` 返回全部。

## 5. 低优先级建议

### P2-1：减少 `Platform` 剩余职责

`runtime/platform.go` 仍有 403 LOC，包含 deploy、reachability check、process creation、run/start/resume/child。可以在行为稳定后继续拆：

- `deploy.go`：`Deploy` / `Undeploy` / `checkGoalsReachable`
- `process_lifecycle.go`：`createProcess` / `RunAgent` / `StartAgent` / `ContinueProcess`
- `hitl_resume.go`：`ResumeProcess`
- `child_process.go`：`CreateChildProcess`

这属于可读性优化，不是当前最大风险。

### P2-2：统一文档与代码现状

`agent/docs/REFACTOR_PLAN.md` 中多项 P0 已经完成，但文档仍以“计划”口吻描述。建议：

- 在开头加入“已落地项”状态表。
- 将未完成项迁移到本文或新的 roadmap。
- 避免后续读者误以为 `AgentProcess` / `Platform` 仍未拆分。

### P2-3：补 public API 示例

`examples` 只有 hello/blog。建议补：

- HITL confirmation 示例。
- concurrent process 示例。
- child process + budget rollup 示例。
- tool resolver 示例。

这些示例会倒逼 API 易用性，也能作为文档测试。

## 6. 建议落地顺序

1. **修 HITL resume 闭环**：这是最像真实功能缺口的问题。
2. **修 Budget 默认语义**：避免用户以为预算生效但实际不生效。
3. **补测试矩阵第一批**：planner + runtime state machine + event。
4. **落地或收敛 Prune/ClearBlackboard/ToolGroups**：减少“看起来可用但实际不生效”的 API。
5. **优化 determiner 与 condition safety**：提高 tick 稳定性。
6. **治理 process registry 生命周期**：为长期运行服务做准备。

## 7. 快速验收清单

每轮修改后建议固定跑：

```bash
go test ./agent/...
go test -race ./agent/...
go test ./...
```

对 runtime 状态机、planner、HITL、concurrent 执行的改动，应至少包含：

- terminal 状态不会重复发布完成事件。
- context cancellation 会进入 `StatusKilled`。
- replan 不会无限重复同一个 action。
- waiting process 可以 resume 并继续完成。
- concurrent action 的 failure / waiting / paused 优先级符合预期。
- budget 对 child process rollup 生效。

