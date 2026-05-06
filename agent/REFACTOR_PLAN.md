# lynx/agent 重构计划

> 配套文档：[`README.md`](./README.md) / [`GUIDE.md`](./GUIDE.md) / [`DESIGN.md`](./DESIGN.md) / [`EMBABEL_GAP_ANALYSIS.md`](./EMBABEL_GAP_ANALYSIS.md)
>
> 本文档：**第 4 轮重构**——在前三轮（embabel-parity / simplify-skill polish / static-fn collapse）之后，对剩余架构清晰度问题做一次系统化梳理。**不立即落地**，先成稿、对齐方向。

---

## 0. TL;DR

经过三轮重构后，agent 模块在「核心抽象」层面已收敛得不错（5,751 LOC 主代码 + 432+1,193 LOC 文档）。剩下的提升点集中在 **god struct / god interface** 和 **文件组织**：

| 优先级 | 重构项 | 收益 | 改动量 | 风险 |
|---|---|---|---|---|
| **P0-1** | 拆 `AgentProcess`（57 方法 / 24 字段 / 4 文件 / 936 LOC） | 高 — 架构清晰 | ~300 LOC 移动 | 低 |
| **P0-2** | 拆 `Platform` 内部为 `agentRegistry` + `processRegistry` | 中 | ~200 LOC 移动 | 低 |
| **P0-3** | `Blackboard` 拆 `Reader / Writer` 接口 | 中 — 类型安全 | ~30 LOC + 调用点 | 低 |
| **P0-4** | `event/event.go` (501 LOC) 按主题拆 7 文件 | 中 — 可读性 | 纯 file split | **零** |
| **P1-5** | 去掉 `ProcessContextDeps` 中间类型 | 低 | ~50 LOC | 中 |
| **P1-6** | `ProcessContext` 抽出 internal retry helpers | 低 — API 收敛 | ~30 LOC | 中 |
| **P2-7** | 未使用字段加 `// TODO(future)` 注释 | 低 — 可读性 | 注释级 | 零 |
| **P2-8** | `ProcessOptions` 内 `ProcessControl` flatten | 低 | API 浅化 | 中 |

**最值得做的 3 项**：P0-1（拆 AgentProcess）+ P0-3（Blackboard Reader/Writer）+ P0-4（event 文件拆分）。

---

## 1. 现状度量

```
agent/ 总 LOC（production）:    5,751
最长文件:
  event/event.go              501 LOC / 29 types / 56 funcs
  planner/goap/astar.go       491 LOC（单一职责，OK）
  runtime/platform.go         442 LOC / 3 types / 20 funcs
  runtime/agent_process.go    382 LOC（+ run.go 296 + execute_action.go 230 + concurrent.go 124）
                              = AgentProcess 总跨 4 文件 936 LOC / 57 methods

最大接口:
  core.Blackboard       16 方法
  core.Process          15 方法

最大单文件 type 数:
  event/event.go        29 types（17 events + 4 summaries + Multicast/Listener/ListenerFunc/BaseEvent + envelope）

子包文件数:
  core/                 25 文件（多数为 single-concept small files，组织 OK）
  runtime/              8 文件
  其余各 1 文件
```

---

## 2. P0 重构（架构清晰度）

### 2.1 P0-1：拆分 `runtime.AgentProcess`

#### 症状

`AgentProcess` 单一 struct 承担 **6 类职责**：

```go
type AgentProcess struct {
    // ① 身份元数据
    id, parentID  string
    agent         *core.Agent
    options       *core.ProcessOptions
    startedAt     time.Time

    // ② 状态机（mu 保护）
    mu              sync.RWMutex
    status          core.AgentProcessStatus
    goal            *core.Goal
    lastWorld       core.WorldState
    history         []ActionInvocation
    failure         error
    excludedActions map[string]struct{}

    // ③ 子树预算
    children  []*AgentProcess
    ownCost   float64
    ownTokens int

    // ④ 信号 / 等待原子位
    terminate         chan core.TerminationScopeSignal
    pendingAwaitable  atomic.Pointer[awaitSlot]
    toolCallCancel    atomic.Pointer[context.CancelFunc]

    // ⑤ 接线
    blackboard core.Blackboard
    determiner worldStateDeterminer
    planner    plan.Planner
    system     *plan.PlanningSystem
    platform   *Platform
}
```

跨 4 个文件共 **57 方法**：
- `agent_process.go` (382 LOC) — 状态读写 + 信号 + budget + setX 设置器
- `run.go` (296 LOC) — OODA tick 编排
- `execute_action.go` (230 LOC) — 单 action retry / panic 守护
- `concurrent.go` (124 LOC) — 并行 tick

#### 对照 embabel

`core/SimpleAgentProcess.kt` 也大，但 Kotlin 用 **companion object / 扩展函数 / Spring 注入**让职责分散自然。Go 没有扩展函数，只能靠**结构组合（embedding）显式分**。

#### 建议结构

```go
// runtime/process_state.go
type processState struct {
    mu              sync.RWMutex
    status          core.AgentProcessStatus
    goal            *core.Goal
    lastWorld       core.WorldState
    history         []ActionInvocation
    failure         error
    excludedActions map[string]struct{}
}
// 方法：Status / Goal / LastWorldState / Failure / History /
//      setStatus / setGoal / setLastWorld / setFailure /
//      recordInvocation / excludeAction / clearExclusions / snapshotExclusions /
//      makeRunning

// runtime/process_budget.go
type processBudget struct {
    mu        sync.RWMutex   // 也可借 processState 的 mu，看依赖
    children  []*AgentProcess
    ownCost   float64
    ownTokens int
}
// 方法：Usage / RecordUsage / addChild

// runtime/process_signals.go
type processSignals struct {
    terminate        chan core.TerminationScopeSignal
    pendingAwaitable atomic.Pointer[awaitSlot]
    toolCallCancel   atomic.Pointer[context.CancelFunc]
}
// 方法：queueTermination / drainTerminate / TerminateAgent / TerminateAction /
//      TerminateToolCall / AwaitInput / deliverResponse / registerToolCallCancel

// runtime/process_wiring.go
type processWiring struct {
    blackboard core.Blackboard
    determiner worldStateDeterminer
    planner    plan.Planner
    system     *plan.PlanningSystem
    platform   *Platform
}
// 仅持有引用，无方法（或只少量 helpers）

// runtime/agent_process.go
type AgentProcess struct {
    // 元数据（值类型）
    id, parentID string
    agent        *core.Agent
    options      *core.ProcessOptions
    startedAt    time.Time

    // 组合
    *processState
    *processBudget
    *processSignals
    *processWiring
}

// run.go / execute_action.go / concurrent.go 上的方法保持不变
// 只是显式 receiver 是 *AgentProcess，访问字段走 embed 路径
// p.status → p.processState.status（自动 promote）
```

#### 收益

- 字段相关性立刻清晰：mu 保护谁一目了然（在 `processState` 里就是保护 status/goal/...，不用脑补）
- 测试时可以 mock 单个 sub-struct（例如只测 `processBudget.Usage()` 递归逻辑）
- AgentProcess 本身瘦身到 ~30 LOC（结构体定义 + ID/ParentID/StartedAt/AgentDef/Blackboard/Options 等只读 getter）
- 5 个小 type 各 30~150 LOC，比 1 个 936 LOC 易读
- 未来想分 process 持久化（`processState` 落到 Redis / DB）时只需替换该 sub-struct

#### 风险

- **行为零变化** —— 公开 API（`Status() / Goal() / Usage() / TerminateXxx() / AwaitInput()`）签名都不变
- 内部 method receiver 仍是 `*AgentProcess`，访问字段通过 embedding 自动 promote
- `core.Process` interface 仍由 `*AgentProcess` 实现

#### 建议落地分 3 个 commit

1. **commit 1**: 抽 `processSignals` (terminate + awaitable + toolCallCancel + 7 方法) → 新文件 `runtime/process_signals.go`，`AgentProcess` 改成 embed
2. **commit 2**: 抽 `processBudget` (3 字段 + 2 方法) → 新文件 `runtime/process_budget.go`
3. **commit 3**: 抽 `processState` (mu + 6 字段 + ~14 方法) → 新文件 `runtime/process_state.go`

每步独立验证 `go test -race -count=1 ./agent/...`。

---

### 2.2 P0-2：拆分 `runtime.Platform` 三职责

#### 症状

```go
// platform.go 442 LOC，Platform 16 个方法，3 个职责混在一起：

// ① Agent 注册表
Deploy / Undeploy / Agents / FindAgent

// ② Process 注册表 + 生命周期
GetProcess / ActiveProcesses / KillProcess / ResumeProcess /
createProcess / RunAgent / StartAgent / CreateChildProcess

// ③ 横切（事件、DI）
AddListener / RemoveListener / publish / Services
```

#### 对照 embabel

embabel 拆成多个 Spring bean：
- `AgentDeployer` —— Deploy / Undeploy
- `AgentRunner` —— RunAgent / StartAgent
- `ProcessRegistry` —— GetProcess / ActiveProcesses / KillProcess
- `AgenticEventListener`（多播）—— 事件分发

bean 间通过 Spring DI 注入。Go 没 DI 容器，等价物是**内部组合**。

#### 建议结构

```go
// runtime/agent_registry.go
type agentRegistry struct {
    mu     sync.RWMutex
    agents map[string]*core.Agent
}
func (r *agentRegistry) deploy(a *core.Agent) error { ... }
func (r *agentRegistry) undeploy(name string) error { ... }
func (r *agentRegistry) all() []*core.Agent { ... }
func (r *agentRegistry) find(name string) (*core.Agent, bool) { ... }

// runtime/process_registry.go
type processRegistry struct {
    mu    sync.RWMutex
    procs map[string]*AgentProcess
}
func (r *processRegistry) register(p *AgentProcess) { ... }
func (r *processRegistry) get(id string) (*AgentProcess, bool) { ... }
func (r *processRegistry) active() []*AgentProcess { ... }
func (r *processRegistry) remove(id string) { ... }   // 当前缺，可顺便补上

// runtime/platform.go
type Platform struct {
    *agentRegistry        // embed → Platform.Deploy / Undeploy / Agents / FindAgent
    *processRegistry      // embed → Platform.GetProcess / ActiveProcesses

    events   *event.Multicast
    services *core.ServiceProvider

    plannerFactory PlannerFactory
    idGen          IDGenerator
    tools          core.ToolGroupResolver
    actionMW       core.ActionMiddlewares  // (如未来落地 Extension 模型)
}

// Platform 仍然提供顶层方法（Deploy 是 embed agentRegistry.deploy 的转发；
// RunAgent / StartAgent / CreateChildProcess 协调多个 sub-struct）。
// 但 platform.go 文件瘦身到 ~150 LOC，agent_registry.go ~80 LOC，process_registry.go ~80 LOC。
```

#### 收益

- 三个职责文件分明
- 单测：`agent_registry_test.go` 可独立验证注册逻辑（无需启 Platform）
- 未来想做"远程 process 注册表"（Redis-backed）只换 `processRegistry` 即可
- `Deploy → publish AgentDeployedEvent` 这种"注册 + 发事件"的耦合仍然在 Platform 上协调，因为它跨 registry + events 两个职责

#### 风险

- 公开 API 不变（`Platform.Deploy / GetProcess / ...` 经 embedding 自动 promote）
- 注意 lock 边界：当前 `Platform.mu` 保护的是 `agents + procs`，拆完后两个 sub-struct 各自有 mu，但没有跨表事务，应该没问题（顺序无要求）

---

### 2.3 P0-3：`Blackboard` 接口拆 `Reader / Writer`

#### 症状

`core.Blackboard` 是 16 方法 god interface。`Condition.Evaluate(ctx, oc)` 拿到的 `OperationContext.Blackboard` 是完整接口——**condition 评估理论上能写黑板**，破坏 Observe 阶段的纯函数语义。

#### 对照 embabel

embabel 在 `Blackboard.kt` 中已经分了：

```kotlin
interface BlackboardRead {
    fun get(name: String): Any?
    fun getValue(variable: String, type: String): Any?
    fun objects(): List<Any>
    // ... 仅读
}

interface Blackboard : BlackboardRead {
    fun set(name: String, value: Any)
    fun bind(value: Any)
    // ... 写
}
```

condition / planner 接到 `BlackboardRead`，action body 接到完整 `Blackboard`。**类型强制**只读上下文不能写。

#### 建议结构

```go
// agent/core/blackboard.go

type BlackboardReader interface {
    ID() string
    Get(key string) (any, bool)
    GetValue(variable, typeName string) (any, bool)
    HasValue(variable, typeName string) bool
    Objects() []any
    GetCondition(key string) (bool, bool)
    InfoString(verbose bool) string
}

type BlackboardWriter interface {
    Set(key string, value any)
    AddObject(value any)
    Bind(value any)
    BindAll(map[string]any)
    BindProtected(key string, value any)
    Hide(target any)
    SetCondition(key string, value bool)
    Clear()
}

type Blackboard interface {
    BlackboardReader
    BlackboardWriter
    Spawn() Blackboard
}
```

#### 调用点改动

```go
// 之前：
type OperationContext struct {
    Process    Process
    Blackboard Blackboard      // 完整接口
}

// 之后：
type OperationContext struct {
    Process    Process
    Blackboard BlackboardReader  // 只读
}
```

action body 仍然拿到 `pc.Blackboard` 是完整 `Blackboard`（因为 action 是写黑板的主路径）。

#### 收益

- **编译期保证**：condition impl 不能误写黑板
- 借鉴 embabel 已验证的设计
- 给将来"只读视图 / 持久化层 / 缓存层" 的实现提供更小契约

#### 风险

- 现有 condition 实现都不写黑板（GUIDE.md 已经文档化），改动只是显式化
- `inMemoryBlackboard` 作为完整 Blackboard 自动满足三个接口
- `Get[T] / Last[T] / ObjectsOfType[T]` 顶层泛型 helper 需要把参数从 `Blackboard` 改成 `BlackboardReader`（向后兼容，因为 `Blackboard` 是超集）

---

### 2.4 P0-4：拆 `event/event.go` 为多文件

#### 症状

单文件 501 LOC / 29 types / 56 funcs。新加事件、改 listener 都得在一个大文件里翻。

#### 对照 embabel

embabel 在 `core/events/` 下分多文件：
- `PlatformEvents.kt` — AgentDeployed / AgentUndeployed
- `ProcessEvents.kt` — Process lifecycle
- `LlmEvents.kt` — LLM 调用事件
- `PlanningEvents.kt` — 规划事件

#### 建议结构

```
event/
├── event.go        Event interface + BaseEvent + envelope/emit + errString
├── multicast.go    Listener + ListenerFunc + Multicast (snapshot pattern)
├── platform.go     AgentDeployedEvent / AgentUndeployedEvent
├── process.go      ProcessCreatedEvent / ProcessCompletedEvent / ProcessFailedEvent /
│                   ProcessStuckEvent / ProcessKilledEvent / ProcessTerminatedEvent /
│                   ProcessWaitingEvent / ProcessPausedEvent
├── planning.go     ReadyToPlanEvent / PlanFormulatedEvent / ReplanRequestedEvent
├── execution.go    ActionExecutionStartEvent / ActionExecutionResultEvent /
│                   GoalAchievedEvent / ObjectBoundEvent
├── llm.go          LLMRequestEvent / LLMResponseEvent
└── summaries.go    goalSummary / planSummary / worldSnapshot / awaitableSummary
                    + summarizeGoal / summarizePlan / snapshotWorld / summarizeAwaitable
                    + actionName / actionNames
```

每文件 50~120 LOC。

#### 收益

- 主题分组，导航变快
- diff review 友好（加一个 LLM 事件不会污染 process 事件文件）
- 可读性显著提升

#### 风险

**零** —— 纯 file split，无 API / 行为改动。所有 type / func 仍在 `package event` 下导出。

#### 落地建议

可以单 commit 完成：`docs(event): split event.go by topic — no behavior change`。

---

## 3. P1 重构（语义清晰）

### 3.1 P1-5：去掉 `ProcessContextDeps` 中间类型

#### 症状

```go
type ProcessContextDeps struct {
    Process       Process
    Blackboard    Blackboard
    Options       *ProcessOptions
    OutputChannel OutputChannel
    Services      *ServiceProvider
    Publish       EventPublisher
    ResolveTools  ToolResolver
    ToolCallCancel ToolCallCanceller
}

type ProcessContext struct {
    Process       Process
    Blackboard    Blackboard
    Options       *ProcessOptions
    OutputChannel OutputChannel
    Services      *ServiceProvider
    publishEvent   EventPublisher
    resolveTools   ToolResolver
    toolCallCancel ToolCallCanceller
    lastErr error
}

func NewProcessContext(deps ProcessContextDeps) *ProcessContext {
    // 复制 8 个字段
}
```

`Deps` 只用一次，立刻丢弃。中间结构。

#### 对照 embabel

embabel 用直接构造：`OperationContext(process, blackboard, ...)`。无中间类型。

#### 建议方案 A — 使用 functional options

```go
type PCOption func(*ProcessContext)

func WithEventPublisher(fn EventPublisher) PCOption { ... }
func WithToolResolver(fn ToolResolver) PCOption { ... }
func WithToolCallCanceller(fn ToolCallCanceller) PCOption { ... }
func WithOutputChannel(oc OutputChannel) PCOption { ... }
func WithServices(sp *ServiceProvider) PCOption { ... }

func NewProcessContext(p Process, bb Blackboard, opts *ProcessOptions, options ...PCOption) *ProcessContext {
    pc := &ProcessContext{Process: p, Blackboard: bb, Options: opts}
    for _, opt := range options {
        opt(pc)
    }
    return pc
}
```

runtime 调用：

```go
pc := core.NewProcessContext(p, p.blackboard, p.options,
    core.WithOutputChannel(p.options.OutputChannel),
    core.WithServices(p.platformServices()),
    core.WithEventPublisher(p.publishAny),
    core.WithToolCallCanceller(p.registerToolCallCancel),
    core.WithToolResolver(resolveToolsFor(resolver)),
)
```

`ProcessContextDeps` 删除。

#### 建议方案 B — 直接公开字段构造

```go
pc := &core.ProcessContext{
    Process:       p,
    Blackboard:    bb,
    Options:       opts,
    OutputChannel: oc,
    Services:      svcs,
}
pc.SetEventPublisher(fn)    // 私有 setter，runtime-only
pc.SetToolResolver(fn)
pc.SetToolCallCanceller(fn)
```

私有 setter 通过 internal pkg 或同包仅 runtime 可见。

#### 推荐

**方案 A**（functional options）—— Go 惯例，且对未来添加新依赖友好。

#### 风险

中等：API 改动。但 `ProcessContextDeps` 只有 runtime 内部用，用户不应该直接构造 ProcessContext。

---

### 3.2 P1-6：把 `ProcessContext` 上的 internal helpers 拆出去

#### 症状

```go
type ProcessContext struct {
    // 公共字段
    Process, Blackboard, Options, OutputChannel, Services
    // 私有字段
    publishEvent, resolveTools, toolCallCancel
    // ↓ 这两个是 runtime + typed-action 内部 retry 用的
    lastErr error
}

// User-visible methods:
func (pc *ProcessContext) Publish / ResolveTools / ToolCallContext / AwaitInput / RecordUsage / Tracer

// ↓ 以下是 runtime + typed-action 内部用的，不是 user API：
func (pc *ProcessContext) ExecuteSafely(ctx, action) ActionStatus
func (pc *ProcessContext) recordError(err error)
func (pc *ProcessContext) recordPanic(panicValue any)
func (pc *ProcessContext) LastError() error
func (pc *ProcessContext) ResetError()
```

`LastError() / ResetError()` 是 exported（小写不行——typed-action 在 core 同包内访问），但用户拿到 pc 时根本不该调它们。

#### 对照 embabel

retry 用 throwing exception (`ReplanRequestedException`)，没有"记录 last error 然后检查"的模式。

#### 建议

把 retry helpers 抽到独立的内部上下文：

```go
// agent/core/retryable_context.go (internal to package)
type retryableContext struct {
    *ProcessContext
    lastErr error
}

func (rc *retryableContext) executeWithRecovery(ctx context.Context, a Action) ActionStatus {
    defer func() {
        if r := recover(); r != nil {
            rc.recordPanic(r)
        }
    }()
    return a.Execute(ctx, rc.ProcessContext)
}

// typed action 的 Execute 用 retryableContext 而不是 ProcessContext，
// 但传给 user fn 的还是 *ProcessContext。
```

公开 `ProcessContext` 由 13 方法瘦身到 ~10。

#### 风险

中：需要 typed action 实现协调改动。但用户只看到 `ProcessContext` 干净版，受益。

---

## 4. P2 重构（修剪 / 标注）

### 4.1 P2-7：未使用字段加 `// TODO(future)` 注释

| 字段 | 注释建议 |
|---|---|
| `ProcessControl.ToolDelay / OperationDelay` | `// TODO(future): not consumed yet — placeholder for throttling` |
| `ProcessOptions.Prune` | `// TODO(future): planner pruning hook (see plan/planner.go)` |
| `Verbosity.ShowPrompts/ShowLLMResponses/Debug/ShowPlanning` | `// TODO(future): consumed by integration LLM listeners; framework just stores` |
| `Identities.ForUser/RunAs` | `// TODO(future): wire into audit log + RBAC checks via Extension` |
| `ActionConfig.ReadOnly / Trigger / ClearBlackboard` | `// TODO(future): planner read-only optimisation / trigger types / blackboard clear policy` |
| `Goal.Tags / Examples / Export` | `// TODO(P0-1 in EMBABEL_GAP_ANALYSIS): consumed by MCP server when implemented` |
| `core.PlannerUtility` constant | `// TODO(future): utility-based planner; default factory still returns GOAP` |

#### 收益

- grep `TODO(future)` 立刻定位"留给将来扩展但现在不通电"的字段
- 新 contributor 不会以为这些是死代码而误删（已经死过又复活一次了）
- 可读性，零行为改动

#### 风险

零。

---

### 4.2 P2-8：flatten `ProcessControl`

#### 症状

```go
type ProcessOptions struct {
    // ...
    ProcessControl ProcessControl  // 三层嵌套
    // ...
}

type ProcessControl struct {
    EarlyTerminationPolicy EarlyTerminationPolicy
    ToolDelay              time.Duration   // 未使用
    OperationDelay         time.Duration   // 未使用
}

// 用户访问：
opts.ProcessControl.EarlyTerminationPolicy
```

`ProcessControl` 这个 group 名字不直观（"控制"很泛），且嵌套深。两个 delay 字段未使用。

#### 建议

```go
type ProcessOptions struct {
    // ...
    EarlyTerminationPolicy EarlyTerminationPolicy  // 直接顶层
    // ToolDelay / OperationDelay 删除（如未来有节流需求再加）
    // ...
}
```

#### 风险

中：API 改动（`opts.ProcessControl.EarlyTerminationPolicy` → `opts.EarlyTerminationPolicy`）。但 lynx 0.0.0，无外部用户。

#### 收益

低（taste 级别）。**可选项**——如果后续要加 throttling 字段，分组会再起来；现在 flatten 了未来又得 group 回去。

---

### 4.3 P2-9：小文件合并（可选）

| 当前 | 合并后 |
|---|---|
| `core/domain_type.go` (32 LOC) | 合到 `core/iobinding.go`（同主题——类型派生） |
| `core/early_termination.go` | 留独立（接口 + 3 实现，自洽） |
| `core/output_channel.go` | 留独立（接口 + 3 实现，自洽） |

只有 `domain_type.go` 值得合，其余 OK。

---

## 5. 对照 embabel 的启发表

| 维度 | embabel 做法 | lynx 现状 | 借鉴价值 |
|---|---|---|---|
| 大组件分散 | Spring bean DI 自然拆 | 单 struct + 多文件 method | **借鉴 embedding 拆 AgentProcess / Platform**（P0-1, P0-2） |
| 接口分层 | `BlackboardRead` / `Blackboard` 父子 | 单 16-method 接口 | **借鉴 Reader/Writer 分**（P0-3） |
| 事件组织 | 主题分 4 文件 | 单文件 501 LOC | **借鉴 file split**（P0-4） |
| Context 构造 | 直接构造函数 | `Deps struct → New` 中间层 | **借鉴去中间层**（P1-5） |
| 内部 vs 外部 API | Spring `@Internal` annotation | 同一公共 type 上混 user/runtime | **借鉴：把 internal helpers 移走**（P1-6） |
| 死字段保留 | Spring `@Deprecated` 标注 | Go 无标注，注释靠人读 | **加 `// TODO(future)`**（P2-7） |
| god 接口拆 | embabel 已拆 | 还没拆 | 见 P0-3 |
| 异常控制流 | `TerminateActionException` | error + signal channel | **不抄** — Go 惯例 |
| 反射注解 | `@Agent` / `@Action` | 删了（commit `53ea5a3`） | **不复活** — 正确取舍 |

---

## 6. 不要做的"重构"

| 提议 | 为何不做 |
|---|---|
| 把 `Action.Execute` 改成 `(any, error)` 而非 `ActionStatus` | ActionStatus 三态（Succeeded/Failed/Waiting/Paused）非 error/result 二态能表达。信息丢失 |
| 用 channel 重写 OODA 循环 | 当前 for-loop tick 简单清晰；channel 反而引入调度复杂度 |
| 把 `Goal` / `Action` / `Condition` 抽到 sub-package | 它们互相紧密引用（Goal.Inputs 是 IOBinding；Condition 用 Determination）；分包要循环引用 |
| 引入 reflection-based action 注册 | 上次 commit `53ea5a3` 已删，是正确的 Go 取舍 |
| 把 GOAP planner 抽成独立 module | 没必要：`planner/goap` 已是独立 sub-package，外部不会单独 import |
| 拆 `astar.go` (491 LOC) 为多文件 | 单一职责（A* 算法），函数已分（`PlanToGoal / searchForGoal / expandNeighbors / backwardOptimize / forwardOptimize`），再拆反而散 |
| 重写 `dsl/builder.go` | 11 个一行 setter，符合 fluent builder 惯例。已 OK |
| 强行把所有 hook 统一为 Extension 模型 | 应该是单独提案（前序对话已分析）。重构不应改语义边界 |

---

## 7. 推荐落地顺序

### 阶段 A — 安全的 file split（先做）

**目标**：纯组织调整，零行为变化，零风险，立刻提升可读性。

| 步骤 | 文件 | 改动 | commit |
|---|---|---|---|
| A1 | event/event.go (501 LOC) | 拆 7 文件 | `docs(event): split event.go by topic` |

预计 1 个小时。

### 阶段 B — 接口拆分（中等改动）

**目标**：编译期类型安全提升，向后兼容。

| 步骤 | 改动 | commit |
|---|---|---|
| B1 | `core.Blackboard` 拆 `Reader / Writer` + `OperationContext.Blackboard` 改为 `BlackboardReader` | `refactor(core): split Blackboard into Reader/Writer interfaces` |
| B2 | `core.Get[T] / Last[T] / ObjectsOfType[T]` 参数改 `BlackboardReader` | （并入 B1） |

预计 半天。

### 阶段 C — 拆 AgentProcess（最大胜利）

**目标**：架构清晰度。每步独立 verifies + commit。

| 步骤 | 改动 | commit |
|---|---|---|
| C1 | 抽 `processSignals` (terminate / awaitable / toolCallCancel + 7 方法) | `refactor(runtime): extract processSignals from AgentProcess` |
| C2 | 抽 `processBudget` (3 字段 + 2 方法) | `refactor(runtime): extract processBudget from AgentProcess` |
| C3 | 抽 `processState` (mu + 6 字段 + ~14 方法) | `refactor(runtime): extract processState from AgentProcess` |

预计 1 天。

### 阶段 D — 拆 Platform

**目标**：职责清晰。

| 步骤 | 改动 | commit |
|---|---|---|
| D1 | 抽 `agentRegistry` (4 方法) | `refactor(runtime): extract agentRegistry from Platform` |
| D2 | 抽 `processRegistry` (4 方法) | `refactor(runtime): extract processRegistry from Platform` |

预计 半天。

### 阶段 E — ProcessContext 收敛（可选）

**目标**：API 表面清晰。**有 API 改动风险，最后做**。

| 步骤 | 改动 | commit |
|---|---|---|
| E1 | 抽 internal retry helpers | `refactor(core): hide retry internals from ProcessContext` |
| E2 | 去掉 `ProcessContextDeps`，改 functional options | `refactor(core): replace ProcessContextDeps with functional options` |

预计 半天。

### 阶段 F — 注释 / 标注（任意时机）

| 步骤 | 改动 | commit |
|---|---|---|
| F1 | 未使用字段加 `// TODO(future)` | `docs(core): mark future-extension fields with TODO` |

预计 15 分钟。

---

## 8. 推算总成本

| 阶段 | 预估时间 | LOC 净变化 | commit 数 |
|---|---|---|---|
| A | 1 小时 | ±0（仅 split） | 1 |
| B | 半天 | +30 LOC | 1 |
| C | 1 天 | ±0（仅 move） | 3 |
| D | 半天 | ±0（仅 move） | 2 |
| E | 半天 | -30 LOC | 2 |
| F | 15 分钟 | +20 LOC（注释） | 1 |
| **总计** | **~3 天** | **+20 LOC** | **10** |

---

## 9. 决策点 — 哪些一定做、哪些可选

### **强烈推荐做**（high ROI / low risk）

- ✅ **A1**: event.go split（1 小时，零风险，立竿见影）
- ✅ **C1-C3**: AgentProcess 拆分（最大架构胜利）
- ✅ **B1-B2**: Blackboard Reader/Writer（编译期安全）
- ✅ **F1**: TODO 标注（15 分钟）

### **看情况做**（中等 ROI / 中等风险）

- ⚠️ **D1-D2**: Platform 拆分（清晰但 442 LOC 不算大，可缓）
- ⚠️ **E1**: 抽 retry helpers（清理但要协调 typed-action 实现）

### **不建议做**（低 ROI 或主题级判断）

- ❌ **E2**: 去 ProcessContextDeps（functional options 见仁见智，不强求）
- ❌ **P2-8**: ProcessControl flatten（可能未来又要 group 回去）
- ❌ **P2-9**: 合并 small files（taste 级，不重要）

---

## 10. 后续

落地阶段 A-C-B-D-F-E 后，agent 模块预计：

```
之前:                          之后:
event/event.go        501  →   event/{event,multicast,platform,process,planning,
                                     execution,llm,summaries}.go ×8 ~80 LOC each
runtime/agent_process 936  →   runtime/{process_state,signals,budget,wiring,agent_process,
                                       run,execute_action,concurrent}.go ~150-300 LOC each
runtime/platform.go   442  →   runtime/{platform,agent_registry,process_registry}.go
                                ~150+80+80 LOC

接口:
core.Blackboard       16   →   BlackboardReader 7 + BlackboardWriter 8 + Blackboard 3 (compose)
core.Process          15   →   不变（未识别为 god 接口；语义统一）

god 文件:             3 个 → 0
god struct:           1 个 → 0
god interface:        1 个 → 0
```

**架构清晰度显著提升，公开行为零变化**。

---

## 11. 不在本计划范围

以下属于 **新功能 / 设计提案**，不是 refactor：

- Extension 统一模型（前序对话提案）—— 单独立项，需要 RFC
- ActionMiddleware 中间件抽象 —— 单独立项
- Plugin 模式 —— 单独立项
- ToolLoop runner（P0-2 in `EMBABEL_GAP_ANALYSIS.md`）—— 新功能
- MCP server 端（P0-1）—— 新功能
- SupervisorAgent 模式（P0-3）—— 新功能 / 文档

这些落地后**可能**触发新一轮 refactor，但当前不影响本计划。

---

> **下一步**：读完本计划，决定要做哪些阶段。建议从 **阶段 A（event split）** 开始热身——零风险一小时活，立刻获得心智松弛。然后视情况推进 C / B。
