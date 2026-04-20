# Lynx Agent 框架设计（embabel-agent Go 移植）

> 本目录是 `embabel-agent`（Spring AI 的 Java/Kotlin agent 框架）向 Lynx 生态的 Go 移植的完整架构设计。
> **核心思想保持一致**：GOAP（Goal-Oriented Action Planning）+ 黑板模式 + OODA 循环。
> **但充分考虑 Java/Kotlin 与 Go 的语言差异**，不做机械翻译，Go 风格优先。
>
> **初版基线**：embabel-agent ~0.3.5（2026-04-18 写作）
> **2026-04-20 复核基线**：embabel-agent `0.4.0-SNAPSHOT`（见 §0.4 新抽象章节）
> **实现进度**：本目录仍纯为设计文档，**尚无 Go 代码落地**（`agent/` 顶层 module 未创建）。

---

## 0.4 上游新抽象（2026-04-20 补遗）

Embabel 从 0.3.5 进到 0.4.0-SNAPSHOT 期间新增了三个第一级抽象 + 若干语义修订，Go 端设计必须回填。完整细节见对应章节：

| 新抽象 | 影响文档 | 摘要 |
|-------|--------|------|
| 🔴 **ToolGroup 生态**（`ToolGroupRequirement` / `ToolGroupPermission` / `ToolGroupMetadata` / `ToolGroup.tools` 懒加载）| 01 / 02 / 04 | Agent 新增 `toolGroups: Set<ToolGroupRequirement>` 字段；MCP 集成从「直接 bind tool」改为「通过 ToolGroup 懒加载」 |
| 🔴 **Skills 模块**（`embabel-agent-skills/`：YAML 规格 + Docker/Process 执行引擎 + GitHub 分发）| 01 / README | 新顶层 module；SPI：`SkillScriptExecutionEngine` / `DirectorySkillDefinitionLoader`。Lynx 规划对标走 `agents/skills/` 外挂 |
| 🔴 **`TerminationScope`**（`AGENT / ACTION` 枚举 + `terminateAgent(reason)` / `terminateAction(reason)`）| 03 | 从「抛异常」升级为「结构化终止」；在 tick 边界检测 |
| 🟠 **`LlmMessageStreamer` SPI** | 01 §3.8 | 厂商中立流式工具循环接口；Lynx 的 `chat.StreamHandler` 天然对应 |
| 🟠 **`TokenCountEstimator<T>` SPI**（实验性）| 01 | token 预算预留接口；Lynx 的 `core/tokenizer/` 可作为实现入口 |
| 🟡 **Autonomy dual binding**（`runAgent(input)` 同时按 `"it"` 和类型派生名绑定）| 02 | Blackboard 语义细化，不破坏现有设计 |
| 🟡 **Planner 重规划黑名单**（`bestValuePlanToAnyGoal(system, excludedActionNames)`）| 03 | action 抛 `ReplanRequestedException` 后加入黑名单重规划 |
| 🟡 **HITL 泛化**（`FormRequest` → `Awaitable<P, R>` 接口，`ConfirmationRequest<P>` / `FormBindingRequest` 为子类）| 02 | Go 端建模为泛型 `Awaitable[P, R] interface` |
| 🟡 **`@Agent` 校验收紧**（构造器不再允许 `OperationContext` 注入，fail-fast）| 04 | Go 的 DSL/struct reflect 方案无此约束，但差异表需注明 |

**与 Spring AI 2.0 的交叉**：Spring AI 2.0 同期上线了 MCP 全家桶（`mcp-annotations` + `mcp-spring-{webmvc,webflux}` + 5 个 starter）——这与 Embabel 的 ToolGroup 在「跨进程工具接入」上形成了合流。Lynx 的 `agents/mcp/` 规划可以一次性参照两者，详见 `doc/SPRING_AI_COMPARISON.md §11`。

---

## 文档导航

| # | 文档 | 主题 |
|---|------|-----|
| 01 | [architecture.md](./01-architecture.md) | 整体架构、模块拓扑、Java→Go 差异对照 |
| 02 | [core-abstractions.md](./02-core-abstractions.md) | 核心类型：Agent/Action/Goal/Condition/Blackboard 等 Go 定义（「为什么」） |
| 03 | [planner-and-runtime.md](./03-planner-and-runtime.md) | GOAP A* 规划器 + AgentProcess 执行引擎 |
| 04 | [user-api.md](./04-user-api.md) | 三种 Agent 定义风格：DSL / Struct+反射 / 代码生成 |
| 05 | [integration-and-examples.md](./05-integration-and-examples.md) | 与 Lynx `core/` 的集成（Chat、Observation、RAG）+ 完整示例 |
| 06 | [rollout.md](./06-rollout.md) | 分阶段落地计划 |
| 07 | [data-structures.md](./07-data-structures.md) | **核心数据结构目录**（实现蓝图，结构化索引） |

---

## 一分钟速览

### 核心概念对照

```
embabel-agent (Java/Kotlin)           →    Lynx Agent (Go)
────────────────────────────────────────────────────────────
@Agent class                          →    DSL builder / struct + reflection
@Action / @AchievesGoal method        →    agent.Action("name", func(...))
@Condition method                     →    agent.Condition("name", func(...))
Spring AI ChatClient                  →    core/model/chat.Client
Micrometer Observation                →    core/observation.Registry
AStarGoapPlanner (Kotlin)             →    planner/goap.AStarPlanner (Go)
Blackboard                            →    blackboard.Store
AgentProcess.run() loop               →    AgentProcess.Run(ctx) with goroutines
ThreadLocal AgentProcess.get()        →    context.Context 透传
Reactor Flux streaming                →    iter.Seq2 迭代器
Jakarta Validation                    →    go-playground/validator 或自写
```

### 模块拓扑（计划新增）

```
lynx/
├── core/                 (已有)
│   ├── model/            chat / embedding / ...
│   ├── observation/      抽象 + noop + slog
│   ├── rag/              RAG pipeline
│   ├── vectorstore/      接口 + filter DSL
│   └── document/         Reader/Writer/Transformer
│
├── agent/                ★ 新增顶层 module — agent 框架核心
│   ├── go.mod
│   ├── core/             Agent / Action / Goal / Condition / Blackboard
│   ├── plan/             Plan / WorldState / 接口
│   ├── planner/
│   │   ├── goap/         A* GOAP 实现（核心）
│   │   └── utility/      效用规划器（开放式任务）
│   ├── runtime/          AgentProcess / 执行引擎 / 事件
│   ├── dsl/              流式 builder API
│   ├── reflect/          struct 反射注册（@Action 注解的 Go 等价物）
│   ├── event/            事件类型 + 多播监听器
│   └── hitl/             Human-in-the-loop
│
└── agents/               ★ 新增顶层 module — 可选扩展（对标 observations/）
    ├── a2a/              Agent-to-Agent 协议
    ├── mcp/              MCP 服务端集成
    └── shell/            REPL shell（参考 embabel-agent-shell）
```

### 关键设计决策

1. **核心抽象 Go 化**：Agent/Action/Goal/Condition 都是 Go 接口或 struct，用 **context.Context 替代 ThreadLocal**，用 **泛型顶层函数替代 Kotlin 泛型方法**。

2. **三种 Agent 定义方式**：
   - **DSL（推荐）**：`agent.New("...").Action(...).Goal(...)` 流式构建
   - **Struct + 反射**：在 struct 方法上打标签（struct tag 或 marker comment），反射解析
   - **代码生成**（可选）：`//go:generate lynx-agent-gen` 读源码生成注册代码

3. **LLM 调用走 Lynx 自己的 chat.Client**：不直接嵌入 Spring AI 概念，`ProcessContext` 提供 `LLM()` 方法返回 `*chat.Client`。

4. **观测走 `core/observation`**：每个 tick / plan / action 生成 span，与 Lynx 现有观测一致。

5. **并发优先**：`ConcurrentAgentProcess` 用 goroutines + `errgroup`，比 Kotlin Coroutines 更贴近 Go 生态。

---

## 设计原则

| # | 原则 | 含义 |
|---|-----|------|
| P1 | **核心思想不妥协** | GOAP + 黑板 + OODA 这三件是 embabel 的灵魂，Go 版必须保留 |
| P2 | **Go 风格压倒 Java 惯性** | 别照搬抽象类继承、别造 Spring 式 DI、别 ThreadLocal |
| P3 | **类型安全全程贯穿** | 用 Go 1.21+ 泛型，让 Action 的 In/Out 类型编译期可检查 |
| P4 | **与 Lynx 现有组件深度集成** | 复用 chat.Client、observation.Registry、rag.Pipeline、vectorstore 等 |
| P5 | **三个 API 层次** | DSL 做主推，struct+反射做便利层，codegen 给追求极致的用户 |
| P6 | **`agent/` 是独立 go module** | 与 `core/` 分离，可独立演进；但依赖 `core/` |
| P7 | **适配器放 `agents/`** | 和 `observations/` 同规格，外挂扩展走独立 module |

---

## 对 embabel-agent 功能的取舍

| 功能 | 对应 embabel 模块 | Go 移植策略 |
|-----|-----------------|-----------|
| 核心 agent 运行时 | embabel-agent-api | ✅ 完整移植 → `agent/` |
| GOAP A* 规划器 | embabel-agent-api plan/ | ✅ 完整移植 → `agent/planner/goap` |
| 注解编程模型 | api/annotation | ⚠️ 无对应物，用 DSL + struct tag + codegen 三选一 |
| Kotlin DSL | api/dsl | ✅ 移植为 Go 流式 API → `agent/dsl` |
| 事件系统 | event/ | ✅ 移植为 Go channel + listener → `agent/event` |
| HITL (表单 / 确认) | core/hitl | ✅ 移植 → `agent/hitl` |
| RAG | embabel-agent-rag | ✅ **复用 Lynx 现有 `core/rag`** |
| 代码分析 | embabel-agent-code | ❌ 暂不移植，用户按需自写 |
| MCP 服务端 | embabel-agent-mcpserver | 🟡 放 `agents/mcp` |
| A2A 协议 | embabel-agent-a2a | 🟡 放 `agents/a2a` |
| Shell REPL | embabel-agent-shell | 🟡 放 `agents/shell` |
| Spring 自动配置 | autoconfigure / starters | ❌ Go 无 DI 容器，不需要 |

---

## 移植的价值

1. **填补 Go 生态空白**：当前 Go 几乎没有 GOAP 风格的 agent 框架，主流（LangGraph、CrewAI）都是 Python。
2. **与 Lynx 生态闭环**：Lynx 已经有 chat/embedding/RAG/vectorstore，加上 agent 就是完整栈。
3. **类型安全超越 embabel**：Kotlin 泛型在 JVM 层会擦除，Go 泛型保留到编译末端，能做更多静态检查。
4. **性能潜力**：Go 的 goroutine + channel 比 JVM + Reactor 在并发 agent 场景有天然优势。

---

阅读顺序建议：`README.md` → `01-architecture.md` → `02-core-abstractions.md` → 其余按需。
