# 02. 核心抽象的 Go 定义

> 本文件给出 embabel-agent 核心类型的 Go 等价设计。所有 Go 代码是**伪代码 / 设计稿**，不保证编译，但表达了意图。

---

## 1. Determination — 三值逻辑

embabel 用 `ConditionDetermination` 枚举表达「真/假/未知」：

```go
// agent/core/determination.go
package core

type Determination int8

const (
    Unknown Determination = iota  // 未知（还没评估出结果）
    True                           // 成立
    False                          // 不成立
)

func (d Determination) String() string { ... }

// 逻辑运算（与 Kotlin operator and/or/not 等价）
func (d Determination) And(other Determination) Determination {
    // FALSE 占优；遇到 UNKNOWN 传播
    if d == False || other == False { return False }
    if d == Unknown || other == Unknown { return Unknown }
    return True
}
func (d Determination) Or(other Determination) Determination {
    if d == True || other == True { return True }
    if d == Unknown || other == Unknown { return Unknown }
    return False
}
func (d Determination) Not() Determination {
    switch d {
    case True:  return False
    case False: return True
    default:    return Unknown
    }
}
```

**关键**：三值逻辑是 GOAP 规划器能处理「不完全信息」的基石，Go 实现完全复刻 Kotlin。

---

## 2. IoBinding — 输入/输出绑定

Kotlin 用 `@JvmInline value class IoBinding(val value: String)` 包装字符串 `"varName:TypeName"`。

Go 版本：

```go
// agent/core/iobinding.go
package core

import "reflect"

const (
    DefaultBinding    = "it"          // 默认变量名
    LastResultBinding = "lastResult"  // @Trigger 专用
)

// IoBinding 描述一个输入或输出绑定："varName:com.example.Type"
type IoBinding struct {
    Name string  // 变量名，默认 "it"
    Type string  // Go 类型全名（带 package 路径）
}

func (b IoBinding) String() string {
    if b.Name == "" {
        b.Name = DefaultBinding
    }
    return b.Name + ":" + b.Type
}

// NewIoBinding 从 Go 反射类型构造
func NewIoBinding[T any](name string) IoBinding {
    var zero T
    rt := reflect.TypeOf(zero)
    if rt == nil {
        // T 是 interface，需要 runtime 解析
        rt = reflect.TypeOf((*T)(nil)).Elem()
    }
    return IoBinding{
        Name: stringOrDefault(name, DefaultBinding),
        Type: rt.PkgPath() + "." + rt.Name(),
    }
}

func ParseIoBinding(s string) IoBinding { /* 解析 "name:Type" */ }
```

Go 的类型名比 Java FQN 略繁琐（`pkg/path.TypeName`），但仍然是字符串可比较的——规划器需要这一点。

---

## 3. Condition — 条件接口

### Kotlin
```kotlin
interface Condition : ConditionMetadata, HasInfoString {
    val name: String
    val cost: ZeroToOne
    fun evaluate(context: OperationContext): ConditionDetermination
}
```

### Go

```go
// agent/core/condition.go
package core

import "context"

// Condition 是一个命名的谓词，可在给定上下文中求值。
type Condition interface {
    Name() string
    Cost() float64  // 0.0-1.0，给规划器估算代价
    Evaluate(ctx context.Context, oc *OperationContext) Determination
}

// OperationContext 传给 Condition.Evaluate 的上下文，持有黑板、LLM 等
type OperationContext struct {
    Process *AgentProcess
    Bb      Blackboard
    // 注意：不再用 ThreadLocal；Condition 通过参数访问
}

// ComputedCondition 函数式实现（最常用）
type ComputedCondition struct {
    name string
    cost float64
    fn   func(ctx context.Context, oc *OperationContext) Determination
}

func NewCondition(name string, fn func(ctx context.Context, oc *OperationContext) Determination) *ComputedCondition {
    return &ComputedCondition{name: name, fn: fn}
}

func (c *ComputedCondition) Name() string                                                 { return c.name }
func (c *ComputedCondition) Cost() float64                                                { return c.cost }
func (c *ComputedCondition) Evaluate(ctx context.Context, oc *OperationContext) Determination { return c.fn(ctx, oc) }

// 布尔组合子（替代 Kotlin 的 operator and/or/not）
func And(a, b Condition) Condition { /* 短路 */ }
func Or(a, b Condition) Condition  { /* 短路 */ }
func Not(a Condition) Condition    { /* Determination.Not */ }

// PromptCondition 通过 LLM 判断（实验性，对应 embabel 的 PromptCondition）
type PromptCondition struct {
    name   string
    prompt string
    llm    *chat.Client  // 依赖 lynx core/model/chat
}
```

---

## 4. Blackboard — 黑板

### Kotlin 要点
- 同时存储 "objects"（无名对象列表）和 key→value 的 map
- 按类型查找（含父类型匹配）
- 可 `spawn()` 子黑板（继承父内容）
- 可 `hide()` 对象
- 线程安全

### Go 设计

```go
// agent/core/blackboard.go
package core

import (
    "reflect"
    "sync"
)

type Blackboard interface {
    // ID
    ID() string

    // Key-value API
    Set(key string, value any)
    Get(key string) (any, bool)

    // Typed lookup（按变量名+类型）
    // variable="it" 时返回最新同类型对象
    GetValue(variable, typeName string) (any, bool)
    HasValue(variable, typeName string) bool

    // 对象列表
    AddObject(value any)
    Objects() []any

    // 命名条件（显式 setCondition）
    SetCondition(key string, value bool)
    GetCondition(key string) (bool, bool)

    // 批量 / 保护
    BindAll(m map[string]any)
    BindProtected(key string, value any)  // spawn 子黑板时保留
    Hide(target any)                       // 不可检索但不删除

    // 子黑板
    Spawn() Blackboard
    Clear()
}

// Generic 顶层 helper（替代 Kotlin 泛型方法）
func Get[T any](bb Blackboard, name string) (T, bool) {
    rt := reflect.TypeOf((*T)(nil)).Elem()
    v, ok := bb.GetValue(name, rt.PkgPath()+"."+rt.Name())
    if !ok {
        var zero T
        return zero, false
    }
    return v.(T), true
}

func ObjectsOfType[T any](bb Blackboard) []T {
    var out []T
    for _, obj := range bb.Objects() {
        if v, ok := obj.(T); ok {
            out = append(out, v)
        }
    }
    return out
}

func Last[T any](bb Blackboard) (T, bool) {
    list := ObjectsOfType[T](bb)
    if len(list) == 0 {
        var zero T
        return zero, false
    }
    return list[len(list)-1], true
}
```

### InMemory 实现

```go
// agent/runtime/inmemory_blackboard.go
type InMemoryBlackboard struct {
    id         string
    mu         sync.RWMutex
    named      map[string]any      // key -> value
    protected  map[string]bool     // 哪些 key 是 protected
    objects    []any               // 按添加顺序
    hidden     map[any]struct{}    // 隐藏对象
    conditions map[string]bool     // 显式条件
}

func (b *InMemoryBlackboard) Set(key string, value any) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.named[key] = value
    // 同步加入 objects 末尾（"it" 语义：最近的）
    b.objects = append(b.objects, value)
}

func (b *InMemoryBlackboard) GetValue(variable, typeName string) (any, bool) {
    b.mu.RLock()
    defer b.mu.RUnlock()

    // 变量名为 "it" 时找类型匹配的最新对象
    if variable == DefaultBinding {
        for i := len(b.objects) - 1; i >= 0; i-- {
            if _, hidden := b.hidden[b.objects[i]]; hidden {
                continue
            }
            if typeMatches(b.objects[i], typeName) {
                return b.objects[i], true
            }
        }
        return nil, false
    }

    // 显式变量名：先查 named
    if v, ok := b.named[variable]; ok && typeMatches(v, typeName) {
        return v, true
    }
    return nil, false
}

func (b *InMemoryBlackboard) Spawn() Blackboard {
    b.mu.RLock()
    defer b.mu.RUnlock()

    child := &InMemoryBlackboard{
        id:         newID(),
        named:      maps.Clone(b.named),
        protected:  maps.Clone(b.protected),
        objects:    slices.Clone(b.objects),
        hidden:     maps.Clone(b.hidden),
        conditions: maps.Clone(b.conditions),
    }
    return child
}
```

**类型匹配**：`typeMatches(obj, typeName)` 要支持**接口匹配 + 嵌入类型匹配**，对应 Kotlin 的子类型检查。用 reflect 实现。

---

## 5. Action — 行动接口

### Kotlin 要点
- `inputs: Set<IoBinding>` / `outputs: Set<IoBinding>`
- `preconditions: EffectSpec` / `effects: EffectSpec`（自动从 inputs/outputs 推断）
- `canRerun: Boolean`
- `qos: ActionQos`（重试策略）
- `execute(processContext): ActionStatus`

### Go 设计

```go
// agent/core/action.go
package core

import "context"

// Action 是 agent 的单个可执行步骤
type Action interface {
    // 元数据
    Name() string
    Description() string
    Inputs() []IoBinding
    Outputs() []IoBinding
    Preconditions() EffectSpec
    Effects() EffectSpec
    CanRerun() bool
    ReadOnly() bool
    QoS() ActionQos
    ToolGroups() []ToolGroupRequirement
    Cost(ws WorldState) float64
    Value(ws WorldState) float64

    // 执行
    Execute(ctx context.Context, pc *ProcessContext) ActionStatus
}

// EffectSpec 条件到 Determination 的映射
type EffectSpec map[string]Determination

// ActionStatus 执行结果
type ActionStatus int8
const (
    ActionSucceeded ActionStatus = iota
    ActionFailed
    ActionWaiting   // 需等待外部输入（HITL）
    ActionPaused
)

// ActionQos 重试/超时配置
type ActionQos struct {
    MaxAttempts       int
    BackoffMillis     int64
    BackoffMultiplier float64
    BackoffMaxMillis  int64
    Idempotent        bool
}

func DefaultActionQos() ActionQos {
    return ActionQos{MaxAttempts: 5, BackoffMillis: 10_000, BackoffMultiplier: 5.0, BackoffMaxMillis: 60_000}
}
```

### 类型安全的 Action 构造器（Go 泛型的亮点）

这是 Lynx 超越 embabel 的地方——**编译期类型安全**：

```go
// agent/core/action_typed.go
package core

// TypedAction 是用户提供的类型化函数
type TypedAction[In, Out any] func(ctx context.Context, pc *ProcessContext, input In) (Out, error)

// NewAction 把类型化函数包装成 Action 接口
func NewAction[In, Out any](
    name string,
    fn TypedAction[In, Out],
    opts ...ActionOption,
) Action {
    inputBinding  := NewIoBinding[In](DefaultBinding)
    outputBinding := NewIoBinding[Out](DefaultBinding)

    a := &typedActionImpl[In, Out]{
        name:    name,
        inputs:  []IoBinding{inputBinding},
        outputs: []IoBinding{outputBinding},
        fn:      fn,
        qos:     DefaultActionQos(),
    }
    for _, opt := range opts {
        opt(a)
    }
    a.computePreconditionsAndEffects()
    return a
}

type typedActionImpl[In, Out any] struct {
    name    string
    inputs  []IoBinding
    outputs []IoBinding
    fn      TypedAction[In, Out]
    qos     ActionQos
    pre     []string
    post    []string
    canRerun bool
    // ... preconditions, effects 自动计算后填入
}

func (a *typedActionImpl[In, Out]) Execute(ctx context.Context, pc *ProcessContext) ActionStatus {
    // 1. 从黑板拉取 In 类型的对象
    in, ok := Get[In](pc.Blackboard, a.inputs[0].Name)
    if !ok {
        // 无法满足输入 → 失败
        return ActionFailed
    }

    // 2. 执行类型化函数（带重试）
    var out Out
    var err error
    for attempt := 0; attempt < a.qos.MaxAttempts; attempt++ {
        out, err = a.fn(ctx, pc, in)
        if err == nil {
            break
        }
        time.Sleep(backoff(a.qos, attempt))
    }
    if err != nil {
        pc.SetFailure(err)
        return ActionFailed
    }

    // 3. 写回黑板
    pc.Blackboard.Set(a.outputs[0].Name, out)
    return ActionSucceeded
}
```

**关键**：用户写的是 `func(ctx, pc, UserInput) (Intent, error)`，**类型完全保留到编译期**。框架内部用 `reflect.TypeOf` 一次性算出 `IoBinding`，之后运行时无反射。

### Preconditions/Effects 自动计算

对应 `AbstractAction.preconditions`（embabel 的核心魔法）：

```go
// agent/core/abstract_action.go
func (a *typedActionImpl[In, Out]) computePreconditionsAndEffects() {
    pre := EffectSpec{}
    eff := EffectSpec{}

    // 显式 pre 列表
    for _, p := range a.pre {
        pre[p] = True
    }
    // 每个 input 绑定 → True 前提
    for _, in := range a.inputs {
        pre[in.String()] = True
    }

    // 显式 post 列表
    for _, p := range a.post {
        eff[p] = True
    }
    // 每个 output 绑定（含父子类型） → True 效果
    for _, out := range a.outputs {
        for _, typ := range typeHierarchy(out.Type) {
            eff[out.Name+":"+typ] = True
        }
    }

    // canRerun=false 的特殊逻辑
    if !a.canRerun {
        for _, out := range a.outputs {
            pre[out.String()] = False  // 输出尚未存在
        }
        pre[hasRunKey(a.name)] = False
    }
    eff[hasRunKey(a.name)] = True  // 执行后标记

    a.preconditions = pre
    a.effects = eff
}
```

---

## 6. Goal — 目标

### Go 设计

```go
// agent/core/goal.go
package core

// Goal 是规划器追求的目标状态
type Goal struct {
    Name        string
    Description string
    Pre         []string       // 显式前提条件
    Inputs      []IoBinding    // 输入类型要求
    OutputType  *string        // 目标产出类型（可选）
    Value       float64        // 静态价值（0-1）；动态用 ValueFn
    ValueFn     ValueFunc      // 动态价值计算
    Tags        []string
    Examples    []string
    Export      ExportConfig
}

// ValueFunc 根据当前世界状态计算目标价值
type ValueFunc func(ws WorldState) float64

type ExportConfig struct {
    Name    string
    Remote  bool
    Local   bool
}

// Preconditions 自动合并 Pre + Inputs
func (g *Goal) Preconditions() EffectSpec {
    spec := EffectSpec{}
    for _, p := range g.Pre {
        spec[p] = True
    }
    for _, in := range g.Inputs {
        spec[in.String()] = True
    }
    return spec
}

// 泛型工厂（等价 Goal.createInstance）
func GoalProducing[T any](description string) *Goal {
    rt := reflect.TypeOf((*T)(nil)).Elem()
    typeName := rt.PkgPath() + "." + rt.Name()
    return &Goal{
        Name:        "produce_" + typeName,
        Description: description,
        Inputs:      []IoBinding{NewIoBinding[T](DefaultBinding)},
        OutputType:  &typeName,
    }
}
```

---

## 7. Agent — Agent 数据模型

```go
// agent/core/agent.go
package core

// Agent 是一组 Actions + Goals + Conditions 的聚合
type Agent struct {
    Name         string
    Provider     string
    Version      Semver
    Description  string
    Actions      []Action
    Goals        []*Goal
    Conditions   []Condition
    StuckHandler StuckHandler
    Opaque       bool
    DomainTypes  []DomainType
}

// WithSingleGoal 返回只含单个目标的副本（聚焦执行）
func (a *Agent) WithSingleGoal(goal *Goal) *Agent { ... }

// PlanningSystem 组装成规划器输入
func (a *Agent) PlanningSystem() *plan.PlanningSystem {
    return &plan.PlanningSystem{
        Actions:    a.Actions,
        Goals:      a.Goals,
        Conditions: a.Conditions,
    }
}

// Clone / WithXxx 等不可变更新方法
```

---

## 8. ProcessContext — 执行上下文

对应 Kotlin 的 `ProcessContext`，是 Action 执行时唯一传入的上下文载体。

```go
// agent/core/process_context.go
package core

import (
    "github.com/Tangerg/lynx/core/model/chat"
    "github.com/Tangerg/lynx/core/observation"
)

// ProcessContext 传给 Action.Execute 的上下文
type ProcessContext struct {
    ProcessOptions *ProcessOptions
    Process        *AgentProcess        // 当前进程（替代 ThreadLocal）
    Blackboard     Blackboard           // 黑板（= Process.Blackboard 的便捷访问）
    OutputChannel  OutputChannel        // 向用户推送消息
    Listener       EventListener        // 事件发布

    // 集成 Lynx 核心组件（关键改动：不再是 Spring AI）
    chatClient   *chat.Client
    observations observation.Registry
}

// LLM 返回 Lynx chat 客户端，Action 代码直接用
func (pc *ProcessContext) LLM() *chat.Client {
    return pc.chatClient
}

// Observe 启动 observation span
func (pc *ProcessContext) Observe(ctx context.Context, name string, attrs ...observation.Attr) (context.Context, observation.Observation) {
    return pc.observations.Start(ctx, name, attrs...)
}

// SetFailure / OnEvent 等
func (pc *ProcessContext) SetFailure(err error) { pc.Process.setFailure(err) }
func (pc *ProcessContext) OnEvent(e Event)      { pc.Listener.OnEvent(e) }
```

**重要设计**：`ProcessContext` 里的 `LLM()` 不是可选字段——**Agent 框架必须绑定 chat.Client**，这是 LLM 驱动 agent 的前提。

---

## 9. ProcessOptions — 进程配置

```go
// agent/core/process_options.go
package core

type ProcessOptions struct {
    ContextID            string
    Verbosity            Verbosity
    Budget               Budget
    ProcessControl       ProcessControl
    Prune                bool
    Listeners            []EventListener
    OutputChannel        OutputChannel
    PlannerType          PlannerType   // GOAP / Utility
    ToolCallContext      map[string]any
    Blackboard           Blackboard    // 可选：复用已有黑板
}

type Verbosity struct {
    ShowPrompts     bool
    ShowLLMResponses bool
    Debug           bool
    ShowPlanning    bool
}

type Budget struct {
    CostLimit   float64  // 默认 2.0
    ActionLimit int      // 默认 50
    TokenLimit  int      // 默认 1_000_000
}

type ProcessControl struct {
    EarlyTerminationPolicy EarlyTerminationPolicy
    ToolDelay              time.Duration
    OperationDelay         time.Duration
}

// Functional options 构造
type ProcessOptionFunc func(*ProcessOptions)

func WithBudget(b Budget) ProcessOptionFunc { ... }
func WithVerbosity(v Verbosity) ProcessOptionFunc { ... }
// ...

func NewProcessOptions(opts ...ProcessOptionFunc) *ProcessOptions { ... }
```

---

## 10. 类型层次图

```
core/
├── Agent                    （struct）
│   ├── Actions: []Action    （Action 接口，多种实现）
│   ├── Goals: []*Goal       （struct）
│   └── Conditions: []Condition

├── Action（接口）
│   └── typedActionImpl[In, Out]  ← NewAction[In,Out] 生成
│   └── customActionImpl           ← 手动实现接口
│
├── Condition（接口）
│   ├── ComputedCondition    （函数式）
│   ├── PromptCondition      （LLM 驱动）
│   ├── AndCondition / OrCondition / NotCondition  （组合）
│
├── Blackboard（接口）
│   └── InMemoryBlackboard   （默认实现）
│
├── ProcessContext (struct)
│   └── 嵌入 chat.Client、observation.Registry 引用
│
└── AgentProcess (struct, 见 03)
    └── 状态机 + tick 循环
```

---

下一份文档详细描述 `AgentProcess` 状态机与 GOAP A* 规划器的 Go 实现 → `03-planner-and-runtime.md`
