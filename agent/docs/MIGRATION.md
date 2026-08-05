# Agent 开发期迁移

## 统一领域词汇

本轮直接删除旧名称，不提供 alias、shim 或双字段。调用方按下列语义迁移：

| 旧名称 | 新名称 | 语义 |
|---|---|---|
| `agent.AgentConfig` / `AgentDescriptor` | `agent.Config` / `Descriptor` | 根 façade 去掉 package stutter；`core.AgentConfig` / `AgentDescriptor` 保持不变 |
| `GoalConfig.Preconditions` / `Inputs` | `RequiredConditions` / `RequiredBindings` | Goal 声明要求，不再借用 Action 的前置条件和输入术语 |
| `ActionConfig.ToolGroups` | `ToolRoles` | Action 声明抽象角色，Runtime 才将角色解析为具体 `ToolGroup` |
| `ActionMetadata.RunCondition` | `SuccessCondition` | 条件表示 Action 已成功，而不是仅被调用 |
| `AgentConfig.SnapshotState` | `SnapshotBindings` | 声明允许进入 snapshot 的 binding，不暗示拥有另一份运行状态 |
| `Condition.Cost` | `EvaluationCost` | 只表示观察成本，不参与 Action/Plan 成本 |
| `ConditionKind` / `ConditionRef.Kind` | `ConditionSourceKind` / `Source` | 明确字段描述条件值的来源 |
| `KnownConditions` / `ResolvedState` | `ConditionRefs` / `StateWithResolvedConditions` | 返回值及副作用在名称中可见 |
| `Segment` | `RunHandle` | handle 拥有一次异步 run 的 join 边界 |
| `TerminalError` | `CompletionError` | 返回当前 run 的完整完成错误，不只服务 terminal 状态 |
| `Engine.Resume` / `ResumeAsync` | `Respond` / `RespondAndContinueAsync` | 提交外部响应与继续执行不再混用同一个动词 |
| `RunChildWithState` | `RunChildWithWorkingState` | 明确复制的是 Blackboard working state |
| `MaxToolRounds` / `MaxRounds` | `MaxModelCalls` | 限额实际计数模型调用 |
| `MaxConcurrentCalls` | `MaxConcurrentToolCalls` | 并发宽度只约束工具调用 |
| `ChildOptions` | `ConfigureChild` | 字段是构造 child options 的回调，不是 options 值 |
| `StopPolicy.Check` | `ShouldStop` | 返回布尔决策的方法使用判定语义 |
| `Interaction.Stream` | `OnDelta` | 字段是流式增量回调，不是 streamer 能力 |
| `ProcessContextConfig.ToolCallCancel` | `RegisterToolCallCancellation` | 字段注册一次工具调用的取消函数，并返回配对 release，不是取消动作本身 |
| `ResumeSchema` | `ResponseSchema` | schema 校验外部 response，而不是恢复动作 |
| `Advertise` / `DeferredTool` | `InitialManifest` / `ToolDeferrer` | 初始可见工具投影与延迟公开能力各自命名 |
| `HandleInterrupt` / `IsInterrupt` | `HandleSuspension` / `IsSuspended` | 统一使用 framework suspension 术语 |

Workflow 配置也使用同一动词风格：`Joiner` 改为 `Join`，`Accept` 改为 `Until`，
`AcceptableScore` 改为 `AcceptanceThreshold`，Consensus 的 `Key` / `DefaultKey` 改为
`VoteKey` / `StringKey`，`Feedback.Text` 改为 `Rationale`。

这些变化同时升级 portable wire：ProcessSnapshot v17、Suspension v7、ToolLoop Checkpoint v5，
Runtime 私有 suspension checkpoint v5，以及 deployment definition format v2。开发期旧数据直接
判为不兼容，不猜测字段、不双读旧 schema。

## Condition 按需观察

函数条件构造已直接切换为配置对象：

```go
ready := agent.NewCondition(agent.ConditionConfig{
    Name: "ready",
    EvaluationCost: 2,
    Evaluate: func(ctx context.Context, env *agent.ConditionEnv) agent.Truth {
        return agent.True
    },
})
```

旧的 `NewCondition(name, fn)` 已删除，不提供 wrapper 或 variadic 兼容层。`EvaluationCost` 是 evaluator
观察成本，不是 Action cost，也不会改变 Plan ranking。

Runtime 不再在 tick 开始时执行全部 named Condition。内置 Planner 通过
`planning.ConditionResolver` 按需解析，并通过 `Domain.Satisfies`、
`Domain.Unsatisfied`、`Domain.ApplicableActions` 按成本解析。自定义 Planner 若要获得同样语义，
应迁移到这些 Domain 入口；直接读取 `WorldState.Conditions()` 时，尚未观察的 evaluator 仍是
`core.Unknown`。`Satisfies` 可在首个 mismatch 短路，`Unsatisfied` 为生成完整差集会解析全部相关条件。
动态 score 应使用 `Domain.StateWithResolvedConditions` 的投影视图；score 依赖的 evaluator 必须同时出现在
goal、action 或 method requirement 中，否则 Planner 没有依据主动观察它。
自定义 `core.WorldState.Apply` 必须保留原 observation timestamp；effect layering 和 resolved-state
projection 都是同一观察时刻的规划模拟，不是新的 Runtime observation。

未被任何 goal、action precondition 或 HTN method 实际需要的 Condition 不再执行。依赖 evaluator
副作用的代码必须删除该副作用；Condition 是只读 predicate，不是 lifecycle hook。

`ActionConfig.ClearWorkingState` 的行为没有变化：成功后清除全部 binding、object、condition 和
hidden marker，再绑定输出。此前“保留 protected ambient entry”的注释是错误合同，现已删除。
