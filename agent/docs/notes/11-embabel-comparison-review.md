# Lynx agent vs embabel-agent 对比评审

> 对比对象：`/Users/tangerg/Desktop/embabel-agent/embabel-agent-api`  
> 口径：只看核心 agent runtime / planning / blackboard / API ergonomics。  
> 结论：Lynx 不应照搬 embabel；应吸收少量已验证语义，保持 Go 化。

## 总体判断

Lynx 的方向是对的：它没有复制 Spring 注解、DI、反射扫描，而是用显式 builder、泛型 action、struct options 和 context。这比 embabel 更符合 Go。

差距主要不在“能力数量”，而在 **公共 API 的语义完整度**。embabel 的一些字段已经形成闭环；Lynx 有些字段目前只是 metadata 或 TODO。

## 1. API 设计

### Lynx 更 Go 的地方

- `agent.New(...).Actions(...).Goals(...).Build()` 比 `@Agent` / `@Action` 更显式。
- `NewAction[In, Out]` 比运行时反射解析方法签名更可控。
- `ProcessOptions{}` 作为零值配置入口是 Go 风格。
- `ReplanRequest` 用 error 表达，比 embabel 的 exception 控制流更自然。

这些都应该保留。

### embabel 更成熟的地方

embabel 将 process 创建和执行分得更清楚：

- `createAgentProcess(...)` 创建但不启动。
- `runAgentFrom(...)` 创建并运行。
- `AgentProcess.run()` 可以继续驱动已有 process。

Lynx 目前 `RunAgent` 总是创建新 process，HITL resume 后缺少继续驱动同一个 process 的公开入口。这个点值得补。

建议：

```go
CreateProcess(...)
RunAgent(...)
ContinueProcess(ctx, id)
```

不要为了对齐 embabel 引入复杂 typed ops。

## 2. ProcessOptions

embabel 的 `Budget` 能生成默认 `EarlyTerminationPolicy`：

```kotlin
processControl = ProcessControl(
    earlyTerminationPolicy = budget.earlyTerminationPolicy()
)
```

Lynx 也有 `Budget` 和 `ProcessControl`，但默认 budget 目前不会自动接入 early termination。这里 Lynx 的 API 语义弱于 embabel。

建议：让 `ProcessOptions.ApplyDefaults` 安装默认 `BudgetPolicy`，或明确把 `Budget` 改成纯数据字段。前者更符合用户预期。

## 3. Blackboard

两边 blackboard 心智模型一致：

- named binding
- ordered objects
- `it`
- `lastResult`
- hidden objects
- protected bindings
- child spawn

Lynx 的 `BlackboardReader` / `BlackboardWriter` 拆分比 embabel 更克制，也更 Go。

但 embabel 的 blackboard 语义更完整：

- `clear()` 已被 action state transition 使用。
- `clearBlackboard` 会影响 hasRun 写入逻辑。
- 类型匹配包含父类/子类/aggregation 等 richer semantics。

Lynx 不需要复制 aggregation 和 JVM 类型体系，但应补齐 `ClearBlackboard` 的实际行为，或暂时从主 API 降级。

## 4. Action 语义

Lynx 与 embabel 的 action metadata 基本对齐：

- pre/post
- canRerun
- readOnly
- clearBlackboard
- outputBinding
- cost/value
- trigger
- qos/toolGroups

问题是 embabel 中这些字段多数有运行时消费路径；Lynx 中部分字段还只是保留。

优先补三个：

1. `ClearBlackboard`
2. `ToolGroups`
3. `Trigger`

`ReadOnly` 可以继续只做 metadata。不要过早做 planner reorder 优化。

## 5. Process 接口

embabel 的 `AgentProcess` 很大，甚至直接扩展 `Blackboard`。这在 Kotlin/Spring 生态里方便，但不是 Go 的好方向。

Lynx 当前 `core.Process` 已比 embabel 窄，但仍偏大：读状态、控制、await、usage 都在一个接口里。

建议不要学 embabel 的大接口。可以内部拆小接口：

- `ProcessView`
- `ProcessController`
- `UsageRecorder`

公开 API 可以暂时不动。

## 6. Platform 架构

embabel 的 `DefaultAgentPlatform` 依赖 Spring，职责很多：agent registry、process repository、context repository、event listener、LLM ops、tool resolver、scheduler、blackboard provider。

Lynx 的 `Platform` 更轻，这是优势。

但 embabel 有两个值得借的边界：

- `AgentProcessRepository`
- `BlackboardProvider`

Lynx 目前 process registry 是内置 map，只增不清。短期可以不抽 repository，但应至少提供删除或 retention policy。`BlackboardProvider` 则可以暂缓，除非要支持持久化 blackboard。

## 7. Planner

Lynx 的 A* 基本复刻了 embabel 的核心算法：

- reachability pre-check
- precondition count heuristic
- specificity-first action expansion
- backward + forward optimization
- excluded action 防 replan loop

差异上，Lynx 更 Go、更可测试；embabel 的 planner 与 `WorldStateDeterminer` 关系更重。

建议 Lynx 保持当前结构，只补 planner 诊断信息：不可达时返回缺失条件，而不是只有 nil plan。

## 8. 并发执行

embabel concurrent process 会并发执行当前 plan 中所有 achievable actions，并对 replan 只处理第一个请求。

Lynx 也做了类似事情，但应更谨慎：Go 里并发写 blackboard 虽然有锁，业务语义仍可能不确定。

建议 Lynx 增加“同 tick 输出 binding 冲突检测”。冲突时降级 sequential。这个比照搬 embabel 更稳。

## 9. 命名

Lynx 大多数命名和 embabel 对齐，有利于迁移文档：

- `Action`
- `Goal`
- `WorldState`
- `PlanningSystem`
- `Blackboard`
- `IOBinding`

但 Go 用户不一定熟 embabel。几个名字可以软调整：

- `Determination`：可考虑文档中称为 tri-state value。
- `IOBinding`：文档中称 binding/slot，少强调 IO。
- `ProcessControl`：如果只剩 early termination，名字偏重。

不建议为了命名做破坏性改动。

## 最少改动建议

按收益排序：

1. 补 `ContinueProcess`，闭合 HITL resume。
2. 让 `Budget` 默认接入 `BudgetPolicy`。
3. 实现或降级 `ClearBlackboard` / `ToolGroups` / `Trigger`。
4. 给 process registry 增加清理能力。
5. 给 planner stuck/no-plan 增加诊断信息。

不建议做：

- 不要引入注解式扫描。
- 不要引入 Spring 式 DI 容器。
- 不要把 `AgentProcess` 做成 embabel 那样的大接口。
- 不要为了“对齐 embabel”扩大顶层 re-export。

一句话：**Lynx 应保留 Go 化骨架，只补 embabel 已证明必要的运行时语义。**

