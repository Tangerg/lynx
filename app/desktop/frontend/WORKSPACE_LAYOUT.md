# Workspace Layout —— 三轨道 + 响应式宽度预算（提案）

> **Status: Proposed / 未实现。** 本文件记录 2026-06-14 关于"工具详情 / diff / terminal 等附加内容如何呈现"的设计讨论结论，作为未来实现的依据。**当前代码尚未按此实现。** 落地后，其中稳定的规范应合并进 `DESIGN.md` / `ARCHITECTURE.md`，本文件届时转为历史记录或删除（遵循 CLAUDE.md 第一法则：不留并存的过期文档）。
>
> 本文只描述**面向用户的布局与交互**，不锁定具体实现 API；真正的架构决策点集中在 §6 与 §7，留待实现前定夺。

---

## 0. TL;DR

把主区组织成**三轨道**：

- **中轨 = 视觉重心** —— 对话流，永远在，永不被替换。
- **左轨 = 导航区** —— 项目 / 会话树，可在「展开 ↔ rail」两态间切换。
- **右轨 = 附加区** —— 工具详情 / diff / terminal / files 等 workspace view。

**核心规则**：

1. 右轨是**会话的附属**，不是顶层 tab —— 打开它不切走对话，对话始终在中轨。
2. 右轨展开时，左轨**自动收成 rail（窄栏）**而非全量展开，把宽度让给「中 + 右」。
3. 收不收、并排还是叠层，由**窗口宽度预算**驱动，分三档自适应（§4）。

一句话：这是业界主流的「右侧并存附属面板」范式（对标 Claude Artifacts / ChatGPT Canvas / OpenHands 工作台），叠加一层「开右收左」的响应式宽度回收。

---

## 1. 背景与问题

### 1.1 当前行为

AI 吐出工具调用后，工具渲染为消息流内的 `ToolCard`（中轨，`max-w-760px` 列）：

- **点卡片本体** → `onToggleExpand`，在卡片内**就地展开** inline preview（`ToolCard.tsx`）。这一层是对的，保留。
- **点 inline preview 里的「Open in Terminal / Open full diff」** → 把对应 workspace view **提升为主区视图**：当前实现把 view 推进顶部 tab strip（`mainViewTabs`，与 chat session tab 混排），并占据主区。

> ⚠️ **待核准的现状细节**：代码里 `openViewForTool → openMainViewBeside`（`state/toolRouting.ts`）走的是 `splitViewId`（分屏 beside）；但用户实测体验是「新开一个 tab 并整屏切进去」（即 `openMainView` → `activeMainView` 的 full 形态）。两条路径都存在（`sessionStore.ts` 同时有 `openMainView` 与 `openMainViewBeside`）。**实现前需先揪出工具详情默认走的是哪个入口**，这决定了改造从哪里下手。

### 1.2 三个问题

1. **切走对话，丢失 agent 现场**：full 形态下中轨对话被整屏替换，agent 仍在该会话继续产出，用户却看不见，要手动点回 chat tab。即使是 beside 分屏，也把对话压窄、打断阅读。
2. **层级错位（最根本）**：terminal / diff 详情本**从属于某条消息**，却被提升成与「会话」平级的**顶层 tab**，和 chat session tab 混在同一条 strip。「一个会话」和「某次 bash 的输出」不该同级；点几次就堆几个 view tab，概念与视觉都乱。
3. **右侧偏窄**：若把详情放右侧而中轨对话不让位，右侧空间不足以渲染宽 diff / 宽代码（详见 §4 的宽度账）。

---

## 2. 业界调研：详情呈现的几种模式

> 数据来自对 Codex / Cherry Studio / LobeChat / OpenHands / Cline / Continue / Proma / MiMo-Code 等的横向分析。

详情内容在业界有四种承接模式：

- **A. 就地 inline**（消息流内折叠展开）：轻量详情的绝对主流。Cline `CodeAccordian`、Continue `ToolCallDiv`、Cherry / LobeChat 工具卡、Codex 类型化 cell。视线不离开对话。**Lyra 已有（卡片就地展开）。**
- **B. 右侧「与对话并存」的附属面板**：重量详情（完整 terminal / 大 diff / artifacts）的主流承接。OpenHands 右侧常驻工作台、LobeChat 右侧 Portal、MiMo 评审区，以及最有影响力的 **Claude Artifacts / ChatGPT Canvas**——打开右侧画布时，对话缩到左侧但**始终可见**，关掉即恢复。三个共同点：① 对话永不消失；② 面板内部就算有标签，那也是**面板内标签，不是顶层 tab**；③ 可开可关、可随 agent 自动更新。
- **C. 逃逸到宿主主编辑器 / 独立窗口**：看超大内容。Cline / Continue 丢给 VS Code 主编辑器；Proma 弹独立窗口（640–1100px）。
- **D. 临时全屏总览，看完即回**：Codex `Ctrl+T` transcript（alternate screen 临时叠层，Esc 回），是「可丢弃的全屏」，**不是持久 tab**。

### 两派分野

- **chat-first 派**（OpenHands / LobeChat / Claude / ChatGPT / Cherry / Cline / Continue）：详情走 **A inline + B 右侧并存**，对话永远在、不切走。
- **多文档工作台派**（仅 Proma 沾边）：点改动文件会开 preview tab 切过去——但它用「常驻改动列表打底 + 可弹独立窗口」两道缓冲软化，且不把任意工具调用升成会话级 tab。

**结论**：Lyra 是 chat-first 的 agent client，应走 **A 打底 + B 承重**。本提案即把「B 右侧并存」落成一条正式的右轨道。

---

## 3. 提案：三轨道布局

### 3.1 轨道职责

```
┌──────┬───────────────────────┬──────────────┐
│ 左轨  │       中轨（重心）      │   右轨（附加）  │
│ 导航  │        对话流          │  详情 / 产物   │
│      │                       │              │
│ 项目  │  消息、工具卡(inline)、  │ terminal     │
│ 会话  │  审批、composer        │ diff         │
│ 树    │                       │ files / 等    │
└──────┴───────────────────────┴──────────────┘
  rail↔展开      永远在、不被替换       可开关、会话附属
```

- **中轨**：唯一的视觉重心。对话流、inline 工具卡、HITL 审批、composer 全在此。**任何时候都不被右轨或 view 替换。**
- **左轨**：导航（项目 / 会话树）。沿用现有 `SidebarRail`(56px) ↔ `SidebarExpanded`(248px) 两态。
- **右轨**：附加区，承载 workspace view（terminal / diff / files / …）。默认关闭；按需打开。

### 3.2 核心原则：右轨是「会话附属」，不是顶层 tab

- 右轨**不进顶部 tab strip**。顶部 tab strip 从此**只留 chat session tab**（干净）。
- 右轨**内部**可以有自己的小切换（terminal / diff / files 间切），但那是轨道内的，与会话 tab 无关、不同级。
- **点工具详情 = 让右轨打开并聚焦对应 view**（模式 B），不是开 tab、不切走对话。
- 右轨可整体关闭（→ 中轨恢复全宽），可调宽（有上限，见 §4）。

### 3.3 「开右收左」

打开右轨时，左轨**自动从展开收成 rail**，把宽度让给「中 + 右」。这是对标 Claude Artifacts「打开画布 → 左侧会话列表自动收窄」的行为。但**不是无条件**——由 §4 的宽度预算决定。

---

## 4. 响应式宽度预算

### 4.1 现状数字（实现前的约束基线）

- `--app-min-width: 1180px`（`styles/layout.css`，注释 = 760 中侧 + 248 sidebar + gutters）；视口被 `body { min-width }` + WKWebView 锁死，**不会小于 1180**。
- 左轨：展开 `248px` / rail `56px`（`.app-main` grid）。
- 中轨对话列：`max-w-760px`（`MessageStream`）—— 理想可读宽。
- chrome 开销：`.app-main` 的 `gap: 8px` + `padding: 8px`；三轨道时约 `padding×2 + gap×2 = 32px` 非内容。

### 4.2 三档自适应

| 档位（视口宽） | 左轨 | 中轨（对话） | 右轨（附加） | 说明 |
|---|---|---|---|---|
| **宽屏 ≳1560px** | 展开 248 | ≥760 | ~480 | 三轨全开，中轨仍舒适，**不必收左** |
| **中屏 ~1180–1560** | **收 rail 56** | ~700–760 | ~360–400 | **本提案核心区间，最常见窗口尺寸**：开右即收左 |
| **窄屏 / 右轨需很宽** | rail 56 | 保中轨 | 转**叠层 overlay** | 并排放不下时，右轨浮在中轨上（不挤中），ESC 关 |

```
宽屏:  [导航 248][      对话（重心）≥760      ][ 附加 480 ]
中屏:  [▮56 ][       对话 ~720        ][ 附加 360 ]   ← 开右自动收左
窄屏:  [▮56 ][        对话           ]▒[ 附加浮层 ]▒   ← 右转叠层
```

### 4.3 计算依据（1180 最小视口下）

内容宽 ≈ `1180 − 32(chrome) = 1148`。左 rail 56 + 右 X + 中：

- 右 `X=360` → 中 `732`（≈760，✅）
- 右 `X=400` → 中 `692`（尚可）
- 右 `X=480` → 中 `612`（憋屈，❌）

宽屏 1560 下：`1560 − 32 = 1528`；左展开 248 + 右 480 → 中 `800`（✅ 可三开）。

### 4.4 两条硬约束

1. **中轨守最小可读宽 ~700px**（贴近 760）。右轨不得喧宾夺主——它是「附加区」。
2. **右轨宽度上限 ~360–400px**（中屏档）；超大内容走 overlay（窄屏档）或局部块内横滚（§5.5）。

> ⚠️ 三轨道并排可能需要**重新评估 `--app-min-width`**：若要在最小视口也支持「rail + 中760 + 右360」，需 `56 + 760 + 360 + 32 ≈ 1208 > 1180`。两种取舍：(a) 上调 min-width 到 ~1220；(b) 维持 1180，在最窄区间强制右轨走 overlay。**待定（§7）。**

---

## 5. 配套设计决策

### 5.1 状态归属

- **右轨「是否打开」+「当前显示哪个 view」→ per-session**（切会话时恢复该会话的右轨状态）。落 `agentStore`（per-session, ephemeral）或 `sessionStore`。
- **右轨宽度、左轨收 rail 偏好 → 全局**。落 `uiStore`（持久化偏好）。
- 复用现有 store 分层（`agentStore` per-session / `sessionStore` tab&选择 / `uiStore` 全局偏好），不新增 store。

### 5.2 收 rail：默认行为，可手动覆盖

「开右收左」是**默认建议**，不锁死。右轨开着时用户仍可手动顶开左轨（如要切会话）。即：自动收 rail 是一次性建议态，用户操作优先。

### 5.3 顶部 tab strip 只留会话

`mainViewTabs`（workspace view 进顶部 tab）这条**移除**。workspace view 只活在右轨内部，不再与 chat session tab 混排。`PanelTabBar` 回归「纯会话 tab strip」。

### 5.4 入口统一

把「看完整详情」的入口从「`openMainView` 整屏切走 / push 顶层 tab」**统一到「打开右轨 + 聚焦 view」**。现有 `openMainViewBeside`(`splitViewId`) 是右轨的雏形，可作为改造起点；`openMainView`(full 切走) 这条对工具详情场景应废弃。

### 5.5 溢出处理（右轨渲染宽内容）

右轨比中轨更窄，宽内容（diff / 宽代码 / JSON）必须**块内自吞横滚**，不能靠外层。已发现两处缺口需在落地时一并修：

- **`DiffView`**：用 `whitespace-pre`（不换行）但**无 `overflow-x` 容器**，父链 `.panel-scroll` 又故意只有 `overflow-y`（加 `overflow-x` 会破坏 use-stick-to-bottom），导致宽 diff 行**被 `.panel { overflow:hidden }` 静默裁掉**（既不换行也无横滚条）。
- **`ToolInspector`** JSON 路径：同病（`whitespace-pre` + 仅 `overflow-y-auto`）。

修法：给这两处的内容子容器加块内 `overflow-x: auto`，**照搬仓库已有的 `.shiki-block .shiki` / markdown 表格 `.md-table-wrap` 范式**（它们已正确块内横滚）。**注意：不能改 `.panel-scroll` 加 `overflow-x`**（layout.css 注释明确说会破坏 use-stick-to-bottom），必须加在组件自己的子容器上。因 `min-w-0` / `minmax(0,1fr)` 防撑爆惯例已彻底落实，加横滚不会引发新的撑爆。

> 取舍提示：对 diff / 代码，业界一致选「块内横滚」而非软换行（软换行破坏缩进对齐）。目标不是「消灭横滚条」，而是「把横滚关进块内的笼子，绝不冒泡成整页 / 整布局横滚」。

---

## 6. 与现有架构的关系 / 落地路径（仅记录，不实现）

### 6.1 可直接复用的现有基建

- **左轨两态**：`SidebarRail`(56) ↔ `SidebarExpanded`(248)，`uiStore.sidebarRail` 已有。
- **右轨雏形**：`MainSplit` + `SplitResizer`（拖拽比例 `splitRatio`，clamp 0.25–0.75）、`openMainViewBeside`(`splitViewId`)、`ViewPlacement`（`placement: full|split`，`splittable` 标志）。
- **workspace view 系统**：`plugins/builtin/workspace/workspace-views` + `defineWorkspaceView`（terminal / diff / files / … 已是插件贡献的 view）。
- **store 分层**：`agentStore` / `sessionStore` / `uiStore`（§5.1 直接用）。
- **slot 系统**：kernel 用命名 Slot（`app.sidebar` / `app.main` / `app.statusbar` / `app.overlay`）。
- **块内横滚范式**：`ShikiCodeBlock`、markdown 表格、`MermaidBlock` 已正确实现（§5.5 抄它们）。

> 用户回忆「最早的版本有右侧轨道」——若有残留结构，优先考据复用。

### 6.2 关键架构决策点：右轨道放在哪一层？

两个候选，需实现前定夺：

- **(a) kernel 级第三条 grid 轨道**：把 `.app-main` 从 `[sidebar] [main]` 扩成 `[sidebar] [main] [aux]`，右轨与左轨同级、由 kernel 统一协调「开右收左」的宽度预算。**优点**：三轨道心智最纯、宽度预算集中在一处；左 rail 与右展开天然联动。**缺点**：动 kernel grid + 新增一个 slot（如 `app.aux`）。
- **(b) 在 `app.main`（ChatPanel）内部做**：右轨是 ChatPanel 内部的分屏（沿用 `MainSplit` 思路扩展）。**优点**：改动局部、不动 kernel。**缺点**：左 rail 收缩（kernel 层）与右展开（main 内部）的宽度预算分散在两处协调，易割裂。

> 倾向 (a)（三轨道是 kernel 级布局语义，宽度预算应集中），但成本更高。**待定（§7）。**

### 6.3 大致涉及的改动面（参考，非任务清单）

- kernel grid / slot：`pages/AgentClientPage.tsx`、`styles/layout.css`（新增右轨 track + 响应式断点）。
- 右轨容器 + 内部 view 切换：新增（或由 `MainSplit` 演化）。
- 入口改造：`state/toolRouting.ts`、`state/sessionStore.ts`（废 `openMainView` 工具详情路径、移除 `mainViewTabs` 进顶部 strip）。
- tab strip：`components/chat/panel/PanelTabBar.tsx`（回归纯会话）。
- 溢出修复：`DiffView`、`ToolInspector`（§5.5）。
- 状态：`uiStore`（右轨宽度 / 收 rail 偏好）、`agentStore` 或 `sessionStore`（右轨开关 / 当前 view，per-session）。

---

## 7. 待定问题（Open Questions）

1. **右轨道所在层级**：kernel 级第三 track（6.2a）还是 `app.main` 内部（6.2b）？
2. **断点具体值**：1560 / 1180 是建议值，需实测；右轨默认宽（360？400？）。
3. **`--app-min-width` 是否上调**（§4.4）：上调到 ~1220，还是维持 1180 + 最窄区强制 overlay？
4. **右轨内容由谁驱动**：仅「点工具」打开聚焦？是否也支持「常驻自动跟随 agent 最新动作」（OpenHands 模式）？若支持，需解决「自动跟随 vs 用户手动锁定某个 view」的焦点争夺。
5. **宽屏是否真三开**：宽屏默认三轨全开，还是右轨仍按需打开（只是不必收左）？
6. **overlay 形态细节**：窄屏 / 超宽内容时右轨叠层——是右侧抽屉（drawer）还是居中放大？是否支持「钉住」升级为常驻并排？

---

## 8. 设计原则约束（确保与 Lyra 一致）

落地时必须遵守 `DESIGN.md` 既有原则：

- **No lines**：三轨道之间的分隔用 **surface ladder + 8px gap + cards-on-canvas**（轨道是浮在 canvas 上的卡片，靠间隙和 surface 层级区分），**不用 1px 边线**。
- **Cards-on-canvas**：右轨与中轨、左轨一样是 canvas 上的浮起 panel（8px gap、radius、阴影），不是贴边硬分栏。
- **Radix-first**：右轨的拖拽 resizer 若 Radix 无对应 primitive，沿用现有 `SplitResizer` 的自写豁免（已在 DESIGN/CLAUDE 约定内）；可折叠 / 标签等交互优先 Radix。
- **第一法则**：本提案是 forward-looking 设计，**不为兼容旧的「顶层 view tab」行为留并存路径**——实现时直接移除 `mainViewTabs` 进顶部 strip 这条，不做迁移 shim。
- **响应式 = 宽度预算驱动**：所有「收 / 展 / 叠层」由窗口宽度阈值决定，不硬编死布局；给隐式 / 单列 grid 显式 `minmax(0,1fr)` 防撑爆（CLAUDE.md §4 硬约定）。
