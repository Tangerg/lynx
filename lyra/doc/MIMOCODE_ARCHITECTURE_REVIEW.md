# MiMoCode 架构剖析 —— 对 lyra 的启发

> **日期**：2026-06-13。**对象**：`MiMoCode`（`/Users/tangerg/Desktop/MiMo-Code`，小米开源，TypeScript/Bun monorepo，**OpenCode 的 fork**）。
> **目的**：第一手读 MiMoCode 的记忆 / 上下文 / 自治实现，判断它在 OpenCode 之上净加的设计对 lyra 有何启发。
> **结论先行**：MiMoCode 把 OpenCode 的核心全继承，净增集中在 **「记忆 / 上下文 / 自治」一线** —— 这恰是 lyra 当前最薄的一块。但它这套是**重度机器**，与 lyra「记忆极简（单一可编辑 LYRA.md）」的 KISS/YAGNI 取向有张力。真正该抄的只有 **「Goal + 独立 judge 停止闸」**（且能落到 lyra 现成 `GoalApprover` 扩展点）；外加一招便宜的 **microcompaction**。其余（上下文重建、FTS5 记忆、dream/distill）是"值得想清楚的方向"，不是"该抄"。
> **配套**：能力面对比见 [`AGENT_CAPABILITY_COMPARISON.md`](AGENT_CAPABILITY_COMPARISON.md)（已含 mimocode 列 + §2 mimocode 复盘追加）。本文只谈**架构**。同系列另有 [`PLANDEX_ARCHITECTURE_REVIEW.md`](PLANDEX_ARCHITECTURE_REVIEW.md)。

---

## 0. 定位：它是 OpenCode + 一条记忆/自治线

MiMoCode 明确是 OpenCode 的 fork（README "Relationship to OpenCode"）。多 provider / TUI / LSP / MCP / plugin 全承自 OpenCode（lyra 矩阵里 `opencode` 列已覆盖）。**只需看它净加了什么**：

1. 持久跨会话记忆（FTS5 + 分层结构化文件）
2. 上下文「重建」而非「压缩」
3. checkpoint-writer fork 子 agent
4. Goal + 独立 judge 停止闸
5. 树状任务 + per-task progress 日志
6. dream / distill 自我改进

代码主体在 `packages/opencode/src/`（fork 保留原包名）。

---

## 1. 六个净增机制怎么实现的

### ① 记忆 = SQLite FTS5 全文索引 + 分层结构化文件

**文件**：`memory/service.ts`（BM25 检索）、`memory/fts.sql.ts`（schema）、`memory/paths.ts`（scope 分类）、`memory/reconcile.ts`（指纹增量索引）。

- **scope 分层**：`global/MEMORY.md`（跨项目）/ `projects/<pid>/MEMORY.md`（项目）/ `sessions/<sid>/checkpoint.md` / `notes.md` / `tasks/<id>/progress.md`。
- **索引**：递归 walk `<data>/memory/`，按 `(size, mtime)` 指纹增量 upsert 进 `memory_fts` 表（带 scope/type 元数据）。
- **检索**：查询前先 reconcile 磁盘（让 off-tool 写入可见）→ FTS5 BM25 → 按 top 命中分数 ×0.15 做相对地板裁噪 → 返回 path + snippet + score。
- **要点**：文件仍是真相源、用户可编辑，FTS 只是**额外索引**。

### ② 上下文「重建」而非「压缩」——最大的架构分歧

**文件**：`session/checkpoint.ts::renderRebuildContext`、`session/budgeted-read.ts`、`session/checkpoint-templates.ts`。

上下文满时 agent **不 summarize，而是从多个持久源重建**，每段带 token 预算：

- 任务账本（SQL，层级缩进）/ checkpoint.md（默认 11K cap，分段截断）/ 项目 MEMORY.md（10K）/ 全局 MEMORY.md（6K）/ notes.md（6K）/ 活跃 actor 账本（~500）/ 记忆 keys 索引（FTS 查，排除已推送）。
- **重要性排序 = 分段 token cap**：任务账本 + checkpoint + actors 最高；MEMORY/notes 次之；设计决策/文件引用更低。某池分数高则动态多分配。
- **分段感知截断**（budgeted-read.ts）：解析 `##` 段，保留 header + 斜体指令 + index 行，正文按长度比例截断；超预算则只给骨架 + Read() 提示。
- **microcompaction**（重建后）：清空可压缩工具（read/bash/edit/write/webfetch/websearch）的 `tool_result` body，**但保留 `tool_use` 帧**（模型仍看得到做过什么动作）→ 重建后首个未缓存请求更小、连续性不断。

> 这是 summarize-压缩 的替代哲学：**有损快照 vs 从结构化产物重建**。重建对长任务更稳（结构化状态长存），但机器重得多。

### ③ checkpoint-writer：fork 子 agent 维护会话状态

**文件**：`session/checkpoint.ts::tryStartCheckpointWriter`、`agent/prompt/checkpoint-writer.txt`、`agent/agent.ts`（`native:true, hidden:true, mode:subagent`）。

接近 token 上限时自治触发。它是 **fork agent** —— 继承父 agent 的 prefix（system + tools + 到水位线的消息）做冻结 `ForkContext`，复用 prompt-cache。职责：读现有 checkpoint/MEMORY/notes → 把会话尾巴对账进 **11 段**（活跃意图 / 下一具体动作 / 指令 / 任务树 / 当前工作 / 文件代码 / 发现的知识 / 错误 / 活资源 / 设计决策 / 开放笔记）→ 并行写回各段（不动 header）→ 有持久发现时 append 进 MEMORY.md → 立即停（不 summarize）。
- §1 必须含**块引用的逐字用户请求**（下一轮的锚点）；§3 会话指令不复制进 MEMORY（MEMORY 才是 canonical）。

> ⚠️ 命名陷阱：lyra 的 **checkpoint** 指**文件影子 git**；MiMoCode 的 checkpoint 指**会话状态** —— 是 lyra 没有的另一根轴，别混淆。

### ④ Goal + 独立 judge 停止闸（防乐观停止）

**文件**：`session/goal.ts`、`session/prompt.ts`（runLoop 停止门）。

`/goal <condition>` 后：状态存 per-session；主 runLoop 停止检查查 `Goal.get(sessionID)`，有 goal 就拒停；agent 想停时触发 judge（独立模型调用）：
- 输入 = **完整 transcript**（转 native messages，保留 tool calls/results/images）；system prompt 指令 judge 返回 JSON `{ok, impossible, reason}`；温度 0。
- `ok:true` 放行停；`impossible:true` 放弃 goal；`ok:false` 重进 agent loop（`MAX_GOAL_REACT` 上限防死循环）。
- **关键**：judge **只读 transcript（证据）**，不信 agent 自报 —— 防"乐观假装做完"。

### ⑤ 树状任务 + per-task progress 日志

**文件**：`task/task.sql.ts`（`task` 表 + `task_event` 审计）、`session/checkpoint-paths.ts`、checkpoint-writer.txt §4。

- SQL 表复合键 `(session_id, id)`，`parent_task_id` 自连接成层级；status = open/in_progress/blocked/done/abandoned。
- 子 agent 退出（postStop）写 `tasks/<id>/progress.md`；checkpoint-writer 读它对账，每行带 `(progress: …, last-reconciled-written-at: <ts>)` 让下一 agent 知道读哪些。
- checkpoint §4 渲染整棵树带状态图标。

### ⑥ dream / distill 自我改进

**文件**：`agent/prompt/dream.txt`、`agent/prompt/distill.txt`、`session/auto-dream.ts`（定时触发）。

- **`/dream`**（每 7 天自动）：从 checkpoint.md（发现的知识/错误/设计决策）取候选 → 对原始轨迹（SQL）验证 → 整合进 MEMORY.md（Rules/Architecture/Patterns/Gotchas）+ 剪枝过期。
- **`/distill`**（每 30 天自动）：盘点现有 skills/agents/commands → 扫记忆找重复 pattern（须 2+ 次或高成本）→ 选最小形态打包成 skill/subagent/command（只产高置信资产）。

---

## 2. 对 lyra 的启发分级

跳过前端/产品：voice、TUI（Tab 切 agent）、theme、OAuth、slack/desktop/enterprise 包。按"契合 lyra 架构 + 价值/成本"排序：

### ✅ 【最强，该做】Goal + 独立 judge 停止闸 → 落 `GoalApprover` 扩展点

lyra 的 agent SDK **已有 `GoalApprover` 扩展点**（`collectExtensions[T]` 会发现它）。MiMoCode 的 judge **几乎就是一个 GoalApprover 实现**：

- 一个 GoalApprover 在 agent 想结束时，用一个独立（可指定更便宜/更强）模型读 transcript（lyra 有 Item 流，天然就是证据源），判定 goal 是否真满足；
- 不满足 → 不批准结束 → planner 继续产 action（lyra 的 planner 驱动 loop 天然支持重进）；
- 复用现有 per-role / per-run model seam 给 judge 选模型；
- 加上限防死循环。

**架构契合度最高、独立可加、对自治长任务价值高。** 与 plandex 的 `execStatusShouldContinue`（§见 plandex 文档）同源，但 MiMoCode 的"judge 只读证据"更干净。**建议作为 §6 落地顺序新增项。**

### 🟡 【便宜半招，顺手】microcompaction

独立于"是否做完整重建"，**清空 read/bash/edit 等 `tool_result` 的 body、保留 `tool_use` 帧** 是一个低成本压缩改进：模型仍看得到做过什么动作，但巨大的工具输出不再占 context。lyra 的压缩路径可单独吸收这一招，不必上整套重建。

### 🤔 【该深想，非显然赢】上下文「重建」vs lyra 的「压缩」

这是真架构抉择：

| | lyra（现状） | MiMoCode |
|---|---|---|
| 满了怎么办 | summarize 对话成摘要（有损快照） | 从 checkpoint.md + MEMORY.md + 任务 progress + 近期消息**重建**，分段预算+重要性排序 |
| 真相源 | 摘要本身 | 持久结构化产物（多文件） |
| 长任务稳健性 | 摘要可能丢关键细节 | 结构化状态长存，更稳 |
| 机器复杂度 | 低（一个 compactor） | 高（checkpoint-writer fork agent + 11 段模板 + 多源预算注入 + budgeted-read） |

lyra 已有 extractor（写 LYRA.md）+ compactor。**要不要让压缩演进成"重建"是值得想清楚的方向，但成本高、且和记忆极简哲学冲突** —— 不建议现阶段整套上。若要试，最小切口是先有一个"会话态 checkpoint"概念（区别于现有文件影子 git），再谈重建。

### ⚠️ 【与 KISS 取向冲突，谨慎】FTS5 记忆 + dream/distill

- lyra 后端已是 SQLite，给 LYRA.md 加 FTS 索引技术上自然；但 lyra **刻意只保留单一可编辑 LYRA.md**，MiMoCode 的多 scope（global/projects/sessions）+ 11 段 checkpoint + notes + progress 是**重度机器**，直接抄会破坏 lyra 的记忆极简。
- lyra 的 extractor ≈ 一个轻量持续版 `/dream`，已覆盖"提炼持久知识"的核心；`/distill`（自动产 skill）是 lyra 没有的，但属"自我改进"高阶能力，YAGNI 角度当前不急。

### 【不要做】明确排除项

- 整套 11 段 checkpoint + 多 scope 记忆 + dream/distill **打包照搬** —— 违 lyra 记忆极简（KISS/YAGNI）。
- checkpoint-writer 的 **fork-with-prefix-cache** spawn 模式 —— lyra 委派刻意 `SpawnChildProtectedOnly`，fork 全继承 prefix 是另一套语义，不为单个能力引入。
- 把 MiMoCode 的 `checkpoint` 概念和 lyra 的文件影子 git checkpoint **混为一谈** —— 两根不同的轴。

---

## 3. 一句话总览

MiMoCode 在 OpenCode 上加的是一条**重度记忆/上下文/自治线**，方向正是 lyra 当前最薄处，但实现机器重、且与 lyra 记忆极简哲学有张力。净启发收敛到：

1. **该做一条**：Goal + 独立 judge 停止闸 → 落 lyra 现成 `GoalApprover`（防乐观停止，自治长任务价值高）。
2. **顺手半招**：microcompaction（清 tool_result body、留 tool_use 帧）。
3. **想清楚再说**：上下文重建 / FTS 记忆 / dream/distill —— 是方向不是任务，须先与 lyra「单一可编辑 LYRA.md」的极简哲学权衡。

> 与 plandex 的对照很有意思：plandex 教 lyra「Role 即配置」（per-role model），MiMoCode 教 lyra「judge 停止闸」（GoalApprover）。两者都不是让 lyra 抄架构，而是各贡献一个能精准落到 lyra 现有扩展点的设计。
