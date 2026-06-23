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

### ✅ T1.2　运行中实时反馈(`run.progress`:活动 / 成本 / 步数 / 计时)　【runtime + desktop · M】　**已实现**(activity + step + elapsed 计时器 + mid-run usage/cost)
> mid-run 用量补完:engine 在每个 LLM round 边界(`recordRound`)累计 token roll-up + cost,经新增的 `toolObserver.OnUsage` 回调 → `turn.UsageReported` 事件 → translator 投影成 `run.progress{usage}`(ephemeral,reuse `optCostUSD` 省略零成本)。累计口径与 `chatOutput` 一致,故 mid-run 预览与终态 `run.finished.result.usage` 对得上。前端早已消费 `progress.usage`(`onRunProgress` → `tokens.used` + cost,有 reducer 测试),纯属点亮既有暗特性 —— wire `RunProgress.usage` 形状不变,additive。
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

### ✅ T2.1　边跑边排队 + 插话(steer)　【desktop · M】　**已实现**(真·mid-run steer:运行中发消息注入活跃 turn,模型下一轮即读到)
> 真·steer 补完(用户签发,彻底治本,取代客户端排队):**core SDK** 给 tool-loop 加了一个通用 additive 钩子 `tool.Config.BeforeRound`(每个延续轮前调用,返回的消息追加在该轮请求里 —— 落在最新工具结果之后,memory 中间件随即持久化进历史;这是唯一正确的位点,注入 loaded history 会错序到 in-flight tool_call 之前)。**runtime**:turn 经 RunChatRequest.Steer 提供 SteerSource(进程 extension → runChatTurn 把它塞进交给 stream 的 ctx,保证送达,不赌平台 ctx-value 传播);drain 时发 `turn.SteerMessage` → translator 投影成 userMessage Item(实时显示 + 落 durable transcript,与 chat-memory 一致);新 wire `runs.steer{runId,message}`。**desktop**:运行中 Enter → `runs.steer` 注入当前 turn(run 已结束则 run_not_found 回退发新回合);**删掉**客户端 queueStore/chips/auto-drain(steer 严格更优,排队是后端没能力时的 stopgap)。next-turn flushSteering 仍作"最后一轮之后才到"的回退。
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

### ✅ T2.4　限流 / 429 退避　【runtime + desktop · M】　**已实现**(runtime 填 `retryable`+`retryAfterSeconds`、desktop 倒计时 + 退避感知 Retry 禁用 + `provider_error` 符号按模式拆分)
> 符号拆分补完(用户签发,彻底治本):`classifyRunError` 现按失败模式给每类一个稳定 run 级符号 —— `rate_limited`(429,retryable)/`invalid_api_key`(401·403,不可重试)/`timeout`(超时·连接,retryable)/`provider_unavailable`(5xx,retryable)/`provider_rejected`(400,不可重试;与 RPC 级 `invalid_request` 同名不同物,靠 `channel` 区分),其余落 `internal_error`。客户端只按 `type`(+`retryable`)分支,不再 substring-match `detail`。前端 `errorCopy` 给每个符号文案,API.md §8.4 列入。
- **竞品**:Claude Code `withRetry`(前 3 次重试不打扰、第 4 次起才显倒计时)+ 过载透明换模型(`query.ts:894` Opus→Sonnet,非阻塞提示);codex `rate_limits.rs` 75/90/95% 阶梯警告 + 换模型 nudge("永不再提");opencode `session-retry.tsx` 行内倒计时重试卡(非 toast)。
- **我方**:wire 有 `retryAfterSeconds`+`retryable`(`delivery/protocol/errors.go:34-38`)但**两端零消费**;runtime 也**从不填**(自审 R7),429/认证/超时全塌成一个 `provider_error` 靠 `detail` free-text 区分。
- **改法**:**runtime** 在 `infra/llm` 解析 provider `Retry-After` → 填 `retryable`+`retryAfterSeconds`,并把 `provider_error` 拆成稳定符号(`rate_limited`/`invalid_api_key`/`timeout`);**desktop** 据此做倒计时 + 退避感知的 Retry 禁用。

### ✅ T2.5　审批增强:⌘↩ bug 修复 + 持久 allow-RULE + 危险命令 banner　【desktop(+runtime)】　**已实现**(⌘↩/⇧⌘⌫ bug 修复 + 危险命令 banner + 持久细粒度 allow-RULE)
> 持久规则补完(用户签发 细粒度版,彻底治本):approval 域从"session 内存级、按 tool-name 键"换成持久细粒度规则引擎 —— `Rule{scope, tool, subject, decision}`,`subject` 按工具从被批准调用里提取(shell 的 command / 文件工具的 file_path),所以记的是「`npm run *` 在本 project」而非笼统整个 shell。匹配取最具体(session>project>global,再 exact>glob>任意),同特异度冲突取 deny。新 SQLite `approval_rules` 表 + 确定性 id 上 upsert;`interrupts.Resolution.Remember bool`→`RememberScope string`(三 scope 端到端全持久,不再只 session);新 wire `approval.listRules`/`forgetRule` 取代提案期的 listRemembered/forget;前端审批卡加 scope 选择器(session/project/global)、设置面板列规则按 id 删,8 locale 补齐。
- **竞品**:Claude Code 批准按钮上**可编辑 allow 规则**(`bashToolUseOptions.tsx`:`Yes, and don't ask again for: [npm run:*]` 预填可改)+ 拒绝带反馈("tell Claude what to do differently");crush `PermissionKey{session,tool,action,dir}`(按目录记,不是只按工具名);Kimi `DANGER_PATTERNS` **客户端危险命令 banner**(`rm -rf`/`sudo`/`curl|sh`/`dd of=`/`mkfs`/`chmod 777`,独立于后端);codex 拒绝时把自定义 decline 键和 Esc 撞键剥离,Esc 永远是稳定取消。
- **我方**:审批卡已强,但 **⌘↩ 快捷批准丢 edited-args + remember**(`submitPendingApproval.ts` 用裸 `{type:"approval",decision}` resume,自审 D6 实锤);remember 仅本会话、无持久 allow/deny 规则;无危险命令提示。
- **改法**:**⌘↩ bug 快修**(让键盘路径也带 editedArgs + remember,now 即可);**危险命令 banner**(纯前端正则、可移植 Kimi 列表);**持久 allow-RULE** 接 [`RUNTIME_COMPARISON.md`](./RUNTIME_COMPARISON.md) "细粒度权限规则"那条(runtime 出规则引擎、desktop 出配置 UI)。

---

## Tier 3 —— 锦上添花 / 多为 runtime 一行级

### ✅ T3.1　压缩上 wire　【runtime · S】　**已实现**
translator 现把 `turn.CompactBoundary` 投影成 `compaction` Item(`item.started`+`item.completed`,`droppedMessages`=压缩前后净减条数),前端 `foldCompaction` 直接接住渲染分隔条;新增 `protocol.ItemTypeCompaction` + Item 的 `summary`/`droppedMessages` 字段(API.md §4.3 已同步)。`MemoryUpdated` 仍内部消化 —— 前端无消费方,按 YAGNI 不发推测性 `custom` 信号。

### ✅ T3.2　loop/stuck 别拍平成 internal_error　【runtime · XS】　**已实现**
`classifyRunError` 现读 translator 透传的 `errCode`(经 `ErrorEvent.Code` 捕获):`AGENT_STUCK` → 稳定 wire 符号 `agent_stuck`(治本,按 code 判别而非 message 文本),不再塌成 `internal_error`;其余 engine error 仍走 provider-pattern 分类。前端 `errorCopy` 给了友好文案,API.md §8.4 列入 run 级错误类型。

### ✅ T3.3　session live status 别硬编码 idle　【runtime · S】　**已实现**
`sessionToWire` 现收 caller 算好的 status:running 来自 in-memory run 注册表,waiting 来自 open interrupts。`ListSessions` 批量取(一次 interrupts 查询 + 一次锁),单会话路径用 `liveStatus`,避免 N+1。前端 `SessionRow` 早已渲染 `StatusDot`+"Running/Needs input";statusbar 在 run 起始边也 invalidate sessions 列表,让"运行中"徽标即时亮起。

### ✅ T3.4　终态补 duration + maxBudget detail　【runtime · XS】　**已实现**
`RunResult` 新增 `durationMs`(承载 `TurnEnd.Duration`,任一终态可显"took 12.4s");`OutcomeMaxBudget` 现填 `detail`("spent $X of $Y budget",`TurnEnd` 回带 MaxBudget/MaxCostUSD 让文案精确)。前端 run-end 时间线优先显 `outcome.detail` 而非裸 type;API.md 同步 `RunResult.durationMs`。run-summary 早有客户端推导的耗时,wire 值更准且供任意消费方。

### ✅ T3.5　消息列表虚拟化　【desktop · M】　**已实现**(content-visibility,非 react-virtual)
真虚拟化(react-virtual,已装却不用)会拆 DOM,而 ⌘F(走 `.msg-content` TreeWalker + CSS Custom Highlight + scrollIntoView)、copy-all、stick-to-bottom 高度测量全依赖完整 DOM —— 拆了就回归这些既有强项(正是 §12.2 标"刻意"的原因)。改用 `content-visibility:auto` + `contain-intrinsic-size:auto 220px`:浏览器跳过视口外消息的 layout+paint(消长会话悬崖),但节点**留在 DOM 且可被 find-in-page 命中**(不同于 `hidden`),⌘F/复制/滚动锚定全照常;`auto` 记住首渲后的真实高度,滚动高度保持准确。老 webview 自动降级回当前行为。

### ✅ T3.6　web_search 富卡片接死代码　【desktop · S】　**已实现**(走 tool preview,非 fold-emit block)
真有生产者:`web_search` 工具(Tavily,opt-in)+ runtime 已 shape 成 `{results: WebSearchResult[]}`,只是退化成 JSON dump。fold-emit 一个 `search` content block 是错的层(kernel reducer 不该知道 chat 插件的 block 形状);工具结果走 `TOOL_PREVIEW`(同 shell/grep/edit)才对 —— 加 `web_search` 预览渲染卡片网格(url 推 domain)。`SearchResults`+类型移到共享组件层(`@/components/tools/previews`),live tool preview 与休眠的 `search` content block 共用一份;推测性 `time` 字段(wire 无来源)删除,改用 url 做 key。`code`/`checkpoint` content block 暂留作 cards-in-prose + citation 的将来面(无 emitter)。

### ✅ T3.7　首次启动引导　【desktop · M】　**已实现**(API-key 引导,非 device-code)
welcome 屏检测 keyless(`useProviders` → 无 saved key),用一张 setup 卡替掉那些死路建议:"接入一个模型 provider" + 按钮深链直达 Settings 的 providers pane。`SettingsPage` 新增一次性 `settingsPane` target(transient、不持久的 sessionStore 字段),mount 时消费并清除,之后手动开仍落第一个 pane。runtime 用 API key(无 OAuth,§6.2),故是"贴 key"而非 device-code 流;文案覆盖 8 个 locale。

### ✅ T3.8　流式平滑(runtime 侧)　【runtime · M】　**已实现**(机会式合批,未动背压契约)
`Events()` 迭代器现把**已缓冲在 turn channel 上**的同类文本 delta(MessageDelta/ReasoningDelta)合并成一个再 yield —— 非阻塞 drain + 一格 lookahead(中途取到异类事件 park 进 `spill`,下一轮 yield,保序)。效果:负载下(生产者快过 SSE 消费者)1-token-1-frame 量塌缩 → hub 丢事件率降 → 少"跳";涓流仍逐 token 即时 yield(零延迟,只合并已排队的);durable transcript 不变(item.completed 仍带全文,delta 本就 ephemeral 无 SSE id)。**有意不动** hub 的 drop-on-slow 策略 —— reconnect-to-recover(durable replay + items.list backstop)是设计,这里打的是驱动丢弃的"量"而非策略。

---

## Runtime 侧 lever 汇总(很多根因在这,集中看)

| Lever | 现状 | 影响 | 工 |
|---|---|---|---|
| emit `run.progress`(activity/usage/step)+ plumb maxSteps | ✅ activity/step(toolStart)+ usage/cost(每 round 边界)已 emit;maxSteps 现真 plumb 进 turn(StartRunRequest→turnBudget,engine round-loop 在到顶时净停)+ 收尾 `outcome.maxSteps`(治了"accept 了但 ignore"的债;单位=工具轮,mid-run `N/M` 计数面待 step-unit 对齐再 surface) | 解锁实时活动/成本/步数(T1.2) | M |
| 首条消息自动标题 | 注释承诺、无代码 | 纯赢、无协议改(T1.4) | S |
| toolCall Item 上盖 `safetyClass` + 填 `ApprovalPayload.risk/reason` | ✅ 门禁的安全类现盖在 live toolCall Item(started+completed)+ 审批 prompt 带 risk/reason(write→medium/exec→high) | 审批卡显风险(零前端改,已消费 payload.risk/reason);工具卡 SafetyClass 就位 | S |
| 持久细粒度审批规则(T2.5) | ✅ approval 域规则引擎 + SQLite + `approval.listRules`/`forgetRule` | 跨会话/项目/全局记住 allow·deny,subject-glob 粒度 | M |
| 投影 compaction Item(T3.1) | ✅ 已 emit;MemoryUpdated 仍内部消化(无消费方) | 前端 CompactionBlock 接住 | S |
| stuck 信号透传(T3.2) / live session status(T3.3) / duration + maxBudget detail(T3.4) | ✅ 全部已透传/已填 | 多为一行级 | XS-S |
| 填 retryable/retryAfterSeconds + 拆 provider_error 符号(T2.4) | 字段在、从不填 | 客户端做正确退避 | M |
| 投影 todos 到 `state.snapshot{todos}` | ✅ todo_write 成功后 observer 读 todo store 发 `turn.TodosUpdated` → translator 投影 `state.snapshot{todos}`(前端 `useSharedState("todos")` 早已消费,零前端改) | 任务清单成一等面板 | M |

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
