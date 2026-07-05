# 架构评审 —— 资深架构师视角下的 Agent（向 Clean Arch + DDD 方向）

> **日期**：2026-06-18。**视角**：资深架构师命题作文 —— "以 Linus（反仪式、看代码不看口号）+ Uncle Bob（Clean Arch 依赖规则、ports）的合体视角，评审 agent 现状；如果从零写，系统架构与文件组织怎么设计，向 clean arch + DDD 方向？"
> **状态**：**批判性评审（review），非新基准**。与 [`GUIDE.md`](GUIDE.md) / [`EXTENSION_DESIGN.md`](EXTENSION_DESIGN.md) 的关系：那两份是"怎么用 / SPI 怎么设计"的指南；本文是"**对现状的一次独立架构体检**" —— 验证库的分层是否健康、指出真债、并裁决"DDD 方向"里哪些是该做的、哪些是仪式。本文不推翻既有设计，~90% 收敛于现状。
> **2026-07-05 里程碑更新**：本评审列出的 P0/P1 结构项已经落地到代码面：`agent/internal/arch/arch_test.go` 机器防腐已存在，`autonomy.Autonomy` 已改为 `autonomy.Router`，`agent/core.ChatClient` port 已隔离具体 `*chat.Client`。实际实现保留 `core/model/chat` 的 request/tool/options/middleware 作为共享协议原语，避免复制一套 agent-local chat 协议和 adapter ceremony；当前边界是“不依赖具体 chat client”，不是“不引用任何 chat 协议类型”。
> **方法**：第一手通读 agent 现状（core 原语 / runtime 引擎 / planning 策略 / Extension SPI / 各文件的 chat 依赖）+ 对照 [`../../DESIGN_PHILOSOPHY.md`](../../DESIGN_PHILOSOPHY.md) §2.3（一个扩展机制）+ §2.5（具体度向上流动）与 [`../../REFACTORING.md`](../../REFACTORING.md) §5.1（单后端不叠 DDD 层）+ §8（大但内聚不拆）。与 [`EMBABEL_ORGANIZING_PRINCIPLES.md`](EMBABEL_ORGANIZING_PRINCIPLES.md) 的 convergent-design 印证对齐。
>
> **结论先行**：
> 1. **最关键的认识：`agent` 是 SDK 库，不是应用 —— 这与 `lyra`（应用）正相反。** 教科书 Clean Arch 的环（delivery/use-case/domain/infra）对库**不直接适用**。库的自然形态是「原语（core）+ 执行引擎（runtime）+ 策略插件（planning/workflow）+ SPI（Extension）」，与应用的环不同。项目自己的 `agent/CLAUDE.md` 强约定"库内部用具体类型，不做内部 ISP" + `DESIGN_PHILOSOPHY §2.5` ISP「库 vs 应用」规则 —— **这套反仪式立场对库是对的。**
> 2. **等级 B+ → A-（差一步到 A）。** 三个真强项：core 的充血领域模型（`Determination` 三值逻辑 / `Blackboard` Reader-Writer ISP / `ActionMetadata` 状态转移）、Extension SPI（`collectExtensions[T]` 单分发，§2.3 教科书落地）、planning 分包（OCP 典范）。原评审列出的机器防腐、Router 命名、concrete chat client 泄漏已在 2026-07-05 前后收口；剩余主要是 `runtime/` 偏大但多数内聚，以及少量真实能力 gap。
> 3. **"向 DDD 方向"对库的真正含义是：保持原语层纯 + 保持 SPI 干净 + 充血模型 —— 而不是加 repository/use-case/aggregate-root 层。** `REFACTORING §5.1` 明确拒绝叠 DDD 层，对库更是过设计。agent 已经做对了充血模型，不需要再加层。
> 4. **最大杠杆的一步已落地为克制版 ChatClient port** —— 定义 `core.ChatClient`，消除 `core` / runtime 对具体 `*chat.Client` 的依赖；chat request/tool/options 仍作为共享协议原语保留。

---

## 0. 怎么读这份文档

- **作者评审视角**：本文是体检报告，定位"现状哪里是经得起推敲的好设计、哪里是真债、哪里是被反仪式立场正确拒绝的仪式"。它**不引入新债、不要求改名**（与 lyra 侧 [`../../lyra/doc/GREENFIELD_ARCHITECTURE.md`](../../lyra/doc/GREENFIELD_ARCHITECTURE.md) §0 同纪律）。
- **与既有文档的关系**：`GUIDE.md` 是"怎么用"、`EXTENSION_DESIGN.md` 是"SPI 怎么设计"、`EMBABEL_*` 是"与业界对照"；本文是"**架构健康度体检**"。落地动作只认 §5 的 P0/P1。
- **核实声明**：本文原始 file:line 引用来自 2026-06-18 的第一手核实。2026-07-05 里程碑后，`agent/internal/arch/arch_test.go` 已存在，`autonomy.Router` 已落地，concrete `*chat.Client` 依赖已通过 `core.ChatClient` port 收口；文中旧引用只保留为历史评审上下文。

---

## 1. 现状裁决

**等级：B+ → A-（差一步到 A）。** 对于 SDK 库来说，架构骨骼是对的。

### 做得好的 4 件事

1. **`core/` 的充血领域模型** —— `Determination` 三值逻辑（`And/Or/Not`，零值 `Unknown`，uninit 友好）、`Blackboard` 的 Reader/Writer ISP 拆分（读写边界由类型系统强制）、`ActionMetadata.IsApplicableIn` 纯状态转移、`Goal.Preconditions()` 合并、`Agent.KnownConditions()` 惰性缓存（`sync.OnceValue`）。这些是**真正的行为挂在类型上**，不是贫血数据袋。正是 `REFACTORING §5.1` 要的充血。
2. **Extension SPI 是 `DESIGN_PHILOSOPHY §2.3` 的教科书落地** —— `collectExtensions[T]` 泛型类型分发（`runtime/dispatch.go`），9 个能力子接口（`ActionMiddleware`/`ToolDecorator`/`AgentValidator`/`GoalApprover`/`ToolGroupResolver`/`IDGenerator`/`ChatClientProvider`/`EarlyTerminationPolicy`/`EventListener`）全走同一条路。加新能力 = 加 interface + 加一个 dispatch 点，不改 loop —— OCP 干净。比 embabel 的 30+ 异质 SPI 更统一（见 `EMBABEL_ORGANIZING_PRINCIPLES.md`）。
3. **`planning/` 分包是 OCP 典范** —— `Planner` 接口定义策略（`planning/planner.go`），4 个算法各占一个子包（`goap`/`htn`/`reactive`/`utility`）。加新 planner 放 `planning/planner/<name>/`，不动 runtime。
4. **HITL first-class** —— `Awaitable[T]` 在 core 是原语，`hitl/` 做 typed 层，状态机支持 `StatusWaiting` 暂停 / `Resume` 恢复。lyra 的整个 turn 中断/恢复建在其上。

### 原评审识别的 3 处债（2026-07-05 已部分收口）

1. **无 `arch_test.go` 机器防腐** —— 已落地。
2. **`core/` 原语层被具体 `*chat.Client` 污染** —— 已通过 `core.ChatClient` port 收口；chat request/tool/options 作为共享协议原语保留。
3. **`runtime/` 文件数偏大** —— 但大部分是内聚的状态机逻辑。是否拆需要谨慎裁决，不能为整洁而拆。（详见 §2.3）

### 3 处中等债

4. **`autonomy.Autonomy` 命名 stutter** —— 已改为 `autonomy.Router`。
5. **`workflow/` import `runtime` 并持 `*runtime.Platform`** —— 库内部按约定允许，但意味着 workflow 不能独立于 runtime 测试。当前形态务实。（详见 §2.8）
6. **`core/service_provider.go` 是 `map[string]any` service locator** —— 类型不安全、依赖隐藏。但这是库的 architecture tax（不能预知用户依赖），可接受。（详见 §2.9）

---

## 2. 与教科书 Clean Arch / DDD 的偏差逐条裁决

### 2.1 `arch_test.go` 机器防腐 —— 已落地

lyra 机器强制同心环 DAG；agent 完全没有。但 agent 需要的规则**不同于 lyra 的同心环** —— 对库，应机器强制的是：

```
core     ──── 不能 import ────▶  runtime, planning, event, workflow, hitl, toolpolicy
planning  ──── 不能 import ────▶  runtime
event     ──── 不能 import ────▶  runtime
```

这条规则捕获"原语层不依赖引擎层"的本质。目前全凭约定维持 —— 而 `core/` 已被 chat 污染，正说明约定不够硬。

**裁决：已落地。** `agent/internal/arch/arch_test.go` 现在编码上述 DAG，作为后续防腐基线。

### 2.2 `core/` import `core/model/chat` —— 混合债（最核心的架构问题）

现状：6 个 `core/` 生产文件 import `core/model/chat`：

| 文件 | 用途 | 用到的 chat 类型 |
|---|---|---|
| `process_context.go:11` | `ProcessContext.Chat()` / `ChatWithActionTools()` | `*chat.Client`, `*chat.ClientRequest`, `chat.Options`, `chat.Tool` |
| `condition.go:8` | `PromptCondition.Evaluate()` 调 LLM | `*chat.Client` |
| `prompt_runner.go:10` | `PromptRunner` — action 体内 LLM 调用门面 | `*chat.Client`, `*chat.ClientRequest`, `chat.Options`, `chat.Tool` |
| `guardrails.go:3` | `Guardrails` — 全局 chat middleware 包装 | `chat.CallMiddleware`, `chat.StreamMiddleware` |
| `extension.go:6` | `ChatClientProvider` — per-process 选 chat client | `*chat.Client` |
| `tool_group.go:10` | `AgentTool` 包装 `chat.Tool` | `chat.Tool` |

**问题本质**：chat 是 LLM 基础设施，不是 agent 原语。`Action`/`Goal`/`Condition`/`Determination`/`Blackboard` 是纯 agent 概念，不需要知道 LLM 长什么样。但 `PromptCondition` 需要调 LLM，`ProcessContext.Chat()` 是 action 体调用 LLM 的入口 —— chat 能力确实是普适需求。

按 `DESIGN_PHILOSOPHY §2.5` 判据："**是每个消费者都需要它，还是只有这一个消费方需要？**" —— 每个 action 体都需要调 LLM，`PromptCondition` 是 planner 可推理的原语。**chat 能力是普适的，应作为 interface 沉到 `core/`；具体实现留在外部。** 这是 §2.5 的标准用法：沉接口，不沉实现。

**两种切法：**

**方案 A（推荐）：在 `core/` 定义 `ChatClient` interface，把具体 `*chat.Client` 隔离出去。**
- `core/` 新增 `chat.go`，定义最小 `ChatClient` / `ChatRequest` / `ChatTool` / `ChatOptions` / `ChatCallResult` / `ChatStreamChunk` interface（不 import `chat` 包）
- `PromptRunner`/`PromptCondition`/`Guardrails`/`ProcessContext`/`ChatClientProvider`/`AgentTool` 全部依赖 interface 而非 `*chat.Client`
- `core/model/chat.Client` 隐式满足 `core.ChatClient`
- `core/` 不再 import `core/model/chat`，纯了
- **代价**：interface 须覆盖 `chat.Client` 实际使用面（~5-8 方法），中等大小。`ProcessContext.Chat()` 返回类型从 `*chat.ClientRequest` 变 interface —— **破坏性改动，需咨询用户**。

**方案 B：把 chat 相关文件从 `core/` 搬到 `runtime/`（或新建 `agent/llm/`）。**
- `prompt_runner.go`/`guardrails.go`/`ProcessContext` 的 Chat 方法/`ChatClientProvider`/`PromptCondition` 全搬 runtime 层
- `core/` 只留纯原语
- **代价**：`ProcessContext` 被拆成两半（核心在 `core/`，chat 在 `runtime/`），action 体要 import 两个包，API 体验倒退太大。**不推荐。**

**裁决：混合债，推荐方案 A。** 理由：(1) chat 是普适需求，按 §2.5 应沉核（作为 interface）；(2) `core/model/chat` 是整个 lynx 的基础设施包，不属于 agent，`core/` import 它的**具体类型**不对，import **interface**是对的；(3) 方案 B 拆 `ProcessContext` 体验倒退；(4) `PromptCondition` 是 planner 可推理的原语，放 `core/` 是对的，只需把它的 LLM 依赖改成 interface。

### 2.3 `runtime/` god package（53 文件 / ~8200 LOC）—— 混合（多数内聚，不拆）

`runtime/` 职责分布：

| 文件 | 职责 | 能否拆 |
|---|---|---|
| `agent_process.go` (440) | 状态机核心 + 扩展访问器 + 事件发布 | 核心，不能拆 |
| `run.go` (394) | tick 循环 + plan→act→status 转换 | 核心，不能拆 |
| `execute_action.go` (276) | action 执行 + retry + middleware dispatch | 核心，不能拆 |
| `platform*.go` (多文件) | Platform 构造 + deploy/run/process | 核心，不能拆 |
| `extension.go` + `dispatch.go` | Extension 注册 + 类型分发 | 内聚高，留 |
| `subagent.go` + `subagent_background.go` | 子进程 spawn + agent-as-tool | 紧依赖 `*Platform`/`*AgentProcess` 具体类型 |
| `mcp.go` + `publish.go` | MCP 集成 | 同上 |
| `concurrent.go` / `child.go` | 并发 action / child 进程 | 核心 |

按 `DESIGN_PHILOSOPHY §2.4`「包大 ≠ god package」+ `REFACTORING §8`「大但内聚、单一职责的不拆」—— `runtime/` 核心 ~15 个文件是**同一个状态机**的不同面，拆了反而要加跨包 import，违反 `agent/CLAUDE.md`「库内部用具体类型」。

**裁决：混合，推荐保持为一个包。** `subagent.go`/`mcp.go` 关注点不同，但紧密依赖 `*Platform`/`*AgentProcess` 具体类型 —— 拆出去增加的 import 噪音 > 整洁性收益，且违反「库内部用具体类型」。**等到 agent 有第二个外部消费者时再按实际痛感决定。** 53 文件在阈值内。

### 2.4 God files —— 内聚大文件，不拆

| 文件 | LOC | 判定 |
|---|---|---|
| `agent_process.go` | 440 | 三个 concern sub-struct（state/budget/signals）+ `core.Process` 实现 + 扩展访问器。**内聚** —— 就是一个 `AgentProcess` 的完整定义，注释已分区。`REFACTORING §8`：「拆了反而破坏内聚」→ 不拆 |
| `run.go` | 394 | tick 循环 + plan→act→status + stuck handler。**内聚** —— 就是主循环。不拆 |
| `process_context.go` | 388 | `ProcessContext` + Chat/ResolveTools/ActionTools。三个 concern sub-struct 已结构化分区。**内聚**。若执行方案 A，chat 方法改 interface 后自然缩到 ~300，更不需拆 |
| `execute_action.go` | 276 | action 执行 + retry + middleware + panic recovery。单一职责。不拆 |
| `condition.go` | 265 | `Condition` + `ComputedCondition` + `PromptCondition` + 组合子（And/Or/Not）。**内聚** —— 就是条件系统。不拆 |

### 2.5 `autonomy.Router` 命名收敛 —— 已落地

原问题是 `package autonomy` + `type Autonomy` 形成 `autonomy.Autonomy` stutter。`REFACTORING §1` 明确禁用 package-name stutter。

**裁决：已落地。** 主类型已改为 `Router`，文件为 `router.go`。这个名字贴合本质：接收 userInput，用 Ranker 在 Candidate 中选最高置信度的 (agent, goal) 对，再把请求路由给对应 agent。

### 2.6 Extension 系统 —— 真强项，不动

9 个能力子接口全走 `collectExtensions[T]` 一条分发 —— 完美契合 `DESIGN_PHILOSOPHY §2.3`「一个同质机制 + 类型分发」。加新能力只加 interface + 一个 dispatch 点，不改 loop —— OCP 到位。与 embabel 30+ 异质 SPI 的对比已由 `EMBABEL_ORGANIZING_PRINCIPLES.md` 裁决：lynx 的单分发在扩展统一性上优于 embabel 的多插槽。9 个子接口粒度合理 —— 每个对应一类具体运行时行为，不冗余。

**裁决：不债务，真强项。不需要加更多 hook。**

### 2.7 `core/` 充血模型 —— DDD-rich-model 的正确用法

| 类型 | 行为 | 评定 |
|---|---|---|
| `Determination` | `And/Or/Not` 三值逻辑，零值 `Unknown` | **真充血** —— 逻辑在类型上 |
| `ActionMetadata` | `IsApplicableIn(state)`, `EffectiveRunKey()` | **真充血** —— 状态转移在实体上 |
| `Goal` | `Preconditions()` 合并 Pre + Inputs | **真充血** |
| `Blackboard` | Reader/Writer ISP 拆分，`Spawn()` 原型模式 | **真充血** —— 读写边界由类型系统强制 |
| `Agent` | `KnownConditions()` 惰性缓存（`sync.OnceValue`） | **真充血** |
| `Condition` | `ComputedCondition`/`PromptCondition`/组合子（And/Or/Not） | **真充血** —— 组合模式在领域类型上 |

`REFACTORING §5.1` 说"单后端不叠 DDD 层"。agent 没有叠这些层 —— 它只有充血原语。**这是对的。** 没有显式 Aggregate Root 概念 —— `AgentProcess`（runtime 层）就是事实上的聚合根（持有 Blackboard/Planner/System/State/Budget/Signals）。对 SDK 库不需要在 `core/` 定义 `AggregateRoot` interface。

### 2.8 `workflow/` import `runtime` —— 务实，不急还

`workflow/sequence.go:9` import `runtime`，因 builder 需 `*runtime.Platform` 调 `SpawnChildFresh`。这违反纯分层（builder 不该依赖引擎），但在库上下文中：(1) workflow 产出 `*core.Agent`，是 agent 定义工厂；(2) `*runtime.Platform` 是构造期依赖（运行时需 platform 创建子进程），依赖真实；(3) embabel 同样依赖 agent process 跑子任务；(4) 替代方案更糟 —— 抽 `SubprocessSpawner` interface 是 YAGNI 仪式（单实现、库内部），下沉 spawn 到 core 违反"原语不依赖引擎"。

**裁决：当前形态务实。** 遵循 `agent/CLAUDE.md`「库内部用具体类型」。若将来 workflow 出现第二实现再抽 interface 不迟。

### 2.9 `core/service_provider.go` —— 库的 architecture tax，可接受

`ServiceProvider` 是 `map[string]any` + `sync.RWMutex`。问题：类型不安全（`ServiceOf[T]` 用 type assertion，运行时才发现类型错）+ service locator 反模式（依赖隐藏）。但存在理由合理：**agent 是库，不能预知用户依赖**。预定义 typed slots（ChatClient/RAG/VectorStore）会绑定特定生态。`map[string]any` 的开放性允许任何 action 体通过 `ServiceOf[MyCustomThing]` 取自己的依赖。

**裁决：库的 architecture tax，不是应用层反模式。不推荐改。** pre-1.0 不值得为类型安全微优化。

---

## 3. 如果从零写：系统架构（库的依赖规则，非应用环）

### 3.1 分层形状

不 paste 教科书应用环。库的依赖规则：

```
                    ┌──────────────────────────┐
                    │   lyra (application)      │  ← 消费方，定义自己的窄接口
                    └──────────┬───────────────┘
                               │ depends on
        ┌──────────────────────┼──────────────────────────┐
        │              agent (library)                      │
        │                                                   │
        │  ┌──────────────────────────────────────────┐    │
        │  │  workflow/  (combinators)                 │    │
        │  │  toolpolicy/ (decorators)                 │    │
        │  │  hitl/       (HITL primitives)            │    │
        │  └────────────────┬─────────────────────────┘    │
        │                   │ depends on                    │
        │  ┌────────────────▼─────────────────────────┐    │
        │  │  runtime/  (engine)                       │    │
        │  │  Platform + AgentProcess + dispatch       │    │
        │  │  autonomy/  MCP/  subagent/  event wiring │    │
        │  └──────┬──────────────┬────────────────────┘    │
        │         │              │ depends on               │
        │  ┌──────▼──────┐ ┌────▼──────────────┐          │
        │  │ planning/    │ │  event/            │          │
        │  │ Planner SPI  │ │  Lifecycle events  │          │
        │  │ + 4 algos    │ │  + Multicast       │          │
        │  └──────┬───────┘ └────┬───────────────┘          │
        │         │              │ depends on                │
        │  ┌──────▼──────────────▼───────────────────┐      │
        │  │  core/  (primitives — pure domain)       │      │
        │  │  Action / Goal / Condition / Blackboard  │      │
        │  │  Determination / Extension / Process     │      │
        │  │  ChatClient interface (not *chat.Client) │      │ ← 方案 A 的关键
        │  └──────────────────────────────────────────┘      │
        │                     │ depends on (interface)        │
        │  ┌──────────────────▼────────────────────────┐     │
        │  │  core/model/chat  (external infra)         │     │
        │  │  implements core.ChatClient implicitly    │     │
        │  └───────────────────────────────────────────┘     │
        └────────────────────────────────────────────────────┘
```

**核心规则：`core/` 不 import `core/model/chat` 的具体类型。** `core/` 只定义 interface。`core/model/chat.Client` 隐式满足 `core.ChatClient`。runtime 装配层持有 `*chat.Client` 并传给 `ProcessContext.Chat()`（返回 interface）。

### 3.2 核心纯洁性：ChatClient interface 的归属

按 `DESIGN_PHILOSOPHY §2.5`：每个 action 体都需要调 LLM → `ProcessContext.Chat()` 是普适需求；`PromptCondition` 是 planner 可推理的原语；`Guardrails` 是普适 chat middleware 包装。**chat 能力普适，应作为 interface 定义在 `core/`。** 具体实现留 `core/model/chat`，由 runtime 装配。这是 §2.5 标准用法：沉接口，不沉实现。

### 3.3 聚合根与充血模型 —— 确认已做对，不加层

**确认：agent 已有正确的 DDD 充血模型。不要加显式聚合根层。**
- `Agent` + `AgentConfig` 是配置聚合（deploy-time）
- `AgentProcess` 是运行时聚合根（持有 Blackboard/Planner/System/State/Budget/Signals）
- 不需要在 `core/` 定义 `AggregateRoot` interface —— 那是应用层 DDD 仪式，对库是 YAGNI

### 3.4 Repository 模式 —— 当前命名正确，不改

`ProcessStore` / `SessionStore` 是持久化 SPI —— 存/取/列/删 process 快照和 session 记录。它们本质是 Store（KV 持久化），不是 DDD 的 Repository（领域对象集合的抽象）。`REFACTORING §1`：「命名必须与承载的数据/所做的事一致」。这些接口存的是快照字节，不是领域对象 —— **Store 是对的，Repository 是错的。不改名。**

### 3.5 领域事件 —— event/ 包是正确设计

`event/` 定义 lifecycle 事件类型层次（`ProcessStarted`/`ActionExecutionStart`/`PlanFormulated`/`GoalAchieved`/`ProcessFailed` 等）。runtime 在状态转移时发布。对库：(1) 事件是观察者模式的正确表达 —— lyra 通过 `EventListener` 扩展订阅做 audit/tracing；(2) 事件单向（runtime → observer），不形成循环依赖；(3) `event.Multicast` 是解耦机制；(4) embabel 有同样的 `AgentProcessEvent` 层次，验证此设计。

**不需要"DDD domain events 框架"**（EventBus/EventStore/事件溯源）。当前设计够了 —— 事件是 lifecycle 信号，不是领域建模工具。

### 3.6 Extension SPI —— 确认"一个机制"，不加 hook

9 个子接口全通过 `collectExtensions[T]` 分发。加新能力 = 加 interface + 一个 dispatch 点。比 embabel 30+ 异质 SPI 更统一。**这是正确设计，不要加更多 hook。**

### 3.7 如何保持 runtime/ 内聚而不变 god package

**关键纪律**：(1) 不在 `runtime/` 加非状态机关注点 —— 新能力若不需访问 `*AgentProcess` 私有字段/锁，放独立包（像 `hitl/`/`toolpolicy/`）；(2) 大类型定义保持一个文件，衍生关注点（metrics/publish/MCP）保持独立文件；(3) 若将来 runtime/ 超 60 文件，考虑把 `subagent.go`+`mcp.go`+`publish.go` 拆到 `runtime/adapters/`。当前 53 文件未到阈值。

---

## 4. 如果从零写：文件组织（库的 greenfield 树）

```
agent/
├── core/                       # 原语层 — 纯领域模型 + 公开 SPI
│   ├── agent.go                # Agent + AgentConfig（配置聚合）
│   ├── action.go / action_typed.go / action_config.go   # Action + ActionMetadata + ActionQoS
│   ├── goal.go                 # Goal + GoalExport + GoalProducing
│   ├── condition.go            # Condition + ComputedCondition + 组合子（And/Or/Not）
│   ├── prompt_condition.go     # PromptCondition — LLM-driven（用 core.ChatClient）
│   ├── determination.go        # Determination 三值逻辑
│   ├── blackboard.go           # Blackboard(Reader+Writer) + InMemoryBlackboard
│   ├── process.go              # Process interface + Status enum
│   ├── process_context.go      # ProcessContext — action 体的唯一入口
│   ├── process_options.go      # ProcessOptions + Budget + Session
│   ├── chat.go                 # ★ NEW: ChatClient/ChatRequest/ChatTool/... interfaces（方案 A）
│   ├── guardrails.go           # Guardrails — chat middleware wrapper（用 core.ChatMiddleware）
│   ├── prompt_runner.go        # PromptRunner — action 体 LLM 调用门面（用 core.ChatClient）
│   ├── extension.go            # Extension marker + 9 capability interfaces
│   ├── tool_group.go           # ToolGroup + AgentTool（用 core.ChatTool）
│   ├── service_provider.go     # ServiceProvider — 开放 service locator
│   ├── early_termination.go    # EarlyTerminationPolicy + BudgetPolicy
│   ├── process_store.go        # ProcessStore SPI
│   ├── session.go              # SessionStore SPI + Session
│   ├── awaitable.go            # Awaitable[T] — HITL 原语
│   ├── invocation.go           # LLMInvocation / EmbeddingInvocation — 计费记录
│   ├── hooks.go / id_gen.go / output_channel.go / io_binding.go / enum.go / ...
│   └── doc.go
│
├── runtime/                    # 执行引擎 — 状态机 + dispatch（保持一个包，不拆）
│   ├── platform.go / platform_deploy.go / platform_run.go / platform_process.go
│   ├── agent_process.go        # AgentProcess struct + state/budget/signals + 扩展访问器
│   ├── run.go                  # run() tick loop + tickSimple + tickConcurrent
│   ├── execute_action.go       # executeAction + retry + middleware chain
│   ├── process_snapshot.go / concurrent.go / child.go
│   ├── extension.go / dispatch.go / registries.go
│   ├── subagent.go / subagent_background.go / agent_tool.go
│   ├── mcp.go / publish.go / metrics.go
│   └── autonomy/               # 自动目标选择
│       ├── router.go           # ★ RENAMED: Autonomy → Router
│       ├── ranker.go           # LLMRanker / PlanRanker
│       └── ...
│
├── planning/                   # 规划策略层 — Planner SPI + 算法族（不变）
│   ├── planner.go              # Planner interface + WorldState + System
│   └── planner/{goap,htn,reactive,utility}/
│
├── event/                      # 领域事件 — 类型定义 + 多播（不变）
│   ├── event.go / process.go / execution.go / planning.go / invocation.go / platform.go
│   ├── listener.go / multicast.go / marshaling.go / summaries.go
│
├── workflow/                   # 高阶组合器 — 产出 *core.Agent（不变，保持 import runtime）
│   ├── types.go / sequence.go / loop.go / parallel.go / repeat_until.go
│   ├── repeat_until_acceptable.go / scatter_gather.go / consensus.go / supervisor.go
│
├── hitl/                       # HITL 原语（不变）
├── toolpolicy/                 # Chat tool 装饰器（不变）
│
├── internal/                   # ★ NEW: internal 防腐
│   └── arch/
│       └── arch_test.go        # ★ NEW: 机器强制依赖规则
│
└── examples/                   # Demo（非 production，refactor 时 ignore）
```

### 与当前目录树的差异（DIFF）

| 改动 | 类型 | 理由 |
|---|---|---|
| **`core/chat.go` — NEW** | 结构 | 方案 A：定义 `ChatClient` interface，消除 `core/model/chat` 具体类型 import |
| **`internal/arch/arch_test.go` — NEW** | 结构 | 机器强制 DAG：core→runtime/planning/event/workflow 方向不可逆 |
| **`autonomy/autonomy.go` → `autonomy/router.go` — DONE** | 命名 | 消除 `autonomy.Autonomy` stutter；`type Autonomy` → `type Router` |
| **`core/` 6 文件删除 `"core/model/chat"` import** | 结构 | 改 import `core.ChatClient` interface |
| **`workflow/` 保持 import `runtime`** | 不变 | 库内部用具体类型。不抽 `SubprocessSpawner` interface（YAGNI） |
| **`runtime/` 不拆** | 不变 | 内聚的状态机包。53 文件在阈值内 |
| **`event/` 不拆** | 不变 | 内聚的事件包 |
| **`planning/` + `planning/planner/*/` 不变** | 不变 | OCP 正确 |
| **`core/service_provider.go` 不变** | 不变 | 库的 architecture tax，可接受 |

### 命名裁决

| 现状 | 替代 | 裁决 |
|---|---|---|
| `core` | `primitives` / `domain` | **core** —— Go 社区通用，"原语层"语义清楚。`primitives` 太学究，`domain` 是 DDD 术语但 agent 不全是 DDD domain（含 SPI/Store） |
| `runtime` | `engine` / `usecase` | **runtime** —— 描述"执行引擎"本质。`engine` 曾用名（已改），`usecase` 是应用层术语，对库不准 |
| `planning` / `event` / `hitl` / `toolpolicy` / `workflow` | — | 全部保留，名实相符 |
| `autonomy.Router` | **已落地** | **Router** —— 消除 stutter，且"路由器"准确描述其本质（选哪个 agent 处理请求） |

---

## 5. 落地建议（按 impact/risk 排序）

### P0：现在做，低风险 / 高收益

| # | 改动 | 类型 | scope |
|---|---|---|---|
| **1** | **加 `internal/arch/arch_test.go`** | 小重构（纯加测试） | **已落地**。编码规则：`core` 不能 import `runtime`/`planning`/`event`/`workflow`/`hitl`/`toolpolicy`；`planning`/`event` 不能 import `runtime`；`workflow` 可 import `runtime`（documented exception）。 |
| **2** | **`autonomy.Autonomy` → `Router`** | 小重构（命名） | **已落地**。`autonomy.go` → `router.go`，公开类型改为 `Router`，消除 stutter。 |

### P1：值得做，结构性（需咨询用户）

| # | 改动 | 类型 | scope |
|---|---|---|---|
| **3** | **方案 A：`core/` 定义 `ChatClient` port** | 结构性 | **已落地**。实际实现采用克制版：`core.ChatClient` 隔离具体 `*chat.Client`，`ProcessContext` / `PromptCondition` / `ChatClientProvider` / runtime platform 依赖 port；chat request/tool/options 作为共享协议原语保留。 |

### P2：设计取向，等触发再做

| # | 改动 | 触发条件 |
|---|---|---|
| **4** | `runtime/subagent/` + `runtime/mcp/` 拆分 | agent 有第二个外部消费者，或 runtime/ 超 60 文件 |
| **5** | `ServiceProvider` 改泛型注册 | 痛感积累（当前够用） |

### 明确不建议做的（YAGNI 戒律）

| 不建议做 | 理由 |
|---|---|
| 给 workflow 抽 `SubprocessSpawner` interface | 库内部单实现，YAGNI 仪式（违反 `agent/CLAUDE.md`「库内部用具体类型」） |
| 给 agent 叠 DDD layer（AggregateRoot / Repository / ApplicationService） | `REFACTORING §5.1` 明确拒绝，对库是过设计。agent 已有充血模型，不需再加层 |
| 把 `runtime/` 强行按 Clean Arch 环拆成 `usecase/`+`adapter/`+`domain/` | 库不是应用，没有 delivery/infra 这两层。拆了违反「库内部用具体类型」 |
| `ProcessStore`/`SessionStore` 改名 `Repository` | 存的是快照字节不是领域对象，Store 名实相符（`REFACTORING §1`） |
| 给 `event/` 加 EventStore / EventBus / 事件溯源 | 观察者模式够用，事件是 lifecycle 信号不是领域建模工具 |
| 给 `core/` 强行拆 `core/domain` + `core/spi` + `core/store` | 44 文件内聚，拆了增加 import 噪音。方案 A 让 core 变纯后更不需拆 |
| 拆 `agent_process.go` / `run.go` / `process_context.go` | `REFACTORING §8`：大但内聚不拆。拆了破坏内聚 |

### 最大杠杆的两件事

**1. 加 `arch_test.go`（P0-1）** —— 已落地，机器防腐闭合。

**2. 方案 A：`core/` 定义 `ChatClient` port（P1-3）** —— 已落地为克制版：不再依赖具体 `*chat.Client`，但保留 chat request/tool/options 共享协议原语，避免复制协议类型和 adapter ceremony。

---

## 一句话收尾

**`agent` 是库不是应用 —— 教科书 Clean Arch 环对它不直接适用，项目哲学的「库内部用具体类型、不叠 DDD 层」对它是对的。它的充血原语 + Extension 单分发 + planning 策略插件已是教科书级别的好设计。本轮已补齐机器防腐、Router 命名和 ChatClient port；后续再动应只挑真实职责混杂或真实能力 gap。**
