# Agent 未发布变更

## Condition evaluation

- named Condition 改为 Planner-driven 按需观察，不再由 Runtime 每 tick 全量执行；
- `Condition.Cost` 现在决定 evaluator 解析顺序，便宜 mismatch 可短路昂贵观察；
- `And` 与 `Or` 先执行成本更低的 operand，并保留相同成本时的声明顺序；
- 单 tick 内同名 evaluator 只执行一次，`Unknown`、error 和 panic 都走稳定、可归因的结果路径；
- 动态 Cost/Value 通过 `Domain.ResolvedState` 读取已观察条件，且不会改写不可变搜索状态或 action effect；
- `planning.State.Apply` 保留 observation timestamp，模拟 effect 不再伪造新的观察时刻；
- GOAP、HTN、Reactive 和 Utility Planner 统一使用 Domain 的 condition-resolution contract；
- `NewCondition` breaking 改为 `ConditionConfig{Name, Cost, Evaluate}`，没有兼容层。

## Contract correction

- `ClearWorkingState` GoDoc 与实际 Blackboard 语义对齐：清除全部工作状态，不再声称存在并保留
  protected ambient entries。

这些变化尚未发布，也不代表已创建 tag 或 release。
