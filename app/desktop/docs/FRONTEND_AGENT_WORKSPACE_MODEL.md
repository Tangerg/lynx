# Frontend Agent Workspace Model

> 本文记录 ScopeApp desktop 现行的主 UI 心智模型。它回答的是：
> **左侧 / 中间 / 右侧分别为什么存在，什么东西应该放在哪，后续重构怎么判断方向。**
>
> 视觉 token、阴影、字体、圆角等细节看 [`../frontend/DESIGN.md`](../frontend/DESIGN.md)
> 与 [`../frontend/DESKTOP_UI_POLISH.md`](../frontend/DESKTOP_UI_POLISH.md)；插件限界上下文与依赖规则看
> [`FRONTEND_PLUGIN_CONTEXTS.md`](./FRONTEND_PLUGIN_CONTEXTS.md)；协议权威定义看
> [`app/runtime/doc/API.md`](../../runtime/doc/API.md)。

## 1. North Star

**ScopeApp 是一个 AI agent 工作台，不是普通 IDE，也不是功能菜单驱动的后台系统。**

主界面不是回答“ScopeApp 有哪些功能”，而是回答：

```text
我的 agent 正在 / 曾经在哪些工作上下文里做事？
当前这段工作正在发生什么？
当前工作需要哪些代码、diff、工具和上下文材料？
```

因此主 UI 使用三段式心智模型：

```text
Left   = Work Index
Center = Agent Narrative
Right  = Context Dock
```

更口语地说：

```text
左侧找工作，中间做工作，右侧放材料。
```

这不是“传统三栏布局”的泛化，而是贴近 AI agent 协议生命周期的布局：

- **Work Index**：低心率的工作上下文索引。
- **Agent Narrative**：当前 session 的主叙事。
- **Context Dock**：当前 session / cwd 的上下文工作台。

## 2. Protocol Mapping

ScopeApp Runtime Protocol 的核心原语已经决定了 UI 的主轴：

```text
Session
  ├─ root Run
  │    ├─ Segment
  │    ├─ Item
  │    └─ delegated Run
  │         ├─ Segment
  │         ├─ Item
  │         └─ delegated Run ...
  └─ later root Run ...
```

协议语义到 UI 区域的映射：

| Protocol / Projection                     | UI Area                                        | Meaning                                                                   |
| ----------------------------------------- | ---------------------------------------------- | ------------------------------------------------------------------------- |
| `Session`                                 | Work Index                                     | 可恢复、可继续、可 fork 的 agent 工作上下文                               |
| root `Run`                                | Agent Narrative                                | 用户发起的一条主执行；composer、plan 与顶层 status 只跟随当前 root        |
| delegated `Run`                           | Agent Narrative disclosure + Context Dock      | 由准确 `spawnedByItemId` 挂到父 task；默认摘要，按需展开                   |
| `RunRef` lineage                          | Context Dock Timeline                          | parent/root/source identity 的 durable 审计树                              |
| `Segment`                                 | 不单独占据导航区域                             | 同一 Run 的执行/等待/续跑边界；用于生命周期与恢复，不冒充新 Run            |
| `Item`                                    | Agent Narrative                                | message、reasoning、plan、tool call、question 等 durable source-owned 单元；terminal AgentMessage phase 区分过程与最终回答 |
| `Session.cwd`                             | Work Index + Context Dock                      | 项目身份 + 文件系统工具根                                                 |
| `Project`                                 | Work Index decoration                          | 按 `Session.cwd` 派生的分组视图，无 opaque id、无 active 标记             |
| `workspace.*`                             | Context Dock                                   | 当前 cwd 的文件、diff、grep、skills、recipes、memory、hooks 等            |
| `PendingInterruptSet` / waiting root tree | Agent Narrative first, Work Index badge second | agent 等待用户介入；action 仍留在其 source narrative                       |

几个强结论：

- 用户选择的是 **Session**，不是 Project。
- `cwd` 是 session 的工作身份，不是连接级 active project。
- `Project` 只是 distinct `Session.cwd` 的派生分组，不应该变成独立管理主资源。
- `workspace.*` 能力大多显式收 `cwd?`，它们属于当前 session/cwd 的 Context Dock，不属于左侧顶级导航。
- Agent Narrative 是 root-first，不是把整棵树按到达顺序摊平；descendant material
  必须保留 source Run，并挂到其 parent task。
- Timeline、tool log 与 Run tree 是整个 active Session 的材料；当前 root plan、stop
  和 running attention 才是 current-root scope。公开 API 名称必须把两者写清楚。
- commentary / final answer 是 Runtime terminal AgentMessage 的持久事实，不是 UI 根据“最后一条”、stream closure 或文本形态派生的分组。Frontend 只投影该 phase，并让 live、replay 与 mixed hydration 收敛。

## 3. Layout Responsibilities

### 3.1 Work Index

Work Index 的心率要低。它应当稳定、稀疏、可扫视，像 Codex 的左侧一样只承载工作索引。

允许内容：

- global actions：New Session、Search、Scheduled、Plugins。
- cwd / project group：由 `Session.cwd` 派生的分组。
- session rows：title、recency、favorite、running/waiting/error dot。
- attention badges：running、waiting for approval/input、system error。
- footer：settings、theme、runtime/account 状态等全局动作。

不应放入 Work Index：

- files / file tree。
- diff / review。
- semantic index search；文件检索只通过明确的 grep / file / symbol 能力进入。
- memory / skills / recipes / hooks。
- tool detail。
- session-scoped settings 或 cwd-scoped panels。

判据：

```text
如果这个入口离开 active session / cwd 就没有明确语义，它不属于左侧顶级。
```

### 3.2 Agent Narrative

Agent Narrative 是主舞台。它承载用户和 agent 的共同时间线。

允许内容：

- session header：title、status、overflow actions；模型、权限和 context 占用留在 Composer control rung，不在标题右侧重复。
- root transcript：全部 root-owned user / assistant turns。
- work/final hierarchy：commentary、reasoning、tool activity 组成可折叠 work narrative；最终回答是独立 message row，只有该 row 拥有 message actions。
- delegated disclosure：按 `spawnedByItemId` 锚定的 child / sibling / nested narrative。
- run progress：当前 root plan；每个 delegated Run 自己的 reasoning、plan、progress、usage。
- HITL：approval、question、client tool result。
- composer：draft、attachments、permission/model controls、send/stop。

不应把文件树、长期资源目录、settings panes 放在中间主线里。它们可以被引用、预览或触发，但完整工作面应在 Context Dock。

Agent Narrative 不负责重新发明 tree 事实：

- 不从 tool name、文本、Item 顺序或相邻事件猜 lineage；
- 不把 child Item 拼进 root assistant turn；
- 不把 commentary/tool work 与 terminal final answer 拼进同一 Assistant row，也不按位置猜 phase；
- 不用一个 `running: boolean` 表达 waiting/finished/error/canceled；
- cancel target 永远是 disclosure 对应的 exact RunID，UI 不先伪造 terminal。

### 3.3 Context Dock

Context Dock 是当前 session/cwd 的材料区。它不是永久抢戏的第三栏，而是可轻可重的上下文工作台。

默认形态应轻：

- 类 inspector，低 chrome。
- 可折叠。
- tab / segmented controls 少而稳定。
- 没有上下文时可以收起或显示轻量 project overview。

进入专业场景时可以变重：

- review mode：changed files + checklist + diff + inline comments。
- file mode：file tree + opened file + breadcrumb。
- tool mode：selected tool call detail + outputs。
- run mode：完整 Run tree、按 source Run 组织的 Timeline、Session tool log。
- search mode：grep / symbols；不维护独立向量索引状态。
- memory / skills / recipes：围绕当前 cwd 展示。

Context Dock 的内容由 active `Session.cwd` 驱动。切换 session 后，应恢复该 session 自己的 dock 状态，而不是共享一份全局状态。

## 4. Visual Direction

整体视觉不是“更炫”，而是更像本地桌面 agent 工具：

- 左侧低对比、低密度、低变化。
- 中间留白充足，保持阅读主线。
- 右侧像 inspector/editor，不像卡片 dashboard。
- 信息密度高，但 chrome 要少。
- 状态靠 dot、badge、row fill、small label 表达，不靠大色块。
- 不用巨大圆角制造“高级感”。
- 不用硬边框堆结构，优先 surface ladder + shadow hairline。

### 4.1 Theme and Surface Rules

ScopeApp 已经有主题系统，后续 UI 调整必须把主题系统当作唯一视觉权威：

- 组件不直接写品牌色、灰阶、透明白、散落 shadow。
- surface、border、text、accent、focus、shadow、radius 都从 theme token 读取。
- 新增视觉层级时先补 token，再消费 token。
- 不为了某个页面单独发明“临时高级感”。

主界面应使用清晰的 surface ladder，而不是一层灰雾：

```text
app background
  -> sidebar material
  -> content surface
  -> floating composer / popover
  -> modal / blocking approval
```

每一层只允许通过少量变量表达：

- background delta：非常轻的明度 / 色相差。
- hairline：只在需要分割阅读区域时出现。
- shadow：表达浮起，不表达边界。
- radius：随空间密度变小，工作台界面默认克制。

左侧尤其要避免两种错误：

- 过白、过亮、过强对比，导致“蒙了一层白雾”。
- 过多按钮 / 卡片 / 圆角，把 Work Index 做成普通功能菜单。

### 4.2 Remove the Web Feel

ScopeApp 是桌面工作台，不能像网页后台。判断标准：

- 导航像本地 app 的 source list，不像 SaaS sidebar。
- 右侧像 inspector / editor，不像 dashboard card grid。
- composer 像输入设备，不像网页表单。
- 状态变化要稳定，不因为 hover / loading / badge 改变布局尺寸。
- 列表、树、diff、工具输出优先用密集但克制的信息结构，不用营销式大卡片。

交互细节：

- icon button 用真实图标 + tooltip，不用文字胶囊按钮堆满工具栏。
- popup / menu / select 走 Base UI primitive，视觉由 ScopeApp theme 接管。
- transient surface 使用短而轻的 enter/exit animation，但不能抢主叙事注意力。
- resize / collapse / split view 应保持 session context，不把用户正在看的材料清掉。

三种目标状态：

| Mode             | Use Case               | Shape                                                     |
| ---------------- | ---------------------- | --------------------------------------------------------- |
| Baseline         | 日常对话与轻量代码任务 | Work Index + Agent Narrative + light Context Dock         |
| Collapsed Dock   | 用户专注对话           | 右侧只留窄 handle / icon rail，需要时展开                 |
| Review Workspace | 审查 diff / 多文件改动 | 右侧变重：逐文件可折叠 diff + 变更文件导航，dock 按密度加宽 |

默认应偏向 **Baseline + Collapsed Dock**，只有用户或 agent 明确进入 review/diff/file context 时才进入 Review Workspace。

**密度由材料声明，不由用户切开关**：dock view 在 `WorkspaceViewSpec.density` 上声明自己要多宽（`light` / `review`），dock 为每种密度各记一份宽度。所以"进入 Review Workspace"就是打开一个 review 密度的 destination —— 没有第三个模式开关，也不会因为为读代码拖宽过一次，就让之后每个清单都停在 review 宽度。

**checklist / comments 不进 Review Workspace**：todos 与 plan 已是 run-scoped destination，各自独立更清楚；comments（行内评审批注）当前**没有任何生产者**，所以不留空槽 —— 声明一个没人实现的面板，就是让 UI 替不存在的能力打广告。

## 5. Plugin Contribution Model

保持插件式架构，但不要让插件直接把任何东西塞到左侧。

不要再建一套万能 placement 枚举。现在的模型是两套聚焦注册表：

1. **Work Index Item**：左侧 source list / collapsed rail 的贡献点，只表达“找工作”和“切工作”的入口。
2. **Context Dock Destination**：右侧工作上下文的贡献点，只表达 active session/cwd/run 下可展开的材料。

Work Index 贡献声明 scope 与 variant：

```ts
type WorkIndexItemScope = "global" | "session";
type WorkIndexItemVariant = "expanded" | "rail";
```

Context Dock 贡献只声明 scope；placement 由 `scopeapp.contextDock.destination` 这个扩展点身份隐含：

```ts
type ContextDockDestinationScope = "workspace" | "session" | "run";
```

推荐归属：

| Feature                       | Registry                   | Scope       | Variant / Placement                            |
| ----------------------------- | -------------------------- | ----------- | ---------------------------------------------- |
| New Session                   | `WorkIndexItem`            | `global`    | `expanded` and `rail`                          |
| Current session list          | `WorkIndexItem`            | `session`   | `expanded` and `rail`                          |
| Settings                      | `WorkIndexItem`            | `global`    | `rail` utility                                 |
| Context launcher              | `WorkIndexItem`            | `session`   | `rail` handle into Context Dock                |
| Files / File Tree             | `ContextDockDestination`   | `workspace` | Context Dock placement is implicit             |
| Diff / Review                 | `ContextDockDestination`   | `workspace` | Context Dock placement is implicit             |
| Grep / Symbol Search          | `ContextDockDestination`   | `workspace` | Context Dock placement is implicit             |
| Skills / Recipes / Agent Docs | `ContextDockDestination`   | `workspace` | Context Dock placement is implicit             |
| Memory                        | `ContextDockDestination`   | `workspace` | Context Dock placement is implicit             |
| Tool Detail                   | `ContextDockDestination`   | `run`       | Context Dock placement is implicit             |
| Timeline / run notes          | `ContextDockDestination`   | `session`   | Context Dock placement is implicit             |
| Approval / Question           | Agent Narrative projection | `run`       | Narrative first, Work Index attention second   |

规则：

- Work Index 只接受 `global` 或 `session` scope；workspace/run 级材料不能回到左侧顶级。
- `expanded` 与 `rail` 是同一 Work Index 的两种呈现，不是两个业务入口；贡献方需要明确自己在哪个 variant 出现。
- Context Dock destination 不声明 placement；它的 placement 由扩展点身份决定，spec 只负责声明 `workspace / session / run` scope。
- run-scoped blocking action 优先在 Agent Narrative 完成，只把 attention 投影到 Work Index，避免用户必须去右侧找“为什么停住了”。
- 插件贡献 UI 可以多样，但 contribution registry 必须表达心智归属，不能只表达 slot。

## 6. Current Frontend Architecture

`navigation` 限界上下文已经成为左侧 Work Index 的功能边界：它拥有 project/session grouping、recent-session read model、attention 投影、action wiring 与 Work Index contribution surface；`sidebar` 只负责渲染。

当前目录：

```text
plugins/builtin/navigation/
  domain/
    workIndex.ts
  application/
    buildWorkIndex.ts
    useWorkIndex.ts
    workIndexActions.ts
  public/
    workIndex.ts
```

Context Dock 可作为 workspace 上下文的子域演进：

```text
plugins/builtin/workspace/context-dock/
  index.ts
plugins/builtin/workspace/application/
  contextDock.ts
  contextDockDestinations.ts
  contextDockDestinationGroups.ts
  useContextDockLauncher.ts
```

关键边界：

- Work Index UI 只消费 navigation read model，不直接 join `useSessions()` / `useProjects()` / active view state。
- Context Dock 只围绕 active session/cwd 组织 workspace destinations。
- Agent Narrative 只消费 Agent public conversation / Run / HITL language，不读取 Agent
  Store 或 Runtime wire。
- Agent context 以一份 `AgentSessionView` 保存 normalized `runsById` 与 source-owned
  material；root narrative、delegated disclosures 和 Session-wide audit 都是 selector。
- Runtime AgentMessage phase 经 adapter 进入同一 `AgentSessionView`；fold 可在 terminal frame 到达时把 provisional text 从 commentary rehome 到稳定 `final:<itemId>` row，但不得新增第二 transcript store/writer。
- `agent/public/run.ts` 的 current-root、active-Session 与 exact-Run command 名称不能互换。
- app-global surface state 与 session/cwd-scoped dock state 不应混在同一个 store shape。

## 7. State Ownership

状态按归属拆分：

| State                 | Owner                                             | Example                                                  |
| --------------------- | ------------------------------------------------- | -------------------------------------------------------- |
| App-global chrome     | shell / ui store                                  | theme、sidebar collapsed、settings route、每密度的 dock 宽度 |
| Work index read model | navigation application                            | groups、session rows、attention badges                   |
| Agent runtime view    | agent context                                     | normalized runs、source-owned messages/tools/plans/timeline、interrupts、usage、message phase |
| Context dock state    | workspace context, scoped by `sessionId` or `cwd` | active dock tab、opened file、selected diff、tool detail |
| Ephemeral UI state    | local component                                   | hover、temporary filter、expanded disclosure             |

Context Dock state 必须能回答：

```text
切到 session A，右侧打开的是 A 上次看的文件/diff；
切到 session B，右侧恢复 B 自己的上下文；
切回 A，不丢 A 的右侧工作台。
```

不要用“切 Session 时清空一堆 patch”掩盖错误 ownership；Context Dock material
必须按 Session（将来有真实共享需求时再显式按 cwd）建模。

## 8. Implementation Record

以下阶段均已完成；本节记录当前完成态与回归门，不再作为兼容迁移计划。

P128 的 Agent Narrative 收口建立在同一模型上：Runtime 在 terminal boundary 写入 `commentary | finalAnswer`，Transcript/SQLite/Artifact/public surface 共同持久该事实；Frontend provisional stream 先留在 work narrative，terminal final answer 再以稳定 identity 独立呈现。commentary/canceled/waiting 行不发布 context menu/message actions，同 Run 紧邻最终回答只让前一行 process material 进入 Codex wave folding。

P129 进一步固定 Conversation 与 Transcript 的可见性边界：Application 生成的 fresh autonomous Goal 控制提示只进入 provider Conversation，不创建用户 Transcript Item，也不向 Frontend 返回伪 `UserItemID`；真实用户 start/resume input 仍保持可见。standing Goal 只从 `goals.get` / `goals.changed` 投影 lifecycle、objective 与真实 actions，Frontend 不以字符串过滤、CSS 隐藏或本地缓存修补内部控制提示泄漏。

### Phase 1: Document and Guard the Model — `DONE`

- 本文作为主 UI 心智模型。
- 在 CLAUDE / ARCHITECTURE 中引用本文。
- 将“左侧不是功能菜单”作为后续 review 的判断标准。

### Phase 2: Build Navigation Read Model — `DONE`

- navigation context 已拥有 Session/cwd grouping、attention 与 actions。
- sidebar 只消费 navigation public read model，已不现场 join 多个业务数据源。

验收：

- sidebar UI 不直接拼 projects + sessions。
- session row 状态来自统一 read model。
- 项目分组只表达 `cwd` 派生视图。

### Phase 3: Move Workspace Destinations to Context Dock — `DONE`

- 移除左侧顶级 `skills / recipes / tools / memory` 类 workspace destinations；废弃的语义索引入口不再注册。
- 建立 context-dock destination contribution。
- Files / Diff / Search / Skills / Recipes / Memory 进入右侧 dock。

首批已落地：

- 左侧顶级 workspace destinations 已移除。
- 右侧提供 `context` launcher / handle。
- Search、active-session destinations、rail context 入口都打开到 Context Dock。
- Context Dock destinations 已进入 `scopeapp.contextDock.destination` contribution registry，首批内置入口由 workspace 插件贡献，launcher 按 `workspace / run / session` scope 渲染 read model。
- 左侧 active session 下不再嵌 workspace/run 快捷入口；Work Index 只表达 session 选择与状态。
- 左侧顶部不再暴露 workspace grep 假装全局 Search；文件搜索从 Context Dock 的 workspace scope 进入。

验收：

- 左侧只剩 global actions + cwd groups + sessions + global footer。
- 当前 session/cwd 的工具都从右侧进入。

### Phase 4: Split Workspace Navigation State — `DONE`

- 拆出 app-global surface state。
- 拆出 session/cwd-scoped context dock state。
- 去掉靠切 session 清空全局 workspace patch 的长期依赖。

已落地：

- Context Dock 的 `splitViewId`、`activeFile`、`fileViewer`、`selectedToolId`、`expandedToolIds` 已按 active session scope 保存/恢复。
- 切换 session 会保存离开的 dock scope，恢复进入的 dock scope；没有保存过的 session 使用空 scope。
- 关闭 session 后会清理不再打开的 dock scope。
- app-global surface state 已进入 `workspaceSurfaceStore`，session-scoped dock state 已进入 `contextDockStore`。

后续如需 cwd 级共享，再在 workspace application 层显式引入 `sessionId -> cwd` 的归属规则。

验收：

- 每个 session 能恢复自己的 dock 状态。
- Settings 不和 workspace file/tool state 混在同一个 store shape 里。

### Phase 5: Structural Visual Pass — `DONE`

- 左侧按低心率 Codex-like 方向降噪。
- 右侧做 light Context Dock + Review Workspace 两种密度。
- 所有 surface 深度走 theme token 和 `DESKTOP_UI_POLISH.md` 的 shadow model。

已落地：

- **Review Workspace 成形**：diff view 不再按 active file 过滤查询 —— 它展示整个改动，逐文件可折叠卡片在同一个滚动区里，右侧一条可筛选的变更文件导航（点行**滚动**到那个文件，不替换内容）。active file 从"过滤器"变成"焦点"：导航高亮它、打开时滚到它。
- **两种密度**：`WorkspaceViewSpec.density` + 每密度一份 dock 宽度（见 §4）；一个没有任何 view 声明的密度会让 gate 变红。
- **深度与边缘全部进 token**：逃出守卫的 6 处硬编码 shadow、3 处手挑 border alpha 全部收敛；内部竖分割线有唯一作者（`.agent-pane-split`，`data-split-side` 选边）；`check-design-tokens` 增两条规则挡住回归。
- 几何数字对齐 `~/Desktop/synara`（chrome bar 46px、列宽下限 208/640、seam ring clip 到自身半径），实现走 ScopeApp 自己的 token 而非它的组件库。

验收：

- 左侧没有 session-scoped tools。✅
- 右侧可以折叠。✅
- Review/Diff 模式信息密度高但 chrome 克制。✅

剩余像素级 Synara 对齐属于总计划 W7：以 `~/Desktop/synara` 为参考做真机截图、布局和
视觉回归，不改变本文已经冻结的 Work Index / Agent Narrative / Context Dock
information architecture。

## 9. Anti-Patterns

以下都是回归：

- 把 Files / Diff / Memory / Skills / Recipes 放回左侧顶级。
- 把 Project 做成带 opaque id 的主动资源，或维护 active project。
- Work Index 直接拼业务数据源，而不是消费 navigation read model。
- Context Dock 使用一份全局 active file / selected diff，切 session 后互相污染。
- 把 approval/question 只放右侧，导致用户在主叙事里看不到 agent 正在等什么。
- 根据消息位置、文本内容或 stream closure 猜 final answer，或让 commentary/canceled work row 继承最终回答 actions。
- 用更多卡片、边框、圆角、hover 来制造“高级感”。
- 插件只贡献 slot，不声明 Work Index / Context Dock / Narrative 这类心智归属，导致信息架构再次失控。

## 10. Decision Checklist

加一个入口或面板前，先回答：

1. 它是 global、session、workspace 还是 run scope？
2. 离开 active session/cwd 是否仍有明确语义？
3. 它是帮助用户“找工作”，还是帮助用户“做当前工作”？
4. 它应该在 Narrative 中完成，还是在 Context Dock 中展开？
5. 它是否会让左侧变成普通功能菜单？

判断结果：

| Answer                       | Surface                                      |
| ---------------------------- | -------------------------------------------- |
| global action                | Work Index                                   |
| session selection / status   | Work Index                                   |
| run blocking action          | Agent Narrative + Work Index attention badge |
| workspace/cwd material       | Context Dock                                 |
| detailed file/diff/tool view | Context Dock                                 |

如果判断不清，默认不要放到左侧。
