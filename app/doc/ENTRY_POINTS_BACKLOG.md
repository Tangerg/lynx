# 功能入口 / 可发现性 Backlog —— app/desktop 前端

> **目的**：功能越堆越多，但用户摸不到——把"已实现却藏起来/失效的入口"和"竞品用来surface大量功能的 IA 模式"落成**可勾选、带源码证据、分优先级**的待办。配套 [`DESKTOP_COMPARISON.md`](./DESKTOP_COMPARISON.md)（比 GUI 形态：引擎位置/插件化/设计系统）/ [`UX_POLISH_BACKLOG.md`](./UX_POLISH_BACKLOG.md)（比"小处打磨"：通知/进度/草稿）——**本篇只比"功能入口在哪、藏没藏、怎么 surface"**（信息架构 / 可发现性）。
>
> **方法**：4 个 source-grounded subagent 第一手核实——1 个自审（盘点 `app/desktop/frontend/src` 全部入口）+ 3 个竞品（**Proma**=最近同类 / **Cherry Studio**=多功能区标杆 / **LobeChat**=可发现性标杆，clone 均在 `~/Desktop/`）。竞品路径相对各自 clone 根；我方路径相对 `app/desktop/frontend/src`。基线 **2026-06-22**。
>
> **一句话元结论**：lynx 的真正同类是 **Proma / Codex / Claude Code（单工作区编码 agent）**，不是 Cherry/LobeChat 那种多模态 AI hub——所以差距**不在缺产品区**，而在 ① **已实现的 7+ 个 view 只有 ⌘K 一条路**（无 nav/无按钮，等于隐形）② **两个假 affordance**（死的 exec-mode chip、假的搜索入口）③ **composer 缺少后端早已支持的能力入口**（权限/计划模式、工具可见、手动 compact）。根因九成是**前端没给入口**，能力都在。

---

## 0. 定位校准（先划清"该不该学"）

把竞品按定位分两类，**避免照着错的对象补**：

- **同类（编码 agent，IA 直接可比）**：**Proma**（Chat+Agent 工作台，Claude Agent SDK，workspace/skills/MCP/scheduler——几乎与 lynx 同功能集）。它的入口选择是 lynx 最好的镜子。
- **异类（多模态 AI hub，只借 IA 机制、不借产品区）**：**Cherry Studio**（11 个功能区 + mini-app）、**LobeChat**（Discover 市场 + 多模态）。它们的 Paintings/Translate/知识库/市场是 hub 定位，**不是 lynx 目标**；但它们的命令面板/统一 + 菜单/可配置侧栏/右侧 inspector 等**可发现性机制**值得借。

> 判据：凡是"加一个新产品区"的，先问"这是 hub 才需要的吗"——是则不追；凡是"让已有能力浮出来 / 更好地组织入口"的，照学。

---

## 1. 速查矩阵（入口 / 可发现性维度）

> ✅ 有且成熟 · 🟡 部分/弱 · ❌ 无 · ★ 标杆

| 维度 | Proma | Cherry Studio | LobeChat | **app/desktop** |
|---|---|---|---|---|
| 导航模型 | 模式切换侧栏 + 极简 tab ★ | 图标 rail-of-apps + 浏览器 tab + Launchpad ★ | portal 式上下文左栏 ✅ | 单侧栏（nav 组 + 项目树）+ tab 🟡 |
| 全局搜索（跨会话/消息） | ✅（+ AI 搜索）★ | ✅ | ✅（CMDK 跨实体）★ | ❌（仅 ⌘F 当前对话 DOM 高亮） |
| 命令面板能力 | ❌（无；Ctrl+Tab MRU 切换） | 🟡（命令注册→右键/⋯菜单；QuickPanel 当 `/`@`） | ✅（上下文感知 + 跨实体搜索 + AI 模式）★ | 🟡（**只索引 command**，无搜索/会话/文件源） |
| composer 权限/计划模式入口 | ✅（Auto/Bypass/Plan）★ | 🟡（Permission Mode 按钮） | 🟡（Agent 模式） | ❌（**死 chip，纯展示**） |
| composer 工具/MCP 可见入口 | ✅ | ✅★ | ✅★（统一 + 菜单） | ❌（只在 Settings） |
| composer mention 词汇 | `@`文件 `/`skill `#`MCP `&`会话 ★ | QuickPanel（模型/命令/资源） | ✅ | 🟡（仅 `@`文件 + `/`slash） |
| 右侧 inspector（agent 在干啥 / 文件 / diff） | ✅（会话/工作区/改动 三 tab）★ | 🟡 | ★（双 inspector：工作区 + 制品/工具栈） | 🟡（split-beside diff/terminal/file，无常驻面板） |
| 市场 / 安装入口 | 🟡（skills 商店占位） | ✅（MCP 市场 + mini-app + 资源库）★ | ★（Discover：agent/mcp/model/provider/skill 统一安装） | ❌（skills/recipes/mcp/tools 是 view，但无浏览/安装面） |
| 全局唤起（tray / 快捷启动 / 全局热键） | ✅（Alt+Space 快任务 + 语音 + tray）★ | ✅（Quick/Selection Assistant + tray）★ | 🟡（web 为主） | ❌ |
| 已实现 view 的可发现性 | 全 surface | 全 surface | 全 surface | ❌（7+ 个仅 ⌘K，见 §3） |

---

## 2. 我们已经不弱的（别再重造）

自审确认这些达第一梯队，**不在本 backlog**：

- **⌘K 命令面板**（`command/command-palette/`，cmdk，模糊匹配 label+desc+group+keywords）——机制在，差的是**源**（见 T3.1），不是面板本身。
- **Codex 式项目→会话树 + 侧栏会话过滤 + per-session 状态点**（`sidebar/projects.tsx`）。
- **统一 tab 条**（chat tab 与 view tab 同条）+ **split-beside view**（chat | resizer | view，可拖拽、比例持久）+ **⌘1–9 切 tab** + tab 右键（关闭/关其他/左/右/全部）。
- **tool-click 路由**（`state/toolRouting.ts`：bash/command 卡 → terminal beside；fileEdit/read 卡 → diff beside）——这是 lynx 已有、competitor 多数没有的"从工具卡进检视面"的好入口。
- **分组设置**（12 面板，general/models/agent/integrations/advanced）+ 首启 keyless setup 卡深链 Providers。
- **per-message / per-session 操作**（复制 MD/Plain/Code、编辑回填、重生成、fork、pin、rename、delete）。
- **审批能力**（全局模式 + 持久细粒度规则，`settings/approvals/`）——比多数竞品强；**但运行时入口缺**（见 T2.1）。
- recipes（`/`slash + $ARGUMENTS 展开）、scheduler、@codebase、MCP 配置 + 每工具 gating、hooks——**功能都在**，部分入口要补（见 §3）。

---

## 3. Tier 1 —— 已实现却藏起来/失效的入口（快赢，直接解决"藏得深"）

> 纯前端、低风险，基本是侧栏/路由/命令注册层的调整。**功能都在，只是给个可见入口。**

### ☐ T1.1　7+ 个 workspace view 只有 ⌘K 一条路　【desktop · S】
- **我方**：命令面板给每个 view 自动生成 `view.open.<id>`（`plugins/builtin/defaults/commands.ts:104-131`，无排除），所以**不是不可达**——但下列**无 nav 行、无按钮、无 badge、无 tool-click**，对不住在面板里的用户等于隐形：

  | 功能 | view id | 备注 |
  |---|---|---|
  | 改动文件列表 | `files` | 它能打开 diff，但自己没人打开 |
  | 文件树浏览 | `explorer`（**非** `filetree`） | id 与文件名漂移，grep 不到 |
  | 工作区全文搜索 grep | `search` | 见 T1.2 |
  | Run 摘要 | `run-summary` | — |
  | Agent plan | `plan` | Proma 把 Plan 做进 composer 权限模式 |
  | 任务清单 todos | `todos` | — |
  | AGENTS.md 列表 | `agent-docs`（**非** `agentDocs`） | id 漂移 |
  | 对话导出/导入 | （无 view） | 仅 3 条 ⌘K 命令（`workspace/conversation-export/`） |

- **竞品**：Proma 把 Automations/Skills 做成**侧栏一键全屏 takeover**（`automation/AutomationsListView.tsx`、`agent-skills/AgentSkillsView.tsx`），不藏进子页。
- **改法**：把高频的（files/explorer/search/plan/todos）提进侧栏 Workspace 组（`sidebar/nav.tsx` 的 `WORKSPACE_DESTINATIONS`）或一个"+"溢出菜单；run-summary/agent-docs 可挂在 run 横幅/会话菜单。顺手修 id 漂移（`explorer`/`agent-docs`）。

### ☐ T1.2　搜索这条线整个是断的 + 假入口　【desktop · S-M】
- **我方**：**侧栏搜索框**（`sidebar/search.tsx`："Search… ⌘K"）和**折叠 rail 放大镜**（`sidebar/rail.tsx:34`）**都只是开命令面板**——而面板**不做内容搜索**；真正的工作区 grep view（`search`）反而藏在 ⌘K。三处指向都错位。`⌘F`（`chat-search/`）只在当前对话做 DOM 高亮，无跨会话。
- **竞品**：Proma `SearchDialog`（⌘⇧F，搜会话标题+消息内容，还有 Agent 语义搜索）；LobeChat CMDK 跨实体搜索（消息/话题/agent/文件，加权排序）；Cherry SearchPopup + ContentSearch。
- **改法**：让侧栏搜索框真的搜（会话+消息），或直连 grep view；命令面板加搜索源（见 T3.1）。**至少先让放大镜/搜索框不再是假入口。**

### ☐ T1.3　死的 "Execution mode" chip　【desktop · S】（与 T2.1 同根）
- **我方**：composer 页脚有个盾牌 chip "Workspace · Auto"（`chat/composer/index.tsx:155`），看着像审批模式开关，**无点击、无后端字段**，纯死。审批模式实际只能去 Settings→Agent→Approvals 配。
- **改法**：要么删掉（消除误导），要么按 T2.1 做成真的权限模式选择器（**推荐后者**）。

### ☐ T1.4　mid-run steer 完全无 UI　【desktop · S】
- **我方**：运行中发消息会 steer 当前 turn（`runs.steer`，`lib/agent/useChatSend.ts`）——**无按钮、无指示、无队列 UI**，用户不知道这能力存在，也看不到自己 steer 了。
- **竞品**：见 UX_POLISH_BACKLOG 的"边跑边排队/插话"——多数竞品有可见的排队/steer 反馈。
- **改法**：运行中给 composer 一个"插话/steer"态指示 + 已 steer 的消息在时间线上可见（SteerMessage 事件已有）。

---

## 4. Tier 2 —— composer 缺的能力入口（对标 Proma，后端大多已支持）

### ☐ T2.1　权限/计划模式选择器（把死 chip 变活）　【desktop · M】　**价值最高的单点**
- **竞品**：Proma `PermissionModeSelector.tsx` 就在输入框上——Auto / BypassPermissions / Plan，外加 in-flow 的 PermissionBanner / AskUserBanner / **ExitPlanModeBanner** 浮在 composer 上方（控制面出现在注意力所在处，而非模态弹窗）。
- **我方**：后端有 approval mode + plan + HITL（`domain/approval`、exit_plan_mode），前端只有那个死 chip + Settings 里的全局/规则配置。运行中无 per-turn 模式入口。
- **改法**：死 chip → 真选择器（按 turn 选 Auto/Plan/…）；HITL 确认做成贴近输入框的 banner（若现在是别处）。

### ☐ T2.2　工具 / MCP 运行时可见入口　【desktop · M】
- **竞品**：Cherry composer 有 MCP Tools 按钮（`pages/home/Inputbar/tools/`，**collapsible 工具注册表**，加能力=注册一个 tool 不重设计栏）；LobeChat 把工具/skills/MCP/web搜索/记忆/参数收进一个**统一 `+` 菜单**（`ChatInput/ActionBar/Plus/index.tsx`，带实时计数、渐进展开）。
- **我方**：MCP 工具开关只在 Settings；运行时看不到"这轮有哪些工具可用 / 哪些被 gating"。
- **改法**：composer 加一个工具/MCP popover（列本 turn 可用工具 + 跳管理）；**若 composer 要加多个按钮，直接上 LobeChat 的统一 `+` 菜单模式**，避免工具栏拥挤。

### ☐ T2.3　mention 词汇扩展 + 手动 compact / 上下文控制　【desktop · M】
- **竞品**：Proma `@`文件 `/`skill `#`MCP `&`会话 一套统一 mention；context-usage ring + **手动 compact 按钮**；LobeChat history-range（限轮）。
- **我方**：仅 `@`文件 + `/`slash（slash-hints + recipes）；有 usage chip 但**无手动压缩/限轮入口**（压缩只在 turn 后自动跑）。
- **改法**：mention 加 `#`(MCP)/`&`(会话引用，若后端支持)；usage chip 旁加手动 compact 触发。

> reasoning/thinking 开关：Cherry/Proma/LobeChat 都在 composer 给——lynx 现靠模型默认。**低优先**，按需。

---

## 5. Tier 3 —— 导航 / 可发现性模式（从 Cherry & LobeChat 借机制）

### ☐ T3.1　命令面板升级为"万能入口"　【desktop · M-L】
- **竞品**：LobeChat CMDK（`features/CommandMenu/`）= 命令 + **跨实体搜索**（消息/话题/agent/文件，tRPC，去抖、上下文加权）+ **上下文感知**（在 settings 里就出 settings 子页命令）+ **AI 模式**（Tab 问 AI）。一个 ⌘K 覆盖导航/搜索/设置子页/AI。
- **我方**：只索引 command，无搜索/会话/最近/文件源。
- **改法**：分步——先加**会话+消息搜索源**（补上整条断掉的搜索线，见 T1.2），再考虑上下文感知子页命令。**这是补全局搜索的最省力路径。**

### ☐ T3.2　常驻右侧 inspector（agent 在干什么）　【desktop · M-L】
- **竞品**：Proma 右栏 = 会话文件 / 工作区文件 / **改动 diff** 三 tab（`agent/SidePanel.tsx`）；LobeChat 双 inspector——WorkingSidebar（Space/Review-diff/Files/Params）+ Portal（制品/文档/工具输出/线程栈）。**agent 输出就地可检视，不用切去单独目的地。**
- **我方**：用 split-beside view 近似了 diff/terminal/file，但**无常驻"本次运行在动哪些文件/工具"的检视面**。
- **改法**：考虑一个常驻（或一键唤出）的运行检视右栏，把 files/diff/plan/todos 这几个本来 ⌘K-only 的 view 收编进去——一举解决 T1.1 的一半 + 给 agent 工作可见性。**与现有 split-beside 模型如何取舍需先设计。**

### ☐ T3.3　全局唤起（tray / 快捷启动窗 / 全局热键）　【desktop + Wails 壳 · L】
- **竞品**：Proma 三件套——Alt+Space 快任务悬浮窗（丢文件即起会话自动发）+ Ctrl+` 语音 + tray（含运行中会话状态）；Cherry Quick Assistant / Selection Assistant + tray。**应用可从 OS 任何地方唤起+观测。**
- **我方**：无 tray、无快捷启动、无全局热键。
- **改法**：Wails 壳层加 tray（运行中会话 + 新建）+ 全局热键唤起。**成本在壳层，留作后续。**

### ☐ T3.4　可配置侧栏 + 溢出网格（功能区多到 rail 装不下时）　【desktop · M】
- **竞品**：Cherry 11 个功能区，用户 pin ~5 到 rail，其余在 Home Launchpad 网格（`useNavLayout` 可拖拽/隐藏）；LobeChat 侧栏 = config-driven 可重排可隐藏列表 + "Customize Sidebar" 弹窗。
- **我方**：Workspace 组固定 5 项；将来功能区变多会挤。
- **改法**：把 Workspace 组做成可配置 + 一个"全部功能"溢出面（顺带收编 T1.1 的隐藏 view）。**功能区还不算多，触发条件未到时不必做。**

---

## 6. 明确**不追**（产品取舍 / 已决议，别重提）

- **Paintings / Translate / 多模态创作区 / 知识库 hub / Discover 市场**：Cherry/LobeChat 的 "AI hub" 定位，**不是 lynx（编码 agent）目标**。别为对齐它们加产品区。（市场作为"安装能力"的统一入口思路可借——但 lynx 的 skills/recipes/mcp 量级未必需要专门市场面。）
- **底部状态栏**：已删（`project_desktop_statusbar_removed`），别加回；持久遥测在 composer 页脚。
- **react-virtual**：已否（用 content-visibility）。
- **@codebase 的 in-composer attach**：已缓（`StartRunRequest.Context` 后端侧被丢弃）。
- **多 tab**：保留（已决议），不退回单 tab。
- **icon-rail-of-apps**：lynx 功能区少 + Codex 极简美学方向，**不照搬** Cherry 的多 rail；优先用"可配置侧栏 + 溢出"（T3.4）。

---

## 7. 建议优先级

1. **Tier 1 全做**（T1.1–T1.4，纯前端 / 低风险 / S）——直接消除"藏得深"，让已有功能浮出来。
2. **T2.1 权限/计划模式选择器**——把死 chip 变活，价值最高的单点，后端已支持。
3. **T3.1 命令面板加搜索源**——补全整条断掉的搜索线（与 T1.2 同根）。
4. 其余（T2.2/T2.3 composer 能力、T3.2 右侧 inspector、T3.3 全局唤起、T3.4 可配置侧栏）按需，触发条件到了再做。

> 落手前若涉及结构性改动（右侧 inspector / 导航模型），先按 `app/desktop/CLAUDE.md` 出 scope + 影响面 + 备选，确认再动。
