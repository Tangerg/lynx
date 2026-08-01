# Agent Framework / App Runtime 抽象边界审计与防复发台账

> 作者：Codex
> 状态：`DONE`
> 建档日期：2026-08-01
> Goal：彻底清理 Agent Framework 与 `app/runtime` 之间的全部抽象泄露，并建立可执行的防复发机制
> 当前基线提交：`03773c52d`
> 兼容策略：允许 breaking change；不保留 alias、shim、双读写、旧 schema 或迁移分支
> 验证策略：普通 build/vet/test/static analysis；按维护者要求不执行 fuzz

## 1. 文档职责

本文是本轮专项 Goal 的唯一审计台账，持续记录：

- Framework 与 App 的责任宪法；
- 审计维度、证据、发现、裁决和爆炸半径；
- 每一批清理的计划、进度、门禁和提交；
- 防止同类泄露再次进入代码或权威文档的 fitness rules。

本文不复制 Agent API、App wire 或 SQLite schema 的全部字段。当前事实仍以源码、自动化测试和
Git 为最终证据；本文负责解释所有权与演进过程。完成 Goal 前，任何“零泄露”结论都必须同时有
静态依赖证据、行为测试和反向架构守卫，不能只靠关键词扫描或人工感觉。

## 2. 第一性边界

### 2.1 关系模型

```text
Agent Framework
  提供所有消费者都成立的执行原语与核心运行时
        ↓ 被消费
app/runtime
  选择、编排、持久化并投影这些能力，生长为具体应用
```

App 可以理解并调用 Framework 的公开执行合同；Framework 不得反向理解 App。防腐层可以同时
看见两侧类型，但必须做单向翻译，不能让任一侧的具体类型穿透到另一侧。

### 2.2 唯一 owner

| 关注点 | 唯一 owner | Framework 可提供 | Framework 禁止拥有 |
|---|---|---|---|
| Agent definition、planning、action、condition | Agent | 声明、校验、编译和执行 | 产品 Run、协议方法、权限产品策略 |
| Process tree、状态迁移、HITL continuation | Agent | 捕获、校验、恢复、纯计划和 live apply | Store、事务、幂等、补偿、提交协议 |
| ToolLoop checkpoint 与执行顺序 | Agent | provider-neutral checkpoint、continue/resume | App Item、审批产品文案、SQLite shape |
| 通用执行 usage | Agent | tokens/calls/actions/opaque cost 聚合 | provider/model 账本、USD 产品计费、审计时间线 |
| Session、Run、Segment、Item、Pending | App | 领域状态、用例与投影 | 要求 Agent 知道这些 identity 或状态机 |
| 原子性、幂等、事务、恢复、补偿 | App | UoW、CAS、fail-closed、发布顺序 | 要求 Agent 提供 Commit/Abort/Repository |
| Build identity、retention、schema policy | App | checkpoint metadata、epoch、清理策略 | 进入 Agent snapshot 或 deployment identity |
| Agent ↔ App 翻译 | `adapter/agentexec` | codec、identity/prompt/event projection | 类型穿透到 Domain/Application/Infra/Delivery |
| SQLite 与具体 I/O | App Infra/Adapter | opaque payload、原子写集实现 | 解析或重建 Agent continuation |
| JSON-RPC/HTTP/SSE | App Delivery | wire 映射与 transport I/O | 进入 Agent 或 App Domain |

### 2.3 “应该沉入 Framework”与“必须留在 App”的裁决

一个能力只有同时满足以下条件，才可以进入 Agent：

1. 对任意合理 Framework 消费者都成立；
2. 不拥有某个 Host 的 identity、持久化或产品策略；
3. 忽略它会破坏 Framework 自身执行正确性，而不只是降低某个 App 的功能；
4. 能用 execution vocabulary 完整解释，无需借用 Run/Item/DB/UoW 等 App 词汇。

只被当前 App 使用并不自动说明它属于 App。若实现该能力必须解释 Agent 私有 checkpoint 或维护
process-tree 不变量，它仍属于 Framework；App 只拥有何时调用、如何提交和失败后如何恢复。

## 3. 泄露分类与判定方法

| 编号 | 维度 | 泄露信号 | 必须取得的证据 |
|---|---|---|---|
| D1 | 依赖方向 | Agent import App；App 内环 import Agent SDK；绕过防腐层 | module/package DAG、production import inventory、AST guard |
| D2 | 类型边界 | Agent concrete snapshot/event/config 穿入 App Domain/Application/Infra/Delivery | exported signature inventory、字段类型与 call-site trace |
| D3 | 词汇与错误 | 内层 API/error/comment 使用外层产品或存储语义 | public API、sentinel、error chain、GoDoc 语义审查 |
| D4 | 状态所有权 | 同一事实两边各存一份或由非 owner patch/derive | writer inventory、state transition trace、restore path |
| D5 | 控制流 | Framework 参与 Host UoW、发布顺序、retry、补偿或产品裁决 | prepare/apply/cancel/resume 调用图、failure matrix |
| D6 | 生命周期与并发 | Framework lock/claim 跨 Host I/O；App 绕过 Framework mutation owner | lock/claim lifetime、ctx/cancel、goroutine owner、race tests |
| D7 | 持久化与协议 | Store 解析 Framework payload；Framework 定 schema/retention/BuildID | codec owner、SQL columns、schema policy、restart tests |
| D8 | 抽象不足/过度 | 胖接口、单实现仪式接口、错误命名、职责混装 | consumer-method matrix、实现数、变化理由与 package cohesion |
| D9 | 文档治理 | 旧文档继续宣称已删除的 API/schema 是当前事实 | authority graph、stale-symbol scan、文档更新门禁 |

关键词扫描只用于发现候选，不能单独判定泄露。`Snapshot`、`Checkpoint`、`Plan`、`Apply` 等词在
execution framework 中可能完全合理；反之，一个没有敏感词的 callback 也可能实际把 Host
transaction choreography 倒灌进 Framework。

## 4. Goal 执行结果

| 阶段 | 状态 | 目标 | 完成证据 |
|---|---|---|---|
| G1 基线与责任矩阵 | `DONE` | 固定判据、文档权威与现有保护 | 本文第 1–3 节 |
| G2 依赖/API/类型审计 | `DONE` | 证明 concrete type 只停留在合法边缘 | production import inventory、Agent API baseline、双侧 AST guard |
| G3 数据/错误/协议审计 | `DONE` | 清除双 owner、payload 解释和外层语义倒灌 | schema 46、opaque payload、atomic failure/restart tests |
| G4 控制流/并发/生命周期审计 | `DONE` | 清除跨层锁、事务、补偿和 lifecycle choreography | waiting/terminal/boot timeline 与 failure matrix |
| G5 Breaking 清理 | `DONE` | 按根因重构，不留兼容路径 | 全仓消费者一次切换；旧类型、列和接口删除 |
| G6 防复发与定稿 | `DONE` | architecture tests、权威文档和零泄露结论闭环 | `framework_boundary_test.go`、现行架构基准与本文 |

## 5. 依赖面与 API 面结论

### 5.1 静态依赖

- Agent production graph 不 import `app/**`；
- App production 只有 `internal/adapter/agentexec` 与实现 Agent tool capability 的
  `internal/adapter/toolset` import Agent SDK；
- App Domain、Application、Infra、Delivery 与其他 adapter 不 import Agent SDK；
- SQLite 不导入 Agent SDK，也不解释 `process_states.payload`；
- Application 只看 `ExecutorSource`、`EngineEvent`、`ProcessCheckpointWrite` 等 App-owned
  consumer contract，不接收 `core.ProcessSnapshotTree`、`runtime.Process` 或 ToolLoop checkpoint。

这不是“Framework 与 App 完全无交互”。正确结构是 concrete Framework type 只存在于
`adapter/agentexec` 防腐层，由该层向内投影稳定的 App value/capability。

### 5.2 Agent 导出面

逐项检查 Agent `core`、`runtime` 与 `toolloop` exported baseline 后确认：

- 没有 Session、Run、Segment、Item、BuildID、Repository、transaction、idempotency、SQLite、
  journal 或产品计费账本；
- snapshot 只表达 Agent definition、process identity/topology、blackboard、suspension、usage 与
  ToolLoop continuation；
- Runtime `Config` 只包含 chat、middleware、tool rounds、child depth 和 extension；
- `PendingSuspensionsIn(tree)` 是对已捕获 execution snapshot 的纯查询，不做 I/O，也不认识 App；
- Framework plan 不提供 Persist、Commit、Abort、Rollback 或发布能力。

因此本轮只增加了一个纯函数 `runtime.PendingSuspensionsIn`，用于让 adapter 从同一 immutable
tree 同时投影 checkpoint 与外部 boundary，避免再次独立读取 live tree 产生时序裂缝。

## 6. 真实泄露、根因与处理

| ID | 分类 | 发现 | 根因 | 最终处理 |
|---|---|---|---|---|
| F1 | D4/D5/D7 | 普通 waiting 由 `turnProcess.Await` 先单独 `SaveTree`，App 随后才提交 Pending/Run suspension | executor completion 偷偷拥有 Host durability；一个语义事实被拆成两个 transaction | `Await` 变为纯 join；`CaptureWaitingCheckpoint` 只捕获 immutable tree；App `CommitTreeBarrier` 在一个 transaction 中依次写 checkpoint、Pending、全部 Run suspension |
| F2 | D4/D6/D7 | terminal `TurnProcess.Discard` 在后台删除 durable snapshot，早于或独立于 Run terminal transaction | live registry cleanup 与 durable retention 混为一个方法 | `Discard` 只 `RemoveTree`；root terminal commit 携带 `ObsoleteProcessTreeRootID`，由 App transaction 同时 terminalize Run 并删除 checkpoint；child terminal 不删除 root aggregate |
| F3 | D4/D8 | ProcessStore 把 child process 派生为隐藏 `Session(kind=subtask)`，同时系统已经有 first-class child Run | executor topology 被误当成 product conversation identity，形成双模型和双清理树 | breaking 删除 `Session.Kind`、`Subtask`、`SaveSubtask`、Children cascade、`sessions.kind` 列及全部消费者；delegation 只由同一 Session 下的 child Run 表达 |
| F4 | D4/D7 | boot recovery 只删除 Pending 指向的失效 checkpoint，crash 在 checkpoint 写入后、Pending 提交前可能留下永久孤儿 | retention 从一个 projection 反推，而非计算精确 durable owner set | boot 先证明完整 Interrupted Run tree + coherent Pending + resumable state，随后只保留精确 process root set，`DeleteUnownedTrees` 清除其余 aggregate |
| F5 | D4/D7 | Session delete 只能通过 Pending 间接找到 process tree，无 Pending 的 session-owned checkpoint 会残留 | cleanup identity 缺少 App owner metadata | `process_states` root row 增加 App-owned `session_id` cleanup metadata；`DeleteSessionTrees` 在 Session transaction 内删除该 Session 的全部 checkpoint aggregate |
| F6 | D3/D8 | 终态字段名只说 `ProcessCheckpointRootID`，没有表达为什么出现；root 查询实现重复；旧注释仍描述 hidden subtask Session | 命名和实现仍暴露已删除设计的影子 | 改为事实型 `ObsoleteProcessTreeRootID`；提取私有 root-query 实现；清理旧注释与现行文档 |
| F7 | D9 | 总入口仍把 epoch 44 和 prepared transaction 描述为当前事实 | 历史台账没有随 breaking cutover 更新 current-state 区域 | 总入口改为 epoch 46/P25–P26；现行架构补充三条原子生命周期；历史记录保留且明确是历史 |

### 6.1 Waiting 原子边界

```text
Agent segment reaches Waiting
  -> adapter snapshots one immutable process tree          (no I/O)
  -> adapter derives pending suspensions from that tree    (pure query)
  -> TreeInterrupted{Checkpoint, Suspensions}
  -> App reduces every active Run
  -> one App transaction:
       SaveTree(checkpoint)
       Put(root-owned Pending)
       Suspend(all active Runs, deterministic postorder)
  -> publish Run events
```

任一步失败，checkpoint、Pending 和 Run state 一起回滚；不存在“数据库里可恢复，但产品说还在
Running”或“产品说 Interrupted，但没有 continuation”的半状态。

### 6.2 Terminal 原子边界

```text
Agent root reaches terminal
  -> adapter emits TurnEnd
  -> Framework live tree may be removed from registry      (memory only)
  -> App root terminal EventCommit marks process tree obsolete
  -> one App transaction:
       append terminal projections
       terminalize root Run
       delete obsolete checkpoint tree
  -> publish terminal RunEvent
```

transaction 失败时 Run 与 checkpoint 都保持原状。Framework cleanup 不再决定 durability；App
可以重试或在下次启动按自己的 recovery policy 收口。

### 6.3 Boot 与 Session cleanup

```text
boot:
  load non-terminal Run trees + Pending
  -> validate topology/transcript/continuation ownership
  -> ask agentexec only whether opaque process state is resumable
  -> preserve exact proven root set
  -> recover every other Run tree as run_lost in one transaction
  -> delete every process tree outside the preserved root set

session delete:
  one App transaction
  -> delete transcript/history/interrupts/process trees/runs/todos/
     approvals/goals/tool results/session
  -> post-commit live cleanup and filesystem checkpoint cleanup
```

## 7. KEEP 裁决：必要 seam 不是泄露

| 候选 | 裁决 | 理由 |
|---|---|---|
| `RetiredChildUsage` | `KEEP` | child detach 后仍需保持完整 process-tree budget/usage 守恒；这是 Framework 自身 accounting invariant，不是 App 账本 |
| waiting-subtree `Plan/Apply` | `KEEP` | plan 纯计算 resulting execution state 且返回前不持 lock/claim/goroutine；apply 只维护 Framework tree invariant。App 独立拥有 transaction、Commit/Abort 和 recovery |
| `ChildAdmitter` | `KEEP` | 对任何需要在 child publication/first tick 前协调外部资源的消费者都成立；接口只见 `ProcessView` 和 `error`，不见 Run/Session/Store。Runtime 不跨 callback 持有 tree mutation lock，App 自己完成 child Run transaction |
| `ResumeAsync(admissionCtx, runCtx, ...)` | `KEEP` | admission cancellation 与新 Segment lifetime 是两个不同 execution lifecycle；双 context 防止请求取消错误终止已经获准的 continuation，与 App 类型无关 |
| `ProcessCheckpointWrite` | `KEEP` | Application 只决定 opaque write 何时加入 transaction；codec 与具体 snapshot 留在 adapter。若把 bytes 拉入 Application，反而会泄露 representation |
| App `ProcessTreeState` envelope | `KEEP` | identity/topology/time 是 Store 清理与原子替换所需的 App contract；payload 保持 opaque，SQLite 不解释 Agent wire |
| `TurnScope`/BuildID/usage metadata | `KEEP IN APP` | 它们是 Host 恢复、工作目录、产品预算和 build compatibility policy，只能由 adapter 投影并由 App Store 保存，不得进入 Agent snapshot |
| `ResumableProcessValidator` callback | `KEEP` | SQLite owns recovery transaction，agentexec owns opaque continuation validation；callback 是双方最窄的布尔能力，没有让 Store 解码 Framework state |
| `agentexec`/`toolset` import Agent | `KEEP` | 前者是防腐层，后者实现 Agent tool capability；禁止它们 import Agent 会制造无意义的转发层，真正的规则是类型不得穿入 App 内环 |

`ChildRunAdmissionEnabled` 也保持在 App `StartTurn` policy：产品决定本 Run 是否将 delegated process
投影为 first-class child Run，adapter 只据此安装通用 `ChildAdmitter`。Framework 不知道该开关。

## 8. 防复发机制

### 8.1 已机器化的 fitness rules

`internal/arch` 现在同时保护：

1. App 环依赖规则以及 Agent import allowlist；
2. Framework concrete snapshot 不得进入 App Domain/Application/Infra/Delivery；
3. ProcessStore 不得 import Session domain，Session 不得恢复 `Subtask`/`KindSubtask` 等双 identity；
4. `agentexec` 整个生产包只有 `capturedProcessTree.PersistCheckpoint` 可以调用 `SaveTree`；
5. `agentexec` 生产代码不得调用任何 process-tree durable delete；
6. `CommitTreeBarrier` 的同一 transaction 必须包含 checkpoint persistence、Pending 写入和 Run commit；
7. terminal `CommitEvent` transaction 必须拥有 process-tree deletion；
8. Agent exported API baseline 与旧 Host settlement symbol guard。

守卫扫描完整 production package，而不是只看 `Await`/`Discard` 的表面方法体，因此不能通过抽取
一个名称温和的 helper 绕过。

### 8.2 每次 Agent/App seam 变更的人工 checklist

1. 这个名词能否只用 execution vocabulary 解释？不能则先留在 App。
2. 它是否对至少两个合理 Framework consumer 都成立？只有 Lyra 产品需要时不能下沉。
3. 谁是事实的唯一 writer？是否存在 snapshot/DB/projection 各写一份的双 owner？
4. callback 返回前 Framework 是否持有 lock、claim、live plan 或 goroutine？
5. 持久化失败时，哪些事实必须一起回滚？transaction 必须由 App 明确列出完整 write-set。
6. adapter 是否是 concrete type 的最后出现位置？Domain/Application/Infra/Delivery 不得解释它。
7. cleanup 是 live resource cleanup 还是 durable retention？两者不得共用一个含糊方法。
8. 新 Session/Run/Process identity 是产品真实概念，还是从另一层 topology 机械派生？
9. 错误是否包含 operation、identity 与 `%w` cause，同时避免泄露 private snapshot/wire？
10. breaking cutover 是否删除旧字段、接口、列、测试和 current-state 文档，而非加 shim？

### 8.3 文档治理

- 现行实现边界以 `app/runtime/doc/EXECUTION_CENTERED_ARCHITECTURE.md` 为准；
- 跨专项总账以 `codex_architecture_execution_master_plan.md` 的 current-state 区域为准；
- 本文是 Agent/App abstraction seam 的专项判据与历史；
- `PROTOCOL_DESIGN.md`、旧 P13/P24 日志等可以保留历史术语，但不得被引用为当前实现；
- 每次 schema/API breaking change 必须同批扫描现行文档中的旧 epoch、旧 symbol 与旧 owner 描述。

## 9. Breaking cutover 与数据政策

- SQLite `schemaEpoch` 从 45 直接切换为 46；
- `sessions.kind` 删除，不迁移 hidden subtask Session；
- `process_states` root row 新增 `session_id`，只作为 App cleanup metadata；
- 旧数据库直接以 epoch mismatch 拒绝；
- 不保留 alias、deprecated API、双读写、fallback decoder、migration 或 shadow state；
- Agent exported API 只增加纯查询 `PendingSuspensionsIn`，其余改动均留在 App contract/implementation。

这是 dev 阶段正确的债务策略：让 schema 与代码只表达一个当前真相。

## 10. 验证结果

已执行且通过：

- `go test ./agent/runtime ./agent/internal/arch`；
- `go test ./app/runtime/...`；
- `MODULE=agent FAST=1 scripts/check.sh build vet test lint`；
- `MODULE=app/runtime FAST=1 scripts/check.sh build vet test lint`；
- Agent/App `staticcheck ./...` 与 `go mod tidy -diff`；
- waiting rollback、terminal delete rollback、session delete orphan cleanup、boot exact-retention 的真实
  SQLite failure/restart tests；
- Agent/App architecture tests；
- production import、旧 symbol、旧 Session kind、Store I/O owner 与文档 current-state 反向扫描；
- `git diff --check`。

以上门禁全部通过；执行时使用受限 `GOMAXPROCS` 降低本机负载。遵照维护者要求，本 Goal 不执行
fuzz；race 也未在未获重新授权时运行。

## 11. 执行日志

| 日期 | 进展 | 证据 | 结果 |
|---|---|---|---|
| 2026-08-01 | 建立专项 Goal 与责任矩阵 | D1–D9 分类、依赖/API inventory、旧文档漂移 | 找到审计方法与权威入口 |
| 2026-08-01 | 关闭 waiting durability split | pure capture + App tree-barrier transaction + rollback test | F1 FIX |
| 2026-08-01 | 删除 process-derived hidden Session | Session/SQLite/Application/Delivery 全仓 breaking cutover，schema 46 | F3 FIX |
| 2026-08-01 | 关闭 terminal/background deletion 与 orphan retention | App terminal write-set、session metadata cleanup、boot exact preserved set | F2/F4/F5 FIX |
| 2026-08-01 | 完成反证审计与 KEEP 裁决 | exported API、ChildAdmitter/Plan/Apply/dual-context/codec/recovery callback trace | 无新增泄露 |
| 2026-08-01 | 建立防复发守卫并同步现行文档 | package-wide AST guard、master/runtime architecture/current schema 更新 | Goal DONE |
