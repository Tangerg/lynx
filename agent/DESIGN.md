# Lynx Agent 框架设计与实现

> 一个基于 GOAP（Goal-Oriented Action Planning） + 黑板 + OODA 循环的 Go agent 框架。
> 移植自 [embabel-agent](https://github.com/embabel/embabel-agent)（Spring AI 生态的 Java/Kotlin agent 框架），但对核心机制做了 Go 化重写。

---

## 目录

1. [设计哲学](#1-设计哲学)
2. [整体架构](#2-整体架构)
3. [分层与依赖](#3-分层与依赖)
4. [核心抽象](#4-核心抽象)
5. [GOAP A\* 规划器](#5-goap-a-规划器)
6. [OODA 执行循环](#6-ooda-执行循环)
7. [并发执行模型](#7-并发执行模型)
8. [事件系统](#8-事件系统)
9. [人机协同（HITL）](#9-人机协同hitl)
10. [使用指南](#10-使用指南)
11. [扩展点](#11-扩展点)
12. [代码地图](#12-代码地图)
13. [设计权衡](#13-设计权衡)

---

## 1. 设计哲学

### 1.1 三件不动的事

无论语言、生态如何变化，框架的**心智模型**必须保留 embabel 的三大设计：

| # | 概念 | 解释 |
|---|------|------|
| **GOAP** | Goal-Oriented Action Planning | 用户声明 Action（带前提、效果、代价）和 Goal（带前提、价值），由规划器搜索一条 Action 序列以达成 Goal |
| **黑板（Blackboard）** | 类型化共享内存 | Action 之间不直接传参，而是写入/读取黑板上的对象。规划器根据黑板状态决定下一步 |
| **OODA** | Observe-Orient-Decide-Act 循环 | 每个 tick：读黑板推世界状态 → 规划 → 选择动作 → 执行 → 写回黑板 |

### 1.2 Go 化的取舍

| Java/Kotlin | Go 等价物 | 理由 |
|-------------|----------|------|
| `@Agent` / `@Action` 注解 | DSL `agent.New(...).Actions(...)` | Go 没有运行时注解；显式 builder 是惟一入口，无反射、可调试、IDE 友好 |
| `ThreadLocal AgentProcess.get()` | `core.WithProcess(ctx, p)` / `core.ProcessFrom(ctx)` | `context.Context` 是 Go 的标准透传机制 |
| `throw ReplanRequestedException` | `return &core.ReplanRequest{...}` (实现 `error`) | Go 不依赖异常做控制流；error + `errors.As` 一样表达力 |
| Spring DI `@Autowired` | 用户自己往 `core.ServiceProvider` 注册任意服务 + `core.ServiceOf[T]` 类型化取出 | 不预设 LLM / RAG / VectorStore 等典型槽位，框架不绑生态 |
| `<T> T resultOfType(Class<T>)` | `core.ResultOfType[T](proc)` | Go 1.18+ 泛型，但只能在顶层函数 |
| Reactor `Flux<T>` | `iter.Seq2[T, error]` | Go 1.23+ pull-based 迭代器 |
| Kotlin Coroutines | `goroutine` + `errgroup` | 更直接，无 schedulers/dispatchers 概念 |

### 1.3 核心原则

| # | 原则 | 含义 |
|---|------|------|
| P1 | **不继承，用组合** | 没有 `AbstractAgentProcess`；状态机方法直接挂在具体 struct 上 |
| P2 | **零反射** | 用户用显式 DSL 定义 Agent；运行时全程直调，无 reflect.Method.Func 间接 |
| P3 | **不 DI 容器** | functional options + 显式依赖注入 |
| P4 | **类型安全到底** | `NewAction[In, Out]` 把 In/Out 保留到编译期 |
| P5 | **零依赖核心** | `core/` 只用标准库 + OTel，不依赖 lynx/core，避免环依赖 |
| P6 | **观测走 OTel API 直接调用** | 不建 observation 抽象层，全程 `otel.Tracer("lynx/agent")` |

---

## 2. 整体架构

```
┌──────────────────────────────────────────────────────────────────────┐
│  用户层                                                                │
│   DSL builder（agent.New(...).Actions(...).Goals(...).Build()）        │
└──────────────────────────────────┬───────────────────────────────────┘
                                   ↓
┌──────────────────────────────────────────────────────────────────────┐
│  Platform (runtime/)                                                  │
│   ┌───────────────┐  ┌──────────────┐  ┌──────────────────────────┐  │
│   │ AgentProcess  │  │   Planner    │  │  event.Multicast         │  │
│   │ (状态机+tick) │←→│ (A* GOAP)    │  │  (15+ 种生命周期事件)    │  │
│   └───────┬───────┘  └──────────────┘  └──────────────────────────┘  │
│           ↓                                                            │
│  ┌────────────────────────────────────┐                               │
│  │ Blackboard (sync.RWMutex 保护)      │                               │
│  │  - named map[string]any            │                               │
│  │  - objects []any (按时间)           │                               │
│  │  - conditions map[string]bool       │                               │
│  └────────────────────────────────────┘                               │
└──────────────────────────────────┬───────────────────────────────────┘
                                   ↓
┌──────────────────────────────────────────────────────────────────────┐
│  Action 调用环境（每 tick 重建一次 ProcessContext）                    │
│   ┌──────────────────────────────────────────────────────────────┐   │
│   │ ProcessContext { Process, Blackboard, Services,              │   │
│   │                   publishEvent, resolveTools }                │   │
│   └──────────────────────────────────────────────────────────────┘   │
└──────────────────────────────────┬───────────────────────────────────┘
                                   ↓
┌──────────────────────────────────────────────────────────────────────┐
│  外部集成（可选注入）                                                  │
│   用户注册到 ServiceProvider 的任意服务 + ToolGroupResolver /         │
│   OTel TracerProvider                                                 │
└──────────────────────────────────────────────────────────────────────┘
```

**核心信息流（一个 tick 的生命周期）**：

```
Tick 开始
  │
  ├─ 1. 检查终止信号（TerminationScopeAgent / Action）
  │
  ├─ 2. OBSERVE: 内部 worldStateDeterminer 扫描黑板 + 评估命名 Condition
  │      → 产出 ConditionWorldState (key → True/False/Unknown 的不可变快照)
  │
  ├─ 3. ORIENT: Planner.BestValuePlan(start, system, opts)
  │      → A* 搜索最高 NetValue 的 Plan
  │
  ├─ 4. DECIDE:
  │      Simple    → 取 plan.Actions[0]
  │      Concurrent → filter 出当前可达的 actions，errgroup 并发
  │
  ├─ 5. ACT: executeAction
  │      ┌─ buildProcessContext (services + publish + tool resolver)
  │      ├─ runWithRetry (QoS-aware 指数退避 + panic recovery)
  │      ├─ 写回 hasRun_<name> 条件（如果成功）
  │      └─ 发布 ActionExecutionResultEvent
  │
  └─ Tick 结束 → 决定是否继续下一 tick（看 status 是否 terminal）
```

---

## 3. 分层与依赖

### 3.1 包依赖图（无环）

```
        ┌──────────────────┐
        │   examples/      │  示例
        └────────┬─────────┘
                 ↓
        ┌──────────────────┐
        │   agent/         │  顶层 re-export
        └────────┬─────────┘
                 ↓
        ┌──────────────────────────────────────┐
        │  dsl/  hitl/  event/                 │  用户/集成层
        └──────────────┬───────────────────────┘
                       ↓
        ┌──────────────────┐
        │    runtime/      │  执行引擎
        └────────┬─────────┘
                 ↓
        ┌──────────────────┐
        │ planner/goap/    │  规划算法
        └────────┬─────────┘
                 ↓
        ┌──────────────────┐
        │      plan/       │  规划数据结构
        └────────┬─────────┘
                 ↓
        ┌──────────────────┐
        │      core/       │  原语（无外部框架依赖）
        └──────────────────┘
```

### 3.2 五层抽象

| Layer | 包 | 内容 |
|-------|----|----|
| **L1 原语** | `core/determination.go`, `iobinding.go`, `effect_spec.go`, `enum.go`, `world_state.go`, `semver.go`, `domain_type.go` | 三值逻辑、IO 绑定、条件键映射、枚举、WorldState 接口 |
| **L2 领域** | `core/action.go`, `condition.go`, `goal.go`, `agent.go`, `blackboard.go`, `process_context.go`, `process_options.go`, `tool_group.go`, `service_provider.go`, `replan.go`, `stuck.go`, `early_termination.go` | Action / Condition / Goal / Agent / Blackboard / ToolGroup |
| **L3 规划** | `plan/plan.go`, `planning_system.go`, `planner.go`, `condition_world_state.go` | Plan / PlanningSystem / Planner 接口 / WorldState 实现 |
| **L4 算法** | `planner/goap/astar.go` | A* 实现 |
| **L5 运行时** | `runtime/*.go` | Platform / AgentProcess / 调度 / 重试 |

### 3.3 关键不变式

| 类型 | 不变式 |
|------|--------|
| `Determination` | 值 ∈ {Unknown=0, True=1, False=2}，零值即 Unknown（关键，A* 依赖此） |
| `IOBinding` | `Type` 非空；`Name == ""` 视同 `"it"` |
| `EffectSpec(nil)` | 等价空 map，所有 helper 容忍 nil |
| `WorldState` | **完全不可变** —— Apply 返回新实例，从不修改 receiver |
| `Plan.Actions` | 不含 nil；顺序有意义 |
| `Agent` | 构造后只读；KnownConditions 用 `atomic.Pointer` 缓存 |
| `AgentProcess.ID` / `.agent` / `.options` / `.startedAt` | 创建后永不变 |
| `inMemoryBlackboard`（内部） | 所有可变字段受 `mu` 保护；读用 RLock，写用 Lock |

---

## 4. 核心抽象

### 4.1 三值逻辑 `Determination`

```go
type Determination int8
const (
    Unknown Determination = 0
    True    Determination = 1
    False   Determination = 2
)
```

**为什么三值？** 规划器要处理「这个条件还没评估出来」的情况。如果只有 true/false，未评估的条件就只能默认为 false——会让规划器误以为某些前提永远不满足。Unknown 让规划器可以**安全地推迟决策**。

零值 = Unknown 是有意为之：未初始化的条件 map 直接就是「全部 Unknown」。

```go
True.And(Unknown)   // = Unknown
False.And(Unknown)  // = False  (False 占优)
True.Or(Unknown)    // = True
Unknown.Not()       // = Unknown
```

### 4.2 IOBinding —— 输入/输出绑定

```go
type IOBinding struct {
    Name string  // 变量名，"" 等同于 "it"
    Type string  // 全限定类型名 "pkg/path.TypeName"
}
```

**核心思想**：在黑板上找东西有两种坐标：
1. **按变量名**（"topic"、"outline"）—— 用户显式指定
2. **按类型**（"github.com/foo.Topic"）—— 默认 "it" 模式下找最新同类型对象

`IOBinding.String()` 序列化为 `"name:Type"`，这是规划器条件键的统一格式。

```go
NewIOBinding[Topic]("")  // → "it:github.com/myapp.Topic"
NewIOBinding[Topic]("input")  // → "input:github.com/myapp.Topic"
```

### 4.3 EffectSpec —— 条件规格

```go
type EffectSpec map[string]Determination
```

同时承载 Action 的 `Preconditions`（"我能跑的前提"）和 `Effects`（"我跑完后会成立的事"），以及 Goal 的 `Preconditions`（"目标达成的前提"）。

**自动推导规则**（在 `core/action_typed.go: computePreconditionsAndEffects`）：

```go
// 输入  → 前提 (True)
pre["it:Topic"] = True

// 输出  → 效果 (True)
eff["it:BlogPost"] = True

// 显式 WithPre / WithPost → 加入对应集合
pre["user_authenticated"] = True

// canRerun=false → 加入 hasRun 守卫
pre["hasRun_research"] = False  // 跑过就不能再跑
eff["hasRun_research"] = True   // 跑完打标
```

### 4.4 WorldState —— 不可变世界快照

```go
type WorldState interface {
    State() map[string]Determination  // 防御性拷贝
    Timestamp() time.Time
    HashKey() string                  // A* 闭集去重
    Apply(effects EffectSpec) WorldState  // 纯函数：返回新实例
}
```

实现是 `plan.ConditionWorldState`：

- `State()` 总是返回新 map（A* 中传递、修改都安全）
- `HashKey()` 懒计算：排序后 `key=det|...` 拼接，跳过 Unknown（保持紧凑）
- `Apply` 跳过 effects 中的 Unknown 值（"无信息"不应覆盖已知值）

### 4.5 Action —— 行动单元

接口最小化：

```go
type Action interface {
    Metadata() ActionMetadata
    Execute(ctx context.Context, pc *ProcessContext) ActionStatus
}
```

`ActionMetadata` 是创建后不可变的元数据包（`Inputs`/`Outputs`/`Preconditions`/`Effects`/`QoS`/`ToolGroups`/...）。

**用户写法（推荐）—— 类型安全的泛型构造器**：

```go
agent.NewAction("research",
    func(ctx context.Context, pc *core.ProcessContext, t Topic) (Research, error) {
        return Research{...}, nil
    },
    core.ActionConfig{
        Description: "从知识库检索相关资料",
        QoS:         core.ActionQos{MaxAttempts: 3},
        ToolGroups:  core.ToolRolesFor("web_search"),
    },
)
```

框架内部 `NewAction[In, Out]` 做的事：
1. 用 `reflect.TypeFor[In/Out]()` 算出默认 IOBinding
2. 闭包捕获用户函数，包装成内部 `*typedAction[In, Out]`（实现 Action 接口）
3. `computePreconditionsAndEffects` 自动算出规划用的 EffectSpec

**运行时 Execute**（`core/action_typed.go`）：

```go
func (a *TypedAction[In, Out]) Execute(ctx context.Context, pc *ProcessContext) ActionStatus {
    if pc == nil { return ActionFailed }
    if pc.Blackboard == nil {
        pc.recordError(errors.New("typed action requires non-nil Blackboard"))
        return ActionFailed
    }

    input, ok := loadTypedInput[In](pc.Blackboard, a.metadata.Inputs)
    if !ok {
        pc.recordError(fmt.Errorf("action %q: required input not on blackboard (binding=%s)",
            a.metadata.Name, formatBindings(a.metadata.Inputs)))
        return ActionFailed
    }

    output, err := a.fn(ctx, pc, input)
    if err != nil {
        pc.recordError(err)
        return ActionFailed
    }

    a.writeOutput(pc.Blackboard, output)  // Bind() / Set() 看绑定名
    return ActionSucceeded
}
```

### 4.6 Goal —— 目标

```go
type Goal struct {
    Name        string
    Pre         []string       // 显式前提
    Inputs      []IOBinding    // 类型化前提（→ "name:Type")
    OutputType  *string        // 期望产出类型
    ValueStatic float64        // 价值（0-1）
    ValueFn     CostFunc       // 动态价值（覆盖静态）
    Tags        []string
    Examples    []string       // 给 LLM/MCP 的示例
    Export      ExportConfig
}
```

最常见的工厂：

```go
agent.GoalProducing[BlogPost]("blog post produced")
// 等价于：
Goal{
    Name:        "produce_pkg.BlogPost",
    Inputs:      []IOBinding{{Name: "it", Type: "pkg.BlogPost"}},
    OutputType:  ptrOf("pkg.BlogPost"),
    ValueStatic: 1.0,
}
```

`Goal.Preconditions()` 把 `Pre + Inputs` 合并成 `EffectSpec`，传给规划器作为搜索目标。

### 4.7 Blackboard —— 类型化共享内存

```go
type Blackboard interface {
    ID() string

    // 名字空间
    Set(key string, value any)
    Get(key string) (any, bool)

    // 类型化查询
    GetValue(variable, typeName string) (any, bool)
    HasValue(variable, typeName string) bool

    // 对象列表（按时间排序）
    AddObject(value any)
    Objects() []any

    // dual-binding：同时存到 "it" 和派生类型名
    Bind(value any)
    BindAll(map[string]any)
    BindProtected(key string, value any)  // Spawn 后保留

    Hide(target any)

    // 命名条件
    SetCondition(key string, value bool)
    GetCondition(key string) (bool, bool)

    Spawn() Blackboard  // 子黑板
    Clear()
}
```

#### 4.7.1 三种查询语义

```go
// 1. 按显式 key
bb.Get("topic")  // → Topic{...}

// 2. 按类型 + "it"（最新同类型）
bb.GetValue("it", "pkg.Topic")  // → 倒序找第一个 Topic 类型对象

// 3. 按 lastResult（最新对象，不限类型）
bb.GetValue("lastResult", "")   // → 最近 Add/Bind 的对象
```

顶层泛型 helper 是日常代码用的接口：

```go
core.Get[Topic](bb, "it")          // (Topic, bool)
core.Last[Topic](bb)               // 最新 Topic
core.ObjectsOfType[Topic](bb)      // 所有 Topic
core.Count[Topic](bb)              // Topic 数量
core.ResultOfType[BlogPost](proc)  // 从 process 取最终结果
```

#### 4.7.2 Dual-binding（autonomy）

`Bind(value)` 同时把值存到两个键：

- `"it"` ——默认引用
- 派生类型键 ——`reflect.TypeOf(value).Name()` 首字母小写

```go
type UserInput struct{ ... }
bb.Bind(UserInput{...})
// 现在以下两个查询都返回同一个对象：
bb.Get("it")
bb.Get("userInput")  // 自动派生
```

这让 prompt 模板可以写 `${userInput.field}` 而不必关心实际变量名（embabel 0.4 的 autonomy dual-binding 行为）。

#### 4.7.3 InMemoryBlackboard 实现细节

`runtime/in_memory_blackboard.go`：

```go
type InMemoryBlackboard struct {
    id string
    mu sync.RWMutex
    named      map[string]any        // 命名查询
    protected  map[string]struct{}   // Spawn 时保留的 key
    objects    []any                 // 时间序列
    hidden     []any                 // 不可发现（Hide）但不删
    conditions map[string]bool       // 显式 setCondition
}
```

**注意**：`hidden` 用 slice + `reflect.DeepEqual` 而非 `map[any]struct{}`，因为黑板上可能有 slice/map 字段（不可哈希）。线性扫描在隐藏列表通常很短的现实下足够快。

### 4.8 ProcessContext —— Action 的运行环境

```go
type ProcessContext struct {
    Process       Process            // 当前 process（替代 ThreadLocal）
    Blackboard    Blackboard         // 黑板
    Options       *ProcessOptions
    OutputChannel OutputChannel
    Services      *ServiceProvider   // 用户注册的任意服务（key→any）

    publishEvent EventPublisher      // 由 runtime 注入
    resolveTools ToolResolver        // 由 runtime 注入

    errSlot atomic.Pointer[error]    // 用于 ReplanRequest 检测
    lastErr error
}
```

每个 tick 重建一次（避免上一 action 的状态泄漏到下一个）。常用方法：

```go
chat, _ := core.ServiceOf[*chat.Client](pc.Services, "chat") // 类型化查
pc.Tracer()                        // → otel.Tracer("lynx/agent")
pc.ResolveTools(ctx, "web", "calc") // → []AgentTool 懒解析
pc.Publish(myCustomEvent)          // 推送自定义事件
pc.AwaitInput(req)                 // HITL 暂停
```

### 4.9 ToolGroup —— 跨进程工具的懒加载

```go
type ToolGroupRequirement struct {
    Role              string
    RequiredToolNames []string
    TerminationScope  TerminationScope  // AGENT / ACTION
}

type ToolGroup interface {
    Metadata() ToolGroupMetadata
    Tools(ctx context.Context) ([]AgentTool, error)  // 首次调用才连接
}
```

`LazyToolGroup` 用 `sync.Once` 缓存，确保 MCP handshake / plugin load 只发生一次。`StaticToolGroupResolver` 是单进程默认实现，生产部署可以替换为对接注册中心的版本。

---

## 5. GOAP A* 规划器

文件：`planner/goap/astar.go`

### 5.1 算法概览

A* 在**世界状态空间**中搜索（不是在动作空间中）。每个节点是一个 `WorldState`，每条边是一个 Action。

- **g(n)**：从起点到 n 的累计 Action 代价
- **h(n)**：n 到目标的启发式估计（**未满足的 Goal 前提数量**）
- **f(n) = g(n) + h(n)**

启发式是 **admissible**（绝不高估）：每个未满足条件至少需要一个 Action 来修复，所以 A* 保证找到最优解。

### 5.2 核心数据结构

```go
type searchNode struct {
    state  core.WorldState
    gScore float64  // 已走代价
    hScore float64  // 启发值
    fScore float64  // g+h，最小堆排序键
    index  int      // heap.Interface 用
}

type openList []*searchNode  // 实现 heap.Interface
```

`container/heap`（Go 标准库）做最小堆。

### 5.3 主循环（已重构为多个小函数）

```go
func (p *AStarPlanner) PlanToGoal(ctx, start, system, goal, opts) (*Plan, error) {
    // 1. 输入校验
    if start == nil { return nil, errors.New("...") }
    // ...

    // 2. OTel span
    ctx, span := plannerTracer.Start(ctx, "lynx.agent.planner.astar", ...)
    defer span.End()

    // 3. 起点已满足？
    if isGoalSatisfied(start, goal) {
        return &Plan{Actions: nil, Goal: goal}, nil
    }

    // 4. A* 搜索（独立函数）
    candidates := candidateActions(system.Actions, opts.ExcludedActions)
    bestGoalNode, cameFrom, iter, err := p.searchForGoal(ctx, start, candidates, goal, p.iterationCap(opts))
    if err != nil { return nil, err }
    if bestGoalNode == nil { return nil, nil }

    // 5. 路径重建 + 前向优化
    path := reconstructPath(cameFrom, bestGoalNode.state.HashKey(), start.HashKey())
    path = forwardOptimize(path, start)
    return &Plan{Actions: path, Goal: goal}, nil
}
```

### 5.4 `searchForGoal` 主循环

```go
for open.Len() > 0 && iterations < maxIter {
    if err := ctx.Err(); err != nil { return ..., err }  // 取消

    current := heap.Pop(open).(*searchNode)
    key := current.state.HashKey()
    if seen { continue }   // 闭集去重
    closed[key] = struct{}{}

    if isGoalSatisfied(current.state, goal) {
        // 找到一条路径，但不立即 break——队列里可能有更便宜的
        if current.gScore < bestGoalCost {
            bestGoalNode = current
            bestGoalCost = current.gScore
        }
        continue
    }

    expandNeighbors(current, candidates, start, gScores, cameFrom, open, goal)
}
```

### 5.5 邻居展开

```go
for _, action := range actions {
    if !isApplicable(action, current.state) { continue }  // 前提不满足

    nextState := current.state.Apply(action.Metadata().Effects)
    if nextState.HashKey() == currentKey { continue }     // 无变化

    tentativeG := current.gScore + action.Metadata().Cost(start)
    if existing, ok := gScores[nextKey]; ok && tentativeG >= existing { continue }  // 已有更优

    gScores[nextKey] = tentativeG
    cameFrom[nextKey] = edge{prevKey: currentKey, prevState: current.state, action: action}
    heap.Push(open, &searchNode{...})
}
```

**Cost 取自 `start`**（不是 `current.state`）：避免动态成本在搜索中漂移导致非单调。

### 5.6 多目标排序：`BestValuePlan`

```go
func (p *AStarPlanner) BestValuePlan(ctx, start, system, opts) (*Plan, error) {
    plans, err := p.PlansToGoals(ctx, start, system, opts)  // 每个 goal 一个 plan
    if err != nil { return nil, err }
    if len(plans) == 0 { return nil, nil }
    return plans[0], nil  // 已按 NetValue desc 排序
}
```

```go
plan.NetValue(ws) = goal.Value(ws) - sum(action.Cost(ws))
```

### 5.7 排除列表（Replan 防循环）

```go
type PlanOptions struct {
    ExcludedActions map[string]struct{}
    MaxIterations   int
}
```

当某个 Action 抛 `ReplanRequest`，runtime 把它加进 `excludedActions`，下一轮规划自动跳过。这是 embabel 0.4 的标准防循环机制。

### 5.8 复杂度

- **时间**：最坏 O(b^d)，b=分支因子（applicable actions），d=最短路径深度
- **空间**：O(N)，N=访问过的状态数
- 实际运行时 b 通常 1-5（每个状态下能跑的 Action 不多），10k 节点上限对 50+ Action 的 agent 都绰绰有余

---

## 6. OODA 执行循环

文件：`runtime/run.go` + `runtime/execute_action.go`

### 6.1 状态机

```
                ┌──── Run() ──────┐
                │                  │
NotStarted ──→ Running ←──[tick 成功]
                  │
                  ├─[goal 达成]      → Completed
                  ├─[plan = nil]    → Stuck → handleStuck() → 可能回 Running
                  ├─[action 失败]    → Failed
                  ├─[action waiting] → Waiting
                  ├─[action paused]  → Paused
                  ├─[term signal]    → Terminated
                  ├─[ctx 取消]       → Killed
                  └─[budget 超限]    → Terminated

Terminal 态：Completed / Failed / Stuck / Terminated / Killed
```

### 6.2 Run 主循环（守卫子句风格）

```go
func (p *AgentProcess) Run(ctx context.Context) error {
    if !p.makeRunning() { return nil }                 // 幂等：重入直接返回

    if err := p.validateAgentForRun(); err != nil {    // GOAP 必须有 goal
        p.failProcess(err)
        return err
    }

    for {
        if err := ctx.Err(); err != nil {              // 取消
            p.markCancelled(err)
            return err
        }
        if p.checkEarlyTermination() { return nil }    // budget / max actions

        if err := p.Tick(ctx); err != nil { return err }

        if p.Status().IsTerminal() {                   // 终态
            p.publishTerminalEvent()
            return nil
        }
    }
}
```

### 6.3 单 Tick

```go
func (p *AgentProcess) Tick(ctx context.Context) error {
    ctx = core.WithProcess(ctx, p)                     // 透传 process

    if signal := p.drainTerminate(); signal != nil {   // 早退
        return p.handleTerminationSignal(*signal)
    }

    ctx, span := p.startTickSpan(ctx, "lynx.agent.tick")
    defer span.End()

    worldState := p.observe(ctx, span)                 // OBSERVE

    if p.options.ProcessType == core.ProcessConcurrent {
        return p.tickConcurrent(ctx, worldState)
    }
    return p.tickSimple(ctx, worldState)
}
```

### 6.4 SimpleTick：顺序模式

```go
func (p *AgentProcess) tickSimple(ctx, ws) error {
    planResult, err := p.formulatePlan(ctx, ws)        // ORIENT
    if err != nil      { p.failProcess(err); return nil }
    if planResult == nil { return p.handleStuck(ctx, ws) }
    if planResult.IsComplete() {                       // goal 已达成
        p.completeForGoal(planResult.Goal)
        return nil
    }

    p.setGoal(planResult.Goal)
    p.publishEvent(event.PlanFormulatedEvent{...})

    action := planResult.Actions[0]                    // DECIDE: 只取第一个
    status, replan := p.executeAction(ctx, action)    // ACT
    if replan != nil {
        p.applyReplan(action, replan)
        return nil                                     // 下 tick 重 Orient
    }
    p.translateActionStatus(action, status)
    return nil
}
```

### 6.5 ExecuteAction：重试/退避/Panic 防护

```go
func (p *AgentProcess) executeAction(ctx, action) (ActionStatus, *ReplanRequest) {
    meta := action.Metadata()
    startedAt := core.Now()

    p.publishEvent(event.ActionExecutionStartEvent{...})

    ctx, span := core.AgentTracer().Start(ctx, "lynx.agent.action")
    defer span.End()

    pc := p.buildProcessContext()
    status, replan, attempts, lastErr := p.runWithRetry(ctx, action, pc, meta.QoS)
    duration := time.Since(startedAt)

    p.recordInvocation(ActionInvocation{
        ActionName: meta.Name, Timestamp: startedAt, Duration: duration,
        Status: status, Attempts: attempts,
    })

    if status == core.ActionSucceeded {
        p.blackboard.SetCondition(meta.HasRunKey(), true)  // 防重跑
    }

    span.SetAttributes(...)
    finishSpanWithError(span, lastErr)

    p.publishEvent(event.ActionExecutionResultEvent{...})

    if status == core.ActionFailed && replan == nil {
        p.recordActionFailure(meta.Name, lastErr)
    }
    return status, replan
}
```

#### 重试循环

```go
func (p *AgentProcess) runWithRetry(ctx, action, pc, qos) (...) {
    op := func() error {
        attempts++
        pc.ResetError()
        status = runWithPanicRecovery(ctx, action, pc)   // panic → ActionFailed
        lastErr = pc.LastError()

        if rr := core.AsReplanRequest(lastErr); rr != nil {
            replan = rr
            return lastErr                               // 不再重试，让 retry 短路
        }
        switch status {
        case core.ActionSucceeded:
            return nil                                   // retry 完成
        case core.ActionWaiting, core.ActionPaused:
            return haltSignal{status}                    // 不重试，但也不算失败
        }
        return lastErr                                   // ActionFailed → retry
    }

    _ = retry.Do(op,
        retry.WithContext(ctx),
        retry.WithMaxAttempts(qos.MaxAttempts),
        retry.WithBaseDelay(qos.BaseDelay),
        retry.WithMaxDelay(qos.MaxDelay),
        retry.WithExponentialBackoff(),
        retry.WithRetryCondition(shouldRetryAction),     // replan / halt 不重试
    )
    return status, replan, attempts, lastErr
}
```

退避策略（`core/action_qos.go`）—— 复用 `pkg/retry` 提供的指数退避 + 抖动：

```go
func DefaultActionQos() ActionQos {
    return ActionQos{
        MaxAttempts: 5,
        BaseDelay:   10 * time.Second,
        MaxDelay:    60 * time.Second,
    }
}

// 退避序列（pkg/retry 的 ExponentialBackoff，×2 步进 + jitter）：
// 10s, 20s, 40s, 60s（被 cap 限制）, 60s
```

`runtime/execute_action.go` 把 `ActionQos` 翻译成一组 `retry.Option` 交给 `retry.Do`；自动获得 ctx 取消传播、抖动、溢出保护。

### 6.6 WorldStateDeterminer：OBSERVE 阶段

文件：`runtime/world_state_determiner.go`

```go
func (d *BlackboardDeterminer) DetermineWorldState(ctx) WorldState {
    state := map[string]Determination{}
    oc := &OperationContext{...}

    for cond := range d.system.KnownConditions() {     // 只评估已知键
        state[cond] = d.evaluateCondition(ctx, cond, oc)
    }
    return plan.NewConditionWorldState(state)
}
```

四种条件键的优先级解析：

```go
func (d *BlackboardDeterminer) evaluateCondition(ctx, key, oc) Determination {
    if strings.Contains(key, ":") {
        return d.evaluateTypeBinding(key)              // "it:pkg.Topic"
    }
    if strings.HasPrefix(key, hasRunPrefix) {
        return d.evaluateHasRun(key)                   // "hasRun_research"
    }
    if cond := d.findNamedCondition(key); cond != nil {
        return cond.Evaluate(ctx, oc)                  // 命名 Condition
    }
    if value, ok := d.blackboard.GetCondition(key); ok {
        return core.FromBool(value)                    // 显式 setCondition
    }
    return core.Unknown                                // 未知 → Unknown
}
```

---

## 7. 并发执行模型

文件：`runtime/concurrent.go`

启用方式：

```go
platform.RunAgent(ctx, agent, bindings,
    core.ProcessOptions{ProcessType: core.ProcessConcurrent},
)
```

### 7.1 算法

```go
func (p *AgentProcess) tickConcurrent(ctx, ws) error {
    planResult := /* ... 同 simple ... */
    achievable := filterAchievable(planResult.Actions, ws)  // 当前可达的 actions

    if len(achievable) == 0 {
        return p.tickSimple(ctx, ws)                        // 退化到顺序
    }

    results, replans := p.runActionsInParallel(ctx, achievable)

    if p.applyReplansFromParallel(achievable, replans) {
        return nil
    }
    p.setStatus(mergeStatuses(results))                    // 合并子状态
    return nil
}
```

### 7.2 `runActionsInParallel`

```go
func (p *AgentProcess) runActionsInParallel(ctx, actions) (statuses, replans) {
    g, egCtx := errgroup.WithContext(ctx)
    var slotMu sync.Mutex
    for index, action := range actions {
        index, action := index, action
        g.Go(func() error {
            status, replan := p.executeAction(egCtx, action)
            slotMu.Lock()
            results[index] = status
            replans[index] = replan
            slotMu.Unlock()
            return nil
        })
    }
    _ = g.Wait()
    return results, replans
}
```

`errgroup.WithContext`：任一 action 返回 error 会取消 ctx（让其他 in-flight action 能感知）。这里我们 `g.Go` 总返回 nil（错误已经在 `pc.lastErr` 里了），所以不会触发取消——并发动作互不干扰。

### 7.3 状态合并

```go
func mergeStatuses(statuses []ActionStatus) AgentProcessStatus {
    // 优先级：Failed > Waiting > Paused > Running
    for _, s := range statuses { if s == ActionFailed   { return StatusFailed }  }
    for _, s := range statuses { if s == ActionWaiting  { return StatusWaiting } }
    for _, s := range statuses { if s == ActionPaused   { return StatusPaused }  }
    return StatusRunning
}
```

### 7.4 收益示例

博客 agent（research / outline 互不依赖；write 依赖二者）：

| 模式 | tick 1 | tick 2 | tick 3 | 总计 |
|------|--------|--------|--------|------|
| Simple | research | outline | write | 3 ticks |
| Concurrent | research \|\| outline | write | — | **2 ticks** |

---

## 8. 事件系统

文件：`event/event.go`

### 8.1 事件类型

19 种生命周期事件，分 6 组：

| 分组 | 事件 |
|------|------|
| 平台 | `AgentDeployedEvent` / `AgentUndeployedEvent` |
| 进程生命周期 | `ProcessCreatedEvent` / `ProcessCompletedEvent` / `ProcessFailedEvent` / `ProcessStuckEvent` / `ProcessWaitingEvent` / `ProcessPausedEvent` / `ProcessKilledEvent` / `ProcessTerminatedEvent` |
| 规划 | `ReadyToPlanEvent` / `PlanFormulatedEvent` / `ReplanRequestedEvent` |
| 执行 | `ActionExecutionStartEvent` / `ActionExecutionResultEvent` / `ObjectBoundEvent` / `GoalAchievedEvent` |
| LLM | `LLMRequestEvent` / `LLMResponseEvent`（埋点接入时使用） |

每个都嵌入 `BaseEvent { At time.Time; PID string }`，并实现 `Event` 接口。

### 8.2 Multicast Listener

```go
type Listener interface {
    OnEvent(e Event)
}

type Multicast struct {
    mu        sync.RWMutex
    listeners []Listener
}

func (m *Multicast) OnEvent(e Event) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    for _, listener := range m.listeners {
        safeDeliver(listener, e)  // panic 隔离
    }
}
```

**panic 隔离**：单个 listener 出错不会传染到其他 listener 或 runtime。

### 8.3 自定义 Listener

```go
platform.AddListener(event.ListenerFunc(func(e event.Event) {
    switch ev := e.(type) {
    case event.ActionExecutionResultEvent:
        log.Printf("action %s done in %v", ev.Action.Metadata().Name, ev.Duration)
    case event.GoalAchievedEvent:
        log.Printf("goal achieved: %s", ev.Goal.Name)
    }
}))
```

---

## 9. 人机协同（HITL）

文件：`hitl/awaitable.go` + `core/awaitable.go` + `runtime/agent_process.go`

### 9.1 设计

**Action 端**：

```go
agent.NewAction("approve_payment",
    func(ctx context.Context, pc *core.ProcessContext, p Payment) (Approval, error) {
        req := hitl.NewConfirmation(p, func(approved bool) core.ResponseImpact {
            // ... 处理回调，决定是否触发重 plan ...
            return core.ResponseImpactUpdated
        })

        // 暂停 process，等待外部输入
        pc.AwaitInput(req)
        // ↑ 此时 process 进入 StatusWaiting，executeAction 返回 ActionWaiting
        return Approval{}, nil
    },
)
```

**外部**（HTTP handler 等）：

```go
func handlePaymentApproval(w http.ResponseWriter, r *http.Request) {
    processID := r.PathValue("id")
    var approved bool  // 解析 body

    if err := platform.ResumeProcess(processID, approved); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
}
```

### 9.2 内部机制

```go
type awaitSlot struct {
    awaitable core.Awaitable
    respond   chan any
}

// AgentProcess
type AgentProcess struct {
    ...
    pendingAwaitable atomic.Pointer[awaitSlot]
}

func (p *AgentProcess) AwaitInput(req core.Awaitable) core.ActionStatus {
    slot := &awaitSlot{awaitable: req, respond: make(chan any, 1)}
    p.pendingAwaitable.Store(slot)
    p.publishEvent(event.ProcessWaitingEvent{...})
    return core.ActionWaiting
}

func (p *AgentProcess) deliverResponse(response any) bool { // 内部，由 Platform.ResumeProcess 调用
    slot := p.pendingAwaitable.Swap(nil)
    if slot == nil { return false }
    select {
    case slot.respond <- response: return true
    default: return false
    }
}
```

### 9.3 类型化 Awaitable

```go
// 确认（yes/no）
hitl.NewConfirmation(payload, func(approved bool) core.ResponseImpact { ... })

// 复杂表单（任意 P 入参 → R 出参）
hitl.NewTypedRequest[OrderForm, OrderResponse](
    formData,
    func(resp OrderResponse) core.ResponseImpact { ... },
)
```

---

## 10. 使用指南

### 10.1 最小示例

```go
package main

import (
    "context"
    "fmt"
    "log"
    "strings"

    "github.com/Tangerg/lynx/agent"
    "github.com/Tangerg/lynx/agent/core"
)

type CountResult struct{ Length int }

func main() {
    a := agent.New("Hello").
        Description("count uppercase characters of a phrase").
        Actions(agent.NewAction("count_upper",
            func(ctx context.Context, pc *core.ProcessContext, in string) (CountResult, error) {
                return CountResult{Length: len(strings.ToUpper(in))}, nil
            },
            core.ActionConfig{},
        )).
        Goals(agent.GoalProducing[CountResult](core.Goal{
            Description: "uppercase length determined",
        })).
        Build()

    platform := agent.NewPlatform(runtime.PlatformConfig{})
    if err := platform.Deploy(a); err != nil {
        log.Fatal(err)
    }

    proc, err := platform.RunAgent(
        context.Background(), a,
        map[string]any{core.DefaultBindingName: "hello"},
        core.ProcessOptions{},
    )
    if err != nil { log.Fatal(err) }

    result, _ := core.ResultOfType[CountResult](proc)
    fmt.Printf("status=%s length=%d\n", proc.Status(), result.Length)
}
```

输出：`status=completed length=5`

### 10.2 多 Action 规划示例（完整版）

见 `examples/blog/main.go`。三个 Action：

| Action | 输入 | 输出 |
|--------|------|------|
| `research` | `Topic` | `Research` |
| `outline` | `Topic` | `Outline` |
| `write` | `Outline`（+ Research/Topic 通过 Get） | `BlogPost` |

Goal：`agent.GoalProducing[BlogPost](core.Goal{Description: "blog post produced"})`。

规划器自动找出顺序：`research → outline → write`（或 `outline → research → write`，代价相同）。

### 10.3 Platform 配置

```go
services := core.NewServiceProvider()
services.Set("chat", myLLMClient)         // 任意类型
services.Set("rag",  ragPipeline)
services.Set("vector", vstore)

platform := agent.NewPlatform(runtime.PlatformConfig{
    Services:    services,
    Tools:       toolResolver,             // 工具解析器仍然是单独字段（runtime 内部用）
    IDGenerator: runtime.NewCounterIDGenerator("test"),
    Listeners:   []event.Listener{myEventListener},
})

// 在 action 里类型化取出：
client, ok := core.ServiceOf[*chat.Client](pc.Services, "chat")
```

### 10.4 ProcessOptions

```go
proc, _ := platform.RunAgent(ctx, a, bindings, core.ProcessOptions{
    Budget: core.Budget{
        CostLimit:   5.0,        // USD
        ActionLimit: 100,
        TokenLimit:  500_000,
    },
    Verbosity:   core.Verbosity{ShowPlanning: true},
    ProcessType: core.ProcessConcurrent,
    ProcessControl: core.ProcessControl{
        EarlyTerminationPolicy: core.MaxActionsPolicy{Max: 50},
        ToolDelay:              100 * time.Millisecond,
    },
})
```

### 10.5 错误诊断

```go
proc, err := platform.RunAgent(...)
switch proc.Status() {
case core.StatusCompleted:
    result, _ := core.ResultOfType[Result](proc)
    // ...
case core.StatusFailed:
    log.Printf("agent failed: %v", proc.Failure())
case core.StatusStuck:
    log.Printf("planner could not find a path; world state: %+v",
        proc.LastWorldState().State())
case core.StatusTerminated:
    log.Printf("budget/policy triggered termination")
}

for _, inv := range proc.History() {
    log.Printf("  %s: %s in %v (attempts=%d)",
        inv.ActionName, inv.Status, inv.Duration, inv.Attempts)
}
```

### 10.6 与 OpenTelemetry 集成

`agent/` 内部所有 span 走 `otel.Tracer("lynx/agent")` 和 `"lynx/agent/planner"`。host 配置全局 TracerProvider 即可：

```go
import (
    "go.opentelemetry.io/otel"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

tp := sdktrace.NewTracerProvider(/* exporters */)
otel.SetTracerProvider(tp)
defer tp.Shutdown(ctx)
```

trace 形态：

```
lynx.agent.tick (一次 OODA)
├── lynx.agent.planner.astar (orient)
│     attrs: goal.name, astar.iterations, astar.plan_length
└── lynx.agent.action (act)
      attrs: action.name, action.status, action.attempts
```

---

## 11. 扩展点

### 11.1 自定义 Planner

实现 `plan.Planner` 接口：

```go
type Planner interface {
    PlanToGoal(ctx, start, system, goal, opts) (*Plan, error)
    PlansToGoals(ctx, start, system, opts) ([]*Plan, error)
    BestValuePlan(ctx, start, system, opts) (*Plan, error)
    Prune(system) *PlanningSystem
}
```

注入：

```go
platform := agent.NewPlatform(runtime.PlatformConfig{
    PlannerFactory: func(t core.PlannerType) plan.Planner {
        return myCustomPlanner
    },
})
```

### 11.2 自定义 Blackboard

实现 `core.Blackboard` 接口（17 个方法）。常见需求：Redis-backed 黑板用于跨进程恢复、事件溯源黑板等。

```go
proc, _ := platform.RunAgent(ctx, a, bindings, core.ProcessOptions{
    Blackboard: myCustomBlackboard,
})
```

### 11.3 ToolGroupResolver

```go
type myResolver struct{ /* ... */ }

func (r *myResolver) Resolve(ctx context.Context, req core.ToolGroupRequirement) (core.ToolGroup, error) {
    // 例如：从 MCP server 动态发现工具
    return core.NewLazyToolGroup(metadata, func(ctx context.Context) ([]core.AgentTool, error) {
        return mcpClient.Discover(ctx, req.Role)
    }), nil
}

platform := agent.NewPlatform(runtime.PlatformConfig{
    Tools: &myResolver{},
})
```

### 11.4 StuckHandler

当 planner 返回 nil plan 时触发。可以在这里降级 Goal、放松约束、向用户发起 HITL 请求等。

```go
agent.New("...").
    StuckHandler(core.StuckHandlerFunc(func(ctx context.Context, p core.Process) core.StuckHandlingResult {
        // 例如：移除某个 protected condition
        p.Blackboard().SetCondition("strict_mode", false)
        return core.StuckHandlingResult{Code: core.StuckReplan, Reason: "relaxed strict_mode"}
    })).
    /* ... */ .Build()
```

### 11.5 EarlyTerminationPolicy

实现 `core.EarlyTerminationPolicy`，组合用 `core.CompositePolicy`：

```go
core.CompositePolicy{Policies: []core.EarlyTerminationPolicy{
    core.MaxActionsPolicy{Max: 50},
    core.BudgetPolicy{Budget: core.Budget{TokenLimit: 100_000}},
    myCustomPolicy{},
}}
```

### 11.6 注册自定义服务

框架不预设任何 LLM / RAG / VectorStore 类型 —— 用户自己往 `core.ServiceProvider` 里塞任意东西，action 里用泛型 helper 取：

```go
services := core.NewServiceProvider()
services.Set("chat",   lynxChatClient)   // *chat.Client
services.Set("rag",    myRAGPipeline)    // 任意 RAG 实现
services.Set("oracle", domainOracle)     // 用户自己的领域服务

platform := agent.NewPlatform(runtime.PlatformConfig{Services: services})

// action 里：
chat,   _ := core.ServiceOf[*chat.Client](pc.Services, "chat")
oracle, _ := core.ServiceOf[*MyOracle](pc.Services, "oracle")
```

这样框架就跟具体 LLM / RAG 生态完全解耦了。

---

## 12. 代码地图

```
agent/
├── go.mod                                  独立 module
├── README.md                               快速上手
├── DESIGN.md                               (本文档)
├── agent.go                                顶层 re-exports
│
├── core/                                   Layer 1-2：原语 + 领域
│   ├── determination.go                    三值逻辑
│   ├── iobinding.go                        变量名:类型 编码
│   ├── effect_spec.go                      条件 → Determination map
│   ├── enum.go                             ActionStatus / AgentProcessStatus / ...
│   ├── world_state.go                      WorldState 接口
│   ├── semver.go                           语义版本
│   ├── domain_type.go                      类型 schema
│   │
│   ├── condition.go                        Condition 接口 + And/Or/Not
│   ├── operation_context.go                Condition.Evaluate 上下文
│   ├── action.go                           Action 接口 + ActionMetadata
│   ├── action_qos.go                       重试策略
│   ├── action_typed.go                     NewAction[In,Out]（typedAction 私有）
│   ├── action_config.go                    ActionConfig 结构 + 默认值/校验
│   ├── goal.go                             Goal + GoalProducing[T]
│   ├── agent.go                            Agent 聚合 + KnownConditions 缓存
│   │
│   ├── blackboard.go                       Blackboard 接口 + Get/Last/Count 泛型 helper
│   ├── process.go                          Process 接口 + ctx 透传
│   ├── process_context.go                  ProcessContext (Action 看到的)
│   ├── process_options.go                  ProcessOptions / Budget / Verbosity / ...
│   ├── output_channel.go                   消息推送
│   ├── service_provider.go                 ServiceProvider 注册表 + ServiceOf[T] 泛型 helper
│   ├── tool_group.go                       ToolGroup 生态 + TerminationScope
│   ├── awaitable.go                        Awaitable 非泛型基接口
│   ├── replan.go                           ReplanRequest 错误类型
│   ├── stuck.go                            StuckHandler
│   └── early_termination.go                EarlyTerminationPolicy
│
├── plan/                                   Layer 3：规划数据结构
│   ├── condition_world_state.go            ConditionWorldState 实现
│   ├── plan.go                             Plan + Cost/Value/NetValue
│   ├── planning_system.go                  Action+Goal+Condition 集合
│   └── planner.go                          Planner 接口
│
├── planner/
│   └── goap/
│       └── astar.go                        A* GOAP 算法
│
├── runtime/                                Layer 5：执行引擎
│   ├── id_gen.go                           UUID / Counter ID 生成器
│   ├── in_memory_blackboard.go             默认 Blackboard 实现
│   ├── world_state_determiner.go           OBSERVE 阶段
│   ├── agent_process.go                    AgentProcess 状态机
│   ├── run.go                              Run/Tick 主循环（Simple）
│   ├── concurrent.go                       Concurrent tick
│   ├── execute_action.go                   单 Action 执行 + 重试 + panic
│   └── platform.go                         Platform 入口
│
├── event/
│   └── event.go                            19 种事件 + Multicast listener
│
├── dsl/
│   └── builder.go                          流式 builder
│
├── hitl/
│   └── awaitable.go                        ConfirmationRequest / FormBindingRequest
│
└── examples/
    ├── hello/main.go                       单 Action 单 Goal
    └── blog/main.go                        三 Action GOAP 规划
```

---

## 13. 设计权衡

### 13.1 为什么不提供 struct + 反射注册的便利层？

**答**：embabel 大量用 Kotlin 反射读 JVM 签名信息——这在 JVM 上几乎免费且元信息丰富。Go 反射比直接调用慢 10-50×，且对自定义泛型参数的支持很弱；更关键的是 Go 社区共识是「显式优于隐式」，反射框架（@Action / @Agent / 隐式注入）会让错误推迟到运行时、IDE 重构失效、命名约定成为隐藏契约。

所以我们只保留**泛型构造器** `NewAction[In, Out]` 一种入口：编译期保留所有类型、零反射、IDE 跳转/重命名全程友好。从 Spring 迁移过来的用户付出的代价是要写一行显式 `agent.New("X").Description(...).Actions(...).Build()` 而不是给方法加注解——这点学习成本远低于运行时调试反射栈的代价。

### 13.2 为什么 ServiceProvider 是开放的 key→any 注册表，而不预设 LLM / RAG 槽位？

**答**：依赖反转 + 不绑生态。如果 `core.ProcessContext` 暴露 `ChatClient` / `RAGClient` 这种领域接口，框架就会偏向某种 LLM 抽象（embabel 选了一种，langchain 又是另一种）。Lynx 选择不参与这场抽象大战 —— `core.ServiceProvider` 是个无类型的 map[string]any 注册表，用户自己决定塞什么、用什么 key、用什么 Go 类型。

```go
services := core.NewServiceProvider()
services.Set("chat", lynxChatClient)            // *chat.Client（lynx 自己的）
services.Set("rag",  langchainRetrievalChain)   // 任意第三方
services.Set("billing", myDomainService)        // 用户的业务服务

// action 里类型化取出：
chat, _ := core.ServiceOf[*chat.Client](pc.Services, "chat")
```

代价：取服务时要写一行类型断言；收益：core/agent 完全无 chat/rag 依赖、用户可以注册任意领域服务。

### 13.3 为什么 Blackboard.Hide 用 slice 而非 map？

**答**：黑板上的对象可能含 slice / map / func 字段，这些类型不可哈希——`map[any]struct{}` 会 panic。Hide 调用本身罕见（一般业务用不到），线性扫描足够。

### 13.4 为什么 WorldState 完全不可变？

**答**：A* 在搜索过程中需要保留每个节点的状态以便：

1. **闭集去重**（`HashKey()` 当 key）
2. **路径重建**（`cameFrom` 引用旧 state）
3. **代价比较**（重新计算同状态的 g 值）

如果 Apply 修改 receiver，搜索过程会变成"破坏性的"——上一步访问过的状态会被改写。不可变让算法直观、可调试、能并行。

### 13.5 为什么不抽象 Observation 层？

**答**：embabel 用 Micrometer Observation API 包了一层，因为 Spring 生态有多种监控后端（Prometheus / Datadog / OTLP），需要统一。

Go 生态已经收敛到 OpenTelemetry，做抽象是浪费：
- 直接 `otel.Tracer("lynx/agent")` 在 host 配 TracerProvider 后自动指向任意后端
- 用户不需要学新 API
- 减少 50+ 行间接层代码

### 13.6 为什么 Concurrent tick 用 errgroup 而不是 sync.WaitGroup？

**答**：`errgroup` 的核心价值是 **`WithContext` 派生的可取消 ctx**——任一 goroutine 返回 error 会自动取消其他在 flight 的 goroutine。我们的 `g.Go` 闭包不主动返回 error（错误已在 `pc.lastErr`），但保留 errgroup 是为了将来扩展（例如某 action 显式要求 fail-fast）。

### 13.7 为什么 ReplanRequest 不用 panic？

**答**：embabel 用 `throw ReplanRequestedException` 是因为 Kotlin 没有零成本检查异常机制，throw 是表达"特殊控制流"的惯用法。

Go 的 idiom 是 error + 类型断言：

```go
if err != nil {
    if rr := core.AsReplanRequest(err); rr != nil {
        // 重 plan 路径
    } else {
        // 普通失败
    }
}
```

panic 在 Go 是真正的异常情况（程序员错误），不应承载控制流。

### 13.8 为什么 Goal.Inputs 转成 EffectSpec 时只设 True？

**答**：`it:Topic` 在 EffectSpec 里设为 True 表达 "黑板上必须有 Topic 类型的对象"。

没有反向需求（"黑板上不能有 Topic"），所以不需要 False。如果将来某 agent 需要表达"达到目标时 X 必须不存在"，EffectSpec 已经支持——只是 `Goal.Preconditions()` 默认不生成。

---

## 附录 A：常见错误模式

### A.1 "process stuck"——规划器找不到计划

可能原因：

1. **输入类型不在黑板上**：检查 `bindings` 里的 key 类型是否匹配 Action 的 In
2. **action 链断裂**：`A: In=Topic Out=Outline`、`B: In=Research Out=BlogPost`，但没有 Action 产出 Research → 永远到不了 Goal
3. **canRerun=false 阻塞**：所有 Action 都跑过一次后 hasRun_ 为 True，新 plan 排除它们 → 无可用 Action

调试：

```go
log.Printf("world state: %+v", proc.LastWorldState().State())
log.Printf("history: %+v", proc.History())
```

### A.2 "action input not on blackboard"

```go
// ❌ 错误：write 期望 BlogPost 输入但黑板上是 Outline
agent.NewAction("write",
    func(ctx context.Context, pc *core.ProcessContext, in BlogPost) (Article, error) {...},
)

// ✅ 正确：用 Outline 作为 In，从 pc.Blackboard 获取其它依赖
agent.NewAction("write",
    func(ctx context.Context, pc *core.ProcessContext, in Outline) (Article, error) {
        topic, _ := core.Get[Topic](pc.Blackboard, "it")
        ...
    },
    core.ActionConfig{Pre: []string{"it:" + core.TypeFullNameOf[Research]()}},
)
```

### A.3 "agent has no goals"

GOAP 必须有目标。如果只想运行单个 Action（Function-as-a-Service 模式），仍要给一个 `GoalProducing[Out]` 或显式构造 `&core.Goal{Name: ..., Description: ..., Pre: []string{...}}`。

---

## 附录 B：性能边界

| 维度 | 现实预期 |
|------|---------|
| Action 数 | < 100：A* 10k 迭代上限够用；> 100 建议加 ExcludedActions 或自定义 Planner |
| Goal 数 | < 50：`PlansToGoals` 是 O(goals × A* iterations) |
| 黑板对象数 | < 1000：`findLatestByType` 是 O(n) 反向扫描 |
| Tick 间隔 | 主要瓶颈是 LLM 调用（数百 ms+），框架本身 < 1ms |
| 并发 process | 任意：每个 process 完全独立，Platform 只用 RWMutex 保护进程列表 |

每个层都可定向优化（自定义 Blackboard 用 Redis、自定义 Planner 用 utility 替代 A*），但默认实现对 demo 和小型生产已经够用。

---

## 附录 C：与 embabel 的特性对照

| 特性 | embabel | Lynx Agent | 备注 |
|------|---------|-----------|------|
| GOAP A* | ✅ | ✅ | 算法行为完全一致 |
| 黑板 | ✅ | ✅ | dual-binding (autonomy) 已实现 |
| OODA loop | ✅ | ✅ | Simple + Concurrent 两种 tick |
| 三值逻辑 | ✅ | ✅ | `Determination` |
| 注解 (`@Action`) | ✅ | ❌ | 显式 DSL 替代；Go 没有运行时注解 |
| HITL (`Awaitable`) | ✅ | ✅ | 含 ConfirmationRequest / FormBindingRequest |
| TerminationScope | ✅ (0.4) | ✅ | TerminateAgent / TerminateAction |
| ToolGroup 生态 | ✅ (0.4) | ✅ | LazyToolGroup + StaticResolver |
| ReplanRequest | ✅ | ✅ | error 接口 |
| Budget / Early termination | ✅ | ✅ | MaxActionsPolicy / BudgetPolicy / Composite |
| StuckHandler | ✅ | ✅ | |
| Sub-agent (CreateChildProcess) | ✅ | ✅ | 子黑板 Spawn() |
| MCP 集成 | ✅ | 🟡 | 接口已就位 (ToolGroupResolver)，具体桥接在 lynx/mcp |
| A2A 协议 | ✅ | ❌ | 待 `agents/a2a` 子模块 |
| Shell REPL | ✅ | ❌ | 待 `agents/shell` 子模块 |
| Skills (YAML+Docker) | ✅ | ❌ | 待 `agents/skills` 子模块 |
| Spring 自动配置 | ✅ | N/A | Go 无 DI 容器 |

---

**文档维护**：当框架结构变更时，更新本文 §3（依赖图）、§4（核心抽象）、§12（代码地图）相应小节。性能边界（附录 B）建议每季度跑一次 benchmark 校准。
