# Lynx Agent / Runtime / Desktop 架构演进总执行台账

> 作者：Codex
> 状态：`DONE`
> 建档日期：2026-07-30
> W4.1 实施提交：`40cffd81e`；W4.2：`cc85d3039`；W4.3：`ca0949949`
> W4.4 实施提交：`49b6494bd` + `fcbf8f558` + `34a875d29`
> W6.1 实施提交：`eae67ca4e`；W6.2：`47970e41d`
> W7.0 基线提交：随本原子 slice 提交
> W7.1 实施提交：`051ea578d`
> W7.2 实施提交：`8bb0fa7ba`
> W7.3 实施提交：`9c87d93fc`
> W7.4 实施提交：`d0380d0a2`
> W7.5 实施提交：`ed411fe93`；clipboard 证据：`c5590aad0`
> W8 实施提交：随本原子 slice 提交
> P25 实施提交：`03773c52d`
> P26 实施提交：随本原子 slice 提交
> P27 实施提交：随本原子 slice 提交
> P28 实施提交：随本原子 slice 提交
> P31 实施提交：`60729739f`、`14aae8eda`、`1ff0c8db6`、`277f36b72`、`12998274d`、`01177233f`、`9a2c9222a`
> 当前主任务：无；P31 已闭环，后续并发、RAG 或 Agent authoring 变更必须通过本节反向不变量
> 执行进度：`W2–W8 DONE · P25–P31 DONE`
> 当前协议：`protocol.current = protocol.minSupported = "2026-08-02"`
> 当前 Artifact：`SessionArtifactVersion = 9`
> 当前 Store：`schemaEpoch = 51`

## 0. 文档职责

本文是后续实施的**总入口和进度总账**，统一记录：

- 最终目标与不可突破的架构边界；
- Agent Framework、`app/runtime`、`app/desktop` 与 UI 的依赖关系；
- 当前真实进度、下一任务、验收门禁和提交纪律；
- 跨专项计划的关键路径、风险和已经接受的决策。

本文**不是第三份协议规范**，也不复制字段级 wire shape。协议权威顺序如下：

1. [`API.md`](../desktop/docs/protocol/API.md)、
   [`AUX_API.md`](../desktop/docs/protocol/AUX_API.md) 与
   [`TRANSPORT.md`](../desktop/docs/protocol/TRANSPORT.md)
   定义现行语义；Contract Registry 及其生成物定义字段、方法、能力、错误和约束。
2. 本文
   总执行台账，回答“为什么做、先做什么、做到哪了、何时算完成”。
3. [`codex_runtime_protocol_conformance_plan.md`](codex_runtime_protocol_conformance_plan.md)
   Runtime 协议一致性与 child Run 专项台账。
4. [`codex_desktop_run_tree_execution_plan.md`](codex_desktop_run_tree_execution_plan.md)
   Desktop B1.6 的目标读模型、爆炸半径与原子执行卡。
5. [`codex_synara_visual_baseline_and_execution_plan.md`](codex_synara_visual_baseline_and_execution_plan.md)
   W7 的视觉目标、状态映射、冻结决策、验收矩阵与原子执行卡。
6. [`../../agent/doc/EXECUTION_PLAN.md`](../../agent/doc/EXECUTION_PLAN.md)
   Agent Framework 专项台账；稳定架构和公共合同由同目录其他 owner 文档维护。
7. [`../runtime/doc/ARCHITECTURE.md`](../runtime/doc/ARCHITECTURE.md)、
   [`../desktop/frontend/ARCHITECTURE.md`](../desktop/frontend/ARCHITECTURE.md) 与
   [`../desktop/docs/FRONTEND_PLUGIN_CONTEXTS.md`](../desktop/docs/FRONTEND_PLUGIN_CONTEXTS.md)
   各模块的现行架构基准。
8. Contract Registry、生成物、源码、测试和 Git
   证明“当前实现实际上是什么”。
9. [`codex_runtime_protocol_2026_07_27_archive.md`](codex_runtime_protocol_2026_07_27_archive.md)、
   `PROTOCOL_DESIGN.md`、`PROTOCOL_VNEXT_REVIEW.md`、
   `VNEXT_IMPLEMENTATION_PLAN.md` 与各类 comparison 文档
   保留为决策历史和证据，不再承担当前进度总账。

冲突处理：

- 目标 shape 冲突：以冻结契约为准；
- 当前进度冲突：以 Git、工作树和最近一次可复核门禁为准，再回填台账；
- 架构归属冲突：以根目录
  [`CLAUDE.md`](../../CLAUDE.md)、
  [`DESIGN_PHILOSOPHY.md`](../../DESIGN_PHILOSOPHY.md) 和
  [`REFACTORING.md`](../../REFACTORING.md) 为准；
- 不允许为了迁就现状反向弱化目标契约。

截至 2026-07-31，W2–W6 与 W7.0 已完成。B1.7 已用一次 breaking、无兼容分支的原子切片
接通 frozen Run profile、生产 child admission、cold rehydrate、Artifact tree fidelity、
生成式 contract 约束以及 Runtime/Desktop capability opt-in。W6 又完成 Agent 泄漏复核、
Runtime/Desktop 依赖与事实作者审计、Desktop Runtime anti-corruption boundary、全局语义
命名清理和全门禁收口；未发现需要重做 Agent/Runtime 主模型的证据。W7.0 又冻结了
Synara/Lynx 的真实视觉基线、状态映射、目标 token、有意分歧、验收矩阵和 W7.1–W7.5
原子边界，并完成 foundation、shell/Work Index、Agent Narrative/Composer/Run tree/
HITL、Context Dock/Workspace/Settings 与最终 WebView/accessibility closure。W8 又以
“抽象不泄露、不过少、不过度”为主线完成全仓复核，关闭 Desktop design-system 绕行、
复合交互错误抽象、Go receiver 命名回归和视觉尾部取整不确定性；Runtime/Frontend
生成契约、协议生命周期与模块依赖仍保持闭环。P25 将 Agent 的 Host transaction protocol
替换为纯 Plan/Apply，并把 Framework snapshot codec 收口到 `agentexec`。P26 继续关闭三项
同源泄露：普通 waiting checkpoint 不再先于 App barrier 单独落盘；terminal `Discard` 不再
后台删除 durable state；child process 不再派生隐藏 product Session。P27 进一步删除穿过
Application 的 checkpoint write capability 和 App-owned process topology envelope，把完整
Framework tree 收敛成单个 root-owned opaque `ExecutorCheckpoint`；boot recovery 提升为
Application use case，SQLite 只读写事实并应用 `RecoveryCommit`。P28 再把 separately-valid
aggregate 之间的 root/Session/workspace/model-selection/goal-lease binding、parked Session
execution-policy freeze 和 checkpoint owner-bound save/delete 补成显式不变量。App 现在独占
checkpoint、Pending、Run、Session、recovery 的原子性和 retention。P29 用反例驱动终审继续
清除了 Pending overwrite、root-only mutation authority、checkpoint policy/usage 回退、Goal
recovery 漏记账、parked tree root-only terminalization、Run/Continuation 冻结事实可漂移，
以及 executor `Budget` 与 Run `Limits` 双重权威；SQLite 直接切换 epoch 50，不保留兼容路径。
P30 随后删除 App durable state 与 product hook 中最后的 Framework topology 泄露，并阻止 peer
adapter 互相借用具体端口。P31 转向 Framework 使用面：继续保持顶层 Process 串行逐 tick
重规划，把所有并发限制在显式 fan-out / child Process / ToolLoop；补齐 RAG 唯一 TopK、Markdown
结构化切分、集合 metadata membership 与 managed typed prompt。整个阶段没有引入自动并行
Plan Stage、共享 Blackboard、RAG 大 Pipeline、test-only 公共包或 provider-native schema 猜测层。
专项判据、裁决和防复发规则见
[`codex_agent_app_abstraction_boundary_audit.md`](codex_agent_app_abstraction_boundary_audit.md)。

---

## 1. 总目标

### 1.1 一句话目标

把 Lynx 建成一套边界清晰、协议优雅、行为完整、可恢复且可持续演进的 Agent 系统：

```text
Agent Framework
  提供通用执行原语、进程树、交互、checkpoint 与确定性生命周期

App Runtime
  作为 Framework 消费者，拥有产品领域、事务、幂等、投影与协议

Desktop
  通过稳定协议消费权威状态，以插件化限界上下文承载用户意图

UI
  在不破坏领域边界的前提下，达到 Synara 参考实现的桌面级布局与视觉品质
```

### 1.2 成功标准

最终系统同时满足：

1. **API 足够优雅且务实**
   - 一个用户意图只有一个主入口；
   - 类型、命名和错误直接表达领域语义；
   - query 是权威事实，stream 提供实时体验和有界 replay；
   - capability 显式协商，不能静默降级；
   - 调用方不需要理解内部 executor、checkpoint JSON、goroutine 或数据库事务。
2. **扩展性与功能性都处于顶级水平**
   - 新能力优先归约为参数化、组合或装饰，不复制运行时；
   - 闭合联合、条件约束、错误与 capability 由机器契约统一生成；
   - Run tree、HITL、恢复、取消、重放和冷查询形成完整闭环。
3. **最符合 Agent 心智模型**
   - 用户只理解 `Session → Run → Segment → Item`；
   - Framework 只理解 Agent execution；
   - App 将 execution 投影成产品事实；
   - Desktop 从产品事实构建 `Work Index / Agent Narrative / Context Dock`。
4. **架构长期可维护**
   - 高内聚、低耦合、依赖单向；
   - 接口在消费方定义且保持最小；
   - 一个事实只有一个作者；
   - 没有兼容层、双写、旧 decoder、迁移 shim 或“以后再删”的历史债务。
5. **质量可证明**
   - 每个状态转换、竞态、故障和恢复路径有可复核测试；
   - contract generation 可重复；
   - build、vet、test、lint、race、architecture、tidy 与前端检查全部通过；
   - 文档、生成物和实现随同一个原子 slice 更新。

### 1.3 非目标

- 不为假想的第二个实现创建接口、Repository 基类、Manager 或通用事务框架；
- 不把 Clean Architecture / DDD 变成目录和样板仪式；
- 不在 Agent 中加入 App 的 Session、Run、Item、BuildID、计费、幂等或 SQLite 概念；
- 不为旧 API、旧 schema、旧 snapshot 或旧 frontend shape 提供兼容路径；
- 不在完整 child Run 闭环前打开 `features.subagents`；
- 不在没有真实测量时做性能优化；
- 不在本计划中执行 tag、release 或外部数据迁移。

---

## 2. 不可破坏的执行法则

### 2.1 Breaking-first

项目仍处于 dev 阶段。错误的命名、API、wire、store schema 或包边界直接替换：

- 不保留 deprecated alias；
- 不双字段、双读、双写；
- 不提供 fallback decoder；
- store schema 需要变化时直接提升 epoch，以 fresh store 验证；
- 删除旧路径必须与新路径在同一个原子 slice 完成。

Breaking change 已获得总授权；实施前仍需计算爆炸半径，但默认选择正确终态，不选择兼容折中。

### 2.2 治本，不治标

每个问题都先回答：

> 根因消失了吗，还是只有当前现象不出现了？

禁止：

- 在 presenter 补造 domain/application 没有提交的状态；
- 在 consumer 用 fallback 掩盖 producer 的非法 shape；
- 在失败点重试不具备幂等保证的副作用；
- 用状态补丁冒充 executor 已停止、事务已提交或 checkpoint 已更新；
- 用注释、词法扫描或调用方约定代替构造期不变量。

### 2.3 抽象既不能不足，也不能过度

必要抽象的信号：

- 同一事实由多个层重复维护；
- 一个调用方只需要具体实现的一小部分能力；
- 真实存在跨边界替换、测试或协议生成需求；
- 同一规则已经在三个以上位置同步变化。

过度抽象的信号：

- 只有一个实现、一个消费者且没有边界价值；
- 为未来猜测创建 hook、SPI、通用 Repository 或 Manager；
- 为“分层好看”让调用路径多穿一层；
- 把固有内聚的大包强拆成互相转发的小包；
- 用泛型或反射隐藏本应显式的领域语义。

Clean Architecture 在本项目中首先是**所有权与依赖方向**，不是层数竞赛；DDD 首先是
**明确语言、不变量和限界上下文**，不是框架化 AggregateRoot / DomainEvent 仪式。

### 2.4 抽象绝不泄露

判据：

> 这个概念是否对所有 Framework 消费者都成立？

- 对所有消费者成立的 execution 原语可以进入 Agent；
- 只有 Lyra App 需要的事务、幂等、产品 identity、投影和协议必须留在 App；
- wire shape 只存在于 delivery / generated client；
- UI state、交互和视觉语言只存在于 Desktop；
- 任何层不得要求内层理解自己的具体概念。

### 2.5 一个事实只有一个作者

典型唯一作者：

- execution tree / checkpoint：Agent Framework；
- Run / Segment / Item / PendingInterruptSet：App domain/application；
- 原子写集与 store policy：App adapter/infra；
- wire shape / method metadata：Contract Registry；
- runtime projection：App query/store；
- frontend fold：Agent frontend context；
- 视觉 token 与组件规格：Desktop design system。

其他层只能翻译、调用或投影，不能创建第二份规则。

---

## 3. 最终所有权边界

| 概念                                                                | 唯一 owner                                   | 允许提供                                                                | 禁止泄露                                         |
| ------------------------------------------------------------------- | -------------------------------------------- | ----------------------------------------------------------------------- | ------------------------------------------------ |
| Agent 定义、Process tree、interaction、tool checkpoint、Suspension  | Agent Framework                              | 通用执行 API、不可变 snapshot、prepared mutation                        | App Run/Item、数据库、产品错误、幂等键           |
| Waiting subtree execution replacement                               | Agent Runtime                                | `prepare / commit / abort`、精确 canceled process、surviving suspension | Store、Repository、transaction、BuildID、SQLite  |
| Session / Run / Segment / Item / Interrupt / protocol profile       | App domain/application                       | 领域值、不变量、用例编排、consumer ports                                | JSON-RPC DTO、Agent 内部 state JSON、SQLite 实现 |
| 幂等、原子性、CAS、BuildID、usage ledger、durable checkpoint commit | App                                          | transaction write-set、恢复与失败策略                                   | Agent public API                                 |
| Agent ↔ App 翻译                                                    | `adapter/agentexec`                          | process/source/suspension 映射，App-private prepared port               | delivery DTO、SQL、产品 presenter                |
| SQLite 与具体事务                                                   | `infra/storage/sqlite`、`adapter/runsegment` | application/domain port 实现                                            | 业务用例裁决、wire shape                         |
| 方法、shape、error、capability、constraints                         | `delivery/dispatch` Contract Registry        | 生成 metadata 与 fail-closed 校验                                       | Run 生命周期与 durable 拼装                      |
| JSON-RPC / HTTP / SSE                                               | `delivery`                                   | envelope、context metadata、wire ↔ application 翻译                     | executor 生命周期、事务、query 语义              |
| Frontend wire                                                       | generated RPC 层                             | generated type、validator、method metadata                              | 手写第二份协议联合                               |
| Frontend 业务                                                       | plugin bounded contexts                      | command、event、selector、fold、read model                              | transport internals、backend concrete client     |
| UI 视觉                                                             | design system + feature composition          | token、primitive、atom、agent shell                                     | 协议规则、领域状态写入                           |

### 3.1 Agent 泄露审查硬门

每次修改 Agent public API、snapshot 或 event，必须证明：

- 不出现 `RunID`、`SegmentID`、`ItemID`、`SessionArtifact` 等 App 产品概念；
- 不出现 Store、Repository、transaction、idempotency、CAS policy、BuildID 或 cost ledger；
- Framework 不发布任意 App result，不生成产品文案；
- snapshot 只表达可恢复 execution state，不表达宿主持久化协议；
- prepared mutation 只冻结、验证和应用 Framework state；
- App 不解析、patch 或重写 `FrameworkState`；
- Agent 的 `Commit` 在 prepare 成功后不执行 I/O，不重新运行工具，不伪造用户 Resume。

若某能力只服务 App，它应当成为 App consumer port 或 adapter，而不是下沉到 Agent。

---

## 4. 当前事实与进度

### 4.1 总览

| 工作流                       | 状态          | 已完成                                                                                                | 下一步                            |
| ---------------------------- | ------------- | ----------------------------------------------------------------------------------------------------- | --------------------------------- |
| Protocol A-track             | `DONE`        | A1–A7；冻结契约、Registry、生成物、Runtime 与 Desktop consumer 一致                                   | 只随真实新能力同步                |
| Agent P24                    | `DONE`        | host-settled checkpoint、prepared subtree mutation、App durable transaction 与完整恢复/竞态门禁       | 只按后续 consumer 的真实需要演进  |
| Runtime B1.1                 | `DONE`        | durable Run-tree identity、root admission                                                             | —                                 |
| Runtime B1.2                 | `DONE`        | first-class child producer、source routing、独立 Segment/Item/metrics                                 | —                                 |
| Runtime B1.3                 | `DONE`        | tree barrier、整树 resume、restart/race/failure conformance                                           | —                                 |
| Runtime B1.4a–b              | `DONE`        | immutable cancel plan、Running child subtree cancellation                                             | —                                 |
| Runtime B1.4c                | `DONE`        | prepared runtime mutation + App-owned atomic waiting-subtree transaction                              | —                                 |
| Runtime B1.4d                | `DONE`        | W2.1 ownership；W2.2 failure/rollback；W2.3 restart/query/publication；W2.4 race/hygiene/full closure | —                                 |
| Runtime B1.5                 | `DONE`        | W3.0–W3.4 query / stream / replay / cold recovery / full closure                                      | —                                 |
| Desktop B1.6                 | `DONE`        | normalized Run tree、source-owned fold、durable recovery、root-first UI 与 scope-exact public API     | —                                 |
| Runtime/Desktop B1.7         | `DONE`        | frozen policy、producer/recovery、Artifact/contract、server/Desktop opt-in 与最终 conformance 已闭环  | —                                 |
| Runtime/Desktop 架构持续演进 | `DONE`        | W6.0–W6.3 完成依赖、事实作者、Agent 泄漏、Runtime context、命名、错误、并发生命周期与全门禁收口       | 后续由架构回归门持续守护          |
| Synara UI 对齐               | `DONE`        | W7.0–W7.5 已完成 foundation、Shell、Agent、Workspace、responsive、accessibility 与 WebView closure    | 后续由视觉与架构回归门持续守护    |

不使用跨工作流“总百分比”。一个竞态闭环不能与一个命名修正等权；进度只由原子 slice
和完成证据表达。

### 4.2 当前 Git 与质量基线

已提交并推送：

- `a2a3bb2de feat(agent): settle paused tool checkpoints`
- `1d5f7eb86 feat(agent): prepare waiting subtree cancellation`
- `a4e153fd4 feat(runtime): commit waiting subtree cancellation atomically`
- `52f46fbed docs(runtime): record waiting cancellation completion`
- `e7452288b test(runtime): prove cancellation ownership arbitration`
- `3a512fe71 test(runtime): prove waiting cancellation rollback`
- `42658445a test(runtime): prove cancellation restart consistency`
- `ef04f5bfd docs(runtime): freeze B1.5 execution slices`
- `6460bede9 feat(runtime): complete descendant queries`
- `4100af80d test(runtime): prove root stream recovery`
- `f38085bf3 fix(runtime): preserve run facts on recovery`
- `b75d2a1d9 refactor(runtime): close B1.5 conformance`
- `26e43fd0e docs(desktop): freeze run tree execution plan`
- `40cffd81e feat(desktop): make run tree projection source-owned`
- `cc85d3039 feat(desktop): converge agent session synchronization`
- `ca0949949 feat(desktop): present durable run trees`
- `49b6494bd fix(protocol): distinguish run opening acknowledgements`
- `fcbf8f558 refactor(desktop): clarify run application surface`
- `34a875d29 refactor(desktop): make interrupt settlement deterministic`
- `9429373e5 docs(architecture): record subagent capability cutover audit`
- `2f8b65c0d feat(runtime): enable negotiated run trees`
- `eae67ca4e refactor(desktop): isolate runtime connection boundary`
- `47970e41d refactor(desktop): clarify frontend vocabulary`

W6.3 收口时的实现基线（收口文档提交前）：

```text
HEAD        = 47970e41d
origin/main = 47970e41d
worktree    = clean
```

P24-03 / B1.4c 已提交的 App 实现包括：

- `application/runs` 的 tree continuation、cancel transformation 与 consumer ports；
- `adapter/agentexec` 的 prepared waiting-subtree bridge；
- `adapter/runsegment` 的 App-owned transaction；
- SQLite Interrupt / Run / Transcript persistence；
- Bootstrap 与相关测试 fixture。

2026-07-30 完成门禁：

```text
MODULE=agent FAST=1 scripts/check.sh build vet test lint
MODULE=app/runtime FAST=1 scripts/check.sh build vet test lint
  → PASS

go test -race ./runtime -count=1                              # agent
go test -race ./internal/application/runs \
  ./internal/adapter/agentexec \
  ./internal/adapter/agentexec/turn \
  ./internal/adapter/runsegment \
  ./internal/infra/storage/sqlite -count=1                    # app/runtime
  → PASS

forbidden import / persistence leakage / compatibility scan
git diff --check
  → PASS
```

Coordinator remaining/final boundary、durable failure Abort、restart Rehydrate、committed
parent-tool reducer、adapter claim/deadlock，以及真实 SQLite transaction success/rollback
均有行为测试。P24-03 / B1.4c 已完成；capability 仍不得打开，完整并发交错和 query
conformance 进入 W2 / B1.4d。

2026-07-30 本轮重新验证：

```text
MODULE=agent FAST=1 scripts/check.sh build vet test lint
  → PASS

MODULE=app/runtime FAST=1 scripts/check.sh build vet test lint
  → PASS

cd app/desktop/frontend && npm run check
  → PASS
  → 178 test files / 1078 tests passed
  → circular / contexts / published-boundaries / layers / tokens / chrome /
    locales / bootstrap / bundle gates passed

cd agent && GOWORK=off go mod tidy -diff
cd app/runtime && GOWORK=off go mod tidy -diff
git diff --check
  → PASS
```

计划校准轮没有把当时尚未运行的 `-race`、`govulncheck` 或“真实 SQLite 重启后的
B1.4d 新矩阵”伪装成已验证；随后 W2.1–W2.4 已分别补齐 ownership race、failure
matrix、真实重启证据与全量 race/hygiene。Desktop bundle gate 虽通过，但
Vite/Lightning CSS 仍报告
`shadow-[var(--shadow-*)]` 与 CSS Custom Highlight 语法告警，已在 §4.6 记账。

2026-07-30 W2.1 新增并验证：

```text
go test ./internal/application/runs \
  -run 'Test(...ownership arbitration...)$' -count=10
go test -race ./internal/application/runs \
  -run 'Test(...ownership arbitration...)$' -count=10
  → PASS

MODULE=agent FAST=1 scripts/check.sh build vet test lint
MODULE=app/runtime FAST=1 scripts/check.sh build vet test lint
cd agent && GOWORK=off go mod tidy -diff
cd app/runtime && GOWORK=off go mod tidy -diff
  → PASS
```

确定性 barrier 覆盖 parked child/root、child/resume 的双向胜者与重复请求；live handle
覆盖 root/child、natural terminal/child 的双向裁决。唯一生产修正是在既有 root handle
owner 内把 root-first 的 losing child 错误统一为可判定的 `ErrSessionBusy`；没有新增
coordinator、锁层、协议方法或兼容分支。

### 4.3 架构文档状态

- `app/runtime/doc/ARCHITECTURE.md` 已使用 root-owned direct
  suspension set，并记录 waiting child cancellation 的 prepared mutation / atomic
  transaction 边界；其取消与恢复心智和 B1.4d 最终实现一致，未发现 root-only /
  single-interrupt 残留；
- query/subscribe/Running cold recovery 的未完成边界明确属于 B1.5/W3，不以兼容路径
  或模糊注释提前承诺。

### 4.4 当前仓库审计结论

结论不是“新 API 尚未实现”，而是：

> vNext 的协议骨架、核心读写面和绝大多数行为已经落地；剩余工作是按依赖顺序完成
> child Run 的取消一致性、完整读面、Desktop 消费和 capability 放行。

| 审计面            | 当前事实                                                                                                                 | 判断                                                |
| ----------------- | ------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------- |
| Machine contract  | 单一 Contract Registry 生成 manifest、Schema、OpenRPC、Go validator、TS types/validator/method map；arch drift gate 通过 | 目标结构已成立，不再新增平行 registry               |
| 协议表面          | manifest 当前有 85 个方法，4 个 stream method；协议只服务 `2026-07-27`；Artifact 只接受 v7                               | hard cutover 已成立，没有版本协商假象               |
| Run 心智模型      | `Session → Run → Segment → Item`、Run 三态、正交 outcome/metrics、typed Interrupt、durable query 均已落地                | API 主模型无需重做                                  |
| Agent Framework   | execution tree、HITL、checkpoint、无资源保留的 waiting-subtree Plan/Apply、Continue 与 consumer/recovery 门禁已成立       | P25/P26 证明 Host transaction、Store 与产品 identity 未下沉 |
| App Runtime 写面  | child admission、source routing、tree barrier/resume、Running/Waiting child cancel 与 B1.4d conformance 已完成           | 事实 owner 正确；后续不得重新分配所有权             |
| App Runtime 读面  | descendant paging、exact/subtree items、root stream replay、cold tree recovery 与 child subscribe 拒绝均已闭环           | durable tree query 与 stream ownership 已成立       |
| Desktop transport | root stream、durable snapshot、reattach、replay fallback、exact child cancel 与 source-owned fold 已闭环                 | first-party consumer 已完整 opt in                  |
| Desktop fold      | root/child/sibling/nested Run 均按 source identity 独立折叠，并形成 root-first tree/narrative UI                         | B1.6 完成，无 single-run 或 synthetic recovery path |
| Capability        | server 稳定广告 `features.subagents.enabled=true`，Desktop 显式请求；未协商调用仍 fail closed                            | B1.7 已原子启用且保持 opt-in                        |
| 依赖治理          | Go architecture tests、Frontend layer/context/public-boundary/cycle gates 全绿                                           | 不需要全局换目录；每个 slice 内做局部治本           |
| 历史兼容          | SQLite 单 epoch 46、protocol current=min、旧 store/artifact/schema 直接拒绝                                              | 符合 dev 阶段 breaking-first                        |

### 4.5 B1.4d 证据闭环矩阵

以下矩阵记录 B1.4d 的对抗性证明如何闭环，作为 W2 完成后的可复核索引。

| 场景                                        | 原有基础                                                                                                                 | W2 完成证据                                                                                                    |
| ------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------- |
| active / later sibling / nested target      | Agent prepared mutation 已覆盖 active/later sibling 与 nested ancestor；App pure transform 覆盖 nested subtree postorder | W2.1 以 nested target + descendant + surviving sibling 贯穿真实入口；W2.2 收口 adapter 失败拓扑                |
| root cancel vs child cancel                 | live handle 已证明 Running tree 只能有一个 owner                                                                         | `W2.1 DONE`：Running 与 Waiting/parked 双向胜者、loser busy、零额外 durable mutation                           |
| resume vs child cancel                      | root resume vs root cancel 已共享 admission                                                                              | `W2.1 DONE`：Waiting child cancel / resume 双向胜者与单一 durable opening/cancel truth                         |
| natural terminal vs child cancel            | root cancel vs natural terminal 已覆盖                                                                                   | `W2.1 DONE`：target terminal before/after child claim 均稳定返回 finished 且释放 claim                         |
| duplicate cancel / stale input              | root 已有“turn already gone”幂等完成；parent Item CAS stale 可回滚                                                       | W2.1 固定 duplicate root/child；W2.2 证明 stale Pending CAS、稳定错误分类与零 publication                      |
| teardown / checkpoint / transaction failure | root cleanup failure、prepared Abort、SQLite 整体 rollback 已覆盖                                                        | W2.2 对 checkpoint、Item、Run、Pending、Resume、opening、commit 与 teardown 逐点注入并证明 rollback/owner 语义 |
| exact response / invalidation               | command 返回 committed target/root；成功 transaction 返回两者                                                            | W2.3 证明 exact field equality、Runs/Interrupts/Sessions 精确失效集合且无 post-commit re-query                 |
| subtree quiescence                          | Agent Commit 后 child 从 registry 移除                                                                                   | W2.3 证明 cancel 成功返回后 Journal 不再出现 target/descendant 事件                                            |
| process restart                             | application 有 fake Rehydrate；Agent portable replacement 可 restore/Continue                                            | W2.3 用 file-backed SQLite close/reopen 证明 target 不复活、survivor/checkpoint/query 一致                     |
| canonical publication                       | Domain `RunTree` 与 pure transform 已固定 descendant-before-ancestor                                                     | W2.3/W2.4 证明 descendants first、siblings lexical、root last，并在 race 下重复                                |

### 4.6 Hygiene 观察与处理策略

本轮只记录可复核事实，不因“文件大”或“名字看起来一般”机械重构：

1. **Store receiver 告警已清除**
   - 当前生产代码中的 `*Store` receiver 使用 `s`，未再发现同一 Store 类型同时使用
     `s` / `base`。
2. **Receiver 一致性已全量收口**
   - `application/sessions.Snapshot` 统一使用 `snapshot`；
   - `delivery/protocol.StreamEvent` 与生成 validator 统一使用 `value`；
   - Agent / Runtime 的生产与测试 Go 文件按 package + receiver type 全量扫描无混用。
3. **大文件不是自动拆分条件**
   - 当前热点包括 `runs/usecases.go`、SQLite `runs.go`、Agent
     `waiting_subtree_cancellation.go`、runsegment waiting transaction 与 Frontend
     `rpc/methods.ts`；
   - 只有当文件同时承担多个变化原因、迫使无关消费者依赖或重复同一规则时才拆；
     单一完整用例或单一表的内聚实现允许保持较大。
4. **Agent 根包 alias 不是兼容债**
   - `agent/facade.go` 是明确的 canonical standard-path façade，类型事实仍由
     `core/runtime/interaction` 拥有；它不是 deprecated old-name shim；
   - 后续若改变公共门面，直接选择一个 canonical surface，不能再叠第二层 alias。
5. **Frontend 构建告警需在 UI 阶段清零**
   - `shadow-[var(--shadow-*)]` 需要追到产生该 class 的源码或构建规则；
   - CSS Custom Highlight 告警需确认是 optimizer 误报还是需要明确的兼容配置；
   - 不阻塞已完成的 Runtime B1.4d，但 W7 完成定义要求零未知构建告警。
6. **兼容词扫描需按语义裁决**
   - provider 的 OpenAI/Anthropic-compatible、UI fallback、错误 fallback 和文件系统
     fallback 都是现行业务能力，不是旧协议兼容；
   - 生产路径未发现旧 vNext decoder、双字段、双写或 schema migration shim；
   - 每个 slice 仍必须重复扫描，不能把本轮结论永久化。

---

## 5. 关键路径

```text
W0  固化总台账
 │
 ▼
W1  P24-03 / B1.4c App durable waiting-child cancellation
 │
 ▼
W2  P24-04 / B1.4d cancel、race、restart、query conformance
 │
 ▼
W3  B1.5 durable child query / subscribe / cold recovery
 │
 ▼
W4  B1.6 Desktop Run-tree consumer
 │
 ▼
W5  B1.7 full conformance + capability enablement
 │
 ├─────────────► W6 Runtime / Desktop architecture final sweep
 │
 ▼
W7  Synara visual baseline + pixel-level Desktop UI alignment
```

质量与架构审查不是最后补做的阶段。命名、错误、接口、所有权、无泄露和无兼容债门禁贯穿
W1–W7；W6 是最终全局复核，不是把已知坏味道推迟到最后。

---

## 6. 工作包与完成定义

### W0 — 固化总台账

状态：`DONE`

交付：

- 建立本文；
- 对齐 Git、两份专项计划、冻结契约、Runtime/Desktop 架构基准；
- 明确当前草稿只算 `IN PROGRESS`；
- 固化后续进度更新格式。

完成定义：

- 文档链接有效；
- 当前 commit、状态和下一任务与仓库一致；
- 本文单独提交，不夹带当前代码草稿。

### W1 — P24-03 / B1.4c App durable waiting-child cancellation

状态：`DONE`（2026-07-30，implementation commit `a4e153fd4`）

目标：在不执行用户代码、不伪造外部 Resume、不让 Agent 理解 App persistence 的前提下，
完成 Waiting child subtree 的原子取消。

必须形成的单一顺序：

```text
root/tree admission
  → freeze immutable cancellation plan
  → Agent prepare execution replacement
  → App derive complete immutable write-set
  → one App transaction commits durable truth
  → apply prevalidated Agent live mutation
  → publish committed application events
```

事务失败必须调用 `Abort`，保持 live runtime 与 durable state 均不变。事务成功后的 Agent
commit 只能应用 prepare 阶段已经验证的内存变换，不得做 I/O、重新执行工具或重新裁决。
若随后需要激活 surviving tree，activation failure 服从既有“opening 已提交后 error
terminalize”语义，不能回滚已经提交的历史。

原子写集至少包含：

- process-tree checkpoint replacement；
- canceled target subtree 的全部非终态 Run；
- parent spawning Item 的唯一 `child_run_canceled`；
- 缩减后的 Pending / Continuation；
- 已由 Host 结算的 parent tool projection；
- 当最后一个 Interrupt 被移除时，所有 surviving Run 的新 Segment identity 与 opening；
- exact target child 与 root committed snapshot。

两条合法结果：

1. **仍有 Interrupt**
   - target subtree canceled；
   - parent tool settled exactly once；
   - reduced Pending 持久化；
   - root 与 surviving tree 继续 Waiting；
   - 不启动 executor。
2. **最后一个 Interrupt 被移除**
   - target subtree canceled；
   - parent tool settled exactly once；
   - Pending 整体消费；
   - surviving suspended Runs 一次性打开新 Segment；
   - 使用 prepared checkpoint 继续执行；
   - 不生成第二个 application barrier，不伪造用户回答。

完成定义：

- application transformation 是无 I/O、可单测的领域/用例值变换；
- App-private port 不暴露 Agent snapshot JSON 或 FrameworkState；
- transaction 只有一个事实作者，不由 Coordinator、Agent adapter 与 SQLite 各决定一部分语义；
- parent tool 不会同时收到 canceled result 与普通 result；
- commit/abort 顺序无 durable/live split；
- targeted unit、SQLite transaction、Agent adapter、Coordinator integration tests 全绿；
- P24-03 与 B1.4c 台账、架构文档随提交更新；
- 形成一个独立、可 revert、全绿的 commit 并 push。

完成结果：

- application 以无 I/O transformation 固定 canceled postorder、parent Item、
  reduced Pending/private tree continuation 与完整 durable write-set；
- prepared Agent mutation 的 checkpoint write 加入 App transaction，失败 Abort、成功
  Commit；Agent 不接触 BuildID、store、CAS 或 transaction；
- remaining boundary 保持整树 Interrupted；最终 boundary 在同一 transaction Resume
  surviving Runs 与写入 opening Items，再用 Agent Continue 推进 ready checkpoint；
- parent `DrainedTool` 转为 `CommittedTool`，reducer 不重放 tool start、不重复 transcript
  Item；
- Coordinator、adapter、reducer、restart 与真实 SQLite commit/rollback 覆盖通过，完整
  build/vet/test/lint 和高风险 race 全绿。

### W2 — P24-04 / B1.4d 完整取消一致性

状态：`DONE`；当前：`W2.1–W2.4 DONE`

本工作包不重做 B1.4a–c。既定链路保持：

```text
root-owned admission / live tree arbiter
  → immutable cancellation plan
  → prepared Agent execution mutation
  → one App-owned durable write-set
  → committed live mutation
  → exact response + committed invalidation/publication
```

实施拆成四个可独立提交、可独立回滚的原子 slice：

| Slice | 状态   | 目标                                 | 主要 owner                                       | 退出证据                                                                       |
| ----- | ------ | ------------------------------------ | ------------------------------------------------ | ------------------------------------------------------------------------------ |
| W2.1  | `DONE` | 确定性 ownership arbitration         | `application/runs` + Agent live arbiter          | parked root/child、resume/child、terminal/child、duplicate 双向胜者            |
| W2.2  | `DONE` | stale/failure/rollback conformance   | `application/runs` + `adapter/runsegment`        | stale Pending、checkpoint、transaction、teardown/Continue 每个失败点           |
| W2.3  | `DONE` | restart/query/publication/quiescence | SQLite + application query + delivery projection | file-backed close/reopen、exact target/root/read model、无 canceled-late-event |
| W2.4  | `DONE` | race、hygiene 与全量收口             | 跨 Agent / Runtime                               | 重复 race、全量 gates、命名/错误/接口/兼容扫描、文档同步                       |

#### W2.1 — 确定性 ownership arbitration

测试必须使用 channel、显式 barrier 或等价的同步原语控制交错；`time.Sleep` 不能充当正确性
证明。每组竞态至少覆盖 A 先、B 先两种方向：

1. Waiting child cancel vs root cancel；
2. Waiting child cancel vs whole-tree resume；
3. Running child cancel vs target natural terminal；
4. 同一 child 的重复 cancel；
5. nested child 与 active/non-active sibling 组合。

胜者必须满足：

- 只有一个 tree owner；
- 只有一个 terminal/durable transaction；
- exact target 与 root snapshot 可确定；
- 失败方返回稳定、可行动错误；
- 失败方没有 durable、live、publication 或 admission 残留。

若测试暴露生产缺口，只允许修正现有 owner 或不变量；不得创建第二套 child coordinator、
专用协议方法或跨层锁。

完成结果（2026-07-30）：

- channel barrier 在真实 Coordinator 入口固定 Waiting child/root、child/resume 的双向赢家；
- 同一 owner 持有期间，duplicate root/child 与交叉命令统一返回 `ErrSessionBusy`，且 loser
  不执行 durable transaction、prepared Commit、opening 或 runtime mutation；
- live handle 固定 root/child 的稳定错误分类，以及 natural terminal 在 child claim 前后
  的 `ErrRunFinished` 语义和 claim 释放；
- fixture 使用 nested target、target descendant 与 surviving sibling，证明仲裁不退化成
  单节点特例；
- 定向普通测试与 race 均 `-count=10` 通过，Agent/Runtime build、vet、test、lint 与两个
  module 的 tidy diff 全绿；
- 生产代码只修改既有 arbiter 的错误 wrapping；协议、Agent API、Store schema 和
  capability 均未变化。

#### W2.2 — stale/failure/rollback conformance

按真实副作用顺序逐点注入：

1. stale Pending CAS；
2. replacement checkpoint write failure；
3. parent Item CAS failure；
4. terminal Run write failure；
5. remaining Pending / tree resume / opening Item failure；
6. prepared runtime Commit / Continue / executor teardown failure。

每个故障都要断言：

- transaction 内所有 App facts 一起 rollback，或在已经提交的 activation boundary 后按既定
  error-terminalize 语义收口；
- Agent prepared mutation 正确 `Abort` 或只应用一次；
- target、root、parent Item、Pending、checkpoint 与 process tree 没有 split brain；
- admission/claim 一定释放；
- error 使用“操作 + 对象 identity + 原因”，并以 `%w` 保留可判定因果。

完成结果（2026-07-30）：

- real SQLite fixture 按真实写序逐点注入 stale Pending、replacement checkpoint、parent
  Item CAS、terminal Run、reduced Pending、tree Resume、opening Item 与 transaction
  completion failure；每个失败都证明 Pending、parent Item、全部 Run、checkpoint/process
  tree 与 transcript 完整回滚；
- stale Pending 统一分类为可判定 `ErrSessionBusy`，所有 adapter/application 错误均保留
  cause，并补足 target/root/turn/process identity；
- durable transaction 失败只 `Abort` 一次 prepared mutation；Agent runtime Commit
  failure 保留 claim 至显式 Abort 后释放，未伪造补偿事务；
- final-boundary transaction 已提交后若 `Continue`/activation 失败，不反向回滚历史：
  prepared mutation 只 Commit 一次、Abort 为 no-op，surviving child 与 root 由既有 pump
  error-terminalize，registry 与 admission 均释放；
- Running child executor subtree teardown failure 释放 child-cancel claim；post-commit
  process discard failure 保留 turn retry ownership，并由 shutdown 重试后释放；
- 定向普通测试与 race 均通过，其中 failure matrix 在 race 下 `-count=10`；Agent/Runtime
  build、vet、全量 test、lint、tidy diff 与 `git diff --check` 全绿；
- public protocol、Agent API、Artifact、Store schema 与 capability 均未变化；没有新增
  transaction framework、兼容路径或第二个 lifecycle owner。

#### W2.3 — restart/query/publication/quiescence

使用 `t.TempDir()` 下的真实 SQLite 文件，不能以 `:memory:` 代替 restart：

1. 构造 root + target child + surviving sibling/nested descendant；
2. 提交 Waiting child cancellation；
3. 关闭数据库并重新 `sqlite.Open`；
4. 从 target child identity 查询完整 tree；
5. 验证 canceled subtree 不复活、survivor checkpoint/usage/BuildID 一致；
6. 对 remaining-boundary 走 boot reconcile + rehydrate/continue；
7. 对 final-boundary 验证 committed query truth，不把 W3 尚未实现的 Running cold recovery
   伪装成 W2 已完成；
8. 比较 command response 与 durable target/root exact snapshot；
9. 验证 Runs、Interrupts、Sessions invalidation 只覆盖实际变更的 read set；
10. cancel 返回后继续 drain/观察，证明 target/descendant 不再发布事件。

完成结果（2026-07-30）：

- file-backed final/remaining 两类 fixture 在提交后真实关闭并重新打开 SQLite；从 canceled
  child identity 可读完整 tree，canonical postorder、target/root command response 与 durable
  query snapshot 一致；
- replacement process tree、BuildID 与 usage checkpoint 精确 round-trip；canceled target 和
  nested descendant 不会从旧 checkpoint 复活；
- remaining boundary 的 root-owned Pending 经过 tree-aware boot reconciliation 后保留 root
  与 sibling Interrupted；恢复校验覆盖每个 continuation 的 lineage/model/time、Running
  interrupt Item、drained/committed tool，而不是只校验 root；
- final boundary 在重启前可读 committed Running root；当前 boot policy 随后诚实地将无人
  驱动的 root 收口为 `run_lost`，没有提前伪造 W3 Running cold recovery；
- canceled subtree 的 Running Question/Approval Item 与 Run/parent Item/Pending/checkpoint
  在同一 transaction 内结算，新增 failure seam 证明任一点失败完整 rollback；
- Runs、Interrupts、Sessions invalidation 覆盖精确 read set；运行中 child cancel 返回后的
  Journal replay 不再出现 target/descendant event；
- application/runs、adapter/runsegment、SQLite recovery 高风险矩阵在 race 下均
  `-count=10` 通过；public protocol、Agent API、schema、Artifact 与 capability 均未变化。

W2.3 与 W3 的边界：

- W2 证明“B1.4c 写出的事实可重启、可精确读取、不会复活 canceled subtree”；
- W3 才实现通用 descendant paging、root multi-source subscribe conformance，以及完整
  Running tree 的 restart `run_lost` 收口；
- 不得为了让 W2 测试通过而提前打开 `features.subagents`。

#### W2.4 — race、hygiene 与全量收口

1. 对 W2.1–W2.3 的高风险 package 运行 `-race -count=N`；
2. 复核 canonical postorder：descendants before ancestors、siblings lexical、root last；
3. 统一同一类型 receiver；生成物问题修 generator；
4. 审查错误信息、接口宽度、文件职责与依赖方向；
5. 扫描并拒绝旧 decoder、alias shim、双字段、双读/双写、migration path；
6. 运行 Agent / Runtime 全量门禁、tidy、contract drift 和 diff check；
7. 更新本文、协议专项台账、Agent P24 台账与 Runtime 架构基准；
8. 独立 commit 并 push 后，才把 P24 改为 4/4、B1.4 改为 `DONE`。

必须覆盖：

- active sibling、non-active sibling、nested target；
- root cancel vs child cancel；
- resume vs child cancel；
- natural terminal vs child cancel；
- duplicate cancel 与 stale Pending；
- teardown failure、checkpoint failure、transaction rollback；
- cancel response 的 target/root exact snapshot；
- canceled subtree quiescence：成功返回后不再发布事件；
- SQLite process restart 后 target 不复活、surviving tree 可继续；
- canonical postorder：descendants before ancestors、siblings lexical、root last；
- race detector 下重复运行。

完成定义：

- P24 4/4；
- B1.4a–d 全部 `DONE`；
- Agent/App 的职责边界审计无泄露；
- Runtime 架构文档删除 root-only / single-interrupt 旧心智；
- 全量 Agent/Runtime 门禁通过并独立提交、推送。

完成结果（2026-07-30）：

- Agent runtime/toolloop 与 Runtime application/runs、adapter/runsegment、SQLite 四组高风险
  package 均在 race detector 下 `-count=10` 通过；
- 生产写入、恢复与发布顺序全部复用领域 `RunTree.Postorder/SubtreePostorder`，确认
  descendants before ancestors、siblings lexical、root last；
- Agent/Runtime 全部 Go 文件按 package + receiver type 扫描无混用；`Snapshot` 统一为
  `snapshot`，`StreamEvent` 统一为 `value`，生产代码静态 `fmt.Errorf("constant")` 清零；
- Agent 公开 GoDoc 删除 App 的 Run/Item/Interrupt/Segment/持久事务措辞，只保留
  caller-coordinated external-state 语义；未改公开签名；
- 接口宽度、文件职责和依赖方向复核通过：prepared capability 保持 consumer-owned
  窄口，较大文件仍各自承载单一完整用例，不为行数机械拆层；
- 当前 ToolLoop checkpoint、process snapshot、内部 suspension envelope/relation 与 SQLite
  分别只接受 v4、v15、v3、epoch 43；无旧 decoder、alias shim、双字段、双读写或
  migration path；
- `features.subagents` 仍由组合根固定为 disabled；protocol、OpenRPC、Artifact、Agent
  API/wire、Store schema 与生成物均未变化；
- Agent/Runtime build、vet、全量 test、lint、tidy diff、contract/arch drift 与
  `git diff --check` 全绿。

### W3 — B1.5 durable query、subscribe 与 cold recovery

状态：`DONE`；当前：`W3.0–W3.4 DONE`

交付：

- `runs.get/list` 对 root/child/descendant 的权威语义；
- descendant paging 的稳定顺序和 cursor；
- `items.list` 精确 child scope 与 `includeDescendants`；
- root stream 中多 source Run 的 replay scope；
- root subscribe 的 capability/profile 前置条件，以及 child subscribe 的确定性
  `run_not_root` 拒绝；
- process restart 后完整 tree query 与 cold recovery；
- replay 不可用时 query 能恢复到同一 durable truth；
- 不从 transcript、live registry 或 event 时序反推 tree identity。

冻结契约裁决：

- `runs.subscribe` 始终寻址 active **root** Segment；child Run 即使 Running 也不是独立
  订阅根，必须返回 `run_not_root`；
- 一个 root Journal 承载整棵树的多 source Run event，cursor scope 仍是
  `(process epoch, root run, root segment)`；
- 因此 W3 不新增 child stream、child Journal 或 child subscribe capability 分支。

内部原子切片：

| Slice | 状态   | 边界                                | 完成定义                                                                                                                                                             |
| ----- | ------ | ----------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| W3.0  | `DONE` | 冻结契约与实现差距审计              | 纠正 child subscribe 误述；确认 `runs.get`、root stream、tail-only replay、profile gate 与 tree-aware boot recovery 已有事实；定位 descendant query 三项真实缺口     |
| W3.1  | `DONE` | durable descendant query            | `runs.list.includeDescendants` 进入 application/store；cursor 绑定该 filter；`items.list` run subtree scope 不丢失；每页 Runs 包含直接引用 Run 与完整 ancestor chain |
| W3.2  | `DONE` | root stream / replay conformance    | 多 source event 共用 root cursor；child subscribe 拒绝；profile 不覆盖时拒绝；tail-only + query 在边界竞态下不漏 authoritative terminal                              |
| W3.3  | `DONE` | restart / cold recovery conformance | file-backed restart 后完整 Running tree canonical `run_lost` 且保留最后 committed facts；完整 Waiting tree 保留；旧 epoch / 已恢复终态均收敛到 durable query         |
| W3.4  | `DONE` | B1.5 full closure                   | Runtime 高风险 race、全量门禁、contract drift、命名/错误/接口/兼容债审计与文档同步                                                                                   |

W3.1 的所有权：

- delivery 只把 wire scope/filter 翻译成 application query；
- application query 拥有 scope、cursor identity 与 page enrichment 语义；
- SQLite projection 只实现 descendant membership、稳定 keyset seek 与 ancestor closure；
- durable `runs` lineage 是唯一 tree identity，不允许扫描 transcript 或 live registry
  猜父子关系；
- 这些都是 App consumer 侧能力，不要求修改 Agent Framework public API。

W3.1 完成结果（2026-07-30）：

- application `ItemScope` 改为私有字段的闭合值，只能由 Session、exact Run 与 Run
  subtree 三个构造器产生，非法的双 subject / Session descendants 状态不可构造；
- `RunPageFilter` 显式聚合 session、normalized statuses 与 descendant filter；
  runs/items cursor 都绑定 `includeDescendants`，exact 与 subtree cursor 不可跨用；
- SQLite `PageRuns` 在同一 `(createdAt DESC, runId DESC)` keyset 上选择 roots 或全部
  descendants；`PageRunTreeItems` 用 durable `parent_run_id` 递归选择任意 child
  subtree；
- `RunsWithAncestors` 一次递归查询本页直接 Run 与完整 ancestor closure，允许 ancestor
  位于请求 subtree 之外，不扫描 Session 全量 Runs；
- delivery 只保真翻译已由 Registry capability rule 接受的 filter；public wire、
  Registry、schema、OpenRPC、Artifact、Store epoch、Agent API 与 capability 均未变化；
- application queries、delivery server、SQLite 定向测试与 race `-count=10` 通过；
  Runtime build、vet、全量 test、lint、tidy diff、arch/contract drift 与 diff check 全绿。

W3.2 完成结果（2026-07-30）：

- 真实 nested root → child → grandchild fixture 证明所有 source event 的 envelope 保留各自
  Run/Segment，但 `Sequence` 连续且每个 opaque cursor 都绑定同一
  `(process epoch, root run, root segment)`；
- `runs.subscribe(child)` 在 delivery 边界确定性映射为 `run_not_root`，不创建 child
  Journal 或降级到 root；
- 既有 application profile gate 继续一次返回完整 `ProfileNotCovered`，delivery 将其
  映射为结构化 capability gap，不删减 stream；
- tail-first fixture 覆盖 terminal 在 attach 前、attach 后/query 前、query 后三种
  线性化；结合既有 commit-before-publish 不变量，durable snapshot 与 buffered tail
  至少一侧必含 terminal；
- 只增强 conformance tests；production、public wire、Registry、schema、OpenRPC、
  Artifact、Store epoch、Agent API 与 capability 均未变化；
- application/runs 与 delivery/server race `-count=10`、Runtime build/vet/全量
  test/lint、arch/contract drift、tidy diff 与 diff check 全绿。

W3.3 完成结果（2026-07-30）：

- recovery 删除独立的缩水版 `nonTerminalRun` row model，统一复用完整 Run decoder；
  只在 boot repair 显式允许 Interrupted row 缺失 root-owned Pending，其余 lineage、
  accounting、limits、model 与 profile 约束不降级；
- `recoverLostRun` 从完整 durable aggregate 演进终态，不再重建字段子集，因此每个
  root/child/nested Run 的最后 committed metrics、limits、model selection、
  root-owned protocol profile、lineage 与 creation time 全部保留；
- 真实 file-backed close/reopen fixture 证明 Running root → child → grandchild 全树
  收敛为 `failed(error:run_lost)`，清空 active Segment、结算 terminal boundary 并释放
  admission；完整 Waiting root/child tree 则保留 Pending、continuations 与 process
  snapshot；
- stream epoch 与 lifecycle 使用明确优先级：新进程仍有 active stream 时 old epoch
  返回 `replay_unavailable` 后读 durable projection；boot 已先把 orphan 收口时返回
  `run_finished`，cold query 读取 `run_lost`，不为旧 cursor 伪造 replay；
- production change 仅限 SQLite recovery/read 聚合边界；public wire、Registry、
  schema、OpenRPC、Artifact、Store epoch、Agent API/wire 与 capability 均未变化；
- SQLite、application/runs、delivery/server race `-count=10`、Runtime
  build/vet/全量 test/lint、arch/contract drift、tidy diff 与 diff check 全绿。

W3.4 完成结果（2026-07-30）：

- query scope 的存储错误与 not-found 分支改为显式控制流，删除降低可读性的
  `cmp.Or`；`statuses` 非法值错误改为直接指出字段和值；
- 删除 production 注释中与冻结契约冲突的 “child stream enabled” 残留，明确
  capability 打开的是完整 child Run feature，而非独立 child stream；
- recovery scan policy 从 SQL `join` 术语改为 root-owned Pending set 语义；普通读取
  的损坏错误明确为 `interrupted with no root-owned Pending set`；
- Run row decoder 只描述 row/run 解码上下文，每个 store use case 统一添加一次
  `sqlite:` operation context，清理重复 `sqlite: ...: sqlite: ...` 错误链；
- production receiver 按类型扫描保持唯一命名：`RunStore=s`、`TranscriptStore=s`、
  `Coordinator=c`、`Server=s` 等；没有再次出现同一 receiver 的 `s/base` 混用；
- consumer ports、文件职责和依赖方向复核通过：query port 只暴露实际读用例，
  run codec/recovery/validation 各自高内聚；未为行数机械拆层，也未增加 repository
  base、manager 或第二套模型；
- root-only/single-Run 假设、child Journal/stream、旧 cursor fallback、compatibility
  decoder、双读写、migration、TODO/FIXME/HACK 扫描无生产残留；
- public wire、Registry、schema、OpenRPC、Artifact、Store epoch、Agent API/wire 与
  capability 均未变化；`features.subagents` 仍 disabled；
- 四个 B1.5 高风险 package race `-count=10`、runsegment/bootstrap race、Runtime
  build/vet/全量 test/lint/vuln、tidy、arch/contract drift 与 diff check 全绿。

完成定义：

- query、stream、replay、cold recovery 四个事实面一致；
- capability 未协商时显式 `capability_not_negotiated`；
- 不返回看似完整的降级结果；
- 若 machine contract shape 未变化，不机械重生成 Registry/Go/TS；行为测试与 canonical
  docs 必须同步；
- Runtime 全量与高风险 race 门禁通过。

### W4 — B1.6 Desktop Run-tree consumer

状态：`DONE`；`W4.0 DONE · W4.1 DONE · W4.2 DONE · W4.3 DONE · W4.4 DONE`

专项执行卡：

- [`codex_desktop_run_tree_execution_plan.md`](codex_desktop_run_tree_execution_plan.md)

W4.0 审计裁决：

- transport 已正确承载、过滤、去重和重连完整 Run tree，主要缺口在 Agent
  projection、cold recovery、runtime invalidation 与 cancel consumer；
- 当前 `view.run`、global `turnMessageId/plan/error` 无法表达 child/sibling/nested；
- child start/finish 仅写 timeline、child progress 丢弃，只是防止污染 root，不是
  first-class Run-tree consumer；
- target 冻结为 Session narrative + normalized `runsById` + source-owned
  message/tool/plan/timeline/interrupt；
- live fold 必须接收完整 RunEvent provenance；cold/local path 使用 snapshot/local
  mutation，不再伪造空 RunID、unknown profile 或 zero metrics StreamEvent；
- 不保留 `view.run` alias、dual write 或 compatibility reducer。

交付：

- frontend reducer 按完整 RunEvent envelope 的 `source Run` 折叠 root stream；
- root、child、sibling、nested child 拥有独立状态与 Item timeline；
- tree 展开/折叠、Waiting 状态、取消交互和 parent tool result 一致；
- reconnect 使用 replay，replay 不可用使用 cold refetch；
- runtime invalidation 只触发权威 query，不自行补造状态；
- 协议 fold 留在 Agent bounded context，组件不直连后端；
- command、event、selector 和 extension point 是跨 context 的唯一协作面。

完成定义：

- exhaustive reducer 覆盖全部 tree event；
- root/child cancel UI 只发送现有 `runs.cancel`；
- 乐观状态最终与 committed response/query 对账；
- frontend layer guards、typecheck、lint、format、tests、knip、circular 和 bundle gate 全绿；
- 不手写第二份 wire union，不把 transport 状态泄漏进组件。

原子切片：

| Slice | 状态   | 边界                                                                                                               |
| ----- | ------ | ------------------------------------------------------------------------------------------------------------------ |
| W4.0  | `DONE` | 现状、爆炸半径、目标模型与全门禁基线                                                                               |
| W4.1  | `DONE` | canonical Session/Run-tree projection、完整 provenance、source-owned fold，删除 single-run shape 与 synthetic wire |
| W4.2  | `DONE` | durable snapshot、replay/cold/invalidation、root/child committed cancel response merge                             |
| W4.3  | `DONE` | root-first narrative、task child disclosure、tree/timeline/cancel UI                                               |
| W4.4  | `DONE` | start/resume exact ack、无启发式对账、scope-exact Run API、architecture/docs/full gates                            |

### W5 — B1.7 最终 conformance 与 capability enablement

状态：`DONE`

只有以下项目全部通过后，才能将 `features.subagents.enabled` 改为 `true`：

- Running / Waiting child cancel 两条 Waiting disposition；
- root/child/resume/terminal 全竞态；
- subtree quiescence；
- parent `child_run_canceled` exactly once；
- child get/list/items 与 child subscribe 的 `run_not_root` 拒绝；
- restart query/recovery；
- negotiated-capability 下的 frontend tree reducer / durable recovery / exact cancel；
- Contract Registry、schema、OpenRPC、manifest、API Reference、Go/TS validator 与 SDK 一致；
- runtime/frontend 完整门禁；
- generation 连续两次无 diff；
- 历史兼容扫描无生产命中。

Capability enablement 必须与实现、生成物、客户端、canonical docs 和测试在同一个原子
slice 中完成，不能先改布尔值。

原子切片：

| Slice | 状态   | 边界                                                                |
| ----- | ------ | ------------------------------------------------------------------- |
| W5.0  | `DONE` | read-only completion audit：逐项绑定现有 producer/conformance 证据  |
| W5.1  | `DONE` | 根治 profile/恢复/Artifact/contract 缺口并原子启用 server + Desktop |
| W5.2  | `DONE` | Runtime/Desktop 高风险竞态、全门禁、双 generation 与无兼容残留收口  |

### W6 — Runtime / Desktop 架构最终复核

状态：`DONE`；`W6.0 DONE · W6.1 DONE · W6.2 DONE · W6.3 DONE`

审计维度：

- package 是否按领域/完整用例内聚，而不是按泛化技术名堆叠；
- application/domain 是否依赖外环具体实现；
- delivery 是否持有任何业务裁决；
- consumer ports 是否过胖或被实现方定义；
- 单实现内部胶水是否被机械接口化；
- domain 的无 I/O 不变量是否仍散落在 service/SQL/wire；
- 是否存在 `Manager`、`Helper`、`Impl`、`Data`、`Info` 等失真命名；
- 文件名、接口名、方法名、变量/常量名是否表达真实语义；
- 同一 receiver 命名是否一致；
- 错误是否包含“操作 + 对象 + 原因”，并用 `%w` 保留因果；
- 是否存在反射/`any`/type switch 掩盖明确领域联合；
- 是否出现兼容分支、死代码、过期注释或 TODO；
- goroutine 是否有 owner、停止路径与 join；
- Desktop store 是否只持有 context state，而非变成跨上下文 god store；
- plugin kernel 是否保持薄，业务能力是否归所属 bounded context。

发现坏味道时按根因和 owner 分批修复；不得为“统一形式”做无收益搬迁。

W6.0 的证据裁决：

- Runtime package graph 未出现 domain/application 向 delivery、bootstrap 或具体
  SQLite 实现的反向依赖；delivery 既有 consumer-defined use-case ports，架构测试已
  锁定 domain purity、application framework-free、delivery 无生命周期裁决以及
  bootstrap 无业务状态；
- `application/runs` 的 `SessionLifecycle`、`TurnControl` 与 `Effects` 虽然方法数
  高于经验值，但分别承载一个完整 Run 用例所需的生命周期、execution control 与原子
  effect boundary。当前没有第二组消费者以不同子集使用它们，机械拆分只会把一次
  transaction/use case 切成转发接口，裁决为**保留**；
- Agent production graph 不依赖 `app/**`，public execution vocabulary 中没有
  Session/Run/Segment/Item、SQLite、BuildID、idempotency store、transaction 或
  产品计费账本；process snapshot、无资源保留的 waiting-subtree Plan/Apply 与 opaque cost
  projection 都对任意 Framework 消费者成立，裁决为**不迁移、不增加 App seam**；
- Runtime/Agent production 未发现 `Manager`、`Helper`、`Impl`、generic
  `impl.go/helper.go/utils.go`、常量专用 `fmt.Errorf`、同类型 receiver 多命名或精确
  `TODO/FIXME/HACK` 残留；goroutine callsite 均落在已有 process/taskgroup/
  shutdown/transport owner 范围，后续只在真实生命周期缺口出现时调整；
- Desktop 现有跨 context `public/`、design-system rings、adapter-only composition
  root 等 guards 有效，但 Runtime bounded context 存在两个同根问题：
  application 直接读取 `main/config`、SDK 全局配置与 i18n；capability discovery
  application 直接持有 `RpcClient`、`DiscoverResponse` 和
  `"runtime.discover"` transport method；
- `main/config.RUNTIME_BASE` 同时表示“用户可切换的 Runtime 协议 endpoint”和“固定
  本地 desktop shell base URL”。二者仅当前值相同，生命周期和变化原因不同，属于
  偶然相等的两个事实，不应共享一个常量作者；
- W6.0 将 `lib/utils.ts` 识别为语义偏泛的独立命名问题；W6.2 已将文件改为
  `lib/classNames.ts`。函数名 `cn` 则保留，因为它是 `components.json` / shadcn
  generator 的稳定工具契约，不是内部职责不明的缩写。

冻结的原子切片：

| Slice | 状态   | 唯一边界                                                       | 完成定义                                                                                                                                                                                                       |
| ----- | ------ | -------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| W6.0  | `DONE` | read-only architecture evidence audit                          | 每个“改/不改”裁决均绑定依赖、owner、consumer 或行为证据；不按行数和目录形式制造工作                                                                                                                            |
| W6.1  | `DONE` | Desktop Runtime endpoint 与 discovery anti-corruption boundary | application 只依赖两个 consumer ports；错误返回稳定语义而非本地化文案；typed SDK call 只在 adapter；local shell URL 与 selectable endpoint 分属不同事实作者；新增通用 architecture guard；删除旧入口且无 alias |
| W6.2  | `DONE` | Desktop semantic naming / stale commentary                     | 有证据的失真类型、函数、文件与 public surface 已语义化；rename 无行为变化；generic production filename 与 application/domain suffix guard 已落地                                                               |
| W6.3  | `DONE` | W6 full closure                                                | Runtime、Agent、Desktop 全量 gates、高风险 race、dependency/leak/compat scan、生成物 no-diff 与文档收口均通过                                                                                                  |

W6.1 breaking blast radius：

- 删除 `main/config` 中含混的 `RUNTIME_BASE` 与 Runtime endpoint config key；
- local shell/sideload 改用明确的 local shell base URL；
- Runtime application 通过 replacement-safe singleton ports 读写 endpoint、执行
  discovery，不再导入组合根、全局 store、raw RPC client 或 i18n；
- endpoint 修改结果改为 `applied | rejected` 闭合联合，拒绝原因使用稳定 code，
  Settings UI 是唯一文案翻译者；
- discovery adapter 调用 typed `client.runtime.discover()` 并只向 application
  返回 capabilities，不透传 response envelope；
- 删除 `endpointMirror.ts`、`runtimeRpc.ts` 等旧路径，不保留 re-export、dual path
  或 compatibility alias；
- `main/container` 的 Runtime client URL 通过 Runtime public application surface
  读取；在 plugin 尚未安装时，default endpoint 仍是明确且可回答的值；
- architecture guard 从若干 context 特例提升为通用规则：
  builtin application/domain 不得依赖 `main/**`，application 不得持有 raw
  `RpcClient` / `DiscoverResponse`。

### W7 — Synara 视觉基线与像素级 Desktop UI 对齐

状态：`DONE`；`W7.0 DONE · W7.1 DONE · W7.2 DONE · W7.3 DONE · W7.4 DONE · W7.5 DONE`

参考仓库：`~/Desktop/synara`

实施顺序：

1. 审计参考仓库的 shell、布局、间距、字体、surface、颜色、阴影、边界、动效和状态；
2. 建立 Lyra 与 Synara 的页面/组件/状态映射，不复制其业务架构；
3. 先收敛 design tokens，再实现 shell、Work Index、Agent Narrative、Context Dock；
4. 将交互 primitive 留在 Base UI，视觉落在 Lyra atoms/agent shell；
5. 覆盖空态、加载、Waiting、Running、Finished、错误、长文本、窄窗口和高密度 tree；
6. 使用固定 viewport、截图基线和视觉 diff 做像素级验收；
7. 真机验证 Wails/WebView 下的字体、DPI、滚动、焦点、拖拽和窗口边缘行为。

边界：

- 复刻布局与视觉品质，不复制 Synara 的领域模型、状态管理或协议；
- 不为视觉相似牺牲可访问性、插件边界或响应式稳定性；
- 不在 callsite 手搓 token、focus、border、shadow 或交互 primitive；
- 不通过绝对定位和固定文案制造只在截图上正确的页面。

完成定义：

- 目标页面在约定 viewport 下达到可复核的像素级一致；
- 长内容、不同状态和窗口缩放不破版；
- keyboard/focus/aria 与 motion preference 正确；
- `npm run check` 全绿；
- 视觉基线、差异说明和剩余有意分歧有记录。

原子切片：

| Slice | 状态   | 唯一边界                                                                            | 完成定义                                                                                                                                          |
| ----- | ------ | ----------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| W7.0  | `DONE` | read-only reference audit、页面/状态映射、固定 viewport 与截图基线                  | 已在 `codex_synara_visual_baseline_and_execution_plan.md` 冻结 shell、组件、token、状态、复刻项、有意分歧和截图；production code 无变化           |
| W7.1  | `DONE` | visual foundation：fixture、字体、颜色、间距、圆角、边界、阴影、层级与 motion token | production-backed fixture 覆盖权威状态；token 有唯一作者；CSS/build warning 与 motion hygiene 门禁已闭环；实施提交 `051ea578d`                    |
| W7.2  | `DONE` | desktop shell 与 Work Index                                                         | production shell、导航密度、四种 query 状态、single-owner toggle、pointer/keyboard resize、窄窗与 Retina 已闭环；实施提交 `8bb0fa7ba`             |
| W7.3  | `DONE` | Agent Narrative、composer、Run tree 与 HITL                                         | 12 个 canonical state、production projection、精确交互与明暗 golden 已闭环；实施提交 `9c87d93fc`                                                  |
| W7.4  | `DONE` | Context Dock、workspace views 与 settings                                           | production view/plugin path、独立 density width、精确 navigation identity、Settings/overlay 语义与 12 张明暗 golden 已闭环；实施提交 `d0380d0a2`   |
| W7.5  | `DONE` | responsive、accessibility、Wails/WebView 与 visual regression closure               | 120 项 Chromium/WebKit 矩阵、WCAG/键盘/IME/clipboard/粗指针/字号/DPR、真实 Wails 窗口及全量门禁闭环；实施提交 `ed411fe93`、`c5590aad0`               |

---

## 7. API 与代码人体工程学检查表

每个 slice 在设计和收工时各检查一次。

### 7.1 API

- 方法名描述用户意图，不描述内部执行步骤；
- command 返回同一提交边界后的 exact result，不事后查询拼装；
- query 返回权威 projection，stream 不承担唯一正确性；
- 必填 identity 显式传递，不从字符串、顺序、最近事件或参数相等猜测；
- 闭合联合由 discriminator 表达并 fail closed；
- 列表有稳定顺序、分页和边界语义；
- capability 是协议能力，不是实现完成度遮羞布；
- 错误有稳定 problem type、准确字段路径和可行动 recovery；
- transport metadata 不进入业务 envelope；
- 扩展 payload 的开放性不能扩大核心协议对象。

### 7.2 Go API

- 接口由消费方定义，通常保持 1–3 个方法；
- accept interfaces，return structs；
- DTO 保持数据，领域值拥有无 I/O 不变量和派生行为；
- 不使用 `GetX`、package stutter、Java 后缀或空泛名；
- 同一类型 receiver 命名一致；
- 文件名描述职责，不使用 `impl.go`、`helper.go`、`utils.go`；
- 常量错误用 `errors.New`，包装用 `%w`；
- context 贯穿 I/O 和长任务，不存入 struct；
- slice/map 在跨 goroutine 或跨边界前复制；
- 并发顺序和 shutdown path 显式；
- 不用 reflect/generic carrier 回避清晰的领域方法；
- 新抽象必须指出真实消费者、替换点或已发生的重复。

### 7.3 Frontend API

- generated wire、application intent、view model 三层分离；
- components/pages 不 import composition root 或 concrete RPC client；
- reducer 只派发，领域 fold 归所属 plugin context；
- selector 读、command 写，render 路径不回写 store；
- 外部 wire/用户输入用 Zod，内部 typed flow 不重复校验；
- state ownership 明确，不建立跨 context god store；
- UI primitive、atom、agent shell 依赖单向；
- 视觉变化不改变协议或领域事实。

---

## 8. 测试与质量门禁

### 8.1 每个实现 slice

最低顺序：

1. 先写能证明目标语义的失败测试；
2. 实现根因修复；
3. 跑 owner package targeted tests；
4. 跑相关 adapter/SQLite/integration tests；
5. 涉及并发时跑 `-race`，并使用 channel、明确 barrier 或 `testing/synctest`；
6. 跑 module build、vet、test、lint；
7. 跑 `go mod tidy -diff`；
8. 跑 architecture/API/wire/diff gate；
9. 同步文档和进度；
10. 单独 commit 并 push。

### 8.2 Agent 阶段门

```bash
MODULE=agent scripts/check.sh build vet test lint
MODULE=agent scripts/check.sh race
cd agent && go mod tidy -diff
```

### 8.3 Runtime 阶段门

```bash
MODULE=app/runtime scripts/check.sh build vet test lint vuln
cd app/runtime && go test -race ./internal/application/runs/... \
  ./internal/adapter/agentexec/... \
  ./internal/adapter/runsegment/... \
  ./internal/infra/storage/sqlite/...
cd app/runtime && go mod tidy -diff
```

具体 package 集随改动面调整，但不能用 narrower gate 代替阶段退出的全量 gate。

### 8.4 Desktop 阶段门

```bash
cd app/desktop/frontend
npm run check
```

协议变化还必须验证：

- contract generation 连续两次无 diff；
- schema / OpenRPC / manifest / API Reference / Go validator 同源；
- TypeScript types / validator / method map / SDK 同步；
- canonical API / AUX_API / TRANSPORT 描述实际行为；
- compatibility differ 正确识别 breaking；
- 生产代码和当前生成物不存在旧 shape、alias、decoder 或双路径。

### 8.5 提交纪律

- 一个原子语义一个 commit；
- 不提交其他人或其他工作流的未完成改动；
- commit message 写清为什么；
- commit 必须包含：

```text
Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
```

- 完整门禁通过后及时 push；
- 未经单独授权不 tag、不 release；
- `DONE` 必须有 commit、测试证据和文档记录，三者缺一不可。

---

## 9. 风险台账

| 风险                                   | 后果                                            | 控制                                                                     |
| -------------------------------------- | ----------------------------------------------- | ------------------------------------------------------------------------ |
| Agent 吸收 App transaction/idempotency | Framework 被单一消费者污染                      | P24 ownership gate；Agent public API/AST 审查                            |
| App 解析或改写 FrameworkState          | checkpoint schema 泄漏、升级脆弱                | 只调用 Agent prepared mutation                                           |
| durable commit 与 live mutation 分裂   | restart 复活已取消 child 或 live/durable 不一致 | freeze → complete App transaction → infallible prepared commit；故障注入 |
| parent tool 双重完成                   | transcript 与模型上下文冲突                     | committed-tool projection + exact identity + exactly-once tests          |
| 最后一个 Interrupt 被当成普通 resume   | 伪造用户输入或产生第二 barrier                  | framework-ready continuation；无 Resume event                            |
| cancel / resume / terminal 多 owner    | 双提交或孤儿状态                                | root tree admission + parked claim + transaction CAS                     |
| Query 从 live state 推断 child         | restart 后读面不一致                            | durable tree identity 与 projection 是唯一读事实                         |
| Frontend 按 root 合并所有 source       | sibling/child 内容串线                          | source-aware reducer 与 exhaustive tests                                 |
| 为 Clean Architecture 机械加层         | 接口泛滥、调用链变长                            | ownership-first；单实现内部直接用 concrete type                          |
| 为 KISS 拒绝必要抽象                   | 同一规则散落多层                                | 一个事实一个作者；consumer-defined port                                  |
| 命名和错误在大改中漂移                 | API 难读、诊断不可行动                          | 每 slice hygiene sweep + lint/arch gate                                  |
| UI 像素复刻污染业务架构                | 视觉目标绑死状态和协议                          | 只映射视觉；先 token，再 shell，再 feature                               |
| 文档宣称超过实现                       | capability 或行为失真                           | Git/CI 决定进度；无证据不标 DONE                                         |

---

## 10. 已接受的关键决策

| ID   | 决策                                                                            | 状态       |
| ---- | ------------------------------------------------------------------------------- | ---------- |
| M-01 | 冻结契约是目标真相，当前实现不是弱化理由                                        | `ACCEPTED` |
| M-02 | dev 阶段直接 breaking，不保留历史兼容                                           | `ACCEPTED` |
| M-03 | Agent 是 Framework；App 是消费者                                                | `ACCEPTED` |
| M-04 | 幂等、原子性、事务、BuildID 和产品账本归 App                                    | `ACCEPTED` |
| M-05 | Agent 只提供 execution 原语与核心运行时逻辑                                     | `ACCEPTED` |
| M-06 | Waiting child cancel 使用 Agent prepared mutation + App transaction             | `ACCEPTED` |
| M-07 | `runs.cancel` 同时服务 root/child，不新增 child 专用方法                        | `ACCEPTED` |
| M-08 | command 返回 exact committed snapshot，不 post-commit re-query                  | `ACCEPTED` |
| M-09 | 完整 child 能力闭环前 `features.subagents=false`                                | `ACCEPTED` |
| M-10 | Clean Architecture 是所有权/依赖方向，不是层数；DDD 是语言/不变量，不是框架仪式 | `ACCEPTED` |
| M-11 | Desktop 保持 plugin kernel，业务插件演进为 bounded contexts                     | `ACCEPTED` |
| M-12 | UI 参考 Synara 的布局与视觉，不复制其业务架构                                   | `ACCEPTED` |
| M-13 | 质量、命名、错误与架构清理属于每个 slice 的完成定义                             | `ACCEPTED` |

---

## 11. 进度更新规则

状态只使用：

- `TODO`：未开始；
- `IN PROGRESS`：已有实际工作，但未完成全部验收；
- `DONE`：目标、实现、测试、文档、commit 和 push 全部闭环；
- `DEFERRED`：有明确理由与重新进入条件；
- `BLOCKED`：存在当前执行者无法消除的外部阻塞；
- `ONGOING`：贯穿所有 slice 的持续性纪律，不替代具体完成状态。

每完成一个 slice，必须同时更新：

1. 本文 §4 总览、§5 关键路径和对应 W 项；
2. Runtime / Agent 专项台账；
3. 如边界变化，更新架构基准与 ADR；
4. commit SHA、生成物、测试命令和残余风险；
5. 下一项唯一焦点。

更新记录使用：

```md
### YYYY-MM-DD — W?

- 状态：DONE
- Commit：<sha>
- 目标：<一句话>
- 事实作者：<修改的 owner>
- 关键裁决：<最终语义与放弃的错误方向>
- 生成物：<schema / OpenRPC / TS / snapshot / store epoch 等>
- 验证：
  - `<command>` → PASS
- 架构复核：
  - 抽象不过度 / 不足 → PASS
  - Agent/App/Delivery/Desktop 无泄露 → PASS
  - 无兼容路径与历史债务 → PASS
- 残余风险：无 / <进入哪个后续 slice>
- 下一步：<唯一下一任务>
```

不得用“代码写完”“能编译”或“测试大部分通过”代替 `DONE`。

### 2026-07-30 — W1

- 状态：`DONE`
- Commit：`a4e153fd4`
- 目标：把 parked child subtree cancellation 收口为 prepared execution mutation +
  App-owned atomic write-set。
- 事实作者：application 决定 tree transformation；runsegment transaction 提交 durable
  truth；Agent runtime 只冻结并应用 execution replacement。
- 关键裁决：Pending 消费、replacement checkpoint、canonical terminal Runs、parent
  `child_run_canceled`、reduced Pending 或 surviving Segment openings 必须同事务；最终
  boundary 由 Continue 推进，不能伪造 Resume。
- 生成物：协议/OpenRPC/Artifact shape 不变；App Interrupt checkpoint payload 增加
  committed-tool projection；SQLite Item exact CAS 与 tree resume timestamp 进入事务端口。
- 验证：
  - `MODULE=agent FAST=1 scripts/check.sh build vet test lint` → `PASS`
  - `MODULE=app/runtime FAST=1 scripts/check.sh build vet test lint` → `PASS`
  - Agent Runtime 与 App runs/agentexec/turn/runsegment/SQLite 高风险 race → `PASS`
  - real SQLite commit/rollback、application-level restart Rehydrate、Abort、
    remaining/final boundary → `PASS`
- 架构复核：
  - 抽象不过度 / 不足 → `PASS`；没有通用 transaction framework，prepared capability
    只存在于真实 owner boundary
  - Agent/App/Delivery/Desktop 无泄露 → `PASS`
  - 无兼容路径与历史债务 → `PASS`
- 残余风险：完整竞态、重复命令、teardown failure 与 exact query matrix 进入 W2。
- 下一步：W2 / P24-04 / B1.4d。

### 2026-07-30 — W2.1

- 状态：`DONE`
- Commit：随本原子 slice 提交
- 目标：证明 root cancel、Waiting child cancel、resume、natural child terminal 与
  duplicate cancel 只能产生一个 tree owner 和一份 durable truth。
- 事实作者：parked tree 继续由 application admission 决定 owner；Running tree 继续由
  root handle arbiter 决定 owner；Agent/App persistence 所有权未变化。
- 关键裁决：所有 losing mutation 使用稳定的 `ErrSessionBusy`；自然终态已提交时使用
  `ErrRunFinished`。为此只在现有 arbiter 根因处修正 root-first child loser 的 wrapping，
  不新增并发协调层。
- 生成物：protocol、OpenRPC、Agent API/wire、Artifact 与 SQLite schema 均不变。
- 验证：
  - ownership arbitration 普通测试 `-count=10` → `PASS`
  - ownership arbitration race `-count=10` → `PASS`
  - Agent / Runtime build、vet、test、lint → `PASS`
  - Agent / Runtime `go mod tidy -diff` → `PASS`
- 架构复核：
  - 抽象不过度 / 不足 → `PASS`；复用 admission 与 root handle 两个既有真实 owner
  - Agent/App/Delivery/Desktop 无泄露 → `PASS`
  - 无兼容路径与历史债务 → `PASS`
- 残余风险：stale Pending、checkpoint / transaction / teardown / Continue 的逐点失败矩阵
  进入 W2.2。
- 下一步：W2.2。

### 2026-07-30 — W2.2

- 状态：`DONE`
- Commit：随本原子 slice 提交
- 目标：证明 waiting-subtree cancellation 的每个 pre-commit failure 都整体回滚，
  post-commit Continue/teardown failure 都由原 owner 收口且可重试。
- 事实作者：App runsegment transaction 仍是 durable write-set 的唯一提交者；Agent
  prepared mutation 仍只拥有 execution replacement；Coordinator/pump 与 turn dispatcher
  分别拥有 application segment 和 executor turn 的 post-commit terminal/cleanup。
- 关键裁决：stale Pending 使用 `ErrSessionBusy`；已经提交的 opening 不执行反向补偿，
  而是 error-terminalize surviving tree；执行器清理失败保留 owner 供 shutdown 重试。
- 生成物：protocol、OpenRPC、Agent API/wire、Artifact、SQLite schema 与 capability 均不变。
- 验证：
  - SQLite pre-commit failure matrix → `PASS`
  - application/turn post-commit Continue 与 teardown matrix → `PASS`
  - 高风险 race `-count=10` → `PASS`
  - Agent / Runtime build、vet、全量 test、lint、tidy diff、diff check → `PASS`
- 架构复核：
  - 抽象不过度 / 不足 → `PASS`；failure seam 只放在现有 owner 边界
  - Agent/App/Delivery/Desktop 无泄露 → `PASS`
  - 无兼容路径、双写或历史债务 → `PASS`
- 残余风险：file-backed restart、exact query/publication 与 canceled-subtree quiescence
  进入 W2.3；完整 race/hygiene 收口进入 W2.4。
- 下一步：W2.3。

### 2026-07-30 — W2.3

- 状态：`DONE`
- Commit：随本原子 slice 提交
- 目标：证明 waiting-subtree cancellation 的 committed truth 经进程重启仍可精确读取，
  canceled subtree 不复活，publication 与返回后事件边界一致。
- 事实作者：App runsegment transaction 继续唯一拥有 durable write-set；SQLite boot
  recovery 以 root-owned Pending + 完整活动 Run tree 为恢复单位；Agent API 与 execution
  ownership 未变化。
- 关键裁决：
  - target subtree 的 Running Question/Approval Item 必须与 Runs、parent Item、Pending 和
    checkpoint 在同一 transaction 内结算；
  - boot 以 Pending continuations 精确覆盖所有 non-terminal tree members，逐 member 校验
    lineage/model/time/interrupt/drained/committed tool；
  - remaining tree 可保留 Interrupted；final-boundary 的 Running root 在 W3 前仍按
    `run_lost` 收口，不伪造 cold recovery。
- 生成物：protocol、OpenRPC、Agent API/wire、Artifact、SQLite schema 与 capability 均未变化。
- 验证：
  - file-backed close/reopen + exact query/checkpoint/Pending → `PASS`
  - tree-aware `ReconcileOrphans` + canceled subtree non-resurrection → `PASS`
  - exact invalidation read set + post-return Journal quiescence → `PASS`
  - 三组高风险 race `-count=10` → `PASS`
- 架构复核：
  - App persistence/transaction/idempotency ownership → `PASS`
  - Agent/App/Delivery/Desktop 无概念泄漏 → `PASS`
  - 无兼容 reader、双写、schema/API/capability 变化 → `PASS`
- 残余风险：完整 canonical-order、命名/错误/receiver/接口与兼容债扫描，以及 Agent /
  Runtime 全量门禁进入 W2.4。
- 下一步：W2.4。

### 2026-07-30 — W2.4

- 状态：`DONE`
- Commit：随本原子 slice 提交
- 目标：以重复 race、架构/hygiene 审计与全量门禁关闭 P24/B1.4，不遗留兼容债或
  consumer/framework 概念泄漏。
- 事实作者：领域 `RunTree` 继续唯一拥有 canonical ordering；App 继续拥有事务/幂等/
  persistence；Agent 只表达 caller-coordinated execution replacement。
- 关键裁决：receiver 和静态错误坏味道全仓清零；Agent GoDoc 不借用 App 的
  Run/Item/Interrupt/Segment 或 durable transaction 心智；大而内聚的用例文件不按行数
  机械拆分。
- 生成物：protocol、OpenRPC、Agent API/wire、Artifact、SQLite schema 与 capability
  均未变化；`features.subagents` 继续 disabled。
- 验证：
  - Agent runtime/toolloop、Runtime runs/runsegment/SQLite race `-count=10` → `PASS`
  - Agent / Runtime build、vet、全量 test、lint、tidy diff → `PASS`
  - contract/arch drift、receiver/错误/兼容扫描、`git diff --check` → `PASS`
- 架构复核：
  - 抽象不过度 / 不足 → `PASS`
  - Agent/App/Delivery/Desktop 无概念泄漏 → `PASS`
  - 无兼容 reader、shim、双写或 migration → `PASS`
- 残余风险：通用 descendant paging、root multi-source stream conformance 与完整
  Run-tree cold recovery 明确进入 W3/B1.5。
- 下一步：W3 / B1.5。

### 2026-07-30 — W3.0

- 状态：`DONE`
- Commit：随本次文档审计提交
- 目标：逐条对照冻结协议，固定 B1.5 的真实差距、原子切片和所有权，避免按旧计划
  实现错误的 child subscription。
- 事实作者：冻结协议继续唯一定义 public behavior；application query 拥有 scope、
  cursor 与 enrichment；SQLite 只拥有 durable projection；root Journal 继续拥有
  tree-wide live stream。
- 关键裁决：
  - `runs.subscribe(child)` 的正确结果是 `run_not_root`，不是新增 child stream；
  - 当前 `runs.get(child)`、root tail-only/replay/profile gate 与 tree-aware boot
    reconciliation 已有实现基础；
  - 当前真实缺口是 `runs.list.includeDescendants` 被 server 丢弃、
    `items.list.scope.includeDescendants` 被 delivery 丢弃，以及
    `items.list.runs` 缺少 ancestor chain。
- 生成物：本 slice 只修正文档与计划；public wire、Registry、schema、OpenRPC、
  Go/TS 类型、Artifact、Store epoch 与 capability 均不变。
- 验证：
  - 冻结契约 §5.4、§6.3、§8.1、§14.2、§14.4 与实现入口逐项核对 → `PASS`
  - application/server/SQLite query ports、Journal/Subscribe、ReconcileOrphans 只读审计
    → `PASS`
- 架构复核：
  - 抽象不过度 / 不足 → `PASS`；W3.1 只增加三个真实 read-model 语义
  - Agent/App/Delivery/Desktop 无概念泄漏 → `PASS`
  - 无兼容路径与历史债务 → `PASS`
- 残余风险：W3.1–W3.4 尚未实施，`features.subagents` 必须继续 disabled。
- 下一步：W3.1 durable descendant query。

### 2026-07-30 — W3.1

- 状态：`DONE`
- Commit：随本原子 slice 提交
- 目标：让冻结协议中已经存在的 descendant query 语义从 delivery 到 application 与
  SQLite 全程保真，并让每页 Item summaries 成为连通的 Run tree。
- 事实作者：application query 拥有 scope、cursor identity 与 page enrichment；
  SQLite projection 实现 durable lineage membership；delivery 只做 wire 翻译。
- 关键裁决：
  - application `ItemScope` 使用闭合构造，不再暴露两个可冲突 subject 字段；
  - `includeDescendants` 是 query identity，必须进入 cursor，不能作为显示选项后处理；
  - subtree membership 与 ancestor closure 只读 `runs.parent_run_id`，不从 transcript
    内容、spawning Item payload 或 live registry 猜测；
  - Store 一次递归读取 ancestor closure，不做逐 Run N+1，也不加载整个 Session。
- 生成物：public wire、Registry、schema、OpenRPC、Go/TS 类型、Artifact、SQLite epoch、
  Agent API/wire 与 `features.subagents` 均未变化。
- 验证：
  - application queries + delivery server + SQLite targeted tests → `PASS`
  - 三个高风险 package `go test -race -count=10` → `PASS`
  - `MODULE=app/runtime FAST=1 scripts/check.sh build vet test lint` → `PASS`
  - `GOWORK=off go mod tidy -diff`、arch/contract drift、`git diff --check` → `PASS`
- 架构复核：
  - 抽象不过度 / 不足 → `PASS`；只增加真实 scope 与两个 projection 语义
  - Agent/App/Delivery/Desktop 无概念泄漏 → `PASS`；Agent 无变化
  - 无旧 helper、alias、双读写、fallback 或 migration → `PASS`
- 残余风险：root multi-source stream、child refusal/profile gate 与 tail-first terminal
  race 进入 W3.2；capability 继续 disabled。
- 下一步：W3.2 root stream / replay conformance。

### 2026-07-30 — W3.2

- 状态：`DONE`
- Commit：随本原子 slice 提交
- 目标：证明 root stream 是完整 Run tree 的唯一 live/replay scope，且 tail-first
  recovery 在 terminal 边界没有丢事件窗口。
- 事实作者：root Journal 继续唯一分配 stream sequence/cursor；source route 只拥有
  event envelope；durable projection 继续唯一拥有 cold snapshot。
- 关键裁决：
  - child/nested event 保留自己的 Run/Segment envelope，但 cursor 永远绑定 root
    Run/Segment；
  - child 不是订阅根，必须 `run_not_root`，不新增 child stream；
  - profile 不覆盖时拒绝整条 stream，不过滤事件；
  - tail attach 与 head capture 原子；terminal 在 attach 前由 query 看见，在 attach 后
    由 buffered stream 看见，commit-before-publish 保证两者之间无空窗。
- 生成物：只增强 conformance tests；production、public wire、Registry、schema、
  OpenRPC、Go/TS 类型、Artifact、Store epoch、Agent API/wire 与 capability 均未变化。
- 验证：
  - root/child/grandchild shared cursor scope + child subscribe refusal → `PASS`
  - profile refusal + tail-first terminal 三线性化 + commit-before-publish → `PASS`
  - application/runs、delivery/server race `-count=10` → `PASS`
  - Runtime build、vet、全量 test、lint、arch/contract drift、tidy diff、diff check
    → `PASS`
- 架构复核：
  - 抽象不过度 / 不足 → `PASS`；未新增生产抽象
  - Agent/App/Delivery/Desktop 无概念泄漏 → `PASS`
  - 无 compatibility cursor、child stream 或 replay shadow store → `PASS`
- 残余风险：file-backed complete Running-tree loss settlement、Waiting-tree preservation
  与 old-epoch replay → cold query convergence 进入 W3.3。
- 下一步：W3.3 restart / cold recovery conformance。

### 2026-07-30 — W3.3

- 状态：`DONE`
- Commit：随本原子 slice 提交
- 目标：证明完整 Run tree 在真实进程重启后的 Running loss settlement、Waiting
  preservation 与 stream/cold-query recovery 语义一致。
- 事实作者：SQLite durable Run aggregate 继续唯一拥有 cold truth；boot
  reconciliation 只演进 lifecycle，不重新解释或重建 immutable/accounting facts；
  live Journal 只拥有当前 process epoch 的有界 replay。
- 关键裁决：
  - recovery 与普通 query 共用完整 Run row decoder，不再维护可能漏字段的 recovery
    shadow model；
  - recovery 唯一放宽项是允许读取“Interrupted 但 Pending 已丢失”的损坏关系，以便
    原子收口为 `run_lost`；模型、profile、lineage 和 accounting 仍 fail closed；
  - active replacement stream 面对 old epoch 返回 `replay_unavailable`；真实 boot
    已收口 orphan 时 lifecycle preflight 返回 `run_finished`，随后 query 给出
    `run_lost`，两者都禁止把 cursor 重绑到新 authority；
  - complete Waiting tree 只有 Pending、continuations、transcript 与 process snapshot
    全部自洽且 executor validator 接受时才能跨重启保留。
- 生成物：production 仅调整 SQLite recovery/read 内部实现；public wire、Registry、
  schema、OpenRPC、Go/TS types、Artifact、Store epoch、Agent API/wire 与
  `features.subagents` 均未变化。
- 验证：
  - file-backed Running root/child/grandchild loss settlement + committed facts
    preservation → `PASS`
  - file-backed complete Waiting tree preservation → `PASS`
  - old epoch replay / recovered terminal error precedence + cold projection → `PASS`
  - SQLite、application/runs、delivery/server race `-count=10` → `PASS`
  - `MODULE=app/runtime FAST=1 scripts/check.sh build vet test lint` → `PASS`
  - `GOWORK=off go mod tidy -diff`、arch/contract drift、`git diff --check` → `PASS`
- 架构复核：
  - 抽象不过度 / 不足 → `PASS`；删除 recovery shadow model，保留一个明确 scan policy
  - Agent/App/Delivery/Desktop 无概念泄漏 → `PASS`；变更只在 App SQLite adapter
  - 无兼容 decoder、旧 epoch fallback、双读写或 migration → `PASS`
- 残余风险：B1.5 全量 hygiene、compatibility 与完整质量门禁进入 W3.4；
  capability 继续 disabled。
- 下一步：W3.4 B1.5 full closure。

### 2026-07-30 — W3.4

- 状态：`DONE`
- Commit：`b75d2a1d9`（`refactor(runtime): close B1.5 conformance`）
- 目标：完成 B1.5 的命名、错误、接口、职责、兼容债与全量质量门收口。
- 关键裁决：
  - query not-found 使用显式分支，错误链由 decoder 描述局部事实、store use case
    统一添加一次 adapter/operation context；
  - recovery policy 以 Pending set 领域语义命名，不以 SQL join 命名；
  - child Run capability 不等于 child stream capability，production 注释不得传播错误
    心智模型；
  - 现有 consumer ports 和文件边界已与真实变化轴一致，不为形式上的“更小文件”增加
    indirection。
- 生成物：production 仅做 Runtime 内部 hygiene；public wire、Registry、schema、
  OpenRPC、Go/TS types、Artifact、Store epoch、Agent API/wire 与
  `features.subagents` 均未变化。
- 验证：
  - receiver / stale terminology / TODO-FIXME-HACK / compatibility residue scan
    → `PASS`
  - application/queries、application/runs、delivery/server、SQLite race
    `-count=10` → `PASS`
  - adapter/runsegment、bootstrap race → `PASS`
  - `MODULE=app/runtime FAST=1 scripts/check.sh build vet test lint` → `PASS`
  - `MODULE=app/runtime scripts/check.sh vuln` → `PASS`
  - `GOWORK=off go mod tidy -diff`、arch/contract drift、`git diff --check` → `PASS`
- 漏洞门说明：只剩仓库已审且当前不可修复的 Ollama allowlist
  `GO-2025-3557/3558/3559/3582/3689/3695/3824/4251`，以及不可达模块提示
  `GO-2026-5750/5932`；本 slice 未新增依赖。
- 架构复核：
  - 抽象不过度 / 不足 → `PASS`
  - 高内聚 / 低耦合 / consumer-owned ports → `PASS`
  - Agent abstraction 无 App persistence、Run/Item/idempotency 泄漏 → `PASS`
  - 无兼容层、双读写、旧 decoder、fallback 或 migration → `PASS`
- 残余风险：Runtime B1.5 无；Desktop 尚未折叠/呈现完整 Run tree，进入 W4/B1.6；
  capability 继续 disabled。
- 下一步：W4 Desktop Run-tree consumer。

### 2026-07-30 — W4.0

- 状态：`DONE`
- Commit：`26e43fd0e`（`docs(desktop): freeze run tree execution plan`）
- 目标：完成 Desktop Run-tree consumer 的 read-only blast-radius audit，冻结
  projection、recovery、invalidation、cancel 与 presentation 的治本边界。
- 关键裁决：
  - RPC stream 的 tree membership、dedupe、root termination 与 reattach 已成立，W4
    不修改 Runtime wire；
  - `AgentViewState.run`、global `turnMessageId/plan/error` 是 single-run 根因，不能通过
    增加 child side map 后继续双写；
  - target 是 Session narrative + normalized `runsById` + source-owned
    message/tool/plan/timeline/interrupt；
  - live fold 消费完整 RunEvent provenance；history、Run snapshot 与 local optimistic
    mutation 使用各自入口，不再 synthetic wire；
  - cold projection off-store 构建并 atomic replace；runtime event 只触发 authoritative
    refresh；cancel 合并 committed `CancelRunResponse`。
- 专项文档：
  - [`codex_desktop_run_tree_execution_plan.md`](codex_desktop_run_tree_execution_plan.md)
- 生成物：无；本 slice 只更新文档，未修改 wire、Registry、schema、OpenRPC、Go/TS
  generated types、Artifact、Store epoch、Agent API 或 capability。
- 验证：
  - `cd app/desktop/frontend && npm run check` → `PASS`
  - 178 test files / 1078 tests；
  - type/lint/format/knip/circular/context/published-boundary/layer/token/chrome/locale/
    bootstrap/bundle gates → `PASS`
  - `git diff --check` → `PASS`
- 已知非阻塞告警：无效 `shadow-[var(--shadow-*)]` 生成 rule、Lightning CSS
  `::highlight(...)` 识别、bundle large-chunk 提示；继续由 W7/UI hygiene 收口。
- 架构复核：
  - 不复制整份 view 给每个 Run，不保存双 lineage index；
  - components/pages 继续不直连 RPC；
  - Runtime/Agent ownership 无变化；
  - 无兼容 alias、dual write 或 fallback 方案。
- 残余风险：W4.1–W4.4 尚未实施；`features.subagents` 继续 disabled。
- 下一步：W4.1 projection core 与 source-owned fold。

### 2026-07-30 — W4.1

- 状态：`DONE`
- 目标：以 breaking change 建立 Desktop canonical Session/Run-tree projection 与
  source-owned fold，删除 single-run 心智模型和 synthetic wire；
- 实施：
  - `AgentSessionView` 成为唯一 projection contract；
  - Run lifecycle 规范化为 `runsById`，plan 与 assistant turn 按 RunID 分桶；
  - Message、ToolCall、timeline 与 interrupt 保留准确 Run owner；
  - live fold 只消费完整 `RunEvent`，history/snapshot/local mutation 使用独立入口；
  - store、ports、selectors、SDK 与跨 context consumers 同步 breaking migration；
- 证明：
  - root、siblings、nested child 独立 lifecycle/progress/terminal；
  - interleaved message/plan/tool/turn/timeline 不串 Run；
  - duplicate terminal replay 幂等；
  - live terminal 与 durable RunRef snapshot 收敛；
  - malformed source owner fail closed；
- 删除：
  - single `view.run`、global plan/turn/error；
  - optional fold source、implicit root fallback；
  - synthetic StreamEvent、空 RunID、unknown profile 与 zero metrics reconstruction；
  - 旧 type/port/selector/helper alias，无 dual write 或 compatibility reducer；
- 验证：
  - `cd app/desktop/frontend && npm run check` → `PASS`
  - 178 test files / 1076 tests；
  - type/lint/format/knip/circular/context/published-boundary/layer/token/chrome/locale/
    bootstrap/bundle gates → `PASS`；
- 残余风险：W4.2 的 atomic durable refresh、runtime invalidation 与 committed cancel
  merge 尚未实施；`features.subagents` 继续 disabled。
- 下一步：W4.2 durable recovery、invalidation 与 cancel。

### 2026-07-30 — W4.2

- 状态：`DONE`
- 实施基线：`main@40cffd81e`
- 目标：把 cold、replay-lost、runtime invalidation 与 cancel 全部收敛到同一
  durable Session projection，并禁止取消路径伪造 terminal；
- 实施：
  - `AgentRuntimeGateway.loadSessionSnapshot` 并发读取 Items、完整 RunRef page、
    PendingInterruptSet 与 optional StateSnapshot；
  - pure snapshot builder 在 Store 外构造完整 `AgentSessionView`；
  - Store 以 `refreshSequence + viewRevision` 做 latest-request-wins 与
    live/local-write CAS，history rewrite 另以 `viewEpoch` 隔离旧 stream batch；
  - driver remount 使用 `ensureSession`，不清空 materialized view；
  - cold open、rollback、replay unavailable、`runs/interrupts/state.changed` 与
    `resync` 共用 authoritative refresh；
  - root/child cancellation 只合并 exact committed `CancelRunResponse`，失败不改变
    lifecycle；
  - waiting child cancellation 若发布 root 新 Segment，取消旧 follower 并按新的
    `activeSegmentId` 订阅；
  - timeline 按 runtime timestamp 稳定排序并只保留最新 500 条，修复
    `runs.list` newest-first 对 cold timeline 的倒序与截断污染；
- 删除：
  - incrementally replay history/state/run snapshot 的 Store/port API；
  - 独立 `recoverSessionState` 路径；
  - optimistic `cancelRunningRun`、pump-local cancel identity 与 synthetic terminal；
  - `resetSession`、`reduceCompletedItem` 等失真命名；
  - capability silent fallback、alias、dual read/write 与 compatibility reducer；
- 证明：
  - Running root + Running/Waiting child、pending HITL、restart `run_lost` cold
    projection；
  - older/newer refresh、refresh/live event、failure/stale CAS；
  - capability-aware descendant/root-only query；
  - root cancel、child + surviving sibling、last waiting child → root new Segment；
  - cancel failure 保留 lifecycle，重叠 follower 不互相清除；
  - runtime invalidation 按 event `sessionIds` 精确同步 mounted Sessions；
- 验证：
  - `cd app/desktop/frontend && npm run check` → `PASS`
  - 182 test files / 1098 tests；
  - type/lint/format/knip/circular/context/published-boundary/layer/token/chrome/locale/
    bootstrap/bundle gates → `PASS`；
  - `git diff --check` → `PASS`；
- 已知非阻塞告警与 W4.0/W4.1 基线一致：无效 shadow utility、Lightning CSS
  `::highlight(...)` 识别与 large-chunk 提示；
- 残余风险：W4.3 presentation 与 W4.4 full closure 尚未实施；
  `features.subagents` 继续 disabled。
- 下一步：W4.3 Tree presentation 与交互。

### 2026-07-30 — W4.3

- 状态：`DONE`
- 实施基线：`main@cc85d3039`
- 目标：把完整 Run tree 变成 root-first、可展开、可审计、可精确操作的 Desktop
  presentation，同时保持 Agent public boundary wire-free；
- 实施：
  - root narrative 覆盖 Session 全部 root-owned turns；descendant material 只挂到
    `spawnedByItemId` 对应 parent task；
  - sibling/nested delegated narrative、Run plan、progress、metrics 与 outcome 按 source Run
    投影；
  - waiting disclosure 自动展开，其他状态默认摘要；child cancel 只调用 exact RunID
    command；
  - Context Dock Timeline 按 lineage tree 而非相邻 arrival group 组织 audit；
  - child row 可切回 Chat、原子 reveal parent tool、滚动并恢复键盘焦点；
  - shell settlement 区分 needs-input / finished / error / canceled / limit；
- 交互与视觉：
  - 对照 Synara compact subagent row，状态色收敛到 status dot，终态不制造视觉噪声；
  - 40px standalone actions、ARIA disclosure/region、keyboard focus 与 reduced-motion
    transition 全部落地；
- 治本清理：
  - 删除 `CurrentRootMessages` 与 `CurrentRootRunning` 的 single-run/boolean 失真命名；
  - selector 不保存 child/root 第二索引；component 不读取 RPC、wire 或 capability；
  - 修复 external-store selector 返回新对象导致的 snapshot 不稳定，并加引用稳定回归测试；
- 证明：
  - root/child/sibling/nested/waiting/cancel/reconnect；
  - lineage grouping 与 unknown evidence preservation；
  - exact child cancel、parent locate、same-root settlement；
- 验证：
  - `cd app/desktop/frontend && npm run check` → `PASS`
  - 187 test files / 1125 tests；
  - type/lint/format/knip/circular/context/published-boundary/layer/token/chrome/locale/
    bootstrap/bundle gates → `PASS`；
  - `git diff --check` → `PASS`；
- 已知非阻塞告警与既有基线一致：shadow utility、Lightning CSS `::highlight(...)` 与
  large-chunk 提示；
- 残余风险：W4.4 full closure 尚未实施；`features.subagents` 继续 disabled。
- 下一步：W4.4 architecture/hygiene/docs/contract closure。

### 2026-07-30 — W4.4

- 状态：`DONE`
- Commits：
  - `49b6494bd`（start/resume exact acknowledgements 与 exact ItemID reconciliation）；
  - `fcbf8f558`（scope-exact Run application surface 与 Store action semantics）；
  - `34a875d29`（命令确认边界显式提供 HITL settlement time，view mutation 保持确定性）；
- 协议收口：
  - `StartRunResponse.userItemId` 成为 required non-empty identity；
  - `ResumeRunResponse` 独立建模，`userItemId` 只在请求携带 input 时出现；
  - Contract generator 正确生成 optional non-empty text validation，Go/TS/schema/
    OpenRPC/API Reference 同源；
  - Runtime idempotency cache 只依赖 start/resume 共同的 stream address；
- Desktop 收口：
  - 删除普通 send 的内容启发式对账，只按 start ack exact ItemID relabel；
  - local optimistic identity 留在 application view，不进入 Agent domain/public language；
  - 非 terminal `item.completed` fail closed，不再补造 incomplete；
  - `activeRun.ts` 拆为 read model、commands、root attention；删除全部旧 alias；
  - public names 明确 current root / active Session / exact Run scope；
  - Session-owned stop action 返回是否接受，Store action types 由 application port 唯一定义；
  - `agentStore` callback receiver 全部统一为 `state`；
  - approval result 的本地时间事实由 resume 成功确认边界提供，projection 不再读取隐式时钟；
- 文档：
  - `frontend/ARCHITECTURE.md` 更新为 normalized Run tree、完整 provenance、atomic
    durable projection 与 consumer-owned protocol port；
  - Workspace / Plugin Context 文档从迁移愿景改为当前完成态与回归门；
  - Desktop 专项计划标记 B1.6 `DONE`，下一任务切到 W5/B1.7；
- 验证：
  - Runtime `go build ./... && go vet ./... && go test ./...` → `PASS`；
  - Desktop `npm run check` → `PASS`，188 test files / 1127 tests；
  - knip/circular/contexts/published-boundaries/layers、contract/generated drift 与
    `git diff --check` → `PASS`；
  - 434 个 package-local Go production receiver type 均保持单一 receiver 名称；
  - production TODO/FIXME/HACK、compat shim/decoder 与 dual read/write marker
    扫描无命中；
- 架构复核：
  - 一个事实一个作者，无 single-run alias、compat shim、dual write、fallback
    decoder 或 synthetic protocol fact；
  - Agent Framework API 未引入 App Run/Item、persistence、idempotency、transaction
    或 cost 概念；
  - `features.subagents` 继续诚实 disabled，留给 W5 原子启用。

### 2026-07-31 — W5.0

- 状态：`DONE`
- 类型：read-only completion audit；没有修改 production code、wire、store 或
  capability。
- 审计基线：`main@f5d285cce`
- 验证基线：
  - Runtime targeted packages：
    `go test ./internal/application/runs ./internal/adapter/runsegment ./internal/adapter/agentexec/turn ./internal/delivery/server ./internal/delivery/dispatch ./internal/infra/storage/sqlite -count=1`
    → `PASS`；
  - Agent：
    `go test ./internal/arch ./runtime -count=1` → `PASS`；
  - Desktop Agent/RPC：
    `npx vitest run src/plugins/builtin/agent src/rpc/preflight.test.ts src/rpc/sdk.test.ts src/rpc/stream.test.ts`
    → `PASS`，41 files / 253 tests。

#### W5.0 完成证据矩阵

| Gate                                        | Production owner                                                              | 现有关键行为证据                                                                                                                                                                                         | 裁决                                                                |
| ------------------------------------------- | ----------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------- |
| child 在发布/执行前原子 admission           | Agent `runtime.ChildAdmitter` + App `agentexec` adapter + Runs coordinator    | `TestChildAdmissionCompletesBeforeCreatedEventAndExecution`、`TestRejectedChildAdmissionRemovesUnpublishedProcess`、`TestCoordinatorAtomicallyAdmitsChildRunFromSpawningItem`、SQLite child opening test | `PASS`                                                              |
| child/sibling/nested lineage 与 root stream | Runs coordinator + root Journal                                               | child source projection、nested lineage、canonical postorder、drained-stream barrier tests                                                                                                               | `PASS`                                                              |
| 完整 tree HITL barrier                      | Agent snapshot + Runs transformation + runsegment transaction                 | complete sibling answer set、tree barrier、boot-resumable triplet、commit failure rollback tests                                                                                                         | `PASS`                                                              |
| Running child cancel                        | root-owned cancellation arbiter + executor subtree teardown + App transaction | exact subtree、surviving root、quiescence、teardown failure、natural terminal race tests                                                                                                                 | `PASS`                                                              |
| Waiting child cancel：仍有 boundary         | prepared Agent mutation + App write-set + runsegment transaction              | reduced Pending set、restart、rollback、root/child arbitration tests                                                                                                                                     | `PASS`                                                              |
| Waiting child cancel：最后 boundary         | 同上；一次打开 surviving suspended Runs 的新 Segment                          | final-boundary continuation、activation failure、SQLite restart tests                                                                                                                                    | `PASS`                                                              |
| parent `child_run_canceled` exactly once    | executor settlement + App terminal projection                                 | live/waiting cancel、natural terminal/approve/resume race tests                                                                                                                                          | `PASS`                                                              |
| durable child get/list/items                | Queries + SQLite keyset projections + delivery state-dependent gate           | descendant page、exact/subtree item scope、ancestor summaries、child capability refusal tests                                                                                                            | `PASS`；启用后的正向 delivery case 待 W5.1                          |
| child subscribe refusal                     | Runs subscribe root identity check                                            | `TestChildRunCannotBecomeAnIndependentSubscriptionRoot`                                                                                                                                                  | `PASS`；保持 `run_not_root`                                         |
| restart/cold recovery                       | SQLite reconciliation + process-tree snapshot + Runs rehydrate                | complete parked tree preservation、lost-tree settlement、sibling answer restart tests                                                                                                                    | `PASS`；未来 child admission policy 恢复有阻断缺口                  |
| Desktop tree fold/recovery/cancel           | Agent bounded context application view + one root stream + durable snapshot   | source-owned reducer、tree/narrative、reattach、replay、exact child cancel tests                                                                                                                         | `PASS`；first-party client 尚未 opt in                              |
| Artifact tree fidelity                      | terminal portable projection + aggregate restore                              | lineage export与 disabled-build refusal                                                                                                                                                                  | `PARTIAL`；启用后的整树 import/export 尚无最大 round-trip 证据      |
| capability/contract/generated               | Feature/Method/Shape registries                                               | dispatcher/discovery/SDK 条件 gate 等价、drift gates                                                                                                                                                     | `PARTIAL`；production composition、profile constraints 仍有阻断缺口 |
| Agent Framework 边界                        | Agent execution primitives                                                    | architecture guards + admission lifecycle tests                                                                                                                                                          | `PASS`                                                              |

#### Agent Framework 抽象泄漏复核

本轮再次从“App 是 Agent Framework 的消费者”出发审计，而不是从现有调用方便性
反推 Framework API：

- Framework 的 `ChildAdmitter` 只暴露通用、只读的 `core.ProcessView`，并在
  `ProcessCreated` 与首次执行前同步调用；
- Framework 自己拥有未发布 child 的拒绝/ panic 清理、process tree、budget tree、
  checkpoint 与执行生命周期；
- App adapter 把 `ProcessView` 收窄为 `ChildProcess`，App coordinator 才把 process
  identity 投影成 Run/Item lineage，并在自己的 transaction 中完成 admission；
- Agent production packages 未出现 App 的 Session/Run/Item、SQLite、repository、
  transaction、idempotency、artifact、retention 或 protocol DTO；
- `SpawnCallID` 是 Framework 的 execution causal identity，只能经 agentexec 形成一次性
  live routing source；App 不持久化它，也不把它暴露到 hook/wire，因此该 transient seam
  本身不是泄漏。

裁决：当前 child admission seam 是必要且足够窄的 Framework 能力，不迁入 App
事务，也不需要再抽象一层。W5.1 不修改 Agent public API。

#### W5.1 的四项阻断缺口

1. **composition 仍 disabled**
   - `server.go` 仍固定广告 `features.subagents.enabled=false`；
   - 这是当前诚实状态，不是待单独翻转的开关。
2. **协商结果未驱动生产 child admission**
   - delivery 已把 opt-in 协商为 frozen profile；
   - `runs.Coordinator.Start` 只把 Interrupt kinds 送进 `StartTurn`，没有把 child
     policy 送给 executor；
   - 因而只翻 server boolean 会“广告支持但永远不创建 first-class child Run”。
3. **cold rehydrate 从当前拓扑猜未来能力**
   - `prepareTurn` 使用 `len(pending.Continuations) > 1` 决定是否重装 child
     admission；
   - 一个允许 subagents、但在首次 park 前尚未生成 child 的 root，重启后会失去
     未来创建 child 的能力；
   - capability 必须来自 root 的不可变 admission policy，绝不能从当前树形反推。
4. **Artifact / contract 尚未达到启用态**
   - Artifact 只证明 tree lineage export 和 disabled composition refusal，尚未证明
     enabled composition 下 root profile + child lineage 的最大 round-trip；
   - import 尚未在写入前拒绝 root profile 中 composition 不支持/不认识的 required
     feature；
   - `RunProtocolProfile.requiredFeatures/interruptTypes` 的
     `uniqueItems` 与合法 required-feature vocabulary 尚未进入 Registry 生成链；
   - Desktop 固定 request capabilities 尚未声明 `subagents:true`，因此
     capability-aware descendant query 仍只会走 root-only 分支。

#### W5.1 架构裁决与最小 breaking blast radius

采用一个原子切片，不采用“加一个 `Contains(\"subagents\")` 再翻开关”的补丁：

1. **内层 profile 改为 typed semantic policy**
   - 保留 wire 的 `RunProtocolProfile.requiredFeatures`，因为这是冻结协议；
   - App inner model 不再保存 opaque wire key，改为显式 `ChildRuns bool` +
     `InterruptKinds`；
   - delivery 是 `subagents` wire key 与 `ChildRuns` 语义之间唯一 translator；
   - 不引入 feature interface、generic policy registry、Manager 或第二个 bool。
2. **policy 成为唯一生产来源**
   - start 从 frozen root policy 设置 `StartTurn.ChildRunAdmissionEnabled`；
   - live continuation沿用已安装 admission；
   - cold rehydrate 只从 `pending.ProtocolProfile.ChildRuns` 恢复，不读取
     continuation 数量、tool name、transcript 或 live registry。
3. **fresh-store cutover**
   - SQLite profile JSON 改存语义字段 `childRuns`，并把 `schemaEpoch` 从 43 提升；
   - 删除 `requiredFeatures` 的 inner/store 表达，不双读、不迁移、不 fallback。
4. **wire/Artifact 完整收口**
   - negotiation/presenter/profile-gap 显式双向映射；
   - import 在任何写入前拒绝未知或当前 composition 不支持的 required feature；
   - enabled build 增加 root profile + child lineage 的最大 Artifact v7 round-trip，
     删除“disabled 所以不可达”的测试豁免；
   - Artifact version 不变：live/portable wire shape 没变，变化的是实现能力与 fresh
     store epoch。
5. **server + first-party client 同时启用**
   - composition 广告 `subagents=true`；
   - Desktop 固定 request profile 同时 opt in；
   - 补 start/rehydrate propagation、positive/negative get/list/items/cancel、
     Minimal Profile 和 Desktop request-meta 回归测试。
6. **Registry 约束补齐并重新生成**
   - 两个 profile 集合都由 Registry 生成 `uniqueItems`；
   - required feature 只能是 Registry 中 `requiredByRunProtocol:true` 的合法 key；
   - Go/TS validator、Schema、OpenRPC、manifest、API Reference 与 samples 同步。

允许的 breaking scope：App inner Go type、SQLite fresh schema epoch、测试 fixture 与
first-party Desktop 默认 capability。明确不变：protocolVersion、Artifact v7、JSON-RPC
method/DTO shape、root stream 语义、child subscribe refusal、Agent public API。

被否决的替代：

- 只翻 `server.go`：会形成虚假广告；
- 保留 opaque `RequiredFeatures []string` 再由 application 判断 wire key：把 delivery
  vocabulary 泄漏进 domain/application；
- 同时保存 `RequiredFeatures` 与 `ChildRuns`：两个事实作者，必然漂移；
- 用 continuation 数量恢复：当前拓扑不是 admission policy；
- 兼容读取旧 profile JSON：项目处于 dev，直接升 epoch，不能制造永久 decoder 债务。

---

### 2026-07-31 — W5.1 / W5.2

- 状态：`DONE`
- 事实作者调整：
  - application/domain 的 frozen profile 只保存 `ChildRuns` 与
    `InterruptKinds` 语义，不再保存 opaque wire feature key；
  - delivery 是 `features.subagents` 与 `ChildRuns` 的唯一 translator；
  - start 与 cold rehydrate 都只从 root frozen profile 安装 child admission，不再从
    当前 continuation 拓扑反推未来能力；
  - fresh SQLite encoding 改为 `childRuns`，`schemaEpoch` 直接提升到 44，没有旧格式
    decoder、dual read/write 或迁移分支。
- 协议与 Artifact：
  - server 稳定广告 `features.subagents.enabled=true`，Desktop first-party request
    同时显式 opt in；
  - wire 新增 closed `RunProtocolFeature` vocabulary；profile 两个集合均由 Registry
    生成 `uniqueItems` 与 closed-enum item validator；
  - generator 已从只校验直接 enum 字段扩展为同时校验 enum slice，避免 Go validator
    比 Schema/TypeScript 更宽；
  - Artifact v7 最大 round-trip 覆盖 root profile、child lineage 与完整 tree；Minimal
    root 携带 child 或未知 required feature 均在写入前拒绝；
  - `protocolVersion`、Artifact v7、JSON-RPC method/DTO shape、root stream 与 child
    `run_not_root` 语义保持不变。
- 行为证据：
  - start 与 single-continuation cold rehydrate 都证明 child admission 来自 frozen
    policy；
  - child get/list/items 同时覆盖未协商拒绝与已协商正向路径；
  - capability Registry coverage test 保证每个 `requiredByRunProtocol` feature 都有
    application semantic mapping；
  - Desktop request metadata test 证明 first-party client 明确请求 subagents。
- W5.2 门禁：
  - Runtime `go test ./... -count=1`、`go build ./...`、`go vet ./...`；
  - Agent `go test ./... -count=1`；
  - Runtime 高风险包与 Agent runtime/architecture `go test -race`；
  - Desktop `npm run check`：189 files / 1128 tests；
  - contract generation 连续两次结果一致；
  - production dependency/leakage、topology inference、compatibility residue 与
    `git diff --check` 扫描无命中。
- 已知非阻塞输出仍仅为既有 shadow wildcard utility、Lightning CSS
  `::highlight(...)` 与 bundle large-chunk 提示。

---

### 2026-07-31 — W6.0 / W6.1 / W6.2 / W6.3

- 状态：`DONE`
- 实施提交：
  - `eae67ca4e refactor(desktop): isolate runtime connection boundary`
  - `47970e41d refactor(desktop): clarify frontend vocabulary`
- W6.0 架构裁决：
  - Agent Framework production graph 不依赖 App，公开抽象只表达通用 execution、
    process tree、checkpoint、prepared mutation 与 opaque cost projection；
  - App 的 Run/Item、持久化、事务、幂等、Artifact、计费和协议 DTO 没有反向泄漏；
  - Runtime package graph、consumer ports 与完整 Run use case 保持内聚，没有为了目录
    对称或方法数机械拆接口；
  - 可证实的根因集中在 Desktop Runtime context 的外环依赖和全局语义命名，而不是
    Agent/Runtime 主模型。
- W6.1 anti-corruption boundary：
  - selectable Runtime endpoint 与固定 local shell URL 分成两个事实作者；
  - application 只消费 endpoint configuration 与 discovery ports，不再读取
    `main/config`、全局 SDK store、i18n、`RpcClient` 或 `DiscoverResponse`；
  - adapter 才执行 typed `client.runtime.discover()` 并剥离 transport envelope；
  - endpoint 返回 closed `applied | rejected` 结果，稳定 reason 与本地化文案分离；
  - 删除 `endpointMirror.ts`、`runtimeRpc.ts`、旧 `runtimeConnection` 和
    `public/connection`，没有 alias、dual path 或 fallback；
  - 通用 published-boundary gate 锁定 builtin application/domain 不得反向依赖
    `main/**`，也不得持有 raw RPC/envelope。
- W6.2 语义命名：
  - `utils/helpers/shared/data/info` 等泛化文件或内部类型按真实职责改为
    `classNames`、`declaredContributions`、`pluginActivation`、`queries`、
    `projections`、`read models` 与明确 UI component；
  - application/public 的 `*Info` / `*Data` 改为 `Summary`、`ReadModel`、
    `Configuration`、`Catalog`、`Entry` 等真实语义；
  - `cn` 作为 shadcn generator 契约保留，避免以“命名统一”为由破坏工具人体工程学；
  - architecture gate 新增 generic production filename 与 application/domain
    generic suffix 防回归规则。
- W6.3 完成证据：
  - Agent `build/vet/test/lint` 与全包 `go test -race ./...` → `PASS`；
  - Runtime `build/vet/test/lint`、高风险 application/agentexec/runsegment/SQLite
    `-race` → `PASS`；
  - Agent / Runtime `go mod tidy -diff` → `PASS`；
  - Runtime `govulncheck` allowlist gate → `PASS`；只剩已审阅的 Ollama 上游不可修复
    finding 与不可达的提示级 module finding；
  - `go generate ./...` 后 contract、Go validator 与 Desktop RPC 生成物无 diff；
  - Desktop `npm run check` → `PASS`，189 test files / 1131 tests；typecheck、lint、
    format、knip、circular、contexts、published-boundaries、layers、tokens、chrome、
    locales、bootstrap 与 bundle gates 全绿；
  - production `TODO/FIXME/HACK`、旧 Runtime 入口、generic filename、Agent/App
    leakage、兼容 alias/dual path 与生成漂移扫描无命中；
  - 既有 shadow wildcard、Lightning CSS `::highlight(...)` 和 large-chunk 输出不属于
    API/架构失败，明确进入 W7.1 逐项根治，不以 suppress 伪装完成。
- 最终裁决：
  - vNext API、Run tree、capability negotiation、Artifact、Runtime producer/recovery
    与 Desktop consumer 已完成生产闭环；
  - 当前仓库不存在要求再次重做协议主模型或增加兼容层的证据；
  - 后续功能演进继续遵守 Contract Registry 单一作者、breaking-first 与 consumer-owned
    port，视觉工作不得反向污染领域和协议边界。

### 2026-07-31 — W7.0

- 状态：`DONE`
- 目标：用当前 Synara 与 Lynx 的源码、真实 DOM/computed style 和固定 viewport
  实拍，冻结视觉目标、产品语义映射、状态 fixture、验收矩阵与后续原子执行边界。
- 专项文档：
  - [`codex_synara_visual_baseline_and_execution_plan.md`](codex_synara_visual_baseline_and_execution_plan.md)
- 关键裁决：
  - Synara 只作为视觉/交互品质参考，不复制其领域模型、store、协议或 Electron 边界；
  - shell 目标固定为 256px sidebar、46px header、736px reading column、14.4px
    rounded seam、19.2px composer；
  - 默认 typography 固定为 9/10/11/12/13px、code 11px；默认 row 28px；
  - composer 使用 real border 画 edge、单层 shadow 画 depth；删除当前 shadow ring 与
    全局 shadow system 的相反解释；
  - Work Index / Agent Narrative / Context Dock 继续消费 normalized Run tree 和
    plugin application ports，不反向修改 Agent/Runtime；
  - W7.1 先建立 deterministic visual fixture，再修改 production visual foundation。
- 当前差异：
  - Lynx 的 sidebar/header/content max、gutter、composer radius 和 drawer motion
    已与参考同轴；
  - square seam、14px default base、30px row、8/10px composer footer、composer
    shadow ring 和无稳定 fixture 已明确为待删除坏味道，不是有意分歧。
- 生成物：
  - 只新增审计文档与当前截图证据，并更新总账；
  - production UI、Agent/Runtime wire、Registry、schema、OpenRPC、Go/TS types、
    Artifact、Store epoch 与 capability 均未变化。
- 验证：
  - Synara 隔离实例和 Lynx standalone frontend 在非默认端口启动；
  - 1440×900 light/dark 实拍与 Synara shell computed geometry 记录完成；
  - Markdown format、link/image、Git scope 与 diff hygiene 随本 slice 验证。
- 残余风险：
  - 截图中的动态 notification 只作当前证据，不能作为 golden；
  - standalone Lynx 在无 Runtime 时不能形成完整 Agent panel，W7.1 必须先建立
    deterministic test-only visual fixture。
- 下一步：W7.1 visual foundation。

### 2026-07-31 — W7.1

- 状态：`DONE`
- 实施提交：`051ea578d refactor(desktop): establish visual foundation`
- 目标：建立不污染 production composition 的确定性视觉证据，并以 breaking change
  删除 typography、density、edge、depth、motion 与构建 warning 的双重事实。
- 关键实现：
  - 新增 test-only Vite / Playwright harness；fixture 只冻结环境输入，不新增
    production debug route、business branch 或平行 view model；
  - canonical `AgentSessionSnapshot` 经 production projection 与 selectors 驱动
    empty、idle、Running、Waiting、terminal、error、delegated、long-content；
  - foundation、Agent、dock、settings 共 12 张 light/dark golden；结构测试覆盖
    8 个 Agent 权威状态；
  - 默认 typography 收敛为 9/10/11/12/13px、code 11px，comfortable row 28px，
    composer footer 6/8px；
  - 14.4px seam 由 content edge 与 drawer backing 共同形成；composer 使用 real
    border + one depth shadow；删除旧 ring token 和相反文档；
  - motion 使用 semantic token，scroll lock 读取 computed transition；
    `transition-all` 和 literal duration 进入静态禁止门；
  - 修复 Tailwind custom text size/class merge 冲突、invalid shadow wildcard、
    CSS Custom Highlight 构建告警与 catch-all vendor chunk；重型 lazy capability
    由显式 gzip budget 守护。
- 验证：
  - `npm run visual:test` → 20/20；
  - `npm run check` → 191 files / 1135 tests，typecheck、lint、format、knip 与全部
    architecture/design/bundle guards 通过；
  - production build 与 visual build 无 product warning；
  - Desktop `go build ./...`、`go vet ./...`、`go test ./...`、`wails build` 通过；
  - 12 张 golden 已人工复核，`git diff --check` 通过。
- 边界裁决：
  - 本 slice 落地 seam/composer primitive 是 foundation 单一 edge/depth 机制的一部分，
    不代表 W7.2 shell/Work Index 或 W7.3 Narrative/HITL 页面级完成；
  - Agent / Runtime wire、capability、Run tree projection、workspace plugin registry
    与 persistence/recovery 语义均未改变。
- 下一步：W7.2 shell / Work Index。

### 2026-07-31 — W7.2

- 状态：`DONE`
- 实施提交：`8bb0fa7ba refactor(desktop): align shell and work index`
- 目标：在不复制 Synara 业务状态、不污染 production composition 的前提下，对齐
  desktop shell 与 Work Index，并把 collapse、resize、窄窗和 accessibility 从
  截图相似提升为可验证行为。
- 关键实现：
  - expanded drawer 接管左上窗口 chrome；展开与折叠始终只有一个可见 toggle，
    ownership handoff 后 keyboard focus 跟随新控件；
  - seam rail 使用具名 vertical separator、完整 ARIA range 与
    pointer/Arrow/Home/End 输入；pointermove 只写 CSS custom property，release
    单次提交；
  - sidebar preference 是唯一持久化用户意图，当前窗口宽度只派生 effective clamp；
    窄窗不覆盖偏好，窗口恢复时 layout 与 ARIA 一致恢复；
  - pointer drag 改为起始宽度加位移，删除把绝对 client coordinate 冒充宽度的
    隐式耦合；`maxSidebarWidth` 明确表达 shell 几何，不再以 `Infinity` 套 clamp；
  - collapsed drawer transition 完成后离开 accessibility tree；Settings
    single-surface composition 继续不渲染 drawer/seam；
  - Work Index 对齐 section/group/row density、folder vocabulary、empty/loading/error
    与 live attention；idle relative time 保留于完整 accessible label，不挤占标题；
  - test-only shell fixture 安装 production query/projection/sidebar plugins 与
    workspace navigation port，仅以 data adapter 注入四种 query 状态；没有
    production route、fixture business branch 或第二套 read model；
  - 修复 lazy locale cold-start 将 requested locale 误判为 English fallback 的根因，
    selection identity 与 resource resolution fallback 分离。
- 视觉证据：
  - 8 张 shell golden：1440×900 light/dark populated、1280×800 light loading /
    dark error、1120×720 light/dark collapsed、1440×900 DPR2 light/dark；
  - 15 个 shell 用例覆盖 populated/empty/loading/error、single visible recovery
    control、focus handoff、pointer/keyboard resize、single commit 与窗口往返 clamp；
  - full visual suite `35/35`。
- 验证：
  - `npm run check` → 193 files / 1138 tests；typecheck、lint、format、knip、
    architecture/design/locales/bootstrap/bundle guards 全通过；
  - 892 个 key 在 8 个 locale 中完整；
  - production build 与 visual build 无 product warning；
  - Desktop `go build ./...`、`go vet ./...`、`go test ./...`、`wails build` 通过；
  - `git diff --check` 通过。
- 边界裁决：
  - Work Index 仍只消费 application/public selector 与 plugin contribution；
  - Agent / Runtime protocol、normalized Run projection、persistence、idempotency、
    atomicity 与 recovery 语义均未改变；
  - 本 slice 不宣告 Narrative、Run tree、HITL、Dock 或 Settings 页面级完成。
- 下一步：W7.3 Agent Narrative / Composer / Run tree / HITL。

### 2026-07-31 — W7.3

- 状态：`DONE`
- 实施提交：`9c87d93fc feat(desktop): refine agent narrative and composer`
- 目标：让 Agent Narrative、Composer、Run tree 与 HITL 形成一套紧凑、可读且
  精确服从权威 Run/Item identity 的生产视觉语言，不复制 Synara 的领域模型或状态管理。
- 关键实现：
  - 删除 fixture-only 手搓 Agent 页面；12 个 canonical
    `AgentSessionSnapshot` 经 production projection、event reducer、selectors、
    `ChatPanel`、`ChatStream` 与 `Composer` 驱动真实页面；
  - 新增 `AgentActivityDisclosure` 作为纯 presentation primitive，统一 tool、
    tool group、reasoning、plan 与 delegated Run 的 summary、status、action、
    disclosure 和 detail rail；领域状态与 command 继续留在各 feature；
  - transcript 收敛为单一 reading column 与紧凑 narrative rhythm；empty hero、
    error/recovery banner、HITL 密度及 completed/canceled/limit terminal marker
    使用安静结构，不用大面积状态色或持续终态动画；
  - terminal marker 只消费公开的窄 `useCurrentRootOutcome` read port；新增回归测试
    证明 unrelated projection write 不触发 outcome subscriber render；
  - Composer 保持 real border + single depth shadow、6/8 footer、736px reading
    measure；attach/model/send/stop/steer 均有准确 accessible name；
  - visual runtime spy 安装正式 Agent runtime port，steer 校验 exact
    `Run + Segment + input`；HITL approve/reject/question、exact child cancel、
    parent Item anchor 与 recovery resend 均走 production application path；
  - Shiki 与 reasoning elapsed 的 screenshot ready 条件显式化；生产 bootstrap
    使用推进时钟，截图边界才冻结，未通过容差掩盖竞态。
- 视觉与行为证据：
  - 24 张 Agent golden：12 个状态 × light/dark，覆盖 empty、idle、running、
    steer、waiting、question、terminal、canceled、error、recovery、delegated 与
    long-content；
  - 22 个 Agent interaction/structure 用例覆盖精确 identity、Composer 几何与
    focus、plan disclosure、recovery 及 horizontal overflow；
  - full visual suite `69/69`，关键明暗截图已人工复核。
- 验证：
  - `npm run check` → 194 files / 1141 tests；typecheck、lint、format、knip、
    circular/context/publication/layer、token/chrome/locales/bootstrap/bundle
    guards 全通过；
  - 896 个 key 在 8 个 locale 中完整；
  - production build 与 visual build 通过；
  - Desktop `go build ./...`、`go vet ./...`、`go test ./...`、Wails v2.12
    production build 通过；
  - `git diff --check` 通过。
- 边界裁决：
  - normalized Run projection、Agent/Runtime wire、capability、persistence、
    idempotency、atomicity 与 recovery 语义均未改变；
  - Shell 只通过 Chat/Message public rendering facade 消费 terminal marker，
    未增加 layer whitelist；
  - 本 slice 不宣告 Context Dock、Workspace Views、Settings 或最终
    responsive/accessibility/Wails closure 完成。
- 下一步：W7.4 Context Dock / Workspace Views / Settings。

### 2026-07-31 — W7.4

- 状态：`DONE`
- 实施提交：`d0380d0a2 feat(desktop): align workspace dock and settings`
- 目标：让 Context Dock、Workspace Views、Settings 与通用 overlay 形成同一套紧凑、
  可访问且可扩展的生产视觉语言，同时保持 plugin contribution、navigation identity、
  width preference 与业务状态各自只有一个权威作者。
- 关键实现：
  - Dock tab 改用 Base UI Tabs 的受控语义与 roving focus，不再用普通 button 模拟
    tab；browse、promote、hide/show 继续由 shell 容器拥有并提供准确 tooltip；
  - Dock resizer 增加 separator ARIA、Arrow/Home/End、Shift 精细步长及
    ResizeObserver range 同步；pointer hot path 只写 CSS custom property，结束时
    单次提交 preference，窗口 clamp 不回写用户意图；
  - light/review 宽度继续分别持久化；render、pointer 与 ARIA 共用
    `300px ≤ dock ≤ min(row/2, row - 420px)` 几何规则；
  - hide、promote、close、reopen 只操作既有 navigation application path，关闭
    dock 后仍保留 last view identity，不新增第二个 collapse 状态；
  - Settings host 统一拥有 page title、description、rail、content measure 与
    plugin boundary；pane 只贡献表单内容，`SettingsGroup` 统一单一 outline/edge，
    删除 Appearance 与 Shortcuts 的重复页面框架；
  - Diff 的 loading/empty/error 不再同时渲染无意义 navigator 与第二段 “no files”
    叙事；file、diff、timeline、plan 继续由正式 Workspace View plugin 提供；
  - ConfirmDialog 与 Tooltip 补齐稳定语义锚点及 tooltip role；生产 dialog、menu、
    tooltip、toast、shortcut/provider/model 路径均由浏览器交互测试覆盖。
- 视觉与行为证据：
  - 删除手搓 `VisualWorkspaceFixture` 业务页面，改为安装 production
    `ChatPanel`、Workspace View registry、SettingsPage、PluginToaster 与正式
    application ports；fixture 只注入确定性 data/provider 输入；
  - 6 个 canonical workspace state：dock light/review/empty/loading/error 与
    settings；
  - 12 张 Workspace golden：6 states × light/dark；
  - 20 个新增/重写的 interaction/structure 用例覆盖真实 view plugin、tab 键盘、
    width 独立持久化、single-commit resize、window clamp、identity 往返、Settings
    filter/menu/form validation，以及 dialog/tooltip/toast dismissal；
  - full visual suite `97/97`，关键 light/review/settings 明暗截图已人工复核。
- 验证：
  - `npm run check` → 194 files / 1143 tests；typecheck、lint、format、knip、
    circular/context/publication/layer、token/chrome/locales/bootstrap/bundle
    guards 全通过；
  - 896 个 key 在 8 个 locale 中完整；
  - production build 与 visual build 通过；
  - Desktop `go build ./...`、`go vet ./...`、`go test ./...`、Wails v2.12
    production build 通过；
  - `git diff --check` 通过。
- 边界裁决：
  - Workspace view content 仍由 plugin contribution 提供；Settings host 未读取
    feature RPC/wire，pane 未接管 page shell；
  - Agent/Runtime protocol、normalized Run projection、persistence、idempotency、
    atomicity、recovery 与 capability negotiation 均未改变；
  - 本 slice 不宣告最终 viewport/DPR、screen reader、reduced motion、IME 或
    Wails/WebView 真机 closure 完成。
- 下一步：W7.5 Responsive / Accessibility / Wails-WebView / Visual Closure。

### 2026-07-31 — W7.5

- 状态：`DONE`
- 实施提交：
  - `ed411fe93 feat(desktop): close responsive and accessibility gaps`
  - `c5590aad0 test(desktop): cover production clipboard flow`
- 目标：在不增加第二套 presentation state、不污染 feature/plugin 边界的前提下，
  关闭 responsive、accessibility、WKWebView compatibility、真实 Wails shell 与
  visual regression 的最后缺口。
- 关键实现：
  - Wails 默认窗口保持 `1440×900`，最小窗口从与 CSS 失配的 `1280×720` 收敛为
    `1120×720`；Go options constructor 和 contract test 让原生窗口与
    `--app-min-width/height` 共享一份可验证合同；
  - Composer 只在 mention listbox 实际挂载时发布 `aria-controls`；quiet ink
    light/dark 分别收敛为 `#747474` / `#7c7c7c`，最差 canvas 对比度真实超过
    4.5:1；generic theme 的 faint fallback 不再生成不可读的 38% alpha 文本；
  - pointer-coarse 下真实 button/tab/menuitem/option/form box 统一至少 `44×44px`，
    不使用会与相邻控件重叠的透明伪元素命中区；
  - drawer toggle 的 focus handoff 从 button-local RAF 猜测上移到
    `AgentAppShell` 的状态所有权边界，并等待目标实际可见；Chromium 与 WebKit
    均验证 collapse/expand 后 focus continuity；
  - visual host 使用与 production 相同的 `MotionConfig reducedMotion="user"`、
    motion authority 和 UI type ladder；视觉 diff 单像素阈值从默认 `0.2`
    收紧至 `0.05`，不以扩大 mask 或提高阈值吞掉 quiet-ink 漂移；
  - 新增 Axe、IME、clipboard、粗指针、最大字号、DPR 2 与 WebKit smoke，
    fixture 仍只注入确定性输入，没有 production debug route 或业务分支。
- 自动化证据：
  - 9 个 shell/agent/workspace 关键状态通过 WCAG 2.0/2.1/2.2 A/AA Axe 审计，
    无 disabled rule、无剩余 violation；
  - keyboard-only 覆盖 error recovery Settings、HITL Approve、Settings search；
    IME 证明 composition 中 Enter 不提交；production message context menu 到
    Clipboard API 的真实写入通过；
  - pointer-coarse 真实 `44px` 命中区、OS/application reduced motion、18px 最大
    UI 字号、长英文/CJK/code/diff、1120×720 overflow 与 DPR 2 明暗截图通过；
  - 8 张 W7.5 closure golden 经人工复核；全部 screenshot 使用 `0.05` threshold；
  - 5 个 WebKit 用例覆盖 Shell focus、Agent HITL、CJK/Shiki、review separator
    geometry 与 Settings menu focus return；WebKit 不建立第二套字体栅格 golden；
  - full visual suite `120/120`。
- 全量门禁：
  - `npm run check` → 194 files / 1144 tests；typecheck、lint、format、knip、
    circular/context/publication/layer、token/chrome/locales/bootstrap/bundle
    guards 全通过；
  - 896 个 key 在 8 个 locale 中完整；
  - production build、visual build、Desktop `go build ./...`、`go vet ./...`、
    `go test ./...` 与 Wails v2.12 darwin/arm64 production build 全通过；
  - `git diff --check`、gofmt、generic name/TODO/FIXME/HACK/debug/compatibility
    residue 扫描通过；
  - 新逻辑不进入 stream/resize pointer hot path：coarse target 是 media rule，
    focus effect 只在 sidebar state 边界运行，既有 drag 仍只在结束时提交一次。
- 真机与平台证据：
  - 最终 `lyra.app` 已实际启动；CoreGraphics 对该真实进程返回 onscreen、layer 0、
    `1440×900` 主窗口，与 Go/CSS contract 一致；
  - 当前 macOS 明确拒绝此终端的 Screen Recording 与 Accessibility automation，
    因此本文**不声称**取得 Wails 原生窗口截图或自动 VoiceOver 操作；
  - 可重复的内容像素证据由 Chromium golden 提供，WKWebView 近似引擎兼容由
    WebKit smoke 提供，原生壳层由 production binary + CoreGraphics geometry
    提供。权限型人工截图/VoiceOver 复核保留为环境检查，不转化为 product TODO，
    也不为此加入 debug hook、test backdoor 或兼容路径。
- 边界裁决：
  - Agent/Runtime API、normalized Run projection、persistence、idempotency、
    atomicity、recovery 与 capability negotiation 均未改变；
  - visual host 不承担业务 truth；App shell 只拥有窗口与 focus 语义；
  - W7.0–W7.5 的实现、测试、golden、commit、push 与文档现已一致。
- 下一步：W7 只由现有 visual、accessibility、architecture 和 build gates 防回归。

---

## 12. W7 收口裁决

W7 已完成，不再保留未执行的视觉切片。后续 UI 改动必须继续通过：

```text
production-backed fixture
Chromium pixel regression + WebKit compatibility smoke
WCAG / keyboard / IME / clipboard / coarse-pointer checks
frontend + Go + Wails full gates
```

持续禁止项：

- 不复制 Synara 的领域模型、状态管理、协议或依赖方向；
- 不以临时 CSS override、魔法数字或绝对定位追截图；
- 不在 feature callsite 重复 token、focus、shadow、border 或 motion；
- 不牺牲长内容、窄窗口、键盘、ARIA、reduced motion 与 WebView 稳定性；
- 不把已完成的 API/Runtime 架构重新打开，除非视觉审计发现可复现的真实语义缺口；
- 不保留旧视觉路径、dual component 或“改完再删”的兼容层。

---

## 13. W8 抽象边界、协议对接与坏味道最终收口

### 13.1 目标与完成定义

- 状态：`DONE`
- 实施原则：允许 breaking change；不保留原生交互旁路、旧组件、compatibility alias、
  dual path 或临时 whitelist。
- 主目标：
  1. 证明最新 Runtime API/协议已由前后端共同消费，而不是只在文档中成立；
  2. 清除 Desktop、Runtime 与 Wails shell 中可复现的命名、职责和边界坏味道；
  3. 保证抽象既不泄露，也不因“统一”而变成撤销样式、转发调用或假想扩展点；
  4. 把本次审计结论变成机械门禁，避免同类问题回归。

完成必须同时满足：

- Base UI / browser-native interaction → primitives → atoms → agent → feature/plugin
  单向组合，production code 无绕行；
- Framework、Runtime domain/application、adapter/infra、delivery/protocol、Desktop
  context/rpc/presentation 各守职责，没有外层概念反向污染内层；
- Contract Registry、生成 Desktop wire、Runtime delivery、HTTP lifecycle 与 frontend
  validator/method consumer 无漂移；
- build、vet、test、frontend full check、visual/WebKit/WCAG 全通过；
- 发现“当前实现已经正确”的区域不为制造 diff 而重构。

### 13.2 执行计划与实际进度

| Slice | 工作内容 | 状态 | 结果 |
| --- | --- | --- | --- |
| W8.0 | 全仓基线、receiver/interface/error/compatibility、UI 交互与协议漂移审计 | `DONE` | 现有 Runtime 主分层和 wire 契约无待迁移缺口；Desktop 存在 design-system 绕行 |
| W8.1 | Desktop interaction boundary 治本式收口 | `DONE` | 37 个 production 原生交互调用点全部迁移；新增 AST 边界门禁 |
| W8.2 | 抽象与命名复核 | `DONE` | 建立 `Pressable`；拆分 specialized input atoms；移除失真的 `Button size="content"` |
| W8.3 | Runtime/Frontend 协议与 Go 模块复核 | `DONE` | contract drift、冷启动 lifecycle、全包测试通过；无证据支持额外生产重构 |
| W8.4 | 视觉、文档、全门禁与提交收口 | `DONE` | 120/120 visual；架构文档与本台账同步；单一原子提交 |

### 13.3 关键裁决

#### A. “统一使用 Button”不是正确抽象

审计中首先把原生 `<button>` 迁移到 `Button`，随后发现
`size="content"` 实际同时撤销 Button 的 height、padding、border、font、line-height、
whitespace、alignment 与 SVG opacity。它名义上是尺寸，实际是一个“把 Button 还原成
原始按钮”的逃生口：

- 命名不表达行为；
- atom 的视觉策略泄露给 feature，feature 被迫知道并抵消；
- radius、leading、icon opacity 的任一默认值都可能让复合行产生像素漂移；
- Button 同时承担普通动作与任意复合 surface，职责不闭合。

终态拆成两种正交语义：

- `Button` / `IconButton` / `TextButton` 拥有普通动作的 metrics、tone 与交互视觉；
- `Pressable` 服务 row、card、swatch、image、preview、disclosure header 等
  content-owned surface，只提供 Base UI button 语义与 accessibility baseline。

`Pressable` 不是 unstyled escape hatch；普通动作不得为了省事使用它。19 个复合交互
调用点已按这个语义迁移，`Button` 不再包含反向撤销自身策略的 variant。

#### B. native boundary 按用户意图命名，不按实现技术打包

最终 vocabulary：

- `HiddenFileInput`：固定 `type=file` 与隐藏策略；
- `ColorPickerInput`：固定 `type=color` 与覆盖式点击区域；
- `ExternalLink`：固定 `_blank + noopener noreferrer`；
- `ResizeHandle`：固定 Base UI vertical separator 与 keyboard focus 语义；
- `TextArea`：经 `TextAreaPrimitive` 进入 atom，不在 feature 重复 native element。

曾出现的 `native-input.tsx` 按“实现来自浏览器”分组两个无关职责，已拆为
`hidden-file-input.tsx` 与 `color-picker-input.tsx`。文件名、导出名与单一变化原因一致。

#### C. 防腐层必须由机器守护

新增 `check:design-system`，用 TypeScript AST 检查 production source：

- `@base-ui/react` 只允许在 `ui/primitives`；
- `@/ui/primitives` 只允许 design-system rings 消费；
- `<a>/<button>/<details>/<input>/<select>/<summary>/<textarea>` 只允许 primitives；
- interactive ARIA widget role 只允许 primitives 实现。

终态只有 primitives 中 4 个必要的 browser-native tag implementation；feature/plugin、
agent 和 atoms 不再直接渲染原生交互标签，也没有文件白名单。

#### D. receiver 一致性是类型职责的一部分

全量 AST 审计未再发现同一生产类型混用 `s` / `base` 或其他 receiver 名，说明已知
`store` 告警已经被修复。为避免局部 rename 再次让一个类型读成两个抽象，新增
`TestReceiverNamesStayConsistent`：

- 按 package directory + concrete receiver type 聚合全部 production method；
- 支持 pointer 与 generic receiver；
- 同一类型出现两个 receiver 名时报告准确文件与方法；
- 错误信息直接要求选择能表达该类型单一职责的名字。

#### E. 没有证据时不制造后端抽象

Runtime 复核覆盖依赖方向、framework/domain/application/delivery 泄漏、consumer-owned
ports、较宽 interface、错误 wrapping、compatibility residue 与 receiver consistency。
现有较宽接口均为有文档的 lifecycle/control composition boundary，未发现 feature
依赖 concrete store、application import wire/SDK、domain import framework/I/O 或
delivery 直连 infra 的新证据。

因此 W8 不为“看起来更 Clean”拆包、缩接口或增加 facade。保留已证明内聚的边界，本身
就是避免过度抽象；生产 Runtime 无无效 churn。

#### F. 视觉基线只固定语义终态

复合 surface 迁移过程中，visual gate 捕获了 Button radius/leading/icon opacity 的
抽象泄漏并推动 `Pressable` 裁决。全套预热又暴露 Shiki 异步增高后
`use-stick-to-bottom` 可能停在距尾部 1–2px，而旧最大字号金图偶然固定在 1px 残差。

终态先断言 production follow 已进入 1px 容差，再把截图边界规范化到精确最大
`scrollTop`；light/dark 两张 agent long-content golden 仅随这个有意的 1px
terminal alignment 更新。没有扩大 `0.05` 阈值、mask 内容或接受不稳定截图。

### 13.4 前后端协议闭环结论

W8 没有发现需要新增兼容层或再次改 wire 的缺口。当前“已对接”由以下同一条证据链成立：

```text
Contract Registry
  → generated Runtime schema / OpenRPC / docs / Desktop wire
  → Runtime dispatcher + HTTP transport
  → Desktop rpc validators / typed methods
  → bounded-context consumer ports
  → production projection / UI
```

- `TestGeneratedContractHasNoDrift` 重新生成并比较 contract artifacts，零漂移；
- `TestProtocolLifecycleSurvivesColdRestart` 通过真实 HTTP lifecycle 验证 discover、
  create/start/query/stream/restart/rehydrate 语义；
- Runtime 全包测试覆盖 delivery protocol/dispatch、HTTP/inprocess transport、
  application、persistence/recovery、idempotency 与 architecture fitness；
- Frontend 1148 个测试包含 RPC schema、generated wire validation、method metadata、
  HTTP/memory transport、stream、runtime protocol、agent gateway/fold/projection 与
  feature application port；
- `check:published-boundaries` 证明发布给业务的 facade 不泄露 wire，
  `check:layers` / context DAG 证明 feature 不反向直连 transport。

结论：最新 API 迁移已经完成；W8 的工作是补齐设计系统和持续防回归能力，而不是保留旧
API 或再造一层“新旧桥接”。

### 13.5 最终验收证据

- Frontend `npm run check`
  - `196` 个 test files、`1148` 个 tests 全通过；
  - typecheck、OxLint `--deny-warnings`、Prettier、Knip、circular/context DAG、
    published/layer/design-system/token/chrome/locales/bootstrap/bundle 全通过；
  - `896` 个 locale keys 在 `8` 个语言中完整；
  - production build 与 entry/lazy bundle budgets 通过。
- Visual `npm run visual:test`
  - `120/120`；
  - Chromium golden、DPR2、WebKit、WCAG、keyboard、IME、clipboard、coarse pointer、
    reduced motion、最大字号与窄窗均通过；
  - 更新后的 exact-tail light golden 已人工复核。
- Runtime
  - `go build ./...`、`go vet ./...`、`go test ./...`、`go test -race ./...`
    全通过；
  - receiver consistency、architecture fitness、contract drift、cold-restart protocol
    lifecycle 专项通过。
- Desktop shell
  - `go build ./...`、`go vet ./...`、`go test ./...`、`go test -race ./...`
    全通过；
  - Wails v2 边界保持 thin shell，业务仍经 external Runtime HTTP JSON-RPC；
  - `wails build` 的 darwin/arm64 production compile、package 与 self-sign 全通过，
    frontend bundle 与 Go embed/窗口契约保持一致。
- Repository
  - `gofmt`、Prettier、`git diff --check` 通过；
  - 不含 compatibility branch、旧交互实现、native bypass 或临时 exception。

### 13.6 W8 收口后的持续规则

后续需求必须遵守：

1. 先判断事实作者和消费边界，再决定是否需要 abstraction；
2. 一个抽象如果要求调用者撤销它的大部分默认策略，应拆成正交语义，而不是继续加 variant；
3. 文件和类型按用户/领域意图命名，不按“native/common/base/manager”等实现来源打包；
4. 新 Base UI/native interaction 必须先进入 primitives，再由具有产品语义的 atom 发布；
5. 新 Runtime 能力从 Contract Registry 到 consumer port 一次贯通，不允许 hand-written
   second wire、fallback decoder 或旧字段 alias；
6. 审计没有发现真实问题时保持实现不动；“没有 diff”可以是避免过度抽象的正确结果；
7. 任何例外必须先形成可验证的新不变量，不能靠注释或 whitelist 长期存在。

---

## 14. P27 Agent/App opaque checkpoint 与恢复所有权最终收口

### 14.1 目标与进度

- 状态：`DONE`
- 基线提交：`0674ac1ef`
- 目标：清除 P26 后仍残留的控制权和数据模型泄露，使 Framework 私有 process tree、App
  checkpoint aggregate、Application recovery policy 与 SQLite mechanics 各有唯一 owner。
- 兼容政策：dev 阶段一次 breaking cutover；不保留旧类型、表、列、callback、codec 或 alias。

| Slice | 状态 | 结果 |
|---|---|---|
| P27.1 root-owned opaque aggregate | `DONE` | App 只持有 `ExecutorCheckpoint`；完整 Framework tree 只存在于 opaque payload |
| P27.2 transaction ownership | `DONE` | 删除 executable checkpoint write capability；runsegment 显式保存/删除 concrete value |
| P27.3 Application boot recovery | `DONE` | `runs.Recovery` 生成 `RecoveryCommit`；runrecovery adapter 原子应用；SQLite 不做政策裁决 |
| P27.4 semantic boundary | `DONE` | `executionctx` 中立化；CheckpointRoot 字段与职责型文件名完成 breaking rename |
| P27.5 prevention/doc | `DONE` | AST fitness rules、recovery invariant fixtures、现行架构与专项审计同步 |
| P27.6 full gates | `DONE` | Agent/Runtime build、vet、test、lint、tidy-diff、residue scan、diff hygiene |

### 14.2 最终所有权

```text
Agent Framework
  process topology + snapshot validity + live mutation
        |
adapter/agentexec
  the only Framework codec; pure capture/restore
        |
Application
  Run/Pending policy + atomic write-set + recovery decision
        |
runsegment / runrecovery
  persistence choreography for an already-decided App plan
        |
SQLite epoch 47
  opaque executor_checkpoints rows + factual projections
```

关键 breaking change：

- 删除 App `ProcessState`/`ProcessTreeState`/`ProcessCheckpoint`；
- 删除 `ProcessCheckpointWrite`/`PersistCheckpoint`；
- `executor_checkpoints` 每个 Framework root 只有一个 opaque aggregate；
- `CanResumeCheckpoint` 是 recovery 对 execution adapter 的唯一判断 seam；
- recovery topology/transcript/Pending policy 从 SQLite/Bootstrap 移入 `application/runs`；
- `adapter/runrecovery` 只实现事实读取和一次 `RecoveryCommit` transaction；
- `ObsoleteCheckpointRootID`、`CheckpointRootID(s)` 不再暗示 App 理解 process tree storage；
- `turnctx` 移到中立 `adapter/executionctx`；
- schema epoch 从 46 直接切换为 47，旧数据库 fail fast。

### 14.3 防复发权威

专项责任矩阵、禁止 seam、transaction timeline、architecture guards 和人工 checklist 以
[`codex_agent_app_abstraction_boundary_audit.md`](codex_agent_app_abstraction_boundary_audit.md)
为准；Runtime 现行结构以
[`../runtime/doc/ARCHITECTURE.md`](../runtime/doc/ARCHITECTURE.md)
为准。P25/P26/P27 术语只作为历史过程存在，不能据此恢复旧 API 或 Store shape。

---

## 15. P28 cross-aggregate binding 与恢复上下文零死角收口

### 15.1 目标与结果

- 状态：`DONE`
- 基线提交：`0674ac1ef`
- 目标：证明 opaque checkpoint 不只是自身合法，还与拥有它的 Run/Pending/Session 完全一致；
  清除 restart、waiting-subtree、Session edit 和 Goal continuation 的剩余组合漏洞。
- 兼容政策：直接切换 epoch 48；不提供 epoch 47 migration、reader、fallback 或 owner 修复分支。

| Slice | 状态 | 结果 |
|---|---|---|
| P28.1 binding contract | `DONE` | `ValidateOwnership`/`ValidateFor` 覆盖 root、Session、cwd、isolation、provider、goal lease |
| P28.2 recovery context | `DONE` | Application recovery 加载 canonical Session；isolated checkpoint 重启后 fail-closed 为 `run_lost` |
| P28.3 Session policy freeze | `DONE` | `ClaimIdleSession` 阻止 parked Run 期间修改 cwd/isolation；lifecycle mutation 使用独立 admission |
| P28.4 Goal continuity | `DONE` | `Pending.GoalLeaseID` 贯穿 barrier、resume/rehydrate、waiting cancellation、reducer 和 SQLite |
| P28.5 owner-bound persistence | `DONE` | root owner 不可跨 Session upsert；targeted delete 必须同时匹配 Session，foreign aggregate 原样保留 |
| P28.6 prevention | `DONE` | outer-ring vocabulary、checkpoint binding、owner-scoped deletion和旧命名均有自动守卫/反向扫描 |
| P28.7 full gates | `DONE` | Agent/Runtime build、vet、test、lint、tidy-diff、residue scan、diff hygiene；不执行 fuzz |

### 15.2 恢复判据

一个 parked tree 只有同时满足下列条件才可保留：

1. non-terminal Run tree 拓扑完整，成员全部为 `Interrupted`；
2. root-owned Pending 覆盖每个 active Run，interrupt/transcript/tool hand-off 一致；
3. Pending、checkpoint、Run 与 Session 的 root process、Session、provider 和 goal lease 一致；
4. checkpoint cwd/isolation 与 canonical Session execution policy 一致；
5. build、opaque payload、Framework snapshot/usage 均可恢复；
6. Session 非 isolated；isolated scratch workspace 不跨进程恢复。

前四类不可能的 partial application fact 视为 corruption，启动停止且不写；checkpoint
缺失、build/Framework 不兼容或 isolated workspace 已消失视为 external-resource loss，整树
按 Application policy 原子收口为 `run_lost`。

### 15.3 新的持久化不变量

```text
root_process_id -> exactly one immutable Session owner

SaveCheckpoint:
  same root + same Session     => replace aggregate
  same root + another Session => reject, preserve original

DeleteCheckpoints:
  requires Session + root IDs
  foreign owner => reject, preserve original

DeleteUnownedCheckpoints:
  boot-only exact-retention operation over Application-proven preserved roots
```

该区分避免把普通 targeted lifecycle cleanup 与 boot 全局 retention 混成同一个授权级别。

---

## 16. P29 immutable admission authority 与整树终止最终闭环

### 16.1 目标与结果

- 状态：`DONE`
- 基线提交：`0674ac1ef`
- 目标：用 hostile counterexample 验证 P28，而不是默认“测试通过即无泄露”；清除身份覆盖、
  policy 漂移、恢复记账和在线整树终止的最后组合漏洞。
- 兼容政策：直接切换 epoch 50；不保留 epoch 49 migration、reader、alias、shim 或 fallback。

| Slice | 状态 | 结果 |
|---|---|---|
| P29.1 Pending authority | `DONE` | `Put` breaking rename 为 insert-only `Open`；duplicate barrier 拒绝；Consume/Delete 同时校验 Session + root Run |
| P29.2 checkpoint admission | `DONE` | 冻结 BuildID、TurnScope、完整 `ModelSelection`、RunLimits；cumulative Usage 只能前进 |
| P29.3 durable Goal provenance | `DONE` | root `Run.GoalLeaseID` 成为 admission fact；child 禁止携带；当轮 epoch 50 schema/check 同步 |
| P29.4 self-validating recovery | `DONE` | `RecoveryCommit.Validate` 证明 lost tree/postorder、Item owner、Goal turn、Pending deletion 与 retention identities |
| P29.5 fail-closed model routing | `DONE` | 显式 model selection 缺 resolver/client 时拒绝，不回落 default |
| P29.6 all-path Goal accounting | `DONE` | reducer、boot recovery、在线 cancel/loss 都把 exact Goal turn 与 terminal Run 同事务提交；ledger 只幂等接受 exact retry |
| P29.7 parked-tree terminal | `DONE` | `TerminalPlan.Runs` 携带 canonical child-before-parent 完整树，删除 root-only 单 Run 写集 |
| P29.8 prevention/gates | `DONE` | AST guard、真实 SQLite fixture、full non-fuzz build/vet/test/lint/tidy/residue/diff gates |
| P29.9 continuation congruence | `DONE` | Pending profile canonical 且覆盖 child/interrupt；Run 与 Continuation 的 model、lineage、creation、metrics、limits、profile、goal lease 在 tree barrier、Resume、waiting-subtree cancellation、boot recovery、online terminal 五类边界逐一同值 |
| P29.10 limits single authority | `DONE` | 删除 executor `accounting.Budget`；唯一 `RunLimits{MaxTotalTokens, MaxSteps, MaxBudgetUSD}` 贯穿 admission/checkpoint/restore/store/wire；`maxTotalTokens` 与 `params.maxTokens` 分义 |

### 16.2 终态不变量

```text
Pending.Open:
  new root + new executor root -> insert
  duplicate identity           -> conflict; preserve old barrier

Pending.Consume/Delete:
  require SessionID + RootRunID
  foreign Session -> identity conflict; preserve old barrier

ExecutorCheckpoint.Save:
  immutable = owner + build + scope + model selection + RunLimits
  cumulative usage may only advance

TerminalPlan:
  Runs = exact active parked tree in canonical postorder
  Items + Pending + checkpoint + Runs + optional root GoalTurn = one transaction

RecoveryCommit:
  validates itself before any adapter write
  each Goal-owned lost root <-> exactly one matching GoalTurn

Parked continuation:
  Pending profile = root Run admitted profile
  each Continuation = the same Run's frozen model + lineage + creation + limits
                      + cumulative metrics
  child tree / interrupt kind must be covered by the admitted profile
  contradiction -> fail before checkpoint probe or transaction
```

### 16.3 “幂等”与“忽略冲突”的边界

- exact retry：同一个 Run ID、Session、lease、outcome、cost、steps、completion time，安全返回成功；
- conflicting retry：复用 identity 但事实任一字段不同，返回 typed conflict，不能 `DO NOTHING` 伪装成功；
- replacement：只有明确 lifecycle transaction 先 Consume 旧 barrier，才能 Open 新 barrier；
- Framework 只保证自己的 process-tree concurrency；App 独占产品身份、事务、恢复与 ledger 语义。

---

## 17. P30 双拓扑、产品 hook 与 peer adapter 零残留终审

### 17.1 目标与结果

- 状态：`DONE`
- 目标：在 P29 全绿后继续做反向依赖与 hostile vocabulary 扫描，清除仍能让 App 或外部
  hook 感知 Framework topology 的最后路径，并封死 peer adapter 通过 agentexec 共享端口的捷径。
- 兼容政策：dev 阶段直接 breaking cutover；不保留旧 hook 字段、Continuation 字段、codec、
  package alias 或转发 shim。

| Slice | 状态 | 结果 |
|---|---|---|
| P30.1 durable topology ownership | `DONE` | Continuation/SQLite 删除 `ParentProcessID/SpawnCallID`；只保留 opaque ProcessID binding + App RunLineage |
| P30.2 resumed live binding | `DONE` | 恢复 child route 先按 Run tree 建立；首次 live executor event 验证 parent process 与 App parent Run 的 binding，随后 source immutable |
| P30.3 Subagent hook identity | `DONE` | hook JSON breaking 改为 `runId/parentRunId`；fresh child 只在原子 opening 后可观察，restart 显式恢复 ChildRunBinding |
| P30.4 internal child isolation | `DONE` | 没有 first-class App Run binding 的 Framework child 不再触发产品 SubagentStart/Stop |
| P30.5 peer adapter ports | `DONE` | 删除 `agentexec/toolport`；工具词汇归 `domain/tool`，InterruptFunc 归 runs consumer contract，toolset→agentexec import 清零 |
| P30.6 safety vocabulary | `DONE` | `read_tool_result` 纳入 built-in safe 单一分类表，避免只读回读工具 fall-through 为 Exec |
| P30.7 prevention | `DONE` | architecture guards 固定 Continuation shape、hook Run identity、SQLite 无 topology、toolset 无 agentexec import |
| P30.8 admission failure closure | `DONE` | restored ChildRunBinding 证明单一连通 App tree；非法 confirmation 同时失败 Coordinator/waiter，失败 opening 不携带 binding 且不遗留阻塞 |

### 17.2 终态所有权

```text
Framework checkpoint:
  owns process topology and spawn edges (opaque outside agentexec)

App durable state:
  owns RunLineage and one RunID <-> opaque ProcessID binding

Live routing:
  validates transient ProcessID/ParentID/SpawnCallID once; never persists it

Product hooks:
  expose RunID/ParentRunID only

Peer adapters:
  share inward domain vocabulary or consumer-owned application ports,
  never import another concrete adapter as a convenience namespace
```

### 17.3 回归判据

以下任一变化都直接视为架构回归：

1. Continuation/SQLite 再出现 Framework parent/spawn 字段；
2. Subagent hook 再出现 `processId/parentProcessId`；
3. 未通过 App child opening 的 Framework process 触发产品 hook；
4. `toolset` import `adapter/agentexec/*`；
5. 为旧字段或旧 package 增加 alias、fallback、dual write/read。

专项细节与自动守卫以
[`codex_agent_app_abstraction_boundary_audit.md`](codex_agent_app_abstraction_boundary_audit.md)
为准。

---

## 18. P31 显式并发、RAG 语义与 Agent authoring 人体工学收口

### 18.1 目标与结果

- 状态：`DONE`
- 目标：吸收 Embabel 的并发、RAG 与 PromptRunner 思想，但只保留能在 Lynx 所有权模型中
  严密成立的部分；不把逻辑可执行性误当并发安全，不复制 parser/provider schema，也不把
  Host 原子性重新推回 Framework。
- 兼容政策：dev 阶段直接使用正确终态；无旧 API alias、fallback decoder、双语义 filter 或
  历史 chunker shim。

| Slice | 状态 | 结果 | 提交 |
|---|---|---|---|
| P31.1 Framework/Host lifecycle | `DONE` | Agent transition 只负责 execution；App checkpoint、Pending、Run 与事务继续由 Application 独占 | `60729739f` |
| P31.2 fan-out capability isolation | `DONE` | Generator 只接收 `context.Context + typed input`；managed interaction、工具、Suspend/Terminate 必须进入独立 child Process | `14aae8eda` |
| P31.3 deterministic fan-out failure | `DONE` | 稳定 index join；多个错误按最低声明位置选择非取消 cause；panic 归因；协作式取消后等待已启动分支退出 | `1ff0c8db6` |
| P31.4 unique retrieval ranking | `DONE` | 相同非空 Document ID 只占一个名额，保留最高分；`TopK` 在截断前唯一化，组合顺序不再改变结果 | `277f36b72` |
| P31.5 Markdown semantic chunking | `DONE` | 独立可选模块保留 heading ancestry，并只在 table row、list item、code line 等语义边界切分 | `12998274d` |
| P31.6 collection membership | `DONE` | `IN` 明确为 scalar-in-literals；新增 `HAS` 表达 collection-contains-scalar，并按 provider 官方能力映射或显式拒绝 | `01177233f` |
| P31.7 managed typed prompt | `DONE` | `agent.Prompt[T]` 复用唯一 owner `chatclient.OutputFormat[T]`，保留 ToolLoop/lifecycle/event/usage，删除示例手工 JSON 截取与静默 fallback | `9a2c9222a` |
| P31.8 final audit | `DONE` | 拒绝恢复 public store/provider testkit；清理旧 `Extra`、旧文件名和结构化输出文档漂移；全量非 fuzz 门禁通过 | 随本原子 slice 提交 |

### 18.2 最终执行与数据心智模型

```text
Planner plan
  -> top-level Process executes one Action
  -> observe new Blackboard state
  -> replan next tick

Explicit parallel work
  -> ScatterGather / Consensus: no Framework Process capability in Generator branches
  -> Parallel: isolated child Processes for managed Agent work
  -> ToolLoop: tool opt-in + resource-key conflicts + bounded execution
  -> all joins/commits preserve declaration or model-call order

RAG
  -> independent Retriever combinators
  -> unique-before-TopK ranking
  -> format-aware chunker lives outside the format-neutral base module
  -> provider metadata filtering uses explicit scalar IN vs collection HAS

Typed model output
  -> chatclient.OutputFormat[T] owns contract + decoder
  -> agent.Prompt[T] adapts it to the managed Process interaction
  -> providers and Agent do not grow parallel converter hierarchies
```

### 18.3 刻意不增加的能力

1. 不实现 Embabel `ConcurrentAgentProcess`，Runtime 不扫描“当前可执行”Action 并猜测并发安全；
2. 不增加推测性的 `Plan Stage`、`ParallelSafe`、Blackboard patch/commit、事务、幂等或补偿系统；
3. 不给 raw generator 暴露 `ProcessContext`，也不靠运行时拒绝模拟窄 capability；
4. 不把 RAG 焊进 Agent，不增加固定 Pipeline/QueryRouter/DocumentJoiner 大抽象；
5. 不用 `IN` 同时表达 scalar 与 collection 两种含混语义，不伪造 provider 不支持的映射；
6. 不增加 `PromptJSON`、第二套 schema generator 或 provider-native structured-output 公共协议；
7. 不恢复 `storetest/providertest`：Host-owned 存储合同由真实消费方测试，Extension shape 在真实
   dispatch 边界 fail closed。

### 18.4 防回归判据

以下任一变化都必须重新立项并给出真实消费者与反例：

- top-level Process 同一 tick 并发提交多个 planner Action；
- 并发分支共享可写 Blackboard、生命周期控制或隐式 application context；
- 结果/错误/ToolResult 顺序由 goroutine 完成时序决定；
- RAG `TopK` 先截断后去重，或 format-aware 解析器反向进入基础 document/core；
- collection filter 在不支持的 provider 上退化成错误但“看似成功”的 scalar 比较；
- Agent 自己生成结构化输出 schema、绕过 managed interaction，或重新实现 decoder；
- 为测试方便发布没有生产消费者的公共接口/package。
