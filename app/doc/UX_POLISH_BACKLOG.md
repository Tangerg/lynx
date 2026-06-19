# UX 细节打磨 Backlog —— app/runtime + app/desktop

> **目的**：把"竞品在体验细节上做得好、我们缺/薄"的收敛项落成**可勾选、带源码证据、按层分优先级**的待办。配套前两篇能力对比 [`RUNTIME_COMPARISON.md`](./RUNTIME_COMPARISON.md) / [`DESKTOP_COMPARISON.md`](./DESKTOP_COMPARISON.md)——那两篇比"大件能力"(沙箱/权限/planner/插件/设计语言),本篇比"小处打磨"。
>
> **方法**：7 个 source-grounded subagent 逐维核实——5 个竞品(claude_code / codex / opencode / kimi-code / crush,源码 clone 均在 `~/Desktop/`)按统一的 15 维 UX 清单提取"值得偷的细节"+ 2 个自审(desktop 渲染面 / runtime 行为面),全部带 file 证据。竞品引用的路径相对各自 clone 根;我方路径相对 `app/runtime/internal` 或 `app/desktop/frontend/src`。
>
> **一句话元结论**：竞品的细节优势高度收敛在四件我们几乎全缺的事上——① 失焦完成通知 ② 运行中实时反馈 ③ 边跑边排队/插话 ④ 草稿与历史不丢。**根因一大半在 runtime 不发信号**(协议定义了 `run.progress` 等却从不 emit),不是前端不渲染。所有改动均为 additive,不 bump 协议版本(API.md §12),但触及 wire 的须按"协议同步 review"镜像 API.md + 前端类型。

---

## 我们已经不弱的(别再重造)

自审确认这些已达第一梯队,**不在本 backlog**:审批卡(risk 显示 + 改参 + 记住本会话 + plan/safe/balanced/yolo 模式切换,比多数竞品强)、`ReasoningBlock`(带"思考 Xs"时长 + 流式自展开自收起)、`PlanBlock` + 顶部 `PlanProgressBanner`、`useStreamReveal`(rAF 平滑 / 打字两模式 + 后流 drain)、Shiki/Mermaid/KaTeX/citation markdown 栈、工具卡 4 态 + 分组(`planRenderUnits`)+ 显式截断计数、`ContextBudget` + 低上下文警告 + `CompactionBlock`、⌘K 命令面板、Codex 式项目→会话树、⌘F 高亮搜索、复制(MD/Plain/Code)+ 时间戳、图片粘贴/拖拽/选择。

---

## Tier 1 —— 便宜 + 高频痛点(建议优先整批)

### ✅ T1.1　失焦完成通知(OS 通知 + 窗口标题 + 可选声音)　【desktop · S】　**已实现**(OS 通知部分;窗口标题/声音留作后续)
- **竞品**:**全员**同一套 focus-gated 模式——只在窗口**失焦**时通知,子 agent 永不打扰。opencode `packages/tui/src/attention.ts` `focusSkip(when∈{always,blurred,focused})`(通知默认 `blurred`、声音默认 `always`、subagent 永不);crush `internal/ui/model/ui.go shouldSendNotification`(要求 focus-reporting 且窗口**未聚焦**)+ `internal/ui/notification/`(Native/OSC99→OSC777/Bell/Noop 自动降级);Claude Code `hooks/useNotifyAfterTimeout.ts`(交互时间戳门控 + 6s 阈值);Kimi `terminal-notification.ts`(per-event-id 去重 + `condition:'unfocused'`)。
- **我方**:**完全没有**——无 `Notification` API、不改 `document.title`、无音效(自审 D15)。后台跑完的 run 静默无声。
- **改法**:Wails/浏览器 `Notification` + `document.visibilityState` 门控(仅失焦弹)+ run.finished/interrupt 触发;标题加 `●` working 指示;音效可选、默认关。**根 agent 才通知,子 agent 不**。

### ✅ T1.2　运行中实时反馈(`run.progress`:活动 / 成本 / 步数 / 计时)　【runtime + desktop · M】　**已实现**(activity + step + elapsed 计时器;mid-run cost 待 engine 改)
- **竞品**:Claude Code `✻ Percolating…(12s · ↓1.2k tokens · thought for 5s)`(token 30s 后才显、缓动计数、3s 无 token 渐红);codex 弹性分块 + `(12m 45s • ESC to interrupt)`;Kimi 本地插值 1s 计时器(`lastSnapshot + (now-observedAt)`,`.unref()`,仅活跃时跑);crush 活动动词作 label。
- **我方**:**runtime 从不发 `run.progress`**——类型全定义(`delivery/protocol/events.go:82-90`)、translator 零 emit(`translator.go:282-312`)、`Capabilities().Events` 都不广播(`server.go:134-140`);前端 `RunState.cost` reducer 算了**就丢、无组件读**,**无 elapsed 计时器**(自审 D10/D11)。
- **改法**:**runtime**(头号 lever,自审 R1)——在每个工具起点/回合边界 emit `run.progress{activity,usage,step}` 并加进 `Capabilities().Events`;顺带把 `maxSteps` 真正 plumb 进 turn(现在 `runs.start` 只传 `MaxCostUSD`,accept 了但不 enforce/surface)。**desktop**——渲染运行成本 + 本地 elapsed 计时器(`run.started` 起算,槽位已在 `RunStatus()`)。

### ✅ T1.3　Composer 草稿持久化 + ↑ 历史回溯　【desktop · S-M】　**已实现**(per-session 草稿 + 持久化 + ↑/↓ 历史)
- **竞品**:全员 per-cwd/session 历史(Kimi `md5(cwd)` keyed JSONL、consecutive-dedup;codex `chat_composer_history.rs` 召回置光标 EOL + Ctrl+R 反搜)+ 切会话保草稿(opencode `prompt/stash.tsx` 带 `DRAFT_RETENTION_MIN_CHARS` 阈值;codex `input_restore.rs` 连图片 URL 一起 rehydrate)。光标在首/末行才触发历史。
- **我方**:`composerStore` 是**单个全局、不持久**——切 tab 丢字/串字、刷新即没、**无 ↑ 回溯**(自审 D9:"actively loses user work")。
- **改法**:draft store 改 **per-session keyed + 持久化**;加 ↑/↓ 历史(光标 offset 0 才上、末尾才下),进出保存/恢复当前草稿。

### ✅ T1.4　首条消息自动起会话标题　【runtime · S】　**已实现**
- **竞品**:Claude Code/Kimi 用便宜模型一次性从首条消息生成,可 `/rename` 覆盖。
- **我方**:runtime **从不命名**——`domain/session/service.go:41,126-130` 注释说"auto-generated from the first user message",但 `Rename` 只在 `sessions.update`(手动)/`sessions.fork`(显式)被调;维护用 LLM 管线现成却没接。
- **改法**:首回合结束后异步派生标题调 `Rename`,纯后端、无协议改动。

---

## Tier 2 —— 中等投入、明显拉开体验

### ✅ T2.1　边跑边排队 + 插话(steer)　【desktop · M】　**已实现**(运行中排队 + clean-settle 自动发 + 可删队列 chip;真正的 mid-run steer 仍需 engine 注入,留作后续)
- **竞品**:**全员都有**。codex `chatwidget/input_queue.rs` 三层队列(下个工具后注入 / 回合末 / 排队),Esc 立即发;Kimi `Ctrl-S` 把队列+输入折成一批 `session.steer()` 注入当前回合,↑ 召回最后一条;crush `agent/agent.go drainQueueForStep` 把无 RunID 的排队折进当前回合;可编辑队列 chip。
- **我方**:`useChatSend.ts` 硬 `if(running) return;`(自审 D5 第 1 大 gap)——**只能停、再发**。
- **改法**:**desktop 先做** "运行中排队 + `run.finished` 后自动发"(纯前端,即可消掉最大痛点)。真正的 mid-run steer 需 **runtime/engine** 支持把用户输入注入活跃 turn——当前是 R 模型(park/resume),需评估 lynx-agent loop 能否接 steer;这步较重,先验证可行性再排期。

### ✅ T2.2　Diff 三件套:call-scoped + 行内 word-level + 行内语法高亮　【runtime + desktop · M-L】　**已实现**(runtime 填 call-scoped `FileEdit.diff`(go-difflib);desktop 行内卡 + 全屏 Shiki 高亮 + unified/split 切换 + word-level via Shiki `decorations`)
- **竞品**:Claude Code `StructuredDiff/Fallback.tsx generateWordDiffElements`(双色行内 word diff,`CHANGE_THRESHOLD=0.4` >40% 改动放弃词级,非选中 gutter);crush `diffview/` + `xchroma/chroma.go`(强制把 chroma 前景叠到 diff 背景上,高亮不和红绿打架;>120 列自动 unified→split);codex `diff_render.rs`(512KB/10000 行高亮护栏);Kimi `diff-preview.ts renderDiffLinesClustered`(客户端 LCS 聚簇 + gap 省略 + **流式时压住删除行/全删,直到 newText 到达,不闪红**)。
- **我方**:行内 `DiffPreview`(`chat/tools/previews/index.tsx`)**无语法高亮、无 @@ 头**;全屏 `views/DiffView.tsx` **只 unified、不能折叠/分栏**;且 diff **非 call-scoped**——edit/write 卡显示**整树** `useDiff()` 不是这次的 patch,read 卡是 re-query(§12.1 B/C 仍开放)。
- **改法**:**runtime** 填 `FileEdit.diff`(call-scoped,§12.1 C);**desktop** 行内加语法高亮 + word-level + 大 diff 护栏 + 全屏 split 切换。流式 edit 务必抄 Kimi 的"压删除行直到 newText"。

### ✅ T2.3　@file 提及 + 可点 file:line + 大段粘贴转附件　【desktop + runtime · M】　**已实现**(runtime `workspace.listFiles`/`readFile` 落地;desktop @file fuzzy 补全 + 大段粘贴转 chip + 可点 file:line→文件查看器;markdown 散文 linkify 留作后续 rehype 一轮)
- **竞品**:全员 @file 自动补全。crush `completions/completions.go applyNamePriorityFilter`(fuzzy 之上再按 exactName>prefix>pathSegment>fallback 四级稳定排序);Kimi `scoreCandidate`(100/80/50/30 + 目录加权 + dirs-first)+ delimiter-bounded @(含 `=`/引号)+ `@"带空格路径"` 自动加引号;`@file#L10-20` 行范围语法多家都有;大段粘贴 >N 行转 `[Pasted…]` 附件。
- **我方**:@file **全无**、输出里 file:line **不可点**(markdown `a()` 只开新 tab)、大文本粘贴直接灌输入框(只拦了 `image/*`)(自审 D8)。
- **改法**:主要 **desktop**。注意依赖:@file 的文件列表源需 runtime `workspace.listFiles`(目前是 613 提案、方法表未注册)——要么先推该方法,要么前端临时用现有 grep/已读文件兜底。可点 file:line + 大段粘贴转附件可独立先做。

### ✅ T2.4　限流 / 429 退避　【runtime + desktop · M】　**已实现**(runtime 填 `retryable`+`retryAfterSeconds`(429/5xx/timeout)、desktop 倒计时 + 退避感知 Retry 禁用;`provider_error` 符号拆分是 wire-value 变更,待用户签发,暂只做 additive 字段)
- **竞品**:Claude Code `withRetry`(前 3 次重试不打扰、第 4 次起才显倒计时)+ 过载透明换模型(`query.ts:894` Opus→Sonnet,非阻塞提示);codex `rate_limits.rs` 75/90/95% 阶梯警告 + 换模型 nudge("永不再提");opencode `session-retry.tsx` 行内倒计时重试卡(非 toast)。
- **我方**:wire 有 `retryAfterSeconds`+`retryable`(`delivery/protocol/errors.go:34-38`)但**两端零消费**;runtime 也**从不填**(自审 R7),429/认证/超时全塌成一个 `provider_error` 靠 `detail` free-text 区分。
- **改法**:**runtime** 在 `infra/llm` 解析 provider `Retry-After` → 填 `retryable`+`retryAfterSeconds`,并把 `provider_error` 拆成稳定符号(`rate_limited`/`invalid_api_key`/`timeout`);**desktop** 据此做倒计时 + 退避感知的 Retry 禁用。

### ✅ T2.5　审批增强:⌘↩ bug 修复 + 持久 allow-RULE + 危险命令 banner　【desktop(+runtime)】　**已实现**(⌘↩/⇧⌘⌫ 带 editedArgs+remember 的 bug 修复 + 客户端危险命令 banner;持久 allow-RULE 接 runtime 规则引擎,留作后续一轮)
- **竞品**:Claude Code 批准按钮上**可编辑 allow 规则**(`bashToolUseOptions.tsx`:`Yes, and don't ask again for: [npm run:*]` 预填可改)+ 拒绝带反馈("tell Claude what to do differently");crush `PermissionKey{session,tool,action,dir}`(按目录记,不是只按工具名);Kimi `DANGER_PATTERNS` **客户端危险命令 banner**(`rm -rf`/`sudo`/`curl|sh`/`dd of=`/`mkfs`/`chmod 777`,独立于后端);codex 拒绝时把自定义 decline 键和 Esc 撞键剥离,Esc 永远是稳定取消。
- **我方**:审批卡已强,但 **⌘↩ 快捷批准丢 edited-args + remember**(`submitPendingApproval.ts` 用裸 `{type:"approval",decision}` resume,自审 D6 实锤);remember 仅本会话、无持久 allow/deny 规则;无危险命令提示。
- **改法**:**⌘↩ bug 快修**(让键盘路径也带 editedArgs + remember,now 即可);**危险命令 banner**(纯前端正则、可移植 Kimi 列表);**持久 allow-RULE** 接 [`RUNTIME_COMPARISON.md`](./RUNTIME_COMPARISON.md) "细粒度权限规则"那条(runtime 出规则引擎、desktop 出配置 UI)。

---

## Tier 3 —— 锦上添花 / 多为 runtime 一行级

### ☐ T3.1　压缩上 wire　【runtime · S】
runtime 已算 `CompactBoundary`(前后条数,`turn/service.go:248-269`)却被 translator **直接丢**(`translator.go:116-117`);前端 `CompactionBlock` **就绪等着渲染**。只差 runtime 投影成 `compaction` Item 或 `custom` 事件(Kimi/Claude Code 显示 before→after tokens)。顺带 `MemoryUpdated`(已算的"存了笔记")也一起丢了,可做友好 `custom` 信号。

### ☐ T3.2　loop/stuck 别拍平成 internal_error　【runtime · XS】
runtime 已知 `AGENT_STUCK`(`turn/turn.go:396-397`),但 `classifyRunError`(`translator_outcome.go:31-58`)无匹配 → 落 `internal_error`,用户看到"内部错误"。加一个 `type:"agent_stuck"` 或透传 errCode,一行级。

### ☐ T3.3　session live status 别硬编码 idle　【runtime · S】
`sessionToWire`(`delivery/server/sessions.go:218-233`)硬编码 `SessionStatusIdle`;runtime 其实知道(track `s.runs` + open interrupts)。改成真实 running/waiting/idle,前端即可在会话树显示"运行中/等你审批"徽标。

### ☐ T3.4　终态补 duration + maxBudget detail　【runtime · XS】
`TurnEnd.Duration` 被捕获(`turn/turn.go:335,356`)却**无 wire 字段承载、丢弃**;`OutcomeMaxBudget` 产出时 **无 `detail`**(spec 的"花了 $4.20 / 上限 $4.00"从不填)。两个值都在手,补上即可。

### ☐ T3.5　消息列表虚拟化　【desktop · M】
`MessageStream.tsx` 全量 `messages.map` 进 DOM(自审 D13,§12.2 说是刻意)——长会话扩展悬崖。需虚拟化 + 滚出视口暂停动画(可借 Claude Code `OffscreenFreeze` 思路)。

### ☐ T3.6　web_search 富卡片接死代码　【desktop · S】
`SearchResults` 卡(及 `code`/`checkpoint` block)**已写好但 v2 fold 从不 emit** kind(§12.1 B),现退化成 JSON dump。让 fold 产出对应 block 即可。

### ☐ T3.7　首次启动引导　【desktop · M】
无 first-run wizard——keyless 新用户撞错误 banner 而非"先配 provider"(provider 设置已存在,只是埋在 Settings,自审 D14)。crush 的 750ms 最小 spinner、Kimi/crush 的 device-code 自动开浏览器 + 可复制 code 可借鉴。

### ☐ T3.8　流式平滑(runtime 侧)　【runtime · M】
runtime 1-token-1-SSE **无合批**(`turn/observer.go:150-153`),且 `hub.Append` 对慢订阅者**丢事件**而非节流(`hub.go:48-63`,超 `liveHeadroom=256`)——lossy-drop 比抖动更像"卡顿/跳跃"。可加 min-flush-interval 合批 + 重新评估背压策略(权衡 durable/ephemeral 契约)。前端 `useStreamReveal` 已够好,主要是 runtime 侧。

---

## Runtime 侧 lever 汇总(很多根因在这,集中看)

| Lever | 现状 | 影响 | 工 |
|---|---|---|---|
| emit `run.progress`(activity/usage/step)+ plumb maxSteps | 定义了从不 emit | 解锁实时活动/成本/步数(T1.2) | M |
| 首条消息自动标题 | 注释承诺、无代码 | 纯赢、无协议改(T1.4) | S |
| toolCall Item 上盖 `safetyClass` + 填 `ApprovalPayload.risk/reason` | 值已算、从不盖在 live Item | 审批卡/工具卡无需 join `tools.list` 即可显风险 | S |
| 投影 compaction / MemoryUpdated | 已算 CompactBoundary 却丢(T3.1) | 前端 CompactionBlock 就绪 | S |
| stuck 信号透传(T3.2) / live session status(T3.3) / duration + maxBudget detail(T3.4) | 都已知却丢/拍平 | 多为一行级 | XS-S |
| 填 retryable/retryAfterSeconds + 拆 provider_error 符号(T2.4) | 字段在、从不填 | 客户端做正确退避 | M |
| 投影 todos 到 `state.snapshot{todos}` | todo_write 仅 model-facing | 任务清单成一等面板(现仅 plan 一等) | M |

---

## 贯穿原则(从竞品收敛出的"为什么好")

- **focus-gated 通知**:只在用户离开(失焦)时打扰,子 agent 永不——所有桌面竞品都这么做,做错很烦人。
- **subtle vs loud 配色纪律**(crush 最典型):信息量数据(成本/token/cwd/时间戳)用灰,只有真告警(ERROR、>80% 上下文、危险命令、yolo)用饱和色——告警才显眼,正是因为别处克制。
- **快捷键 hint 全程从 binding registry 取**(Claude Code):改键,所有提示/cheat-sheet/tip 同步更新,绝不硬编码 label。
- **截断永远给"展开后有内容才给入口"**(Claude Code `isResultTruncated`):别给一个展开后空无一物的 affordance。
- **状态转移 narrate delta,不 re-dump**(crush todos:"completed 1, starting next" 而非重列全表)。

---

## 关联文档

- [`RUNTIME_COMPARISON.md`](./RUNTIME_COMPARISON.md) —— app/runtime vs 6 CLI/TUI agent 的大件能力对比(真 gap = OS 沙箱 + 细粒度权限规则/hooks)。
- [`DESKTOP_COMPARISON.md`](./DESKTOP_COMPARISON.md) —— app/desktop vs AionUi/Cherry/Cline 的桌面形态对比(领先在协议分离 + 薄核插件 + 单一设计语言)。
- 竞品源码 clone:`~/Desktop/{claude_code,codex,opencode,kimi-code,crush}`(后续抠具体实现可直接定位)。
