# Execution-Centered Architecture — `app/runtime` 架构基准

> 状态：现行架构基准。历史收敛过程见
> [`EXECUTION_ARCHITECTURE_CONVERGENCE_PLAN.md`](EXECUTION_ARCHITECTURE_CONVERGENCE_PLAN.md)，
> Core/Chat 重构过程见
> [`../../../doc/CORE_ARCHITECTURE_EXECUTION_PLAN.md`](../../../doc/CORE_ARCHITECTURE_EXECUTION_PLAN.md)。

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
├── domain/       execution、session、provider、tool、approval、worktree…
├── application/  runs、sessions、models、workspace、integrations、queries…
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
- 创建、恢复和判断 Agent process tree 是否可恢复；
- 构造 system prompt、`core/chat.Request`、per-run model/process options；
- 把产品侧 streaming、pricing、observer 和工具集合接到 framework managed interaction；
- 将 framework final/stop/error 翻译成 `TurnOutput`。

`Engine` 的公开面只有 `StartTurn`、`RestoreTurn`、`ResumableProcess`。它不拥有
maintenance、MCP、tool catalog、skill discovery 或 capability closer，也不负责 Run
admission、Session 事务、delivery replay、workspace rollback 和协议错误映射。

### 3.1 Segment adapter 与 managed interaction

`internal/adapter/agentexec/turn` 是“一次 Segment 执行”的应用专用适配器：

```text
application Run command
  -> turn.Executor
  -> turn.Dispatcher
  -> agentexec.Engine Start/Restore
  -> Agent Process tree
  -> turn events
  -> application-owned runs.EngineEvent
```

`turn.Dispatcher` 在 goroutine 启动前快照请求值，拥有 live turn handle、subscribe、
cancel/resume/rehydrate、approval/hooks 和 terminal first-wins。它只通过两方法
consumer interface 使用 Agent execution；steering、compaction、extraction 是三片独立
依赖，不再经 Engine 中转。Delivery 不直接驱动 Dispatcher。

每一轮 action 调用 `ProcessContext.Interact`。Agent framework 拥有：

- model/tool iteration；
- pending tool checkpoint；
- suspension/resume continuation；
- model-call usage ledger；
- token/cost/model-call limit；
- tagged final event。

Runtime 只供应 model stream wrapper、`tools.Registry`、limits、attribution 和 observer。
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
- Agent 私有 checkpoint 保存 parent pending call 与 child relation，按 leaf → root
  durable commit；公开 `ProcessSnapshot` / Runtime wire 不增加嵌套字段；
- Runtime suspension prompt 只允许 application-owned typed `runs.Interrupt`；
- resume 回到最深 waiting child，完成原 pending tool 后再继续 parent；
- `claimPark` 是 Resume/Cancel 的线性化点，保证竞争只产生一个 terminal。

因此一个应用 Run 即使跨 root/child、连续 suspension 和进程重启，每个 park boundary
仍只有一个 active interrupt，整个 Run 仍只有一个 journal 和一条 terminal 路径。

### 3.3 Snapshot 与 Build identity

Process checkpoint 的结构、revision、deployment compatibility 和 nested relation 都由
Agent framework 解释。Runtime 只按 process ID 调用 `Resumable` / `RestoreResumable`，
不读取 checkpoint payload 来推断可恢复性。

`cmd/lyra` 在 bootstrap 前计算运行二进制内容的 `sha256:<hex>` BuildID；启用
ProcessStore 时 BuildID 必填，SnapshotFailurePolicy 固定为 `fail_process`。snapshot
写失败立即使 Process/Run 失败；build 不兼容、snapshot 缺失或损坏确定性转为
`run_lost` 并清理 process tree，不做 migration 或旧 shape 兼容。

### 3.4 Tool、MCP 与 maintenance

`adapter/toolset` 创建内建工具、MCP/A2A/LSP/exec 能力、role resolver、diagnostic catalog
和 capability closers。Agent Engine 只在部署 subtask 后把唯一的 `task` tool 注回
Resolver；catalog 仍归 toolset。

MCP status/catalog/connection/registry 四片接口定义在真实消费者
`application/integrations`，由 toolset adapter 实现并由 Bootstrap 直接注入。

turn-boundary steering/compaction/extraction 接口定义在 `adapter/agentexec/turn`；
Bootstrap 默认绑定 conversation 与 `adapter/maintenance`，调用方可显式替换。它们的
生命周期和失败语义由 Dispatcher 管理，不扩大 Engine。

## 4. Domain 与 Application 边界

Domain package 只维护需要跨用例保护的不变量。例如：

- `execution`：run 状态与事件语义；
- `session`：session 身份和状态；
- `provider`/`modelrole`：显式 provider+model 选择；
- `approval`：中断审批语义；
- `worktree`/`editguard`：工作区隔离与编辑安全；
- `knowledge`、`todo`、`schedule`、`tool`：各自稳定领域规则。

Application package 按完整用例组织。它可以协调多个 Domain port，但不 import 具体
SQLite、Git、MCP、Agent runtime、concrete chat client 或 protocol DTO；Core chat/media
只作为跨边界值契约。列表/分页等读模型由 `application/queries` 读取 projection，不强迫
所有查询加载完整 aggregate。

Port 放在消费方。跨环技术边界或真实可替换策略使用窄接口；仅包内单实现胶水直接用
具体类型，不为测试凭空制造 SPI。详见 [`EXTENSIBILITY.md`](EXTENSIBILITY.md)。

## 5. Persistence 与事务

开发期只有一个 SQLite 技术后端，位于 `internal/infra/storage`。单一后端不意味着
Application 依赖 SQLite：用例依赖自己需要的窄读写/事务端口，Bootstrap 注入具体实现。

持久化原则：

- durable state 先 commit，成功后才 publish `RunEvent`；
- fresh start 的 admission 与 opening projections 在同一事务提交；
- resume 的 interrupt consume、run resume state 与 opening projections 同批提交；
- 数据库与 filesystem/Git 不能伪装成一个原子事务，跨资源操作使用显式 intent 和补偿；
  Git work-tree reset 本身也不是跨文件原子操作，因此 files-only rollback 同样先记录
  intent；intent 明确携带是否还需截断 history，启动恢复只重驱已请求的效果；
- 一个 Session 至多一个非 terminal Run 由数据库约束兜底，不只靠内存锁；
- transcript/history 是 projection，不替代 Run aggregate。

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
