# 04. 用户 API：三种定义 Agent 的方式

> embabel 主推 `@Agent` + `@Action` 注解。Go 没有注解，本节给出三种替代方案，**DSL 是一等公民**。

---

## 方式 A：Fluent DSL（推荐、一等公民）

### 设计目标
- 完全显式，无反射、无魔法
- 类型安全（Go 泛型）
- IDE 自动补全完整

### 完整示例 —— 客户意图分类 Agent

```go
// examples/intent_agent.go
package main

import (
    "context"
    "github.com/Tangerg/lynx/agent"
    "github.com/Tangerg/lynx/agent/core"
)

// 领域类型（sealed 风格：接口 + marker 方法）
type Intent interface{ isIntent() }

type BillingIntent struct{ Reason string }
func (BillingIntent) isIntent() {}

type SalesIntent struct{ Product string }
func (SalesIntent) isIntent() {}

type ServiceIntent struct{ Issue string }
func (ServiceIntent) isIntent() {}

type UserInput struct{ Content string }
type Resolution struct{ Department string; Notes string }

// 构建 Agent
func BuildIntentAgent(llm *chat.Client) *core.Agent {
    return agent.New("IntentReceptionAgent").
        Description("Figure out the department a customer wants to transfer to").
        Version("1.0.0").

        Action(core.NewAction("classifyIntent",
            func(ctx context.Context, pc *core.ProcessContext, input UserInput) (Intent, error) {
                // 用 LLM 分类
                resp, err := pc.LLM().
                    ChatWithText("Classify: " + input.Content).
                    Call().
                    Response(ctx)
                if err != nil { return nil, err }

                // 解析 LLM 返回
                switch resp.Result().AssistantMessage.Text {
                case "billing":  return BillingIntent{}, nil
                case "sales":    return SalesIntent{}, nil
                case "service":  return ServiceIntent{}, nil
                }
                return nil, errors.New("unknown intent")
            })).

        Action(core.NewAction("handleBilling",
            func(ctx context.Context, pc *core.ProcessContext, intent BillingIntent) (Resolution, error) {
                return Resolution{Department: "billing", Notes: intent.Reason}, nil
            }).WithDescription("Route to billing department")).

        Action(core.NewAction("handleSales",
            func(ctx context.Context, pc *core.ProcessContext, intent SalesIntent) (Resolution, error) {
                return Resolution{Department: "sales", Notes: intent.Product}, nil
            })).

        Action(core.NewAction("handleService",
            func(ctx context.Context, pc *core.ProcessContext, intent ServiceIntent) (Resolution, error) {
                return Resolution{Department: "service", Notes: intent.Issue}, nil
            })).

        Goal(core.GoalProducing[Resolution]("Department has been determined").
            WithValue(1.0)).

        Build()
}

// 运行
func main() {
    chatClient := newChatClient()  // 来自 lynx core/model/chat
    platform := agent.NewPlatform(
        agent.WithChatClient(chatClient),
        agent.WithObservation(slogReg),
    )

    myAgent := BuildIntentAgent(chatClient)
    platform.Deploy(myAgent)

    proc, err := platform.RunAgent(
        context.Background(),
        myAgent,
        map[string]any{"it": UserInput{Content: "I need help with my bill"}},
    )
    if err != nil { log.Fatal(err) }

    result, _ := core.ResultOfType[Resolution](proc)
    fmt.Println(result.Department)  // "billing"
}
```

### 核心 Builder API

```go
// agent/dsl/builder.go
package dsl

// Builder 流式构造 *core.Agent
type Builder struct {
    name         string
    description  string
    provider     string
    version      core.Semver
    actions      []core.Action
    goals        []*core.Goal
    conditions   []core.Condition
    stuckHandler core.StuckHandler
    opaque       bool
}

func New(name string) *Builder {
    return &Builder{
        name:    name,
        version: core.Semver{Major: 1},
    }
}

func (b *Builder) Description(d string) *Builder { b.description = d; return b }
func (b *Builder) Provider(p string) *Builder    { b.provider = p; return b }
func (b *Builder) Version(v string) *Builder     { b.version = core.ParseSemver(v); return b }

func (b *Builder) Action(a core.Action) *Builder { b.actions = append(b.actions, a); return b }

func (b *Builder) Goal(g *core.Goal) *Builder    { b.goals = append(b.goals, g); return b }

func (b *Builder) Condition(c core.Condition) *Builder { b.conditions = append(b.conditions, c); return b }

func (b *Builder) Opaque(o bool) *Builder        { b.opaque = o; return b }
func (b *Builder) StuckHandler(h core.StuckHandler) *Builder { b.stuckHandler = h; return b }

func (b *Builder) Build() *core.Agent {
    return &core.Agent{
        Name: b.name, Description: b.description, Provider: b.provider, Version: b.version,
        Actions: b.actions, Goals: b.goals, Conditions: b.conditions,
        StuckHandler: b.stuckHandler, Opaque: b.opaque,
    }
}
```

### Action 链式选项（ActionOption）

`core.NewAction` 的 `opts ...ActionOption` 支持细节配置：

```go
// agent/core/action_options.go
package core

type ActionOption func(*actionConfig)

type actionConfig struct {
    name        string
    description string
    pre         []string
    post        []string
    canRerun    bool
    readOnly    bool
    qos         ActionQos
    cost        float64
    value       float64
    toolGroups  []ToolGroupRequirement
    trigger     reflect.Type  // @Trigger 等价
    outputBinding string
}

func WithDescription(s string) ActionOption     { return func(c *actionConfig) { c.description = s } }
func WithPre(conditions ...string) ActionOption { return func(c *actionConfig) { c.pre = conditions } }
func WithPost(conditions ...string) ActionOption { return func(c *actionConfig) { c.post = conditions } }
func WithCanRerun(b bool) ActionOption          { return func(c *actionConfig) { c.canRerun = b } }
func WithQoS(q ActionQos) ActionOption          { return func(c *actionConfig) { c.qos = q } }
func WithCost(v float64) ActionOption           { return func(c *actionConfig) { c.cost = v } }
func WithValue(v float64) ActionOption          { return func(c *actionConfig) { c.value = v } }
func WithTrigger[T any]() ActionOption          { return func(c *actionConfig) { c.trigger = reflect.TypeOf((*T)(nil)).Elem() } }
func WithOutputBinding(name string) ActionOption { return func(c *actionConfig) { c.outputBinding = name } }
func WithToolGroups(groups ...string) ActionOption { ... }
```

用法：
```go
core.NewAction("fetchData",
    func(ctx context.Context, pc *core.ProcessContext, q Query) (Data, error) { ... },
    core.WithDescription("Fetch data from upstream API"),
    core.WithPre("user_authenticated"),
    core.WithPost("data_fetched"),
    core.WithCanRerun(false),
    core.WithQoS(core.ActionQos{MaxAttempts: 3}),
    core.WithToolGroups("web"),
)
```

### Goal 链式

```go
core.GoalProducing[Resolution]("...").
    WithName("resolve_customer").
    WithPre("intent_classified").
    WithValue(1.0).
    WithTags("customer-service").
    WithExport(core.ExportConfig{Remote: true})
```

---

## 方式 B：Struct + 反射（便利层）

如果用户更喜欢面向对象风格（比如从 Java/Spring 迁移过来），支持**用 struct 的方法定义 Action**，框架用反射自动生成注册。

### 用户代码

```go
// examples/intent_agent_struct.go
package main

import "github.com/Tangerg/lynx/agent/reflect"

// IntentAgent 用 struct 承载能力
// struct tag `agent:"..."` 表示元信息（等价于 @Agent 注解）
type IntentAgent struct {
    _ struct{} `agent:"IntentReceptionAgent,description=Figure out department"`

    // 框架注入的依赖（Go 没有 Spring，这里用显式字段）
    LLM *chat.Client
}

// Action 方法：遵循约定 `Action<name>` 或用 struct tag
// 方法签名必须是 func(ctx, Input) (Output, error) 或变体
func (a *IntentAgent) ClassifyIntent(ctx context.Context, input UserInput) (Intent, error) {
    // ... 用 a.LLM 调用
}

func (a *IntentAgent) HandleBilling(ctx context.Context, intent BillingIntent) (Resolution, error) {
    return Resolution{Department: "billing"}, nil
}

// 用 marker method 标识 AchievesGoal
// 约定：方法名以 "Achieves" 结尾，或 struct 包含 `agent:"goal=..."` 标签的特殊字段
func (a *IntentAgent) AchievesResolution() *core.Goal {
    return core.GoalProducing[Resolution]("Department has been determined").WithValue(1.0)
}

// Condition 方法：返回 core.Determination
func (a *IntentAgent) IntentClassified(ctx context.Context, oc *core.OperationContext) core.Determination {
    if _, ok := core.Get[Intent](oc.Bb, "it"); ok { return core.True }
    return core.False
}
```

### 框架反射注册

```go
// agent/reflect/register.go
package reflect

// Register 扫描 struct 的方法，生成 *core.Agent
func Register(instance any) (*core.Agent, error) {
    v := reflect.ValueOf(instance)
    t := v.Type()

    // 解析 agent 标签
    agentMeta := parseAgentTag(t)
    b := dsl.New(agentMeta.Name).Description(agentMeta.Description)

    // 遍历所有方法
    for i := 0; i < t.NumMethod(); i++ {
        method := t.Method(i)

        switch classifyMethod(method) {
        case methodKindAction:
            action, err := wrapAsAction(method, v)
            if err != nil { return nil, err }
            b.Action(action)

        case methodKindCondition:
            cond := wrapAsCondition(method, v)
            b.Condition(cond)

        case methodKindAchievesGoal:
            goal := invokeGoalFactory(method, v)
            b.Goal(goal)
        }
    }

    return b.Build(), nil
}

// wrapAsAction 把反射 Method 包装成 core.Action
// 注意：这里只在注册时用一次反射，生成闭包后调用路径无反射开销
func wrapAsAction(m reflect.Method, receiver reflect.Value) (core.Action, error) {
    // 检查方法签名 func(ctx, In) (Out, error) 或 func(pc, In) (Out, error)
    mt := m.Type
    if mt.NumIn() != 3 || mt.NumOut() != 2 { /* error */ }

    inType  := mt.In(2)   // In 类型
    outType := mt.Out(0)  // Out 类型

    // 用一次反射生成通用闭包
    fn := func(ctx context.Context, pc *core.ProcessContext, in any) (any, error) {
        args := []reflect.Value{receiver, reflect.ValueOf(ctx), reflect.ValueOf(in)}
        results := m.Func.Call(args)
        if !results[1].IsNil() {
            return nil, results[1].Interface().(error)
        }
        return results[0].Interface(), nil
    }

    // 包装成 Action 接口（核心运行时不再走反射）
    return &reflectedAction{
        name:     m.Name,
        inputs:   []core.IoBinding{{Name: core.DefaultBinding, Type: typeName(inType)}},
        outputs:  []core.IoBinding{{Name: core.DefaultBinding, Type: typeName(outType)}},
        fn:       fn,
    }, nil
}
```

### 使用

```go
func main() {
    ia := &IntentAgent{LLM: chatClient}
    myAgent, err := reflect.Register(ia)
    if err != nil { log.Fatal(err) }

    platform.Deploy(myAgent)
    platform.RunAgent(ctx, myAgent, bindings)
}
```

### 约定总结

| 结构 | 约定 |
|-----|------|
| Agent 类 | struct，顶部 `_ struct{} \`agent:"Name,description=..."\`` 或通过 `agent:"name=..."` 标签 |
| Action 方法 | `func(receiver, ctx context.Context, in In) (Out, error)`，方法名驼峰转 snake 作为 action 名 |
| Condition 方法 | `func(receiver, ctx context.Context, oc *core.OperationContext) core.Determination` |
| AchievesGoal | 返回 `*core.Goal` 的无参方法，命名约定 `Achieves<Name>` |
| 元信息覆盖 | 方法上没法打 struct tag，要么约定命名，要么用 **marker 注释 + codegen**（方式 C） |

### 性能说明
- 反射**只在 `Register` 时**发生一次
- 运行时调用走**闭包**，性能接近直接调用（一次 `reflect.Value.Call` 比直接调用慢 ~10×，但在 LLM 调用的百毫秒级背景下可忽略）

---

## 方式 C：代码生成（可选，极致追求零反射）

对**反射都嫌贵**或想要 IDE 级元信息检查的用户，提供 `//go:generate` 工具。

### 用户代码

```go
// examples/intent_agent_codegen.go
package main

//go:generate lynx-agent-gen -out=intent_agent_gen.go

// +lynx:agent name="IntentReceptionAgent" description="Figure out department"
type IntentAgent struct {
    LLM *chat.Client
}

// +lynx:action description="Classify customer intent"
func (a *IntentAgent) ClassifyIntent(ctx context.Context, input UserInput) (Intent, error) { ... }

// +lynx:action pre="intent_classified"
func (a *IntentAgent) HandleBilling(ctx context.Context, intent BillingIntent) (Resolution, error) { ... }

// +lynx:achieves value=1.0
func (a *IntentAgent) AchievesResolution() *core.Goal { ... }
```

### 生成的代码（`intent_agent_gen.go`）

```go
// Code generated by lynx-agent-gen; DO NOT EDIT.
package main

func (a *IntentAgent) BuildAgent() *core.Agent {
    return dsl.New("IntentReceptionAgent").
        Description("Figure out department").
        Action(core.NewAction("classify_intent",
            func(ctx context.Context, pc *core.ProcessContext, in UserInput) (Intent, error) {
                return a.ClassifyIntent(ctx, in)
            },
            core.WithDescription("Classify customer intent"),
        )).
        Action(core.NewAction("handle_billing",
            func(ctx context.Context, pc *core.ProcessContext, in BillingIntent) (Resolution, error) {
                return a.HandleBilling(ctx, in)
            },
            core.WithPre("intent_classified"),
        )).
        Goal(a.AchievesResolution().WithValue(1.0)).
        Build()
}
```

### 工具链
- `cmd/lynx-agent-gen/main.go` — 用 `go/parser` + `go/ast` 扫描源码，识别 `+lynx:xxx` 注释，生成 boilerplate
- 本质是把方式 A（DSL）的样板代码自动化
- 好处：**零反射**、**类型 100% 保留**、**编译期错误可见**

### 适用人群
- 生产环境对性能敏感的场景
- 大型 agent 库希望 agent 定义稳定、代码可审查

---

## 三种方式对比

| 维度 | DSL | Struct + 反射 | Codegen |
|-----|-----|-------------|---------|
| 学习成本 | 低（就是一串链式调用） | 低（Java/Spring 风格熟悉） | 中（要懂 go generate） |
| 类型安全 | ✅ 编译期完整 | ⚠️ 注册时运行时检查 | ✅ 编译期完整 |
| 性能 | ✅ 零反射 | ⚠️ 单次反射 + 闭包 | ✅ 零反射 |
| IDE 补全 | ✅ 完整 | ✅ 完整 | ✅ 完整 |
| 元信息显式度 | ✅ 函数选项显式 | ⚠️ 命名约定 | ✅ 注释标记 |
| 调试难度 | ✅ 可单步进入 | ⚠️ 反射栈 | ⚠️ 看生成码 |
| 适合场景 | 主推、库内置示例 | 从 Spring 迁移、快速原型 | 生产、性能敏感 |

**官方姿态**：
1. **文档 / 示例 / 测试都以 DSL 为主**（一等公民）
2. **反射方式作为便利层**，有完整文档和示例，但不强推
3. **Codegen 是可选工具**，`agent/` 不内置该 cmd，作为独立小工具发布

---

## 关键差异对照

### 等价的「@Action」

```kotlin
// embabel
@Action
fun classify(input: UserInput): Intent { ... }
```

对应 Go：

```go
// DSL
.Action(core.NewAction("classify",
    func(ctx context.Context, pc *core.ProcessContext, input UserInput) (Intent, error) { ... },
))

// 反射
func (a *MyAgent) Classify(ctx context.Context, input UserInput) (Intent, error) { ... }

// Codegen
// +lynx:action
func (a *MyAgent) Classify(ctx context.Context, input UserInput) (Intent, error) { ... }
```

### 等价的「@AchievesGoal」

```kotlin
@AchievesGoal(description = "Done", value = 1.0)
@Action
fun done(intent: Intent): Result { ... }
```

对应 Go：DSL 里加 Action 然后单独给 Goal，Struct 方式用 marker method：

```go
// DSL
.Action(core.NewAction("done",
    func(ctx context.Context, pc *core.ProcessContext, intent Intent) (Result, error) { ... },
)).
Goal(core.GoalProducing[Result]("Done").WithValue(1.0))
```

### 等价的「@RequireNameMatch」

当 Agent 有多个同类型输入，embabel 用 `@RequireNameMatch` 强制按名字绑定。Go 里：

```go
// DSL
core.NewAction("ship",
    func(ctx context.Context, pc *core.ProcessContext, ...) (...) { ... },
).WithInputs(
    core.IoBinding{Name: "from", Type: "Address"},
    core.IoBinding{Name: "to",   Type: "Address"},
)
```

Action 构造器支持**显式覆盖默认 `it` 绑定**，和 `@RequireNameMatch` 语义一致。

---

下一份文档描述如何把这套 API 和 Lynx 现有的 chat、observation、rag 对接 → `05-integration-and-examples.md`
