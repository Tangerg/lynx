# 差距清单 · 他们有 / Lyra 没有（或做得薄）

> 读法：每条 = **是什么** → **谁在做（去读这里）** → **Lyra 现状** → **为何重要** → **落地方向（贴合 Lyra 架构）** → **工作量 · 风险**。
> file:line 为研究当时各仓库的读取结果，落地前复核。对照仓库均在 `~/Desktop/` 下。
> 优先级与总览见 [`README.md`](README.md)。

---

## P0 — 定义产品品类的硬伤

### G1 · 持久状态行（dense data row） 〔P0｜DESIGN.md §8 已要求〕

- **是什么**：app 底部一条常驻、密集的 mono 数据行：`[● running] [branch] [run_8f3a2c] [12,847 / 200k · 6.4%] [$0.0234] [↑ 18.2 t/s]`，数据缺失的槽位自动隐藏。
- **谁在做（去读这里）**：
  - **codex**（金标准）：`codex-rs/tui/src/bottom_pane/status_line_setup.rs:55-180` —— 有序的 `StatusLineItem` 列表（Model / Reasoning / CurrentDir / GitBranch / ContextRemaining / UsedTokens / FiveHourLimit…），**数据不可用即省略自身**，不留空槽。token 用 `status/helpers.rs:100` 的 `format_tokens_compact` → `1.2K`/`3.4M`；context 用 `status_controls.rs:345` 的 `percent_of_context_window_remaining` 钳到 0–100；git branch 异步查询并按 cwd 缓存。
  - **crush**：`internal/ui/model/header.go:135-185` —— cwd + LSP 错误数 + context% + credits + `ctrl+d` 提示，用 ` • ` 连接、右截断到宽度，富余处用 `╱╱╱` 填充。
- **Lyra 现状**：**没建**。`pages/AgentClientPage.tsx:6` 明写 "there is no bottom status bar"。遥测被分散塞进 composer footer：`components/chat/composer/ComposerFooter.tsx` + `status/pill.tsx:133-212`（客户端测的 TTFT / tokens-per-sec）+ context **ring**（非 spec 的 `12,847/200k·6.4%` 文本）+ 会话累计 `Σ tokens $cost`，**且只在 running 时可见**。DESIGN.md §8「the data row」白纸黑字要求了这条。
- **为何重要**：这是"观测一个 runtime"定位最直接的签名面。常驻可见（非仅 running）让用户随时知道分支/上下文余量/花费——dense agent 客户端的基本盘。
- **落地方向**：在 kernel shell 加一个 `statusbar` 命名 slot（24px，`grid` 末行），用插件贡献点喂槽位（与现有 contribution-point 一致）；槽位渲染走 `caption-mono` + tnum；数据缺失即不渲染该 slot。把 footer 里的 TTFT/context/cost 迁过去并改为常驻。
- **工作量 · 风险**：中 · 低（纯增量 + 复用现有遥测来源；注意 §5 effect 纪律：状态行订阅要 `getState()` 而非每 token 重订阅）。

---

### G2 · 只读工具折叠分组 〔P0〕

- **是什么**：把低风险的 read / grep / glob / list 等只读调用，**折叠成一行摘要**（"read 4 files, searched 2"），而不是每个工具一张卡。
- **谁在做（全场最普遍——5 家都做）**：
  - **opencode**：`packages/ui/src/components/message-part.tsx:801-806,936` —— `ContextToolGroup` 把只读上下文工具收成一行。
  - **cline**：`apps/vscode/webview-ui/src/.../ToolGroupRenderer.tsx:143-233`（`:188-201` 合并"已完成 + 正在流式"的项、按 path 去重，活跃项显示 `TypewriterText` "Reading foo.ts…"）。
  - **lobe-chat**：`AssistantGroup/components/WorkflowCollapse.tsx:184-213` —— 3 档（collapsed/semi/full）accordion，**流式中自动展开、完成自动折叠、待审批强制展开**。
  - **continue**：`GroupedToolCallHeader.tsx`（"Performing N actions"）。
  - **codex**：`exec_cell/render.rs:262-340`（`exploring_display_lines`：连续只读命令收成单个 "Read" 组）。
- **Lyra 现状**：`components/tools/ToolCard.tsx` **每个工具一张卡**；长 run 下几十张卡淹没对话。preview 注册表已存在（`components/tools/previews/index.tsx`），但**无分组层**。
- **为何重要**：所有对照里**密度收益最大**的单点改动。autonomous run 动辄几十次读取，分组是"可扫读"与"刷屏"的分水岭。
- **落地方向**：在 Item→message/block 投影里加一个"相邻同类只读工具"聚合（`core-reducer` 的 projection/fold 层最合适，纯映射不动 wire）。渲染加一个 `ToolGroupCard`：折叠态一行摘要、展开态列出子卡；活跃项保持可见（学 cline 合并 live+done）。分类用工具的 capability/risk（已有 risk 概念）判定"只读"。
- **工作量 · 风险**：中 · 低（复用现有 ToolCard/preview；分组是 reducer 投影 + 一个壳组件）。

---

### G3 · 多面板 / 并排工作区 〔P0〕

- **是什么**：chat 与工作区面板（terminal / diff / editor / browser）**并排可拖拽**，而非互相替换。
- **谁在做**：
  - **OpenHands**（最近邻）：`frontend/src/.../conversation-main/conversation-main.tsx:23-109` —— 左 chat + 右 tabbed workspace 的**可拖拽分隔**，宽度持久化到 `localStorage('desktop-layout-panel-width')`，右栏可折叠到 0（300ms transform/opacity），移动端变 bottom sheet。
  - **opencode**：`packages/app/src/context/layout.tsx` + `tabs.tsx` —— context 驱动的侧面板（file tree / review-diff / terminal）。
- **Lyra 现状**：view 升进**同一条顶 tab 并替换 chat body**（`components/chat/panel/WorkspaceViewBody.tsx`）——**永远没法"边看 chat 边看 diff"**。shell 是 `layout.css:62` 的 `248px minmax(0,1fr)` 两列网格。
- **为何重要**：这是"聊天框"与"工作站"的结构性分水岭。agent 改文件时，用户要一边看对话一边盯 diff/终端。
- **落地方向**：把主区从"chat XOR view"改成可选的"chat | 可拖拽分隔 | view"。kernel 加一个 split 容器（宽度持久化到 uiStore，bump version 丢旧值）；保留"全屏单面板"作为默认/折叠态。用 Radix 无对应组件——可用极小自写 resizer（属 §3 例外，注释说明）。**这是最大改动，建议单独立项、A/B/C 分批。**
- **✅ 已落地（Approach A — 用户选定）**：splittable 视图（diff/files/terminal/plan/timeline/run-summary/tools）可"在 chat 旁打开"——`chat | 自写 resizer | view` 并排，比例持久化到 uiStore `splitRatio`（0.25–0.75，`.default` 无需 bump version）。状态：sessionStore `splitViewId`（与 `activeMainView` 互斥）+ `openMainViewBeside`/`closeSplit`。触发：点工具卡"在视图打开"→旁开 diff/terminal；视图 header 经 `ViewPlacement` context 拿到"⇿ 在侧打开 / ✕ 关闭"（不动 tab 条）。app 视图（settings/notifications/…）仍全屏替换。
- **工作量 · 风险**：大 · 中（触 kernel 布局；但不碰协议/对话渲染）。

---

## P1 — 明显的 agent-client 缺口

### G4 · @-context / @-mention 注入 〔P1〕

- **是什么**：在输入框用 `@` 把文件 / 符号 / git / 终端 / URL 钉进 prompt 的上下文。
- **谁在做（签名能力）**：
  - **continue**（黄金标准）：`gui/src/.../AtMentionDropdown/index.tsx:160-380` + `getSuggestion.ts` —— 递归组合框：顶层列 context provider，选 submenu 型下钻过滤子列表（`enterSubmenu`），query 型补全后给内联 `<textarea>`；**插入前体积预校验** `isItemTooBig`（`index.tsx:182-260`，超限拒绝 + toast + 自动删掉 `@`）；选中项变 ProseMirror chip 节点。
  - **cline**：`ContextMenu.tsx` + `ChatTextArea.tsx:1400-1438` —— File/Folder/Problems/Terminal/Git-commits/URL，ripgrep 模糊搜索，**透明高亮 overlay** 在文本背后给 @-mention 上色，完整 a11y（`role=listbox/option`）。
- **Lyra 现状**：composer 有 `SlashSuggestions.tsx`（**仅** slash 命令），**完全没有 @file/@symbol**。
- **为何重要**：coding agent 的核心交互之一。没有它，用户只能靠自然语言描述要看的文件，低效且易错。
- **落地方向**：composer 加 `@` 触发器（与现有 slash 自动补全同构）。第一版：`@file`（走 workspace 文件列表 / ripgrep）；mention 渲染成 composer chip（已有 `composer-chip` token）。**务必抄 continue 的"插入前体积预校验"**——这是 agent 上下文的关键守卫。
- **工作量 · 风险**：大 · 中（需要文件检索数据源；输入层改造）。

---

### G5 · 真实终端 〔P1〕

- **是什么**：能真正看 agent 执行命令的**实时输出流**。
- **谁在做**：
  - **OpenHands**：`frontend/src/.../terminal.tsx`（xterm.js + fit addon），`conversation-tab-content.tsx:75-79` 每会话强制 remount 重置 buffer。
  - **cline**：`CommandOutputRow.tsx:86-102` —— 状态点（脉冲绿=running / 黄=pending）+ codeblock + 自动滚动、输出封顶 75px（展开 200px）、`>5` 行才给 `ExpandHandle`、控制字符可视化。
- **Lyra 现状**：**fixture 假数据**。`workspace-views/terminal.tsx:10` 是 `// TODO: surface real process metadata`，标题 "pnpm typecheck" / cwd / "1 error · 1 warning" 全硬编码；`Terminal.tsx:36` 硬编码 "tsc watching for changes…"；`lib/data/data.ts:134` 的 `HTTP_KEYS = ["terminal"]` 是唯一残留的 HTTP-GET stub。
- **为何重要**：autonomous agent 跑命令是常态，看不到真实输出就无法信任/干预。
- **落地方向**：依赖后端先能流式 command output（协议层）。前端用 xterm 或复用 bash preview 的渲染思路（已有 bash/shell stdout preview）；把假 metadata 换成 run 派生数据。**与 G3 并排面板天然搭配**（终端常驻侧栏）。
- **✅ 重估 + 已落地（docs/613）**：613 澄清 agent 命令输出**已在 wire 上**（`item.delta{toolOutput}` → `view.toolCalls`），**无需新 API**；交互式 PTY 是反向不变量、不做。终端视图改成只读"命令日志"——聚合本会话 command 类工具的 命令 + 输出 + exitCode（running 实时 tail）。`views/CommandLog.tsx` + `terminal.tsx`。
- **工作量 · 风险**：中 · 中（卡后端能力；前端渲染本身不难）。

---

### G6 · HITL 键盘化 + 分级 always-allow + mode segmented 〔P1〕

- **是什么**：审批的**人体工学**——键盘决策、记住"以后免提示"的分级开关、Plan/Ask/Agent 用 segmented 而非循环按钮。
- **谁在做**：
  - **OpenHands**：`shared/buttons/v1-confirmation-buttons.tsx:72-123` —— `⌘↩` continue / `⇧⌘⌫` cancel；HIGH/MED/LOW 风险，高危显式 banner。
  - **codex**：`bottom_pane/approval_overlay.rs:4-9,276-299` —— 选项带单键 `display_shortcut`，**关闭永不等于"继续"**（dismissal never = continue），必须显式决策。
  - **cline**：`AutoApproveModal.tsx` + `constants.ts:3-55` —— **父/子粒度的分级 always-allow**（"读项目文件"→"读所有文件"），`AutoApproveBar.tsx:24-63` 常驻摘要条；`buttonConfig.ts:32-256` 把"状态→按钮"做成查表；`ChatTextArea.tsx:1613-1641` Plan/Act 滑块、切换时带过 draft。
  - **cherry-studio**：`ToolApprovalActions.tsx:79` —— split-button「Run / 永远自动批准此工具」。
  - **crush**：`dialog/permissions.go:29-96` —— **审批时内嵌可滚动 diff**（边看变更边批）。
- **Lyra 现状**：审批卡**内容上是全场最富的**——`components/chat/message/cards/ApprovalCard.tsx` 有可编辑 JSON 参数（`:186-201`）、风险/scope/target/可逆性、"don't ask again" checkbox——但**无键盘快捷键、always-allow 未真正接线**；`Composer.tsx:141` 的 mode 是**点击循环的图标**，非 DESIGN.md `components.segmented-control` 要求的 Agent/Ask/Plan segmented。
- **为何重要**：keyboard-driven 定位下，审批是高频中断点；快捷键 + 分级授权直接决定流畅度。
- **落地方向**：(a) 给 ApprovalCard 绑 `⌘↩`/`⇧⌘⌫`（复用 composer 的键位约定）；(b) 把 "don't ask again" 升级成分级 always-allow（session scope 协议已支持 `remember`）；(c) mode 改 `Segmented`（`common/` 已有该原子）。
- **✅ 已落地（本批）**：
  - (a) `⌘↩` = 有 open approval 时批准（run 已 parked、无可发送），否则发送；`⇧⌘⌫` = 拒绝。命令式提交走新 `lib/agent/submitPendingApproval`（无卡片版，镜像 `useInterruptResume` 的 resume 调用 + 延迟 `resolveInterrupt` + module 级 in-flight 去重双击）；审批卡按钮加 `⌘↵`/`⇧⌘⌫` 提示。
  - (b) **always-allow 本就接好**——"don't ask again" checkbox 已转 `remember:{scope:session}`（AUX_API §6，按工具名记忆）；更细的参数级是 v1 不做的规则引擎域，已是协议天花板，无需再做。
  - (c) mode picker 循环图标 → `Segmented`（Agent/Ask/Plan 直选，DESIGN §components）。
- **工作量 · 风险**：中 · 低（多为接线 + 换原子；协议 `remember` 已具备）。

---

### G7 · 上下文预算可观测 + 一键压缩 〔P1〕

- **是什么**：常驻的上下文占用条（带阈值升级），以及接近上限时的"一键压缩对话"入口。
- **谁在做**：
  - **continue**：`mainInput/ContextStatus.tsx` —— %-条 60% 才显、被裁剪转红，**一键 compaction**；`StepContainer/ResponseActions.tsx:46` 过 80% 升级成警告。
  - **cline**：`ContextWindow.tsx` —— Radix Progress（used/max）+ hovercard 拆解（in/out/cache）+ **Compact 按钮 + 内联确认**；`TaskHeader.tsx:96-104` 成本 pill。
  - **OpenHands**：`context-window-section.tsx:26-37`（perTurnToken/contextWindow %）+ `budget-progress-bar.tsx:13-23`（>80% 转红）。
- **Lyra 现状**：仅 `status/pill.tsx` 的 context **ring 且只在 running 时**；**无压缩入口、无持久预算**。
- **为何重要**：长会话必然撞上下文墙；提前可见 + 主动压缩，避免静默截断/失忆。
- **落地方向**：与 G1 状态行合并呈现（持久 context%）；过阈值在 composer footer 给"compact"动作（协议若有 compaction/summarize 能力则接线，否则先做可见预算 + 阈值提示）。
- **工作量 · 风险**：中 · 中（可见性易；压缩动作依赖后端能力）。

---

## P2 — 打磨 / 对齐 / 快速止血

### G8 · 清死占位符 〔P2｜快速见效〕

假 UI 比"没有"更伤信任。逐项（全在 `frontend/src/`）：

| 占位符 | 位置 | 处理 |
|---|---|---|
| sidebar 搜索框接空 | `plugins/builtin/sidebar/.../search.tsx:6`（"Placeholder until global search lands"） | 要么接 G4 的检索、要么先撤掉只留 ⌘K |
| 附件 chip UI 已渲染但无管线 | `components/chat/composer/index.tsx:270`（"attachments pipeline isn't live"） | 后端就绪前隐藏入口，别露死 chip |
| 硬编码用户卡 "Jamie Doe" | `plugins/builtin/sidebar/.../footer.tsx:23-31` | 撤掉或换成真实/无 user 概念（协议本就无 user） |
| 终端假数据 | `workspace-views/terminal.tsx` / `Terminal.tsx:36` | 见 G5 |
| plan view 假 goal/ETA（item 是真的） | `workspace-views/plan.tsx:11`（hardcoded "est. 2 min"） | 拉真数据或撤掉假 header |
| Files "Stage all"/"More" 空按钮 | `workspace-views/files.tsx:40-46` | 接 handler 或撤掉 |

- **工作量 · 风险**：小 · 低。**建议最先做**——一轮把所有假 UI 撤/补干净，立刻提升可信度。

---

### G9 · 收 spec 漂移 〔P2｜DESIGN.md 多处已要求〕

实现偏离了自己的设计稿。先分清"真漂移（改代码）"与"有意决策（改 spec）"。

**✅ 已修（本批 G9）：**

| 项 | 处理 |
|---|---|
| ALL-CAPS 标签（~20 处：区域标题 / 表头 / badge — tools / run-summary / FilesChanged / PlanList / Approval+QuestionCard / Diagnostics / Shortcuts / Timeline / MessageOutline / ToolInspector / projects / tasks） | 用户选 **sentence-case**：去 `uppercase` + 宽 tracking，保留作者原文大小写（多为 sentence-case）。DESIGN.md §10 已更新。顺带删掉 PlanList 的假 `ApprovalNote`。 |
| 语义色首屏满饱和非暗调 | `globals.css :root` 首屏默认改暗调（`#f85149` 等），消除首屏闪色；主题运行时本就合规（lyra-dark 暗调 / lyra-light 饱和），仅 :root 默认漂移。 |

**✅ 已了结：**

| 项 | 处理 |
|---|---|
| mode 点击循环非 segmented | 已在 G6 改 `Segmented`（Agent/Ask/Plan 直选）。 |
| reasoning block 填充盒 vs 左边框 | 重估为**有意设计**——`ReasoningBlock` 是带 hover header + 流式自动展开的 polished 折叠面板，填充 surface-2 盒比左边框更贴合。改 DESIGN.md §components.reasoning-block 名实相符，不动代码。 |

**✅ 已确认为有意决策 — 已改 DESIGN.md 名实相符（不动代码）：**

| 项 | 决策 | DESIGN.md 已更新 |
|---|---|---|
| 侧栏默认展开 248px（非 rail） | 用户确认：session tree 是主导航，默认展开 | §4 Sidebar |
| 顶栏保留会话 tab（非弃用） | 用户确认：顶栏混排会话 tab + view tab | §4 Tabs |
| dark 上面板阴影 | 代码注释自承覆盖：cards-on-canvas 让浮起面板阴影可见且有意 | §5 + §10 |

- **工作量 · 风险**：小 · 低。原则（取其精华去其糟粕）：spec 与代码冲突时，先判"漂移 vs 决策"——决策就改 spec、别盲从被推翻的旧 spec。

---

### G10 · 流式打磨 〔P2〕

- **是什么**：更稳更省的流式渲染 + 状态文案 crossfade。
- **谁在做**：
  - **codex / crush**：`codex-rs/tui/src/streaming/controller.rs:1-37` + `crush/internal/ui/chat/streaming_markdown.go:10,230` —— **stable-prefix（已提交、永不重渲）+ mutable-tail（每帧重渲尾部）**，只在安全 markdown 边界切分，**表格 holdback 到 finalize**（新行会重排整列）。codex 还把不可变历史写进终端 scrollback（`insert_history.rs:1`）。
  - **opencode**：`message-part.tsx:190-220` 的 `createPacedValue`（按词块匀速揭示）；`tool-status-title.tsx` 的 "Reading…→Read" 共享前缀 width-animation crossfade。
- **Lyra 现状**：`MarkdownMessage.tsx` 用 streamdown 的 `parseMarkdownIntoBlocks` + 块级 memo（"只有尾块每帧重 parse"）——这正是 codex/crush 的 stable-prefix + mutable-tail。
- **✅ 重估：非缺口。** (a) 流式性能已由 streamdown 解决（块级 memo，尾块才重 parse），无需再造。(b) "Reading…→Read" 动词 crossfade 与 Lyra 工具卡的 **RPC-log 嗓音**（`{fn}` 签名 + 状态字形，DESIGN §8）相冲——opencode 是动词叙述、Lyra 是签名，刻意不同，不强加。

---

### G11 · 消息分支导航 `< i/N >` 〔P2〕

- **是什么**：编辑/重发产生多个分支后，在消息上 `< 2/3 >` 切换查看。
- **谁在做**：cherry-studio `SiblingNavigator.tsx`；assistant-ui `BranchPickerCount.tsx:7`（`branchNumber`/`branchCount` 一等状态）。
- **Lyra 现状**：`editAndRerunMessage` 走 `sessions.rollback`（**销毁**旧轮次后重跑），`forkFromMessage` 走 `sessions.fork`（**新会话/标签页**）。viewState 无 branch/sibling 概念。
- **✅ 重估：非缺口（有意的模型差异）。** Lyra 的"分支"= **fork-to-session**（每条探索是独立会话）；cherry/assistant-ui 的 inline `< i/N >` 是**保留同消息兄弟节点**的另一种模型。要在 Lyra 做 inline 分支，需后端**保留**被 rollback 销毁的轮次并按消息暴露兄弟链——既要后端配合、又与 fork-to-session 重复。属深思熟虑的模型选择，不强加；真要做是后端协调特性（届时归 BACKEND-DEPENDENCIES）。

---

### G12 · 文件树 / 工作区浏览 〔P2〕

- **是什么**：浏览 workspace、打开任意文件，而不只看"改动过的文件"。
- **谁在做**：OpenHands（内嵌 VS Code tab / file explorer）；opencode（file tree 侧面板）。
- **Lyra 现状**：仅"changed files"扁平列表（`workspace-views/files.tsx`），无工作区浏览。
- **落地方向**：一个 file-tree view（与 G3 并排面板搭配最佳）；数据走 workspace 能力。
- **工作量 · 风险**：中 · 低。

---

## 附录 A · 别抄进来的反模式（第一法则护栏）

对照仓库普遍背着历史债，Lyra 的插件+注册表架构恰好躲过了——**保持住，别在落地时抄回来**：

- **多套样式系统并存**：cherry（antd + styled-components + Tailwind + 半迁移的自有 kit + `legacy-vars.css`）、cline、continue 都是 Tailwind + styled-components 双轨。Lyra 已是纯 Tailwind 4，别回退。
- **god-switch 巨组件**：cline `ChatRow.tsx`（~1300 行）、opencode `prompt-input.tsx`（81KB）/ `message-part.tsx`（80KB）。Lyra 的 content-block 注册表就是为避免这个——继续用注册表，别堆 switch。
- **v1/v2 双组件树**：OpenHands（`chat/` + `v1/chat/`）、opencode（`*-v2.tsx`）、assistant-ui（deprecated 与 current 并存）都在迁移债里。第一法则禁止——要改就一次改对。
- **字符串匹配判状态**：OpenHands `get-observation-result.ts:18` 靠 content 里有无 `"error:"` 判错。Lyra 的 typed Item/status 更好，别退化。
- **stringly-typed payload 每帧 re-parse**：cline 每次渲染从 `message.text` 里 `JSON.parse`。Lyra 的 typed wire 更好。
- **状态 emoji 烧进 markdown 字符串**：OpenHands `get-action-content.ts:120`。用 typed 渲染，别拼字符串。

---

## 附录 B · Lyra 已经赢的（别动）

对照下来 Lyra 普遍**强于** lobe/agent-chat-ui、部分强于 cline/OpenHands 的地方——是护城河：

- **RPC-log 工具卡 + 15 类 typed preview**（`components/tools/ToolCard.tsx` + `previews/`）：bash/diff/read/grep/lsp_*/skill/task/glob/ask_user，远超"裸 JSON"。
- **可编辑参数的审批卡**（`cards/ApprovalCard.tsx:186-201`）：多家根本没有 inline 改参数。
- **消息操作矩阵**（`MessageContextMenu.tsx`）：copy(md/plain/code) / edit / edit-&-rerun(±file-restore) / fork / regenerate(±file-restore) / feedback。
- **真 shiki diff**（`workspace-views/DiffView.tsx`）：worktree-vs-branch、高亮、截断提示。
- **派生可观测**：Timeline 审计日志 + Run Summary digest + plan-progress banner + 客户端测 TTFT/tok-s。
- **主题工程**：12 主题 + `defineThemePlugin` + 实时 token 换 + 自定义主题编辑器 + 完整 settings + i18n(8 locale)。
- **⌘K 命令面板 / ⌘F in-chat find / checkpoints（file-restore，能力门控）**。
- **插件 + 扩展点 + 注册表架构**：避开了上面附录 A 的全部债。

---

## 附录 C · 增强项（非缺口，锦上添花）

- **OKLCH 种子色生成主题**：opencode `packages/ui/src/theme/color.ts`（`generateScale`/`themeToCss`/`applyTheme`）从种子色程序化生成 ~38 套主题。Lyra 已有 12 套 + 自定义编辑器；可加"从种子色生成"降低每主题成本。
- **diff split-view 切换 + 每文件 +/- 迷你条**：opencode `diff-changes.tsx:22-105`（按增删比例的 5 格 SVG 条）；OpenHands old/diff/new 三态切换；crush/codex 的 split 模式。Lyra diff 是单一 inline，可加 split 切换。
- **`content-visibility:auto` 逐消息**：assistant-ui `thread.tsx:345`，长会话滚动性能。
- **审批时内嵌 diff**：crush `dialog/permissions.go` —— 批准文件编辑时直接在卡里显示 diff（Lyra 审批卡可内嵌已有的 diff preview）。
