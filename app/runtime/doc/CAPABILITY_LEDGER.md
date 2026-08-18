# Lyra Runtime 能力台账

> 状态：当前能力快照；P116 已封板，P117 Desktop workspace、Session title 与 production visual composition 纵切已完成。
>
> 基线日期：2026-08-18。

本文只回答“现在具备什么能力、谁拥有它、用什么证据守住”。详细 wire/storage 版本见
[`CONTRACT_BASELINE.md`](CONTRACT_BASELINE.md)，实施历史见
[`EXECUTION_PLAN.md`](EXECUTION_PLAN.md)。P0–P114 的逐批原始台账已冻结在 Git 快照
`babec316e:app/runtime/doc/CAPABILITY_LEDGER.md`，不在本文重复提交日志。

## 1. 总体 verdict

- Runtime 是 Lyra 的应用后端，同时提供 HTTP Runtime Protocol 与同进程 Go binding。
- 公共 Go API 仅由 `runtime/protocol` 和 `runtime/embedded` 拥有；内部 exported identifiers 不构成兼容承诺。
- 当前合同为 Protocol `2026-08-17`、Artifact v19、SQLite epoch 75、Agent Framework Baseline 20。
- Runtime 只经 `internal/adapter/agentexec` 消费 Agent Framework public API；Domain、Application、Infra、Delivery 和通用 Toolset 对 Agent Framework 零依赖。
- 真实产品是一个 Desktop actor 对一个逻辑 Runtime。HTTP、socket、同进程 binding、连接重建和 Runtime 进程重启不改变这个拓扑。SQLite 仍是 durable winner；局部 generation 只决定可替换进程内 owner 的提交权。
- P113–P116 已完成；P116 已完成 claimed Resume 补偿、Mutation Journal durable identity 校准与 published-boundary 静态守卫审计。

## 2. 架构与所有权

### 2.1 Domain

- Run、Segment、Transcript、Interrupt、Plan、Goal、Tool lifecycle 和 accounting 使用明确领域语言；不由 Delivery DTO 或数据库 row 反向定义。
- aggregate 保护状态迁移与跨事实不变量。Domain 不知道 Runtime transport、SQLite、Desktop、Agent Framework concrete types 或恢复实现。
- Goal incarnation、Run lineage、Interrupt occurrence、ToolCall identity 和 terminal outcome 是领域/应用事实；进程 generation、commit marker、lease 和 schema epoch 是技术身份，二者不混用。

### 2.2 Application

- Application use case 组织完整业务纵切、授权、事务输入和提交后的事件发布，不直接解析 Agent Framework private snapshot 或 SQLite shape。
- Run pump 是 authoritative model/tool observation 的唯一 reducer owner；外部调用事实只有在完整 write-set 提交后才能替换 live state。
- `sessions.snapshot` 在一个应用用例中校验并组装挂载 Session 的 HITL、Plan、Goal、Run、Tool material closure。
- fresh start、resume、child admission、waiting barrier、cancellation 和 terminalization 都有明确 command identity 与事务结算规则。
- staged executor 在 Segment opening 前由 `stagedExecutionHandoff` 唯一拥有；claimed Resume 由 `claimedResumeAttempt` 直接携带 claim 返回的 immutable `Pending` 提交匹配 `resuming` 的 terminal write-set，durable `RunLost` 成功后才释放 exact continuation executor，不再重读 open projection 或依赖每个错误分支手写补偿。

### 2.3 Adapter / Infra / Delivery

- `adapter/agentexec` 独占 Agent Framework 类型映射、TreeSnapshot encode/decode/restore、interaction input 和 observation 翻译。
- Toolset 只暴露 framework-neutral manifest/invocation 合同；具体工具、MCP、LSP 和 executor lifetime 由各自 adapter owner 管理。
- Infra 独占 SQLite、filesystem、Git、HTTP sidecar 和进程机制；它实现 Application ports，不创造业务终态。
- Delivery 独占 JSON-RPC/HTTP/SSE binding、strict validation、error mapping、version/capability negotiation 和生成合同；它不持有领域状态。
- Bootstrap 是唯一 composition root，required dependency 必须在 constructor/start boundary 被证明，不允许以 nil、安装顺序或静默降级表达依赖。
- Bootstrap 不发布 Stack/service locator，也不传播宽 assembly foundation。Policy、workspace、execution capsule 分别拥有共同构造不变量；Host 私有 application capsule 向 Delivery、startup recovery 和 workers 提供行为，Instance 只拥有完整 operation delivery 与 join handles。工具/执行 acquisition 仍在错误返回前转交唯一 `hostLifetime`，失败 Open 与重复 Close 保持逆序、可重试回滚。

## 3. 执行与交互能力

### 3.1 Root execution

- 支持 root Run fresh start、观察、取消、终态和资源释放；durable opening 成功后才激活 Agent Framework Process。
- model/tool observation 通过单一有序 stream 进入 Application；pre-call failure 禁止外呼，post-call receipt failure 形成 unknown 并由事务证明收敛。
- checkpoint 由 Application 持有 envelope identity，payload 对其不透明；`adapter/agentexec` 独占 Agent Framework TreeSnapshot v4。

### 3.2 HITL、resume 与 steer

- Question、approval、answer、checkpoint、Interrupt claim 和 continuation opening 形成一个可验证链路。
- open Interrupt 只能被 exact Session/Run/Item/occurrence 回答；edited approval 继续绑定原始 ToolCall identity，而非按 name/arguments 猜测。
- answer claim、Question answer/approval decision、checkpoint 删除和 commit receipt 原子提交。
- waiting tree 只从 quiescent complete-tree checkpoint 恢复；active-step crash 不伪装为可恢复。
- steer 使用 Framework-neutral signal identity；迟到或跨代 signal 不能写入 successor Run。

### 3.3 Child Run 与 waiting subtree

- Delegate child admission、reservation、conclusion、parent ToolCall 和 root waiting barrier 有一致身份链。
- prepared waiting-subtree capability 把 fallible staging 放在事务之前，durable commit 后只执行 contextless apply。
- terminal、rollback、restore、delete 和 boot recovery 会按 owner 清理 callback/reservation ledger；不靠 TTL 或全局 sweep 猜测。

## 4. Tool 能力

- Tool 发现、可见性、调用、审批、结果、diffstat、duration 和 cancellation 使用统一 ToolCall lifecycle。
- Tool start 不抢占 Transcript insertion order；同一模型 Tool batch 的完成事实按声明位置形成 canonical write-set。
- execution duration 排除审批等待；无法证明时保持 unknown，不从 wall-clock 或 UI loading 反推。
- 工具所属 Run 的取消表示为 `toolCanceled`，与执行失败、审批拒绝和父 Run 的 `childRunCanceled` 分离。
- MCP reconnect 和诊断 Tool 服从 Host/Runtime generation；旧在途调用、结果和 busy/error material 不能进入 successor。

## 5. Persistence 与恢复

### 5.1 SQLite

- 当前 schema epoch 75；一个 build 只接受一个精确 epoch，没有 migration chain、dual read 或 compatibility column。
- Session/Run/Interrupt/Goal/Transcript/checkpoint 的跨表不变量由事务和 strict reader 校验共同守住。
- 同一 canonical data directory 可由同版本 Runtime 共享；SQLite uniqueness/CAS 决定 durable winner，OS advisory lease 只协调活跃业务 owner。
- Runtime 进程死亡由内核释放 lease；boot recovery 采用单一 winner 和固定顺序，不使用 TTL/heartbeat 猜测存活。

### 5.2 回执丢失与幂等

- Runtime durable idempotency namespace 与数据库 identity 同生共死；保留数据库重启不变，重建数据库时变化。
- `runs.commit_segment_id` / `runs.commit_id` 证明 exact Application command write-set 是否已经 COMMIT。
- EventCommit、opening、terminal、HITL answer claim、waiting barrier 和 child cancellation 在事务尾写入 marker；事务返回错误后只做 request-detached exact proof。
- 相同 identity replay 可收敛成功；不同 identity、不同 Segment 或 recovery 代际 fail closed。普通 Suspend/Resume/Restore 不沿用旧 marker。

### 5.3 冷启动与进程恢复

- Runtime 每次启动发布新的 opaque `instanceId`；info/live/ready/discovery 必须同源一致，Desktop 才提交 ready inspection。
- 同 endpoint、同版本重启形成新的 process/event incarnation，但仍属于同一逻辑 Runtime 和 durable store。进程重启前创建的 response、event iterator、stream callback 和 teardown 只能结算其进程内 owner。
- local transport token 由 durable data path 唯一拥有：首次以 0600 完整候选原子发布，后续 Runtime generation 读取并严格校验同一 32-byte credential；显式删除/替换文件才构成凭据轮换。
- SIGKILL 后 SQLite durable material、HITL、Plan、Goal、Run、Tool 与 navigation 通过权威 snapshot/stream handoff 恢复，不拼接旧进程内存。
- boot reconciliation 的 Session writer claims 由 exact-once `recoverySessionClaims` 持有并逆序释放；`recoveryPlanner` 继续独占 ownership-scoped snapshot 到 atomic `RecoveryCommit` 的推导。
- transaction failure 与 success receipt loss 分别由 rollback 和 exact marker proof 处理，不靠刷新、延时重试或 optimistic 猜测。

## 6. Desktop 真实接线

### 6.1 Renderer、Host 与 Runtime generation

- `DesktopRenderer`、Plugin Host、Runtime connection、command cohort、query writer 和 mounted Session projection 在真实可替换边界拥有 exact generation；generation 不代表第二个逻辑 Runtime。
- 进程内 owner replacement 先发布新实例，再同步退休旧实例；final close 不可逆，重复 dispose 共享 settlement。
- 异步 workflow 跨可替换 owner 时在 admission 捕获 exact dependency。只有等待期间可能发生 replacement 且下一步修改当前共享状态时，才重新证明 apply/cleanup 权。
- replaceable application owner 共享一个只持有 process-local exact object identity 的 publication slot；业务 task、abort、serialization、cache repair、typed error 和 material 仍由 concrete owner 持有。生产代码不再以每类 `static #active` 复制 lifecycle protocol。
- cold-start ports 由 composition root 依赖图显式保证；Composer、Recipes、Workspace Events 不依赖偶然插件安装顺序。

### 6.2 Mutation、query 与 material

- Desktop durable mutation journal 只持久化当前 v2 shape 的未决命令身份：salted fingerprint、idempotency key、Runtime durable namespace 与 retention boundary；transport endpoint、请求参数、renderer owner、generation、lease、heartbeat 和 settlement 状态不落盘。
- renderer-local exact object identity 是 mutation response/error/cleanup 的唯一提交权。replacement 先发布 successor 再退休 predecessor；旧异步结果和旧 disposer 不能删除 successor 复用的 durable identity。
- 同一 Runtime durable namespace 可在 renderer/client 重建以及 HTTP/socket/in-process binding 变化后恢复 exact idempotency key；只有 namespace 变化或 retention 到期才退休该 durable identity。
- command owner 持有 single-flight/serialization、optimistic effect 的补偿、navigation 和迟到 settlement；裸 gateway/singleton locator 不能绕过 owner。
- DATA_PROVIDER read 在入口一次捕获 Runtime client 与 query generation；多阶段 RPC 不跨 transport 拼接。
- query handoff 捕获交接时真实 Query identity，迟到 cancel/reset 不能命中交接后才创建的 successor Query。
- mounted Goal 已并入 `sessions.snapshot` shared material；Plan、HITL、Run、Tool 同属一次 immutable Session view，只有获胜 refresh token 可以提交。
- event 是失效/恢复信号，不是第二 material writer；mutation response 只提交自己证明的事实。

### 6.3 产品表面

- Run Summary 按 exact root Run 聚合全部 continuation Segment，并使用 authoritative outcome 区分 success/error/canceled/limit/unknown。
- Terminal 与 Tool selection 使用 exact Tool identity；长对话 compaction 或 material replacement 删除目标时会确定回退或清空，不悬挂旧 selection。
- completed Tool 可从 durable end-only material 恢复 command、files 和 approval，不要求 live-only `tool-start`。
- Context Dock 以 Session identity 隔离 presentation scope；URL 唯一拥有 active destination，`contextDockStore` 仅持久化 exact Session 的 open tab、last view 与 file target，使折叠/恢复、Session 切换和 renderer replacement 保留本 Session 工作区而不复活其他 Session 的 tab/scroll/feedback。Agent Session open set 负责同步退休已关闭 scope；Tool selection/expanded material 不持久化，旧版或 invalid scope 整体丢弃。
- 无 active Session 时不挂载 Context Dock destination、view 或 toggle；Runtime 默认 workspace 只可用于显式创建 Session，不能冒充 Session-owned material。
- 顶层 New Session 继承点击时 active Session 的 exact cwd；active summary 尚在 resolving 时禁用该动作，不回落到 Runtime 默认目录。目录选择由 Projects 标题栏唯一拥有，project row 的 `+` 继续表示在该 exact cwd 建立 Session。
- Session title maintenance 只有 `runsegment.Finalizer` 一个 owner，并只经 Session Application first-writer 持久提交；utility model 缺失、空回复或 provider error 时，opening user text 的首个有效行提供 Unicode-safe、有界 deterministic fallback。provider error 仍进入既有 maintenance telemetry，不以“未命名会话”或 Frontend 第二 writer 吞掉降级事实。
- Goal、Plan、HITL/审批只呈现当前 projection generation；accepted mutation intent 可在 authoritative projection 追平前保持稳定 busy 反馈，不写第二 cache。
- 流式消息的底部点赞/点踩与操作行只在所属可见 turn 达到稳定展示边界时出现，不随每个输出 delta 反复挂载。
- 中央 transcript 的 mount geometry、异步 Markdown/Shiki materialization 与用户滚动共享 `MessageStream` 一个 presentation owner：跟随状态来自既有 scroll library；DOM materialization 只在仍位于 tail 时即时补偿，wheel/scroll escape 后不再改写 reader-owned `scrollTop`，也不靠固定 RAF/timer 窗口判断布局完成。
- 右栏 Diff 与 File preview 共用 file-path → Shiki grammar 映射；preview 以 query 对应的 exact path 选语言并进行一次 whole-file highlight，未知 grammar 降为 text，不从内容猜测或复制 extension table。

## 7. 公共合同

- Runtime Protocol 当前版本 `2026-08-17`，唯一 replay scope 为 `runtimeInstanceRootSegment`。
- Artifact 当前版本 19；旧版本在写入前确定性拒绝，不猜测缺失事实。
- SQLite 当前 epoch 75；shape 变化必须一次前移 owner codec、fresh schema tests、baseline 与生成物。
- Agent Framework 当前 Baseline 20；Runtime 不依赖 private state 或迁移前 module path。
- 所有生成合同必须 diff-free；consumer 缺口记录在 [`CONSUMER_HANDOFF.md`](CONSUMER_HANDOFF.md)，服务端不为消费者恢复旧字段。

## 8. 结构清理结论

- required dependency 已从运行期偶然性提升为构造/启动期合同。
- shared map、cancel set、replacement、join 和 retirement 必须由单一并发对象拥有。
- 文件/package 按 change reason 和边界拆分，不按行数拆分；只有一个调用者、没有独立不变量/生命周期/替换价值的微模块应吸回 owner。
- 静态 extension registration 直接属于 plugin composition entry；只投影 `{id, order, component}` 等对象字面量的 application factory 与对应 literal-only test 不构成独立边界。保留的 contribution module 必须拥有策略、行为、映射不变量或跨 context published language。
- 通用 helper 只在确有多个独立消费者且语义稳定时存在；不为测试便利暴露 singleton accessor 或 raw owner getter。
- 空目录、迁移 alias、compat adapter、双状态 codec、刷新旁路和 shadow owner 不属于当前架构。

## 9. 验收证据

| 维度               | 当前守卫                                                                                                                                                                                                                                                                                                    |
| ------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Domain/Application | aggregate/use-case 单元测试、事务失败与状态迁移反例                                                                                                                                                                                                                                                         |
| Agent Framework    | public baseline、architecture import gate、snapshot/interaction/child recovery tests                                                                                                                                                                                                                        |
| SQLite             | fresh schema、codec、CAS/uniqueness、cross-table invariant、真实 reader/writer 与 SIGKILL recovery                                                                                                                                                                                                          |
| Protocol           | strict validation、golden samples、manifest/OpenRPC/schema/Go API digest 与 generator diff                                                                                                                                                                                                                  |
| Desktop state      | exact-generation replacement、late settlement、Session material、query writer 和 navigation tests                                                                                                                                                                                                           |
| Frontend           | type/lint/format/knip/layer/API/style/design/token/locales/bundle 全门禁与 `--detectAsyncLeaks`；production visual composition 必须提供真实 setup service 和 command/interrupt lifecycle owner，当前 agent/workspace/closure 矩阵 255 tests 覆盖 HITL、Dock、WCAG、长内容、IME、Retina 与 light/dark golden |
| Production shell   | Wails v3 Go test/vet/build、生产冷启动、Runtime health/discovery 与 fresh database smoke                                                                                                                                                                                                                    |

最近完整基线为 Frontend 308 files / 1926 tests，普通与 `--detectAsyncLeaks` 全绿；Runtime standalone 全量 test/vet/build 与全包 race 通过；Desktop Go test/vet/build、Wails v3 production package、fresh database 冷启动与真实 SIGKILL replacement smoke 通过。隔离 smoke 中 Runtime PID 37363→37494、`instanceId` 换代、local token 哈希保持一致，Desktop 进程存活，loopback established connections 维持 12，前后 discovery/RPC 均为 200。数字只表示最近一次封板证据，不替代后续改动必须重跑受影响门禁。

## 10. 已知未闭环

- P117 仍需完成 streaming、HITL continuation、renderer replacement、Runtime restart、Run/Terminal/Diff/Goal/Plan 的完整可见恢复矩阵与 Wails production 验收。

## 11. 当前结论

P0–P116 已把主要缺陷从“调用处补判断”上移到领域不变量、Application transaction、进程/renderer generation、credential lifecycle 和 read-model owner；P116 又消除了 claimed-state 自身不可见、transport binding 冒充 durable identity，以及一次性重构语法被永久制度化三处耦合。后续工作必须从真实产品反例开始；若不能说明唯一 owner、提交能力和失败后的 durable winner，就不能以新增 helper、刷新或兼容路径进入生产代码。
