# 深度对比：lynx/agent vs embabel-agent（2026-06 刷新）

> **基线**
> - **lynx** HEAD `b02185f`（branch `main`，2026-06-18）；`agent/` 子模块 **~13.2k Go LOC / 112 文件 / 18 个内部包**：`core/`（~4.5k）、`runtime/`（~5.3k）+`runtime/autonomy/`（~0.6k）、`planning/`（~0.5k）+`planning/planner/{goap,htn,reactive,utility}`、`event/`（~0.7k）、`hitl/`（~0.3k）、`toolpolicy/`（~0.2k）、`workflow/`（~1.2k）。仅依赖 `core`/`mcp`/`pkg` + `otel`/`uuid`/`go-sdk`。
> - **embabel** HEAD `e3392133b`（main，2026-06-18）；~16 Maven module + 21 starter / ~240k LOC（Kotlin 主导），其中 `embabel-agent-api` ~151k LOC。
>
> **状态**：本文 **supersedes** 2026-05-29 版（基线 lynx `e480bc7` / embabel `9dc8a897`）。两边均重新通读源码。lynx 侧自旧基线经历 **~85 个 agent 相关 commit**（全仓 ~504），能力轮廓已发生实质变化；embabel 侧 **~26 个 commit**，增量集中在 provider 原生特性与观测迁移。更早的 `EMBABEL_COMPARISON.md`（2026-05-14）已 doubly superseded 并于 2026-06-18 删除，历史见 git。
>
> **方法论**：源码级（非文档级）。所有 file:line 断言均在第一手源码核实基础上，关键"Δ since old baseline"断言由预验证锚（verified anchors）加固。与旧对比的关系：本文 **不重复** 旧对比已固定的事实（如 GOAP 算法骨架、Determination 三值逻辑基础形态、Plan NetValue 3 项公式），重点标记 **Δ 增量（已关闭 gap / 新 gap / 仍 open）**。组织哲学单轴见 [`EMBABEL_ORGANIZING_PRINCIPLES.md`](EMBABEL_ORGANIZING_PRINCIPLES.md)（仍成立，不重复）；lynx 侧架构体检见 [`ARCHITECTURE_REVIEW.md`](ARCHITECTURE_REVIEW.md)。

---

## 0. TL;DR

**旧叙事**（2026-05）：lynx 在抽象卫生上全面领先，embabel 在原始能力广度上领先——ToolLoop SPI 全家桶、SupervisorAgent、表单、观测框架、内置工具等是 embabel 的决定性优势。

**新叙事**（2026-06）：lynx 在 2026-06 的密集开发窗口中 **关闭了大部分旧 P0/P1 gap**。核心变化来自 tool loop 全面重写——从"递归无界、串行、硬错"升级为 `DefaultMaxIterations=50` + `LoopDetection`（先于 cap 触发的固定点检测）+ `ConcurrentTool` 并行执行（`maxConcurrentToolCalls=8`）+ tool-error 默认恢复（ToolNotFound 合成结果反馈给模型）+ `FeedbackOnEmptyResponse`（opt-in）+ HITL park/resume 透传。同时补齐了 `workflow.Supervisor`（LLM 编排多 agent）、best-of-N 返回最优、OTel metrics、per-call LLM/embedding 事件、`Interrupt[R]` 统一 HITL 模型、以及 `ProcessStore` 可换持久化 SPI。embabel 同期投资在 provider-native 特性上（NativeStructuredOutput、thought signatures、fuzzy ToolNotFound matching、OCI AI starter、REST/SSE 端点）+ Spring Observation 观测迁移 + 模型覆盖 + ArchUnit 升级。

**今天的格局**：lynx 在抽象整洁度上维持领先（Extension 单分发、ISP 拆 Blackboard、HTN/后向 STRIPS/LLMPlanRanker、HITL 单泛型模型），**并在原始能力宽度上大幅追赶**——tool loop 健壮性、并行执行、Supervisor、best-of-N 已从 gap 变成持平甚至反超。embabel 继续在 framework 生态广度（starter/REST/Shell/A2A/RAG ingestion 深度/form 表单层/OperationScheduler/ArchUnit 机器防腐）上占有 library 无法追赶的天然位差。2026-07-05 后，lynx/agent 的机器防腐、Router 命名、concrete chat client 泄漏已收口；剩余 gap 多为 by-design 哲学差异或低优先级能力差。

### 总评分卡（7 维度 × 两轴：抽象整洁度 / 原始能力）

| 维度 | 抽象整洁度 | 原始能力 | Δ 说明 |
|---|---|---|---|
| 1. 哲学/定位 | **lynx** | 各有胜场 | lynx 库 vs embabel framework — 不变 |
| 2. 核心抽象 | **lynx** | embabel | ISP Blackboard / Process HAS-a / 单泛型 HITL — lynx 维持 |
| 3. 规划 | **lynx** | **lynx** | HTN + 后向 STRIPS + LLMPlanRanker — lynx 已反超 |
| 4. 运行时/执行 | **lynx** | **lynx** | 并发动作真并行 + spawn 梯度 + kill-race 全族 + ProcessStore SPI — lynx 补上持久化差距后领先 |
| 5. Extension/SPI | **lynx** | 各有胜场 | lynx 单分发 + 12 子接口 vs embabel 30+ 异质 SPI + **ArchUnit 机器防腐（lynx 缺）** |
| 6. Tool/ToolLoop | **lynx** | **lynx** | **维度级反转** — lynx 新 tool loop 并行+限流+检环+错误恢复+空回复重试+ParkStore HITL，全面超越 embabel DefaultToolLoop；embabel 仍有 injection strategy 体系 |
| 7. HITL/Workflow/生态/事件/观测 | **lynx** | embabel | lynx Interrupt[R] + Supervisor + best-of-N 关闭旧 gap；embabel 生态广度 + 观测框架 + form 表单层维持领先 |

---

## 1. 哲学 / 定位

**Δ since old baseline**：无实质变化。lyra 后端继续承担 HTTP/RPC/调度/认证，agent 库形态不变。embabel 新增 `embabel-agent-starter-webmvc`（REST/SSE 端点，#1695），强化 framework 定位。

### 当前状态对照

| 维度 | embabel-agent | lynx/agent |
|---|---|---|
| **角色** | Spring Boot **应用 framework** | 可嵌入的 Go **库** |
| **分发** | ~16 Maven module + 21 starter | 1 Go 包，复用 sibling 模块 |
| **DI/组装** | Spring `@Bean` + autoconfigure | `PlatformConfig.Extensions []Extension` 显式构造 |
| **自带前端** | Shell REPL + A2A server + MCP server + **REST/SSE**（#1695 新增） | 无（lyra 承担） |
| **持久化** | `AgentProcessRepository` + `ContextRepository` SPI（强制，tick-by-tick 更新） | **`ProcessStore` SPI**（快照式，opt-in）— **升级自旧 baseline 的"完全空白"** |
| **并发** | kotlinx.coroutines + `CompletableFuture` | 原生 goroutine + `context.Context` + `errgroup` |
| **进程传播** | `ThreadLocal<AgentProcess>` | `context.Context`（`core.WithProcess`/`ProcessFrom`） |
| **观测** | Spring Observation + Micrometer + `@Tracked` + **ObservationRegistry** 接线全 autoconfigure（#1718） | 直连 OTel + 新增 OTel metrics（`231cd21`） |
| **LOC** | api 自身 ~151k | agent ~13.2k |

### 裁决

两边的"库 vs 框架"差距没变，且各自在强化自己这条线上走得更远——embabel 加 REST/SSE 端点 + OCI autoconfigure，lynx 加 `ProcessStore` SPI + `ChatClientProvider` extension + `ParkStore` HITL 持久化。**lynx 没有理由变成 framework**，embabel 没有理由砍成 library。这是品味分叉，不是 gap。

---

## 2. 核心抽象建模

**Δ since old baseline**：HITL 从 `TypedRequest[P,R]` + 两个 tool-halt marker 升级为 `Interrupt[R any]` 统一模型（`hitl/interrupt.go:69`）。`ChatClientProvider` extension 新增（`core/extension.go:87`）。`ProcessStore` / `ProcessSnapshot` SPI 新增（`core/process_store.go:152`）。Blackboard 快照支持 `TaggedValue` 类型保持（`snapshot_codec.go:8`）。其余核心抽象骨架未变。

### 2.1 Action 形态

**未变**。lynx 维持 2 方法 `Action` + `NewAction[In,Out]` 泛型，embabel 维持 5 个接口交集（`DataFlowStep`/`ConditionAction`/`ActionRunner`/`DataDictionary`/`ToolGroupConsumer`）约 12 成员。

| | embabel | lynx | Δ |
|---|---|---|---|
| 接口成员数 | ~12（5 接口叠合） | 2（`Metadata()`+`Execute()`） | 未变 |
| 类型安全 | JVM 反射（注解模型） | Go 泛型 + `reflect.TypeFor[T]()` 编译期 | 未变 |
| 多输入绑定 | 反射绑 N 个参数 | 自动绑 `inputs[0]`，多余手 `Get[T]` | 未变 |
| Schema 自描述 | `DataDictionary`/`DomainType`/`PropertyDefinition` | 砍掉全层 | 未变 |

### 2.2 Process IS Blackboard vs Process HAS Blackboard

**未变**。lynx `Process`（~22 方法）vs embabel `AgentProcess`（继承 Blackboard + 5 接口，含继承成员 ~50+ god interface）。lynx 生命周期藏在 `runtime.AgentProcess` unexported 类型上。

### 2.3 Blackboard ISP

**未变**。lynx `BlackboardReader`（8 方法）/ `BlackboardWriter`（8 方法）/ `Blackboard`（Reader+Writer+Spawn+Clear）三拆，编译期读写隔离。embabel 单 `Blackboard` 接口 ~25 成员，但超类型/label/Aggregation 查找语义更强（`satisfiesType()` JVM 层级匹配）。

### 2.4 Determination 三值逻辑

**未变**。lynx `Determination`（`And` cost = `left+right` 求和 vs embabel `minOf`）+ `Unknown=0` 零值友好。

### 2.5 新增 / 变更的原语

| 新增 | 文件 | 作用 | embabel 对应 |
|---|---|---|---|
| `Interrupt[R any]` | `hitl/interrupt.go:69` | 统一 HITL：`Interrupt[R](ctx, key, value) (R, bool, error)`，idempotency guard 走 `bb.Get(resumeSlotKey(key))` | sealed `AwaitableResponse` 子型层级 + `ux/form` — lynx 更简洁但无表单层 |
| `InterruptError` | `hitl/interrupt.go:25` | 实现 `chat.ToolHalt`（`Abort()==false`），经 `HandleInterrupt` 一转 → `pc.AwaitInput` | `AwaitableResponseException` + 子型 handler |
| `ProcessStore` | `core/process_store.go:152` | 可换持久化 SPI（`Save/Load/Delete/List`），`ProcessSnapshot` 携带 `TaggedValue` 类型保持的黑板+条件+对象+调用历史 | `AgentProcessRepository`（tick-by-tick 更新，比 lynx 更"实时"） |
| `ChatClientProvider` | `core/extension.go:87` | per-process LLM 覆盖：一个 Platform 服务多模型 | `LlmService` SPI（不同形态） |
| `ParkStore` | `core/model/chat/middleware/tool/middleware.go:47` | HITL 中断轮次持久化（conversation-id keyed），transparent resume | 无对等独立抽象（HITL park 在 embabel 内化于 `AgentProcess`） |

---

## 3. 规划子系统

**Δ since old baseline**：GOAP A* 重构为 search-object（`13a412c`），embabel 侧 planners 未变（TODO 仍标注 `OptimizingGoapPlanner.kt:44` 的 >1 unknown-condition 未处理）。lynx 侧 `planning/plan.go` 的 `NetValue` 公式已是 3 项（`Value + ActionsValue − Cost`），在旧 baseline 截止前已修复。

### 3.1 Planner 接口

| | embabel | lynx | Δ |
|---|---|---|---|
| 方法数 | 5（含 `worldState()`/`prune()`） | **1**（`PlanToGoal`） + 包级辅助函数 | 未变 |
| 状态性 | 有状态（`worldState()` 暴露内部 determiner） | stateless（start 状态每次传入） | 未变 |
| 泛型参 | 3（`S,W,P`） | 0（接口无参，类型擦除到 `core.WorldState`） | 未变 |

### 3.2 GOAP A*

| 特性 | lynx | embabel | Δ |
|---|---|---|---|
| 闭集重开 | ✗（首 pop 最优假设，consistent 启发式保障） | ✓（`closedSet.remove` 更便宜路径） | 未变 |
| forward 优化护栏 | ✗（仅删无副作用 no-op，实践安全） | ✓（破坏可达性 → 回退） | 未变 |
| unknown-condition 惰性分支 | ✗ | ✓（1 unknown → True/False variant 各规划 → 评估 → commit；>1 TODO） | **embabel 独有，仍 open** |
| 后向 STRIPS 相关性剪枝 | ✓ `relevance.go`（不动点回归，A* 入口前剪枝） | ✗（仅 post-hoc `prune`） | **lynx 独有，已巩固** |
| search-object 重构 | ✓ `13a412c` 塌缩进 search object | ✗ | lynx 架构改进 |

### 3.3 HTN（lynx 独有）

**未变**。`planning/planner/htn/`：`Task`/`Method`/`Library`，递归分解上限 64，soft fail 回溯。embabel 全仓库 0 处 HTN（旧 baseline 已确认，新 baseline 未变）。

### 3.4 Utility / Reactive

**未变**。lynx `reactive.Planner` 内建 0-progress 守卫（`reactive.go:116`），embabel `UtilityPlanner` 无此守卫（NIRVANA + 非自禁动作 = 永动）。lynx 另有 `utility.Planner` 忠实移植 embabel 可循环行为。两者都有：安全版（`reactive`）+ 忠实版（`utility`）。

### 3.5 规划综合裁决

**lynx 在规划维度已全面反超**（旧 baseline 是"平手偏 lynx"）：HTN 独有、后向 STRIPS 独有、LLMPlanRanker 独有、reactive 0-progress 独有、Planner 1-method ISP 完胜、stateless 并发安全。embabel 唯一优势点：unknown-condition 惰性分支（但 >1 时仍是 TODO，实用面窄）。

---

## 4. 运行时 / 执行引擎

**Δ since old baseline**：这是 lynx 变化最大的维度之一。主要增量：并发动作从"每动作一 goroutine + 预分配槽"（已是真并行）维持不变；**新增** tool-call 级并行（`ConcurrentTool` + `maxConcurrentToolCalls=8` + `segmentEnd` 冲突检测）、child spawn 完整梯度（`SpawnChild` / `SpawnChildProtectedOnly` / `SpawnChildFresh` / `RunFresh`）、kill-race 全族（first-terminal-wins / idempotent KillProcess / cancel-park race + budget leak / reject concurrent HITL parks / drop leaked child）、child EventListener 继承、`ProcessStore` 持久化 SPI、auto-snapshot。

### 4.1 并发执行

| 层 | lynx 当前 | embabel | Δ |
|---|---|---|---|
| **动作级** | `tickConcurrent`：`wg.Go` 每动作一 goroutine，预分配结果槽，真并行 | `actions.map{async}.map{runBlocking}` — 发起并发但收集循环串行阻塞 | 未变（lynx 始终更并行） |
| **工具调用级** | **`invoker.invokeToolCalls`**：`ConcurrentTool` 接口（`concurrency.go:32`），`segmentEnd` 冲突分组（`invoker.go:347`），`ParallelTool` 批 bounded by `maxConcurrentToolCalls=8`（`invoker.go:27`），abort 时 cancel 同级批（`invoker.go:419-430`） | `ParallelToolLoop`：asyncer + per-tool/batch 超时 | **CLOSED — 旧 gap"串行 for-range"已补，lynx 方案更精细（资源键冲突检测）** |

### 4.2 Child Spawn 梯度

```
lynx spawn 梯度（4 档）：
  SpawnChild              → 全继承父黑板（supervisor 流）
  SpawnChildProtectedOnly → 白板 + 仅 ambient（委派默认）
  SpawnChildFresh         → 干净黑板（orchestration/loop 分支隔离）
  RunFresh                → 顶层 root

embabel：统一委派 + 共享黑板，不形式化隔离梯度
```

**裁决**：lynx spawn 梯度已是完整能力，显式两档 → 四档升级，远超 embabel。

### 4.3 持久化

| | embabel | lynx 旧（2026-05） | lynx 新（2026-06） |
|---|---|---|---|
| **SPI** | `AgentProcessRepository`（强制，create-save / tick-update / `findByParentId`） | ✗ 仅内存 map | **`ProcessStore` SPI**（`Save/Load/Delete/List`，可换后端） |
| **更新频率** | 每 tick 自动 update | N/A | 快照式（opt-in：`AutoSnapshot` 或状态变更时/显式 save） |
| **子树递归** | `findByParentId` O(n) 扫描 | 直接 child 指针递归 | 直接 child 指针递归 |
| **类型保持** | JVM 序列化 | N/A | `TaggedValue` + `snapshotTypeTable`（type-preserving） |
| **参考实现** | In-mem + LRU + hierarchy-aware eviction | 无 | `InMemoryProcessStore` + `FileStore`（`3a9364e`） |
| **状态** | **embabel 胜在"强制实时"** | **完全空白** | **⚠️ PARTIALLY CLOSED — 有 SPI 但非 tick-by-tick 实时** |

### 4.4 状态机 / kill-race 不变式 / 其它

| 特性 | lynx | embabel | Δ |
|---|---|---|---|
| 状态机 | 9 状态 1:1 映射，`makeRunning` 闸更严（拒 FAILED/STUCK 重跑） | 9 状态，`else` 分支允许从 FAILED/STUCK/WAITING/PAUSED 重跑 | 未变 |
| kill-race | first-terminal-wins (`a2c17da`) + idempotent KillProcess (`15490f5`) + cancel/park race + budget leak (`50a996f`) + reject concurrent HITL parks (`9efee00`) + drop leaked child (`ce59304`/`e6b68e8`) | 基于 `InterruptedException` + `stop()` flag | lynx 更完备 |
| auto-snapshot | `AutoSnapshot=true` + `ProcessStore` 配置时自动持久化完成进程 | 内建于 repository 强制 tick-update | lynx 补上 |
| OperationScheduler | ✓ `scheduleAction` → Pronto/Delayed/Scheduled | ✗ 无（YAGNI by-design） | **embabel 独有，by-design skip for lynx** |
| deploy 可达性校验 | ✓ `ValidateAgent` + `checkGoalsReachable` + `runAgentValidators` | ✗（裸 map put，STUCK 时暴露） | **lynx 独有，已巩固** |

---

## 5. Extension / SPI 模型

**Δ since old baseline**：lynx Extension 子接口从 ~10 个增至 **12 个**（新增 `ChatClientProvider` + `BudgetPolicy`）。embabel 侧 ArchUnit 1.3.0 → 1.4.0，**embabel 有机器强制架构测试，lynx 仍没有**。

### 5.1 lynx Extension 子接口全集（2026-06）

| # | 接口 | 嵌 Extension | 分发模式 | Δ |
|---|---|---|---|---|
| 1 | `ActionMiddleware` | ✓ | collect（onion） | 未变 |
| 2 | `ToolDecorator` | ✓ | collect（wrap） | 未变 |
| 3 | `AgentValidator` | ✓ | collect（fail-fast） | 未变 |
| 4 | `GoalApprover` | ✓ | collect（AND 否决） | 未变 |
| 5 | `ToolGroupResolver` | ✓ | collect（首非 nil 胜） | 未变 |
| 6 | `IDGenerator` | ✓ | last-wins 单例 | 未变 |
| 7 | `EarlyTerminationPolicy` | ✓ | collect（OR） | 未变 |
| 8 | `BudgetPolicy` | ✓ | collect（OR） | **NEW** |
| 9 | `ChatClientProvider` | ✓ | collect（首非 nil 胜，process → platform） | **NEW**（`core/extension.go:87`） |
| 10 | `EventListener` | ✓ | collect → 启动时 Multicast.Add | 未变 |
| 11 | `core.Blackboard` | ✗（按类型检测，prototype `Spawn()`） | last-wins prototype | 未变 |
| 12 | `planning.Planner` | ✗（自带 `Name()`，name 分发） | name-match | 未变 |

### 5.2 关键对比点

| 维度 | lynx | embabel | 裁决 |
|---|---|---|---|
| **插槽数** | 12 子接口，1 条 `collectExtensions[T]` 分发 | 30+ 异质 Spring SPI | lynx 更统一（OCP 干净） |
| **加新能力** | 实现接口 + 注册，不改 dispatch loop | 实现 SPI + `@Bean` 注册 | lynx 更轻 |
| **机器防腐** | ✓ `agent/internal/arch/arch_test.go` | ✓ **ArchUnit 1.4.0**（#1670）— 机器强制分层规则 | 双方都有机器防腐，形态不同 |
| **验证子系统** | 单 `AgentValidator` + deploy 内建 reachability | manager + 分型 validator（structure/path）+ `ValidationPromptGenerator` LLM 校验 | embabel 更层化 |
| **ChatClientProvider** | ✓ **NEW** — per-process model 覆盖 | ✓ `LlmService`/`AutoLlmSelectionCriteriaResolver` SPI | 对等，形态不同 |

### 5.3 裁决

lynx 的 Extension 分发模型在 **OCP 清洁度**上继续领先——一个 `collectExtensions[T]` 收掉 embabel 的 30+ 异质 SPI。机器防腐已经补齐：lynx 用 `agent/internal/arch/arch_test.go` 编码库内依赖规则；embabel 用 ArchUnit 编码 framework 分层规则。lynx 侧 concrete `*chat.Client` 泄漏也已通过 `core.ChatClient` port 收口。

---

## 6. Tool / ToolLoop 系统 —— **维度级反转**

**Δ since old baseline**：这是 **变化最大的维度**。旧 baseline 判定"embabel 决定性领先"——lynx tool loop 极薄（递归无界、串行、硬错、无 max-iter、无 ToolNotFound 策略、无 EmptyResponse 策略、无动态注入、无 inspector、无并行）。现在：**几乎所有 gap 已关闭**，多处 lynx 方案更精细。

### 6.1 Tool 抽象

**未变**。核心差异仍是 `chat.Tool.Call` 返回 `(string, error)`（lynx）vs sealed `Result { Text / WithArtifact / Error }`（embabel）。artifact 通道仍是 embabel 能力优势。

### 6.2 ToolLoop 能力对角表（新旧对照）

| 能力 | embabel 2026-05 | embabel 2026-06 | lynx 2026-05（旧） | lynx 2026-06（新） | 裁决 |
|---|---|---|---|---|---|
| 自驱多轮 loop | ✅ | ✅ | ✅ 递归 | ✅ 递归（不变） | 对等 |
| returnDirect 短路 | ✅ | ✅ | ✅ | ✅ | 对等 |
| **流式 tool loop** | 部分（framework 拥有，只观测） | 部分 | ✅ **自驱** `executeStreamRecursively` | ✅ **自驱** + 合成 ToolMessage chunk（不变） | **lynx 更强** |
| **max-iteration cap** | ✅ `MaxIterationsExceededException` | ✅ | ❌ 无界递归 | ✅ **`DefaultMaxIterations=50`** + `MaxIterationsError`（`middleware.go:19`） | **CLOSED ✅** |
| **LoopDetection** | ✗ 无 | ✗ 无 | ✗ 无 | ✅ `loop_detection.go`：SHA256 round-signature + 滑动窗口（default `Window=10`/`Threshold=5`，"六轮字节相同=固定点"），**先于 cap 触发** | **lynx 独有 🆕** |
| **ToolNotFound 策略** | ✅ `AutoCorrectionPolicy` | ✅ **#1690 增强**：双 tier（delimiter 规范化 + token Jaccard），top-3，max 3 retries | ❌ 硬错 | ✅ **framework default**：合成 error result + available tools 列表，反馈给模型自纠（`invoker.go:468-470`） | **CLOSED ✅** — 不同策略：embabel fuzzy match + retry，lynx 反馈给 LLM 自纠 |
| **EmptyResponse 策略** | ✅ `RetryWithFeedbackPolicy` | ✅ | ❌ 空回复直接结束 | ✅ `FeedbackOnEmptyResponse`（opt-in），one-shot nudge：`emptyResponseNudge`（`middleware.go:23,360-373`） | **CLOSED ✅** |
| **tool-error 恢复** | ✅ per SPI policy | ✅ | ❌ hard stop | ✅ **framework default**：非 control-flow 错误全部 fold 进 tool result，反馈给模型（`invoker.go:128-137,476-479`） | **CLOSED ✅ — lynx 更彻底（默认开，不用配）** |
| **并行 tool 执行** | ✅ `ParallelToolLoop` | ✅ | ❌ 串行 for-range | ✅ **`ConcurrentTool`** interface + `ConcurrencyKey`（per-call 资源键）+ `segmentEnd` 冲突分组 + `maxConcurrentToolCalls=8` limiter + abort 时 cancel 同级 batch（`concurrency.go`/`invoker.go`） | **CLOSED ✅ — lynx 方案更精细（资源键避免假冲突）** |
| 动态 tool 注入 | ✅ `ToolInjectionStrategy`（Full/Chained/Unfolding/ToolChaining） | ✅ | ❌ tool 集冻结 | ❌ 仍冻结 | **仍 open — 但 planner 重规划 obviate 大半** |
| loop inspector/transformer | ✅ | ✅ | ❌（仅 per-tool OTel span） | ❌（per-tool OTel span + per-call span 增强） | **仍 open — 中低优先级** |
| HITL park/resume | ✅ 内化于 `AgentProcess` | ✅ | ❌（旧：在 runtime 层 park 整个 process） | ✅ **`ParkStore`** + conversation-id keyed transparent park/resume（`middleware.go:414-520`） | **CLOSED ✅ — lynx 方案隔离在 tool loop 层，不影响 process 状态机** |

### 6.3 裁决

**维度级反转**。旧 baseline 判定 embabel 在 ToolLoop 上"决定性领先"——今天这个判定已经不成立。lynx 新 tool middleware 在 **并行执行、loop 检测、错误恢复、ToolNotFound 反馈、空回复重试、HITL park/resume** 上已全面实现，且 `ConcurrentTool` 资源键冲突检测 + `LoopDetection` 固定点检测是 embabel 没有的增量。embabel 剩下的优势集中在 **`ToolInjectionStrategy` 体系**（Unfolding/Matryoshka 渐进披露、ToolChaining）和 **artifact 通道**（sealed `Result.WithArtifact`），前者被 lynx 的 planner 重规划 obviate 大半，后者仍是能力差。

---

## 7. HITL / Autonomy / Workflow + 生态 / 事件 / 观测

**Δ since old baseline**：lynx 在这几个维度关闭了大量旧 gap：`Interrupt[R]` 统一 HITL、`workflow.Supervisor` 补齐 LLM 编排、`RepeatUntilAcceptable` best-of-N 修正、per-call LLM/embedding 事件新增、OTel metrics 新增。embabel 侧新增 REST/SSE 端点 + 观测迁移 + `#1703` 多 goal 类型限制。

### 7.1 HITL

| | lynx 旧（2026-05） | lynx 新（2026-06） | embabel | Δ |
|---|---|---|---|---|
| **核心模型** | `TypedRequest[P,R]` + 两个 tool-halt marker | **`Interrupt[R any]`**（`hitl/interrupt.go:69`）：`(resp R, resumed bool, err error)` — 统一模型，idempotency guard 走 `bb.Get(resumeSlotKey(key))` | sealed `AwaitableResponse` 子型层级（`ConfirmationRequest`/`FormBindingRequest`/`TypeRequest`） | **CLOSED ✅ — lynx 更简洁（1 个泛型 vs 3+ sealed 子型）** |
| **表单绑定** | ✗ 无 | ✗ 无 | ✅ `ux/form/`：`Form` + 11 控件 + `FormBinder<T>` 反射 | **仍 open — by-design skip** |

### 7.2 Autonomy / Supervisor

| | lynx 旧（2026-05） | lynx 新（2026-06） | embabel | Δ |
|---|---|---|---|---|
| **SupervisorAgent** | ❌（"embabel 独有"——旧 baseline §7.2） | ✅ **`workflow.Supervisor[In,Out]`**（`workflow/supervisor.go:60`）：LLM-orchestration over named sub-agents, ReAct-style，编译成 `*core.Agent` | ✅ `SupervisorAgent`/`SupervisorAction` ReAct loop（max 10 iter） | **CLOSED ✅ — 旧 gap 关闭** |
| **LLM 选 plan** | ✅ `LLMPlanRanker` | ✅（不变） | ❌ 无 | **lynx 独有** |
| **Goal 否决** | ✅ `GoalApprover` | ✅ | ✅ | 对等 |
| **narrow platform 接口** | ✅ autonomy 包定义 2 方法 `platform` 接口（后内联回具体类型 — SDK 库内 ISP 规则） | ✅（不变） | ❌ `Autonomy` 直接抱 `AgentPlatform` | lynx DIP 更干净 |

### 7.3 Workflow primitives

| | lynx 旧（2026-05） | lynx 新（2026-06） | embabel | Δ |
|---|---|---|---|---|
| **RepeatUntilAcceptable 返回** | 返最后一次 | ✅ **返 best-of-N**（`AttemptHistory.Best()`，commit `fdb2c47`） | ✅ bestSoFar（按分） | **CLOSED ✅** |
| **分支隔离** | ✅ `SpawnChildFresh` 干净子黑板 | ✅（不变）+ `SpawnChildProtectedOnly` 新中间档 | ❌ 同一黑板 parallelMap | **lynx 反超** |
| **over-sub-agent 组合** | ✅ `Sequence`/`Parallel`/`Loop` over sub-agent | ✅（不变） | ❌ `ScatterGather`/`RepeatUntil` 只 over **闭包** | **lynx 反超 — 更广** |

### 7.4 事件

| | lynx 旧（2026-05） | lynx 新（2026-06） | embabel | Δ |
|---|---|---|---|---|
| **事件数** | 16 | **17**（+2 invocation：`LLMInvocationRecorded` + `EmbeddingInvocationRecorded`，`event/invocation.go`） | ~32（sealed `AgenticEvent` + Jackson 多态往返） | **LYNX PARTIALLY CLOSED — 补上了 LLM/embedding per-call 事件** |
| **可往返性** | 单向 lossy JSON | 单向 lossy JSON（未变） | Jackson 多态 可往返 | embabel 优势（未变） |
| **LLM/tool 层事件** | ❌ 无 | ✅ `LLMInvocationRecorded` / `EmbeddingInvocationRecorded` — 但 **tool-call 级事件仍无** | ✅ `LlmRequest/Response`、`ToolLoopStart/Completed`、`ToolCallRequest/Response`(correlationId)、`Embedding*` | **PARTIALLY CLOSED — LLM/embedding 有了，tool-call 级仍缺** |

### 7.5 观测

| | lynx 旧（2026-05） | lynx 新（2026-06） | embabel | Δ |
|---|---|---|---|---|
| **机制** | 直连 OTel（5 处 callsite） | 直连 OTel + **OTel metrics**（`231cd21`：ticks/actions/plans/run exits） | Spring Observation + Micrometer + `ObservationRegistry` 全 autoconfigure 接线（#1718） | **CLOSED ✅ — metrics gap 关闭** |
| **注解埋点** | ✗ | ✗ | ✅ `@Tracked` + AspectJ | by-design skip |
| **per-call token 追踪** | ❌ | ✅ per-call LLM/embedding 事件 + `RecordLLMInvocation`/`RecordUsage` | ✅ `LlmInvocationEvent`/`EmbeddingInvocationEvent` | **CLOSED ✅** |

### 7.6 生态广度

| 能力 | embabel | lynx | 裁决 |
|---|---|---|---|
| **LLM provider 数** | ~13（starter）+ OCI 新增（#1700） | **41**（`lynx/models/` 41 个 provider dir） | lynx 广度胜 |
| **RAG 深度** | RAG core+pipeline（~22k LOC，层级内容模型、entity-graph 抽取、chunk 合并/压缩） | `lynx/rag`(~1.6k) — 流水线较薄 | embabel RAG 机器更"深" |
| **向量后端** | 实质 1-2 | **27 后端**（`lynx/vectorstores/`） | lynx 广度胜 |
| **REST/SSE** | ✅ **NEW** #1695（`AgentProcessController`/`PlatformInfoController`/`SseController`） | ✗（lyra 承担） | by-design |
| **Shell/A2A/Skills/ONNX** | ✅ 内建 | ✗（lyra / 其他模块承担） | by-design |
| **Native Structured Output** | ✅ **#1715**（`NativeStructuredOutputMode` enum，policy gate，provider-neutral，OpenAI/DeepSeek 实践） | ✅ `core/model/chat` 的 structured output 走 parser 族（`MapParser`/`SchemaParser`），不依赖 provider-native | 策略不同 — 对等 |
| **Thought Signature / Reasoning** | ✅ **#1691**（`SpringAiLlmService.thinkingSupported` + `SuppressThinkingConverter` + `ThinkingResponse<O>` — 适配器层 bolt-on） | ✅ `core/model/chat` 的 `ReasoningPart`（first-class `OutputPart`，与 `TextPart`/`ToolCallPart` 并列，`part.go:92`）+ `Reasoning` struct（`modelinfo.go:201`）+ `HasReasoning()`/`JoinedReasoning()`/`ReasoningDelta()` — **protocol-native，非 provider bolt-on** | **lynx 更原生**（protocol 层支持，非适配器层） |
| **多 goal 类型限制** | ⚠️ **#1703 新增限制**：`AgentMetadataReader.kt:259-266` — 多个 `@AchievesGoal` action 有 >1 distinct return type → 返回 null（SupervisorAgent 豁免） | ⚠️ `AgentConfig.Goals` 是 `[]*Goal`，支持多个 goal — lynx 无此限制 | **embabel 新加限制，lynx 更宽松** |
| **ArchUnit 机器防腐** | ✅ ArchUnit 1.4.0（#1670） | ❌ **无**（ARCHITECTURE_REVIEW P0-1） | **embabel 独有 — lynx 真债** |
| **代码覆盖率** | 73%（#1701） | 待测 | embabel 可量化 |

---

## 8. Gap 闭合总表

| 旧 gap（DEEP_COMPARISON 2026-05） | 当前状态 | 证据 | 类型 |
|---|---|---|---|
| §6.3 并行 tool 执行（串行 for-range） | ✅ **CLOSED** | `ConcurrentTool` + `maxConcurrentToolCalls=8`（`concurrency.go:32`, `invoker.go:27,303-338`） | 能力补齐 |
| §6.3 max-iteration cap（无界递归） | ✅ **CLOSED** | `DefaultMaxIterations=50` + `MaxIterationsError`（`middleware.go:19,192-194`） | 能力补齐 |
| §6.3 LoopDetection（无） | 🆕 **NEW capability** | `loop_detection.go`（SHA256 round-signature + 滑动窗口，default `Window=10`/`Threshold=5`），先于 cap 触发 | 反超 |
| §6.3 ToolNotFound（硬错） | ✅ **CLOSED** | `invoker.go:468-470` — 合成 error result + available tools list，反馈模型 | 能力补齐 |
| §6.3 EmptyResponse 策略（空回复直接结束） | ✅ **CLOSED** | `FeedbackOnEmptyResponse` opt-in, one-shot nudge（`middleware.go:23,360-373`） | 能力补齐 |
| §6.3 tool-error 恢复（hard stop） | ✅ **CLOSED** | `invoker.go:128-137,476-479` — framework default: fold recoverable errors into tool result | 能力补齐 |
| §7.2 SupervisorAgent（embabel 独有） | ✅ **CLOSED** | `workflow/supervisor.go:60` `Supervisor[In,Out]` — LLM-orchestration over sub-agents, compiles to `*core.Agent` | 能力补齐 |
| §7.3 RepeatUntilAcceptable（返最后） | ✅ **CLOSED** | `AttemptHistory.Best()`（commit `fdb2c47`） | 正确性修正 |
| §8.3 metrics（agent 内无 metrics） | ✅ **CLOSED** | `231cd21` OTel metrics: ticks, actions, plans, run exits | 能力补齐 |
| §8.2 per-call LLM/embedding 事件（无） | ✅ **CLOSED** | `44c4d77` `LLMInvocationRecorded` + `EmbeddingInvocationRecorded` events（`event/invocation.go`） | 能力补齐 |
| §6.4 内置工具（~zero） | ⚠️ **PARTIALLY** | `e22be69` built-in math + sandboxed file tools — base exists, no full toolbox | 部分补齐 |
| §4.5/§10.2 持久化 SPI（完全空白） | ⚠️ **PARTIALLY** | `ProcessStore` SPI（`Save/Load/Delete/List`）+ `ProcessSnapshot` type-preserving — swappable backend EXISTS but snapshot-based (not tick-by-tick live) | 部分补齐 |
| HITL 模型（`TypedRequest[P,R]` + two markers） | ✅ **REWORKED** | `Interrupt[R any]` universal model（`hitl/interrupt.go:69`）+ `InterruptError`（`Abort()==false` satisfies `chat.ToolHalt`） | 架构升级 |
| per-process LLM override | 🆕 **NEW capability** | `ChatClientProvider` extension（`core/extension.go:87`） | 新增能力 |
| GOAP A* | ✅ **REFACTORED** | `13a412c` collapse into search object | 架构改进 |
| child spawn 梯度 | ✅ **COMPLETE** | `SpawnChild` / `SpawnChildProtectedOnly` / `SpawnChildFresh` / `RunFresh` 4 档 | 反超 |
| kill-race invariants | ✅ **COMPLETE** | first-terminal-wins / idempotent KillProcess / cancel-park race / reject concurrent HITL / drop leaked child | 反超 |
| child EventListener inheritance | ✅ **CLOSED** | `8ac8fc9` inherit parent EventListeners on child spawn | 能力补齐 |
| async/background subagent | 🆕 **NEW capability** | `44ce90c SpawnChildAsync + AsBackgroundChatTool` | 新增能力 |
| GOAP unknown-condition lazy branching | ❌ **STILL OPEN** | embabel `OptimizingGoapPlanner` 有 1-unknown 分支（>1 TODO），lynx 全无 | real gap（低优先级） |
| artifact-bearing tool | ❌ **STILL OPEN** | `chat.Tool.Call` 仍返回 `(string, error)` — no sealed `Result/WithArtifact` | real gap |
| `ux/form` 表单层 | ❌ **STILL OPEN** | by-design skip（UX 出 agent scope） | by-design |
| OperationScheduler | ❌ **STILL OPEN** | by-design skip（lyra 调度） | by-design |
| A2A server / Shell / Skills / ONNX in agent | ❌ **STILL OPEN** | by-design skip（在 lyra/`agents/` 外模块） | by-design |
| concrete `*chat.Client` 泄漏到 agent core | ✅ **CLOSED** | `core.ChatClient` port 已落地；chat request/tool/options 保留为共享协议原语 | 正向收敛 |
| 无 `arch_test.go` 机器防腐 | ✅ **CLOSED** | `agent/internal/arch/arch_test.go` 已落地 | 防腐闭合 |
| `autonomy.Autonomy` 命名 stutter | ✅ **CLOSED** | `autonomy.Router` 已落地 | 命名闭合 |
| ToolInjectionStrategy（Unfolding/Matryoshka） | ❌ **STILL OPEN** | planner 重规划 obviate 大半 — 仅超大 tool 集（>20）场景值 | low priority |

---

## 9. ROI 路线图（更新）

旧 baseline 的 P0 项（NetValue 补项、retry 清 effect、max-iter cap、ToolNotFound feedback、per-call LLM 事件）**全部已关闭**。旧 P1 项（持久化 SPI、metrics）**也已关闭或部分关闭**。新路线图聚焦剩余真债 + 低优先级能力差。

### P0 — 现在就做

| # | 项 | 工作量 | 价值 | 来源 |
|---|---|---|---|---|
| 1 | **加 `arch_test.go` 机器防腐** | 已完成 | 机器强制分层规则 | ARCHITECTURE_REVIEW P0-1 |
| 2 | **清理 concrete `*chat.Client` 泄漏** | 已完成 | 定义 `ChatClient` port，runtime/provider 依赖抽象；chat 协议原语保留共享 | ARCHITECTURE_REVIEW §2.2 |

### P1 — 下一批

| # | 项 | 工作量 | 价值 |
|---|---|---|---|
| 3 | **artifact-bearing tool**（扩 `chat.Tool.Call` 返回类型） | 中（breaking） | 图像/表格/PDF 工具带外传 typed 对象，不用 base64/副作用 |
| 4 | **tool-call 级事件**（`ToolCallRequest/Response` 事件类型） | 低-中 | 工具遥测完整性（当前 LLM/embedding 已有，tool 仍缺） |
| 5 | **`autonomy.Autonomy` → `autonomy.Router`** 改名 | 已完成 | 消 stutter（ARCHITECTURE_REVIEW + REFACTORING §1） |

### P2 — 有真实场景时

| # | 项 | 价值 |
|---|---|---|
| 6 | GOAP unknown-condition 惰性分支 | 仅当出现"评估某 condition 才知道选哪个 plan"的真实场景 |
| 7 | `ProcessStore` tick-by-tick 实时更新模式 | 仅当 snapshot 不够（需要跨 tick resume 中间态） |
| 8 | loop inspector/transformer（history 改写/压缩） | 仅超大 tool 集场景 |

### 仍然故意不做

| # | embabel 有但 lynx 不抄 | 原因 |
|---|---|---|
| A | `ToolInjectionStrategy`（Unfolding/Matryoshka/ToolChaining） | planner 重规划 obviate 大半；仅 >20 tool 场景值 |
| B | `ux/form` 表单层 | UX 出 agent scope |
| C | `OperationScheduler` | lyra 调度 |
| D | A2A server / Shell / Skills / ONNX in agent | lyra / `agents/` 外模块承担 |
| E | Spring autoconfigure / starter 矩阵 | Go 库不是 framework |
| F | 多前端（注解 / DSL / builder） | Go 无注解；当前 programmatic 已够 |
| G | `@Tracked` AOP + Micrometer / Spring Observation | OTel 已是 Go 事实标准 |
| H | ArchUnit → 加 `arch_test.go` 等价物 | 但要（见 P0）；不是抄 ArchUnit，是补齐机器防腐 |

---

## 10. 一句话定档

**lynx/agent 在 2026-06 到 2026-07 窗口完成了从"高保真原型"到"生产就绪库"的关键一跃：tool loop 全面重写（并行+限流+检环+恢复+park/resume）、Supervisor 补齐、best-of-N 修正、metrics + per-call 事件 + 持久化 SPI 三件套落地，并补齐机器防腐、Router 命名、ChatClient port。旧对比中"embabel 决定性领先"的 ToolLoop 维度已反转；embabel 的优势回归到其 framework 天生态位（生态广度/REST/Shell/A2A/form/RAG ingestion 深度）。lynx 的剩余真 gap 集中在低优先级能力差（artifact 通道 / tool-call 事件 / ToolInjectionStrategy 等）。继续巩固 library-kit 哲学，不追 framework 能力。**

---

*对比结束。双方 HEAD 截至 2026-06-18（lynx `b02185f` / embabel `e3392133b`）。本文由源码级重新通读产出，关键"Δ since old baseline"断言均以 verified anchors + 第一手源码核实为据。*
