# Lyra — Agent 能力横向对比（全量重写 · 2026-06-17）

> **视角**：能力面（运行时 capability），不比 UI 样式。**基线**：lyra = Go agent-runtime backend（前端在 `/Users/tangerg/Desktop/lyra/`）。
> **本轮新增对比对象**：**eve**（Vercel 出品的 durable agent 框架）、**flue**（Cloudflare 系的 agent harness 框架）。这两者与 lyra 是**同一类——"跑 agent 的基础设施"**，比桌面 app 更贴近 lyra 的对标轴。
> **本轮变化**：lyra 自上次（2026-06-14）后能力又有提升（多模态图片输入**已接通**、plan-as-stance、idle/stall backstop、checkpoint 精修、schema-from-struct 等）；竞品仓库也都更新了（cline 一等团队、codex Code Mode、opencode ACP server、claude_code 27 hooks+MCP server…）。本文按**能力维度逐项对比**（session / 工具 / LSP / HITL / memory / durability / 扩展 / 自主性…），是 2026-06-17 的现状快照。
> **公平前提**：lyra / eve / flue 都是"基础设施"（框架/运行时）；桌面 app（Proma/AionUi/goose…）是 frontend+backend 打包，**前端/桌面 UX**（语音 ASR、全局热键）归 lyra 前端，不算 runtime 缺口。

---

## 0. 一句话结论

> **lyra 在「单次 agent turn 的执行质量 + 代码智能 + 安全」上是第一梯队**（LSP / 编辑安全 / fork+checkpoint / HITL 可跨重启 / A2A / 多 provider / OTel 三驾马车 全都有，且多项与 opencode/claude_code 齐平或独有）。
> **真正落后的仍是「自主性与触达」**——调度/自动化、远程 IM 桥接（**flue 的头牌**：17 个 channel 适配器）、多 agent **团队编排**（**cline 18 team 工具 / kimi 128 swarm**；lyra 已有并行委派、缺命名角色团队 + 消息板 + 生命周期）、hooks（**claude_code 27 / pi 27**）、OS sandbox、evals。
> **新对标 eve/flue 揭示一条新轴——「durability + 云原生部署 + 触达」**：eve 把"持久化"做成平台原语（Vercel Workflow，step 级 checkpoint、可停放数天），flue 把"channel 触达 + Cloudflare DO 持久化"做成头牌。lyra 已有跨重启 resume（snapshot rehydrate），但**没把它做成"任意停放 + 云部署 + 多 channel 触达"的产品级形态**——这是 eve/flue 给出的新参照。

---

## 1. 对比对象（三类）

### 1.1 Agent 运行时 / 框架基础设施 —— lyra 的最直接对标（本轮重点）

| 项目 | 出品 | 形态 | 一句话 |
|---|---|---|---|
| **lyra** | 本项目 | Go 后端运行时（自托管） | 协议层薄、业务层厚、传输可换；welded loop + LSP + HITL + fork/checkpoint + A2A |
| **eve** 🆕 | **Vercel** | TS 框架，编译成可部署 HTTP 服务（Nitro），主打 Vercel 云 | **filesystem-first** 授权 + **durability-by-default**（Vercel Workflow，step checkpoint、可停放数天）+ 7 channel + 5 sandbox + 一等 evals |
| **flue** 🆕 | **Cloudflare 系**（@earendil-works） | TS harness，编译成 Hono 服务，跑 Node / Cloudflare Workers+DO | **agent harness**（loop 委派给外部 pi-agent-core）+ **17 channel 适配器** + Durable-Object 持久化 + 渐进式 Skills；**无 HITL / 无长期记忆 / 无 LSP** |

> **关键洞察**：lyra / eve / flue 是"跑 durable agent 的基础设施"的三种取法——**Go 自托管 / Vercel 云 / Cloudflare 边缘**。三者都不是桌面 app；eve/flue 比下面两类更贴近 lyra。

### 1.2 本地优先桌面 agent

`Proma`（Electron+Claude Agent SDK）· `AionUi`（统一多 CLI agent）· `goose`（Block，Rust）· `cherry-studio` · `lobehub`

### 1.3 编码 agent / runtime

`claude_code` · `codex` · `crush` · `opencode` · `cline` · `OpenHands` · `plandex` · `harness9` · `pi`(pi-mono) · `kimi-code`

> 排除：纯库/框架（a2a-go / adk-go / eino / koog / spring-ai / `ai` SDK / go-sdk …）、catwalk（模型目录）、agent-chat-ui / assistant-ui（前端壳/组件库）。

---

## 2. lyra 现状能力基线（2026-06-17 实测）

**单 turn 执行**：agent loop（复用 lynx `agent/runtime` 的 `for{}`，不手写）· 工具循环 + **冲突感知并行执行（独占默认 / parallel / per-path keyed，上限 8）+ 并行子 agent 委派 + per-path 写锁** · **HITL R 模型**（park-on-interrupt + resume，可持久化/审计/**跨重启 rehydrate**）· **plan-as-stance**（agent/chat/plan 三模式塌缩成一个 loop + `exit_plan_mode`）· steering 注入 · **MaxBudget/MaxCostUSD + idle/stall backstop**（codex 式空闲超时，非总时长硬切）。
**防失控**：loop detection（SDK）· budget backstop · todo 校验 · **`.git` 子路径只读守卫** + **per-path 写锁** · **首终态胜出 CAS 不变量**（kill/自然完成竞态，本轮加固）。
**上下文**：压缩按消息数 **或 window-相对 token 触发**（默认模型 window×80%，catalog miss 回退 100k）· wholesale 摘要 + **结构化模板**（Goal/Progress/State/Decisions/Next）+ 保留最近 · **LYRA.md 长期记忆 + extractor 提取事实**（独有：竞品多数无跨会话记忆）。
**代码能力**：**LSP 6 操作 + diagnostics**（definition/references/implementation/hover/call-hierarchy/symbols，单 `lsp` 工具 + `lsp_diagnostics`，对齐 opencode/claude_code）· 编辑安全（read-before + stale 守卫 + per-path 锁）· fs/bash/web(fetch+search) · **model-facing todo（SQLite 持久化）** · **工具 JSON schema 从 struct 生成**（不手写漂移）。
**会话/状态**：Session→Run→Item · **fork + 影子 git 文件 checkpoint（gated 整库，seed from real .git，2MB/file cap）+ export/import** 三件套 · per-session cwd · **跨重启 durable resume**（type-tagged snapshot rehydrate）。
**集成**：MCP client（5 态生命周期 + auth 基座）· **A2A**（标准 agent-to-agent 跨 runtime）· Skills（project+global）· **多 provider×多 model（21 已接 / 38 adapter，显式配对）** · **多模态图片输入已接通**（inline base64，attachment 子系统已删）。
**委派**：subagent 3 种 spawn 模式（protected-only 作委派默认）+ **并行委派**（一轮内多 `task` 并发拉起隔离子进程，HITL→waiting result 不阻塞兄弟）。
**可观测**：OTel 三驾马车 → slog（vendor-neutral，去品牌 semconv）。

> **自 2026-06-14 已落地、别再当缺口的**：多模态图片输入（旧头号 todo，已接）、plan-as-stance、idle/stall backstop、checkpoint 精修、schema-from-struct、原子 memory.Replace、type-preserving snapshot、一批并发竞态加固（kill-race / cancel-park / concurrent-HITL）、**冲突感知并行工具执行 + 并行子 agent 委派**（修复了 observer 装饰器吞掉并发声明、令一切实为串行的 bug + tool-loop panic 就地兜底）。

---

## 3. 逐维度对比（核心）

> 记号：✅ 有且第一梯队 · 🟢 有 · 🟡 部分/弱 · ❌ 无。每维度给 **lyra 状态 + 谁领先 + 判断**。

### A. 执行核与状态

**3.1 Session / 状态管理** ✅
lyra：Session→Run→Item 三层 + per-session cwd + **fork + 影子 git 文件 checkpoint + export/import**。
field：**opencode 同级**（shadow-git restore + 一等 `Session.fork`/children + 分享）；claude_code = content-store copyFile（非 shadow-git、无 undo）；codex = 仅会话 JSONL/SQLite（**不碰工作树文件**）；crush = 仅 per-file SQLite 版本；plandex = git-backed plan 版本 + rewind；**eve/flue 无 fork**。
判断：**lyra 与 opencode 并列最强**——"整库文件 checkpoint + 会话 fork + export/import"三者同时具备，全场罕见。

**3.2 Agent loop / 工具循环 / 并行执行** ✅
lyra：复用 SDK `for{}` welded loop + **冲突感知并行执行** + per-call 取消 + **per-path 写锁**。一轮内的工具调用按冲突分段执行：默认**独占**（保守，单独跑），声明 parallel 的（读 / web / LSP / **子 agent**）并发跑，声明 **per-key** 的（文件编辑按路径）同路径串行、异路径并行；并发上限 8，结果按调用顺序回填，单个调用 panic 就地兜住不波及兄弟。**并行委派**：模型在一条 assistant 消息里发多个 `task` 即并发拉起多个**隔离**子 agent（各自独立 blackboard + session；子 agent HITL 停放返回 `{"status":"waiting"}`，不阻塞兄弟）。设计取 kimi 资源冲突图 + claude_code `isConcurrencySafe` 分区里最优雅的做法。
field：人人都有 loop + 并行工具（claude_code `isConcurrencySafe` 分区、kimi 128-swarm 资源冲突图、codex spawn/wait）。**新范式**：codex **Code Mode**（模型写 JS 在 V8 里 `await tools.*()`，一步编排多工具）、eve 同类 "code mode" workflow 工具。flue 把 loop **委派给外部 pi-agent-core**（自己只管 loop 外的一切）。
判断：齐平——并行工具 + 并行子 agent 委派本轮补齐；Code Mode 是值得关注的新方向（lyra 暂不需要）。

**3.3 Durability / 跨重启恢复** 🟢（eve/flue 是头牌，lyra 有但形态较轻）
lyra：**type-tagged snapshot + rehydrate**——HITL 停放可跨重启 resume（从持久化 snapshot 重建 parked process）。
field：**eve = 最深**（每个 turn 跑成 Vercel Workflow durable workflow，**step 级 checkpoint**、崩溃/重部署 replay 已完成步、HITL/OAuth/subagent 等待**零算力停放数天**）；**flue = turn-journal**（before_provider→provider_started→tool_request→committed 四相 + typed resume 模式 + 流/工具调用崩溃修复，Cloudflare 上 DO-SQLite + fiber `schedule()` 唤醒）；codex 会话级 resume/fork/rollback。
判断：**lyra 有真 durable resume，但只覆盖"HITL 停放点"**；eve/flue 把 durability 做成**贯穿每个 step 的平台原语 + 可停放任意时长**。这是 eve/flue 给的最大新参照（成本：高，且与 lyra 自托管定位相关，非必跟）。

### B. 代码能力

**3.4 LSP / 代码智能** ✅（lyra 第一梯队）
lyra：**LSP 6 操作 + diagnostics**，config-driven server table，平台无关。
field：**opencode 最富**（~22 个自动安装的 server，9 ops）；**claude_code**（9 ops）；**crush**（有，含 lsp_diagnostics/references/restart + sourcegraph）；**codex 零 LSP**（仅 fuzzy 文件名 + BM25-over-工具描述）；cline 仅插件级 TS 示例；**plandex/harness9/pi/kimi/OpenHands/eve/flue 全无 LSP**。
判断：**LSP 是 lyra 的强项**——仅 opencode/claude_code/crush 同级，**eve/flue 完全没有**（它们靠 sandbox 里的 grep/bash）。

**3.5 工具集** ✅
lyra：fs/bash/web(fetch+search)/todo + **schema 从 struct 生成**（防漂移）+ read/write/edit 参数对齐 peer（`file_path`）。
field：大同小异（read/write/edit/grep/glob/bash/web）。**编辑写法分两派**：`old_string/new_string`（lyra/claude_code/crush）vs **V4A apply_patch**（codex Lark 文法 / opencode 独立 apply_patch）。
判断：齐平；lyra 守"guarded 单编辑"被验证正确（claude_code 同款）。

**3.6 编辑安全** ✅（lyra 第一梯队）
lyra：read-before-edit + mtime stale 守卫 + **per-path 写锁** + `.git` 只读守卫。
field：claude_code/crush（read-before + mtime，**无 path 锁**，竞态已知）；**opencode 最富**（9 级级联 replacer + per-path Semaphore(1) + 不成比例匹配守卫）。
判断：**lyra 与 opencode 同为最严**（read-before + stale + per-path 锁三件套；多数 peer 缺 path 锁）。

**3.7 语义检索 / repo-map** ❌（全场最稀缺）
lyra：有 `rag/` pipeline 但**未喂给 agent 选文件**。
field：**plandex 是唯一真有 tree-sitter 符号图的**（10+ 语言，defs/signatures/行号）；cursor/windsurf（向量，非本地）；其余 OpenHands/pi/kimi 都是 glob+grep（tree-sitter `NotImplementedError` 占位）。**eve/flue 也无**。
判断：普遍缺口；plandex 独领。lyra 接 `rag/` + tree-sitter 是中等成本。

### C. 上下文

**3.8 Memory / 压缩** ✅
lyra：window-相对 token 触发（默认 window×80%）+ 结构化摘要模板 + 保留最近。
field：人人有压缩。**进阶**：claude_code（full + time-based microcompact + API `clear_tool_uses`）、kimi（micro + full）、codex（结构化模板 + tool-output 截断）；eve/flue（threshold 触发 + LLM 摘要 + 保留窗）。**AionUi 无压缩**（实测）。
判断：齐平；剩 microcompaction（先评估 Claude `clear_tool_uses`）作精修。

**3.9 长期 / 跨会话知识** ✅（lyra 领先）
lyra：**LYRA.md 长期记忆 + extractor 自动提取事实**（用户可编辑文件）。
field：**harness9**（SQLite+FTS5 跨会话 LTM，auto-extract + dedup/TTL）；goose（memory MCP server）；**eve/flue 明确无长期记忆**（eve 文档直说"超出会话的放 connection 或你自己的 DB"，flue 压缩摘要只活在会话内）；Proma 外接 MemOS 云（非本地）。
判断：**lyra 领先**——内建可编辑的项目级长期记忆 + 自动提取，eve/flue 这两个新对标都没有。

### D. 控制与安全

**3.10 HITL / 审批** ✅（lyra 第一梯队，flue 缺）
lyra：**R 模型**——park-on-interrupt + resume，**可持久化/审计/跨重启**，原子 Consume 防重复 resume，本轮加固并发 park。
field：claude_code（同步 `canUseTool` gate，可异步 resolve，5 模式）；codex（`AskForApproval` 5 档 + Starlark execpolicy，可恢复）；crush（channel 阻塞 + pubsub，多客户端 first-wins）；opencode（async/resumable Deferred）；eve（durable park，跨 OAuth 不重复提示）；**flue ❌ 完全无 HITL**（只有 abort+resume，无 tool-approval gate）。
判断：**lyra 第一梯队**，且"可跨重启 + 审计"形态强；**flue 是显著缺这一项的对标**。

**3.11 Sandbox / OS 隔离** ❌（lyra 缺，但有路径守卫兜底）
lyra：`.git` 只读 + per-path 锁，**无 OS sandbox**。
field：**codex 最可移植**（policy→argv，Linux seccomp+Landlock+bwrap / macOS Seatbelt / Windows 受限令牌）；claude_code（macOS Seatbelt + Linux bwrap，gated）；**harness9/OpenHands/pi 给每个子 agent 独立 Docker/VM**；**eve = 5 后端**（Vercel microVM / Docker / microsandbox / just-bash + 凭证经防火墙注入、密钥不入沙箱）；**flue = 2 个落地**（Node-local + Cloudflare DO，其余 9 个 sandbox 是 blueprint 让你自己写适配器）；crush/opencode/plandex 无。
判断：lyra 缺口（缓做）；可移植抽象确实存在（多家收敛）。eve 的"凭证防火墙注入"是漂亮设计。

**3.12 防失控** ✅（lyra 已足）
lyra：loop-detect（SDK）+ budget + idle/stall backstop + todo 校验 + 首终态 CAS。已足；stall 边际、denial-stop 与 lyra"可恢复 denial"设计冲突——**不做**。

### E. 集成与扩展

**3.13 MCP / Skills** 🟢（client 齐，server 缺）
lyra：MCP **client**（5 态 + auth 基座）+ Skills（project+global）。
field：**claude_code / codex 同时是 MCP server**（claude_code 全功能含 Cross-App-Access OAuth；codex `codex mcp-server`，`resources/read` 仍 stub）；crush/opencode/cline/kimi/eve/flue 均**仅 client**；Skills 几乎人人有（agentskills.io 标准；flue/eve 有**渐进式披露** activate_skill/load_skill）。
判断：client 齐平；**MCP-as-server 是 lyra 缺口**（OpenHands/crush/claude_code/codex 有）。

**3.14 Hooks / 扩展点** ❌（lyra 最大扩展性缺口）
lyra：**零 hooks**（613 文档提过 approval/codeintel 等 RPC 提案，未实现）。
field：**claude_code 27 事件**（PreToolUse = 改入参+权限+注上下文 三合一）· **pi 27 typed 事件**（observe/on 语义 + 运行时 `registerTool()` 自扩展）· **opencode 15+**（plugin mutable output）· **codex 10** · **harness9**（HookRegistry + **权限从磁盘热重载**）· plandex（hook 注册表）· crush 1（PreToolUse）· **eve = observe-only**（hook 不能改 loop，改上下文走 defineDynamic）· flue = route-middleware。
判断：**最大扩展性缺口**；claude_code/pi 是标杆。成本中（事件分发 + SPI）。

**3.15 多 provider / per-role 模型** ✅
lyra：多 provider×model 显式配对（21 接/38 adapter）；可复用 per-run-model seam 做 per-role。
field：**plandex 最细**（9 角色 Planner/Coder/Architect/… + model packs）；crush（large/small）；codex（review/guardian/memory 专用模型）；eve（每 subagent + compaction + eval judge 各自模型，经 Vercel AI Gateway）；flue（per-subagent override + thinkingLevel）。
判断：广度+显式性 lyra 领先；**per-role 细分**（Compactor/Extractor/Planner 各用合适模型）是中等成本的待办。

**3.16 A2A / 跨 runtime** ✅（lyra 独有标准实现）
lyra：**标准 A2A**（agent-to-agent 跨 runtime 协议）。
field：**eve 有 remote-agent**（durable callback，但**自有协议非 A2A 标准**）；其余几乎都是进程内 spawn-and-return，无跨 runtime 标准协议。
判断：**lyra 的标准化 A2A 仍较独特**；eve 用自有协议做了类似的事（durable 跨部署调用）。

### F. 自主性与触达（lyra 的主战场缺口）

**3.17 多 agent 团队 / 并行编排** 🟡（lyra 有并行委派、缺团队编排）
lyra：`SpawnChildProtectedOnly` 委派（3 种 spawn 模式）+ **并行委派**——一轮内多 `task` 并发拉起隔离子 agent（各自 blackboard+session，HITL→waiting result 不阻塞兄弟，受同一并发限）。**并发执行的底座已具备**；缺的是上层**团队编排**：命名角色、agent 间消息板 / mailbox、团队生命周期、跨子 agent 成本汇总。
field：**cline = 一等团队**（**18 个 `team_*` 工具**：spawn/task/run/await/broadcast/mailbox/mission-log + 5 工具 outcome 聚合）· **kimi = 128 agent rate-limit-aware swarm** · **AionUi ACP Team**（leader/teammate 槽 + handoff）· claude_code（Agent 工具 + 文件 mailbox SendMessage + teams）· codex（spawn/send/wait + Ed25519 身份）· harness9/pi（每子 agent 独立容器/进程）。**几乎人人都升级了团队能力**。
判断：**这是本轮竞品提升最猛的维度**——lyra 补齐了**并行委派（执行层）**，但仍缺**团队编排（协作层）**：命名角色 + 消息板 + 生命周期 + 成本 roll-up。成本高。

**3.18 调度 / 自动化** ❌（lyra 零调度器）
lyra：**纯 turn 驱动，零 scheduler**。
field：**Proma**（30s anti-drift tick + daily-rolling session + 70% context 安全阀）· **AionUi**（croner，CRUD+时区修复）· **goose**（tokio cron 持久化磁盘）· **kimi**（cron + 后台任务）· **eve**（cron→Vercel Cron Job）· claude_code（gated cron）· **flue ❌**（仅 HTTP/事件触发，外部 cron 打它的端点）。
判断：**最大品类差距之一**；本地优先 agent 的核心卖点。成本中（调度表 + tick + run 记账，可复用 session/run 基建）。

**3.19 Channels / IM 桥接 / 远程触达** ❌（flue 的头牌，lyra 缺）
lyra：仅 `IM_GATEWAY.md` 蓝图，**未建**。
field：**flue = 17 个真适配器**（slack/discord/telegram/whatsapp/github/linear/intercom/zendesk/shopify/stripe/teams/twilio/notion/messenger/resend/google-chat/salesforce——**签名验证 + replay 窗 + 规范 conversationKey**，故意做薄：只管验签+身份，dispatch/出站由 app）· **eve = 7 个**（Slack/Discord/GitHub/Teams/Telegram/Twilio/Linear，HMAC/Ed25519 验签，channel 持有续连 token）· **AionUi 5**（Telegram/Feishu/DingTalk/WeChat/WeChat-Work）· **Proma 3**（Feishu/DingTalk/WeChat）· goose 1（Telegram）。
判断：**flue 把它做成头牌**——这是 lyra 最显眼的触达缺口。成本中-高（每平台一座桥 + 验签；可先做一个 webhook 入口）。

**3.20 proactive 监控 / 事件→触发 run** ❌
lyra：有 `workspace.subscribe`（文件事件）但不"事件→触发 run"。
field：Proma（规则信号→建议任务/监控 + 持久 IM 长连作事件源）；flue/eve 的 channel webhook 本质就是"外部事件→run"。
判断：niche，但与 §3.18/§3.19 同源（建了 channel/scheduler 就顺带有）。

### G. 横切

**3.21 可观测性** ✅（lyra 独有形态）
lyra：**OTel 三驾马车（trace+metric+log）→ slog**，vendor-neutral 去品牌 semconv。
field：**eve**（OTel 注入 `ai` SDK spans + Vercel "Agent Runs" 仪表盘 tag，云专属）· **flue**（`observe()` 事件总线 + 一等 `@flue/opentelemetry` GenAI semconv；braintrust/sentry 走 blueprint）· codex（OTLP）· crush（PostHog）· OpenHands（仅 trace）。
判断：**lyra 的"三驾马车→slog vendor-neutral"形态独有**（生产换 OTLP exporter 即全导云端、业务零改）。

**3.22 多模态** 🟢（lyra 图片输入已接通）
lyra：**图片输入已接**（inline base64，catalog modalities gate）；无音频、无图片生成。
field：图片输入几乎人人有（claude_code/codex/crush/opencode/eve/flue/cline/kimi…）；**kimi 还有视频输入**；图片**生成**：codex/claude_code/Proma/AionUi；语音输入：Proma/AionUi/goose（前端层）。eve/flue 的图片输入在 IM channel 上多数仍"不支持入站附件"。
判断：旧头号缺口**已补**；音频/图片生成归前端层，不背为 runtime 缺口。

**3.23 Evals / 回归框架** ❌（lyra 零）
lyra：无。
field：**harness9 最干净**（`ScriptedProvider` 确定性 mock LLM + Hard/Soft 断言 + 16 golden + **CI 闸**）· **eve 一等**（`eve eval`，确定性 + autoevals LLM-judge，可打本地或线上部署，Braintrust/JUnit reporter）· cline（contract+smoke pass@k+E2E）· pi（faux provider + 25 回归）· plandex（promptfoo POC）；**flue 无**（blueprint 接 Braintrust）。
判断：lyra 缺口（中等成本）；harness9/eve 是标杆。

---

## 4. 能力总矩阵

> 列：lyra | **eve**(Vercel) | **flue**(CF) | claude_code | codex | opencode | crush | 谁领先（含未列出的）。✅第一梯队 🟢有 🟡部分 ❌无。

| 维度 | lyra | eve | flue | cc | codex | oc | crush | 领先 |
|---|---|---|---|---|---|---|---|---|
| Session/fork/checkpoint | ✅ | 🟡(无fork) | 🟡(无fork) | 🟡 | 🟡 | ✅ | 🟡 | **lyra=opencode** |
| Durability 跨重启 | 🟢 | ✅ | ✅ | 🟡 | 🟢 | 🟡 | ❌ | **eve / flue** |
| Agent loop/并行工具+子agent | ✅ | ✅ | 🟢 | ✅ | ✅(Code Mode) | ✅ | ✅ | 齐平 |
| **LSP / 代码智能** | ✅ | ❌ | ❌ | ✅ | ❌ | ✅ | ✅ | **lyra/oc/cc** |
| repo-map / 语义检索 | ❌ | ❌ | ❌ | 🟡 | ❌ | ❌ | ❌ | **plandex** |
| 编辑安全 | ✅ | 🟢 | 🟢 | 🟢 | 🟢 | ✅ | 🟢 | **lyra/opencode** |
| Memory 压缩 | ✅ | 🟢 | 🟢 | ✅ | 🟢 | 🟢 | 🟢 | 齐平 |
| **跨会话长期记忆** | ✅ | ❌ | ❌ | 🟢 | 🟢 | ❌ | ❌ | **lyra / harness9** |
| **HITL 审批** | ✅ | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ | lyra第一梯队(flue缺) |
| OS sandbox | ❌ | ✅ | 🟡 | 🟢 | ✅ | ❌ | ❌ | **codex / eve / harness9** |
| MCP client | 🟢 | 🟢 | 🟢 | ✅ | ✅ | 🟢 | 🟢 | 齐平 |
| MCP server | ❌ | ❌ | ❌ | ✅ | 🟢 | ❌ | ❌ | **cc / codex** |
| Skills | 🟢 | 🟢 | 🟢 | 🟢 | 🟢 | 🟢 | 🟢 | 齐平(eve/flue渐进披露) |
| **Hooks / 扩展点** | ❌ | 🟡(observe) | 🟡(middleware) | ✅(27) | 🟢(10) | ✅(15+) | 🟡(1) | **cc / pi(27)** |
| 多 provider / per-role | ✅ | 🟢 | 🟢 | 🟢 | 🟢 | 🟢 | 🟢 | 广度=lyra / per-role=**plandex** |
| **A2A 跨 runtime** | ✅ | 🟡(自有协议) | ❌ | 🟡 | 🟡 | 🟡(ACP) | ❌ | **lyra(标准)** |
| **多 agent 团队** | 🟡(并行委派,无团队编排) | 🟢 | 🟢(task) | ✅ | 🟢 | 🟢 | 🟢 | **cline(18)/kimi(128)** |
| **调度 / 自动化** | ❌ | ✅(cron) | ❌ | 🟢 | ❌ | ❌ | ❌ | **Proma/AionUi/goose/eve/kimi** |
| **Channels / IM** | ❌ | ✅(7) | ✅(17) | 🟢 | ❌ | ❌ | ❌ | **flue(17)/eve(7)** |
| 可观测性 | ✅(triad→slog) | 🟢(OTel) | 🟢(OTel) | 🟢 | 🟢 | 🟡 | 🟡 | **lyra 形态独有** |
| 多模态图片输入 | 🟢 | 🟡 | 🟢 | ✅ | ✅ | ✅ | 🟢 | 齐平(kimi 含视频) |
| Evals 框架 | ❌ | ✅ | ❌ | 🟢 | 🟢 | ❌ | ❌ | **harness9 / eve** |

---

## 5. lyra 领先 / 独有（勿误报为缺口）

- **A2A（标准 agent-to-agent 跨 runtime）** —— 全场仅 lyra 用标准协议（eve 是自有 remote-agent 协议）。
- **LSP 代码智能（6 ops + diagnostics，内建非外部）** —— 第一梯队；**eve/flue 这两个新对标完全没有**，codex 也没有。
- **跨会话长期记忆（LYRA.md + extractor 自动提取，用户可编辑）** —— eve/flue 明确不做（punt 给外部 DB）；仅 harness9 同级。
- **fork + 影子 git 文件 checkpoint + export/import 三者同时** —— 与 opencode 并列最强；eve/flue 无 fork。
- **HITL R 模型可持久化跨重启 + 审计 + 原子 Consume** —— flue 完全无 HITL；强于 cline 同步阻塞 gate。
- **OTel 三驾马车 → slog（vendor-neutral 去品牌）** —— 独有形态（生产换 exporter 业务零改）。
- **编辑安全三件套（read-before + stale + per-path 锁）** —— 与 opencode 并列最严。
- **多 provider 广度 + `(provider,model)` 显式配对纪律** —— 广度领先。
- **协议参数纪律**（typed 枚举 / 单 `type` 判别 / `Page[T]` / 开放 features map）—— 见 `FRONTEND_API_REVIEW.md`。

---

## 6. lyra 缺口（按价值/普遍性，三梯队）

### 第一梯队 —— 定义"自主 agent / 触达"，本轮竞品提升最猛

| 缺口 | 谁领先 | 成本 | 备注 |
|---|---|---|---|
| **多 agent 团队编排** | cline(18 工具) · kimi(128 swarm) · AionUi(ACP Team) | 高 | 本轮竞品升级最猛；lyra **并行委派已具备**（并发子 agent），缺的是命名角色团队 + 共享消息板 + 团队生命周期 + 成本汇总 |
| **调度 / 自动化运行时** | Proma · AionUi · goose · eve(Vercel Cron) · kimi | 中 | 把 agent 从"一问一答"变"会自己定时跑" |
| **Channels / IM 桥接** | **flue(17)** · eve(7) · AionUi(5) · Proma(3) | 中-高 | flue 的头牌；先做一个 webhook 入口 |
| **Hooks / 扩展点（PreToolUse 三合一）** | claude_code(27) · pi(27) · opencode(15+) | 中 | 最大扩展性缺口；lint/拦截/审计地基 |

### 第二梯队 —— 进阶，数家有

| 缺口 | 谁领先 | 成本 |
|---|---|---|
| **OS sandbox（macOS first）** | codex(4 OS) · eve(5 后端) · harness9(per-agent Docker) | 中-高 |
| **MCP-as-server / per-workspace MCP** | claude_code · codex · OpenHands · crush(daemon) | 中 |
| **per-role 模型分配** | plandex(9 角色) · crush · codex | 中（复用 per-run-model seam） |
| **语义检索 / repo-map** | plandex(tree-sitter) | 中（接 `rag/` + tree-sitter） |
| **Durability 升级为"任意停放 + 云部署"** | eve(Vercel Workflow) · flue(CF DO) | 高（与自托管定位相关，按需） |

### 第三梯队 —— 防御性 / niche

| 缺口 | 谁领先 | 成本 |
|---|---|---|
| **Evals 回归框架** | harness9(ScriptedProvider+CI) · eve(`eve eval`) | 中 |
| **microcompaction** | claude_code(`clear_tool_uses`) · kimi | 低-中 |
| **proactive 监控 / 事件→run** | Proma | niche（建 channel/scheduler 后顺带） |

---

## 7. 刻意不做 / 非 runtime 缺口

- **apply_patch（V4A 多文件 patch）** —— 刻意不做；claude_code/crush 也用 `old_string/new_string`，仅 codex/opencode 用 V4A。lyra 守"guarded 单编辑"被验证正确。
- **PTY 后台命令** —— 协议层刻意不做（深层子进程孤儿不追踪的 trade-off）。
- **图片生成 / 语音输入(ASR) / 桌面 UX（全局热键、语音窗）** —— 前端/产品层，归 lyra 前端；runtime 至多提供数据通道。
- **多租户 / 用户鉴权 / 订阅** —— 协议层零 user 概念，由更外层解决。
- **把 loop 外包给外部 agent-core**（flue 的做法）—— lyra 选 welded loop（复用 lynx `agent/runtime`），不外包。
- **storage 焊死单后端**（eve 焊死 Vercel Workflow）—— lyra 单 SQLite 但 SPI 可换；不把 durability 绑死在某云。

---

## 8. 落地优先级（平台无关 + 价值/成本最优）

| 序 | 项 | 梯队 | 成本 | 为什么 / 前置 |
|---|---|---|---|---|
| 1 | **Hooks（PreToolUse 三合一为核心）** | 1 | 中 | 扩展性地基；claude_code/pi 标杆；其余能力（lint/审计/per-workspace）都长在它上 |
| 2 | **调度 / 自动化运行时** | 1 | 中 | 同品类核心差距；后端基座可起步，可复用 session/run |
| 3 | **Channels / IM 桥接（先一个 webhook 入口）** | 1 | 中-高 | flue/eve 的头牌；触达；需外部平台凭证 |
| 4 | **多 agent 团队编排** | 1 | 高 | 本轮竞品升级最猛；需团队生命周期 + 消息板 + 成本 roll-up |
| 5 | **per-role 模型分配** | 2 | 中 | 复用 per-run-model seam，便宜 |
| 6 | **microcompaction（先评估 `clear_tool_uses`）** | 3 | 低-中 | 长会话质量 |
| 7 | **语义检索 / repo-map（接 `rag/`+tree-sitter）** | 2 | 中 | plandex 独领；选文件质量 |
| 8 | **MCP-as-server** | 2 | 中 | 让 lyra 被别的 agent 当工具 |
| 9 | **OS sandbox（macOS first）/ evals** | 2-3 | 中-高 | 防御性；触发条件驱动 |

---

> **维护**：本文是 2026-06-17 全量重写的现状快照（新增 eve/flue 两个框架对标 + 逐维度对比）。能力落地后回来更新 §2 基线 + §3 对应维度 + §4 矩阵 + 勾掉 §6/§8；新对比对象出现时增列 §1。机制级细节（压缩不变量、durability 实现、sandbox 各平台成本）在落地该项时展开，不在本对比文档堆砌。
