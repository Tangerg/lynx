# 契约审查反馈 · `2026-06-07`

> 后端（lyra）侧对本目录 `API.md` / `TRANSPORT.md` 的审查意见。**先定稿契约、再迁后端**——避免后端忠实
> 复制契约里的问题。整体评价：本版大幅改进，几乎全部采纳了上一轮关于"领域中立核心 / 统一 `type` 判别 /
> question 自包含 / 全 list 用 Page / 开放 features / durable 去冗余 / 错误自描述 channel"等建议，方向正确。
>
> 下面是本版**新引入**的问题，按需要前端动 contract 的程度分级：⭐ 应改 / ◐ 建议改 / ○ note。
> 记号 §x.y 指本目录 API.md / TRANSPORT.md 的节号。

---

## ⭐ N1 — 总成本字段重复（`Usage extends ModelUsage` 的副作用）

**位置**：API.md §4.6（Usage / ModelUsage）、§4.2（RunResult）、§5（RunProgress）。

**问题**：本版让 `Usage extends ModelUsage`，而 `ModelUsage` 带 `costUsd`。于是总成本出现在**两个地方**：

```ts
interface ModelUsage { …; costUsd?: number }
interface Usage extends ModelUsage { byModel?: Record<string, ModelUsage> }  // ← Usage 也继承了 costUsd
interface RunResult { usage?: Usage; costUsd?: number; … }                   // ← 又有一个 costUsd
```

- `RunResult.costUsd` 与 `RunResult.usage.costUsd` 都表示**总成本** → 客户端不知该读哪个、可能不一致。
- `RunProgress { …, usage?: Usage, costUsd?: number }` 同样重复。

上一版 `Usage` 没有 `costUsd`（总成本只在 `RunResult.costUsd`，per-model 成本在 `byModel[*].costUsd`），本是
干净的。让 Usage 与 ModelUsage 对称（为补 cacheRead/cacheWrite）时，把 `costUsd` 一起提到了共享基底，意外
引入这处重复。

**建议**：总成本**只留一处**。最干净：

- 删 `RunResult.costUsd` 与 `RunProgress.costUsd`；总成本统一读 `usage.costUsd`。
- `costUsd` 保留在 `ModelUsage` 上——它对 `byModel[*]`（per-model 成本）是必要的，对顶层 `Usage` 即"总成本"。

> 或反过来：`costUsd` 不放进共享的 `ModelUsage`，只在 `byModel` 的条目类型和 `RunResult` 上各有一个。
> 但 (建议) 那条更简单：一个 `ModelUsage` 形状，顶层即总量、byModel 即分量，`RunResult`/`RunProgress` 不再
> 自带 `costUsd`。

---

## ⭐ N3 — §2.4 断言"id 字典序≈创建时间序"，但后端 SSOT 当前不满足

**位置**：API.md §2.4（资源 id）、§4.11（分页）。

**问题**：§2.4 新增普适断言——

> id 内嵌创建时间戳，**字典序 ≈ 创建时间序**…历史稳定排序、游标分页、日志排序都不需要额外 seq 字段——
> 分页 `cursor` 即可直接用一个资源 id 表达。

但后端（机械 SSOT）当前的 id 生成实测**不满足**：

| 资源 | 实际生成 | 时间可排序？ |
| --- | --- | --- |
| Session | `ses_` + `uuid.NewString()`（**UUIDv4，纯随机**） | ❌ |
| Item | `item_<runId>_<seq>`（run 内序号） | 仅 run 内局部 |
| Event | `evt_<11 位补零 seq>`（进程内单调，**重启重置**） | 仅进程内 |

即 **SSOT 违反了契约自己的 §2.4**。§4.11 靠"cursor 不透明、server 可用任意编码"兜住了**分页正确性**，但
§2.4 写成了**普适属性**——客户端若据此"按 id 排序 = 时间序"会出错（尤其 session 列表）。

**建议二选一**：

- **(a) 后端换时间可排序 id**（ULID / KSUID 式：前缀 + 时间高位 + 随机低位，保留 `ses_`/`run_`/`item_` 前缀），
  真正兑现 §2.4——则 §2.4 成立、cursor-by-id 有意义。后端愿意做的话最理想。
- **(b) 弱化 §2.4**：改为"id **可选**时间可排序；client 一律视为**不透明**、**不据 id 推时间序**；排序锚定
  `createdAt`（ISO-8601）"。cursor 仍可不透明地由 server 用任意编码（§4.11 已支持）。

> 需要前端确认：当前前端有没有"按 id 字典序排会话/历史"的逻辑？有 → 必须走 (a) 或改前端排序键为 `createdAt`。

---

## ◐ N2 — `FileChange` 与 `FileChangeEntry`：两个近义类型 + 枚举分叉

**位置**：API.md §4.5。

**问题**：两个高度相似、又不完全一样的类型并存，且分类枚举词汇分叉：

```ts
interface FileChange      { path; status: "added"|"modified"|"deleted"|"renamed"|"untracked" }  // workspace VCS 视图
interface FileChangeEntry { path; type:   "add"|"modify"|"delete"|"rename"; diff? }              // 工具编辑结果
```

- 命名撞车（`FileChange` / `FileChangeEntry`），新人难分。
- 过去式（`added`）vs 祈使式（`add`）两套词；`untracked` 只在 `FileChange` 里。
- 一个用 `status`、一个用 `type`（虽都合 §2.1，但同域两种命名加深混淆）。

二者确实是两件事（VCS 工作区状态 vs 一次编辑的应用结果），但当前形状没把这层区分讲清。

**建议**：重命名区分意图（如 `WorkspaceFileChange` / `EditedFile`，或 `FileStatus` / `FileEdit`），并在 §4.5
注明"为何两个、各用在哪"。能统一枚举词汇更好（但 VCS 的 `untracked` 在编辑结果里确实无意义，可保留差异并
说明）。

---

## ○ note（小修 / 记录在案，多半可保留）

- **N4** §5.2 落点表：`item.delta{toolOutput}` 的权威落点写的是 `tool.result`，更精确是 **`tool.result.output`**
  （result 现在是对象 `{exitCode, output, outputTruncated?}`，§4.4.2）。措辞收紧一下，免得实现以为整 `result`
  由 toolOutput delta 累积。

- **N5** `workspace.getDiff` 返回裸 `DiffRow[]`、无上限/无截断信号——全仓 diff 可能很大，绕过了 §4.11 给 list
  立的"no silent caps"纪律。建议加 `limit?` + 截断信号（参照 `GrepResult.total ≥ matches.length` 的自描述）。

- **N6**（S4 的既定代价，记录）删掉 `RunEvent.durable` 后，SSE 重放层要靠 `event.type`（+ `custom.durable`）判
  该不该带 `id:` / 是否可重放——transport 因此需要读业务事件语义。TRANSPORT §6.4/§7 已写明，可接受；只是把
  "每帧自描述 durable"换成了"transport 懂事件表"。无需改，标注。

- **N7** G1 落地后（通用 tool + **非规范**展示约定 §4.4.2），富渲染的 `result` 形状**不再机器保证**。这意味着
  §14 的**黄金样本契约测试 + 从 SSOT 导出 JSON Schema/OpenRPC 不再是"建议"而是"必需"**——否则 §4.4.2 的
  约定（bash 的 `{exitCode,output,outputTruncated}`、grep 的 `{hits}` 等）会在前后端间无声漂移，正是它本想防
  的那类 bug。建议把 §14 标成迁移的硬前置项，而非可选。

---

## 处理建议

- **N1**：纯契约笔误，删一个重复字段即可，前端改 contract（顺带通知后端按定稿迁移）。
- **N3**：需决策——后端换 ULID 式 id（兑现 §2.4），或前端弱化 §2.4 + 排序改用 `createdAt`。请前端确认有无
  "按 id 排序"的依赖。
- **N2**：命名 + 文档，前端改 contract。
- **N4/N5**：契约措辞 / 加字段，前端定。
- **N6/N7**：N6 仅记录；N7 建议把 §14 提为迁移硬前置。

后端这边：上述定稿后，按 lyra `doc/PROTOCOL_ALIGNMENT_REVIEW.md` 第五轮的 B1–B6 批次迁移对齐（含丢弃旧
`lyra.db` 历史 blob，§12 dev 无 migration）。
