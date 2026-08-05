# Agent 未发布变更

## Semantic API vocabulary

- Goal requirement、Action success、snapshot binding、condition source/evaluation cost 均使用唯一且可判别的领域术语；
- 根 façade 去除 `AgentConfig` / `AgentDescriptor` 口吃，标准入口统一为 `agent.Config` / `agent.Descriptor`；
- Runtime 的一次异步执行统一为 `RunHandle` / `RunCompletion`，外部回答统一使用 `Respond`，child working-state 复制在名称中显式表达；
- model-call 与 concurrent-tool-call 限额不再使用模糊的 round/call 名称；
- ToolLoop 初始 manifest、延迟公开、并发 batch 与 Workflow callback 字段统一为动词或精确名词；
- ProcessContext runtime SPI 以 `RegisterToolCallCancellation` 明示注册与配对释放语义；
- suspension 协议统一使用 `ResponseSchema`，不再把响应 schema 命名成 resume schema；
- 删除全部旧名称和兼容入口；完整映射见 [MIGRATION.md](./MIGRATION.md)。

Portable schema 直接升级到 ProcessSnapshot v17、Suspension v7、ToolLoop Checkpoint v5、Runtime
私有 suspension checkpoint v5；deployment canonical definition 升级为 format v2。

## Condition evaluation

- named Condition 改为 Planner-driven 按需观察，不再由 Runtime 每 tick 全量执行；
- `Condition.EvaluationCost` 现在决定 evaluator 解析顺序，便宜 mismatch 可短路昂贵观察；
- `And` 与 `Or` 先执行成本更低的 operand，并保留相同成本时的声明顺序；
- 单 tick 内同名 evaluator 只执行一次，`Unknown`、error 和 panic 都走稳定、可归因的结果路径；
- 动态 Cost/Value 通过 `Domain.StateWithResolvedConditions` 读取已观察条件，且不会改写不可变搜索状态或 action effect；
- `planning.State.Apply` 保留 observation timestamp，模拟 effect 不再伪造新的观察时刻；
- GOAP、HTN、Reactive 和 Utility Planner 统一使用 Domain 的 condition-resolution contract；
- `NewCondition` breaking 改为 `ConditionConfig{Name, EvaluationCost, Evaluate}`，没有兼容层。

## Contract correction

- `ClearWorkingState` GoDoc 与实际 Blackboard 语义对齐：清除全部工作状态，不再声称存在并保留
  protected ambient entries。

这些变化尚未发布，也不代表已创建 tag 或 release。
