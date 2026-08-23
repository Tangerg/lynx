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
