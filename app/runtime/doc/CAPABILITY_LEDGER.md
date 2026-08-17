# Lyra Runtime 能力迁移台账

> 状态：当前能力事实，随每个实施批次更新
>
> 基线日期：2026-08-17

本文记录当前能力事实、目标 owner、迁移 verdict、实施阶段和验收证据。它不重复目标架构和 ADR。代码变化后必须在同一批更新对应条目；不能用“计划保留”冒充“已经迁移”。

## 1. Verdict

| Verdict | 含义 |
|---|---|
| `Retain` | 所有权和抽象正确，保留实现，只做必要接线/命名同步 |
| `Refactor` | 产品语义保留，但 API、package、职责或依赖需要治本调整 |
| `Rewrite` | 能力保留，但当前实现建立在旧执行模型上，按新合同从零实现 |
| `Remove` | 能力重复、owner 错误或已经由 Agent Framework 拥有，完成阶段必须删除 |
| `Defer` | 不是当前服务端重构前置条件，有真实需求后单独设计 |

## 2. 当前基线事实

### 2.1 规模与依赖

- Runtime 源码、测试、`go.mod` 与 `go.sum` 对旧 `github.com/Tangerg/lynx/agent` 的依赖均为零；architecture guard 将其作为永久禁止边，而非迁移数量台账；
- `go.mod` 直接绑定已发布 Agent Framework Baseline 20 pseudo-version `v0.0.0-20260811152247-8e667d716b22`；workspace 与 `GOWORK=off` 消费同一 canonical source，standalone tidy/build/vet/test/race/staticcheck/lint 全绿且没有 `replace` 或 Runtime sanitizer；
- Agent Framework production import 只允许位于 `adapter/agentexec`；Domain、Application、Infra、Delivery、Bootstrap 与通用 Toolset 对 Agent Framework concrete types 为零；
- P2 已删除 `domain/execution` 及全部 forwarding/alias path；Domain 生产代码与测试对 Application/Adapter/Infra/Delivery/Bootstrap 零 import，context-based I/O port 为零；
- Run、Accounting、Conversation、Transcript、Interrupt、Tool 与 ToolResult 已成为准确顶层 bounded-context package；P16-01 已由 `run.Run` 接管完整 Run aggregate，Transcript 不再承载第二 Run lifecycle，Run/Tool failure taxonomy 按 owner 分离；executor checkpoint/ref、pending continuation 与 workspace mutation 由 Application consumer 拥有；
- P3 已删除 Application 的 `ExecutionControl`、`SegmentExecutor`、`SessionLifecycle` 与 `Effects` 胖接口；root start/observe/release、Session reads/termination 与 Run projection write-sets 均由真实 consumer-owned ports 表达；
- Application executor tree identity 已统一为 `ExecutorMember`/`MemberID`；Framework `ProcessID` 只存在于 `adapter/agentexec` 内部映射，SQLite technical shape 使用 `root_member_id`/`memberId`；
- 同一 Interaction tree 已在生产 Bootstrap 接通 conclusive child start、durable Delegate child Run attribution、nested/sibling child reconciliation，以及 one-shot prepared waiting-subtree cancellation；
- SQLite 当前唯一 shape 为 epoch 75：durable replay store 拥有独立 opaque idempotency namespace；Run 行保存 latest Application-owned command identity，统一证明 fresh/resume opening、Model/Tool/Transcript/Conversation/Progress、HITL tree barrier、waiting-child cancellation 与 terminal 不明 COMMIT 的精确 write-set；model/tool invocation operational journal 与 Transcript semantic final 分离，interrupt row 具有 `open`/`resuming` answer-claim 状态，accepted Question answer 与 claim/checkpoint 更新同事务提交，approval input-request binding 保留 exact Tool call identity，child-start reservation payload 由 `runsegment` 显式编码；Goal/Run provenance 使用目标 incarnation，Goal 同时冻结 canonical Run capabilities；Transcript/committed-tool failure payload 使用准确的 `failure` vocabulary，并区分 owning Run 取消、执行失败、审批拒绝与父 Run 上的 child Run 取消；Run pump 是唯一 reducer/persistence writer；
- Agent Framework production dependency 只存在于执行防腐层；Runtime 其他 ring 对 Framework concrete type 为零。
- P114-01a 已关闭首个 renderer generation 缺口：composition root 的 `desktopBootstrap` 原为模块级可变状态，旧 renderer bootstrap 可在 `resetContainer` 已发布 successor 且 successor 已恢复新 local token 后迟到结算，把新 client 的 Runtime identity 覆盖回旧 token；窗口在 bootstrap/window-chrome await 期间 final close 时，旧 `main.tsx` 还会继续安装 watcher 和 React root。bootstrap identity 现归 exact container owner，replace/close 同步撤销旧代提交权；充血 `DesktopRenderer` 统一拥有 bootstrap→chrome→watcher→React root→Runtime close 状态机，final close 不可逆且重复 dispose 共享唯一 settlement。没有 callback 延时、reload workaround 或第二 lifecycle path；
- P114-01b 已关闭 Plugin Host generation 之间的 process-local mutation/stream identity 泄漏：Session create、Goal command 与 Run opening 不再共享模块级 settler，每次安装构造 exact gateway owner并在 dispose 时撤销 attempt、retained identity 与已接受 Run stream；successor 只能通过自己的 live client 打开命令，跨 renderer/client/cold-start handoff 仍由 durable mutation journal 的 immutable generation/fence 独占，旧 `MutationPromise` closure 不会进入新代。Codex Rust 的 connection RPC gate、thread listener generation 与 listener-serialized resume snapshot/tail ordering被吸收为 owner不变量；其多连接 subscription/idle unload形状没有进入单 client/server 产品；
- P114-01c 已关闭 Runtime Plugin Host replacement/final close 对 connection read model 的跨代清空与read-port空窗：service observation 与 negotiated capabilities 不再由两个全局 adapter独立写入，`RuntimeConnectionOwner` 同时拥有 inspection controller、polling/timeout、read ports与原子 projection；replacement两阶段交接先安装successor port并转移current generation，再退役旧controller/port并发布新代初态；final dispose先撤writer、在retiring read port仍有效时发布空投影完成同步reconcile，最后撤port，因此capability subscriber始终有可读owner。旧 inspection迟到、旧 polling callback与旧 disposer均不能提交或清理current state。capability Application port已breaking收窄为只读，两个分裂store adapter已物理删除；Codex Rust gate-first close与exact listener generation cleanup只作为owner不变量吸收，没有引入其多连接产品形状；
- P114-01d 已关闭 HITL atomic response barrier 的 Plugin Host generation 泄漏：approval/question staged choices、submitting latch、session projection subscription、continuation callbacks与retirement现在由每次Agent bootstrap安装的同一个`InterruptResponseCoordinator`实例拥有；successor安装先退役旧实例并回滚其本地pending hooks，旧disposer和迟到accept/reject只访问旧实例，不能清successor同键barrier或提交旧选择。键盘审批不再维护第二套模块级`inFlight`集合，所有card/shortcut去重与settlement服从同一coordinator。Codex Rust listener generation与pending server-request callback take/cancel语义被吸收为exact-owner不变量，不复制其多connection replay形状；
- P114-01e 已关闭 Agent query/material writer 的 Plugin Host generation 泄漏：旧 `AgentSessionViewPort` 对象即使在 successor state ports 安装后仍可被在途 refresh closure 调用，原有 request sequence 与 view revision 只能排列同一 writer 内的请求，不能撤销旧 writer 对共享 Agent/Plan/Goal material 的提交权。现在每次 state-port 安装先声明一个 exact `AgentViewRefreshOwner`，再发布 ports；successor 声明同步撤销 predecessor，旧 snapshot 的 begin/commit、伴随 material writer 与旧 disposer 均只作用于自己的 owner，不能提交或退役 current generation。Codex Rust tool metadata cache 的 generation-before-commit 规则被吸收为 writer 不变量，不复制其 cache 或多 client 形状；
- P114-01f 已关闭 Session/approval command continuation 的 Plugin Host generation 泄漏：create/fork single-flight、rollback exclusion、Session summary revision queue 与 approval mode queue 不再是 Application 模块级进程单例，await 后也不再重新读取 successor gateway/state/view。每次 Runtime gateway 安装先声明 exact `AgentCommandOwner`，该充血 owner 统一拥有本代 single-flight、rollback lease、summary CAS 串行化、approval 串行化与 optimistic query/steer effect；successor 同步退役 predecessor 并补偿其本地 effect，旧 response、旧 queued task与stale disposer不能发命令、导航或写 current cache。原 summary settlement 转发薄壳已删除。Codex Rust RPC pending/disconnect 原子注册与 metadata pending-generation exact apply 被吸收为 command owner 不变量，不复制其多连接协议形状；
- P114-01g 已关闭 mounted Session usage query writer 的 Plugin Host generation 泄漏：TanStack Query 原本在 Runtime gateway replacement 后继续把旧 fetch 视为当前 cache writer，旧 response 可在 successor 已安装后提交旧 usage，successor 也不会立即发起权威 read。现在 exact `AgentSessionUsageOwner` 同时拥有本代 gateway、lifetime cancellation 与 product singleton QueryClient handoff；successor 声明先同步退役/abort predecessor，再接管唯一 query writer，非协作旧 transport 的迟到返回还须通过 exact-owner commit 检查，stale disposer 不能取消 successor fetch。测试与产品坚持一个 QueryClient，没有为第二 cache 增加兼容路径。Codex Rust resume picker 的 exact request token completion 被吸收为 writer settlement 不变量，不复制其分页 UI 状态机；
- P114-01h 已关闭通用 `DATA_PROVIDER` query/cache writer 的 Plugin Host generation 泄漏：queryFn 不再在执行时动态查找当前 provider，而由充血 `DataQueryOwner` 持有 exact Host、resolved provider map、generation lifetime 与 product singleton QueryClient handoff。Host replacement 会同步退役旧代并交接全部 provider key；同一 Host 内只交接实际新增、删除或 identity 变化的 key。每次交接在当下捕获 exact predecessor Query 对象，异步 cancel/reset/remove 不会误伤随后才创建的同 key successor writer；旧 signal 与非协作迟到结果均须服从 generation retirement/current 检查，stale Host stop 不能清 successor cache，真实 renderer→Host final close 会物理移除 retired cache entry。query key 与公共 SDK shape 保持唯一，没有全量刷新、第二 QueryClient 或延时掩盖。Codex Rust search cancellation 的 `Arc::ptr_eq` cleanup 与 TUI picker 的 `(root ThreadId, request UUID)` completion 被吸收为 exact current identity 不变量，不复制其多连接或 TUI 状态形状；
- P114-01i 已关闭默认 DATA_PROVIDER 单次 read 内的 Runtime client 跨代拼接：`models` 原来在 `providers.list` await 前后分别动态读取 container，旧代第一阶段响应可把第二阶段 `models.list` 发到 successor transport。现在同文件充血 `RuntimeProviderRead` 在每次 fetch admission 一次捕获 exact `LyraClient` 与 generation signal，25 个默认 provider 的 workspace、paging 和多阶段 RPC 全部消费同一 owner；下一次独立 fetch 重新 admission 并使用 successor client，因此没有安装期 client 钉死、公共 helper 微模块或单点特例。其余 Goal/Pending/MCP DATA_PROVIDER 已审计为每次 fetch 只有一次 client read，不存在同类 await 后重取。Codex Rust app-server `model_list` 在 request admission 时一次 clone `Arc<ThreadManager>` 与 HTTP factory并贯穿完整 async call chain的做法被吸收为 dependency snapshot 不变量，不复制其服务端 processor 结构；
- P114-01j 已关闭 Session usage cache handoff 的 future-query 误伤：gateway 安装时即使没有 predecessor usage Query，原异步 `cancelQueries().then(invalidateQueries)` 仍会在 settlement 时按 key 重新解析 cache，把交接后才挂载且已成功的 successor Query 错当旧 writer并产生第二次 RPC。现在 `AgentSessionUsageOwner` 在交接/终止边界同步捕获 exact TanStack `Query` 对象集合；空集合没有异步尾巴，非空集合的 cancel/reset 只接受该 identity set，未来同 key Query、stale disposer 与随后 owner 均不受影响，旧 material 也不通过 invalidate 暂留。Codex Rust `client_recovery` 与 `local_process` 只允许 exact `Arc` identity 触发 recovery 或完成 `Starting` replacement 的规则被吸收为 settlement capability，不复制其多连接、多进程表结构；
- P114-02a 已建立 Runtime process incarnation 的公开唯一 identity：Bootstrap 每次打开进程生成新鲜 opaque `instanceId`，同一个值由 `runtime.discover.serverInfo`、`/v2/info.server`、liveness 与 readiness 同源发布；它与跨重启稳定的 SQLite idempotency namespace、instance-local replay scope 严格分离。Desktop inspection 只有四路 identity 完全一致才原子提交 generation/service/capabilities，拼接两个进程的 cohort 整体失败；connection projection 以同一 state transaction 拥有 generation 与 capabilities，Workspace 事件流由充血 `RuntimeEventLoopOwner` 按 exact generation 交接，因此同代健康轮询不重启，ready→ready 同 endpoint 进程替换会同步撤销旧 stream 并立即启动 successor。Codex Rust exec-server 将 durable session identity 与每次 attach 新建的 `ConnectionId` 分离、detach/expire 只接受 exact connection owner 的不变量被吸收；其多连接 session registry 没有进入 Lyra 单 client/server 产品；
- P114-02b 已关闭 Runtime process identity 发布与 successor event tail 建立之间的 read-writer 空窗：旧实现直到新 `runtime.subscribe` opening 成功后的 `invalidateAll` 才同时取消旧 query/snapshot 并启动权威重读，因此旧 Runtime 已受理的 `sessions.snapshot`、Goal/provider query 与 rAF stream batch 可在 successor generation 已发布后短暂提交。现在充血 `RuntimeEventLoopOwner` 独立观察 connection generation 与 stream capability；每次真实 generation 变化先同步执行 retire phase，再 abort predecessor/start successor tail。retire phase 只撤销提交权而不读数据：TanStack writer 被取消，mounted Agent coordinator 终止 active/queued snapshot、Run opening/stream，material refresh sequence 与 view epoch 同步前移，使非协作迟到 snapshot、associated Goal material 和已排队 stream callback 全部失效；已提交 material 保持可见。只有 successor tail 成功建立后，既有 `invalidateAll` 才执行 replace phase 并从同一 SQLite snapshot 重建 Agent/HITL/Plan/Goal/Run/Tool 与其余 cache，因此同时满足“旧代永不提交”和 snapshot→tail 无事件缺口。Codex Rust app-server 把 running-thread resume snapshot、connection subscription 与后续通知串行放入同一个 thread listener，并用 exact `listener_generation` 限制旧 listener cleanup；本批吸收 serialized snapshot/tail handoff 与 exact retirement，不复制其多 connection、per-thread registry、server-request replay 或 unload 产品形状；
- P114-04a 已关闭流式 transcript 尾部终态动作的 owner 错位：点赞/点踩、复制与其他 action row 不再仅凭 current-root attention 推断消息已经完成；message presentation 从 exact `TranscriptRow` 的 block、ToolCall 与 delegated Run 聚合唯一 `active/settled` materialization，message-actions policy 在 active 期间返回 `absent`，Slot 不挂载也不进入布局，Runtime waiting/断线交接的全局状态不能让未完成尾消息闪现终态按钮。恢复后的 `incomplete` 是可审计的 settled material，HITL `requires-action` 与未完成 Tool/subagent 仍为 active；settled row 继续使用原有 in-flow hover/pinned 规则。Codex UI 的 item-local `completed → isStreaming/copy availability` 被吸收为准入心智模型，没有复制其大组件或多 surface analytics 形状。Runtime、Protocol、SQLite、Artifact、Desktop Agent inner ring 与 Go Agent Framework 合同均未变化；
- P114 参考事实已冻结：后端只读重点参考 `/Users/tangerg/Desktop/codex/codex-rs` 的恢复、事件代际、取消和 durable state owner；前端只参考 `/Users/tangerg/Desktop/study` 的 Codex（主）与 zcode/minimax（辅）UI。参考结论必须映射回 Lyra owner/lifecycle/transaction，不成为第二实现路径或外部 package layout 规范。

### 2.2 当前架构基础

| 能力 | 当前事实 | Verdict | 阶段 |
|---|---|---|---|
| Run lifecycle | `application/runs` 拥有 start、pump、waiting、resume、cancel、terminal ordering；child opening reservation、conclusive start 与 waiting-subtree transaction ordering 已在生产冻结 | Retain | P3–P8 已完成 |
| Session lifecycle | `domain/session.Session` 独占 construct/edit/fork/restore replacement、revision/time 与 generated-title first-writer；Application 独占 workspace/admission、Run opening 和跨聚合 lifecycle write-set；SQLite 只做 exact Insert/Save CAS | Retain | P2/P8；P16-04 authoritative aggregate |
| Domain framework isolation | P2 已删除十个 context-based Domain I/O port，生产与测试均由机器守卫禁止向外依赖 | Retain + strengthen | P2 已完成；例外为零 |
| Agent anti-corruption | Agent Framework tree、model/tool Effect、waiting/restore/steer、Delegate child 与 prepared subtree change 均由 `adapter/agentexec` 生产 owner 独占 | Retain | P8 已接管生产；旧 Framework lifecycle 已删除 |
| Delivery separation | target DAG 禁止 Delivery import 任意 concrete Adapter；protocol/dispatch/server/transport 已按准确职责收口 | Retain | P1/P9/P10 已完成 |
| Adapter/Infra direction | Adapter 单向使用 Infra；Infra 对 Application/Adapter/Delivery/Bootstrap 反向 import 为零 | Retain | P9 已完成 |
| Shared mechanisms | `component` umbrella 已删除；根级只保留三个真正跨环 capability，Application 共享机制归还 Application | Retain exact owners | P18 所有权纠偏，永久 purity/owner guard |
| Contract generation | `contract/` 的 manifest/OpenRPC/schema/TypeScript/Go validator 来自唯一 registry；public Go API baseline 来自真实 type information；canonical samples 同时经 round-trip 与 strict validator | Retain | P10/P19-06 已完成 |
| SQLite exact epoch | epoch 75 单一 shape，无生产 migration chain | Retain | P6/P8 完成；P15-03 收回 child-start durable wire owner；P16-02 收回 Tool failure vocabulary；P21 区分 owning Run 取消；P32 收敛 Goal incarnation provenance；P34 冻结 Goal capabilities 与 accepted Question answer；P74 增加 durable replay store identity；P93/P94 建立全部 EventCommit durable identity；P95/P96 扩展到 opening、tree barrier 与 waiting-child composite command boundary；P107 固化 approval Tool call replay identity |

### 2.3 P19 公共 Go binding 事实

| 能力 | 当前事实 | Verdict | 阶段 |
|---|---|---|---|
| 公共 protocol | 唯一 binding-neutral DTO/validation/version owner；旧 internal path 已物理删除 | Retain exact owner | P19-02 完成 |
| Typed operation | 唯一私有 catalog/invocation/policy owner；HTTP 与 embedded 已直接复用 | Retain exact owner | P19-03/P19-05 完成 |
| Embedded Runtime | 公共 concrete binding 完整覆盖唯一 operation catalog；直接复用严格验证、能力、幂等、problem 与 replay，不建立 transport round-trip | Retain exact owner | P19-05 完成 |
| Runtime instance lifecycle | `bootstrap.Instance` 唯一创建 Runtime root 并拥有 cancel/join；同一 root 显式注入 Assembly、operation、Interaction、Toolset、LSP、MCP/OAuth 与 workers，request/startup Context 不成为共享资源 owner；retryable Close 在依赖释放前完成 operation、worker 与 Host join。canonical data directory 仅在 setup/seeding 期间串行，多个 Runtime instance 可共享其 durable store | Retain exact owner | P19-04；P84 修订；P113-06 lifecycle closure |
| Architecture fitness | 长期门禁守 DAG、package/词汇/state owner、合法构造、process context root、公共合同与持久化 shape；可执行编排使用统一复杂度预算，生成物、声明 catalog 与递归 validator 明确排除，不冻结历史 identifier、局部变量或声明文件位置 | Retain semantic guard | P113-06 取代历史布局台账 |
| Consumer wiring | CLI/前端/TUI 均不在本阶段修改；Runtime 不提供旧接口适配 | Defer | P19 完成后专项 |

### 2.4 P20 真实 consumer 回归事实

| 能力 | 当前事实 | Verdict | 阶段 |
|---|---|---|---|
| TUI 核心交互 | 隔离 mock PTY 已证明 Shift+Enter 多行、运行中 PageUp 和 Ctrl+O Tool 详情均可工作；这些行为不解释真实 Run failed | Retain + regression | P20-01 已验证 |
| embedded 可观测性 | `embedded` 作为库不安装进程级 OTel provider；HTTP dev 的 `lyra.log` 只是 Makefile 对独立服务进程 stderr/stdout 的重定向。当前 CLI 宿主未安装 provider，`LYRA_LOG_LEVEL=debug` 的真实 embedded 查询仍保持 stderr 为空、stdout 为单行合法 JSON，隔离数据目录也只产生 Runtime 数据库与锁文件 | Host composition gap；禁止 library 擅改 globals | P20-01 已审计 |
| model stream/final 对账 | reasoning/text Delta 各自保持同一开放 Item，直到 `ModelCallCompleted` 以最终完整消息完成原 identity；类型切换不再提前落库或复制 reasoning | Retain authoritative reducer state | P20-02 完成 |
| provider metadata schema | Agent `SchemaFor` 已在唯一 Framework owner 按 `encoding/json` 修正：`json.RawMessage` 接受任意合法 JSON，`[]byte` 接受 null/base64 string；真实 DeepSeek object/string metadata 与 signature 通过，Runtime 未删除 metadata、识别 provider 或绕过校验 | Framework-owned correction；Runtime 无 workaround | P20-03 完成 |
| Tool invocation segment | 审批/恢复后，canonical Tool Item 可保留原 Segment identity，外部调用尝试 journal 归恢复后的 Segment；Run/Item 关联和 SQLite foreign-key/integrity 均正常 | Retain exact semantics | P20-01 已反证非串写 |

### 2.5 P21 运行链路与交互控制事实

| 能力 | 当前事实 | Verdict | 阶段 |
|---|---|---|---|
| Effect / Item identity | canonical model/tool Item、provider SourceCallID 与外部 invocation attempt 各有唯一 owner；重复/乱序/replay 不再产生第二次 started | Retain exact identities | P21-01 完成 |
| waiting / recovery | checkpoint schema v1 冻结 product capabilities；Pending、open Item/context、answer claim、continuation opening 与 checkpoint 删除保持原子顺序，boot recovery 在 Goal 注入后探测真实 Agent tree | Retain exact transaction | P21-02 完成 |
| approval / HITL | allow once、deny、session/project/global remember 及作用域边界已由真实 DeepSeek 验证；问题 choice/text、取消、同进程与 crash/restart resume 共用一个 typed continuation contract，restore 不重跑 policy/hook/admission | Retain Runtime ownership | P21-03 完成 |
| cooperative cancellation | Runtime adapter 将 product root/subtree intent 绑定到对应 Agent Effect context；root 覆盖全树，child 只覆盖目标及后代，Framework 继续拥有 safe settlement 后的 lifecycle | Retain anti-corruption owner | P21-04 完成 |
| Segment active duration | 每个 product Segment 在首次激活/continuation 时独立计时，不从长寿命 Agent Process started time 重算，授权/HITL 等人工等待不计入 active | Retain exact accounting | P21-05 完成 |
| Tool cancellation | Tool 所属 Run 的取消造成的终止使用 `tool_canceled` / `toolCanceled`；父 Run 的 Delegate Tool 使用 `child_run_canceled`，内部 sentinel 不进入产品 detail，CLI 显示 canceled | Retain first-class failure | P21-06 完成 |

### 2.6 P24 Runtime/Desktop 全链路硬化事实

| 能力 | 当前事实 | Verdict | 阶段 |
|---|---|---|---|
| Session activity / cold hydration | Session activity 由 durable non-terminal Run 派生；Desktop Session projection 使用语义订阅，reload 从 durable running Item 恢复 fold 状态 | Retain durable read model | P24-01 完成 |
| Workspace invalidation | Runtime Git watcher 比较 HEAD/index 语义指纹且禁用 optional lock；Desktop workspace event 只失效对应 query scope，不再把 scoped resync 扩张为 global | Retain exact observation boundary | P24-02 完成 |
| Segment activation arbitration | root-owned arbiter 串行化 opening 后 activation 与 cancel；cancel 先赢时不跨 executor boundary，activation 先赢时 cancel 等待其结果后分类 | Retain single lifecycle owner | P24-03 完成 |
| Boot invocation settlement | Application recovery snapshot 包含全部 open model/tool invocation 并形成 exact write-set；Adapter 与 Run tree、Goal、cleanup 在同一 transaction 应用，Infra 只存 technical journal | Retain cross-aggregate transaction | P24-04 完成 |
| Desktop E2E acceptance | HITL、Plan、Goal、并发 cancel/resume、reload 与真实 `kill -9` 均已由隔离 Runtime/Desktop/SQLite 闭环；空闲页面无自激 RPC | Retain regression matrix | P24-05 完成 |

### 2.7 P25 第二轮反证事实

| 能力 | 当前事实 | Verdict | 阶段 |
|---|---|---|---|
| Session/workspace projection serialization | live Run stream 活动期拥有前端 fold；durable snapshot 延迟到 stream tail/idle 后应用；snapshot→subscribe 期间 Run 进入 terminal/waiting/stale 时立即重读 durable projection，ack 后跟随失败不回流 start-error 通道；Session adapter 将权威 `session_not_found` 投影为 application port 的 absent snapshot，运行故障继续失败；每次成功 Session 集合读都对账 active/open 导航，本地 delete 不在 Runtime commit 前伪造身份缺失，rename/favorite 失败回滚后必须重读并发权威变更；URL/history 直接挂载的 active Session 在 material view 建立前进入 held-open/last-session memory，权威对账保留 active 并清理 stale ref，relocate 条件写失败重读当前 cwd/revision；workspace resolution 区分 default/authoritative unavailable/transient failure，瞬时失败由 event application 有界退避恢复，retarget/dispose 可取消旧 generation 且旧订阅不发布；Run/Item/Plan duplicate/late fold 单调并保留 HITL continuation；提交中的 HITL batch 若被 standing projection 淘汰则立即释放本地 staged 状态，延迟 rejection 不再覆盖远端已胜出的 continuation | Retain single projection/subscription owner | P25-01/P35/P36/P38/P39/P40/P41 完成 |
| JSON-RPC envelope ambiguity | `delivery/transport.DecodeMessage` 在 SDK decode 前递归拒绝 duplicate/unknown member、空 method、非字符串 id、request/response 混合及 result/error 双载荷；HTTP 只接受 request/notification 并投影 transport problem | Retain exact transport owner | P25-02 完成 |
| Adversarial recovery matrix | HITL 双提交与真实 ask_user resume、Plan revision 2、Goal completed/blocked/budget、cancel/resume、idempotency drift、cursor、transaction failure、断线与 active-Run `kill -9` 已覆盖；崩溃 started invocation 收口 unknown，真实终态无开放 lifecycle | Retain regression matrix | P25-03 完成 |
| Standalone Framework dependency | Agent Baseline 20 已发布为 commit `8e667d716b22`；Runtime 直接绑定远端 pseudo-version `v0.0.0-20260811152247-8e667d716b22`，没有 `replace`、Schema 复制或 metadata 降形旁路 | Retain canonical dependency | P25-04 完成 |
| Goal terminal settlement projection | Domain `complete` 保持 objective terminal fact，Application drive 独占最终 Run accounting 与条件清除；Delivery 将可观察窗口投影为公共 `completing`，Desktop Goal context 保留占位且禁止 lifecycle command，随后由 `goals.changed` 收敛到 `null` | Retain exact cross-layer owners | P37 完成 |
| Desktop command settlement replay | 生成 method policy 是 40 个 command replay 类别的唯一输入；Desktop RPC SDK 为每个 logical mutation 持有稳定 key，对 settlement-unknown transport failure 有界重放一次，并按 typed `idempotency_in_progress.retryAfterSeconds` 有界等待一次；definitive business/4xx refusal 与 caller cancellation 不重放。Run opening 每次 attempt 独占新 event stream，失败流先释放；Session create 的 attempt deadline 留在 Runtime Adapter，Application 只见 Session 结果 | Retain generated policy + SDK settlement owner | P42 完成 |
| Desktop Run opening / cancel settlement | RPC Agent Adapter 将 opening handshake deadline 与 accepted stream lifetime 分离：单次 30 秒，首个 ambiguous timeout 只换 delivery signal 并复用原 mutation identity，第二次 timeout 有限返回；ack 后 deadline 释放而 session signal 继续拥有长流。Cancel response 只在发起 revision 仍成立时提交；失败后由中性 Agent projection 重读 terminal 事实，远端胜出不产生陈旧错误 | Retain Adapter deadline + Application neutral revalidation | P43 完成 |
| Desktop Session mutation settlement | destructive rollback 经 mounted Session 唯一同步 owner 等待权威 commit 并按 Session single-flight；Adapter 消费 `droppedRuns.userInput` 后只向 Application 投影中性 AgentInput。无 checkpoint 的 files/both 不降级为 history。rename/favorite/relocate 条件写按 Session 串行并链式携带成功 revision，失败只条件回滚本字段 | Retain Application command/projection owners + Adapter wire translation | P44 完成 |
| Desktop Goal/Plan lifecycle boundary | Goal lifecycle unary command 共用 RPC 有界同-key settlement；Goal Application 按 Session 串行命令、以 typed response 单调提交并重读，Adapter 独占 wire→Goal read model 与 provider 注册。预算耗尽的 blocked Goal 不暴露无效 Resume，但可由 launcher 原位替换；launcher 状态按 Session identity 隔离。Plan wire snapshot/event 在 Agent Adapter 转成中性 Plan domain 后才进入 fold/view；`goals.changed` 只失效点名 Session | Retain Goal-owned adapter + neutral Agent Plan boundary | P45 完成 |
| Desktop Agent Runtime fact boundary / Composer model restore | Agent SDK 用中性 facts 表达 live event、durable Run/Item/Interrupt 与 cancel projection，Runtime wire 的校验和映射只存在于 Agent Adapter；pending work provider 由 Agent bootstrap 装配，defaults/public surface 不拥有 Agent 内部能力。Composer 在无有效显式偏好时等待 active durable Session model，只有无 Session 才退回 catalog 首项 | Retain Agent-owned anti-corruption boundary + durable model precedence | P46 完成 |

## 3. 产品领域能力

### 3.1 Run、Segment 与 execution package

| 当前能力 | 当前 owner | 目标 | Verdict | 验收 |
|---|---|---|---|---|
| Run aggregate | `domain/run.Run` | 保持唯一 aggregate owner | Retain + Refactor | P16-01 已用私有字段统一 identity、lineage、frozen admission facts、lifecycle、active Segment、metrics 与 terminal facts；所有 mutation 经 admission/restore/advance/suspend/resume/terminate/cancel/lost 行为，Application/Persistence/Delivery 只消费完整值 |
| Segment identity/lifecycle | `domain/run` + `application/runs` | 保持；P3 重推 root port | Retain + Refactor port | resume 保持 RunID、打开新 Segment |
| Run limits/capabilities | `domain/run` | 保持 | Retain | admission/restore 同值，不能重新谈判 |
| Terminal outcome taxonomy | Completed/Canceled/TimedOut/Failed/MaxBudget/MaxSteps/Lost | Agent Framework Termination + Application intent 唯一映射 | Retain | P8 完整 matrix 已冻结并覆盖 |
| ExecutorRef/checkpoint | `application/runs` opaque executor binding/checkpoint | P3/P6 按真实 consumer 演进 | Refactor port | Run entity 不保存执行端口细节，无 Agent Framework concrete type/payload parsing |
| Executor member identity | Application `ExecutorMember`、continuation/child binding | 保持不透明 member identity | Retain | P3 Application `ProcessID` 归零；旧 adapter 显式映射 |
| Step count | Run usage | 保留产品需要的计数，区分 Agent Framework Step | Refactor | 不把两种 Step 当同一类型 |

### 3.2 Conversation、Transcript、Knowledge

| 当前能力 | 当前 owner | 目标 | Verdict | 验收 |
|---|---|---|---|---|
| Conversation message log | `domain/conversation` + `application/conversations` I/O | 保持 | Retain | Count/Truncate/Seed 不依赖 Run executor |
| Transcript Items | `domain/transcript` | 保持唯一 Item aggregate owner | Retain + Refactor complete | P16-02 已关闭公开 tagged-union mutation：非 Tool variants 构造即 complete，ToolCall 只经 start/complete/fail/abandon/classify 行为迁移；Application 仅拥有 provisional stream `ItemStart`，Persistence/Artifact codec 只在机器守卫允许的技术边界使用严格 snapshot |
| Offloaded transcript content | `domain/toolresult` | 保持准确独立 capability | Retain | 无泛化 blob service |
| Knowledge/LYRA.md | `domain/knowledge` + `application/workspace` + `infra/knowledgefile` | 保持独立 | Retain + CAS hardening | P50 以 opaque content revision、权威返回与 committed-only `knowledge.changed` 消除单进程丢更新；P51 以唯一 physical identity containment、跨进程 document lease、权限继承、fsync+rename 与 cold staging recovery 关闭 symlink 越界和 crash/CAS 缺口；P52 以精确路径指纹观察外部 create/write/rename/remove，API write 则在发布前按 exact identity 接受新基线，避免漏失效和重复 refetch；用户编辑与 Agent state 无关，wire 只在 Delivery/Desktop Adapter 映射 |
| WorkingContext | Application composer + Agent Framework Interaction private state | 保持 Host composition 与 executor state 分离 | Retain | fresh root 读取产品事实；restore 只用 opaque checkpoint，不从 Conversation 重算 |

### 3.3 Interrupt 与 approval

| 当前能力 | 当前 owner | 目标 | Verdict | 验收 |
|---|---|---|---|---|
| Interrupt semantics | `domain/interrupt` | 保持 | Retain | Kind/Key/Resolution 纯领域值，无 I/O/executor |
| Pending continuation | `application/runs.Pending` + `adapter/persistence` mapping | 已保持 owner 并接入 Agent Framework public pending input | Retain | 一个 root tree 一个 pending hand-off；Infra 只见 technical record；claim 后普通读取不可见 |
| Approval domain | `domain/approval` | 保持产品策略 | Retain + remove I/O ports | 不进入 Agent Framework |
| Ask-user/approval tool input | Toolset product Interrupt + `agentexec/interactioninput` | public Interaction helper → product Interrupt | Retain | 不解析 private Framework payload；旧 adapter 已删除 |
| Answer/resolution | runs + Interaction input adapter | semantic Application command → WaitID-addressed Signal | P6 Agent Framework bridge 已完成；P14 内部职责精修 | 无任意 Signal API；text/choice answer 各自验证其语义，answer claim/segment opening/Signal 顺序受测试 |

### 3.4 Accounting

| 当前能力 | 当前 owner | 目标 | Verdict | 验收 |
|---|---|---|---|---|
| Token/model-call accounting | `domain/accounting` + authoritative model decorator | final/usage/pricing/Run progress 同一事务 | Refactor bridge | P5 已完成；Delta drop 不丢 final 或 usage |
| Pricing/USD | adapter/modelcatalog + observer | Runtime adapter/domain value | Retain | Agent Framework Usage 无价格字段 |
| Framework resource usage | old Agent aggregate | Agent Framework Usage | Replace | 只翻译需要的中性事实 |
| Goal budget attribution | goals/runs | 保持 Application | Retain | child/root、resume、lost 归属准确 |

### 3.5 其他产品上下文

| 能力 | 当前 owner | Verdict | 迁移影响 |
|---|---|---|---|
| Goal | domain/application/toolset | Retain | autonomous Run admission retry、等待边界后的 incarnation ownership refresh 与 terminal resolution 分属准确私有行为；消费端口名为 `AutonomousRuns`，不下沉 Agent Framework |
| Plan | `domain/plan` + `application/plans` | Retain + Refactor complete | P16-03 已由私有 `plan.State` 独占 replacement/invariant/revision/time；Application 形成 CAS replacement 并在成功保存后发布 invalidation，Tool 只消费用例，SQLite 不决定 transition；保持 Plan 唯一术语且不与 Goal/Todo 合并 |
| Schedule | domain/application/toolset | Retain | 通过 `RunStarter` 启动并返回 `StartedRun` 事实；有界 `occurrenceBatch` 分别处理 pending dispatch 与 due claim，不直接调用 Agent Framework。更新 workspace 的 wire 三态由 Protocol/Delivery 拥有，Domain/Application 继续只消费空值=Runtime 默认、非空=已准入显式路径的 `CWD` |
| Skill/Proposal | domain/application/adapter/toolset | Retain | deferred manifest 接线更新 |
| Agent memory | domain/application/toolset | Retain | 与 Conversation/Knowledge 分开 |
| Model/provider catalog | domain/application/adapters | Retain | 每 Run exact model binding 进入 deployment assembly |
| MCP/A2A/LSP | domain/application/infra/toolset | Retain | 保持技术能力，不进入 Agent Framework Kernel |
| Workspace/change/isolation | application/adapters/infra | Retain + recovery audit | 外部事实失效由 Host policy 处理 |
| Hooks | domain/application/adapter | Retain | P5 已在普通 Tool 边界触发；post-hook 是 observation，不覆写 settlement；P50 trust commit 后发布专用 `hooks.changed`，Desktop 按 project 串行且 UI pending 锁定；P52 让 global/project/cwd `.lyra/hooks.json` 的外部新增、替换与删除进入同一专用失效流，文件布局仍只属于 Workspace Adapter |
| Feedback/codebase index | domain/application | Retain | 与 execution migration 无直接耦合 |

## 4. Application 能力

### 4.1 Runs use cases

| 当前能力 | 当前形态 | Verdict | 目标证据 |
|---|---|---|---|
| Start admission | `ValidateRootStart` → `StageRoot` → durable opening → `BeginRoot` | P4 real consumer 已验证；P14 内部命名/依赖边界精修 | Start 与 Resume 各自验证 staging dependencies，共享准确的 segment-supervision dependency boundary；stage 不外呼 model/tool，commit 前失败只 Release |
| Event observation | `ExecutionObserver.Observe` | P4 real consumer 已验证 | 只流 Application-owned executor facts；final 来自 Result |
| Executor release | `ExecutionReleaser.Release` | Retain | 与产品 Cancel 分离；非 Waiting 终止恰好一次 |
| Product Cancel | durable control intent → `RunningRootCancellationRequester` → continued observation → release | Retain；P14 内部职责精修 | durable cancellation Run tree 独占 topology/lifecycle 校验，process-local member binding 保持独立；request cancel 不提前切断 pump，确定终态后才释放 |
| Resume | `WaitingExecutionContinuer.StageContinuation/BeginContinuation` | Retain | exact BuildID/deployment/scope/capabilities restore；waiting continuation 的 envelope/topology/order 与 resume binding 各有准确内部 owner；per-Run reducer、Segment identity 和 deterministic preorder 由私有 resumed-route builder 原子重建；opening commit 后才 Signal |
| Steer | `RunningExecutionSteerer.SubmitSteer` | Retain | semantic steer 只在下一 model safe boundary 投影 |
| Child subtree cancel | `MemberID` + `WaitingSubtreeCancellationPreparer` | P7 real consumer 已重推 | Application 只看 member projection、resulting checkpoint 与一次性 Apply/Discard capability |
| Waiting subtree mutation | `WaitingSubtreeChange` 持有 execution ACL capability | P7 Agent Framework bridge 已完成；P14 精修 commit invariant owner | Application 不见 Agent Framework plan；source 冻结跨过 transaction；contextless Apply 只安装状态，final-boundary Continue 独立激活，失败时 exact restore/terminal 收口；`WaitingSubtreeCancellationCommit` 内部按 boundary、disposition envelope、pending-tree topology 和 surviving continuation 分阶段证明同一原子 write-set，不产生第二 validator owner |
| Run pump | executor event reducer | Retain；P14 内部职责精修 | `segmentStartup` 只拥有 durable opening 前可回滚资源，commit 后转交 pump；ItemStarted projection 与 park-boundary write-set 各有准确 owner；不推进 Agent Framework internal state |
| Run journal | committed RunEvent | Retain | persist-before-publish |

P8 production cutover 已用真实 Bootstrap consumer 冻结 root stage/observe/begin、cancel request/release、waiting restore/answer/steer、child reservation/start outcome 与 waiting-subtree prepare/commit/apply-or-restore 的准确端口集合。

### 4.2 Cross-aggregate writes

| 写集合 | 当前事实 | Verdict | 阶段 |
|---|---|---|---|
| Run admission | Session + Run/Transcript | Retain/refactor types | P3 |
| Waiting tree barrier | Run + Pending + checkpoint + Items | Retain semantics；P6 Interaction 已验证 | P6 已完成 |
| Waiting child cancellation | Run tree + Pending + checkpoint + Items | P7 Agent Framework boundary 已完成，保留 Application transaction | P7 已完成 |
| Terminal tree | Runs + Pending + checkpoint + transcript + Goal | Retain | P8 已统一 write-set/first-wins/release ordering |
| Boot recovery | stored facts + opaque checkpoint probe | Retain | P8 已由 Agent Framework production probe 接管 |
| Rollback/fork cleanup | Conversation + Transcript + active/waiting Run | Retain | P8 已保持 cleanup intent 与 live release 分相 |

## 5. Agent execution 能力

### 5.1 当前 `adapter/agentexec`

| 当前实现 | 作用 | Verdict | Agent Framework replacement |
|---|---|---|---|
| `InteractionExecutor` | 唯一生产 tree owner | Retain | per-Run Agent Framework Engine + Interaction；P4–P8 完整纵切 |
| old Engine/GOAP/TurnProcess/turn controller | 已删除 | Remove complete | Agent Framework Process/Engine public lifecycle + Application Run pump |
| private process-tree/suspension codec | 已删除 | Remove complete | opaque public TreeSnapshot + Interaction pending-input ACL |
| child execution/configuration | 传播 model/hooks/budget | Rewrite | exact child Deployments + ProcessAdmitter |
| authoritative projection | model/tool/usage/final 同步 receipt | Retain | P5 authoritative decorators；旧 observer 已删除 |
| tool decorator | approval/hooks/presentation | Retain semantics, Rewrite attribution | P5 ToolInvocation context + P6 interactive HITL 已完成 |
| usage accounting | subtree token/cost | Retain | cumulative accounting/pricing、child attribution 与 terminal tree 已收口 |
| deferred manifest glue | old toolloop promotion | Rewrite | P5 唯一 `toolset.Manifest` + Interaction DeferredTools/AdvertiseTools 已完成 |
| maintenance/restore checks | per-Run Interaction session + checkpoint probe | Retain | exact Deployment/BuildID/scope/capabilities 与 tree-wide boot owner 已收口 |

### 5.2 Agent Framework 已提供的合同

| Runtime 需要 | Agent Framework Baseline 20 | Runtime 责任 |
|---|---|---|
| root execution | Engine/Deployment/Interaction | 组装产品配置并翻译结果 |
| tree identity | ProcessID/Relation/root/parent/depth | 映射不透明 executor member/child Run |
| child admission | `ProcessAdmitter` + prospective identity + `ProcessStartOutcomeAcknowledger` conclusive started/aborted | durable Opening reservation、public Running 与 aborted cleanup |
| waiting | WaitID/Signal + Interaction pending helper | 产品 Interrupt 与事务 |
| restore | Snapshot/TreeSnapshot + exact resolver | Store、BuildID、Host metadata |
| steer | Interaction steer signal + `ModelInvocation.AppliedSteerSignalIDs` exact attribution | Signal ID 到产品内容的 adapter-owned opaque checkpoint mapping；首次可见消息整批原子 projection |
| subtree cancel | `PreparedWaitingSubtreeCancellation` 冻结 source，并提供 prospective result 与 contextless one-shot Apply/Discard | Application write-set 只见 projection + opaque checkpoint；commit 后 apply-or-exact-restore |
| model/tool attribution | ModelInvocation/ToolInvocation | Transcript/accounting/hooks/pricing |
| deferred tools | Tools/DeferredTools/AdvertiseTools | Tool catalog、发现语义和权限策略 |
| observation | Event/Delta | lifecycle projection/临时流；权威记录另行同步 |
| resource facts | Budget/Limits/Usage | 产品 limits、USD accounting 和 policy |
| unknown Effect | UnknownEffectIDs | live/recovery tree 由 wake + bounded public query 收口为 durable RunLost；不开放任意 resolution |
| prepared-step durability | PreparedStepAcknowledger 只有单 Process Snapshot | 初版不启用；只恢复 committed quiescent TreeSnapshot |

当前 root Interaction、ordinary Tool/model、waiting capture/restore、steer、durable Delegate child admission 与 waiting-subtree change 均没有已知 Agent Framework blocker。P7 真实 Runtime consumer 已推动并验证两个中性 Framework 合同：conclusive `ProcessStartOutcome` 与 one-shot `PreparedWaitingSubtreeCancellation`；Agent Framework 仍不感知 Run、Store、transaction 或产品恢复策略。单 Process prepared-step acknowledgment 本轮明确不启用，不算待实现 Runtime 能力。

### 5.3 WorkingContext provenance

| 来源 | Runtime owner | checkpoint 投影 | purpose |
|---|---|---|---|
| base prompt | `adapter/agentexec` | `base_prompt` + builtin reference | instruction |
| home/projectRoot/cwd LYRA.md 级联 | Knowledge reader + composer | `user_knowledge` / `project_knowledge` | instruction |
| pinned/recalled Memory | Agent Memory + composer | 只记录实际注入的 item ID | data |
| AGENTS.md cascade | prompt-source Adapter 保留 canonical path + home/projectRoot/cwd provenance，Application 验证闭合集合，composer 只消费内容 | 只记录预算后实际渲染的 canonical path | instruction |
| Session Plan | Plan reader + composer | `session_plan` | instruction |
| lifecycle hook context | Hook Application + composer | 精确 SessionStart/UserPromptSubmit Part source | instruction |

这些 kind 是 `agentexec` 私有诊断合同，不进入 Runtime Protocol、Application port、SQLite schema 或 Agent Framework。文本顺序、header、预算与 best-effort 读取语义未改变；metadata 只伴随自包含 WorkingContext checkpoint。

## 6. Tool 能力

| 当前能力 | Verdict | Agent Framework 接线变化 |
|---|---|---|
| built-in tool schemas/names/descriptions | Retain | 作为 frozen Tools/DeferredTools 输入 |
| strict typed decode | Retain | P5 Dispatcher 调用同一 Tool contract，无第二 schema/decode |
| approval/safety | Retain Runtime ownership | P5 自动 allow/deny/rewrite + P6 interactive approval/remember resolution；restore 不重跑 plan/hook |
| activity/presentation | Retain | P5 已接线，不进入 Agent Framework Event/Delta |
| `search_tools` | Retain | P5 使用注入的 precise advertiser 调用 `AdvertiseTools`；Toolset 对 Agent Framework 零 import |
| delegation wrapper | Rewrite | Interaction `Delegate`，不再 old AgentTool |
| ask_user | Retain | 真实 Runtime Tool 注入 Interaction pending input；旧生产构造已删除 |
| Plan/Goal/Schedule/Skill tools | Retain | Application ports 更新 |
| MCP/A2A dynamic tools | Retain | 本 Run deployment assembly 冻结 authority |
| old agent/core/toolloop imports | Remove | Toolset 最终 Framework-neutral |

完整模型工具面、参数和历史删除裁决仍以 [`TOOL_SYSTEM.md`](TOOL_SYSTEM.md) 为准。

## 7. Persistence 与 recovery

| 能力 | 当前事实 | Verdict | 验收 |
|---|---|---|---|
| SQLite schema | epoch 75；Run 保存 latest opaque Application command identity，覆盖 opening、EventCommit、tree barrier、waiting-child cancellation 与 terminal；Running marker 绑定 exact active Segment，Waiting barrier/terminal 保留生产 Segment，already-Waiting command 使用 empty Segment + unique identity，普通 Suspend/Resume/Restore/recovery 不跨代继承；durable replay store 拥有 opaque namespace；保留 `root_member_id`/`memberId`，approval binding 持有 exact `toolCallId`，open/resuming answer audit；accepted Question answer 与 resume claim 同事务；Goal/Run 使用 `incarnation_id`/`goal_incarnation_id`，Goal 冻结 canonical Run capabilities；child-start payload 为 adapter-owned canonical JSON；Transcript/committed-tool 使用 closed `failure` taxonomy；tool invocation identity 按 Segment 隔离 | Retain pattern | 旧 epoch/列/codec 被拒绝，无 migration |
| executor checkpoint | Host metadata + Agent Framework public complete-tree snapshot | Retain opaque payload owner | Application/Store 完全 opaque；exact capabilities 纳入 expectation |
| checkpoint transaction | runsegment/persistence 组合 | Retain semantics | waiting facts 同事务 |
| BuildID | Host-owned | Retain | 不进入 Agent Framework deployment/snapshot |
| boot recovery | Application policy + agentexec framework probe | Retain | P8 production recovery wiring 已收口 |
| unknown Effect live/recovery | tree-wide reconciliation | Retain | wake 丢失由 public polling 收敛；RunLost 写失败重试且 release 不提前 |
| isolated recovery | fail closed | Retain | 不猜测重建 scratch world |
| terminal deletion | root aggregate transaction | Retain/converge | terminal 与 delete 原子 |
| waiting subtree persistence | Application transaction + execution-owned prepared change | P7 Agent Framework boundary 已完成 | one-shot prepared change + opaque resulting TreeSnapshot + exact restore fallback |
| active-step crash durability | 旧路径能力不作为新合同 | Defer | 不启用 PreparedStepAcknowledger；无 committed quiescent TreeSnapshot 时 RunLost |

## 8. Delivery 与协议

| 能力 | 当前 owner | Verdict | 说明 |
|---|---|---|---|
| Protocol types/errors | `protocol` + `contract` | Retain | Protocol `2026-08-17`、Artifact v19；Question `answers` 只投影 Runtime 已接受响应，未回答/取消不伪造；ToolCall `approvalDecision` 只投影该调用实际接受的人类决定，不从当前策略或终态推断；ToolCall lifecycle 与 optional exact execution duration 分离，后者排除审批等待且 unknown 时不伪造；Tool cancellation 是独立 `tool_canceled` / `toolCanceled` variant；Runtime 失效闭集覆盖 models / approvals / agent memory / codebase 的 API 与后台任务变化，且不携带业务值；机器真相仍在 contract |
| Runtime method implementation | `delivery/server` | Retain | Protocol server side 与 projection；构造按 required use-case validation、defaults、contract facts、instance、notification observation 分阶段，不持有 transport listener |
| JSON-RPC dispatch/registry | `delivery/dispatch` | Retain | method registry/router/idempotency；typed params decode 与 response projection 分属 `params.go`/`response.go`，不与 server 合并 |
| HTTP/SSE | transport/http | Retain | envelope I/O、stream/backpressure |
| Embedded Go | 旧 internal channel prototype 已删除；P19 新建类型化公共 binding | Rewrite, do not revive transport | CLI 已成为真实消费者；复用 operation/Application，不导出 envelope/Router |
| Server product-value projections | Application read/write use cases + 必要 immutable Domain values | Retain | 只做 wire validation/error mapping/projection，不读取 repository、不持有 lifecycle owner |
| Delivery adapter imports | architecture guard 禁止 | Retain | Delivery 只驱动 Application；对 concrete Adapter/Infra/Bootstrap/Agent Framework import 为零 |
| frontend/TUI/CLI generated consumers | Desktop generated Runtime bindings、strict validators、samples 与 handwritten SDK 已同步 P25/P26/P33；Schedule SDK update 直接消费生成 request，不以 create shape 猜测 update surface；其他消费者按各自阶段消费公共 `protocol` / `embedded` | Retain boundary | 精确 cutover 见 `CONSUMER_HANDOFF.md`；Runtime 不为消费者建立兼容 shape |

## 9. 结构清理台账

### 9.1 Shared mechanism ownership

| 能力 | 最终 owner | 证据 |
|---|---|---|
| completion | `internal/completion` | Application run/goal、Application taskgroup 与 Infra teardown 共享同一 completion-first join rule |
| HTTP origin | `internal/httporigin` | Application MCP policy 与 Infra MCP redirect security 共用纯 normalization |
| idempotency | `internal/idempotency` | Delivery consumer port 与 SQLite implementation 共享 opaque record/errors |
| opaque token framing | `application/opaquetoken` | pagination 与 Run replay 两个 Application continuation owner 共用 strict URL-safe framing；payload 语义、版本与校验仍归各自 owner |
| pagination cursor | `application/pagination` | 多个 Application read 共用 keyset pagination 语义；Delivery 只消费 Page/error contract，不拥有 cursor policy |
| replay cursor | `application/runs` 私有实现 | 位置、scope、retention、版本与校验均由 Run journal 独占；只复用 ring 外的 opaque framing |
| teardown step | `infra/teardown` | non-cooperative close serialization 是 Bootstrap 消费的纯技术机制；组合根只装配与关闭 |
| task group | `application/taskgroup` | Application coordinator 启动 request-detached work；Bootstrap 只构造、注入和关闭该 Application lifecycle owner，Delivery 不消费 |
| path identity | `infra/pathidentity` | filesystem/symlink identity 是技术机制，Adapter 单向消费 Infra |
| secret masking | `application/secrets` | model/MCP 两个 Application consumer 共享 presentation-boundary policy |
| process-local notification connection | Bootstrap `notification_source.go` closure | 只连接 composition-time producer 与 Delivery observer；两侧各得到 publish/observe function，不建立 Adapter package、implementor-owned interface 或产品事件总线 |

`internal/component` 已物理删除；永久架构守卫把根级跨环 capability 与 Application-owned shared mechanism 分开约束。前者不得依赖任何产品 ring，后者不得吸收 Domain 语言或 Adapter/Infra/Delivery/Bootstrap 实现职责。

### 9.2 Adapter/Infra

- 原混合 `adapter/maintenance` 已按真实 owner 物理拆分：`runmaintenance` 只拥有 clean-Run 边界的 history compaction、memory consolidation、Skill proposal mining 与 idle-Skill archival；`sessiontitle` 只生成 Session title；`utilitymodel` 只提供辅助能力共享的 middleware-free 单次模型调用。旧目录、`llm.go`、`extraction.go`、`skillmine.go`、`skillcurate.go` 与 `title.go` 均由架构守卫禁止回流；
- `codebaseindex`、`codeintel`、`executionctx`、`hooks`、`isolation`、`mcpconnection`、`modelcatalog`、`modelclient`、`persistence`、`providerregistry`、`runrecovery`、`runsegment`、`skillproposal`、`toolset`、`workspace` 与 `workspacepath` 按当前调用图继续拥有应用值映射、外部错误翻译、跨机制组合、安全策略或 transaction write-set；P113 已删除没有独立防腐/变化轴的 `adapter/toolname` 与 `adapter/notification`；
- 原单消费者 `adapter/pricing` 只读取与 `modelcatalog` 相同的静态模型目录且无独立变化轴，已收回 `modelcatalog.Pricing`；其余小 Adapter 均有明确 Application port、外部 SDK 防腐、安全策略或多个消费者证据，不按文件数机械合并；
- SQLite、knowledge-file、Git、exec、sandbox、LSP、MCP/A2A client、checkpoint、telemetry、path identity 与 advisory lock 只提供 technical mechanism，保留 Infra；Runtime ownership Adapter 将短期 data-directory setup、Session writer、working-tree shared/exclusive 与 Goal drive identity 映射到中性 advisory-lock 原语，Knowledge document CAS 继续独立翻译自身生命周期/错误；Infra 对 Application/Adapter/Delivery/Bootstrap 反向 import 为零；
- 原 `infra/storage` 无行为 umbrella 已删除：SQLite 直接归 `infra/sqlite`，LYRA.md 文件布局与原子替换归 `infra/knowledgefile`；原误置于 Adapter 的进程级 OTel global 配置归 `infra/telemetry`。三个 package 分别只因数据库、知识文件和进程遥测而变化，不再共享泛化 storage/observability 目录；
- MCP live registry 中的已配置服务器状态统一称为 `configuredServer`，从 live projection 移除且等待关闭的连接统一称为 `detachedSession`，按名称过滤的 API 参数称为 `serverName`；进程内连接生命周期不再依赖 `ms`/`old` 等上下文猜测；
- `workspacepath` 原有第二份 symlink/containment 判定已删除，统一消费 `infra/pathidentity` 的 physical identity；Adapter 只保留 Application path-policy 错误与相对路径投影；
- `agentexec` 已按 root execution、effect attribution、waiting/restore、Delegate admission/projection、tree mutation 分离变化原因；Delegate 的 model input/description 是 Interaction executor 独占的策略合同，已从单消费者 `toolset/delegation` 回归该 ACL；没有第二 controller、scheduler、mailbox、private wire owner 或为了文件拆分制造的子 package；
- 原先每个 Runtime built-in tool 一个目录的 `toolset/{agentmemorysearch,askuser,conversationsearch,goal,lsp,offload,plan,schedule,shell,skill}` 已按共同变化轴收敛为 `toolset/builtin`，文件继续按能力家族组织；deferred discovery 回归 Resolver owner，稳定 model-facing identity 在 P113 进一步从常量-only Adapter package 收回已有 `domain/tool` vocabulary。Run Application 的 process-local lifecycle owner 仍统一为 `runTreeOwner`；Toolset 的 manifest assembly、Call decorator、schedule/shell command group 与 deferred search ranking分别由准确私有行为表达，不进入 Agent Framework；
- Chat provider catalog 以 `ProviderOpenAICompatible`/`ProviderAnthropicCompatible` 准确表达 caller-defined compatible endpoint，以 `buildOpenAIModel`/`buildAnthropicModel` 表达模型 adapter 构造；Ollama 的 chat/embedding 同样只在 Infra composition 内消费 provider-scoped OpenAI-compatible protocol 与 `/v1` endpoint，不再为客户端能力引入完整 Ollama 服务端 module；`Compat` 不再伪装成兼容层，`Native` 不再混称厂商 wire、宿主平台或 Framework 执行能力；
- Hook wire root、codebase in-memory corpus、MCP current dial/mutation scope/tool-list snapshot、usage fold bucket 与 teardown attempt backing state 分别使用 `hooksFile`、`cachedCorpus`、`activeDial`、`mutationScope`、`toolListTarget`、`usageAccumulator`、`attemptState`，不再依赖 `config`/`loaded`/`dial`/`mutation`/`target`/`accumulator`/`attempt` 的上下文猜测；具体错误结构统一使用 `Error` 后缀，注释与测试同步，不保留旧别名；
- `persistence.SessionStores` 仍是 Session aggregate 原子 write-set 的单一 Adapter，但 rollback boundary/drop projection、workspace restore intent/cleanup、restore projection rebuild、parked terminal cleanup/terminalize/Goal record 已各归准确私有行为；portable snapshot 的 Run/parent index、cycle detection 与 resolved-root lineage 由单一 `snapshotRunTree` 拥有，不存在只转发 `Transactor` 的伪抽象；
- Application consumer-owned ports 均按用例拆分；`Coordinator` 只用于确实协调多个 use-case collaborators 的 package aggregate，不存在 package-name + exported-type 口吃或跨用例胖 executor interface；
- Application 的 post-commit read-again 信号统一归 `application/invalidation`：`Notice` 只携 resource/identity，不复制查询值，Goals/Plans/Runs/Sessions/Skills/Codebase、Bootstrap Relay 与 Delivery projection 全链路只使用 `Invalidations` 这一术语。Skill 不再保留独立空值 Relay；首次/过期 codebase search 的隐式 reconcile 也发布可重读的 indexing/settle 状态，而显式 operation identity 仍只归 Coordinator。`application/sessionadmission` 决定 Session/working-tree 业务冲突，`adapter/runtimeownership` 为其提供跨进程 lease；单消费者 `approvals/approvaltest` 已回收到其黑盒测试，不再伪装成共享生产 package；
- Delivery `server` 只做 wire validation/projection，`dispatch` 只做 JSON-RPC registry/routing/idempotency，二者对 concrete Adapter/Infra/Bootstrap/Agent Framework import 为零，现名准确且不合包；
- `internal/component`、temporary exception、空目录、历史 fixture 与一层纯转发 wrapper 均为零；最终 DAG、命名和 shared-capability purity 由永久 architecture tests 守卫。

### 9.3 Delivery/Bootstrap

- Delivery 的 `protocol`、`dispatch`、`server`、`transport` 四层分别拥有 wire vocabulary、method routing、Application projection 和 envelope I/O；StreamFrame 使用完整的 `Notification` 语义，HTTP transport 只消费预编码 frame；无消费者且不可能成为公共客户端 API 的 internal in-process prototype 已删除；
- `server` 的 Session import/export、Run/Plan event presentation、workspace subscription lifecycle 分别归 `session_transfer.go`、`presenter_run_event.go`、`workspace_stream.go`；workspace hub 的 notification coalescing、subscription admission 与 queue drain 仍在同一并发 owner 内，没有拆出第二状态机；
- Bootstrap 仍是唯一 composition root；conversation、model、MCP、tool 的 composition-time environment 和 post-Run maintenance 各由 focused builder 组装，`Stack` 只暴露 Application entrypoints/notification sources，`Host` 单独拥有 shutdown graph；
- `hostLifetime` 以 `goalDriver`、`mcpCoordinator`、`codebaseCoordinator`、`runCoordinator`、`executor`、`runEffectTasks`、`toolResources`、`hostResources` 表达真实关闭职责；旧 `goals`/`mcp`/`execution`/`effectsTasks`/`resources` 一类含混字段已删除；
- P113 已删除按历史文件名冻结 Bootstrap/Delivery 声明位置的门禁；composition、projection、transport 与 shutdown 继续由 import DAG、唯一 owner、合法构造、process root 和公共/持久 shape 语义守卫约束，声明可在同一 owner package 内随真实职责移动。
- Tool `Call` 装饰只由 `call_decorator.go` 的 `decorateCall`/`callDecorator` 表达；HTTP transport 的请求级 tracing、response metrics 与 panic containment 只由 `request_instrumentation.go` 的 `instrumentRequests` 表达。JSON-RPC 路由消费面统一使用 `messageDispatcher.Dispatch`/`dispatch.Result`，注册项的完整解码—调用—编码行为统一称为 `pipeline`；这些名称分别对应 Tool capability、HTTP instrumentation 与 RPC dispatch，不再共享含混的 `decorate`、`observability`、`messageHandler` 或 `HandleResult`。
- 六份权威 Runtime 文档统一使用 `Interaction` 表达 Framework Strategy，只有存在真实对照维度时才使用额外限定词；精确文档守卫禁止把 Interaction 与含混的原生/native 限定组合，不误伤 provider wire、OS path 等确有区分对象的 native 语义。

## 10. 删除清单

以下条目已在对应阶段清零，并由永久门禁防止回流：

- 临时 Agent module path imports；
- root chat GOAP single-action wrapper；
- `TurnProcess` 和旧 Turn lifecycle vocabulary；
- `adapter/agentexec/turn` 中第二 controller/pump/registry；
- old suspension private JSON decoder；
- old process-tree private codec；
- old `toolloop` promotion/deferred glue；
- old AgentTool delegation wrapper；
- Application/Domain 中 `execution` package path；
- Product-facing `ProcessID`/`TurnID` terminology；
- Delivery 对 concrete agentexec/persistence/infra 的直接控制；
- `component` umbrella（P9 已删除并由永久 guard 禁止）；
- temporary architecture exceptions；
- alias、dual wire、compat decoder、空目录和历史 TODO。

## 11. 验收覆盖矩阵

| 场景 | Domain | Application | Agent Framework real vertical | Persistence | Delivery |
|---|---:|---:|---:|---:|---:|
| start/terminal | 必须 | 必须 | 必须 | 必须 | 必须 |
| stream + Delta drop | — | 必须 | 必须 | Transcript 必须 | 必须 |
| ToolCall/result/usage | accounting/tool | 必须 | 必须 | 必须 | 必须 |
| waiting/checkpoint | run/interrupt | 必须 | 必须 | 必须 | 必须 |
| restore/resume | run/interrupt | 必须 | 必须 | 必须 | 必须 |
| approval allow/deny/remember | approval/tool | 必须 | Tool input | 必须 | 必须 |
| question choice/text/cancel | interrupt/transcript | 必须 | Tool input | 必须 | 必须 |
| steer | run command | 必须 | 必须 | 按产品事实 | 必须 |
| Delegate child | run/lineage | 必须 | 必须 | 必须 | 必须 |
| subtree cancel | run/interrupt | 必须 | scoped Effect settlement | 必须 | 必须 |
| cancel/deadline/lost | run outcome/tool failure | 必须 | tree-wide Effect settlement | 必须 | 必须 |
| rollback/fork | conversation/transcript | 必须 | parked cleanup | 必须 | 必须 |
| recovery after restart | run/session | 必须 | restore probe | 必须 | projection |

## 12. 当前结论

- P113 已完成第一批所有权纠偏：Runtime 内建 Tool identity 直接归 `domain/tool`，原 `adapter/toolname` 物理删除；Bootstrap-only notification connection 与既有 observation seam 合并，原 `adapter/notification` 及其 one-method interface 壳物理删除；runmaintenance 只供测试调用的 receiver wrapper 同步移除。该批没有改变 Tool 字符串、Runtime Protocol、SQLite、Artifact 或 Agent Framework 合同。
- P113 合法构造纵切已完成：Runs、Sessions 与 durable Run-segment effects 在 constructor boundary 拒绝缺失/typed-nil required dependency，运行期不再把非法对象降级成 use-case unavailable；Session Plan 是 boundary+replacement 的完整可选 capability。Runsegment 的 durable transaction effects、terminal maintenance 与 live workspace notification 已成为三个独立 owner，后两者不再扩大持久化 Config；Title maintenance 启用时必须同时具有 Session title use case、generator 与 lifecycle task launcher。公共合同与持久 shape 未改变。
- P113 operation capability 纵切已完成：87 方法 `delivery/operation.Service` 物理删除，catalog 中每个注册只声明并恢复自己调用的匿名单方法 capability；Endpoint 仍是 HTTP 与 embedded 的唯一 type-erasure waist，idempotency replay 和 capability negotiation 也只消费各自精确窄能力。生产 Server 的 87/87 覆盖、无额外导出 handler 与旧胖接口缺失由语义架构门禁双向证明，focused test fake 不再借 nil interface embedding 伪造全集实现。公共 Protocol、生成合同与持久 shape 未改变。
- P113 core state owner 纵切已完成：`interactionSession` 不再直持 mutex/once/waitgroup/map/channel/Context，而由 Process/Delegate state、session lifetime、child projection、accounting、Tool repetition、committed reply 与 Segment clock 分别拥有同步不变量；跨 owner checkpoint 使用固定锁序保持 usage+pending-steer 同代。Runs 的 process-local Segment task/replay/live registry/teardown 与 durable-write-before-notify 分别归 `segmentLifecycle`、`runPublications`，`Coordinator` 不再平铺这些 Dependencies/ProjectionPorts。两者都留在原 package，没有新微包、store façade 或第二生命周期 owner；公共合同与持久 shape 未改变。
- P113 lifecycle/fitness 纵切已完成：`bootstrap.OpenInstance` 创建并拥有唯一 Runtime root，Assembly、operation、Interaction、Toolset、LSP、MCP/OAuth 与 workers 只消费该 lifetime；startup/request context 不再成为共享 resource owner，agentexec 还把 owner cancellation 重新绑定到 Framework 有意 detached 的 effect dispatch context。内部 nil Context fallback 已收敛为显式错误/Go programmer contract。架构门禁从历史 identifier/精确文件台账改为 process-root、DAG、唯一 owner、合法构造和公共/持久 shape；production executable orchestration 使用统一复杂度预算且不能靠移动文件规避。该门禁实际推动 Bootstrap 单体装配按 foundation 与 Session/Run core 分阶段，而非增加微包或函数白名单。
- P113 最终验收已完成：production-Go directory census 从 109 降到 107，删除的两个 package 没有换名补回；七个纵切没有引入 alias、shim、forwarder、compat path、新微包或 service locator。Runtime public Go/Protocol、87-operation catalog、SQLite epoch 75、Session Artifact v19 与 Agent Framework public shape 零变化。standalone build/vet/staticcheck/lint/deadcode/tidy/test/full race/generator、architecture/documentation facts 全绿。
- Runtime 产品领域、协议、持久化和工具能力大部分保留；
- 执行 Framework integration 是主要 Rewrite 区；
- P8 已将 Agent Framework vertical 原子切为唯一生产 owner；root、managed Delegate child、waiting subtree、termination、unknown 与 recovery 均由真实 Bootstrap consumer 验证；
- Agent Framework Baseline 20 已提供 P4–P7 公共合同与通用 RawMessage/byte JSON Schema 修订，并以 canonical module pseudo-version 被 Runtime standalone 直接消费。Framework 没有引入任何 Runtime 产品、持久化或 transaction 抽象；
- Runtime 对原框架 source/test/module dependency 与临时 module path 已归零；唯一 `agent` Framework 仍只拥有中性合同，产品 Run、Store、transaction、WorkingContext composition 与 recovery policy 均留在 Runtime；
- P11 删除迁移期 execution/port 快照文档；P12 继续删除已完成的架构清洗台账。当前架构、端口与工具接线分别只有 `ARCHITECTURE.md`、真实 consumer code/GoDoc 和 `TOOL_SYSTEM.md` 一个 owner，历史实施事实归 Git，不保留第二套错误现状；
- P12 全量静态审计捕获的格式漂移与嵌入字段冗余已在各自源码 owner 治本清除；Runtime 与 Agent Framework 的 tracked production TODO/FIXME/HACK、旧 Framework 路径、旧 replay scope、空文件、空目录和内部死代码均为零。
- P12 最终行为矩阵证明 Runtime root/child Interaction、authoritative model/tool、waiting、restore、resume、steer、unknown、recovery、rollback 与全部外环能力仍自洽；SQLite 当前 epoch 75 由 baseline consistency test 永久守卫。`interactioninput` 是 pending continuation/prompt/resolution 的唯一 ACL 与 codec owner，原单消费者子包和第二 decoder 已删除；三个 Runtime strict-codec fuzz owner与 Agent Framework 十三个 wire/state fuzz owner均全绿。
- P14 完成了当时的 Runtime 内部职责与命名精修；P113 已 supersede 其历史 identifier 与精确文件位置 ledger，只保留仍表达当前 package/词汇/state owner、DAG、公共合同或持久化语义的 guard。当前命名和复杂度门禁验证行为边界，不冻结局部实现布局。
- P15-02 再次反证 Domain/Application 后，将 system-invariant 的说明性 catalog 从生产 Application graph 移至 `cmd/contractgen`，而真实跨聚合不变量继续由对应 write-set 和 integration fixture 独占；无消费者导出链已删除，结果值、水位与 child/executor 语言按真实语义统一。Domain 仍只拥有聚合、值与纯策略合同，Application 仍独占 I/O ports；本批没有新增 Framework concrete type、协议 wire、SQLite shape 或消费者兼容路径。
- P15-03 反证 Adapter/Infra/Delivery/Bootstrap 后，executor checkpoint 在 `agentexec` 之外重新成为纯 opaque bytes：Bootstrap 与 runsegment 不再复制或解释 TreeSnapshot shape，持久化测试只证明 checkpoint envelope 的原子保存、替换和删除。非 Framework 层的 member/request/child-Run 词汇、LLM catalog/profile 命名、execution scope 文件职责与 SQLite epoch 事实已同步；新增 architecture guard 禁止外环重新拼装 Framework tree wire。本批没有改变协议、SQLite shape 或 Agent Framework 合同。
- P16-01 已完成完整 Run aggregate 纵切：`domain/run.Run` 是 lifecycle、frozen admission facts、cumulative metrics 和 terminal facts 的唯一 mutation owner；`domain/transcript` 不再定义 Run 或跨 Run/Tool 的通用 Problem。SQLite 只重放并验证 aggregate transition 后执行 CAS，不再以裸 State 形成第二状态机。Application 的 Run tree/Pending/checkpoint/Goal/Conversation/Transcript 原子 write-set 仍留在原 owner，Agent Framework concrete import island、Protocol wire、SQLite epoch 和消费者实现均未改变。
- P16-03 已完成 Plan aggregate 纵切：`domain/plan.State` 是 Steps、replacement、revision 与 updated time 的唯一 mutation owner；`application/plans` 读取当前状态并形成 immutable CAS replacement，Tool Adapter 只调用该用例，SQLite 保存上层已决定的精确状态。Session fork/rollback/restore 继续由 Application 组织跨聚合原子 write-set，restore 不再删除 Plan row 后重置 revision；Protocol、Artifact、SQLite shape 与 Agent Framework 边界均未改变。
- P16-04 已完成 Session aggregate 纵切：`domain/session.Session` 以私有字段独占 fresh/scheduled construction、edit、generated-title arbitration、fork、restore workspace installation、revision 与 time；Application 继续拥有 workspace admission、identity/clock、Run opening、mutation claim 和跨 aggregate write-set。SQLite SessionStore 只保存 Domain/Application 已决定的 exact initial/replacement，旧 Create/Ensure/Patch/Fork/Restore/field-setter owner 已删除；portable restore 保持目标 revision 单调，Protocol、Artifact、SQLite shape/epoch、Agent Framework 和消费者均未改变。
- P16-05 已逐一冻结 21 个 Domain package 的独立词汇、变化原因、消费者和 import DAG 理由：没有按体量机械合包，也没有保留无 owner 的微包。Live workspace projection 与 CWD admission sentinel 收回 Workspace Application，Schedule availability 收回 Schedules Application；Domain 文件名/GoDoc、Interrupt 零值和 codebase ranking 边界同步精修。新增永久 guard 要求每个 Domain package 的目录名、package 名、边界说明和直接测试共同存在；Protocol、Artifact、SQLite、Agent Framework 和消费者合同均未改变。
- P16 已完成：Run、Transcript Item、Plan、Session 的单 aggregate 行为均只有一个 Domain mutation owner，Application 继续独占跨 aggregate 和外部 lifecycle。最终 standalone/build/vet/test/race/lint/staticcheck/deadcode/generator、三个 strict codec fuzz、architecture、文档与 hygiene 门禁全绿；没有兼容路径、协议/Artifact/SQLite/Agent Framework 变化或消费者接线项。
- P17 已完成：package 总数从 115 收敛到 100；shared capability、Toolset、Application、Adapter、Infra 与 Delivery 的伪边界和错层完成当期收敛；in-process transport 因零生产消费者且不可越过 Go `internal` 边界而删除。全 internal package GoDoc/目录名/零行为 umbrella 永久门禁、全量 race/lint/staticcheck/deadcode/generator 与空残留扫描共同通过。
- P18 进一步以行为 owner 而非 import 数量反证根级 package：pagination 的全部策略调用属于 Application read，taskgroup 的启动者属于 Application coordinator，Bootstrap/Delivery 的引用不产生共同所有权；opaque-token 的两个 payload owner同属 Application continuation。三者已移动到 `application`，旧根路径永久禁止；真正跨环的根级 pure capability 只剩 completion、HTTP origin 与 idempotency。
- P19-02 已建立唯一公共 `protocol` owner：外部 Go consumer 与 HTTP 共用相同 values、strict validation、version 和 client-visible problem identity；服务端 method interface/context/enrichment、JSON-RPC numeric code、reflection shape walker、enum/sample artifact catalog 分别收回私有 `operation`、`dispatch`、`contractshape` 与 `contractcatalog`。旧 internal protocol path 已物理删除，无 alias、shim、双份 DTO 或 generator internals 暴露。
- P19-03 已建立唯一私有 typed operation pipeline：method catalog、strict request/result/event validation、capability gate、safe problem projection、durable idempotency 与 run-stream reattach 由 `delivery/operation` 独占；HTTP dispatch 只适配 JSON-RPC envelope/numeric code/frame。幂等 replay payload 具有显式版本且未知形状 fail closed；旧 dispatch registry、capability、idempotency owner 与 catalog forwarding 均已删除。
- P19-04 已建立唯一私有 Runtime instance owner：canonical data-directory advisory lease 早于 SQLite/recovery，`bootstrap.Instance` 统一拥有 Host、operation endpoint、Server projection、scheduler 与 reverse-order retryable Close。HTTP cmd 不再组装 Server/recovery/worker，HTTP transport 只消费 instance-owned Endpoint；同路径、symlink alias 和第二进程争锁均被拒绝，未完成 Close 不释放 lease。
- P19-05 已建立公共 concrete `embedded.Runtime`：全量 operation 以类型化 Go 方法直接消费唯一 operation pipeline，公开面只含 `protocol` values、精确 options、`iter.Seq2` streams 与完整 Open/Close lifecycle。外部 module 无需且不能依赖 Runtime internal；AST+reflection 门禁保证 operation 覆盖和签名不漂移，真实集成保证 protocol、idempotency、notification、stream close、directory lease 与 reopen 行为。
- P19-06 已冻结完整公共 Go 合同：生成的 `contract/go-api.json` 来自真实 Go type information，架构门禁同时守住唯一 public package 集合、visible import、operation/method parity、external-module compilation 与 artifact digest。稳定失败由 protocol sentinel + `ProblemError` 提供准确的 `errors.Is`/`errors.As` 语义；README/consumer handoff、准确 capability 文件和最终质量矩阵均已收口，P19 不留下 consumer shim 或 transport duplicate。
- P21 已以真实 DeepSeek、SQLite、mock 与 PTY 四类证据闭环普通/连续/并发 Tool、授权五种选择、问题 HITL、resume、取消、重连、重复/乱序和 crash/restart。Runtime adapter 只把产品取消作用域翻译为对应 Agent Effect context，未把 Run、授权、Store 或 transaction 职责泄露进 Framework；终态没有 checkpoint、open interrupt、unknown Effect 或未完成 invocation。
- P22 已在 WorkingContext 唯一组合 owner 内为 base/Knowledge/Memory/Agent document/Plan/hook 建立私有 typed provenance；预算后的实际来源与 instruction/data purpose 随 Message/Part metadata 进入 opaque checkpoint，文本、Protocol、Artifact、SQLite 与 Agent Framework 合同均未变化。
- P23 将 WorkingContext provenance 从“调用点同步填充的数据”收敛为行为所有权：source kind 唯一派生并验证 purpose，预算后的 Memory/Agent document fragment 原子拥有 text+sources，hook result 自己应用拒绝/注入，`WorkingContextComposer` 统一拥有 system/Plan/recall/hook。公开 DTO、Protocol、Artifact、SQLite 与 Agent Framework 合同均未变化。
- P25 已关闭 P23 暴露的发布顺序缺口：Agent Baseline 20 canonical 发布后 Runtime 一次性前移真实 module version，原先两条冷恢复 consumer 回归转绿；Runtime 没有复制 Schema、清洗 provider metadata、增加 `replace` 或跳过回归。
- P24 以真实 Desktop、Runtime 与隔离 SQLite 将 read model、query invalidation、Git observation、Segment activation/cancel 和 boot recovery 重新对账：终态无 started invocation、非终态 Run、open interrupt 或 checkpoint，崩溃恢复的 Run/Goal/invocation 使用同一终结时间；Agent production graph 仍不 import Runtime，Framework concrete type 仍只存在于 `adapter/agentexec`。
- P25 第二轮反证进一步把前端 live/durable projection、workspace retarget/reconnect、Run/Item/Plan late-event 单调性与 JSON-RPC envelope 歧义收回唯一 owner：真实 Runtime 对合法请求返回 200、notification 返回 204，对 duplicate/unknown、空 method、显式 null/numeric id、client response 与 mixed envelope 返回 400；真实 Desktop 在 active Run `SIGKILL` 后把 Run/model invocation 收口为 lost/unknown，无刷新重连后继续完成新 Run。真实 ask_user、两次 Plan 替换、Goal completed/blocked 与 blocked 后普通 Run 均完成，终态无 open interrupt/checkpoint/active Run。Agent Baseline 20 已发布，Runtime 绑定远端真实 pseudo-version 后 standalone tidy-diff/build/vet/test/race/staticcheck/lint 全绿；原 Baseline 18 下失败的两条冷恢复 consumer 回归转绿，且没有引入 `replace`、Runtime sanitizer 或 Framework 抽象泄露。
- P26 将 Tool 可见 lifecycle 与真实 executor interval 分离：Reducer 是 exact execution duration 的唯一计算 owner，Transcript terminal fact 持有 optional duration，SQLite 只编码、Delivery 只投影，恢复无法证明时保持 unknown。Protocol `2026-08-12`、Artifact v17 与 Desktop generated consumer 已同步；真实 HITL lifecycle 31.160s、execution 2.016s、UI `2s`，Goal/Plan/普通 Run 后续矩阵与全部质量门禁全绿，Agent Framework 边界未变化。
- P27 以真实依赖图收缩发布信任面：Frontend clean install 解析到修复后的 Mermaid/DOMPurify/NanoID；Runtime 的 Ollama chat/embedding 在 Infra 内复用既有 OpenAI-compatible protocol，移除完整 Ollama 服务端 module。`npm audit` 与 Runtime 可达 `govulncheck` 均为零，真实 HITL/Tool/Goal/Plan 崩溃恢复、双 Session 隔离、约 92.6 万次 codec fuzz 和终态数据库不变量全绿；Agent/Application/Domain/Delivery 公共合同未变化。
- P28 将同一依赖边界修复推进到独立 `models/ollama` owner：provider module 以私有窄 wire 保留原生 `/api/chat`、`/api/embed`、NDJSON stream、Core extensions 与 HTTP failure 语义，同时移除完整 daemon repository 及其 server/auth/ordered-map 依赖。该模块 `govulncheck` 从 8 条可达路径降为零，公开构造器与 Runtime/Agent 合同不变化。
- P29 关闭 CLI standalone consumer 的发布图缺口：CLI 直接绑定已推送 commit `420f627f131a` 对应 Runtime pseudo-version，并随其消费 Agent Baseline 20；旧 Runtime→`models/ollama`→daemon 依赖链从 standalone graph 删除。CLI 全量 normal/race/static/lint/build/vet 与 `govulncheck` 全绿，未使用 workspace `replace` 或夹带 CLI 功能改动。
- P33 关闭 Schedule 默认工作区更新的协议表达缺口：Protocol/Delivery 精确拥有保持、设置、回到默认三态及互斥校验，Desktop handwritten SDK 直接消费生成 `UpdateScheduleRequest`。Domain/Application 仍只拥有 Schedule/CWD 行为，Agent Framework 未接收任何 Runtime workspace、wire 或 UI 抽象；真实 UI/SQLite 证明失败路径已持久化收口。
- P34 将 HITL 的最终显示事实收回 Runtime：Question Transcript 在 accepted resume claim 的同一事务内不可变补充答案，Artifact v18/Protocol/SQLite codec/Delivery/Desktop 只消费该事实；取消保持未回答。Goal 在 fresh Start 冻结协商能力，Resume 验证调用方覆盖，自治 Run 与 Goal 内 `create_goal` 继承它；执行上下文 carrier 留在 Runtime adapter，不修改 Agent Framework。Desktop 的未知 raw HTML 在 Markdown AST 边界按字面量展示，不把 sanitizer 或 UI 语义下沉 Runtime。
- P34 的真实 HTTP 反证进一步关闭 Goal resume 与 parked Run 的 durable admission 缺口：`application/runs` 的 startable observation 同时等待本地 gate 和权威 non-terminal Run，committed lifecycle change 只负责唤醒，状态仍由 durable projection 重读；Goal 不再把同一 waiting Run 误判为新 Run start failure。
- P35 将这个 wake-only 原则补齐到完整生命周期：Runtime Session change generation 只由活跃 Application waiter 持有，多观察者共享当代通知并以幂等 disposer 释放，无 waiter 时不保存 Session 条目；它不是 durable state/cache，也不承担 reservation。
- P35 同时关闭 Desktop 的 snapshot→subscribe TOCTOU 与 accepted boundary 混淆：initial recovery/replay reattach 遇到 terminal/waiting/stale 时经 Agent application port 重读 durable projection；只有 Run ack 前拒绝进入 command error/HITL rollback，ack 后 stream failure 不否定已提交命令。Runtime DTO、Store、transaction 与 Agent Framework concrete type 均未进入 Agent context。
- P36 关闭 Desktop 旁路 file watch 的静默失联：Session workspace adapter 只把 `session_not_found` 解释为权威 unavailable，Workspace event application 对其他瞬时失败执行同 identity 可取消退避；retarget/dispose 使旧 generation 失效，global topics 与 file watch 继续由唯一 `runtime.subscribe` consumer 拥有。该恢复策略没有进入 Agent context，Runtime/Protocol/Artifact/SQLite shape 均未变化。
- P37 纠正 Goal 完成态不可观察的错误协议假设：Domain/Application 继续分别拥有 terminal fact 与最终 settlement，Delivery 只将该窗口投影为 `completing`；生成合同与 Desktop 自有 read model 同步，banner 保留目标但不提供 stop/resume，最终清除仍经 invalidation 收敛。真实 HTTP 回归在 `report_goal_outcome(completed)` 后卡住 owning Run，证明 `goals.changed → goals.get(completing) → null` 全链闭合；Agent Framework、Artifact 与 SQLite 均未变化。
- P38 关闭 Desktop 失效 Session 深链接的假故障：Runtime Gateway adapter 识别协议 `session_not_found` 并向 Agent application port 返回 absent snapshot，projection refresh/history action 只消费中性缺失语义；transport/protocol 等运行失败仍然 reject 并进入原 recovery error owner。前端不 import Runtime DTO，Agent Framework 和 Runtime 合同未变化。
- P39 关闭 Desktop 多客户端 Session 删除后的幽灵导航：启动记忆恢复仍为一次性动作，但每次权威 Session 列表读都对账 active/open 集合；delete 在 commit 前不乐观改写身份集合，Adapter 把已缺失视为目标达成；rename/favorite 失败的快照回滚后会重读 Runtime，不会复活并发删除的行。Runtime/Agent Framework 零变更。
- P40 关闭 Desktop 深链/history active Session 未进入 held-open lifecycle 的断裂：mounted driver 在 material view 前建立 open/last-session memory，权威 selection model 保证存活 active 不被 stale open cleanup 清除；close/reconcile 同步纠正 cold-start seed，relocate 的条件写失败重读 Session truth。改动止于 Desktop Adapter/Application，Runtime 与 Agent Framework 零变更。
- P41 关闭 Desktop 双客户端 HITL resume 的延迟拒绝反转：Application 的 staged batch 随 standing projection 淘汰而幂等退休，并将“opening 已被更新视图取代”的中性事实交给 Adapter 抑制过时 command error；Adapter 不识别 HITL wire error，延迟本地 ack 也不能覆盖已物化的远端结果。Goal lifecycle 命令无论成功或结算不明都回读自身 query，回读失败不替换原命令错误。Runtime、Protocol、SQLite 与 Agent Framework 零变更。
- P42 关闭 Desktop 未消费 Runtime command replay 合同的全局缺口：RPC SDK 对全部生成 command policy 统一完成有界同-key settlement recovery，流式 opening 的每次 attempt 使用并清理自己的 event stream；Agent Application 不接触 `MutationPromise`、transport 或 idempotency。Session create 的 attempt deadline 归 Adapter，provisional draft owner 跨冷启动持久化，而“刚在本进程创建、可跳过首次 durable read”的 freshness 独立保持 ephemeral；冷启动从权威 snapshot 判定毕业，导航只让 fresh create 补充尚未出现的列表身份。Runtime、Protocol、SQLite 与 Agent Framework 零变更。
- P43 关闭 Desktop Run opening 永久 pending 与 cancel 迟到响应反转：RPC Agent Adapter 对 replayable opening 建立两次有界 delivery attempt，同 key 但各自 fresh signal，ack 后只解除握手 deadline、不解除长流的 session ownership；传输失败投影为稳定产品问题而不进入 Application。Cancel controller 用发起 revision 条件提交快照，失败后只询问 Application 的权威 terminal 事实，另一客户端先结束时不识别 wire error 也能收敛。Runtime、Protocol、SQLite 与 Agent Framework 零变更。
- P44 关闭 Desktop Session 历史重写与条件写收敛缺口：rollback 不再绕过 mounted Session 的 stream/snapshot 串行 owner，重复动作不会双重截断或执行两次 follow-up；Runtime 返回的 dropped user input 在 Adapter 转成 AgentInput 后驱动 edit/regenerate。第一轮没有 checkpoint 时 files/both 不再被省略字段误投影成 history-only 全量删除。rename/favorite/relocate 共用 Session 级 revision settlement，失败只回滚自己仍拥有的乐观字段。Runtime、Protocol、SQLite 与 Agent Framework 零变更。
- P45 关闭 Desktop Goal/Plan 的能力不可达、命令竞态与抽象泄露：blocked/paused Goal 可从 composer 原位替换，预算耗尽不再显示 Runtime 必然拒绝的 Resume；Goal command 按 Session 串行、有界同-key settlement，并以 typed response 单调提交后重读，provider 与 wire mapping 均回归 Goal Adapter。Plan shared snapshot/event 在 Agent Adapter 转为中性 Plan domain，bootstrap fold composition 不再伪装成公共产品 surface；旁路 `goals.changed` 只触发点名 Session。真实 Runtime 证明 Goal replacement、budget block、Plan live/cold projection 与 SQLite 权威状态一致。Runtime、Protocol、SQLite 与 Agent Framework 零变更。
- P46 关闭 Desktop Agent 四条剩余 Runtime DTO 泄露路径：SDK 中性 facts 承载 live event、durable Run/Item/Interrupt 和 cancel projection，wire 不变量只在 Agent Adapter 校验，pending work provider 也回归 Agent bootstrap；静态发布边界门禁防止 Application/Domain/public 与 SDK event contract 再次导入 RPC。真实冷启动进一步发现 Composer catalog 默认模型先于 active Session summary 落地并形成错误 override，现按显式偏好、durable Session 模型、catalog fallback 的所有权顺序解析并等待未决 summary。真实两步 Plan、冷重载与第二次 Run 均保持 `deepseek-v4-pro`；Runtime、Protocol、SQLite 与 Agent Framework 零变更。
- P47 关闭 Workspace subscription opening 的取消传播缺口：retarget/dispose 的 lifecycle signal 从 Workspace Application 经 Runtime Adapter 与 SDK 到达 `workspaces.resolve`，旧 watch-root resolution 不再阻塞唯一重连循环。Runtime Delivery 同时将 `watchIds` 与非 `files.changed` topics 的跨字段矛盾视为不可猜测输入并扩大到完整 subscription resync；watch、wire 与 HTTP 取消语义没有进入 Agent 或 Runtime Domain/Application。真实 HITL、Plan、Goal、旁路 Session、Git 文件事件与 Runtime 重启恢复均收敛。
- P48 关闭 Desktop “丢弃旧结果但不取消底层身份读取”与 Schedule mutation 返回事实未消费的缺口：Workspace Application 的每代 signal 经 Adapter/SDK 到达 `sessions.get`，retarget/dispose 不再积累孤儿请求；`schedules.runNow` 的 Session/Run identity 由 Schedule 用例保留，并经 Agent 公开会话动作进入 Agent 自有 durable recovery，Schedule 条件更新则先提交后端返回的新 revision 再重验。Runtime DTO/transport 没有进入 Agent 内层，Runtime、Protocol、SQLite shape 与 Agent Framework 均未变化。
- P49 关闭 Desktop 非 void mutation 返回事实未消费及连续写乱序覆盖的系统性缺口：Provider、Approval、MCP、Agent Memory、Codebase 各自的 Adapter 将 Runtime response 投影为中性产品资源，所属 Application 按真实冲突域串行并先提交权威 fact 再失效重验；通用 serial queue 对失败后续跑与 keyed independence 有直接回归。MCP data provider/wire projection 回到 MCP context Adapter，defaults 和 Agent 内层均不接触其 wire；Provider `requiresBaseUrl` 驱动真实必填校验，UI 用 saved resource 重建规范化草稿。Runtime、Protocol、SQLite shape 与 Agent Framework 均未变化。
- P50 关闭 Knowledge 丢更新、空文件不可创建及 Knowledge/Hook 多窗口静默陈旧：Knowledge Domain 只持有 scope/revision conflict，Application 拥有条件更新与 committed invalidation，Infra 独占内容 revision 和同目录原子替换，Delivery 独占 Protocol problem/topic 投影；Desktop Workspace/Hook Adapter 将 wire 转为中性模型，干净编辑器跟随事件、脏草稿保留且冲突后显式 rebase，Hook intent 按 project 串行并有 UI latch。Agent/Agent Framework 未导入 Runtime、Knowledge revision、Hook topic 或前端状态，SQLite/Artifact/Framework shape 不变。
- P51 将 Knowledge containment、read/revision/atomic replacement 收敛到唯一 physical identity，并以中性 advisory lease 保证跨进程只有一个 stale writer 提交；权限继承、file/parent sync 与 crash staging recovery 均留在 Infra。Application/Delivery 只投影既有 path policy/problem，Agent/Agent Framework、SQLite、Artifact shape 不变。
- P52 让外部进程对 Knowledge/Hooks 的 create、write、atomic rename、remove 与 symlink target 更新进入既有 committed invalidation：Infra 只观察精确路径指纹，Workspace Adapter 独占文件布局，Application 只接收语义资源，Delivery 只投影既有 topic。API write 在发布前接受 exact path 基线以去除 filesystem 回声；Agent/Framework、Protocol、SQLite、Artifact shape 不变。
- P53 关闭 Desktop Goal lifecycle mutation snapshot 的第二事实源：Application 不再用 `updatedAt` 猜测延迟响应的新旧，也不直接写 standing cache；command port 只返回中性 Session receipt，完整 wire 响应由 Goal Adapter 消费，长期状态唯一来自 `goals.get` data provider。同 Session 串行与成功/失败后的权威回读仍由 Goal Application 拥有，Agent/Agent Framework、Runtime/Protocol、SQLite/Artifact shape 不变。
- P54 关闭 Checkpoint source index 与 shadow backstop ignore 的 ownership 冲突：`infra/checkpoint` 先以 `ls-files --exclude-standard` 唯一选择 source-tracked / untracked-unignored 路径，再应用类型与大小策略，最后只对精确候选 force-add；合法跟踪的 build 输入不会被 shadow exclude 二次否决，同目录 ignored generated sibling 仍不归 checkpoint。Run/Goal maintenance 继续显式报告失败，Agent/Agent Framework、Protocol、SQLite、Artifact shape 不变。
- P55 关闭 Git subprocess 继承宿主仓库控制面的缺口：中立 `infra/gitprocess` 是 Runtime 内唯一 Git OS-process owner，剥离父进程全部 `GIT_*` 后再安装命令显式参数；checkpoint source/shadow index、workspace VCS read 与 watcher 不再被 foreign repo/index/object/config/pathspec 重定向。Watcher 把 Git metadata directory 收敛为 physical identity，architecture guard 禁止 owner 外直接启动 Git。Application/Delivery/Agent/Framework、Protocol、SQLite、Artifact shape 不变。
- P56 关闭 HTTP sidecar 未纳入唯一合同和 Desktop 产品消费的缺口：Runtime HTTP Delivery endpoint registry 同源驱动 handler/auth/info/contractgen，生成 manifest/TS/validator/reference 精确覆盖 info/liveness/readiness；Desktop Runtime context 以中性 service inspection port 拥有 timeout/coalescing/dispose/retry，Settings 不接触 HTTP DTO。Workspace event Adapter 同时把客户端可折叠 topic 与 discovery 声明取交集，旧 Runtime 不再因新增 topic 拒绝全部旁路失效流。Agent/Agent Framework 未导入 sidecar、HTTP、service status、topic negotiation 或 UI 状态，Runtime Application/Domain、SQLite、Artifact shape 不变。
- P57 关闭 Desktop 冷启动一次性 discovery、stream 断线滞后感知与相同 watch 重复订阅：Runtime context connection supervisor 同代聚合三条 sidecar 与 RPC discovery，统一拥有 deadline、退避、健康巡检、能力撤销和恢复；Workspace 只经公共 service port 上报 stream 连接证据。Session identity 与同 Session projection 采用不同 retarget 语义，active cwd 未变化时不重开 stream；旧 stream generation 不能清除新 generation ownership。Agent/Agent Framework 未导入 sidecar/RPC/重连或 Workspace 生命周期，Runtime Application/Domain、Protocol、SQLite、Artifact shape 不变。
- P59 将 Skill 提交后通知并入 Application 唯一 invalidation vocabulary，删除 Bootstrap/Delivery 的专用空值旁路；同时让 `codebase.search` 的隐式 reconcile 公开 indexing/ready/error 两端失效，显式 reindex 继续由 Coordinator operation 投影 transient indexing。Delivery 仍独占既有 wire topic，Desktop 继续只消费公开 query identity；Agent/Framework、Protocol、SQLite、Artifact shape 不变。
- P83-02 关闭 Runtime event 新代已建立、mounted Agent projection 却仍被旧不合作 Run stream 永久占有的恢复缺口：Workspace global resync 以中性 `replace-live` ownership 撤销旧 opening/iterator，Agent Adapter 有界回收迟到 stream 并释放 live owner，唯一 snapshot coordinator 随后统一重建 HITL、Plan、Run 与 Tool；Goal 和其他 query read model 继续由同一 global Query invalidation 收敛。普通增量事件仍在 live owner 后串行，RPC/Runtime DTO、transport、QueryClient 与 persistence 没有进入 Agent Application/Domain，Go Runtime 与 Agent Framework 合同未变化。
- P83-03 关闭旧 renderer 在读到 mutation owner 后、replacement 接管前后的非原子关闭与 settlement 窗口：Desktop RPC journal 以 generation-addressed records 追加 takeover，旧 owner 只能写自己的低代；接管代的确定结算保留 settled fence，retention/Runtime-store 清理也只删除到已观察代际上界，因而迟到 dispose、成功、未知失败和 stale cleanup 都不能覆盖、删除或复活 successor。已发布 v2 entry 单向迁移为 generation zero；Host 仍只拥有 opaque KV，Runtime、Desktop Agent Application/Domain 与 Go Agent Framework 均不感知 journal schema 或 renderer lifecycle。
- P83-04 关闭 Runtime-event duplicate/迟到帧倒退 Workspace recovery watermark 并反复替换 mounted Agent generation 的缺口：每条 subscription generation 只接受严格递增 sequence，前向 gap 触发一次 global authoritative resync，低于或等于 high-watermark 的旧帧被丢弃；新连接仍从零建立自己的序列边界。Goal query 与 HITL、Plan、Run、Tool material projection 因而在同一 gap snapshot 后只消费单调新失效；wire、transport、Runtime 与 Agent Framework 合同未变化。
- P83-05 关闭 Tool 外部调用已开始而 durable Transcript read model 尚不存在的事务缺口：Runtime Run Application 现在把 running Tool Item 与 `ToolInvocationStarted` journal、Run progress 放入同一 authoritative transaction，提交成功才允许 Agent adapter 跨越外部调用边界。真实 stdio MCP marker 证明强杀前 `items.list` 可读 running Tool，Runtime 重启后同一 Item 原位收敛为 incomplete、Run 收敛为 lost；active Goal-owned Run 同时按既有恢复顺序只记账一次并暂停。SQLite 只执行 Application write-set，Desktop 只消费公开 read model，Protocol、Artifact 与 Agent Framework 合同未变化。
- P83-06 关闭 running Tool 已持久化后 terminal Item、invocation journal 与 Run lifecycle 仍可能分批提交的缺口：每个 Tool journal transition 必须在同一实际 write-set 携带同 identity Item，normal/lost/cleanup terminal 统一合并为一个原子 `EventCommit`，reducer 只在提交成功后接管终态 clone。事务失败会保留 started journal + running Item，而不会泄露 closure 或 finish；operational completed 与产品 Item success/classified failure 的双状态语义已显式冻结。SQLite shape、Protocol、Desktop、Runtime→Agent 防腐层与 Agent Framework 均未变化。
- P83-07 关闭同一 renderer 内旧 client settlement 与 `dispose → replacement reserve` 交错时复用并删除同一 mutation generation 的缺口：只有仍持有 active claim 的同一 journal instance 可以续用当前 generation，hot-swap replacement 必须追加 successor generation；所有 process-local claim 释放均按 journal owner 比较后执行，旧代不能删除 durable successor 或释放其内存 ownership。仍存活的同进程 twin 继续拥有独立 key，Host opaque KV、Runtime/Protocol、Desktop Agent inner ring 与 Go Agent Framework 合同不变。
- P83-08 关闭首次 mutation 响应不明后自动 replay 把旧 idempotency key 发送到 replacement Runtime store 的缺口：journal reservation 在每个 transport attempt 前重验 endpoint/namespace、retention、generation 与 active claim；明确 store replacement/successor ownership fail closed，discovery 或 Host storage 暂不可证明时保持 settlement unknown 与原 identity，待同 namespace 恢复后再 replay。通用 mutation 驱动、Runtime/Protocol、Host KV、Desktop Agent inner ring 与 Go Agent Framework 合同不变。
- P83-09 关闭 Desktop 完成 store ownership 校验后、同一 HTTP endpoint 在服务端 admission 前切换 durable store 的 TOCTOU：每个 replayable attempt 携带 journal 已证明的 `Idempotency-Namespace`，Runtime Delivery 将其映射为中性 operation option，endpoint 在 claim、参数解码和业务执行前与当前 store namespace 比较；不一致返回专用 problem 且零副作用，相同 namespace 延续原 replay。embedded public option、Protocol/CORS/生成合同随唯一语义同步，v0.11 不保留兼容旁路；Runtime Domain/Application、SQLite shape、Desktop Agent inner ring 与 Go Agent Framework 均未接触 transport metadata。
- P83-10 关闭 Runtime replacement 时旧 durable Session snapshot 不合作而永久占有 mounted projection 的缺口：Session synchronization coordinator 为每代 recovery 持有可替换 AbortController，Application snapshot boundary 对旧 Promise fail-fast 且观察迟到 settlement；signal 经 Agent Adapter 贯穿 items/runs/interrupts/plan 的全部分页 attempt和后续 run reattach。successor 以既有 view token 一次提交 HITL/Plan/Run/Tool，旧代不能写回；Goal query 继续由同一 global resync invalidation 收敛。RPC/Runtime DTO 未进入 Desktop Agent Application/Domain，Go Runtime、Protocol、SQLite 与 Agent Framework 合同未变化。
- P83-11 关闭 Runtime replacement 后旧 reattach subscribe opening 迟到返回、其 event iterable 被遗弃的资源所有权缺口：opening 与 generation abort 竞速，旧代立即释放 recovery ownership；迟到 iterable 会主动调用 iterator `return()` 并观察异步关闭，迟到失败同样被观察，且 successor view token 始终独占 projection/live-pump 提交。修复只位于 Agent Adapter 的 foreign-resource boundary，Desktop Agent Application/Domain、Go Runtime、Protocol、SQLite 与 Agent Framework 合同未变化。
- P83-12 关闭 active Run stream 异常 EOS 后 replay/tail subscribe opening 不合作而永久保留 live ownership 的缺口：初次 recovery 与 dropped-stream reconnect 共用唯一 Adapter opening boundary，abort 立即释放 pump，迟到 stream 主动 `return()`，cold projection read 复用同一 generation signal；successor durable snapshot 因而可统一重建 HITL/Plan/Run/Tool，旧代不能接管或写回。Desktop Agent Application/Domain、Go Runtime、Protocol、SQLite 与 Agent Framework 合同未变化。
- P83-13 关闭 stream settlement 后 exact `runs.get` 不合作而永久保留 live ownership、迟到 RunRef 又可能越代写回的缺口：pump exact read 与 generation signal 竞速并在 apply 前重验 cancellation，abort 会立即释放 active Run/segment 并发布唯一 idle boundary；successor durable snapshot 接管 HITL/Plan/Run/Tool，旧 read 只被观察而不能提交。修复止于 Agent Adapter，Desktop Agent Application/Domain、Go Runtime、Protocol、SQLite 与 Agent Framework 合同未变化。
- P83-14 关闭 mounted Goal 首次 read 在无 cached data 时被 global invalidation 复用旧 Runtime Promise、无法建立 successor generation 的缺口：global replacement 先取消 query generation 再 invalidation，并与 Agent material `replace-live` 共用边界；Data Provider query SPI 将 TanStack ownership signal 贯穿 Goal Adapter/SDK transport，旧 transport 即使不合作也不能向 cache 提交迟到 fact。Goal public facade、Desktop Agent Application/Domain、Go Runtime、Protocol、SQLite 与 Agent Framework 合同未扩张。
- P83-15 关闭 query-backed committed event 撞上首次无缓存 read 时复用事件前 Promise、一次性变化信号被旧快照吞掉的缺口：每个定向 query scope 都先 cancel 当前 writer 再 invalidate，保留原 key、Session exactness 与 scoped-resync 闭集；迟到 settlement 只被观察，successor read 独占 cache commit。非 Query 的 Agent material projection 与所有 Runtime/Protocol/SQLite/Agent Framework 合同未变化。
- P83-16 关闭常驻 pending-work provider 丢弃 Query generation signal、install-wide `interrupts.list` 首/续页 transport 在 cache writer 退休后仍保留旧 RPC/HTTP owner 的缺口：Agent Adapter 将同一 signal 交给 paged SDK，唯一 continuation policy 为所有 cursor attempt 复用它；abort 直接释放 correlation/fetch 且不发送第二取消协议。Agent Application/Domain、Runtime、Protocol、SQLite 与 Agent Framework 合同未变化。
- P83-17 关闭 Session usage chip 与 cross-session Usage pane 的自定义 queryFn 未消费 TanStack signal、事件替换 cache writer 后旧 `usage.session` / `usage.summary` correlation 与 HTTP fetch 仍存活的缺口：signal 经中性 Application port、Adapter 与 SDK transport option 单向贯穿；旧 read 立即退休，successor 独占 view commit。RPC/Runtime DTO 未进入 Application，Go Runtime、Protocol、SQLite 与 Agent Framework 合同未变化。
- P83-18 关闭默认 Session-list Data Provider 丢弃 Query generation signal、`sessions.list` 首页/continuation 在 cache writer 退休后仍保留旧 RPC/HTTP owner 的缺口：provider 将同一 signal 交给 paged SDK，唯一 pagination policy 为全部 cursor attempt 复用它；abort 精确释放整代 correlation/fetch，successor 独占 sidebar read model。Runtime wire 仍止于 defaults Adapter/SDK，Agent Application/Domain、Go Runtime、Protocol、SQLite 与 Agent Framework 合同未变化。
- P83-19 关闭 Session-derived Workspace project refresh 直接 invalidation 复用提交前 Promise、默认 provider/SDK `workspaces.list` 又丢弃 Query signal的双重代际缺口：projection commit 复用唯一 cancel-before-invalidate policy，signal 单向贯穿 transport；successor 独占 project catalog commit，旧 correlation/fetch 与迟到 project fact 同时退休。Session/Workspace 只经 public read-model facade，Go Runtime、Protocol、SQLite 与 Agent Framework 合同未变化。
- P83-20 关闭 active Session cwd retarget 后参数化 cache 已切换、旧 `workspace.changes.list` transport owner 却因 provider/bound SDK 丢弃 Query signal而继续存活的缺口：同一 signal 贯穿 change-summary request，cwd/event/Runtime/renderer 换代会同时释放旧 correlation/fetch，successor 独占 Header/Files read model。Runtime wire 仍止于 defaults Adapter/SDK，Workspace/Agent Application/Domain、Go Runtime、Protocol、SQLite 与 Agent Framework 合同未变化。
- P83-21 关闭 FileTree cwd/path 换代后旧 `workspace.files.list` continuation 继续存活的缺口：默认 provider 将 Query signal 交给 bound paged SDK，唯一 pagination policy 为首页与全部 cursor attempt 复用它；abort 释放整代 correlation/fetch，successor 独占树 read model。Runtime wire 仍止于 defaults Adapter/SDK，Workspace/Agent Application/Domain、Go Runtime、Protocol、SQLite 与 Agent Framework 合同未变化。
- P83-22 关闭 mounted Agent material view 由 `items.list`、`runs.list`、`interrupts.list` 与 `plan.get` 四个独立事务拼接的缺口：`sessions.snapshot` 由 Session Application use case 定义一致性与跨投影引用不变量，Persistence 在一个 SQLite transaction 中读取 Session/Items/Runs/open Interrupt/Plan，Delivery 只做 capability-preserving wire 投影；Desktop Adapter 删除旧四读路径，只消费这一响应并映射到中性 Agent facts。Goal 保持独立 `goals.get` owner，不为合并请求扩大 Agent material 合同；Agent Framework、SQLite shape、Artifact 与协议版本均未变化。
- P84 关闭 CLI/Desktop 各自内嵌 Runtime 却被 data-directory 全局单实例锁阻断，以及简单删锁后会出现的 Session/Goal/recovery 双 owner 缺口：setup lease 只串行 store/schema/config seeding；`adapter/runtimeownership` 以 OS advisory lease 实现 per-Session writer、physical working-tree Run shared/destructive exclusive、per-Session Goal drive 和全局 ordered recovery sweep ownership。单一 sweep winner 固定 Run-before-Goal；Run recovery 再竞争 Session lease并只清理实际接管的 checkpoint/child reservation，存活 Runtime 周期重验后可接管被强杀进程；Schedule/HITL 继续由数据库 claim 单赢。SQLite `data_version` 将其他 connection 的提交投影为 scoped full resync，使已挂载 HITL、Plan、Goal、Run/Tool 重读 durable truth。两个真实 embedded Runtime 共享同一目录/namespace、跨实例 event convergence 与进程强杀 lease transfer 已验证；公共 `ErrDataDirectoryInUse` breaking 删除，Protocol wire、SQLite epoch、Artifact、checkpoint 与 Agent Framework 均未变化。
- P85 关闭 Desktop 新建 Session 无法选择 CWD、只能隐式使用 home 的缺口：Wails v3 Desktop Host 独占 native directory dialog 与路径存在性/目录校验，Navigation Application 通过独立 picker port 将选择结果交给既有 Session create `{cwd}`，取消与失败均零写入且不回退默认目录；Work Index 提供产品入口。只读对照 Proma 右栏源码确认附件目录不等于 Session CWD；Lynx 现有 per-Session Context Dock 状态 owner 保持不变，只通过 Workspace Navigation port 暴露 `view.toggle-dock` / `Mod+Shift+B`，折叠与重开不丢标签。Runtime operation/Protocol/SQLite/Agent Framework 和 Desktop Agent inner ring 均未变化。
- P86 关闭 owner Runtime 强杀后 survivor 本地 Run recovery commit 无法被 connection-local `PRAGMA data_version` 观察、已挂载客户端可能永久保留旧 HITL/Plan/Goal/Run/Tool 投影的缺口：Run Recovery Application 从唯一原子 `RecoveryCommit` 派生 Session-scoped Runs/Interrupts/Sessions/Goals invalidation，且只在 durable commit 成功后发布；Bootstrap 仅注入 publisher，SQLite observer 继续只负责 foreign-connection full resync。真实 claimed-resume SQLite 恢复保持 accepted Question answer，删除隐藏 resuming row，原位闭合 conversation Tool call并标记 RunLost；事务失败和 live peer ownership 都不会误发通知。Protocol、SQLite schema、Artifact、Desktop Agent inner ring 与 Agent Framework 合同均未变化。
- P87 关闭根 Start/HITL Resume 已 durable opening、Operation/idempotency 却仍被 executor activation 阻塞而无法结算的缺口：Application 将 opening commit 定义为唯一 acceptance point，注册 live owner并发布 opening 后立即返回 Run/Segment identity，纯 executor activation 与 pump 继续由既有 Run lifecycle task拥有；activation 失败仍形成权威 terminal。waiting-child cancellation 因同时拥有 post-commit Apply/Continue 验证而保持同步，不以通用异步路径破坏事务语义。真实 SQLite 覆盖强杀发生在 answer claim 后及 opening 已提交但 activation 未完成两侧，survivor 均保留 accepted Question answer并将无 owner Run 收敛为 Lost。Protocol、SQLite schema、Artifact、Desktop Agent inner ring 与 Agent Framework 合同均未变化。
- P88 关闭 Desktop final teardown 已释放当前 mutation claim、旧 composition-root factory 却仍可复活 successor client 的缺口：owner 在退休资源前同步进入不可逆 closed 状态，旧引用不能再创建 RPC transport、sidecar 或 journal lease；重复 dispose 共享唯一 settlement，测试 reset 仍先发布全新 owner再 join旧代。旧 mutation 在途、replacement 接管同 key 后窗口再次关闭、retired response 迟到的交错保持 successor generation，清空 Host owner KV，并由下一冷启动接回原 identity。Runtime/Protocol/Host KV schema、Desktop Agent inner ring 与 Go Agent Framework 合同均未变化。
- P89 关闭多轮长对话 compaction 只替换 conversation、既有 terminal Run `message_mark` 却留在旧坐标的跨聚合缺口：Conversation Domain 定义 summary-prefix 折叠与 retained-suffix 平移的唯一坐标变换，Conversations Application 从完整 Run snapshot 产生 exact aggregate replacements，Persistence 在一个 SQLite transaction 内重验消息数/Run set、Replace history 并以 expected-mark CAS 更新水位。连续两次压缩、历史/最新 Run fork、直接 SQLite watermark invariant 与 Run 更新失败整笔回滚均已验证；没有读取时 clamp、双路径或 Infra 业务公式。Protocol、Artifact、SQLite schema epoch、Desktop Agent inner ring 与 Go Agent Framework 合同均未变化。
- P90 关闭 HITL Resume 替换 active Segment 后，旧 Segment 的迟到 `EventCommit` 仍能按 Run/Session 写入纯 Transcript/Conversation 或结束新 Segment 的缺口：Application 以 Run/Session/Segment envelope 统一拥有完整 event write-set，Reducer/route/authoritative combine/child opening 禁止混代，Model/Tool/Progress 必须与 envelope 同代；runsegment 在同一 SQLite transaction 的首个 projection write 前要求 exact Running Segment。旧 terminal 与 item/message-only 写集均 fail closed，waiting cancellation 继续使用 Waiting aggregate 专用路径。Protocol、Artifact、SQLite schema epoch、Desktop Agent inner ring 与 Go Agent Framework 合同均未变化。
- P91 关闭模型/Tool 外部调用前 Runtime 权威事实提交失败被误作 provider/Tool 业务错误的缺口：Agent Interaction 以中性 `HostFailure`、protocol v5 互斥 definite settlement 和稳定 terminal code 拥有 Host 前置边界拒绝；Runtime 只在 `adapter/agentexec` 标记 Model/Tool start、自动审批拒绝结果、applied steer 等 pre-call commit failure并投影 internal failure。provider/Tool 零调用，Tool/denial/steer 后续模型 turn 为零；调用后 unknown-effect recovery 与普通 provider/Tool error 保持原合同。Agent ADR-A2-076 / Baseline 22 先发布 canonical source，Runtime standalone 绑定同一 pseudo-version；Agent 内层没有 Runtime、RPC、数据库、事务或产品终态抽象，Protocol、Artifact、SQLite schema epoch与 Desktop 均未变化。
- P92 关闭单 Tool HostFailure 被错误泛化到包含已发生 sibling 外部调用的并行 Tool Effect：`adapter/agentexec` 的 per-Effect attempt 同时拥有 external-boundary count、Host projection failure 与 pending projection retirement。失败先发生会阻止后续 sibling 跨界；任一 sibling 已跨界则整批只走既有 unknown-effect/RunLost，并主动释放等待 canonical prefix 的 post-external receipt，不再 definite internal或形成等待环。两个显式并行 Tool、窗口 2 的单客户端交错冻结外部调用一次、唯一 unknown、零 SegmentEnded；单 Tool与 accepted root cancellation 语义保持不变。Agent Framework、Runtime Application/Domain/Persistence、Protocol、Artifact、SQLite 与 Desktop 合同均未变化。
- P93 关闭 terminal `EventCommit` 已 durable COMMIT、success receipt 却丢失或 caller 被取消后的无限失败重放：Application reducer 为每个不可变 terminal write-set铸造稳定 identity，runsegment/SQLite 将该 identity 与生产它的 Segment 在同一事务写入 terminal Run 行；事务错误后以有界、request-detached read只证明这一个 write-set。精确已提交与同 identity replay 收敛成功，不同 Segment、Restore/Recovery terminal 或另一 terminal attempt均 fail closed；同一已取消 context重放不会重复追加 conversation。该批先建立 terminal 专用 marker，P94 再由当前唯一 shape 统一；Protocol、Artifact、Desktop 与 Agent Framework 合同均未变化。
- P94 关闭非终态 authoritative `EventCommit` 已 durable COMMIT、success receipt 丢失后内存 reducer 回退并把已完成 Model/Tool 外部事实误判为 HostFailure/RunLost 的缺口：Application 为每个顶层提交铸造唯一 identity，runsegment 在完整 projection write-set 末尾把当前 Segment latest marker 写入 Run 行，任何事务错误都以 request-detached exact marker 统一结算；terminal marker 复用同一机制并永久保留。单泵串行使一个 latest marker 足够，不建立无限增长 ledger；Suspend/Resume 清除旧代，Restore/Recovery 不伪造回执。真实 SQLite Model completion 在 COMMIT 后丢回执并取消 caller 仍结算成功，invocation/message/metrics/marker 原子可读，同 canceled context exact replay 不重复写，下一 canonical fact 覆盖 marker 后旧 identity 不再匹配；Protocol、Artifact、Desktop 与 Agent Framework 合同均未变化。
- P95 关闭 fresh Start、HITL Resume、durable child start 与 HITL tree barrier 已 COMMIT、success receipt 丢失时仍被当作失败的 composite command 缺口：Application 为 opening 与 barrier 各铸造一次 immutable identity；runsegment 在完整事务末尾把 opening marker 写入 owner Run，在 root Waiting transition 内写入 barrier marker，并在所有 transaction error 后以 request-detached exact Session/Run/Segment/identity 结算。child reservation 已 concluded 只在 exact child opening marker 匹配时幂等成功，不再以相同 coarse 结论接受另一 write-set。latest marker 继续复用 P94 的单行机制，普通 Suspend/Resume/Restore/Recovery 清除旧代；Protocol、Artifact、Desktop 与 Agent Framework 合同均未变化。
- P96 关闭 waiting-child cancellation 定制 opening 丢弃 `OpeningCommit.CommitID`、自身 composite transaction 在 durable COMMIT 回执丢失后仍被误判失败的缺口：Application 的 remains-Waiting 命令单独铸造 identity，恢复 Running 的命令精确复用 opening identity；runsegment 只在 checkpoint/Pending/conversation/Items/terminal children/remaining disposition 全部提交后，把 marker 写入 root Run。already-Waiting 命令不伪造 Segment，以 empty Segment + unique identity 表示；恢复命令绑定 exact new Segment。事务错误后 request-detached proof 同时读取 durable target/root 结果，同 identity replay 不重复 conversation，不同 identity fail closed；SQLite 唯一 shape 在该批前移，历史值由版本库保留。Protocol、Artifact、Desktop 与 Agent Framework 合同均未变化。
- P97 关闭 HITL Resume 的 answer claim 已 durable COMMIT、success receipt 丢失后仍向上返回失败的缺口：Application 为 `ResumeClaimCommit` 铸造唯一 identity，runsegment 在 `open → resuming`、Question answer replacement 与 one-shot checkpoint deletion 全部完成后，向 root Waiting Run 写入 empty-Segment marker；事务错误只以 request-detached exact Session/root/identity 结算。发起事务的原调用用已加载 checkpoint 重建 claimed hand-off，进程崩溃后仍由既有 recovery 收敛，不新增 checkpoint replay ledger；marker 写失败时全部 claim write-set 回滚。SQLite epoch 保持 74，Protocol、Artifact、Desktop 与 Agent Framework 合同均未变化。
- P98 关闭 dougong 接入后 Desktop composition root 启动 Host 却不停止、renderer-global installation Map 与 sideload Platform 又允许旧代串写 successor 的缺口：唯一 `beforeunload` Host teardown 先同步 unmount React root，PluginProvider effect 同步撤销 exact Host publication，再按 Host-bound Platform → Host 顺序异步 join；startup AbortSignal、ContributionView、installation read model 与 blob URL set均绑定同一代，旧 stop/discovery/settlement 只能回收本代。Plugins remove 在 Installation transaction 成功后才删除 read model，并以 handle identity拒绝旧 settlement 删除同名 replacement；Platform disposal 失败不阻止 Host stop或 URL回收。Frontend 插件架构文档同步到 dougong contract graph/structured lifetime事实；Runtime operation、Protocol、Artifact、SQLite、Desktop Agent inner ring 与 Go Agent Framework 合同均未变化。
- P99 关闭第三方插件已在 dougong Platform 成功注册、却未进入 Plugins read model 而无法显示和卸载，以及 renderer-global 来源表允许失败/旧代 sideload 重标 successor 同名内置插件的缺口：exact Host 现在唯一拥有 `{name, origin, handle}`，内置与 sideload 共用同一已提交事实源；sideload 只在 Platform transaction 成功后登记真实 registration handle，并在 transaction 前拒绝当前 Host 同名安装。Plugins pane 直接消费 active Host records，全局 `pluginOrigin` 双路径删除，Host 替换同时切换名称、来源与卸载身份。Runtime operation、Protocol、Artifact、SQLite、Desktop Agent inner ring 与 Go Agent Framework 合同均未变化。
- P100 关闭 renderer replacement 后 URL 仍持有右侧 Dock destination、全新 per-session memory 却反向清空位置，以及同一 Session 的 Host/plugin 重绑用 remembered tab 擅自复活已折叠 Dock 的缺口：Context Dock 以 `null` 唯一表示 renderer 尚未接管，合法 sessionless scope 继续使用 `""`；首次接管和 same-session rebind 采用 navigator 当前位置，以一次 store write 同时补齐 open/last memory，exact Session identity 变化才恢复目标 scope。URL-backed location 与 session-scoped tab memory 不再互相镜像或越权。Workspace public ports、Runtime operation、Protocol、Artifact、SQLite、Desktop Agent inner ring 与 Go Agent Framework 合同均未变化。
- P101 关闭 `selectedToolId` 被消息流与 Tool card 路由持续写入、右侧 Terminal 却从未读取，以及长对话 compaction/Runtime 恢复删除旧 Tool 后 selection 永久悬空的缺口：Workspace navigation 暴露 reactive exact Tool identity，ChatStream 只在 Tool id membership变化时保留、回退或清空选择，output delta 不进入 selection effect；Terminal 将 Tool 精确映射到 command，旧/非命令目标回退最新 command并回写，激活时滚到唯一选中 card。历史目标退出 pinned-tail，最新目标继续追尾；视觉复用现有 selected token。只读对照 study/Codex 的 item/delta 心智模型，Lyra 的聚合 Terminal 与 per-session Context Dock 信息架构保持不变。Runtime operation、Protocol、Artifact、SQLite、Desktop Agent inner ring 与 Go Agent Framework 合同均未变化。
- P102 关闭右侧 Run Summary 把任何 `run-end` 都判为成功，导致 canceled、maxSteps 与 maxBudget 显示绿色 Done 的缺口：Workspace presentation 通过既有 Agent public read model读取 exact current-root outcome；completed 或明确 terminal `status=ok` 才是 success，failure/run-error 为 error，canceled独立为 neutral，maxSteps/maxBudget 统一为 warning limit，无法证明的终态 fail closed为 unknown。badge 复用 Run Tree 现有文案/tone，不从 summary 文本重建领域事实，也不引入 Runtime DTO。Runtime operation、Protocol、Artifact、SQLite、Desktop Agent inner ring 与 Go Agent Framework 合同均未变化。
- P103 关闭同一 root Run经 HITL interrupt/resume 产生多个 continuation Segment start后，右侧 Run Summary 从最后一个 `run-start` 切片而丢失审批前命令、文件、approval与起始耗时的缺口：digest 以 selected Run第一个可用 start建立坐标，再按 exact runId聚合全部 continuation Segments到 terminal；后续 Segment start不重置 Run边界，child/其他 root material继续隔离，P102 authoritative outcome状态准入保持不变。Runtime operation、Protocol、Artifact、SQLite、Desktop Agent inner ring 与 Go Agent Framework 合同均未变化。
- P104 关闭两个 Session 打开同名右侧 view时 React 只按 `viewId` 复用 mounted subtree，导致 Terminal pinned-tail/scroll、Diff collapsed files/anchor 与 Run Summary copied feedback等局部状态跨 Session 泄漏的缺口：Shell kernel以 exact active Session ID key整个 Context Dock实例边界，Session切换统一退休 view-local state；同一 Session内 tab仍由既有 Activity保持挂载，open/active/last navigation facts继续由 P100逐 Session store恢复。视觉样式、Workspace public ports、Runtime operation、Protocol、Artifact、SQLite、Desktop Agent inner ring 与 Go Agent Framework 合同均未变化。
- P105 关闭 Runtime 重启、冷启动或 authoritative snapshot替换后 completed Tool只按持久 Item重建 `toolCalls + tool-end`，Run Summary却要求 live-only `tool-start` 才纳入 material，导致 command、changedFiles与readFiles全部消失的缺口：digest以 exact Run timeline中 Tool start/end任一首次出现建立因果 identity，live双事实自动去重，durable end-only路径从同一个 Tool read model恢复参数、状态与diffstat；不伪造事件或复制 DTO。Runtime snapshot/Protocol/Artifact/SQLite、Desktop Agent inner ring 与 Go Agent Framework 合同均未变化。
- P106 让 exact ToolCall Transcript 持有唯一 accepted human approval decision，并在 answer claim 的同一事务中与 Pending、checkpoint 和 commit receipt 原子提交；resume reducer、Protocol、Artifact 与 Desktop cold read model 只消费这一事实，不从 policy、outcome 或 optimistic result 猜测。
- P107 关闭单客户端并行同名 Tool 在 edited approval 后丢失原 Item identity 的缺口：Application-private interrupt binding 持有 exact provider CallID，tree barrier、answer claim、SQLite epoch 75 与 resume reducer 逐层保留；name/arguments 只保留一般 replay correlation，不再承担审批 identity。Transcript、Protocol、Artifact、Desktop 与 Agent Framework 公共合同均未变化。
- P108 关闭 `sessions.snapshot` 虽在单个 SQLite transaction 读取、却只校验 Pending root 而允许 ownerless HITL/部分 approval/Run-Item 矛盾穿过 Application 边界的缺口：`Pending.ValidateProjection` 统一拥有 parked Continuation→Run→Transcript Item 闭包，在线挂载与启动恢复共同验证 exact Session/Run/Item/occurrence、typed payload、accepted-decision 空值、continuation facts、running Item 唯一认领及 waiting root ownership；Material Snapshot 同时复用 Run lineage closure并拒绝 terminal Run/running Item。真实单客户端 edited approval E2E 在第一次 Pending 后 SIGKILL Runtime，再启动并完成同名 sibling 的第二次审批，证明崩溃恢复后仍只有各自唯一 Tool lifecycle。Protocol、Artifact、SQLite epoch、Desktop Agent inner ring 与 Agent Framework 合同均未变化。
- P109 将 Desktop 插件 composition root 从 dougong 0.2.0 升级到 0.3.0：umbrella package 与 `@dougongjs/core`、`platform`、`reactive` 保持同一 0.3.0 代际且依赖树唯一；0.3.0 收紧的 Plugin/Installation generic、normalized Plugin/Artifact trust boundary 与 capability function variance 均由现有 SDK boundary 直接满足，无需兼容 adapter 或双 API。Host replacement、installation/remove、sideload Platform、lazy activation 与 reactive lifetime 的定向严格回归和 Frontend 全量异步泄露检测通过。Runtime、Protocol、Artifact、SQLite、Desktop Agent inner ring 与 Go Agent Framework 合同均未变化。
- P110 清除 dougong 0.3.0 升级复核中发现的最后一处旧版兼容缝：sideload lazy Artifact 的 `placeholder` 直接传递 `AnyPlugin`，不再用针对 0.2.0 声明差异的 `as never` 绕过；插件 kernel 测试也不再把当前 Host read contract 绑定到历史版本号。生产源码与插件测试中已无 dougong 0.2.0 语义注释或兼容类型路径，Platform 继续独占 opaque Artifact trust boundary。Runtime、Protocol、Artifact、SQLite、Desktop Agent inner ring 与 Go Agent Framework 合同均未变化。
- P111 收敛 Desktop 左右结构面板的展开/收起时钟：性能 trace 证明长对话上的旧 300ms 近匀速过渡没有 long task 或持续掉帧，卡顿感来自整个时钟内可见的匀速正文重排，而非 React render；只读对照 study/Codex 后，将其 500ms / 0.1-bounce Motion spring按 25ms 均匀采样为唯一原生 CSS `linear()` progress，左右 flank、spacer、window-corner yield 与边界阴影共同消费，React 不新增逐帧 owner。Drawer 与 Context Dock 的固定宽度 descendant tree 同时建立 `contain: layout paint`，不参与 reading-plane reflow。Chromium 与 WebKit、长对话与全 Dock 矩阵、reduced-motion authority 均已验证；Runtime、Protocol、Artifact、SQLite、Desktop Agent inner ring 与 Go Agent Framework 合同均未变化。
- P112 关闭 Runtime 重启/resync 后已挂载 HITL、Plan、Goal、Run、Tool 可能跨 durable generation 拼接的缺口：Application `MaterialSnapshot` 与 SQLite reader transaction 现在共同拥有同 Session Goal，Protocol `sessions.snapshot` capability-aware 地一次传递完整 material；Desktop 将响应作为 unit-of-work，仅当 Agent projection 赢得当前 view token 后同步提交伴生 Goal。generic Agent Application published port隔离具体 read model，Runtime DTO只在adapter，Agent→Goal保持为零；full/scoped resync退休旧Session writer并取消独立mounted Goal query，未挂载Goal仍单独refetch。独立reader/writer SQLite反例、foreign owner、旧代/abort/live-write/rollback/plugin-dispose与真实HTTP SIGKILL恢复均已验证；SQLite schema、Artifact与Go Agent Framework合同未变化。
