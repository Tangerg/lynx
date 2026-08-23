# app2 架构决策

> 状态：R0 accepted。已接受 ADR 不覆写；变化通过新 ADR 显式 supersede。协议、transport 与 Desktop
> binding 的当前规范决定是 ADR-A2-022；ADR-A2-002/004/005/006/010/011/012 的相关段落仅保留决策历史。

## ADR-A2-001：建立独立 app2 绿地实现

**决定**：在 `app2/` 重写，不在原 `app/runtime`、`app/desktop` 上继续修补。

**原因**：用户已明确授权 breaking greenfield。旧 Runtime 已经历 P0–P142，结构和合同为连续局部演进服务；
继续原地改造会让迁移顺序、兼容顾虑和旧抽象继续主导设计。

**替代**：原地重构；长期维护 runtime2/app2 双实现。均拒绝。

**后果**：本 ADR supersede 旧 `ADR-RT-001`“不创建 runtime2”的产品决定。旧代码只作行为证据，完成后删除。

## ADR-A2-002：保留能力和语义，不保留 surface（协议部分由 ADR-A2-022 取代）

**决定**：89 个 operation、3 个 probe、16 个 topic 和 Desktop 产品表面必须等价保留；方法名、Go API、
wire、DB、Artifact、frontend public surface、transport 和组件树无需兼容。

**原因**：用户价值存在于行为与不变量，旧 surface 正是重写要摆脱的约束。

**后果**：能力账本以场景验收，不做字节级 response 对比；旧数据不会自动出现于 app2。

## ADR-A2-003：用领域 package 实现整洁架构

**决定**：Runtime 使用 `run/session/interrupt/...` 与按需 `runflow/sessionflow/...`，不复制
`domain/application/adapter/infra/delivery/bootstrap` 深树。

**原因**：整洁架构的本质是依赖方向和 owner，不是通用层名。当前 24 application、23 adapter、21 domain、
17 infra 子包证明通用层目录已成为主要导航成本。

**后果**：architecture test 检查 import；只有真实变化轴才产生新 package/interface。

## ADR-A2-004：Desktop 默认监督独立 Runtime 子进程（binding 部分由 ADR-A2-022 取代）

**决定**：Wails Desktop 启动并监督 `lyra-app-server`；本地默认 stdio JSONL。

**原因**：保留 Runtime crash/SIGKILL 后窗口存活与恢复能力，同时删除 loopback HTTP token/CORS/port/SSE。
Codex app-server 证明 bidirectional JSON-RPC over stdio 是富客户端的成熟边界。

**替代**：把 Runtime 全嵌入 Wails；继续本地 HTTP。前者失去进程隔离，后者保留无价值机制，均拒绝为默认。

## ADR-A2-005：所有 transport 共享一个 dispatcher（transport 清单与握手部分由 ADR-A2-022 取代）

**决定**：stdio、远程 WebSocket 和测试 in-process transport 只实现 framing/connection/backpressure，调用同一个
initialize gate、operation catalog 和 handler graph。

**后果**：没有 HTTP-specific business API、embedded shortcut 或第二套 method semantics。

## ADR-A2-006：远程连接改为 WebSocket，不兼容旧 HTTP/SSE endpoint（由 ADR-A2-022 取代）

**决定**：保留切换外部 Runtime 的产品能力，但新配置使用 `ws/wss` 双向连接。旧 HTTP endpoint、SSE URL 和
local token shape 不兼容。

**原因**：一个双向 transport 可以承载 request/response/notification，避免 HTTP+SSE 两个生命周期 owner。

**后果**：远程模式必须单独完成 TLS/auth/Origin/slow-client/reconnect 纵切后才能标记 verified。

## ADR-A2-007：保留 Session/Run/Segment 领域语言

**决定**：学习 Codex 的 Thread/Turn/Item primitive 和生命周期，但不把 Lyra 的 Session/Run/Segment 强行改名。

**原因**：Lyra 的 Run tree、delegated Run 和跨 HITL Segment 有独立业务含义，Codex Turn 不能无损覆盖。

**后果**：参考实现提供机制，不成为词汇真相源。

## ADR-A2-008：一个 SQLite durable truth，从 epoch 1 开始

**决定**：app2 使用独立 data home、SQLite epoch 1，不读旧 epoch 77；不增加 JSONL rollout 第二真相源。

**原因**：breaking 授权允许直接建立正确 schema。SQLite 可同时提供事务一致性、查询和恢复；双 store 会引入
排序、提交和修复仲裁。

**后果**：Artifact/备份从 SQLite facts 生成；需要旧数据时必须另立一次性显式导入项目，而非兼容 reader。

## ADR-A2-009：合同从 Go canonical registry 单向生成

**决定**：Go protocol types/catalog 唯一拥有 wire，TS client/validators/schema/OpenRPC/manifest/examples 全生成。

**原因**：旧 `API.md` 已出现“只有两种 Interrupt”与旧章节“三变体含 toolResult”并存的事实漂移。

**后果**：人类文档不再复制完整 union；generator diff 是强制门禁。

## ADR-A2-010：Wails RuntimeBridge 转发 envelope，不复制 operation facade（由 ADR-A2-022 取代）

**决定**：Desktop Go bridge 只提供少量 request/notification/connection-state IPC；强类型业务方法生成在 TS client。

**原因**：为 89 个 operation 手写 Wails forwarder 会制造第二 API、第二错误映射和同步负担。

**后果**：Runtime 仍 strict validate renderer 输入；Wails exported method set 由测试锁定。

## ADR-A2-011：Session mount 是一致快照与订阅的原子客户端动作（由 ADR-A2-022 取代）

**决定**：`session/mount` 同时返回 Session/Run/Item/Interrupt/Goal/Plan closure 并建立 connection subscription；
不让 Desktop 自行拼 `snapshot + run subscribe + resource queries` 的交接窗口。

**后果**：server listener 必须保证 snapshot watermark 与后续 notification 顺序；unmount/close 结构化释放。

## ADR-A2-012：所有跨任务队列有界并显式过载（错误码部分由 ADR-A2-022 取代）

**决定**：transport ingress/outbound、Desktop scheduler、watch/event 和 background writer 有容量、超时和关闭语义；
满载 request 返回 `-32001 overloaded`。

**原因**：无限 channel 把延迟和内存增长隐藏到故障时刻。Codex 的有界 transport 与过载合同值得采用。

**后果**：容量是可测配置；critical control lane 有公平性；notification gap 走 resync。

## ADR-A2-013：按业务 identity 串行化，数据库仍是最终仲裁

**决定**：同 Session/Run/MCP server 等 mutation FIFO，不同 identity 并行；共享读可合并并发。revision/CAS/unique
constraint 继续决定 durable correctness。

**原因**：全局锁过宽，无序并发让 handler 重复补偿；纯内存队列又不能应对多进程和 crash。

## ADR-A2-014：只发布 committed facts

**决定**：业务事件、snapshot 和 UI terminal state 只来自已提交 write-set；外部 effect 使用 attempt/receipt/unknown。

**后果**：客户端不乐观伪造 terminal；回执丢失用 marker 证明；unknown fail closed。

## ADR-A2-015：Agent Framework 只有一个适配边界

**决定**：`agentexec` 独占 Framework concrete API、snapshot、input 和 observation 映射；Framework Engine 拥有执行
lifecycle，app2 application 拥有产品 Run transaction/HITL/recovery。

**后果**：Agent Framework import fitness test 必须精确到 package；toolset/persistence/transport 不识别 Framework type。

## ADR-A2-016：前端使用静态限界上下文组合

**决定**：删除通用 Plugin Host/Plugin SDK/installation lifecycle。生产功能按 context 静态装配；只有 tool renderer、
workspace destination、command 等真实多生产者使用具名 registry。

**原因**：当前生产只加载同 bundle built-ins，外部 sideload 与动态 production seam 已在 P140/P142 删除；通用插件
内核不再对应产品变化轴。

**后果**：保留模块化和 error boundary，不保留插件身份、requires/provides graph 或万能 extension points。

## ADR-A2-017：保持 Work Index / Agent Narrative / Context Dock 信息架构

**决定**：继续采用当前已验证且重点参考 Codex 的三面模型；重写组件和状态 owner，不重新发明导航。

**原因**：这是用户可观察能力与长期工作心智模型，不是旧代码坏味道。

**后果**：workspace/run material 不回到左侧功能菜单；blocking HITL 在 Narrative 可完成；dock 按 Session 隔离。

## ADR-A2-018：TanStack Query、Agent projection、URL 与 UI store 各守一类事实

**决定**：server resources 属于 context query cache；mounted streaming material 属于单一 AgentSessionView；导航属于
URL；Zustand 只持有偏好和 scoped ephemeral state。

**原因**：避免 query/store/plugin/组件同时写一个事实，并让 renderer reload 可由 server snapshot 恢复。

## ADR-A2-019：旧 app 只做隔离的语义 oracle

**决定**：对照测试在不同临时 HOME/DB/process 中运行旧 app 与 app2，比较规范化业务结果和用户场景，不共享代码、
fixture builder、数据库或 DTO。

**原因**：共享实现会把旧 bug/shape 带入 app2；完全不对照又容易漏能力。

**后果**：每个差异必须人工裁决为 app2 defect、旧 bug 或 intentional breaking，并写入 ledger evidence。

## ADR-A2-020：切片完成后立即回收外部资源

**决定**：browser/agent-browser/Playwright/Wails/Vite/Runtime/MCP/LSP/watcher/child process/temp DB 均属于切片
lifetime；完成门禁包含资源归零证明。

**原因**：资源泄漏会污染后续恢复测试、端口、文件 watch 和用户桌面状态。

**后果**：测试 harness 必须提供结构化 disposer，CI 检查残留进程/端口/临时 home。

## ADR-A2-021：先完成 Runtime/Desktop，再迁移其余 app consumer

**决定**：Wave A 完成 app2 Runtime 与 Desktop 全能力；Wave B 迁移 CLI 等剩余 `app` consumer；Wave C 删除旧树。

**原因**：用户明确指定首批 Runtime/Desktop，同时目标是整个 app 最终彻底迁移。

**后果**：Runtime/Desktop parity 不是整个 goal 的终点；goal 只在剩余 consumer 和旧 app 删除后完成。

## ADR-A2-022：当前 Lyra Runtime Protocol 是 app2 合同基线

**决定**：app2 在实现、package、存储和前端 owner 上做绿地重写，但第一阶段精确继承当前
`2026-08-21` Lyra Runtime Protocol。Codex 是生命周期、并发、恢复与工程机制的参考，不是 Lyra 的 wire、
method 命名或 transport 模板。

必须保留的协议资产包括：

- 当前 89 个 dotted methods，以及 typed operation catalog 对分类、幂等、分页、capability、错误和
  materialized facts 的单点声明；
- `RequestMeta + runtime.discover` 协商，不新增 connection initialize 状态机；
- `POST /v2/rpc` 的 strict JSON-RPC、unary JSON、stream SSE、`Last-Event-Id` 回放、显式 cancel 和 detach 语义；
- `/v2/info`、`/v2/health/live`、`/v2/health/ready` sidecars；
- local token/CORS 与 remote HTTP(S) 的部署边界；
- HTTP 和 embedded binding 共用一个 operation endpoint；
- `sessions.snapshot + runs.subscribe + runtime.subscribe`，不发明 `session/mount`；
- 当前 generated error mapping；特别是 `-32001` 仍是 `provider_error`，不得挪作 overloaded。

Desktop 仍可监督独立 Runtime，但 renderer 使用生成的 Lyra client 直连 `/v2/rpc`；Wails 只提供 bootstrap、
supervision 和 native shell，不转发第二份 JSON-RPC envelope。远程连接继续使用相同 HTTP(S) contract，除非真实
部署证据支持增加新 binding。

**原因**：复核现有实现后，发现其 canonical typed registry、strict JSON-RPC、streamable HTTP/SSE、回放、幂等、
sidecars 和 embedded conformance 已经是成熟的架构资产。把 Codex 的 stdio、slash methods、initialize 或
Thread/Turn 语言原样覆盖到 Lyra，会制造无产品收益的 churn，并丢失当前协议已经解决的问题。

**breaking 门槛**：允许 breaking change 代表不承担旧实现兼容成本，不代表主动破坏良好合同。任何协议变化必须先
给出明确 defect/反例，记录新 ADR，提升 exact protocol version，并让 generator、HTTP/embedded conformance、
Runtime、Desktop 和 capability ledger 同批闭环。

**取代范围**：本 ADR 取代 ADR-A2-002 的 wire/transport 任意变更部分、ADR-A2-004 的 stdio 默认 binding、
ADR-A2-005 的 transport 清单与 initialize gate、ADR-A2-006、ADR-A2-010、ADR-A2-011，以及 ADR-A2-012 对
`-32001` 的重新定义。独立进程、共享 operation pipeline 和有界资源原则继续成立；新增 client-visible
overload 必须先扩展并版本化 Lyra problem manifest，不能从 Codex 借用错误码。

## ADR-A2-023：本地监督使用一次性 bootstrap descriptor

**决定**：每次本地 spawn 都由 Desktop supervisor 创建一个权限收紧的独立临时根，并把 exact descriptor path、
随机 nonce、`127.0.0.1:0` listen address、token path 和 app2 data/config path 作为 `lyra-runtime serve` 的显式
启动配置。Runtime 自己先 `net.Listen` 持有 listener，再以原子 replace 发布 one-shot descriptor：

```text
bootstrapVersion, nonce, pid, instanceId, protocolVersion, baseURL, tokenPath
```

supervisor 只接受 nonce/PID 匹配、loopback URL、预期临时根内 token path 的 descriptor；随后读取 token，并用
`/v2/info`、live、ready 和 `runtime.discover` 交叉校验同一 instance。成功或失败都删除 descriptor；child 退出后
回收该次 spawn 的 token 与临时根。下一次 restart 使用新根、新 nonce、新 token 和新 generation。

**原因**：先选“空闲端口”再让 child bind 存在 TOCTOU；固定 17171 会冲突；解析 stdout/stderr 会把日志变成
隐式协议；继承 listener/额外 FD 的跨平台行为又不适合作为 Wails 默认。一次性 descriptor 是更小的 host
bootstrap 合同，并且不创建第二套业务 RPC。

**约束**：descriptor 不进入 Runtime Protocol、不会用于 remote attach、不得包含 token 原文，也不能从用户可控
路径读取。Runtime 在 listener/descriptor 发布前失败时保持 not-ready；supervisor timeout 只终止自己记录的 exact
child PID。`lyra-runtime` command 由 Cobra factory 构造，配置经 fresh Viper 解析为 typed Config；Runtime 业务和
host packages 不 import Cobra/Viper。

## ADR-A2-024：本轮止于 app2 Runtime/Desktop 独立交付

**决定**：当前目标只完成 R2–R11 的 app2 Runtime/Desktop 全功能重写、统一门禁与独立 package。R12 的 CLI/
其余 consumer 迁移和 R13 的入口切换/旧树删除全部延期；不得修改或删除旧 `app`。实现期一次性写完生产代码，
最终只运行一轮总体测试与修复；每个边界清晰的实现批次仍提交并推送作为防丢失 checkpoint。

**原因**：用户明确收窄本轮授权，并要求用集中验证减少反复测试成本，同时用远端提交降低长周期重写的丢失风险。

**后果**：本 ADR 在当前目标范围内取代 ADR-A2-021 的 Wave B/Wave C 连续执行要求；它不引入旧实现兼容层，
也不把未运行最终门禁的能力标为 `verified`。后续切换必须由新的明确目标重新授权。

## ADR-A2-025：Goal 使用 Lyra 原生资源与 exact Run ownership

**决定**：自主循环继续使用 Lyra 已发布的 `goals.*`、Goal DTO、Run/Segment、Conversation/Transcript 与
`goals.changed` 合同。服务端把 Goal 重写为私有聚合 + CAS repository + 单一 Driver：Goal 的
`activeRunId` 是唯一 durable continuation owner，Run 不保存第二份 Goal provenance；fresh autonomous control
只写入 provider-neutral Conversation journal，不创建伪造的 user Transcript Item。

Driver 在 Agent execution 启动前以 exact `sessionId + incarnationId + revision + runId` 认领 Run。模型侧始终可读
`create_goal/get_goal`，只有当前 Goal exact owned Run 才获得 `report_goal_outcome`。`completed` 先进入
`completing` settlement window，待该 Run 终态与 usage 原子折叠后才删除 Goal；`blocked` 必须给出具体原因。
人工输入 interrupt 仍由 Lyra Run/Interrupt 合同恢复，Goal 只恢复同一个 owned Run，不发明新的 continuation
协议。

Runtime restart 不自动继续 predecessor Goal。runflow 先把不可重放的 running effect 收敛为 `lost`，goalflow 再按
durable Run 真相结算；没有 owned Run 的 active Goal 显式变为 `paused/runtimeRestarted`。所有 API、tool、driver、
recovery mutation 都从 Goal owner 发布 `goals.changed`，application facade 不重复发布。

**原因**：Codex 的机制研究证明了 durable ownership、可恢复 loop 与 outcome handshake 的价值，但 Lyra 已有更适合
当前产品的 Goal/Run/Interrupt/HTTP 合同。复制 Codex 的 Thread/Turn、工具参数或 transport 会制造第二套协议；只吸收
并发与恢复原则，既能治本，也保留现有 Lyra Protocol 的成熟资产。

**后果**：Goal status 闭合为 `active | paused | blocked | completing`；停止原因是闭合 code；Goal body 的 SQLite
表示是 adapter-owned DTO，不序列化领域对象或协议 DTO。该能力在最终统一门禁前只标记 `implemented`。

## ADR-A2-026：Plan 是独立资源，Plan mode 是正交的 Session safety policy

**决定**：继续使用现有 Lyra `plan.get`、`Plan`、`plan.changed` 与 `plan.updated` 合同，不引入 Codex Plan wire、
Thread/Turn 身份或第二套 planning DTO。服务端删除共管 Plan/Interrupt 的 `stateflow`，把 Plan 重写为私有有序聚合、
CAS repository 与 `planflow.Service`；SQLite 只保存 adapter-owned body。Plan 整体替换，revision 单调递增，任一
snapshot 至多一个 `in_progress`。

成功的 `set_plan` 先提交 canonical Plan，再通过 `agentexec` Run-scoped typed fact sink 把该 exact snapshot 绑定到
同一个 observed ToolCall；runflow 在其 `item.completed` 后投影 authoritative/replayable `plan.updated`。它不解析工具
参数或字符串 result 反造 Plan。`plan.changed` 仍是无 payload 的冷读失效信号，Desktop 必须回读 `plan.get`，或回读
显式声明 materialize 该同源事实的 `sessions.snapshot`。

Plan mode 单独持久化为 Session-scoped safety policy，不放进 Plan 聚合，也不改全局 Approval mode。进入后，Run
catalog 的动态外层 gate 在 approval 之前拒绝 write/exec/network effect；read-only investigation、Plan replacement
与用户 question 仍可进行。退出必须以 durable HITL question 展示并批准 exact stored Plan；拒绝或重启后继续保持
Plan mode，批准时还要匹配提问时的 Plan revision。Session delete/rollback/import 清理 mode，fork 不继承 mode。

Plan/Goal control tools 只向 root Run 暴露。每次 root Run terminal transition 在同一 SQLite transaction 捕获 Plan
boundary；fork remap 已复制 Run 的 known boundaries，并以所选边界初始化 child live Plan；history rollback 通过 CAS
把 known boundary 提交成更高 live revision。没有 boundary 的 imported Run 保持 unknown，不能被当作 empty。boundary
通过 Run foreign key 回收，Artifact 继续只携带 canonical live Plan，不扩张现有 Lyra wire。

**原因**：Plan 内容、Plan mode 与 Interrupt 分别有不同的不变量、生命周期和变化原因；generic state owner 会把
资源、策略和人机交互重新耦合。Codex 的机制研究说明 read-only planning gate 与 explicit approval 很有价值，但现有
Lyra Plan/Run/Interrupt 合同已经更贴合当前产品，复制其协议只会产生双重真相。

**后果**：Session snapshot/fork/rollback/import/export 统一消费 Plan domain state；`plan.updated` 与 `plan.get` 共享
committed revision。Desktop compact projection 直接消费 snapshot/`plan.changed`，不创建第二份 Plan state；Plan
Runtime、tools 与 Desktop 在最终统一门禁前只标记 `implemented`。

## ADR-A2-027：Run 外诊断目录与 Run 内渐进工具权限分离

**决定**：保留 Lyra 既有 `tools.list/tools.invoke` 合同，且只允许无需 Session、模型循环、approval、network 或 process
lifecycle 的 direct diagnostics 进入该目录；当前集合为 `read/glob/grep`。完整 Agent 工具权限在 Run admission 时一次冻结，
其中核心工具初始可见，Skill、Memory 与 MCP 工具作为 deferred executable bindings。Run 内 `search_tools` 只检索这些冻结
definitions，并通过 exact names 改变下一次 model call 的可见清单；它不能增加 executable authority。

direct diagnostics 与 Agent filesystem tools 必须共用 `workspacefs` 的 physical-root confinement。delegated Run 的 routed
manifest 必须同时冻结 definition、safety、deferred 与 intrinsic-input metadata；第一次绑定真实 child identity 时，在锁外解析并
核对整份 manifest。Framework checkpoint 是 advertised-name state 的 owner，Lyra wire、Run/Item/Interrupt/Transcript 不增加
对应字段或兼容层。

**原因**：Run 外诊断、Run 内执行权限与模型 context visibility 有不同的安全边界和变化原因。把三者合成一个“全局工具目录”会让
Desktop 绕过 Run policy，或为了减少 prompt schema 又动态授予能力；复制参考实现的 tool protocol 则会破坏已经成熟的 Lyra
operation/stream/typed-client 合同。冻结权限后渐进显示，可以降低上下文噪声而不引入第二套真相。

**后果**：`tools.*` 不承担 approval 或 Agent catalog 配置语义；Desktop Tools 页明确是 safe diagnostics。deferred Tool 在进入
索引前仍绑定 Lyra safety/Plan/Hook/approval 链，`search_tools` 自身也被观察。ordered manifest metadata 纳入 deployment configuration
digest：同一清单 waiting/resume 恢复已加载名称，漂移则 fail closed；后续 fresh Run 获得新清单，既有 Run 不能借恢复或 child routing
换入新定义。最终统一门禁前该能力标记为 `implemented`。

## ADR-A2-028：Session 持有完整模型身份，Provider 配置以 CAS 收敛

**缺陷与反例**：Lyra `runs.start` 已明确规定 provider 不能由 model id 推断，但 `Session`、`sessions.create/update`
与 Session Artifact 只保存 model。若 `openai-compatible` 与 `ollama` 同时发布 `qwen3`，一个持久化的
`model=qwen3` 在 reload、fork、import 或下一次 Run admission 时没有唯一含义；现有 Desktop 又不把选择传给
`runs.start`，使 Session 字段成为不参与执行的装饰状态。这是 Lyra 自身规则的矛盾，不是参考产品协议缺失。

**决定**：Lyra Runtime Protocol 提升为 `2026-08-23`。`Session`、`CreateSessionRequest`、
`UpdateSessionRequest` 与 Artifact Session 均保存完整的 `provider + model`；两者只能同时缺席、同时清空或同时
赋值。Run 选择优先级闭合为 explicit `runs.start` pair → Session durable pair → Runtime default。Artifact 提升为
`app2/2`，SQLite 提升到 epoch 10；开发期不迁移旧 app2 数据或 artifact。

Provider durable aggregate 使用私有 revision 与 SQLite CAS；并发的 base URL/key 非重叠 patch 冲突后必须重读并
重放，因此不会互相覆盖。模型角色持久化 typed private record，不再序列化 protocol DTO 或接受 `any`。动态模型
发现失败返回 provider error，不再把不可用伪装成空 catalog；Provider/role no-op 不写 revision，也不发布
`models.changed`。Provider test 只返回脱敏、有界诊断。

**边界**：89 个 dotted methods、JSON-RPC/SSE、RequestMeta/discover、problem code、Runtime topic 与 operation
catalog 均保持 Lyra 既有设计；没有引入 connection handshake、Thread/Turn、stdio、别名 method 或第二套模型
协议。该变更只修复可证明的身份歧义，并由 canonical Go types 重新生成 Desktop client。

## ADR-A2-029：MCP 配置与授权共享代际，Lyra MCP wire 保持不变

**缺陷与反例**：既有 MCP Runtime 把 protocol DTO 直接持久化，配置没有 revision；`timeoutSeconds` 被保存却不治理
真实 dial。更严重的是，HTTP server A 开始 OAuth 后，用户可把同名配置切到 endpoint B，而旧 flow 仍把 A 的 token
写进同一记录。共享 SDK client 的 tool-list callback 也无法证明 notification 属于当前 session generation；断连、失败和
授权终态没有完整地收敛到 `mcp.changed`。这些是内部 ownership 与并发缺陷，不是 Lyra MCP 合同不足。

**决定**：保留 `mcp.servers.*`、`mcp.tools.list`、`mcp.authorizationAttempts.*` 共 9 个既有 dotted methods，以及
streamable HTTP/stdio、secret keep/set/clear、six-state lifecycle 与 attempt retention 的现有 wire。MCP durable owner
改为私有 `Configuration` aggregate：connection replacement 原子验证，revision/updatedAt 只在真实变化时推进，SQLite
用 exact CAS 保存 safe body 与独立 secret。配置 mutation、OAuth save/clear 进入同一 server identity lane；OAuth writer
只携带启动 flow 时的 aggregate generation，endpoint/origin 或 revision 漂移即拒绝 durable write。

live owner 为每次 connect/reconnect/tool refresh/authorization 分配 generation 和独立 SDK client/session；旧 result 只能
close，不能提交。`timeoutSeconds` 真实约束 dial/test；HTTP credential 禁止跨 origin，stdio directory 使用 host path
语义。authorization attempt 自己拥有 pending→terminal lifecycle，restart recovery 与 retention pruning 只处理 domain
record。remote tool 始终以 `(server, original name)` 执行，仅在模型 definition 边界由 domain 生成稳定、bounded 的
model-visible name；结果保留完整 MCP envelope。

**后果**：MCP service 成为 durable/live event 的唯一发布 owner，application facade 不重复发 `mcp.changed`。Desktop
继续消费 generated Lyra client，并用明确 MCP Settings section 表达 candidate test、write-only secret、OAuth 与 tool trust；
不存在 generic plugin host 或第二套配置协议。SQLite exact epoch 提升到 11，开发期直接重建旧 app2 data home；Runtime
Protocol 仍为 `2026-08-23`，contract generator 无 shape diff。该纵切在 R11 最终统一门禁前标记为 `implemented`。

## ADR-A2-030：Approval 是动态 effect policy，既有 Lyra wire 保持不变

**缺陷与反例**：既有 app2 把 approval mode/rules 与 Schedule 放在同一 `settingsflow`，并把 protocol DTO 直接保存为
SQLite JSON。Rule list 忽略请求 Session，forget missing 返回合同未声明错误；更关键的是，Agent catalog 在 Run 建立时
冻结 mode，执行链既不匹配 rule，也不保存 Interrupt 的 remember，所有 prompt 都标记为不可记忆。于是公开合同描述的
standing decision 实际没有进入 effect lifecycle。这是实现 ownership 断裂，不是 Lyra 协议缺字段。

**决定**：保留 `approval.getMode`、`approval.setMode`、`approval.listRules`、`approval.forgetRule` 与既有
Approval Interrupt/Remember wire。新建私有 `approvalpolicy` domain 与 `approvalflow` owner；mode 在每次 Tool effect
读取。Rule visibility/specificity 固定为 Session > Project > Global、exact > glob > whole-tool，同级冲突 deny。
当前 remember 永远写 exact subject；protocol 已有 subject 字段继续作为投影，不把 private scope key、match kind 或 revision
泄漏到 wire。edited args 仍只对本次 effect 生效。

Hook rewrite 后才计算 effective subject；Hook `ask` 与默认 stance 合并为一个 durable Interrupt。MCP auto-approve 只豁免
默认 mode prompt。高置信 root/home/device wipe 是独立 confirmation override，不能被 Yolo、auto-approve 或 remembered
allow 绕过；它不是 shell sandbox。remember/forget/mode 只在 durable fact 变化时由 Approval owner 发布
`approvals.changed`，application facade 不重复发布。

**后果**：SQLite 使用 normalized mode/rule state、Session FK cascade 与 changed-only upsert，exact epoch 提升到 12；
开发期直接重建旧 app2 data home。Desktop 新增明确 Approval Settings section，模式与 selected Session visible rules 均从
Runtime authority 冷读并消费现有 invalidation topic。Runtime Protocol 保持 `2026-08-23`，不生成新合同；该纵切在
R11 最终统一门禁前标记为 `implemented`。

## ADR-A2-031：Schedule 先持久化 occurrence，再原子 admission Run

**缺陷与反例**：既有 app2 在 `settingsflow` 中持久化 protocol Schedule DTO，worker 在真实 Run launch 前就更新
`lastRunAt/nextRunAt`，并吞掉 launch failure。Runtime 若在 cursor 前进后、Session/Run 建立前退出，该 occurrence 永久丢失；
runNow 先建 Session 再启动 Run，失败会留下 orphan Session。普通标题编辑还会重排 cron。Schedule、Approval 与 worker 共用
一个 owner，Agent Schedule tool 接入时又会形成 Tool catalog → Run engine → worker 的依赖环。这些是持久化边界和 ownership
缺陷，不是 Lyra Schedule wire 不足。

**决定**：保留既有 Lyra `schedules.list/create/update/delete/runNow`、Schedule DTO、五字段 cron 与
`schedules.changed`。新建私有 Schedule aggregate 与 ScheduleOccurrence。到期 claim 在一个 SQLite transaction 内以 revision
CAS 推进 next run，并写入带 immutable Schedule snapshot、due/fired/next time、固定 Session/Run identity 的 pending occurrence；
同 Schedule 至多一个 pending。worker 每轮先恢复 pending，再 claim 新 due work。

Run admission 使用第二个原子 transaction 同时创建 Session、Run、opening Item/Conversation/events，接受 exact occurrence，并在
Schedule 仍存在时记录 last admitted time。多 Runtime 对同 occurrence 竞争时只有 transaction owner 可以 launch；loser 回读 exact
Run，不重复执行。claim 后删除 Schedule 不删除 pending snapshot；runNow 复用 admission transaction但没有 occurrence、也不推进 cron。
accepted 后的不可重放 effect 继续遵循 Run recovery 的 visible lost 语义，不能为“重试”伪造新 identity。

管理 Service 与 active Dispatcher 分离：前者只依赖 repository/events 并可安全提供给 root Agent Schedule tools；后者在 Run engine
建立后组合 launcher、timer、recovery 与 cancel/join。由此避免 late-bound locator 和依赖环。Desktop 继续使用 generated Lyra client，
通过明确 Schedules Settings section 消费 authoritative revision/next/last 与既有 invalidation topic。

**后果**：SQLite 使用 normalized Schedule 与 durable occurrence，exact epoch 提升到 13；protocol 仍为 `2026-08-23`，不生成新
合同。Schedule edit/delete/runNow 均允许 breaking implementation change，但不破坏成熟 Lyra wire。该纵切在 R11 最终统一门禁前
标记为 `implemented`。

## ADR-A2-032：Runtime subscription 用 watch-ready initial resync 建立冷读栅栏

**缺陷与反例**：既有 Bus 先返回 `runtime.subscribe` ack，再由 goroutine 创建 filesystem watcher，也不发送 initial resync。
Desktop 通常先完成 query 再订阅；若 durable mutation 发生在 query 与 subscriber registration 之间，或 hooks.json 在 ack 与 watcher
ready 之间改变，该变化没有 frame。断线重连创建新 per-subscription sequence，却也不要求冷读；Desktop 更没有检查 frame sequence gap，
并遗漏订阅 `hooks.changed`。bounded queue overflow 已能发 resync，但无法覆盖这些窗口。

**决定**：保留现有 Lyra `runtime.subscribe`、RuntimeEvent/RuntimeTopic、WatchSpec、per-subscription sequence 与 `resync` wire。
Subscribe 必须按以下顺序建立 delivery boundary：先把 subscriber 放入 Bus registry；并行启动该请求的 file/skill/knowledge/hook
watchers；每个 watcher 在成功观察目标或发出 scoped recovery instruction 后标记 ready；全部 ready 后向该 subscriber 发一次包含 exact
requested topics/watch IDs 的 initial resync，再返回 stream ack。此后 committed producer event 与 external watcher event 共用一个
subscriber sequence。

queue overflow 继续清空不可信 backlog并只保留全订阅 resync。Desktop 对每个连接检查 sequence 连续性；gap 合成同订阅范围 resync，
每次 transport 重连由服务端 initial resync 收敛。generation 仍由 RuntimeConnection/query-key 隔离，不把 instance sequence 跨代比较。
`hooks.changed` 加入 Desktop 完整 topic 清单与 Hooks query invalidation；Hooks Settings 只消费现有 `hooks.list/setTrust`，不增加协议。

**后果**：Runtime resource event 仍是 invalidation，不建立 durable snapshot/replay store，不与 RunEvent 的 root-Segment replay 混合。
Watcher startup、递归目录数量、subscription cancel 与 Bus Close 均有硬边界，并由 Runtime task ledger join。Protocol/version/generated contract shape 均不变；
该纵切在 R11 最终 fault/reconnect/resource 门禁前标记为 `implemented`。

## ADR-A2-033：Session artifact 是终态输入文档，长历史继续复用 Lyra transcript cursor

**缺陷与反例**：既有高级 Session 实现把 import 当成 replace，identity 冲突时会覆盖另一个 client 可拥有的聚合；Message
归属靠 user-message heuristic 重建，child lineage、nested Item、Tool result 与 chat journal 没有完整 closure 检查。snapshot
又无界读取全部 Item，长会话 mount 成本随历史增长。Desktop 缺少 native import/export 与 boundary recovery consumer。把 Codex
Thread/Turn 或其 artifact 搬进来虽然表面省事，却会让 Lyra 已成熟的 Session/Run/Item/Conversation 四种事实出现第二套身份。

**决定**：保留既有 Lyra `sessions.snapshot/fork/rollback/export/import`、`items.list`、app2/2 artifact 与 HTTP/SSE/generated
client，不增加方法、别名、Thread/Turn 或兼容 reader。snapshot 只返回最新 200 个 Item 及 render/resume 所需 Run closure；旧历史
继续通过 `items.list(order=desc)` 的 query-bound keyset cursor 获取。

Artifact 是 terminal-only durable input document。export 从 canonical Conversation journal 为每个 root Run 计算单调 MessageMark；
import 以它恢复 exact message owner，不猜用户消息。导入在写入前递归验证 Session/model/workspace/time、Run DAG/root profile、Item
union 与 nested content/question/tool、Tool-result binding、message marks 和 chat ToolCall closure；existing Session ID 返回
`revision_conflict`。Session、Run、Item、Conversation、Plan boundary、Plan 与 Tool result 只在一个 SQLite transaction 中创建，失败
不留下 partial aggregate。Artifact 仍只携带协议已经定义的 portable semantic values，不加入 source revision、live status、Goal、
checkpoint tag 或 Codex provenance。

fork 只在 terminal root Run boundary 截断复制，并为新 aggregate remap identity；rollback 只接受 root boundary，删除其后的整棵
root/child material，恢复 known Plan boundary，返回 dropped root input 给 Composer，并清理 dropped checkpoint refs。files-only 不改
history；both 先要求 checkpoint restore 成功再提交 history。Desktop native host 只拥有 bounded file dialog/read/write，建议文件名只由
受控 Session ID 生成，title 与 artifact 内容都不能参与 path construction。

**原因**：bounded hydration、strict portable closure 与明确 recovery boundary 是可以借鉴的机制；Lyra 的 Session/Run/Item、
Conversation journal、dotted operations 和 generated client 已经是更贴合本产品的协议资产。治本意味着修复 owner 与 transaction，
而不是用参考产品概念重命名现有真相。

**后果**：O02 与 Work Index/history actions 到 `implemented`，最终 fault/parity/package 前不标记 verified。Session 没有独立
`archived` 状态；这里的 archive 指 portable artifact，favorite 仍由既有 revisioned Session field 表达。conversation search 与
long-history UI 只会在现有 transcript owner 上增加 R9c consumer，不建立第二份 history store。

## ADR-A2-034：Usage 只汇总终态 Run 事实，Feedback 不参与 Run 结果

**缺陷与反例**：既有 operations 实现直接解码 SQLite Run JSON，并把公开 `FeedbackRequest` 原样 JSON 持久化；这让
应用层依赖 adapter 表示，也让可选 Session/Run/Item 身份缺少 canonical ownership 校验。既有 cost 聚合还会在一部分
contribution 有价格、另一部分价格未知时保留已知小计，使 Desktop 把不完整成本误报成完整成本。把参考产品 analytics、
Thread/Turn attribution 或 event taxonomy 搬进来不会修复这些 owner 问题，反而会建立第二套运营协议。

**决定**：继续使用 Lyra 既有 `usage.session`、`usage.summary` 与 `feedback.create`，不增加 method、topic、event 或 wire
字段。SQLite adapter 从 terminal durable Run facts 投影私有 accounting record；operations owner 只消费 typed record，按
exact provider、served model 与 UTC day 汇总。只有每个 contributing Run/model slice 都具有已知 cost 时才返回 `costUsd`；
任一 contribution 未知就省略整个对应 total/bucket 的 cost，绝不以 0 或已知部分代替。

Feedback 进入私有 bounded record：先按最具体的 Item → Run → Session 查 canonical ownership，再拒绝请求中互相矛盾的
身份；文本按 UTF-8 4000 bytes 限制并在领域边界确定性移除 credential-like material。SQLite 保存 normalized identity、rating、
redacted text 与时间，不保存协议 DTO。Feedback 是 write-only operational fact；成功或 transport/storage failure 都不修改
Run outcome、Session revision、Transcript 或 Runtime invalidation。Desktop 只在 terminal root Run 的 completed final answer
上提供局部 rating action，delegated answer 不冒充用户当前任务的最终结果。

**后果**：SQLite exact epoch 提升到 14，开发期直接重建旧 app2 data home；Runtime Protocol 仍为 `2026-08-23`，generated
contract 无 shape 变化。O20/O23 与 Usage Settings/answer feedback 到 `implemented`，只有 R11 的 aggregate、restart、corruption、
transport-failure、UI 与 package 门禁通过后才升级为 `verified`。

## ADR-A2-035：Transcript 搜索是 Item 的派生索引，长历史继续使用既有 cursor

**缺陷与反例**：bounded snapshot 解决了 mount 成本，却没有给模型提供跨 context 的历史召回，也没有给 Desktop 提供按需
查看 200-Item window 之前材料的 consumer。若为此复制 Codex Thread/Turn search、增加第二套 conversation store，或新增一个
Desktop 专用搜索 RPC，都会让 Lyra 的 Session/Run/Item 身份和既有 `items.list` cursor 出现平行真相。

**决定**：Item body 仍是唯一 durable Transcript source。应用边界只从 user/agent message 与 Question prompt 生成 bounded
`SearchableText`；reasoning、Tool arguments/result 和 Conversation model journal 明确不进入索引。SQLite 在同一个 Item transaction
保存 private search projection，并用 external-content FTS5 + insert/update/delete trigger 维护派生索引；Session/Run cascade、fork、
rollback 与 import 因而自动共享同一 lifecycle，不创建异步 index worker 或第二 revision。

`search_conversations` 是 Run-frozen catalog 中的 deferred safe Tool，不是第 90 个 Runtime operation。它只允许 current Session 或
exact workspace scope，返回 bounded identity/time/snippet，并明确 historical excerpts 是不可信数据而非指令；它继续通过既有
`search_tools` 渐进发现。Desktop 只调用 `items.list(order=desc)`：initial query 与 200-Item snapshot 重叠的 page 自动跳过，用户每次
显式加载一个真实 older window；已加载材料支持 `Cmd/Ctrl+F`、Enter/Shift+Enter、局部高亮和 match navigation，不在 mount 时 eager
读取全历史，也不调用 Agent Tool 冒充 UI API。

**后果**：SQLite exact epoch 提升到 15；Runtime Protocol、89-method catalog、SSE event 与 generated contract shape 均不变。
R9 production 纵切闭合，U19 与 `search_conversations` 到 `implemented`；FTS corruption/rebuild、large-history cursor、rollback/import、
keyboard/a11y/visual/package 与资源门禁仍集中到 R11。

## ADR-A2-036：Compaction 是 immutable journal 上的可回退 context projection

**缺陷与反例**：协议已经声明 Compaction Item 与 `PreCompact`，但 app2 没有真实 producer；直接删除或重排 Conversation rows 会让
fork/rollback/export 的完整历史与 Run message ownership 一并丢失。把 Agent Framework checkpoint 当历史真相，或复制参考产品的
Thread/Turn compaction wire，同样会绕开 Lyra 已有 Session/Run/Item journal。

**决定**：完整 Conversation journal 永不因自动压缩删除。私有 `conversation_compactions` 以 absolute message ordinal、summary body、
creator Run 和 before/after count 保存 effective-context projection；多代 projection 独立存在，rollback 删除 creator Run 后可自然回退上一代。
post-Run bounded worker 只有在 message/byte threshold 触发后才建 candidate，并在任何 model call/commit 之前执行真实 `PreCompact`；deny 是
no-op，inject 只作为 trusted summary context。summary 与 Compaction Item 在同一 SQLite transaction 提交，CAS 防止并发新消息产生陈旧 winner。
Runtime startup 在 Run lost-recovery 之后重新排入各 Session 最新终态 root Run；完整 journal 与相同 CAS 让关机窗口恢复和重复排队保持幂等。

**后果**：SQLite exact epoch 提升到 16；Lyra protocol shape 与 generated artifacts 不变。O12 到 implemented，最终 summary quality、
concurrent next Run、rollback/fork/import、restart、fault 与 shutdown leak 仍在 R11 统一验证。

## ADR-A2-037：Remote 只改变 Lyra HTTP 部署策略，Desktop secret 归 OS keyring

**缺陷与反例**：local-only listener 与 generated client 的 loopback 限制让现有 remote contract 无法真正部署；若新增一套 remote method、
initialize handshake 或 descriptor reader，会破坏已经成熟的 Lyra wire，并把本地进程所有权错误延伸到远端。

**决定**：remote 继续精确复用 `/v2/rpc`、SSE、`/v2/info`、live 与 ready。Runtime 只有 explicit remote mode 可监听非 loopback，且必须
TLS、path-owned bearer token 和 exact non-wildcard CORS；每个 RPC 重读 0600 token file，允许原子 secret rotation。local bootstrap descriptor
与 remote mode 互斥。Desktop profile 只保存 origin 与 server name，bearer secret 由 OS keyring 持有；attach 对系统 TLS 与四个既有 endpoint
做同 instance/protocol 验证，remote instance replacement 只推进 connection generation，Desktop 不发送 stop signal。

通用 Desktop/generated-client connection 把凭据字段 breaking rename 为 `bearerToken`，不再用 `localToken` 把部署方式泄漏进传输模型；
endpoint contract 的认证语义同步从 `localToken` 收敛为标准 `bearer`，不提供旧字段 alias 或兼容 reader。

**后果**：Runtime method/event/error shape 零变化；contract generator 放宽 transport URL policy 到 loopback HTTP 或 origin-only HTTPS，
并生成 `bearerToken` connection 字段，manifest 记录 `bearer` endpoint authentication。这是允许的显式 breaking transport-client change，
不是引入第二套 remote protocol。
P01–P03 remote production path 到 implemented；offline/backoff/manual-local UI、secret rotation、TLS/CORS、slow-client 与 packaged app matrix
仍由 R10c/R11 完成。

## ADR-A2-038：内容渲染是 Lyra `ContentBlock` 的不可信 presentation boundary

**缺陷与反例**：原有 Desktop 用三反引号 split 区分 prose/code，无法表达 nested Markdown、table、math、diagram，也把未经前端约束的
inline image 直接拼进 DOM。若复制 Codex/其他参考产品的 Thread/Turn renderer 或直接引入其成套 streaming component，会把外部产品身份、
协议假设与 lifecycle 一并带入 Lyra；若手写 Markdown parser，则会在 presentation 层长期维护一套残缺语法和安全规则。

**决定**：现有 Lyra `Item.Content []ContentBlock` 是唯一输入合同，不增加 renderer-specific wire。Desktop 建立自有 content components：
semantic Markdown/GFM/CJK parse 后，raw HTML 经 basic allowlist，URL 由单一 policy 裁决；KaTeX 在清洗后 materialize。code/table/math/diagram
分别拥有最小 presentation owner，Shiki 与 Mermaid 按需 dynamic import；Mermaid 使用 strict mode、per-instance identity 和二次 SVG sanitation，
所有 enhancement 失败都回退 source。Markdown image 不自动加载 remote body，媒体只从经过 MIME/base64/size gate 的 image ContentBlock 进入 gallery。

图片落盘仍由既有 `NativeHost.SaveImage` authoritative validation 与 native dialog 完成；renderer 不接收路径、不实现 browser download。
lightbox 的 zoom/navigation/focus/reduced-motion 属于 transient local state，不写 Query cache、Session 或协议。参考 Codex/Streamdown 的只有 lazy
enhancement、安全分层与交互机制，不复制协议、component family 或 naming。

**后果**：Lyra Protocol、89 operations、topics/events、SQLite epoch 与 generated contract shape 零变化；新增前端 parser/render dependencies
只属于 Desktop bundle。U17/U18 到 `implemented`，CSP/bundle budget、streaming incomplete syntax、large payload、keyboard/screen reader、Retina/
WebKit 与 packaged native save 的证据在 R11 统一产出。

## ADR-A2-039：Appearance 是有限本地偏好，Runtime target change 是实例替换

**缺陷与反例**：旧实现曾把 theme、accent、font、density、plugin contribution 与任意 custom colors 聚在大型 persisted UI store；app2 若照搬，
会在只需要两个 taste axis 时重新建立 plugin kernel 与无界 token writer。另一方面，把 remote/local Runtime 当普通下拉偏好只替换 endpoint，会让旧
Query cache、selected Session、draft 与 stream owner 继续活在新实例上。

**决定**：app2 当前 appearance 范围只有 finite theme 与 accent。单一 `ShellPreferencesProvider` strict 读取/写入 local value、解析 system scheme、
监听 OS change，并把 resolved Linen/Graphite token ladder 与 functional accent 一次性发布到 root；组件不得各写 localStorage，也不开放 custom CSS/
theme plugin host。Mermaid 等需要 literal scheme 的 renderer 订阅同一 owner，并在自己的 serialized lifecycle 内重建。

Runtime target 仍由 ADR-A2-037 的 DesktopHost/remote manager/keyring owner 修改。Settings 只提交 write-only secret 或选择 saved/local profile；active
target 改变后，App 重新 bootstrap，并以 endpoint/instance/generation key remount Workspace。启动阶段 active remote 不可达时，failure surface 可调用
同一个 `UseLocalRuntime` owner 逃生，不增加 bypass 或第二连接 store。

**后果**：theme/accent/light-dark production path 到 implemented，locale/RTL 单独继续，避免伪完成；remote/local switch 不改变 Lyra method/event/
error shape。appearance persistence failure 不改变当前 in-memory preference，Runtime mutation failure 不替换当前 consumer tree。视觉、offline/restart、
keyring/native、WebKit 与 package 证据仍在 R11 统一验证。

## ADR-A2-040：本地化是 typed presentation language，不是协议或插件系统

**缺陷与反例**：旧 Desktop 的大型 i18next/plugin graph 混合 locale discovery、dictionary merge、raw key fallback 与组件文案，既难证明某个
locale 完整，也允许未翻译 key 在运行时泄漏。直接复制 Codex 的 key、文案层级或 locale lifecycle 会把参考产品身份带入 Lyra；反过来，仅设置
`html.lang` 或提供一个能选但只翻译部分页面的 selector，会制造不可验收的假能力。

**决定**：app2 以 English flat semantic dictionary 作为唯一 canonical key set，`MessageKey` 从它静态派生；每个可发布 locale 必须用 TypeScript
exact-key constraint 提供完整 dictionary，缺失 key 不允许 raw-key 或旧 app fallback。插值只接受 named string/number values；日期和数字由 active
locale 的 `Intl` formatter 负责。Context 提供完整 English default，允许独立 component/test 渲染，但 production 只有在 dictionary 真实加载后才
发布该 locale 并同步 `html.lang/dir`。locale 最终与 theme/accent 同属 Shell preference owner；不建立 registry、runtime plugin、remote translation
fetch 或第二 persisted store。

翻译只发生在 presentation copy、accessibility label 与 presentation error fallback。Lyra Protocol method/event/error code、Run/Item/Tool identity、
用户和模型内容、文件路径、命令、provider/model 名称及 durable fact 保持原值；需要人类化的已知状态由显式 display resolver 映射，不修改 wire。
旧 app locale 只作为用户能力与人工译文证据，Codex 只作为 locale lifecycle/RTL 机制参考，二者都不是 app2 import 或兼容来源。

**后果**：首批 typed boundary 可先迁移 production surface，而 selector 保持隐藏；只有全部 Desktop copy、八个既有 locale、Arabic RTL 与 logical CSS
闭合后，U22 才能到 `implemented`。本决策不改变 Runtime contract、SQLite、transport 或 Desktop bridge；compile、screen-reader、RTL visual、
WebKit 与 packaged evidence 仍在 R11 统一门禁产出。

## ADR-A2-041：命令目录是有限静态 read model，toast 不是错误真相

**缺陷与反例**：旧 Desktop 通过 plugin extension point 动态贡献 command、shortcut、settings pane 与 toast；对于 app2 随同一 bundle 发布的五个 shell
动作，这会重新引入 installation order、late contribution、collision repair 与隐式全局 listener。恢复 command palette 也只会给已有按钮/快捷键增加第三条重复路径。
另一方面，若每个组件自行监听 `keydown` 或随手弹 toast，modifier、IME、input scope、错误处理和 screen-reader 语义会持续分叉，瞬时提示还可能冒充
Runtime/Session 的 authoritative failure record。

**决定**：app2 只有一个 finite typed command catalog，稳定记录 ID、semantic label、scope 与 shortcut；同一 read model 同时驱动 dispatcher、
`aria-keyshortcuts` 和 Settings discoverability。global dispatcher 使用 physical `KeyboardEvent.code` 适配非美式键盘，按平台解释 Mod，并在执行前检查
composition、repeat、modifier、editable target、active overlay 与 enabled predicate。context-local menu/listbox navigation 可以使用具体 UI primitive，但不得
注册第二个 command registry 或 arbitrary command bus。所有 async command failure 在 dispatcher boundary settle，进入一个最多四条、可暂停和自动关闭的
transient toast owner。

toast 只表达刚发生的 action feedback；Runtime error、mutation conflict、HITL failure 与 recovery state 继续由原 surface/Query authority 展示。
toast 不持久化、不进入通知 feed、不影响 Run outcome，也不承载跨重启任务。command catalog 与 toast 都是 Desktop presentation detail，不增加 Lyra
operation、event、error、Wails method 或 plugin protocol。

**后果**：U23 可在不复活旧 plugin host 的前提下拥有统一 shortcut、可发现性和失败隔离；menu focus/keyboard、tooltip 与最终 collision/a11y/IME/
packaged evidence 继续在 R10e/R11 闭合。新增 shortcut 或 toast producer 必须进入同一静态目录/owner，不能直接增加 window listener 或第二队列。

## ADR-A2-042：长历史保留语义 DOM，follow-scroll 由 reader 独占

**缺陷与反例**：Narrative 与 Terminal 原先各自只在字符串长度变化时执行一次尾部滚动。图片 decode、Mermaid 增强、字体换行或窗口缩放发生在 effect 之后时，
尾部会无提示漂移；Narrative 的 reader escape 还只是不可观察 ref。直接引入 variable-height JavaScript virtualizer 虽能减少节点 layout，却会复制一份窗口状态，
破坏 loaded-history 搜索、精确 `scrollIntoView`、动态 disclosure 高度与 accessibility tree，并把分页 authority 从 Runtime 拉回 presentation。

**决定**：既有 `items.list(desc)` cursor 与显式 100-item page 是唯一长历史加载 owner，Desktop 不 eager 拉取全史。已加载 material 继续保留 semantic DOM；
对已完成且不拥有可见浮层的 Narrative/Terminal block，使用浏览器原生 `content-visibility: auto` 与 intrinsic block estimate 跳过离屏 layout，unsupported WebKit
自然回退完整渲染。Narrative 与 Terminal 共用一个 concrete follow-scroll hook，只有 reader 处于 tail threshold 内时才允许追尾；用户滚动、搜索或加载旧页会 escape，
新增 durable/live material 只产生可观察提示。`ResizeObserver` 同时观察 viewport 与 content，使 image、diagram、font 与 width materialization 经过同一 owner；不得新增
第二 scroll listener、item mirror、virtual window store 或 protocol field。

**后果**：长历史仍可被 DOM search、screen reader 与现有 highlight/navigation 消费，动态高度无需预测；大历史的内存上界继续由用户显式加载页数决定，而非伪装成
无限滚动。40px action target、fluid native-minimum geometry、IME 229 guard 与 CJK line-break 同属 Desktop presentation 收口，不改变 Lyra Protocol、Runtime query、
Session/Run/Item identity、Wails method 或 persistence。large-history、Retina、WebKit、keyboard/screen-reader 与 packaged evidence 在 R11 统一验证。

## ADR-A2-043：命令幂等以首个持久 outcome 为唯一权威

**缺陷与反例**：R11 入口验收发现，generated client、discovery limits 与 SQLite schema 已声明幂等能力，但 app2
operation endpoint 只解析 header，没有真正执行 claim、fingerprint、replay、conflict 和跨重启恢复。若只在内存按 key
去重，Runtime replacement 后会重复 mutation；若 completion 只返回 `error`，则“写入成功但确认丢失”时无法证明哪个
outcome 先持久化；若 pending claim 按时间过期，又会把未知提交状态误判成未执行。

**决定**：operation 依赖 persistence-neutral idempotency port，SQLite adapter 原子 claim 并保存 versioned safe outcome。
key 必须携带 exact discovered namespace；fingerprint 由 method、typed params 与协商后的 client capabilities 共同决定，
client identity 不参与业务语义。completed outcome 按公开 86400 秒 retention 回收，pending claim 永不自动过期。Store
completion 返回权威 durable record，使首个持久 payload 在确认丢失、重复 completion 或竞争恢复时始终胜出；claim 丢失
时只有重新赢得相同 fingerprint 的 fresh claim 才能补写已知 outcome，绝不重跑 mutation。`runs.start`/`runs.resume`
重放只重新附着已存在 Run stream；shutdown 在关闭 admission、join invocation 后 flush 已知 receipt，再关闭 SQLite。

**后果**：这是对既有 Lyra 幂等合同的落地，不修改 method、header、problem、wire shape 或 protocol version。R11 新增
HTTP 跨 Runtime replacement、namespace fence、query rejection、first durable winner、claim/ack loss、shutdown flush 与
Run reattach 证据；完整 race/fault 总门禁通过前相关 operations 仍保持 `implemented`。

## ADR-A2-044：内置 Tool 目录精确收敛为 30 项

**缺陷与反例**：R11 机械盘点发现，账本定义的文件与代码 family 是 `read/glob/grep/lsp/apply_patch`，但实际 catalog
还因底层通用工具箱顺手暴露了 `edit` 与 `write`，导致 model-visible 内置 surface 从 30 漂移到 32。同一文件 mutation
同时存在三种表达，会扩大 schema/context、审批和 Hook policy 的测试矩阵，并重新制造重叠入口。

**决定**：`apply_patch` 是唯一 model-visible 文件 mutation primitive；它覆盖新增、局部修改与删除。`edit`、`write`
不进入 app2 Run catalog，也不提供 alias。其余 29 项由真实 catalog 组装，`delegate_task` 由 agentexec deployment family
注入；测试从最终 executable definitions 收集名称并精确断言 30 项，MCP dynamic names 单独保留为开放外层。

**后果**：没有删除用户能力，也不改变 Runtime 89-method protocol；只是删除重复的模型调用语法，使账本、approval、
Hook、progressive discovery 与执行表面重新一致。新增内置 Tool 必须先更新 ADR/ledger 与 exact inventory test，不能因
底层库已经提供就自动暴露。

## ADR-A2-045：terminal problem 是 durable Run fact，不由冷读猜测

**缺陷与反例**：R11 provider fault 黑盒发现，同一个失败的实时 `segment.finished` 正确携带
`provider_unavailable`，但 `runs.get` 只根据 domain outcome=`failed` 猜成 `internal_error`；Session snapshot 与
JSON artifact 也经过另一份猜测逻辑。transport reconnect、restart 或 export/import 后，用户因此会看到与实时流不同的
错误分类。盘点同时发现 `timedOut` 冷读和 Segment 投影把 detail 放在协议禁止的位置，而没有 required Error。

**决定**：Run domain 继续只拥有 lifecycle outcome，不 import protocol；runflow 的 durable facts 与 metrics/profile 同批
保存 exact terminal `ProblemData`。`timedOut`、`failed`、`lost` 都通过一个 terminal projection 产生 Error，分别以
`timeout`、已分类 failure、`run_lost` 为 fallback；completed 不携带 detail，maxSteps/maxBudget/canceled 才携带 optional
detail。Session material presenter 与 artifact exporter 读取同一 fact，artifact import 把 portable problem 恢复回该 fact，
不建立第二套错误表或从 provider 文案反推类型。

**后果**：Lyra method、union、problem vocabulary、protocol version、SQLite table 与 artifact v2 shape 均不变化；这是对
既有合同的持久化落实。R11 用 provider 503 的 public HTTP/SSE→`runs.get`→export/delete/import→`runs.get` round-trip、
MCP dial failure，以及 timedOut/failed/lost wire-valid unit matrix 锁定实时、冷读与迁移语义一致。
