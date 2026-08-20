# Lyra Runtime 能力台账

> 状态：当前能力快照；P122 已完成。
>
> 基线日期：2026-08-20。

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
- P113–P122 已完成；Work Index、Agent Narrative、Composer standing stack 与 Context Dock 的 Codex 深度对齐、权威前端接线和 production renderer/Runtime 恢复验收均已闭环；文件变更叙事只消费 exact ToolCall 的持久补丁回执，审批请求只呈现 Runtime 权威 material 与用户明确选择的 allow scope。

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
- pending approval 使用单一 24px Codex request surface：工具身份、Runtime reason 与 command/args 是唯一可见事实；risk badge、泛化 approval 标题、客户端危险正则、scope/grant/reversibility 猜测和 checkbox 不进入 production renderer。
- Allow once 与注册的键盘 approve 始终是不带 remember scope 的一次性动作；只有用户从 split action 明确选择 Session/Project/Global 时才随 approve 提交 scope，Deny 永远不携带 allow scope。edited args、Runtime unavailable、optimistic settlement 与 exact Run/Item resume 继续由既有 owner 持有。
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
- Context Dock 只在 live row 同时容纳 640px conversation 与 420px Dock 时呈现；ResizeObserver 提供只读 presentation capability，空间不足由既有 navigation owner 折叠 destination，并保留 exact Session 的 tab membership、last view 与持久宽度。安全宽度恢复后 toggle 重新可用，用户重开原 panel；窄窗不把 transcript/HITL/composer 压成第二种 compact UI，也不覆盖左侧 drawer preference。
- Goal lifecycle 与当前 Run command 在同一中栏具有明确作用域：Goal 通过与 composer 同宽、重叠 1px 接缝的 top-tray surface 呈现专用 goal-arrow、lifecycle、objective 与 pause/resume；预算、额度、花费、步数、model 与 last move 不成为永久 chrome。composer 的 Run stop 保持简洁，两者继续调用各自既有 command owner，不靠重复按钮名或 aria-only 补丁隐藏命令差异。
- 无 active Session 时不挂载 Context Dock destination、view 或 toggle；Runtime 默认 workspace 只可用于显式创建 Session，不能冒充 Session-owned material。
- 顶层 New Session 继承点击时 active Session 的 exact cwd；active summary 尚在 resolving 时禁用该动作，不回落到 Runtime 默认目录。目录选择由 Projects 标题栏唯一拥有，project row 的 `+` 继续表示在该 exact cwd 建立 Session。
- Runtime command capability 同时约束顶层 New、Projects 目录选择、project-row `+` 与 palette/快捷键 New；断线时这些真实 `sessions.create` 入口同步撤权，不以失败 toast 充当 availability。native picker 返回后在投递前重新证明当前 capability。顶层 New 作为 active-project blank destination 可复用当前空 draft，显式 Projects 创建仍始终分配新 Session。
- project-row `+` 只在 `sessions.create` 返回新 Session identity 后把焦点交给 composer；command owner 拒绝创建时保留当前 Session 的焦点与可见上下文，不把旧 composer 冒充新建结果，也不写第二导航状态。
- Session title maintenance 只有 `runsegment.Finalizer` 一个 owner，并只经 Session Application first-writer 持久提交；utility model 缺失、空回复或 provider error 时，opening user text 的首个有效行提供 Unicode-safe、有界 deterministic fallback。provider error 仍进入既有 maintenance telemetry，不以“未命名会话”或 Frontend 第二 writer 吞掉降级事实。
- Goal、Plan、HITL/审批只呈现当前 projection generation；accepted mutation intent 可在 authoritative projection 追平前保持稳定 busy 反馈，不写第二 cache。
- Plan 只在 active Run 期间以环形进度和“第 N / M 步”pill 呈现；完整 Session plan 由同一 projection 在 hover/focus tooltip 展开，不复制成 disclosure card、progress bar 或第二 expanded state。Plan 位于 composer overlay，Goal 使用 overlay 中的 attached top tray；空 contribution 不留下固定边线或高度。
- Composer Context 环把当前 root Run 的最新 `segment.progress.contextTokens` 与 active Session 实际 served model 的 `contextWindow` 配对，按 `min(used, window) / window` 投影占比；终态只保留最后一次真实 context footprint 并退休其他瞬态 progress。缺任一权威事实时不绘制，Session 累计 usage 不再参与该读数。
- 标题栏不呈现 Session 累计 token/cost；普通 completed/failed Run 不在最终回答后重复绘制耗时、步数、token、费用或“完成”结算条。canceled/limit 的 quiet reason 与 actionable failure recovery 仍按各自既有 owner 呈现。
- Composer 的 composition lifecycle 是 Enter 提交判定的唯一 owner：`compositionend` 后保留一次性 commit intent，首个无修饰 `Enter/keyCode=13/isComposing=false` 只结束中文输入法的中英混合文本提交，随后明确 Enter 才发送。active composition、浏览器缺失 `compositionend` 的 plain-input recovery、focus/pointer/paste/drop retirement 与 Mod/Shift shortcut 均消费同一 intent，不使用 timeout、timestamp/UA 猜测、第二 draft 或第二发送入口。
- 流式消息的底部点赞/点踩与操作行只在所属可见 turn 达到稳定展示边界时出现，不随每个输出 delta 反复挂载。
- 用户消息 bubble 使用独立 semantic marker 和 theme-owned 5% text neutral surface；12×8px inset、16px 圆角与 77% 行宽共同保持 Codex 的低强调层级，不复用 accent selection/command feedback token。
- 中央 transcript 的 mount geometry、异步 Markdown/Shiki materialization、实测 composer clearance 与用户滚动共享 `MessageStream` 一个 presentation owner：跟随状态和 exact target 来自既有 scroll library；DOM mutation 与 border-box ResizeObserver 覆盖内容和动态 padding 两种高度来源，compact HITL 首次打开即可把 blocking action 留在 composer 上方，wheel/scroll escape 后不再改写 reader-owned `scrollTop`，也不靠固定 RAF/timer 窗口判断布局完成。
- 右栏 Diff 与 File preview 共用 file-path → Shiki grammar 映射；preview 以 query 对应的 exact path 选语言并进行一次 whole-file highlight，未知 grammar 降为 text，不从内容猜测或复制 extension table。目标行定位只响应 path/content/line navigation，Shiki 异步 materialization 不重放滚动或覆盖 reader-owned 位置。
- File preview 的 generic `File` tab 与 material identity 分工明确：dock view bar 从同一 `viewer.path` 呈现左侧截断的 exact path，并与 line count/truncation 同行；full placement 继续以 path 为 title。文件选择、query 参数、syntax grammar 与可见身份不再分叉。
- Timeline 在 Runtime 不可接收命令时继续呈现 exact Session/Run 审计事实，但 locate/cancel 共同由 Runtime command capability 禁用；恢复后沿同一 owner 重新可用，不从 connection phase 复制第二布尔状态。
- 右栏 tab strip 保留可读标签；active identity 变化时由 tab owner 执行 nearest scrolling，strip 自己的 scroll geometry 驱动 start/end edge fade。renderer 恢复或 picker 打开末尾 tab 后选中身份始终可见，两侧隐藏内容也有明确提示。
- 中栏 Markdown presentation 由一个 renderer owner 统一持有：semantic rich/plain table copy 与展开预览、Shiki code wrap/copy、HTML/SVG fence 隔离、Mermaid、selection copy、matched citation、image-only grouping、message-scoped image gallery、inline/fenced LTR isolation、Han/RTL direction、Codex heading/paragraph/list/task/blockquote/rule rhythm 均消费同一 message material。表格正文固定 14/21px、表头 14/16px、容器零额外块间距；task checkbox 是 list grid 的直接子项，正文统一位于第二列；行内代码使用可换行的 cloned neutral well、0.92em 字号、6px 圆角与 `overflow-wrap:anywhere`。图库在 nearest exact message-content root 内提供全屏 90% 黑色画布、显式关闭、前后/方向键导航、Escape、100–400% 缩放与切图重置，并排除 nested delegated message；代码块使用同材质 14px sans caption、4×8px header、8px source inset，且不设人工高度上限。模型 raw HTML 只允许无属性 basic inline 标签与 `br`；远程图片、style、native disclosure/layout 和属性注入不会成为活动 DOM。上述能力不持久化、不注册全局 owner。
- KaTeX 样式由既有 Markdown 动态 loader 唯一拥有；构建分块必须把 `katex.min.css` 与静态可达的 KaTeX JavaScript 分离，产物可以存在但不得由 `index.html` 提前引用。当前启动 CSS 为 107.0KB，动态 math CSS 为 29.0KB，bundle gate 同时约束启动预算和 lazy ownership。
- context compaction 使用左对齐 quiet activity row 而非居中 divider；压缩图标保持可见，Runtime 提供 summary 时沿同一可键盘操作的 disclosure 展开，不增加第二 transcript/material owner。
- Agent message 的 authoritative phase 尚未进入公共能力：当前 Protocol/persistence/Artifact/SDK 不能区分 commentary 与 final answer。Frontend 保持现有 durable transcript 顺序，不从 streaming/位置/文案推断分组；该能力必须先获 Runtime Protocol、SQLite/Artifact 与 public generated surface 的 breaking-surface 授权。
- 图片预览 Download 尚未进入公共能力：Frontend 没有 Desktop 原生 save-file owner；在获准增加 Desktop binding/生成 surface 前，不以浏览器下载伪装桌面能力。

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

| 维度               | 当前守卫                                                                                                                                                                                                                                                                                                                                                     |
| ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Domain/Application | aggregate/use-case 单元测试、事务失败与状态迁移反例                                                                                                                                                                                                                                                                                                          |
| Agent Framework    | public baseline、architecture import gate、snapshot/interaction/child recovery tests                                                                                                                                                                                                                                                                         |
| SQLite             | fresh schema、codec、CAS/uniqueness、cross-table invariant、真实 reader/writer 与 SIGKILL recovery                                                                                                                                                                                                                                                           |
| Protocol           | strict validation、golden samples、manifest/OpenRPC/schema/Go API digest 与 generator diff                                                                                                                                                                                                                                                                   |
| Desktop state      | exact-generation replacement、late settlement、Session material、query writer 和 navigation tests                                                                                                                                                                                                                                                            |
| Frontend           | type/lint/format/knip/layer/API/style/design/token/locales/bundle 全门禁与 `--detectAsyncLeaks`；production visual composition 必须提供真实 setup service、Runtime service status 和 command/interrupt lifecycle owner，当前 agent/shell/workspace/closure/foundation/WebKit 矩阵 312 tests 覆盖 streaming、HITL、Session/Dock、WCAG、键盘、coarse pointer、长内容、IME、CJK、18px、reduced motion、Retina 与 light/dark golden |
| Production shell   | Wails v3 Go test/vet/build、production `.app` package、renderer reload、Runtime health/discovery 与 fresh database/SIGKILL smoke                                                                                                                                                                                                                             |

P122 Frontend 基线为 322 files / 1999 tests；Approval request hierarchy、scoped approve、纯 Deny、edited args、键盘一次性允许和 published presentation facade 契约红例全绿。light/dark/menu/320px 实图与包含 6 张 Agent、2 张 Retina Approval golden 的完整 visual/WCAG/keyboard/IME/CJK/Retina/WebKit 矩阵 312 tests 全绿，typecheck/lint/format/knip/design/token/chrome/locales/bundle 等全部门禁保持全绿。P121 的调用级补丁回执能力保持不变；P119 的完整 Runtime/Desktop/Wails recovery 基线也未被修改。P122 没有改变 Protocol、Artifact、SQLite schema、公共 Go API、Frontend published SDK 或 Agent Framework baseline。

Runtime standalone 与 Desktop 全量 test/vet/build、Wails v3 production package 和 strict codesign verification 通过。fresh HOME/SQLite smoke 中 renderer reload 后权威 `sessions.list` 与 SQLite 均保持唯一 Session；Runtime PID 89768→93411、`instanceId` 换代，Desktop PID 90579 保持，0600 durable token digest 不变。同一 renderer 在锁屏后台且没有 reload 或手工刷新时自动连接后继 Runtime 并恢复 RPC。数字只表示最近一次封板证据，不替代后续改动必须重跑受影响门禁。

## 10. 已知未闭环

- P122 完成定义内没有已知未闭环项。旧 approval risk/scope/reversibility/danger presentation helpers 仍保留在 Frontend published facade 以避免未授权 breaking cleanup，但 production renderer 已不消费或展示这些客户端推导；其删除与 `ToolCall` 历史兼容字段、commentary/final answer phase、原生图片 Download 一样，分别等待显式 breaking-surface、Runtime public surface 或 Desktop binding 授权。

## 11. 当前结论

P0–P122 已把主要缺陷从“调用处补判断”上移到领域不变量、Application transaction、进程/renderer generation、credential lifecycle、read-model 与 presentation owner；P120 把中央 Plan/Goal/Context/terminal narrative 从“移动旧 chrome”收敛为 Codex 的紧凑 standing grammar，P121 让文件变更 narrative 与右侧 Run Summary 共同服从 exact ToolCall 的持久补丁回执，P122 又让审批可见事实与动作 scope 回到 Runtime material 和用户明确意图。后续工作必须从真实产品反例开始；若不能说明唯一 owner、提交能力和失败后的 durable winner，就不能以新增 helper、刷新或兼容路径进入生产代码。
