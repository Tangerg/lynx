# Agent Framework 同类框架对比

> 状态：外部证据快照，不是架构、决策或进度记录
> 建立日期：2026-08-08
> 对比对象：`embabel-agent` @ `e2d7b987c`（2026-08-06）、`Koog` @ `164f57a71`（2026-06-30）
> 本地基线：`Scope Agent` Baseline 9（P10 进行中），仓库 @ `6714ef84f`

本文只记录同类框架的可核实事实、与本模块的差异，以及由此暴露的能力缺口。它不定义目标架构、不修订已接受决策、不追踪阶段进度。

- 目标架构见 [`ARCHITECTURE.md`](ARCHITECTURE.md)。
- 决策及其理由见 [`DECISIONS.md`](DECISIONS.md)。
- 旧能力裁决见 [`CAPABILITY_LEDGER.md`](CAPABILITY_LEDGER.md)。
- 公共合同见 [`API_BASELINE.md`](API_BASELINE.md)。
- 阶段与执行事实见 [`EXECUTION_PLAN.md`](EXECUTION_PLAN.md)。

使用约束：

1. 本文中的"缺口"和"候选动作"只是**待裁决输入**。任何一条要落地，必须先在 `DECISIONS.md` 追加 ADR，再改架构与代码；不得直接引用本文修改生产合同。
2. 所有规模、版本和文件路径都是上述提交时点的**快照**，会随对方演化失效。核对差异时以对方仓库当时状态为准，不以本文措辞为准。
3. 参考对手**只取思想，不作命名锚**。本文出现的外部类型名仅用于定位证据，不构成本模块的命名依据。

---

## 1. 三方坐标

| | 中心命题 | Process 的本质 |
|---|---|---|
| embabel | 给定目标与动作，由系统规划达成路径 | 活的 JVM 对象，显式拒绝序列化 |
| Koog | 用类型安全的图描述 LLM 工作流，并可回滚重放 | 协程 + 图游标，checkpoint 可序列化 |
| Scope Agent | 任意执行策略共享同一套可暂停、可快照、可治理的生命周期 | 状态机，snapshot 是一等公民 |

三者不在同一条轴上竞争。embabel 的差异化在**自动规划**，Koog 的差异化在**上手成本与工程完成度**，Scope Agent 的差异化在**恢复语义与治理边界**。

复杂度—收益轴的相对位置：

```
「好上手」 <──────────────────────────────────> 「好托管」
   Koog             embabel                   Scope Agent
 一行起步         注解 + GOAP 自动规划        显式内核 + 可恢复树
 1.0.0 稳定       有线上产品                  未接生产流量
```

---

## 2. 事实底座（快照）

| | embabel-agent | Koog | Scope Agent |
|---|---|---|---|
| 出品方 | Embabel Pty Ltd（Spring 创建者） | JetBrains | 本仓库 |
| 语言与运行时 | Kotlin + Java，Spring Boot | Kotlin Multiplatform：JVM / JS / WasmJS / Android / iOS | Go |
| 主源码规模 | 103,785 LOC / 1,588 文件 | 115,737 LOC / 897 文件（`*Main` source set） | 生产 18,781 + 测试 14,715 + 示例 3,135 LOC |
| 核心模块规模 | `embabel-agent-api` 485 文件 | `agents-core` 23,171 LOC | 7 个生产 package |
| 成熟度 | 1.5.0-SNAPSHOT，有线上产品 | 1.0.0 正式版，semver + 版本策略文档 | Baseline 9，尚未承载生产流量 |
| 运行时依赖 | Spring Boot、Spring AI、Micrometer、Jackson、Spring Retry | kotlinx-coroutines / serialization / datetime | `chatclient`、`tool`、`core/chat`；OTel 隔离在独立 adapter |
| 恢复能力 | 无 | 有：per-run checkpoint + rollback | 有：Snapshot + TreeSnapshot |
| 生态附件 | rag / mcp / a2a / shell / skills / onnx / observability / starters（18 个 Maven 模块） | a2a(3) / acp / mcp(3) / longterm-memory(+aws) / chat-history(jdbc,aws) / opentelemetry / trace / tokenizer / sql / rag / ktor / spring-boot / cli（17 个 feature 模块） | 全部是平级 sibling module，不进 Framework |

---

## 3. 维度对比总表

| 维度 | embabel | Koog | Scope Agent |
|---|---|---|---|
| 中心抽象 | GOAP 规划是内核字段 | 有向图 + 类型化边 | Definition/Execution 窄腰，策略平权 |
| 执行循环 | 阻塞 `tick()`，副作用同栈 | 协程 node 循环，副作用在 node 内 | `Step` 纯归约 + Effect 外置，prepare/finalize |
| 主状态 | 全局可变 Blackboard | `AIAgentContext`（prompt + storage） | 策略私有 `ExecutionState` 信封 |
| 恢复边界 | 无 | node 完成后 | last-stable 与 prepared-step |
| 副作用重放 | 不适用 | at-least-once + 工具回滚补偿 | 按 EffectID replay contract；不可证明即停为 unknown |
| 恢复单位 | 不适用 | 单个 run | 单 root 或完整 Process tree |
| 子 Agent | Tool → 同步嵌套 run | Tool → 同步嵌套 run | Framework Effect → 真 child Process |
| 预算 / 能力 | USD Budget + 早停策略 | 仅 `maxAgentIterations` | Budget 划拨 + CapabilitySet 衰减 + TreeLimits |
| HITL | Awaitable 放黑板 + 回调 | 同步 tool 回调 | Engine 铸造 WaitID + Signal，可快照 |
| 扩展点 | Spring DI + 事件监听 | Feature pipeline，**可改控制流** | 三个语义不同的小接口，listener 无 veto |
| 可观测性 | Micrometer 埋进内核 | Feature → OTel（含 Langfuse / Weave exporter）+ 远程调试器 | Kernel 零 OTel import，独立 adapter |
| 重试 | 每 Action 带 QoS RetryTemplate | 内建 retry | 默认执行一次，无 retry layer |
| 装配 | classpath 扫描 + 注解 | 显式 builder / DSL | 显式装配，零全局容器 |
| 作者体验 | `@Agent` + `@Action` 三行 | `AIAgent(...).run("hi")` 一行 | Descriptor + Definition + Dispatcher + Deployment + EngineConfig |
| 验收纪律 | 常规单测 + Sonar | 常规单测 + 多目标 CI + 用户侧 mock 工具包 | API/wire digest 冻结、architecture DAG、fuzz、race、独立消费者示例 |
| 兼容承诺 | 演进中 | semver + checkpoint 旧格式双读迁移 | 显式 breaking，禁止双读 |

---

## 4. 逐维度详述

### 4.1 中心抽象

**embabel**：GOAP 是唯一主干。`AgentProcess` 接口本身带 `planner`、`goal`、`lastWorldState` 字段；`ProcessOptions.plannerType` 默认 GOAP；每个 tick 重新决定世界状态、重新规划、执行计划首个 Action。其 `Autonomy` 进一步用 LLM Ranker 给全部 Goal 打分，超过置信阈值就动态合成一个单目标 Agent，并按输入做 A\* 剪枝。

**Koog**：没有中心规划器。图拓扑由作者写死，`agents-planner` 是可选模块。

**Scope Agent**：GOAP 被隔离进 `planning/`，共同 Process 明确不拥有 Goal / WorldState / Plan。这是与 embabel 最根本的分岔：它使 Interaction（ReAct）不必伪装成 Action 序列，代价是失去 embabel 那种"给能力和目标、系统自己找路"的自动性。

### 4.2 执行循环

**embabel** 的循环是阻塞的：`run()` 反复 `tick()`，`tick()` 内直接执行模型调用、工具调用甚至 `Thread.sleep`。当前进程通过 ThreadLocal 隐式全局可见。

**Koog** 的循环沿边行走，节点是 suspend 函数，模型与工具在节点内同步执行：

```
while (true) {
    iterations++ > maxAgentIterations → throw
    nodeOutput = currentNode.execute(context, currentInput)   // 模型 / 工具在此发生
    edge = currentNode.resolveEdge(context, nodeOutput)
    edge == null → (finish ? break : throw AIAgentStuckInTheNodeException)
    currentNode = edge.toNode
}
```

Subgraph 本身也是 node，可递归嵌套，并可在层内覆盖 tools / model / params，或以 `freshHistory` 只继承 system 消息。

**Scope Agent** 把状态推进与外部效果彻底分离：Step 只产生候选状态、Transition 与 Effect 意图，禁止读 clock / random / 全局状态；Engine 先原子记录 prepared step，再调度 Effect，最后原子 finalize。

差异后果：

- Koog 的拓扑显式、可绘制，这是它相对 Scope Agent 最直观的优势；Scope Agent 只有 `workflow` 的 Stage 序列具备同等显式度，Interaction 与 Planning 的控制流藏在 phase 状态机内。
- Koog 的 node 允许任意 I/O，因此不具备纯归约保证，无法重放、无法对 Step 做 fuzz，恢复粒度只能到 node。
- "卡住"在 Koog 是抛异常，在 embabel 是 `STUCK` 状态码，在 Scope Agent 是 Planning 的 Output 值。

### 4.3 状态所有权

**embabel** 的 Blackboard 是全局可变对象袋：取值靠类型简单名字符串匹配加反射遍历父类型，默认绑定名是字面量 `"it"`，语义为"最后一个满足该类型名的对象"，还支持用反射从黑板自动拼装聚合对象。`AgentProcess` 直接委托实现 `Blackboard`——进程就是黑板。

**Koog** 的 `AIAgentContext` 由 LLM 上下文（prompt / messages / model / tools）与类型安全 key-value `storage` 组成。比 Blackboard 干净，但 storage 仍是袋子，且被 feature 用作跨切面通信通道。

**Scope Agent** 的 Kernel 只见信封 `ExecutionState{Kind, SchemaVersion, Payload}`，对 Payload 无导入权也无解释权；Blackboard 在裁决台账中被整体移除。

代价必须承认：embabel 里两个 Action 通过黑板隐式通信极其顺手，Scope Agent 强制显式 Input/Output schema 后顺手程度显著下降，换来的是可验证性与可恢复性。

### 4.4 持久化与恢复

这是与 Koog 唯一正面对话的维度。

**embabel**：没有进程快照。`AgentProcess` 上挂着拒绝序列化的 Jackson 序列化器，仓内唯一实现是带窗口淘汰的内存仓库，进程持有活的 planner 实例、平台服务与任意 JVM 对象。跨运行持久化走旁路的 `Context` / `ContextRepository`——那是重新播种，不是恢复。JVM 重启等于在途进程全部蒸发。

**Koog**：`Persistence` 是一个可选 feature（约 1,657 LOC）。

- 快照内容：`checkpointId`、`version`、`messageHistory`、`llmParams` / `llmModel` / `tools`、`storage`(JSON)、`agentIterations`，加上图形态的 `nodePath` + `lastOutput`，或 planner 形态的 `executionPoint` + `state` + `plan`。
- 装载点：拦截"节点执行完成"自动存档，拦截"策略完成"写 tombstone。
- 恢复：拦截"策略开始"回滚到最近 checkpoint，再强制执行点跳到目标节点。
- 存储 provider：内存 / 文件 / JDBC；version 成链；提供旧格式迁移序列化器。

**Scope Agent**：Snapshot v6 / TreeSnapshot v4，捕获点只有 last-stable 与 prepared-step 两个原子边界；prepared snapshot 完整包含候选状态、拟消费 Signal 范围、EffectID、冻结 payload 与已有 settlement。

关键差异在边界位置：

| | Koog | Scope Agent |
|---|---|---|
| 捕获点 | 节点完成后 | last-stable 与 Effect dispatch **之前** |
| 崩在模型/工具调用中间 | 回到上一节点，整节点重跑（含已发生副作用） | 沿原 EffectID 与冻结 payload 继续，只按 dispatcher replay contract 重投 |
| 不可证明可重放时 | 依赖工具回滚注册表补偿 | 保持 unknown、可观察、待显式裁决 |
| 持久边界 | feature 直接写 provider | `PreparedStepAcknowledger` 握手；未启用则诚实标记只恢复到最后已持久化 snapshot |
| 恢复单位 | 单个 run | 单 root 或完整 TreeSnapshot；成树后禁止只恢复父级或把 child 当新 root |
| 序列化失败 | 运行期（缺类型令牌时抛出） | 构造/捕获期显式失败 |

概括：**Koog 是 at-least-once 加补偿，Scope Agent 是"要么证明可重放，要么停下来让人裁决"。** 两条路都成立，Koog 对用户更友好，Scope Agent 对正确性更诚实。

### 4.5 Planner

三者中最接近的一处。Koog 的 planner 抽象是 `initializeState` / `buildPlan` / `executeStep` / `isPlanCompleted` / `provideOutput` 五个 hook，加一个可持久化的执行点枚举（计划已建 / 步骤已执行 / 完成度已评估），用于恢复时跳过已完成阶段——与 `planning` 的 phase 状态机形状高度相似。Koog 提供 GOAP、LLM 规划器、带批评者的 LLM 规划器三个实现。

三处实质差异：

1. **执行 vs 声明**：Koog 的 `executeStep` 直接执行 Action 副作用；Scope Agent 的 Step 只声明 Effect。
2. **重观测确认**：Koog 判完成只查目标条件，不验证 Action 的预测效果是否成真；embabel 也只设置"已运行"条件。`planning` 强制在每个 Action 后重新观察，预测未兑现即记为 unconfirmed 并把该 Action 加入排除集。**这一点 Scope Agent 严于两个对手**，且正是 GOAP 在动态环境下的核心正确性来源。
3. **条件表示**：Koog 的目标与前置条件是闭包，因而计划只能序列化成 Action 名列表、状态靠类型令牌序列化；`planning` 的 Condition / Truth / WorldState 是规范化值对象，可 JSON、可 fuzz，成本函数 panic 也被收敛成确定错误。

### 4.6 子 Agent 与组合

| | embabel | Koog | Scope Agent |
|---|---|---|---|
| 本质 | Tool 触发同步嵌套 run | Tool 触发同步嵌套 run | Framework Effect 启动真 child Process |
| 身份 | 父名与子 id 字符串拼接 | 父 id 加调用序号字符串拼接 | ProcessID / RootProcessID / ParentProcessID / ChildKey |
| 上下文 | 复制父黑板全量 | 共享或新建会话 | 按任务投影 typed Input |
| 预算 | 共用父 ProcessOptions | 无 | 从父剩余永久划拨 |
| 能力 | 无 | 无 | 单调衰减子集 |
| 等待 | 阻塞父线程 | 协程挂起 | 父进 Waiting，child settlement 作为 Signal 唤醒 |
| 树恢复 | 无 | **无**：checkpoint 按 run 组织，父 checkpoint 不含子 agent 状态 | TreeSnapshot 全树原子 restore |
| 取消 | 递归遍历仓库下发 | 协程 scope 级联 | 控制意图 + 终态矩阵 + 禁孤儿 + 明确的 Await 线性化点 |

Koog 在此有一处实质缺口：父 checkpoint 不覆盖子 agent，从父 checkpoint 恢复会让子 agent 的全部工作重来。这与 Scope Agent「一旦成树，恢复单位必须是完整 TreeSnapshot」正面冲突，且证据倾向 Scope Agent 的取舍正确。

### 4.7 限制、预算与治理

Koog 的全部执行闸门是一个默认 50 的最大迭代数；tokenizer 是统计而非闸门；没有成本、墙钟、递归深度、fan-out、活跃子进程数限制，也没有部署目录、版本路由或启动准入。embabel 至少有 USD 预算与早停策略族。

Scope Agent 拥有 Limits、TreeLimits（深度 / 子数 / 活跃子数 / 树内总进程数）、Budget 划拨、CapabilitySet 衰减、ProcessAdmitter，以及 Platform 的 `(name, SemVer)` 槽位治理与 exact binding 历史。

这一维 Koog 最弱，原因是它假设 agent 在作者自己的进程内运行，不承担多租户治理。

### 4.8 HITL

embabel 把 Awaitable 放进黑板并以回调改写进程状态；Koog 提供的是普通同步工具与确认回调。二者都没有"进程停在某个可寻址的等待目标上、回答被路由回来"的语义——Koog 的 checkpoint 能让执行事后从某点重来，但那不是等待。

Scope Agent 的 WaitID 由 Engine 铸造，Execution 不能自行生成外部等待身份；同一 SignalID 重复提交只消费一次，过期与错目标输入确定失败，Waiting 状态可快照可恢复。

**这是 Scope Agent 相对两个对手都领先、且对产品级审批与澄清场景属于刚需的一维。**

### 4.9 扩展点哲学

这里存在一处真正的哲学分歧。

Koog 的 feature pipeline 提供大量拦截点（策略、节点、子图、模型调用、工具调用、计划创建、步骤执行……），而且**拦截器可以改变执行**：`Persistence` 正是在"策略开始"拦截点回滚并强制执行点，通过往共享上下文塞标记来劫持控制流，节点循环里以"发现被塞入的数据即中断"响应。

Scope Agent 明确站在对面：观察接口无错误返回，"返回值既不会改变事实，也不应制造可否决执行的误解"；横切点按真实消费位置拆成三个语义不同的小接口（启动准入 / 观察 / 预调度持久握手），拒绝合并成 Policy/Guard/Middleware 近义层。

评价：Koog 的模型功能更强——checkpoint/rollback 能作为可插拔 feature 存在正以此为前提；代价是没人知道当前控制流被几个 feature 改过，共享上下文的魔法 key 是隐式耦合。Scope Agent 的模型可推理性更强但更死板：若将来需要回滚，只能做进 Kernel。当前证据不支持改变立场——`WaitingSubtreeCancellationPlan` 那种"先纯计算、再同一栅栏内原子 apply"已经给出了更干净的等价物。

### 4.10 可观测性

embabel 把 Micrometer 观测直接埋进抽象进程基类，并散落带 emoji 的日志。Koog 把观测做成 feature，内置 OpenTelemetry 及 Langfuse、W&B Weave exporter，另有 trace feature 与远程调试器服务端（可外接调试器实时接收事件流）。

Scope Agent 的 Kernel 由 architecture gate 禁止任何 OTel import，事实集合固定为 Process / Signal / Step / Effect / Delta 五类，每个 Event 自足携带 ProcessID、exact DeploymentRef、ProcessRelation、Step/Effect identity 与 attempt/committed 相位；`otel` 是只消费 Event 的独立 adapter。Delta 与 Event 严格分离，且完成 Output 必须独立导出、不得由 Delta 拼接。

### 4.11 类型系统与 schema

embabel 的类型是字符串：绑定按类型简单名匹配，Action 的输入 schema 靠反射从 IoBinding 反推。Koog 用类型令牌加 kotlinx.serialization，节点与边在编译期检查类型衔接，是三者中静态检查最强的。Scope Agent 以 JSON Schema 作为权威结构合同并进入 Deployment digest，泛型只留在类型擦除边缘的 adapter。

### 4.12 重试与副作用

embabel 每个 Action 带 QoS 重试模板，重试前把效果条件清回假。Koog 内建重试与工具回滚注册表。Scope Agent 默认执行一次，不重复实现 provider SDK 已有的重试，只在明确幂等或有补偿语义时配置；Engine 为 Effect 提供稳定身份，但不据此宣称外部业务副作用具备事务或幂等语义。

这是有意取舍：两个对手的用户默认能免费获得瞬时失败自愈，本模块的使用者必须自己想清楚。

### 4.13 装配与发现

embabel 依赖 Spring DI 与 classpath 扫描，注解元数据读取器承担建模。Koog 用显式 builder 与类型安全 DSL，无扫描。Scope Agent 零全局容器、零扫描、零注解，Catalog 是不可变内存快照，活跃槽位键为名称加语义版本，同槽位不同 digest 必须显式替换，历史 binding 只增不误删且只用于精确恢复。

### 4.14 作者体验

这是 Scope Agent 明显落后的一维。

- embabel：三个注解、两个函数即成一个可规划 Agent，前置条件与效果由参数与返回类型推导；另有 Kotlin DSL、builder 与 YAML 四套作者入口。
- Koog：最简形态是构造 agent 后直接 `run`，复杂了再写图 DSL；且提供**给用户的**测试工具包（mock 模型应答的 DSL）。
- Scope Agent：需要 Descriptor（含输入输出 schema）、Definition、Dispatcher、Deployment、EngineConfig（Limits / TreeLimits / Capabilities / resolver / admitter / listeners）。

Koog 证明了"好上手"与"能恢复"并不互斥：它把 checkpoint 做成可选 feature，不装的用户完全无感。本模块把恢复做成内核骨架，正确性上限更高，但代价是每个使用者都要支付内核复杂度税，无论是否需要恢复。

### 4.15 工程验收与兼容承诺

Scope Agent 在此维度是压倒性的：七个 package 的完整文档输出做 SHA-256 冻结，五份 wire digest 独立冻结并带覆盖守卫，全图 architecture test 锁定 package 集合与允许边，加上 fuzz、race 与八个不共享测试 helper 的独立消费者示例。两个对手都没有等价机制。

反向事实同样重要：Koog 已是 1.0.0 且对 checkpoint 旧格式做双读迁移，embabel 有线上产品；**Scope Agent 尚未承载生产流量**（P10 进行中，Host 仍运行原框架实现）。当前所有正确性证据来自自有测试与示例。

### 4.16 生态广度

embabel 与 Koog 都把 RAG、MCP、A2A、记忆、聊天历史、可观测导出器做进各自发行版。Scope Agent 的对应能力全部是仓内平级 sibling module，Framework 不认识它们——这是本模块刻意的边界，不构成缺口。

---

## 5. 继承、丢弃与拒绝

### 5.1 从 embabel 保留的思想（形态已全部重写）

- GOAP 的核心洞察：目标可机器验证、存在多条路径、Action 具备可声明的前置/效果/成本、环境会变，因而每步后重观测重规划。
- Condition / Truth / WorldState 的三值语义。
- 子进程即递归：Definition 可相同，Process 永远不同。
- Deployment 具备版本与 digest 并进入恢复身份。
- 重规划排除集，用于避免刚触发重规划的 Action 立刻被再次选中。

### 5.2 治本式丢弃清单

| 丢弃项 | 理由 |
|---|---|
| Blackboard | 全局可变袋加字符串类型匹配，既不可恢复也不可验证 |
| ThreadLocal 当前进程 | 隐式全局状态 |
| HTN / Utility / Reactive planner | 无真实消费者；Reactive 是单步规则选择，既非 ReAct 也无独立推进/恢复语义 |
| Supervisor 独立 Strategy | 已证明可由 Interaction + Delegate + Workflow Map 组合 |
| 通用 Action-to-Tool adapter | 预测元数据推不出模型可理解的参数 schema |
| Action QoS 重试 | 与项目级"不建 retry layer"裁决冲突 |
| USD 预算与价格表 | Framework 只发中性 usage |
| STUCK 共同状态 | 那是 Planning 的 Output，不是生命周期状态 |
| conversation / compaction | 产品历史归 Host，Framework 只有自足的 WorkingContext |
| Spring DI 与 classpath 扫描 | 违反"透明胜过魔法" |

### 5.3 明确不采纳的对手设计

- **Feature 可劫持控制流**（Koog）：与"观察接口无 veto"的既有决策冲突，且共享上下文魔法 key 是隐式耦合。
- **checkpoint 不覆盖子 agent**（Koog）：这是缺口而非特性。
- **单一迭代数作为唯一闸门**（Koog）：不足以承担树形递归治理。
- **历史压缩与运行中换模型进 Framework**（Koog）：属于 `chatclient` 与 Host 的边界。
- **旧格式双读迁移**（Koog）：本仓库处于显式 breaking 阶段，且不受外部兼容承诺约束。
- **所有可达成 Action 并发执行**（embabel `ConcurrentAgentProcess`）：违反"不允许并发完成顺序决定业务结果"。

---

## 6. 已识别缺口与候选动作（待 ADR 裁决，勿直接实施）

按证据强度排序。每条都必须先落 ADR 再动生产合同。

| # | 缺口 | 证据来源 | 候选方向 | 备注 |
|---|---|---|---|---|
| 1 | **作者路径起点过高** | Koog 的分层起步；embabel 的注解模型 | 让复杂度按需付费：默认装配（默认 Limits、无 admitter、无 listener）使最小 Interaction 两行可跑，需要预算/树/恢复时再向下拆 | 与 `ARCHITECTURE.md` §10 的最小充分阶梯一致，只是阶梯第一级太高；**不是**引入注解或扫描 |
| 2 | **缺少面向消费者的测试工具包** | Koog `agents-test` 的 mock 模型 DSL | 提供外部作者可用的确定性模型/工具替身 | 现有八个示例是给自己的合同证据，不解决外部作者的测试问题 |
| 3 | **Planner 家族只有 GOAP** | Koog 的 LLM 规划器与带批评者的 LLM 规划器 | 以既有 Planner SPI 接入"模型出计划"形态 | 比 HTN / Utility 有更现实的需求证据；SPI 形状已由 GOAP 证明 |
| 4 | **Snapshot 是孤立值** | Koog checkpoint 的 version 链与 tombstone | 让"前驱快照"与"树已正常终结"成为 Framework 事实 | 成本低，对 Host 调试价值明确 |
| 5 | **拓扑不可导出** | Koog strategy 可绘制为图 | 为 Workflow 提供只读 topology 投影 | 仅只读投影，不引入图编辑器或反向扩张 Kernel |
| 6 | **无动态目标选择** | embabel `Autonomy`（目标排序 + 置信阈值 + 剪枝） | 需先证明选择、版本与权限合同 | `ARCHITECTURE.md` §8 已为此留门；应给出"证明后做"或"永久拒绝"的明确裁决，不再悬置 |
| 7 | **无只读 / dry-run 语义** | embabel `Action.readOnly` | 评估 CapabilitySet 是否已足够表达 | 可能无需新增概念 |

最高优先级仍不在上表：**P10 第一纵切上真实流量**。本文所有对比都建立在 Scope Agent 未经生产验证的前提上，该前提不消除，缺口排序随时可能失效。

---

## 7. 证据索引

对比时点的关键路径，便于复核。

**embabel-agent** @ `e2d7b987c`

- 进程与循环：`embabel-agent-api/src/main/kotlin/com/embabel/agent/core/AgentProcess.kt`、`core/support/AbstractAgentProcess.kt`、`core/support/SimpleAgentProcess.kt`、`core/support/ConcurrentAgentProcess.kt`
- 状态：`core/Blackboard.kt`、`core/Context.kt`、`spi/ContextRepository.kt`、`spi/support/InMemoryAgentProcessRepository.kt`
- 领域：`core/Action.kt`、`core/Agent.kt`、`core/ProcessOptions.kt`、`core/hitl/Awaitable.kt`
- 规划与自治：`src/main/kotlin/com/embabel/plan/`、`api/common/autonomy/Autonomy.kt`
- 组合：`core/support/DefaultAgentPlatform.kt`、`api/tool/Subagent.kt`
- 作者层：`src/main/java/com/embabel/agent/api/annotation/`、`api/annotation/support/AgentMetadataReader.kt`、`api/dsl/`

**Koog** @ `164f57a71`

- 图与循环：`agents/agents-core/src/commonMain/kotlin/ai/koog/agents/core/agent/entity/AIAgentSubgraph.kt`、`entity/AIAgentNode.kt`、`entity/AIAgentEdge.kt`
- 上下文：`core/agent/context/AIAgentContext.kt`、`core/agent/config/AIAgentConfig.kt`
- 恢复：`agents/agents-features/agents-features-snapshot/src/commonMain/.../Persistence.kt`、`AgentCheckpointData.kt`、`RollbackToolRegistry.kt`、`providers/`
- 规划：`agents/agents-core/.../core/planner/AIAgentPlanner.kt`、`agents/agents-planner/.../goap/GOAPPlanner.kt`、`.../llm/`
- 扩展点：`core/feature/pipeline/AIAgentPipeline.kt` 及其实现
- 组合与工具：`core/agent/AIAgentTool.kt`、`agents/agents-ext/src/commonMain/.../tool/`
- 内建策略：`core/../ext/agent/AIAgentStrategies.kt`

**Scope Agent** @ `6714ef84f`（Baseline 9）

- 内核：`engine.go`、`process_loop.go`、`transition.go`、`effect.go`、`step_commit.go`、`mailbox.go`
- 恢复：`process_snapshot.go`、`tree_snapshot.go`、`waiting_subtree_*.go`
- 策略：`interaction/`、`planning/`、`planning/goap/`、`workflow/`
- 治理与观测：`platform/`、`otel/`、`observation.go`、`event.go`
- 验收：`baseline_test.go`、`architecture_test.go`、`package_dag_test.go`、各 package 的 `wire_baseline_test.go`
