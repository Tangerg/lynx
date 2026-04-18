# 03. 规划引擎 & 执行运行时

> 本文件描述 `AStarGoapPlanner` 和 `AgentProcess` 的 Go 等价实现。

---

## 1. Plan / WorldState 基础类型

```go
// agent/plan/plan.go
package plan

import "context"

// WorldState 规划器看到的世界快照
type WorldState interface {
    // 条件到三值的映射
    State() map[string]core.Determination
    Timestamp() time.Time
    HashKey() string  // 用于 A* 的 closedSet 去重
}

// ConditionWorldState 标准实现
type ConditionWorldState struct {
    state     map[string]core.Determination
    timestamp time.Time
}

// Apply 应用 action.Effects 得到新状态（不可变返回）
func (w *ConditionWorldState) Apply(effects core.EffectSpec) *ConditionWorldState {
    next := make(map[string]core.Determination, len(w.state)+len(effects))
    for k, v := range w.state { next[k] = v }
    for k, v := range effects { next[k] = v }
    return &ConditionWorldState{state: next, timestamp: time.Now()}
}

// Plan 有序的 action 序列
type Plan struct {
    Actions []core.Action
    Goal    *core.Goal
}

func (p *Plan) IsComplete() bool     { return len(p.Actions) == 0 }
func (p *Plan) Cost(ws WorldState)   float64 { /* sum action.Cost */ }
func (p *Plan) NetValue(ws WorldState) float64 { /* goal.Value - total cost */ }

// PlanningSystem 规划器的输入集合
type PlanningSystem struct {
    Actions    []core.Action
    Goals      []*core.Goal
    Conditions []core.Condition
}

func (s *PlanningSystem) KnownConditions() map[string]struct{} {
    // 合并所有 actions.preconditions/effects 的 key + goals.preconditions 的 key
}
```

---

## 2. Planner 接口

```go
// agent/plan/planner.go
type Planner interface {
    // 单目标规划
    PlanToGoal(ctx context.Context, start WorldState, actions []core.Action, goal *core.Goal) (*Plan, error)

    // 对系统中所有 Goal 规划，按 netValue 降序返回
    PlansToGoals(ctx context.Context, start WorldState, system *PlanningSystem) ([]*Plan, error)

    // 取价值最高的计划，支持排除黑名单 action
    BestValuePlan(ctx context.Context, start WorldState, system *PlanningSystem, excluded []string) (*Plan, error)

    // 剪枝：删除无法帮助达成任何 goal 的 action
    Prune(system *PlanningSystem) *PlanningSystem
}
```

---

## 3. A* GOAP 规划器（Go 实现核心）

```go
// agent/planner/goap/astar.go
package goap

import (
    "container/heap"
    "context"
)

type AStarPlanner struct {
    maxIterations int  // 默认 10000
}

func NewAStarPlanner() *AStarPlanner {
    return &AStarPlanner{maxIterations: 10000}
}

// searchNode A* 的节点
type searchNode struct {
    state    *plan.ConditionWorldState
    gScore   float64  // 已走代价
    hScore   float64  // 启发值
    fScore   float64  // g+h，优先队列按此排序
    index    int      // heap.Interface 用
}

// openList 最小堆
type openList []*searchNode

func (o openList) Len() int             { return len(o) }
func (o openList) Less(i, j int) bool   { return o[i].fScore < o[j].fScore }
func (o openList) Swap(i, j int)        { o[i], o[j] = o[j], o[i]; o[i].index = i; o[j].index = j }
func (o *openList) Push(x any)          { /* ... */ }
func (o *openList) Pop() any            { /* ... */ }

// PlanToGoal 核心 A* 搜索
func (p *AStarPlanner) PlanToGoal(
    ctx context.Context,
    start plan.WorldState,
    actions []core.Action,
    goal *core.Goal,
) (*plan.Plan, error) {

    startCws := start.(*plan.ConditionWorldState)

    // 快速检查：已在目标状态
    if isGoalSatisfied(startCws, goal) {
        return &plan.Plan{Actions: nil, Goal: goal}, nil
    }

    // 剪枝：若 goal 不可达则直接返回 nil（避免昂贵搜索）
    if !isGoalReachable(startCws, actions, goal) {
        return nil, nil
    }

    // A* 数据结构
    var open openList
    heap.Init(&open)
    gScores  := map[string]float64{startCws.HashKey(): 0}
    cameFrom := map[string]struct{ prev *plan.ConditionWorldState; action core.Action }{}
    closed   := map[string]struct{}{}

    heap.Push(&open, &searchNode{
        state:  startCws,
        gScore: 0,
        hScore: heuristic(startCws, goal),
    })

    var bestGoalNode *searchNode
    bestGoalScore := math.Inf(+1)

    for i := 0; i < p.maxIterations && open.Len() > 0; i++ {
        if err := ctx.Err(); err != nil {
            return nil, err
        }

        current := heap.Pop(&open).(*searchNode)
        key := current.state.HashKey()
        if _, seen := closed[key]; seen { continue }
        closed[key] = struct{}{}

        if isGoalSatisfied(current.state, goal) {
            if current.gScore < bestGoalScore {
                bestGoalNode = current
                bestGoalScore = current.gScore
            }
            continue
        }

        // 按前提条件数量降序排序（更具体的 action 优先）
        sortedActions := sortActionsBySpecificity(actions)

        for _, action := range sortedActions {
            if !isActionApplicable(action, current.state) { continue }

            nextState := current.state.Apply(action.Effects())
            if nextState.HashKey() == current.state.HashKey() { continue }  // 无变化

            tentativeG := current.gScore + action.Cost(startCws)
            nextKey := nextState.HashKey()
            if tentativeG < gScores[nextKey] || gScores[nextKey] == 0 {
                cameFrom[nextKey] = struct{ prev *plan.ConditionWorldState; action core.Action }{current.state, action}
                gScores[nextKey]  = tentativeG
                heap.Push(&open, &searchNode{
                    state:  nextState,
                    gScore: tentativeG,
                    hScore: heuristic(nextState, goal),
                    fScore: tentativeG + heuristic(nextState, goal),
                })
            }
        }
    }

    if bestGoalNode == nil { return nil, nil }

    // 路径重建
    path := reconstructPath(cameFrom, bestGoalNode.state, startCws)

    // 双重优化
    path = backwardOptimization(path, startCws, goal)
    path = forwardOptimization(path, startCws, goal)

    return &plan.Plan{Actions: path, Goal: goal}, nil
}

// heuristic 启发式：未满足的 goal 条件数（可接受启发式，保证最优）
func heuristic(ws *plan.ConditionWorldState, goal *core.Goal) float64 {
    unsatisfied := 0
    for key, required := range goal.Preconditions() {
        if ws.State()[key] != required {
            unsatisfied++
        }
    }
    return float64(unsatisfied)
}

// isActionApplicable 检查 action 的 preconditions 在当前状态下是否都为 True
func isActionApplicable(action core.Action, ws *plan.ConditionWorldState) bool {
    for key, required := range action.Preconditions() {
        if ws.State()[key] != required {
            return false
        }
    }
    return true
}

// backwardOptimization 反向剪枝
func backwardOptimization(actions []core.Action, start *plan.ConditionWorldState, goal *core.Goal) []core.Action {
    // 从 goal 反推，删除不贡献任何 effect 的 action
}

// forwardOptimization 正向验证
func forwardOptimization(actions []core.Action, start *plan.ConditionWorldState, goal *core.Goal) []core.Action {
    // 正向模拟执行，删除不产生新状态的 action
}
```

**要点**：
- 用 `container/heap` 实现优先队列（Go 标准库）
- 启发式保持「未满足条件数」，算法行为与 embabel 完全一致
- `context.Context` 在长循环中周期性检查取消

---

## 4. WorldStateDeterminer — 世界状态评估器

```go
// agent/runtime/world_state_determiner.go
package runtime

type WorldStateDeterminer interface {
    DetermineWorldState(ctx context.Context) plan.WorldState
}

// BlackboardDeterminer 从黑板推导世界状态（主要实现）
type BlackboardDeterminer struct {
    agent      *core.Agent
    blackboard core.Blackboard
    logicParser *LogicalExpressionParser
}

func (d *BlackboardDeterminer) DetermineWorldState(ctx context.Context) plan.WorldState {
    state := map[string]core.Determination{}
    oc := &core.OperationContext{Bb: d.blackboard}

    // 遍历所有已知条件（来自 actions + goals + conditions）
    for condition := range d.agent.PlanningSystem().KnownConditions() {
        state[condition] = d.determineCondition(ctx, condition, oc)
    }

    return &plan.ConditionWorldState{State: state, Timestamp: time.Now()}
}

// determineCondition 按优先级解析条件
func (d *BlackboardDeterminer) determineCondition(ctx context.Context, cond string, oc *core.OperationContext) core.Determination {
    // 1. 逻辑表达式？
    if d.logicParser.IsLogical(cond) {
        return d.logicParser.Parse(cond).Evaluate(d.blackboard)
    }

    // 2. 数据绑定？（含 ":"）
    if strings.Contains(cond, ":") {
        ib := core.ParseIoBinding(cond)
        if d.blackboard.HasValue(ib.Name, ib.Type) {
            return core.True
        }
        return core.False
    }

    // 3. hasRun 前缀？
    if strings.HasPrefix(cond, "hasRun_") {
        v, ok := d.blackboard.GetCondition(cond)
        if !ok { return core.False }
        if v { return core.True }
        return core.False
    }

    // 4. 命名条件？
    for _, c := range d.agent.Conditions {
        if c.Name() == cond {
            return c.Evaluate(ctx, oc)
        }
    }

    // 5. 显式 setCondition
    if v, ok := d.blackboard.GetCondition(cond); ok {
        if v { return core.True }
        return core.False
    }

    return core.Unknown
}
```

---

## 5. AgentProcess — 执行实例状态机

### 状态机

```
NotStarted
    ↓ makeRunning()
Running  ←──────────────────────┐
    │                           │
    ├─[tick 成功] ──────────────┘
    ├─[action 失败]     → Failed
    ├─[goal 达成]       → Completed
    ├─[plan 为空]       → Stuck → handleStuck()
    ├─[action 等待]     → Waiting
    ├─[action 暂停]     → Paused
    ├─[早终止策略]      → Terminated
    └─[kill()]         → Killed
```

### Go 定义

```go
// agent/runtime/agent_process.go
package runtime

type Status int8
const (
    StatusNotStarted Status = iota
    StatusRunning
    StatusCompleted
    StatusFailed
    StatusStuck
    StatusWaiting
    StatusPaused
    StatusTerminated
    StatusKilled
)

// AgentProcess 一次 agent 执行的全部状态
type AgentProcess struct {
    ID        string
    ParentID  string
    Agent     *core.Agent
    Options   *core.ProcessOptions
    Blackboard core.Blackboard

    // 依赖注入
    platform   *Platform
    planner    plan.Planner
    determiner WorldStateDeterminer

    // 运行时状态
    mu          sync.RWMutex
    status      Status
    goal        *core.Goal
    lastWorld   plan.WorldState
    history     []ActionInvocation
    llmLog      []LLMInvocation
    failure     error
    replanBlacklist map[string]struct{}

    // 时间
    startedAt time.Time
}

type ActionInvocation struct {
    Name      string
    Timestamp time.Time
    Duration  time.Duration
}
```

### Run / Tick 主循环

```go
// agent/runtime/process_run.go

// Run 同步执行到完成/失败/卡住
func (p *AgentProcess) Run(ctx context.Context) error {
    if !p.makeRunning() { return nil }  // 幂等

    // 校验：GOAP 模式要有 Goal
    if p.Options.PlannerType == core.PlannerGOAP && len(p.Agent.Goals) == 0 {
        return errors.New("agent has no goals but planner type is GOAP")
    }

    // 首次 tick
    if err := p.Tick(ctx); err != nil {
        return err
    }

    // 循环 tick
    for p.Status() == StatusRunning {
        if p.identifyEarlyTermination() != nil {
            return nil
        }
        if err := p.Tick(ctx); err != nil {
            return err
        }
    }

    // 发布终态事件
    p.publishTerminalEvent()
    return nil
}

// Tick 执行单步
func (p *AgentProcess) Tick(ctx context.Context) error {
    // 绑定 process 到 ctx（替代 ThreadLocal）
    ctx = core.WithProcess(ctx, p)

    // Step 1: observe
    ctx, obs := p.observation.Start(ctx, "agent.tick")
    defer obs.End()

    worldState := p.determiner.DetermineWorldState(ctx)
    p.lastWorld = worldState
    p.publish(ReadyToPlanEvent{Process: p, World: worldState})

    // Step 2: orient + decide + act（子类实现）
    return p.formulateAndExecutePlan(ctx, worldState)
}
```

### SimpleAgentProcess 实现（顺序执行）

```go
// agent/runtime/simple_process.go

// formulateAndExecutePlan 顺序模式
func (p *AgentProcess) formulateAndExecutePlan(ctx context.Context, ws plan.WorldState) error {
    // 规划
    excluded := slices.Collect(maps.Keys(p.replanBlacklist))
    plan, err := p.planner.BestValuePlan(ctx, ws, p.Agent.PlanningSystem(), excluded)
    if err != nil {
        return err
    }

    // 无计划 → STUCK
    if plan == nil {
        return p.handlePlanNotFound(ws)
    }

    // 计划为空 → 目标已达成
    if plan.IsComplete() {
        p.setStatus(StatusCompleted)
        p.goal = plan.Goal
        p.publish(GoalAchievedEvent{Process: p, Goal: plan.Goal})
        return nil
    }

    // 发布 PlanFormulatedEvent
    p.publish(PlanFormulatedEvent{Process: p, Plan: plan, World: ws})

    // 只执行 plan 的第一个 action（embabel 的 SimpleAgentProcess 行为）
    action := plan.Actions[0]
    actionStatus, replanReq := p.executeAction(ctx, action)

    if replanReq != nil {
        // action 主动请求重规划（类似 ReplanRequestedException）
        p.replanBlacklist[action.Name()] = struct{}{}
        replanReq.BlackboardUpdater(p.Blackboard)
        p.publish(ReplanRequestedEvent{Process: p, Reason: replanReq.Reason})
        return nil  // 保持 RUNNING，下次 tick 重规划
    }

    p.setStatus(p.actionStatusToProcessStatus(actionStatus))
    return nil
}
```

### ConcurrentAgentProcess（并发执行）

```go
// agent/runtime/concurrent_process.go

// formulateAndExecutePlan 并发模式
func (p *AgentProcess) formulateAndExecutePlan(ctx context.Context, ws plan.WorldState) error {
    plan, err := p.planner.BestValuePlan(ctx, ws, p.Agent.PlanningSystem(), nil)
    if err != nil || plan == nil {
        return p.handlePlanNotFound(ws)
    }
    if plan.IsComplete() {
        p.setStatus(StatusCompleted)
        return nil
    }

    // 找出计划中所有当前可达的 action
    achievable := filterAchievable(plan.Actions, ws)

    // 用 errgroup 并发执行
    g, egCtx := errgroup.WithContext(ctx)
    results := make([]core.ActionStatus, len(achievable))
    for i, action := range achievable {
        i, action := i, action
        g.Go(func() error {
            status, _ := p.executeAction(egCtx, action)
            results[i] = status
            return nil
        })
    }
    if err := g.Wait(); err != nil {
        return err
    }

    // 合并状态
    p.setStatus(mergeStatuses(results))
    return nil
}

func mergeStatuses(statuses []core.ActionStatus) Status {
    // 任一 FAILED → FAILED
    // 任一 PAUSED → PAUSED
    // 任一 WAITING → WAITING
    // 都 SUCCEEDED → RUNNING（继续下一 tick）
}
```

**关键差异**：Kotlin 用 coroutines + `runBlocking`，Go 用 `errgroup` + `Wait` —— 语义等价但更直接。

---

## 6. executeAction —— 单 Action 执行（含重试）

```go
// agent/runtime/execute_action.go
func (p *AgentProcess) executeAction(ctx context.Context, action core.Action) (core.ActionStatus, *ReplanRequest) {
    // 1. 发布 start event
    startEv := ActionExecutionStartEvent{Process: p, Action: action, StartedAt: time.Now()}
    p.publish(startEv)

    // 2. 记录黑板前状态（用于检测是否被清空）
    bbBefore := p.Blackboard.Objects()

    // 3. 重试循环
    qos := action.QoS()
    var status core.ActionStatus
    var replanReq *ReplanRequest
    for attempt := 0; attempt < qos.MaxAttempts; attempt++ {
        pc := &core.ProcessContext{
            Process:    p,
            Blackboard: p.Blackboard,
            // ... 填充其他字段
        }

        // 给 Action.Execute 一个 panic 防护
        func() {
            defer func() {
                if r := recover(); r != nil {
                    err := fmt.Errorf("action %s panicked: %v", action.Name(), r)
                    p.setFailure(err)
                    status = core.ActionFailed
                }
            }()
            status = action.Execute(ctx, pc)
        }()

        // 检测 ReplanRequest（Action 可通过 panic 或 error 传递）
        if rr, ok := p.consumeReplanRequest(); ok {
            replanReq = rr
            break
        }

        if status == core.ActionSucceeded { break }
        if !qos.ShouldRetry(status) { break }

        // 指数退避
        time.Sleep(backoff(qos, attempt))
    }

    // 4. 记录历史
    p.history = append(p.history, ActionInvocation{
        Name:      action.Name(),
        Timestamp: time.Now(),
        Duration:  time.Since(startEv.StartedAt),
    })

    // 5. 设置 hasRun 条件（canRerun=false 专用）
    bbCleared := isBlackboardCleared(bbBefore, p.Blackboard.Objects())
    if !bbCleared {
        p.Blackboard.SetCondition(hasRunKey(action.Name()), true)
    }

    // 6. 发布结果 event
    p.publish(ActionExecutionResultEvent{
        Process: p, Action: action, Status: status, Duration: time.Since(startEv.StartedAt),
    })

    return status, replanReq
}
```

---

## 7. Replan 请求机制

embabel 用 `ReplanRequestedException` 让 Action 主动请求重规划。Go 风格用 **error 传递**或**context**：

### 方式 A：专用 error 类型（推荐）

```go
// agent/core/replan.go
type ReplanRequest struct {
    Reason            string
    BlackboardUpdater func(core.Blackboard)
}

// 实现 error 接口便于 errors.As
func (r *ReplanRequest) Error() string { return "replan requested: " + r.Reason }

// Action 代码：
func MyAction(ctx context.Context, pc *core.ProcessContext, input Foo) (Bar, error) {
    if shouldReplan(...) {
        return Bar{}, &ReplanRequest{
            Reason: "routing condition changed",
            BlackboardUpdater: func(bb core.Blackboard) { bb.Set("route", "alternate") },
        }
    }
    // 正常逻辑
    return Bar{...}, nil
}
```

运行时检查：
```go
var rr *ReplanRequest
if errors.As(err, &rr) {
    replanReq = rr
}
```

---

## 8. 事件系统

```go
// agent/event/event.go
package event

// Event 所有 agent 事件的基接口
type Event interface {
    Timestamp() time.Time
    ProcessID() string
}

// 具体事件类型（类似 embabel 的事件层次）
type (
    AgentDeployedEvent       struct{ AgentName string; At time.Time }
    ProcessCreatedEvent      struct{ ProcessID string; At time.Time }
    ReadyToPlanEvent         struct{ ProcessID string; World plan.WorldState; At time.Time }
    PlanFormulatedEvent      struct{ ProcessID string; Plan *plan.Plan; At time.Time }
    ActionExecutionStartEvent struct{ ProcessID string; Action core.Action; At time.Time }
    ActionExecutionResultEvent struct{ ProcessID string; Action core.Action; Status core.ActionStatus; Duration time.Duration }
    GoalAchievedEvent        struct{ ProcessID string; Goal *core.Goal; At time.Time }
    ProcessCompletedEvent    struct{ ProcessID string; At time.Time }
    ProcessFailedEvent       struct{ ProcessID string; Err error; At time.Time }
    ReplanRequestedEvent     struct{ ProcessID string; Reason string; At time.Time }
    LLMRequestEvent          struct{ ProcessID string; Prompt string; Model string }
)

// Listener 事件监听器
type Listener interface {
    OnEvent(e Event)
}

// Multicast 多播（线程安全）
type Multicast struct {
    mu        sync.RWMutex
    listeners []Listener
}

func (m *Multicast) Add(l Listener) { ... }
func (m *Multicast) OnEvent(e Event) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    for _, l := range m.listeners { l.OnEvent(e) }
}
```

**集成 observation**：
可以写一个 `ObservationListener`，把事件自动转为 observation span/event，让 embabel 的丰富事件模型无缝接入 Lynx 观测体系。

---

## 9. Platform — 平台入口

```go
// agent/runtime/platform.go
package runtime

type Platform struct {
    mu          sync.RWMutex
    agents      map[string]*core.Agent
    processType core.ProcessType  // SIMPLE / CONCURRENT
    plannerFactory PlannerFactory
    events      *event.Multicast
    processRepo ProcessRepository

    // 集成 Lynx core
    chatClient  *chat.Client
    observations observation.Registry
}

// NewPlatform 构造器
func NewPlatform(opts ...PlatformOption) *Platform { ... }

type PlatformOption func(*Platform)

func WithChatClient(c *chat.Client) PlatformOption      { ... }
func WithObservation(r observation.Registry) PlatformOption { ... }
func WithPlannerFactory(f PlannerFactory) PlatformOption { ... }
func WithProcessType(t core.ProcessType) PlatformOption { ... }

// Deploy 注册 agent
func (p *Platform) Deploy(agent *core.Agent) error {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.agents[agent.Name] = agent
    p.events.OnEvent(event.AgentDeployedEvent{AgentName: agent.Name})
    return nil
}

// RunAgent 同步执行
func (p *Platform) RunAgent(ctx context.Context, agent *core.Agent, bindings map[string]any, opts ...core.ProcessOptionFunc) (*AgentProcess, error) {
    proc, err := p.createProcess(agent, bindings, opts...)
    if err != nil { return nil, err }
    err = proc.Run(ctx)
    return proc, err
}

// StartAgent 异步执行（返回 channel 供等待）
func (p *Platform) StartAgent(ctx context.Context, agent *core.Agent, bindings map[string]any, opts ...core.ProcessOptionFunc) (*AgentProcess, <-chan error) {
    proc, _ := p.createProcess(agent, bindings, opts...)
    done := make(chan error, 1)
    go func() {
        done <- proc.Run(ctx)
        close(done)
    }()
    return proc, done
}

// CreateChildProcess 子 Agent 调用
func (p *Platform) CreateChildProcess(agent *core.Agent, parent *AgentProcess) (*AgentProcess, error) {
    child, err := p.createProcess(agent, nil)
    if err != nil { return nil, err }
    child.ParentID   = parent.ID
    child.Blackboard = parent.Blackboard.Spawn()  // 继承父黑板
    return child, nil
}

func (p *Platform) KillProcess(id string) error { ... }
func (p *Platform) GetProcess(id string) (*AgentProcess, bool) { ... }
```

---

## 10. OODA 循环可视化

```
┌────── Tick begins (one iteration) ──────┐
│                                         │
│  OBSERVE                                │
│    determiner.DetermineWorldState(ctx)  │
│    → ConditionWorldState                │
│         ↓                               │
│  ORIENT                                 │
│    planner.BestValuePlan(start, system) │
│    → Plan (ordered actions)             │
│         ↓                               │
│  DECIDE                                 │
│    SimpleProc: pick plan.Actions[0]     │
│    ConcurrentProc: pick all achievable  │
│         ↓                               │
│  ACT                                    │
│    action.Execute(ctx, pc) with retry   │
│    → ActionStatus                       │
│         ↓                               │
│    write result to blackboard           │
│    set hasRun_<action> = true           │
│    record in history                    │
│                                         │
└─── if RUNNING: next Tick; else: exit ───┘
```

---

## 11. 与 embabel 的主要差异小结

| 方面 | embabel (Kotlin) | Lynx Agent (Go) |
|-----|-----------------|-----------------|
| **当前 process 访问** | `AgentProcess.get()` ThreadLocal | `core.ProcessFrom(ctx)` |
| **Action 类型参数** | 方法反射从 JVM 签名读 | `NewAction[In, Out]` 泛型函数 |
| **并发** | Kotlin coroutines + `runBlocking` | `errgroup` + `Wait` |
| **Replan 请求** | `throw ReplanRequestedException` | `return &ReplanRequest{}` (error 接口) |
| **事件监听器** | Spring 事件总线 | 自写 Multicast listener + channel |
| **状态快照** | `data class.copy()` | `Clone()` + `WithXxx` |
| **ProcessOptions** | `copy(budget=...)` | functional options `WithBudget(...)` |

整体**语义等价**、**Go 风格**、**性能无 JVM 包袱**。

---

下一份文档描述用户视角：如何定义 Agent → `04-user-api.md`
