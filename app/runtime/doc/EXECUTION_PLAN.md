# Lyra Runtime 执行计划

> 状态：P0–P122 已完成并形成里程碑。
>
> 最近基线：2026-08-20，P122 Codex approval request surface 与 scoped allow 语义闭环。

本文只拥有四类信息：当前授权、长期约束、里程碑索引、下一阶段准入。能力现状由
[`CAPABILITY_LEDGER.md`](CAPABILITY_LEDGER.md) 拥有；稳定合同由
[`CONTRACT_BASELINE.md`](CONTRACT_BASELINE.md) 拥有；架构与实施规则分别由
[`ARCHITECTURE.md`](ARCHITECTURE.md) 和
[`ENGINEERING_STANDARDS.md`](ENGINEERING_STANDARDS.md) 拥有。

P0–P114 的逐批红例、文件清单和门禁原始记录已冻结在 Git 快照
`babec316e:app/runtime/doc/EXECUTION_PLAN.md`。Git 是历史审计源，本文不再复制提交日志。

## 1. 当前授权

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
- Codex 的 commentary 与 final answer 精确分组仍缺权威 `agentMessage.phase` 事实。若补齐会同时影响 Runtime Protocol Item、transcript persistence、SQLite JSON codec、Artifact import/export、公共 Go/生成 TypeScript surface、Frontend published SDK 与 fold/render planner；在取得显式 breaking-surface 授权前，Frontend 不按顺序、流式状态或文本形态猜测 phase，也不增加第二 transcript writer。
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
| P118     | Desktop 左中右接线与 Codex 交互深度对齐                                                                          | 创建失败不转移焦点，compact HITL 动态净空进入唯一滚动 owner，Goal/Run 命令可辨；Context Dock 服从 640px 阅读下限并呈现 exact file identity，真实 renderer/Runtime 换代后状态与能力收敛       |
| P119     | Desktop 前后端完整接线与 Codex 三栏深度对齐                                                                      | Plan/Goal/Steer、Work Index、统一 narrative renderer 与 Context Dock 服从权威 Session/Runtime owner；IME commit Enter、production renderer replacement 与 Runtime SIGKILL 恢复闭环       |
| P120     | Composer stack、Context 占用与终态 narrative 精确收敛                                                            | Plan/Goal 改为 Codex 紧凑 standing surface；Context 环消费真实 `contextTokens/contextWindow`；标题栏与普通结束态移除累计统计和结算杂项                                                  |
| P121     | 调用级补丁回执与 Codex file-change narrative                                                                      | `apply_patch` 的持久结果成为 inline preview 与 Run Summary 唯一变更事实；删除 `edit`/`write` 死路径和全工作区 diff 猜测，无权威行数时不伪造增删统计                                    |
| P122     | Codex approval request surface 与 scoped allow 语义                                                               | pending approval 收敛为单一中性请求面；可见层只读工具身份、Runtime reason 与调用 material，scope 只随明确 scoped approve 提交，Deny 保持纯拒绝                                      |

## 5. 当前里程碑结论

P113–P122 共同建立了以下不可回退的心智模型：

- 产品始终只有一个 Desktop actor 和一个逻辑 Runtime。renderer、Plugin Host、Runtime process、connection、command、query writer 和 mounted material 仅在真实可替换边界拥有局部 generation。
- Runtime 每次进程实例发布新的 opaque `instanceId`；同 endpoint 重启只替换进程内资源，不替换逻辑 Runtime、SQLite durable identity 或 mutation store identity。
- 进程内 owner replacement 先发布新实例，再同步退休旧实例。只有异步间隙可能发生 replacement，且后续会修改当前共享状态时，提交和 cleanup 才需要 exact owner proof。
- `sessions.snapshot` 是挂载 Session 的原子 material owner；HITL、Plan、Goal、Run、Tool 不能再由独立 query/event/material 多路拼接。
- durable mutation 以事务 marker/identity 判断“已提交但成功回执丢失”；不得靠重试猜测或本地 optimistic 状态冒充服务端事实。
- Run Summary、Terminal、Diff、Tool selection、Goal、Plan、审批、Session/Dock navigation 只消费所属 Session 与 generation 的权威投影。
- Desktop 冷启动依赖在 composition root 显式声明；Composer、Recipes 和 Workspace Events 的 session ports 不再依赖偶然安装顺序。
- local transport token 由 durable data path 拥有，不属于 Runtime process generation；`instanceId` 换代不撤销仍存活 Desktop 的认证能力。
- 流式输出期间，消息底部反馈/操作区服从可见 turn 的稳定 material 边界，不能跟随每个 delta 反复挂载造成闪烁。
- Work Index 只有在 `sessions.create` 返回新 identity 后才交接焦点；中央 transcript 的内容增长与 composer/HITL border-box 净空共享一个 follow fact，用户取得阅读位置后不再被异步 materialization 抢回。
- Composer 的发送入口只消费键盘意图 owner：IME 提交中英混合文本后的首个普通 Enter 仍属于 composition commit，只有下一次独立 Enter 才能发送；不得用 timeout、UA 分支或撤销已发送命令模拟该边界。
- Context Dock 只有在 conversation 仍保有 640px 阅读宽度时展开；空间不足时只折叠 URL destination，并保留 Session-owned tab membership、last view 与宽度偏好。File view 必须同时呈现 exact path 与 material 统计，Goal lifecycle 与当前 Run command 必须具有可辨作用域。
- Composer 顶部 standing stack 只有紧凑 Plan pill 与 Goal lifecycle row；Goal 限制仍是 Runtime/launcher 事实而非永久 chrome。Context 环只读 Runtime `contextTokens` 与 served model window，标题栏与普通完成态不重复累计 accounting。
- 一次 `apply_patch` 的展开体和 Run Summary 只读该 ToolCall 已持久化的 `PatchResult.changes`；当前工作区状态、工具参数和文件内容都不能回填历史调用。Runtime 没有发布行级 diff 或增删行数时，Frontend 不猜测这些事实。
- pending approval 是一个 Codex request surface，而不是风险 dashboard：工具身份、Runtime reason、command/args 是可见事实；客户端不从命令字符串推导危险、可逆性或权限。Allow once 与键盘动作不持久化规则，只有用户选择 Session/Project/Global scoped allow 才提交 remember scope，Deny 不继承 allow scope。

最近一次完整验收基线：Frontend 322 files / 1999 tests 全绿，97 条 published context edge 无环，87/87 Runtime operation fact families、3/3 sidecars、16/16 events 有产品消费者；agent/shell/workspace/closure/foundation/WebKit visual 312 tests 覆盖 streaming、HITL、Session/Dock、WCAG、键盘、coarse pointer、IME、CJK、18px、reduced motion、Retina 与 light/dark golden，P122 的 Approval request surface、scoped actions、窄宽和键盘语义已进入该矩阵。Runtime standalone 与 Desktop 全量 test/vet/build、Wails v3 production `.app` package 和 strict codesign verification 沿用 P119 恢复基线；P122 未改动这些层。fresh HOME/SQLite 的真实 smoke 中，renderer reload 与 Runtime 89768→93411 的 SIGKILL 换代均保持 exact Session，Desktop PID 90579 始终不变；原 renderer 在锁屏后台且没有 reload 时自动连接后继实例，durable token 保持 0600 与相同 digest，SQLite Session 数保持 1。

## 6. 新阶段准入

新 Goal 必须先完成以下内容，才可开始生产代码：

1. 用当前产品构建提出一个可复现的红色反例；“代码看起来不舒服”只能触发审计，不能直接触发抽象。
2. 指明唯一状态 owner、生命周期、事务边界和允许的 breaking surface。
3. 对照服务端/前端参考，分别记录采纳机制与拒绝理由。
4. 按改动风险定义一个 Desktop/逻辑 Runtime 的真实恢复矩阵和必要门禁；不要求每个局部批次机械运行 SQLite、Frontend、race、fuzz 与生产 Wails 的全集。
5. 证明没有引入第二 writer、第二执行循环、兼容双读、刷新旁路、timer 掩盖或对 `app/cli` 的改动。
6. 证明没有为多窗口、多服务端、假想 transport 组合或不可达状态引入抽象与防御分支。

候选方向保留在 [`inspiration/`](inspiration/)；它们不是实施授权。P118 已完成，下一阶段必须先形成新的真实产品反例与独立授权。开始下一阶段时只在本文新建简短阶段条目，完成后更新里程碑结论与能力事实，不恢复逐提交流水账。
