# Execution-Centered Architecture — `app/runtime` 架构基准

> 状态：现行架构基准。

Lyra Runtime 以 Run 生命周期为中心，而不是以某个 agent loop 类型或 transport 为中心。
Delivery 接收 wire request，Application 拥有完整用例和副作用顺序，Adapter/Infra
实现消费方端口，Bootstrap 只装配和关闭。

## 1. 依赖方向

```text
                         bootstrap + cmd
                         /      |      \
                 delivery    adapter    infra
                       \       |       /
                         application
                              |
                            domain
```

这是源码依赖图，不是运行时调用图。`internal/arch` 将以下规则编码为测试：

- Domain 不依赖 Application、Adapter、Infra、Delivery、Bootstrap；
- Application 只依赖 Domain、自己定义的消费方端口、中立的 Core chat/media 值契约和
  无领域语义的 `component` 进程内原语；
- Adapter 实现 Application/Domain 端口，可以包装 Lynx SDK 和外部能力；
- Infra 实现技术型 Domain 端口，不组织应用用例；
- Delivery 只做协议、dispatch、transport 与 application projection；
- Bootstrap/Config 可以看见全部环，但没有环反向 import 组合根。

目录名就是环名：

```text
internal/
├── domain/       execution、session、provider、tool、approval、editguard…
├── application/  runs、sessions、models、workspace、usage、integrations、queries…
├── adapter/      agentexec、modelclient、maintenance、toolset、runsegment…
├── infra/        storage、git、checkpoint、exec、lsp、mcp、a2a、llm
├── delivery/     protocol、dispatch、server、transport/{http,inprocess}
├── component/    无领域语义的进程内小组件
├── bootstrap/    Stack、Host 与 wiring
├── config/       外部配置解析
└── arch/         依赖与所有权 fitness tests
```

不为目录对称制造空 package。新增能力先判断它属于领域规则、完整用例、能力适配、技术
设施还是 wire 边界，再选择环。

## 2. Run 是 Application 的生命周期单元

`internal/application/runs.Coordinator` 拥有一个 Run 从 admission 到 terminal 的完整流程：

```text
Start/Resume request
  -> validate + durable admission
  -> create/attach executor handle
  -> reduce EngineEvent
  -> EventCommit (persist before publish)
  -> RunEvent journal
  -> Delivery protocol projection
```

关键所有权：

- `RunID` 标识用户看到的完整 run，resume 后保持不变；
- `SegmentID` 标识一次底层执行段；
- `EngineEvent` 是 Application 接收 executor 的唯一事件族；
- `EventCommit` 描述需要原子持久化的 projection；
- `RunEvent` 是 application journal 向 delivery 发布的唯一事件族；
- terminal checkpoint 是释放 Session admission 前的顺序栅栏，下一轮不能越过上一轮的
  文件边界；title generation 不定义边界，可以在 checkpoint 后异步执行；
- pump、cancel、活跃 handle 和 terminal ordering 归 runs，而非 transport/server。

Delivery 不自行创建 goroutine 驱动 run，也不持有 executor 生命周期。HTTP/SSE 和
inprocess transport 共享同一个 application use case，差异只在 envelope I/O。

## 3. Agent Execution 防腐层

`internal/adapter/agentexec` 是 Lyra 对 Lynx Agent/Core 的防腐层。顶层 `Engine` 直接
持有一个具体 `*agent/runtime.Engine`，只负责：

- 部署 root/subtask Agent definition；
- 创建、恢复 Agent process tree，并判断 opaque checkpoint 是否可恢复；
- 构造 system prompt、`core/chat.Request`、per-run model/process options；
- 把产品侧 streaming、pricing、observer 和工具集合接到 framework managed interaction；
- 将 framework final/stop/error 翻译成 `TurnOutput`。

`Engine` 的公开面只有 `StartTurn`、`RestoreTurn`、`CanResumeCheckpoint`。它不拥有
maintenance、MCP、tool catalog、skill discovery 或 capability closer，也不负责 Run
admission、Session 事务、delivery replay、workspace rollback 和协议错误映射。

### 3.1 Segment adapter 与 managed interaction

`internal/adapter/agentexec/turn` 是“一次 Segment 执行”的应用专用适配器：

```text
application Run command
  -> turn.Executor
  -> concrete in-process turn control
  -> agentexec.Engine Start/Restore
  -> Agent Process tree
  -> turn events
  -> application-owned runs.EngineEvent
```

`turn.New` 返回具体的进程内 turn control；它在 goroutine 启动前快照请求值，拥有 live
turn handle、subscribe、cancel/resume/rehydrate、approval/hooks 和 terminal first-wins。
`turn.Executor` 作为应用层的直接消费者，在自己的包内定义只含所需方法的窄控制端口；
Bootstrap 和 run-segment 同样只声明各自需要的关闭或 process-lookup 切片。turn 自身只通过
两方法 consumer interface 使用 Agent execution；steering 与完整的 turn-boundary maintenance
是两片独立依赖，不再经 Engine 中转。Delivery 不直接驱动 turn control。

每一轮 action 调用 `ProcessContext.Interact`。Agent framework 拥有：

- model/tool iteration；
- pending tool checkpoint；
- suspension/resume continuation；
- provider-neutral 的 token/cost/model-call 聚合计数与 limit；
- tagged final event。

Runtime 供应 model stream wrapper、`tools.Registry`、limits、cost projection 和 observer；
observer 从 model-response boundary 建立 application-owned per-model USD 账本。该账本不进入
Agent snapshot，而是作为 App-owned `ExecutorCheckpoint` metadata 与 opaque process-tree
payload 一起原子提交；只有 agentexec 能解释 payload 并校验两侧 usage 聚合。
正常完成直接读取 framework tagged `Final`；只有 budget/step 提前停止才保留局部 partial
文本。模型空闲 timeout 只包围一次 provider stream，长工具执行不进入该计时。

### 3.2 委派树与 HITL

`task` 是 Agent framework 的同步 AgentTool：root process 创建 child process，Runtime
通过显式 child options 传播本 Run 的 provider/model、observer、approval/hooks 和预算
归属，而不修改 Agent 默认的最小继承策略。

- token/cost/model-call 计入完整 process subtree；
- cancel root 会递归终止存活 child；
- `task` 本身是纯编排，不重复审批或执行 tool hooks；child 的真实工具调用逐个 gating；
- child approval/question 产生真实 nested suspension，不编码为普通工具 JSON；
- Agent 私有 checkpoint 保存 parent pending call 与 child relation；App 在完整 waiting tree
  capture 后，以一个 tree-barrier transaction 同时提交 checkpoint、root-owned Pending 与
  所有 active Run 的 suspension；公开 `ProcessSnapshot` / Runtime wire 不增加产品字段；
- Runtime suspension prompt 只允许 application-owned typed `runs.Interrupt`；
- suspension prompt 与 resolution 的 JSON 只在 `adapter/agentexec/suspension` 编解码；
  `runs.Interrupt` 与 `interrupts.Resolution`（包括 `approval.Scope`）不携带 agent 或 wire
  shape；
- resume 接受 root-owned direct suspension response set，按绑定的 process/suspension
  identity 推进完整树；已由 Host 结算的 ready checkpoint 直接 Continue，不伪造响应；
- `claimPark` 是 Resume/Cancel 的线性化点，保证竞争只产生一个 terminal。

child 的产品生命周期身份只来自 App opening：Framework 在发布/执行 child 前等待
`ChildOpeningRequest`，Application 原子写入 parent spawning Item 与 child Run 后返回
`ChildRunBinding{ProcessID, RunID, ParentRunID}`。SubagentStart/Stop hooks 对外只编码
`runId/parentRunId`；没有 App binding 的 Framework-internal child 不触发产品 hook。恢复时
Application 从 Continuation 传入既有 child binding，因此 Stop hook 不需要读取或暴露
Framework parent topology。binding set 必须组成以 root Run 为根的单一连通 App tree；opening
result 无论成功或失败都只完成一次，contract failure 同时送达等待 executor 并返回
Coordinator 触发 fail-close，不能让任一侧继续或永久等待。

因此一个应用 Run 即使跨 root/child、并列 suspension 和进程重启，每个 park boundary
仍只有一个 root-owned Pending aggregate；其中可以包含多个 source-aware direct
Interrupt，但只有一个 claim owner、一个 transaction 和一条 terminal 路径。

Interrupted tree 中取消 child 时，application 先在 root/worktree admission 内取得自己的
cancel claim，再通过 Agent adapter 规划 execution-only subtree transition。Agent plan 返回前
已经释放全部 Runtime ownership，不在 App transaction 期间持锁。App 的纯 transformation
随后一次确定 canceled postorder、parent spawning Item、reduced Pending / private tree
continuation 与 replacement checkpoint；runsegment 在一个 transaction 中提交这些 durable
facts。失败只释放 App claim，成功后才 Apply live Agent transition。
若 durable commit 已成功但 live Apply 失败，App 立即把 root Run tree 以 `run_lost` fail closed，
删除已失效的 durable checkpoint aggregate 并 teardown 旧 runtime tree；该补偿/恢复政策完全属于 App。
若仍有外部 Interrupt，整树继续静止；若移除了最后一个外部边界，同一 transaction 打开
surviving Runs 的新 Segment，Agent 通过 Continue 推进 ready checkpoint，不构造用户
Resume。Agent 始终不知道 BuildID、checkpoint Store、CAS 或 SQLite transaction。

### 3.3 Snapshot 与 Build identity

Process checkpoint 的执行状态、Agent 声明 compatibility 和 nested relation 由 Agent
framework 解释；Agent 不拥有 backend、宿主 build identity 或应用计费明细。
`adapter/agentexec` 在 Waiting 段边界调用 `SnapshotTree`，把完整
`core.ProcessSnapshotTree` 编码成一个 opaque payload，并返回 concrete App-owned
`execution.ExecutorCheckpoint`。App envelope 只包含 root process identity、payload、
BuildID、TurnScope、完整 model selection、RunLimits/usage，不包含 child 节点、parent topology、started-at 或
Framework type。capture 是纯计算，不执行 Store I/O，也不返回可执行 write capability。

Application 把这个 immutable value 放进完整的 tree-barrier write-set；`runsegment` 在一个
transaction 内调用自己的窄 `ExecutorCheckpointStore.SaveCheckpoint`，同时提交 Pending 和
所有 Run suspension。SQLite `executor_checkpoints` 每个 root 只有一行，只把 payload 当作
BLOB；`session_id` 仅是 App cleanup metadata。加载时 agentexec 经只读 `CheckpointReader`
取得 aggregate，检查 BuildID，解码并校验 tree/usage，最后调用 Agent 的
`ValidateRestoreTree` / `ValidateResumableSnapshot` / `RestoreTree`。具体 Store 的提交时间、
替换、删除、retention 和 transaction 全部留在 App。

同 root 的 checkpoint advance 不是任意 overwrite：Session owner、BuildID、TurnScope、完整
model selection 与 `RunLimits{MaxTotalTokens, MaxSteps, MaxBudgetUSD}` 一经 admission 即冻结；
cumulative Usage 必须保留所有已出现 model，
且 token/cost/call counters 只能前进。stale policy 或 usage regression 返回 invalid checkpoint，
原 aggregate 保持不变。

单值合法不等于 cross-aggregate 合法。Application 在 tree barrier、waiting-subtree、boot
recovery 和 restore 边界同时证明 checkpoint 与 Pending/Run/Session 的 root process、Session、
完整 model selection 和 goal lease 一致；restart 还独立核对 cwd/isolation。`Pending.GoalLeaseID` 是
App-owned continuation fact，保证 resumed root reducer 继续向同一 Goal incarnation 记账。
同一 lease 同时冻结在 root `Run.GoalLeaseID` admission provenance 中，使 boot/online
`run_lost` 即使不经过普通 reducer，也能把 exact Goal turn 与 terminal Run 原子提交；child
Run 禁止复制该 root-only lease。

RunLimits 只有一个 App 作者：同一个值进入 Run、Pending continuation、executor checkpoint、restore
expectation、SQLite 与 wire。`maxTotalTokens` 是整棵 Run tree 的 prompt + completion 累计 ceiling；
`params.maxTokens` 仍是单次 generation 输出上限，二者不共用名称或恢复来源。

Continuation 也不是一份可独立修改的恢复配置，而是 Run admission 的 durable hand-off。
它只携带 App Run facts 与 `RunID <-> ProcessID` opaque binding，不保存
`ParentProcessID/SpawnCallID`。恢复 route 的 parent relation 完全来自 `RunLineage`；child 首次
live event 到达时再验证 executor `ParentID/SpawnCallID`，验证通过后冻结该 transient source，
后续 topology 漂移直接失败。SQLite 因此没有第二份 executor tree。
`RunProtocolProfile` 只有一种 canonical 表示；Pending 子树只有在 profile 允许 child Run 时合法，
每个 durable interrupt kind 也必须已协商。tree barrier 写入、Resume、waiting-subtree cancellation、
boot recovery 和在线 parked-tree cancel/loss 都逐 Run 证明 lineage、creation time、完整 model
selection、cumulative metrics、frozen limits 与 profile 和 Run 记录同值；root 还必须同一 Goal lease。任一矛盾都在 executor probe 或
transaction 前 fail closed，不能通过 resume 把旧 accounting 回退、重新谈判 budget，或改写协议。
parked Run 存在时，`ClaimIdleSession` 禁止修改 Session cwd/isolation；能消费 Pending 的
lifecycle write-set 使用另一条 `ClaimSessionMutation`，不把两类授权混为一个“通用 mutation”。

Waiting snapshot 可以处于未回答或已回答待继续阶段。Agent 只校验和重建这两种 execution
state；App 决定是否把 response、请求幂等记录和 checkpoint 放入同一事务。恢复后未回答
状态进入 `Resume`，已回答状态直接进入 `Continue`，不在 App 重放或解释 Agent checkpoint。

`cmd/lyra` 在 bootstrap 前计算运行二进制内容的 `sha256:<hex>` BuildID；配置 App
execution adapter 时 BuildID 必填，并作为 checkpoint 行元数据保存，不进入 Agent deployment
digest 或 snapshot wire。只有 Waiting 段拥有可恢复 continuation，因此终态不写 process
snapshot。Waiting checkpoint 提交失败时 App 不暴露 interrupt，而是终止并失败收口该 Run；
build 不兼容、checkpoint 缺失或损坏确定性转为 `run_lost` 并清理 aggregate，不做 migration
或旧 shape 兼容。

终态遵循同一所有权：Agent `Discard` 只从 live Framework registry 移除终态 tree，不触碰
durable state；root Run 的 terminal `EventCommit` 把 `ObsoleteCheckpointRootID` 带入
App transaction，由 `runsegment` 将 Run terminalization 与 checkpoint 删除一起提交。child
terminal 不单独删除 root-owned aggregate。若 terminal transaction 失败，Run 与 checkpoint
同时保留。启动时 `application/runs.Recovery` 读取事实、校验完整 Interrupted Run tree +
Pending 和 canonical Session，并通过完整 `ExecutorCheckpointExpectation` 调用
`CanResumeCheckpoint` 查询 opaque continuation；它生成一个
`RecoveryCommit`，由 `adapter/runrecovery` 在同一 transaction 中把无效 Run 收口为
`run_lost`、修复 projection、记录 Goal-owned lost root 的 exact turn、删除 Pending，并删除
所有不在精确 preserved root set 中的 checkpoint aggregate。`RecoveryCommit.Validate` 在任何
adapter write 前校验 tree/postorder、Item owner、Goal turn、owner-bound deletion 与 retention
identity；SQLite 和 Bootstrap 都不拥有恢复政策。

在线 cancel 或 checkpoint loss 同样按 tree barrier 语义收口。Application 从 Pending 和 canonical
Session snapshot 生成 `TerminalPlan.Runs` canonical postorder，`adapter/persistence` 在一个
transaction 内依次修复 interrupt Items、删除 root checkpoint/Pending、terminalize 每个 child
再 terminalize root，并记录可选 root Goal turn。root-only 单 Run terminal write-set 已删除。

隔离工作区是当前进程拥有的 scratch copy，不进入 durable checkpoint。进程重启后即使
opaque payload 可解码，也不能把 executor 恢复到已经消失或重新创建的 world；因此 isolated
parked tree 由 Application 确定性收口为 `run_lost`，不尝试猜测式重建。

Session 与 executor process 是两套不同 identity。用户对话及 fork 才是 `Session`；delegated
work 由同一 Session 下的 first-class child Run 表达，不再从 child process 派生隐藏 Session。
当前 SQLite `schemaEpoch = 51`，没有旧 `process_states`、`sessions.kind`、双读写或迁移分支。
同一 `root_process_id` 的 Session owner 不可被 upsert 改写；普通 lifecycle 的定向删除必须
同时携带 Session，只有 boot exact-retention 使用全局 unowned cleanup。

### 3.4 Tool、MCP 与 maintenance

`adapter/toolset` 创建内建工具、MCP/A2A/LSP/exec 能力、role resolver、diagnostic catalog
和 capability closers。Agent Engine 只在部署 subtask 后把唯一的 `task` tool 注回
Resolver；catalog 仍归 toolset。

MCP status/catalog/connection/registry 四片接口定义在真实消费者
`application/integrations`，由 toolset adapter 实现并由 Bootstrap 直接注入。
OAuth 会话另走 `infra/mcp` 消费方定义的窄持久化接口：SQLite 只保存 opaque payload，
并以 server name + 规范化 HTTP origin 绑定；endpoint/transport 变化或授权拒绝都会先失效凭据。

turn-boundary steering 与 `BoundaryMaintenance` 接口定义在 `adapter/agentexec/turn`；
Bootstrap 默认绑定 conversation 与 `adapter/maintenance.Suite`。Suite 持有 mining、
curation、compaction、extraction 的顺序与条件，Dispatcher 只提供回合事实、记录失败并
发布摘要压缩边界；调用方仍可显式替换整个 maintenance 语义，不扩大 Engine。

## 4. Domain 与 Application 边界

Domain package 只维护需要跨用例保护的不变量。例如：

- `execution`：run 状态与事件语义；
- `session`：session 身份和状态；
- `provider`/`modelrole`：显式 provider+model 选择；
- `approval`：中断审批语义；
- `session`/`editguard`：会话隔离语义与编辑安全；
- `knowledge`、`todo`、`schedule`、`tool`：各自稳定领域规则。

Application package 按完整用例组织。它可以协调多个 Domain port，但不 import 具体
SQLite、Git、MCP、Agent runtime、concrete chat client 或 protocol DTO；Core chat/media
只作为跨边界值契约。列表/分页等读模型由 `application/queries` 读取 projection，不强迫
所有查询加载完整 aggregate。跨 Session 的 durable metering 由
`application/usage` 聚合；workspace 文件、Git 和发现查询由
`application/workspace` 协调其窄端口，Delivery 只投影结果。

Port 放在消费方。跨环技术边界或真实可替换策略使用窄接口；仅包内单实现胶水直接用
具体类型，不为测试凭空制造 SPI。详见 [`EXTENSIBILITY.md`](EXTENSIBILITY.md)。

## 5. Persistence 与事务

开发期只有一个 SQLite 技术后端，位于 `internal/infra/storage`。单一后端不意味着
Application 依赖 SQLite：用例依赖自己需要的窄读写/事务端口，Bootstrap 注入具体实现。

持久化原则：

- durable state 先 commit，成功后才 publish `RunEvent`；
- fresh start 的 admission 与 opening projections 在同一事务提交；
- resume 的 interrupt consume、run resume state 与 opening projections 同批提交；
- waiting tree 的 checkpoint、Pending 与全部 Run suspension 在同一事务提交；
- Pending 只允许 insert-only `Open`；replacement 必须在同一 lifecycle transaction 先 Consume
  旧 barrier；Consume/Delete 同时携带 Session owner 与 root Run identity；
- root terminalization 与 obsolete executor checkpoint 删除在同一事务提交，定向删除同时匹配
  Session owner；
- parked tree 的全部 child/root terminalization、interrupt Item repair、Pending/checkpoint 删除与
  root Goal turn 在同一事务提交；
- Session delete 通过 checkpoint root 的 App-owned `session_id` 元数据删除该 Session 的全部
  checkpoint aggregates；boot 只保留被完整 Interrupted Run/Pending 及 resumability 证明拥有的 roots；
- 数据库与 filesystem/Git 不能伪装成一个原子事务，跨资源操作使用显式 intent 和补偿；
  Git work-tree reset 本身也不是跨文件原子操作，因此 files-only rollback 同样先记录
  intent；intent 明确携带是否还需截断 history，启动恢复只重驱已请求的效果；
- 一个 Session 至多一个非 terminal Run 由数据库约束兜底，不只靠内存锁；
- transcript/history 是 projection，不替代 Run aggregate。

当前 SQLite `schemaEpoch = 51`，`runs.goal_lease_id`、`interrupts.goal_lease_id`、
`mcp_oauth_sessions` 与
`runs.max_total_tokens` 均属于唯一现行 shape；不读取或迁移任何其他 epoch 的数据库。

用户可编辑的 `LYRA.md` 是有意保留的文件型知识源，不属于通用存储开关。

## 6. Transport 与协议

`internal/delivery/protocol` 是 Lyra Runtime Protocol 的 Go 投影；完整 wire 规范在
[`../../desktop/docs/protocol`](../../desktop/docs/protocol)。改变 method、error、event
或 header 必须前后端同步。

Transport 只负责：

1. decode/encode envelope；
2. 把 transport metadata 放进 context；
3. 调用一个 server/application 入口并传输结果流。

HTTP 使用 JSON-RPC over streamable HTTP/SSE，每个流式调用使用自己的 POST response
stream；inprocess transport 为未来 CLI/TUI 复用同一协议入口。业务错误在 JSON-RPC
error 中表达，HTTP status 只代表 transport failure。

## 7. Bootstrap 与资源生命周期

`bootstrap.Stack` 是 server 所需 coordinator/notifier 的 discovery 聚合，不拥有业务
方法，也不拥有 closer。`bootstrap.Host` 通过共享的 immutable `hostLifetime` 持有唯一
shutdown graph；Host 被复制、并发 Close 或其公开 Stack 被改写，都不会改变实际关闭的
资源集合。

关闭顺序固定为：

```text
integrations reconcile tasks
  -> codebase reindex tasks
  -> active Run pumps
  -> active Agent turn/process trees
  -> run-boundary effects
  -> tool capability closers (reverse creation order)
  -> injected process resources / persistence (reverse creation order)
```

Engine 没有空壳 `Close`。toolset 创建的 MCP/A2A session、LSP analyzer 和 background shell
从成功创建起由 bootstrap 的 staged ownership guard 暂管，只有 Host 完整构造后才转移；
中途失败先关 Dispatcher，再逆序释放工具资源。调用方注入的 `Config.Resources` 仅在
Assemble 成功后转移，失败时仍归调用方。

后台 goroutine 必须绑定 Host 或 Run 的 context，不能泄漏到 package 全局。Provider
credential、MCP session、LSP process、SQLite handle 等 runtime resource 都由组合根
明确拥有，并且 Close 必须幂等、聚合错误、不因单个资源失败跳过后续清理。

## 8. 并发与事件

- turn 的两个异步入口在启动 goroutine/process 前完整快照 `chat.Options`、media 和
  interrupt kinds；goroutine 不读取调用方可变 slice/map/pointer；
- 通用 chat options 约束由 Core `Validate` 负责，Runtime 只追加应用特有约束；
- 同一 Run 只有一个 application owner 驱动 terminal transition；
- terminal first-wins，重复 cancel/close 必须幂等；
- journal 的 publish 顺序与 durable commit 顺序一致；
- transport subscriber 慢不能反向改变领域状态；backpressure/drop 策略在 delivery 明确；
- context 从 request/Run 向 model、tool、MCP、exec、Git 和 storage 传播；
- model stream timeout、tool context、Run cancel 各有独立 owner，不复用一个整轮 timer；
- race test 覆盖 pump、cancel/resume、nested child、subscriber、Host Close 和 terminal
  ordering。

## 9. 可观测性

Trace 关联使用 W3C context propagation。Chat/provider、Agent、VectorStore、MCP 等 SDK
埋点在外圈 wrapper；Core 保持标准库依赖。Application span 使用稳定的 Run/Segment
身份，Delivery span 只描述 protocol/transport，日志经 `slog` 输出并避免记录 credential。

统一约定见 [`../../../doc/OBSERVABILITY.md`](../../../doc/OBSERVABILITY.md)。

## 10. 架构变更完成定义

一次 Runtime 架构变更只有在以下条件同时成立时才完成：

- 所有权能用 Domain/Application/Adapter/Infra/Delivery/Bootstrap 中唯一一环解释；
- 没有新增 facade、双事件族、全局 registry 或短命兼容层；
- `internal/arch` 对依赖方向和关键所有权的测试通过；
- 相关 domain/application/adapter/delivery/infra 测试通过；
- 涉及并发时 race test 通过；
- workspace 与 `GOWORK=off` standalone module graph 均可 build/vet/test；
- 协议变化已同步 desktop protocol 文档；
- 本文、模块 CLAUDE/README 和 GoDoc 与实现一致。

## 11. 明确不做

- 不引入 DI container、EventBus、Mediator、CQRS/Saga framework；
- 不建立统一 Repository 基类或 AggregateRoot marker；
- 不把 agent loop、transport、server 或 coordinator 做成插件市场；
- 不让 Delivery 重新拥有 Run 生命周期；
- 不让 Domain/Application import Lynx provider SDK、SQLite、Git 或 wire DTO；
- 不为历史 API/wire/database shape 保留 bridge、dual-read/write 或兼容字段。
