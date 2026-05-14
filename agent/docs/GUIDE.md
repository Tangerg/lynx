# Lynx Agent 深度指南

> 配套文档：
> - [`README.md`](./README.md) — 5 行 quick-start
> - [`EXTENSION_DESIGN.md`](./EXTENSION_DESIGN.md) — Extension / SPI 系统当前形态
> - [`EMBABEL_COMPARISON.md`](./EMBABEL_COMPARISON.md) — 与 embabel-agent 的多角度对比
> - 本文档 — **架构 + 实现 + 使用 + 构建上层库**

---

## 目录

- [Part I — 宏观架构](#part-i--宏观架构)
- [Part II — 核心抽象的微观实现](#part-ii--核心抽象的微观实现)
- [Part III — 运行时机制](#part-iii--运行时机制)
- [Part IV — 使用指南](#part-iv--使用指南)
- [Part V — 构建更高层库](#part-v--构建更高层库)
- [速查表](#速查表)

---

# Part I — 宏观架构

## I.1 模块拓扑

```
agent/
├── core/             ← 原语：Action / Goal / Condition / Agent / Blackboard / Awaitable / Process
├── plan/             ← WorldState、Plan、Planner 接口、PlanningSystem
├── planner/goap/     ← A* GOAP 实现（plan.Planner 的具体实现）
├── runtime/          ← Platform、AgentProcess、OODA tick、in-memory blackboard
├── event/            ← 事件类型 + Multicast Listener
├── dsl/              ← 流式 Builder（用户唯一推荐入口）
├── hitl/             ← Awaitable 类型化（TypedRequest + NewConfirmation）
└── examples/         ← hello (1 action) / blog (3 action 链式)

agent.go              ← 顶层包：再导出 New / NewAction / NewCondition / GoalProducing / NewPlatform
                        + 类型别名 Agent / Goal / Action / Platform / ...
```

依赖方向（无环）：

```
                        agent.go
                           ↓
              ┌────── dsl ──────┐
              ↓                 ↓
         core ←─ plan ←─ planner/goap
              ↑       ↑
              └─ runtime → event
              ↑
              hitl
```

`runtime` 是唯一同时 import `core / plan / planner/goap / event` 的包；`core` 不依赖任何下游包。

## I.2 五层抽象

| 层 | 关键类型 | 职责 |
|---|---|---|
| **L0 原语** | `Determination` / `IOBinding` / `EffectSpec` / `WorldState` / `CostFunc` | 不可变值类型；3 值逻辑、类型化绑定、条件规格 |
| **L1 对象** | `Action` / `Goal` / `Condition` / `Agent` / `Blackboard` / `Awaitable` | 用户可见的 agent 构件 |
| **L2 上下文** | `Process` / `ProcessContext` / `OperationContext` / `ProcessOptions` | 一次 run 的运行时上下文 |
| **L3 规划** | `Plan` / `PlanningSystem` / `Planner` / `AStarPlanner` | A* GOAP 算法 |
| **L4 调度** | `Platform` / `AgentProcess` / `Multicast` | OODA 循环、事件分发、生命周期 |

依赖严格自上而下（除了 `Platform` 反向引用 `core.Process` 接口）。

## I.3 OODA 循环 — 核心数据流

```
┌─────────────────────────────────────────────────────────────────┐
│                  Platform.RunAgent(ctx, agent, opts)            │
│                            │                                     │
│                            ▼                                     │
│                  createProcess() → AgentProcess                  │
│                            │                                     │
│                            ▼                                     │
│                       proc.run(ctx)                              │
│                            │                                     │
│         ┌──────────────────┴───────────────────────┐            │
│         │              for { tick(ctx) }           │            │
│         │                     │                     │            │
│         │       ┌─────────────▼──────────────┐     │            │
│         │       │   1. drainTerminate()      │  signal? → exit  │
│         │       │   2. checkEarlyTermination │  budget? → exit  │
│         │       │   3. observe()             │  → WorldState    │
│         │       │      ↓ DetermineWorldState │                  │
│         │       │   4. formulatePlan()       │  ← Planner       │
│         │       │      ↓ BestValuePlan       │                  │
│         │       │   5. executeAction()       │  ← typed action  │
│         │       │      ↓ retry / panic guard │                  │
│         │       │   6. translateActionStatus │                  │
│         │       └────────────┬───────────────┘                  │
│         │                    ▼                                   │
│         │       Status terminal? → publishTerminalEvent + exit  │
│         └──────────────────────────────────────────┘            │
└─────────────────────────────────────────────────────────────────┘
```

每个 tick 执行：
- **Observe** — `worldStateDeterminer.DetermineWorldState(ctx)` 读 `Blackboard` 渲染出当前 `WorldState`
- **Orient + Decide** — `planner.BestValuePlan(ctx, ws, system, opts)` 跑 GOAP A* 找最优 plan
- **Act** — 执行 plan 第一个 action（simple 模式）或并行可达的所有 action（concurrent 模式）

## I.4 并发模型

| 组件 | 并发原语 | 备注 |
|---|---|---|
| `Blackboard` (inMemoryBlackboard) | `sync.RWMutex` | 读用 RLock，写用 Lock |
| `AgentProcess` 状态字段 | `sync.RWMutex` | 保护 status/goal/lastWorld/history/failure/excludedActions |
| `pendingAwaitable` (HITL slot) | `atomic.Pointer[awaitSlot]` | Swap 原子地认领 slot |
| `toolCallCancel` | `atomic.Pointer[context.CancelFunc]` | CompareAndSwap 防覆盖 |
| `event.Multicast` | `sync.RWMutex` + snapshot | listeners 列表在 RLock 下复制，分发在锁外 |
| `ServiceProvider` | `sync.RWMutex` | 服务注册表 |
| 子进程递归 `Usage()` | 每个 process 各自 RLock | 父子树是 DAG，无循环 |
| `concurrent` tick | `errgroup` + 唯一 index 写 | `g.Wait()` 自带 happens-before 同步 |

**关键不变式**：`WorldState` 一旦构造就**不可变**，`Apply()` 返回新实例。`HashKey()` 在构造时算好，并发安全。`Agent` 在 `NewAgent()` 后不可变。

---

# Part II — 核心抽象的微观实现

## II.1 三值逻辑 `Determination`

```go
type Determination int8
const (
    Unknown Determination = 0
    True    Determination = 1
    False   Determination = 2
)
```

零值是 `Unknown`，A\* 把 Unknown 当作"不满足"——避免 GOAP 在缺失信息时出错路径。`d.And(other)`、`d.Or(other)`、`d.Not()` 是三值布尔代数，`FromBool(b)` 提升为 True/False。

## II.2 IOBinding 与类型推导

```go
type IOBinding struct {
    Name string  // 变量名，"it" 是默认
    Type string  // 全限定类型名（含 PkgPath）
}
b := core.NewIOBinding[Topic]("specialName")
// → IOBinding{Name: "specialName", Type: "github.com/Tangerg/lynx/agent/examples/blog.Topic"}
```

`String()` 输出 `name:Type` 形式，**这是 GOAP precondition / effect key**。即"输入 X 已存在于黑板"=`x:Type{} = True`。

## II.3 WorldState — 不可变快照

```go
type WorldState interface {
    State() map[string]Determination
    Timestamp() time.Time
    HashKey() string
    Apply(effects EffectSpec) WorldState
}
```

`Apply` 返回**新** WorldState，原对象不动。`HashKey()` 是稳定字符串，A\* 用它去重 closed set。具体实现：`plan.NewConditionWorldState(state)`，构造时算好 hash（防止并发 tick 竞态）。

## II.4 Action — 行动单元

```go
type Action interface {
    Metadata() ActionMetadata
    Execute(ctx, pc) ActionStatus
}

type ActionMetadata struct {
    Name          string
    Description   string
    Inputs        []IOBinding
    Outputs       []IOBinding
    Preconditions EffectSpec  // 自动从 Inputs + cfg.Pre 推导
    Effects       EffectSpec  // 自动从 Outputs + cfg.Post 推导
    CanRerun      bool
    QoS           ActionQos
    Cost          CostFunc    // 用 core.Static(v) 表静态值
    Value         CostFunc
}
```

`NewAction[In, Out]` 是泛型构造器，自动从 `[In, Out]` 反射出输入/输出类型名，写入 `Inputs[0]` / `Outputs[0]`。每次成功执行后自动设置 `hasRun_<name> = True`，并把 `hasRun_<name> = False` 加入 precondition——所以 `CanRerun=false`（默认）的 action 在一个 process 内最多跑一次。

## II.5 Goal & Condition

```go
type Goal struct {
    Name        string
    Description string
    Pre         []string      // 显式 precondition keys（除了 Inputs）
    Inputs      []IOBinding
    OutputType  *string
    Value       CostFunc
    Tags, Examples []string   // (尚未被框架消费, 留给 MCP 暴露用)
}
g.Preconditions() // → Pre + Inputs 合并的 EffectSpec
```

`Condition` 是用户自定义的命名谓词：
```go
type Condition interface {
    Name() string
    Cost() float64               // 评估成本（planner 选择更便宜的分支）
    Evaluate(ctx, oc) Determination
}
```

`core.And(a, b)` / `core.Or(a, b)` / `core.Not(c)` 提供布尔组合，自带短路。

## II.6 Blackboard — 类型化共享内存

```go
type Blackboard interface {
    ID() string
    Set(key, value)              // 命名 + 追加 objects
    Get(key) (any, bool)
    GetValue(variable, typeName) // 三种语义：
                                  //   "" / "it" → 找最新的匹配类型
                                  //   "lastResult" → 找最新对象（不论类型）
                                  //   其他 → 直接命名查（带类型校验）
    HasValue(variable, typeName) bool
    AddObject(value)              // 仅追加，不命名
    Objects() []any
    Bind(value)                   // dual-binding: "it" + 类型派生名
    BindAll(map[string]any)
    BindProtected(key, value)     // Spawn() 时保留
    Hide(target)                  // 不再被 GetValue 查到
    SetCondition / GetCondition   // 布尔条件
    Spawn() Blackboard            // 子进程使用
    Clear()                       // 保留 protected
    InfoString(verbose) string    // 调试输出
}

// 类型化助手（非接口方法，避免 Go 不支持方法泛型）：
core.Get[T](bb, "name")     // 类型安全查
core.Last[T](bb)            // 找最新的 T 类型对象
core.ObjectsOfType[T](bb)   // 所有 T 类型对象
core.Count[T](bb)
```

**Dual-binding**（autonomy 0.4 特性）：`Bind(UserInput{...})` 让该值同时落在 `"it"` 和 `"userInput"`（类型名首字母小写）两个 key 下，prompt 模板可以用任意一个名字引用。

## II.7 Process — 读视图与可变完整体

`core.Process` 是 Action / Listener 看到的**只读接口**（13 个方法），`runtime.AgentProcess` 是完整可变体。这层划分：

```go
type Process interface {
    ID, ParentID, StartedAt, Status, Goal,
    Blackboard, Options, Failure, LastWorldState() ...
    TerminateAgent(reason)
    TerminateAction(reason)
    TerminateToolCall(reason)
    AwaitInput(req Awaitable) ActionStatus
    RecordUsage(cost, tokens)
    Usage() (cost, tokens, actions)  // 子树聚合
}
```

`AgentProcess` 还有 `setStatus`/`setGoal`/`recordInvocation` 等内部 mutator（小写）和 `Tick(ctx)` 公开钩子（测试用）。

## II.8 ProcessContext — 每 tick 重建的运行时上下文

每次 action.Execute 都拿到全新的 `*core.ProcessContext`：

```go
type ProcessContext struct {
    Process       Process            // 父 process
    Blackboard    Blackboard
    Options       *ProcessOptions
    OutputChannel OutputChannel       // "say something" 流
    Services      *ServiceProvider    // open registry: chat client, RAG, etc.
    
    // 私有：runtime 注入的 hooks
    publishEvent   EventPublisher
    resolveTools   ToolResolver
    toolCallCancel ToolCallCanceller
    lastErr        error
}

// 主要方法：
pc.Publish(event)                  // 走 multicast
pc.ResolveTools(ctx, "search")     // 通过 ToolGroupResolver
pc.ToolCallContext(parent)         // 返回 (ctx, cleanup)，TerminateToolCall 触发 cancel
pc.AwaitInput(req)                 // 委托给 Process
pc.RecordUsage(cost, tokens)       // 委托给 Process
```

构造通过 `ProcessContextConfig`（runtime 填好后调 `core.NewProcessContext(deps)`）——这避免在公共 API 上散布 setter。

## II.9 HITL 三件套

```go
// L0：非泛型根接口（让 core.Process.AwaitInput 不依赖 hitl 包）
type core.Awaitable interface {
    ID() string
    PromptAny() any
    OnResponseAny(any) (ResponseImpact, error)
}

// L1：泛型典型实现
type hitl.TypedRequest[P, R any] struct {
    IDStr   string
    Payload P
    Handler func(R) core.ResponseImpact
}
hitl.NewTypedRequest[Form, Reply](payload, handler)
hitl.NewConfirmation[Form](payload, func(approved bool) ResponseImpact)
```

恢复时 `Platform.ResumeProcess(id, response)` 原子认领 slot，路由到 typed handler，返回 `ResponseImpact`（`Unchanged` / `Updated`），调用方据此决定是否重启 run loop。

## II.10 ToolGroup — 跨进程工具的懒加载

```go
type ToolGroup interface {
    Metadata() ToolGroupMetadata
    Tools(ctx) ([]AgentTool, error)
}

type ToolGroupResolver interface {
    Resolve(ctx, req ToolGroupRequirement) (ToolGroup, error)
}

// 自带实现：
core.NewLazyToolGroup(meta, loadFn)        // first call → loadFn，结果缓存
core.NewStaticToolGroupResolver()          // map[role]ToolGroup
```

`TerminationScope` 是 0.4 特性，控制工具失效时的爆炸半径：

| Scope | 行为 |
|---|---|
| `TerminationScopeAgent` | 整个 process 终止 |
| `TerminationScopeAction` | 跳过当前 action，重新规划 |
| `TerminationScopeToolCall` | 取消 in-flight tool 调用，action body 继续（**注意：lynx 把取消 ctx 铺好了，但 tool loop runner 还没实现**——参见 [`EMBABEL_COMPARISON.md`](./EMBABEL_COMPARISON.md) gap 清单 ToolLoop） |

---

# Part III — 运行时机制

## III.1 Platform 生命周期

```go
p := agent.NewPlatform(&runtime.PlatformConfig{
    ChatClient: chatClient,       // optional, 共享 LLM 客户端
    Extensions: []core.Extension{ // 所有 pluggable 都走这里
        toolResolver,             // 实现 core.ToolGroupResolver
        idGen,                    // 实现 core.IDGenerator (默认 UUIDv4)
        myPlanner,                // 实现 plan.Planner (默认 goap)
        myBlackboard,             // 实现 core.Blackboard (默认 in-memory)
        debugLogger{},            // 实现 runtime.EventListener
    },
})

p.Deploy(agent)                  // 校验 + 注册（重复 deploy 同名会替换）
p.Undeploy(name)
p.AddListener(l) / p.RemoveListener(l)
p.RunAgent(ctx, agent, bindings, opts)    // 同步：阻塞到 process 终止
p.StartAgent(ctx, agent, bindings, opts)  // 异步：返回 (proc, <-chan error)
p.GetProcess(id) / p.ActiveProcesses()
p.KillProcess(id)
p.ResumeProcess(id, response)
p.CreateChildProcess(agent, parent, opts) // 子 agent，blackboard 默认继承
```

`Deploy` 做两层校验：
1. `core.ValidateAgent(a)` — 名字非空、有 action、有 goal、action/goal name 唯一
2. `runtime.checkGoalsReachable(a)` — 每个 goal 的 True 类型 precondition 至少有一个 action 的 effect 能满足或来自 input binding

未通过 → `Deploy` 返回 error，process 永远不创建。

## III.2 AgentProcess tick 走查

`run(ctx)` 主循环（`runtime/run.go`）：

```go
func (p *AgentProcess) run(ctx context.Context) error {
    if !p.makeRunning() { return nil }                  // 已运行过
    if err := p.validateAgentForRun(); err != nil {     // GOAP 必须有 goal
        p.failProcess(err); return err
    }
    for {
        if ctx.Err() != nil { p.markCancelled(...); return }
        if p.checkEarlyTermination() { return nil }     // budget / max actions
        if err := p.Tick(ctx); err != nil { return err }
        if p.Status().IsTerminal() {
            p.publishTerminalEvent(); return nil
        }
    }
}
```

`Tick(ctx)` 单次：

```go
func (p *AgentProcess) Tick(ctx context.Context) error {
    ctx = core.WithProcess(ctx, p)                       // 注入 ctx
    if signal := p.drainTerminate(); signal != nil {     // 异步终止信号
        return p.handleTerminationSignal(*signal)
    }
    ctx, span := p.startTickSpan(ctx, "lynx.agent.tick")
    defer span.End()
    
    ws := p.observe(ctx, span)                           // OBSERVE
    if p.options.ProcessType == core.ProcessConcurrent {
        return p.tickConcurrent(ctx, ws)                 // 并行可达 actions
    }
    return p.tickSimple(ctx, ws)                         // 单 action
}
```

`tickSimple`：

```go
func (p *AgentProcess) tickSimple(ctx context.Context, ws core.WorldState) error {
    plan, err := p.formulatePlan(ctx, ws)                // ORIENT + DECIDE
    if err != nil { p.failProcess(err); return nil }
    if plan == nil { return p.handleStuck(ctx, ws) }     // planner 没解
    if plan.IsComplete() { p.completeForGoal(plan.Goal); return nil }
    
    p.setGoal(plan.Goal)
    p.publishEvent(event.PlanFormulated{...})
    
    action := plan.Actions[0]
    status, replan := p.executeAction(ctx, action)       // ACT (with retry)
    if replan != nil { p.applyReplan(action, replan); return nil }
    p.translateActionStatus(action, status)              // → process status
    return nil
}
```

## III.3 GOAP A\* 算法

`agent/planner/goap/astar.go`：

```
PlanToGoal(ctx, start, system, goal, opts):
    if isGoalSatisfied(start, goal): return empty plan (already done)
    candidates ← system.Actions \ opts.ExcludedActions, 按 |Preconditions| 降序
    if !goalReachable(start, candidates, goal): return nil  // 一步反向扫描短路
    
    best, cameFrom, iter, err ← searchForGoal(ctx, start, candidates, goal, maxIter)
    if best == nil: return nil
    
    path ← reconstructPath(cameFrom, best.HashKey, start.HashKey)
    path ← backwardOptimize(path, start, goal)  // 删冗余 action
    path ← forwardOptimize(path, start)         // 删 no-op action
    return Plan{Actions: path, Goal: goal}
```

`searchForGoal` 是标准 A\*：openList 是 min-heap on `fScore = gScore + heuristic`，heuristic 是"未满足 precondition 数量"（admissible，保证最优）。closed set 用 `HashKey()` 去重。

`expandNeighbors` 每次扩展用 `meta.IsApplicableIn(currentState)` 过滤可行 action，每个生成新 WorldState 通过 `state.Apply(meta.Effects)`。`tentativeG = current.gScore + meta.Cost(start)`（成本采样在 start 状态保持稳定）。

**Best-value 选择**：`PlansToGoals` 给每个 goal 跑一遍 `PlanToGoal`，按 `NetValue(start) = goal.Value - plan.Cost` 降序排，取第一个。

## III.4 终止三粒度

| 信号 | 触发 | 处理点 |
|---|---|---|
| `TerminationScopeAgent` | `proc.TerminateAgent(reason)` | `handleTerminationSignal` → `StatusTerminated` + 事件 |
| `TerminationScopeAction` | `proc.TerminateAction(reason)` | `handleTerminationSignal` → 跳过当前 tick，直接发 `ReplanRequested` |
| `TerminationScopeToolCall` | `proc.TerminateToolCall(reason)` | `atomic.Pointer[CancelFunc]` 触发：之前从 `pc.ToolCallContext()` 拿到的 ctx 收到 cancel |

前两个是**信号 channel**异步传递，下一个 tick 边界处理；第三个是**立即原子触发**，因为 tool 调用通常在 action body 内同步等待，必须立刻打断。

## III.5 StuckHandler 重新规划

planner 返回 nil plan（GOAP 找不到路径）时 →

```go
func handleStuck(ctx, ws):
    if agent.StuckHandler != nil:
        result := agent.StuckHandler.HandleStuck(ctx, p)
        if result.Code == StuckReplan:
            p.clearExclusions()  // 重置排除列表
            return nil           // 下一个 tick 重新 plan
    p.setStatus(StatusStuck)     // 没有 handler → 终止
    p.publishEvent(ProcessStuck{...})
```

`StuckHandler` 通常会在 blackboard 上做点改动（放松 condition、改 goal、添加缺失输入）然后返回 `StuckReplan` 让 planner 再试。

## III.6 子进程 Budget 汇总

```go
// AgentProcess.Usage() 递归：
func (p *AgentProcess) Usage() (cost float64, tokens int, actions int) {
    p.mu.RLock(); defer p.mu.RUnlock()
    cost, tokens, actions = p.ownCost, p.ownTokens, len(p.history)
    for _, child := range p.children {
        c, t, a := child.Usage()  // 递归
        cost += c; tokens += t; actions += a
    }
    return
}
```

`Platform.CreateChildProcess(agent, parent, opts)` 时 child 加入 parent.children，`BudgetPolicy{Budget: ...}` 自动看整个子树的累计开销——单条 process 内的预算覆盖整个 delegation tree。

## III.7 事件多播 — Snapshot 模式

```go
func (m *Multicast) OnEvent(e Event) {
    m.mu.RLock()
    listeners := make([]Listener, len(m.listeners))
    copy(listeners, m.listeners)  // 在锁内 snapshot
    m.mu.RUnlock()
    
    for _, l := range listeners {
        safeDeliver(l, e)         // 锁外分发：慢 listener 不阻塞 Add/Remove
    }
}
```

`safeDeliver` 用 `defer recover()` 隔离 panic — 一个 listener 崩溃不影响其他。

事件类型（17 种，全部实现 `json.Marshaler` envelope 模式）：
- 平台：`AgentDeployed` / `AgentUndeployed`
- 进程：`ProcessCreated` / `ProcessCompleted` / `ProcessFailed` / `ProcessStuck` / `ProcessKilled` / `ProcessTerminated` / `ProcessWaiting`
- 规划：`ReadyToPlan` / `PlanFormulated` / `ReplanRequested`
- 执行：`ActionExecutionStart` / `ActionExecutionResult` / `GoalAchieved`
- LLM/RAG：`LLMRequestEvent` / `LLMResponseEvent` / `ObjectBoundEvent` / `ProcessPausedEvent`（保留为集成扩展点；框架不主动发射）

---

# Part IV — 使用指南

## IV.1 5 分钟 quick-start

```go
package main

import (
    "context"
    "fmt"
    "github.com/Tangerg/lynx/agent"
    "github.com/Tangerg/lynx/agent/core"
    "github.com/Tangerg/lynx/agent/runtime"
)

type Topic struct{ Title string }
type Post  struct{ Body  string }

func main() {
    a := agent.New("Hello").
        Description("A toy agent that wraps a topic into a post").
        Actions(agent.NewAction[Topic, Post](
            "write",
            func(ctx context.Context, pc *core.ProcessContext, t Topic) (Post, error) {
                return Post{Body: "About " + t.Title}, nil
            },
            core.ActionConfig{},
        )).
        Goals(agent.GoalProducing[Post](core.Goal{Description: "post produced"})).
        Build()

    p := agent.NewPlatform(&runtime.PlatformConfig{})
    if err := p.Deploy(a); err != nil { panic(err) }

    proc, _ := p.RunAgent(context.Background(), a,
        map[string]any{core.DefaultBindingName: Topic{Title: "agents"}},
        core.ProcessOptions{},
    )
    post, _ := core.ResultOfType[Post](proc)
    fmt.Println(post.Body) // → "About agents"
}
```

跑通后看 `examples/blog/main.go` 学多步规划（research → outline → write）。

## IV.2 多步规划 — Action 链

```go
type Topic struct{ Title string }
type Research struct{ Sources []string }
type Outline  struct{ Sections []string }
type BlogPost struct{ Body string }

agent.New("BlogAgent").
    Actions(
        agent.NewAction[Topic, Research](
            "research",
            func(ctx context.Context, pc *core.ProcessContext, t Topic) (Research, error) {
                return Research{Sources: []string{"https://a.com/" + t.Title}}, nil
            },
            core.ActionConfig{Cost: core.Static(2.0)},  // research 比较贵
        ),
        agent.NewAction[Research, Outline](
            "outline",
            func(ctx context.Context, pc *core.ProcessContext, r Research) (Outline, error) {
                return Outline{Sections: []string{"intro", "body", "outro"}}, nil
            },
            core.ActionConfig{},
        ),
        agent.NewAction[Outline, BlogPost](
            "write",
            func(ctx context.Context, pc *core.ProcessContext, o Outline) (BlogPost, error) {
                return BlogPost{Body: strings.Join(o.Sections, "\n")}, nil
            },
            core.ActionConfig{},
        ),
    ).
    Goals(agent.GoalProducing[BlogPost](core.Goal{Description: "blog produced"})).
    Build()
```

GOAP 自动发现执行顺序 `research → outline → write` —— 因为每个 action 的 input type 决定 precondition，output type 决定 effect。**用户不写 sequence**，只写"什么进来什么出去"。

## IV.3 自定义 Condition

```go
isLoggedIn := core.NewCondition("logged_in", func(ctx context.Context, oc *core.OperationContext) core.Determination {
    user, ok := core.Get[User](oc.Blackboard, "currentUser")
    return core.FromBool(ok && user.Authenticated)
})

agent.New("...").
    Conditions(isLoggedIn).
    Actions(
        agent.NewAction[Query, Result]("search", searchFn, core.ActionConfig{
            Pre: []string{"logged_in"},  // 必须 logged_in 才能 search
        }),
    ).
    /* ... */
```

复合 condition：`core.And(c1, c2)` / `core.Or(c1, c2)` / `core.Not(c)`。

## IV.4 HITL — 暂停/恢复

```go
// 在 action body 中：
agent.NewAction[Draft, Approved]("review", func(ctx context.Context, pc *core.ProcessContext, d Draft) (Approved, error) {
    req := hitl.NewConfirmation(d, func(approved bool) core.ResponseImpact {
        if approved {
            pc.Blackboard.Bind(Approved{Text: d.Text})
            return core.ResponseImpactUpdated
        }
        return core.ResponseImpactUnchanged
    })
    pc.AwaitInput(req)  // → ActionWaiting，process 转 StatusWaiting
    return Approved{}, nil  // 占位返回，blackboard 在 handler 里写
}, core.ActionConfig{})

// 应用层（HTTP handler 等）：
proc, _ := platform.StartAgent(ctx, agent, bindings, opts)
// process 跑到 ReviewAction 时变成 Waiting，等待外部输入
// ... UI 收到用户点击 "approve" ...
impact, _ := platform.ResumeProcess(proc.ID(), true)
if impact == core.ResponseImpactUpdated {
    // 重新 start agent 让 planner 看到新状态
    platform.StartAgent(ctx, agent, nil, opts)  // 用同一 blackboard
}
```

`hitl.NewTypedRequest[FormSchema, FormResponse](...)` 用于复杂表单。

## IV.5 工具集成（ToolGroup）

```go
// 启动时配置 resolver：
toolResolver := core.NewStaticToolGroupResolver()
toolResolver.Register("web_search", core.NewLazyToolGroup(meta, func(ctx) ([]core.AgentTool, error) {
    return []core.AgentTool{
        {
            Name: "search",
            Description: "Search the web",
            InputSchema: `{"query": "string"}`,
            Call: func(ctx, args string) (string, error) {
                return doSearch(args)
            },
        },
    }, nil
}))

p := agent.NewPlatform(&runtime.PlatformConfig{Tools: toolResolver})

// 在 action body 中按 role 拉取：
tools, err := pc.ResolveTools(ctx, "web_search")
// 然后把 tools 传给你的 LLM 客户端

// 取消支持：
toolCtx, release := pc.ToolCallContext(ctx)
defer release()
result, err := llm.CallWithTools(toolCtx, tools)
// proc.TerminateToolCall("user cancelled") 会让 toolCtx.Done() 触发
```

## IV.6 子 agent 委托

```go
parentAction := agent.NewAction[Task, Result]("delegate", func(ctx context.Context, pc *core.ProcessContext, t Task) (Result, error) {
    childAgent := /* 构建一个 specialist agent */
    childProc, err := platform.CreateChildProcess(childAgent, pc.Process.(*runtime.AgentProcess), core.ProcessOptions{})
    if err != nil { return Result{}, err }
    
    // 同步等子 agent 完成
    if err := childProc.Tick(ctx); err != nil { /* 或者你的循环逻辑 */ }
    
    r, _ := core.ResultOfType[Result](childProc)
    return r, nil
}, core.ActionConfig{})
```

子进程 `Usage()` 自动滚入父——父的 `BudgetPolicy` 看到全 tree 的开销。Blackboard 默认 `Spawn()`（拷贝命名 + protected + objects），可以传 `opts.Blackboard` 自定义。

## IV.7 早期终止策略

每个 tick 边界，runtime 会问每一个生效的 `core.EarlyTerminationPolicy`——任意 policy 返回 `true` → `StatusTerminated` + `ProcessTerminated`。Budget 永远隐式参与（从 `ProcessOptions.Budget` 派生），额外 policy 通过 extension 注册：

```go
// Budget 之外再加一道 step-level guardrail 和一个自定义规则
core.ProcessOptions{
    Budget: core.Budget{
        CostLimit:   2.00,   // USD
        TokenLimit:  100_000,
        ActionLimit: 30,
    },
    Extensions: []core.Extension{
        myStepBudget,    // 实现 core.EarlyTerminationPolicy，名字别和 "budget-policy" 重
        myCustomPolicy,  // 同上
    },
}
```

`core.EarlyTerminationPolicy` 已经嵌入 `core.Extension`，所以实现里得提供 `Name() string`。Budget 检查永远先跑（其他 policy 是叠加而非替代）；想完全禁用 Budget 就把 `Budget` 字段置零或传宽松值。

## IV.8 事件监听

```go
type debugLogger struct{}
func (debugLogger) OnEvent(e event.Event) {
    fmt.Printf("[%s] %s\n", e.EventName(), e.ProcessID())
}

p := agent.NewPlatform(&runtime.PlatformConfig{
    Listeners: []event.Listener{debugLogger{}},
})

// 或者用闭包：
p.AddListener(event.ListenerFunc(func(e event.Event) {
    if r, ok := e.(event.ActionExecutionResult); ok && r.Err != nil {
        metrics.Counter("action.failed").Inc()
    }
}))

// JSON 序列化（每个 event 实现 json.Marshaler）：
data, _ := json.Marshal(evt)  // 含 envelope: {"event":"...", "timestamp":..., "payload":{...}}
```

## IV.9 Service 注入（DI）

`core.ServiceProvider` 是开放的 `map[string]any`，框架不预设槽位：

```go
// 启动时：
svcs := core.NewServiceProvider()
svcs.Set("chat",     openaiClient)
svcs.Set("embedder", embeddingClient)
svcs.Set("rag",      ragPipeline)
svcs.Set("logger",   slog.Default())

p := agent.NewPlatform(&runtime.PlatformConfig{Services: svcs})

// 在 action body 中类型安全取：
chat, ok := core.ServiceOf[*openai.Client](pc.Services, "chat")
if !ok { return Result{}, errors.New("chat service not configured") }
```

`ServiceOf[T]` 在 key 不存在或类型不匹配时都返回 `(zero, false)`。

## IV.10 自定义 Blackboard / Planner / IDGenerator

通过 `runtime.PlatformConfig` 或 `core.ProcessOptions` 注入：

```go
// 注册自定义 planner 和 ID 生成器作为 extension
p := agent.NewPlatform(&runtime.PlatformConfig{
    Extensions: []core.Extension{
        myCustomPlanner,  // 实现 plan.Planner，Name() 例如 "my-planner"
        myCounterGen,     // 实现 core.IDGenerator
    },
})

// 让 agent 选用自定义 planner（按 Name() 匹配）：
a := agent.New("agentX").PlannerName("my-planner").
    Actions(...).Goals(...).Build()

// 单个 process 用持久化 blackboard（per-call 直接传实例）：
p.RunAgent(ctx, a, bindings, core.ProcessOptions{
    Blackboard: redisBackedBlackboard,  // 实现 core.Blackboard
})
```

---

# Part V — 构建更高层库

## V.1 包装 Action — Decorator 模式

```go
// 通用：给任意 typed action 加 telemetry
func Traced[In, Out any](inner core.Action) core.Action {
    meta := inner.Metadata()
    return &tracedAction[In, Out]{inner: inner, meta: meta}
}

type tracedAction[In, Out any] struct {
    inner core.Action
    meta  core.ActionMetadata
}

func (a *tracedAction[In, Out]) Metadata() core.ActionMetadata { return a.meta }
func (a *tracedAction[In, Out]) Execute(ctx context.Context, pc *core.ProcessContext) core.ActionStatus {
    start := time.Now()
    defer slog.Info("action", "name", a.meta.Name, "duration", time.Since(start))
    return a.inner.Execute(ctx, pc)
}

// 使用：
agent.New("...").Actions(Traced[Topic, Post](agent.NewAction(...)))
```

## V.2 领域 DSL — 在 Builder 上面加一层

```go
// 域：聊天机器人
type ChatBotBuilder struct {
    inner *dsl.Builder
}

func NewChatBot(name string) *ChatBotBuilder {
    return &ChatBotBuilder{inner: dsl.New(name)}
}

func (b *ChatBotBuilder) Greet(template string) *ChatBotBuilder {
    b.inner.Actions(agent.NewAction[Hello, Greeting]("greet",
        func(ctx, pc, h Hello) (Greeting, error) {
            return Greeting{Text: fmt.Sprintf(template, h.Name)}, nil
        }, core.ActionConfig{}))
    return b
}

func (b *ChatBotBuilder) Build() *core.Agent {
    return b.inner.
        Goals(agent.GoalProducing[Greeting](core.Goal{Description: "greeted"})).
        Build()
}

// 用户视角：
bot := NewChatBot("greeter").Greet("Hello, %s!").Build()
```

## V.3 LLM Agent 模板

```go
// chatcomplete: 通用"调 LLM 生成结构化输出"模板
package chatcomplete

import (
    "github.com/Tangerg/lynx/agent"
    "github.com/Tangerg/lynx/agent/core"
    "github.com/Tangerg/lynx/core/model/chat"
)

type Prompt struct{ Text string }

// LLMAction 生成一个 typed action，调 chat client，写到 blackboard 上
func LLMAction[Out any](name, system string, schema string) core.Action {
    return agent.NewAction[Prompt, Out](
        name,
        func(ctx context.Context, pc *core.ProcessContext, p Prompt) (Out, error) {
            client, ok := core.ServiceOf[chat.Model](pc.Services, "chat")
            if !ok { return *new(Out), errors.New("no chat client") }
            
            // 用 ToolCallContext 让 TerminateToolCall 能取消
            llmCtx, release := pc.ToolCallContext(ctx)
            defer release()
            
            resp, err := client.Call(llmCtx, system, p.Text, schema)
            if err != nil { return *new(Out), err }
            
            // 上报 usage 给 budget tracker
            pc.RecordUsage(resp.Cost, resp.Tokens)
            
            var out Out
            if err := json.Unmarshal([]byte(resp.Content), &out); err != nil {
                return *new(Out), err
            }
            return out, nil
        },
        core.ActionConfig{
            Cost: core.Static(5.0),    // LLM 比较贵
            QoS:  core.DefaultActionQos(),  // 5 retry
        },
    )
}

// 用户视角：
agent.New("structured-extraction").
    Actions(chatcomplete.LLMAction[Invoice]("extract", "Extract invoice fields", invoiceSchema)).
    Goals(agent.GoalProducing[Invoice](core.Goal{Description: "extracted"})).
    Build()
```

## V.4 RAG Agent 模板

```go
// 在 rag/ 模块（lynx 顶层 module）之上构建 agent action：
import "github.com/Tangerg/lynx/rag"

func RAGRetrieveAction(name string) core.Action {
    return agent.NewAction[Query, RetrievedDocs](
        name,
        func(ctx context.Context, pc *core.ProcessContext, q Query) (RetrievedDocs, error) {
            pipeline, ok := core.ServiceOf[rag.Pipeline](pc.Services, "rag")
            if !ok { return RetrievedDocs{}, errors.New("no rag pipeline") }
            
            docs, err := pipeline.Retrieve(ctx, q.Text)
            if err != nil { return RetrievedDocs{}, err }
            return RetrievedDocs{Docs: docs}, nil
        },
        core.ActionConfig{
            Cost: core.Static(1.0),
            Pre: []string{"rag_index_built"},  // 显式 precondition
        },
    )
}
```

## V.5 多 Agent Supervisor 模式

```go
// supervisor agent 用 LLM 决定调哪个子 agent
type AgentRoute struct{ Name string }

func RouterAction(specialists map[string]*core.Agent) core.Action {
    return agent.NewAction[UserRequest, AgentResult](
        "route",
        func(ctx context.Context, pc *core.ProcessContext, req UserRequest) (AgentResult, error) {
            // 1. 让 LLM 选 sub-agent
            chat, _ := core.ServiceOf[chat.Model](pc.Services, "chat")
            choice, _ := chat.Choose(ctx, req, keys(specialists))
            
            sub, ok := specialists[choice.Name]
            if !ok { return AgentResult{}, fmt.Errorf("unknown sub-agent %q", choice.Name) }
            
            // 2. 委托给子 agent
            platform := pc.Services.Get("platform").(*runtime.Platform)
            childProc, err := platform.CreateChildProcess(sub,
                pc.Process.(*runtime.AgentProcess),
                core.ProcessOptions{})
            if err != nil { return AgentResult{}, err }
            
            // 3. 同步等待 + 收割 result
            // ... 跑 childProc 直到终止 ...
            result, _ := core.ResultOfType[AgentResult](childProc)
            return result, nil
        },
        core.ActionConfig{Cost: core.Static(1.0)},
    )
}
```

> **未来更好的方案**：等 SupervisorAgent 模式（参见 [`EMBABEL_COMPARISON.md`](./EMBABEL_COMPARISON.md)）落地后，把这个抽到 `agent/orchestration` 子包标准化。

## V.6 测试基础设施

```go
package agenttest  // 你在自己的 lib 里写

import (
    "github.com/Tangerg/lynx/agent/core"
    "github.com/Tangerg/lynx/agent/runtime"
)

// MockBlackboard 让测试快速断言绑定
type MockBlackboard struct {
    *runtime.AgentProcess  // 复用 inMemoryBlackboard 实现
}

// CounterIDGen 给测试用确定性 ID
func DeterministicIDs(prefix string) runtime.IDGenerator {
    return runtime.NewCounterIDGenerator(prefix)
}

// RecordAllEvents 抓所有事件供断言
type EventRecorder struct {
    mu     sync.Mutex
    events []event.Event
}
func (r *EventRecorder) OnEvent(e event.Event) {
    r.mu.Lock(); defer r.mu.Unlock()
    r.events = append(r.events, e)
}

// AssertGoalAchieved 断言指定 goal 完成
func AssertGoalAchieved[T any](t *testing.T, proc *runtime.AgentProcess) T {
    t.Helper()
    if proc.Status() != core.StatusCompleted {
        t.Fatalf("expected completed, got %s; failure=%v", proc.Status(), proc.Failure())
    }
    result, ok := core.ResultOfType[T](proc)
    if !ok { t.Fatalf("no %T on blackboard", *new(T)) }
    return result
}
```

## V.7 MCP Server 暴露（forward-looking）

> 当前 lynx/mcp 仅有客户端端，server 端是 P0-1 路线图项。一旦实装：

```go
import (
    "github.com/Tangerg/lynx/agent"
    "github.com/Tangerg/lynx/mcp/server"  // 待实现
)

// 把 lynx agent 暴露为 MCP server，让 Claude / Cursor 等可以调用
mcpServer := server.New(server.Config{
    Platform: agent.NewPlatform(...),
    // 自动从 Goal.Export.Remote=true 的 goal 生成 MCP schema
})
mcpServer.Listen(":3000")
```

构建你的 lib 时如果想 MCP-ready：在 `Goal` 上设置 `Tags`、`Examples`、`Export.Remote=true`，未来 MCP server 端能直接消费这些元数据。

## V.8 反模式与陷阱

| 反模式 | 后果 | 正确做法 |
|---|---|---|
| 在 action body 内调 `pc.Blackboard.Spawn()` | 子 blackboard 无人持有，写入丢失 | `Spawn` 是 `Platform.CreateChildProcess` 内部用的；用户用 `core.ServiceOf[T]` 拿外部依赖 |
| Action 把状态存在 closure 里 | 跨 process 共享，并发不安全 | 状态写到 `pc.Blackboard`；closure 只捕获只读配置 |
| 直接修改 `WorldState.State()` 返回的 map | 违反不可变约定，破坏 A\* closed set | 视为只读，用 `Apply()` 派生新状态 |
| Action 的 `In/Out` 用 `interface{}` | GOAP 失效（无法 derive type binding） | 用具体 struct，让 `NewIOBinding[T]` 反射出稳定类型名 |
| `Goal` 与某 action 的 `Output` type 不一致 | `checkGoalsReachable` 拒绝 deploy | `agent.GoalProducing[T]` 自动对齐 |
| 在 Listener 里调 `Platform.RunAgent`（同步） | 死锁（事件分发线程阻塞） | Listener 内只做记录；要重启 process 在外面调 |
| 把 LLM 客户端塞 `ProcessOptions.Services` | 每个 process 有自己实例，浪费连接 | 在 `PlatformConfig.Services` 一次注册，所有 process 共享 |
| 多个 Action 都写同名 condition / binding | 后写覆盖前写，planner 行为不可预测 | 命名规范化（用 type 派生 / 加前缀） |
| 忘记 `pc.RecordUsage(cost, tokens)` | `BudgetPolicy` 永远不触发 | LLM 调用后必须上报 |

---

# 速查表

## 顶层包入口（`agent.go`）

```go
agent.New(name) *dsl.Builder                            // 流式 Builder
agent.NewAction[In, Out](name, fn, cfg) core.Action     // 类型安全 action
agent.NewCondition(name, fn) *core.ComputedCondition    // 命名条件
agent.GoalProducing[T](g core.Goal) *core.Goal          // 类型化 goal
agent.NewPlatform(cfg) *runtime.Platform                // 平台
```

## 类型别名

```go
agent.Agent          = core.Agent
agent.Goal           = core.Goal
agent.Action         = core.Action
agent.ActionMetadata = core.ActionMetadata
agent.Condition      = core.Condition
agent.Blackboard     = core.Blackboard
agent.WorldState     = core.WorldState
agent.Process        = core.Process
agent.ProcessContext = core.ProcessContext
agent.Determination  = core.Determination
agent.Platform       = runtime.Platform
agent.AgentProcess   = runtime.AgentProcess
```

## Builder 链式方法

```go
b := agent.New("name")
b.Description(s)
b.Provider(s)
b.Version(s)              // semver.MustParse
b.Opaque(bool)
b.StuckHandler(h)
b.Actions(...core.Action)
b.Goals(...*core.Goal)
b.Conditions(...core.Condition)
b.DomainTypes(...core.DomainType)
b.RequiresToolGroups(...core.ToolGroupRequirement)
b.Build() *core.Agent
```

## ActionConfig 字段

```go
core.ActionConfig{
    Description     string
    Pre, Post       []string         // 显式 condition keys
    CanRerun        bool             // false = 一个 process 只跑一次
    ReadOnly        bool
    QoS             core.ActionQos   // MaxAttempts/BaseDelay/MaxDelay
    Cost            core.CostFunc    // core.Static(2.0) 或 fn
    Value           core.CostFunc
    ToolGroups      []core.ToolGroupRequirement
    Trigger         reflect.Type     // 见 core.TriggerType[T]()
    InputBinding    string
    OutputBinding   string
    Inputs          []core.IOBinding
    Outputs         []core.IOBinding
    ClearBlackboard bool
}
```

## ProcessOptions 字段

```go
core.ProcessOptions{
    Blackboard    core.Blackboard      // override 默认（per-call 直接传实例）
    Budget        core.Budget          // CostLimit/ActionLimit/TokenLimit；隐式参与 early termination
    OutputChannel core.OutputChannel   // 默认 DevNullOutputChannel
    ProcessType   core.ProcessType     // ProcessSequential（默认）/ ProcessConcurrent
    Extensions    []core.Extension     // per-process 扩展（IDGenerator / Planner / Policy / ...）
}
```

> 选 planner 不在 ProcessOptions 上 —— 走 `AgentConfig.PlannerName` / `Builder.PlannerName(name)`，agent build 时决定。EarlyTerminationPolicy 也不是单值字段，而是 OR-composable 的 extension 集合（Budget 隐式参与）。

## Blackboard 类型化助手

```go
core.Get[T](bb, name) (T, bool)       // 命名 + 类型查
core.Last[T](bb) (T, bool)            // 最新 T
core.ObjectsOfType[T](bb) []T         // 所有 T
core.Count[T](bb) int
core.ResultOfType[T](proc) (T, bool)  // 走 proc.Blackboard
```

## 常量

```go
core.DefaultBindingName    = "it"          // 默认输入名
core.LastResultBindingName = "lastResult"  // 最近添加对象的别名
core.Unknown / True / False                 // Determination
core.ResponseImpactUnchanged / ResponseImpactUpdated
core.StuckGiveUp / StuckReplan
core.TerminationScopeAgent / Action / ToolCall
core.ProcessSequential / ProcessConcurrent
core.StatusNotStarted / Running / Waiting / Paused / Completed /
     Failed / Stuck / Killed / Terminated
core.ActionSucceeded / Failed / Waiting / Paused
```

## Helpers

```go
core.Static(v float64) core.CostFunc
core.FromBool(b bool) core.Determination
core.NewIOBinding[T](name) core.IOBinding
core.TypeFullName(rt reflect.Type) string
core.TypeFullNameOf[T]() string
core.NewServiceProvider() *core.ServiceProvider
core.ServiceOf[T](sp, key) (T, bool)
core.NewCondition(name, fn) *core.ComputedCondition
core.And(a, b core.Condition) core.Condition
core.Or(a, b core.Condition) core.Condition
core.Not(c core.Condition) core.Condition
core.NewLazyToolGroup(meta, loadFn)
core.NewStaticToolGroupResolver()
core.ToolRolesFor("role1", "role2") []core.ToolGroupRequirement
core.AsReplanRequest(err) *core.ReplanRequest
core.WithProcess(ctx, p) context.Context
core.ProcessFrom(ctx) core.Process
core.AgentTracer() trace.Tracer
core.DefaultBudget() core.Budget
core.DefaultActionQos() core.ActionQos
hitl.NewTypedRequest[P, R](payload, handler)
hitl.NewConfirmation[P](payload, handler)
runtime.NewUUIDIDGenerator()
runtime.NewCounterIDGenerator(prefix)
event.NewMulticast() *event.Multicast
event.ListenerFunc(fn) event.Listener
event.NewBaseEvent(processID) event.BaseEvent
```

---

## 进一步阅读

- [`README.md`](./README.md) — 5 行 quick-start
- [`EXTENSION_DESIGN.md`](./EXTENSION_DESIGN.md) — Extension / SPI 系统当前形态
- [`EMBABEL_COMPARISON.md`](./EMBABEL_COMPARISON.md) — 与 embabel-agent 多角度对比 + gap 清单
- [`../examples/`](../examples/) — 可跑的 hello / blog 示例
