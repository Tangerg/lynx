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

**一句话**：lynx 是**编译期保证的 planner-driven 框架**；ADK 是**runtime 灵活的 LLM-driven runtime**。两者解决的核心问题不同——lynx 强在"agent 行为可静态推理"，ADK 强在"作为产品级 chat backend 的运行时基础设施"。

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

这是 ADK 给 lynx 启发**最大**的一节。

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
- 用户：long-term knowledge 而非 ephemeral

#### c) ArtifactService — 版本化二进制输出
- `Save(name, *genai.Part) → SaveResponse{version}`
- `Load(name)` / `LoadVersion(name, v)` / `List()`
- 用途：图像、PDF、生成的报告 —— 不放进 conversation history、但需要持久引用

### 启发

**这是 lynx 真实存在的概念缺口**——不是工程师没想过，而是 lynx 一直把"长程对话状态"推给用户自己用 `core.ServiceProvider` 拼。但实际工程中：

| 场景 | lynx 现状 | ADK 现状 |
|---|---|---|
| 多 turn chat agent，用户三天后回来继续聊 | 用户自己持久化 blackboard / 自己拼 history | `SessionService.GetSession(id)` 直接拿到完整状态 |
| 跨 session 记住用户偏好（"上周我说过我喜欢咖啡"） | 完全不在 framework 范围 | `MemoryService.SearchMemory("preferences")` |
| agent 生成 PNG 图，下次 turn 引用 | 用户自己塞 blackboard / 自己写 file store | `ArtifactService.Save(...)` + Load by version |

**建议**：在 lynx 做一个**轻量分层**，不是把 ADK 三件套全搬：

#### 分层 A — `core.SessionStore`（接口 + 默认 in-memory 实现）
```go
type Session interface {
    ID() string
    AppName() string
    UserID() string
    Blackboard() Blackboard
    History() []event.Event   // 已发生 events 的 log
}

type SessionStore interface {
    Get(id string) (Session, bool)
    Create(appName, userID string) Session
    Append(s Session, e event.Event) error
}
```
- runtime 启动 process 时从 SessionStore 拿/建 session，复用其 blackboard
- 终止时把 events 写回 store
- 持久化（Redis / SQL）走 SessionStore 实现替换 —— 跟 lynx 现有 `BlackboardFactory` 同形

> ROI：高。让"多 turn chat"成为 framework 顶层概念。约 200 LOC + 测试。

#### 分层 B — `core.MemoryService`（仅 SPI，不内置实现）
```go
type MemoryService interface {
    Search(ctx, query string, limit int) ([]MemoryHit, error)
    Ingest(ctx, session Session) error
}
```
- 不内置向量库（lynx 哲学：embedding/向量库交给 sibling repo）
- 暴露 hook 给 action body：`pc.Memory().Search(...)`
- 用户挂自己的实现（pgvector / qdrant / lynx-rag-sibling）

> ROI：中。可后置；先做 A。

#### 分层 C — `core.ArtifactStore`（接口 + 默认 in-memory）
```go
type ArtifactStore interface {
    Save(ctx, name string, data []byte, mime string) (version int, err error)
    Load(ctx, name string) (data []byte, mime string, version int, err error)
    LoadVersion(ctx, name string, version int) ([]byte, string, error)
}
```
- 多模态/文件场景。生成图、生成报告时常用。
- 默认 in-memory，自定义走接口。

> ROI：低-中。先看用例；blackboard `BindProtected` + 手动 versioning 已能覆盖。

**总结**：lynx 缺的不是"再加一种 storage"，而是缺 **多 turn / 多 session** 的概念抽象。Session 是核心；Memory + Artifact 可后做。

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
- `ScatterGatherAgent[In, Element, Result]` — N 个 generator 并行 → 1 个 joiner
- `RepeatUntilAgent[In, Out]` — 一个 task action 反复执行直到 condition 满足
- `ConsensusAgent[In, Element]` — 多投票收敛

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

**lynx 当前没有 agent 级 workflow**——子 agent 调用走 `AsChatTool`（要 LLM 决定），workflow 走 action 级。**值得加**：

```go
// 提议：workflow/agent_level.go
package workflow

func SequentialAgents(name string, agents ...*core.Agent) *core.Agent { ... }
func ParallelAgents(name string, agents ...*core.Agent) *core.Agent { ... }
```

实现：编译成一个 lynx agent，里面是若干 actions，每个 action 调 `runtime.RunAgent(ctx, sub)` 跑一个子 agent。串行 / 并行只是 action 间的依赖关系。

> ROI：中。一些 chat-orchestration 场景明显适合 agent 级，但 lynx 当前能用 N 个 `AsChatTool` + 父 agent 手写编排绕过。先观察用例。

**关于 branch isolation**：ADK ParallelAgent 让每个子 agent 看不到 peer 的 session events，是**正确设计**——避免 LLM 互相污染上下文。lynx ProcessConcurrent 当前所有 action 共享 blackboard；如果加 agent 级 ParallelAgents，应该走 `Blackboard.Spawn()` 给每个子 agent 独立空间。

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
    // 直接发 SSE / WebSocket / chunked HTTP
    respondToClient(ev)
}
```
- 顶层 API 是 `iter.Seq2[*Event, error]`
- **Partial event** 标记：`event.Partial=true` 时是 token delta，不持久化但发给 client；`Partial=false` 是最终 event，持久化到 session
- 内部通过 channel + goroutine 实现，对外是 Go 1.23 iterator

### 启发

**lynx 应该补一个 streaming API**——这是 ADK 在 Go 标准库（`iter`）上更新的设计，对 server 集成是质变。提议：

```go
// 提议：runtime/stream.go
func (p *Platform) RunAgentStream(
    ctx context.Context,
    agentDef *core.Agent,
    bindings map[string]any,
    options core.ProcessOptions,
) iter.Seq2[event.Event, error]
```

实现细节：
1. 内部 `chan event.Event` + 一个临时 listener 把所有 event 发进 channel
2. 启动 goroutine 跑 `RunAgent`，process 终止时 close channel
3. 返回 `iter.Seq2` 包装这个 channel

Partial event 的概念可后置——lynx 当前 chat 中间件已有 streaming 能力，但 agent runtime 层没有"半成品 event"概念；如果未来加 token-level event，再引入 `Partial` flag。

> ROI：高。HTTP/SSE 集成立刻受益。约 80-120 LOC + 测试。**P1 候选**。

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

### 启发

**适度借鉴**——lynx 应该有：

1. **`runtime/http`**（轻量 SSE server）—— 拿 `RunAgentStream` 的 iterator 包成 SSE。约 100 LOC。一旦 streaming API 落地这是**几乎免费**的。
2. ~~A2A 协议~~ —— A2A 是 Google 推的 spec，生态还没起来；lynx 已有 MCP，先观察。
3. ~~Vertex AI managed runtime~~ —— 厂商绑定，不做。

> ROI：中-高（条件：先做 streaming API）。

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
| Tool 级 HITL | ✅ `hitl.WithAwaiting / WithConfirmation / RequireType[T]`（typed） | ✅ `tool.RequestConfirmation()` → adk_request_confirmation event（dynamic） |
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

按 ROI 排序的 **ADK 启发候选**：

| 优先级 | 项目 | 借鉴自 ADK | 改动量 | 备注 |
|---|---|---|---|---|
| **P1** | **`Platform.RunAgentStream() iter.Seq2[event.Event, error]`** | iter-based runner.Run | ~80 LOC + 测试 | 解锁所有 server 集成；Go 1.23 iter，纯叠加 |
| **P1** | **`core.Session` + `SessionStore`** + 让 process 关联 session | SessionService 三件套之一 | ~200 LOC + 测试 | 让"多 turn chat" 成为 framework 顶层概念 |
| **P2** | **轻量 SSE server** `runtime/http`（条件：streaming API 已落地） | adkrest 简化版 | ~100 LOC | streaming API 落地后近乎免费 |
| **P2** | **Agent 级 workflow agents**（`workflow.SequentialAgents` / `ParallelAgents` / `LoopAgents`） | workflowagents | ~150 LOC + 测试 | 与现有 action 级 workflow 互补 |
| **P3** | **`core.MemoryService` SPI**（不内置实现） | MemoryService | ~80 LOC | 跨 session 语义记忆；用户自挂 RAG |
| **P3** | **`core.ArtifactStore` SPI** + `core.PromptTemplate` 模板渲染 | ArtifactService + Instruction template | ~100 LOC each | 多模态/模板场景 |
| **不做** | YAML agent 定义 | configurable | — | 分歧；lynx typed DSL 是核心价值 |
| **不做** | Plugin callback model 替换 Extension | plugin/Plugin | — | 现有 9 capability 表达力已够 |
| **不做** | A2A 协议 | adka2a | — | 协议生态未成熟；MCP 已能覆盖 |
| **不做** | LLM-as-decision (放弃 planner) | 整个 ADK 范式 | — | 这是 lynx 核心；放弃即换框架 |

### 顺序建议

1. **先做 streaming API**（P1 #1）—— 最低成本最高 ROI；之后所有 server-shaped 集成都受益
2. **再做 Session 抽象**（P1 #2）—— 真实工程缺口；让 lynx 能直接做 chat backend 而非"一次性 task runner"
3. **看用例决定** P2/P3 —— Memory / Artifact / agent-level workflow / SSE server 都按需
4. **embabel 那边的 P0 三件**（[EMBABEL_GAP_ANALYSIS.md §15.5](./EMBABEL_GAP_ANALYSIS.md)：`Goal.Export` / `PromptCondition` / `AchievableGoalsToolGroupFactory`）和这边正交，可并行

---

## 12. 一句话总结

ADK 是为**LLM-driven chat backend 工程化**优化的 runtime；lynx 是为**planner-driven agent 行为可推理**优化的 framework。两者**不是替代关系，是互补**。

ADK 给 lynx 的最大启发不是"换范式"——是**让 lynx 学会做长程对话产品的 plumbing**：streaming iterator、Session 抽象、SSE server。这些不冲突 lynx 的 GOAP 内核，只是把 lynx 从"一次性 task runner"扩成"也能做 chat product 的 runtime"。

P0/P1 候选已记入路线图；具体落地时机看真实用例驱动。
