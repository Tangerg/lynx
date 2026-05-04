# lynx/agent vs embabel-agent — 深度对比与缺口分析

> 第三轮深度对比。基线：embabel-agent (Kotlin/Spring) v0.4 / lynx/agent (Go) HEAD。
> 配套文档：[`doc/agent/08-vs-embabel.md`](../doc/agent/08-vs-embabel.md)（首轮架构对比），本文档聚焦**缺口与下一步**。

---

## 0. TL;DR

| 层 | 状态 |
|---|---|
| **核心规划/执行** | **已追平 embabel 0.4** —— GOAP 后处理、subtree budget、TerminationScope 三粒度、JSON 事件、HITL 路由、ToolCallContext 等 embabel-parity TODO 全部落地 |
| **生态集成深度** | **显著落后** —— embabel 是 "batteries-included"（11 LLM starter、完整 RAG、Shell、MCP server、A2A、Supervisor），lynx 是 "minimal kernel + BYO" |
| **下一步 ROI 最高** | (1) MCP server 端、(2) ToolCall loop runner、(3) Supervisor 模式与示例 |

设计取舍合理，**不应抹平**的差异（反射注册、Megazord、Spring DI、SpEL、ONNX 等）见 §6。

---

## 1. 模块拓扑

### embabel-agent —— 17 个 Maven 子项目，~720 个 Kotlin 文件

```
embabel-agent/
├── embabel-agent-api/          ← 公共接口（Action/Goal/Condition/Agent/...）
├── embabel-agent-code/         ← 实现（AgentProcess/Planner/Blackboard/...）
├── embabel-agent-common/       ← 工具
├── embabel-agent-domain/       ← 预置 domain 类型
├── embabel-agent-rag/          ← 完整 RAG 栈（Lucene+Neo4j+155 个 .kt）
├── embabel-agent-mcp/          ← MCP server / client
├── embabel-agent-a2a/          ← Agent-to-Agent 协议
├── embabel-agent-shell/        ← 交互式 REPL（14 .kt）
├── embabel-agent-skills/       ← 可复用 skill 包（35 .kt）
├── embabel-agent-observability/← Spring Observation 深度集成
├── embabel-agent-onnx/         ← 本地模型嵌入（embedding / 小 LM）
├── embabel-agent-openai/       ← OpenAI 适配
├── embabel-agent-autoconfigure/← Spring auto-configure × 7 子模块
├── embabel-agent-starters/     ← 11 个 LLM provider Boot starter
├── embabel-agent-test-support/ ← 测试基础设施 + Mockito 集成
├── embabel-agent-dependencies/ ← BOM
└── embabel-agent-docs/         ← 文档
```

### lynx/agent —— 8 个 sub-package，~5,700 LOC Go

```
agent/
├── core/             原语（Action/Goal/Condition/Agent/Blackboard/Awaitable）
├── plan/             WorldState / Plan / Planner 接口
├── planner/goap/     A* GOAP 实现
├── runtime/          Platform / AgentProcess / OODA 循环
├── event/            事件类型 + Multicast Listener
├── dsl/              流式 Builder（唯一入口）
├── hitl/             Awaitable 类型化 / TypedRequest
└── examples/         hello / blog
```

**对应关系**：lynx 的 `core+plan+planner+runtime+event+dsl+hitl` ≈ embabel 的 `embabel-agent-api+embabel-agent-code` 子集。其余 13 个 embabel 模块在 lynx 中**几乎都没有对应物**。

---

## 2. 哲学分歧 —— 合理取舍，不是缺陷

| 维度 | embabel | lynx | 评价 |
|---|---|---|---|
| **编程模型** | `@Agent` / `@Action` 注解扫描 + Kotlin DSL **双轨** | 流式 DSL **唯一入口** | lynx 严格、Go 习惯，删除路径见 commit `53ea5a3` |
| **DI 哲学** | Spring Bean + `@Autowired` 构造注入 | `PlatformConfig` 直接初始化 | lynx 显式 |
| **LLM 抽象** | `ChatClient` 钉死 Spring AI | `ServiceProvider[string]any` 开放注册 | lynx 零生态绑 |
| **多输入** | Megazord 反射聚合（黑板自动拼装多参对象） | `core.Get[T](pc.Blackboard, name)` 显式 | lynx 数据流可 `grep` |
| **控制流** | 异常（`TerminateActionException` / `ReplanRequestedException`） | error + signal channel | Go 惯例 |
| **状态机** | `@State` sealed class 解构 + 自动 flatten action | 用户写 `Goal.Pre` / `Action.Pre/Post` | Go 无 sealed |
| **Cost/Value** | `(static, fn)` 双字段 + 优先级规则 | 单 `CostFunc` 字段 + `core.Static(v)` helper | lynx 设计更简单 |
| **HITL** | 一个 `Awaitable<P, R>` 泛型 + 多个具体类 | `core.Awaitable` 非泛型 + `hitl.TypedRequest[P, R]` | lynx 二层分明 |
| **Observability** | Spring Observation + 13 种 span 类型 + parent 自动 resolution | 直接 OTel `trace.Tracer` + 手动 instrument | embabel 更 enterprise |

---

## 3. 概念对齐情况 —— lynx 已做到

下述 embabel 0.4 的核心概念，lynx **均已实现且语义等价**：

| 概念 | embabel 位置 | lynx 位置 |
|---|---|---|
| 三值逻辑 `Determination` | `api/Condition.kt: ConditionDetermination` | `core/determination.go: Determination` |
| GOAP A\* 搜索 | `core/AStarGoapPlanner.kt` | `planner/goap/astar.go` |
| 后向 + 前向 plan 优化 | `AStarGoapPlanner.kt` | `astar.go: backwardOptimize / forwardOptimize` |
| Goal 可达性预检 | implicit in planner | `astar.go: goalReachable`（短路 10k 迭代） |
| WorldState / Apply / HashKey | `api/WorldState.kt` + impl | `plan/condition_world_state.go`（HashKey 在构造时算好，规避并发竞态） |
| Action 输入/输出/precondition/effect | `api/Action.kt` | `core/action.go` + `core/action_typed.go` |
| 自动 hasRun_<name> 防重入 | `core/Action.kt` | `core/action_typed.go: computePreconditionsAndEffects` |
| OODA 循环 | `core/SimpleAgentProcess.kt: tick()/run()` | `runtime/run.go: tick(ctx)/run(ctx)` |
| Blackboard 双绑定（"it" + 类型派生名） | `core/InMemoryBlackboard.kt: bind` | `runtime/in_memory_blackboard.go: Bind` |
| Subtree budget 汇总 | `core/AgentProcess.kt` 递归 child | `runtime/agent_process.go: Usage()` 递归 child |
| TerminationScope 三粒度 (Agent/Action/ToolCall) | `api/TerminationScope.kt` | `core/tool_group.go: TerminationScope` |
| HITL Awaitable + ResponseImpact | `api/Awaitable.kt` + `FormBindingRequest` | `core/awaitable.go` + `hitl/TypedRequest[P,R]` + `NewConfirmation` |
| HITL resume 真实路由 | `core/AgentProcess.resume()` | `runtime/platform.go: ResumeProcess` 走 `OnResponseAny`（commit `445c23d` 修了之前的死管道） |
| StuckHandler 替代逻辑 | `api/StuckHandler.kt` | `core/stuck.go: StuckHandler` + `StuckCode (Replan/NoResolution)` |
| EarlyTerminationPolicy + Composite | `api/EarlyTerminationPolicy.kt` | `core/early_termination.go: MaxActionsPolicy / BudgetPolicy / CompositePolicy` |
| 事件 JSON 序列化 + discriminator | Jackson `@JsonTypeInfo(SIMPLE_NAME)` | `event/event.go: emit + envelope.Event` |
| ToolGroup + Resolver | `tools/*` | `core/tool_group.go: ToolGroup / ToolGroupResolver / StaticToolGroupResolver / LazyToolGroup` |
| 并发安全 Blackboard | embabel 用 `synchronized` | lynx 用 `sync.RWMutex` + `atomic.Pointer` |
| 启动校验（结构） | `DefaultAgentStructureValidator` | `core/agent.go: ValidateAgent` |
| 启动校验（可达性） | `GoapPathToCompletionValidator` | `runtime/platform.go: checkGoalsReachable`（轻量版本，一步反向扫描） |
| ToolCall 取消上下文 | `DefaultToolLoop` 内部检查 termination | `core/process_context.go: ToolCallContext` + `AgentProcess.TerminateToolCall` |
| Process 父子关系 + ParentID | `AgentProcess.parent` | `runtime/agent_process.go: parentID` + `Platform.CreateChildProcess` |
| Version 元数据（semver） | `AgentMetadata.version` | `core/agent.go: AgentConfig.Version *semver.Version` |
| 平台事件多播 + panic 隔离 | `AgenticEventListener` | `event/event.go: Multicast.OnEvent`（snapshot listeners 跳出 RLock） |

---

## 4. 详细缺口表 —— 按概念

> 仅列**真实缺失**或**深度差异**，不列已对齐的部分。

### 4.1 Agent / AgentMetadata

| embabel 有 | lynx 状态 | 影响 |
|---|---|---|
| `actionRetryPolicy` 表达式动态配置（`@Agent.actionRetryPolicyExpression`） | 无 | per-action 重试只能通过 `ActionConfig.QoS` 静态配 |
| `AgentMetadataReader`（591 LOC，反射扫 `@Agent` bean） | 已删（commit `53ea5a3`） | **N/A** —— 设计取舍 |
| `scan = false` 控制 | 无 | **N/A** |

### 4.2 Action

| embabel 有 | lynx 状态 | 影响 |
|---|---|---|
| Mixin 接口设计：`DataFlowStep + ConditionAction + ActionRunner + DataDictionary + ToolGroupConsumer` | 单一 `Action` 接口 | 内省能力弱（`referencedInputProperties()` / `shortName()` / 子接口分类） |
| 任意函数签名反射解析 | 固定 `func(ctx, *pc, In) (Out, error)` | **N/A** —— Go 泛型已够用 |
| `actionRetryPolicy` 动态表达式 | 无 | 同 4.1 |
| `ToolGroups` 字段在 framework 层自动 resolve | 仅作为 metadata 存储 | **见 4.10** |

### 4.3 Goal

| embabel 有 | lynx 状态 | 影响 |
|---|---|---|
| `Export(Remote, Local, StartingTypes)` 真实消费（MCP / A2A 暴露 goal） | `ExportConfig` 字段已定义但**没有任何代码读它** | MCP 暴露时少了语义信息 |
| `tags` / `examples` 真实消费 | 字段已定义但**没有任何代码读它** | LLM prompt 时少了 few-shot |
| `withPreconditions()` / `withFixedValue()` builder 方法 | 用 `Goal{...}` literal | **N/A** —— Go literal 即可 |

### 4.4 Condition

| embabel 有 | lynx 状态 | 影响 |
|---|---|---|
| `or` 的 cost = `min(a, b)`（短路便宜分支） | `or` 的 cost = `a + b`（与 and 同公式） | **小 bug 等级** —— planner 选择略次优，不影响正确性 |
| `not()` / `inv()` 操作符重载 | 函数 `Not()` | **N/A** —— Go 无操作符重载 |

### 4.5 WorldState

无显著差异。lynx 的 HashKey 并发安全已修复（commit `445c23d`）。

### 4.6 Plan / Planner

| embabel 有 | lynx 状态 | 影响 |
|---|---|---|
| **Utility planner**（reward-based） | 仅 GOAP | reward-based 场景需用户实现 `plan.Planner` |
| **SupervisorAgent planner**（LLM-driven agent selection） | 仅 GOAP | 多 agent 编排无开箱方案 |
| `ProcessOptions.PlannerType` 切换 | 字段存在但 `defaultPlannerFactory` 总返回 GOAP | 切换 Utility/Supervisor 不工作 |
| `Planner.PlansToGoals` / `PlanToGoal` 公共暴露 | 仅 `BestValuePlan`（其他在 AStarPlanner 私有） | 调试 / "show all plans" UI 需要走私有路径 |

### 4.7 AgentProcess / 运行时

| embabel 有 | lynx 状态 | 影响 |
|---|---|---|
| 异常驱动控制流 | error + signal channel | **N/A** —— Go 惯例 |
| 预定义 service 槽位（`ChatClient` / `RagService` / `ToolGroupResolver`） | 开放 `ServiceProvider[string]any` + `ServiceOf[T]` | lynx 灵活但需手动转型 |
| `agent.process.id` baggage 自动传播 | 手动通过 `core.WithProcess(ctx, p)` | **MEDIUM** —— 子 ctx / goroutine 容易丢上下文 |

### 4.8 Blackboard

| embabel 有 | lynx 状态 | 影响 |
|---|---|---|
| **Megazord 多输入聚合**（`getValue()` 反射拼装多参对象） | 故意不做，用户 `Get[T]` 显式取 | **N/A** —— 设计取舍 |
| `expressionEvaluationModel(): Map<String,Any>` 供 SpEL 用 | 无（lynx 不用 SpEL） | **N/A** |
| `getOrPut[V]()` 线程安全 helper | 无 | **LOW** |
| `Bindable` 操作符语法糖 | 无 | **N/A** —— Go 无运算符重载 |
| `MayHaveLastResult` 独立接口 | 合并入 `Blackboard.GetValue("lastResult", ...)` | 等价 |

### 4.9 Tool / ToolGroup / Resolver

| embabel 有 | lynx 状态 | 影响 |
|---|---|---|
| **`DefaultToolLoop` 框架级 tool 循环 runner** | 无 —— 用户 LLM 客户端自己跑 | **HIGH** —— 详见 §5.P0-2 |
| `ToolLoopConfiguration` / `EmptyResponsePolicy` | 无 | 同上 |
| `OneShotPerLoopTool` / `ProgressiveTool` / `PlaybookTool` 高级模式 | 无 | tool 模式定制困难 |
| MCP resolver 内置 | `lynx/mcp` 在 agent 包外，**仅客户端** | **CRITICAL** —— 详见 §5.P0-1 |
| Spring AI tool callback 桥 | **N/A** —— Go 无 Spring AI |

### 4.10 HITL / Awaitable

| embabel 有 | lynx 状态 | 影响 |
|---|---|---|
| `UserInputRequest` 类型（自由文本输入） | 仅 `TypedRequest[P, R]` 通用 + `NewConfirmation` 特化 | 用户可用 `NewTypedRequest[P, string]` 覆盖，但少便利 |
| `Approver` / `Generator` pattern 等扩展 | 无 | **LOW** |

### 4.11 Event 系统

| embabel 有 | lynx 状态 | 影响 |
|---|---|---|
| `LlmInvocation` 内置 cost 计算（`pricingModel.costOf(usage)`） | 推给集成层，用户调 `pc.RecordUsage(cost, tokens)` | **N/A** —— lynx 不绑价格表 |
| `AgenticEventListener` 三 callback：`onProcessEvent / onPlatformEvent / onLlmEvent` | 单一 `OnEvent(Event)` | **LOW** —— 路由由 listener 自己 switch |
| Spring Observation 深度集成（13 span 类型 + parent 自动 resolve + metrics 桥） | 直接 OTel | **MEDIUM** —— 详见 §5.P1-2 |

### 4.12 ProcessOptions

| embabel 有 | lynx 状态 | 影响 |
|---|---|---|
| `EarlyTerminationPolicy.firstOf(a, b, c)` 流式构造 | `CompositePolicy{Policies: ...}` | **LOW** —— 美学差异 |
| 多个 LLM provider 内置 budget tier | 无 | **N/A** |

### 4.13 启动校验

| embabel 有 | lynx 状态 | 影响 |
|---|---|---|
| 3 层 validator pipeline（`AgentValidationManager`） | 2 层（结构 `ValidateAgent` + 一步反向 `checkGoalsReachable`） | **MEDIUM** —— 详见 §5.P1-1 |
| `GoapPathToCompletionValidator` 真跑 GOAP | 一步反向扫描（避免假阴性，权衡合理） | 设计有意为之 |
| 方法签名 / type binding 一致性检查 | 无 | 错误在第一次 tick 才发现 |

---

## 5. 缺口优先级 Roadmap

### P0 —— 真实项目阻塞，3 个月内

#### P0-1：MCP Server 暴露端

**现状**：`lynx/mcp` 仅有客户端框架（`provider.go` 等），**无 server 端**。`Goal.Export.Remote=true` 字段已恢复但无人消费。

**需要做**：
- 新增 `lynx/mcp/server` 子包，参考 embabel `embabel-agent-mcpserver` 但去 Spring 化
- 复用现有 `core.Goal.Export` / `ActionMetadata.ToolGroups` / `Action.Description` 自动生成 MCP schema
- 暴露 lynx agent 作为 MCP resource server，让 Claude / IDE / 其他 MCP client 可以调用 lynx agent

**为什么 P0**：MCP 是当前 LLM 生态的事实集成协议（Anthropic / OpenAI / Claude / Cursor 全在用）。lynx agent 不能被外部调用就只是孤立的库。

**embabel 参考**：`embabel-agent-mcp/embabel-agent-mcpserver/src/main/kotlin/.../McpServerConfiguration.kt`

#### P0-2：ToolCall Loop Runner

**现状**：
- `core.ProcessContext.ToolCallContext` 已就位（提供 cancel ctx）
- `runtime.AgentProcess.TerminateToolCall` 已就位（`atomic.Pointer[CancelFunc]` 触发）
- **但缺一个真正用它的 tool-loop runner** —— 当前 lynx 把 tool 调用循环全推给用户的 LLM 客户端

**需要做**：决定方向，二选一：
- **方向 A**（embabel-style）：在 `agent/toolloop` 子包提供框架级 tool 循环 runner，循环调 LLM tool calls 时检查 termination signal，自动 cancel in-flight
- **方向 B**（minimal）：明确文档化「tool loop 是用户责任」，并提供示例代码模板，告诉用户如何在他们的 LLM client 里桥接 `ToolCallContext`

**为什么 P0**：你已经把取消管道铺好（CancelFunc + TerminationScopeToolCall），但没有任何 lynx 内部代码真正使用它 —— 这是个**半截功能**，要么完成要么删。

**embabel 参考**：`embabel-agent-code/src/main/kotlin/.../DefaultToolLoop.kt`

#### P0-3：SupervisorAgent 模式 + 官方示例

**现状**：`Platform.CreateChildProcess` 已经存在，理论上可以用它组合多 agent。但：
- 没有官方推荐模式
- 没有示例展示「LLM-driven agent selector」如何写
- 用户每次都得自己琢磨

**需要做**：
- 在 `agent/examples/supervisor` 加一个示例：上层 agent 用 LLM 选择下层 sub-agent
- 抽出常见模式到 `agent/orchestration` 子包：goal-routing、capability-matching、parallel-fanout
- **不要**抄 embabel 的反射魔法 —— 只需文档 + 范式

**为什么 P0**：多 agent 编排是 embabel 0.4 的杀手特性，也是 LLM 应用最常见的形态（router → specialist → aggregator）。lynx 现在用户得自己琢磨。

**embabel 参考**：`embabel-agent-api/src/main/kotlin/.../SupervisorAgentFactory.kt`（200+ LOC）

---

### P1 —— 提升体验，6 个月内

#### P1-1：启动校验深度增强

**目标**：在 `Platform.Deploy` 时增加更多结构性检查（**保持快速**，不要跑全 GOAP）。

**新增检查**：
1. ToolGroup role 是否有 resolver
2. Action input type 是否与上游 Action / Goal output type 兼容
3. Goal 的 effect key 是否拼写错误（与 known conditions 对照）
4. Condition.Name 唯一性
5. StuckHandler 与 EarlyTerminationPolicy 不冲突

**实现位置**：扩展 `core.ValidateAgent` 或新增 `runtime.checkAgentWiring`。

#### P1-2：可观测性 enterprise 化

**不要**引入 Spring Observation（JVM-only）。可以做：

- **自动 span parenting**：当前 lynx 每个 `lynx.agent.tick` / `lynx.agent.action` span 都从 ctx 拿 parent，但没有强制 hierarchy。引入 span 类型注册表（process / goal / action / tool-call / llm-call）
- **Metrics bridge**：直接发 OTel metrics（counter/gauge/histogram）而非全靠 traces。Plan / Action / Goal 各自暴露 latency histogram、success rate counter
- **用户自定义 span 类型系统**：让用户代码用统一 API 报 LLM-call、RAG-query 等 span（隐藏 OTel API），与 lynx 自己的 span attributes 对齐

**embabel 参考**：`embabel-agent-observability/src/main/java/.../EmbabelTracingObservationHandler.java`

#### P1-3：消费 Goal.Export / Tags / Examples 字段

**现状**：`Goal.Export`、`Goal.Tags`、`Goal.Examples` 字段已恢复但**没有任何代码读它们**。

**需要做**（与 P0-1 联动）：
- MCP server 端读 `Export.Remote=true` 决定是否暴露该 goal
- LLM prompt 模板（如果 lynx 提供）读 `Tags` / `Examples` 做 few-shot

否则这些字段就是死字段，应该删。

#### P1-4：Condition.Or 的 cost 修正

**现状**：`agent/core/condition.go: orCondition.Cost() = c.left.Cost() + c.right.Cost()`，与 and 公式相同。

**修正**：embabel 的 `or` 用 `min(a, b)`（短路便宜分支）。一行修改：

```go
func (c *orCondition) Cost() float64 {
    return math.Min(c.left.Cost(), c.right.Cost())
}
```

**影响**：planner 在 evaluate 复合 condition 时选择更优分支。

---

### P2 —— 生态，9-12 个月

| 项 | 说明 | embabel 对应 |
|---|---|---|
| **RAG 后端落地** | `lynx/core/rag` 接口齐全，缺 vectorstore 的 Chroma/Weaviate/Pinecone 驱动 | `embabel-agent-rag`（155 .kt） |
| **Shell / REPL** | Go 版 ~500 LOC，开发/演示交互式 agent 调试 | `embabel-agent-shell` |
| **Skill 库** | `agent/skills/` 下的可复用 action 模板（HTTP、文件、文本） | `embabel-agent-skills`（35 .kt） |
| **A2A 标准协议** | gRPC / HTTP 上的 agent 间通信，让分布式 agent 网络写起来简单 | `embabel-agent-a2a`（19 .kt） |
| **Test support** | `agent/agenttest` 提供 mock blackboard、stub planner、record/replay process | `embabel-agent-test-support` |
| **`UserInputRequest` 便利类型** | `hitl.NewUserInput[P](payload, handler func(string) ResponseImpact)` | `api/UserInputRequest.kt` |
| **Plan Show-All UI** | 重新暴露 `Planner.PlansToGoals` 给调试工具 | `Planner.PlansToGoals` |

---

## 6. 不应抹平的设计差异（**不要做**）

下列是 embabel 的特性，但**不应**移植到 lynx：

| 特性 | 原因 |
|---|---|
| `@Agent` / `@Action` 反射注解扫描 | Go 社区共识「显式优于隐式」；注解扫描推迟错误到运行时、打破 IDE refactor、命名约定成隐藏契约。已在 commit `53ea5a3` 删除 |
| Megazord 自动多输入聚合（黑板反射拼装多参对象） | 违反 Go「显式数据流」哲学。多输入 action 用 `core.Get[T]` 显式取，所有数据流可 grep |
| Spring DI / `@Autowired` 构造注入 | Go 无标准 DI 容器。`PlatformConfig` 已经够用 |
| `@State` sealed class 解构 + 自动 flatten action | Go 没有 sealed class。用户显式写 `Goal.Pre` / `Action.Pre/Post` 替代 |
| `expressionEvaluationModel()` / SpEL 模板 | lynx 不绑定模板引擎；prompt 渲染交给用户的 LLM client 库 |
| 内置 `pricingModel.costOf(usage)` | 让框架知道每个 model 的 USD/token 价格不可维护（价格变化频繁、各厂商不一致） |
| ONNX 本地模型嵌入（`embabel-agent-onnx`） | 与 agent 框架职责无关。用户在 `Services` 注册任意 embedding/LLM client 即可 |
| 11 个 LLM provider Spring Boot starter | Go 无 Boot starter 机制。用户 `services.Set("chat", openaiClient)` 一行搞定 |
| Spring Observation 13 种 span 类型 + ObservationHandler 注册表 | JVM-only。OTel 直接 API 已能实现等效效果（见 P1-2） |
| 异常驱动控制流（`TerminateActionException`） | Go 不用 panic 控制流。signal channel + error 是 Go 惯例 |

---

## 7. 关键架构对比矩阵

| 维度 | embabel | lynx | 谁更好 |
|---|---|---|---|
| **核心 LOC** | ~30k+ Kotlin（主体） | ~5,700 Go | lynx 更紧凑 |
| **依赖图** | Spring Boot 全家桶 + Spring AI + Jackson + ... | `Masterminds/semver`、`google/uuid`、`golang.org/x/sync` + OTel | lynx 更小 |
| **类型安全（动作输入）** | Kotlin reflection + 多参 Megazord | Go 泛型 `NewAction[In, Out]` | lynx 编译期保证更强 |
| **Plan 多样性** | GOAP / Utility / Supervisor 三种 | 仅 GOAP（其他 pluggable） | embabel 更丰富 |
| **多 agent 编排** | SupervisorAgent 内置 + a2a 协议 | 用户用 `CreateChildProcess` 自组合 | embabel 更现成 |
| **可观测性** | Spring Observation 多 span 类型 | OTel 单 tracer | embabel 更深 |
| **MCP 暴露** | `embabel-agent-mcpserver` 完整 | **缺 server 端** | embabel 完胜 |
| **RAG** | Lucene + Neo4j 完整栈 | 接口齐全无后端 | embabel 完胜 |
| **HITL 真实可用** | ✅ | ✅（之前死管道，commit `445c23d` 修了） | 持平 |
| **GOAP 后处理** | backward + forward optimize | backward + forward optimize | 持平 |
| **subtree budget** | 父子递归汇总 | `Usage()` 父子递归 | 持平 |
| **TerminationScope 三粒度** | Agent / Action / TOOLCALL | Agent / Action / ToolCall | 接口持平，runner 缺（见 P0-2） |
| **HashKey 并发安全** | 隐含（synchronized） | 显式（构造时算好） | lynx 更明确 |
| **JSON 序列化事件** | Jackson `@JsonTypeInfo` | 21 events 显式 `MarshalJSON` + envelope | 持平（不同实现） |
| **静态/动态成本字段** | `(static, fn)` 双字段 + 优先级规则 | 单 `CostFunc` + `core.Static(v)` helper | lynx 更简单（最近收敛） |
| **ConfirmationRequest / FormBindingRequest 区分** | 两个独立类（重复 boilerplate） | `TypedRequest[P, R]` + `NewConfirmation` 工厂 | lynx 更紧凑（最近收敛） |
| **核心 API surface** | ~720 .kt 文件，大量 mixin | 17 ~ 30 个核心接口/类型 | lynx 更小 |
| **学习曲线** | 入门快（注解 + Boot starter）/ 进阶慢（Spring 生态） | 入门慢（要懂 GOAP）/ 进阶快（核心紧凑） | 各擅胜场 |

---

## 8. 核心结论

### lynx 已做对的事
1. **核心规划/执行层完全追平 embabel 0.4** —— GOAP 后处理、subtree budget、TerminationScope ToolCall、JSON 事件、HITL 真实路由、Version 元数据、ToolCallContext
2. **删掉了 embabel 不该有的复杂度** —— 反射注册、Megazord、双轨 DSL/注解、SpEL、内置 pricing model
3. **应用了正确的简化模式**：
   - `static + fn` 双字段 → 单 `CostFunc` + `Static()` helper（用户提议）
   - `Register/Clear` 双 fn → 单 `ToolCallCanceller(cancel) func()`（释放闭包）
   - `ConfirmationRequest + FormBindingRequest` → `TypedRequest[P, R]` + `NewConfirmation` 工厂
   - ad-hoc `proc.(interface{Usage()...})` 类型断言 → `Process.Usage()` 接口方法
4. **保住了扩展性** —— `Verbosity` / `Identities` / `ProcessControl` / `BindAll` / `Hide` / `BindProtected` / `MaxActionsPolicy` / `Writer/ChannelOutputChannel` / `LazyToolGroup` / `StaticToolGroupResolver` / type aliases 等都还在，未来可用

### 真正的差距
**生态集成深度**，而非核心抽象：
- embabel "open the box and run"：11 LLM starter、完整 RAG、Shell、MCP server、A2A、Supervisor……
- lynx "minimal kernel + bring your own"：专家友好但新手要自己组装 RAG / MCP server / tool loop

### 接下来 ROI 最高的 3 件事
1. **P0-1：MCP server 端** —— 连通 Claude / IDE 生态，是 lynx agent 走出实验阶段的前提
2. **P0-2：ToolCall loop runner**（或明确 BYO 文档） —— 你已经把 cancel 管道铺好但少了一个真正用它的 runner，半截功能
3. **P0-3：Supervisor 模式 + 示例** —— 多 agent 编排是 LLM 应用最常见的形态，光靠 `CreateChildProcess` API 不够

P1（启动校验、可观测性、Goal 字段消费、Condition.Or 修正）后续 6 个月内做。

P2（RAG 后端、Shell、Skill 库、A2A、Test 支持）9-12 个月内做。

embabel 的 Spring/JVM 特性（Observation 全套、autoconfigure、ONNX、@Agent 注解）**不要追** —— 那是另一个生态。

---

## 9. 附：embabel 参考文件速查

| 主题 | embabel 文件 |
|---|---|
| Agent metadata 反射 | `embabel-agent-api/src/main/kotlin/com/embabel/agent/api/AgentMetadata.kt` + `AgentMetadataReader.kt`（591 LOC） |
| AgentScopeBuilder DSL | `embabel-agent-api/src/main/kotlin/com/embabel/agent/api/AgentScopeBuilder.kt` |
| GOAP planner | `embabel-agent-code/src/main/kotlin/com/embabel/agent/core/AStarGoapPlanner.kt` |
| AgentProcess | `embabel-agent-code/src/main/kotlin/com/embabel/agent/core/SimpleAgentProcess.kt` |
| Blackboard + Megazord | `embabel-agent-code/src/main/kotlin/com/embabel/agent/core/InMemoryBlackboard.kt`（注意 lines 339-393 的反射 aggregation） |
| Tool loop | `embabel-agent-code/src/main/kotlin/com/embabel/agent/core/DefaultToolLoop.kt` |
| Validation manager | `embabel-agent-code/src/main/kotlin/com/embabel/agent/core/DefaultAgentValidationManager.kt` |
| Goap path-to-completion | `embabel-agent-code/src/main/kotlin/com/embabel/agent/core/GoapPathToCompletionValidator.kt` |
| SupervisorAgent | `embabel-agent-api/src/main/kotlin/com/embabel/agent/api/SupervisorAgentFactory.kt` |
| Awaitable | `embabel-agent-api/src/main/kotlin/com/embabel/agent/api/Awaitable.kt` |
| Observability handler | `embabel-agent-observability/src/main/java/com/embabel/agent/observability/EmbabelTracingObservationHandler.java` |
| MCP server | `embabel-agent-mcp/embabel-agent-mcpserver/src/main/kotlin/com/embabel/agent/mcp/server/McpServerConfiguration.kt` |
| A2A handler | `embabel-agent-a2a/src/main/kotlin/com/embabel/agent/a2a/A2ARequestHandler.kt` |
| RAG pipeline | `embabel-agent-rag/src/main/kotlin/com/embabel/agent/rag/` |

---

> **更新节奏建议**：每实现一项 P0 后回看本文档，移到「已对齐」节，或更新缺口表。
> embabel 自身在演进中（0.5 路线图未公开），但本文档基于 embabel 0.4 编写，与 lynx 当前的 embabel-parity 提交一致。
