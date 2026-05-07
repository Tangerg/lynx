# agent 代码细节评审

> 范围：API ergonomics、Go idiom、组件边界、命名、设计模式。  
> 口径：只记录值得改的点；能不改的默认不动。

## 总体判断

代码整体是偏 Go 的：显式依赖、接口小心放置、错误返回替代异常、`context.Context` 传递运行态、泛型用于 typed action，方向是对的。

主要问题不在“架构不清”，而在 **公共 API 面偏厚**、**部分字段未通电**、**少数命名从 embabel 心智模型迁移过来后不够 Go 化**。

## 1. API 手感

### 做得好的

- `agent.New(...).Actions(...).Goals(...).Build()` 足够直接，比反射注册更适合 Go。
- `NewAction[In, Out]` 用泛型保留输入/输出类型，比 `any` 回调更稳。
- `core.Get[T]`、`ResultOfType[T]` 是 Go 里绕开“方法不能有类型参数”的合理做法。
- `PlatformConfig`、`ProcessOptions` 用 struct 配置，比堆很多 `WithXxx` 更克制。

### 建议收紧

#### `agent` 顶层 re-export 太满

`agent/agent.go` 几乎把 core/runtime/event 全量搬到顶层。短期方便，长期会让用户不知道哪些是稳定入口，哪些是底层模型。

建议：

- 顶层只保留高频入口：`New`、`NewAction`、`GoalProducing`、`NewPlatform`、核心状态常量。
- 低频类型让用户显式 import `core` / `runtime` / `event`。

这会让 API 更有层次，也减少未来破坏性变更压力。

#### `RunAgent` 参数仍略重

当前调用形态：

```go
platform.RunAgent(ctx, a, bindings, core.ProcessOptions{})
```

可以接受，但最后一个零值 options 有轻微噪音。建议只在高频路径加一个薄封装：

```go
Run(ctx, agent, bindings)
RunWithOptions(ctx, agent, bindings, opts)
```

不要引入复杂 functional options。

## 2. Go 范式

### 基本符合

- 接口多由消费者侧定义，例如 planner/runtime 之间的边界较自然。
- 公开构造函数返回具体类型的地方是合理的，如 `NewCondition` 返回 `*ComputedCondition`。
- nil receiver 在 `ServiceProvider` 上做 no-op，符合“工具型容器”的宽容语义。
- `ProcessContext` 每次 action 新建，避免跨 goroutine 共享 mutable state，这是好的。

### 不够 Go 化的地方

#### `Process` 接口偏大

`core.Process` 同时包含：

- 只读信息：`ID`、`Status`、`Goal`、`Blackboard`
- 控制动作：`TerminateAgent`、`AwaitInput`
- 计费：`RecordUsage`、`Usage`

这让 action、condition、listener 都看到同一个大接口。建议后续按使用场景拆小接口：

```go
type ProcessView interface { ID(); Status(); Blackboard() }
type ProcessControl interface { TerminateAgent(...); AwaitInput(...) }
type UsageRecorder interface { RecordUsage(...) }
```

不一定立刻改公共 API，但内部参数可以先收窄。

#### `Builder.Version` panic 可讨论

`semver.MustParse` 的解释说得通，但 builder 是用户 API，panic 会降低可恢复性。更 Go 的形式通常是：

```go
Version(s string) *Builder
Build() (*Agent, error)
```

不过这会明显改变 API 手感。当前可以保留，至少补一个 `VersionMust` / `BuildValidated` 之类的出口再考虑迁移。

## 3. 架构边界

### 合理的边界

- `core` 放原语，`plan` 放 planner-facing model，`runtime` 放执行引擎，方向清楚。
- `runtime.AgentProcess` 拆出 state/budget/signals 后，职责已经比早期健康。
- `event.Multicast` 简单、可预测，没有过度抽象。

### 需要克制修正的边界

#### `Platform` 还是偏“门面 + 容器 + 生命周期管理器”

它目前同时负责 deploy、process 创建、运行、resume、child process、event、service/tool wiring。作为 facade 可以接受，但文件层面已经偏厚。

建议只做文件级拆分，不引入新抽象：

- `deploy.go`
- `process_lifecycle.go`
- `resume.go`
- `child_process.go`

不要再拆一堆 manager interface。

#### `Blackboard` 抽象合理，但语义重

`Set` 会同时写 named map 和 objects list；`Bind` 又做 dual-binding。这是框架核心语义，但对新用户不直观。

建议：

- 文档里把 `Set` / `Bind` / `AddObject` 的差异放到最前。
- 命名上可以考虑 `SetNamed` 替代 `Set`，但这属于破坏性变更，暂不建议马上做。

## 4. 命名

### 好的命名

- `PlanningSystem`、`WorldState`、`ActionMetadata`、`ProcessContext` 都能准确表达职责。
- `processState`、`processBudget`、`processSignals` 比把所有字段摊在 `AgentProcess` 里清楚。
- `GoalProducing[T]` 很直观。

### 值得调整的命名

#### `Determination`

语义准确，但不够 Go、也不够日常。它表达的是三值判断结果。可考虑：

- `TruthValue`
- `TriState`
- `ConditionValue`

如果已有文档大量使用 `Determination`，可以不改；这是“可读性偏好”，不是 bug。

#### `IOBinding`

偏内部模型名。对用户而言它其实是 blackboard slot / binding key。

可考虑内部继续叫 `IOBinding`，用户文档里统一称 “binding” 或 “slot”，避免解释 IO。

#### `ProcessControl`

名字像控制面，但里面目前只有 early termination 和 delay 占位。若 delay 长期不用，可以把 `EarlyTerminationPolicy` 提到 `ProcessOptions` 顶层，删除 `ProcessControl`。

## 5. 设计模式

### 使用得当

- Builder：用于 agent 组装，合适。
- Strategy：`Planner`、`EarlyTerminationPolicy`、`StuckHandler` 合理。
- Observer：`event.Multicast` 简单够用。
- Facade：`agent` 顶层包、`Platform` 都是 facade，方向对。

### 有过度风险

- 顶层 re-export facade 过宽。
- `ProcessOptions` 里保留太多 future 字段，会让 API 看起来比实际能力更完整。
- `ServiceProvider` 是 Service Locator。这里可以接受，因为框架不想绑定 DI 容器；但不应继续扩大它的职责。

## 6. 最少改动建议

优先级从高到低：

1. 收敛未通电字段：`Prune`、`ClearBlackboard`、`ToolGroups` 要么实现，要么从主路径文档里降级。
2. 给 `Process` 内部使用场景拆小接口，不急着改公开 API。
3. 缩窄 `agent` 顶层 re-export，只保留真正高频入口。
4. `Platform` 做文件级拆分，不新增 manager 抽象。
5. 给 `Blackboard` 的 `Set` / `Bind` / `AddObject` 补一段非常短的用户文档。

一句话：**保留当前架构，不做大重构；主要减公共面、补语义闭环、让名字更贴近日常使用。**

