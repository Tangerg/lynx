# 01. 整体架构 & Java→Go 差异分析

## 1. embabel-agent 架构速览

```
┌──────────────────────────────────────────────────────────────────┐
│  用户层                                                            │
│   @Agent 类 / Kotlin DSL / Shell REPL / HTTP API                 │
└──────────────────────────────────┬───────────────────────────────┘
                                   ↓
┌──────────────────────────────────────────────────────────────────┐
│  AgentPlatform 运行时                                             │
│   ┌──────────────┐  ┌──────────────┐  ┌────────────────────┐    │
│   │ AgentProcess │  │   Planner    │  │  EventPublisher    │    │
│   │ (OODA Loop)  │←→│ (A* GOAP /   │  │  (生命周期事件)    │    │
│   │              │  │  Utility)    │  │                    │    │
│   └──────┬───────┘  └──────────────┘  └────────────────────┘    │
│          ↓                                                         │
│   ┌────────────────────────────────────────┐                      │
│   │           Blackboard                   │                      │
│   │   (类型安全的共享状态存储)              │                      │
│   └────────────────────────────────────────┘                      │
└──────────────────────────────────┬───────────────────────────────┘
                                   ↓
┌──────────────────────────────────────────────────────────────────┐
│  SPI 扩展层                                                       │
│   PlannerFactory / ToolGroupResolver / BlackboardProvider /      │
│   AgentProcessRepository / OperationScheduler                    │
└──────────────────────────────────┬───────────────────────────────┘
                                   ↓
┌──────────────────────────────────────────────────────────────────┐
│  基础设施                                                          │
│   Spring AI (LLM) / RAG / MCP / A2A / OpenAI / Anthropic / ...  │
└──────────────────────────────────────────────────────────────────┘
```

**核心机制**：
1. 用户用注解或 DSL 定义 Agent（能力集合）
2. 调用 `platform.runAgentFrom(...)` 触发执行
3. 创建 `AgentProcess`（带黑板、规划器、ProcessContext）
4. 进入 **tick 循环**：每个 tick 做一次 OODA
   - **Observe**：从黑板推导世界状态（`WorldStateDeterminer`）
   - **Orient**：规划器用 A* 搜索从当前状态到目标的 Action 序列
   - **Decide**：选取 Plan 的第一个 Action（或并发模式下选所有可达 Action）
   - **Act**：执行 Action → 修改黑板 → 准备下次 tick
5. 终态：`COMPLETED` / `FAILED` / `STUCK` / `PAUSED`

---

## 2. Lynx 目标架构

```
┌──────────────────────────────────────────────────────────────────┐
│  用户层                                                            │
│   DSL builder / struct + reflection / codegen                    │
└──────────────────────────────────┬───────────────────────────────┘
                                   ↓
┌──────────────────────────────────────────────────────────────────┐
│  agent/ 顶层 Module                                                │
│  ┌───────────────┐  ┌──────────────┐  ┌──────────────────────┐  │
│  │ AgentProcess  │  │   Planner    │  │  EventListener       │  │
│  │ (Go struct +  │←→│ (A* GOAP /   │  │  (multicast + hook)  │  │
│  │  goroutines)  │  │  Utility)    │  │                      │  │
│  └───────┬───────┘  └──────────────┘  └──────────────────────┘  │
│          ↓                                                         │
│  ┌────────────────────────────────────┐                          │
│  │        Blackboard (sync.Map +       │                          │
│  │         typed accessors via Go       │                          │
│  │            generics)                 │                          │
│  └────────────────────────────────────┘                          │
└──────────────────────────────────┬───────────────────────────────┘
                                   ↓
┌──────────────────────────────────────────────────────────────────┐
│  集成 Lynx core/                                                   │
│  ┌──────────┐  ┌──────────────┐  ┌──────────┐  ┌─────────────┐  │
│  │chat.Client│  │observation.  │  │rag.Pipe- │  │vectorstore  │  │
│  │(LLM 调用) │  │Registry      │  │line      │  │(检索)        │  │
│  └──────────┘  └──────────────┘  └──────────┘  └─────────────┘  │
└──────────────────────────────────┬───────────────────────────────┘
                                   ↓
┌──────────────────────────────────────────────────────────────────┐
│  可选外挂 agents/ Module（仿 observations/ 规格）                  │
│   a2a / mcp / shell                                              │
└──────────────────────────────────────────────────────────────────┘
```

---

## 3. Java/Kotlin → Go 语言差异对照

这是整个设计中最关键的一节。embabel 大量使用了 Java/Kotlin 特性，Go 需要换一套方式等价表达。

### 3.1 注解 vs 无注解

**Java/Kotlin**：
```kotlin
@Agent(description = "分类客户意图")
class IntentAgent {
    @Action
    fun classify(input: UserInput): Intent { ... }

    @AchievesGoal(description = "意图已确定")
    @Action
    fun done(intent: Intent): Result { ... }
}
```

**Go 无对应物**。三种替代方案：

| 方案 | 写法 | 优缺点 |
|-----|------|-------|
| **A. Fluent DSL（推荐）** | `agent.New("...").Action(...)...` | ✅ 显式、可调试、无魔法；⚠️ 略冗长 |
| **B. Struct + 反射** | 定义 struct，方法名/标签约定为 Action | ✅ 接近注解体验；⚠️ 反射慢、类型检查弱 |
| **C. 代码生成** | `//go:generate lynx-agent-gen` 扫描 marker 注释 | ✅ 零运行时开销；⚠️ 增加构建复杂度 |

**决策**：**三种都支持，但 DSL 是一等公民**（文档、示例、测试都以 DSL 为主）。详见 `04-user-api.md`。

### 3.2 反射方法调用

**Kotlin**：`method.invoke(instance, *args)`，JVM 提供完整泛型类型信息。

**Go**：`reflect.Value.Call(...)` 可以，但：
- 类型擦除严重，自定义泛型参数在反射中看不到
- 性能比直接调用慢 ~10-50×
- 错误信息不友好

**对策**：
- 核心路径**不走反射**——用**泛型函数 + 闭包**捕获类型：
  ```go
  // 注册时用泛型函数，编译期生成专用 adapter
  agent.Action("classify", func(ctx context.Context, input UserInput) (Intent, error) {
      ...
  })
  ```
- struct 反射方案**只在便利层**用一次反射扫描，生成闭包后缓存，运行时调用闭包。

### 3.3 Spring DI 依赖注入

**Java**：构造注入 `@Autowired` 来的 bean。

**Go**：无 DI 容器，用**显式构造**：
```go
platform := agent.NewPlatform(
    agent.WithChatClient(chatClient),
    agent.WithObservation(reg),
    agent.WithPlanner(goap.NewAStarPlanner()),
)
```

这是 Go 惯用法（`net/http.Server`、`database/sql` 都是这种），用户不需要学 DI 框架。

### 3.4 ThreadLocal `AgentProcess.get()`

**Kotlin**：Action 内部 `AgentProcess.get()` 读 thread-local。

**Go**：用 `context.Context`：
```go
// 框架层：执行 action 前把 process 塞进 ctx
ctx = agent.WithProcess(ctx, proc)

// Action 层：
func MyAction(ctx context.Context, input Foo) (Bar, error) {
    proc := agent.ProcessFrom(ctx)  // 显式读取
    // ...
}
```

### 3.5 泛型方法 vs 方法-less 泛型

**Kotlin**：
```kotlin
fun <O> AgentProcess.resultOfType(outputClass: Class<O>): O { ... }
```

**Go** 不允许在具体类型上定义泛型方法。用**顶层泛型函数**：
```go
func ResultOfType[T any](proc *AgentProcess) (T, error) {
    // 从黑板取出类型为 T 的对象
}

// 用户调用：
result, err := agent.ResultOfType[*Intent](proc)
```

### 3.6 Sealed class / 联合类型

**Kotlin**：
```kotlin
sealed class Intent
class BillingIntent : Intent()
class SalesIntent : Intent()
```

**Go**：接口 + marker 方法：
```go
type Intent interface { isIntent() }

type BillingIntent struct{}
func (BillingIntent) isIntent() {}

type SalesIntent struct{}
func (SalesIntent) isIntent() {}
```

Action 中 `switch v := intent.(type) { case BillingIntent: ... }`。

### 3.7 Kotlin 操作符重载

**Kotlin**：
```kotlin
val c = conditionA and conditionB
val c = !conditionA
```

**Go**：方法链或顶层函数：
```go
c := agent.And(conditionA, conditionB)
c := agent.Not(conditionA)

// 或链式：
c := conditionA.And(conditionB).Not()
```

### 3.8 Reactor Flux → iter.Seq2

**Java**：`Flux<ChatResponse>` 需要 Reactor schedulers、`publishOn`、`contextWrite`。

**Go**：`iter.Seq2[*Response, error]`（Go 1.23+）pull-based 迭代器，无需调度器，天然支持 context 取消。

### 3.9 data class copy/mutate → Clone + With

**Kotlin**：
```kotlin
val agent2 = agent.copy(name = "new")
```

**Go**：
```go
// 方式 A：Clone + 字段赋值
a2 := agent.Clone()
a2.Name = "new"

// 方式 B：With 方法（不可变风格）
a2 := agent.WithName("new")
```

推荐 B，因为 Lynx 其他地方已经广泛用 `With*` 风格。

### 3.10 Kotlin Coroutines → Goroutines

**Kotlin**：
```kotlin
platformServices.asyncer.async { executeAction(action) }
```

**Go**：
```go
g, ctx := errgroup.WithContext(ctx)
for _, action := range achievable {
    action := action
    g.Go(func() error { return executeAction(ctx, action) })
}
err := g.Wait()
```

更原生，且 context 取消语义一致。

### 3.11 Jakarta Validation

**Java**：`@NotNull`, `@Size` 等注解校验。

**Go**：
- 用 `github.com/go-playground/validator/v10` + struct tag
- 或走 JSON Schema 路径（我们已经用 `invopop/jsonschema`）

### 3.12 Spring ApplicationEvent → Channel + Listener

**Java**：`ApplicationEventPublisher.publishEvent(...)`，Spring 分发到所有监听器。

**Go**：自写多播监听器（无外部依赖）：
```go
type EventListener interface {
    OnEvent(event Event)
}

type MulticastListener struct {
    listeners []EventListener
}

func (m *MulticastListener) OnEvent(event Event) {
    for _, l := range m.listeners { l.OnEvent(event) }
}
```

---

## 4. 总体设计原则小结

| 原则 | 理由 |
|-----|------|
| **不继承**，用组合 | Go 没有 `open class`；embabel 的 `AbstractAgentProcess` 在 Go 里是 struct embedding |
| **不 ThreadLocal**，用 ctx | 线程模型完全不同，ctx 更显式 |
| **不反射**（核心路径） | 性能+类型安全双优势 |
| **不 DI 容器**，用构造器注入 | Go 文化不接受隐式 DI |
| **不 Reactor**，用 iter.Seq2 | Go 原生异步更简洁 |
| **不 annotation processor**，用 codegen（可选） | 显式 `go generate` 替代编译期注解处理 |

---

## 5. 模块拓扑决定

### 为什么 `agent/` 要做独立顶层 module

1. **依赖面清晰**：`agent/` 依赖 `core/`，但 `core/` 不依赖 `agent/`（核心不该被 agent 污染）
2. **独立版本演进**：agent 作为高阶功能，API 会更快迭代；独立 module 避免拖累 core
3. **可选引入**：用户只想用 Lynx 的 chat / RAG 不想要 agent？不装 `agent/` 即可，零成本
4. **架构一致性**：和 `models/`、`vectorstores/`、`observations/` 同规格，规则统一

### `agent/` 内部不再分子 module

与 `observations/` 等不同，`agent/` 内部（`core/`、`plan/`、`planner/`、`runtime/`、`dsl/` 等）是紧耦合的——它们一起构成框架的最小功能集，无可替代性，不必拆多 module。

### `agents/` 是未来扩展位

如果之后加 A2A / MCP / Shell 等集成，每个都可能引入重依赖（如 MCP SDK、ssh server 等），**同样走外部 module 路线**——新起 `agents/` 顶层 module，内部子目录按用途分。

---

## 6. 接下来怎么看

- 想看**核心类型长什么样** → `02-core-abstractions.md`
- 想看**GOAP 怎么在 Go 里实现** → `03-planner-and-runtime.md`
- 想看**用户代码怎么写** → `04-user-api.md`
- 想看**怎么跟 Lynx 其它部分配合** → `05-integration-and-examples.md`
- 想看**什么时候能做完** → `06-rollout.md`
