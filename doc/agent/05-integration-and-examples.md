# 05. 与 Lynx 集成 & 完整示例

---

## 1. 集成点总览

```
              ┌──────────────────────────┐
              │   agent/ (framework)      │
              └────────┬──────────────────┘
                       │ 依赖 / 组合
           ┌───────────┼─────────────────┬─────────────┐
           ↓           ↓                 ↓             ↓
    core/model/chat  core/observation  core/rag   core/vectorstore
    (LLM 调用)       (追踪/指标)       (RAG 管道)  (向量检索)
```

所有集成走 `core.ProcessContext` 对外暴露的方法，**Action 代码无需直接 import lynx 底层包**。

---

## 2. 集成：Chat Client（LLM 调用）

### 2.1 平台持有 chat.Client

```go
// agent/runtime/platform.go（节选）
type Platform struct {
    // ...
    chatClient *chat.Client
}

func WithChatClient(c *chat.Client) PlatformOption {
    return func(p *Platform) { p.chatClient = c }
}
```

### 2.2 创建 process 时透传到 context

```go
func (p *Platform) createProcess(...) (*AgentProcess, error) {
    proc := &AgentProcess{...}
    // 构造 ProcessContext，注入 chat client
    proc.ctx = &core.ProcessContext{
        Process:    proc,
        chatClient: p.chatClient,  // 关键：把 platform 的 chat 下发
        // ...
    }
    return proc, nil
}
```

### 2.3 Action 里使用

```go
core.NewAction("analyze",
    func(ctx context.Context, pc *core.ProcessContext, input Document) (Analysis, error) {
        // 直接用 Lynx chat API
        resp, err := pc.LLM().
            ChatWithText("Summarize: " + input.Text).
            WithOptions(chat.WithMaxTokens(500)).
            Call().
            Response(ctx)
        if err != nil { return Analysis{}, err }

        return Analysis{Summary: resp.Result().AssistantMessage.Text}, nil
    },
)
```

### 2.4 结构化输出

```go
// 等价 embabel 的 createObject<O>()
parser := chat.NewJSONParser[Analysis]()

result, _, err := pc.LLM().
    ChatWithText("Extract key points: " + input.Text).
    Call().
    Structured(ctx, parser)

// result 是强类型 Analysis
```

### 2.5 Streaming

```go
for chunk, err := range pc.LLM().ChatWithText(prompt).Stream().Text(ctx) {
    if err != nil { return err }
    pc.OutputChannel.Write(chunk)  // 推送给用户
}
```

---

## 3. 集成：Observation（追踪/指标）

### 3.1 平台持有 Registry

```go
func WithObservation(r observation.Registry) PlatformOption {
    return func(p *Platform) { p.observations = r }
}
```

### 3.2 框架层自动埋点

```go
// agent/runtime/process_run.go
func (p *AgentProcess) Tick(ctx context.Context) error {
    ctx, obs := p.platform.observations.Start(ctx, "agent.tick",
        observation.String("agent.name", p.Agent.Name),
        observation.String("agent.process_id", p.ID),
    )
    defer obs.End()

    // ...
    worldState := p.determiner.DetermineWorldState(ctx)
    obs.SetAttr("agent.world_state.size", int64(len(worldState.State())))

    err := p.formulateAndExecutePlan(ctx, worldState)
    if err != nil { obs.SetError(err) }
    return err
}

func (p *AgentProcess) executeAction(ctx context.Context, action core.Action) (core.ActionStatus, *ReplanRequest) {
    ctx, obs := p.platform.observations.Start(ctx, "agent.action",
        observation.String("agent.action.name", action.Name()),
    )
    defer obs.End()
    // ...
}
```

每个 tick / action / plan 都是一个 span，自动形成调用树。

### 3.3 规划器埋点

```go
// agent/planner/goap/astar.go
func (p *AStarPlanner) PlanToGoal(ctx context.Context, ...) (*plan.Plan, error) {
    reg := observation.FromContext(ctx)
    ctx, obs := reg.Start(ctx, "agent.planner.astar",
        observation.String("goal.name", goal.Name),
    )
    defer obs.End()

    // 迭代完成后：
    obs.SetAttr("astar.iterations", int64(iterations))
    obs.SetAttr("astar.plan_length", int64(len(path)))
    // ...
}
```

### 3.4 Action 自定义属性

```go
func myAction(ctx context.Context, pc *core.ProcessContext, input Foo) (Bar, error) {
    ctx, obs := pc.Observe(ctx, "my_action.custom_work",
        observation.String("input.id", input.ID),
    )
    defer obs.End()

    // 业务逻辑
    result := process(input)
    obs.SetAttr("output.size", int64(len(result.Items)))
    return result, nil
}
```

**所有 span 最终流向用户配的后端**（noop / slog / observations/otel），零额外集成工作。

---

## 4. 集成：RAG

Lynx 已有 `core/rag.Pipeline`。在 agent 中把 RAG 当作一个 Action 使用：

```go
core.NewAction("retrieve_context",
    func(ctx context.Context, pc *core.ProcessContext, query UserQuery) (RetrievedDocs, error) {
        // 从 platform 或 pc 里获取已配置的 RAG pipeline
        pipeline := pc.Services().RAG()

        augmented, err := pipeline.Execute(ctx, &rag.Query{Text: query.Text})
        if err != nil { return RetrievedDocs{}, err }

        docs := augmented.Extra[rag.DocumentContextKey].([]*document.Document)
        return RetrievedDocs{Docs: docs}, nil
    },
)
```

`ProcessContext` 可以挂载更多服务：

```go
// agent/core/services.go
type ServiceProvider struct {
    chat        *chat.Client
    rag         *rag.Pipeline
    vectorStore vectorstore.VectorStore
    observations observation.Registry
}

func (s *ServiceProvider) Chat() *chat.Client      { return s.chat }
func (s *ServiceProvider) RAG() *rag.Pipeline       { return s.rag }
func (s *ServiceProvider) VectorStore() vectorstore.VectorStore { return s.vectorStore }
// ...
```

这样 Platform 构造时注入一组 services，Action 按需取用。

---

## 5. 集成：Tools（工具调用）

embabel 的 `toolGroups: Set<ToolGroupRequirement>` 声明 action 需要的工具。Lynx 已有 `chat.Tool` 接口。

### 5.1 Action 声明工具组

```go
core.NewAction("research_topic",
    func(ctx context.Context, pc *core.ProcessContext, topic Topic) (Research, error) {
        // 向 LLM 请求，平台会自动注入 tool 定义
        resp, err := pc.LLM().
            ChatWithText("Research: " + topic.Name).
            WithTools(pc.ResolveTools("web", "calculator")).  // 从 toolGroups 拿
            Call().
            Response(ctx)
        // ...
    },
    core.WithToolGroups("web", "calculator"),  // 声明需求
)
```

### 5.2 Tool Group 解析器

```go
// agent/core/tool_group.go
type ToolGroupRequirement struct {
    Role string  // "web" / "calculator" / "code" 等
}

type ToolGroupResolver interface {
    Resolve(requirements []ToolGroupRequirement) []chat.Tool
}

// 平台注入默认实现
type StaticToolGroupResolver struct {
    groups map[string][]chat.Tool
}

func (r *StaticToolGroupResolver) Register(role string, tools ...chat.Tool) {
    r.groups[role] = append(r.groups[role], tools...)
}
```

### 5.3 ToolMiddleware 注册

在 Lynx 里 ToolMiddleware 已经是普通中间件（我们之前移除了硬编码）。Agent 框架在创建 `chat.Client` 时自动帮用户注册：

```go
func (p *Platform) createChatClientForAction(action core.Action) *chat.Client {
    req := p.chatClient.Chat()
    if len(action.ToolGroups()) > 0 {
        tools := p.toolResolver.Resolve(action.ToolGroups())
        callMW, streamMW := chat.NewToolMiddleware()
        req = req.WithTools(tools...).WithMiddlewares(callMW, streamMW)
    }
    return newClientFromRequest(req)
}
```

---

## 6. 完整示例：博客文章生成 Agent

一个能从话题生成带引用的博客文章的 agent，演示**完整的 GOAP 规划流程**。

### 6.1 领域类型

```go
// examples/blog_agent/types.go
package blog

type Topic struct {
    Title    string
    Audience string
}

type Outline struct {
    Topic    Topic
    Sections []string
}

type Research struct {
    Topic   Topic
    Sources []Source
}

type Source struct {
    URL      string
    Snippet  string
}

type BlogPost struct {
    Topic    Topic
    Outline  Outline
    Research Research
    Content  string
    Citations []string
}
```

### 6.2 Agent 定义（DSL 风格）

```go
// examples/blog_agent/agent.go
package blog

import (
    "context"
    "github.com/Tangerg/lynx/agent"
    "github.com/Tangerg/lynx/agent/core"
    "github.com/Tangerg/lynx/core/model/chat"
)

func NewBlogAgent() *core.Agent {
    return agent.New("BlogAgent").
        Description("Generate a well-researched blog post on a given topic").
        Version("1.0.0").

        // 研究：从 Topic 得到 Research
        Action(core.NewAction("research",
            func(ctx context.Context, pc *core.ProcessContext, topic Topic) (Research, error) {
                ctx, obs := pc.Observe(ctx, "blog.research")
                defer obs.End()

                // 用 RAG 拉相关文档
                docs, err := pc.Services().RAG().Retrieve(ctx, topic.Title)
                if err != nil { return Research{}, err }

                sources := make([]Source, len(docs))
                for i, d := range docs {
                    sources[i] = Source{URL: d.Metadata["url"].(string), Snippet: d.Text}
                }
                return Research{Topic: topic, Sources: sources}, nil
            },
            core.WithToolGroups("web"),
            core.WithQoS(core.ActionQos{MaxAttempts: 3}),
        )).

        // 大纲：从 Topic 得到 Outline（独立可并行）
        Action(core.NewAction("outline",
            func(ctx context.Context, pc *core.ProcessContext, topic Topic) (Outline, error) {
                parser := chat.NewJSONParser[Outline]()
                result, _, err := pc.LLM().
                    ChatWithText("Generate 5-section outline for: " + topic.Title).
                    Call().
                    Structured(ctx, parser)
                if err != nil { return Outline{}, err }
                return result, nil
            },
        )).

        // 写作：综合 Research + Outline 输出 BlogPost
        Action(core.NewAction("write",
            func(ctx context.Context, pc *core.ProcessContext, _ WriteInput) (BlogPost, error) {
                // WriteInput 是聚合类型（下面解释）
                outline, _ := core.Get[Outline](pc.Blackboard, core.DefaultBinding)
                research, _ := core.Get[Research](pc.Blackboard, core.DefaultBinding)
                topic, _ := core.Get[Topic](pc.Blackboard, core.DefaultBinding)

                prompt := buildBlogPrompt(topic, outline, research)
                resp, err := pc.LLM().ChatWithText(prompt).Call().Response(ctx)
                if err != nil { return BlogPost{}, err }

                return BlogPost{
                    Topic: topic, Outline: outline, Research: research,
                    Content: resp.Result().AssistantMessage.Text,
                    Citations: extractCitations(research.Sources),
                }, nil
            },
            // 显式声明依赖 Outline 和 Research（黑板必须同时有二者才可执行）
            core.WithPre("it:blog.Outline", "it:blog.Research"),
        )).

        // 目标：产出一篇 BlogPost
        Goal(core.GoalProducing[BlogPost]("A complete blog post with citations").
            WithValue(1.0)).

        Build()
}

// WriteInput 是个聚合占位，用来满足 NewAction 的类型签名
// 实际数据从黑板多次取出
type WriteInput struct{}
```

### 6.3 运行

```go
// examples/blog_agent/main.go
package main

func main() {
    ctx := context.Background()

    // 1. 搭起 Lynx 基础设施
    chatClient := buildChatClient()           // core/model/chat
    ragPipeline := buildRAGPipeline()          // core/rag
    obs := slogobs.New(slog.Default())         // core/observation（slog）

    // 2. Agent Platform
    platform := agent.NewPlatform(
        agent.WithChatClient(chatClient),
        agent.WithObservation(obs),
        agent.WithServices(&core.ServiceProvider{RAG: ragPipeline}),
        agent.WithProcessType(core.ProcessSimple),  // 先用顺序模式
    )

    // 3. 部署 Agent
    blogAgent := blog.NewBlogAgent()
    platform.Deploy(blogAgent)

    // 4. 运行
    topic := blog.Topic{Title: "GOAP in Go agent frameworks", Audience: "Go developers"}
    proc, err := platform.RunAgent(ctx, blogAgent, map[string]any{
        "it": topic,  // 绑定到默认 "it"
    })
    if err != nil { log.Fatal(err) }

    // 5. 取结果
    post, ok := core.ResultOfType[blog.BlogPost](proc)
    if !ok { log.Fatal("agent did not produce BlogPost") }

    fmt.Println(post.Content)
    fmt.Println("References:")
    for _, c := range post.Citations { fmt.Println("-", c) }
}
```

### 6.4 规划器的工作轨迹

输入黑板：`{it: Topic}`

**Tick 1**：
- 世界状态：`{"it:blog.Topic": True, "hasRun_*": False}`
- 目标：`produce BlogPost` 的前提是 `{"it:blog.BlogPost": True}`
- 规划器 A* 搜索：
  - 能达成 `BlogPost` 的只有 `write` → 但 `write` 前提是 `Outline` 和 `Research`
  - 能达成 `Outline` 的是 `outline`（前提：`Topic` ✓）
  - 能达成 `Research` 的是 `research`（前提：`Topic` ✓）
  - **计划**：`[research, outline, write]` 或 `[outline, research, write]`（A* 选代价更低的）
- SimpleProcess 执行第一个：`research`

**Tick 2**：
- 黑板新增 `it:blog.Research`
- 计划剩 `[outline, write]`，执行 `outline`

**Tick 3**：
- 黑板新增 `it:blog.Outline`
- 计划剩 `[write]`，执行 `write`

**Tick 4**：
- 黑板新增 `it:blog.BlogPost`
- 规划器发现 goal 已达成 → 空计划 → `StatusCompleted`

### 6.5 并发优化（如果改用 ConcurrentProcess）

- Tick 1：`research` 和 `outline` 都可达（都只依赖 `Topic`）→ 并发执行
- Tick 2：`write` 可达 → 执行
- Tick 3：完成

**总耗时从 3×action_time 降到 max(research, outline) + write_time**，这是 GOAP 的典型收益。

---

## 7. 集成：HITL（人工介入）

embabel 有 `Awaitable` / `ConfirmationRequest` / `FormBindingRequest`。Go 版本：

```go
// agent/hitl/awaitable.go
package hitl

type Awaitable interface {
    ID() string
    Prompt() string  // 给人看的提示
}

type ConfirmationRequest struct {
    id     string
    prompt string
}

type FormRequest struct {
    id     string
    schema *jsonschema.Schema  // 用 Lynx 已有的 jsonschema
}

// Action 返回 AwaitingTools 状态
func (pc *ProcessContext) AwaitInput(req Awaitable) core.ActionStatus {
    pc.Blackboard.Set("lynx:hitl:pending", req)
    return core.ActionWaiting
}
```

Platform 看到 `ActionWaiting` 时暂停流程，等外部 API 调用 `Platform.ResumeProcess(id, userInput)`，写入黑板然后重启 tick。

---

## 8. 事件监听示例

```go
// 自写一个 slog 监听器
type SlogListener struct{ logger *slog.Logger }

func (l *SlogListener) OnEvent(e event.Event) {
    switch ev := e.(type) {
    case event.ActionExecutionStartEvent:
        l.logger.Info("action start", "name", ev.Action.Name(), "process", ev.ProcessID)
    case event.ActionExecutionResultEvent:
        l.logger.Info("action done", "name", ev.Action.Name(), "status", ev.Status, "dur", ev.Duration)
    case event.GoalAchievedEvent:
        l.logger.Info("goal achieved", "goal", ev.Goal.Name)
    case event.ProcessFailedEvent:
        l.logger.Error("process failed", "err", ev.Err)
    }
}

// 注册
platform.AddListener(&SlogListener{logger: slog.Default()})
```

---

## 9. 与 observation 的事件桥接

可选加一个桥接 listener，把事件自动转为 observation span（让不写自定义 listener 的用户也能通过 observation 看到 agent 行为）：

```go
// agent/event/bridge_observation.go
type ObservationBridge struct {
    registry observation.Registry
    spans    sync.Map  // processID:actionName -> observation.Observation
}

func (b *ObservationBridge) OnEvent(e event.Event) {
    switch ev := e.(type) {
    case event.ActionExecutionStartEvent:
        _, obs := b.registry.Start(context.Background(), "agent.action", ...)
        b.spans.Store(ev.ProcessID+":"+ev.Action.Name(), obs)

    case event.ActionExecutionResultEvent:
        key := ev.ProcessID+":"+ev.Action.Name()
        if v, ok := b.spans.LoadAndDelete(key); ok {
            obs := v.(observation.Observation)
            obs.SetAttr("agent.action.status", ev.Status.String())
            obs.End()
        }
    }
}
```

---

## 10. 小结

完整的集成拓扑：

```
Action 代码
    ↓ pc.LLM()
chat.Client ──────→ 复用 Lynx 已有 core/model/chat
    ↓
Action 代码
    ↓ pc.Services().RAG()
rag.Pipeline ─────→ 复用 Lynx 已有 core/rag
    ↓
Action 代码
    ↓ pc.Observe()
observation.Registry → 复用 Lynx 已有 core/observation
    ↓
(外挂) agents/a2a / agents/mcp / agents/shell
    ↓
独立 go module，不污染 core
```

**关键原则**：
1. 用户 Action 代码**不直接 import `github.com/Tangerg/lynx/core/...`**，全部通过 `core.ProcessContext` 的方法访问
2. 这样将来替换底层（换 LLM 客户端 / 换观测后端 / 换 RAG 实现）都不影响 Action 代码
3. Agent 框架是 **Lynx 生态的首个高阶功能**，把 chat / RAG / observation 编排在一起

---

下一份：落地计划 → `06-rollout.md`
