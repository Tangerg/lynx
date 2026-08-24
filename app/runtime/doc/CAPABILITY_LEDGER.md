# Lyra Runtime 能力台账

> 状态：当前能力快照；P156 已完成。
>
> 基线日期：2026-08-24。

本文只回答“现在具备什么能力、谁拥有它、用什么证据守住”。详细 wire/storage 版本见
[`CONTRACT_BASELINE.md`](CONTRACT_BASELINE.md)，实施历史见
[`EXECUTION_PLAN.md`](EXECUTION_PLAN.md)。P0–P114 的逐批原始台账已冻结在 Git 快照
`babec316e:app/runtime/doc/CAPABILITY_LEDGER.md`，不在本文重复提交日志。

## 1. 总体 verdict

- Runtime 是 Lyra 的应用后端，同时提供 HTTP Runtime Protocol 与同进程 Go binding。
- 公共 Go API 仅由 `runtime/protocol` 和 `runtime/embedded` 拥有；内部 exported identifiers 不构成兼容承诺。
- 当前合同为 Protocol `2026-08-24`、Artifact v23、SQLite epoch 82、Agent Framework Baseline 20。
- Runtime/Desktop 只接受当前精确 Protocol 版本；没有上一发行版 baseline、版本范围或兼容 reader。生产协议不再声明无生产者的 `custom` RunEvent、`clientTools` feature 或 `toolResult` interrupt/response variant。
- Plan 是一等 `plan.updated` / `plan.changed` / `plan.get` / `SessionSnapshot.plan` / `SessionArtifact.plan` 资源；没有通用 state registry、state key/scope/writer metadata、Artifact union 或 Desktop shared-state Plan reader。Artifact v23 只接受当前显式 `plan` shape。
- Desktop 只加载编译进同一 bundle 的内置插件；Wails Bootstrap 只返回本地 Runtime 连接，不扫描 `~/.lyra/plugins`，前端不执行用户目录 JavaScript、不发布 `window.__LYRA__`，也没有外部 manifest、Host API version、permission whitelist、origin 双态或 lazy-activation placeholder。图片、粘贴和 `@file` 附件仍由 Composer 自己拥有。
- Runtime 只经 `internal/adapter/agentexec` 消费 Agent Framework public API；Domain、Application、Infra、Delivery 和通用 Toolset 对 Agent Framework 零依赖。
- 真实产品是一个 Desktop actor 对一个逻辑 Runtime。HTTP、socket、同进程 binding、连接重建和 Runtime 进程重启不改变这个拓扑。SQLite 仍是 durable winner；局部 generation 只决定可替换进程内 owner 的提交权。
- P113–P156 已完成；Work Index、Agent Narrative、Composer standing/input stack 与 Context Dock 的 Codex 深度对齐、权威前端接线和 production renderer/Runtime 恢复验收均已闭环；Work Index 由单一 shell geometry 拥有 275px default、240px floor、520px ceiling 与 240px live reading remainder，首帧、resize、ARIA、偏好和 visual fixture 不再各写宽度；Context Dock 的 loading/Shell 渲染与几何 observer 共用 `shellVisible` 派生事实，窄窗从首个可交互帧就服从 640px conversation floor，不再先打开后闪退；projectless welcome composer 由 overlay top 提供 Codex 缩进 rear tray，项目选择继续服从 Work Index/native picker 与唯一 Session Workspace owner，不在 footer 或第二 selection state 复制；文件变更叙事只消费 exact ToolCall 的持久补丁回执，审批只呈现 Runtime material 与明确 allow scope，exact `itemId` 的待决 ToolCall 与 approval 虽同时保留为可恢复事实，但只由 approval request 拥有可见命令与动作，结算后工具历史自然恢复；Question 只在 composer rung 呈现一次并以 ordered empty values 表达明确 Skip，普通工具调用统一使用透明 work-narrative row 而非状态着色卡片；设置主色由单一 appearance preference 驱动；Goal update/clear 由 Runtime 权威纵切拥有，新 Goal start 通过同一 Composer submit mode 与 per-Session command owner 提交，不展示限制条件或建立第二 draft/writer；AgentMessage phase 由 Runtime terminal boundary 权威写入，Frontend 将 commentary work narrative 与 final answer 分行且只给最终回答 actions；fresh autonomous Goal 的 Application 控制输入只进入 provider Conversation，不再伪装成用户 Transcript Item；图片查看器的 Download 由 Wails DesktopHost 原生 save-file owner 完成，不使用浏览器伪下载；全部 dialog 在 light/dark 下共用 `#00000022` modal scrim，由各自 surface 承担边框、材质和阴影深度；Run 最后一次真实 `contextTokens` 现在跨终态、renderer reload、Runtime restart 与 Artifact import 恢复，Composer Context button 保持原光学中心并支持键盘 tooltip；中央流式滚动只让 raw follow lock 写 exact tail，wheel-up 后 public near-bottom 展示值不能抢回读者位置。Session workspace 已从松散 `cwd` 字符串收敛为 Domain exact value，存在性与物理 canonicalization 仍只属于 filesystem adapter；checkpoint/Pending/recovery 的长期不变量由行为测试而非逐文件局部变量 marker 守卫；operation catalog 现在唯一发布 idempotency 与 replay cursor 的适用性，HTTP、embedded、生成合同和 Desktop preflight 不再各自猜测或静默忽略带外元数据；Runtime/Desktop 的 module、standalone workspace 与 external-binding fixture 已统一到 Go 1.27.0；Bootstrap Host 的 component/executor join 与 terminal Sequence 共用一个独立于 caller wait 的 shutdown generation，关闭诊断不阻塞已终结依赖，失败 Open timeout 后也不会丢 cleanup owner。

- P152 进一步把 Bootstrap shutdown 所有权从 Host 提升到完整 Instance：operation Endpoint、scheduler/database/recovery workers 与 Host 现由一个 Instance-owned generation 连续加入，CLI/public Close 的 caller timeout 不再丢弃下层图；Instance 直接消费 Host 已注入 lifetime，不复制第二 context owner。
- P153 删除没有 Agent consumer、只剩手动客户端入口的独立 Codebase 向量索引纵切：Runtime 不再发布相关 operation/feature/topic/schema 或后台 owner，Desktop/CLI direct consumer 同步消失；代码发现由 workspace grep/file 与 LSP 等可组合能力承担。Embedding role/vector codec 继续由 Agent Memory 真实消费，未配置时 keyword fallback 保持可用。
- P154 把 Agent Memory vector 从 Curation 后台职责收敛为 Search-owned derived cache：每条 cache 绑定 exact embedding space 与 content digest，role/维度/内容变化在下一次搜索惰性重算；迟到 I/O 只能在 item 仍 active 且 digest 未变时提交。embedding 不可用或 cache 写失败仍走 keyword signal，不建立第二 rebuild lifecycle。
- P155 把 Agent Memory recall 从 caller-selected 单 scope 收敛为当前项目上下文的联合 corpus：exact-project 与 user-scope active items 在一个 SQLite snapshot 中共享 query signal、ranking 与全局 top-k，未 pinned 的用户偏好不再成为无 Agent consumer 的持久孤岛。
- P156 让 Agent Memory Domain 唯一拥有 4096 Unicode-character item/ledger fact 上限，并由 SQLite epoch 82、生成 request/result validators 和 Desktop 合同消费；pinned/per-turn recall 都以 whole-item 4096-token aggregate budget 截断，首项不再例外。

## 2. 架构与所有权

### 2.1 Domain

- Run、Segment、Transcript、Interrupt、Plan、Goal、Tool lifecycle 和 accounting 使用明确领域语言；不由 Delivery DTO 或数据库 row 反向定义。
- aggregate 保护状态迁移与跨事实不变量。Domain 不知道 Runtime transport、SQLite、Desktop、Agent Framework concrete types 或恢复实现。
- Goal incarnation、Run lineage、Interrupt occurrence、ToolCall identity 和 terminal outcome 是领域/应用事实；进程 generation、commit marker、lease 和 schema epoch 是技术身份，二者不混用。
- Session 直接拥有 immutable `Workspace`：路径必填、绝对且 lexical clean；它不查询目录存在性、不解析 symlink，也不施加 app2 的 filesystem-root 限制。

### 2.2 Application

- Application use case 组织完整业务纵切、授权、事务输入和提交后的事件发布，不直接解析 Agent Framework private snapshot 或 SQLite shape。
- Session Application 是 Runtime 默认模型的唯一安装点：fresh/scheduled admission 把 configured provider/model pair 写入聚合；Runs 省略选择时只读 Session pair，不再持有第二份全局默认。
- Session Application 是 workspace admission 的唯一协调点：filesystem port 证明存在性并返回物理 canonical identity，随后构造 Domain `Workspace`；restore/fork/relocation 不绕过同一 exact value。
- Run pump 是 authoritative model/tool observation 的唯一 reducer owner；外部调用事实只有在完整 write-set 提交后才能替换 live state。
- fresh autonomous Goal opening 显式区分 model-only control input：opening transaction 原子提交 provider Conversation 而不创建用户 Transcript Item；普通外部 start/resume input 仍同时进入两种投影。
- `sessions.snapshot` 在一个应用用例中校验并组装挂载 Session 的 HITL、Plan、Goal、Run、Tool material closure。
- Goal objective update 先 quiesce exact owned drive，再以 fresh incarnation CAS 保存；原 lifecycle 为 active 时才按冻结事实重启 drive。Goal clear 在 owned drive 静止后 CAS 删除，已不存在时幂等成功，外部 owner 或 complete objective 不被越权改写。
- fresh start、resume、child admission、waiting barrier、cancellation 和 terminalization 都有明确 command identity 与事务结算规则。
- staged executor 在 Segment opening 前由 `stagedExecutionHandoff` 唯一拥有；claimed Resume 由 `claimedResumeAttempt` 直接携带 claim 返回的 immutable `Pending` 提交匹配 `resuming` 的 terminal write-set，durable `RunLost` 成功后才释放 exact continuation executor，不再重读 open projection 或依赖每个错误分支手写补偿。

### 2.3 Adapter / Infra / Delivery

- `adapter/agentexec` 独占 Agent Framework 类型映射、TreeSnapshot encode/decode/restore、interaction input 和 observation 翻译。
- Toolset 只暴露 framework-neutral manifest/invocation 合同；具体工具、MCP、LSP 和 executor lifetime 由各自 adapter owner 管理。
- Infra 独占 SQLite、filesystem、Git、HTTP sidecar 和进程机制；它实现 Application ports，不创造业务终态。
- SQLite 只在 `sessions.workspace_path` 保存 Session exact Workspace，strict codec 重新建立 Domain 不变量；旧 `cwd` 列、空默认值、兼容双读与 migration 均不存在。
- Delivery 独占 JSON-RPC/HTTP/SSE binding、strict validation、error mapping、version/capability negotiation 和生成合同；它不持有领域状态。
- Bootstrap 是唯一 composition root，required dependency 必须在 constructor/start boundary 被证明，不允许以 nil、安装顺序或静默降级表达依赖。
- Bootstrap 不发布 Stack/service locator，也不传播宽 assembly foundation。Policy、workspace、execution capsule 分别拥有共同构造不变量；Host 私有 application capsule 向 Delivery、startup recovery 和 workers 提供行为，Instance 只拥有完整 operation delivery 与 join handles。工具/执行 acquisition 仍在错误返回前转交唯一 `hostLifetime`，失败 Open 与重复 Close 保持逆序、可重试回滚。

## 3. 执行与交互能力

### 3.1 Root execution

- 支持 root Run fresh start、观察、取消、终态和资源释放；durable opening 成功后才激活 Agent Framework Process。
- Session 永久拥有 exact provider/model selection；显式 Run pair 在 executor staging 成功后随 opening 原子替换，省略 pair 使用 Session durable identity，provider 不从 model id 推断。
- model/tool observation 通过单一有序 stream 进入 Application；pre-call failure 禁止外呼，post-call receipt failure 形成 unknown 并由事务证明收敛。
- checkpoint 由 Application 持有 envelope identity，payload 对其不透明；`adapter/agentexec` 独占 Agent Framework TreeSnapshot v4。

### 3.2 HITL、resume 与 steer

- Question、approval、answer、checkpoint、Interrupt claim 和 continuation opening 形成一个可验证链路。
- open Interrupt 只能被 exact Session/Run/Item/occurrence 回答；edited approval 继续绑定原始 ToolCall identity，而非按 name/arguments 猜测。
- answer claim、Question answer/approval decision、checkpoint 删除和 commit receipt 原子提交。
- pending Question 直接从 mounted transcript material 选择最新 unanswered exact Run/Item，但暂时替换普通 composer，只呈现一个 24px 中性 request surface；问题 prompt 是标题，单选首项预选、description 内联，多题分页，单选自动前进，text/multi 使用 Next。Frontend 不复制 Interrupt query/read model，settlement 前不把 local draft 冒充权威 answer。
- Skip 仍提交与 fields 等长、同序的 `string[][]`，其中当前或全部 field 的空 inner values 表示明确跳过；Runtime Application 和 durable transcript validation 接受该语义，outer field 缺失、数量/顺序/Run/Item identity 错误仍拒绝。Question text/custom input 与 Composer 共用 composition key intent，不靠 timer、UA 或第二 draft 处理中文输入法的英文 commit Enter。
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

- 当前 schema epoch 82；一个 build 只接受一个精确 epoch，没有 migration chain、dual read 或 compatibility column。
- `sessions.workspace_path` 非空；读回时必须重新构造 exact `Workspace`，相对路径或 lexical-unclean row 即使绕过正常 writer 也 fail closed。
- `agent_memory_items.embedding_space` 与 vector 必须成对为空或成对有效；reader 只恢复有限、4-byte aligned 的 vector。Search 只消费当前 space 且维度匹配的 cache，写入以 item id + content digest + active status 为条件；prior-epoch 裸 vector 不读取。
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

- Desktop 的 consumer-owned Session summary 只投影一个 `workspace: { path, availability }` 对象；Work Index、Shell、创建继承、recipe/project revision 与 workspace event 都从该对象派生，不再维护 `cwd` / `cwdMissing` 平行状态。
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
- Work Index 默认展开为 275px，pointer/keyboard resize 夹在 240–520px，且 live shell 始终给 reading plane 留至少 240px；`shellGeometry.ts` 是生产 shell、持久 preference、ARIA range 与 visual fixture 的唯一数值 owner。Context Dock 继续使用独立的 640px conversation floor，不借用 Work Index clamp。
- Goal lifecycle 与当前 Run command 在同一中栏具有明确作用域：Goal 使用与 composer 同宽、重叠 1px 接缝的 attached top tray，整段 objective 是编辑入口，右侧动作固定为 clear、pause/resume、edit；420px compact editor 使用 20px inset、36px identity block 与 12 行输入区。所有动作继续经过同一个 per-Session Goal command owner，预算、额度、花费、步数、model、last move 与限制条件不成为永久 chrome；composer 的 Run stop 保持独立简洁。
- modal scope 由全局 `--color-scrim` 唯一拥有，light/dark 都精确为 `#00000022`；Goal editor、Markdown table preview 与其他 dialog 只消费该 token，自身 surface 拥有 border/material/shadow。不得用 scheme 分叉或 per-dialog backdrop 重复表达深度。
- 新 Goal 的创建入口是同一 Composer 的 submit mode：`/goal` 只武装 mode 并移除命令文本，active footer 只显示紧凑 Goal identity；Composer 继续唯一拥有 draft、附件、IME、历史、Enter/send 与成功清空。Goal contribution 只拥有 replacement confirmation 和 `goals.start` transaction，不发布第二 objective field、launcher bus 或 limits form；standing projection 早于 mutation response 时，`starting` phase 保留 exact commit ownership，直到命令 owner 接受或拒绝该 draft。
- 无 active Session 时不挂载 Context Dock destination、view 或 toggle；Runtime 默认 workspace 只可用于显式创建 Session，不能冒充 Session-owned material。
- 顶层 New Session 继承点击时 active Session 的 exact cwd；active summary 尚在 resolving 时禁用该动作，不回落到 Runtime 默认目录。目录选择由 Projects 标题栏唯一拥有，project row 的 `+` 继续表示在该 exact cwd 建立 Session。
- Runtime command capability 同时约束顶层 New、Projects 目录选择、project-row `+` 与 palette/快捷键 New；断线时这些真实 `sessions.create` 入口同步撤权，不以失败 toast 充当 availability。native picker 返回后在投递前重新证明当前 capability。顶层 New 作为 active-project blank destination 可复用当前空 draft，显式 Projects 创建仍始终分配新 Session。
- project-row `+` 只在 `sessions.create` 返回新 Session identity 后把焦点交给 composer；command owner 拒绝创建时保留当前 Session 的焦点与可见上下文，不把旧 composer 冒充新建结果，也不写第二导航状态。
- projectless welcome state 的“选择项目”位于 composer overlay rear tray：surface 相对 composer 双侧缩进 12px、后层起点相差 37px并重叠 22px，只显示 folder 与可访问 label。它直接消费 Work Index groups，现有项目调用 `startSessionInFolder`，新增目录调用 Wails native `chooseSessionFolder`；active Session 建立后 tray 退出，不在 footer、标题或本地 state 保留第二份项目身份。
- Session title maintenance 只有 `runsegment.Finalizer` 一个 owner，并只经 Session Application first-writer 持久提交；utility model 缺失、空回复或 provider error 时，opening user text 的首个有效行提供 Unicode-safe、有界 deterministic fallback。provider error 仍进入既有 maintenance telemetry，不以“未命名会话”或 Frontend 第二 writer 吞掉降级事实。
- Goal、Plan、HITL/审批只呈现当前 projection generation；accepted mutation intent 可在 authoritative projection 追平前保持稳定 busy 反馈，不写第二 cache。
- Plan 只在 active Run 期间以环形进度和“第 N / M 步”pill 呈现；完整 Session plan 由同一 projection 在 hover/focus tooltip 展开，不复制成 disclosure card、progress bar 或第二 expanded state。Plan 位于 composer overlay，Goal 使用 overlay 中的 attached top tray；空 contribution 不留下固定边线或高度。
- Composer Context 环把当前 root Run 的 live `segment.progress.contextTokens` 或 durable `RunRef.contextTokens` 与 active Session exact provider/model 对应的 `contextWindow` 配对，按 `min(used, window) / window` 投影占比；只按 model id 命中同名 provider 候选是非法推断。Run progress writer、SQLite durable shape、Artifact v23、Protocol/generated surface 与 mounted Session projection 共同持久化最后一次正值 footprint。Session view 从 root history 选择最新正值，尚无模型响应的 successor 不会抹掉已有证据；终态、renderer reload、Runtime restart 与 Artifact import 均恢复同一读数。缺任一权威事实时不绘制，Session 累计 usage 或客户端估算不参与该读数。触发器是保持原 glyph 光学中心的 28px 可聚焦按钮，hover 与 keyboard focus 共用同一 tooltip。
- 设置主色的唯一状态 owner 是 appearance preference；preset/custom 控件、`aria-pressed`、document CSS paint 与 localStorage 恢复都消费同一次 mutation。custom theme plugin 对单键 `COLOR_THEME` contribution 持有 exact disposable，更新时先退休旧 contribution 再发布新值，cleanup/HMR 同步退订 listener 并释放 contribution；不得以重复注册、异常吞噬、刷新或第二 theme cache 代替 replacement lifecycle。
- 标题栏不呈现 Session 累计 token/cost；普通 completed/failed Run 不在最终回答后重复绘制耗时、步数、token、费用或“完成”结算条。canceled/limit 的 quiet reason 与 actionable failure recovery 仍按各自既有 owner 呈现。
- Composer 的 composition lifecycle 是 Enter 提交判定的唯一 owner：`compositionend` 后保留一次性 commit intent，首个无修饰 `Enter/keyCode=13/isComposing=false` 只结束中文输入法的中英混合文本提交，随后明确 Enter 才发送。active composition、浏览器缺失 `compositionend` 的 plain-input recovery、focus/pointer/paste/drop retirement 与 Mod/Shift shortcut 均消费同一 intent，不使用 timeout、timestamp/UA 猜测、第二 draft 或第二发送入口。
- 流式消息的底部点赞/点踩与操作行只在所属可见 turn 达到稳定展示边界时出现，不随每个输出 delta 反复挂载。
- 用户消息 bubble 使用独立 semantic marker 和 theme-owned 5% text neutral surface；12×8px inset、16px 圆角与 77% 行宽共同保持 Codex 的低强调层级，不复用 accent selection/command feedback token。
- 中央 transcript 的 mount geometry、异步 Markdown/Shiki materialization、实测 input-rung clearance 与用户滚动共享 `MessageStream` 一个 presentation owner：exact target 来自既有 scroll library，但只有 raw `state.isAtBottom` follow lock 能授权 viewport 写入；public `isAtBottom` 的 near-bottom convenience value 只用于展示，不能决定 motion。DOM mutation 与 border-box ResizeObserver 覆盖内容和动态 padding 两种高度来源，compact Approval 首次打开把 blocking action 留在 composer 上方，Question replacement 则完整占据同一输入层；停在尾部时持续跟随，wheel-up 同步释放 raw lock 后，即使仍在 near-bottom band，后续 token/Markdown/Shiki/clearance 增长也保留 reader-owned `scrollTop`，主动回到底部或点击回到最新才恢复跟随。不存在固定 RAF/timer、第二 scroll state 或阈值猜测。
- 右栏 Diff 与 File preview 共用 file-path → Shiki grammar 映射；preview 以 query 对应的 exact path 选语言并进行一次 whole-file highlight，未知 grammar 降为 text，不从内容猜测或复制 extension table。目标行定位只响应 path/content/line navigation，Shiki 异步 materialization 不重放滚动或覆盖 reader-owned 位置。
- File preview 的 generic `File` tab 与 material identity 分工明确：dock view bar 从同一 `viewer.path` 呈现左侧截断的 exact path，并与 line count/truncation 同行；full placement 继续以 path 为 title。文件选择、query 参数、syntax grammar 与可见身份不再分叉。
- Timeline 在 Runtime 不可接收命令时继续呈现 exact Session/Run 审计事实，但 locate/cancel 共同由 Runtime command capability 禁用；恢复后沿同一 owner 重新可用，不从 connection phase 复制第二布尔状态。
- 右栏 tab strip 保留可读标签；active identity 变化时由 tab owner 执行 nearest scrolling，strip 自己的 scroll geometry 驱动 start/end edge fade。renderer 恢复或 picker 打开末尾 tab 后选中身份始终可见，两侧隐藏内容也有明确提示。
- 中栏 Markdown presentation 由一个 renderer owner 统一持有：semantic rich/plain table copy 与展开预览、Shiki code wrap/copy、HTML/SVG fence 隔离、Mermaid、selection copy、matched citation、image-only grouping、message-scoped image gallery、inline/fenced LTR isolation、Han/RTL direction、Codex heading/paragraph/list/task/blockquote/rule rhythm 均消费同一 message material。表格正文固定 14/21px、表头 14/16px、容器零额外块间距；task checkbox 是 list grid 的直接子项，正文统一位于第二列；行内代码使用可换行的 cloned neutral well、0.92em 字号、6px 圆角与 `overflow-wrap:anywhere`。图库在 nearest exact message-content root 内提供全屏 90% 黑色画布、显式关闭、前后/方向键导航、Escape、100–400% 缩放与切图重置，并排除 nested delegated message；代码块使用同材质 14px sans caption、4×8px header、8px source inset，且不设人工高度上限。模型 raw HTML 只允许无属性 basic inline 标签与 `br`；远程图片、style、native disclosure/layout 和属性注入不会成为活动 DOM。上述能力不持久化、不注册全局 owner。
- KaTeX 样式由既有 Markdown 动态 loader 唯一拥有；构建分块必须把 `katex.min.css` 与静态可达的 KaTeX JavaScript 分离，产物可以存在但不得由 `index.html` 提前引用。当前启动 CSS 为 112.8KB，动态 math CSS 为 29.0KB，bundle gate 同时约束启动预算和 lazy ownership。
- context compaction 使用左对齐 quiet activity row 而非居中 divider；压缩图标保持可见，Runtime 提供 summary 时沿同一可键盘操作的 disclosure 展开，不增加第二 transcript/material owner。
- Agent message 的 authoritative phase 已进入公共能力：terminal AgentMessage 必带 `commentary | finalAnswer`，running shell 不带 phase；Runtime completion/termination boundary、Domain Transcript、SQLite durable shape、Artifact v23、Protocol/generated Go/TypeScript surface 与 Frontend published SDK 共同持有该事实。Frontend 不从 streaming、位置或文案推断分组，live/replay/mixed hydration 都把 final answer 归入稳定 `final:<itemId>` row。
- 图片预览 Download 是 Desktop 应用能力而非 Runtime Protocol：Frontend gallery 把当前 restricted inline image 交给 `DesktopHost.SaveImage`，Host 在打开原生 save sheet 前校验 MIME 与解码内容，Wails adapter 绑定 exact window 并只在用户选定目的地后写入；取消返回 `false`，remote URL 与 browser download 没有 fallback。

## 7. 公共合同

- Runtime Protocol 当前版本 `2026-08-24`，唯一 replay scope 为 `runtimeInstanceRootSegment`。
- Artifact 当前版本 23；旧版本在写入前确定性拒绝，不猜测缺失事实。
- SQLite 当前 epoch 82；shape 变化必须一次前移 owner codec、fresh schema tests、baseline 与生成物。
- Agent Framework 当前 Baseline 20；Runtime 不依赖 private state 或迁移前 module path。
- 所有生成合同必须 diff-free；consumer 缺口记录在 [`CONSUMER_HANDOFF.md`](CONSUMER_HANDOFF.md)，服务端不为消费者恢复旧字段。

## 8. 结构清理结论

- required dependency 已从运行期偶然性提升为构造/启动期合同。
- shared map、cancel set、replacement、join 和 retirement 必须由单一并发对象拥有。
- 文件/package 按 change reason 和边界拆分，不按行数拆分；只有一个调用者、没有独立不变量/生命周期/替换价值的微模块应吸回 owner。
- 静态 extension registration 直接属于 plugin composition entry；只投影 `{id, order, component}` 等对象字面量的 application factory 与对应 literal-only test 不构成独立边界。保留的 contribution module 必须拥有策略、行为、映射不变量或跨 context published language。
- 通用 helper 只在确有多个独立消费者且语义稳定时存在；不为测试便利暴露 singleton accessor 或 raw owner getter。
- Architecture guard 只守 import/owner/public-or-persistent shape/lifecycle 等长期语义；checkpoint binding、Pending ownership 与 parked continuation closure 由其 Domain/Application/SQLite 行为 owner 验证，不把调用文件、局部变量或表达式拼写冻结成合同。
- 空目录、迁移 alias、compat adapter、双状态 codec、刷新旁路和 shadow owner 不属于当前架构。

## 9. 验收证据

| 维度               | 当前守卫                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Domain/Application | aggregate/use-case 单元测试、事务失败与状态迁移反例                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| Agent Framework    | public baseline、architecture import gate、snapshot/interaction/child recovery tests                                                                                                                                                                                                                                                                                                                                                                                                                               |
| SQLite             | fresh schema、codec、CAS/uniqueness、cross-table invariant、真实 reader/writer 与 SIGKILL recovery                                                                                                                                                                                                                                                                                                                                                                                                                 |
| Protocol           | strict validation、golden samples、manifest/OpenRPC/schema/Go API digest 与 generator diff                                                                                                                                                                                                                                                                                                                                                                                                                         |
| Desktop state      | exact-generation replacement、late settlement、Session material、query writer 和 navigation tests                                                                                                                                                                                                                                                                                                                                                                                                                  |
| Frontend           | type/lint/format/knip/layer/API/style/design/token/locales/bundle 全门禁与 `--detectAsyncLeaks`；production visual composition 必须提供真实 setup service、Runtime service status 和 command/interrupt lifecycle owner，当前 agent/shell/workspace/closure/foundation/WebKit 矩阵 321 tests 覆盖 streaming、HITL、Session/Dock、projectless rear tray、设置主色、Goal editor/submit mode、commentary/final answer、WCAG、键盘、coarse pointer、长内容、IME、CJK、18px、reduced motion、Retina 与 light/dark golden |
| Production shell   | Wails v3 Go test/vet/build、production `.app` package、renderer reload、Runtime health/discovery 与 fresh database/SIGKILL smoke                                                                                                                                                                                                                                                                                                                                                                                   |

P127 Frontend 基线为 323 files / 2005 tests；Goal composer submit mode、零 duplicate fields、无 limits/usage/model、Runtime failure draft preservation 与 standing projection 早到时的 exact commit ownership 红例全绿。完整 visual/WCAG/keyboard/coarse-pointer/IME/CJK/Retina/WebKit 矩阵 319 tests，以及 typecheck/lint/format/knip/circular/context/published-boundary/layer/port/API/style/design/token/chrome/locales/bootstrap/bundle 全门禁保持全绿；89/89 Runtime operation fact families、3/3 sidecars、16/16 events 均有产品消费者。fresh `LYRA_HOME` 的 production page smoke 已验证 Goal create、pause、clear、成功提交后 composer 清空与 `spinbutton=0`；Wails v3 dev host 已完成 frontend/Go build、开发 `.app` strict codesign、frontend connection 与 1439×899 原生窗口启动。

P128 Frontend 基线为 323 files / 2009 tests；Runtime phase contract、live/replay/mixed fold、commentary 无 actions、final answer 独立 actions 与 same-Run wave folding 红例全绿。完整 visual/WCAG/keyboard/coarse-pointer/IME/CJK/Retina/WebKit 矩阵 320 tests，以及 typecheck/lint/format/knip/circular/context/published-boundary/layer/port/API/style/design/token/chrome/locales/bootstrap/bundle 全门禁保持全绿；Runtime/Desktop test/vet/build 和 generator diff 全绿。fresh then-current SQLite schema 的真实 HITL/tool/final answer smoke 与 reload 后 DOM/Artifact phase 分层通过；Wails v3 dev host 完成 frontend/Go build、开发 `.app` strict codesign、frontend connection 与 1439×899 原生窗口启动。

P129 保持 Frontend 323 files / 2009 tests 基线；Goal model-only opening 红例、Runs 全 package 回归、Runtime/Desktop test/vet/build、generator diff、Frontend 全门禁与 Wails v3 production build 均通过。fresh SQLite 的 26 次自治 drive 中，provider Conversation 持久保留 26 条 Application 控制消息，Transcript 控制提示泄漏为 0，真实用户 Item 为 1；Runtime restart 后 paused Goal tray 与零泄漏同时恢复。

P130 把新会话的 working-directory 能力收敛到显式 exact-cwd：顶层 New、project row 与 welcome draft 都必须先完成项目/目录选择，取消不会创建隐式 home Session、切换旧 selection 或吞掉输入；选择完成后仍由唯一 Session create/navigation owner 提交。红例 `efd696dd7` 与根修复 `93ebd28a5` 已推送，8 locale 与相关 Frontend 门禁全绿。

P131 完成 Codex 内容区像素收口：settled Question 默认折叠为一行 `Asked N questions` activity disclosure，问答只在展开后显示；active Plan 使用固定 32px composer-owned 槽、8px tooltip inset/row gap、16px mark 与无删除线 completed ink；Goal tray 使用 4px 垂直 inset且继续只显示 lifecycle/objective/actions；正文统一为 `font-size + 8px`。streaming reasoning 因真实 overflow 获得键盘滚动入口。Frontend 324 files / 2016 tests、全静态/构建门禁与 320 项 visual/WCAG/keyboard/coarse-pointer/IME/CJK/18px/Retina/WebKit 矩阵全绿，Plan tooltip 与 Question collapsed/expanded golden 已纳入守卫。production page 的空态、显式项目菜单、exact-cwd draft 与标题项目入口已通过；Wails v3 dev host 完成 frontend/Go build、开发 `.app` codesign、frontend connection 与 1411×881 onscreen 原生窗口启动。未改变后端合同或 `app/cli`。

P132 完成图片查看器原生保存纵切：Codex 的 Download/Close 工具组顺序、40px target、busy/disabled 与失败反馈进入 production lightbox；当前 gallery item 只经窄 adapter 交给新增的 `DesktopHost.SaveImage`。Host 只接受受限 inline image data URL，校验、解码并生成建议文件名，Wails v3 adapter 将 save sheet 附着到 exact window，选择完成后才写文件；取消不报错，远程 URL 与浏览器下载无 fallback。红例 `2bbc84cbe`、根修复 `b4f69c6a0` 已推送；Frontend 324 files / 2019 tests、全静态/构建门禁、320 项 visual 矩阵、Desktop test/vet/build 与 `wails3 build` 全绿，production binary 已启动 smoke。Runtime/Protocol/Artifact/SQLite 与 `app/cli` 未改变。

P133 完成 Codex modal scope 层级收口：当前 production 红例证明 Goal editor 的 light 22% / dark 50% scrim 比 Codex 统一 `#00000022` 更重；全局 `--color-scrim` 现已成为 scheme-independent 唯一 owner，dialog surface 自己承担边框、材质与阴影深度。Goal editor 几何、actions、快捷键和 Goal/Plan/Context lifecycle 均未改写，也没有为历史错误截图增加兼容 UI。红例 `f10d63d33`、根修复 `670510280`、golden 收口 `ab1688654` 已推送；light/dark computed style 精确为 `rgba(0, 0, 0, 0.133)`，Goal editor 与 Markdown table preview 四张 golden 已人工核对。Frontend 324 files / 2019 tests、全静态/构建门禁、320 项 visual 矩阵、Runtime/Desktop test/vet/build 与 `wails3 build` 全绿；fresh then-current SQLite schema 的 production Wails↔Runtime RPC smoke 通过。Runtime/Protocol/Artifact/SQLite shape、Frontend published SDK 与 `app/cli` 未改变。

P134 完成 Codex Work Index 几何与 composer tail 收口：本地 Codex Desktop owner 明确给出 275px default、240px floor、520px ceiling 与 240px live reading remainder；Lyra 原先 256px default、CSS fallback、fixture literal 和 1440px 下可达 800px 的 End 已删除。`shellGeometry.ts` 现为首帧、偏好、pointer/keyboard、ARIA 与 visual fixture 的唯一 owner，Dock 保持独立 640px conversation floor。三栏几何传播又暴露 fractional overlay/整数 `scrollTop` 可让 1rem composer tail gap 少 1px，唯一 clearance owner 现增加 1px rounding guard。红例 `bbe20ecac`、根修复 `66f5e6231`、live clamp 测试 `6e7498324`、Work Index golden `8e6831be9`、净空修复 `41bc002de` 与尾部 golden `67b9707c0` 已推送；82 张三栏传播 golden、10 张尾部 golden、long-content 五次定向重复、Frontend 324 files / 2019 tests、320 项 visual 矩阵、Runtime/Desktop test/vet/build 与 `wails3 build` 全绿；fresh then-current SQLite schema 的 production Wails binary 已完成单 listener、同源 health/info `instanceId` 与真实 Frontend OPTIONS/POST `/v2/rpc` smoke。Runtime/Protocol/Artifact/SQLite shape、Frontend published SDK 与 `app/cli` 未改变。

P135 完成 Codex 待审批命令单一叙事表面收口：fresh production 会话证明同一 `itemId` 的待决 shell ToolCall 与 approval 同时出现时，透明工具行和 request surface 会重复展示命令；Codex 本地实现只让 approval surface 拥有请求层级。Frontend presentation planner 现在仅在双方均为 `requires-action` 时省略 exact 工具影子，Runtime/Fold 继续保留两个事实用于重连、重放与审计，结算后工具历史自动恢复。红例 `11fd71095`、根修复 `9fc1fc4c7`、visual 合同 `53ee0b6f9` 与格式收口 `7fd14b0ba` 已推送；真实修复后 DOM 为 1 份命令、2 个审批动作、0 条工具影子。Frontend 324 files / 2020 tests、320 项 visual 矩阵、Runtime/Desktop test/vet/build、`wails3 task build`、strict codesign 与 fresh then-current schema production Wails↔Runtime RPC smoke 全绿。Runtime/Protocol/Artifact/SQLite shape、Frontend published SDK 与 `app/cli` 未改变。

P136 完成 Codex 空态项目 rear tray 与 exact-cwd 接线收口：原 production picker 属于 `composer.toolbar.start`，只是在 footer 放了一个 utility；现在 projectless ChatStream 也安装 production overlay top，`ComposerProjectTray` 以 12px 双侧缩进、37px 后层起点和 22px 重叠附着在前景 composer 背后。托盘没有 chevron 或第二项目状态，只有 folder 与通过 light/dark WCAG 的 muted label；菜单仍直接消费 Work Index，现有项目与 Wails native 新项目最终由同一 Session owner 提交 exact cwd，active Session 建立后托盘退出。红例 `f46ff79b8`、根修复 `1c7c4a7f8`、WCAG 收口 `fadab9baa` 与格式收口 `54a5767b4` 已推送；Frontend 324 files / 2021 tests、321 项 visual 矩阵、Runtime/Desktop test/vet/build、Wails v3 production `.app` package、strict codesign 与 fresh exact-cwd Wails↔Runtime RPC smoke 全绿。Runtime/Protocol/Artifact/SQLite shape、Frontend published SDK 与 `app/cli` 未改变。

P137 完成 Codex 窄窗 Context Dock 可用性时序收口：fresh 1180px production 审计证明 Session loading placeholder 先让 `ChatPanel` 的几何 effect 在无 row 时退出，loading 结束又未重绑，导致可见 Dock 入口先接受点击、再因 `dockOpen` 触发迟到 reconcile 而闪退。原 render condition 现命名为唯一 `shellVisible` 派生事实，渲染与几何 observer 共同消费；实际 row 仍唯一决定 `dockAvailable`，navigation owner 继续负责 URL fold 和 Session-owned tab/last-view/width 恢复。红例 `8f2f4ff8d` 与根修复 `96e768cd1` 已推送；Frontend 324 files / 2022 tests、321 项 visual 矩阵、Runtime/Desktop test/vet/build、Wails v3 production `.app` package、strict codesign 与 fresh HOME/local-token/exact-cwd Wails↔Runtime RPC smoke 全绿。Runtime/Protocol/Artifact/SQLite shape、Frontend published SDK 与 `app/cli` 未改变。

P138 完成 durable Context footprint 与 Codex 流式滚动逃逸收口：`contextTokens` 从 live-only progress 提升为 Domain Run 事实，then-current SQLite shape、Artifact v21、Protocol/generated surface、Session snapshot 与 Frontend selector 同步携带；fresh 真实 Run 得到 `4152` tokens，终态、整页 reload 与 Runtime restart 后仍恢复同一 tooltip，累计 usage 不参与。Context trigger 使用可聚焦 28px button，同时保持原 16px ring 光学中心。流式滚动红例证明 public near-bottom convenience value 会在 wheel-up 后误授权额外 observer 抢回视口；`MessageStream` 现只读 raw follow lock。真实页面从旧底部 `936px` 向上离开至 `896px`，内容把新底部推进至 `1136px` 后仍保持 `896px`。durable Context 红例 `4e4776499`、根修复 `a01c10e9a`、光学收口 `e91eccebd`，以及滚动红例 `d655e3408`、根修复 `b5378fa94` 已推送。Frontend 324 files / 2025 tests、98 条 published context edge、89/89 operations、3/3 sidecars、16/16 events、全部静态/构建门禁与 321 项 visual 矩阵全绿；Runtime/Desktop test/vet/build、generator diff、Wails v3 production `.app` package 与 strict codesign 全绿。Protocol 日期保持 `2026-08-17`，`app/cli` 未修改或暂存。

P139 完成 Runtime/Desktop 证据化熵回收：Desktop 的三份 Runtime 协议镜像与三份设计导出都没有真实消费者或动态入口，现已删除，所有引用回到 Runtime canonical docs；客户端审批危险度推断和 risk/scope/reversibility 公共副本被移除，Runtime wire risk 仍保留为协议事实，但 Desktop 不再把它复制到 Agent event/content block 或据此建立第二套 presentation。被 scoped lifecycle owner 取代的串行队列、unscoped settlement 包装、无消费者 locale/facade/barrel export，以及 Runtime 仅供 dispatch 测试使用的 request/id 构造 helper 同步删除。Frontend published approval facade 的缩减是显式 breaking cleanup；没有 deprecated alias、兼容双读或 fallback。generated wire、compatibility baseline 与动态 sideload plugin SDK 均因真实生成/发布入口保留。Frontend 323 files / 2017 tests、完整静态/构建门禁、98 条 published context edge、89/89 operations、3/3 sidecars、16/16 events 全绿；Runtime/Desktop test/vet/build 与受影响 transport/dispatch race tests 全绿。Protocol、Artifact、SQLite 与 Runtime 公共 Go API 未改变。

P140 已完成第二轮 Runtime/Desktop 根因级熵回收，明确不保留兼容。前三批分别删除上一发行版兼容基线与 protocol range、生产不可达的 `custom`/`clientTools`/`toolResult` wire，以及没有真实安装入口且样例已失效的 Desktop 外部 sideload 整条链路与随附空扩展点。第四批证明 Feature、Method、StateKey 的 stability 元数据在全部生产注册中恒为 `stable`，没有协商、路由、降级或 UI 决策读取它；canonical Go 类型与校验、operation/state 注册、合同生成器、OpenRPC extension、manifest/schema/Go API、TypeScript binding、Desktop samples/tests/fixture 已同步删除该字段。第五批把唯一 state 变体 Plan 收敛为一等资源，删除 registry/key/scope/writer、通用 unions、RuntimeEvent key、Artifact `states[]`、Desktop generic shared-state reader 与 Application 重复 Step DTO；Artifact 前移到 v22。唯一精确 Protocol 版本为 `2026-08-21`，旧 wire/archive shape 没有 alias、双读、fallback 或迁移层；SQLite shape 未改变。Frontend 318 files / 1993 tests、全部静态/边界/消费者/bundle 门禁、Runtime/Desktop 全量 test/vet/build/staticcheck、受影响 Runtime 与 Desktop 全量 race tests 均通过；生成器重跑 diff-free，Runtime/Desktop TypeScript 合同与样例逐字节一致。

P141 完成第三轮 Runtime/Desktop 根因级熵回收。Desktop 删除 Composer draft 单实现 port/use-case/adapter 转发链与重复输入 DTO，把行为归还唯一 state adapter；测试 reset/discard seam 改由真实 extension/interrupt owner 的 disposer 结算；十二份 gateway installation 接口改由安装函数推断；无消费者 capability subscription、RPC request discriminator 与 test-only Run selector 连同自证测试被删除。Runtime 全生产树的 `deadcode` 与 consumer 复核没有发现可安全删除的内部实现；`embedded` 公共 API、operation 泛型入口与 testsupport 因发布、动态 dispatch 或测试基础设施义务保留。七个实现批次共净减 212 行；Frontend 318 files / 1990 tests、全部静态/边界/消费者/bundle 门禁以及 Runtime/Desktop test/vet/build/staticcheck、生成合同 diff 全绿。Protocol `2026-08-24`、Artifact v23、then-current SQLite shape、Wails v3 binding 与用户能力均未改变。

P142 完成第四轮 Runtime/Desktop 根因级熵回收。Desktop 依次删除命令面板遗留的 `when` 解析器、失去 custom wire/外部插件消费者的状态 patch DSL、生产动态插件安装/移除及其 handle/revision seam、从未被 Runtime fold 发出的 preview content block/citation/renderer/copy-code 纵切，以及命令目录零读取展示字段与 selector；动态插件变更只保留为明确 test harness，生命周期测试直接使用 `Host` stop。Runtime 全生产树 `deadcode` 与 consumer 复核仍只命中有发布、动态 dispatch 或测试基础设施义务的 `embedded`、operation 泛型入口和 testsupport。公开 `sessions.export/import` 与 conversation archive 因唯一非测试 operation consumer 义务保留，没有以删除真实消费者或放宽守卫制造虚假减法。五个实现批次涉及 67 个文件、154 行新增、1438 行删除，净减 1284 行；Frontend 315 files / 1969 tests、97 条 published context edge、89/89 operations、3/3 sidecars、16/16 events 及全部静态/边界/bundle 门禁全绿，Runtime/Desktop test/vet/build/staticcheck、生成合同 diff、Runtime standalone 与 Desktop `GOWORK=off` 验证均通过。Protocol `2026-08-24`、Artifact v23、then-current SQLite shape、Wails v3 binding、用户能力与 `app/cli` 均未改变。

P143 完成 Session exact provider/model 身份纵切。反例同时覆盖两个 provider 发布同名 model 与省略 Run selection 误用全局默认；Session Domain 现将 configured `modelref.Selection` 作为构造、恢复、编辑和 fork 的必备不变量，Runtime 默认只在 Session admission 安装。显式 Run pair 在 staging 后随 opening 原子替换 Session pair，省略 pair 只读 durable Session。then-current SQLite shape、Artifact v23、Protocol `2026-08-24`、公共 Go/Schema/TypeScript 生成合同与 Desktop consumer-owned read model 一次性切换；旧 database/artifact/wire 无 alias、双写、双读、fallback 或 migration。Desktop Composer 与 Context gauge 按 exact pair 解析同名目录项，React 派生不复制第二 state。该批只修改 `app/runtime` / `app/desktop`，`app/cli` 保持零 diff；app2 的 exact identity/vertical slice/consumer adapter 经验被采纳，opaque JSON、额外 public package、god facade、能力删减与低覆盖被拒绝。Frontend 315 files / 1972 tests、97 条 public context edge、89/89 operations、3/3 sidecars、16/16 events、全部静态与 bundle 门禁全绿；Runtime/Desktop test/vet/build/staticcheck、受影响 Runtime race、standalone tests、生成合同与 Wails production build 通过。

P144 完成 Session exact Workspace 身份纵切。失败优先反例证明旧 Domain 可直接接纳 `relative/work` 与 `/work/../work`，SQLite 也以允许空默认值的 `sessions.cwd` 保存同一必填事实。Session Domain 现由 immutable `Workspace` 唯一表达必填、绝对、lexical-clean 路径；filesystem adapter 继续独占存在性与物理 canonicalization，未照搬 app2 的 filesystem-root 禁令。Draft/Patch/Snapshot/restore/fork、Application admission/read model、SQLite strict codec 与 Desktop consumer adapter 一次性切换；then-current SQLite shape 只保存非空 `workspace_path`，旧 `cwd` 列、alias、双写、双读与 migration 均已删除。Desktop summary 只暴露 `{path, availability}` workspace 投影，React 消费端不复制第二状态；执行/sandbox 命令自身的 `cwd` 仍是不同技术事实。Protocol `2026-08-24` 与 Artifact v23 的 `WorkspaceRef` 机器 shape 未变，因此没有虚增版本。Frontend 316 files / 1973 tests、97 条 public context edge、89/89 operations、3/3 sidecars、16/16 events 与全部静态/bundle 门禁通过；Runtime/Desktop test/vet/build、Go 1.26.5 staticcheck、受影响 Runtime race、standalone/GOWORK-off、生成合同与 Wails production native build 通过。该批只修改 `app/runtime` / `app/desktop`，`app/cli` 保持零 diff。

P145 完成 architecture guard 熵回收。当前代码的可逆反例证明，仅将恢复函数局部变量 `sess` 等价改名为 `currentSession`，Application checkpoint ownership 行为测试仍通过，旧 architecture test 却因缺少 `sess.Workspace` / `sess.Isolated` 字符串失败；P144 也曾为同类 owner 改名被迫维护该 marker。三条逐文件 `strings.Contains` 守卫共 217 行已删除，checkpoint binding、Pending mutation ownership 与 parked continuation closure 回归 `ExecutorCheckpoint.ValidateFor`、`Pending.ValidateProjection`、Application write-set 和 SQLite owner predicate 的真实行为矩阵。新增 boot recovery 测试证明 root member、Session、working directory、Workspace、isolation、Goal incarnation、provider、model、limits 与 capabilities 任一漂移都在 executor probe 前 fail closed；tree barrier、waiting subtree、material snapshot、interrupt/checkpoint SQLite 既有反例继续全绿。Runtime 全量 test/vet/build/staticcheck、受影响五包 race 与 `GOWORK=off` standalone 全绿；Protocol、Artifact、SQLite、公共 Go API、Desktop 与 `app/cli` 均未改变。

P146 完成 operation 带外调用元数据适用性闭环。public HTTP 红例证明 query 上的 `Idempotency-Key` 曾被 operation 静默忽略，而 embedded surface 根本不允许表达相同元数据。Registry 现在以 `ReplayCursorPolicy` 与既有 operation/idempotency facts 唯一发布承诺；operation 在 capability/handler admission 前拒绝非 replay 方法的 key、无 key 的 namespace、无 cursor 能力的方法携带 `AfterEventID`，以及无 idempotency key 的 run-command cursor。仅 `runs.start`、`runs.resume`、`runs.subscribe` 接受 run cursor；manifest、OpenRPC、生成 TypeScript、embedded guard 与 Desktop preflight 都消费该事实，不维护方法名旁路。Frontend 316 files / 1974 tests 与全部静态/bundle 门禁、Runtime/Desktop test/vet/build/staticcheck/standalone、受影响 Runtime race 和合同基线全绿；Protocol、Artifact、SQLite、业务事务与 `app/cli` 均未改变。

P147 完成 Runtime/Desktop Go 1.27 工具链统一。两个 app module、Desktop standalone `go.work` 与 Runtime public binding 的外部消费者编译夹具一次性切换到 Go `1.27.0`；根 workspace 因真实解析失败同步提升 coordinator 版本，但 `app/cli` module 本身零修改。Go 1.27 `tidy` 只规范化 Runtime 依赖分组，未改变任何版本。稳定版 Staticcheck 2026.1 不认识 Go 1.27 export-data v4，因此验收使用 Go 1.27 构建的官方主分支固定提交 `v0.7.0-0.dev.0.20260821203000-f2e7b72a56da`；Runtime/Desktop 全量 test/vet/build/staticcheck/full-race/standalone、根 workspace tests、generator 零漂移与 Wails production build 全绿。Protocol、Artifact、SQLite、生成合同、公共 Go API 和运行时行为均未改变。

P148 完成 Bootstrap teardown settlement 闭环。生产语义红例证明，A2A/LSP/Shell/SQLite 的 one-shot closer 首次返回诊断后已经 terminal，旧 Host 却把 error 当作 unfinished，停止依赖关闭并在以后永久回放同一错误。P148 当时让 Infra step 显式区分 Terminal/Retryable 并同时返回 settled/diagnostic；Host 只保留真实 unfinished 前缀，terminal diagnostic 继续逆序释放依赖并最终关闭 graph。Config 只接收 one-shot `TerminalResource.Close`，SQLite Bundle 的伪 Shutdown adapter 与对应 test-only surface 已删除。P148 曾暂把 MCP session ledger 作为唯一 retryable tool resource；该分类已在 P149 按 SDK 的底层终止合同纠正，失去生产消费者的过渡双态再由 P150 删除。并发 copy、失败 Assembly rollback、超时 join 与后续 Close 均由行为测试覆盖；Runtime/Desktop test、vet、build、Staticcheck、full race、standalone，根 workspace tests、Runtime generator 零漂移与 Wails v3 production build 全绿。Protocol、Artifact、SQLite shape、公共 Go API、Desktop 与 `app/cli` 均未改变。

P149 完成 MCP teardown 终止合同纠偏。与 SDK 同形的 one-shot 红例证明：`ClientSession.Close` 已消费底层 transport closer 并返回诊断后，旧 Connections 仍保留 session，下一次 `Shutdown` 只能重放同一错误而不能释放新资源。session close attempt 现在一旦返回便退出 ownership ledger；异步 Detach 的 attempt/diagnostic 单独保留到 `Shutdown` 汇总，加入当代 generation 的 caller 收到诊断，后续调用为幂等 no-op。Bootstrap 将 MCP pool 注册为 terminal step，action 以 `context.WithoutCancel` 跑到真实终态，因此下层 host resources 不会被伪 retry 阻塞。P150 随后把 caller wait 提升到整张 terminal Sequence，并删除失去生产消费者的 Retryable 原语。Runtime/Desktop 全量 test/vet/build、Go 1.27-compatible Staticcheck、full race、standalone，根 workspace tests、Runtime generator 零漂移与 Wails v3 production build 全绿；临时检查器已回收。本批没有修改 Protocol、Artifact、SQLite、公共 Go API、Desktop source 或 `app/cli`。

P150 完成失败 Open terminal resource graph 所有权闭环。真实红例让 Assembly 在完成 tool acquisition 后构造失败，并让两次有界 rollback 都在同一个 terminal closer 上 timeout；closer 随即完成，但旧实现一秒内仍未关闭下层 host resource，因为 Step 只拥有动作、没有 owner 继续遍历逆序图。Bootstrap 现在把 host/tool steps 按 creation order 交给唯一 `teardown.Sequence`；Sequence 的一个 uncancelled reverse generation 独立于 caller wait 运行，timeout 后继续释放依赖，后续 Close 只 join immutable diagnostic。P149 已证明生产没有真正 retryable resource，因此 Retryable constructor、Step 自身的重复 Attempt/join/settlement 状态、失败前缀切片与对应 test-only tests 同批删除。修复阶段相对已提交的红例基线净减 239 行 Go；P150 连同红例相对 P149 净减 192 行。Runtime/Desktop 全量 test/vet/build、Go 1.27-compatible Staticcheck、full race、standalone，根 workspace tests、Runtime generator 零漂移与 Wails v3 production build 全绿；临时检查器已回收。本批不改变 Protocol、Artifact、SQLite、公共 Go API、Desktop source 或 `app/cli`。

P151 完成 Host component-to-resource shutdown graph 所有权闭环。失败先行反例让 Run component 的 join 超过 caller wait deadline；旧 Host 正确地没有提前关闭 tool resource，但 component 随后结束一秒后仍无 owner 继续图。这与 post-transfer startup recovery 失败的真实 Open 合同同形：函数不返回 Host，外层无法再次 Close。`hostLifetime` 现在持有唯一 active shutdown attempt；第一个 Close 以注入 Runtime lifetime values 启动 uncancelled generation，广播 cancellation 后加入 component/run-effect/executor，然后进入 P150 terminal Sequence。每个 caller 只有独立有界 wait，并发 Close 只 join；已完成的 unsettled component error 才允许下一次显式 generation，没有 timer/backoff/retry loop 或第二图。Runtime/Desktop 全量 test/vet/build、Go 1.27-compatible Staticcheck、full race、standalone，根 workspace tests、Runtime generator 零漂移与 Wails v3 production build 全绿；临时检查器已回收，未启动 agent-browser。本批不改变 Protocol、Artifact、SQLite、公共 Go API、Desktop source 或 `app/cli`。

P152 完成 Instance delivery-to-Host shutdown graph 所有权闭环。红例让一个已接受 operation 在 Instance Close caller deadline 后才返回；旧实现没有越过它提前关闭 Host resource，但 operation 结束一秒后仍无 owner 推进 Host。CLI defer 与公共 embedded Close 都只调用一次，不能把 retry 当成合同。Instance 现在持有唯一 active shutdown attempt；它从 Host 的 Runtime lifetime 派生 uncancelled generation，退休 delivery 后依次 join Endpoint、scheduler/database/recovery workers 与 Host attempt。每个 caller 只有独立有界 wait；并发 Close 只 join，明确 phase error 才允许后续显式 attempt。没有 timer/backoff/retry loop、第二 Host graph、重复 context owner 或新公共 surface。Runtime/Desktop 全量 test/vet/build、Go 1.27-compatible Staticcheck、full race、standalone，根 workspace tests、Runtime generator 零漂移与 Wails v3 production build 全绿；临时检查器已回收，未启动 agent-browser。本批不改变 Protocol、Artifact、SQLite、公共 Go API、Desktop source 或 `app/cli`。

P153 完成独立 Codebase 向量索引纵切删除与 source-discovery 失败语义收口。消费者审计证明 Agent 没有读取该索引，Desktop/CLI 只剩手动入口；Runtime 的 Domain/Application/SQLite/coordinator/operations/feature/topic/public bindings/生成合同，以及 Desktop/CLI 的完整 direct-consumer surface 已一起删除，没有 disabled registration、旧 binding、alias 或 fallback。Embedding role/vector codec 继续唯一服务 Agent Memory，缺少 embedder 时 keyword ranking 可用；代码发现继续由 grep/glob/read/shell/LSP 组合。SQLite shape 当批前移，Protocol 保持精确 `2026-08-24`。Git/workspace 反例同时证明 cancellation 与 exit-128 failure 不得退化成 filesystem fallback；现在仅明确 non-repository/unavailable 进入 fallback，unborn HEAD 保持合法语义。Frontend 313 files / 1950 tests、96 条 public context edge、86/86 operations、3/3 sidecars、15/15 events 与全部静态/bundle 门禁通过；Runtime/Desktop test/vet/build/Go 1.27-compatible Staticcheck/full-race/standalone、根 workspace tests、生成器零漂移和 Wails production build 全绿。CLI 受影响六包通过，其余 `runtimeembedded` 合同漂移独立处理；临时检查器已回收，未启动 agent-browser。

P154 完成 Agent Memory embedding cache 身份与所有权闭环。失败优先反例让两个同维度 embedding role 对同一语料给出相反坐标；旧裸 BLOB 被新 query vector 直接比较并返回错误记忆。第二反例证明按 item id 回填会在内容编辑后重新写入旧内容 vector。Searcher 现唯一拥有 query-time cache validation/refresh：只复用 exact space、同维度、有限 vector，其余内容批量重算并在本次请求中使用；SQLite 当批前移 storage shape 保存 `embedding_space`，并以 item id + content digest + active status 条件写，迟到结果失去写权。Curation/Run maintenance 的 embedding resolver、unembedded query 与 backfill methods 已删除；未配置或失败仍 keyword-only。Runtime/Desktop 的 test/vet/build/standalone、Runtime full race、Go 1.27-compatible Staticcheck、根 workspace tests、生成器零漂移、Frontend 313 files / 1950 tests 与完整静态/bundle 门禁、Wails v3 production build 全绿；临时检查器已回收，未启动 agent-browser。该批不改变 Protocol、Artifact、公共 Go API、Desktop source 或 CLI。

P155 完成 Agent Memory recall corpus 可达性闭环。失败优先反例证明 active user-scope memory 若未 pinned，虽然在 SQLite 与 Desktop 管理面可见，却因 per-turn recall 和 `search_memory` 固定查询 project scope 而没有任何 Agent consumer。Searcher 现只接受 project context；SQLite `SearchCorpus` 用一个 query 返回 exact-project 与 user-scope active items，二者共享一次 query embedding、一次 keyword/vector fusion 与一个全局 top-k。prompt/tool 不建立双查询、双 budget 或客户端 merge；pinned item 仍由 always-on prompt 注入并从 per-turn block 过滤。Runtime/Desktop 的 test/vet/build/standalone、Runtime full race、Go 1.27-compatible Staticcheck、根 workspace tests、生成器零漂移、Frontend 313 files / 1950 tests 与完整静态/bundle 门禁、Wails v3 production build 全绿；临时检查器已回收，未启动 agent-browser。该批不改变 Protocol、Artifact、SQLite epoch、公共 Go API、Desktop source 或 CLI。

P156 完成 Agent Memory durable content 与 prompt budget 闭环。失败优先反例证明 Domain/wire 可接受 4097 个 Unicode 字符，且 pinned prompt 首项可绕过 4096-token budget。Domain 现以同一个 4096 code-point 常量规范 item 与 ledger fact；构造、编辑、embedding identity、strict read、SQLite epoch 82 CHECK、generated Go/Schema/TypeScript Add/Update/Item validation 与 Desktop request/result guard 一次性投影。pinned core 与 per-turn recall 分别执行 whole-item 4096-token aggregate budget；不截断内容、不建立 consumer 长度 owner。Runtime/Desktop test/vet/build/standalone、Runtime full race、Go 1.27-compatible Staticcheck、根 workspace tests、generator/go mod 零漂移、Frontend 313 files / 1950 tests 与全部静态/bundle gates、Wails v3 production build 全绿；临时检查器已回收，未启动 agent-browser。Artifact、Agent Framework 与 CLI 不变。

普通 ToolCall 现在一律投影为透明 activity row，identity mark、summary、真实 accessory 与末尾按需 disclosure 构成单一阅读序列；展开体由 shell、patch 或 reasoning material 自己声明 reading-edge inset。denied、error 与非零 exit code 保留 exact verdict，但不再创建 warning badge、negative card、完成勾或常驻 action chrome。`card`/`flagged` 只保留给 delegated Run 等有独立层级和生命周期的复合产品边界。

Runtime standalone 与 Desktop 全量 test/vet/build、Wails v3 production package 和 strict codesign verification 通过。fresh HOME/SQLite smoke 中 renderer reload 后权威 `sessions.list` 与 SQLite 均保持唯一 Session；Runtime PID 89768→93411、`instanceId` 换代，Desktop PID 90579 保持，0600 durable token digest 不变。同一 renderer 在锁屏后台且没有 reload 或手工刷新时自动连接后继 Runtime 并恢复 RPC。数字只表示最近一次封板证据，不替代后续改动必须重跑受影响门禁。

## 10. 当前结论

P0–P156 已把已证明无 owner 的文档、设计资产、客户端风险推断、生命周期 facade、测试 convenience API、转发层、平行返回类型、无消费者订阅/selector、历史源码 marker 守卫，以及 added-then-abandoned 的条件解析、状态 patch、动态插件、内容渲染和独立 Codebase 向量索引纵切从生产面删除；Session 模型身份从分散的 model-only/global-default 推断收敛为一个可恢复 exact pair owner，workspace identity 也从裸 `cwd` 和多份 read-model 字段收敛为一个 exact Domain value 与一次 consumer projection，operation 带外元数据也从 binding 私有行为收敛为 Registry 单一方法事实，Agent Memory cache 从无身份裸 vector/curation backfill 收敛为 Search-owned exact-space/digest 条件缓存，recall corpus 从只消费 project scope 收敛为 exact-project + user 的单次联合 ranking，durable content 与 prompt material 也有了 Domain/consumer 各自唯一且可证明一致的上限。两个 app module 的 Go 基线统一为 1.27.0，Bootstrap 关闭图不再把 terminal diagnostic 误当成可重试生命周期状态，也不再让 caller timeout 取消唯一 Host shutdown generation。Git/workspace source discovery 只在明确 non-repository/unavailable 时进入 filesystem fallback，取消和仓库故障保持可见。保留项都有生成入口、运行时消费者、动态入口、测试基础设施或发布兼容义务。

P0–P138 已把主要缺陷从“调用处补判断”上移到领域不变量、Application transaction、进程/renderer generation、credential lifecycle、read-model 与 presentation owner；P120 把中央 Plan/Goal/Context/terminal narrative 从“移动旧 chrome”收敛为 Codex 的紧凑 standing grammar，P121 让文件变更 narrative 与右侧 Run Summary 共同服从 exact ToolCall 的持久补丁回执，P122 让审批可见事实与动作 scope 回到 Runtime material 和用户明确意图，P123 让 Question 的唯一展示位置、输入动作、IME 与 ordered Skip 共同服从 transcript/Runtime 权威结算，P124 让普通 ToolCall 回到统一的透明 work narrative，P125 让设置主色的 mutation、动态 contribution replacement、document paint、反馈与恢复服从同一 preference owner，P126 让 Goal edit/clear 的可见动作、Runtime quiesce/CAS 事务与 mounted snapshot 收敛服从同一权威纵切，P127 把新 Goal 从重复表单收敛为 Composer submit mode，并让早到 standing projection 与 Runtime mutation settlement 服从同一个 exact commit owner，P128 让 Runtime terminal phase、durable transcript 与 Frontend work/final presentation 共同服从同一事实，最终回答不再与过程工作共享 turn 或 actions，P129 进一步让 Application 生成的 Goal 控制上下文只对模型可见，不再污染用户 Transcript，P130 让所有新会话入口服从显式 exact-cwd，P131 把完成态 Question、active Plan、Goal tray 和正文节奏的剩余视觉偏差归还给各自唯一 presentation owner，P132 让图片查看器的 Download 从缺失/伪浏览器路径收敛到 exact Wails window 的原生 save owner，P133 让所有 dialog 的 interaction scope 回到同一个 scheme-independent scrim owner，由 surface 而非深色遮罩表达层级，P134 让 Work Index 的首帧、resize、ARIA、偏好与视觉证据服从同一 Codex geometry，并给 fractional composer tail 保留确定净空，P135 让同一待决命令的 approval/ToolCall 双事实服从单一 Codex request surface，结算后再恢复工具历史，P136 让 projectless 项目入口服从 composer overlay 层级、Work Index/native picker 与唯一 exact-cwd Session owner，P137 让 loading placeholder、真实 Shell 与 Context Dock 几何 observer 服从同一个可见性事实，入口能力不再早于布局 owner，P138 则让 Context footprint 成为可恢复 Run 事实，并让流式 viewport 的写权严格服从 Codex 式 raw follow/reader escape 边界。后续工作仍必须从真实产品反例开始，若不能说明唯一 owner、提交能力和失败后的 durable winner，就不能以新增 helper、刷新或兼容路径进入生产代码。
