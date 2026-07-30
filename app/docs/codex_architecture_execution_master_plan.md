# Lynx Agent / Runtime / Desktop 架构演进总执行台账

> 作者：Codex
> 状态：`IN PROGRESS`
> 建档日期：2026-07-30
> 最新已提交基线：`main@e7452288b`，与 `origin/main` 一致；W2.2 随本原子 slice 完成
> 当前主任务：`W2.3 — restart / query / publication / quiescence`
> 执行进度：`W2.1–W2.2 DONE · W2.3 READY`
> 当前协议：`protocol.current = protocol.minSupported = "2026-07-27"`
> 当前 Artifact：`SessionArtifactVersion = 7`
> 当前 Store：`schemaEpoch = 43`

## 0. 文档职责

本文是后续实施的**总入口和进度总账**，统一记录：

- 最终目标与不可突破的架构边界；
- Agent Framework、`app/runtime`、`app/desktop` 与 UI 的依赖关系；
- 当前真实进度、下一任务、验收门禁和提交纪律；
- 跨专项计划的关键路径、风险和已经接受的决策。

本文**不是第三份协议规范**，也不复制字段级 wire shape。文档权威顺序如下：

1. [`codex_runtime_protocol_vnext_final.md`](codex_runtime_protocol_vnext_final.md)
   唯一目标契约，回答“最终 API 与协议语义是什么”。
2. 本文
   总执行台账，回答“为什么做、先做什么、做到哪了、何时算完成”。
3. [`codex_runtime_protocol_conformance_plan.md`](codex_runtime_protocol_conformance_plan.md)
   Runtime 协议一致性与 child Run 专项台账。
4. [`../../doc/AGENT_FRAMEWORK_ARCHITECTURE_EXECUTION_PLAN.md`](../../doc/AGENT_FRAMEWORK_ARCHITECTURE_EXECUTION_PLAN.md)
   Agent Framework 专项台账。
5. [`../runtime/doc/EXECUTION_CENTERED_ARCHITECTURE.md`](../runtime/doc/EXECUTION_CENTERED_ARCHITECTURE.md)、
   [`../desktop/frontend/ARCHITECTURE.md`](../desktop/frontend/ARCHITECTURE.md) 与
   [`../desktop/docs/FRONTEND_PLUGIN_CONTEXTS.md`](../desktop/docs/FRONTEND_PLUGIN_CONTEXTS.md)
   各模块的现行架构基准。
6. Contract Registry、生成物、源码、测试和 Git
   证明“当前实现实际上是什么”。
7. `PROTOCOL_DESIGN.md`、`PROTOCOL_VNEXT_REVIEW.md`、
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

本轮（2026-07-30）先以当前仓库、生成物、测试和质量门禁校准本文，再完成 W2.1
确定性 ownership arbitration。后续实施必须从 §12 的唯一执行卡继续。

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

| 概念 | 唯一 owner | 允许提供 | 禁止泄露 |
|---|---|---|---|
| Agent 定义、Process tree、interaction、tool checkpoint、Suspension | Agent Framework | 通用执行 API、不可变 snapshot、prepared mutation | App Run/Item、数据库、产品错误、幂等键 |
| Waiting subtree execution replacement | Agent Runtime | `prepare / commit / abort`、精确 canceled process、surviving suspension | Store、Repository、transaction、BuildID、SQLite |
| Session / Run / Segment / Item / Interrupt / protocol profile | App domain/application | 领域值、不变量、用例编排、consumer ports | JSON-RPC DTO、Agent 内部 state JSON、SQLite 实现 |
| 幂等、原子性、CAS、BuildID、usage ledger、durable checkpoint commit | App | transaction write-set、恢复与失败策略 | Agent public API |
| Agent ↔ App 翻译 | `adapter/agentexec` | process/source/suspension 映射，App-private prepared port | delivery DTO、SQL、产品 presenter |
| SQLite 与具体事务 | `infra/storage/sqlite`、`adapter/runsegment` | application/domain port 实现 | 业务用例裁决、wire shape |
| 方法、shape、error、capability、constraints | `delivery/dispatch` Contract Registry | 生成 metadata 与 fail-closed 校验 | Run 生命周期与 durable 拼装 |
| JSON-RPC / HTTP / SSE | `delivery` | envelope、context metadata、wire ↔ application 翻译 | executor 生命周期、事务、query 语义 |
| Frontend wire | generated RPC 层 | generated type、validator、method metadata | 手写第二份协议联合 |
| Frontend 业务 | plugin bounded contexts | command、event、selector、fold、read model | transport internals、backend concrete client |
| UI 视觉 | design system + feature composition | token、primitive、atom、agent shell | 协议规则、领域状态写入 |

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

| 工作流 | 状态 | 已完成 | 下一步 |
|---|---|---|---|
| Protocol A-track | `DONE` | A1–A7；冻结契约、Registry、生成物、Runtime 与 Desktop consumer 一致 | 只随真实新能力同步 |
| Agent P24 | `IN PROGRESS` | P24-01 host-settled checkpoint；P24-02 prepared subtree mutation；P24-03 App durable transaction | P24-04 完整恢复/竞态门禁 |
| Runtime B1.1 | `DONE` | durable Run-tree identity、root admission | — |
| Runtime B1.2 | `DONE` | first-class child producer、source routing、独立 Segment/Item/metrics | — |
| Runtime B1.3 | `DONE` | tree barrier、整树 resume、restart/race/failure conformance | — |
| Runtime B1.4a–b | `DONE` | immutable cancel plan、Running child subtree cancellation | — |
| Runtime B1.4c | `DONE` | prepared runtime mutation + App-owned atomic waiting-subtree transaction | — |
| Runtime B1.4d | `IN PROGRESS` | W2.1 ownership arbitration；W2.2 stale/failure/rollback matrix | W2.3 restart/query/publication/quiescence |
| Runtime B1.5 | `TODO` | — | durable child query/subscribe/cold recovery |
| Desktop B1.6 | `TODO` | 协议 fold 与插件架构基线已存在 | Run-tree fold 与交互 |
| Runtime/Desktop B1.7 | `TODO` | capability seam 已存在且保持 disabled | 全门禁后启用 subagents |
| Runtime/Desktop 架构持续演进 | `ONGOING` | 依赖环、consumer ports、plugin contexts 与多项 architecture gate 已存在 | 随每个 slice 审查并最终 sweep |
| Synara UI 对齐 | `TODO` | 参考仓库已明确为 `~/Desktop/synara` | B1.6 后做视觉基线与像素级实现 |

不使用跨工作流“总百分比”。一个竞态闭环不能与一个命名修正等权；进度只由原子 slice
和完成证据表达。

### 4.2 当前 Git 与质量基线

已提交并推送：

- `a2a3bb2de feat(agent): settle paused tool checkpoints`
- `1d5f7eb86 feat(agent): prepare waiting subtree cancellation`
- `a4e153fd4 feat(runtime): commit waiting subtree cancellation atomically`
- `52f46fbed docs(runtime): record waiting cancellation completion`

本轮审计开始时：

```text
HEAD        = 6eb2f7f55622ab765465e11555f881631b4e9fc7
origin/main = 6eb2f7f55622ab765465e11555f881631b4e9fc7
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

计划校准轮没有把尚未运行的 `-race`、`govulncheck` 或“真实 SQLite 重启后的 B1.4d
新矩阵”伪装成已验证。W2.1 现已完成定向 race；W2.3 的真实重启和 W2.4 的全量 race
仍保持未完成。Desktop bundle gate 虽通过，但 Vite/Lightning CSS 仍报告
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

### 4.3 剩余文档工作

- `app/runtime/doc/EXECUTION_CENTERED_ARCHITECTURE.md` 已改为 root-owned direct
  suspension set，并记录 waiting child cancellation 的 prepared mutation / atomic
  transaction 边界；
- 当前未发现已知架构文档与 B1.4c 实现漂移；
- B1.4d 结束时仍需同步 query/recovery 的最终对抗性证据，这不是保留旧实现或兼容路径的
  理由。

### 4.4 当前仓库审计结论

结论不是“新 API 尚未实现”，而是：

> vNext 的协议骨架、核心读写面和绝大多数行为已经落地；剩余工作是按依赖顺序完成
> child Run 的取消一致性、完整读面、Desktop 消费和 capability 放行。

| 审计面 | 当前事实 | 判断 |
|---|---|---|
| Machine contract | 单一 Contract Registry 生成 manifest、Schema、OpenRPC、Go validator、TS types/validator/method map；arch drift gate 通过 | 目标结构已成立，不再新增平行 registry |
| 协议表面 | manifest 当前有 85 个方法，4 个 stream method；协议只服务 `2026-07-27`；Artifact 只接受 v7 | hard cutover 已成立，没有版本协商假象 |
| Run 心智模型 | `Session → Run → Segment → Item`、Run 三态、正交 outcome/metrics、typed Interrupt、durable query 均已落地 | API 主模型无需重做 |
| Agent Framework | execution tree、HITL、checkpoint、prepared waiting-subtree mutation 与 Continue 已成立 | P24 只差 consumer/recovery/race 完整门禁 |
| App Runtime 写面 | child admission、source routing、tree barrier/resume、Running/Waiting child cancel 已到 B1.4c | 事实 owner 正确；W2 不得重新分配所有权 |
| App Runtime 读面 | `runs.get/list`、`items.list`、`interrupts.list` 已读 durable projection；`RunTree` 可由任意节点取整树 | root 能力成熟；descendant paging/subscribe/cold recovery 尚未闭环 |
| Desktop transport | root stream 已追踪 descendant membership、去重、reattach 与 cold history recovery | 已具备 W4 基础，不等于完整 child view model |
| Desktop fold | child start/finish 目前主要折成共享 timeline，root readout 被保护但没有每个 child 的独立状态树 | B1.6 仍是真缺口，不能提前开启 capability |
| Capability | `features.subagents` 已发布为 stable opt-in seam，但 server 明确 `enabled=false` | 正确；必须等 W2–W5 全完成再原子启用 |
| 依赖治理 | Go architecture tests、Frontend layer/context/public-boundary/cycle gates 全绿 | 不需要全局换目录；每个 slice 内做局部治本 |
| 历史兼容 | SQLite 单 epoch 43、protocol current=min、旧 artifact/schema 直接拒绝 | 符合 dev 阶段 breaking-first |

### 4.5 B1.4d 证据缺口矩阵

以下矩阵区分“已经有局部证明”和“完成 B1.4d 仍缺的对抗性证明”。只有右列全部闭环，
W2 才能标记 `DONE`。

| 场景 | 已有证据 | W2 必补 |
|---|---|---|
| active / later sibling / nested target | Agent prepared mutation 已覆盖 active/later sibling 与 nested ancestor；App pure transform 覆盖 nested subtree postorder | W2.1 以 nested target + descendant + surviving sibling 贯穿真实入口；adapter 失败拓扑继续由 W2.2 收口 |
| root cancel vs child cancel | live handle 已证明 Running tree 只能有一个 owner | `W2.1 DONE`：Running 与 Waiting/parked 双向胜者、loser busy、零额外 durable mutation |
| resume vs child cancel | root resume vs root cancel 已共享 admission | `W2.1 DONE`：Waiting child cancel / resume 双向胜者与单一 durable opening/cancel truth |
| natural terminal vs child cancel | root cancel vs natural terminal 已覆盖 | `W2.1 DONE`：target terminal before/after child claim 均稳定返回 finished 且释放 claim |
| duplicate cancel / stale input | root 已有“turn already gone”幂等完成；parent Item CAS stale 可回滚 | duplicate root/child 已由 W2.1 固定；stale Pending CAS、错误分类与零 publication 进入 W2.2 |
| teardown / checkpoint / transaction failure | root cleanup failure、prepared Abort、SQLite 整体 rollback 已覆盖 | 显式 checkpoint write failure、Waiting teardown/Continue failure、每个失败点的 durable/live 断言 |
| exact response / invalidation | command 返回 committed target/root；成功 transaction 返回两者 | exact field equality、Runs/Interrupts/Sessions 的精确失效集合、不得 post-commit re-query |
| subtree quiescence | Agent Commit 后 child 从 registry 移除 | cancel 成功返回后 App/Delivery 不再出现 target/descendant 事件 |
| process restart | application 有 fake Rehydrate；Agent portable replacement 可 restore/Continue | file-backed SQLite close/reopen：target 不复活、survivor 可继续、query 与 checkpoint 一致 |
| canonical publication | Domain `RunTree` 与 pure transform 已固定 descendant-before-ancestor | 集成写入/发布顺序、siblings lexical、root last，并在 race 下重复 |

### 4.6 Hygiene 观察与处理策略

本轮只记录可复核事实，不因“文件大”或“名字看起来一般”机械重构：

1. **Store receiver 告警已清除**
   - 当前生产代码中的 `*Store` receiver 使用 `s`，未再发现同一 Store 类型同时使用
     `s` / `base`。
2. **仍有两处 receiver 一致性候选**
   - `application/sessions.Snapshot` 在不同文件使用 `snapshot` 与 `s`；
   - `delivery/protocol.StreamEvent` 的手写方法使用 `se`，生成 validator 使用
     `value`。
   - 处理时必须改唯一 owner：前者统一手写 receiver；后者若要求一致，应改 generator，
     不能手改生成物。
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
   - 当前不阻塞 Runtime B1.4d，但 W7 完成定义要求零未知构建告警。
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

状态：`IN PROGRESS`；当前：`W2.1–W2.2 DONE · W2.3 READY`

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

| Slice | 状态 | 目标 | 主要 owner | 退出证据 |
|---|---|---|---|---|
| W2.1 | `DONE` | 确定性 ownership arbitration | `application/runs` + Agent live arbiter | parked root/child、resume/child、terminal/child、duplicate 双向胜者 |
| W2.2 | `DONE` | stale/failure/rollback conformance | `application/runs` + `adapter/runsegment` | stale Pending、checkpoint、transaction、teardown/Continue 每个失败点 |
| W2.3 | `READY` | restart/query/publication/quiescence | SQLite + application query + delivery projection | file-backed close/reopen、exact target/root/read model、无 canceled-late-event |
| W2.4 | `TODO` | race、hygiene 与全量收口 | 跨 Agent / Runtime | 重复 race、全量 gates、命名/错误/接口/兼容扫描、文档同步 |

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

W2.3 与 W3 的边界：

- W2 证明“B1.4c 写出的事实可重启、可精确读取、不会复活 canceled subtree”；
- W3 才实现通用 descendant paging、child subscribe 与完整 Running tree cold recovery；
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

### W3 — B1.5 durable query、subscribe 与 cold recovery

状态：`READY`

交付：

- `runs.get/list` 对 root/child/descendant 的权威语义；
- descendant paging 的稳定顺序和 cursor；
- `items.list` 精确 child scope 与 `includeDescendants`；
- root stream 中多 source Run 的 replay scope；
- child subscribe 的 capability/profile 前置条件；
- process restart 后完整 tree query 与 cold recovery；
- replay 不可用时 query 能恢复到同一 durable truth；
- 不从 transcript、live registry 或 event 时序反推 tree identity。

完成定义：

- query、stream、replay、cold recovery 四个事实面一致；
- capability 未协商时显式 `capability_not_negotiated`；
- 不返回看似完整的降级结果；
- Registry、Go/TS validator、SDK、canonical docs 同步；
- Runtime 全量与高风险 race 门禁通过。

### W4 — B1.6 Desktop Run-tree consumer

状态：`TODO`

交付：

- frontend reducer 按 `source Run` 折叠 root stream；
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

### W5 — B1.7 最终 conformance 与 capability enablement

状态：`TODO`

只有以下项目全部通过后，才能将 `features.subagents.enabled` 改为 `true`：

- Running / Waiting child cancel 两条 Waiting disposition；
- root/child/resume/terminal 全竞态；
- subtree quiescence；
- parent `child_run_canceled` exactly once；
- child get/list/items/subscribe；
- restart query/recovery；
- frontend tree reducer；
- Contract Registry、schema、OpenRPC、manifest、API Reference、Go/TS validator 与 SDK 一致；
- runtime/frontend 完整门禁；
- generation 连续两次无 diff；
- 历史兼容扫描无生产命中。

Capability enablement 必须与实现、生成物、客户端、canonical docs 和测试在同一个原子
slice 中完成，不能先改布尔值。

### W6 — Runtime / Desktop 架构最终复核

状态：`TODO`，但规则持续执行

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

### W7 — Synara 视觉基线与像素级 Desktop UI 对齐

状态：`TODO`

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

| 风险 | 后果 | 控制 |
|---|---|---|
| Agent 吸收 App transaction/idempotency | Framework 被单一消费者污染 | P24 ownership gate；Agent public API/AST 审查 |
| App 解析或改写 FrameworkState | checkpoint schema 泄漏、升级脆弱 | 只调用 Agent prepared mutation |
| durable commit 与 live mutation 分裂 | restart 复活已取消 child 或 live/durable 不一致 | freeze → complete App transaction → infallible prepared commit；故障注入 |
| parent tool 双重完成 | transcript 与模型上下文冲突 | committed-tool projection + exact identity + exactly-once tests |
| 最后一个 Interrupt 被当成普通 resume | 伪造用户输入或产生第二 barrier | framework-ready continuation；无 Resume event |
| cancel / resume / terminal 多 owner | 双提交或孤儿状态 | root tree admission + parked claim + transaction CAS |
| Query 从 live state 推断 child | restart 后读面不一致 | durable tree identity 与 projection 是唯一读事实 |
| Frontend 按 root 合并所有 source | sibling/child 内容串线 | source-aware reducer 与 exhaustive tests |
| 为 Clean Architecture 机械加层 | 接口泛滥、调用链变长 | ownership-first；单实现内部直接用 concrete type |
| 为 KISS 拒绝必要抽象 | 同一规则散落多层 | 一个事实一个作者；consumer-defined port |
| 命名和错误在大改中漂移 | API 难读、诊断不可行动 | 每 slice hygiene sweep + lint/arch gate |
| UI 像素复刻污染业务架构 | 视觉目标绑死状态和协议 | 只映射视觉；先 token，再 shell，再 feature |
| 文档宣称超过实现 | capability 或行为失真 | Git/CI 决定进度；无证据不标 DONE |

---

## 10. 已接受的关键决策

| ID | 决策 | 状态 |
|---|---|---|
| M-01 | 冻结契约是目标真相，当前实现不是弱化理由 | `ACCEPTED` |
| M-02 | dev 阶段直接 breaking，不保留历史兼容 | `ACCEPTED` |
| M-03 | Agent 是 Framework；App 是消费者 | `ACCEPTED` |
| M-04 | 幂等、原子性、事务、BuildID 和产品账本归 App | `ACCEPTED` |
| M-05 | Agent 只提供 execution 原语与核心运行时逻辑 | `ACCEPTED` |
| M-06 | Waiting child cancel 使用 Agent prepared mutation + App transaction | `ACCEPTED` |
| M-07 | `runs.cancel` 同时服务 root/child，不新增 child 专用方法 | `ACCEPTED` |
| M-08 | command 返回 exact committed snapshot，不 post-commit re-query | `ACCEPTED` |
| M-09 | 完整 child 能力闭环前 `features.subagents=false` | `ACCEPTED` |
| M-10 | Clean Architecture 是所有权/依赖方向，不是层数；DDD 是语言/不变量，不是框架仪式 | `ACCEPTED` |
| M-11 | Desktop 保持 plugin kernel，业务插件演进为 bounded contexts | `ACCEPTED` |
| M-12 | UI 参考 Synara 的布局与视觉，不复制其业务架构 | `ACCEPTED` |
| M-13 | 质量、命名、错误与架构清理属于每个 slice 的完成定义 | `ACCEPTED` |

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

---

## 12. 下一张执行卡

唯一下一任务：

```text
W2.3 — P24-04 / B1.4d
Restart / query / publication / quiescence
```

开工顺序：

1. 使用 `t.TempDir()` 下的真实 SQLite 文件建立 final-boundary 与
   remaining-boundary fixtures；
2. 提交 Waiting child cancellation，关闭数据库并通过新的 store/process 重新打开；
3. 从 target child identity 查询 exact target、root 与完整 tree，证明 canceled subtree
   不复活；
4. 校验 surviving tree 的 checkpoint、usage、BuildID、Pending/Continuation 与 Segment
   identity；
5. 对 remaining-boundary 执行 boot reconcile + rehydrate/continue；final-boundary 只证明
   当前已实现的 committed truth，不提前伪造 W3 Running cold recovery；
6. 对照 command response、durable query 与 publication/invalidation 的 target/root/read
   set；
7. 在 cancel 返回后继续 drain/观察，证明 target/descendant 不再发布事件；
8. 对高风险矩阵运行 race，执行 Agent/App 全量门禁并更新三份台账；
9. 独立 commit/push 后进入 W2.4，不提前实现 B1.5 或打开 capability。

W2 收口前的禁止项：

- 不新增协议方法；
- 不让 Agent 接触 App persistence；
- 不拆出通用 transaction framework；
- 不用 sleep/概率性测试冒充竞态证明；
- 不通过 post-commit re-query 修补 command response；
- 不提交或推送工作树里未通过门禁的代码。
