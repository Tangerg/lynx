# 双向 Feature-Diff：embabel-agent vs lynx/agent

> **基线**：lynx HEAD `b02185f` / embabel HEAD `e3392133b`（2026-06-18）。
> **定位**：本文是"**一眼看谁有什么**"的查阅清单，与 [`EMBABEL_DEEP_COMPARISON.md`](EMBABEL_DEEP_COMPARISON.md)（7 维度源码级深度对比）互补 —— 那份回答"两边各自怎么做的、为什么"，本文回答"哪些功能只有一边有"。组织哲学单轴见 [`EMBABEL_ORGANIZING_PRINCIPLES.md`](EMBABEL_ORGANIZING_PRINCIPLES.md)。
> **方法**：源码级核实。所有"embabel 有"项均经 embabel 当前 HEAD 代码确认（`ux/form` 9 文件、`OperationScheduler`、`ContextRepository`、SpEL、`ValidationPromptGenerator` 均在）；所有"lynx 有"项均经 lynx 当前 HEAD 代码确认。双方都有的形态差异不列入本文（见 DEEP_COMPARISON §C）。
>
> **三类标签**：
> - **真能力差** —— lynx 可考虑补，有实际价值
> - **by-design skip** —— lynx 库哲学决定不抄，抄了反而违背 [`../../DESIGN_PHILOSOPHY.md`](../../DESIGN_PHILOSOPHY.md) / [`../CLAUDE.md`](../CLAUDE.md)
> - **framework 天生态位** —— library 追不上也不该追，是 framework 红利

---

## A. embabel 有、lynx 没有

### A1. 真能力差（lynx 可考虑补）

| # | embabel 功能 | lynx 现状 | 优先级 | 备注 |
|---|---|---|---|---|
| 1 | **artifact-bearing tool** —— sealed `Result { Text / WithArtifact(content, artifact) / Error }`，typed 域对象带外传不进 LLM 上下文，可 sink 到黑板 | `chat.Tool.Call` 返回 `(string, error)`，artifact 走 base64/副作用 | 中（breaking） | 图像/表格/PDF 工具场景；扩返回类型是 breaking change |
| 2 | **ToolInjectionStrategy**（Full/Chained/Unfolding/Matryoshka/ToolChaining）—— 大 tool 集（>20）渐进披露 | tool 集冻结 | 低 | planner 重规划 obviate 大半；仅超大 tool 集场景值 |
| 3 | **ToolLoop inspector/transformer** —— 只读观测 + history 改写（压缩/摘要） | 仅 per-tool OTel span | 低 | 中低优先级 |
| 4 | **tool-call 级事件**（`ToolCallRequest/Response` 带 correlationId） | 有 LLM/embedding per-call 事件，缺 tool-call 级 | 低-中 | 工具遥测完整性 |
| 5 | **AgentValidationManager 多层 + ValidationPromptGenerator**（structure/path-to-completion 分型 + LLM 驱动校验 prompt） | 单 `AgentValidator` + 内建 reachability | 中 | deploy 时一次报全问题；LLM 校验是 embabel 独有 |
| 6 | **ContextRepository**（跨 run 工作记忆持久化 SPI） | 仅 `ProcessStore`（进程快照），跨 run context 靠 `chathistory` | 低 | `chathistory` 部分承担 |
| 7 | **GOAP unknown-condition 惰性分支**（1 unknown → True/False variant 各规划 → 评估 → commit） | 全无 | 低 | embabel >1 unknown 也 TODO，实用面窄 |
| 8 | **ArchUnit 机器强制架构测试**（1.4.0） | 已有 `agent/internal/arch/arch_test.go` | 已完成 | 形态不同：lynx 用 Go 测试编码库内依赖规则，embabel 用 ArchUnit 编码 framework 分层规则 |
| 9 | **事件可往返**（Jackson 多态 round-trip） | 单向 lossy JSON | 低 | by-design 倾向（往返 = 内存里 type-assert） |
| 10 | **事件粒度**（~32 vs 17：`ObjectBound`/`ProgressUpdate`/`Ranking`/`RagRequest` 等） | 17 个，缺 tool/ranking/progress 层 | 低 | 部分由 `models` 中间件 + lyra 合成 |

### A2. by-design skip（lynx 库哲学，不该抄）

| # | embabel 功能 | 不抄的理由 |
|---|---|---|
| 11 | **OperationScheduler**（Pronto/Delayed/Scheduled + rate-limit） | lyra 调度；Go 用 `context.WithDeadline` + `time.Timer` 显式即可 |
| 12 | **`ux/form` 表单层**（Form + 11 控件 + FormBinder + SimpleFormGenerator，9 文件） | UX 出 agent scope；服务端表单绑定不是库的事 |
| 13 | **Spring autoconfigure / starter 矩阵**（21 starter） | Go 库不是 framework，无 DI 容器传统 |
| 14 | **`@Tracked` AOP + Micrometer / Spring Observation** | OTel 已是 Go 事实标准，再套抽象无意义 |
| 15 | **多前端**（注解 / Kotlin DSL / builder 三条漏斗） | Go 无 runtime annotation；programmatic builder 已够 |
| 16 | **A2A server / Shell / Skills / ONNX in agent module** | 在 lyra / `agents/` 外模块承担 |
| 17 | **REST/SSE 端点 in agent**（#1695 `AgentProcessController`） | lyra 承担 RPC/HTTP |
| 18 | **SpEL 表达式语言**（`SpelLogicalExpression`） | Go 闭包 + `Determination` 三值逻辑更类型安全 |
| 19 | **`DataDictionary`/`DomainType` 知识图谱 schema 层**（relationship/cardinality/YAML 动态类型/classpath 扫描） | lynx 刻意剪枝 —— 无消费者，YAGNI |
| 20 | **Action 5 契约继承**（`DataFlowStep`/`ConditionAction`/`ActionRunner`/`DataDictionary`/`ToolGroupConsumer`） | ISP 拆 + 元数据 struct 集中，类型层级更扁 |
| 21 | **`AgentProcess` IS-a Blackboard god interface**（50+ 方法） | 这是 embabel 的债不是优势；lynx HAS-a 更干净 |
| 22 | **`LlmService`/`AutoLlmSelectionCriteriaResolver` SPI** | lynx 直接用 `chat.Client` + `ChatClientProvider`，不抽 LLM 路由层 |
| 23 | **多 distinct goal 类型限制**（#1703） | 这是 embabel 新加的**限制**，lynx `Goals []*Goal` 更宽松 —— lynx 反而更自由 |

### A3. framework 天生态位（library 追不上，也不该追）

| # | embabel 功能 | 性质 |
|---|---|---|
| 24 | **RAG ingestion 深度**（entity-graph 抽取、chunk 合并/压缩、RSS ingestion，~22k LOC） | embabel RAG 机器远更深；lynx `rag` ~1.6k 走流水线，广度靠 `vectorstores` 27 后端 |
| 25 | **生态广度**（~43 module vs 1） | starter/autoconfigure 全家桶是 framework 红利 |
| 26 | **代码覆盖率 73% 量化** | 工程化指标，非能力 |

---

## B. lynx 有、embabel 没有

### B1. 独有算法/能力（embabel 全无，已核实）

| # | lynx 功能 | 证据 | embabel |
|---|---|---|---|
| 1 | **HTN planner**（`Task`/`Method`/`Library`，递归分解上限 64） | `planning/planner/htn/` | 全仓库 0 处 HTN（grep 确认） |
| 2 | **后向 STRIPS 相关性剪枝**（不动点回归，A* 搜索**前**剪枝） | `planning/planner/goap/relevance.go` | 仅 post-hoc `prune`（要先跑完整 A*） |
| 3 | **LLMPlanRanker**（LLM 评估候选 plan） | `runtime/autonomy/plan_ranker.go` | 无任何 plan-ranking |
| 4 | **reactive 0-progress 守卫**（`progress==0 → continue`） | `planning/planner/reactive/reactive.go:116` | `UtilityPlanner` 无守卫，NIRVANA + 非自禁动作会死循环（其自家注释承认） |
| 5 | **LoopDetection**（SHA256 round-signature + 滑动窗口，固定点检测，先于 max-iter 触发） | `agent/toolloop/loop_detection.go` | 无 |
| 6 | **ConcurrentTool 资源键冲突检测**（`ConcurrencyKey` + `segmentEnd` 分组 + `maxConcurrentToolCalls=8`） | `agent/toolloop/concurrency.go` / `agent/toolloop/invoker.go` | `ParallelToolLoop` 无资源键（假冲突无法避免） |
| 7 | **tool-error recovery as framework default**（默认开，不用配） | `agent/toolloop/invoker.go` | per-SPI policy，要配 |
| 8 | **deploy-time 可达性校验**（`ValidateAgent` + `checkGoalsReachable` + `runAgentValidators` 三层） | `runtime/platform_deploy.go` | 裸 map put，STUCK 时才暴露 |
| 9 | **kill-race invariant 完整族**（first-terminal-wins / idempotent KillProcess / cancel-park race + budget leak / reject concurrent HITL parks / drop leaked child） | commits `a2c17da`/`15490f5`/`50a996f`/`9efee00`/`ce59304` | 基于 `InterruptedException` + `stop()` flag |
| 10 | **spawn 梯度 4 档**（`SpawnChild`/`SpawnChildProtectedOnly`/`SpawnChildFresh`/`RunFresh`） | `runtime/child.go` | 统一委派 + 共享黑板，不形式化隔离 |
| 11 | **workflow over sub-agent**（`Sequence`/`Parallel`/`Loop` over sub-agent + 分支隔离） | `workflow/` | `ScatterGather`/`RepeatUntil` 只 over **闭包**，无 over-sub-agent 组合 |
| 12 | **async/background subagent**（`SpawnChildAsync` + `AsBackgroundChatTool`） | commit `44ce90c` | 无对等 |
| 13 | **流式 tool loop 自驱**（`executeStreamRecursively` 注入合成 ToolMessage chunk） | `agent/toolloop/` | `LlmMessageStreamer` 文档明说流式下不能自驱、只能观测 |
| 14 | **child EventListener 继承**（child spawn 时继承父 listeners） | commit `8ac8fc9` | 无对等 |

### B2. 独有抽象形态（更整洁，embabel 无对等形态）

| # | lynx 形态 | embabel 对应 | 差异 |
|---|---|---|---|
| 15 | **`collectExtensions[T]` 单类型分发**（1 marker + 12 子接口，加能力不改 dispatch loop） | 30+ 异质 Spring SPI（各自 `@Bean` 注入） | OCP 干净度 lynx 完胜 |
| 16 | **ISP 拆 Blackboard**（`Reader`/`Writer`/`Spawn`，编译期读写隔离） | 单巨接口 ~25 成员 | lynx `ConditionEnv` 拿 `Reader` 编译期保证 condition 不能写 |
| 17 | **Process HAS-a Blackboard**（~22 方法，lifecycle 藏 unexported `*AgentProcess`） | `AgentProcess` IS-a Blackboard（50+ god interface） | 教科书级 ISP/SRP |
| 18 | **`Interrupt[R]` 单泛型统一 HITL**（`Interrupt[R](ctx,key,val) (R,bool,error)` + idempotency guard） | sealed `AwaitableResponse` 子型层级（Confirmation/FormBinding/TypeRequest） | 1 泛型 vs 3+ sealed 子型 |
| 19 | **Planner 1-method stateless 接口**（`PlanToGoal` + 包级辅助） | 5-method 有状态（`worldState()`/`prune()`）+ 3 泛型参 | 并发安全 by construction |
| 20 | **`Determination` Unknown=0 零值友好**（`int8`，未 init 的 map 项天然 Unknown，GOAP 依赖此） | `enum {TRUE,FALSE,UNKNOWN}` | Go idiom，JVM enum 复制不了 |
| 21 | **`ChatClientProvider` extension**（per-process model override 走 Extension 类型分发） | `LlmService`/`AutoLlmSelectionCriteriaResolver` SPI | 形态不同，lynx 更轻 |

### B3. 独有生态广度

| # | lynx | embabel |
|---|---|---|
| 22 | **41 LLM providers**（`lynx/models/` 41 个 provider dir，含 audio/image/TTS） | ~13 starter |
| 23 | **27 vector store backends**（`lynx/vectorstores/`） | 实质 1-2（Lucene/Neo4j） |

---

## C. 不列入本文的"双方都有"（见 DEEP_COMPARISON §C）

避免误判，这些**不是**单方独有，形态差异已在 [`EMBABEL_DEEP_COMPARISON.md`](EMBABEL_DEEP_COMPARISON.md) 详述：

Reasoning/thinking 支持 · per-process LLM 选择 · 持久化 · Supervisor · best-of-N · metrics · per-call 事件 · native structured output · HITL park/resume · 并行 tool 执行 · max-iter cap · ToolNotFound 策略 · EmptyResponse 策略 · Goal 否决 · Ranker · MCP 暴露 · agent-as-tool。

---

## 一句话总结

**lynx 独有集中在"规划算法 + tool loop 健壮性 + 并发/隔离不变式 + 抽象卫生"四线（14 个真独有能力 + 7 个更整洁形态 + 2 项生态广度）—— 都是 embabel 全无或 god interface 的地方。embabel 独有集中在"framework 生态 + 表单 + 调度 + schema 层 + 校验 manager"—— 其中只有 ArchUnit 机器防腐（#8）是 lynx 真 debt，artifact 通道（#1）和 validation manager（#5）是值得考虑的真能力差，其余全是 by-design 库哲学或 framework 天生态位。**

---

*双方 HEAD 截至 2026-06-18（lynx `b02185f` / embabel `e3392133b`）。本文所有"独有"断言均经源码级核实。*
