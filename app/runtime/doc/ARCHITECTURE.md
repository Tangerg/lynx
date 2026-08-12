# Lyra Runtime 目标架构

> 状态：已接受的重构目标设计
>
> 适用范围：`app/runtime` 及其为完成服务端重构必须调整的直接后端依赖

本文定义 Lyra Runtime 重构完成后的稳定架构、统一语言、所有权和依赖方向。它不记录逐批进度，不枚举完整协议字段，也不复制 Agent Framework 的内部设计。

## 1. 定位

Lyra Runtime 是产品应用后端，不是第二个 Agent Framework。

它面向桌面、Web、CLI 和 TUI 客户端，负责把用户意图、产品状态、Agent 执行、工具能力、持久化和 Runtime Protocol 组织为一套可恢复、可观察的应用生命周期。

Runtime 的中心是用户能够理解和持久追踪的 `Run`，不是某一种模型循环、规划策略、Agent Process 或网络连接。

Runtime 必须同时满足：

- 领域模型表达产品规则，而不是数据库行、JSON-RPC DTO 或 Agent SDK 类型；
- Application 拥有用例、副作用顺序和跨聚合一致性；
- Agent Framework、SQLite、模型 Provider、Git、Shell、MCP、LSP 和网络都停留在外环；
- Delivery 只把协议请求投影为应用命令，把应用事实投影为协议响应；
- Bootstrap 是唯一组合根，不承载业务行为；
- streamable HTTP 与公共 embedded binding 只承载协议，且不改变应用语义；二者必须复用同一 binding-neutral operation 入口与 Application 用例；
- Agent Framework 是唯一托管执行内核，Runtime 不复制它的 Process loop、tree scheduler 或 snapshot 解释器。

## 2. 设计原则

### 2.1 Run-centered

一次用户请求从 admission、首个流式区段、可能的中断与恢复，到唯一终态，构成一个 `Run`。恢复只打开新的 `Segment`，不创建第二个逻辑 Run。

Run 生命周期是 Application 的应用事务边界和 Delivery 的产品观察边界。Agent Framework Process 是执行实现，不是产品生命周期本身。

### 2.2 依赖指向策略和领域

源码依赖只能指向更稳定、更接近产品语义的一侧。运行时调用方向可以相反，但不能通过 import 反转所有权。

### 2.3 每个事实只有一个权威所有者

同一事实不能在 Framework state、Runtime domain、SQLite row、协议 DTO 和 UI cache 中各自推进。其他形态只能投影或保存所有者提供的不透明值。

### 2.4 生命周期只有一个推进者

Agent Framework Engine 是 Process 和 Process tree 的唯一推进者。Runtime 只发出开始、恢复、语义输入、取消等请求，并消费已发生事实；不得在外部再建一套 Turn controller、Tool loop、child scheduler 或 continuation state machine。

### 2.5 事务属于 Application

跨 Run、Session、Transcript、Interrupt、Goal 和 checkpoint 的原子写入由具体应用用例拥有。Domain 保护单个模型的不变量；Adapter 执行消费方端口；Agent Framework 不认识 Store、transaction、CAS、lease、schema epoch 或业务幂等。

### 2.6 防腐而非转发

Adapter 必须翻译两侧语义。只把 Agent Framework、SQLite 或 Provider 的类型原样转发给内环，不叫适配，而是抽象泄露。

### 2.7 Go-first DDD

DDD 用于澄清语言、实体行为、聚合和所有权，不用于复制 Java 式 Repository/Service/Manager 层级。包只在有独立语义、独立变化原因或需要切断依赖时存在；接口由真实消费者定义。

## 3. 统一领域语言

| 术语 | 唯一含义 | 明确不表示 |
|---|---|---|
| `Session` | 一段持久产品会话，拥有 workspace 绑定、默认模型和根 Run admission 约束 | Agent Framework Process、网络连接 |
| `Run` | 用户看到的一次完整逻辑执行，跨中断与恢复保持同一身份 | 一次模型调用、Agent Framework Execution |
| `Segment` | 一个 Run 中连续输出的一段；首启和每次恢复分别打开新 Segment | Run、Agent Framework Step |
| `Conversation` | Host 拥有、用于构造未来模型上下文的产品消息历史 | Transcript、WorkingContext |
| `Transcript` | 面向用户和审计的权威 Items-and-Runs 记录 | 模型恢复状态、Framework Event 流 |
| `Knowledge` | 用户可编辑的 home/projectRoot/cwd `LYRA.md` 级联 | Conversation、Agent memory index |
| `Interrupt` | 一个 Run tree 等待外部答案或裁决的产品请求 | Agent Framework Signal payload 本身 |
| `Plan` | 当前工作请求的步骤状态 | Goal、Todo 的别名 |
| `Goal` | 跨多个 Run 持续推进的自主目标 | 当前 Run 的 Plan |
| `Executor` | Application 消费的、实现 Run 执行能力的端口 | Agent Framework Engine 的别名 |
| `Executor member` | Application 为路由或产品 child Run 映射保存的不透明执行树成员身份 | 可由 Application 解释的 Process 对象 |
| `Checkpoint` | Application 拥有的恢复聚合，包含 Host metadata 和不透明 executor payload | Agent Framework snapshot 的第二份可解释模型 |
| `Run event` | Application 已提交并可向 Delivery 发布的产品事实 | Agent Framework Event 或 Delta 的原样转发 |

以下术语只属于 Agent Framework：`Deployment`、`Process`、`Execution`、`Signal`、`Effect`、`WaitID`、`Snapshot`、`TreeSnapshot`。它们不得成为 Domain、Application、Delivery 或 Runtime wire 的公共语言。

## 4. 真相源

### 4.1 Conversation、Transcript 与 WorkingContext

三者必须永久分离：

- Conversation 是 Host 产品历史，可被用户 truncate、fork 或重建；
- Transcript 是已经发生的用户可见事实和 Run/Item 观察记录；
- `interaction.WorkingContext` 是一个 Agent Framework Process 精确恢复所需的私有执行状态。

创建新 Process 时，Agent adapter 可以用 Conversation 构造 Interaction Input。恢复既有 Process 时，只能使用它自己捕获的 TreeSnapshot；不得从后来变化的 Conversation 猜测或重算 WorkingContext。

### 4.2 权威观察与临时流

最终模型响应、ToolCall 开始/结果、token usage 和价格归属必须在 caller-owned model/tool 调用链中同步投影。模型 chunk 只用于低延迟展示；它可以作为 provisional Transcript 内容写入，但不能成为最终响应的唯一副本。最终响应必须包含可独立恢复的完整语义内容。

Agent Framework Event 只承载 Framework 生命周期事实；Delta 是有界、可能丢失、恢复后不重放的临时输出。Runtime 不得用 Delta 拼接最终结果或构建权威 Transcript。

每个外部调用使用一个 dispatch-attempt scope 串起 outer Agent Framework Dispatcher decorator、model client decorator 和 Tool decorator。调用前的 authoritative `started` write 必须先成功；失败时不得调用外部能力。外部调用一旦开始，最终响应/结果/usage 的 durable write 失败就必须由 outer Dispatcher 作为 indeterminate outcome 返回，使 Agent Framework 记录 unknown settlement；不能把“外部已成功、Runtime 写盘失败”伪装成普通 Tool error 或模型失败并允许自动重试。chunk/progress projection 失败只产生可观察 drop，不改变 Effect settlement。

### 4.3 Checkpoint

Application checkpoint 包含：

- Runtime BuildID；
- Session、Run 和 executor root 的应用身份；
- frozen model selection、Run limits、capabilities 和产品 accounting metadata；
- child Run 与 executor member 的应用映射；
- Agent Framework `TreeSnapshot` 的不透明编码。

只有 `adapter/agentexec` 可以编解码 TreeSnapshot 外层并调用 Agent Framework 的校验/恢复 API。任何 Runtime 层都不能解析其中的 Process snapshot、ExecutionState payload、Interaction phase、mailbox 或 Strategy protocol。

## 5. DDD 边界

### 5.1 `run`

`run` 是执行生命周期领域，拥有：

- Run、Segment 和稳定身份；
- 合法状态迁移和唯一终态；
- lineage、limits、capabilities、usage snapshot 与 outcome；
- 对取消、丢失、完成和中断边界的领域行为。

它不拥有 executor reference/checkpoint、Conversation 内容、Transcript Item 排序、Agent Framework Process 状态或 Store 操作。executor binding 是 Application 为驱动外部执行端口保存的应用状态，不是 Run entity 的领域属性。现有 `domain/execution` 将在重构中治本改名为 `domain/run`，不保留 alias。

### 5.2 `session`

`session` 拥有产品会话身份、workspace 绑定、默认模型、隔离选择和 admission 相关状态。它不驱动 Run，也不拥有 executor handle。

### 5.3 `conversation`

`conversation` 拥有模型上下文历史、count watermark、truncate、seed 和 fork 语义。它不是 Run aggregate 的内部 slice，也不从 Transcript 或 Agent snapshot 反向生成。

### 5.4 `transcript`

`transcript` 拥有稳定 Item、Run projection、顺序、rollback/fork boundary、ToolCall 开始与完成时间以及可观察内容。Question Item 还不可变保存 Runtime 已接受的响应；未回答或取消不生成答案。它不决定 Run 状态机，不保存 WorkingContext。

### 5.5 `interrupt`

`interrupt` 拥有外部请求、答案、scope 与 Kind/Resolution 等纯语义。可观察的 question/approval Item 仍由 `transcript` 投影；把一个 root Run tree 的产品事实、executor binding 和 continuation 组合成可恢复 pending hand-off 的职责属于 `application/runs`，不能为了把它塞进 Domain 而让 `interrupt` 依赖 Application 或 executor 类型。Agent Framework pending input 只在防腐层被翻译为这些产品值。

### 5.6 `accounting`

`accounting` 拥有模型 token、调用次数、价格和产品预算值。Agent Framework Usage 只提供 Steps、Effects、Signals、Delta 等框架资源事实，不能替代产品 accounting。

### 5.7 其他上下文

以下上下文继续保持独立语义：

- `goal`：目标 incarnation、fresh Start 时冻结的 Run capabilities、预算、跨 Run 进度与终态；
- `plan`：当前 Plan 与 Step 状态；
- `knowledge`：人类可编辑项目知识；
- `agentmemory`：经过蒸馏和检索的 Agent memory；
- `approval`：工具风险与用户裁决规则；
- `provider` / `modelref`：Provider 注册和精确模型选择；
- `tool`：产品侧工具事实、结果和展示语义；
- `skills`：Skill 与 Proposal；
- `schedule`：重复执行定义；
- `feedback`、`hooks`、`mcpserver`、`codebaseindex`：各自已有的真实产品能力。

一个 package 是否继续独立，必须由实体所有权、变化原因和真实消费者证明，不能由目录对称性决定。

### 5.8 Aggregate roots

| Aggregate root | 内部一致性边界 | 不拥有 |
|---|---|---|
| `Run` | lifecycle、Segment、lineage、limits、capabilities、outcome | Transcript Items、executor state、Store |
| `Session` | workspace/default model/isolation 和 root admission 状态 | Run 推进、Conversation 内容 |
| `Conversation` | message sequence、watermark、truncate/seed/fork | Transcript、WorkingContext |
| `Transcript` | Item/Run projection 顺序、accepted Question response、rollback/fork boundary | Run 状态机、Agent state |
| `Pending` | 一个 root Run tree 的 open Interrupt 集与答案/claim | Framework wait/mailbox |
| `Goal` | incarnation、frozen Run capabilities、budget、progress、terminal outcome | 当前 Run Plan、executor loop |
| `Plan` | 当前 Plan revision 与 Step 状态 | Goal lifecycle、delegated Process |
| `Schedule` | cadence、enabled state 和下次触发定义 | Run admission 和执行 |

读取 projection 不等于取得 aggregate mutation 权。跨 aggregate 行为必须回到明确 Application use case。

### 5.9 跨聚合一致性

Domain entity 只保护自身不变量。需要同时改变多个 aggregate 时，由一个明确命名的 Application use case 计算并提交完整 write-set，例如：

- Run admission + Session active root；
- waiting checkpoint + Pending interrupts + Run state；
- accepted Question answer + resume claim/checkpoint replacement；
- terminal Run tree + Transcript repair + checkpoint deletion + Goal outcome；
- conversation rollback + transcript boundary + executor invalidation/cleanup intent。数据库事务不能直接 kill live tree；commit 后由 Run owner 执行 release，进程在两者之间崩溃则由启动 reconciler 根据已提交 intent 收口。

不得建立通用 `UnitOfWork`、`Repository[T]`、EventBus、Saga 或 CQRS framework。每个写集合使用消费方定义、语义准确的窄端口。

### 5.10 Context map

| 消费方 | 上游事实 owner | 关系与限制 |
|---|---|---|
| `transcript` | `run` identity、`tool`/`modelref` values | 只保存观察投影，不修改 Run/Tool/Model |
| `interrupt` | `run` identity、Transcript Item identity、`approval` decision | 只表达外部等待，不推进 executor |
| `accounting` | `modelref` | 计算产品 usage/cost，不决定模型选择 |
| `goal` | `run` identity/outcome | 记录跨 Run 进度；启动下一 Run 只通过 Application port |
| `schedule` | `modelref` 和 Application Run starter | 定义触发，不直接依赖 runs concrete coordinator |
| `conversation` | stable chat value contract | 管理产品消息历史，不依赖 Transcript/Run implementation |
| `application/runs` | Run、Session、Conversation、Transcript、Interrupt、Accounting | 作为明确用例协调者，不把跨聚合规则塞进任一 Store |
| Goal/Schedule/Tool application use cases | Runs capability | 在各自消费者处定义窄接口，由 composition 绑定 runs concrete type，避免 application package 横向胖依赖 |

跨上下文共享只允许引用权威 owner 的稳定 identity/value。不得为了避免 import 而复制 enum/ID，也不得为了复用一个小 helper 建立“shared domain”包。

## 6. Clean Architecture 依赖图

```text
                              cmd
                               |
                         bootstrap/config
                        /       |        \
                 delivery    adapter     infra
                    |        /     \       |
                    |   application  \      |
                    |       |         \     |
                    +-------+----------> domain

adapter/agentexec --------------------------> agent
adapter/* ----------------------------------> external SDKs
```

图表示源码依赖，不表示运行时调用方向。

### 6.1 Domain

Domain 只包含 entity、value object、aggregate 行为、纯领域策略和 sentinel errors。

允许依赖：

- 标准库中的纯值/算法包；
- 仓库中已经证明稳定且没有 I/O/Framework 语义的值契约；
- 依赖方向明确的其他领域值包，但禁止环。

禁止依赖：

- Application、Adapter、Infra、Delivery、Bootstrap；
- Agent Framework、SQLite、JSON-RPC、HTTP、OTel SDK、文件系统和外部 Provider SDK；
- 用 `context.Context` 包装的 Store/Client/Repository；这类消费端口默认属于 Application。

### 6.2 Application

Application 以用例为 package，协调 Domain、消费方端口、事务顺序、并发所有权和产品事件。

Application：

- 定义它实际消费的最小接口；
- 接受 interface，返回应用值；
- 可以使用官方 OTel API 表达用例 span/metric，但不依赖 exporter/SDK，且 telemetry 不参与业务判断；
- 不导入 Adapter、Infra、Delivery、Bootstrap、Agent Framework 或外部驱动 SDK；
- 不解释 wire DTO 或 executor snapshot；
- 不用一个 `Coordinator`/`Service` 横跨多个无关用例。

### 6.3 Adapter

Adapter 实现 Application 消费端口并翻译外部能力。若 Domain 存在纯策略合同，只能由无 I/O 的纯实现满足；外部 I/O 不能借 Adapter 名义反向成为 Domain port。Adapter 可以依赖 Domain、Application、Infra 和对应 SDK，但不能依赖 Delivery 或 Bootstrap。

典型 adapter：

- `agentexec`：Agent Framework 防腐层；
- `persistence` / `runsegment` / `runrecovery`：把应用 write-set 映射到 SQLite 技术事务；
- `modelclient`：把 Provider/model selection 映射到 chat client；
- `toolset`：组装通用 Tool 能力；
- `workspace`、`maintenance`、`pricing`、`hooks`：各自准确的应用适配。

Adapter 不能只是同名 Infra 类型的透传包装。没有翻译、组合或策略意义的中间层必须合并。

### 6.4 Infra

Infra 提供可被多个 Adapter 复用的底层技术机制，例如 SQLite primitive、Git、进程执行、sandbox、LSP、MCP/A2A client 和 checkpoint filesystem mechanism。

Infra：

- 不组织应用用例；
- 不依赖 Application、Adapter、Delivery 或 Bootstrap；
- 可以使用稳定 Domain value 实现技术 mechanism；需要 I/O 的消费接口由 Application 定义，并由 Adapter 把 Infra mechanism 翻译为该端口；
- 不持有产品生命周期和业务事务策略；
- 不以“未来可能复用”为由提前抽象。

`adapter` 与 `infra` 不整体合并。每个包按以上规则裁决；纯转发包装删除，真实的技术 mechanism 保留。

### 6.5 Protocol 与 Delivery

公共 `protocol` 只包含 binding-neutral Runtime 合同：DTO、枚举、请求/响应、事件、客户端可见 problem、版本与严格验证。它不包含服务端 method-group interface、context key、Router、numeric JSON-RPC code、HTTP status、reflection registry 或生成器实现细节。

内部 Delivery 包含四个职责：

- `operation`：类型化 operation catalog、validation、capability、idempotency、problem projection 与 stream replay attachment；
- `server`：Application use case 与 protocol value 的双向 projection；它不拥有 listener、connection 或 Runtime lifecycle；
- `dispatch`：JSON-RPC envelope routing、numeric error mapping 与 stream frame encoding；
- `transport`：JSON-RPC envelope vocabulary 与 HTTP/SSE I/O。

`server`、`operation` 与 `dispatch` 有不同变化原因，不合并。HTTP 和 embedded 都从 operation 进入，任何 binding 都不能直接绕过 validation、capability、idempotency、problem projection 或 replay 规则。

Delivery 只依赖公共 Protocol、Application 和必要的 Domain projection values；不得导入 Agent Framework、Infra、具体 persistence、agentexec 或持有 Run lifecycle state。

### 6.6 Bootstrap、Config、Embedded 与 Cmd

私有 Runtime instance builder 是唯一组合根：创建 concrete dependency、组装 consumer port、取得数据目录独占权、执行恢复、启动有 owner 的后台任务并按逆序关闭资源。HTTP executable 和公共 `embedded.Open` 都只能调用它，不得各自复制装配图。

Bootstrap 不提供业务 API，不成为 service locator，不让运行时对象反向取得完整 Stack。公共 `embedded.Runtime` 只持有私有 instance 与 operation endpoint，提供完整 `Open/Close` 和类型化方法，不泄露内部资源。Config 只解析外部设置和完成静态验证，不执行业务选择。Cmd 只负责进程参数、BuildID、信号、HTTP listener 与 Runtime 生命周期。

### 6.7 共享原语

目标结构不保留可无限收纳的 `component`、`common`、`core` 或 `utils` 杂物层。

- 单一 owner 的原语移动到 owner package；
- 多个平级消费者确实共享且无领域语义的原语，可以作为准确命名的 `internal/<capability>` package；
- Delivery/Bootstrap 对 Application 值或机制的引用不产生新的所有权；行为在哪一环被决定，package 就归哪一环；
- 迁移只为切断依赖或表达真实所有权，不为目录美观制造 package。

## 7. Agent Framework 防腐层

### 7.1 唯一 import island

生产代码中，Agent Framework import 应收敛到 `adapter/agentexec` 及其明确的 Framework-integration 叶子。Domain、Application、Delivery、Infra 和通用 Toolset 不得导入 Agent Framework。

如果某个 Tool 必须使用 Interaction-only context，例如 deferred tool advertisement，它属于 Agent integration，应通过 agentexec decorator 或消费方注入的精确回调连接；不能让整个 Toolset 成为第二个 Framework adapter。

### 7.2 Root chat

普通根聊天直接部署 `interaction.Definition`。不得再用 GOAP 单 Action 包裹完整 Interaction；没有 world state、预测 Action 和 replan 语义时，不得引入 Planning。

每棵 root Run tree 使用独立 Engine、精确 root/child Deployment 集和 caller-owned resolver。当前迁移不需要 Agent Framework Platform；只有产品出现真实的多 Deployment 发布、版本选择和治理用例时才引入。

### 7.3 生命周期映射

```text
Application StartRun
  -> Application reads Host Conversation and appends current user message
  -> agentexec stages exact Interaction deployment + per-root Engine (no model/tool I/O)
  -> Application attaches executor observation
  -> Application commits Run + initial Segment as Opening
  -> Agent Framework Engine.Start invokes ProcessAdmitter after durable Opening
  -> ProcessAdmitter binds prospective root member
  -> Agent Framework constructs, registers, and publishes root Process
  -> authoritative model/tool projection
  -> agentexec awaits Result and translates final Output/Termination to application facts
  -> Application commits RunEvent
  -> Delivery projects protocol output
```

P8 已把 P4–P7 验证的 stage/observe/commit/begin、authoritative model/tool、Runtime Toolset、live unknown 与 waiting/restore/answer/steer 路径切为生产唯一 owner。Framework Event/Delta 仍只能作为 wake/best-effort observation；model/tool durable truth 来自同步 Application commit receipt，terminal truth来自 `Process.Await` 或 Application 已先提交的产品终态。

普通 model/tool Effect 的提交协议是：

1. agentexec outer Dispatcher 为一个 Effect 建立 context-scoped attempt tracker；model/tool decorator 通过 Agent Framework invocation context 验证同一 Effect、Process、model-call sequence 与 Tool-call index；
2. 外部调用前，decorator 把 started fact 送入该 Run 已有的 executor stream并等待 commit receipt；Application Run pump 使用 speculative reducer 计算完整 write-set，成功提交后才允许跨过外部边界；
3. model final、usage、pricing 与 Run progress 同事务提交；Tool result、presentation、mutation fact 与 invocation terminal 同事务提交。任何 post-call authoritative commit 失败都会让整个 Effect unknown，不能降级为普通 provider/Tool error；
4. model/tool invocation journal 只保存 operational attempt boundary，不复制 semantic final/result。Transcript 只保存用户可观察的完整 final 与 Tool Item；二者在同一个 Application write-set 中结算；
5. 一个并发 Tool batch 可以乱序完成，但 start 不占 Transcript insertion order。Run pump 暂存完成结果，只提交模型声明顺序形成的最长连续前缀；一个前缀批次失败时所有 receipt 一起失败，speculative result 全部丢弃，started calls 最终成为 incomplete，不产生部分成功历史；
6. Delta 有界且 best-effort，drop 通过 Framework usage/event 或 Runtime OTel event 可观察；完整 final/usage 从独立 authoritative path 导出。可重新读取的 product projection 与 post-hook 也是 observation，失败不能把已经确定的 Tool settlement 改成 unknown。

发现 unresolved unknown Effect 后，Application 在 release 前原子提交 incomplete diagnostic 与 `RunLost`；事务失败保持 Process 阻塞并重试。Dispatcher wake 只是低延迟路径，per-Run reconciliation 仍周期查询 public `UnknownEffectIDs`，因此 listener/wake 丢失不能留下永久挂起。

Waiting：

```text
Agent Framework pending input + quiescent tree
  -> agentexec reads public Interaction helper
  -> agentexec captures opaque TreeSnapshot
  -> Application commits Run/Pending/Checkpoint write-set
  -> committed Interrupt becomes externally visible
```

Resume：

```text
Application transaction records the exact answer claim
  + marks the interrupt row resuming/invisible
  + invalidates the old checkpoint
  -> agentexec stages the exact live waiting tree, or rebuilds exact Deployment and Engine.RestoreTree
  -> Application transaction proves the durable claim and opens the next Segment
  -> semantic answer becomes one WaitID-addressed Interaction Signal
  -> Process resumes
```

answer claim 是不可逆的恢复线性化点，不是普通“读取并删除”。claim 成功后，旧 checkpoint 已不存在，`resuming` row 只保留答案审计与 crash diagnosis，普通 waiting 查询看不到它；continuation opening 必须在同一事务中先证明该 claim 存在，再把 Run tree 改回 Running。下一次 quiescent barrier 只能由相同 Session/executor/root-member owner 原子替换该 row，terminal/recovery 则原子删除。claim 后任何校验、restore、observe 或 opening 失败都先提交根 Run `RunLost`，成功后才 release live tree；若 `RunLost` 自身无法持久化，tree 与 hidden claim 都保持，不伪造已清理状态。进程在 claim 后、RestoreTree 后或 Signal accepted 后到下一 checkpoint 前崩溃，boot recovery 一律 `RunLost`，绝不再次投递旧答案。

Interaction 只通过 public pending-input helper 读取 prompt/WaitID，通过 public `TreeSnapshot`/`RestoreTree` 恢复，通过 typed answer/steer constructor 产生 Signal。产品 Interrupt 与 response 的 strict codec 位于独立防腐包；旧 private suspension adapter 已删除。Ask-user 使用真实 Runtime Tool 注入 pending-input capability；interactive approval 的 plan/hook 只在首次调用执行，restore 只解析持久 prompt 并应用答案。deferred advertisement 属于 Interaction snapshot，恢复后无需 Runtime 重建影子清单。

### 7.4 Child Process 与 child Run

Managed Delegate 的 child Process 是 first-class child Run 的来源。模型响应中的 Delegate ToolCall 必须先 durable commit；Agent Framework `ProcessAdmitter` 随后在 Process 发布前调用 Application-owned admission use case，用 prospective identity 和同一 Delegate child key 原子创建不可见的 child opening reservation、产品 child Run identity 和父因果 binding。`EventProcessStarted` 只作为对账唤醒；agentexec 验证 live Process/关系后产生 executor fact，Application 提交后才把 child Run 公开为 Running。

Admission 成功不等于 Process 已启动。root 的 `Engine.Start` 失败由直接调用者把已存在 Opening Run 终结为 start failure；进程崩溃后，任何没有 checkpoint/started fact 的 Opening root 都按 recovery loss 收口。Framework-neutral `ProcessStartOutcomeAcknowledger` 提供 prospective identity 对应的 conclusive started/aborted outcome：started 后才公开 child Run，aborted 只闭合不可见 reservation。Runtime 不用超时、私有 ID 算法、Event 顺序或父 Effect payload 猜测结果。

不是所有 Framework child 都自动成为产品 Run。Planning/Workflow 的内部 child 是否投影为产品 Run，由 agentexec 根据 exact Deployment 和组合语义决定；不得把产品 `Run` 标记塞入 Agent Framework ProcessRelation 或 ChildSpec。

Delegate 因果映射使用 Interaction 提供的稳定 child key；Runtime 不自行重算另一套 identity。

### 7.5 Waiting subtree cancellation

取消等待 child 必须使用一个 Framework-owned、one-shot prepared tree change：

1. agentexec 在完整 tree safe boundary 取得 prepared change；该 capability 在 `Apply` 或 `Discard` 前冻结 source tree，但不暴露 Agent Framework plan、lock 或 snapshot type；
2. agentexec 把 canceled/paused executor member、resulting opaque checkpoint 投影为 Application 值，concrete capability 只在当前 use case 内存活；
3. Application 提交 Run/Transcript/Pending/resulting checkpoint write-set；transaction 失败时调用 `Discard`，live tree 保持 source state；
4. durable commit 成功后调用 contextless `Apply`，把仍被冻结的 live tree 线性化到已持久化的 resulting state；若该变换移除了最后一个外部边界，已提交 Segment 的 activation 再单独调用 `Continue(ctx)`，不能把执行启动失败混称为 apply failure；
5. 进程在 commit 后、Apply 前崩溃时，重启直接恢复已提交 resulting checkpoint；Agent Framework 的 contextless `Apply()` 不允许请求取消撤销已提交决定。若内部不变量仍使 Apply 无法证明成功，Application 释放旧 owner并从 resulting checkpoint 精确恢复；只有精确恢复失败才提交 `RunLost`。

prepared change 必须 one-shot、`Discard` 幂等且有 Host-owned preparation deadline；agentexec 取得后立即注册 `defer Discard`。deadline 只允许在 application transaction 提交前释放冻结边界，提交后的 contextless Apply 不再受请求生命周期控制。Framework 不允许因调用方遗漏或 transaction 卡住而无限冻结 tree。

P7 已由真实 Runtime consumer 把 Agent Framework 收敛为中性 `PreparedWaitingSubtreeCancellation`：Prepare 在返回前完成全部可失败 staging并持续冻结 source tree，Application 不持久化 Framework capability、不复刻 source digest，也没有重新引入通用 Mutation lease。Runtime execution ACL 进一步把 resulting state 的 `Apply` 与 final-boundary `Continue` 分成两个准确阶段；只有后者推进 Process。`WaitingExecutionRestorer` 只消费 Application `WaitingContinuation` 与 opaque checkpoint，不解析 Agent Framework tree wire。

### 7.6 终态翻译

Run 终态不能由 `error` 文本或 `context.Canceled` 猜测。agentexec 同时读取 Application 已记录的控制意图和 Agent Framework `Termination`，再产生应用事实：

| Agent Framework/Host 事实 | Runtime 产品结果 |
|---|---|
| `TerminationCauseCompletion` | `Completed` |
| Host/parent cancellation | `Canceled`，保留取消 owner/reason |
| Process/parent/Host deadline | 独立 `TimedOut` outcome，不能压成普通 error |
| execution/contract/external/panic failure | `Failed` + Runtime-owned structured failure category/code |
| Interaction/Runtime 已知 step/model-call limit | 精确的 `MaxSteps`/对应 limit outcome |
| Runtime 产品 token/USD budget 触发的受控停止 | `MaxBudget` |
| Application 已提交终态后的 executor teardown kill | 不产生第二个 Run outcome |
| 活跃 Run 收到无法对应 Application intent 的 Engine kill | fail closed 为 internal failure，并保留诊断事实 |
| checkpoint 缺失/损坏、BuildID 或外部 world 不可恢复 | Application `RunLost` recovery outcome，不伪装成 Framework failure |

同一 Agent Framework cause 在 root/child 和不同 Application intent 下可以映射为不同产品细因，因此映射属于 agentexec + Application 合作边界，不下沉 Agent Framework，也不散落在 Delivery。

### 7.7 Unknown Effect settlement

Interaction 的模型和普通 Tool Effect 默认不能证明未知外部结果可以安全重放。当前 Runtime 不向用户暴露通用 `ResolveEffect`，也不伪造 succeeded/failed owner payload。

live 和 recovery 使用同一 fail-closed policy。outer Dispatcher 在返回 indeterminate error 前先标记 per-tree reconciliation state并唤醒 Run pump；pump 通过每个已知 Process 的公共 `UnknownEffectIDs` 对账，Event listener 丢失时由有界 reconciliation tick 收敛。发现 unknown 后，Application 原子提交 started/incomplete Transcript 诊断、`RunLost` outcome、checkpoint invalidation 和 cleanup intent；事务失败时保持 live Process 阻塞并重试，不能先 kill。提交成功后 agentexec 对 tree 发出明确 Kill 并 release。

若 unknown 与用户 cancel/deadline 竞争，只要产品 terminal 尚未提交，外部结果不确定性优先映射为 `RunLost`，cancel intent 作为诊断保留；已经提交的产品 terminal 仍遵守 first-terminal-wins。恢复探测发现 unresolved unknown Effect 时同样把 checkpoint 判为不可自动恢复并执行这一收口。未来若产品需要人工裁决未知副作用，必须先由具体 Strategy owner 提供可构造、可验证的 typed resolution contract，再单独增加产品用例；不能把 arbitrary Settlement payload 暴露到协议。

## 8. Application execution port

Application port 只表达产品真正需要的执行能力，不能镜像 Agent Framework Engine 的全部方法。

目标能力包括：

- 校验并启动一个 Run execution；
- 订阅应用侧 executor facts；
- 捕获不透明 checkpoint；
- 从 checkpoint 恢复；
- 提交语义 interrupt answer；
- steer 当前 Run；
- 取消完整 execution 或一个已知 executor member subtree；
- 规划并在应用事务后应用 waiting subtree cancellation；
- 释放终态或失效的 executor tree。

P8 已按 consumer-discovered interface 原则冻结完整生产端口，不兼容旧 `ExecutionControl`、`PrepareStart`、`Activate`、`Prepare` 或 `TurnProcess`。`RootExecutionStarter` 用 validate/stage/begin 准确表达 admission 两侧，`ExecutionObserver` 只观察 Application facts，`RunningRootCancellationRequester` 只请求安全停止，`ExecutionReleaser` 只释放资源；`WaitingExecutionContinuer`、`WaitingExecutionRestorer`、`RunningExecutionSteerer` 与 waiting-subtree capability 分别表达 continuation、exact restore、semantic steer 和跨事务 one-shot tree change。命名以 Run 用例语义为准，不用含糊的 `Handle`、`Manager` 掩盖阶段。

## 9. 工具能力

Runtime Toolset 负责产品工具清单、schema、执行 capability、安全分类、展示语义和应用端口装配；`toolset.Manifest` 是 visible/deferred authority 的唯一 Runtime 值，现有 `toolset.Resolver` 直接满足 agentexec 消费端口，不再复制 Interaction 专用 Manifest。Agent Framework Interaction 负责冻结当次 Process 可执行的 Tools/DeferredTools、模型 Tool loop、checkpoint 和 advertisement 状态。agentexec 在 manifest resolution 与实际 Tool invocation 上绑定同一不可变 execution scope，Toolset 仍对 Agent Framework 零 import。

边界规则：

- Tool name、description、参数名和 schema 只有一份权威定义；
- schema 与执行使用同一 strict typed decode 路径；
- Runtime policy 不进入通用 `tool.Tool`；
- Tool 不取得 Engine、Process 或完整 Runtime Stack；
- `search_tools` 只广告已冻结、已授权的 deferred tool，不动态增加执行权限；
- Tool activity 优先复用领域字段，不建立通用 UI metadata 袋；
- Plan、Goal、Schedule、Skill Proposal 保持不同领域，不合并为一个通用 task 系统。

现有工具面和历史裁决继续由 [`TOOL_SYSTEM.md`](TOOL_SYSTEM.md) 拥有；本文件只定义它与 Agent execution 的边界。

## 10. 持久化与恢复

- 当前开发阶段只有一个 SQLite shape；schema 变化直接提升 epoch，不双读、不迁移旧 shape；
- Store 保存 Application aggregate，不保存 live Engine、Dispatcher、context、goroutine 或 SDK object；
- root Process tree 是 executor snapshot 的不可拆分恢复单位；
- checkpoint 写入和产品 waiting facts 必须属于同一个 Application write-set；
- terminalization 与 checkpoint deletion 必须按应用不变量原子提交；
- isolated workspace 在进程重启后不可假装恢复，按产品 policy fail closed；
- BuildID、workspace revision 等外部事实由 Host 校验，不能污染 Agent Framework snapshot；
- 初版 Runtime 不配置 Agent Framework `PreparedStepAcknowledger`。它当前只提供单 Process `Snapshot`，不能形成可恢复的完整 `TreeSnapshot`；Runtime 只承诺恢复最后一个已提交的 quiescent tree checkpoint，active tree 在进程崩溃且没有此边界时按 `RunLost` 收口；
- 将来若要求 pre-dispatch crash durability，必须先由 Agent Framework 提供中性的 tree-wide durability contract，再由真实产品需求启用；Runtime 不拼装 private tree wire，也不把 Application Store 注入 Framework；
- Agent Framework payload baseline 是 public TreeSnapshot v4 JSON；Runtime 不再包一层自创 payload schema。Host envelope 由 exact BuildID、root member、scope、model、limits、usage 和 Agent Framework DeploymentRef 校验共同冻结；旧/损坏 TreeSnapshot 由 public parser 严格拒绝；
- unknown external Effect 不自动重放、不猜测结算。Framework unknown 状态无法通过 quiescent `CaptureTree` 形成 committed recovery point；restore 仍防御性查询 public `UnknownEffectIDs` 并拒绝异常 payload。

## 11. 并发和生命周期

- 每个 goroutine 必须有 owner、停止条件和 join 路径；
- Application owns Run pump、Goal loop、Schedule loop 和维护任务的生命周期；
- Agent Framework owns Process/Effect/child loop；
- agentexec 可以为一棵 live root tree 保存准确的 Engine、Process handles 和 admission reconciliation state；它们由一个 tree session 拥有并在终态/release 时整体销毁，不形成跨 Run 的全局 lifecycle registry；
- Transport owns connection/request goroutine，不拥有 Run；
- fan-out 必须有显式上限；
- Delegate/Planning/Workflow 每个 Framework child 都是真 Process；并发 child、单父 child、tree process 总量和产品 child Run fan-out 必须同时受配置上限约束，不提供无界 `Map`/N-way fork；
- 慢 Delivery subscriber 不得阻塞或改变 Run 终态；
- cancel、terminal、waiting 和 resume 的线性化点必须有行为测试；
- Engine 使用 Run-owned lifecycle context，不使用短命 Delivery request context；请求返回或连接断开不得隐式取消仍在执行的 Run；
- 不使用 `time.Sleep` 证明并发正确性；使用 channel、明确 barrier 或 `testing/synctest`。

Agent Framework Event/Delta listener 都是 observation/wake-up 边界，不是 Application durable commit callback。listener failure/panic 不会回滚 Framework 状态，因此 Application 不能仅因“收到一个 Event”就假定产品事实已经可靠提交。per-Run pump 必须通过 Agent Framework public Process status/result、Strategy public helper 和 quiescent complete-tree capture 对账 waiting/terminal；Event 丢失后仍能在下一次 wake、control 或 recovery reconciliation 收敛。terminal truth 最终来自 `Process.Await`/`Result`，不是 Event payload。

## 12. 协议与 binding

Runtime Protocol 是外部语义契约，机器真相源在 `contract/`。重构可以 breaking，但一次变更只能有一个新 shape：

- 不保留旧字段 alias、双 method、双 event 或双 artifact reader；
- server-side phase 可以先更新 Go contract 和生成物，前端/CLI/TUI 在独立 consumer phase 接线；
- API semantic、transport binding 和 auxiliary capability 分别由现有三份规范拥有；
- HTTP status 只表示 transport，业务失败使用协议 error；
- transport metadata 不进入 JSON-RPC body；
- HTTP/SSE 与 embedded 都驱动同一 binding-neutral operation 和 Application use case；binding 只投影 metadata、options 与结果流，不得复制业务路径。
- embedded 不经过 JSON-RPC/SSE 编解码，但必须遵守同一严格验证、capability、idempotency、replay cursor 与 problem 语义。
- 同一 data directory 只有一个 Runtime owner；独占锁早于 store/recovery，成功关闭时最后释放。

## 13. 目标目录

```text
app/runtime/
├── protocol/
├── embedded/
├── cmd/
├── contract/
├── doc/
└── internal/
    ├── config/
    ├── domain/
    │   ├── run/
    │   ├── session/
    │   ├── conversation/
    │   ├── transcript/
    │   ├── interrupt/
    │   ├── accounting/
    │   └── <其他真实 bounded context>/
    ├── application/
    │   └── <use-case owned packages>/
    ├── adapter/
    │   ├── agentexec/
    │   ├── toolset/
    │   ├── persistence/
    │   └── <capability adapters>/
    ├── infra/
    │   └── <technical mechanisms>/
    ├── delivery/
    │   ├── operation/
    │   ├── server/
    │   ├── dispatch/
    │   └── transport/
    ├── bootstrap/
    ├── config/
    └── arch/
```

这是一张所有权图，不是要求一次性制造所有目录。只有在迁移完成后仍有真实代码的 package 才能存在；不得保留空目录、旧路径 forwarding package 或 `v2` 后缀。

## 14. 完成定义

Runtime 重构只有同时满足以下条件才算完成：

- `domain/execution` 已被准确的 `domain/run` 及独立上下文取代；
- Domain、Application、Delivery、Infra 和通用 Toolset 对 Agent Framework 零 import；
- `adapter/agentexec` 使用 Interaction，旧 GOAP chat wrapper 和 Turn lifecycle 全部删除；
- Runtime 没有第二 Process loop、Tool loop、tree scheduler 或 snapshot parser；
- child admission、waiting/resume/steer、subtree cancellation 和 authoritative observation 有真实纵切测试；
- Conversation、Transcript、WorkingContext 和 checkpoint 不互为隐式真相源；
- Application 事务与 Agent Framework plan/apply、durability boundary 的顺序有崩溃点测试；
- Delivery 只驱动 Application，transport 对业务零感知；
- Adapter/Infra 无纯转发层，`component` 杂物分类清零；
- 协议、schema epoch、artifact 和生成物只有当前唯一 shape；
- 原框架 import 与 module 已归零，绿色重写实现是唯一 `agent`；
- architecture tests、build、vet、staticcheck、普通测试、race 和相关 fuzz 全绿；
- 当前能力事实、API/contract baseline 和实施进度同步更新，无漂移注释或历史 TODO。

## 15. 明确不做

- 不创建完整 `runtime2` 或长期双 Runtime；
- 不为旧内部 API、旧 Agent、旧 wire 或旧数据库 shape 建兼容层；
- 不把 Runtime 的 transaction、approval、pricing、history、workspace 或 Run 类型下沉到 Agent Framework；
- 不把 Agent Framework Process、Signal、Effect、Strategy payload 暴露到产品协议；
- 不建立通用 EventBus、Mediator、Repository base、UnitOfWork、Plugin registry 或 dependency scope；
- 不把 Platform 当成迁移前置条件；
- 不在这轮服务端重构中顺手重写前端、TUI 或 CLI；它们在协议完成后专项接线；
- 不为目录对称、未来猜测或设计模式本身制造抽象。
