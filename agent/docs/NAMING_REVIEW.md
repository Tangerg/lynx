# `agent/` 命名风格 review — Java 残留清理建议

扫了 100 个非测试 .go 文件后整理出来的命名问题，按"Java 味浓度"从
高到低排。每条都列了：**当前 → 建议 → 文件位置 → 原因 / 改的代价**。

---

## 1. `InfoString(verbose bool) string` — 最 Java 的写法

`core/blackboard.go:43` / `runtime/in_memory_blackboard.go:242`

```go
type BlackboardReader interface {
    InfoString(verbose bool) string  // ← Java toString-with-options 范式
}
func InspectInfoString(bb BlackboardReader, verbose bool) string { ... }
```

**问题**：
- Go 的格式化约定是 `fmt.Stringer` 的 `String() string`（无参）。
  带参数的 `InfoString(bool)` 不在任何标准接口里。
- `Inspect` 前缀的 helper 又把名字拼成 `InspectInfoString` — 两个动词
  叠加。

**建议**：
```go
type BlackboardReader interface {
    // 方案 A：用动词 + bool 参数
    Format(verbose bool) string
    // 方案 B：拆成两个方法（最 Go）
    String() string                 // fmt.Stringer 短摘要
    Dump() string                   // 完整 dump
}

// helper:
func FormatBlackboard(bb BlackboardReader, verbose bool) string { ... }
```

推荐 B：`String()` 默认上 fmt 友好的短摘要，`Dump()` 给调试。

---

## 2. `Get*` 前缀访问器 — Java getter

```go
// core/blackboard.go
GetValue(variable, typeName string) (any, bool)
GetCondition(key string) (bool, bool)

// runtime/platform.go:143
func (p *Platform) GetProcess(id string) (*AgentProcess, bool)
```

**问题**：Go effective-go 明确反对 getter 加 `Get` 前缀。

**建议**：
- `Blackboard.GetValue` → `Lookup`（两个返回值 + 类型断言的查找语义）
- `Blackboard.GetCondition` → `Condition`（与 `SetCondition` 对称）
- `Platform.GetProcess` → `Process` 会与 `*AgentProcess` 重名歧义，
  推荐 `ProcessByID(id)` 或 `LookupProcess(id)`

注意：`SetCondition` 的 `Set` 前缀是 Go 习惯的（mutator 用 `Set`），
保留。

---

## 3. `IDStr` 字段 — Java 避名冲突的产物 ⏸️ DEFERRED

`hitl/awaitable.go:26`

```go
type TypedRequest[P, R any] struct {
    IDStr   string                        // ← 为了不和 ID() 方法重名才叫 IDStr
    Payload P
    Handler func(R) core.ResponseImpact
}
func (r *TypedRequest[P, R]) ID() string { return r.IDStr }
```

**问题**：典型的 Java 模式 — 字段必须导出（为了 JSON 等），但又想暴露
方法，所以加 `Str` 后缀避免冲突。Go 的解法是任选一个：

**建议**（任选其一）：
```go
// 方案 A：直接用 ID 字段，删 ID() 方法
type TypedRequest[P, R any] struct {
    ID      string
    Payload P
    Handler func(R) core.ResponseImpact
}

// 方案 B：保留方法，字段改小写 + JSON tag
type TypedRequest[P, R any] struct {
    id      string  `json:"id"`
    Payload P       `json:"payload"`
    Handler func(R) core.ResponseImpact
}
func (r *TypedRequest[P, R]) ID() string { return r.id }
```

A 更 Go。

---

## 4. `ResponseImpact*` 长前缀枚举 — Java enum 风

`core/awaitable.go:10-11`

```go
const (
    ResponseImpactUnchanged ResponseImpact = iota
    ResponseImpactUpdated
)
```

**对照同仓库**：
- `Determination` 用 `Unknown / True / False`（最 Go）
- `StuckHandlingCode` 用 `StuckReplan` 等（一截前缀）
- `ResponseImpact` 用全名当前缀（最 Java）

**建议**：
```go
const (
    ImpactUnchanged ResponseImpact = iota
    ImpactUpdated
)
```

或者更激进（参考 Determination）：

```go
const (
    Unchanged ResponseImpact = iota  // 但会污染包顶层命名空间
    Updated
)
```

推荐第一种，把 `Response` 去掉留 `Impact*`。

---

## 5. `OperationContext` — 唯一可替换的 `*Context` 类型

`core/condition.go:19`

```go
type OperationContext struct {
    Process Process
    World   WorldState
    ...
}

func (c Condition) Evaluate(ctx context.Context, oc *OperationContext) Determination
```

**问题**：
- 与 `context.Context` 同名后缀，容易让读者误以为是 ctx 派生类型。
- "Operation" 本身没说明这是给 Condition 用的。

**对比** `ProcessContext`：那个的"Context"指代"the running process's
runtime context"，至少有"语义代号"。但 `OperationContext` 完全可以
用具体角色名替代。

**建议**：
```go
type ConditionEnv struct {  // 或 ConditionInput / EvalScope
    Process Process
    World   WorldState
    ...
}

func (c Condition) Evaluate(ctx context.Context, env *ConditionEnv) Determination
```

至于 `ProcessContext` — 因为它已经是行业熟词（cobra `Command.Context()`
也类似），且改动面太大，**保留但加 alias** 或不动。

---

## 6. `HasRunKey() string` — 谓词命名却返回 string

`core/action.go:54`

```go
func (m ActionMetadata) HasRunKey() string {
    if m.RunKey != "" { return m.RunKey }
    return m.Name
}
```

**问题**：`Has*` 在 Go（和 Java）都是 bool predicate 约定。这里返回
string，名字误导。

**建议**：
```go
func (m ActionMetadata) EffectiveRunKey() string { ... }
// 或
func (m ActionMetadata) ResolvedRunKey() string { ... }
```

如果想再加 bool 谓词：

```go
func (m ActionMetadata) HasRunKey() bool { return m.RunKey != "" }
```

---

## 7. `TypeFullNameOf[T]()` — 冗长

`core/io_binding.go:93`

```go
func TypeFullNameOf[T any]() string { ... }
```

**问题**：`FullName` + `Of` 把信息说了两遍。

**建议**：
```go
func TypeName[T any]() string { ... }   // 简洁
func TypeKey[T any]() string { ... }    // 强调用途（key into blackboard）
```

调用面 4 处，改不算贵。

---

## 8. `ToolCallCanceller` 类型别名

`core/process_context.go:37`

```go
type ToolCallCanceller func(cancel context.CancelFunc) (release func())
```

**问题**：双 L 拼写正确但少见；又是函数别名却没用 `Func` 后缀。

**建议**：
```go
type ToolCallCancelFunc func(cancel context.CancelFunc) (release func())
```

跟 `ConditionFunc` / `ToolFunc` 一致。

---

## 9. `*Resolver` 类型名过长

```go
core.StaticToolGroupResolver      // 31 字符
runtime.MCPToolGroupResolver      // 22 字符
```

**问题**：Go 包前缀已经表达了 `mcp` / `core`，类型名里再 `MCP` /
`ToolGroup` 是重复。

**建议**：
```go
// runtime/mcp.go
type ToolGroupResolver struct { ... }  // 调用方写 mcp.ToolGroupResolver
// 或干脆
type Resolver struct { ... }           // 调用方写 mcp.Resolver

// core/tool_group.go
type StaticResolver struct { ... }     // 调用方写 core.StaticResolver
```

`mcp.MCPFoo` 是典型 stutter，effective-go 明文反对。

---

## 10. `*Spec` / `*Options` / `*Config` 不统一

```go
// workflow/ 全用 Spec
ScatterGatherSpec, RepeatUntilAcceptableSpec, RepeatUntilSpec,
LoopSpec, ConsensusSpec, ParallelSpec

// plan / process 用 Options
PlanOptions, ProcessOptions

// runtime / core 用 Config
PlatformConfig, AgentConfig, ActionConfig, ProcessContextConfig,
LLMRankerConfig, LLMPlanRankerConfig
```

**问题**：三种后缀同时存在，读者要切换习惯。

**建议**：统一成 `Config`（Go 生态最普遍：`http.Server.Config`、
`tls.Config`、`grpc.ClientConfig` 等）。

迁移成本：`workflow/*.go` 6 个 Spec 改成 Config，调用面在 workflow
内部 + 测试。

---

## 11. `EffectSpec map[string]Determination` — 误导性 Spec

`core/action.go:98`

```go
type EffectSpec map[string]Determination
```

**问题**：map 本身就是"集合"，叫 `Spec`（spec 通常是说"配置规格"）
不贴切。Go 里更常见的命名是 `EffectSet`（参见 `sets.Set`）或
`Effects`。

**建议**：
```go
type Effects map[string]Determination
```

调用面（`Preconditions Effects` / `Effects Effects`）会有字段名 ==
类型名的情况，Go 这种写法很常见且推荐。

---

## 12. `ActionInterceptor` — AOP 味

`core/extension.go:24`

```go
type ActionInterceptor interface {
    InterceptAction(...)
}
```

**问题**：`Interceptor` 是 Spring AOP / Java EE 词汇。Go 生态更常用
`Middleware` 或前缀化具体行为。

**建议**（视职责定）：
```go
type ActionMiddleware interface { ... }
// 或
type ActionDecorator interface { ... }
```

如果它真的只是 "前后 hook"，叫 `ActionHook` 也行（和 `StuckHandler`
等命名一致）。

---

## 13. `ProcessContextConfig` — 名字嵌套两层

`core/process_context.go:43`

```go
type ProcessContextConfig struct { ... }
```

**建议**：
```go
type ProcessConfig struct { ... }
// 或者直接合到 ProcessOptions 里 — Config 和 Options 当前是分裂的
```

---

## 14. `*Snapshotter` / `*Restorer` — 单方法接口的 -er 命名

`runtime/process_snapshot.go:157-165`

```go
type BlackboardSnapshotter interface { Snapshot() ... }
type BlackboardRestorer interface { Restore(...) ... }
```

**评价**：这俩**已经是 Go 风格**（动词 + -er = 单方法接口），没问题。
唯一可以小调整：合二为一为 `BlackboardSerializer { Snapshot()
Restore() }`，但拆开也成立（写者 / 读者分离）。**不动**。

---

## 15. `CounterIDGenerator` — 包里同义重复

`core/id_gen.go:37`

```go
type CounterIDGenerator struct { ... }
type RandomIDGenerator struct { ... }   // 同文件假定有
```

**问题**：`ID` 包是 `core`，`Generator` 接口可能叫 `IDGenerator`。具体
类型再叫 `CounterIDGenerator` 就重复了。

**建议**：
```go
type IDGenerator interface { Next() string }
type Counter struct { ... }   // core.Counter
type Random struct { ... }    // core.Random
```

或者退一步只去掉 ID：
```go
type CounterGen struct { ... }
type RandomGen struct { ... }
```

---

## 16. `event.NamedListener` — 字段 + 方法 + 接口三层包装

`event/listener.go`

```go
type NamedListener struct {
    name string
    fn   func(Event)
}
func NewNamedListener(name string, fn func(Event)) *NamedListener { ... }
func (l *NamedListener) Name() string { return l.name }
func (l *NamedListener) OnEvent(e Event) { l.fn(e) }
```

**评价**：本身没问题。但和 `multicast.go` 里 `ListenerFunc` 适配器
重复（`ListenerFunc` 是裸函数适配，`NamedListener` 是带名字的）。
**保留但归并**：把 `NamedListener` 合并到 `multicast.go` 里，文件名
和包内层级更扁。

---

## 17. `OnResponseAny(response any)` — 双方法显得多余

`core/awaitable.go:39` + `hitl/awaitable.go:59`

```go
type Awaitable interface {
    OnResponseAny(response any) (ResponseImpact, error)
}

type TypedAwaitable[R any] interface {
    OnResponse(r R) ResponseImpact
}
```

**问题**：`OnResponseAny` 在 Java 里叫 "raw / type-erased version"。
Go 通常通过泛型 + 类型断言桥接。当前的 dual API 看着可以但**命名**
冗余 — `Any` 后缀是 Java 的 `Object`-style 标记。

**建议**：
```go
type Awaitable interface {
    Respond(response any) (ResponseImpact, error)  // 动词，去 Any
}
```

或者保留 `OnResponse` 名字但用 ctx 标注 untyped：

```go
type Awaitable interface {
    // OnResponse accepts an untyped response; impls type-assert.
    OnResponse(response any) (ResponseImpact, error)
}
```

---

## 18. 包名口吃 (stutter) — `plan/` 包是重灾区

按 effective-go 的 stutter 规则：导出标识符不应以包名为前缀。从外部
导入视角看，`pkg.PkgFoo` 形式都算口吃。

### 严重口吃（强烈建议改）

`plan/` 包下的全部公开类型几乎都口吃：

| 当前 | 外部读起来 | 建议 |
|---|---|---|
| `plan.Plan` (`plan/plan.go:12`) | `plan.Plan` 🔴 | `plan.Document` 或 `plan.Result`，或**包名改 `planning`** |
| `plan.PlanOptions` (`plan/planner.go:13`) | `plan.PlanOptions` 🔴 | `plan.Options` |
| `plan.Planner` (`plan/planner.go:28`) | `plan.Planner` 🟡 | 保留（Planner 是英文单词，业界惯例可接受）；或包改 `planning` |
| `plan.PlanningSystem` (`plan/planning_system.go:12`) | `plan.PlanningSystem` 🔴 | `plan.System` 或 `plan.Engine` |
| `plan.ConditionWorldState` | `plan.ConditionWorldState` ✅ | 不口吃，保留 |

**最干净的方案**：把包名 `plan` 改为 `planning`，然后类型名都正常化：

```go
package planning

type Plan struct { ... }       // planning.Plan ✓
type Options struct { ... }    // planning.Options ✓
type Planner interface { ... } // planning.Planner ✓
type System struct { ... }     // planning.System ✓
```

`planning.Plan` 比 `plan.Plan` 自然得多。

**成本**：所有 import path 改 `agent/plan` → `agent/planning`（约 20+ 处
跨包引用）。一次性脚本 + grep 替换。

### 子包口吃（轻微）

| 当前 | 建议 |
|---|---|
| `goap.AStarPlanner` (`plan/planner/goap/astar.go:39`) | `goap.Planner`（`A*` 是实现细节，包名已是 GOAP） |
| `htn.Planner` (`plan/planner/htn/htn.go:111`) | 保留 ✓（htn 不是英文，`Planner` 不口吃） |
| `reactive.Planner` / `utility.Planner` | 保留 ✓ |

### 边界情况

| 当前 | 评级 |
|---|---|
| `event.Event` (`event/event.go:12`) | 🟡 边界 — `time.Time` / `context.Context` 等先例已为这种"包名 = 类型名核心概念"留口子。可保留。或包改 `events` 复数：`events.Event` 自然些 |
| `event.BaseEvent` | 🟡 同上。如果包改 `events`，自然变 `events.BaseEvent` |

**重要**：`event.NamedListener` / `event.Listener` / `event.Multicast` /
`event.ListenerFunc` 不口吃 ✅。问题集中在 `Event` 本身。

### 内部使用也算？

用户的规则是"包括内部使用和外部引入包"。内部（同包内）写裸 `Plan` /
`Options` 没问题，**Go 语法不会强制加包前缀**。但读 `plan/plan.go`
看到 `type Plan struct` + 同文件内 `var p Plan`，仍然有"plan.Plan"
那种隐性回响。改包名一次性解决双向。

---

## 19. 数据载体（Data Carrier）的 getter/setter 审计

用户规则：**纯数据载体不要 Java 风的 getter/setter，按可见性范围直接
public 或 private 字段**。

### 全仓库扫描结果

| 类型 | 字段 vs 方法 | 评级 |
|---|---|---|
| `core.Session` | 全 public 字段 (`ID`, `UserID`, `AgentName`, `StartedAt`, `UpdatedAt`, `Metadata`) | ✅ 完美 |
| `core.Goal` | 全 public 字段 | ✅ |
| `core.LLMInvocation` / `EmbeddingInvocation` / `TokenTotals` | 全 public 字段 | ✅ |
| `core.ProcessSnapshot` / `SnapshotActionInvocation` | 全 public 字段 | ✅ |
| `core.ActionMetadata` / `ActionQoS` | 全 public 字段 | ✅ |
| `core.GoalExport` / `AssetCoordinates` / `TerminationSignal` | 全 public 字段 | ✅ |
| `core.IOBinding` | 全 public 字段 + 谓词方法 `IsDefault()` | ✅ |
| `runtime.ActionInvocation` / `ScopeRun` | 全 public 字段 | ✅ |
| 所有 `workflow.*Spec` | 全 public 字段 | ✅ |
| 所有 `event.ActionExecutionStart` 类事件记录 | `BaseEvent` 嵌入 + public 字段 | ✅ |
| `core.Agent` | 全 public 字段 + 一个 `KnownConditions()` 方法（**派生数据**，不是 getter） | ✅ |

**结论**：纯数据载体已经全部按规则写好了。除了一个例外 ↓

### 唯一违反规则的 ❌

**`hitl.TypedRequest`** (`hitl/awaitable.go:25`)

```go
type TypedRequest[P any, R any] struct {
    IDStr   string                              // ← 为避开 ID() 方法重名而起的丑名
    Payload P
    Handler func(R) core.ResponseImpact
}
func (r *TypedRequest[P, R]) ID() string { return r.IDStr }
```

这是 Java "private field + getter" 模式在 Go 里的扭曲变种。原因是
`Request` 接口要求 `ID() string` 方法，导致字段不能也叫 `ID`。

**建议**（任选其一）：

**方案 A**（最 Go：删 ID 方法，直接 public 字段）：
```go
// 改 Request 接口，去掉 ID() 方法（如果调用方都通过具体类型用，可行）
type Request[P, R any] interface {
    Prompt() P
    OnResponse(r R) core.ResponseImpact
}
type TypedRequest[P, R any] struct {
    ID      string  // 直接 public
    Payload P
    Handler func(R) core.ResponseImpact
}
```

**方案 B**（保留接口，字段降级 unexported）：
```go
type TypedRequest[P, R any] struct {
    id      string  // unexported
    Payload P
    Handler func(R) core.ResponseImpact
}
func (r *TypedRequest[P, R]) ID() string { return r.id }
func NewTypedRequest[P, R any](...) *TypedRequest[P, R] {
    return &TypedRequest[P, R]{id: uuid.NewString(), ...}
}
```

推荐方案 B（保留接口契约），但字段名是 `id` 不是 `IDStr`。

### 接口约束下的"伪 getter"（不是问题）

下列类型有 `Foo() T { return f.foo }` 的 getter 形态，但**全部是因为
接口要求方法签名**，Go 没法用字段实现接口，**不是 Java 病**：

- `runtime.AgentProcess` 的 `ID()` / `ParentID()` / `StartedAt()` /
  `Blackboard()` / `Failure()` — 实现 `core.Process` 接口
- `core.ComputedCondition` 的 `Name()` / `Cost()` — 实现 `Condition`
- `core.StaticToolGroupResolver.Name()` — 实现 `ToolGroupResolver`
- `runtime.MCPToolGroupResolver.Name()` — 同上
- `event.NamedListener.Name()` — 实现 `Listener`
- `core.CounterIDGenerator.Name()` — 实现 `IDGenerator`
- `core.BudgetPolicy.Name()` — 实现 `EarlyTerminationPolicy`
- `plan.ConditionWorldState.HashKey()` / `State()` / `Timestamp()` —
  实现 `WorldState`
- `runtime.Platform.SessionStore()` / `ProcessStore()` /
  `NewBlackboard()` — Platform 主门面，方法形式给外部访问

这些**保留**。Go 没有 Java 那种"属性"语法，要满足接口必须用方法。

### `BaseEvent` 的"三重身份"

`event.BaseEvent` 有点意思：

```go
type BaseEvent struct {
    At  time.Time `json:"timestamp"`
    PID string    `json:"process_id"`
}
func (b BaseEvent) Timestamp() time.Time { return b.At }
func (b BaseEvent) ProcessID() string    { return b.PID }
```

**三种命名同一概念**：字段 `At` / `PID`，方法 `Timestamp()` /
`ProcessID()`，JSON tag `timestamp` / `process_id`。属于 Java
"DTO+getter+wire-format" 残留。

**建议**：字段直接用全名，省一层间接：
```go
type BaseEvent struct {
    Timestamp time.Time `json:"timestamp"`
    ProcessID string    `json:"process_id"`
}
// 删 Timestamp() / ProcessID() 方法
// 改 Event 接口为有这两个字段的 embed 约定（Go interface 不能要字段，
// 所以这一步要把 Event 接口的对应方法也删掉，改成"凡 Event 都嵌入
// BaseEvent"约定）
```

或者更激进 — 直接断 Event interface 的 Timestamp/ProcessID 约束，
靠 BaseEvent 嵌入提供这两个 public 字段（外部直接 `e.Timestamp` /
`e.ProcessID`）。这才是符合用户"数据载体直接 public"的 Go 写法。

**调用面**：`Timestamp()` 方法只在 `event/event.go:52,79` 内部用，
`ProcessID()` 在 `runtime/extension_test.go` + 2 个 example 里。可改。

---

## 不动 / 已经 OK 的 (供 review 时跳过)

- `Determination` enum (Unknown/True/False) — 标杆 ✅
- `*Func` 类型别名 (`ConditionFunc` / `ToolFunc`) — 标准 Go ✅
- `Builder` / `Provider` 这两个词在 Go 也常用，**不是** Java 味 ✅
- `StuckHandler` — `Handler` 后缀 + 动词，OK ✅
- `IDGenerator` 接口名 — 缩写大写正确 ✅
- 所有 `ID` / `URL` / `API` 字段命名 — 47 处全是大写，零小写 ✅
- 类型名嵌入 (`ProcessContext.Process Process`) — 习惯写法 ✅
- `EarlyTerminationPolicy` — `Policy` 后缀在 Go 也常见，可保 ✅

---

## 优先级建议

按 ROI 排序（影响调用面 × 改善度）：

**P0 — 低成本高收益**
1. ⏸️ **DEFERRED** — `IDStr` → `id` 字段（unexported）+ 保留 `ID()`
   方法。暂不改；等之后看 `hitl.Request` 接口要不要调整时一并处理
   (hitl 内部，调用面 5 处)
2. ✅ **DONE** — `HasRunKey() string` → `EffectiveRunKey() string`
3. ✅ **DONE** — `TypeFullNameOf` → `TypeName`
4. ✅ **DONE** — `ToolCallCanceller` → `ToolCallCancelFunc`
5. ✅ **DONE** — `goap.AStarPlanner` → `goap.Planner`
   (`NewAStarPlanner` → `NewPlanner`)

**P1 — 中等代价**
6. ✅ **DONE** — `InfoString(verbose bool)` → `Inspect(verbose bool)`
   (Blackboard 接口 + 实现 + helper `InspectBlackboard`)
7. ✅ **DONE** — `Blackboard.GetValue` / `GetCondition` →
   `Lookup` / `Condition`
8. ✅ **DONE** — `Platform.GetProcess` → `ProcessByID`
9. ✅ **DONE** — `MCPToolGroupResolver` → `MCPResolver`
   (`NewMCPToolGroupResolver` → `NewMCPResolver`)
10. ✅ **DONE** — `OperationContext` → `ConditionEnv`
11. ⏸️ **SKIPPED** — `BaseEvent.At/PID + Timestamp()/ProcessID()`
    三重身份。Go interface 无法要字段，要消除三重身份必须打破 Event
    接口契约 (要么 method-only / 要么 field-only)，权衡后保留现状。

**P2 — 大改动，可分批**
12. ✅ **DONE** — **`plan` 包改名 `planning`**。一次性消除该包下全部
    stutter：
    - 目录 `agent/plan/` → `agent/planning/` (`git mv`)
    - `plan.PlanOptions` → `planning.Options`
    - `plan.PlanningSystem` → `planning.System`
    - `plan.NewPlanningSystem` → `planning.NewSystem`
    - 全仓库 import path 一次性 rewrite
13. ✅ **DONE** — workflow `*Spec` → `*Config` (6 个类型：Loop /
    Parallel / Consensus / RepeatUntil / RepeatUntilAcceptable /
    ScatterGather)
14. ✅ **DONE** — `ResponseImpactUnchanged` / `ResponseImpactUpdated`
    → `ImpactUnchanged` / `ImpactUpdated`
15. ✅ **DONE** — `EffectSpec` → `Effects`
16. ✅ **DONE** — `ActionInterceptor` → `ActionMiddleware`
    (`runActionInterceptors` → `runActionMiddleware`)

**P3 — 风险高 / 哲学讨论**
17. `ProcessContext` 改名 — 影响整个仓库，**不建议改**，doc 解释即可
18. `CounterIDGenerator` → `Counter` — 改名后需要看包前缀消歧效果
19. `event` 包改名 `events` — 为了 `events.Event` 不那么口吃；改动面
    比 `plan→planning` 还大（每个 event 类型都被引用），收益相对小
    （`time.Time` 等先例已证明可接受），可不做

---

## 落地建议

每条都是一个独立的 commit-able 改动。建议顺序：

```
P0 (4 个改动) — 一个 commit "refactor(agent): tighten naming (P0)"
P1 — 五个 commit，每条一个，方便 review
P2 — 三个 commit，跨包改动各一个
P3 — 先讨论再做
```

每个 commit 内：rename + grep -rn 修调用面 + 跑 `go test ./...`。

不动 `ProcessContext` 是个有意识的选择 — 该词已经是仓库的"通用语"，
改名收益不抵风险。把它的 doc 写清楚是 cheaper 的解法。
