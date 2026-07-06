# 业界对照 —— lyra vs 16 个流行 AI agent 拆解

> **日期**：2026-07-06。**视角**：把 lyra 放进业界横切面里做一次外部体检 —— "领头的 coding agent 在上下文 / 沙箱 / 工具 / 多代理上怎么做，lyra 站在哪，哪些真值得学、哪些是我们威胁模型下的仪式"。
> **来源**：[`NeuZhou/awesome-ai-anatomy`](https://github.com/NeuZhou/awesome-ai-anatomy) —— 16 个 AI coding agent 的 source-level 拆解（Claude Code / Codex CLI / Goose / OpenHands / Cline / Hermes / oh-my-codex / oh-my-claudecode / Dify / DeerFlow …）。逐份深读了与 lyra 最相关的拆解 + `CROSS-CUTTING.md` + `knowledge/patterns/*`；**每一条对照都回 lyra 实际代码核实**（非从 README 推断）。分析用，未 vendor 进本仓。
> **状态**：**外部横向体检 + backlog**，非架构基准。与 [`ARCHITECTURE_REVIEW.md`](ARCHITECTURE_REVIEW.md) 并列（那份是内部体检，这份是外部对照）。actionable 项落在 §3，分「该做 / 取向-等触发 / 别做-仪式」。
>
> **结论先行**：
> 1. **lyra 在这 16 个里属第一梯队** —— 自有 loop（非借框架）、无 god-file（`arch_test` 强制）、有硬成本预算（16 个里只有 Dify 和我们）、loop 检测已达并略超金标准。业界那份 "if building today" 清单，我们做对了大半。
> 2. **深读后的真实 headroom 比初判小**。上下文压缩 (B1) 大部分**已在**（结构化摘要模板 + 压缩标记 + 隐式迭代 refine 都在），只补了「巨型 tool 输出喂爆摘要调用」一处（已做 `80ac015e`）。prompt-cache (B2) **改判非便宜** —— Anthropic 缓存已接线且 todo 确实击穿缓存，但适配层合并 system 块、且除 system 外消息皆被 history 中间件持久化，干净解需新增「ephemeral 上下文消息」原语（中型，跨 core/agent/lyra），见 §3 B2。
> 3. **沙箱是威胁模型的取舍，不是缺陷**：lyra 是本地单用户、OS 信任、无鉴权（有意）。值得加一片薄的（macOS seatbelt-for-bash + 子进程 env 黑名单，防「模型误伤 / 被投毒的 MCP·hook 配置」），**不值得**抄 Codex 的三 OS 堡垒 / MITM proxy / 可插拔 inspector 管线 —— 那是给「不可信 / 多租户」威胁模型的，我们没有。
> 4. **多代理唯一被验证的收益是「并行文件编辑靠 git worktree 隔离」**（业界拆解自己的 takeaway）；其余多代理编排大多是「找问题的解法」。这一条恰好契合我们已有的 `worktree` domain。

---

## 1. 记分卡：lyra vs 语料

| 维度 | 业界领先做法（样板） | lyra 现状（核实） | 裁决 |
|---|---|---|---|
| 核心 loop | 自有 while-loop 或 event-loop；**借框架的都吃了 upstream 债**（DeerFlow 借 LangGraph、MiroFish 借 OASIS、OMC 借 Claude Code） | planner-driven（GOAP）+ 自有 `agent/toolloop`，无外部框架借核 | **领先** |
| god-file | 10/12 有 loop 的项目有 1.4K–9K 行巨石（Cline `Task` 3756、Hermes `run_agent.py` 9000、Codex `codex.rs` 7786）。只有 DeerFlow / Goose 靠结构避开 | Clean Arch + `internal/arch/arch_test.go` 机器强制；最大文件 ~627 行 | **领先** |
| 成本预算 | 16 个里**只有 Dify** 有硬上限（500 步/1200s）；其余全「信任模型自停」（业界点名 anti-pattern） | `kernel/agent.go`：`MaxBudget`(token)/`MaxCostUSD`/`MaxSteps` + `StoppedOnBudget/Steps` | **领先** |
| loop / stuck 检测 | 金标准 = 调用签名 hash（order-independent）、window≈20、warn@3 / kill@5（DeerFlow）；OpenHands 487 行 StuckDetector 做恢复 | `agent/toolloop/loop_detection.go`：签名=**调用+结果**、window=10、nudge@3（软提醒）→ halt@5 | **达标/略超**（键控「结果」比 DeerFlow 只键「调用」更准） |
| 上下文管理 | 谱系：单策略 → 多策略 → **级联**（Claude Code 4 层、OpenHands 10-condenser、Hermes 5 步结构化） | `compaction.go` `MaybeCompact`：已是**结构化多字段摘要**（Goal/Progress/…）+ 压缩标记 + tail 保护 + 隐式迭代 refine；摘要输入现按 `summaryToolResultCap` 截断巨型 tool 输出 | **达标（近多策略；深读后从「大 gap」修正，见 §3 B1）** |
| prompt-cache | Hermes 把 memory 在会话开始 freeze 进 system prompt，之后写盘不改运行提示 → 保住 prefix cache | `kernel/prompt.go`：前缀（base+LYRA.md+AGENTS.md）稳定 ✓，但 `appendTodos` 把**易变 todo 拼进 system 块** → 每次 `todo_write` 击穿最贵的 system 缓存 | **gap（中型：需 ephemeral 消息原语，见 §3 B2）** |
| 沙箱 / containment | Codex：seatbelt(SBPL)/Landlock+bwrap+seccomp/Windows 受限令牌（17K 行，3 OS）；Claude Code：macOS `sandbox-exec` | shell/hook/MCP 子进程**无 OS 沙箱**（grep 确认无 seatbelt/landlock）。安全=policy（approval）+recovery（shadow-git 回滚），**无围栏层** | **gap（威胁模型取舍）** |
| 子进程 env 硬化 | Goose 对 extension 声明的 env 挡 **31 个危险键**（`LD_PRELOAD`/`DYLD_INSERT_LIBRARIES`/`APPINIT_DLLS`…）防注入 | shell(`infra/exec/exec.go`)/hook(`adapter/hooks/shell.go`)/MCP stdio 均**无 env 黑名单**，透传父环境 | **gap（最高性价比）** |
| 工具并发 | Claude Code RWLock：只读并行、写独占 | `chat.Tool.ConcurrencyKey`（并行读 / 独占写），装饰器强制转发 | **达标** |
| 子代理隔离 | oh-my-codex：**git worktree per agent**，~30 个并行零文件冲突（业界唯一被验证的多代理收益）；语料几乎都 cap 深度=1 | 并行子代理**共享 cwd**，靠 `kernel/lifecycle` working-tree admission **串行化**避冲突；有 `worktree` domain；**深度上限已加**（`maxSpawnDepth` 结构性 backstop，`66dd466a`） | **部分（深度已保险；worktree 并行仍是提案）** |
| MCP / provider | Goose MCP 6 flavor 统一 `McpClientTrait`；声明式 provider JSON | 已有 MCP 配置子系统 + 数据驱动 provider 表 | **达标** |
| 记忆存储 | 反面：flat-file 并发损坏（DeerFlow mtime JSON）；正解：SQLite | 单 SQLite（消息/会话/中断）+ LYRA.md（唯一文件，用户可编辑） | **达标** |

**一句话**：11 个维度里多数领先/达标；上下文压缩深读后**近达标**（结构化 + 标记 + 迭代已在，只补了摘要输入截断）；仍开着的是取舍/提案（沙箱、worktree）+ 中型的 prompt-cache(B2)。

---

## 2. 深读要点（可引用的 source-level 机制）

- **Claude Code —— 4 层级联**，原则「Lossless before lossy, local before global」：`HISTORY_SNIP`（无损删未被后续引用的消息）→ `Microcompact`（cache 级隐藏，不改内容）→ `CONTEXT_COLLAPSE`（结构化归档）→ `Autocompact`（全量有损，最后手段）。L1/L2 每轮跑，L3/L4 按 token 阈值门控。大 tool 输出存盘只喂**文件路径**（无损卸载）。
- **Hermes —— 5 步压缩 + frozen snapshot**：① 无 LLM 预剪旧 tool 结果（占位 `[Old tool output cleared…]`）② 保护 head（system+首轮）③ 保护 tail（~20K token 逐字）④ **结构化摘要模板**：Goal / Progress / Decisions / Files / Next ⑤ 迭代 refine（改上一次摘要而非重摘）。压缩块前缀 `SUMMARY_PREFIX`（"[CONTEXT COMPACTION] Earlier turns … were compacted…"）明确告诉模型「这是替换掉的历史」。MEMORY.md 会话开始 freeze 进 system prompt → 保住 prefix cache（~30 行，省真钱）。
- **OpenHands —— 10 condenser 管线**：`Pipeline` 组合子把 cheap→expensive 串起（如 `BrowserOutput→ObservationMasking→LLMSummarizing`）；`StructuredSummaryCondenser`（带任务进度）、`AmortizedForgettingCondenser`（存活概率随年龄衰减）、`CondensationRequestTool`（agent 主动请求压缩）。压缩是**可审计的 event**（进事件流），治「模型不知道自己忘了」。另有 487 行 `StuckDetector`：识别重复动作 / error-fix-error 环 / 语法错误环，检测后**换策略恢复**而非只 kill。
- **Codex CLI —— 四层防御**：Approval 层级（`AskForApproval`：Never/OnFailure/UnlessSafe/Always）→ **Guardian AI**（另一个模型 `gpt-5.4` 给 0–100 风险分，<80 自动放行、否则拒、超时/异常 **fail-closed**）→ OS sandbox（3 OS）→ network MITM proxy（域白名单）。sandbox-denied 命令**不重新询问、直接放宽重试**（approval 已缓存）。
- **Goose —— 5-inspector 管线**：Security→Egress→**Adversary（调 LLM 复核可疑调用）**→Permission→Repetition，各返 `Allow/RequireApproval/Deny`。+ 31 键 env 黑名单。+ MCP 6 flavor（Stdio/Builtin/Platform/StreamableHttp/Frontend/InlinePython）统一 `McpClientTrait`。
- **oh-my-codex —— git worktree 隔离**：每个 worker 一个 worktree（分支 + detached HEAD），~30 agent 并行**零文件冲突**，git merge/`reset --hard` 回收。拆解自评「被低估的模式；磁盘成本 vs 共享文件系统的协调成本，微不足道」。hook 是 `.mjs` **子进程**（stdin JSON / 魔法 stdout 前缀 / 超时 kill），坏插件永不阻塞宿主。

---

## 3. 启发 backlog（排序）

> 判据继承根 CLAUDE.md：治本、反仪式、不为不存在的威胁模型加层。每条 = 现状(lyra file) → 样板 → 具体最小改动 → 裁决。

### B1 上下文压缩升级 —— **深读改判：多数已在，只补一处（已做 `80ac015e`）**
读 `adapter/maintenance/compaction.go` 后，本项初判（依赖「lyra 只做自由文本单摘要」的判断）**大部分不成立** —— compactor 已经相当好，四个初拟改动里三个已在：
1. **先无损预剪** —— 初判「省掉整次摘要」在 lyra 不成立（older 恒被整段摘成 1 条），但**真正的价值在 token 触发那条**：触发正是「几个巨型 tool 输出」，而它们被**原样喂给摘要 LLM**（最贵、且逼近摘要模型自身 window）。**✅ 已做**：`renderTranscript(msgs, cap)` 加 `toolResultCap`，摘要输入按 `summaryToolResultCap`(4000 字符, head+tail 留痕、rune-safe) 截断每条 tool 结果；触发估算与事实提取传 0（须见真实 footprint）。存储历史不动，只 bound 摘要**调用**的输入。
2. **head 保护** —— **考虑后不做**：`## Goal` 段已「引用原始诉求」保留了目标；把首个 user 消息保原文会打乱 stored-history 里 system 摘要的合并/顺序（`MergeSystem` 会把摘要 hoist 进 system 块），风险 > 收益。
3. **结构化摘要模板** —— **已在**：`summarize` 的 prompt 早已是固定字段 `## Goal / Progress / Current state / Decisions & constraints / Next steps`（compaction.go:227–253）。初判「自由文本」是误读。
4. **压缩留痕 + 迭代 refine** —— **已在**：摘要带 `[Earlier conversation summary]` 前缀标记；迭代 refine 隐式发生（上次摘要作为 `older[0]` 被重新喂入下次摘要）。强化 marker 措辞是边际 gilding，未做。

**净结论**：B1 的大头（结构化模板 + 标记 + 迭代）本就在；真正开着的只有「巨型 tool 输出喂爆摘要调用」，已补。

### B2 prompt-cache：todo 移出被缓存前缀 —— **取向-中型（深读改判，非便宜）**
Anthropic 缓存**已接线**（`models/anthropic/chat.go applyPromptCaching`：在 tools+system 前缀尾 + 会话末各下一个 ephemeral breakpoint，注释明写前缀须「byte-identical on every call」）。所以 `kernel/prompt.go appendTodos` 把易变 todo 拼进 system 块**确实**击穿缓存 —— 且 system 在 byte 0，一变把下游 history 缓存一并冲掉。**问题在于干净解比初判贵**：
- 适配层 `buildSystem` 用 `MergeSystem()` 把**所有 system 消息合并成一块** → 无法把 todo 拆成「稳定 system 段（缓存）+ todo 段（不缓存）」两个 system 块。
- history 中间件**只对 system 消息不持久化**；任何非-system 消息（把 todo 挪去当普通消息）都会被逐轮持久化 → 历史里堆积每轮 stale todo 快照（噪声 + 膨胀），比缓存成本更糟。
- 故干净解需要**新增「ephemeral 上下文消息」原语**（core `chat` 加 transient 标记 + history mw 跳过持久化 + 适配层把它放在 rolling breakpoint 之后）—— 一个跨 core/agent/lyra 的中型特性，不是单文件便宜改。

**改判**：B2 从「便宜该做」降为「中型取向」。要么当我们**通用地**引入 ephemeral system-reminder 机制时让 todo 顺这条 seam（那时才做），要么维持现状（缓存成本仅限 todo-active 会话的 todo-变更轮，范围有限）。**不 hack** —— 挪成持久化消息是负收益。

### B3 沙箱 + 子进程硬化 —— **分档**
- **✅ DONE — B3a env 黑名单**（`dad8ba7e`）：落地为 `mcpserver.Server.SafeEnv()` —— 实体丢弃无正当用途的 linker/loader 注入键（`LD_*`/`DYLD_*`，大小写不敏感），在 dial 边界 `internal/runtime/mcp.go configFromServer` 应用。**收窄了初始设想**：只做 MCP config env（真正配置驱动、可被投毒的面）；shell/hook 继承操作者自己的 `os.Environ()`（无配置来源，本地信任下 scrub 属仪式），故不在范围。`PATH`/`NODE_OPTIONS`/`PYTHONPATH` 刻意保留（server 可能正当设置）。
- **B3b macOS seatbelt-for-bash —— 取向（值这一薄片）**：给 shell 的 `exec.CommandContext` 包 `/usr/bin/sandbox-exec -p <SBPL>`（硬编码路径防注入）+ workspace 写白名单 + 默认拒网络。对齐 Claude Code。**明确不做**：Codex 的 Windows 受限令牌/ACL 堡垒 + MITM proxy —— 那是给「不可信/多租户」威胁模型的，lyra 本地单用户、OS 信任、无鉴权（有意），操作者本就有 shell，沙箱只防「模型误伤 / 注入的 tool 输出」，不防敌意用户。Linux Landlock 可作 phase-2。

### B4 并行子代理隔离 + 深度上限 —— **取向 + 一个廉价该做**
- **worktree per 并行子代理 —— 取向（真提案）**：基于已有 `worktree` domain，给每个并行子代理独立 git worktree（分支 → 完成时 merge/commit 回收），解除 `kernel/lifecycle` working-tree admission 的**串行化**，让并行子代理真正并行。业界唯一被验证的多代理收益（oh-my-codex）。权衡：每 worktree 磁盘成本 + fold-in 时的 merge 步；admission-slot 退化为非-git workspace 的 fallback。
- **✅ DONE — 子代理深度上限**（`66dd466a`）：`AgentProcess` 带 delegation depth（顶层 0、child=parent+1，`CreateChildProcess` 设置、snapshot/restore 保留）；单一 spawn choke point `childSpawn.prepare`（四种 spawn 共用）在超过 `maxSpawnDepth`(8) 时**创建 child 前**以 `ErrMaxSpawnDepth` 快速失败，agent-as-tool wrapper 把它当可恢复 tool error 返回给模型。全局 `MaxBudget/MaxSteps` 只兜住成本；depth cap 补一个**结构性**快速失败（budget 未设时也有界）。刻意用 const 不做 config（YAGNI，需要调再提升）；8 足够深、不碰正当嵌套，仍保留业界多数禁止的 depth>1。lyra 的 `SpawnChildProtectedOnly`（仅 ambient 继承）本就站在「限制继承、不粗暴禁工具」这一侧。
- **model 路由（cheap→expensive）—— 别做/YAGNI**：OMC 的 role→tier（critic 用便宜、reviewer 用贵，省 30–50% token）。lyra 的 per-run model 已是人手的升级杠杆；只有当我们真的大量 spawn 廉价子代理时才值得自动化便宜端 —— 否则是 KISS 退步。

### 明确 validation（不动，写下防再议）
loop 检测（已达/略超金标准，键控「调用+结果」）、成本/步数预算（领先 15/16）、无 god-file（`arch_test`）、SQLite 记忆（非 flat-file）、`ConcurrencyKey`≈RWLock、MCP + 数据驱动 provider、自有 loop（非借核）。

---

## 4. 反仪式护栏（明确别抄）

- **Goose 5-inspector 可插拔管线 / Codex Guardian-as-security**：我们 approval 是**有意焊死的单 Service**（Mode + Rules + `SafetyClass` gate）—— 本地单用户下，可插拔 inspector 框架是纯仪式。`SafetyClass` 已≈把 Goose 的 inspector 裁决收成一个分类器、Mode 已≈ Codex 的 `AskForApproval` 层级。唯一能**加性**接上而不破坏单-Service 的是「Guardian 式 LLM 风险分作为 `ModeBalanced` 下的一个可选放行来源，减审批疲劳」—— 那是 **UX 改进不是安全边界**（fail-open、判断与模型相关），只作 opt-in 便利，绝不当 containment。
- **Codex 三 OS 堡垒 / MITM proxy / Windows ACL**：威胁模型不匹配（见 B3b），别抄。
- **多代理编排本身**：业界拆解自己的 takeaway —— 「多代理是找问题的解法，除了并行文件编辑」。除 B4 的 worktree 并行，别为「planning/review/critic agent」建多代理，那些是单 agent 的顺序工具调用。
- **middleware 拓扑排序 / Dify 7 容器 / LangGraph 借核**：我们 typed 小链 + 单 binary + 自有 loop，都不适用。
- **已做的别抄**：MCP、SQLite 记忆、声明式 provider、硬预算 —— 我们已经有。

---

## 5. 引用来源

- 语料：`NeuZhou/awesome-ai-anatomy`（16 teardowns + `CROSS-CUTTING.md` + `knowledge/architecture-insights.md` + `knowledge/patterns/{loop-detection,subagent-delegation,memory-frozen-snapshot}.yaml`）。
- 深读：`claude-code/` `hermes-agent/` `openhands/`（上下文）；`codex-cli/` `goose/` `cline/`（沙箱/安全/工具）；`oh-my-codex/` `oh-my-claudecode/`（多代理/隔离）。
- lyra 侧核实点：`app/runtime/internal/adapter/maintenance/compaction.go`、`app/runtime/internal/kernel/prompt.go`、`app/runtime/internal/infra/exec/exec.go`、`app/runtime/internal/adapter/hooks/shell.go`、`app/runtime/internal/infra/mcp/`、`app/runtime/internal/domain/approval/`、`app/runtime/internal/infra/checkpoint/`、`app/runtime/internal/kernel/agent.go`、`app/runtime/internal/kernel/lifecycle/working_tree_admission.go`、`app/runtime/internal/domain/worktree/`、`agent/toolloop/loop_detection.go`、`agent/runtime/child.go`。
