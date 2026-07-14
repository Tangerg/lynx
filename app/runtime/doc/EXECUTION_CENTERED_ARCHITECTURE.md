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
- Application 只依赖 Domain 和自己定义的消费方端口；
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
- pump、cancel、活跃 handle 和 terminal ordering 归 runs，而非 transport/server。

Delivery 不自行创建 goroutine 驱动 run，也不持有 executor 生命周期。HTTP/SSE 和
inprocess transport 共享同一个 application use case，差异只在 envelope I/O。

## 3. Agent Execution 防腐层

`internal/adapter/agentexec` 是 Lyra 对 Lynx Agent/Core 的防腐层。它负责：

- 解析一次 run 的 provider/model 与 `chatclient.Client`；
- 构造 system prompt、普通 `core/chat.Request` 和邻接 `tools.Registry`；
- 用唯一的 `agent/toolloop.Runner` 驱动 model/tool 事件；
- 将 Lynx event 投影为 application-owned `runs.EngineEvent`；
- 保存/恢复 Lynx `ProcessSnapshot` 和 tool-loop `Checkpoint`；
- 协调 turn 内的 steering、compaction、memory extraction 与 toolset。

它不负责 Run admission、Session 事务、delivery replay、workspace rollback、MCP 配置
写入或协议错误映射。

### 3.1 Tool-loop

工具执行使用目标 `core/chat` + `tools.Tool` + `agent/toolloop` 契约：

```text
chat.Request + tools.Registry
  -> model_request / model_response
  -> tool_call / tool_result (serial)
  -> pause(checkpoint) | final | error
```

Runner 不自动 retry，普通工具错误反馈成 error ToolResult，round limit 由调用方配置。
暂停时持久化 JSON-safe Checkpoint；恢复从 pending tool 继续，不重调模型、不重跑已完成
工具。Provider stream 在 adapter 边界汇聚为同步 Model 调用，同时单独投影 UI delta 与
usage；协议 Response 不携带 runtime 状态。

## 4. Domain 与 Application 边界

Domain package 只维护需要跨用例保护的不变量。例如：

- `execution`：run 状态与事件语义；
- `session`：session 身份和状态；
- `provider`/`modelrole`：显式 provider+model 选择；
- `approval`：中断审批语义；
- `worktree`/`editguard`：工作区隔离与编辑安全；
- `knowledge`、`todo`、`schedule`、`tool`：各自稳定领域规则。

Application package 按完整用例组织。它可以协调多个 Domain port，但不 import 具体
SQLite、Git、MCP、Agent SDK 或 protocol DTO。列表/分页等读模型由
`application/queries` 读取 projection，不强迫所有查询加载完整 aggregate。

Port 放在消费方。只有第三方或另一实现确实可能替换的能力才抽接口；内部单实现胶水
直接用具体类型。详见 [`EXTENSIBILITY.md`](EXTENSIBILITY.md)。

## 5. Persistence 与事务

开发期只有一个 SQLite 技术后端，位于 `internal/infra/storage`。单一后端不意味着
Application 依赖 SQLite：用例依赖自己需要的窄读写/事务端口，Bootstrap 注入具体实现。

持久化原则：

- durable state 先 commit，成功后才 publish `RunEvent`；
- fresh start 的 admission 与 opening projections 在同一事务提交；
- resume 的 interrupt consume、run resume state 与 opening projections 同批提交；
- 数据库与 filesystem/Git 不能伪装成一个原子事务，跨资源操作使用显式 intent 和补偿；
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
方法，也不拥有 closer。`bootstrap.Host` 持有 dispatcher、background task、engine 和
外部资源，并按反依赖顺序关闭。

构造失败必须关闭已经成功创建的资源。后台 goroutine 必须绑定 Host 或 Run 的 context，
不能泄漏到 package 全局。Provider credential、MCP session、LSP process、SQLite handle
等 runtime resource 都由组合根明确拥有。

## 8. 并发与事件

- 同一 Run 只有一个 application owner 驱动 terminal transition；
- terminal first-wins，重复 cancel/close 必须幂等；
- journal 的 publish 顺序与 durable commit 顺序一致；
- transport subscriber 慢不能反向改变领域状态；backpressure/drop 策略在 delivery 明确；
- context 从 request/Run 向 model、tool、MCP、exec、Git 和 storage 传播；
- race test 覆盖 pump、cancel、resume、subscriber 和 terminal ordering。

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
- 协议变化已同步 desktop protocol 文档；
- 本文、模块 CLAUDE/README 和 GoDoc 与实现一致。

## 11. 明确不做

- 不引入 DI container、EventBus、Mediator、CQRS/Saga framework；
- 不建立统一 Repository 基类或 AggregateRoot marker；
- 不把 agent loop、transport、server 或 coordinator 做成插件市场；
- 不让 Delivery 重新拥有 Run 生命周期；
- 不让 Domain/Application import Lynx provider SDK、SQLite、Git 或 wire DTO；
- 不为历史 API/wire/database shape 保留 bridge、dual-read/write 或兼容字段。
