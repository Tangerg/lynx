# Agent 开发期迁移

## Condition 按需观察

函数条件构造已直接切换为配置对象：

```go
ready := agent.NewCondition(agent.ConditionConfig{
    Name: "ready",
    Cost: 2,
    Evaluate: func(ctx context.Context, env *agent.ConditionEnv) agent.Truth {
        return agent.True
    },
})
```

旧的 `NewCondition(name, fn)` 已删除，不提供 wrapper 或 variadic 兼容层。`Cost` 是 evaluator
观察成本，不是 Action cost，也不会改变 Plan ranking。

Runtime 不再在 tick 开始时执行全部 named Condition。内置 Planner 通过
`planning.ConditionResolver` 按需解析，并通过 `Domain.Satisfies`、
`Domain.Unsatisfied`、`Domain.ApplicableActions` 按成本解析。自定义 Planner 若要获得同样语义，
应迁移到这些 Domain 入口；直接读取 `WorldState.Conditions()` 时，尚未观察的 evaluator 仍是
`core.Unknown`。`Satisfies` 可在首个 mismatch 短路，`Unsatisfied` 为生成完整差集会解析全部相关条件。
动态 score 应使用 `Domain.ResolvedState` 的投影视图；score 依赖的 evaluator 必须同时出现在
goal、action 或 method requirement 中，否则 Planner 没有依据主动观察它。
自定义 `core.WorldState.Apply` 必须保留原 observation timestamp；effect layering 和 resolved-state
projection 都是同一观察时刻的规划模拟，不是新的 Runtime observation。

未被任何 goal、action precondition 或 HTN method 实际需要的 Condition 不再执行。依赖 evaluator
副作用的代码必须删除该副作用；Condition 是只读 predicate，不是 lifecycle hook。

`ActionConfig.ClearWorkingState` 的行为没有变化：成功后清除全部 binding、object、condition 和
hidden marker，再绑定输出。此前“保留 protected ambient entry”的注释是错误合同，现已删除。
