# Lyra Runtime 执行计划

> 状态：P0–P134 已完成并形成里程碑。
>
> 最近基线：2026-08-21，P134 Codex Work Index 几何、首帧所有权与 composer 尾部净空收口。

本文只拥有四类信息：当前授权、长期约束、里程碑索引、下一阶段准入。能力现状由
[`CAPABILITY_LEDGER.md`](CAPABILITY_LEDGER.md) 拥有；稳定合同由
[`CONTRACT_BASELINE.md`](CONTRACT_BASELINE.md) 拥有；架构与实施规则分别由
[`ARCHITECTURE.md`](ARCHITECTURE.md) 和
[`ENGINEERING_STANDARDS.md`](ENGINEERING_STANDARDS.md) 拥有。

P0–P114 的逐批红例、文件清单和门禁原始记录已冻结在 Git 快照
`babec316e:app/runtime/doc/EXECUTION_PLAN.md`。Git 是历史审计源，本文不再复制提交日志。

## 1. 当前授权

- P134 已完成：本批准入时 production Work Index 默认宽度仍为 256px，End 在 1440px 窗口曾可扩到 800px；本地 Codex Desktop 的同一 owner 明确使用 275px default、240px floor、520px ceiling，并让 live window 至少保留 240px reading plane。问题属于 shell geometry owner 漂移，不以局部 CSS、截图专用宽度或 Dock 的 640px reading floor 修补。
- `shellGeometry.ts` 现在唯一拥有 Work Index 的 275/240/520/240 边界；`AgentAppShell` 在 layout phase 写入已夹取的持久偏好，删除全局 256px fallback，agent/foundation/workspace/shell visual fixture 全部消费同一 default。右侧 Context Dock 保持自己的 640px conversation floor；Work Index 与 Dock 不共享 clamp 或 writer。
- 本批视觉传播进一步复现了 fractional overlay 与整数 `scrollTop` 之间的 1px 尾部净空缺口；唯一 `COMPOSER_CLEARANCE` 在原 1rem 节奏外增加 1px rounding guard，long-content 五次定向重复、明暗/18px golden 与完整矩阵共同证明最后一条消息不进入 composer glass。红例 `bbe20ecac`、根修复 `66f5e6231`、Dock live-clamp 测试 `6e7498324`、Work Index golden `8e6831be9`、净空修复 `41bc002de` 与尾部 golden `67b9707c0` 已逐轮推送；未改变 Runtime/Protocol/Artifact/SQLite、Frontend published SDK 或 `app/cli`。
- P133 已完成：当前 production Goal editor 的 scrim 在 light 下使用 22% 黑、dark 下使用 50% 黑，用户截图中的背景层级因此明显重于 Codex；本地 Codex Desktop owner 则在两种 scheme 下统一使用 `#00000022`，由 dialog surface 自己承担深度。当前 Goal editor 的 420px 几何、actions、快捷键与 attached Goal row 已与 Codex source 一致，不因历史截图重新引入旧 Goal 表单、limits chrome 或第二状态。
- 全局 `--color-scrim` 是 modal scope 的唯一 presentation owner；light/dark 均收敛到 `#00000022`，Goal editor、Markdown table preview 等现有 dialog 自动消费同一事实，不建立 per-dialog override。红例 `f10d63d33`、根修复 `670510280` 与 golden 收口 `ab1688654` 已逐轮推送；四张受影响的 light/dark modal golden 已人工核对。
- Frontend 324 files / 2019 tests、完整静态/构建门禁与 320 项 visual/WCAG/keyboard/coarse-pointer/IME/CJK/18px/Retina/WebKit 矩阵全绿；Runtime/Desktop test/vet/build 与 `wails3 build` 全绿。fresh SQLite epoch 76 的 production Runtime 与 Wails binary 已通过同一 `instanceId` health/info、单 listener、真实 OPTIONS/POST `/v2/rpc` 接线 smoke；历史测试数据与本轮 fresh smoke 数据已退出 active data path。本批未改变 Runtime、Protocol、Artifact、SQLite shape、Frontend published SDK 或 `app/cli`。
- P132 已完成：能力台账中长期保留的图片 Download 缺口已由本轮“全部内容渲染必须真实前后端接线”的明确授权触发。Codex 本地实现证明图片查看器默认提供 Download，但其 cloud/HTTP materialization 不适用于 Lyra；本批只采纳“显示内容先交给 Desktop 原生 save-file owner”的机制，继续拒绝远程图片与浏览器 `<a download>` 伪接线。
- `DesktopHost.SaveImage` 是新增且唯一的 Wails v3 IPC owner：只接受前端已允许渲染的 inline image data URL，在打开原生面板前校验 MIME、解码内容并生成建议文件名；Wails adapter 把 save sheet 绑定到发起请求的 exact window，用户选定路径后才写入，取消明确返回 `false`。Frontend gallery 只经窄 adapter 调用该 owner，不取得路径、不写第二 download 状态、不增加 Runtime/Protocol/Artifact/SQLite surface。
- lightbox 右上角现在按 Codex 工具组顺序呈现 Download、Close，二者均为 40px target；保存期间 exact action disabled/`aria-busy`，失败只给本地 toast，不向 transcript 写杂项。红例提交 `2bbc84cbe`、根修复 `b4f69c6a0` 已推送；Frontend 324 files / 2019 tests、完整静态/构建门禁与 320 项 visual/WCAG/keyboard/coarse-pointer/IME/CJK/18px/Retina/WebKit 矩阵全绿，Desktop test/vet/build 与 `wails3 build` 全绿，production binary 已启动 smoke。本批未修改 Runtime、Protocol、Artifact、SQLite、Frontend published SDK 或 `app/cli`。
- P131 已完成：以 Codex 本地解包源码和四张 production 现状截图交叉定位，不把附件中的粗糙实现当成目标。完成态 `request_user_input` 已从常驻灰卡收敛为默认关闭的 `Asked N questions` activity disclosure，只有用户展开后才显示 13px、16px 行高的问答；pending Question 的 exact Run/Item、composer replacement、ordered answer 与 IME owner 保持不变。
- active Plan 继续位于 composer 上方，但改为 Codex 的固定 32px 布局槽与底部 4px 对齐；tooltip 使用 8px inset、8px row gap、16px mark 和 13/16px 文本，完成项只降 ink、不画删除线。Goal attached tray 改为 4px 垂直 inset，并以默认 16px、放大字号时不裁切的等价行高呈现 lifecycle/objective；预算、限制、用量、模型与结算信息仍不进入前端。
- 正文 `leading-prose` 从 `font-size + 10px` 收敛到 Codex 的 `font-size + 8px`；由此形成真实 overflow 的 streaming reasoning scrollport 同批补上键盘入口，没有新增第二 disclosure/scroll owner。Frontend 324 files / 2016 tests、完整静态/构建门禁和 320 项 visual/WCAG/keyboard/coarse-pointer/IME/CJK/18px/Retina/WebKit 矩阵已通过；新增 Plan tooltip 与 settled Question collapsed/expanded 三张 golden。本批未修改 Runtime、Protocol、Artifact、SQLite、Desktop/Wails、Frontend published SDK 或 `app/cli`。
- P130 已完成新会话 exact-cwd 收口：顶层 New、project row 与 welcome draft 都必须先由用户明确选择既有项目或目录；不存在隐式 home Session、空 identity 后聚焦旧 Session 或 pending first-message handoff。选择目标后仍由唯一 Session create/navigation owner 提交，取消选择保留 welcome draft。
- P129 已于 2026-08-20 完成：fresh production smoke 证明自治 Goal 每轮由 Application 生成的控制提示仍被持久成 `userMessage`，因而在中间 Narrative 形成巨大的用户气泡；Codex 本地实现则用独立 Goal state 和 `appendTranscriptItem: false` 表达同一边界。问题属于 Runtime opening projection 的语义混同，不是 Frontend CSS 或字符串过滤缺陷。
- fresh Goal opening 现在显式携带 model-only input：同一个 opening transaction 仍把控制提示原子写入 provider Conversation，却不创建 Transcript Item，也不返回 `UserItemID`。普通外部 `runs.start` 合同不变；真实用户 resume input 继续同时进入 Conversation 与 Transcript，不能被内部控制路径吞掉。
- 独立红测提交 `343caabf0`、Runtime 根修复 `1985e95a9` 与架构词汇守卫修复 `c222a9060` 均已推送。Frontend 323 files / 2009 tests 与完整静态/构建门禁全绿；Runtime/Desktop test/vet/build、generator diff 和 Wails v3 production package 全绿。本批未改变 Protocol、Artifact、SQLite schema、公共 Go API，也未修改或暂存 `app/cli`。
- fresh SQLite 的 26 次自治 drive 与 Runtime restart smoke 已证明：Conversation 中保留 26 条模型控制消息，Transcript 控制提示泄漏为 0，唯一真实用户 Item 保持可见；停止后 Goal attached tray 只显示 paused lifecycle/objective/actions，reload 后恢复同一状态且 DOM 仍无控制文本。
- P128 已于 2026-08-20 完成：production-equivalent 盲测先证明过程说明、工具工作与最终回答仍被折进同一个 Assistant turn，并共享最终回答操作栏；Codex `codex-rs` 的协议、thread state 与 streaming owner 共同证明 commentary / final answer 必须是 Runtime authored phase，而不是 Frontend 根据位置、流式状态或文案推断的视觉标签。
- Runtime 现在只在权威 ModelCall completion boundary 为 terminal AgentMessage 写入 `commentary` 或 `finalAnswer`；带 tool calls 的回复属于 commentary，无 tool calls 的终结回复属于 final answer，interrupt/suspend/cancel/loss/steer/tool boundary 关闭的 partial stream 保持 commentary。Domain Transcript、SQLite epoch 76、Artifact v20、Protocol/generated Go/TypeScript/validator/docs 与 Frontend published model 同步携带该事实，旧 Artifact 版本确定性拒绝；Protocol 日期保持 `2026-08-17`。
- Frontend provisional text 先进入既有 work narrative，terminal `finalAnswer` 到达后再以稳定 `final:<itemId>` identity 从过程行移入独立回答行；live、replay 与 mixed hydration 因而收敛。commentary/canceled/waiting narrative 不发布 context menu 或 message actions，只有 final answer 拥有 Copy/Regenerate/Good/Poor；同 Run 紧邻 final answer 时，前一过程行继续按 Codex wave grammar 折叠 reasoning/tools。
- 红测 `e6daf2295`、Runtime 根修复 `89335ef6f`、Frontend 分层 `dc7450a47`、contract fixture `8c85216f9`、fold/presentation/visual 收口 `7247f9e8d`、`dccf4ec61`、`347a8f3cc`、published facade `de585ccc7` 均已逐轮推送。Frontend 323 files / 2009 tests、320 项 visual/WCAG/keyboard/coarse-pointer/IME/CJK/Retina/WebKit 矩阵与全静态/构建门禁全绿；Runtime/Desktop test/vet/build、generator diff、fresh SQLite epoch 76、真实 HITL/tool/final answer reload 与 Wails v3 1439×899 原生 smoke 均通过。本批未修改或暂存 `app/cli`。
- 用户指出的两张历史 Goal/composer 错误截图已从本机删除并排除出视觉证据；唯一保留的附件参考是 context 环占用提示。后续视觉判断继续以 Codex 真实产品/源码和可复现 product smoke 为准。
- P127 已于 2026-08-20 完成：用户再次确认两张历史附件展示的是待修错误现状，不是 Codex 目标；Goal 不能把旧表单或旧 banner 换个位置继续呈现。可见依据只取自 Codex 本地解包源码与真实产品行为，错误附件不再参与视觉判断。
- 新 Goal 现在由 `/goal` 武装同一个 Composer submit mode；原 Composer 继续独占 draft、附件、历史、IME、Enter/send 与提交成功后的清空。非激活时不留图标、边线或高度，激活时只在 footer 显示紧凑 Goal identity，并可从同一入口退出；不再存在第二 objective draft、Goal launcher bus 或目标/轮次/花费/步数表单。
- `COMPOSER_SUBMIT_MODE` 是通用 Frontend extension point，Goal 只贡献 mode identity 与事务 owner。Runtime `goals.start` 成功前不清 draft；失败、编辑、切换 Session、附件存在或 replacement confirmation 均由同一 owner 明确结算。真实 smoke 发现 standing Goal projection 可先于 start promise 到达，现由 `starting` phase 保留提交权，直到 exact command owner 完成 `accept()`，避免已创建 Goal 却残留输入文本。
- Goal tool content 只呈现 objective、lifecycle 与必要说明，不渲染 Runtime limits、budget、usage、model 或轮次/花费/步数轴。红测 `14664ffed` 与 `654e224c7`、根修复 `baf3ed264` 与 `661054419` 已逐轮提交推送；Frontend 323 files / 2005 tests、完整静态/构建门禁与 319 项 visual/WCAG/keyboard/coarse-pointer/IME/CJK/Retina/WebKit 矩阵全绿。fresh `LYRA_HOME` 的 production smoke 已验证 Goal create、pause、clear、composer 清空与 `spinbutton=0`；Wails v3 dev host 完成 frontend/Go build、strict codesign 与 1439×899 原生窗口启动。
- P126 已于 2026-08-20 完成：用户前两次附件是待修错误现象，不是目标参考，已从视觉依据中完全排除；本批只以 Codex 本地解包源码和真实产品行为提取 Goal/Plan 的层级、动作顺序、尺寸与反馈机制。Plan 继续沿用已由源码确认的紧凑进度 pill，Goal 收敛为 composer attached top tray：整段 objective 可点击，右侧依次为 clear、pause/resume、edit。
- Goal editor 对齐 Codex compact dialog：420px 宽、20px content inset、36px identity block、12 行 sans textarea、Cancel/Save 与 Cmd/Ctrl+Enter；Frontend standing UI 仍只展示 lifecycle 与 objective，不渲染限制条件、预算、用量、模型或其他结算 chrome。composition 中的 Enter 不提交编辑，所有动作共享既有 command busy/retirement owner。
- 独立红测先证明 `goals.update` 与 `goals.clear` 在 Runtime catalog、Frontend gateway 和 Goal surface 中缺失；Runtime 现在在 quiesce exact owned drive 后以 fresh incarnation/CAS 更新 objective，只有原 lifecycle 为 active 才按冻结事实重启 drive；clear 在 drive 静止后 CAS 删除，Goal 已不存在时幂等成功，外部 owner 与 complete Goal 不被越权修改。
- Frontend 的 update/clear 继续经过同一个 per-Session Goal command owner、generation retirement 与 mounted `sessions.snapshot` repair，没有第二 writer、乐观 Goal cache、刷新旁路或伪成功。新增合同是 additive operation，未改变 Protocol version、Artifact、SQLite epoch 或既有 durable shape。
- P125 已于 2026-08-20 完成：production-equivalent 设置页先证明主色按钮点击会触发 `Duplicate contribution`，异常中断同一个偏好更新 listener 链，使 React 选中态、document theme painter 与持久化都收不到反馈；问题不是纯视觉弱，而是动态 custom theme 每次更新都向单键 extension point 重复注册且不退休旧 contribution。
- P125 没有增加第二 accent state、刷新旁路或协议字段。custom theme plugin 现在只拥有一个可替换 contribution，更新时先 dispose 旧值再发布新值，plugin cleanup/HMR 同步退订 preference listener 并释放 contribution；主色控件继续由权威 preference 驱动 `aria-pressed`、实际主题和持久化，28px target、hover/press、selected ring 与 check mark 只加强同一结算事实的点击反馈。
- P124 已于 2026-08-20 完成：production-equivalent 盲测先证明 read、shell、patch、failure 与 denied 调用仍被各自的蓝/红/黄卡片切碎，running shell 也长期占据高饱和容器；对照 Codex 本地 activity、subagent 与 exec-shell 实现后，Frontend 现在把单次工具调用统一投影为透明 work-narrative row，卡片材质只保留给 delegated Run 等真正的复合产品边界。
- P124 没有改变 ToolCall material、Runtime Protocol、Desktop/Wails 或 Frontend published SDK。工具身份先于摘要，展开箭头后置且只在 hover/focus/open 时出现；展开体回到 reading edge，由 shell、patch、reasoning 等 material 自己声明内部 inset。denied、error 与 exit code 继续呈现真实结论，但不再借助黄色 badge、红色卡片或第二状态 chrome 放大。
- P123 已于 2026-08-20 完成：production-equivalent 盲测先证明 pending Question 仍把旧式 `Input needed` 卡片、header chip、preview sidecar 与普通 composer 同时呈现；对照 Codex native request 后，Frontend 现在只从已挂载 transcript rows 选择最新 unanswered Question，把它作为 composer rung 的唯一请求表面，不新增 query、read model 或第二 interrupt owner。
- P123 沿用 exact Run/Item 和 ordered `string[][]` response wire，不增加字段或兼容分支；仅把每个 field 的空 inner values 定义为用户明确 Skip，outer field 数量、顺序和 identity 校验保持不变。Question freeform 与 Composer 共同消费既有 IME 键盘意图 classifier，Runtime Application 与 durable transcript validation 同步接受并持久化 Skip。
- P122 已于 2026-08-20 完成：production-equivalent 盲测先证明 Approval/HITL 仍是旧式黄边警告卡，并叠加 `Approval required`、risk badge、客户端危险正则和 `Don't ask again` checkbox；独立红测锁定后，pending approval 已改为 Codex 的单一中性 request surface，以工具身份、Runtime reason、调用 material 和底部 scoped actions 形成唯一层级。
- P122 不改变 Runtime approval wire、HITL identity、resume transaction 或 Frontend published SDK；`reason`、`risk`、`rememberable`、exact Run/Item 与 edited args 继续来自现有权威 material。Frontend 不再从命令文本推测危险事实，remember scope 只随用户明确选择的 approve action 发送，不附着到 decline。
- P121 已于 2026-08-20 完成：在 P120 基线上继续按 Codex 级桌面外观与既有编码约束打磨左中右三栏，优先修复中间内容区真实渲染与交互接线；每轮编码独立提交并推送，及时关闭 agent-browser、临时服务和测试资源，不修改或暂存 `app/cli`。
- 首个 production 反例已闭环：Runtime 唯一内建文件修改工具 `apply_patch` 发布调用级 `changes:[{path,status,from?}]`；Frontend 现在由既有 Agent fold 持久携带该结果，再由共享 strict parser 同时供 inline preview 与 Run Summary 消费。旧的全工作区 diff 回落和 Runtime 不存在的 `edit`/`write` 内部注册已删除，没有新增 Runtime 读、第二 diff owner 或伪造行数。
- P121 首批不突破 Runtime Protocol、Artifact、SQLite schema、公共 Go API、Agent Framework baseline 或 Frontend published SDK。公开 `ToolCall` 历史字段的 breaking cleanup、commentary/final phase 与原生图片 Download 继续遵守先报告爆炸半径、再取得显式授权的边界。
- P120 已于 2026-08-19 获得实施授权：纠正 P119 后续实现中仍停留在“移动旧卡片”的 Plan/Goal 表面，逐像素对齐 Codex composer stack；移除 composer 上方固定分割线、标题栏累计上下文信息与普通回合结束后的统计杂项，并让 composer Context 环接到真实模型窗口占用。中央内容区继续服从 Codex narrative grammar，不增加结算 dashboard。
- P120 验收后的 Goal 视觉反馈已按同一授权纠正：Goal 不是悬浮 quiet row，而是与 composer 同宽、以重叠 1px 接缝相连的 top-tray surface；材质、上圆角、专用 goal-arrow、行高/间距与 Codex 本地化状态文案由各自唯一 owner 持有。Frontend 仍不展示预算、限制、用量或模型等用户已明确排除的信息。
- P120 不修改 Runtime Protocol、Artifact、SQLite schema、公共 Go API 或 Agent Framework baseline。现有 `segment.progress.contextTokens` 已是 Runtime 发布的最新 prompt/context footprint；Frontend 只把该事实与当前 Session 实际 served model 的 `contextWindow` 配对，不以累计 input/cache token、费用或估算值代替。
- 成立缺口已先由独立红测提交 `98f0c45d4` 锁定并推送；根修复只进入 composer overlay、Plan/Goal presentation、Run fold/read model、Context gauge 与普通终态 narrative owner，继续不修改或暂存 `app/cli`。
- P119 已于 2026-08-18 获得实施授权：在 P118 基线上继续逐项证明 Desktop 左侧 Work Index、中间 Agent Narrative 与右侧 Context Dock 的全部可见能力确实接到权威 Frontend/Runtime owner，并深度对齐 Codex 的交互反馈与信息层级；Plan、Goal、Steer 的前后端语义、生命周期、恢复与 UI 是本阶段首要纵切。
- P119 同时处理 production composer 的 IME 缺陷：中文输入法处于或刚结束 composition、用户实际输入英文并按 Enter 时不得直接发送；修复必须落在 composer 键盘意图 owner，而不是靠延时、debounce、平台特判或取消一次已发出的命令掩盖。
- 默认改动范围为 `app/desktop` 与 `app/desktop/frontend`。只有红例证明前端缺少权威事实时才进入 `app/runtime`；突破 Runtime Protocol、Artifact、SQLite schema、公共 Go API、Agent Framework baseline 或 Frontend published SDK 前必须重新报告爆炸半径并取得确认。
- 第一批只从 production-equivalent Desktop 的盲测建立 Plan/Goal/Steer、IME 与左中右三栏的接线、交互、空态/加载态/错误态、键盘、窄宽、Retina、light/dark 证据矩阵；问题不能形成可复现用户动作与可见错误时，不以视觉偏好或 Codex 私有实现形状修改生产 owner。
- 每个成立问题先形成独立红测提交并推送，再由唯一 presentation/application/domain owner 根修复并独立推送；禁止第二 read-model writer、全局 generation、server registry、transport matrix、刷新旁路、兼容层、离线队列、timer/debounce 竞态掩盖或通用 Owner/Coordinator/状态机。
- Codex 前端与 `codex-rs` 后端只用于提取 Plan/Goal/Steer 的页面心智、命令能力、safe-boundary 和恢复机制；拒绝其多 connection、remote/projectless、浏览器 panel、Electron webview handoff 与私有状态分支。P119 不修改或暂存 `app/cli`，并保留所有无关工作区改动。

### P134 准入与完成条件（2026-08-21）

- 必须从当前 production Work Index 的默认宽度、pointer/keyboard resize、window clamp 与首帧持久偏好形成红例，并由 Codex 当前 Desktop source owner 校准；不得把 Context Dock 的 reading floor、fixture literal、CSS fallback 或用户截图宽度当成第二几何事实。
- Work Index 只能由共享 shell geometry 输出 275px default、240px floor、520px ceiling 与 240px live reading remainder；CSS、production shell、fixture、ARIA range 和 commit path 必须消费同一结果。composer 尾部净空必须在 fractional layout 下仍保留完整 1rem，不能以测试容差掩盖 1px overlay。
- 验收结论：红例 `bbe20ecac`、根修复 `66f5e6231`、live Dock clamp 测试 `6e7498324`、Work Index golden `8e6831be9`、composer 净空修复 `41bc002de` 与尾部 golden `67b9707c0` 已推送。Work Index 默认/键盘/窗口夹取测试、long-content 五次定向重复、82 张三栏几何传播 golden 与 10 张尾部净空 golden 已核对；Frontend 324 files / 2019 tests 与完整静态/构建门禁、agent/shell/workspace/closure/foundation/WebKit 320 项矩阵、Runtime/Desktop test/vet/build 和 `wails3 build` 全绿。fresh SQLite epoch 76 的 production Wails binary 已完成单 listener、同源 health/info `instanceId` 与真实 Frontend OPTIONS/POST `/v2/rpc` smoke。后端合同与 `app/cli` 未修改或暂存。

### P133 准入与完成条件（2026-08-20）

- 必须从当前 production modal 形成可复现视觉红例，并以 Codex 当前产品/source owner 校准；用户提供的历史错误截图中的常驻 answered Question/Goal 表面不得作为兼容目标，也不得借机改变已经对齐的 Goal editor 几何、动作或 Runtime lifecycle。
- modal scope 只能有一个 scheme-independent presentation token；dialog surface 自己拥有边框、阴影与材质深度。不得增加 Goal 专用 backdrop、第二 theme writer、refresh 旁路、定时器或交互状态。
- 验收结论：红例 `f10d63d33`、根修复 `670510280` 与 golden 收口 `ab1688654` 已推送；light/dark computed style 均精确为 `rgba(0, 0, 0, 0.133)`，Goal editor 与 Markdown table preview 四张 golden 已更新并人工核对。Frontend 324 files / 2019 tests、全门禁与 320 项 visual 矩阵、Runtime/Desktop test/vet/build、`wails3 build` 与 fresh epoch 76 production Frontend↔Runtime RPC smoke 全绿；本批未修改或暂存 `app/cli`。

### P132 准入与完成条件（2026-08-20）

- 必须从当前 production viewer 复现“图片可查看但没有 Codex Download”的产品缺口，并说明 Desktop 原生保存 owner；不得复用会话导出的浏览器下载、向 Frontend 暴露任意路径写入或复制 Codex 的 HTTP/cloud 下载分支。
- 唯一新增 IPC 只能接收 restricted inline image material；校验、解码、建议文件名、原生 save sheet、取消与最终写入必须形成一条可测试纵切。Viewer 只拥有一次 pending action 反馈，不持久化、不建立第二文件状态。
- 验收结论：红例 `2bbc84cbe` 与根修复 `b4f69c6a0` 已推送；Frontend 324 files / 2019 tests 和完整门禁、320 项 visual 矩阵、Desktop test/vet/build、Wails v3 production build 与 binary launch smoke 全绿。两张 light/dark lightbox golden 已更新并人工核对，Download/Close/zoom target 均不小于 40px。本批未修改或暂存 `app/cli`。

### P131 准入与完成条件（2026-08-20）

- 视觉依据只允许来自 Codex 真实产品/本地解包 owner 与 production 可复现现状；不得把用户附件中的旧 Plan 卡、Goal 大段内容、常驻 answered card 或仅移动位置后的旧 chrome 当成目标。
- settled Question 必须默认折叠为一行 activity summary；Plan 必须使用固定 32px composer-owned 槽和 hover tooltip；Goal 必须维持 attached tray 且不显示限制条件。三者继续消费既有 transcript、Session Plan 与 Goal snapshot，不建立第二 read model 或 optimistic state。
- 中央 prose 采用 Codex 的 `font-size + 8px` 节奏；所有因此成立的 overflow、18px 字号、键盘或 WCAG 反例必须在真实 owner 修复，不放宽视觉/无障碍阈值。
- 验收结论：Frontend 324 files / 2016 tests 与完整静态/构建门禁全绿；320 项 visual 矩阵全绿，并新增 Plan tooltip、Question collapsed/expanded golden。production page 验证空态、显式项目菜单、精确 cwd draft 与可再次更改项目的标题入口；Wails v3 dev host 完成 frontend/Go build、开发 `.app` codesign、frontend connection 与 1411×881 onscreen 原生窗口启动。自动化会话、Wails/Vite/Runtime 进程和临时数据已清理，本批未修改或暂存 `app/cli`。

### P130 准入与完成条件（2026-08-20）

- 新建会话不得以 home、默认 workspace 或旧 Session cwd 作为隐式目标；所有入口必须先得到用户明确选择的 exact project/cwd，取消时不创建 Session、不切换 selection、不吞 welcome draft。
- project row、顶层 New 与 welcome composer 共享唯一 Session create/navigation owner；不得恢复 pending first-message handoff、空 identity 后聚焦或第二 draft cache。
- 验收结论：红例提交 `efd696dd7` 与根修复 `93ebd28a5` 已推送；相关 session/sidebar/welcome 测试、8 locale、Frontend 全门禁均通过。本批未修改 Runtime contract、SQLite、Desktop/Wails 或 `app/cli`。

### P129 准入与完成条件（2026-08-20）

- 必须从真实 product page 和 SQLite 同时证明 Goal 内部控制提示可被模型持续消费但不进入用户 Narrative；不得依赖 Frontend 文案过滤、隐藏 CSS、特殊 message ID 或重载后的客户端修复。
- model-only 只允许用于 Application 生成的 fresh autonomous Goal opening。普通用户 fresh start 和 resume input 仍创建可见 Transcript Item；opening transaction 必须允许 Conversation-only projection 原子提交，同时保持空 opening、孤立 transcript 或孤立 provider write 的校验拒绝。
- Runtime restart 后 Goal lifecycle/objective 与 Conversation 控制上下文必须恢复，而 Transcript/DOM 泄漏仍为 0。Start result 对内部 model-only opening 不返回伪 `UserItemID`；公共 `runs.start` wire、Protocol、Artifact、SQLite schema 与 Frontend published contract 保持不变。
- 验收结论：红例与全量 Runtime package 回归通过；Frontend 323 files / 2009 tests 与全门禁、Runtime/Desktop test/vet/build、generator diff、Wails v3 production build 全绿。fresh SQLite 记录 26 条 model-only Conversation message、0 条 Transcript 控制提示、1 条真实用户 Item；Runtime restart 后 Goal paused tray 与零泄漏同时恢复。本批未修改或暂存 `app/cli`。

### P128 准入与完成条件（2026-08-20）

- 必须先以 production page 证明旧实现把过程工作和最终回答合成同一可见 turn，再以 Codex 后端/前端 owner 证明 phase 的来源；不得把“最后一条消息”“最后一段文本”或 stream closure 当成 final answer，也不得以 CSS 拆块冒充协议语义。
- Runtime 在 terminal AgentMessage 上必须提供 `commentary | finalAnswer`；running shell 不得伪造 phase，任何终止边界关闭的 partial stream 必须持久为 commentary。Domain、SQLite、Artifact、Protocol、生成物与 Frontend published model 必须同批前移，旧 Artifact 不猜默认值。
- Frontend 必须让 provisional stream、completed frame、replay 与 mixed hydration 收敛到同一 identity：过程 material 保留在 work narrative，final answer 使用独立稳定 row；只有 final answer 拥有 context menu/message actions，前一同 Run commentary 可由紧邻 final answer 触发 wave folding。
- 验收结论：Frontend 323 files / 2009 tests 与 type/lint/format/knip/circular/context/published-boundary/layer/port/API/style/design/token/chrome/locales/bootstrap/bundle 全门禁通过；完整 320 项 visual/WCAG/keyboard/coarse-pointer/IME/CJK/Retina/WebKit 矩阵覆盖过程折叠、独立最终回答与唯一 actions。Runtime/Desktop `go test ./...`、`go vet ./...`、`go build ./...`、generator diff 全绿；fresh Runtime 的 SQLite epoch 76、真实 HITL/tool/final answer、reload 后 phase/DOM 分层、Wails v3 strict codesign 与 1439×899 原生窗口均已验证。本批未修改或暂存 `app/cli`。

### P127 准入与完成条件（2026-08-20）

- 历史错误截图不得作为设计目标；Goal start 必须是 Codex composer submit mode，而不是独立 dialog、popover、attached form 或移动后的旧 banner。非激活时零占位，激活时只显示紧凑 mode identity；限制条件、轮次、花费、步数、预算、用量与 model 不进入前端 Goal 展示。
- Composer 必须继续是 draft、附件、IME、Enter/send、历史与成功清空的唯一 owner；Goal extension 只持有 mode-specific validation、replacement confirmation 与 Runtime start transaction。Runtime 成功前保留输入，失败或 identity 变化不吞 draft，成功后只清除仍等于 exact submitted objective 的文本。
- standing projection 早于 mutation response 属于合法事件顺序；`starting` owner 不得被 projection observer 提前退休，最终由 exact submit transaction 接受或拒绝同一 draft。不得用延时、乐观 Goal cache、二次 clear、刷新或重复 composer state 掩盖竞态。
- 验收结论：Frontend 323 files / 2005 tests 与 type/lint/format/knip/circular/context/published-boundary/layer/port/API/style/design/token/chrome/locales/bootstrap/bundle 全门禁通过；完整 319 项 visual/WCAG/keyboard/coarse-pointer/IME/CJK/Retina/WebKit 矩阵覆盖 Goal mode light/dark、零 duplicate fields、内容区无 constraints 与早到 projection。fresh Runtime/production page 验证真实 create/pause/clear、成功提交后 composer 清空、`spinbutton=0`；Wails v3 原生 smoke 通过 frontend/Go build、strict codesign、frontend connection 与 1439×899 window。本批未修改或暂存 `app/cli`。

### P126 准入与完成条件（2026-08-20）

- 两张历史附件明确属于错误反馈，不作为目标截图；可见语法只从 Codex 本地解包 Goal/Plan 源码和真实产品行为提取。Goal standing UI 只呈现 lifecycle/objective，使用 attached tray、整段摘要入口和 clear/lifecycle/edit 顺序；editor 固定 420px compact surface、20px inset 与 12 行输入区，不展示限制条件、预算、用量或模型。
- `goals.update` 与 `goals.clear` 必须是完整 Runtime vertical slice。update 先 quiesce exact owned drive，以 fresh incarnation/CAS 保存，再仅为原 active lifecycle 恢复 drive；clear 先静止 owned drive 再 CAS 删除，absence 幂等成功，foreign owner/complete objective 保持领域拒绝。新增 operation 为 additive contract，不提升 Protocol、Artifact、SQLite epoch 或公共 durable shape。
- 红测提交 `0d65892e3`、Runtime 根修复 `676ac3b19` 与 Frontend/Codex surface 根修复 `14d556133` 已逐轮提交推送；Frontend 323 files / 2010 tests、89/89 operation families、完整 type/lint/format/knip/circular/context/layer/API/style/design/token/chrome/locales/bootstrap/bundle 门禁与 317 项 visual/WCAG/keyboard/coarse-pointer/IME/CJK/Retina/WebKit 矩阵全绿，Runtime `go test ./...`、`go vet ./...`、`go build ./...` 全绿。
- fresh `LYRA_HOME` 的真实 HTTP 与 production page smoke 已完成 Goal create、paused projection、update、受预算领域拒绝的 resume、clear 及 `goals.get=null` 收敛；Wails v3 dev host 完成前端构建、Go 构建、开发 `.app` codesign 与 1439×899 原生窗口启动。agent-browser、Wails/Vite/Runtime 进程和临时数据库均已清理，本批未修改或暂存 `app/cli`。

### P125 准入与完成条件（2026-08-20）

- 设置主色点击必须在 production plugin topology 下完成同一条权威链：preference mutation、custom theme contribution replacement、document paint、React selected state 与持久化；任一 listener 抛错都不能把一次已接受点击留在“像没点”的半结算状态。
- 单键动态 extension contribution 必须由插件持有 exact disposable，更新时显式 replace，cleanup/HMR 时退订并退休；不得靠重复 `contribute`、刷新、捕获并忽略 duplicate error 或第二 theme cache 掩盖 lifecycle 违约。
- preset 与 custom color 使用同一可见选择语法：足够的桌面命中区、单一 hover、单一 press、selected ring 与 check mark；`aria-pressed`、CSS accent 和 localStorage 均只读既有 appearance preference owner，reload 后恢复相同选择。
- 独立红测提交 `87c8f110b` 先在 production composition 下锁定 duplicate contribution 与点击无反馈；根修复提交 `5e414cc64`、golden 收口 `b72a91336` 和 token-ladder 收口 `c4ad31c02` 只进入 Frontend custom-theme lifecycle、AccentSection 与 production visual fixture。验收结论：Frontend 323 files / 2002 tests 与 type/lint/format/knip/circular/context/layer/API/style/design/token/chrome/locales/bootstrap/bundle 全门禁通过；完整 agent/shell/workspace/closure/foundation/WebKit visual 315 tests 覆盖点击后 `aria-pressed`、真实 CSS accent、持久化、reload 恢复、selected mark、light/dark、WCAG、键盘、IME、CJK 与 Retina。本批未修改 Runtime、Protocol shape、Artifact、SQLite schema、公共 Go API、Frontend published SDK、Desktop/Wails 或 `app/cli`，因此没有机械重跑无关后端/race 矩阵。

### P124 准入与完成条件（2026-08-20）

- 所有普通 ToolCall，无论 read/write、running/completed/error/denied，都必须使用 Codex 的透明 activity row；`card`/`flagged` 只允许表达 delegated Run 等拥有独立内容层级和生命周期的复合边界，不能按调用状态自动升级材质。
- row 的单一阅读顺序为 identity mark、summary、真实 accessory、末尾 disclosure chevron；closed chevron 默认不可见，只在 hover/focus/open 暴露。展开体从 transcript reading edge 开始，由内部 shell、patch 或 reasoning material 自己拥有 inset，不能继承旧卡片的统一左缩进。
- error、denied 和非零 exit code 仍保留可读文本与 exact verdict，但必须使用 secondary ink；不渲染红色 detail card、黄色 warning surface、denied badge、完成勾或常驻 action chrome。长路径继续由生产 `FilePath` owner 左侧截断并保留可访问完整 identity。
- 独立红测提交 `e7697bc64` 先锁定普通调用误用 `card`/`flagged` 和实体边框；根修复提交 `a78817b91` 只进入 Frontend activity shell、tool presentation/model/group/disclosure owner，没有新增状态、协议或兼容分支。验收结论：Frontend 322 files / 2001 tests 与 type/lint/format/knip/circular/context/layer/API/design/token/chrome/locales/bootstrap/bundle 全门禁通过；完整 agent/shell/workspace/closure/foundation/WebKit visual 314 tests 覆盖透明调用层、mark/summary/chevron 顺序、hover/focus/open、expanded reading edge、长路径、WCAG、键盘、IME、CJK、light/dark 与 Retina。本批未修改 Runtime、Protocol shape、Artifact、SQLite schema、公共 Go API、Desktop/Wails 或 `app/cli`，因此没有机械重跑无关后端/race 矩阵。

### P123 准入与完成条件（2026-08-20）

- pending Question 必须只占据 composer 输入层：24px 圆角、中性 material、问题 prompt 为唯一标题；不再渲染 `Input needed` 泛标题、header chip、preview sidecar、重复 transcript card 或并存的普通 composer。
- 单选首项预选并以数字/选中标记呈现，option description 与 label 同行；多题沿同一请求表面分页。单选点击自动前进或提交，text/multi 使用 Next，Skip 为当前 field 发送空 inner values；提交期间保持原请求禁用，直到 Runtime transcript 给出权威 settlement。
- Question textarea/custom input 与 Composer 共用同一个 composition key intent：active composition、`compositionend` 后中英混合 commit Enter 和缺失 compositionend 的 plain-input recovery 都不能误触发 Next/submit，也不增加 timeout、UA 分支或第二 draft owner。
- 验收结论：Frontend 322 files / 2001 tests 与 type/lint/format/knip/circular/context/layer/API/design/token/chrome/locales/bootstrap/bundle 全门禁通过；完整 agent/shell/workspace/closure/foundation/WebKit visual 313 tests 覆盖 Question exact settlement、ordered Skip、composer replacement、compact height、WCAG、键盘、IME、CJK、light/dark 与 Retina。Runtime `go test ./...`、`go vet ./...`、`go build ./...` 全绿；production fixture 已实测首项自动前进、Skip 后 exact response 与 composer 恢复。本批未修改 Protocol shape、Artifact、SQLite schema、公共 Go API、Desktop/Wails 或 `app/cli`。

### P122 准入与完成条件（2026-08-20）

- pending Approval 使用一个 Codex request surface：24px 圆角、中性 card material、无 warning border/二次 danger card；header 只表达真实工具身份，Runtime `reason` 是主要请求标题，command/args 是独立 body material，actions 使用 16px inset 与既有 28px composer control rung。
- 数字 risk badge、`Approval required` 泛标题和客户端 `dangerHints(command)` 不进入用户可见层级。Runtime 的真实 reason 保留；没有 Runtime reason 时才使用本地化的工具问题句，不从命令字符串推断权限、可逆性或危险等级。
- `rememberable` 通过 primary approve 旁的 scoped-action menu 暴露 Session/Project/Global 权威能力；Allow once 仍是默认按钮和快捷键行为。remember scope 只随对应 approve 提交，Deny 不携带 scope，pending/disabled 与 exact Run/Item settlement 语义不变。
- 先提交 production component 与 visual 红例；根修复后至少覆盖 Allow once、scoped approve、Deny、edited args、decorative/Runtime unavailable、light/dark、narrow layout、键盘与完整 agent golden。未触及 Runtime/Desktop/Wails 时不重复无意义的进程恢复或 race 矩阵。
- 验收结论：24px request surface、工具身份、reason/command/args 层级、Allow once、Session/Project/Global 菜单、纯 Deny、edited args、键盘默认动作、320px 窄宽、light/dark 与 settled exact identity 均已验证；Frontend 322 files / 1999 tests、完整 312 项 visual/WCAG/keyboard/IME/CJK/light-dark/Retina/WebKit 矩阵和全部静态/构建门禁全绿。本批未修改 Runtime、Desktop/Wails、Frontend published SDK 或 `app/cli`。

### P121 准入与完成条件（2026-08-20）

- `apply_patch` inline preview 只消费该 ToolCall 已持久化的 `PatchResult.changes`；不得以 `workspace.diff`、当前 Git 状态、工具参数或文件内容推测这次调用做了什么。运行中无结果、完成后空结果与 malformed result 各自保持诚实空态。
- 变更列表采用 Codex 的 quiet file-change grammar：逐文件显示真实 created/edited/deleted/moved 状态与可辨路径，移动同时保留来源路径；没有 Runtime 行级 diff 时不画 diff 行、`+0/-0`、内容片段或假 preview。完整 Diff 打开动作仍由既有 workspace view owner 持有。
- Frontend 内部工具目录、分类、图标和 preview registration 与 Runtime 30 个内建工具真值对齐；不存在的 `edit`/`write` 不再拥有内部专用路径。published SDK 字段保持兼容，不在本批删除。
- 首批必须先以独立红测提交锁定调用级结果、真实变更解析和死注册；根修复后运行定向 unit、typecheck、lint、format、knip、设计系统/交互 chrome/locale 门禁，并补 production-equivalent light/dark 内容视觉证据。若未触及 Runtime/Desktop/Wails，不重复无意义 race 或重启矩阵。
- 验收结论：`apply_patch` 展开体按 created/edited/deleted/moved 显示调用级文件回执，移动保留来源路径；无权威行数时 inline preview 与 Run Summary 都不显示 diff 片段或 `+/-`。Frontend 322 files / 1995 tests、完整 312 项 visual/WCAG/keyboard/IME/CJK/light-dark/Retina/WebKit 矩阵及全部静态/构建门禁通过；单独复跑的 Runtime HTTP 断线恢复例在 `--detectAsyncLeaks` 下无泄漏。本批未触及 Runtime、Desktop/Wails 或 `app/cli`。

### P120 准入与完成条件（2026-08-19）

- Composer 内部不再为已注册但返回 `null` 的 standing contribution 留下固定 header edge；Plan 是 composer 上方的紧凑进度提示，Goal 是与 composer 上沿无缝相连的同宽 top-tray surface，空态均为零高度，不以 transcript header、composer 内卡片或永久分割线占位。
- Plan 只显示 Codex 式环形进度与“第 N / M 步”紧凑 pill，完整权威 checklist 只在 hover/focus tooltip 中展开；不保留旧 disclosure card、底部 progress bar 或 click-expanded 第二状态。Goal 只显示 lifecycle、objective 与当前可用 pause/resume 命令；预算、额度、花费、轮次、步数、model 与 last move 不进入 standing UI。
- Context 环严格采用 Codex 计算：`used = min(contextTokens, contextWindow)`，百分比为 `used / contextWindow` 的整数读数；tooltip 同时呈现占比、剩余比例与 `used/window` token。Run 终态只保留最后一次真实 `contextTokens`，activity/step/usage 等瞬态 progress 同步退休；没有 Runtime 读数或模型窗口时不绘制，不用 Session 累计 usage 猜测。
- 标题栏不再注册 Session 累计 token/cost chip；普通 completed/failed Run 不追加“完成、耗时、步数、token、费用”结算行，最终 assistant message 已是完成事实。canceled/limit 等非普通终态仍保留一行 quiet narrative reason，actionable failure 继续由既有 recovery surface 负责。
- 生产交互与视觉证据必须覆盖 Plan tooltip、Goal 无约束信息、Plan→Goal→Composer 几何、Context 真实占用 tooltip、正常终态无结算 footer、light/dark 与完整 agent golden；Frontend unit、type/lint/format/knip/design/token/chrome/locales/bundle 门禁全部保持全绿。
- P120 不扩张 commentary/final phase 或原生图片 Download 的未授权公共边界；中央其余 Markdown/Tool/HITL/compaction renderer 继续使用 P119 已验收的唯一 material owner。

### P119 准入与完成条件（2026-08-18）

- Plan 必须明确区分当前请求的步骤读模型与自治 Goal；Goal 必须在 active/paused/blocked/completing/absent 各阶段只开放权威命令；Steer 必须表达“送入当前 Run 下一安全模型边界”而不是新建 Run、覆盖 draft 或伪装即时消息。三者的 projection、accepted mutation feedback、Session/generation identity 与 Runtime restart 恢复必须闭合。
- Composer 的 Enter/Shift+Enter、中文/日文/韩文 IME、英文 composition、`compositionend` 与相邻 key event 顺序必须由真实 browser/WebKit 反例定义；提交入口只消费同一个可证明的 composing intent，不增加 timeout、平台 UA 分支或第二 draft state。中文输入法提交混合文本后紧邻的普通 `Enter/keyCode=13/isComposing=false` 已由 production Composer→agent bridge 红例闭环：composition lifecycle 保留一次性 commit intent，首个无修饰 Enter 只结束输入法提交，下一次明确 Enter 才发送；缺失 `compositionend` 的 plain-input recovery 同样走这个 owner。
- 左栏继续逐项验证全局动作、Projects、project row、Session row、目录选择、创建/关闭/切换与离线撤权；中栏验证 transcript/streaming/composer/HITL/Plan/Goal/Steer/scroll；右栏验证 Run Summary/Terminal/Diff/File/Timeline/Tool/Settings 的身份、material、命令 capability、空态与恢复态。
- UI 精修继续服从现有 theme/design-system token、单一 edge mechanism、最小 40px hit area、键盘焦点、reduced motion、CJK/长路径/长标题、tabular numerals、容器宽度与 Retina device-pixel 约束；不以额外 chrome、卡片、圆角或动画代替信息层级。
- 完成前运行 Frontend 全门禁与 `--detectAsyncLeaks`、三栏完整 visual/WCAG/keyboard/IME/CJK/light-dark/Retina/WebKit 矩阵、必要的 Runtime/Desktop gates、Wails v3 production package/codesign，以及 fresh database、renderer replacement、Runtime restart/SIGKILL 的真实产品 smoke；关闭本阶段自动化会话与进程，清理临时数据库、Playwright 和 Go build/test cache。
- P119 内容区纵切继续闭环：production Markdown renderer 以同一 message-content root 统一协议图片与 Markdown 图片，图库支持显式关闭、前后按钮、左右键、Escape、100–400% 缩放、切图重置与 40px 命中区；全屏 90% 黑色媒体画布、明暗基线和 nested delegated message 隔离均已验证。表格保留 semantic DOM、rich/plain clipboard 与 Codex 式展开预览，正文固定 14/21px、表头 14/16px、容器零额外块间距，inline/preview 的明暗基线均已验证。Markdown 段落、段落→列表、root list、标题、blockquote、task/nested/RTL list 与 horizontal rule 使用 Codex 精确节奏；task checkbox 是 list grid 的直接子项，正文统一落在第二列。行内代码使用可换行的 cloned neutral well、0.92em 字号、6px 圆角与 `overflow-wrap:anywhere`。代码块改为同材质、14px sans caption、4×8px header、8px source inset、无 360px 人工高度上限，并保留 copy/wrap、HTML fence、SVG、Mermaid、selection copy、citation、图片 grouping 与 inline/fenced bidi isolation。模型 HTML 只接受无属性的 Codex basic inline 集合，`style`、原生 disclosure/layout 与属性注入保持 inert/literal。
- 非 Markdown 内容块同样进入统一 narrative grammar：context compaction 不再画居中双横线，改为 Codex 式左对齐 quiet activity row，始终保留压缩图标，并在 Runtime 提供 summary 时通过同一键盘 disclosure 展开；未引入第二内容模型、全局 registry、remote fetch 或 Runtime 依赖。
- 上述内容纵切与 IME 的每个成立缺口均先以独立红测推送，再提交根修复；Frontend 全量与严格 `--detectAsyncLeaks` 均为 321 files / 1996 tests，typecheck/lint/format/knip/circular/context/API/locales/style/design-system/token/chrome/bundle 门禁全绿。用户消息使用 Codex 的 5% text neutral bubble、12×8px inset 与 16px 圆角，不再借 accent wash 强调；Markdown task marker 从同一可见正文获得语言无关的 accessible name；长代码 clipping 守卫只认可真实、位于边界内且拥有水平滚动范围的 descendant scroller，不再把停靠在视口外的 Dock material 当作正文滚动能力。agent/shell/workspace/closure/foundation/WebKit 完整 311 项 visual/WCAG/keyboard/IME/CJK/light-dark/Retina 矩阵全绿，中文输入法中英混合提交继续证明首个 Enter 只归输入法、下一次 Enter 才发送。KaTeX CSS 已从错误的 startup ownership 分离到既有动态 loader，启动 CSS 从 136.1KB 降至 107.0KB，并由“产物存在但 `index.html` 不得引用”的门禁锁定。
- P119 最终 recovery smoke 使用 fresh HOME/SQLite、0600 durable local token 与 production Wails `.app`：renderer reload 前后 Desktop PID 保持 90579，后继 renderer 重新执行完整 Runtime inspection/query，SQLite 与权威 `sessions.list` 均保持唯一 `P119 中英混合 recovery smoke` Session。Runtime PID 89768 被精确 `SIGKILL` 后由 PID 93411 接替且 `instanceId` 换代，token digest 不变；同一 Desktop/renderer 在锁屏后台、没有 reload 或手工刷新时自动连接后继实例并恢复 RPC。Runtime standalone 与 Desktop 全量 test/vet/build、Wails production package 和 strict codesign verification 全绿；本阶段自动化进程、临时数据库、Playwright 与 Go build/test cache 已清理。
- P128 已补齐 Codex commentary 与 final answer 的权威 `agentMessage.phase`：Runtime completion boundary、Transcript persistence、SQLite codec、Artifact v20、公共 Go/生成 TypeScript surface、Frontend published SDK 与 fold/render planner 同步前移。Frontend 仍不按顺序、流式状态或文本形态猜 phase，也没有增加第二 transcript writer。
- Codex 图片查看器的 Download 仍缺 Desktop 原生 save-file owner；直接使用浏览器下载会伪造桌面接线。补齐该能力需要新增 Desktop binding/生成 surface，取得显式边界授权前只呈现已接线的查看、导航与缩放能力。

### P118 准入与完成条件（2026-08-18）

- 每个生产修复先形成能够失败的真实产品反例和独立红测提交，再由唯一 owner 完成根因修复提交；批次均须精确暂存并推送。
- 左栏必须把全局动作、Projects 目录入口、project row 与 Session row 的范围、禁用、焦点和创建/关闭/切换结果表达清楚；中栏必须让权威 Run/Item/Interrupt/Goal/Plan 在长对话、流式、历史加载和 continuation 中稳定呈现；右栏必须让 tab 身份、内容 material、命令 capability、空态和恢复态一致。
- UI 精修继续服从现有 theme/design-system token、单一 edge mechanism、最小 40px 可点击区域、键盘焦点、reduced motion、CJK/长路径/长标题、tabular numerals、容器宽度与 Retina device-pixel 约束；不以额外 chrome、卡片、圆角或动画代替信息层级。
- 完成前运行 Frontend 全门禁与 `--detectAsyncLeaks`、三栏完整 visual/WCAG/light-dark/Retina 矩阵、必要的 Runtime/Desktop gates、Wails v3 production package，以及 fresh database、renderer replacement、Runtime restart/SIGKILL 的真实产品 smoke；自动化会话、daemon、临时数据库和测试进程必须清理。
- 左栏首个 production 反例已锁定：project-row `+` 的 `sessions.create` 被 command owner 拒绝并返回空 identity 后，`useWorkIndexActions` 仍无条件聚焦 composer，焦点落回旧 Session 并制造创建成功的假反馈。唯一修复 owner 是该 action 的异步完成语义；现在仅在返回新 Session identity 后聚焦，与顶层 New 和目录选择入口一致，不增加 optimistic navigation、toast 旁路或第二 Session writer。
- 中栏紧凑窗口反例已闭环：在 1280×577 首次打开 waiting/question 时，Approve/Submit 的底边分别落到 composer 内 53px/89px。`use-stick-to-bottom` 只观察 content box，而 ChatStream 的实测 composer 净空通过 transcript padding 改变 border box；最终净空没有进入 follow lifecycle，初始视口停在尾部之外。`MessageStream` 仍是唯一滚动 owner，现在以同一个 follow fact 同时消费 subtree mutation 与 border-box ResizeObserver，并写库自己的精确 target，用户逃逸后保持 reader-owned 位置；未增加 timer、第二 scroll state 或 HITL 专用滚动旁路。紧凑 waiting/question、既有 tail clearance 与 reader escape 共 6 项 browser 反例及定向 unit/typecheck 全绿。
- 右栏窄窗反例已闭环：1120px window 保持 256px Work Index 时，旧 split 把 conversation 与 Dock 各压到约 432px，违反 640px reading floor，HITL、composer 与正文一起降成不可读窄栏。采用 Codex 的“空间不足时折叠 panel、保留 panel membership”机制：`ChatPanel` 只从 row ResizeObserver 推导 presentation capability，低于 640px conversation + 420px Dock 时调用既有 navigation owner 清除 destination，`contextDockStore` 的 tab set/last view/width preference 不变；toggle 同步禁用并解释需增大窗口。恢复安全宽度后由用户重开原 tab，宽窗 resizer 也以 640px conversation floor 计算上限；没有第二 visibility state、覆盖 drawer preference 或改写持久宽度。
- 右栏 File material identity 反例已闭环：query/store 已持有 `app/…/DockResizer.tsx`，但 dock placement 的通用 `ViewHeader` 无条件丢弃动态 title，只显示“8 lines”，用户无法确认正在阅读哪个文件。`ViewHeader` 现在区分 generic tab name 与可选 dock material identity；File view 将同一 `viewer.path` 通过既有 `FilePath` 左侧截断呈现为“exact path · line count”，full placement 仍使用原 title。未把路径复制进 tab、内容猜测或增加第二 file selection。
- 中栏命令语义反例已闭环：同一 running fixture 同时呈现 Goal lifecycle 与当前 Run composer，但两个按钮都暴露为“Stop”，键盘与读屏用户无法判断将停止自治目标还是当前回合。Goal banner 继续直接调用既有 `stopGoal`/`resumeGoal` owner，只把八种语言的可见动作明确为“Stop goal/Resume goal”及等义译文；composer 的 Run “Stop”保持不变，同页现在只有一个 exact “Stop”，没有新增 aria-only 别名、command route 或生命周期状态。
- P118 最终 recovery smoke 使用 fresh HOME/SQLite、0600 local token 与 production Wails `.app`：renderer reload 前后 Desktop PID 保持 36610，WebArea generation 完整替换，exact Session 数保持 1，composer、`explorer` Dock 与资源树均由权威状态恢复。首次 Runtime SIGKILL 后 PID 36578→38590、`instanceId` 换代，离线期所有 Session 创建入口同步撤权而 Dock 仍可读；再次在窗口保持前台时 SIGKILL 38590→39832，原窗口无需 reload 即清除告警并恢复命令能力。Frontend 普通与 `--detectAsyncLeaks` 均为 313 files / 1946 tests，agent/shell/workspace/closure/foundation/WebKit visual 287 tests 全绿；Runtime 与 Desktop 全量 test/vet/build、Wails production package 和 strict codesign verification 全绿，自动化进程与临时数据库已清理。

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
| P118     | Desktop 左中右接线与 Codex 交互深度对齐                                                                         | 创建失败不转移焦点，compact HITL 动态净空进入唯一滚动 owner，Goal/Run 命令可辨；Context Dock 服从 640px 阅读下限并呈现 exact file identity，真实 renderer/Runtime 换代后状态与能力收敛    |
| P119     | Desktop 前后端完整接线与 Codex 三栏深度对齐                                                                     | Plan/Goal/Steer、Work Index、统一 narrative renderer 与 Context Dock 服从权威 Session/Runtime owner；IME commit Enter、production renderer replacement 与 Runtime SIGKILL 恢复闭环        |
| P120     | Composer stack、Context 占用与终态 narrative 精确收敛                                                           | Plan/Goal 改为 Codex 紧凑 standing surface；Context 环消费真实 `contextTokens/contextWindow`；标题栏与普通结束态移除累计统计和结算杂项                                                    |
| P121     | 调用级补丁回执与 Codex file-change narrative                                                                    | `apply_patch` 的持久结果成为 inline preview 与 Run Summary 唯一变更事实；删除 `edit`/`write` 死路径和全工作区 diff 猜测，无权威行数时不伪造增删统计                                       |
| P122     | Codex approval request surface 与 scoped allow 语义                                                             | pending approval 收敛为单一中性请求面；可见层只读工具身份、Runtime reason 与调用 material，scope 只随明确 scoped approve 提交，Deny 保持纯拒绝                                            |
| P123     | Codex Question composer request 与 ordered skip 语义                                                            | pending Question 成为 composer rung 的唯一请求表面；选项、分页、IME 与 Skip 服从同一 draft/action owner，Runtime 以有序空 inner values 持久表达明确跳过                                   |
| P124     | Codex tool work narrative 与 disclosure 层级                                                                    | 普通 ToolCall 统一回到透明 activity row；identity、summary、末尾按需 chevron 与展开 material 形成一个阅读流，failure/denied 不再升级成彩色 dashboard chrome                               |
| P125     | 设置主色选择反馈与动态 theme contribution 生命周期                                                              | preference、theme replacement、document paint、selected state 与持久化恢复同链结算；动态单键 contribution 先退休旧 owner，再发布新值，点击不再被 duplicate exception 中断                 |
| P126     | Goal 权威 update/clear 与 Codex composer tray 管理                                                              | attached tray、摘要入口与 clear/lifecycle/edit 服从 Codex 层级；update/clear 进入 Runtime 权威纵切，Frontend 不展示限制条件或建立第二 Goal writer                                         |
| P127     | Goal Codex composer submit mode 与提交所有权                                                                      | Goal start 复用唯一 Composer draft/IME/send owner；standing projection 早到与 Runtime mutation settlement 由 exact commit owner 结算，不增加第二表单或 limits chrome                    |
| P128     | Agent commentary / final answer 权威分层                                                                          | Runtime terminal phase 贯通 Transcript、SQLite、Artifact 与公共 surface；Frontend 将过程叙事和最终回答分行，只有 final answer 拥有 actions，live/replay/mixed hydration 收敛               |
| P129     | Goal 模型控制输入与用户转录隔离                                                                                   | fresh autonomous opening 将 Application 控制提示只写入 provider Conversation，不创建 Transcript Item；真实用户 start/resume 仍可见，重启后 Goal 上下文与零泄漏同时恢复                 |
| P130     | 新会话显式 exact-cwd                                                                                              | 顶层 New、project row 与 welcome draft 先由用户选择既有项目或目录；取消不创建隐式 home Session、不切换旧 selection、不吞 draft                                                          |
| P131     | Narrative / Plan / Goal / typography 像素收口                                                                     | settled Question 默认折叠；Plan 固定 32px composer 槽；Goal 保持 attached tray 且不显示限制；正文与 streaming reasoning 服从 Codex 节奏和可访问滚动                                    |
| P132     | 图片查看器原生保存                                                                                                | Download/Close 工具组与 action feedback 对齐 Codex；restricted inline image 只经 `DesktopHost.SaveImage`、exact-window native save sheet 和最终文件写入完成，不使用浏览器 fallback        |
| P133     | Codex modal scope 层级收口                                                                                         | 全部 dialog 共用 scheme-independent `#00000022` scrim，surface 自己承担深度；Goal editor 与 table preview 不再在 dark mode 被过重遮罩压暗                                               |
| P134     | Codex Work Index 几何与尾部净空                                                                                    | Work Index 统一为 275px default、240–520px resize，并始终给 reading plane 留 240px；首帧、fixture、ARIA 与持久偏好共用 owner，composer tail 不再因 fractional scroll 舍入少 1px          |

## 5. 当前里程碑结论

P113–P134 共同建立了以下不可回退的心智模型：

- 产品始终只有一个 Desktop actor 和一个逻辑 Runtime。renderer、Plugin Host、Runtime process、connection、command、query writer 和 mounted material 仅在真实可替换边界拥有局部 generation。
- Runtime 每次进程实例发布新的 opaque `instanceId`；同 endpoint 重启只替换进程内资源，不替换逻辑 Runtime、SQLite durable identity 或 mutation store identity。
- 进程内 owner replacement 先发布新实例，再同步退休旧实例。只有异步间隙可能发生 replacement，且后续会修改当前共享状态时，提交和 cleanup 才需要 exact owner proof。
- `sessions.snapshot` 是挂载 Session 的原子 material owner；HITL、Plan、Goal、Run、Tool 不能再由独立 query/event/material 多路拼接。
- Conversation 是 provider 产品上下文，Transcript 是用户可见事实，两者不要求逐条镜像；只有 Application 生成的 fresh Goal 控制输入可走 model-only opening，Frontend 不负责识别或隐藏内部提示。
- 图片查看器是 Frontend presentation 与 Desktop packaged capability 的纵切：gallery 只提交当前已允许渲染的 inline material，`DesktopHost` 负责校验/解码，Wails save sheet 和文件写入属于 exact window owner；Runtime、浏览器下载与任意路径 API 均不参与。
- modal interaction scope 只有一个全局 presentation owner：light/dark 都使用 `#00000022` scrim，dialog surface 自己拥有边框、材质和阴影深度；Goal、table preview 等消费者不能建立局部 backdrop 或 scheme 分叉。
- Goal objective update 先 quiesce exact owned drive，再以 fresh incarnation/CAS 替换 durable Goal；只有原 lifecycle 为 active 才按冻结事实恢复 drive。Goal clear 在 owned drive 静止后 CAS 删除，absence 幂等成功，Frontend 只能通过同一个 per-Session Goal command owner 发起并等待权威 snapshot 收敛。
- durable mutation 以事务 marker/identity 判断“已提交但成功回执丢失”；不得靠重试猜测或本地 optimistic 状态冒充服务端事实。
- Run Summary、Terminal、Diff、Tool selection、Goal、Plan、审批、Session/Dock navigation 只消费所属 Session 与 generation 的权威投影。
- Desktop 冷启动依赖在 composition root 显式声明；Composer、Recipes 和 Workspace Events 的 session ports 不再依赖偶然安装顺序。
- local transport token 由 durable data path 拥有，不属于 Runtime process generation；`instanceId` 换代不撤销仍存活 Desktop 的认证能力。
- 流式输出期间，消息底部反馈/操作区服从可见 turn 的稳定 material 边界，不能跟随每个 delta 反复挂载造成闪烁。
- AgentMessage 的 commentary/final answer 是 Runtime 在 terminal boundary 写入并持久化的事实。running shell 可暂居 work narrative，但 terminal final answer 必须移入稳定独立 row；过程行、canceled/waiting 叙事不拥有 message actions，同 Run 紧邻 final answer 只触发过程 wave folding，不改写历史 Item 顺序。
- Work Index 只有在 `sessions.create` 返回新 identity 后才交接焦点；中央 transcript 的内容增长与 composer/HITL border-box 净空共享一个 follow fact，用户取得阅读位置后不再被异步 materialization 抢回。
- Work Index 的 geometry 只有一个 owner：275px default、240px floor、520px ceiling，live clamp 始终给 reading plane 留 240px；首帧持久偏好、pointer/keyboard、ARIA 与 visual fixture 都消费同一事实。Context Dock 的 640px conversation floor 是另一条独立 flank 约束。
- Composer 的发送入口只消费键盘意图 owner：IME 提交中英混合文本后的首个普通 Enter 仍属于 composition commit，只有下一次独立 Enter 才能发送；不得用 timeout、UA 分支或撤销已发送命令模拟该边界。
- Context Dock 只有在 conversation 仍保有 640px 阅读宽度时展开；空间不足时只折叠 URL destination，并保留 Session-owned tab membership、last view 与宽度偏好。File view 必须同时呈现 exact path 与 material 统计，Goal lifecycle 与当前 Run command 必须具有可辨作用域。
- Composer 顶部 standing stack 只有紧凑 Plan pill 与 Goal lifecycle row；Goal 限制仍是 Runtime/launcher 事实而非永久 chrome。Context 环只读 Runtime `contextTokens` 与 served model window，标题栏与普通完成态不重复累计 accounting。
- 一次 `apply_patch` 的展开体和 Run Summary 只读该 ToolCall 已持久化的 `PatchResult.changes`；当前工作区状态、工具参数和文件内容都不能回填历史调用。Runtime 没有发布行级 diff 或增删行数时，Frontend 不猜测这些事实。
- pending approval 是一个 Codex request surface，而不是风险 dashboard：工具身份、Runtime reason、command/args 是可见事实；客户端不从命令字符串推导危险、可逆性或权限。Allow once 与键盘动作不持久化规则，只有用户选择 Session/Project/Global scoped allow 才提交 remember scope，Deny 不继承 allow scope。
- pending Question 从 mounted transcript material 选择，但只在 composer rung 呈现一次；普通 composer 暂时退休。首项预选、分页、自动前进、Next 与 Skip 共享一个 draft/action owner，空 inner values 只表示该有序 field 被明确跳过，不能省略 field、重排答案或靠客户端乐观回显冒充 Runtime settlement。
- 普通 ToolCall 属于 Agent work narrative，不按运行/失败/拒绝状态切换卡片类型；mark、summary、accessory 与按需 disclosure 共享一行，展开后的 shell/patch/reasoning material 各自拥有 reading-edge inset。颜色只能辅助 exact verdict，不能制造第二套风险或完成层级。
- 动态单键 extension contribution 是可替换的 plugin-owned resource：每次偏好更新先退休 exact previous contribution，再发布新 material；plugin cleanup/HMR 同步释放 subscription 与 contribution。控件反馈、document paint 和持久化必须消费同一 preference mutation，不允许 listener 异常制造半结算。

最近一次完整验收基线：Frontend 324 files / 2019 tests 全绿，98 条 published context edge 无环，89/89 Runtime operation fact families、3/3 sidecars、16/16 events 有产品消费者；agent/shell/workspace/closure/foundation/WebKit visual 320 tests 覆盖 streaming、HITL、Session/Dock、Goal、modal scope、commentary/final answer、图片原生保存入口、WCAG、键盘、coarse pointer、IME、CJK、18px、reduced motion、Retina 与 light/dark golden。Desktop `go test ./...`、`go vet ./...`、`go build ./...` 与 `wails3 build` 全绿；fresh SQLite epoch 76 的 production Runtime 与 Wails binary 已完成单 listener、同源 health/info `instanceId` 和真实 Frontend OPTIONS/POST `/v2/rpc` smoke。Runtime/Protocol/Artifact/SQLite 本批未变，P131 以前的 Runtime SIGKILL 恢复证据仍是不受本批影响的封板基线。

## 6. 新阶段准入

新 Goal 必须先完成以下内容，才可开始生产代码：

1. 用当前产品构建提出一个可复现的红色反例；“代码看起来不舒服”只能触发审计，不能直接触发抽象。
2. 指明唯一状态 owner、生命周期、事务边界和允许的 breaking surface。
3. 对照服务端/前端参考，分别记录采纳机制与拒绝理由。
4. 按改动风险定义一个 Desktop/逻辑 Runtime 的真实恢复矩阵和必要门禁；不要求每个局部批次机械运行 SQLite、Frontend、race、fuzz 与生产 Wails 的全集。
5. 证明没有引入第二 writer、第二执行循环、兼容双读、刷新旁路、timer 掩盖或对 `app/cli` 的改动。
6. 证明没有为多窗口、多服务端、假想 transport 组合或不可达状态引入抽象与防御分支。

候选方向保留在 [`inspiration/`](inspiration/)；它们不是实施授权。P134 已完成，下一阶段必须先形成新的真实产品反例与独立授权。开始下一阶段时只在本文新建简短阶段条目，完成后更新里程碑结论与能力事实，不恢复逐提交流水账。
