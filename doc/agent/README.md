# Lynx Agent 框架设计（embabel-agent Go 移植）

> 本目录是 [`embabel-agent`](https://github.com/embabel/embabel-agent)（Spring AI 生态的 Java/Kotlin agent 框架）向 Lynx 生态的 Go 移植的完整架构设计。
>
> **核心思想保持一致**：GOAP（Goal-Oriented Action Planning）+ 黑板模式 + OODA 循环。
> **但不做机械翻译**：Java/Kotlin 与 Go 的语言差异充分映射，Go 风格优先。
>
> **基线**：embabel `0.4.0-SNAPSHOT`（含 ToolGroup / Skills / TerminationScope / LlmMessageStreamer / TokenCountEstimator / NestedTool / ToolCallInspector / 流式重构等新抽象）
>
> **实现进度**：本目录仍纯为设计文档，**尚无 Go 代码落地**（`agent/` 顶层 module 未创建）。

---

## 1. 文档导航

| # | 文档 | 主题 |
|---|------|------|
| 01 | [architecture.md](./01-architecture.md) | 整体架构、模块拓扑、Java→Go 差异对照 |
| 02 | [core-abstractions.md](./02-core-abstractions.md) | 核心类型：Agent / Action / Goal / Condition / Blackboard / ToolGroup / Awaitable 等 Go 定义（「为什么」）|
| 03 | [planner-and-runtime.md](./03-planner-and-runtime.md) | GOAP A* 规划器 + AgentProcess 执行引擎 + TerminationScope |
| 04 | [user-api.md](./04-user-api.md) | 三种 Agent 定义风格：DSL / Struct+反射 / 代码生成 |
| 05 | [integration-and-examples.md](./05-integration-and-examples.md) | 与 Lynx `core/` / `mcp/` / `otelbridge/` 的集成 + 完整示例 |
| 06 | [rollout.md](./06-rollout.md) | 分阶段落地计划 |
| 07 | [data-structures.md](./07-data-structures.md) | **核心数据结构目录**（实现蓝图，结构化索引）|

> 上游分析：[`../embabel-architecture-analysis/`](../embabel-architecture-analysis/) 是对外部 embabel-agent 的架构剖析（参考材料）。

---

## 2. 一分钟速览

### 2.1 核心概念对照

```
embabel-agent (Java/Kotlin)           →    Lynx Agent (Go)
────────────────────────────────────────────────────────────
@Agent class                          →    DSL builder / struct + reflection
@Action / @AchievesGoal method        →    agent.New(...).Actions(agent.NewAction("name", func(...)))
@Condition method                     →    agent.New(...).Conditions(agent.NewCondition("name", func(...)))
Spring AI ChatClient                  →    core/model/chat.Client
Micrometer Observation                →    OpenTelemetry API（直接用，见 OBSERVABILITY.md）
AStarGoapPlanner (Kotlin)             →    planner/goap.AStarPlanner (Go)
Blackboard                            →    blackboard.Store
AgentProcess.run() loop               →    AgentProcess.Run(ctx) with goroutines
ThreadLocal AgentProcess.get()        →    context.Context 透传
Reactor Flux streaming                →    iter.Seq2 迭代器
LlmMessageStreamer SPI                →    chat.StreamHandler（Lynx 已天然提供）
TokenCountEstimator<T>                →    core/tokenizer.Tokenizer + 泛型包装
ToolCallInspector SPI                 →    StreamMiddleware + EventListener 组合
Jakarta Validation                    →    go-playground/validator 或自写
```

### 2.2 模块拓扑（计划新增）

```
lynx/
├── core/                 (已有)
│   ├── model/            chat / embedding / image / audio / moderation
│   ├── rag/              RAG pipeline
│   ├── vectorstore/      接口 + filter DSL
│   └── document/         Reader/Writer/Transformer
│
├── mcp/                  (已有 v1) — agent 能直接复用 mcp.Tool / mcp.Provider 接入跨进程工具
│
├── otelbridge/           (已有) — slog/log SpanExporter，agent runtime 埋点接收侧
│
├── agent/                ★ 新增顶层 module — agent 框架核心
│   ├── go.mod
│   ├── core/             Agent / Action / Goal / Condition / Blackboard
│   ├── plan/             Plan / WorldState / 接口
│   ├── planner/
│   │   ├── goap/         A* GOAP 实现（核心）
│   │   └── utility/      效用规划器（开放式任务）
│   ├── runtime/          AgentProcess / 执行引擎 / 事件 / TerminationScope
│   ├── dsl/              流式 builder API（唯一的 Agent 定义入口，显式优于隐式）
│   ├── event/            事件类型 + 多播监听器
│   └── hitl/             Human-in-the-loop（Awaitable[P, R]）
│
└── agents/               ★ 新增顶层 module — 可选扩展（对标 otelbridge/）
    ├── a2a/              Agent-to-Agent 协议
    ├── shell/            REPL shell（参考 embabel-agent-shell）
    └── skills/           外挂 skills（YAML + Docker/Process 执行引擎）
```

> **MCP server 集成不需要 `agents/mcp/`**——Lynx 顶层已有 `mcp/` v1 桥接（`RegisterTools(srv, ...chat.Tool)`），agent 框架直接复用即可。

---

## 3. 关键设计决策

1. **核心抽象 Go 化**：Agent / Action / Goal / Condition 都是 Go 接口或 struct，用 **`context.Context` 替代 ThreadLocal**，用 **泛型顶层函数替代 Kotlin 泛型方法**。

2. **三种 Agent 定义方式**：
   - **DSL（推荐）**：`agent.New(core.AgentMeta{...}).Actions(...).Goals(...)` 流式构建
   - **Struct + 反射**：在 struct 方法上打标签（struct tag 或 marker comment），反射解析
   - **代码生成**（可选）：`//go:generate lynx-agent-gen` 读源码生成注册代码

3. **LLM 调用走 Lynx 自己的 `chat.Client`**：不直接嵌入 Spring AI 概念，`ProcessContext` 提供 `LLM()` 方法返回 `*chat.Client`。

4. **观测直接走 OpenTelemetry**：不建 `core/observation/` 抽象，core 代码就调 `otel.Tracer("lynx/agent")`。每个 tick / plan / action 一个 span，与 Lynx 现有观测策略一致（详见 [`../OBSERVABILITY.md`](../OBSERVABILITY.md)）。

5. **MCP 工具 = `chat.Tool`**：通过 `mcp.Provider.Tools(ctx)` 拿到的远端工具天然就是 Action 体系内的一等公民，不需要 ToolGroup-MCP 适配层。`embabel.ToolGroup` 在 Lynx 端建模为「逻辑命名空间 + 懒加载工具集」。

6. **并发优先**：`ConcurrentAgentProcess` 用 goroutines + `errgroup`，比 Kotlin Coroutines 更贴近 Go 生态。

---

## 4. 设计原则

| # | 原则 | 含义 |
|---|-----|------|
| P1 | **核心思想不妥协** | GOAP + 黑板 + OODA 这三件是 embabel 的灵魂，Go 版必须保留 |
| P2 | **Go 风格压倒 Java 惯性** | 别照搬抽象类继承、别造 Spring 式 DI、别 ThreadLocal |
| P3 | **类型安全全程贯穿** | 用 Go 1.21+ 泛型，让 Action 的 In/Out 类型编译期可检查 |
| P4 | **与 Lynx 现有组件深度集成** | 复用 `chat.Client`、`mcp.Provider`、`rag.Pipeline`、`vectorstore`、`otelbridge` |
| P5 | **三个 API 层次** | DSL 做主推，struct+反射做便利层，codegen 给追求极致的用户 |
| P6 | **`agent/` 是独立 go module** | 与 `core/` 分离，可独立演进；但依赖 `core/` |
| P7 | **可选扩展放 `agents/`** | 和 `otelbridge/` 同规格，外挂扩展走独立 module |

---

## 5. Embabel 0.4 新抽象（Go 端必须回填）

| 新抽象 | 影响文档 | 摘要 |
|-------|--------|------|
| 🔴 **ToolGroup 生态**（`ToolGroupRequirement` / `ToolGroupPermission` / `ToolGroupMetadata` / `ToolGroup.tools` 懒加载）| 01 / 02 / 04 | Agent 新增 `toolGroups: Set<ToolGroupRequirement>` 字段；MCP 集成从「直接 bind tool」改为「通过 ToolGroup 懒加载」|
| 🔴 **Skills 模块**（`embabel-agent-skills/`：YAML 规格 + Docker/Process 执行引擎 + GitHub 分发）| 01 / 本 README | 新顶层 module；SPI：`SkillScriptExecutionEngine` / `DirectorySkillDefinitionLoader`。Lynx 规划对标走 `agents/skills/` 外挂 |
| 🔴 **`TerminationScope`**（`AGENT / ACTION` 枚举 + `terminateAgent(reason)` / `terminateAction(reason)`）| 03 | 从「抛异常」升级为「结构化终止」；在 tick 边界检测 |
| 🟠 **`LlmMessageStreamer` SPI** | 01 | 厂商中立流式工具循环接口；Lynx 的 `chat.StreamHandler` 天然对应 |
| 🟠 **`TokenCountEstimator<T>` SPI**（实验性）| 01 | token 预算预留接口；Lynx 的 `core/tokenizer/` 可作为实现入口 |
| 🟠 **`ToolCallInspector` SPI** | 02 | 拦截/观察 tool 执行流；Lynx 用 `StreamMiddleware` + `EventListener` 组合即可 |
| 🟠 **`NestedTool` 超接口**（合并 Progressive 与 Playbook tool 层级）| 02 | Lynx 当前 tool 层级扁平；agent 框架引入 sub-agent / nested action 时再合并 |
| 🟠 **流式重构**（`StreamingLlmOperationsFactory` + 统一 `LlmMessageStreamer`）| 03 / 05 | Lynx `iter.Seq2` 已是这层 |
| 🟡 **Autonomy dual binding**（`runAgent(input)` 同时按 `"it"` 和类型派生名绑定）| 02 | Blackboard 语义细化，不破坏现有设计 |
| 🟡 **Planner 重规划黑名单**（`bestValuePlanToAnyGoal(system, excludedActionNames)`）| 03 | action 抛 `ReplanRequestedException` 后加入黑名单重规划 |
| 🟡 **HITL 泛化**（`FormRequest` → `Awaitable<P, R>` 接口，`ConfirmationRequest<P>` / `FormBindingRequest` 为子类）| 02 | Go 端建模为泛型 `Awaitable[P, R] interface` |
| 🟡 **`Budget` 跨 nested subprocess 聚合** | 03 / 05 | 从子 process 的 `Usage` 累积往父 process 上传 |
| 🟡 **`@Agent` 校验收紧**（构造器不再允许 `OperationContext` 注入，fail-fast）| 04 | Go 的 DSL/struct reflect 方案无此约束，差异表注明 |

---

## 6. 与 Spring AI 2.0 / Lynx mcp 的合流

Spring AI 2.0 同期上线了 MCP 全家桶（`mcp-annotations` + `mcp-spring-{webmvc,webflux}` + 5 个 starter）；Embabel 0.4 引入的 `ToolGroup` 在「跨进程工具接入」上形成合流——一个 ToolGroup 在概念上类似一个 MCP server connection。

**Lynx 端定型**：
- Lynx 不需要 `agents/mcp/`——顶层 [`mcp/`](../../mcp/) 已经是 v1 桥接（详见 [`../MCP.md`](../MCP.md)）
- Agent 框架的 ToolGroup 直接复用 `mcp.Source` + `mcp.Provider`：每个 ToolGroup 对应一组（或多组）MCP source 的懒加载工具集
- v2 反向能力（`ServerSessionFromContext`）落地后，agent 内的本地 Action 可反向调 client LLM，闭环完整

---

## 7. 对 embabel-agent 功能的取舍

| 功能 | 对应 embabel 模块 | Go 移植策略 |
|-----|-----------------|-----------|
| 核心 agent 运行时 | embabel-agent-api | ✅ 完整移植 → `agent/` |
| GOAP A* 规划器 | embabel-agent-api plan/ | ✅ 完整移植 → `agent/planner/goap` |
| 注解编程模型 | api/annotation | ⚠️ 无对应物，用 DSL + struct tag + codegen 三选一 |
| Kotlin DSL | api/dsl | ✅ 移植为 Go 流式 API → `agent/dsl` |
| 事件系统 | event/ | ✅ 移植为 Go channel + listener → `agent/event` |
| HITL（表单 / 确认）| core/hitl | ✅ 移植为泛型 `Awaitable[P, R]` → `agent/hitl` |
| RAG | embabel-agent-rag | ✅ **复用 Lynx 现有 `core/rag`** |
| 代码分析 | embabel-agent-code | ❌ 暂不移植，用户按需自写 |
| MCP server 暴露 | embabel-agent-mcpserver | ✅ **复用 Lynx 顶层 `mcp/RegisterTools`** |
| MCP client 消费 | embabel `ToolGroup` | ✅ 复用 Lynx `mcp.Provider` + ToolGroup 概念封装 |
| A2A 协议 | embabel-agent-a2a | 🟡 放 `agents/a2a` |
| Shell REPL | embabel-agent-shell | 🟡 放 `agents/shell` |
| Skills | embabel-agent-skills | 🟡 放 `agents/skills` |
| Spring 自动配置 | autoconfigure / starters | ❌ Go 无 DI 容器，不需要 |

---

## 8. 移植的价值

1. **填补 Go 生态空白**：当前 Go 几乎没有 GOAP 风格的 agent 框架，主流（LangGraph、CrewAI）都是 Python
2. **与 Lynx 生态闭环**：Lynx 已经有 chat/embedding/RAG/vectorstore + MCP 桥接，加上 agent 就是完整栈
3. **类型安全超越 embabel**：Kotlin 泛型在 JVM 层会擦除，Go 泛型保留到编译末端，能做更多静态检查
4. **性能潜力**：Go 的 goroutine + channel 比 JVM + Reactor 在并发 agent 场景有天然优势
5. **MCP 一次到位**：通过复用顶层 `mcp/`，agent 上线即支持跨进程工具，避免 embabel 0.3 时代的 ToolGroup 与 MCP 适配 ad-hoc 阶段

---

阅读顺序建议：本 README → `01-architecture.md` → `02-core-abstractions.md` → 其余按需。
