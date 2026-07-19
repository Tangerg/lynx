# Claude Code 启发的能力吸纳 Backlog

> **来源**：对 **Claude Code 源码快照**（2026-03-31 经 npm source-map 泄露；桌面克隆 `~/Desktop/claude_code`，TS strict + Bun + React/Ink TUI，~1900 文件 51 万行）的源码级对比分析。核心：巨型 `QueryEngine.ts`（LLM loop）、`Tool.ts`、`tools/`（~40 工具一工具一文件夹）、`commands/`（~50 slash）、`services/`（api/mcp/oauth/lsp/compact/extractMemories）。大量能力挂在 GrowthBook `tengu_*` feature flag 后。方法与 [`GROK.md`](GROK.md) 一致；跨应用总索引见 [`README.md`](README.md)。
>
> **状态**：全部 proposed。已跳过 parity 项（plan-mode、hooks、slash、MCP、scheduler、结构化输出、checkpoint、LSP、@codebase）——只谈 lyra **缺**或**做得差**的。

---

## 0. 五个 distinctive 子系统 + 一条横切风格

Deferred tool loading（ToolSearch）· 多 agent swarm（coordinator + Team*/Task* + SendMessage）· auto-mode LLM 安全分类器（yoloClassifier）· 持久 auto-memory（memdir + extractMemories）· 结构化 task/todo（TodoWrite V1 + Task* V2）。横切：**配置即 markdown+frontmatter**（output-styles/agents/skills/commands/memories 全 `.md`）。

**两个强收敛信号**（多家独立殊途同归 = 高置信）：① **工具搜索/延迟加载** —— Claude Code(A) 与 [Grok G4](GROK.md) 独立都做；② **检索式跨会话记忆** —— Claude Code(D) 与 [Grok G2](GROK.md)（及 Hermes）独立都收敛到 lyra 的 deferred C8。

## 0.1 筛选准则

沿用 lynx 四道筛子 + 反向不变量，详见 [`GROK.md` §0.1](GROK.md)。

---

## 1. 吸纳清单

### CC1 · Deferred tool loading / ToolSearch（P1，头号；与 [Grok G4](GROK.md) 收敛）

- **来源/为什么**：`tools/ToolSearchTool/`；门控 `utils/toolSearch.ts`；`Tool.shouldDefer`/`isMcp`/`alwaysLoad`。标 `shouldDefer:true` 或 MCP 的工具**不进初始 prompt**、只在 system-reminder 里列名字；模型调 `ToolSearch({query})` 拉 schema——两种 query：`select:A,B,C`（按名直取）或关键词打分搜索（name 拆 CamelCase/`mcp__server__action`、description、`searchHint` 加权、`+term` 必含）。返回是 `tool_reference` block 或**降级为 `<functions>` 文本块**（provider-agnostic），schema 一旦出现即可像顶层工具调用；exact-match / 已加载工具走 no-op 快路径防 retry 抖动。
- **目标**：工具目录膨胀是**上下文预算**问题——把"低频工具 / 全部 MCP 工具"从初始 schema 挪走，换"按需拉取 + 名字清单"。
- **落点**：tool registry 装配处（application）加 `Deferred` 分区 + `toolset/toolsearch/` 一个只读工具；system prompt 注入侧列 deferred 名字。
- **计划**：① registry 加 `deferred` 标志位 ② `search_tools`/`ToolSearch` 工具（`select:` + 关键词双模式，返回走 `<functions>` 文本块）③ fail-soft（选已加载=无害 no-op）④ system-reminder 列 deferred 名字 + "N of M"。
- **验收**：多接 MCP server 时初始 schema token 显著下降；模型经 search 找到并直接构造调用；已加载工具重复 search 无副作用。
- **风险/边界**：**纯上下文优化、零新机制债**（只是 registry 一个标志位 + 一个工具）；provider-agnostic（不依赖 Anthropic `tool_reference`）。**与 [Grok G4](GROK.md) 是同一能力，实现时合并为一条**。
- **优先级**：**P1（首推，杠杆最高、最薄）**。

---

### CC2 · 结构化 TODO / task 执行追踪（P1，头号）

- **来源/为什么**：`tools/TodoWriteTool/`（V1）与 `tools/Task*Tool/` + `utils/tasks.ts`（V2）。**V1 TodoWrite**：单扁平清单、**整表替换**式写、存 `appState.todos[sessionId]`（**内存态非持久**）、全 completed 自动清空；每项 `content`(祈使)+`activeForm`(进行时,给 spinner)+ `status∈{pending,in_progress,completed}`；prompt 硬约束"任何时刻**恰好一个** in_progress""完成立即标记不批量"；loop-exit **结构化 nudge**（关掉 3+ 项且无 verification step 时提醒 spawn verifier）。**V2 Task***：独立 CRUD + `blocks/blockedBy` 依赖 + `owner`（可指派 teammate）。
- **目标**：一个**会话内、模型自管的执行进度清单**——既给模型做 working-memory 式 step tracking（实测显著提升长任务完成率），又给 UI 实时进度条。
- **落点**：`toolset/todo/` 一个工具 + session/blackboard 上 `todos` 分区（domain/application）；desktop 进度条视图（adapter）。
- **计划**：① V1 扁平清单 + 整表替换写 + 内存 session-state ② prompt 契约（恰好一个 in_progress / 完成即标 / ≥3 步才用）③ desktop 进度条 ④ V2 依赖/owner 属 YAGNI，swarm 落地再说。
- **验收**：长任务中模型维护清单、恰好一个 in_progress、完成即标；UI 实时反映。
- **风险/边界**：⚠️**别退化成第二个机制**——lyra 已有 plan-mode。Claude Code 里 **plan-mode 产计划、todo 追执行是分开的**，建议 lyra 同样保持 todo 只做执行追踪，与 plan 两层。
- **优先级**：**P1**。

---

### CC3 · auto-mode LLM 安全分类器（P2）

- **来源/为什么**：`utils/permissions/yoloClassifier.ts` + `classifierDecision.ts` + `yolo-classifier-prompts/*.txt`。auto/yolo 下不逐个弹窗，而**每动作过一个 LLM judge**：**两段式**（stage1 fast，`max_tokens=64`/`stop=</block>`，立即出 yes/no；仅 block 才升级 stage2 thinking CoT 降假阳）；喂分类器的 transcript **只含 user 文本 + assistant `tool_use` 输入、剔除 assistant 文本**（防话术影响判决），每工具 `toAutoClassifierInput()` **只暴露安全相关字段**，JSONL 编码使恶意内容无法伪造 `{"user":...}` 行（**prompt-injection 硬化**）；**fail-closed**（API 错/不可解析/中断 → block）；只读工具走 `SAFE_YOLO_ALLOWLISTED_TOOLS` 跳过省调用。
- **目标**：把"自主模式要不要放行"从**硬编码黑名单**升级成**便宜 LLM judge**，尤其利好无人值守的 headless/scheduler run。
- **落点**：approval `Service`（application/control-surface）内加一条 classifier decision path + 一个 side-model query（复用 per-role utility model）。
- **风险/边界**：**要克制**——每 tool-use 一次额外 LLM 调用（latency+cost），必须 (a) **只在 yolo/auto 模式启用** (b) 只读工具走 allowlist 跳过 (c) **复用单一 approval Service、不新增机制**（是内部多一条 decision path、非新子系统）。**fail-closed + 只喂 tool 投影 + JSON 转义必须一起吸**（否则是安全洞）。第二法则视角：这是 B5/C3 yolo 自否决的**治本智能版**。
- **优先级**：**P2**。

---

### CC4 · 持久 auto-memory / memdir（P2；与 [Grok G2](GROK.md) 收敛到 C8）

- **来源/为什么**：`memdir/findRelevantMemories.ts`（读）、`services/extractMemories/`（写）、`memdir/memoryTypes.ts`（taxonomy）、注入点 `utils/attachments.ts`。**写**：一轮结束后台 fork 一个 "memory extraction subagent"（与主对话同 system prompt 的 perfect fork），**只准读 + 只准写 memory dir**（`rm` 禁、其它工具 deny），把最近消息里持久事实写成**一 topic 一文件**并在 `MEMORY.md` 索引加一行 `- [Title](file.md) — hook`；若主 agent 本轮已自写 memory 就跳过。**读**：每 user turn 扫所有 memory 文件的 frontmatter 组 manifest，喂便宜 side-query 选**最多 5 个**相关文件（过滤已 surface 过的），截断后作为 `relevant_memories` system-reminder 注入；`MEMORY.md` 索引常驻 system prompt。
- **目标**：跨会话记忆的**检索式注入**——不塞整个记忆，而"frontmatter manifest + 便宜模型选 top-5 + 注入"；写侧沙箱化 extraction fork + MEMORY.md 只做索引、内容在分文件。
- **落点**：memory 子系统（application）+ side-model 检索 + 注入 turn 输入（adapter）；选取器复用 lyra 现成便宜 utility-model 角色。
- **风险/边界**：**取**：相关性检索注入 + 分文件+索引 + 后台抽取 fork。**拒**：`teamMemorySync/` + team secret scanner（team 共享=多租户，违 filter #2）。**这是 lyra DEFERRED 的 C8 的一份成熟蓝图**，与 [Grok G2](GROK.md)、Hermes 三方收敛——C8 实现时综合三家。工程量较大且触 schema，破坏性改动先咨询。
- **优先级**：**P2**。

---

### CC5 · 对抗式 verifier 子 agent +「不许自评 verdict」（P3）

- **来源/为什么**：`tools/AgentTool/built-in/verificationAgent.ts` + TodoWrite loop-exit nudge。一个 **built-in、只读、对抗式** "verification specialist" 子 agent：system prompt 极详——"你的工作不是确认能跑，是**试图弄坏它**"；禁改项目/装依赖/git 写；按变更类型给分门别类验证策略 + "PASS 前至少跑一个 adversarial probe"。配套 nudge："你刚关掉 3+ 任务且无 verification step……**你不能靠总结里列 caveat 自评 PARTIAL，只有 verifier 出 verdict**"。
- **目标**：把"验证"做成**一等、只读、有独立人格的子 agent 角色**，用 loop-exit nudge 强制收尾前触发——对抗 LLM"读了代码就说 PASS"的通病。
- **落点**：sub-agent 定义（application/config）+ todo/loop 收尾 hook。
- **风险/边界**：**纯角色/prompt 设计、零新机制**（lyra sub-agent 体系里加一个 built-in 定义 + 一段收尾 nudge）；verifier 只读约束天然契合 lyra editguard/safety-class。
- **优先级**：**P3**（高价值非紧急）。

---

### CC6 · 异步 worker 编排（coordinator + SendMessage-continue）（P3，先证伪）

- **来源/为什么**：`coordinator/coordinatorMode.ts`、`tools/SendMessageTool/`、`tasks/{LocalAgentTask,InProcessTeammateTask,RemoteAgentTask}`。coordinator 模式下主 agent 换成"只派活/综合/对用户说话"的 system prompt，worker 异步跑、结果作为 `<task-notification>` user 消息 fire-and-forget 回流；`SendMessage(to, message)` 可**续跑一个已完成/停的 worker**（复用其已加载 context）或 broadcast；附一张 **"continue vs spawn 决策表"**（按 context 重叠度选）。
- **目标**：取两点——(1) **异步 worker + 通知式结果回流**（vs 同步阻塞委派）；(2) **"续跑复用 context vs 新 spawn 清 context"由重叠度决定** 的 orchestration 心智 + "coordinator 绝不把'理解'甩给 worker、必须自己 synthesize spec" 的纪律。
- **落点**：agent runtime / engine（application）。
- **风险/边界**：**明确拒**整套 teams/mailbox/tmux-pane/coordinator-mode 机器（多机制债，违 filter #3）+ `RemoteAgentTask`（远程/多机，违 filter #2）。只取异步+续跑语义 + spec-synthesis 纪律。SendMessage 的 `queuePendingMessage`("下个 tool round 投递")可映射到 lyra 的 BeforeRound seam。**先证伪"lyra sub-agent 现状是否真同步"再决定，不确定就不吸**。
- **优先级**：**P3，先证伪**。

---

### CC7 · Git worktree 隔离（EnterWorktree）（P3）

- **来源/为什么**：`tools/EnterWorktreeTool/` + `utils/worktree.ts`。在 `.claude/worktrees/` 建基于 HEAD 新 branch 的 git worktree，把 session cwd 切进去（清 system-prompt/CLAUDE.md/plans 缓存）；非 git repo 降级到 hook；`ExitWorktree` 中途退出（keep/remove）；**仅当用户显式说 "worktree" 才用**。
- **目标**：一个**平行隔离工作区**原语——与 checkpoint **正交**两轴：worktree=空间隔离（在别处干活不污染主 tree），checkpoint=时间回溯。
- **落点**：`toolset/worktree/` 一个工具 + session cwd（adapter/infra）。
- **风险/边界**：取 worktree-as-isolation 思想、复用 lyra per-session cwd seam；非 git 时 hook 降级可不做。niche。
- **优先级**：**P3**。

---

### CC8 · Output styles（persona 切换）—— 不吸

- `outputStyles/*.md` 作为 system-prompt persona 注入、`keep-coding-instructions` 决定叠加还是整体替换。**不吸**：lyra 已有 rules（持久约束）+ recipes（调用式模板）+ per-role models，再加"persona 系统提示层"是**第三个重叠机制**（违 filter #3）。若真需 persona 应并入 rules。

---

## 2. 刻意不吸清单

| 项 | 来源 | 不吸理由 |
|---|---|---|
| **output styles（persona）** | `outputStyles/` | 与 rules/recipes/per-role-models 重叠的第三机制；需要就并入 rules |
| **SleepTool** | `tools/SleepTool/` | lyra 已定 `bash_output` 的 `block+timeout` 优于独立 Sleep 工具 |
| **SyntheticOutputTool（结构化输出）** | `tools/SyntheticOutputTool/` | lyra `JSONParser[T]`/`ListParser`/`MapParser` 已闭环 |
| **teamMemorySync / mailbox / tmux panes / RemoteAgentTask / coordinator-mode 整机** | `services/teamMemorySync/`、`coordinator/`、`tasks/RemoteAgentTask` | team 共享=多租户、远程=跨机（违 filter #2）+ 多机制债（违 filter #3）；只抽"异步 worker + 续跑语义"薄层（CC6）|
| **cache_edits microcompaction** | `microCompact.ts` | 依赖 Anthropic `cache_edits`/`cache_reference` beta，provider 专属；lyra A2（超大 offload）+ A3（分级 trim）已覆盖。"按 recency 保留最近 N 个 tool_result"若 A3 未覆盖可小参考，机制本身不吸 |
| **GrowthBook flags / `tengu_*` analytics / OAuth / bridge / voice / buddy / upstreamproxy** | 多处 | Anthropic 内部基建或触 provider-OAuth（反向不变量）/多机，与 lyra 无关或明确反向 |

---

## 3. 建议节奏

- **头号（薄、高杠杆）**：CC1（工具搜索，与 G4 合并实现）、CC2（执行 TODO）。
- **中等**：CC3（yolo 智能分类器，只挂 yolo + 复用 approval Service）、CC4（auto-memory，并入 C8 三方综合）。
- **随缘 P3**：CC5（对抗 verifier 角色）、CC6（异步 worker，先证伪）、CC7（worktree 隔离）。
