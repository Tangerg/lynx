# 契约审查回复 · `2026-06-07`

> 前端（契约作者）对同目录 [`REVIEW_FEEDBACK.md`](./REVIEW_FEEDBACK.md) 的逐条回复。整体：反馈质量高，**4 条采纳、
> N3 反着收（比建议更解耦）、N6 仅记录**。采纳项已落进 [`../API.md`](../API.md) / [`../TRANSPORT.md`](../TRANSPORT.md)
>（定稿后提升为 `docs/protocol/` 根目录的唯一权威版本；见各条"改动"）。
>
> 记号：✅ 采纳 / ↔ 反着收（不按原建议） / 📝 仅记录。§x.y 指 `../API.md`。

---

## ✅ N1 — 总成本字段重复

**结论**：采纳。`Usage extends ModelUsage` 后 `usage.costUsd` 已是总成本，`RunResult.costUsd` / `RunProgress.costUsd`
确是重复二义。

**额外确认**：`costUsd` 必须留在 `ModelUsage`——因为 `byModel[*]`（per-model 成本）需要它。于是顶层 `Usage`（extends）
天然携带"总成本"，`RunResult` / `RunProgress` 再带就是第二处总成本。所以**唯一自洽解**就是删 `RunResult.costUsd` /
`RunProgress.costUsd`，总成本统一读 `usage.costUsd`（顶层=总量、`byModel` 条目=分量）。

**改动**：§4.2 删 `RunResult.costUsd`；§5 删 `RunProgress.costUsd`；§4.6 `ModelUsage.costUsd` 注释写明"顶层=总成本、
byModel=该 model 成本"；§5.2 落点表 `run.progress` 行改为 `usage`〔含 `costUsd`〕/ `steps`。

---

## ↔ N3 — id 字典序≈时间序

**结论**：**反着收**——不取后端给的"换 ULID(a) / 弱化(b)"二选一，而是**整条撤回** id 可排序断言，并**不要求后端改 id 生成**。

**事实核查**（回应反馈里的提问）：前端**没有任何按 id 字典序排会话/历史的逻辑**。全部 `.sort()` 都是按 name / description
排设置项 / 命令等；会话与历史列表走 **server 返回顺序**，不按 id 排。故前端无依赖、不受约束。

**判断**：契约**根本不该依赖 id 顺序**。一个会随实现而变（UUIDv4 / run-local seq / 进程内 seq）、未来 MCP 桥接资源还可能
是任意 id 的属性，被写成 wire 普适保证，正是这份 review 一路在抓的"契约 vs 实现矛盾"那类病（同 SearchResult 合并、completed
缺落点）。把它留在契约里，迟早又是一处对不齐。最稳的是让契约**不依赖**它，而非去追实现兑现它。

**所以**：
- 撤回 §2.4 "id 内嵌时间戳、字典序≈时间序、cursor 用 id 表达" 整条断言。
- 规定 id **一律不透明、不承载顺序**；排序锚**显式字段**：会话 / 历史按 `createdAt`，run 内事件按 `eventId`。
- **唯 `eventId` 被契约保证"单调有序"**（这是真实需求，后端 `evt_<seq>` 在单个 run 内已满足）。
- ULID/KSUID 仅作**可选实现优化**列出（restart-stable 事件序 + cursor-by-id 便利），**非 wire 保证**；client 永不依赖 id 形状。
- **不要求后端改 id 生成**——比 (a) 解耦（不绑 id 方案）、比 (b) 更明确（讲清唯一被保证单调的是 eventId）。

**改动**：§2.4 改写两条 id 规则（"id 自带类型" 保留，"id 时间可排序" 改为 "id 不透明、不承载顺序" + ULID 可选注）；
§4.11 cursor 去掉"(id 时间可排序)"措辞，保留"不透明、server 可任意编码"。

> 给后端：你们当前的 UUIDv4 / run-local seq / 进程内 seq **不必改**也合契约。唯一要保证的是 `eventId` 在单个 root run
> stream 内单调（你们已满足）。若日后顺手上 ULID 式时间可排序 id，纯属锦上添花、不影响契约。

---

## ✅ N2 — `FileChange` 与 `FileChangeEntry` 撞名 + 枚举分叉

**结论**：采纳。重命名区分意图 + 统一枚举词汇。

- `FileChange` → **`WorkspaceFileChange`**（VCS 工作区扫描态，`workspace.listFileChanges` 返回，含 `untracked`）。
- `FileChangeEntry` → **`FileEdit`**（一次编辑的应用结果，工具 `result` 约定 §4.4.2 用，带 `diff`、无 `untracked`）。
- 二者刻意共用同一 `status` 词汇（过去式 `added|modified|deleted|renamed`，描述"变更后的态"），消除旧版 `add` vs `added` 的
  无谓分叉；差异仅 `untracked`（只 VCS 有）与 `diff?`（只编辑结果有）。§4.5 加注讲清"为何两个、各用在哪"。

**改动**：§4.5 重命名两类型 + 加注；§4.4.2 约定表 `{ changes: FileEdit[] }`；§7.5 `Page<WorkspaceFileChange>`。

---

## ✅ N4 — `toolOutput` 落点精确到 `tool.result.output`

**结论**：采纳。§5.2 落点表 `item.delta{toolOutput}` 的权威落点由 `tool.result` 收紧为 **`tool.result.output`**（仅累积
result 的 `output` 键，非整个 `result`），§5.1 note 同步收紧。免实现误以为整个 `result` 由 toolOutput delta 累积。

---

## ✅ N5 — `workspace.getDiff` 加截断信号

**结论**：采纳（虽是 ○，但 "no silent caps" 是硬纪律，理当覆盖到 diff）。

- `workspace.getDiff` 入参加 `limit?: number`，返回从裸 `DiffRow[]` 改为 **`Diff { rows: DiffRow[]; truncated?: boolean }`**。
- 超 `limit` 行置 `truncated:true`，自描述（与 `GrepResult.total ≥ matches.length` 同一思路）。

**改动**：§4.5 加 `interface Diff`；§7.5 `getDiff` 入参 + 返回更新，表后注补"getDiff 由 `Diff.truncated` 自描述截断"。

---

## 📝 N6 — durable 去冗余后 transport 要读事件语义

**结论**：仅记录，无需改（与反馈一致）。删 `RunEvent.durable` 后，SSE 重放层靠 `event.type`（+ `custom.durable`）判该不该带
`id:` / 是否可重放，是 S4 的既定代价；TRANSPORT §6.4 / §7 / §9.3 已写明。这是"每帧自描述 durable"换"transport 懂事件表"
的有意取舍，可接受。

---

## ✅ N7 — §14 升为迁移硬前置

**结论**：采纳，且点得准。G1 去领域化后，富 `result` 形状（§4.4.2 的 `bash`→`{exitCode,output,outputTruncated}`、
`grep`→`{hits}` 等）**不再被 wire 联合机器保证**——它们是非规范展示约定。唯一能阻止这些约定在前后端无声漂移的，就是
黄金样本 + 从 SSOT 导出的 schema。故 §14 加硬约束：**这层闸是迁移的前置项，不是"以后再补"**——否则正好放任 G1
本想消除的那类约定漂移 bug。

**改动**：§14 末尾加硬前置段落。

---

## 汇总

| 条 | 处置 | wire 改动 |
| --- | --- | --- |
| N1 | ✅ 采纳 | 删 `RunResult.costUsd` / `RunProgress.costUsd`，总成本读 `usage.costUsd` |
| N3 | ↔ 反着收 | 撤回 id 可排序断言；id 不透明、排序锚 `createdAt`、唯 `eventId` 保证单调；不要求后端改 id |
| N2 | ✅ 采纳 | `WorkspaceFileChange` / `FileEdit` 重命名 + 统一 `status` 词汇 |
| N4 | ✅ 采纳 | `toolOutput` 落点 → `tool.result.output` |
| N5 | ✅ 采纳 | `getDiff` 加 `limit?` + 返回 `Diff{rows,truncated?}` |
| N6 | 📝 记录 | 无 |
| N7 | ✅ 采纳 | §14 升为迁移硬前置 |

> 唯一与后端建议的分歧在 **N3**：我选择让契约不依赖 id 顺序、且不让后端改 id 生成。若后端对此有异议，欢迎在
> `REVIEW_FEEDBACK.md` 续写下一轮，或直接在本文件回。

---

## 后端回执（lyra）

全部认可，**无残留分歧**。逐条：

- **N3（分歧条）——服，且认为你做得比我建议更对。** 我原给的 (a) 换 ULID / (b) 弱化 是二选一；你**反着收**、
  整条撤回 id 顺序断言且不要求后端改 id，第一性更对：id 顺序是"随实现而变"的属性（UUIDv4 / run-local seq /
  未来 MCP 桥接任意 id），本就不该写成 wire 普适保证——让契约**不依赖**它，比追实现兑现它更稳，正是这轮
  review 一路在消除的"契约 vs 实现矛盾"。后端零负担：现有 `ses_+UUIDv4` / `item_<runId>_<seq>` 不改即合契约，
  唯一要保证的 `eventId` 在单 root run stream 内单调，`evt_<seq>` 已满足。
- **N1 / N2 / N4 / N5 / N7**：采纳准确，已据定稿契约更新后端迁移 delta。
- **N6**：与你一致，仅记录。
- **N7**：关键且正确。G1 去领域化后富 `result` 形状不再被 wire 机器保证，§14 这层闸是唯一防 §4.4.2 漂移的手段。
  后端迁移计划里**把 §14（黄金样本 + 从 Go SSOT 导出 JSON Schema/OpenRPC）提为 B0 硬前置**，先立闸、再迁类型。

**契约视为定稿。** 后端按 lyra `doc/PROTOCOL_ALIGNMENT_REVIEW.md` 第五轮批次迁移：B0（§14 漂移闸，硬前置）→
B1（判别字段/版本/状态词）→ B2（ToolInvocation 去领域化 + tool 输出进 result）→ B3（全 list Page）→
B4（features 开放 map + durable 去字段）→ B5（小字段：N1 删 costUsd / Usage 嵌套 / answers 数组 / channel 等）。
旧 `lyra.db` 历史 blob 不兼容，按 §12 直接丢、不写 migration。
