# Frontend Agent Workspace Model

> 本文记录 Lyra desktop 下一阶段的主 UI 心智模型。它回答的是：
> **左侧 / 中间 / 右侧分别为什么存在，什么东西应该放在哪，后续重构怎么判断方向。**
>
> 视觉 token、阴影、字体、圆角等细节看 [`../frontend/DESIGN.md`](../frontend/DESIGN.md)
> 与 [`../frontend/DESKTOP_UI_POLISH.md`](../frontend/DESKTOP_UI_POLISH.md)；插件限界上下文与依赖规则看
> [`FRONTEND_PLUGIN_CONTEXTS.md`](./FRONTEND_PLUGIN_CONTEXTS.md)；协议权威定义看
> [`protocol/API.md`](./protocol/API.md)。

## 1. North Star

**Lyra 是一个 AI agent 工作台，不是普通 IDE，也不是功能菜单驱动的后台系统。**

主界面不是回答“Lyra 有哪些功能”，而是回答：

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

Lyra Runtime Protocol 的核心原语已经决定了 UI 的主轴：

```text
Session
  └─ Run
       └─ Item
```

协议语义到 UI 区域的映射：

| Protocol                        | UI Area                                        | Meaning                                                        |
| ------------------------------- | ---------------------------------------------- | -------------------------------------------------------------- |
| `Session`                       | Work Index                                     | 可恢复、可继续、可 fork 的 agent 工作上下文                    |
| `Run`                           | Agent Narrative                                | 一次 agent 执行，从用户输入到 outcome                          |
| `Item`                          | Agent Narrative                                | 消息、reasoning、plan、tool call、question 等 durable 工作单元 |
| `Session.cwd`                   | Work Index + Context Dock                      | 项目身份 + 文件系统工具根                                      |
| `Project`                       | Work Index decoration                          | 按 `Session.cwd` 派生的分组视图，无 opaque id、无 active 标记  |
| `workspace.*`                   | Context Dock                                   | 当前 cwd 的文件、diff、grep、skills、recipes、memory、hooks 等 |
| `OpenInterrupt` / waiting state | Agent Narrative first, Work Index badge second | agent 等待用户介入                                             |

几个强结论：

- 用户选择的是 **Session**，不是 Project。
- `cwd` 是 session 的工作身份，不是连接级 active project。
- `Project` 只是 distinct `Session.cwd` 的派生分组，不应该变成独立管理主资源。
- `workspace.*` 能力大多显式收 `cwd?`，它们属于当前 session/cwd 的 Context Dock，不属于左侧顶级导航。

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
- codebase search。
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

- session header：title、status、model/permission summary、overflow actions。
- transcript：user / assistant messages。
- run progress：plan、reasoning、tool progress、usage。
- HITL：approval、question、client tool result。
- composer：draft、attachments、permission/model controls、send/stop。

不应把文件树、长期资源目录、settings panes 放在中间主线里。它们可以被引用、预览或触发，但完整工作面应在 Context Dock。

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
- codebase mode：grep / semantic search / symbols。
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

Lyra 已经有主题系统，后续 UI 调整必须把主题系统当作唯一视觉权威：

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

Lyra 是桌面工作台，不能像网页后台。判断标准：

- 导航像本地 app 的 source list，不像 SaaS sidebar。
- 右侧像 inspector / editor，不像 dashboard card grid。
- composer 像输入设备，不像网页表单。
- 状态变化要稳定，不因为 hover / loading / badge 改变布局尺寸。
- 列表、树、diff、工具输出优先用密集但克制的信息结构，不用营销式大卡片。

交互细节：

- icon button 用真实图标 + tooltip，不用文字胶囊按钮堆满工具栏。
- popup / menu / select 走 Base UI primitive，视觉由 Lyra theme 接管。
- transient surface 使用短而轻的 enter/exit animation，但不能抢主叙事注意力。
- resize / collapse / split view 应保持 session context，不把用户正在看的材料清掉。

三种目标状态：

| Mode             | Use Case               | Shape                                              |
| ---------------- | ---------------------- | -------------------------------------------------- |
| Baseline         | 日常对话与轻量代码任务 | Work Index + Agent Narrative + light Context Dock  |
| Collapsed Dock   | 用户专注对话           | 右侧只留窄 handle / icon rail，需要时展开          |
| Review Workspace | 审查 diff / 多文件改动 | 右侧变重，显示 files + diff + comments / checklist |

默认应偏向 **Baseline + Collapsed Dock**，只有用户或 agent 明确进入 review/diff/file context 时才进入 Review Workspace。

## 5. Plugin Contribution Model

保持插件式架构，但不要让插件直接把任何东西塞到左侧。

插件贡献需要带 scope 与 placement：

```ts
type DestinationScope = "global" | "session" | "workspace" | "run";
type DestinationPlacement =
  "work-index" | "attention" | "narrative-action" | "context-dock" | "footer";
```

推荐语义：

| Feature                       | Scope       | Placement                                          |
| ----------------------------- | ----------- | -------------------------------------------------- |
| New Session                   | `global`    | `work-index`                                       |
| Cross-session Search          | `global`    | `work-index`                                       |
| Scheduled Runs                | `global`    | `work-index`                                       |
| Plugins / MCP catalog         | `global`    | `work-index` or `footer`                           |
| Settings                      | `global`    | `footer`                                           |
| Files / File Tree             | `workspace` | `context-dock`                                     |
| Diff / Review                 | `workspace` | `context-dock`                                     |
| Grep / Codebase Search        | `workspace` | `context-dock`                                     |
| Skills / Recipes / Agent Docs | `workspace` | `context-dock`                                     |
| Memory                        | `workspace` | `context-dock`                                     |
| Tool Detail                   | `run`       | `context-dock`                                     |
| Approval / Question           | `run`       | `narrative-action` first, `attention` badge second |

规则：

- `work-index` 只接受 global 或 session-list 级贡献。
- workspace-scoped destination 默认进 Context Dock。
- run-scoped blocking action 优先在 Narrative 完成，只在左侧显示 attention badge。
- 插件贡献 UI 可以多样，但 contribution registry 必须表达 scope，不能只表达 slot。

## 6. Frontend Architecture Target

`navigation` 限界上下文已经成为左侧 Work Index 的功能边界：它拥有 project/session grouping、recent-session read model、attention 投影、action wiring 与 Work Index contribution surface；`sidebar` 只负责渲染。

当前目标目录：

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
- Agent Narrative 只关心 active session 的 transcript/run/HITL/composer。
- app-global surface state 与 session/cwd-scoped dock state 不应混在同一个 store shape。

## 7. State Ownership

状态按归属拆分：

| State                 | Owner                                             | Example                                                  |
| --------------------- | ------------------------------------------------- | -------------------------------------------------------- |
| App-global chrome     | shell / ui store                                  | theme、sidebar collapsed、settings route                 |
| Work index read model | navigation application                            | groups、session rows、attention badges                   |
| Agent runtime view    | agent context                                     | messages、runs、interrupts、usage                        |
| Context dock state    | workspace context, scoped by `sessionId` or `cwd` | active dock tab、opened file、selected diff、tool detail |
| Ephemeral UI state    | local component                                   | hover、temporary filter、expanded disclosure             |

Context Dock state 必须能回答：

```text
切到 session A，右侧打开的是 A 上次看的文件/diff；
切到 session B，右侧恢复 B 自己的上下文；
切回 A，不丢 A 的右侧工作台。
```

不要用“切 session 时清空一堆 patch”作为长期模型。清空可以作为迁移期保护，但目标是按 session/cwd 建模。

## 8. Migration Plan

### Phase 1: Document and Guard the Model

- 本文作为主 UI 心智模型。
- 在 CLAUDE / ARCHITECTURE 中引用本文。
- 将“左侧不是功能菜单”作为后续 review 的判断标准。

### Phase 2: Build Navigation Read Model

- 新建 navigation context。
- 把当前 sidebar 对 `sessions.list`、`workspace.listProjects`、active session、attention state 的拼装收进 application。
- Work Index UI 变成纯 renderer。

验收：

- sidebar UI 不直接拼 projects + sessions。
- session row 状态来自统一 read model。
- 项目分组只表达 `cwd` 派生视图。

### Phase 3: Move Workspace Destinations to Context Dock

- 移除左侧顶级 `codebase / skills / recipes / tools / memory` 类 workspace destinations。
- 建立 context-dock destination contribution。
- Files / Diff / Search / Skills / Recipes / Memory 进入右侧 dock。

首批已落地：

- 左侧顶级 workspace destinations 已移除。
- 右侧提供 `context` launcher / handle。
- Search、active-session destinations、rail context 入口都打开到 Context Dock。
- Context Dock destinations 已进入 `lyra.contextDock.destination` contribution registry，首批内置入口由 workspace 插件贡献，launcher 按 `workspace / run / session` scope 渲染 read model。
- 左侧 active session 下不再嵌 workspace/run 快捷入口；Work Index 只表达 session 选择与状态。
- 左侧顶部不再暴露 workspace grep 假装全局 Search；文件搜索从 Context Dock 的 workspace scope 进入。

验收：

- 左侧只剩 global actions + cwd groups + sessions + global footer。
- 当前 session/cwd 的工具都从右侧进入。

### Phase 4: Split Workspace Navigation State

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

### Phase 5: Visual Pass

- 左侧按低心率 Codex-like 方向降噪。
- 右侧做 light Context Dock + Review Workspace 两种密度。
- 所有 surface 深度走 theme token 和 `DESKTOP_UI_POLISH.md` 的 shadow model。

验收：

- 左侧没有 session-scoped tools。
- 右侧可以折叠。
- Review/Diff 模式信息密度高但 chrome 克制。

## 9. Anti-Patterns

以下都是回归：

- 把 Files / Diff / Memory / Skills / Recipes 放回左侧顶级。
- 把 Project 做成带 opaque id 的主动资源，或维护 active project。
- Work Index 直接拼业务数据源，而不是消费 navigation read model。
- Context Dock 使用一份全局 active file / selected diff，切 session 后互相污染。
- 把 approval/question 只放右侧，导致用户在主叙事里看不到 agent 正在等什么。
- 用更多卡片、边框、圆角、hover 来制造“高级感”。
- 插件只贡献 slot，不声明 scope/placement，导致信息架构再次失控。

## 10. Decision Checklist

加一个入口或面板前，先回答：

1. 它是 global、session、workspace 还是 run scope？
2. 离开 active session/cwd 是否仍有明确语义？
3. 它是帮助用户“找工作”，还是帮助用户“做当前工作”？
4. 它应该在 Narrative 中完成，还是在 Context Dock 中展开？
5. 它是否会让左侧变成普通功能菜单？

判断结果：

| Answer                       | Placement                                    |
| ---------------------------- | -------------------------------------------- |
| global action                | Work Index or Footer                         |
| session selection / status   | Work Index                                   |
| run blocking action          | Agent Narrative + Work Index attention badge |
| workspace/cwd material       | Context Dock                                 |
| detailed file/diff/tool view | Context Dock                                 |

如果判断不清，默认不要放到左侧。
