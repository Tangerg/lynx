# 后端能力交付 · B1(git)+ 去债 · 前端对接说明 · `2026-06-11`

> 接 [`2026-06-10/BACKEND_CAPABILITIES.md`](../2026-06-10/BACKEND_CAPABILITIES.md) 的同款节奏。本轮交付旁路 API 的
> **B1 批次(git/VCS)** + 已对齐的**去债**。形状的正式契约见 [`../AUX_API.md`](../AUX_API.md);本文只讲**怎么调 + 边界 +
> 本轮 capability 变化**。
>
> **第一原则不变**:按 `runtime.initialize` 的 `ServerCapabilities` 协商,不要硬编码方法可用性。

---

## 1. 新增可对接(B1,AUX_API §2)

| 方法 | 能力 | 门控 | 关键边界 |
| --- | --- | --- | --- |
| `workspace.listFileChanges` | 工作区改动列表(±行数、rename、binary、untracked) | `features.git` | 见 §3 退化三态 |
| `workspace.getDiff` | 结构化 / 原始 diff(`mode` worktree\|base、`format` rows\|raw) | `features.git` | 见 §3 + §2 |

### 调法要点

- **`getDiff` 默认 `mode:"worktree"`(含 untracked,新文件以全 added 行出现)+ `format:"rows"`**(per-file `FileDiff`,
  每行 `DiffRow{type:hunk|context|added|deleted}`)。要原始 patch 传 `format:"raw"` → 返 `{patch, truncated?}`。
- **`mode:"base"`** = 相对默认分支 merge-base(`origin/HEAD → main → master`)。**解析不出基线**(无 remote / detached HEAD
  等)→ `invalid_params`(detail「cannot resolve base branch」),**不是** `vcs_unavailable`。
- **`limit`** 仅作用于 `rows`,**按文件边界截断**(整文件出现或不出现)+ `truncated:true`。
- **`added`/`removed` 是 `*int`**:二进制文件**省略**这俩字段 + `binary:true`(不要把 binary 当 0 行)。`previousPath` 仅 rename 有。
- **path** 相对 cwd,越界 → `path_outside_root`。

完整 wire 形状(`WorkspaceFileChange` / `Diff` / `FileDiff` / `GetDiffRequest`)见 [`../AUX_API.md §2`](../AUX_API.md)。

---

## 2. Capability 快照变化(§9)

```jsonc
{
  "features": {
    "git": true,        // ← 新:有 git 二进制(listFileChanges/getDiff 可用)
    "skills": true, "relocate": true, "memory": true, "mcp": true, "reasoning": true
    // "background" 这个 key 已删除(见 §4)
  },
  "streamingMethods": [ "runs.start", "runs.resume", "runs.subscribe" ]  // workspace.subscribe 待 B2
}
```

新增错误 `type`:**`vcs_unavailable`**(§3)。

---

## 3. git 退化三态(契约级,务必区分)

| 情况 | 表现 | 前端 |
| --- | --- | --- |
| 无 git 二进制 | `features.git=false` | **隐藏 VCS 面板、不调** |
| 有 git、cwd 非仓 | `vcs_unavailable` | 提示"非 git 仓",不是空 |
| 有 git、是仓、无改动 | 成功,`files:[]` / `data:[]` | 显示"无改动" |

---

## 4. 移除 / 变更(去债,已生效)

| 项 | 变化 |
| --- | --- |
| `background.*`(list/subscribe/cancel + `notifications.background.update` + `BackgroundTask`/`TaskId`) | **整组删除**;`features.background` key 已移除。前端同轮删 wire(`streamBackgroundUpdates` 等) |
| `items.edit`(+ `EditItemRequest`/`Response`) | **删除**(turn 粒度下"编辑重跑" = rollback+runs.start,B4) |
| `sessions.fork` 的 `fromItemId` | **改名 `fromRunId`**;现传 `fromRunId` 暂返 `checkpoint_unavailable`(真逻辑在 B4),**整段 fork(不传)照常可用** |

> 这三项对应前端"现在就能落地(纯减法/类型对齐)"清单 —— 前后端可同轮完成。

---

## 5. 仍未实现(本批之外,别提前对接)

| 待批次 | 方法 | 调了会 |
| --- | --- | --- |
| B2 | `workspace.subscribe` + `notifications.workspace.event` | `method_not_found` |
| B3 | `workspace.mcp.reconnect` + `McpServer` 5 态富化 | reconnect `method_not_found`;listServers 暂仍旧形状 |
| B4 | `sessions.rollback` + `fork{fromRunId}` 真逻辑 | rollback `method_not_found`;fromRunId `checkpoint_unavailable` |
| B5 | 审批 `remember{scope}` | 旧后端忽略该字段(UI 可先画原型不接线) |

attachments / checkpoints(v2 影子 git)维持 `false`,见 [`../AUX_API.md §1`](../AUX_API.md)。`sessionExport` 现已 `true`(sessions.export 内联 json/md + sessions.import restore)。

---

**前端本轮可动**:① 接 `workspace.getDiff`/`listFileChanges`(`Diff` 重写为 `{files:FileDiff[]}`、`WorkspaceFileChange`
加字段、DiffView/DiffPreview 改造);② 删 `background.*` 全家 + `items.edit` wire + fork 参数改名;③ `shapes.ts` 补
`streamingMethods` 镜像。git 这条现在就有真实数据可联调(serve 目录是个 git 仓即可)。
