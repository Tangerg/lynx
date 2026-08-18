# Lyra Runtime 执行计划

> 状态：P0–P117 已完成并形成里程碑；P118 实施中。
>
> 最近基线：2026-08-18，P117 完整验收；P118 已获得实施授权。

本文只拥有四类信息：当前授权、长期约束、里程碑索引、下一阶段准入。能力现状由
[`CAPABILITY_LEDGER.md`](CAPABILITY_LEDGER.md) 拥有；稳定合同由
[`CONTRACT_BASELINE.md`](CONTRACT_BASELINE.md) 拥有；架构与实施规则分别由
[`ARCHITECTURE.md`](ARCHITECTURE.md) 和
[`ENGINEERING_STANDARDS.md`](ENGINEERING_STANDARDS.md) 拥有。

P0–P114 的逐批红例、文件清单和门禁原始记录已冻结在 Git 快照
`babec316e:app/runtime/doc/EXECUTION_PLAN.md`。Git 是历史审计源，本文不再复制提交日志。

## 1. 当前授权

- P118 已于 2026-08-18 获得实施授权：在 P117 恢复基线上继续深度对齐 Codex 的 Desktop 交互与 UI，完整覆盖左侧 Work Index 的目录选择和 Session 创建/关闭/切换、中间 Agent Narrative 的 transcript/streaming/composer/HITL/Goal/Plan/scroll，以及右侧 Context Dock 的 Run Summary/Terminal/Diff/File/Timeline/Tool/Settings；同时证明这些可见面已接到现有权威 Frontend read model 与 command owner。
- 默认改动范围仅为 `app/desktop` 与 `app/desktop/frontend`。只有红例证明缺少权威事实时才进入 Runtime；突破 Runtime Protocol、Artifact、SQLite schema、公共 Go API、Agent Framework baseline 或 Frontend published SDK 前必须重新报告爆炸半径并取得确认。
- 第一批只从 production-equivalent Desktop 的盲测建立左、中、右三栏的接线、交互、空态/加载态/错误态、键盘、窄宽、Retina 与 light/dark 证据矩阵；问题不能形成可复现的用户动作与可见错误时，不以视觉偏好或 Codex 的私有实现形状修改生产 owner。
- 每个成立问题由唯一 presentation owner 根修复，不增加第二 read-model writer、全局 generation、server registry、transport matrix、刷新旁路、兼容层、timer 竞态掩盖或通用 Owner/Coordinator/状态机。每批独立提交推送，最终运行 Frontend 全门禁与 async leak、Wails production build 及必要的真实恢复验收。
- Codex 参考只用于提取页面心智模型、操作反馈和 presentation 机制；拒绝其多 connection、remote/projectless、浏览器 panel、Electron webview handoff 与私有状态分支。P118 不修改或暂存 `app/cli`，并保留所有无关工作区改动。

### P118 准入与完成条件（2026-08-18）

- 每个生产修复先形成能够失败的真实产品反例和独立红测提交，再由唯一 owner 完成根因修复提交；批次均须精确暂存并推送。
- 左栏必须把全局动作、Projects 目录入口、project row 与 Session row 的范围、禁用、焦点和创建/关闭/切换结果表达清楚；中栏必须让权威 Run/Item/Interrupt/Goal/Plan 在长对话、流式、历史加载和 continuation 中稳定呈现；右栏必须让 tab 身份、内容 material、命令 capability、空态和恢复态一致。
- UI 精修继续服从现有 theme/design-system token、单一 edge mechanism、最小 40px 可点击区域、键盘焦点、reduced motion、CJK/长路径/长标题、tabular numerals、容器宽度与 Retina device-pixel 约束；不以额外 chrome、卡片、圆角或动画代替信息层级。
- 完成前运行 Frontend 全门禁与 `--detectAsyncLeaks`、三栏完整 visual/WCAG/light-dark/Retina 矩阵、必要的 Runtime/Desktop gates、Wails v3 production package，以及 fresh database、renderer replacement、Runtime restart/SIGKILL 的真实产品 smoke；自动化会话、daemon、临时数据库和测试进程必须清理。
- 左栏首个 production 反例已锁定：project-row `+` 的 `sessions.create` 被 command owner 拒绝并返回空 identity 后，`useWorkIndexActions` 仍无条件聚焦 composer，焦点落回旧 Session 并制造创建成功的假反馈。唯一修复 owner 是该 action 的异步完成语义；现在仅在返回新 Session identity 后聚焦，与顶层 New 和目录选择入口一致，不增加 optimistic navigation、toast 旁路或第二 Session writer。
- 中栏紧凑窗口反例已闭环：在 1280×577 首次打开 waiting/question 时，Approve/Submit 的底边分别落到 composer 内 53px/89px。`use-stick-to-bottom` 只观察 content box，而 ChatStream 的实测 composer 净空通过 transcript padding 改变 border box；最终净空没有进入 follow lifecycle，初始视口停在尾部之外。`MessageStream` 仍是唯一滚动 owner，现在以同一个 follow fact 同时消费 subtree mutation 与 border-box ResizeObserver，并写库自己的精确 target，用户逃逸后保持 reader-owned 位置；未增加 timer、第二 scroll state 或 HITL 专用滚动旁路。紧凑 waiting/question、既有 tail clearance 与 reader escape 共 6 项 browser 反例及定向 unit/typecheck 全绿。
- 右栏窄窗反例已闭环：1120px window 保持 256px Work Index 时，旧 split 把 conversation 与 Dock 各压到约 432px，违反 640px reading floor，HITL、composer 与正文一起降成不可读窄栏。采用 Codex 的“空间不足时折叠 panel、保留 panel membership”机制：`ChatPanel` 只从 row ResizeObserver 推导 presentation capability，低于 640px conversation + 420px Dock 时调用既有 navigation owner 清除 destination，`contextDockStore` 的 tab set/last view/width preference 不变；toggle 同步禁用并解释需增大窗口。恢复安全宽度后由用户重开原 tab，宽窗 resizer 也以 640px conversation floor 计算上限；没有第二 visibility state、覆盖 drawer preference 或改写持久宽度。

### P117 红例、参考裁决与完成结论（2026-08-18）

- production Desktop 已复现：无 active Session 时打开 Context Dock，会把 Runtime 默认 workspace 投影成当前资源管理器内容；唯一 presentation owner 是 `ChatPanel` 的 exact active-Session 边界，默认 workspace 不是 Session material。
- Work Index 行为测试已锁定：顶层“新建会话”必须继承点击时 active Session 的 cwd；目录选择属于 Projects 的新增入口，不再与全局新会话并列成两个竞争动作。Codex 主参考采用“New chat 延续当前 local project”与 Projects 标题栏新增项目的机制；拒绝其 projectless、多 connection、remote project 分支，因为 Lyra 产品拓扑不包含这些身份。
- provider 不可用时已复现多个新 Session 都停留在“未命名会话”。Runtime 代码审计确认 `runsegment.Finalizer` 是唯一 title maintenance owner，`sessions.Coordinator` 以 first-writer 语义持久提交；第二批红例锁定 generator 返回 provider error 时，同一维护链会丢弃可由 opening user text 确定生成的有界 fallback。该缺口只进入内部 Runtime adapter，不在 Frontend 增加第二标题 writer，也不改变 Protocol、Artifact、SQLite schema、公共 Go API 或 Agent Framework baseline。
- 首批可失败测试只覆盖上述已成立边界；Zcode/Minimax 仅在后续恢复反馈或 workspace 密度需要第二证据时使用，不为已由 Codex 与 Lyra owner 共同确定的交互再引入第三套词汇。
- 首个根因纵切已完成：顶层 New Session 绑定 active Session 的 exact cwd，Projects 标题栏唯一拥有目录选择入口，无 active Session 时 ChatPanel 不挂载 Dock view 或 toggle。隔离 Runtime 实测两次 `sessions.create` 均落在 `/private/tmp/lyra-p117-b1-project`，欢迎页不再出现 Dock 入口；定向 11 tests、typecheck、lint、design-system、interactive chrome 与 8 locale 守卫全绿。人工核对 populated/loading/error、light/dark 与 Retina 实际图后，6 张 shell golden 已移除旧全局 Open folder 行，整份 shell visual 17 tests 全绿。
- 第二个根因纵切已完成：`sessiontitle.Generator` 在 utility model 缺失、空回复或 provider error 时返回 opening user text 首个有效行的 Unicode-safe、有界 deterministic fallback；provider error 与 fallback 可同时返回。`runsegment.Finalizer` 仍是唯一维护 owner，先经 `sessions.Coordinator.ApplyGeneratedTitle` 的 first-writer 提交可用标题，再把原错误记录到既有 maintenance span。未增加 Frontend writer 或 Runtime 公共 surface；adapter 定向 test/vet 全绿。fresh database + 不可达 provider 的真实 Runtime smoke 中 Run 仍以 `provider_unavailable` 终结、title maintenance span 仍为 error，而后续 `sessions.list` 已持久返回 `Diagnose provider outage`，证明导航身份与诊断事实同时保留。
- 第二层可见矩阵红例已闭环：production `composerBootstrap` 显式 requires Agent Session service，消息反馈还消费 Runtime stream service；agent visual fixture 原先只装 `agentFold`，并在 composition graph 外手动配置 module port，也没有拥有 command/interrupt lifecycle。Dougong 因缺少 setup-time provider 拒绝安装；补齐 service 后，HITL 与 steer/stop 又以 retired command owner 和未安装 interrupt coordinator 失败。唯一修复 owner 仍是 `installVisualAgentFixture` 的 test composition：现在它通过 production Service contract 发布 deterministic Agent Session/Runtime stream ports，并在 composition cleanup 内安装和释放既有 command/interrupt owner；没有放宽 production dependency、增加第二状态 writer 或伪造等待。empty/running/HITL、Dock、WCAG、长内容、IME、Retina 与 light/dark golden 的整套 255 tests 已全绿，人工抽查确认活动审批不再错误露出终态 footer/message actions，底部跟随与稳定 footer 基线已按当前 production projection 刷新。
- 三栏 Codex 对照审计已完成中央 transcript 首个纵切：Codex `thread-scroll-layout` 以底部距离为权威读数，footer 高度由 ResizeObserver 驱动，并在 wheel/pointer/keyboard 用户滚动时立即退出自动跟随；采用这一 owner/escape 机制，拒绝其 reverse-scroll DOM 形状，因为 Lynx 既有 `use-stick-to-bottom` 已拥有等价的距离与用户逃逸语义。Lynx 原先额外用固定 250ms RAF 窗口反复写 `scrollTop = scrollHeight`，长会话刚打开后用户向上滚动，下一帧会从 `240` 被抢回 `1000`。`MessageStream` 现在只做一次 composer mount geometry 对齐；异步 Markdown/Shiki 的 DOM materialization 在 mutation microtask 内读取既有 `isAtBottom` follow fact 并准确补偿，用户 wheel escape 后同样的 180px 增长只增加底部距离、`scrollTop` 保持 `159`。没有 timer、第二 scroll state 或跨 Session observer；unit 红例、9 项长内容定向 browser matrix，以及覆盖 HITL/footer/Dock/WCAG/IME/Retina/light/dark golden 的完整中央 211 tests 全绿。
- 右侧 Codex 对照首个纵切已闭环：采用 app-shell 将 panel visibility 与 tab membership 分离、关闭/重开不销毁 tab 的机制；拒绝照搬其独立 bottom panel 和“空 panel”状态，因为 Lynx 的 Terminal 是只读 Agent command material，且现有 URL destination 必须继续唯一拥有 Dock 是否可见。`contextDockStore` 现在只持久化 exact Session 的 open tab set、last view、focused diff file 与 file preview target；active Session、active destination、Tool selection/expanded material 均不落盘，后两者随 generation/renderer 退休，避免复活 stale Tool。旧版本与 Zod-invalid payload 整体丢弃，不做 migration 或猜测恢复。renderer module replacement 红例现已恢复 `explorer/diff/file`、last `diff` 与 `src/runtime.ts:42`，定向 38 tests、typecheck、lint，以及覆盖 close/reopen、neighbor selection、keyboard/WCAG、loading/error、light/dark golden 的完整 workspace visual 45 tests 全绿；没有向 Runtime、query cache 或第二导航状态写事实。
- 相邻 Session lifecycle 红例也已闭环：关闭当前 Session 时，Agent Session owner 会先从 `openSessionIds` 释放它，再把 URL 切到相邻 Session；这段同步窗口内旧实现虽从已保存 map 删除旧 scope，persist partializer 却会把仍标作 active 的顶层 scope 重新收录。`forgetSessionScopes` 现在以 Agent Session owner 给出的 exact open set 同步过滤已保存 scope；当前 presentation scope 不在集合时，同一原子写将其身份和 tab/file/tool material 全部退休，随后既有 navigation subscriber 再激活相邻 Session。没有等待下一次 renderer replacement、timer 或第二 lifecycle writer。
- 右栏内容 renderer 的 File preview 红例已闭环：同属 workspace code surface 的 Diff 已按 exact file path 选择 Shiki grammar，File preview 却曾把 Go/Python/JSON 等所有内容硬编码成 TypeScript；因此文件正文虽然正确，语法 token 语义和颜色错误。Codex 的 file/diff surface 都由文件身份决定语言；`FileView` 现在接收 workspace query 对应的 exact path，并复用现有 `langFromPath` 与 `resolveLang` 后执行一次 whole-file highlight，未知或未加载 grammar 明确降为 text。没有复制 extension table、从内容猜语言或增加第二 file identity。
- File preview 的相邻滚动红例也已闭环：plain DOM 已含目标行，旧导航 effect 会先居中一次；Shiki 异步 materialization 改变 `highlighted` 后又重放同一滚动，能覆盖用户在这段时间内取得的阅读位置。定位 effect 现在只响应 exact path、content replacement 或 target line 变化；语法 materialization 只替换 token，不再调用 `scrollIntoView`，没有 timer、二次 scroll state 或延迟补偿。
- Timeline 恢复态红例已闭环：Runtime connection 不可接收命令时，中央 delegated Run card 已禁用 Cancel，右栏 Timeline 的同一 Run Stop 却曾仍可点击并尝试投递。Timeline 的 locate 与 cancel 现在都消费同一 `runtimeAvailable` capability；断线/恢复期间 Run 审计事实仍可读，命令按钮同步禁用且不会调用 cancel owner。没有复制 connection phase、点击后补错误 toast 或让只读投影冒充可写 Runtime。
- 右栏 tab overflow 红例已闭环：renderer 恢复到末尾 tab 或 picker 打开末尾 tab 时，旧实现只更新 active destination，没有把 active tab 带回可见区；首次 nearest-scroll 验证又证明单向 trailing fade 不成立——向右后的左边缘从 `preview` 半词硬切入，用户看不出左侧还有 tab。`AgentDockTabs` 现在仅在 active identity 变化时 nearest-scroll，并由 strip 自己的 scroll geometry 维护 start/end DOM presentation attributes，双向 edge fade 只表达实际隐藏内容。该局部 ResizeObserver 只观察 strip 与 tab box，不写 React/Dock/URL 状态，不轮询或驱动导航。
- production Wails 的真实 Runtime SIGKILL 又暴露左栏命令撤权缺口：连接 banner 已呈现“Runtime 暂时不可用”，composer 与 Goal 已禁用，顶层 New Session、Projects 目录入口和 project-row `+` 却仍会投递 `sessions.create`，最终只留下失败 toast；同一审计还确认顶层 New 为继承 active cwd 传入 `{cwd}` 后，绕过了 Session owner 既有的空 draft destination 复用语义，连续点击会分配第二个隐藏 draft。Work Index 现在分别投影“active workspace 可创建”与“指定目录可创建”两项 capability，事件处理时再次读取同一 Runtime command owner；native picker 跨 await 返回后也重新证明 command capability。palette/快捷键的 New 同样在命令入口撤权。顶层 New 以 `reuseFreshDraft` 明示自己已证明 cwd 属于 active Session，只复用该空 draft；Projects 的显式 cwd 创建仍始终新建。未增加离线队列、toast 旁路、第二 connection 状态或 timer。
- P117 最终 recovery smoke 使用 fresh HOME/SQLite 与 production-equivalent Wails 二进制：renderer reload 后 URL 仍指向同一 Session 与 `explorer` Dock；空 draft 连续 New 前后 SQLite Session 数保持 1。SIGKILL Runtime 后 Desktop PID 保持 31650，顶层 New、Add Project、project-row `+` 与 `⌘N` 同步撤权且没有失败 toast，Session、资源树和 Dock 仍可读；后继 Runtime 以 PID 85165 和新 `instanceId` 启动后，原窗口自动清除连接告警并恢复命令能力，没有 reload、离线队列或本地 optimistic 拼接。Frontend 普通与 `--detectAsyncLeaks` 均为 313 files / 1945 tests，agent/shell/workspace/closure visual 274 tests 全绿；Runtime 全量 test/vet/build、Desktop Go test/vet 与 Wails production `.app` package 全绿。

## 2. 长期产品与架构约束

1. 真实产品严格为一个 Desktop actor 对一个逻辑 Runtime。Runtime 可以经 HTTP、socket 或同进程 binding 接入；进程重启、连接重建或 binding 变化不产生“旧服务端/后继服务端”关系。Desktop 与 CLI 共享目录只属于已有存储并发合同，不扩张为多客户端产品架构。
2. Runtime 只能通过 `adapter/agentexec` 接入 Agent Framework。Agent inner ring 不得依赖 Runtime、RPC、Desktop、SQLite、持久化或产品终态词汇。
3. Domain、Application、Adapter、Infra、Delivery、Bootstrap 各自拥有自己的事实和机制；跨层依赖必须沿既定方向，不能用 locator、全局可变状态或 DTO 反向穿透。
4. mutation、query、event、optimistic state 和 material snapshot 不能同时写同一个 read model。每份可见状态必须有唯一 writer、generation 和提交边界。
5. renderer、Plugin Host、Runtime 进程和 connection 只在各自真实可替换的进程内资源上使用 generation。迟到 response/callback、final close 和重复 dispose 必须服从 exact owner identity；进程代际不能冒充逻辑 Runtime 或 durable store identity。
6. 修复必须落在所有权、生命周期、事务边界或领域不变量上。禁止兼容补丁、双路径、刷新绕过、基于延时的竞态掩盖和“先留 TODO”。
7. 优先 OOP 与充血模型。对象应持有行为所需的状态、不变量和生命周期；不把同一业务动作摊成跨文件过程脚本。
8. 不过度设计和过度防御。抽象、generation、guard、retry、锁、状态机和测试层级都必须对应当前产品中的真实反例；边界已经验证且事实不可变时，不重复包装或校验。

## 3. 参考基线

参考用于提取机制和反证，不是兼容合同，也不是目录/类型形状模板。

- 服务端主参考：[`/Users/tangerg/Desktop/study/codex-server/codex-rs`](/Users/tangerg/Desktop/study/codex-server/codex-rs)，重点研究 Rust 实现中的进程 incarnation、事件/请求 identity、断线恢复、持久状态重建、取消和迟到 settlement。
- 前端主参考：[`/Users/tangerg/Desktop/study/codex`](/Users/tangerg/Desktop/study/codex) 解包 UI，重点研究 Run Summary、Terminal、Diff、Tool/审批卡、Goal/Plan、Session navigation、Dock、loading/empty/error feedback 和长对话心智模型。
- 前端补充参考：[`/Users/tangerg/Desktop/study/zcode`](/Users/tangerg/Desktop/study/zcode) 与 [`/Users/tangerg/Desktop/study/minimax`](/Users/tangerg/Desktop/study/minimax)。
- 每个采纳点必须说明 Lyra 中真正的 owner；不采纳时记录产品约束或架构理由。不得复制 Codex 的多 connection 设计、私有状态、包结构或产品词汇。

## 4. 历史里程碑索引

| 阶段     | 完成主题                                                                                                        | 稳定结论                                                                                                                                                                                  |
| -------- | --------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| P0–P12   | 事实基线、目标依赖 DAG、Run/Interaction/Tool/HITL/child/recovery 纵切、协议与消费者移交                         | 建立 Clean Architecture 与 Agent Framework 防腐边界，删除原执行双环和迁移兼容面                                                                                                           |
| P13–P23  | 领域与 Application 精修、package/命名、公共 `protocol`/`embedded`、真实 consumer、WorkingContext provenance     | 公共 Go surface 和内部职责分离，领域行为回到聚合，消费端只依赖明确合同                                                                                                                    |
| P24–P38  | Runtime/Desktop 时序、HITL、Goal、Run 订阅、事件断号、失效 Session 与恢复                                       | 冷启动、断线、终态和旁路事件开始服从持久事实与单一恢复入口                                                                                                                                |
| P39–P58  | Session lifecycle、command replay、mutation/read model、Knowledge CAS、Git/HTTP sidecar、Runtime 连接和外部失效 | 条件写、文件身份、事件协商、连接 owner 和配置失效形成可证明边界                                                                                                                           |
| P59–P83  | 单客户端真实交错、SQLite owner/lease、事件与物料一致性、幂等和恢复反证                                          | 中间批次持续消除跨代拼接、双 writer、恢复猜测和非原子结算；详细证据见冻结快照                                                                                                             |
| P84–P97  | 多 Runtime 共享库业务所有权、survivor recovery、compaction、EventCommit 与 composite command 回执丢失           | SQLite durable winner、exact command receipt 和 request-detached proof 成为唯一事务结算依据                                                                                               |
| P98–P112 | Desktop Plugin Host、sideload、Context Dock、Tool/Terminal、Run Summary、审批事实、material snapshot            | 插件与 renderer 生命周期收敛；挂载 Session 的 HITL/Plan/Goal/Run/Tool 以一次 material transaction 恢复                                                                                    |
| P113     | Runtime 内部坏味道治本清理                                                                                      | 合法构造、required dependency、共享并发 owner 与 package 粒度收敛；只为单一调用者存在的微模块被吸回真正 owner                                                                             |
| P114     | 单 Desktop/逻辑 Runtime 的进程恢复、真实接线和 UI 打磨                                                          | renderer、Runtime process、command、query 与 material 在真实替换点服从 exact owner；断线、重启、SIGKILL、迟到响应和长对话产品路径完成反证闭环                                             |
| P115     | 前后端可维护性治本收敛                                                                                          | durable unresolved-command、lifecycle publication、Frontend extension、Bootstrap resource 与 Runs execution handoff 各自回到唯一 owner；真实 SIGKILL 验收补齐 path-owned local credential |
| P116     | 真实产品拓扑下的恢复与守卫校准                                                                                  | claimed Resume 由 claim owner 原子补偿；Mutation Journal 只认 durable namespace；删除不能表达架构边界的一次性 object-literal 语法守卫                                                     |
| P117     | Desktop 恢复反馈与 Codex 对齐的三栏 UI 精修                                                                     | Work Index、transcript 与 Context Dock 服从 exact Session、reader-owned scroll 和 Runtime command capability；renderer reload 与 Runtime SIGKILL 后原窗口原 workspace 可见恢复            |

## 5. 当前里程碑结论

P113–P117 共同建立了以下不可回退的心智模型：

- 产品始终只有一个 Desktop actor 和一个逻辑 Runtime。renderer、Plugin Host、Runtime process、connection、command、query writer 和 mounted material 仅在真实可替换边界拥有局部 generation。
- Runtime 每次进程实例发布新的 opaque `instanceId`；同 endpoint 重启只替换进程内资源，不替换逻辑 Runtime、SQLite durable identity 或 mutation store identity。
- 进程内 owner replacement 先发布新实例，再同步退休旧实例。只有异步间隙可能发生 replacement，且后续会修改当前共享状态时，提交和 cleanup 才需要 exact owner proof。
- `sessions.snapshot` 是挂载 Session 的原子 material owner；HITL、Plan、Goal、Run、Tool 不能再由独立 query/event/material 多路拼接。
- durable mutation 以事务 marker/identity 判断“已提交但成功回执丢失”；不得靠重试猜测或本地 optimistic 状态冒充服务端事实。
- Run Summary、Terminal、Diff、Tool selection、Goal、Plan、审批、Session/Dock navigation 只消费所属 Session 与 generation 的权威投影。
- Desktop 冷启动依赖在 composition root 显式声明；Composer、Recipes 和 Workspace Events 的 session ports 不再依赖偶然安装顺序。
- local transport token 由 durable data path 拥有，不属于 Runtime process generation；`instanceId` 换代不撤销仍存活 Desktop 的认证能力。
- 流式输出期间，消息底部反馈/操作区服从可见 turn 的稳定 material 边界，不能跟随每个 delta 反复挂载造成闪烁。

最近一次完整验收基线：Frontend 313 files / 1945 tests 与严格异步泄露门禁全绿，99 条 published context edge 无环，87/87 Runtime operation fact families、3/3 sidecars、16/16 events 有产品消费者；agent/shell/workspace/closure visual 274 tests 覆盖 streaming、HITL、Session/Dock、WCAG、IME、Retina 与 light/dark golden。Runtime standalone 全量 test/vet/build、Desktop Wails v3 Go test/vet 与生产 `.app` 打包通过。fresh HOME/SQLite 的真实 smoke 中，renderer reload 保留 exact Session 与 `explorer` Dock；Runtime SIGKILL 让 PID 32453 换为 85165，Desktop PID 31650 未变，原窗口自动撤权并恢复，空 draft 连续 New 前后 SQLite Session 数保持 1。

## 6. 新阶段准入

新 Goal 必须先完成以下内容，才可开始生产代码：

1. 用当前产品构建提出一个可复现的红色反例；“代码看起来不舒服”只能触发审计，不能直接触发抽象。
2. 指明唯一状态 owner、生命周期、事务边界和允许的 breaking surface。
3. 对照服务端/前端参考，分别记录采纳机制与拒绝理由。
4. 按改动风险定义一个 Desktop/逻辑 Runtime 的真实恢复矩阵和必要门禁；不要求每个局部批次机械运行 SQLite、Frontend、race、fuzz 与生产 Wails 的全集。
5. 证明没有引入第二 writer、第二执行循环、兼容双读、刷新旁路、timer 掩盖或对 `app/cli` 的改动。
6. 证明没有为多窗口、多服务端、假想 transport 组合或不可达状态引入抽象与防御分支。

候选方向保留在 [`inspiration/`](inspiration/)；它们不是实施授权。P117 已完成，下一阶段必须先形成新的真实产品反例与独立授权。开始下一阶段时只在本文新建简短阶段条目，完成后更新里程碑结论与能力事实，不恢复逐提交流水账。
