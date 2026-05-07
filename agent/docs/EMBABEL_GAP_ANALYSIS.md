# lynx/agent vs embabel-agent — 深度对比与缺口分析

> **第四轮深度对比**（2026-05-09 重写）。基线：embabel-agent (Kotlin/Spring) v0.4 / lynx/agent (Go) HEAD（含 Extension 系统已落地、refactor passes 全部完成）。
>
> 配套文档：[`./notes/08-vs-embabel.md`](./notes/08-vs-embabel.md)（首轮架构对比，已部分过时）/ [`./EXTENSION_DESIGN.md`](./EXTENSION_DESIGN.md)（扩展模型设计）/ [`./REFACTOR_PLAN.md`](./REFACTOR_PLAN.md)。
>
> 之前的 1-3 轮分析报告已忽略；本文从零开始，对照实际代码现状写。

---

## 0. TL;DR

| 维度 | lynx/agent | embabel-agent | 差距 |
|---|---|---|---|
| **核心抽象**（Agent / Goal / Action / Condition / Blackboard）| ✅ 完整 | ✅ 完整 | **无差距**——已追平 |
| **GOAP planner** | ✅ A* + 后处理 + reachability check | ✅ A* + HTN + Reactive | HTN/Reactive 未做（路线图）|
| **OODA tick loop** | ✅ Sequential + Concurrent | ✅ Simple + Concurrent | **无差距** |
| **Retry / QoS** | ✅ 委托 pkg/retry | ✅ ActionRetryPolicy | **无差距**，等价 |
| **HITL** | ✅ TypedRequest + ResumeProcess + ContinueProcess | ✅ Awaitable<P,R> + AwaitableFactory + AwaitDecider | 路径对齐；embabel 多 AwaitDecider 自动决策 |
| **Extension 模型** | ✅ http.Pusher 风格，9 capability + 单一 PlatformConfig.Extensions | ✅ 13+ `fun interface` SPI（散在 spi/ 多处）| **lynx 更整洁**；embabel 更广（含 LLM ops 相关）|
| **事件 / 可观测** | ✅ event.Multicast + 16 event 类型 + JSON marshaler + OTel | ✅ AgenticEventListener + Micrometer | **无差距** |
| **Tool 模型** | ✅ AgentTool / ToolGroup / Resolver / Decorator | ✅ Tool / ToolObject / ToolDecorator + ToolLoop | **lynx 缺 ToolLoop**（LLM 驱动循环） |
| **LLM ops** | ❌ 完全没做（设计上不在 agent 层托管，靠 ServiceProvider 注入） | ✅ LlmService + LlmMessageSender + Streaming + 多 provider | **故意分层**——不算缺口，但 embabel 自带 |
| **MCP server-side**（导出 goal 为 tool）| ❌ 未做 | ✅ McpToolExport + Per-Goal Tool Publisher | **真缺口** |
| **A2A 协议** | ❌ 未做 | ✅ JSON-RPC server | **真缺口** |
| **RAG** | ❌ 未做 | ✅ 独立 module（pipeline + Lucene/Neo4j）| **真缺口** |
| **注解/反射注册** | ❌ 不打算做 | ✅ @Agent / @Action / @AchievesGoal / @LlmTool | **故意分歧**——Go 风格用 DSL/builder |
| **Supervisor / Megazord agent**（LLM 编排子 agent）| ❌ 未做 | ✅ SupervisorAgentFactory | **真缺口** |
| **Skills / chatbot loop / WorkflowBuilder** | ❌ 未做 | ✅ embabel-agent-skills + workflow/loop/ | **真缺口** |
| **持久化** | ❌ in-memory only | ✅ ContextRepository / AgentProcessRepository | **路线图项** |

**一句话**：lynx 的 **agent 内核**已经完整，差距集中在 **LLM-driven 循环（ToolLoop）**和**生态集成**（MCP / A2A / RAG / 注解扫描 / Supervisor 模式）。设计哲学不同——lynx 是 "minimal kernel + BYO integrations"，embabel 是 "batteries-included Spring stack"。

---

## 1. 核心抽象 — 已追平

| 抽象 | lynx | embabel |
|---|---|---|
| Agent（包含 actions / goals / conditions） | `core.Agent` | `core.Agent` |
| Action（带 metadata + 重试策略 + tool requirements） | `core.Action` + `core.ActionConfig` + `core.ActionMetadata` | `Action` interface + `AbstractAction` |
| Goal（带 preconditions + cost / value） | `core.Goal` + `core.GoalProducing[T]` | `Goal` data class |
| Condition（条件求值 + And/Or/Not 组合） | `core.Condition` + `ComputedCondition` + composition funcs | `Condition` interface |
| Blackboard（dual-binding "it" + objects + protected） | `core.Blackboard` + `Reader` / `Writer` 拆分 | `Blackboard` interface |
| WorldState（planner 视角的 state snapshot） | `core.WorldState` + `plan.ConditionWorldState` | `WorldState` |
| EffectSpec（preconditions / effects 的 map type） | `core.EffectSpec` | `Effects` type alias |
| IOBinding（"name:type" 槽位） | `core.IOBinding` | `IoBinding` |
| Determination（3 值逻辑） | `core.Determination` | `Determination` |
| ActionStatus / AgentProcessStatus（状态机） | `core.ActionStatus` + `AgentProcessStatus` | `ActionStatus` + `AgentProcessStatusCode` |
| ProcessContext（action 看到的执行上下文） | `core.ProcessContext` | `ProcessContext` |
| ServiceProvider（开放注册表）| `core.ServiceProvider` | Spring DI（更重） |

**结论**：核心模型 **1:1 对齐**，命名也基本一致。lynx 的 ProcessContext 比 embabel 的稍窄（embabel 把 ChatClient 等也塞进 context；lynx 把这些丢到 ServiceProvider，分层更纯）。

---

## 2. Planner — 已追平 GOAP；HTN 未做

| 维度 | lynx | embabel |
|---|---|---|
| A* GOAP | ✅ `plan/planner/goap/astar.go` 510 LOC | ✅ DefaultPlannerFactory |
| Reachability pre-check | ✅ | ✅ |
| Specificity-first action expansion | ✅ | ✅ |
| Backward + forward optimization | ✅ | ✅ |
| Excluded actions（防 replan loop）| ✅ `PlanOptions.ExcludedActions` | ✅ |
| Iteration cap | ✅ `defaultMaxIterations = 10_000` | ✅ |
| HTN planner | ❌ 路线图 | ✅ |
| Reactive planner | ❌ 路线图 | ✅ |
| Plan post-processing（dedupe / reorder） | ✅ | ✅ |
| Goal ranking / 多 plan 排序 | ✅ `BestValuePlan` 按 NetValue | ✅ + `LlmRanker` 做 ranking |

**结论**：lynx 的 GOAP 跟 embabel 等价；embabel 多 HTN/Reactive（reactive 适合事件驱动场景；HTN 适合分层任务）。lynx 的 PlannerFactory extension 接口允许用户自己挂 HTN/Reactive 实现，框架层面已经留口子。

---

## 3. Tool 模型 — 缺 ToolLoop

| 维度 | lynx | embabel |
|---|---|---|
| Tool 定义（name / description / schema / call） | ✅ `core.AgentTool` | ✅ `Tool.Definition` |
| ToolGroup（按 role 聚合） | ✅ `core.ToolGroup` + `LazyToolGroup` | ✅ `ToolObject` |
| ToolGroupResolver（role → ToolGroup） | ✅ Extension 接口 | ✅ `ToolGroupResolver` |
| ToolGroupRequirement（声明依赖） | ✅ 含 `TerminationScope` | ✅ |
| TerminationScope（agent / action / tool_call 三粒度） | ✅ | ✅ |
| ToolDecorator（包装 tool）| ✅ Extension capability | ✅ `ToolDecorator` SPI |
| **ToolLoop runner（LLM 驱动 tool-call 循环）** | ❌ **未做** | ✅ `ToolLoop` interface + `ToolInjectionStrategy` + `LlmMessageSender` |
| MCP tool integration | ❌ | ✅ |

**ToolLoop 是 lynx 最大的功能缺口**。它的语义：在一个 action 里，让 LLM 反复 call tool → 看 result → 再 call tool... 直到 LLM 决定输出最终结果。这是现代 agent framework 的核心循环。

lynx 现在的设计：把这个责任推给用户的 action body（用户自己拿 ServiceProvider 里的 ChatClient + 调 `pc.ActionTools()` 然后写循环）。**最小内核**哲学下这是合理的，但意味着每个 lynx 用户都要重新发明 ToolLoop。

**建议**：在 lynx 提供一个 **可选的** ToolLoop runner（位于一个新子包 `agent/loop/` 或 `agent/runner/llm/`），不进 agent 内核；用户主动拿来用。embabel 的 `ToolInjectionStrategy` / `ToolLoopFactory` 是好参考。

---

## 4. HITL — 大致追平，缺 AwaitDecider

| 维度 | lynx | embabel |
|---|---|---|
| Typed Awaitable<P, R> | ✅ `hitl.TypedRequest` / `hitl.NewConfirmation` | ✅ `Awaitable<P, R>` |
| 类型化 response 路由 | ✅ `OnResponseAny` 类型断言 | ✅ |
| Suspend → Resume → Continue 闭环 | ✅ `ResumeProcess` + `ContinueProcess` | ✅ `awaitable.complete()` |
| AwaitDecider（自动决策"要不要 await"） | ❌ | ✅ `fun interface AwaitDecider` |
| AwaitableFactory（从 tool input 构造 awaitable） | ❌ | ✅ `AwaitableFactory<I, P>` |
| 持久化 awaitable（process restart 后恢复） | ❌ in-memory only | ✅ context repository |

**结论**：lynx 的 HITL 路径跑通了，但 embabel 的 `AwaitDecider` 让"什么时候触发 HITL"可定制。**优先级低**——大多数 agent 不需要这层。

---

## 5. Extension 模型 — lynx 更整洁

| 维度 | lynx | embabel |
|---|---|---|
| 注册方式 | 单一 `PlatformConfig.Extensions` + `ProcessOptions.Extensions` | Spring `@Bean` + 散在 multiple SPI 接口 |
| 入口数 | **1 个**（`PlatformConfig.Extensions`） | 多个（每个 SPI 一个 inject 点） |
| Capability 数 | **9**（ActionInterceptor / ToolDecorator / AgentValidator / GoalApprover / ToolGroupResolver / IDGenerator / PlannerFactory / BlackboardFactory / EventListener） | **13+** SPI |
| 检测方式 | `core.Extension` + type assertion（http.Pusher 风格） | Spring DI + interface impl |
| Per-process 扩展 | ✅ `ProcessOptions.Extensions` | ✅ `ProcessOptions.listeners` |
| 重复检测 | ✅ panic on dup Name | Spring `@Bean` 级别 |

**lynx 在这里反而比 embabel 更优雅**——单一注册入口 + 类型断言检测能力，比 Spring 的多 SPI bean 收口更紧。**embabel 多的能力** 主要在 LLM 相关：`LlmMessageSender` / `LlmMessageStreamer` / `ToolInjectionStrategy` / `EmbeddingService` —— 这些都是 LLM ops 层的扩展点，lynx 不在 agent 内核做 LLM ops，所以不需要。

详见 [`./EXTENSION_DESIGN.md`](./EXTENSION_DESIGN.md)。

---

## 6. 生态集成 — lynx 几乎全空

| 模块 | lynx | embabel |
|---|---|---|
| LLM provider 整合 | ❌（用户自己挂） | ✅ Spring AI（OpenAI / Mistral / DeepSeek / Google / Anthropic） |
| Embedding 服务 | ❌ | ✅ ONNX 本地 + OpenAI 远端 |
| RAG | ❌ | ✅ 独立 module（pipeline / 多 backend / HyDE / reranking） |
| MCP server-side（暴露 agent goal 给外部 LLM） | ❌ | ✅ 同步+异步两种 |
| MCP client-side（消费外部 MCP server） | 部分（ToolGroupResolver 抽象到位，缺具体 MCP adapter） | ✅ |
| A2A 协议 | ❌ | ✅ JSON-RPC server |
| Skills 注册（YAML/Markdown 加载） | ❌ | ✅ 独立 module |
| Shell / CLI | ❌ | ✅ 独立 module |
| Supervisor agent（LLM 编排子 agent） | ❌ | ✅ |
| Megazord（多 agent 合体）| ❌ | ✅ |
| Chatbot loop / ConversationFactoryProvider | ❌ | ✅ |
| WorkflowBuilder DSL | ❌ | ✅ |
| GoalChoiceApprover（autonomy） | ❌（但 GoalApprover capability 已经有） | ✅ `fun interface` |

**这是真正的"功能广度"差距**。lynx 选了"minimal kernel" 路线，把这些定位为**应用层**而非 agent 框架层。优先级排序：

1. **MCP server-side**（P0）—— 让 lynx agent 能被 Claude / 其他 LLM 客户端消费
2. **Supervisor 模式 + 示例**（P0）—— 多 agent 协作的上手门槛
3. **ToolLoop runner**（P1）—— 给愿意把 LLM ops 进框架的用户一个标准答案
4. **MCP client-side adapter**（P1）—— 配 ToolGroupResolver 实现
5. **WorkflowBuilder / chatbot loop**（P2）—— 应用层模式
6. **A2A / RAG / Skills**（P3）—— 独立子模块路线图

---

## 7. 持久化 / 多进程 — lynx 弱

| 维度 | lynx | embabel |
|---|---|---|
| Process registry | ✅ in-memory + Prune/Remove | ✅ + persistent backends |
| Blackboard | ✅ in-memory + Spawn + Clear + protected | ✅ + Redis / DB backends |
| Context repository（跨 session） | ❌ | ✅ |
| Agent process repository | ❌ in-memory only | ✅ |

**lynx 的设计取舍**：`BlackboardFactory` extension 让用户接 Redis/DB；但 lynx **没提供** out-of-the-box impl。embabel 提供了。**这是 P2 级缺口**——多数 agent 是 in-process 的，持久化是企业级特性。

---

## 8. 注解 / 反射注册 — 故意分歧

| 维度 | lynx | embabel |
|---|---|---|
| 注册方式 | DSL Builder + struct config | `@Agent` / `@Action` 注解 + classpath scan |
| 类型安全 | ✅ 编译期 | ✅ Spring AOT 可校验，运行时反射 |
| 上手成本 | 中 | 低（注解直白） |
| Refactor 友好度 | ✅ IDE rename 安全 | 反射依赖名字 |

**lynx 不会做这个方向**——Go 语言哲学（avoid magic / 显式 over 隐式）、Go 没有 Spring AOT 等。这是设计分歧，不是 bug。

---

## 9. 命名 / API ergonomics 对比

| 维度 | lynx | embabel | 谁更好 |
|---|---|---|---|
| Top-level surface | 5 个 constructor（New / NewAction / NewCondition / GoalProducing / NewPlatform） | Spring beans + DSL 多入口 | **lynx** |
| Config 模式 | struct config + ApplyDefaults | data class + sensible defaults | 等价 |
| 错误风格 | `package.Func: %w` 短句 | Kotlin exceptions | 各有所长 |
| 类型层次 | 浅（基本无嵌套） | 深（多层抽象类）| **lynx** |
| 包数量 | 7 个（core / plan / runtime / event / dsl / hitl / agent）| 12+ Spring modules | **lynx** |
| 学习曲线 | Go 用户：低 | Spring 用户：低；其他：高 | 各自优势 |

---

## 10. 路线图建议（按 ROI 排序）

| 优先级 | 项目 | 理由 | 改动量 |
|---|---|---|---|
| **P0-1** | **MCP server-side**：暴露 agent goals 为 MCP tools | 让 lynx agent 在 Claude/Cursor 等 LLM 客户端中可用——立刻接通生态 | 中（新子包 `agent/mcp/server/`） |
| **P0-2** | **Supervisor agent 示例 + helper**：单个 agent 调度多个子 agent | embabel 的多 agent 模式已经成熟；lynx `Platform.CreateChildProcess` 已有底座，缺一个 supervisor 模式的 sample + helper | 小（example + 一个文件级 helper） |
| **P1-3** | **ToolLoop runner**：可选的 LLM-driven 循环 | 用户愿意把 LLM ops 入框架时有 standard answer | 中（新子包 `agent/loop/`） |
| **P1-4** | **MCP client-side ToolGroupResolver**：消费外部 MCP server | 配合 ToolGroupResolver 已有抽象，只缺具体 impl | 中（新子包 `agent/mcp/client/`） |
| **P2-5** | **AwaitDecider** capability | 自动决定"要不要 HITL"的 hook | 小（多一个 Extension capability） |
| **P2-6** | **持久化 BlackboardFactory** 参考实现（Redis） | 让企业级用户开箱即用 | 中（新子包 `agent/blackboard/redis/`） |
| **P3-7** | **HTN planner** | 增加 PlannerType 选项；对复杂任务有用 | 大（新 planner 实现） |
| **P3-8** | **WorkflowBuilder DSL** | 应用层模式，可后续基于 dsl 包扩展 | 中 |

**建议先做 P0-1 + P0-2**——MCP server 是接通生态的硬通货，supervisor 是用户最常问的"我怎么编排 agent"问题。两个一起落地大概 1-2 周工作量。

---

## 11. 故意不要做的事（不是缺口）

- **注解/反射 agent 注册** — 不符合 Go 心智模型，DSL Builder 已经好用
- **Spring DI 容器** — Go 用户不需要重 DI；ServiceProvider 够了
- **AOT 模型分类 / SpEL** — 过度工程
- **Megazord（多 agent 合体注解）** — 反射特化，做了也没人用
- **Kotlin DSL 的 `apply` / `with` 风格 builder** — Go 风格已经清晰

---

## 12. 一句话总结

lynx 把 agent **内核**做到了和 embabel 等价（甚至 Extension 模型更整洁），但选择了"**生态留给用户**"路线。要让 lynx 真正可用于生产，下一步重点是 **MCP server 和 supervisor 模式**——这两个落地后，lynx 就从"框架库"变成"可部署的 agent runtime"。
