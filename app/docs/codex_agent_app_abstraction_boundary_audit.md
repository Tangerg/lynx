# Agent Framework / App Runtime 抽象边界终稿与防复发台账

> 作者：Codex
> 状态：`DONE`
> 建档日期：2026-08-01
> 基线提交：`0674ac1ef`
> 兼容策略：允许 breaking change；不保留 alias、shim、双读写、旧 schema 或迁移分支
> 验证策略：普通 build/vet/test/static analysis；按维护者要求不执行 fuzz

## 1. Goal、范围与完成定义

本 Goal 要治本式清除 Agent Framework 与 `app/runtime` 的双向抽象泄露。App 是 Framework
的消费者：Framework 提供通用执行原语与核心运行时，App 选择、编排、持久化并投影这些能力，
生长为具体产品。

本轮范围包括：

- Agent process-tree checkpoint 的类型、codec、持久化与生命周期所有权；
- waiting、terminal、waiting-subtree cancellation 的原子写集；
- boot recovery 的事实读取、恢复政策和原子提交；
- peer adapter 之间的 execution context 归属；
- 文件、接口、字段、方法、错误和文档中的历史命名；
- 防止同类泄露再次进入代码的 architecture fitness rules。

完成必须同时满足：

1. Framework 不认识 Session、Run、Item、Pending、BuildID、Store、事务或恢复政策；
2. App 内环不认识 Framework concrete snapshot、节点拓扑或 continuation wire；
3. `agentexec` 是 Framework 私有类型与 App 值契约之间唯一 codec/translator；
4. checkpoint、Pending 与 Run 状态的相关变化由 App 一次原子提交；
5. SQLite 只保存/读取 opaque aggregate，不执行恢复裁决或回调 Agent；
6. 旧 API、旧表、旧列、旧文件名和兼容路径全部删除；
7. 结构守卫、真实 SQLite 集成测试和全模块门禁共同提供证据。

## 2. 第一性边界

```text
Agent Framework
  owns: definition / process tree / snapshot validity / continuation / live mutation
                              |
                              v
adapter/agentexec (anti-corruption boundary)
  owns: Framework codec + Framework <-> App projection
                              |
                              v
App Domain + Application
  owns: Run / Pending / host metadata / policy / atomic write-set / recovery
                              |
                              v
App driven adapters + SQLite
  owns: transaction mechanics / opaque bytes / queries / retention effects
```

防腐层可以同时看到两侧类型，但只能做单向翻译。Framework concrete type 到
`adapter/agentexec` 为止；SQLite concrete transaction 到 driven adapter 为止。

### 2.1 唯一 owner

| 关注点 | 唯一 owner | 明确禁止 |
|---|---|---|
| Process tree、节点关系、snapshot validity | Agent Framework | App Domain/Application/SQLite 重建或查询树拓扑 |
| Snapshot JSON codec | `adapter/agentexec` | Application 或 SQLite 编解码 Framework payload |
| Root-owned durable continuation envelope | App `execution.ExecutorCheckpoint` | envelope 暴露 child 节点、parent、started-at 或 Framework type |
| Waiting/terminal 的 durability | App Application + driven transaction adapter | Framework/agentexec 自行 Save/Delete checkpoint |
| Session、Run、Item、Pending | App | Framework 出现产品 identity 或状态机 |
| Build compatibility、workspace scope、model-selection policy、产品 usage | App | 写入 Agent snapshot 或 deployment digest |
| Boot recovery policy | `application/runs.Recovery` | Bootstrap、SQLite 或 storage callback 决定保留/丢失 |
| Recovery transaction mechanics | `adapter/runrecovery` | Application import SQLite；adapter 重新解释 policy |
| Live process-tree mutation | Agent Framework | App 在事务内持有 Framework lock/claim/live callback |
| JSON-RPC/SSE/HTTP | Delivery | 进入 Agent、Domain、Application 或 SQLite |

### 2.2 下沉 Framework 的四个必要条件

一个能力只有同时满足以下条件才可进入 Framework：

1. 对任意合理 Framework consumer 都成立；
2. 不拥有 Host identity、持久化、幂等、事务或产品政策；
3. 缺失它会破坏 Framework 自身正确性，而不只是降低当前 App 功能；
4. 能完全用 execution vocabulary 解释，不借用 Run/Item/Pending/DB/UoW。

只被当前 App 使用不自动说明它属于 App；反过来，数据源自 Framework 也不说明 Framework
应该持久化它。判断依据始终是语义所有权和变化原因。

## 3. 最终 checkpoint 合同

App 只持有一个 root-owned、opaque、不可执行的值：

```go
type ExecutorCheckpoint struct {
    RootProcessID  string
    Payload        []byte
    BuildID        string
    Scope          TurnScope
    ModelSelection modelref.Selection
    Limits         execution.RunLimits
    Usage          accounting.Snapshot
}
```

其中：

- `Payload` 是完整 Framework process tree 的 opaque serialization；
- `RootProcessID` 只是 aggregate identity，不授权 App 理解内部 topology；
- `BuildID`、`TurnScope`、完整 provider+model selection、limits/usage 是 Host
  恢复政策所需的 App metadata；
- value 可 clone、validate、传入 App transaction，但没有 `Persist`、`Commit`、`Abort`
  或 Store method；
- 只有 `agentexec` 可以把 `Payload` 解码成 `core.ProcessSnapshotTree`。

在当轮 SQLite epoch 50 中引入的 executor continuation 只使用一张 root aggregate 表：

```text
executor_checkpoints
  root_process_id  PRIMARY KEY
  session_id       App cleanup metadata
  build_id
  payload           BLOB, opaque
  policy            App-owned JSON
  usage             App-owned JSON
  committed_at
```

不存在 per-node row、`parent_process_id`、`started_at`、App `ProcessState`、App
`ProcessTreeState` 或 `process_states` 旧表。`session_id` 只用于 Session aggregate cleanup，
不能据此把 child process 派生为隐藏 Session；同一 `root_process_id` 的 Session owner
不可被改写，定向删除也必须同时匹配 Session。当前 shape 同时在 App-owned `runs` admission
和 `interrupts` hand-off 中显式保存 `goal_lease_id`，使正常终止、在线 checkpoint loss 与
boot recovery 都能向同一个 Goal incarnation 原子记账；`runs.max_total_tokens` 与 checkpoint
policy 的 `limits.max_total_tokens` 把完整 Run tree 的 token ceiling 保存为同一 App fact。

`interrupts.Continuation` 只保留 `RunID <-> ProcessID` 的 opaque binding；它不保存
`ParentProcessID`、`SpawnCallID` 或任何 executor tree edge。父子关系只有两个合法作者：

- App 的 durable `RunLineage{ParentRunID, RootRunID, SpawnedByItemID}`；
- Framework opaque checkpoint 内部的 process topology。

恢复时 Application 先按 Run lineage 重建 Run routes；child 的 live executor event 首次到达时，
再把它的 `ParentID/SpawnCallID` 与 App route 做一次绑定校验。后续 event 若改变该 live source
立即失败。App 因而能路由实时事件，却不会持久化或重建第三棵 executor tree。

Subagent lifecycle hooks 同样只暴露 `runId/parentRunId`。新 child 必须先完成 App 的原子 Run
opening，admission confirmation 才返回 `ChildRunBinding`；重启时由 durable Continuation 提供
既有绑定。没有成为 first-class App Run 的 Framework-internal child 不触发产品 hook。

### 3.1 为什么必须是 concrete data，而不是 write capability

旧设计让 captured checkpoint 携带可执行 `PersistCheckpoint`/`ProcessCheckpointWrite`。
即使 bytes 保持 opaque，这仍把 App transaction 的持久化能力穿过 Application 塞回 executor
adapter，形成三个问题：

- 数据与副作用混合，Application 无法从类型直接读出完整 write-set；
- adapter 同时拥有 capture 和 durability，事实作者不唯一；
- closure 很容易捕获 Store、transaction 或 live Framework state，扩大生命周期耦合。

终态只传 immutable value。Application 明确决定何时保存，`runsegment` 通过自己定义的窄
`ExecutorCheckpointStore` 在同一 transaction 中执行。opaque 不等于必须使用 capability；
opaque data 才是这里更小、更诚实的抽象。

## 4. 三条原子生命周期

### 4.1 Waiting tree barrier

```text
Framework reaches Waiting
  -> agentexec snapshots one immutable process tree        (pure capture)
  -> agentexec encodes one ExecutorCheckpoint              (no Store I/O)
  -> Application reduces all active Runs
  -> one runsegment transaction:
       SaveCheckpoint(checkpoint)
       Open(root-owned Pending)       // insert-only, never overwrite
       Suspend(all active Runs in deterministic postorder)
  -> publish Run events
```

事务失败时三者一起回滚。不存在 checkpoint 已可恢复但 Run 仍 Running，或 Run 已
Interrupted 但没有 continuation 的半状态。

### 4.2 Root terminal

```text
Framework root reaches terminal
  -> agentexec removes only the live registry tree
  -> Application EventCommit carries ObsoleteCheckpointRootID
  -> one runsegment transaction:
       append terminal projections
       terminalize root Run
       DeleteCheckpoints(root ID)
  -> publish terminal Run event
```

child terminal 不删除 root-owned aggregate。`ObsoleteCheckpointRootID` 描述 App 的
retention 事实，不暴露 process-tree storage shape。

### 4.3 Waiting-subtree cancellation

Framework 负责计算和应用 execution-only mutation；App 负责 transaction 和补偿：

```text
Framework plan -> concrete replacement checkpoint + canceled process IDs
Application transforms Run/Pending/Item facts
one transaction -> Consume old Pending + SaveCheckpoint + Open replacement Pending
                   + commit Run/Item changes
success -> apply live Framework mutation
apply failure -> Application fail-closes the Run tree as run_lost
```

Framework plan 返回前释放内部 ownership，不把 lock、claim、goroutine 或 Store capability
带进 App transaction。

### 4.4 Parked tree 的 cancel / run_lost

Pending 是整棵树的 barrier，因此在线取消或 checkpoint 丢失也必须终止整棵树，不能只改 root：

```text
Application validates Pending + canonical Session snapshot
  -> derives terminal Runs in canonical postorder (children before root)
  -> derives every Running interrupt Item -> Incomplete
  -> derives root GoalTurn when GoalLeaseID is present
  -> one TerminalPlan transaction:
       replace Items
       delete root checkpoint
       delete owner-bound Pending
       terminalize every Run child -> root
       record root GoalTurn
```

`TerminalPlan.Runs` 是显式完整树写集；单 `Run` 形状已经删除。这样在线 loss、用户取消与
boot recovery 对 tree ownership、Goal accounting 和原子性的答案一致。

## 5. Boot recovery 的正确分层

Boot recovery 是 Application use case，不是 SQLite maintenance：

```text
bootstrap
  -> constructs adapter/runrecovery.Persistence
  -> constructs application/runs.Recovery
  -> Recovery.Reconcile

Recovery reads facts
  -> groups complete Run trees
  -> validates Run/Pending/transcript coherence
  -> asks agentexec CanResumeCheckpoint(complete App expectation)
  -> derives RecoveryCommit

runrecovery adapter
  -> applies RecoveryCommit in one SQLite transaction
  -> terminalizes lost Runs and records exact Goal turns
  -> preserves exact checkpoint root set
  -> deletes every unowned checkpoint aggregate
```

`RecoveryStore` 只暴露事实查询、Session 读取和 `CommitRecovery`；`CheckpointResumability`
只暴露 `CanResumeCheckpoint(ExecutorCheckpointExpectation)`。expectation 由 Application
独立提供 root process、Session、workspace、isolation、完整 model selection 和 goal lease；SQLite 没有
Agent callback、validator、`ReconcileOrphans` 或
Run lifecycle policy。Bootstrap 只负责组装和在新 admission 前调用用例。

恢复失败分成两类：

- `false, nil`：checkpoint 缺失、损坏、build/host scope 不兼容、不可恢复，或隔离工作区
  已随进程退出而消失；Application 按
  `run_lost` 政策收口；
- non-nil error：验证过程本身失败；启动停止且不写入，避免把基础设施故障误判成产品事实。

## 6. 本轮发现、根因与最终裁决

| ID | 发现 | 根因 | 最终处理 |
|---|---|---|---|
| F1 | checkpoint 携带 `PersistCheckpoint` 行为穿过 Application | opaque representation 与 transaction capability 被错误绑定 | 删除行为 port；只传 concrete `ExecutorCheckpoint`，由 runsegment 持久化 |
| F2 | App `ProcessTreeState` 保存节点 topology/time | App 为清理方便复制 Framework 私有结构，形成第二事实作者 | 删除 App tree/node 类型；整棵树编码成一个 root-owned payload |
| F3 | SQLite recovery 接收 resumability callback 并裁决 orphan | storage 同时拥有事实、政策和外部执行依赖 | 新建 `application/runs.Recovery`；SQLite 仅投影事实，`runrecovery` 仅应用计划 |
| F4 | `turnctx` 位于 `adapter/agentexec`，却被多个 peer adapter 消费 | 中立的 Host execution scope 被伪装成 Agent adapter 私有能力 | breaking 移到 `adapter/executionctx`，API 收敛为 `WithScope`/`Scope` 等中立名称 |
| F5 | `ProcessIDs`、`ObsoleteProcessTreeRootID` 等写集字段继续暗示 topology | 实现已改变而命名没有同步，文档会重新训练出旧模型 | 改为 `CheckpointRootID(s)`、`ObsoleteCheckpointRootID`；保留合法执行路由 `ProcessID` |
| F6 | 文件名 `process_state.go`、`process_state_codec.go`、`run_recovery.go` 与职责不符 | 历史名字把 aggregate、codec 和 projection 混为一谈 | 改为 `executor_checkpoint.go`、`process_tree_codec.go`、`recovery_projection.go` |
| F7 | 旧审计把行为 capability、App topology envelope、SQLite callback 误判为 `KEEP` | 只检查类型是否 opaque，没有继续检查控制权、变化原因和生命周期 | 反转裁决并建立 AST architecture guards；本文删除旧结论，不保留双重权威 |
| F8 | checkpoint 与 Pending 各自合法，却可能属于不同 root/Session/provider/goal lease | 只校验单值结构，没有校验跨 aggregate binding | 新增 `ValidateOwnership`/`ValidateFor`；tree barrier、waiting cancellation、恢复、restore 和 SQLite 边界全部先证明绑定再产生副作用 |
| F9 | cold recovery 只用 root ID 探测，未独立核对 Session workspace/isolation/provider/goal lease | executor payload 可在一套 host context 下恢复，hooks/工具却使用另一套 context | recovery 加载 canonical Session 并构造完整 `ExecutorCheckpointExpectation`；isolated continuation 重启后直接 `run_lost` |
| F10 | parked Session 仍可修改 `cwd`/`isolated` | 通用 Session mutation slot 没有区分“允许消费 Pending”与“必须完全 idle” | `ClaimIdleSession` 拒绝 active/parked；execution policy edit、export/import 统一使用；生命周期写集使用独立 `ClaimSessionMutation` |
| F11 | `GoalLeaseID` 只在 checkpoint 中，Pending/rehydrate continuation 未保存 | Host continuation metadata 没有形成完整 App hand-off | `Pending.GoalLeaseID` 成为显式 durable fact，贯穿 barrier、waiting cancellation、resume/rehydrate、reducer 与当前 SQLite shape |
| F12 | checkpoint 定向删除只按 root；同 root replacement 可改写 Session owner | root identity 被当作足够的删除/替换授权 | `DeleteCheckpoints(sessionID, roots)` 做 owner-checked delete；同 root 只允许在 immutable owner/policy 下推进，跨 Session 返回 invalid checkpoint 且保留原 aggregate |
| F13 | test fake、错误名和注释仍残留 `ProcessStore`/`turnctx` 心智 | 实现切换后命名治理没有进入反向门禁 | test store 收敛为 checkpoint reader/writer 语义，错误改为 `ErrExecutorCheckpointLost`，清空旧词并加入反向扫描 |
| F14 | Pending `Put` 使用 UPSERT，可静默改写 Session/barrier | 方法名和 SQL 都暗示“覆盖即可”，破坏 barrier identity | API 改为 insert-only `Open`；duplicate root/executor identity 返回 conflict，旧 Pending 原样保留 |
| F15 | Pending `Consume/Delete` 只携带 root Run ID | root identity 被误当成 mutation authority | 所有 destructive mutation 必须同时携带 Session owner；foreign owner fail-closed |
| F16 | checkpoint 只冻结 provider，且同 root 可改写 build/policy 或回退 usage | admission policy 与 progress fact 没有被当作 immutable/cumulative | 冻结完整 `ModelSelection`、build、scope、limits；`Usage.ValidateAdvanceFrom` 禁止模型消失或计数回退 |
| F17 | Goal lease 没写入 root Run admission；boot `run_lost` 不扣 turn | 只把 lease 当 continuation metadata，忽略它也是 terminal accounting provenance | epoch 49 把 `Run.GoalLeaseID` 设为 root-only immutable admission fact；Recovery 原子提交 exact `GoalTurn` |
| F18 | `RecoveryCommit` 是可任意拼装的公开写集 | policy 与 adapter 之间缺少 self-validating contract | `RecoveryCommit.Validate` 校验 tree/postorder、Item owner、Goal turn 一一对应、Pending owner/order 与 retention identity |
| F19 | 显式 model selection 在 resolver 缺失时回落 default | “configured” 被错误降级为 best effort | Start/Rehydrate 对显式选择 fail-closed；resolver 或 resolved client 缺失都返回明确错误 |
| F20 | 在线 `ApplyRunLost/Cancel` 绕过 reducer，Goal-owned Run 不记账；Goal ledger 对冲突 retry 静默成功 | 终止路径新增后没有继承 root accounting contract，幂等被误写成无条件忽略 duplicate | `TerminalPlan.GoalTurn` 与 Run 精确绑定并同事务记录；ledger 只接受 exact retry，冲突 fact 返回 `ErrTurnIdentityConflict` |
| F21 | parked tree 在线终止只携带一个 root Run | 把 root-owned barrier 误缩成 root-only state transition | `TerminalPlan.Runs` 改为 canonical postorder 整树；children/root、Items、Pending、checkpoint、Goal turn 一次提交 |
| F22 | Run、Pending 与 Continuation 各自合法，却可在 metrics/limits/profile 上互相矛盾 | 把 continuation 当成可独立演进的恢复 DTO，而不是 Run admission 的冻结 hand-off | `RunMetrics.Equal` 与 `RunProtocolProfile.Equal/Validate` 进入领域；tree barrier、Resume、waiting-subtree cancellation、boot recovery、在线 cancel/loss 五类边界都逐 Run 核对 model/lineage/time/metrics/limits/profile/goal lease；未协商的 child/interrupt 直接拒绝 |
| F23 | executor 曾独立保存 `accounting.Budget`，而 Run/Pending 只保存部分 `RunLimits`；顶层 `maxTokens` 又与 `params.maxTokens` 同名 | 同一 admission policy 有两个类型作者，累计 Run ceiling 与单次 generation ceiling 命名冲突 | 删除 `accounting.Budget`；唯一 App fact 为 `RunLimits{MaxTotalTokens, MaxSteps, MaxBudgetUSD}`，贯穿 Run、Pending、checkpoint、restore、SQLite、Artifact 与 wire；wire 使用 `maxTotalTokens`，只给单次生成保留 `params.maxTokens` |
| F24 | Continuation 同时保存 App `RunLineage` 与 Framework `ParentProcessID/SpawnCallID` | 为恢复路由复制 executor topology，制造可矛盾的双树事实 | 删除 durable parent/spawn 字段与 SQLite codec；恢复只预装 opaque ProcessID，首次 live event 按 App parent Run 绑定并冻结 executor source |
| F25 | Subagent hook JSON 暴露 `processId/parentProcessId` | 产品 lifecycle 直接复用 Framework event identity，绕过 first-class child Run admission | hook breaking 改为 `runId/parentRunId`；opening confirmation 返回 App binding，rehydrate 显式恢复 binding；Framework-internal child 不触发产品 hook |
| F26 | `toolset` 反向 import `agentexec/toolport` 与 `agentexec/suspension` | peer adapter 的共享词汇和消费端口被错误放进具体执行适配器 | 工具组/名称归入 `domain/tool`，HITL 函数归入 `application/runs` 消费方契约，删除 `toolport`；`read_tool_result` 同时纳入 safe 单一分类表 |
| F27 | child opening 返回非法 binding 时可能只有 executor 见错，或 `error + binding` 直接返回令 waiter 阻塞 | confirmation 把业务 outcome 与握手 contract failure 混成一个返回通道，却没有规定双边失败语义 | `complete` 始终交付一次 authoritative result；contract failure 同时返回 Coordinator 并送达 waiter，Coordinator fail-close；error outcome 强制空 binding |

## 7. 合法 seam 与禁止 seam

### 7.1 保留

| seam | 理由 |
|---|---|
| `ExecutorSource.ProcessID/ParentID/SpawnCallID` | Application 必须把 Framework event 投影为 first-class child Run；它只在运行期完成 routing 与 live source binding，不进入 Continuation/SQLite/hook |
| waiting-subtree pure Plan/Apply | Framework 必须保护自己的 process-tree invariant；plan 不持资源，App 独占 durability |
| `ChildAdmitter` | 通用 child publication 前协调能力，只见 execution view/error，不见 Session/Run/Store |
| `ResumeAsync(admissionCtx, runCtx, ...)` | admission cancellation 与新 Segment lifetime 是两个不同生命周期 |
| App `TurnScope`/BuildID/model selection/limits/usage | 都是 Host 恢复、workspace、产品政策和兼容判断所需 metadata |
| `agentexec` import Agent SDK | 它就是明确的防腐层；禁止 concrete type 穿过它，而不是制造转发包 |

### 7.2 禁止

- Application/Domain/SQLite 出现 `core.ProcessSnapshotTree`、节点列表、parent/process
  topology columns 或 Framework checkpoint wire；
- checkpoint value 持有 `Persist`、`Save`、`Commit`、`Abort`、Repository 或 transaction；
- `agentexec` production 调用 `SaveCheckpoint`/任何 durable delete；
- SQLite/Bootstrap 接收 Agent validator callback 或决定 Run 应保留、终止、补偿；
- 从 child process 自动派生 Session、workspace、Run 或其他产品 identity；
- Pending/Continuation/SQLite 持久化 `ParentProcessID`、`SpawnCallID` 或 executor tree edge；
- user hook、delivery 或其他产品 API 暴露 Framework `processId/parentProcessId`；
- peer adapter 通过 `agentexec` 子包共享 vocabulary 或 consumer port；
- 通过 alias、deprecated field、fallback decoder、双表或 migration 保留 epoch 49 及更早形状。

## 8. 防复发机制

`internal/arch/framework_boundary_test.go` 已机器化以下规则：

1. `ExecutorCheckpoint` 字段必须保持 root identity + opaque payload + Host metadata；
2. SQLite checkpoint store 不得出现 Agent import、Session domain、节点 topology 列或
   Framework snapshot type；
3. `agentexec` production 不得执行 checkpoint durable write/delete；
4. waiting barrier transaction 必须同时调用 `SaveCheckpoint`、Pending 写入和 Run commit；
5. terminal transaction 必须拥有 checkpoint 删除；
6. waiting-subtree transaction 必须保存 replacement checkpoint；
7. SQLite recovery projection 只能读取 non-terminal Run facts；
8. Application 必须拥有 `Recovery`/`RecoveryCommit` 与 checkpoint resumability seam；
9. Bootstrap 只能构造并驱动 Application recovery，不能恢复旧 callback seam；
10. 中立 execution context 不得重新嵌套到 `agentexec`。
11. Domain production source 不得在 import 或注释中反向命名 outer rings；
12. checkpoint/Pending 在所有写入、恢复和 waiting-subtree 边界都必须核对 root、Session、
    完整 model selection、goal lease，并在 restore 核对 cwd/isolation；
13. targeted checkpoint delete 必须携带 Session owner，root owner 不得跨 Session upsert 改写。
14. Pending 必须 insert-only `Open`，Consume/Delete 必须携带 Session owner；
15. parked-tree terminal write-set 必须携带 canonical postorder 的完整 Run tree；
16. 每个 Goal-owned terminal root 必须在同一 transaction 携带且只携带一个 exact Goal turn；
17. checkpoint policy 不可改写，cumulative usage 不可回退。
18. Pending profile 必须是 canonical admission contract；子树和每个 interrupt kind 必须已协商，
    且 tree barrier、Resume、waiting-subtree cancellation、boot recovery、online terminal 五类路径
    都必须证明 Continuation 与 Run 的冻结事实完全一致。
19. Run policy 只能由 `execution.RunLimits` 表达；checkpoint、Run、Pending、restore、SQLite 和 wire
    不得再定义第二个 budget carrier，累计 token ceiling 必须命名为 `maxTotalTokens`。
20. Continuation 字段只能包含 App frozen facts 与一个 opaque ProcessID；SQLite interrupt storage
    不得出现 Framework parent/spawn topology。
21. Subagent hook domain/wire 只能使用 `RunID/ParentRunID`；不得重新暴露 process identity。
22. `toolset` 不得 import `agentexec`；旧 `agentexec/toolport` package 必须不存在。

`internal/arch/invariant_coverage_test.go` 还把以下不变量双向绑定到真实集成 fixture：

- `terminal_run_explains_how_it_ended` × boot recovery；
- `parked_tree_has_exactly_one_open_interrupt_set` × tree barrier / boot recovery；
- `parked_continuation_matches_run_facts` × tree barrier / Resume / waiting-subtree cancellation /
  boot recovery / online parked terminal；
- rollback/delete 等其他既有 transaction invariants。

每次 seam 变更还必须人工回答：

1. 这个名词能否只用 execution vocabulary 解释？
2. 谁是事实的唯一 writer？
3. 是否把 immutable data 错做成 executable capability？
4. callback 返回前是否仍持有 lock、claim、goroutine 或 live plan？
5. 失败时哪些事实必须一起回滚，write-set 是否完整可见？
6. concrete Framework type 是否止于 `agentexec`？
7. cleanup 是 live resource cleanup 还是 durable retention？
8. 新 identity 是产品真实概念，还是从 executor topology 机械派生？
9. 名称是否描述当前语义，而不是已经删除的实现历史？
10. 新增 policy 是扩充现有唯一事实类型，还是偷偷创建了第二个 carrier？
11. breaking cutover 是否同步删除旧 API/schema/test/doc，而不是添加兼容层？

## 9. Breaking cutover

- SQLite `schemaEpoch` 直接切换为 50；旧数据库以 epoch mismatch 拒绝；
- 删除旧 `process_states` shape，唯一现行表为 `executor_checkpoints`；
- 删除 App `ProcessState`、`ProcessTreeState`、`ProcessCheckpoint`；
- 删除 `ProcessCheckpointWrite`/`PersistCheckpoint`/旧 Store 方法；
- 删除 SQLite recovery validator/callback 与 bootstrap orphan policy；
- 删除旧 `adapter/agentexec/turnctx`；
- 删除 `Continuation.ParentProcessID/SpawnCallID` 与对应 SQLite JSON 字段；
- Subagent hook JSON 从 `processId/parentProcessId` 直接切换为 `runId/parentRunId`；
- 删除 `adapter/agentexec/toolport`，不保留 alias 或转发 package；
- `runs.goal_lease_id`、`interrupts.goal_lease_id`、`runs.max_total_tokens`、owner-bound
  Pending/checkpoint store 一次切换，不保留 epoch 49 reader；
- 不提供 alias、deprecated API、双读写、fallback decoder、migration 或 shadow state。

这是 dev 阶段的唯一真相切换，不是兼容迁移。

## 10. 执行计划与进度

| 阶段 | 状态 | 产物/证据 |
|---|---|---|
| G1 基线与责任矩阵 | `DONE` | 第一性 owner、泄露判据、爆炸半径 |
| G2 Opaque root checkpoint | `DONE` | `ExecutorCheckpoint`、单 payload codec、当轮 epoch 50 schema |
| G3 原子生命周期 | `DONE` | waiting/terminal/subtree cancellation 写集由 App transaction 独占 |
| G4 Application boot recovery | `DONE` | `runs.Recovery` + `RecoveryCommit` + `adapter/runrecovery` |
| G5 命名与 peer boundary | `DONE` | `executionctx`、CheckpointRoot 命名、语义文件名 |
| G6 防复发与文档 | `DONE` | architecture guards、invariant fixture index、本文和现行架构同步 |
| G7 全量验证 | `DONE` | Agent/Runtime build、vet、test、lint、tidy-diff；static residue；`git diff --check` |
| G8 零死角复审 | `DONE` | cross-aggregate binding、Session policy freeze、Goal lease continuity、owner-bound save/delete、行为反例与反向守卫 |
| G9 反例驱动终审 | `DONE` | insert-only Pending、immutable checkpoint policy/progress、self-validating recovery、在线整树 terminal、全路径 Goal accounting |
| G10 事实同一性终审 | `DONE` | Run/Continuation metrics、limits、model、lineage、profile、goal lease 全绑定；未协商 barrier fail-closed；五类生命周期边界 fixtures + architecture guard |
| G11 Policy 单一权威终审 | `DONE` | 删除 `accounting.Budget`；`RunLimits` 贯穿 admission/park/recovery/restore/store/wire；`maxTotalTokens` 与 generation `params.maxTokens` 语义分离 |
| G12 双拓扑清零 | `DONE` | Continuation/SQLite 删除 Framework parent/spawn；恢复 route 只用 opaque binding，live source 首次绑定后不可变 |
| G13 产品 lifecycle 身份收口 | `DONE` | Subagent hooks 改用 App Run identity；fresh admission 与 restart binding 均有行为测试 |
| G14 peer adapter 反向依赖清零 | `DONE` | 删除 toolport；tool vocabulary 与 HITL consumer port 归位；architecture guard 禁止 toolset→agentexec |
| G15 admission 失败路径终审 | `DONE` | binding graph 校验、invalid-result 双边失败、error+binding 不阻塞；Coordinator/confirmation 行为 fixtures |

## 11. 验证结果与最终结论

已执行且通过：

- Agent：`go build ./...`、`go vet ./...`、`go test ./...`、`golangci-lint run ./...`；
- Runtime：`go build ./...`、`go vet ./...`、`go test ./...`、`golangci-lint run ./...`；
- Agent/Runtime：`go mod tidy -diff`；
- checkpoint、tree barrier、terminal rollback、waiting-subtree、boot recovery、Session cleanup
  的真实 SQLite integration tests；
- Agent/App architecture tests 与 transaction invariant fixture index；
- production import、旧 symbol、旧 Store owner、旧 topology shape 与兼容路径反向扫描；
- `gofmt` 与 `git diff --check`。

build/vet/test/lint 限制 `GOMAXPROCS=4`；遵照维护者要求没有执行 fuzz。

目标结构已经落地：Framework 独占 process tree 与 snapshot 解释；App 只持有 root-owned
opaque checkpoint aggregate，并独占原子性、幂等、恢复和 retention；SQLite 只保存事实并
执行 App 已决定的写集。全量门禁已经闭环，当前没有保留任何历史兼容路径，也没有发现新的
双向抽象泄露。
