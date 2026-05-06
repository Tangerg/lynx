# 04. 用户 API：用 Fluent DSL 定义 Agent

> embabel 主推 `@Agent` + `@Action` 注解。Go 没有注解，本框架只提供一种入口：**显式的 Fluent DSL**。理由：Go 社区共识是「显式优于隐式」，反射注册让错误推迟到运行时、IDE 重构失效、命名约定成为隐藏契约——付出的代价远超带来的便利。

---

## Fluent DSL

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

        Actions(core.NewAction("classifyIntent",
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

        Actions(core.NewAction("handleBilling",
            func(ctx context.Context, pc *core.ProcessContext, intent BillingIntent) (Resolution, error) {
                return Resolution{Department: "billing", Notes: intent.Reason}, nil
            }).WithDescription("Route to billing department")).

        Actions(core.NewAction("handleSales",
            func(ctx context.Context, pc *core.ProcessContext, intent SalesIntent) (Resolution, error) {
                return Resolution{Department: "sales", Notes: intent.Product}, nil
            })).

        Actions(core.NewAction("handleService",
            func(ctx context.Context, pc *core.ProcessContext, intent ServiceIntent) (Resolution, error) {
                return Resolution{Department: "service", Notes: intent.Issue}, nil
            })).

        Goals(core.GoalProducing[Resolution]("Department has been determined").
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
    version      *semver.Version
    actions      []core.Action
    goals        []*core.Goal
    conditions   []core.Condition
    stuckHandler core.StuckHandler
    opaque       bool
}

func New(name string) *Builder {
    return &Builder{
        name:    name,
        version: semver.MustParse("1.0.0"),
    }
}

func (b *Builder) Description(d string) *Builder { b.description = d; return b }
func (b *Builder) Provider(p string) *Builder    { b.provider = p; return b }
func (b *Builder) Version(v string) *Builder     { b.version = semver.MustParse(v); return b }

func (b *Builder) Actions(actions ...core.Action) *Builder { b.actions = append(b.actions, actions...); return b }

func (b *Builder) Goals(goals ...*core.Goal) *Builder       { b.goals = append(b.goals, goals...); return b }

func (b *Builder) Conditions(conds ...core.Condition) *Builder { b.conditions = append(b.conditions, conds...); return b }

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

## 为什么不提供 struct + 反射 / 代码生成入口

**反射注册**（embabel 的 `@Agent` / `@Action` 风格在 Go 里的常见模仿）的代价：

- **错误推迟到运行时**：方法签名错了——编译能过、单元测试能过，部署时才挂在 `Register()`
- **IDE / 重构工具失效**：rename `ClassifyIntent` 不会同步任何东西；"go to references" 找不到调用方；命名约定（`Achieves` 前缀、`Action` 驼峰转 snake）成为隐藏契约
- **性能间接**：每次调用都过一次 `reflect.Value.Call`（比直接调慢 ~10×），虽然在 LLM 主导的百毫秒背景下不显眼，但是一种「隐性税」
- **维护负担恒定**：每次 `core.Action` 接口微调，反射桥接代码就要跟着改

**代码生成**（`//go:generate` 风格）能解决性能与类型安全问题，但带来：
- 需要单独的工具链（`cmd/lynx-agent-gen`），用户必须懂 `go generate` 工作流
- 生成代码 vs 手写代码的 diff review 体验割裂
- IDE 跳到生成代码反而比 DSL 更绕

**结论**：DSL 一种入口足以覆盖所有用户场景。从 Spring/embabel 迁移过来的用户付出的代价是要写一行显式 `agent.New("X").Actions(...)` 而不是给方法加注解——这点学习成本远低于在反射栈里调试或维护 codegen 模板。

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
.Actions(core.NewAction("classify",
    func(ctx context.Context, pc *core.ProcessContext, input UserInput) (Intent, error) { ... },
))
```

### 等价的「@AchievesGoal」

```kotlin
@AchievesGoal(description = "Done", value = 1.0)
@Action
fun done(intent: Intent): Result { ... }
```

对应 Go：先注册 Action，再单独 Goal——两条意图分开声明：

```go
.Actions(core.NewAction("done",
    func(ctx context.Context, pc *core.ProcessContext, intent Intent) (Result, error) { ... },
)).
Goals(core.GoalProducing[Result]("Done").WithValue(1.0))
```

### 等价的「@RequireNameMatch」

当 Agent 有多个同类型输入，embabel 用 `@RequireNameMatch` 强制按名字绑定。Go 里：

```go
// DSL
core.NewAction("ship",
    func(ctx context.Context, pc *core.ProcessContext, ...) (...) { ... },
).WithInputs(
    core.IOBinding{Name: "from", Type: "Address"},
    core.IOBinding{Name: "to",   Type: "Address"},
)
```

Action 构造器支持**显式覆盖默认 `it` 绑定**，和 `@RequireNameMatch` 语义一致。

---

下一份文档描述如何把这套 API 和 Lynx 现有的 chat、observation、rag 对接 → `05-integration-and-examples.md`
