# Lyra — Agent 能力横向对比

> **日期**：2026-06-11（2026-06-12 追加 plandex；2026-06-13 追加 mimocode）。**基线**：lyra HEAD（含 LSP / session export-import / edit 守卫 / 文件 checkpoint / 后台命令 五项落地后）。
> **对比对象**：桌面源码仓 `claude_code`（Claude Code CLI）、`codex`（OpenAI Codex CLI, Rust）、`opencode`、`cline`（VS Code 扩展）、`kimi-code`（Moonshot）、`plandex`（Go，plan-first 终端 agent）、`mimocode`（小米，opencode fork，主打跨会话记忆）、`crush`（Charmbracelet，Go TUI）、`OpenHands`（Python，自治 agent + Docker runtime），外加桌面应用类 `AionUi` / `Proma` / `lobehub` / `cherry-studio`（§5）与 `aider` / `cursor` / `windsurf` / `amp` / `gemini-cli` 的公开认知。
>
> **范围**：只比 **AI agent 应用**（终端产品），不比库/框架（a2a-go / adk-go / eino / koog / langchain4j / spring-ai / trpc-agent-go 等）。
> **方法**：对每个仓库实地清点能力面（工具名 / 特性 / 架构），再与 lyra 现状拉成矩阵。

---

## 0. 定位校正（公平比较的前提）

Lyra 是**运行时后端**（协议层，经 Lyra Runtime Protocol 服务独立前端），竞品多是**完整产品**（CLI / IDE 扩展）。因此：

- **属于前端、不计入 lyra 缺口**：slash 命令、TUI / Web UI、输出样式（output styles）、diff 渲染、自动补全（autocomplete）、IDE 深集成。
- **真正可比的是运行时能力**：工具集、上下文/记忆、安全、会话、扩展机制、多 agent、可观测。

本文只在**运行时层**判定缺口。

---

## 1. 能力矩阵

✅ 有 · 🟡 部分 · ❌ 无

> `mimocode` 是 **opencode 的 fork**，基础能力承 opencode，净增集中在「记忆 / 上下文 / 自治」一线（见 §2 mimocode 复盘追加）。`crush`（Charmbracelet，Go TUI）/ `OpenHands`（Python，自治 agent + Docker runtime）是 2026-06-13 追加的同类运行时（见 §2 桌面应用批次追加）。

| 维度 | lyra | claude_code | codex | opencode | cline | kimi-code | plandex | mimocode | crush | OpenHands |
|---|---|---|---|---|---|---|---|---|---|---|
| read / write / edit | ✅（edit 有 read-before + stale 守卫） | ✅ | 🟡（仅 apply_patch） | ✅ | ✅ | ✅ | 🟡（沙箱累积，非直写） | ✅ | ✅ | ✅（str_replace，sandbox 内） |
| 多文件 / apply_patch | ❌（刻意不做，见 §4） | ❌ | ✅ V4A | ✅ | ✅ | ❌ | ✅（anchor 结构化编辑） | ✅（承 opencode） | 🟡（multiedit 单文件批量） | 🟡（whatthepatch） |
| notebook 编辑 | ❌ | ✅ | 🟡 | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| grep / glob | ✅ | ✅ | 🟡（走 shell） | ✅ | ✅ | ✅ | 🟡（architect 走 map） | ✅ | ✅（+rg） | ✅（sandbox bash） |
| **LSP 代码智能** | ✅ 6 op | ✅ 9 op | ❌ | ✅ 9 op | 🟡（插件示例） | ❌ | ❌（内部 tree-sitter map，非工具） | ✅ 9 op（承 opencode） | ✅ 5+ op | ❌（LLM 理解，无 LSP） |
| bash 同步 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | 🟡（_apply.sh 延迟执行） | ✅ | ✅ | ✅（async，libtmux） |
| **后台命令** | ✅（无 PTY） | ✅ | ✅（PTY） | ✅ | 🟡 | ✅ | ❌ | ✅ | ✅（60s 自动后台 + job poll/kill） | ✅（libtmux 持久 session） |
| **auto-debug 自纠环** | 🟡（tool loop 隐式：报错回喂模型自行重试） | 🟡（loop 隐式） | 🟡（loop 隐式） | 🟡（loop 隐式） | 🟡（loop 隐式） | 🟡（loop 隐式） | ✅（显式 `AutoDebugTries` 有界重试） | 🟡（loop 隐式 + goal-judge 重进） | 🟡（loop 隐式 + 签名级 loop detection） | 🟡（observation→agent 重试） |
| web fetch / search | ✅（+httpreq） | ✅ | 🟡（仅 search） | ✅ | 🟡（仅 fetch） | ✅ | ❌ | ✅ | ✅（DuckDuckGo） | ✅（Tavily + Playwright 浏览器） |
| **todo / plan 工具** | 🟡（有 plan 模式，无 model-facing todo） | ✅ | ✅ | ✅ | ✅（plan 模式） | ✅（todo + goal） | ✅（subtask 一等 + 完成闸门） | ✅（树状 T1/T1.1 + SQL + progress.md，最完整） | ✅（todos 工具） | 🟡（skill 演示，无内置） |
| **Goal / judge 停止闸** | 🟡（GoalApprover 扩展点已备，无 judge） | ❌ | ❌ | ❌ | ❌ | 🟡（有 goal 概念） | 🟡（execStatusShouldContinue 完成校验） | ✅（独立 judge 只读 transcript 验证） | ❌ | 🟡（max_iterations + 状态机，无独立 judge） |
| 持久记忆（*.md） | ✅ LYRA.md + AGENTS.md | ✅ CLAUDE.md | ✅ AGENTS.md | ✅ AGENTS.md | ✅ .clinerules | ✅ AGENTS.md | ❌ | ✅✅ MEMORY.md + checkpoint.md + FTS5 检索 + 多 scope | ✅（CRUSH/AGENTS/CLAUDE.md 注入） | ✅（.openhands/microagents/repo.md 自动注入） |
| **记忆自我改进（dream/distill）** | 🟡（extractor ≈ 轻量 dream，无 distill） | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅（/dream 提炼+剪枝 · /distill 自动产 skill） | ❌ | ✅（agent 经用户确认维护 repo 知识） |
| 自动压缩 | ✅ | ✅ | ✅ | ✅ | 🟡 | ✅ | 🟡（summary 仅展示，非回灌缩 prompt） | ✅（**重建式** + microcompaction，非纯 summarize） | ✅（200K 阈值，保留 todos） | ✅✅（**Condenser 5+ 策略**：LLMSummarizing/ObservationMasking/Amortized/LLMAttention） |
| 子 agent 委派 | ✅（单个） | ✅ | ✅ | ✅ | ✅ | ✅ | ❌（role 流水，非委派） | ✅（fork 继承 prefix + 并行 + 专用） | ✅（agentic_fetch 子会话，cost 上卷父） | ✅（enable_sub_agents，registry） |
| **多 agent swarm / team** | ❌ | ✅ teams | ✅ agent_jobs | 🟡 | ✅ 16 team tools | ✅ swarm | ❌ | 🟡（子 agent 并行，非 team 工具） | ❌（串行） | 🟡（parent-child 会话，无 in-conv handoff） |
| MCP client | ✅（5 态生命周期） | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ | ✅（stdio/HTTP/SSE） | ✅（FastMCP，StreamableHTTP） |
| **MCP OAuth** | ❌（seam 已备） | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ | ❌ | ❌（OAuth 在 git provider 层） |
| MCP server（自身作为） | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅（FastMCP /mcp 暴露 create_pr 等） |
| **A2A（agent 互联）** | ✅ | ❌ | 🟡（进程内 multi-agent） | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| **多模态（图片输入）** | ❌ | ✅ | ✅ | ✅ | ✅ | ✅（+视频） | ❌ | ✅（+语音输入） | ✅ | ✅（图片 + 截图） |
| 图片输出 | ❌ | ✅ | ✅（生成） | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| **Hooks（pre/post tool）** | ❌ | ✅ 8+ 事件 | ✅ 10 事件 | 🟡（插件 hook） | ✅ | ✅ | ❌ | 🟡（插件 hook，承 opencode） | 🟡（仅 PreToolUse shell） | ✅（.openhands/hooks.json，command/prompt） |
| **OS Sandbox** | ❌（有审批兜底） | 🟡 可选 | ✅ 三平台 | ❌ | ❌ | 🟡 路径限制 | ❌（diff 沙箱 ≠ OS 沙箱） | ❌ | 🟡（bash 白名单，无 OS 级） | ✅✅（Docker/Remote/Process + `SandboxService` 抽象） |
| 审批 / 权限 | ✅ R 模型 4 档 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅（5 档自治 + apply/reject 闸门） | ✅（agent 权限分级） | ✅（per session/tool/path） | 🟡（confirmation_mode + security analyzer） |
| session export / import | ✅ | 🟡（仅 export） | ❌ | ✅ | 🟡（仅 export） | 🟡（仅 export） | ❌ | ✅（承 opencode） | 🟡（SQLite 持久，无导出格式） | ✅（导出 ZIP + trajectory replay） |
| fork | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ✅（plan 分支，token 继承） | ❌（fork agent ≠ session fork） | ❌ | 🟡（parent_conversation_id 隐式） |
| **文件 checkpoint / restore** | ✅ 影子 git | 🟡（file history） | 🟡（compaction 边界） | ❌ | ✅ 影子 git | ❌ | ✅（沙箱 + reject 回退） | ❌（其 checkpoint 是**会话态**，非文件） | ❌（仅 file tracking） | 🟡（trajectory replay） |
| skills | ✅ | ✅ | ❌ | ❌ | ✅ | ✅ | ❌ | ✅（compose 编排 + distill 自产） | ✅（.agents/skills 自动发现） | ✅（microagents：public + repo，关键词触发） |
| 多 provider × model | ✅ 显式配对 | ❌（Anthropic） | ❌（OpenAI） | ✅ | ✅ | 🟡 | ✅（per-role model packs） | ✅（承 opencode + judge/summary 分模型） | ✅（10+，中途可切） | ✅（LiteLLM 10+，profile + fallback） |
| cron / 调度 | ❌ | ✅ | 🟡（cloud-tasks） | ❌ | ❌ | ✅ | ❌ | 🟡（dream/distill 定时自维护） | ❌ | ❌ |
| **OTel 可观测（traces+metrics+logs）** | ✅ 三件套 | 🟡 | 🟡（trace） | 🟡 | ❌ | 🟡 | ❌ | 🟡（承 opencode） | 🟡（PostHog，无 OTel） | ✅（OTLP gRPC） |

---

## 2. Lyra 的真正缺口（运行时层，按价值/成本排序）

### 第一梯队 —— 平台无关、价值高、竞品几乎全有

1. **多模态图片输入** —— **5/5 全有，唯独 lyra 没有**。最显眼的单点缺口。截图 / 设计稿 / 报错图驱动是主流用法。平台无关：牵协议 content block + 附件存储 + model adapter 的 image content。
2. **Hooks（pre/post tool 用户脚本）** —— 4/5 有（claude_code 8 事件、codex 10 事件、cline 子进程 + 文件 IPC、kimi session hooks）。扩展性核心：lint / 格式化 / 拦截 / 审计。平台无关（exec）。lyra 当前零扩展钩子。
3. **model-facing todo 工具** —— 5/5 有。lyra 有 plan 模式，但没有"模型自维护的 live todo list"。**成本极低**（一个会话态状态工具），对长任务连贯性立竿见影。
4. **MCP OAuth** —— 5/5 有；lyra 的 5 态生命周期已含 `needsAuth`，seam 已备好（`internal/engine/mcp.go`），接入是"填空"非"重构"：ServerConfig 加 auth 字段 + dial 识别 401 → needsAuth + OAuth 回调端点 + token 存储。

### 第二梯队 —— 价值中、偏进阶

5. **多 agent swarm / 并行 team** —— 4/5 有（cline 16 个 team 工具、kimi `AgentSwarm`、codex `agent_jobs`、claude_code teams）。lyra 只有单个 `task` 委派。lyra 的 agent runtime + A2A 是好地基。
6. **LSP 操作补全** —— lyra 6 op；claude_code / opencode 9 op（缺 `goToImplementation` + call hierarchy in/out）。纯加法、低成本。
7. **语义代码检索 / repo-map** —— lyra 有 `rag` 模块但**未接进 agent**；aider 的 tree-sitter repo-map、cursor/windsurf 的 embeddings 全仓索引印证此维度有价值。

### 第三梯队 —— 平台相关或小众

8. **OS Sandbox** —— 仅 codex 全做、claude_code 可选；**平台强相关**（每 OS 一套），已论证暂缓（审批模型 = claude_code 的主防御，lyra 已有）。
9. **notebook 编辑 / 图片输出 / cron 调度** —— 小众，按需。
10. **apply_patch** —— **刻意不做**（见 §4），非缺口。

### plandex 复盘追加（2026-06-12）

plandex 与上面 5 家不同：它**不是 tool-calling agent，而是刚性多阶段 prompt 流水线**（planner→architect→coder→builder，工具调用只用于命名/完成校验，文件改动靠自定义 `<PlandexBlock>` 文本块解析）。因此它的多数"产品特性"（累积 diff 沙箱 UI、plan 分支、org/RBAC/Postgres 后端、5 档自治预设）属**前端/产品层**，不进 lyra 运行时缺口。过滤后，对 lyra 真正有借鉴价值的只有 **2.5 条**：

- **A. per-role 模型分配（真启发，成本低）** —— plandex 给 planner / builder / summarizer / auto-continue 各配不同模型（便宜模型做 summarize、强模型做 build）。lyra 的 `maintenance` 域已把 **Compactor / Extractor / Planner** 拆成独立 service —— **它们正好是天然的 role 边界**。lyra 已有 per-run model 的 seam（`core.ChatClientProvider` + `clientResolver`），把它从 per-run 扩成 **per-internal-role**（压缩走 haiku、规划走 opus）几乎顺水推舟。归入第二梯队。
- **B. repo-map 接入的具体范式（强化既有 §2.7）** —— plandex 用 tree-sitter 抽每文件符号签名 → 按文件 hash 缓存成 JSON 地图 → 喂给 "architect" 阶段**先决定加载哪些文件**再进 implementation。这是 §2.7「`rag` 模块未接进 agent」的**可落地蓝图**：repo-map 既可做成一个 model-facing 工具（符号图查询），也可做成上下文注入器（让模型按需拉文件而非全量）。不必照搬流水线。
- **C.（半条，已修正）auto-debug** —— plandex 有显式 `AutoDebugTries`：命令失败 → 回喂 exit code+stderr → 有界重试。**但 lyra 是 tool-calling loop，bash 报错模型本来就看得到、能自行重试**——这层 plandex **因为不是 agent loop 才必须显式补**。故对 lyra 不是真缺口，可借鉴的仅是一个**有界 `debug <cmd>` 便利封装** + 把 exit-code/stderr 结构化回喂，价值有限，列为可选。

**plandex 没有、lyra 已领先的**：真正的 tool-calling loop、真自动压缩（plandex 的 summary 仅展示）、MCP / A2A / skills / LSP 工具 / 多 provider×model 显式配对、本地纯净运行时（plandex 强绑 Postgres + org/billing）。

### mimocode 复盘追加（2026-06-13）

mimocode 是 **opencode 的 fork**，基础能力（多 provider / TUI / LSP / MCP / plugin）全承 opencode；净增集中在 **「记忆 / 上下文 / 自治」一线**，恰好命中 lyra 当前最薄的一块。架构详剖见 [`MIMOCODE_ARCHITECTURE_REVIEW.md`](MIMOCODE_ARCHITECTURE_REVIEW.md)。过滤掉前端/产品（voice / TUI / theme / OAuth / slack）后，对 lyra 真正有价值的按契合度排序：

- **A.（最强）Goal + 独立 judge 停止闸 —— 直接落到 lyra 现成扩展点** —— mimocode `/goal` 设停止条件，agent 想停时一个**独立 judge 模型只读 transcript**（不信 agent 自报）返回 `{ok, impossible, reason}`，没满足就重进 loop（有上限）。这是 plandex `execStatusShouldContinue` 的更干净版（judge 只读证据，防"乐观假装做完"）。**lyra 的 agent SDK 已有 `GoalApprover` 扩展点（`collectExtensions` 会发现），这个 judge 几乎就是一个 GoalApprover 实现** —— 架构契合度最高，独立可加，对自治长任务价值高。**第一梯队候选。**
- **B.（该深想，非显然赢）上下文「重建」vs「压缩」** —— mimocode 满了**不 summarize，而是从持久结构化产物重建**上下文：`checkpoint.md`（11 段会话状态，由 checkpoint-writer fork 子 agent 维护）+ `MEMORY.md` + 任务 progress + 近期消息，**每段带 token 预算 + 重要性排序**注入。lyra 现在是 summarize 压缩（有损快照）。这是同一问题的两种哲学，重建对长任务更稳但机器重得多 —— 是**真架构抉择**，不是显然该抄。**可先低成本摘 microcompaction 一招**（清空 read/bash/edit 的 tool_result body、保留 tool_use 帧，让重建后首个未缓存请求更小）。
- **C. 结构化 todo（树状任务 + progress 日志）** —— mimocode 把层级任务（T1/T1.1）存 SQL，子 agent 退出写 `tasks/<id>/progress.md`，checkpoint-writer 回收对账。这是 §2.3「无 model-facing todo」缺口的**结构化成品形态**，与 A（judge 闸）+ B（重建）天然咬合。
- **D.（与 lyra KISS 取向有张力，谨慎）记忆 = 可检索 DB + 自我改进** —— mimocode 把所有 memory 文件索引进 **SQLite FTS5（BM25）**、按 scope 分层、指纹增量 reconcile；并有 `/dream`（提炼持久知识进 MEMORY.md + 剪枝过期）/ `/distill`（发现重复工作流自动打包成 skill/subagent/command）。lyra 后端已是 SQLite，加 FTS 索引技术上自然；lyra 的 extractor ≈ 一个轻量持续版 dream。**但 mimocode 这套 11 段 checkpoint + notes + 多 scope + dream/distill 是重度机器，与 lyra「记忆极简（单一可编辑 LYRA.md）」的 KISS/YAGNI 取向直接冲突** —— 列为"值得想清楚的方向"，不建议现阶段全抄。

**mimocode 没有、lyra 已领先/独有的**：A2A、多 provider×model **显式配对**（mimocode 承 opencode 的隐式）、OTel 三件套、文件级 checkpoint（影子 git；mimocode 的 checkpoint 是会话态另一根轴）、Go 运行时的分层纪律。

> 净增到 lyra 的可落地新启发只有 **1.5 条**：**A（judge 停止闸，落 GoalApprover）** 是真正该做的；**B 的 microcompaction** 是顺手的便宜半条。其余（重建、FTS、todo 结构化、dream/distill）是"方向"，需先与 lyra 记忆极简哲学权衡。

### 桌面应用批次复盘追加（2026-06-13）

实地清点桌面上其余 AI agent **应用**（排除库/框架与 portai）。两类：

**① 真 coding-agent 运行时（已并入矩阵列）**：
- **crush**（Charmbracelet，Go TUI + SQLite 会话）—— 与 lyra/opencode 最同类。
- **OpenHands**（Python，自治 agent + Docker runtime）—— 自治执行 + 沙箱见长。

**② 桌面/编排类（不进矩阵，属前端/产品层，见 §5）**：AionUi（Electron，内置 agent + 自动接管 13+ CLI agent + Team Mode）、Proma（Electron，Claude Agent SDK，Chat↔Agent 双模 + local-first JSON 存储）、lobehub（agent 编排平台，Team Mode 多 agent 并行）、cherry-studio（50+ provider 聊天客户端）、continue（已归档）、geminisearch（窄搜索工具）。

过滤后，对 lyra 真正有价值的（多家独立印证 = 收敛信号，价值更高）：

- **A.（收敛，强化既有方向）压缩 = 一个设计空间，ObservationMasking 是便宜收敛赢** —— **OpenHands 的 Condenser 有 5+ 策略**（LLMSummarizing / **ObservationMasking** / Recent / Amortized / LLMAttention），mimocode 有 microcompaction。两家独立都做了**「掩盖/清空旧 tool 观察的 body、保留事件结构」**这招。这与 §2 mimocode 复盘 B 的 microcompaction 是**同一个低成本收敛点**，现有两个独立来源印证 —— lyra 压缩路径应吸收（清 read/bash/edit 的 tool_result body、留 tool_use 帧）。
- **B.（挑战既有假设）OpenHands 证明「可移植 sandbox 抽象」是存在的** —— lyra §4 记「Sandbox 暂缓：无可移植抽象（每 OS 一套原语）」。**OpenHands 用一个 `SandboxService` 接口 + Docker / Remote-HTTP / Process 三实现做到了可移植**（代价是依赖 Docker / 远程服务，重）。这不推翻 lyra 的暂缓决定（审批兜底 + 重依赖），但**「无可移植抽象」这条理由需要修正为「有抽象但依赖重、当前不值得」** —— 见 §4 已更新。
- **C.（便宜小招）crush 的签名级 loop-detection** —— crush 对 (tool+input+output) 做签名哈希，10 窗口内同签名重复 ≥5 次即halt。这是一个**便宜的防死循环安全网**，与 Goal+judge 停止闸**互补**（judge 防"乐观假装做完"，loop-detection 防"卡死重复"），lyra 当前两者都无。落地成本极低（一个 ring buffer + 哈希）。
- **D.（强化既有缺口，无新增）** Hooks（crush PreToolUse / OpenHands hooks.json）再次印证 §2 hooks 缺口；多 agent swarm（lobehub Team Mode 并行 + 共享任务板 / AionUi Team Mode）再次印证 §2.5；skills（crush `.agents/skills` / OpenHands microagents 关键词触发）lyra 已有。

**这批没有、lyra 已领先/独有的**：A2A（全无）、多 provider×model **显式配对**（crush/OpenHands 都是隐式选择）、OTel 三件套（仅 OpenHands 有 OTLP，crush 只有 PostHog）、fork+文件 checkpoint+export 三件齐。

> 本批净增可落地启发：**C（loop-detection，极便宜）** 可直接做；**A（ObservationMasking 式 microcompaction）** 现被两家印证，升格为"该做"；**B** 是认知修正（改 §4 措辞），非新任务。

---

## 3. Lyra 反而领先 / 独有的（不是全面落后）

- **A2A（agent-to-agent）**：这 5 家基本都没有（codex 的 multi-agent 是进程内编排，非跨运行时协议）。
- **fork + 文件 checkpoint + export/import 三件齐全**：多数竞品只占其中一两项（codex 无 fork/export；opencode 无 checkpoint；cline/claude_code/kimi 多为单向 export）。
- **多 provider × model 显式配对** + **per-round 成本核算** + **OTel 三件套**：运行时治理比多数 CLI 完整。
- **read-before-edit + stale 守卫**：与最 Claude 优化的 claude_code 的 `readFileState` 对齐（无 PTY、无 V4A，靠守卫式单 edit 保可靠）。

---

## 4. 已记录的刻意决策（非缺口）

- **不做 codex V4A apply_patch**：lyra 默认跑 Claude，而最 Claude 优化的 claude_code **故意不用** apply_patch、专门强化守卫式单 edit。lyra 已对齐其核心（unique-or-fail + replace_all + read-before + stale）。若未来要多文件原子，再单议。
- **后台命令无 PTY**：仅 codex 用 PTY（平台重）；claude_code / opencode 都不用。lyra 选无 PTY + 纯进程 kill（不引入 setpgid / taskkill 等平台特性），代价是深层孙进程可能残留——与现有同步 bash 同档。
- **Sandbox 暂缓**：~~无可移植抽象（每 OS 一套原语）~~ → **修正（2026-06-13）**：可移植抽象**是存在的**（OpenHands `SandboxService` 接口 + Docker/Remote/Process 三实现即证），但代价是**重依赖 Docker / 远程沙箱服务**。lyra 暂缓的真实理由应是「**有抽象但依赖重、审批模型已兜底、当前不值得**」，而非「无抽象」。要做时照 OpenHands 的接口范式（一个 `Sandbox` 接口 + 多 provider 实现）。

---

## 5. 其他几家补充（桌面无源码）

- **aider**：`repo-map`（tree-sitter 抽全仓符号图喂模型）+ git-native commit → 印证缺口 §2.7。
- **cursor / windsurf**：embeddings 全仓索引 + 大型 "apply model"（快速套改）+ 深 IDE → 语义检索 + IDE 集成（后者属前端）。
- **amp（Sourcegraph）**：子 agent + "oracle"（强模型做架构判断）+ 代码搜索 → 印证缺口 §2.5、且与 plandex 的 per-role 模型同源（见 §2 复盘追加 A）。
- **gemini-cli**：超长上下文 + MCP + 内置 web → 无新维度。
- **continue**：IDE 自动补全（非 agent 范畴）。
- **mimocode**（有源码，已并入矩阵 + §2 mimocode 复盘追加）：opencode fork，记忆/上下文/自治一线最完整 —— 上下文重建、FTS5 记忆、goal-judge 停止闸、dream/distill 自改进。架构详剖 [`MIMOCODE_ARCHITECTURE_REVIEW.md`](MIMOCODE_ARCHITECTURE_REVIEW.md)。
- **crush / OpenHands**（有源码，已并入矩阵 + §2 桌面应用批次追加）：crush = Charm Go TUI 编码 agent（签名级 loop-detection 独特）；OpenHands = Python 自治 agent（Condenser 5+ 压缩策略 + Docker/Remote/Process 三沙箱 + 既是 MCP client 又是 MCP server）。

### 桌面应用类（属前端/产品层，不进运行时矩阵）

这些是**桌面/Web 应用或编排平台**，按 §0 范围属前端/产品层，仅作认知补充：

- **AionUi**（Electron + Bun）：内置 agent（本地文件/shell/web）+ **自动检测并接管 13+ CLI agent**（Claude Code/Codex/…）于统一界面 + Team Mode 多 agent 并行 + 远程接入（WebUI/Telegram/飞书/钉钉）+ cron。是"元编排器 + 内置运行时"混合体。
- **Proma**（Electron + Bun + Claude Agent SDK）：**Chat ↔ Agent 双模切换** + 工作区隔离的 Skills/MCP + local-first JSON/JSONL 存储（无 DB）+ 飞书/钉钉桥。agent-first 设计。
- **lobehub**（Next.js）：**agent 编排平台** —— "agent 即工作单元"，Team Mode 多 agent 并行 + 异步信箱 + 共享任务板 + 调度 + 1w+ skill 市场。印证多 agent swarm 维度。
- **cherry-studio**（Electron）：**50+ provider 聊天客户端** + 300+ 预置助手 + MCP client；**无本地代码执行/文件编辑**（纯 chat）。
- **continue**（已归档只读，终版 v2.0.0）：曾是先驱级 IDE 编码 agent（VS Code/JetBrains + CLI），现停维护。
- **geminisearch**（Bun CLI）：Gemini Search Grounding 窄工具（带引用），非 agent。

---

## 6. 结论与建议顺序

跨家最一致、lyra 又缺的运行时能力：**多模态、hooks、todo 工具、MCP OAuth、语义检索（repo-map）**。

按"平台无关 + 价值/成本最优"建议落地顺序：

1. **多模态图片输入**（唯一 5/5 全有而 lyra 没有；缺口最显眼）
2. **todo 工具**（成本极低、顺手）
3. **Hooks**（扩展性核心）
4. **MCP OAuth**（seam 已备）
5. **per-role 模型分配**（复用 per-run-model seam，maintenance 三服务现成 role 边界；plandex / amp 印证 —— 见 §2 plandex 复盘追加 A）
6. **Goal + judge 停止闸**（落 lyra 现成 `GoalApprover` 扩展点，防自治长任务"乐观停止"；mimocode 印证 —— 见 §2 mimocode 复盘追加 A）
7. **microcompaction（ObservationMasking 式）**（清旧 tool_result body、留 tool_use 帧；mimocode + OpenHands **两家独立印证**，便宜 —— 见 §2 桌面应用批次追加 A）
8. **loop-detection（签名哈希防卡死）**（与 judge 停止闸互补，落地极便宜：ring buffer + 哈希；crush 印证 —— 见 §2 桌面应用批次追加 C）
9. **多 agent swarm** / **repo-map 接 rag**（进阶；plandex 的 architect + tree-sitter map 是 repo-map 的可落地范式 —— 见 §2 plandex 复盘追加 B；lobehub/AionUi Team Mode 印证 swarm）

> 待权衡（非即做，需先对齐 lyra 记忆极简哲学）：上下文「重建 vs 压缩」、记忆 FTS 检索、dream/distill 自我改进 —— 见 §2 mimocode 复盘追加 B/D + [`MIMOCODE_ARCHITECTURE_REVIEW.md`](MIMOCODE_ARCHITECTURE_REVIEW.md)。
> 维护提示：本文是**时点快照**。竞品演进快，落地新能力后请回来勾掉对应缺口、更新矩阵。
