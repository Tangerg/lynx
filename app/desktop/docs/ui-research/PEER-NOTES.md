# 对照仓库速查卡

> 每张卡 = 技术栈 + 最值得偷的点（带 file:line）+ 落地时该去读的关键文件。
> 仓库均在 `~/Desktop/`。差距如何映射到 Lyra 见 [`GAP-CATALOG.md`](GAP-CATALOG.md)。
> 排除 `portai*`（用户要求）。

---

## OpenHands —— 最近邻（autonomous coding agent）

- **栈**：React 19 + React Router 7 + Tailwind 4 + HeroUI + Zustand + React Query；Monaco（编辑器）、xterm.js（终端）、socket.io。
- **最值得偷**：
  1. **action + observation 融合模型**：`components/v1/chat/event-message.tsx:189-261` —— 渲染 agent 的"动作"与"观察结果"而非聊天轮；observation 按 `action_id` 找到对应 action，把"思考(prose)"放在结果卡上方。**直接对应 Lyra 的 Item→message/block 投影**。
  2. **per-tool 纯函数渲染管线**：`get-event-content.tsx`（title）/ `get-action-content.ts`（detail markdown）/ `get-observation-result.ts`（success/error/timeout，按 exit_code）——wire shape 与渲染分离，正是 Lyra 的 projections-vs-fold 分层。
  3. **可拖拽多面板**：`conversation-main/conversation-main.tsx:23-109`（宽度持久化、折叠到 0、移动端 bottom sheet）。→ G3
  4. **HITL 风险门控 + 键盘**：`shared/buttons/v1-confirmation-buttons.tsx:72-123`（⌘↩/⇧⌘⌫、HIGH/MED/LOW banner）。→ G6
  5. **diff 三态**：`features/diff-viewer/file-diff-viewer.tsx:151-251`（Monaco old/diff/new、按改动类型 side-by-side vs inline、折叠手风琴懒加载）。
- **别抄**：`chat/` 与 `v1/chat/` 双栈、字符串匹配 `"error:"` 判状态（`get-observation-result.ts:18`）、状态 emoji 烧进 markdown。
- **去读**：`frontend/src/components/v1/chat/`、`features/conversation/`、`features/diff-viewer/`。

---

## cline —— 审批流 + checkpoint 范本（VSCode webview）

- **栈**：React 18 + Vite + Tailwind 4 over `--vscode-*` 桥接（`theme.css:1-90`）；react-virtuoso；混用 Radix/HeroUI/styled-components/webview-toolkit。
- **最值得偷**：
  1. **三 scope checkpoint restore**：`CheckmarkControl.tsx:226-285`（Files & Task / Files Only / Task Only，时间线书签）——真·工作区时间旅行。
  2. **footer 锚定 HITL + 状态→按钮查表**：`buttonConfig.ts:32-256`（每个任务状态映射 primary/secondary 文案与动作），审批不在滚动流里。→ G6
  3. **只读工具分组**：`ToolGroupRenderer.tsx:143-233`（合并 done + live、按 path 去重）。→ G2
  4. **分级 auto-approve + 常驻摘要条**：`AutoApproveModal.tsx` + `constants.ts:3-55`（父/子粒度）+ `AutoApproveBar.tsx:24-63`。→ G6
  5. **@-context 系统**：`ContextMenu.tsx` + `ChatTextArea.tsx:1400-1438`（files/folders/git/problems/terminal/URL via ripgrep + 高亮 overlay + a11y）。→ G4
  6. **命令输出行**：`CommandOutputRow.tsx:86-102`（状态点 + 封顶滚动 + 控制字符可视化）。→ G5
- **别抄**：`ChatRow.tsx` 1300 行 god-switch、`ChatTextArea.tsx` 56KB、三套样式、从 `message.text` 每帧 JSON.parse。
- **去读**：`apps/vscode/webview-ui/src/components/chat/`、`.../settings/AutoApprove*`、`.../common/CheckmarkControl.tsx`。

---

## opencode —— 现代设计 + 主题工程（SolidJS）

- **栈**：SolidJS + Kobalte（headless a11y）+ `data-slot`/`data-component` CSS + Tailwind 混合；desktop = Electron(electron-vite)；`@pierre/diffs`（worker 虚拟化 diff）；Shiki。
- **最值得偷**：
  1. **只读工具收成 context 组**：`packages/ui/src/components/message-part.tsx:801-806,936`。→ G2
  2. **HITL 是键盘优先的内联 dock（非 modal）**：`packages/app/src/pages/session/composer/session-question-dock.tsx`（radio/checkbox 向导、进度段、Cmd+Enter/Esc、draft 缓存）、`session-permission-dock.tsx`（Deny / Allow always / Allow once + 匹配 glob）。→ G6
  3. **两层语义主题 + OKLCH 生成 ~38 套**：`packages/ui/src/styles/theme.css`（OC-2 token：`--surface-*`/`--text-*`/`--syntax-*`/`--surface-diff-*`）+ `packages/ui/src/theme/color.ts`（`generateScale`/`themeToCss`/`applyTheme`，`light-dark()` 切换）。→ 增强项 C
  4. **匀速流式揭示 + 状态 crossfade**：`message-part.tsx:190-220`（`createPacedValue`）、`tool-status-title.tsx`（"Reading…→Read" 共享前缀 width-animate）。→ G10
  5. **虚拟化 timeline + 稳定 key 行复用**：`message-timeline.tsx`（`virtua/solid`，行 tag：UserMessage/AssistantPart/Thinking/DiffSummary/TurnDivider…）。
  6. **proportional +/- 迷你条**：`diff-changes.tsx:22-105`。→ 增强项 C
- **别抄**：`prompt-input.tsx` 81KB / `message-part.tsx` 80KB（SRP）、`v1`/`v2` 双树、`data-slot` 让样式远离 JSX。
- **去读**：`packages/ui/src/components/`、`packages/app/src/pages/session/`、`packages/ui/src/theme/`。

---

## continue —— @-context + 编辑器主题桥（webview GUI）

- **栈**：React 18 + Redux Toolkit + TipTap(ProseMirror) + Headless UI + Heroicons + react-markdown/lowlight；Tailwind 4（+ styled-components 遗留）。
- **最值得偷**：
  1. **递归 @-context 组合框**：`gui/src/.../AtMentionDropdown/index.tsx:160-380` + `getSuggestion.ts`（submenu 下钻、内联 query provider、**插入前体积预校验** `isItemTooBig` `:182-260`）。→ G4（最强范本）
  2. **编辑器 var → token-array-with-fallback 桥**：`styles/theme.ts`（`THEME_COLORS`）+ `tailwind.config.cjs:38`（`varWithFallback()`，`preflight:false`）——整套 UI 免费继承编辑器主题。
  3. **apply-to-file diff 生命周期**：`StepContainerPreToolbar/ApplyActions.tsx`（closed→Apply / streaming→Applying / done→diff 计数 + accept/reject，绑 ⌘⇧⌫/⌘⇧⏎）。
  4. **模板驱动的工具叙述**：`ToolCallDiv/ToolCallStatusMessage.tsx`（per-tool `wouldLikeTo`/`isCurrently`/`hasAlready` + Mustache 插值 args）。
  5. **上下文预算 + 一键压缩**：`mainInput/ContextStatus.tsx`（60% 才显、裁剪转红）+ `ResponseActions.tsx:46`（80% 升级警告）。→ G7
- **别抄**：Tailwind + styled-components 双轨、HITL accept/reject 脱离 inline 工具调用、`window.postMessage` 绕过 action 层、魔法 z-index `200001`。
- **去读**：`gui/src/components/mainInput/`、`.../StepContainer/`、`styles/theme.ts`。

---

## cherry-studio —— 桌面多 LLM 客户端（设计密度）

- **栈**：Electron + React 19 + antd 5 + styled-components 6（半迁移到自有 `@cherrystudio/ui`）；Redux Toolkit；TanStack Router/Query；CodeMirror/Shiki；`@pierre/diffs`。
- **最值得偷**：
  1. **typed per-tool 渲染器**：`MessageAgentTools/`（Read/Edit/MultiEdit/Bash/Grep/Glob/Task/Skill/TodoWrite…）+ `chooseTool.tsx:26` 按名分发 + `ToolHeader.tsx:45,121`（per-tool icon/label + 一行参数摘要）。
  2. **折叠组浮现"活跃工具"头 + 自动滚动**：`ToolBlockGroup.tsx:77,193`。→ G2
  3. **inline diff + unified/split 切换**：`EditTool.tsx:17-40`（`@pierre/diffs` FileDiff + shiki）。→ 增强项 C
  4. **split-button HITL**：`ToolApprovalActions.tsx:79`（Cancel / Run▾「永远自动批准此工具」+ 执行中 Abort）。→ G6
  5. **流式 markdown 引擎**：`Markdown.tsx:210-256`（smooth-stream + append-only 块封存，避免 O(n²) re-parse）。→ G10
  6. **TTFT-excluded 吞吐**：`MessageTokens.tsx:73`（throughput 分母正确剔除 TTFT）。
  7. **分支导航 `< i/N >`**：`SiblingNavigator.tsx`。→ G11
- **别抄**：antd + styled-components + Tailwind + 自有 kit + `legacy-vars.css` 四路样式债、per-message Redux 订阅在流式速率下的 re-render 风暴、`dangerouslySetInnerHTML` 渲染工具输出。
- **去读**：`src/renderer/pages/home/`、`.../MessageAgentTools/`、`.../Inputbar/`。

---

## lobe-chat (lobehub) —— chat UX 抛光基准

- **栈**：Next.js + React 19 + antd 6 / antd-style 4（CSS-in-JS）；全部原子来自 `@lobehub/ui` v5；`@lobehub/editor`(Lexical) composer；cmdk；react-hotkeys-hook。
- **最值得偷**：
  1. **流式感知 workflow accordion**：`AssistantGroup/components/WorkflowCollapse.tsx:184-213`（collapsed/semi/full 三档，**流式自动展开、完成自动折叠、待审批强制展开**，shiny-text headline + 实时计时器）。→ G2
  2. **per-tool 自定义 inspector 注册表**：`Tool/Inspector/index.tsx:75`（按 `(identifier, apiName)` 解析专属渲染器：RunCommand+ANSI / EditLocalFile / Grep）。
  3. **注册表化 composer ActionBar**：`ChatInput/ActionBar/config.ts:19`（model/tools/search/params/token-counter 作 keyed action map + 溢出折叠）——镜像 Lyra 贡献点。
  4. **token-first 主题**：`AppTheme.tsx:158-176`（`customTheme={{primaryColor, neutralColor}}` 枚举 + `motionUnit` 密度档 + 用户可选字体/高亮/mermaid 主题）。
  5. **"无硬线"姿态**：`Block variant="outlined"` / `Accordion variant="borderless"`，surface/fill ladder 而非 1px 线——与 Lyra DESIGN.md 一致。
- **别抄（与 dense 工程定位冲突）**：consumer-rounded 圆角气泡 + 用户气泡/助手全宽不对称读着软；每个 headline/toggle 都上 motion 入退场动画（Lyra 刻意 gate）；edit 用 modal 而非 inline 打断键盘流；antd-style 运行时 CSS-in-JS 成本 vs Lyra 静态 Tailwind；30+ 设置路由的 consumer SaaS 体量别整体引入。
- **去读**：`src/features/Conversation/`、`src/features/ChatInput/`、`src/styles`、`AppTheme.tsx`。

---

## assistant-ui —— React chat 组件范式（偷模式不偷皮）

- **栈**：core 平台无关 primitives + 薄 web 包装 + store 绑定层 + shadcn 风格 styled 层（纯 Tailwind v4，`aui-*` 仅是覆盖 hook）。
- **最值得偷（组件 API）**：
  1. **name-keyed tool-UI 注册表**：`packages/core/src/.../scopes/tools.ts:19`（`setToolUI(name, render, {standalone})`，解析顺序 by-name→MCP→by_name→Fallback→Override，`MessageParts.tsx:391,572`）；renderer 拿到 `args/argsText/result/isError/status` + `addResult/resume/respondToApproval`。**1:1 对应 Lyra 的 content-block 注册表 + HITL resume**。
  2. **离散 parts 数组 + 单 `switch(part.type)`**：`packages/core/src/types/message.ts:138`（text/reasoning/tool-call/source/image/file/data/generative-ui/audio）+ `MessageParts.tsx:361`——Lyra Item→block fold 的干净目标。
  3. **render-prop `<Parts>{({part})=>…}` 带 enriched part**：`MessageParts.tsx:660`（tool UI 元素 + action 回调预绑）。
  4. **`ToolFallback` 复合可折叠 + inline HITL Allow/Deny**：`packages/ui/.../tool-fallback.tsx:274,331`（自动路由 approve/resume/addResult、requires-action 自动展开）。→ G6
  5. **`createActionButton(name, useHook→null=disabled)` + `ActionBar` autohide/interaction-lock**：去掉每按钮 enable/disable 样板 + "popover 开着保持 bar 可见"竞态。
- **别抄**：双 grouping 系统（`groupParts.ts` 树 + 旧 `ChainOfThought` 并存）、`useInlineRender` per-tool 抛 Zustand store、deprecated 与 current API 并存——正是第一法则禁的债。**取 parts-model + 单一 tool 渲染路径即可。**
- **去读**：`packages/core/src/react/primitives/`、`packages/core/src/store/scopes/`、`packages/ui/src/components/assistant-ui/`。

---

## TUI 组：codex · crush · plandex —— 密度与可观测大师

- **栈**：codex = Rust/ratatui（`codex-rs/tui/src/`）；crush = Go/bubbletea/lipgloss（`internal/ui/`）；plandex = Go/bubbletea（`app/cli/stream_tui/`）。codex/crush 是强设计，plandex 简单。
- **最值得偷（密度/可观测，可移植到 GUI）**：
  1. **可配置、缺数据自省略的状态行**：codex `bottom_pane/status_line_setup.rs:55-180` + `status/helpers.rs:100`（`format_tokens_compact` → `1.2K/3.4M`）+ `status_controls.rs:345`（context% 钳 0–100）。→ G1
  2. **工具 = 一行 dense header `● Name mainParam (k=v…)` + 封顶/可折叠 body**：crush `chat/tools.go:608,623`（输出封顶 10 行 + "… N lines"）、codex `exec_cell/render.rs:103,371`（bullet 按 exit code 变色、`└ `-前缀、中间截断 `… +N lines (Ctrl+T)`）。→ G1/G2
  3. **stable-prefix + mutable-tail 流式 + 表格 holdback**：codex `streaming/controller.rs:1-37`、crush `chat/streaming_markdown.go:10,230`；codex 把不可变历史写进 scrollback `insert_history.rs:1`。→ G10
  4. **approve-with-diff-inline、explicit-decision-only HITL**：crush `dialog/permissions.go:29-96`（内嵌可滚动 diff，←/→/Tab 选 Allow/Allow-session/Deny）、codex `bottom_pane/approval_overlay.rs:4-9`（dismissal 永不等于"继续"）。→ G6
  5. **主题/深度自适应 diff**：codex `diff_render.rs:1-32,54-76`（行号 + gutter 符号 + `⋮` hunk 分隔 + per-hunk syntect + truecolor/256/16 回退，匹配 GitHub diff 色）、crush split 模式 `diffview/diffview.go:482`。→ 增强项 C
  6. **动态垂直空间预算**：crush `model/sidebar.go:62-127`（`getDynamicHeightLimits` 按重要度 round-robin 分配 files/LSP/MCP/skill 行）。
  7. **上下文作用域 keymap（优先级 + 重键校验）+ 优雅降级的提示 footer**：codex `keymap.rs:1-50` + `bottom_pane/footer.rs:22-32`（宽度紧张时先丢最不重要的提示）。
- **单点最大启示**：codex = 不可变历史进廉价层、只重渲小 live tail；crush = 动态预算观测面板空间；plandex = 重复多项进度收成一行（`view.go` `doRenderBuild`）。
