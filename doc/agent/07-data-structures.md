# 07. 核心数据结构目录（Data Structure Catalog）

> 本文件是 `agent/` module 所有核心类型的**结构化目录**，为实现阶段直接作为参考。
> 约定：每个类型都列出 **字段 / 不变式 / 线程安全性 / 关键方法签名**。
> 与 `02-core-abstractions.md` 的关系：那一份侧重「为什么这样设计」，本份侧重「结构长什么样」。

---

## 0. 数据分层总图

```
┌─────────────────────────────────────────────────────────────────┐
│ Layer 5: Event / Error（事件与错误信号）                          │
│   Event 接口 + 15 个具体事件类型                                   │
│   ReplanRequest / StuckHandlingResult / EarlyTermination         │
├─────────────────────────────────────────────────────────────────┤
│ Layer 4: Runtime State（运行时状态）                              │
│   AgentProcess / ProcessContext / ProcessOptions                 │
│   ActionInvocation / LLMInvocation / ServiceProvider             │
├─────────────────────────────────────────────────────────────────┤
│ Layer 3: Planning（规划）                                         │
│   WorldState / Plan / PlanningSystem                             │
│   searchNode (internal to A*)                                    │
├─────────────────────────────────────────────────────────────────┤
│ Layer 2: Domain（领域）                                            │
│   Agent / Action / Goal / Condition                              │
│   ActionMetadata / EffectSpec                                    │
├─────────────────────────────────────────────────────────────────┤
│ Layer 1: Primitives（原语）                                        │
│   Determination (3-valued) / IoBinding / Semver / DomainType     │
│   ActionStatus / AgentProcessStatus / PlannerType                │
└─────────────────────────────────────────────────────────────────┘
```

**依赖方向**：上层可以引用下层；下层不引用上层。

---

## Layer 1 —— 原语

### 1.1 `Determination` — 三值逻辑

```go
package core

type Determination int8

const (
    Unknown Determination = 0  // 零值即 Unknown，这是刻意选择
    True    Determination = 1
    False   Determination = 2
)

// 方法
func (d Determination) IsTrue() bool
func (d Determination) IsFalse() bool
func (d Determination) IsUnknown() bool
func (d Determination) String() string
func (d Determination) And(b Determination) Determination
func (d Determination) Or(b Determination) Determination
func (d Determination) Not() Determination
```

- **不变式**：值只能是 0/1/2，其他视为非法
- **零值语义**：`Unknown` 作零值是关键——未评估的条件默认为 Unknown，A* 才能正确剪枝

---

### 1.2 `IoBinding` — 输入/输出绑定

```go
package core

const (
    DefaultBinding    = "it"         // 默认变量名
    LastResultBinding = "lastResult" // @Trigger 专用
)

type IoBinding struct {
    Name string // 变量名，空即 "it"
    Type string // Go 类型全名："pkg/path.TypeName"
}

// 构造
func NewIoBinding[T any](name string) IoBinding
func ParseIoBinding(s string) IoBinding   // 从 "name:Type" 字符串解析
func (b IoBinding) String() string         // 反序列化成 "name:Type"

// 查询
func (b IoBinding) IsDefault() bool        // Name == "it"
func (b IoBinding) ResolveReflectType() reflect.Type
```

- **不变式**：`Type` 非空；`Name == ""` 视同 `DefaultBinding`
- **比较**：`IoBinding` 可作 map key（需 `comparable`）

---

### 1.3 `Semver` — 语义版本

```go
package core

type Semver struct {
    Major      int
    Minor      int
    Patch      int
    PreRelease string  // 如 "alpha.1"
    Build      string  // 如 "001"
}

func ParseSemver(s string) Semver   // "1.2.3-alpha.1+001"
func (v Semver) String() string
func (v Semver) Less(o Semver) bool
```

---

### 1.4 `DomainType` — 领域类型 Schema

```go
package core

// DomainType 描述一个可被规划器识别的数据类型
type DomainType struct {
    Name        string            // "com.example.Intent"
    Description string
    Schema      *jsonschema.Schema // 用 Lynx 已有 invopop/jsonschema
    ReflectType reflect.Type       // 运行时反射类型
    Parents     []string           // 父接口类型名列表（子类型匹配用）
    IsSealed    bool               // 是否是 sealed 接口
}

// 工厂
func DomainTypeOf[T any](description string) DomainType
```

---

### 1.5 `EffectSpec` — 条件规格

```go
package core

// EffectSpec 是条件键到三值的映射。用于 Action.preconditions / Action.effects / Goal.preconditions
type EffectSpec map[string]Determination

// 常用操作
func (s EffectSpec) Clone() EffectSpec
func (s EffectSpec) Merge(other EffectSpec) EffectSpec  // other 覆盖 s
func (s EffectSpec) Keys() []string
```

- **约定**：map 零值 `nil` 视同空 EffectSpec，不 panic

---

### 1.6 枚举类型

```go
package core

// ActionStatus 单次 action 执行结果
type ActionStatus int8
const (
    ActionSucceeded ActionStatus = iota
    ActionFailed
    ActionWaiting       // 等待外部输入（HITL）
    ActionPaused        // 主动暂停
)

// AgentProcessStatus 进程生命周期状态
type AgentProcessStatus int8
const (
    StatusNotStarted AgentProcessStatus = iota
    StatusRunning
    StatusCompleted
    StatusFailed
    StatusStuck         // 无法规划出计划
    StatusWaiting
    StatusPaused
    StatusTerminated    // 早终止策略触发
    StatusKilled        // 外部 Kill
)

// PlannerType 规划器种类
type PlannerType int8
const (
    PlannerGOAP PlannerType = iota   // 默认，A* GOAP
    PlannerUtility                   // 效用规划（开放式任务）
)

// ProcessType 执行模式
type ProcessType int8
const (
    ProcessSimple ProcessType = iota  // 每 tick 1 个 action
    ProcessConcurrent                 // 每 tick 所有可达 action 并发
)
```

- 所有枚举都有 `String() string` 方法供日志使用

---

### 1.7 `Timestamped` / `Timed` 混入接口

```go
package core

type Timestamped interface {
    Timestamp() time.Time
}

type Timed interface {
    RunningTime() time.Duration
}
```

---

## Layer 2 —— 领域

### 2.1 `Condition` 接口

```go
package core

type Condition interface {
    Name() string
    Cost() float64                                                       // [0.0, 1.0]，给规划器估算代价
    Evaluate(ctx context.Context, oc *OperationContext) Determination
}

// OperationContext 是 Condition.Evaluate 的上下文
type OperationContext struct {
    Process    *AgentProcess
    Blackboard Blackboard
    Registry   observation.Registry
}

// 具体实现类型（仅数据）
type ComputedCondition struct {
    name     string
    cost     float64
    evalFunc func(ctx context.Context, oc *OperationContext) Determination
}

type AndCondition struct{ A, B Condition }
type OrCondition  struct{ A, B Condition }
type NotCondition struct{ Inner Condition }

type PromptCondition struct {
    name   string
    cost   float64
    prompt string
    llm    *chat.Client   // 通过 LLM 判断（实验性）
}
```

---

### 2.2 `Action` 接口 + 数据

```go
package core

type Action interface {
    Metadata() ActionMetadata
    Execute(ctx context.Context, pc *ProcessContext) ActionStatus
}

// ActionMetadata Action 的所有只读元数据
type ActionMetadata struct {
    Name          string
    Description   string
    Inputs        []IoBinding
    Outputs       []IoBinding
    Preconditions EffectSpec       // 自动计算 + 用户显式 pre
    Effects       EffectSpec       // 自动计算 + 用户显式 post
    CanRerun      bool
    ReadOnly      bool
    QoS           ActionQos
    ToolGroups    []ToolGroupRequirement
    CostFn        CostFunc         // 动态代价（nil 则用 CostStatic）
    ValueFn       CostFunc         // 动态价值
    CostStatic    float64
    ValueStatic   float64
    Trigger       reflect.Type     // 等价 embabel @Action(trigger = ...)
    OutputBinding string            // 自定义输出变量名（默认 "it"）
    ClearBlackboard bool            // 执行前清空黑板（破坏性）
}

// CostFunc 基于世界状态动态算成本/价值
type CostFunc func(ws WorldState) float64

// ActionQos 重试策略
type ActionQos struct {
    MaxAttempts       int           // 默认 5
    BackoffMillis     int64         // 默认 10000
    BackoffMultiplier float64       // 默认 5.0
    BackoffMaxMillis  int64         // 默认 60000
    Idempotent        bool          // 是否幂等（决定能否并发执行）
}

// 具体实现
type TypedActionImpl[In, Out any] struct {
    metadata ActionMetadata
    fn       TypedActionFunc[In, Out]
}

type TypedActionFunc[In, Out any] func(ctx context.Context, pc *ProcessContext, input In) (Out, error)

type ReflectedAction struct {
    metadata ActionMetadata
    receiver reflect.Value
    method   reflect.Method
}

// ToolGroupRequirement
type ToolGroupRequirement struct {
    Role     string  // "web" / "calculator" / ...
    Required bool    // 缺失是否失败
}

// 构造器（详见 04-user-api.md）
func NewAction[In, Out any](name string, fn TypedActionFunc[In, Out], opts ...ActionOption) Action
```

- **不变式**：Action 构造后元数据不可变（值语义）
- **线程安全**：Action 本身只读；共享给多 goroutine 使用无需同步

---

### 2.3 `Goal`

```go
package core

type Goal struct {
    Name        string
    Description string
    Pre         []string        // 显式前置（字符串形式的条件）
    Inputs      []IoBinding     // 类型要求（等价于 pre 里的绑定形式）
    OutputType  *string         // 目标期望产出类型（可选）
    ValueStatic float64         // 静态价值 [0, 1]
    ValueFn     CostFunc        // 动态价值（优先于 ValueStatic）
    Tags        []string
    Examples    []string        // 给 LLM / MCP 的示例
    Export      ExportConfig
}

// ExportConfig 暴露到外部（MCP、工具系统）的配置
type ExportConfig struct {
    Name          string
    Remote        bool                   // 是否暴露到远程
    Local         bool                   // 是否本地工具可见
    StartingTypes []reflect.Type         // 远程调用时允许的入参类型
}

// 工厂
func GoalProducing[T any](description string) *Goal
func NewGoal(name, description string) *Goal

// 变更器（不可变风格）
func (g *Goal) WithPre(conditions ...string) *Goal
func (g *Goal) WithInputs(bindings ...IoBinding) *Goal
func (g *Goal) WithValue(v float64) *Goal
func (g *Goal) WithValueFn(fn CostFunc) *Goal
func (g *Goal) WithTags(tags ...string) *Goal
func (g *Goal) WithExamples(examples ...string) *Goal
func (g *Goal) WithExport(e ExportConfig) *Goal

// 派生
func (g *Goal) Preconditions() EffectSpec     // Pre + Inputs 合并
```

---

### 2.4 `Agent`

```go
package core

type Agent struct {
    Name         string
    Provider     string
    Version      Semver
    Description  string

    Actions      []Action          // 顺序相关（规划器按此顺序排优先）
    Goals        []*Goal
    Conditions   []Condition

    StuckHandler StuckHandler      // 可空
    Opaque       bool              // 对外隐藏内部
    DomainTypes  []DomainType      // 自动从 Actions 推断

    // 内部字段（未导出；DSL Builder 填入）
    mu sync.RWMutex  // 保护下面的派生缓存
    planningSystemCache *plan.PlanningSystem  // 懒缓存
}

// 工厂
func NewAgent(meta AgentMeta, actions []Action, goals []*Goal, conditions []Condition) *Agent

type AgentMeta struct {
    Name, Provider, Description string
    Version Semver
    Opaque  bool
    StuckHandler StuckHandler
}

// 关键方法
func (a *Agent) PlanningSystem() *plan.PlanningSystem
func (a *Agent) WithSingleGoal(g *Goal) *Agent         // 聚焦执行
func (a *Agent) PruneTo(goals []*Goal) *Agent           // 剪枝
func (a *Agent) ResolveType(name string) *DomainType    // 按类型名查 DomainType
func (a *Agent) Clone() *Agent
```

- **不变式**：Actions/Goals/Conditions 在创建后通常不再修改（通过 `WithXxx` 返回副本）
- **线程安全**：读操作并发安全（RWMutex）；构造期单线程

---

## Layer 3 —— 规划

### 3.1 `WorldState` 接口 + 实现

```go
package plan

type WorldState interface {
    core.Timestamped
    State() map[string]core.Determination  // 只读视图
    HashKey() string                       // 用于 A* closedSet 去重
    Apply(effects core.EffectSpec) WorldState  // 纯函数：返回新状态
}

// ConditionWorldState 主实现
type ConditionWorldState struct {
    stateMap  map[string]core.Determination  // 不可直接暴露
    timestamp time.Time
    hashCache string                          // 懒计算 hash
}

// 工厂
func NewConditionWorldState(state map[string]core.Determination) *ConditionWorldState
func EmptyWorldState() *ConditionWorldState
```

- **不变式**：WorldState 是**不可变值**——每次 Apply 返回新实例
- **HashKey**：要对 `stateMap` 的键值对做稳定序列化（sorted JSON）

---

### 3.2 `PlanningSystem`

```go
package plan

type PlanningSystem struct {
    Actions    []core.Action
    Goals      []*core.Goal
    Conditions []core.Condition

    knownConditions map[string]struct{}  // 懒计算
}

func NewPlanningSystem(actions []core.Action, goals []*core.Goal, conditions []core.Condition) *PlanningSystem

func (s *PlanningSystem) KnownConditions() map[string]struct{}  // 合并 actions.pre/post + goals.pre + conditions.name
func (s *PlanningSystem) FindAction(name string) (core.Action, bool)
func (s *PlanningSystem) FindGoal(name string) (*core.Goal, bool)
```

---

### 3.3 `Plan`

```go
package plan

type Plan struct {
    Actions []core.Action   // 按执行顺序，空 = 已达成
    Goal    *core.Goal
}

func (p *Plan) IsComplete() bool
func (p *Plan) Cost(ws WorldState) float64
func (p *Plan) Value(ws WorldState) float64     // 来自 Goal
func (p *Plan) NetValue(ws WorldState) float64  // Value - Cost
```

- **不变式**：`Actions` 不包含 nil；`Goal` 非空

---

### 3.4 `searchNode`（A* 内部）

```go
package goap

// 仅包内可见
type searchNode struct {
    state   *plan.ConditionWorldState
    gScore  float64  // 已走代价
    hScore  float64  // 启发值
    fScore  float64  // g+h
    index   int      // heap.Interface 用
}
```

---

## Layer 4 —— 运行时

### 4.1 `AgentProcess`

```go
package runtime

type AgentProcess struct {
    // 不可变字段（创建后不变）
    ID        string
    ParentID  string              // 子进程
    Agent     *core.Agent
    Options   *core.ProcessOptions
    StartedAt time.Time

    // 可变状态（受 mu 保护）
    mu           sync.RWMutex
    status       core.AgentProcessStatus
    goal         *core.Goal          // 最近规划的 goal
    lastWorld    plan.WorldState     // 最近 tick 的世界状态
    history      []ActionInvocation
    llmLog       []LLMInvocation
    failure      error
    replanBlacklist map[string]struct{}  // 禁止重入的 action 名
    terminateRequest *TerminationRequest

    // 依赖（构造时注入）
    blackboard   core.Blackboard
    determiner   WorldStateDeterminer
    planner      plan.Planner
    platform     *Platform
}

type TerminationRequest struct {
    Reason string
    Kind   string   // "agent" / "action"
}
```

#### 关键方法签名

```go
func (p *AgentProcess) Run(ctx context.Context) error
func (p *AgentProcess) Tick(ctx context.Context) error
func (p *AgentProcess) Kill() error
func (p *AgentProcess) Status() core.AgentProcessStatus
func (p *AgentProcess) Goal() *core.Goal
func (p *AgentProcess) History() []ActionInvocation  // 返回副本
func (p *AgentProcess) LastWorldState() plan.WorldState
func (p *AgentProcess) Failure() error

// 委托给 blackboard（嵌入模式）
func (p *AgentProcess) Set(key string, value any)
func (p *AgentProcess) Get(key string) (any, bool)
// ... 其他 Blackboard 方法
```

- **不变式**：`ID` 全局唯一；`Agent`/`Options`/`ID` 创建后永不变
- **线程安全**：`Run`/`Tick` 期望单线程调用；读取状态（Status/History）并发安全

---

### 4.2 `ActionInvocation` / `LLMInvocation`

```go
package runtime

// ActionInvocation 执行历史的一条记录
type ActionInvocation struct {
    ActionName string
    Timestamp  time.Time
    Duration   time.Duration
    Status     core.ActionStatus
    Attempts   int             // 重试次数
}

// LLMInvocation LLM 调用历史
type LLMInvocation struct {
    Timestamp    time.Time
    Model        string          // "gpt-4" 等
    Provider     string          // "openai" 等
    InputTokens  int
    OutputTokens int
    Duration     time.Duration
    Cost         float64         // 估算成本（USD）
    Err          error
}
```

---

### 4.3 `ProcessContext` — 传给 Action 的上下文

```go
package core

type ProcessContext struct {
    // 运行时
    Process       *AgentProcess
    Blackboard    Blackboard
    OutputChannel OutputChannel

    // 配置
    ProcessOptions *ProcessOptions

    // 服务（Lynx 集成）
    services *ServiceProvider
}

// 便捷访问方法
func (pc *ProcessContext) LLM() *chat.Client
func (pc *ProcessContext) Services() *ServiceProvider
func (pc *ProcessContext) Observe(ctx context.Context, name string, attrs ...observation.Attr) (context.Context, observation.Observation)
func (pc *ProcessContext) Publish(e event.Event)
func (pc *ProcessContext) ResolveTools(roles ...string) []chat.Tool
func (pc *ProcessContext) AwaitInput(req hitl.Awaitable) ActionStatus
```

```go
// ServiceProvider 聚合 Lynx 核心组件
type ServiceProvider struct {
    Chat         *chat.Client
    RAG          *rag.Pipeline
    VectorStore  vectorstore.VectorStore
    Observations observation.Registry
    Tools        ToolGroupResolver
}
```

---

### 4.4 `ProcessOptions` — 进程配置

```go
package core

type ProcessOptions struct {
    ContextID      string           // 持久化上下文 ID
    Identities     Identities
    Blackboard     Blackboard       // 复用已有黑板
    Verbosity      Verbosity
    Budget         Budget
    ProcessControl ProcessControl
    Prune          bool             // 是否剪枝未使用的 action
    Listeners      []event.Listener
    OutputChannel  OutputChannel
    PlannerType    PlannerType
    ToolCallContext map[string]any  // 传给 tool 的上下文
}

// 功能子配置
type Verbosity struct {
    ShowPrompts      bool
    ShowLLMResponses bool
    Debug            bool
    ShowPlanning     bool
}

type Budget struct {
    CostLimit   float64  // 默认 2.0 (USD)
    ActionLimit int      // 默认 50
    TokenLimit  int      // 默认 1_000_000
}

type ProcessControl struct {
    EarlyTerminationPolicy EarlyTerminationPolicy
    ToolDelay              time.Duration
    OperationDelay         time.Duration
}

type Identities struct {
    ForUser *User  // 代表谁在操作
    RunAs   *User  // 以谁的身份执行
}

type User struct {
    ID       string
    Name     string
    Metadata map[string]any
}

// 构造器
func NewProcessOptions(opts ...ProcessOptionFunc) *ProcessOptions

type ProcessOptionFunc func(*ProcessOptions)

func WithBudget(b Budget) ProcessOptionFunc
func WithVerbosity(v Verbosity) ProcessOptionFunc
func WithPlannerType(t PlannerType) ProcessOptionFunc
// ...
```

- **默认值**：所有字段零值都是合理默认（Budget 默认 {2.0, 50, 1M}，Verbosity 默认全 false）

---

### 4.5 `Blackboard` 接口 + 实现

```go
package core

type Blackboard interface {
    ID() string

    // Key-value
    Set(key string, value any)
    Get(key string) (any, bool)
    BindAll(m map[string]any)
    BindProtected(key string, value any)   // spawn 时保留

    // 类型化查询
    GetValue(variable, typeName string) (any, bool)
    HasValue(variable, typeName string) bool

    // 对象列表
    AddObject(value any)
    Objects() []any       // 返回副本
    Hide(target any)

    // 命名条件
    SetCondition(key string, value bool)
    GetCondition(key string) (bool, bool)

    // 子黑板
    Spawn() Blackboard
    Clear()

    // 调试
    InfoString(verbose bool) string
}

// InMemoryBlackboard 主实现
type InMemoryBlackboard struct {
    id string

    mu         sync.RWMutex   // 保护下面所有字段
    named      map[string]any
    protected  map[string]struct{}
    objects    []any
    hidden     map[any]struct{}
    conditions map[string]bool
}

func NewInMemoryBlackboard(id string) *InMemoryBlackboard
```

#### 泛型 helper（顶层函数，不在接口上）

```go
package core

// 类型安全的查询（替代 Kotlin 泛型方法）
func Get[T any](bb Blackboard, name string) (T, bool)
func Last[T any](bb Blackboard) (T, bool)
func ObjectsOfType[T any](bb Blackboard) []T
func Count[T any](bb Blackboard) int

// ResultOfType 从进程黑板取最终结果（命名与 embabel 对齐）
func ResultOfType[T any](p *AgentProcess) (T, bool)
```

- **线程安全**：InMemoryBlackboard 全方法加锁；`Objects()`/`InfoString()` 返回副本
- **特殊查询**：
  - `variable == "it"` → 最新同类型对象
  - `variable == "lastResult"` → 最近一次 `AddObject` 的对象

---

### 4.6 `OutputChannel` 接口

```go
package core

// OutputChannel 向用户推送消息（Action 可用）
type OutputChannel interface {
    Write(msg string)
    WriteTyped(topic string, payload any)
    Close() error
}

// 内置实现
var DevNullOutputChannel OutputChannel = devNull{}

type WriterOutputChannel struct { w io.Writer }      // 直接写 io.Writer
type ChannelOutputChannel struct { ch chan<- string } // 转发到 Go chan
```

---

### 4.7 `Platform` — 平台入口

```go
package runtime

type Platform struct {
    mu     sync.RWMutex
    agents map[string]*core.Agent  // 已部署的 agents
    procs  map[string]*AgentProcess // 活跃进程

    processType    core.ProcessType
    plannerFactory PlannerFactory
    events         *event.Multicast
    idGen          IDGenerator

    // 服务
    services *core.ServiceProvider
}

// 构造
func NewPlatform(opts ...PlatformOption) *Platform

type PlatformOption func(*Platform)

func WithChatClient(c *chat.Client) PlatformOption
func WithObservation(r observation.Registry) PlatformOption
func WithRAG(p *rag.Pipeline) PlatformOption
func WithVectorStore(s vectorstore.VectorStore) PlatformOption
func WithProcessType(t core.ProcessType) PlatformOption
func WithPlannerFactory(f PlannerFactory) PlatformOption
func WithListener(l event.Listener) PlatformOption

// 生命周期
func (p *Platform) Deploy(a *core.Agent) error
func (p *Platform) Undeploy(name string) error
func (p *Platform) Agents() []*core.Agent
func (p *Platform) FindAgent(name string) (*core.Agent, bool)

// 执行
func (p *Platform) RunAgent(ctx context.Context, agent *core.Agent, bindings map[string]any, opts ...core.ProcessOptionFunc) (*AgentProcess, error)
func (p *Platform) StartAgent(ctx context.Context, agent *core.Agent, bindings map[string]any, opts ...core.ProcessOptionFunc) (*AgentProcess, <-chan error)
func (p *Platform) CreateChildProcess(agent *core.Agent, parent *AgentProcess) (*AgentProcess, error)

// 查询
func (p *Platform) GetProcess(id string) (*AgentProcess, bool)
func (p *Platform) ActiveProcesses() []*AgentProcess
func (p *Platform) KillProcess(id string) error

// 扩展
func (p *Platform) AddListener(l event.Listener)
```

---

## Layer 5 —— 事件与错误信号

### 5.1 `Event` 接口 + 具体事件

```go
package event

type Event interface {
    Timestamp() time.Time
    ProcessID() string
    EventName() string  // 用于日志
}

// baseEvent 所有事件的嵌入类型
type baseEvent struct {
    timestamp time.Time
    processID string
}

func (b baseEvent) Timestamp() time.Time { return b.timestamp }
func (b baseEvent) ProcessID() string    { return b.processID }
```

#### 全部具体事件类型（共 15 个）

```go
// 平台级
type (
    AgentDeployedEvent   struct{ baseEvent; AgentName string }
    AgentUndeployedEvent struct{ baseEvent; AgentName string }
)

// 进程生命周期
type (
    ProcessCreatedEvent   struct{ baseEvent; Bindings map[string]any }
    ProcessCompletedEvent struct{ baseEvent; Goal *core.Goal }
    ProcessFailedEvent    struct{ baseEvent; Err error }
    ProcessStuckEvent     struct{ baseEvent; LastWorld plan.WorldState }
    ProcessWaitingEvent   struct{ baseEvent; Await hitl.Awaitable }
    ProcessPausedEvent    struct{ baseEvent; Reason string }
    ProcessKilledEvent    struct{ baseEvent; Reason string }
)

// 规划
type (
    ReadyToPlanEvent       struct{ baseEvent; World plan.WorldState }
    PlanFormulatedEvent    struct{ baseEvent; Plan *plan.Plan }
    ReplanRequestedEvent   struct{ baseEvent; Action string; Reason string }
)

// 执行
type (
    ActionExecutionStartEvent  struct{ baseEvent; Action core.Action; StartedAt time.Time }
    ActionExecutionResultEvent struct{ baseEvent; Action core.Action; Status core.ActionStatus; Duration time.Duration; Err error }
    ObjectBoundEvent           struct{ baseEvent; Key string; Type string }
    GoalAchievedEvent          struct{ baseEvent; Goal *core.Goal }
)

// LLM
type (
    LLMRequestEvent  struct{ baseEvent; Model string; Provider string; Prompt string }
    LLMResponseEvent struct{ baseEvent; Model string; InputTokens, OutputTokens int; Duration time.Duration; Err error }
)
```

#### Listener 和多播

```go
package event

type Listener interface {
    OnEvent(e Event)
}

type Multicast struct {
    mu        sync.RWMutex
    listeners []Listener
}

func (m *Multicast) Add(l Listener)
func (m *Multicast) Remove(l Listener)
func (m *Multicast) OnEvent(e Event)   // 广播；单个 listener panic 不影响其他
```

---

### 5.2 错误/信号类型

```go
package core

// ReplanRequest Action 主动请求重规划（通过 error 接口传递）
type ReplanRequest struct {
    Reason            string
    BlackboardUpdater func(Blackboard)  // 可 nil；非空则重规划前应用
}

func (r *ReplanRequest) Error() string { return "replan requested: " + r.Reason }

// StuckHandlingResult 卡住处理器的返回
type StuckHandlingResult struct {
    Code   StuckHandlingCode
    Reason string
}

type StuckHandlingCode int8
const (
    StuckReplan StuckHandlingCode = iota  // 重规划尝试
    StuckNoResolution                      // 无解，保持 Stuck
)

type StuckHandler interface {
    HandleStuck(ctx context.Context, p *AgentProcess) StuckHandlingResult
}

// EarlyTerminationPolicy 早终止策略
type EarlyTerminationPolicy interface {
    ShouldTerminate(p *AgentProcess) (bool, string)
}

// 内置实现
type MaxActionsPolicy struct{ Max int }
type BudgetPolicy    struct{ Budget Budget }
type CompositePolicy struct{ Policies []EarlyTerminationPolicy }  // OR 逻辑

func (c CompositePolicy) ShouldTerminate(p *AgentProcess) (bool, string) {
    for _, pol := range c.Policies {
        if stop, reason := pol.ShouldTerminate(p); stop { return true, reason }
    }
    return false, ""
}
```

---

## 关系矩阵

谁引用谁（⇒ 表示「类型 A 的字段或方法里出现类型 B」）：

```
Platform ⇒ Agent, ServiceProvider, PlannerFactory, event.Multicast
AgentProcess ⇒ Agent, ProcessOptions, Blackboard, Planner, WorldStateDeterminer, Platform
ProcessContext ⇒ AgentProcess, Blackboard, ProcessOptions, ServiceProvider
Agent ⇒ Action, Goal, Condition, DomainType, StuckHandler
Action ⇒ ActionMetadata（⇒ IoBinding, EffectSpec, ActionQos, ToolGroupRequirement）
Goal ⇒ IoBinding, ExportConfig, CostFunc
Condition ⇒ OperationContext
Blackboard ⇒ （leaf）
Plan ⇒ Action, Goal
PlanningSystem ⇒ Action, Goal, Condition
WorldState ⇒ Determination
Planner ⇒ PlanningSystem, WorldState, Plan
EffectSpec ⇒ Determination
IoBinding ⇒ （primitive）
```

**关键隔离**：
- `Blackboard`、`Determination`、`IoBinding` 是**无依赖的叶子类型**——规划器与运行时都在这之上构建
- `Agent` 是**只读聚合**——创建后通过 `WithXxx` 返回副本
- `AgentProcess` 是**唯一的可变状态载体**——所有状态集中在此，方便审计

---

## 包与文件映射

```
agent/
├── core/                            (Layer 1-2)
│   ├── determination.go             Determination + 方法
│   ├── iobinding.go                 IoBinding + NewIoBinding[T]
│   ├── semver.go                    Semver
│   ├── domain_type.go               DomainType
│   ├── effect_spec.go               EffectSpec + 操作
│   ├── enum.go                      ActionStatus / AgentProcessStatus / PlannerType / ProcessType
│   ├── timed.go                     Timestamped / Timed 接口
│   ├── condition.go                 Condition 接口 + ComputedCondition/AndCondition/...
│   ├── condition_prompt.go          PromptCondition
│   ├── action.go                    Action 接口 + ActionMetadata + ActionQos
│   ├── action_typed.go              NewAction[In,Out] + TypedActionImpl
│   ├── action_options.go            ActionOption + 全部 With*
│   ├── goal.go                      Goal + GoalProducing[T]
│   ├── agent.go                     Agent + AgentMeta
│   ├── blackboard.go                Blackboard 接口 + 泛型 helper
│   ├── service_provider.go          ServiceProvider
│   ├── process_context.go           ProcessContext + OperationContext
│   ├── process_options.go           ProcessOptions + Verbosity + Budget + ProcessControl + Identities
│   ├── output_channel.go            OutputChannel + 内置实现
│   ├── replan.go                    ReplanRequest
│   ├── stuck.go                     StuckHandler + StuckHandlingResult
│   ├── early_termination.go         EarlyTerminationPolicy + 内置策略
│   └── tool_group.go                ToolGroupRequirement + ToolGroupResolver
│
├── plan/                            (Layer 3)
│   ├── world_state.go               WorldState 接口 + ConditionWorldState
│   ├── plan.go                      Plan
│   ├── planning_system.go           PlanningSystem
│   └── planner.go                   Planner 接口
│
├── planner/
│   ├── goap/
│   │   ├── astar.go                 AStarPlanner + searchNode
│   │   ├── heuristic.go             heuristic 实现
│   │   ├── optimize.go              backward/forward optimization
│   │   └── reachability.go          isGoalReachable
│   └── utility/
│       └── utility.go               UtilityPlanner（效用模式）
│
├── runtime/                         (Layer 4)
│   ├── in_memory_blackboard.go      InMemoryBlackboard
│   ├── world_state_determiner.go    BlackboardDeterminer + LogicalExpressionParser
│   ├── platform.go                  Platform + PlatformOption
│   ├── agent_process.go             AgentProcess
│   ├── simple_process.go            SimpleAgentProcess.formulateAndExecutePlan
│   ├── concurrent_process.go        ConcurrentAgentProcess
│   ├── execute_action.go            executeAction + 重试逻辑
│   ├── llm_invocation.go            LLMInvocation 记录
│   └── id_gen.go                    IDGenerator
│
├── event/                           (Layer 5)
│   ├── event.go                     Event 接口 + baseEvent
│   ├── events_platform.go           AgentDeployed/Undeployed
│   ├── events_process.go            所有 Process* 事件
│   ├── events_plan.go               ReadyToPlan / PlanFormulated / ReplanRequested
│   ├── events_action.go             ActionExecutionStart/Result + ObjectBound + GoalAchieved
│   ├── events_llm.go                LLMRequest/Response
│   ├── listener.go                  Listener 接口 + Multicast
│   └── observation_bridge.go        事件 → observation 桥接
│
├── dsl/                             (用户 API)
│   └── builder.go                   New() / Builder / Build()
│
├── reflect/                         (用户 API 便利层)
│   └── register.go                  Register(instance) → *Agent
│
├── hitl/
│   ├── awaitable.go                 Awaitable 接口
│   ├── confirmation.go              ConfirmationRequest
│   └── form.go                      FormRequest
│
└── internal/
    └── hashutil.go                  state hash 计算（WorldState.HashKey）
```

---

## 不变式速查表

| 类型 | 关键不变式 |
|-----|----------|
| `Determination` | 值 ∈ {0, 1, 2}；零值 = Unknown |
| `IoBinding` | `Type` 非空；`Name == ""` 视同 `"it"` |
| `EffectSpec` | `nil` 等价空 map |
| `Agent.Name` | 创建后不变 |
| `Action.Metadata()` | 返回值只读（指针指向常量结构） |
| `Goal.Name` | 创建后不变 |
| `Plan.Actions` | 不含 nil；顺序有意义 |
| `WorldState` | 完全不可变；Apply 返回新实例 |
| `AgentProcess.ID` | 全局唯一；创建后不变 |
| `AgentProcess.Agent` / `.Options` / `.StartedAt` | 创建后不变 |
| `InMemoryBlackboard.id` | 创建后不变；所有可变字段受 mu 保护 |
| `Event` 字段 | 发布后不可修改（listener 视为只读） |

---

## 线程安全分级

| 级别 | 类型 | 说明 |
|-----|------|-----|
| **完全不可变** | `Determination`, `IoBinding`, `ActionMetadata`, `ActionQos`, `Goal`, `Plan`, `WorldState`, Event 类 | 可在多 goroutine 自由共享 |
| **构造后不可变** | `Agent`, `PlanningSystem` | 创建期单线程；之后只读共享 |
| **读并发安全** | `AgentProcess`（Status/History/...）, `InMemoryBlackboard`, `event.Multicast` | 用 RWMutex 保护 |
| **单线程调用** | `AgentProcess.Run`/`Tick`, `Builder` | 只能由一个 goroutine 调用 |
| **内部同步** | `Platform`（Deploy/Run）, `observation.Registry` 实现 | 方法本身加锁 |

---

## 零值友好度

**所有公开类型都应有合理零值**：

| 类型 | 零值行为 |
|-----|---------|
| `Determination{}` | = `Unknown` |
| `IoBinding{}` | = `"it:"`（类型空，仅用作 placeholder） |
| `EffectSpec(nil)` | 作空 map 使用，不 panic |
| `ActionQos{}` | 所有重试字段 = 0，需构造器 `DefaultActionQos()` 填默认 |
| `ProcessOptions{}` | 所有默认；Verbosity/Budget 内嵌零值也是合理默认 |
| `Budget{}` | **不是合理默认**（0 代价 / 0 action），需用 `DefaultBudget()` |

**规则**：Budget、ActionQos 这类**全零会破坏运行**的类型必须**禁止直接字面量**——构造器强制走 `DefaultXxx()`，或提供 `MustXxx` 校验。

---

## 对应 embabel 的等价速查

| embabel (Kotlin) | Lynx (Go) |
|-----------------|-----------|
| `ConditionDetermination` enum | `core.Determination` |
| `data class IoBinding` | `core.IoBinding` |
| `interface Condition` | `core.Condition` |
| `interface Action` | `core.Action` + `ActionMetadata` |
| `data class Goal` | `core.Goal` |
| `data class Agent` | `core.Agent` |
| `interface Blackboard` | `core.Blackboard` |
| `InMemoryBlackboard` | `runtime.InMemoryBlackboard` |
| `interface WorldState` | `plan.WorldState` |
| `ConditionWorldState` | `plan.ConditionWorldState` |
| `data class Plan` | `plan.Plan` |
| `interface PlanningSystem` | `plan.PlanningSystem` |
| `interface Planner<S,W,P>` | `plan.Planner` |
| `AStarGoapPlanner` | `goap.AStarPlanner` |
| `interface AgentProcess` | `runtime.AgentProcess` |
| `AbstractAgentProcess` | `runtime.AgentProcess`（直接是 concrete，没 abstract） |
| `SimpleAgentProcess.formulate...` | `runtime.simpleFormulateAndExecute` |
| `ConcurrentAgentProcess.formulate...` | `runtime.concurrentFormulateAndExecute` |
| `data class ProcessContext` | `core.ProcessContext` |
| `data class ProcessOptions` | `core.ProcessOptions` |
| `interface AgentPlatform` | `runtime.Platform` |
| `DefaultAgentPlatform` | `runtime.Platform`（唯一实现） |
| `interface AgenticEventListener` | `event.Listener` |
| `MulticastAgenticEventListener` | `event.Multicast` |
| `AgenticEvent` 层次 | `event.Event` + 15 个 struct |
| `throw ReplanRequestedException` | `return &core.ReplanRequest{...}` |
| `interface StuckHandler` | `core.StuckHandler` |
| `EarlyTerminationPolicy` | `core.EarlyTerminationPolicy` |
| `OutputChannel` | `core.OutputChannel` |
| `data class ActionQos` | `core.ActionQos` |
| `data class ActionInvocation` | `runtime.ActionInvocation` |
| `LlmInvocation` | `runtime.LLMInvocation` |

---

## 下一步

这份目录是**实现阶段的蓝图**。建议的落地节奏：
1. **Day 1-2**：Layer 1（原语）全部落地 + 单测
2. **Day 3-5**：Layer 2（Action/Goal/Agent/Condition）落地 + DSL Builder
3. **Day 6-10**：Layer 3（WorldState/Plan/PlanningSystem）+ Layer 4 最小版（InMemoryBlackboard、AgentProcess、Platform）+ 顺序 fallback 规划器
4. **Day 11-15**：GOAP A* 完整实现 + 事件系统
5. **Day 16-20**：QoS 重试 + 并发 Process + HITL

与 `06-rollout.md` 的 M1-M4 阶段完全对齐。

---

*（配套阅读：`02-core-abstractions.md` 讲「为什么」，本文讲「是什么」；`03-planner-and-runtime.md` 讲「怎么跑」）*
