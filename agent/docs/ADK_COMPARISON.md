# lynx/agent vs google/adk-go — 架构对比与启发

> **2026-05-09**。基线：lynx/agent (Go) HEAD `ec13a93` / adk-go HEAD（路径 `/Users/tangerg/Desktop/adk-go`）。
>
> 本文姊妹篇：[`./EMBABEL_GAP_ANALYSIS.md`](./EMBABEL_GAP_ANALYSIS.md)（vs Kotlin/Spring 端的 embabel-agent）。**embabel 与 lynx 是同源框架**（GOAP planner + Action/Goal/Condition），所以那份文档逐项对齐打分。**ADK 是另一种范式**——LLM-first agent runtime，没有 planner、没有 goal 可达性分析、没有 action precondition；本文目的不是打分，而是**抽出 ADK 在另一种范式下解决得好的问题、考虑哪些值得借鉴到 lynx**。

---

## 0. TL;DR — 范式差异

| 维度 | lynx/agent | adk-go |
|---|---|---|
| **决策引擎** | GOAP planner（A* / HTN / Reactive 三选一）搜索 action 序列；LLM 是 action body 内的工具 | **LLM 即决策引擎**；framework 不规划，把 user message + tools + sub-agents 喂给 LLM，让它决定 |
| **Agent 含义** | 一个**有结构**的对象：`Actions / Goals / Conditions / StuckHandler / Version` 的集合，部署后可被 planner 推导 | 一个**无状态 callable**：`Run(InvocationContext) iter.Seq2[*Event, error]`；身份就是"接受消息、产 event"的函数 |
| **状态模型** | per-process **Blackboard**（typed 对象 + named keys + conditions）+ WorldState 快照 | 跨 turn **Session**（不可变 event log + 由 StateDelta 重放出的 State）+ 跨 session **Memory**（语义检索）+ 多版本 **Artifact**（文件） |
| **组合模型** | **inline 嵌套**：`AsChatTool[In, Out]` 把子 agent 包成 LLM 工具，父 LLM 在一次 tool call 里拿到子 agent 的 typed Out | **explicit transfer**：LLM 调 `transfer_to_agent(name)`，**下一条** user message 路由到目标 agent；不能在一次 turn 内 inline 嵌套 |
| **Workflow** | **action 级**：`ScatterGather` / `RepeatUntil` / `Consensus` 编织 actions | **agent 级**：`SequentialAgent` / `ParallelAgent` / `LoopAgent` 编织 sub-agents（不走 LLM） |
| **Streaming** | event 走 `event.Multicast`（fire-and-forget）；调用方 `RunAgent` 同步阻塞 | 顶层 API 直接返回 `iter.Seq2[*Event, error]`；token-by-token streaming + `Partial=true` 标记；client 直接 range |
| **持久化** | 仅 `BlackboardFactory` 扩展点（按设计不在 framework 内做） | **三个一等服务**：`SessionService` / `MemoryService` / `ArtifactService`，runner 直接依赖 |
| **部署形态** | 纯库 | 三个 server frontend：`adkrest`（HTTP）/ `adka2a`（A2A 协议）/ `agentengine`（Vertex AI managed） |
| **配置** | 纯 typed DSL（`agent.New("x").Actions(...).Build()`） | typed DSL **和** YAML（`internal/configurable`，给 no-code / ops 路径） |

**一句话**：lynx 是**编译期保证的 planner-driven 库**；ADK 是**runtime 灵活的 LLM-driven chat backend runtime**。两者解决的核心问题不同——lynx 强在"agent 行为可静态推理 + 嵌入式部署"，ADK 强在"作为产品级 chat backend 的运行时基础设施"。lynx 主动让 Session / Memory / Artifact / SSE server 等 chat-backend plumbing 留在 chat 中间件层 / sibling repo / 用户应用代码，**职责边界是核心设计**，不是缺口。

---

## 1. 决策引擎 — planner vs LLM

### lynx
- `core.Agent` 持有 `Actions / Goals / Conditions`；planner 在 tick 时搜索"effect 满足 goal precondition 的 action 序列"。
- LLM 是 action body 内的工具：`pc.Chat().WithTools(...).Call().Text(ctx)`。
- 编译期可推：`runtime.Platform.Deploy(a)` 拒绝 unreachable goal、duplicate name、nil action。

### ADK
- 没有 planner。LLM 收到 system instruction + tools + history，**它自己决定**调哪个 tool、是否 transfer、何时停。
- agent 没有 goal/precondition；只有 `name / description / instruction / tools / sub_agents`。
- 错误（如调一个不存在的 tool）只能 runtime 发现。

### 启发
**不要照搬**——lynx 的 planner-driven 设计是核心价值；放弃它就退化成另一个 LangChain。但可以学：
- ADK 的 `LlmAgent.Instruction` 模板（[`agent/llmagent/llmagent.go`](file:///Users/tangerg/Desktop/adk-go/agent/llmagent/llmagent.go) 含 `{state.foo}` 变量插值）—— lynx 当前 prompt 拼装靠 `fmt.Sprintf`，加一个 `core.PromptTemplate(...)` 薄包装可借鉴（已记入 [EMBABEL_GAP_ANALYSIS.md §15.5 P2 #7](./EMBABEL_GAP_ANALYSIS.md)）。

---

## 2. Agent 身份 — struct vs iter

### lynx
```go
type Agent struct {
    AgentConfig  // Name / Description / Version / Actions / Goals / Conditions / StuckHandler
    knownConditions atomic.Pointer[map[string]struct{}]
}
```
**有结构**。Platform 持有 `agentRegistry`，`FindAgent(name)` 拿到的是这个完整定义。

### ADK
```go
type Agent interface {
    Name() string
    Description() string
    Run(InvocationContext) iter.Seq2[*session.Event, error]
    SubAgents() []Agent
    FindAgent(name string) Agent
    FindSubAgent(name string) Agent
    internal() *agent
}
```
**无结构**——身份就是 Run。LlmAgent / SequentialAgent / RemoteAgent 都满足这个接口；它们的"是什么"完全藏在 Run 里。

### 启发
**lynx 不需要换**——结构化定义是 GOAP 的前提。但 `Run() iter.Seq2[*Event, error]` 这个接口形态值得思考：

- **lynx 当前 `Platform.RunAgent` 是同步阻塞的**（返回 `*AgentProcess + error`），事件靠 `EventListener` 扩展接收。这意味着 HTTP/SSE 集成必须自己写"创建 listener → 转发到 SSE → 等 process terminate"的胶水。
- **ADK 让顶层 API 直接是 iterator** —— `runner.Run(ctx, msg)` 返回 `iter.Seq2[*Event, error]`，调用方 `for ev, err := range stream { ... }` 即可流式消费。

可以考虑给 lynx 加一个 streaming 入口：

```go
// 提议
func (p *Platform) RunAgentStream(ctx, agent, bindings, opts) iter.Seq2[event.Event, error]
```

实现：内部启动 process + 装一个 channel-backed listener + 把 channel 包成 iterator + process terminate 时 close channel。这是**纯叠加**——同步 API 不变。约 80 LOC + 测试。

> 标记为 P1 候选改进（事件流式 API）。

---

## 3. 状态模型 — Blackboard vs Session/Memory/Artifact 三件套

### lynx
- **Blackboard**（per-process）：typed 对象 + named keys + protected keys + hidden + conditions（[`runtime/in_memory_blackboard.go`](../runtime/in_memory_blackboard.go)）。
- 跨 process / 跨 session：**没有**。Blackboard 随 process 终止释放。`BlackboardFactory` 扩展点允许换持久化后端，但默认 in-memory。

### ADK
三个独立服务：

#### a) SessionService — 多 turn 对话
- `Session` = `id / appName / userID / state / events`
- **Event log 不可变**；State 是 StateDelta 重放出来的派生量
- 用户：跨 turn 持久化（数据库 / 文件 / Vertex managed）

#### b) MemoryService — 跨 session 语义记忆
- `SearchMemory(query) → []MemoryResult`
- 把过往 sessions 喂进向量库，agent 想起来时检索

#### c) ArtifactService — 版本化二进制输出
- `Save(name, *genai.Part) → SaveResponse{version}`
- 用途：图像、PDF、生成的报告

### 启发 — **明确不做**

ADK 把这三件 plumbing 做进 framework，是因为它定位是 **chat backend runtime**——session/memory/artifact 是它的产品形态。

lynx 的定位是**库**而非全栈 chat backend。这三类需求**完全交给 chat 中间件层处理**：
- 多 turn 对话历史 / 跨 session 状态 → `core/model/chat` 已有的 history middleware / message store middleware
- 跨 session 语义记忆 / RAG → `lynx/rag` (sibling repo) 作为 chat middleware 注入
- 多模态 artifact → chat package 的多模态 part 模型 + 用户自己挂存储

**关键认识**：lynx agent runtime 的职责边界停在"一次 process 跑完一个 OODA 循环"，**不持有跨 process / 跨 session 的对话状态**。把 Session 提到 framework 顶层会越界——就是开始做 chat backend 而不是 planner runtime 了。

不做的具体项：
- ~~`core.SessionStore`~~ — chat middleware 解决
- ~~`core.MemoryService`~~ — `lynx/rag` 解决
- ~~`core.ArtifactStore`~~ — chat 的多模态 part + 用户自管存储

**下一节的 §6 streaming、§8 server 同此哲学**——不在 lynx 范围。

---

## 4. 组合模型 — inline subagent vs explicit transfer

### lynx
父 agent 在 action body 里：
```go
tools := []chat.Tool{
    runtime.AsChatTool[Topic, Brief](platform, "researcher"),
    runtime.AsChatTool[Brief, BlogPost](platform, "writer"),
}
text, _ := pc.Chat().WithTools(tools...).Call().Text(ctx)
```
父 LLM 在**一次 tool call** 里拿到子 agent 的 `Brief` typed 输出，可以继续推理。

### ADK
LlmAgent 自动暴露 `transfer_to_agent(name)` 工具；LLM 调它：
```
LLM: "我想让 researcher 处理这个，调用 transfer_to_agent('researcher')"
[Event: TransferToAgent='researcher']
[runner 把当前 turn 结束]

User 下一条 message:
[runner: 找到上次 transfer 目标 'researcher'，路由给它]
researcher 处理...
```
**两个 turn**，不能 inline。父 agent 不能在自己这一 turn 里"先委托给 child，拿到 child 的结果再继续推"。

### 优劣对比

| 维度 | lynx inline | ADK transfer |
|---|---|---|
| 单 turn 完成 多 agent 协作 | ✅ | ❌ 必须多 turn |
| 父 LLM 看到 typed sub-agent 输出 | ✅（`Out` 反序列化进 LLM context） | ❌ 用户下次说话时换了 agent |
| 控制权流转明确性 | 弱（LLM 决定调不调） | 强（每次 transfer 是显式 event） |
| 适合 chat-like 长程对话 | 中（inline 适合一次性任务，长对话切换重） | ✅ |
| 适合 agentic-task pipeline | ✅ | 中（每步要 user driver） |

### 启发
**lynx 已经有 inline，应该补 explicit 模式**——但**不是直接抄 ADK 的语义**。提议方案：

把 ADK 的"transfer 后下一 turn 路由给目标"思路抽象成 lynx 的**"goal handoff"**机制：

```go
// 提议：在 hitl/awaitable 同层加一个 Handoff awaitable
type Handoff struct {
    AgentName string   // 下一 turn 路由到的 agent
    Reason    string
    Bindings  map[string]any  // 给目标 agent 的 blackboard seed
}

// action body 里：
return ActionWaiting, pc.AwaitInput(&runtime.HandoffRequest{To: "researcher", ...})

// 用户继续 process 时：
platform.ContinueProcess(ctx, processID)  // 自动切到 researcher 路径
```

但这又落回 lynx 的 typed-action 风格——本质是 **action 完成时 publish 一个 "next agent" 事件**。runtime 看到 handoff event 就执行新 agent。

> ROI：低-中。lynx inline 已覆盖大部分场景；handoff 是 chat-product 特有需求，等真有用例再做。

---

## 5. Workflow — action-level vs agent-level

### lynx
[`workflow/`](../workflow) 编织 **actions**：
- `ScatterGather[In, Element, Result]` — N 个 generator 并行 → 1 个 joiner
- `RepeatUntil[In, Out]` — 一个 task action 反复执行直到 condition 满足
- `Consensus[In, Element]` — 多投票收敛

每个 builder 返回一个 **agent**，agent 内部是"一组 actions + 一个 goal"。

### ADK
[`agent/workflowagents/`](file:///Users/tangerg/Desktop/adk-go/agent/workflowagents) 编织 **whole sub-agents**：
- `SequentialAgent` — 子 agent 1 → 子 agent 2 → 子 agent 3 串行
- `ParallelAgent` — 子 agent 们并行（**branch isolation**：每个子 agent 看不到 peer 的 history）
- `LoopAgent` — 反复运行 sub-agents 直到终止条件

ParallelAgent 走 errgroup + branch ID（`parent.agent1` / `parent.agent2`）确保 session log 正确切片。

### 启发

**两者解决不同粒度的问题，可以并存**：

| 用例 | 适合 |
|---|---|
| "并行调 3 个 LLM 评估同一份文档" | lynx ScatterGather（action 级） |
| "先研究 → 再写 → 再润色（每个是独立 agent）" | ADK Sequential（agent 级） |
| "同时跑 researcher / fact-checker / writer 三个 agent，gather 它们的输出" | ADK Parallel + branch isolation |

**lynx 当前没有 agent 级 workflow** —— 子 agent 调用走 `AsChatTool`（要 LLM 决定），workflow 走 action 级。**这是 ADK 启发里唯一明确要做的一项**。

### 设计

三个新增函数，全部住在 [`workflow/`](../workflow) 包，与现有的 action 级 builder 同包：

```go
// workflow/sequence.go
//
// Sequence 编译 a₁ → a₂ → ... → aₙ 串行链为单个 agent。
// 每个子 agent 通过 Platform.CreateChildProcess 跑一个 child process,
// 继承父 blackboard via Spawn(); 上一个子 agent 的 typed output 通过
// Bind() 流到父 blackboard, 下一个子 agent 的 planner 通过 dual-binding
// 自动取到。
//
// 类型契约 trade-off: Go variadic 泛型不能表达 heterogeneous 链;
// 相邻 agent 间的 In/Out 类型对齐由用户在 agent 声明里负责; 不匹配
// runtime fail-fast。要严格类型链的用户用 core.NewAction[Mid_i, Mid_{i+1}]
// 自己包一层。
//
// Panics on empty name, len(agents) < 2, or any nil agent.
func Sequence(name string, agents ...*core.Agent) *core.Agent

// workflow/parallel.go
//
// Parallel 编织 N 个同 In/Element 型的 sub-agent 并行 fan-out,
// 然后 Joiner 把 []Element 合并为 Result。每个并行 sub-agent 走独立
// 的 Blackboard.Spawn() 副本 (branch isolation), 避免 LLM 上下文交叉
// 污染——这是 ADK ParallelAgent 设计的关键, lynx Spawn 已有支持。
type ParallelSpec[In, Element, Result any] struct {
    Name           string
    Description    string
    MaxConcurrency int
    Agents         []*core.Agent  // 都消费 In, 产 Element
    Joiner         func(ctx context.Context, pc *core.ProcessContext, results []Element) (Result, error)
}
func Parallel[In, Element, Result any](spec ParallelSpec[In, Element, Result]) *core.Agent

// workflow/loop.go
//
// Loop 反复运行 body sub-agent 直到 Until predicate 返回 true。
// MaxIterations 强制兜底 (≤0 默认 5)。每次迭代 body 走独立 child
// process, blackboard via Spawn(); body 的 Out 通过 Bind() 回流父
// blackboard 给下次迭代 (与 Sequence 同 plumbing)。
type LoopSpec[In, Out any] struct {
    Name          string
    Description   string
    MaxIterations int
    Body          *core.Agent
    Until         func(ctx context.Context, in In, last Out) bool
}
func Loop[In, Out any](spec LoopSpec[In, Out]) *core.Agent
```

### 实现要点

1. **基础设施零新增**：三者皆基于 `Platform.CreateChildProcess` + `Blackboard.Spawn` + `ContinueProcess`——同 [`runtime/subagent.go`](../runtime/subagent.go) 中 `runAsChild` 的 plumbing 一脉。
2. **预算自动聚合**：`CreateChildProcess` 已实现递归 budget 聚合（[runtime/agent_process.go:125](../runtime/agent_process.go)）；sub-agent 的 token / cost 自动算到父 process。
3. **失败传播**：任一 child 非 `StatusCompleted` → 父 action `ActionFailed`，父 process 进入 `StatusFailed`，child failure 作为 cause。
4. **goal 形态**：编译产物是单 action（`CanRerun=false`）+ 单 goal（`Pre: ["{name}-done"]`），与现有 `RepeatUntil` / `ScatterGather` 同形。

### 与现有 building block 互补

| 用例 | 应该用 |
|---|---|
| LLM 决定调哪个子 agent（自由探索） | `runtime.AsChatTool[In, Out]`（LLM-driven） |
| 并行调 N 个 generator 函数（无 agent 身份） | `workflow.ScatterGather`（action 级） |
| 反复调一个 task 闭包直到满意 | `workflow.RepeatUntil`（action 级） |
| **`research → write → edit` 三个独立 agent 串行** | **`workflow.Sequence`（新）** |
| **同时跑 3 个 evaluator agent，各自带 LLM/tools 树** | **`workflow.Parallel`（新）** |
| **反复运行一个完整 critic agent 直到通过** | **`workflow.Loop`（新）** |

**约 250 LOC + 测试**。零核心改动，全部 workflow/ 包内。**P1 推荐落地**。

---

## 6. Streaming — Multicast vs iter.Seq2

### lynx
- 事件走 `event.Multicast.Add(listener)`，listener 是 `OnEvent(e Event)` 回调
- **调用方拿不到 stream**：`platform.RunAgent` 同步阻塞返回 `*AgentProcess`
- HTTP/SSE 集成：用户写一个 `EventListener` 扩展，把事件转到 channel，外面消费 channel

### ADK
```go
stream := runner.Run(ctx, userMessage)
for ev, err := range stream {
    if err != nil { return err }
    respondToClient(ev)
}
```
- 顶层 API 是 `iter.Seq2[*Event, error]`
- **Partial event** 标记：`event.Partial=true` 时是 token delta，不持久化但发给 client
- 内部通过 channel + goroutine 实现，对外是 Go 1.23 iterator

### 启发 — **`RunAgentStream` 不做；改在 event 包提供一个 listener helper**

考虑过加 `Platform.RunAgentStream() iter.Seq2`，**否决**。原因：

- `PlatformConfig.Extensions` 已经是注册 listener 的**单一入口**；如果 `RunAgentStream` 内部偷偷再注入一个临时 listener，就出现"new 时 explicit + RunStream 时 hidden"两条路径，是**黑盒**。一个 process 该用哪些 listener，应该全部由 caller 在 `Extensions` 里显式声明。
- streaming 真实需求（SSE / WebSocket）的核心是"拿到 event 流"，**不是"必须以 iterator 形态拿"**。channel 一样能流式消费。

**改做**：在 [`event/`](../event) 包加一个 listener helper，让"想 channel-style 消费 event 的用户"通过现有 `Extensions` 路径注册：

```go
// event/listener.go (新增)

// NamedListener wraps a function as a runtime EventListener (an
// Extension that observes every Event). Drop into
// PlatformConfig.Extensions or ProcessOptions.Extensions; the
// runtime fans every event through fn.
//
// Example — channel-backed streaming:
//
//	ch := make(chan Event, 64)
//	listener := event.NewNamedListener("sse-stream", func(e Event) {
//	    select { case ch <- e: default: /* drop on backpressure */ }
//	})
//	opts := core.ProcessOptions{Extensions: []core.Extension{listener}}
//	go func() {
//	    defer close(ch)
//	    _, _ = platform.RunAgent(ctx, agent, bindings, opts)
//	}()
//	for e := range ch { sse.Send(e) }
type NamedListener struct {
    name string
    fn   func(Event)
}

func NewNamedListener(name string, fn func(Event)) *NamedListener
func (n *NamedListener) Name() string
func (n *NamedListener) OnEvent(e Event)
```

**好处**：
- 一条注册路径（`Extensions`），无黑盒
- 每个 listener 有 `Name()` 走现有的 dedup / panic-on-duplicate 机制
- 用户决定 channel buffer / 背压 / lifecycle（close 时机）—— framework 不替用户做决策
- 同步 `RunAgent` API 不变；不引入 `iter.Seq2` 新返回类型

**不做的具体项**：
- ~~`Platform.RunAgentStream`~~ — 黑盒注入问题
- ~~Partial event 标记~~ — chat 中间件层已经做 token streaming，agent runtime 不掺和

> ROI：低（~30 LOC + 文档）。但消除一个真实工程痛点（"如何把事件拿出来"）。**P2 推荐落地**。

---

## 7. Plugin / Extension — callback vs interface

### lynx
[`core/extension.go`](../core/extension.go) + [`runtime/extension.go`](../runtime/extension.go)：
- 一个 `Extension` 接口（仅 `Name() string`）
- 9 个 capability 接口：`ActionInterceptor / ToolDecorator / AgentValidator / GoalApprover / BlackboardFactory / EventListener / PlannerFactory / IDGenerator / ToolGroupResolver`
- 用户实现一个 struct 同时满足多个 capability，runtime 用 type assertion 检测
- 一个注册入口（`PlatformConfig.Extensions`）

### ADK
[`plugin/plugin.go`](file:///Users/tangerg/Desktop/adk-go/plugin/plugin.go)：
- 一个 Plugin 接口持有 11 个 callback：
  - `OnUserMessage` / `OnEvent`
  - `BeforeRun` / `AfterRun`
  - `BeforeAgent` / `AfterAgent`
  - `BeforeModel` / `AfterModel` / `OnModelError`
  - `BeforeTool` / `AfterTool` / `OnToolError`
- callback 不实现的方法返回 nil，调用链跳过
- 内置三个 plugin：`loggingplugin` / `retryandreflect` / `functioncallmodifier`

### 优劣对比

| 维度 | lynx interface 风格 | ADK callback 风格 |
|---|---|---|
| 类型安全 | 强（每个 capability 是独立接口） | 弱（11 个 method 都在一个接口） |
| 单点拓展 | 用户每加一个能力写一个新 capability 接口 | 用户每加一个 hook 写一个新 callback |
| 一个 plugin 多个 hook | ✅（一个 struct 实现多个 interface） | ✅（一个 struct 实现多个 callback method） |
| 启动时校验 | ✅（duplicate name panic） | ❌（runtime fail-soft） |
| **AOP 风格的 Before/After** | ❌（lynx 只有 `ActionInterceptor` 是 wrap，没有 `Before*Tool` / `Before*Model`） | ✅（细粒度 hook） |

### 启发

ADK 的 plugin model **有一处真值得借鉴**：**Before/After Tool callback**。

lynx 当前：
- `ToolDecorator` 是 wrap 风格（`DecorateTool(process, action, tool) → tool`）—— 改 tool 行为
- 但没有"在 tool 执行前后跑一段代码"的 hook（用 `ToolDecorator.DecorateTool` 返回一个包装 tool 也能做，但不直观）

ADK 的 `BeforeToolCallback(ctx, tool, args, state) (modifiedArgs, skip bool)` 让用户：
- 改 tool 输入
- 跳过 tool 执行（返回 cached result）
- 加日志/审计/budget guard

这个能力 lynx 用 `ToolDecorator` 包一层 closure 也能模拟。**结论**：现状已够用，**不需要新增 plugin model**。lynx interface 风格 + 9 capability 表达力已经覆盖。

---

## 8. 部署 / Server frontends

### lynx
**纯库**。用户自己起 HTTP server / gRPC / SDK 包装。

### ADK
开箱：
- **adkrest** — REST endpoint，POST /sessions/:id/messages / GET /sessions/:id/events
- **adka2a** — A2A 协议 server（agent-to-agent 通信）
- **agentengine** — Vertex AI managed runtime 适配

### 启发 — **明确不做**

lynx 定位**库**，不是大而全包揽所有能力的 framework。HTTP / SSE / WebSocket / A2A / Vertex 适配等 server frontend 全部**在 lynx 范围之外**：

- ~~SSE server~~ — 用户拿 §6 的 `event.NewNamedListener` + 标准库 `net/http` 自己起 SSE，几行就够；framework 不内置
- ~~A2A 协议~~ — 协议生态未成熟；lynx 已有 MCP（[`lynx/mcp`](../../mcp)）覆盖跨进程 agent 调用
- ~~Vertex AI managed runtime~~ — 厂商绑定，不做

哲学：lynx 让 server 部署成为**用户应用代码的一行 boilerplate**而非 framework 的内置模块。这与 lynx 的"小依赖面 / 嵌入 Go 微服务 / Lambda / CLI 工具"定位一致（[EMBABEL_GAP_ANALYSIS.md A.4](./EMBABEL_GAP_ANALYSIS.md)）。

---

## 9. YAML 配置 — 哲学分歧

### ADK
[`internal/configurable/configurable.go`](file:///Users/tangerg/Desktop/adk-go/internal/configurable/configurable.go)：YAML manifest 反序列化为 agent 定义，给 Vertex Engine 的 no-code 路径用：
```yaml
agent_class: LLMAgent
name: my_agent
model: gemini-2.5-flash
instruction: "You are helpful."
sub_agents:
  - config_path: subagent1.yaml
tools:
  - name: my_tool
```

### lynx
**纯 typed DSL**。`agent.New("x").Actions(...).Build()`，无 YAML 路径。

### 启发
**永久分歧**。lynx 的 typed 风格是核心价值（编译期类型 + IDE rename + planner 推理）；YAML 是 ADK 为 Vertex managed runtime 做的妥协。lynx 不需要做。

唯一可借鉴的：**ADK YAML 把 agent 定义和 sub-agent 定义解耦**（`sub_agents.config_path` 引用另一个 YAML 文件）。lynx 在 large agent graph 场景下，typed DSL 仍然 1 文件搞定，问题不大。

---

## 10. 工具系统差异

| 维度 | lynx | ADK |
|---|---|---|
| Tool 定义 | `core.AgentTool = chat.Tool`（typed） | `tool.Tool` interface（动态 args） |
| Tool 解析 | `ToolGroup` + `ToolGroupResolver` 按 role | `LlmAgent.Tools` + `Toolsets`（动态 list） |
| Tool 装饰 | `ToolDecorator` extension（onion wrap） | Plugin `BeforeTool` / `AfterTool` callback |
| Tool 级 HITL | ✅ `hitl.RequireAwait / RequireConfirmation / RequireType[T]`（typed） | ✅ `tool.RequestConfirmation()` → adk_request_confirmation event（dynamic） |
| Long-running tool | 通过 `ActionWaiting` + Awaitable | `Tool.IsLongRunning() bool` 标志 + 内部 ReAct loop 等 |
| OpenAPI tool | 走 chat package | `OpenAPITool` 内置 |
| MCP tool | ✅ `MCPToolGroupResolver`（[`runtime/mcp.go`](../runtime/mcp.go)） | ✅ `examples/mcp/` 支持 |

**lynx 强**：typed `RequireType[T]`、`AsChatTool[In, Out]` typed 端到端。  
**ADK 强**：Long-running tool 标记 + 内置 OpenAPI tool 适配器。

### 启发
- ~~Long-running tool 标记~~ —— lynx `ActionWaiting` 已覆盖，无需重复。
- ~~OpenAPI tool 内置~~ —— `chat.Tool` + 用户自己写 swagger → tool 转换器即可，不进 framework。

---

## 11. 综合启发：lynx 的下一步

ADK 的大部分功能面是**它作为 chat backend runtime 的产品形态**，lynx 作为**库**主动放弃，由 chat 中间件层 / sibling repo / 用户应用代码各自承担。真正可落地到 lynx framework 的 ADK 启发**只有两项**——**两项都已落地**：

| 优先级 | 项目 | 借鉴自 ADK | 改动量 | 状态 |
|---|---|---|---|---|
| **P1** | **Agent 级 workflow agents** —— `workflow.Sequence` / `Parallel[In, Element, Result]` / `Loop[In, Out]` + 配套 `runtime.SpawnChildFresh` 与 `Platform.NewBlackboard()` | `agent/workflowagents/` | ~600 LOC（含三件 builder + 一对 runtime helper + 测试） | ✅ 已落地 |
| **P2** | **`event.NewNamedListener(name, fn)`** —— 让用户能用 channel-style 消费 event，仍走 `Extensions` 单一注册路径 | runner.Run iterator 形态的简化替代 | ~50 LOC（含测试） | ✅ 已落地 |

### 落地中发现的设计点：fresh blackboard

实现 `Loop` 时发现一个关键设计决策——**agent 级 workflow 不能用默认的 `Blackboard.Spawn()` 继承**：

> 如果 Loop 的 iter action 用 `runtime.SpawnChild`（继承父 blackboard）跑 body 子 agent，那么 iter 1 产 `Out` 后由 typed wrapper 回写到父 blackboard；iter 2 调 SpawnChild 时，子 blackboard 通过 Spawn 拿到这个 Out → 子 agent 的 goal `produce Out` **判定已满足** → body 短路不跑。
>
> 同样，Parallel 的 peer 之间需要 branch isolation（避免 LLM 上下文交叉污染——ADK ParallelAgent 的核心设计）；Sequence 也想要每步只看到上一步的 typed output、不看到 orchestrator 的累积写入。

**解决**：新增 `runtime.SpawnChildFresh` 与 `runtime.SpawnChild` 并列：

- `SpawnChild`（保留）—— 子 blackboard 通过 `Spawn()` 继承父，**supervisor 流**用（`AsChatTool` 的语义）；子 agent 看得到父已 staged 的 artifacts
- `SpawnChildFresh`（新增）—— 子 blackboard 通过 `Platform.NewBlackboard()` 完全干净开局，仅 Bind 输入；**orchestration 流**用（`Sequence` / `Parallel` / `Loop` 都走这条）

两条 plumbing 实质区别只在 `core.ProcessOptions{Blackboard}` 一个 slot；budget 聚合 / 父子 process 关系都一样。

### 明确不做（按职责边界）

| 项目 | 替代方案 / 理由 |
|---|---|
| **`core.Session` / `SessionStore`** | chat 中间件层做（history / message store middleware）。lynx agent 职责停在"一次 process 跑完一个 OODA 循环" |
| **`core.MemoryService`** | sibling [`lynx/rag`](../../rag) 作为 chat middleware 注入 |
| **`core.ArtifactStore`** | chat package 多模态 part + 用户自管存储 |
| **`core.PromptTemplate`** | chat package 已有模板渲染实现 |
| **`Platform.RunAgentStream()`** | 黑盒注入问题（参见 §6）；改用 `event.NewNamedListener` + channel |
| **HTTP / SSE server** | lynx 是库不是 framework；用户应用代码三行起 server |
| **A2A 协议** | 协议生态未成熟；MCP 已覆盖 |
| **YAML agent 定义** | typed DSL 是 lynx 核心价值（编译期类型 + IDE rename + planner 推理） |
| **Plugin callback model 替换 Extension** | 9 capability interface 表达力已够；不需要换 |
| **LLM-as-decision（放弃 planner）** | 这是 lynx 核心；放弃即换框架 |

### 顺序建议

1. **先做 P1 agent 级 workflow agents**（`Sequence` / `Parallel` / `Loop`）—— 真实补全：用户当前要做"researcher → writer → editor"这类确定性 agent 流水线必须自己手写 child process plumbing
2. **再做 P2 `event.NamedListener`** —— 30 LOC 顺手补；让 channel-style event 消费有单一入口
3. **embabel 那边的 P0 三件**（[EMBABEL_GAP_ANALYSIS.md §15.5](./EMBABEL_GAP_ANALYSIS.md)：`Goal.Export` / `PromptCondition` / `AchievableGoalsToolGroupFactory`）和这边正交，可并行

---

## 12. 一句话总结

ADK 是为**LLM-driven chat backend 工程化**优化的 runtime；lynx 是为**planner-driven agent 行为可推理**优化的**库**。两者**不是替代关系**，但启发面也比想象中**窄**——ADK 大量 plumbing（Session / Memory / Artifact / Template / SSE server / YAML / A2A）属于"chat backend runtime"职责，lynx 作为库主动让位给 chat 中间件 / sibling repo / 用户应用代码。

真正能跨范式带回 lynx 的只有两项：
1. **agent 级 workflow agents**（`Sequence` / `Parallel` / `Loop`）—— 确定性 sub-agent 编排，弥补 `AsChatTool` 必须 LLM-driven 的缺口
2. **`event.NamedListener`** —— 让 channel-style 流式消费走单一注册路径

其余 ADK 功能面要么 lynx 主动不做（职责边界），要么 lynx 已有等价（HITL / ToolDecorator / Extension model）。落地顺序参考 §11。
