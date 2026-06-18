# EMBABEL_ORGANIZING_PRINCIPLES.md — 组织原则对比:两个框架如何把"能力"归约到"一个核心"

> 定位:**只谈一条轴 —— 架构组织哲学**。两个框架(lynx `agent` / embabel-agent)各自如何把"sub-agent / workflow / agent-as-tool / consensus …"这些能力**组织**起来?是各造一套机器,还是全部归约到同一个核心?组合 vs 继承 vs 框架反转?窄腰在哪?
>
> **分工**:维度级对比(GOAP 算法 / 类型系统 / ToolLoop SPI / Determination / 事件 / 生态 / 移植保真度 bug)已在 [`EMBABEL_DEEP_COMPARISON.md`](./EMBABEL_DEEP_COMPARISON.md) 写透,本篇**不重复**。本篇是它**没有系统覆盖**的"组织/为什么这么组织"层。
> 配套:lynx 侧的内部自举见 [`./SELF_BOOTSTRAP.md`](./SELF_BOOTSTRAP.md);扩展机制细节见 [`EXTENSION_DESIGN.md`](./EXTENSION_DESIGN.md)。约定见 [`../CLAUDE.md`](../CLAUDE.md)。
> 结论基于 2026-06 对两边源码的重新核对(embabel 给 `module/package` 路径,lynx 给 `file:line`,与既有两篇风格一致)。

---

## 0. 一句话结论(TL;DR)

> **两个框架独立收敛到同一条组织原则:薄规划核 + 一切能力归约成一个普通 agent,没有谁为某个能力重造 runtime。**
> embabel 的 `RepeatUntil`/`ScatterGather`/`ConsensusBuilder` 全都构造**虚拟 Goal/Action/Condition**、返回 `TypedAgentScopeBuilder → Agent`、复用 GOAP A\*;lynx 的 `workflow.*` 全都 `return core.NewAgent(...)`。这是**convergent design**,不是巧合 —— lynx 多个 builder 注释直书 "Mirrors embabel"。
>
> **分歧不在"要不要归约",而在三处"怎么归约":**
> 1. **入口**:embabel 三前端(注解 / Kotlin DSL / builder)漏斗汇到一个 `Agent`;lynx 单一 programmatic builder。
> 2. **扩展**:embabel 30+ 异质 Spring SPI + 注解 + 轻继承;lynx 一个 `collectExtensions[T]` 类型分发 + 纯组合 + 零 DI。
> 3. **谱系位置**:embabel = framework(Spring 反转控制)+ 少量继承链;lynx = library(你调它)+ 无继承(Go 无)。
>
> 收敛验证了 thin-core 原则**不是 lynx 的特异选择,是这类系统的"对的形状"**;分歧则是"Spring 框架 / 反射动态 / 多前端" vs "Go 库 / 泛型静态 / 单前端"两种工程品味的必然结果。

---

## 1. 共识:两边都把能力"编译成一个普通 agent"

这是本篇的地基,先把它坐实 —— 否则"组织哲学对比"无从谈起。

### 1.1 同一个归约目标

| | **embabel** | **lynx** |
|---|---|---|
| 归约目标(窄腰) | `data class Agent`(`embabel-agent-api/.../core/Agent.kt`) | `*core.Agent`(`agent/core/agent.go`) |
| 规划核 | GOAP A\*(`plan/goap/astar/AStarGoapPlanner.kt`),`PlannerFactory` 另支持 Utility/Condition | GOAP A\* 默认(`planning/planner/goap`),另有 htn/reactive/utility |
| 运行核 | `interface AgentProcess`(`core/AgentProcess.kt`) | `runtime.AgentProcess`(`agent/runtime/agent_process.go`) |
| 状态载体 | `Blackboard`(append-only,类型匹配) | `core.Blackboard`(ISP 拆 Reader/Writer) |
| 能力如何进核 | **构造虚拟 Goal/Action/Condition,组合成 Agent** | **同左:构造 Action/Goal/Condition,`core.NewAgent`** |

### 1.2 同一个手法:虚拟 Goal/Action/Condition

两边的 workflow builder **机制完全同构**:不发明新执行器,而是合成一组对 planner 不透明(`opaque`)的虚拟节点,让 GOAP 自己把循环/扇出跑出来。

| 能力 | embabel | lynx |
|---|---|---|
| RepeatUntil | `workflow/loop/RepeatUntil.kt` → `taskAction`(canRerun)+`acceptableCondition`+`consolidateAction`+`resultGoal`,`TypedAgentScopeBuilder(opaque=true)` | `workflow/repeat_until.go` → `task`(CanRerun)+`{name}_acceptable` condition + producing-goal,`core.NewAgent` |
| ScatterGather | `workflow/control/ScatterGather.kt` → `generateAction`+`consolidateAction`+`resultGoal` | `workflow/scatter_gather.go` → `{name}-scatter`+`{name}-gather`+producing-goal |
| Consensus | `workflow/multimodel/ConsensusBuilder.kt` → **包成 ScatterGather** | `workflow/consensus.go` → **`return ScatterGather(...)`** |
| RepeatUntilAcceptable | embabel `RepeatUntilAcceptable.kt`(best-of-N) | `workflow/repeat_until_acceptable.go`(注释直书 "Mirrors embabel's RepeatUntilAcceptable") |

> **结论**:在"能力如何进核"这件事上,两边是**同一个设计**。本篇剩下的全部篇幅,谈的是**进核之前的三层差异**(入口 / 扩展 / 谱系),那才是组织哲学真正分叉的地方。

---

## 2. 分歧一:能力的"入口" —— 多前端漏斗 vs 单前端

两边都归约到 `Agent`,但**喂给它的方式**根本不同。

### embabel:三个前端,共一个核

1. **注解**(主推,最低摩擦):`@Agent` / `@Action` / `@Condition` / `@AchievesGoal`(`api/annotation/annotations.kt`,都是 Spring `@Component`)→ `AgentMetadataReader` 反射扫描 → 生成 domain objects。
2. **Kotlin DSL**:`agent { action {…}; condition {…}; goal(…) }`(`api/dsl/`)→ 同样生成 domain objects,**不依赖 Spring**。
3. **Workflow builder**:`RepeatUntil.build(...)` 等 → `TypedAgentScopeBuilder`。

三条前端**全部漏斗到同一个 `Agent` + GOAP**。这是 embabel 组织上的最强项:**你可以从声明式(注解)一路滑到编程式(builder),核心一行不改**。

### lynx:一个前端

只有 programmatic builder(`agent.New(...).Actions(...).Goals(...).Build()` + `workflow.*`)。没有注解扫描,没有 DSL 方言。

### 裁决

| | embabel 多前端 | lynx 单前端 |
|---|---|---|
| 摩擦 | 注解最低(写个方法加 `@Action` 就行) | 较高(显式构造) |
| 绑定 | 注解路径**绑 Spring + 反射**(类型擦除、运行期才报错) | **纯泛型、编译期报错**、无容器 |
| 一致性 | 三前端语义对齐是持续成本(`AgentMetadataReader` 要追平 DSL) | 一种写法,无对齐成本 |
| 可演进性 | 已证明:**加一个声明式前端不必动核**(因为核是窄腰) | 同样可加(YAGNI:现在不需要) |

> **要点**:embabel 的多前端**不违反** thin-core —— 恰恰相反,它**证明了**窄腰的价值:正因为一切归约到 `Agent`,你才能在上面叠任意多个前端。lynx 今天只要一个;但若哪天要声明式 API,embabel 的经验说明"可以加前端、不动核",这与 [`SELF_BOOTSTRAP.md`](./SELF_BOOTSTRAP.md) 的"组合回核"是一回事。

---

## 3. 分歧二:扩展机制 —— 一个类型分发 vs 异质 SPI 群

"加一个新能力"时,两边的扩展点形态差异最大。

### embabel:30+ 异质 Spring SPI + 注解 + 轻继承

`spi/` 下铺开一片**异质**插槽:`PlannerFactory` / `BlackboardProvider` / `AgentProcessRepository` / `ToolGroupResolver` / `ToolDecorator` / `loop/ToolInjectionStrategy` / `loop/ToolNotFoundPolicy` / `LlmService` / `validation/*GuardRail` …每个是**独立接口**,靠 `@Bean` 注册进 Spring 容器。加能力 = 实现对应 SPI + 注册 bean(或 `extends AbstractAction`)。

### lynx:一个类型分发

只有 `core.Extension` marker + `collectExtensions[T any]([]Extension)`(`runtime/`)。7 个能力子接口(`ActionMiddleware` / `ToolDecorator` / `AgentValidator` / `GoalApprover` / `ToolGroupResolver` / `IDGenerator` / `Blackboard`)**全走同一条分发**。加能力 = 实现接口、塞进 `Extensions` 切片,dispatch 自动按类型发现 —— **OCP 靠泛型分发,不改 dispatch loop**。

### 裁决

| | embabel SPI 群 | lynx 单分发 |
|---|---|---|
| 插槽数 | 30+,粒度细 | 7 子接口,一条分发 |
| 机制同质性 | 异质(每个 SPI 不同形状)+ 绑 Spring | 同质(都是 `Extension`)、无容器 |
| 加新能力 | 实现 SPI + `@Bean` 注册 | 实现接口 + 进切片,编译期断言 |
| 心智负担 | 要知道**哪个** SPI、怎么注册 | 一个模式,类型说话 |
| 代价 | 细粒度换来表面积大 + Spring 依赖 | 粒度粗;真要细分插槽得加子接口 |

> 这条与既有 DEEP_COMPARISON §5 的"112 文件 SPI vs 10 types"互补:那篇比**数量/能力矩阵**,本篇比**组织形态**——embabel 是"**多机制并列、容器编织**",lynx 是"**单机制、类型编织**"。两者都能扩展;区别是 embabel 的扩展性来自"插槽多",lynx 的来自"分发统一"。

---

## 4. 分歧三:谱系位置 —— 组合 / 继承 / 框架反转

用户最初的问题是"这是不是组合大于继承"。把两个框架摆到谱系上,答案更清楚。

### 三条谱系轴

| 轴 | embabel | lynx |
|---|---|---|
| **继承 vs 组合** | **以组合为主**(`data class Agent` = Actions/Goals/Conditions 集合),**但有继承链**:`AbstractAction` → `TransformationAction`/`SupplierAction` → 用户类 | **纯组合**:Go 无继承,只有 embed(慎用)+ interface;Action 是函数+config,不派生 |
| **框架 vs 库** | **framework**:Spring 反转控制(容器扫描、注入、autoconfigure 调你) | **library**:你调它(`NewPlatform` / `StartAgent`),无容器、无反转 |
| **动态 vs 静态** | **动态**:注解反射、类型擦除、运行期装配 | **静态**:泛型 `TypedAction[In,Out]`、编译期绑定与报错 |

### "组合大于继承"在这里的精确含义

- 两边都**不靠继承组织能力**(能力 = 合成虚拟节点,不是 `class XxxWorkflow extends Agent`)。这点上**用户的判断对**,且**两边一致**。
- 但 embabel 仍保留**模板方法式的轻继承**(`AbstractAction` 给默认实现,子类覆写)——这是 Kotlin/JVM 的惯用法。lynx 在 Go 里**连这层继承都没有**(无 `extends`),只能组合。
- 更关键的分叉不是"继承 vs 组合",而是**"框架反转 vs 库调用"**:embabel 用 Spring 把"发现/装配/注入"反转给容器;lynx 把这些留给调用方显式做。在 Go 语境里,thin-core 的纪律不是"忍住别继承"(没得继承),而是 **"忍住别重造、别引入容器反转"**。

> 所以更锋利的命题是:**embabel 验证了"组合大于继承";lynx 把它推到 Go 的极端 —— "组合大于继承,且库大于框架,且静态大于动态"。** 三者叠加,才是 lynx 组织哲学的全貌。详见 [`SELF_BOOTSTRAP.md`](./SELF_BOOTSTRAP.md) 的"三种变体形态(参数化/组合/装饰)"。

---

## 5. 分歧四:sub-agent 复用什么基础设施

两边都认同"sub-agent = 复用同一套 runtime,不另造",但**复用的单元**和**隔离的旋钮**不同。

| | embabel | lynx |
|---|---|---|
| 暴露单元 | **Goal**:`GoalTool`(`tools/agent/GoalTool.kt`)把一个 Goal 包成 LLM 工具,内部 `autonomy.createGoalAgent` + `runAgent` | **Agent**:`AsChatTool`/`AsChatToolFromAgent`/`AsMCPTool`(`runtime/subagent.go`),共用 `newTypedAgentTool` + `processStarter` 策略 |
| 子进程基础设施 | 同一 `AgentProcess` + GOAP + 黑板;子进程共享父黑板 | 同一 `AgentProcess` + planner + 黑板;child 落在同 `platform.procs` |
| 隔离旋钮 | 子进程共享黑板(委派模型) | **显式两档**:`SpawnChild`(继承父黑板)vs `SpawnChildFresh`(干净黑板,分支隔离) |
| 入口策略复用 | `RunSubagent`(异常式,旧)/ `GoalTool`(推荐) | 一个 `processStarter` 抽象,`SpawnChild` vs `RunFresh` 切策略(`subagent.go:139`) |

**两点组织层面的差异**:
1. **单元粒度**:embabel 以 **Goal** 为暴露单元(一个 agent 多 goal,逐个 goal 成工具/MCP/A2A);lynx 以 **Agent** 为单元(典型"一 agent 一 goal",`AsMCPTool` 直接整 agent)。embabel 的 per-goal 暴露更细(也是 DEEP_COMPARISON §7 标的 `Goal.Export` gap)。
2. **隔离显式化**:lynx 把"子上下文要不要干净"做成**两个命名入口**(`SpawnChild`/`SpawnChildFresh`),组织上更显式;embabel 走统一委派 + 黑板共享。

> 呼应本会话的结论:**sub-agent 不是新概念,是"同一 runtime + 不同能力集/不同入口策略"的参数化**。两边都这么做;lynx 把"隔离"也提成了一个可见旋钮。

---

## 6. 分歧五:能力即参数(capability-as-parameterization)

"换一个能力 = 拨一个钮",两边都有,但**钮的形态**不同。

| 钮 | embabel | lynx |
|---|---|---|
| 选 planner | `@Agent(planner = PlannerType.GOAP)` 注解属性 / `PlannerFactory` SPI | `AgentConfig.PlannerName`(空→"goap"),planner 也是 `Extension` |
| 工具呈现 | `ToolInjectionStrategy` / `ToolNotFoundPolicy`(SPI 多档) | tool role 字符串 + `ToolGroupResolver`(sub-agent = 主 agent − `task` role) |
| 子进程入口 | `GoalTool` 固定委派 | `processStarter` 策略(`SpawnChild`/`RunFresh`) |
| 形态 | **注解属性 + SPI 替换**(声明/容器层拨钮) | **泛型参数 + 策略函数 + role 字符串**(类型/代码层拨钮) |

> 同一哲学(能力是数据/参数,基础设施是常量),落到 embabel = "注解枚举 + SPI bean",落到 lynx = "泛型 + 策略 + role"。前者在装配层拨,后者在编译层拨。

---

## 7. 窄腰有多窄

把"腰"画出来,最能看清两种组织的代价。

```
embabel                                   lynx
─────────                                 ─────────
 注解 / DSL / builder  (3 前端)            programmatic builder (1 前端)
        │                                          │
   ┌────▼─────┐  ← 腰周围还缠着:            ┌─────▼─────┐  ← 腰周围只有:
   │  Agent   │    Spring 容器              │ core.Agent│   一个 collectExtensions[T]
   │  + GOAP  │    30+ 异质 SPI             │  + Planner │   7 个同质 Extension 子接口
   │          │    注解反射元数据           │  接口      │   (无容器、无反射)
   └────┬─────┘                            └─────┬─────┘
        │                                          │
  AgentProcess / 黑板 / LLM                  AgentProcess / 黑板 / chat
```

- 两边的**腰本身**(Agent + Planner)都很窄、都很干净。
- 区别在**腰的"周长"**:embabel 的腰外面缠着 Spring 容器 + 异质 SPI + 注解元数据层,所以"穿过腰"要经过装配/反射;lynx 的腰外面只有一条类型分发,"穿过腰"是编译期的事。
- **代价对称**:embabel 用"较粗的腰周"换来声明式易用 + 细粒度插槽 + 自带前端(Shell/MCP/A2A);lynx 用"极细的腰周"换来零依赖、编译期安全、库可嵌入,代价是没有声明式糖、插槽较粗、自带前端少。

---

## 8. 组织原则对照总表

| 维度 | embabel | lynx | 一句话裁决 |
|---|---|---|---|
| 归约目标 | `data class Agent` + GOAP | `core.Agent` + Planner | **相同**(convergent) |
| 能力如何进核 | 虚拟 Goal/Action/Condition,`opaque` | 同左,`core.NewAgent` | **相同** |
| 入口数量 | 3(注解/DSL/builder)漏斗汇一 | 1(programmatic) | embabel 易用、lynx 一致 |
| 扩展机制 | 30+ 异质 Spring SPI + 注解 | 1 个 `collectExtensions[T]`,7 子接口 | embabel 细粒度、lynx 统一 |
| 继承倾向 | 轻继承链(`AbstractAction`) | 无继承(Go) | lynx 更纯组合 |
| 框架 vs 库 | framework(Spring 反转) | library(显式调用) | 取决于宿主生态 |
| 类型 | 动态(注解反射、擦除) | 静态(泛型、编译期) | lynx 调试/IDE 友好 |
| DI 容器 | 必需(注解/SPI 路径) | 无 | lynx 可嵌入任意 Go 进程 |
| sub-agent 暴露单元 | Goal(per-goal) | Agent(整体) | embabel 更细(=lynx 的 `Goal.Export` gap) |
| sub-agent 隔离旋钮 | 统一委派 + 共享黑板 | 显式 `SpawnChild`/`SpawnChildFresh` | lynx 更显式 |
| 能力即参数的"钮" | 注解属性 + SPI bean | 泛型 + 策略函数 + role | 装配层 vs 编译层 |

---

## 9. 结论 + 对 lynx 的启示

### 9.1 收敛说明了什么

embabel(更老、Spring/Kotlin、framework)和 lynx(更新、Go、library)**独立长出同一条组织原则**:**薄规划核 + 一切能力归约成普通 agent + 没人重造 runtime**。这强力佐证了本会话一路推出的判断 —— **thin-core + variants 不是 lynx 的个人趣味,是这类 GOAP-agent 系统的"对的形状"**。lynx 注释里的 "Mirrors embabel" 是诚实的:在**组织原则**上,lynx 是 embabel 的 Go-idiomatic 再表达。

### 9.2 哪些是品味分叉(各自都对,别互抄)

- embabel 的**多前端 + Spring SPI + 注解**适配它的宿主(Spring Boot 生态、声明式优先、自带 Shell/MCP/A2A server)。
- lynx 的**单前端 + 单分发 + 泛型静态 + 无 DI**适配它的宿主(Go 库、被 lyra 等嵌入、编译期安全优先)。
- **这两套不该互相抄**:给 lynx 加 Spring 式 SPI 群是引入容器反转(违背"库大于框架");给 embabel 砍成单分发会丢掉它的声明式入口价值。

### 9.3 唯一值得 lynx 在"组织层"借鉴的一点(非功能,是结构)

embabel 证明了 **"窄腰之上可以无痛叠加声明式前端"**(注解/DSL 与 builder 共核)。lynx 今天只要 programmatic 一种(YAGNI,正确)。但如果未来真出现"让用户声明式定义 agent"的需求,**组织上不必动核**:照 [`SELF_BOOTSTRAP.md`](./SELF_BOOTSTRAP.md) 的"组合回 `core.Agent`"路子,加一个生成 `core.Agent` 的前端即可——这正是窄腰的红利。**现在不做;但知道"能这么做、且不破坏核"**,本身就是这次对比的收获。

> 功能级的 gap(持久化 SPI / ToolLoop 策略 / per-goal MCP 暴露 / SupervisorAgent / Skills / A2A)不在本篇 —— 那些在 [`EMBABEL_DEEP_COMPARISON.md`](./EMBABEL_DEEP_COMPARISON.md) §9-10 已逐条定档(by-design vs 真 gap)。本篇只回答一个问题并给出肯定答案:**两个框架的"能力组织"是同一条原则,lynx 走的是它的 Go 极端形态。**

---

## 一句话收尾

**"把一切能力归约到一个普通 agent" —— 这条原则两个框架不约而同。** 分歧只在腰的周长:embabel 用 Spring 容器 + 30 个 SPI + 三前端把腰缠厚换易用,lynx 用一条类型分发 + 纯组合 + 单前端把腰削到最细换零依赖与编译期安全。**组合大于继承在这里成立,而 lynx 把它推到了"库大于框架、静态大于动态"的极端。**
