# Lyra Runtime Protocol · 旁路 API 规范（`v1`，定稿 `2026-06-11`）

> 本文是 [`API.md`](./API.md) 的**正式补充契约**，定义"旁路 API"——非 LLM 流式的辅助能力：**git/VCS、工作区通知通道、
> checkpoints/回退、审批 scope、MCP 生命周期**，以及一批**移除项**。前后端按本文**并行开工**;落地时各方法并入 `API.md`
> 对应小节(每条标了落点),本文届时退为导读。
>
> 设计依据见 `2026-06-10/` 目录的调研与评审记录(8 家 agent + 四轮评审,已定稿)。本文只写**契约**,不含讨论。
> 约定沿用 `API.md`:id 带类型前缀;list 统一 `Page<T>`(`{data, nextCursor}`);错误用 `ProblemData{type, channel, detail}`,
> 客户端按 `type`(符号名)判错;能力位走 `ServerCapabilities.features`;下行事件沿用 `notifications.*` 信封。
> `§x.y` 指 `API.md`。标 **[v1]** 本期交付,**[v2]** 形状已定、暂不实现。

---

## 1. Capabilities 与协商增量 → `API.md §9`

```jsonc
ServerCapabilities.features += {
  "git":        boolean,   // git 二进制在 PATH(启动探测);false → 客户端隐藏 VCS 面板、不调 workspace.getDiff/listFileChanges
  "fileWatch":  boolean,   // 文件监听已接(workspace.subscribe 的 watches 参数可用)
  "checkpoints": false     // [v2] 影子 git + restoreType 解锁后才 true
}

ServerCapabilities.streamingMethods += [ "workspace.subscribe" ]
```

**新增错误 `type`**（→ `API.md §8.2` 码表）:

| `type` | 通道 | 含义 |
| --- | --- | --- |
| `vcs_unavailable` | rpc | 有 git、但目标 cwd 不是 git 仓(与"干净仓=空结果"区分) |
| `session_busy` | rpc | 该 session 有 run 在跑,拒绝会破坏在 append 历史的操作(rollback) |

**移除**（→ 见 §7）:`background.*`、`items.edit`、`sessions.fork.fromItemId`。

---

## 2. git / VCS  **[v1]** → `API.md §7.5`

### 2.1 退化三态(契约级,客户端据此区分)

| 情况 | 表现 |
| --- | --- |
| 无 `git` 二进制 | `features.git=false`；客户端隐藏面板、**不调**这两个方法 |
| 有 git、cwd 非仓 | 返回 `vcs_unavailable` |
| 有 git、是仓、无改动 | 成功,空结果(`files: []` / `data: []`) |

### 2.2 `workspace.listFileChanges`

- 入参:`WorkspaceQuery & { cursor?: string; limit?: number }`（`WorkspaceQuery = { cwd?: string }`,缺省 = serve 目录）。
- 返回:`Page<WorkspaceFileChange>`。

```ts
interface WorkspaceFileChange {
  path: string;
  status: "added" | "modified" | "deleted" | "renamed" | "untracked";
  previousPath?: string;   // 仅 renamed:源路径
  added?: number;          // 行 +;binary 时省略(不伪造 0)
  removed?: number;        // 行 -;binary 时省略
  binary?: true;           // 二进制文件:added/removed 省略
}
```

- 错误:`vcs_unavailable` / `cwd_unavailable`。

### 2.3 `workspace.getDiff`

- 入参:

  | 字段 | 类型 | 默认 | 说明 |
  | --- | --- | --- | --- |
  | `cwd` | string | serve 目录 | |
  | `path` | string | 全量 | 限定单文件/子路径(jail 同 §7.5 `path_outside_root`) |
  | `mode` | `"worktree" \| "base"` | `worktree` | worktree=工作区改动(**含 untracked**);base=相对基线分支 |
  | `format` | `"rows" \| "raw"` | `rows` | rows=结构化(前端渲染);raw=原始 unified patch |
  | `limit` | number | — | 行上限;超出 → `truncated:true`,**在文件边界截断**(不出半截文件) |

- 返回 `Diff`(sum-type,按 `format` 二选一):

```ts
interface Diff {
  files?: FileDiff[];   // format=rows
  patch?: string;       // format=raw（原始 unified patch）
  truncated?: boolean;
}
interface FileDiff {
  path: string;
  status: "added" | "modified" | "deleted" | "renamed" | "untracked";
  previousPath?: string;
  added?: number; removed?: number; binary?: true;   // 同 WorkspaceFileChange
  rows: DiffRow[];      // binary 时为 []
}
interface DiffRow {     // 同 §4.5,Type ∈ hunk|context|added|deleted
  type: "hunk" | "context" | "added" | "deleted";
  text?: string;        // hunk header
  leftLine?: number; rightLine?: number; code?: string;
}
```

- **`mode:base` 基线**:`git diff $(git merge-base HEAD <defaultBranch>)`,`defaultBranch = origin/HEAD → main → master`。
- **`mode:worktree` 含 untracked**:untracked 文件以 `status:"untracked"` + 全 `added` rows 出现。
- 错误:`vcs_unavailable` / `path_outside_root` / `cwd_unavailable` / **`invalid_params`**（`mode:base` 无法解析基线分支——
  无 remote / 无 `origin/HEAD` / `main`/`master` 不存在 / detached HEAD;**不**塌成空、**不**返 `vcs_unavailable`）。

---

## 3. 工作区通知通道  **[v1]** → `API.md §5`(事件联合)+ `§9` + `TRANSPORT §6.4`

非 run 的状态推送(文件改动 / skills 变更 / MCP 状态)。与 run 流明确分层。

### 3.1 `workspace.subscribe` — Stream

- 入参:`{ watches?: WatchSpec[] }`,`WatchSpec = { watchId: string; cwd?: string; path: string }`
  （`watchId` 客户端起名;`cwd` 缺省 = serve 目录;`path` 相对该 watch 的 cwd,jail 同 §7.5）。
- 返回:流式 `notifications.workspace.event`（进 `streamingMethods`）。InProcess/IPC 上即 notification 回调。
- **作用域 = 这条流本身**:watch 集随订阅参数走(无独立 `watch`/`unwatch` 方法);**改 watch 集 = 关流重订**。
- **连接边界(`TRANSPORT`)**:
  - 整个 app **共享一条** workspace 流(run 流之外的第二条常驻连接;多面板复用,不许每面板开流)。
  - **无 `Last-Event-Id`、不补发**;重订 = 隐式全量失效(等价收到一次 `resync`)。与 run 流的 durable 续传对照。
  - **先订阅、再拉列表**(先订后拉无丢失窗口)。
- **门控**:`features.fileWatch=false` 时带 `watches` → `capability_not_negotiated`;**不带 `watches` 的订阅(只收
  skills/mcp 事件)始终可用**、不受该位门控。

### 3.2 `notifications.workspace.event`

```ts
{ event: WorkspaceEvent }

type WorkspaceEvent =
  | { type: "files.changed";   watchId: string; paths: string[] }       // paths 相对该 watch 的 cwd
  | { type: "skills.changed" }                                          // 不区分 cwd:任何 skill 目录变化均触发
  | { type: "mcp.serverChanged"; server: string; status?: McpStatus;    // 见 §5;增/删/任意字段变均发
      toolCount?: number; error?: ProblemData }
  | { type: "resync" }                                                  // 兜底:丢过事件 → 客户端全量失效一次
```

- 客户端常态按 `type` 局部失效(各域一个缓存 key);`resync` = 全量兜底。**v1 无 seq**。
- `optOutNotificationMethods` 按 **event `type`** 抑制(如 `["mcp.serverChanged"]`)。
- 不变量:`type` 名在 `run`/`workspace` 两个事件联合内**全局唯一**(供 optOut 跨域按名匹配)。

---

## 4. Checkpoints / 回退 → `API.md §7.2`（fork 改写）+ 新增 rollback + `§7.4`（删 items.edit）

**[v1]** turn 粒度,按 `runId` 寻址,**不动文件**。

### 4.1 `sessions.rollback`

- 入参:`{ sessionId: string; toRunId?: string }`。
- 返回:`{ session: Session; droppedRuns: DroppedRun[] }`。

```ts
interface DroppedRun {
  run: RunRef;                 // §4.2
  userInput?: ContentBlock[];  // 该 run 开场 userMessage 的 content（与 StartRunRequest.input 同型,composer 预填零转换;
}                              // resume/edit 类续 run 无开场用户轮 → 省略
```

- **`toRunId` = inclusive-keep**:保留的**最后一个** root run,其后全部丢弃。**省略 `toRunId` = 丢弃全部 root run、回到空会话**
  （覆盖"编辑第一条消息重跑"）。
- `toRunId` **必须是 root run**（子 agent run / continuation run → `invalid_params`）。丢弃一个 run 时,其 continuation 链
  与 subagent 子树一并丢弃;悬在被丢轮次上的 open interrupt 一并清理（`runs.listOpenInterrupts` 不再返回）。
- **运行中拒绝**:session 有 run 在跑 → `session_busy`。
- 不动文件:UI 用 `workspace.getDiff` 自查当前未还原改动（v1 无快照,不归因）。
- 错误:`session_not_found` / `run_not_found`（未知 toRunId）/ `invalid_params`（非 root run）/ `session_busy`。

### 4.2 `sessions.fork`

- 入参:`{ sessionId: string; fromRunId?: string; title?: string }`（`fromRunId` 取代旧 `fromItemId`）。
- 返回:新 `Session`。
- 省略 `fromRunId` = 整段 fork（复制全部历史）;给定 = **含 `fromRunId` 在内**截断复制。
- **随时可调**（快照语义）:**只复制已完结的 run**,in-flight run 不进副本（等价"先 interrupt 再 fork"）。与 rollback 的
  "运行中拒绝"差异即在此。
- 错误:`session_not_found` / `run_not_found`。

### 4.3 **[v2]** 影子 git + `restoreType`

```ts
// 启用后 rollback/fork 增量:
{ ...; restoreType?: "history" | "files" | "both" }   // 默认 history
```

- 快照锚在 run 边界（snapshot ref 盖在 durable `RunRef`/`Item` 上）。
- **`both` 原子**:files 先行,files 失败 → 整体失败、history 不动、返明确错误,**绝不静默降级 history**。
- files/both 还原前自动快照当前态（unrevert 可逆）。
- 受 `features.checkpoints` 门控。

---

## 5. MCP 生命周期  **[v1]** → `API.md §7.5` + `§4.10`

### 5.1 `McpServer`（富化条目,退掉 listServers⨝listTools 的 join）

```ts
type McpStatus = "connecting" | "connected" | "disconnected" | "failed" | "needsAuth";

interface McpServer {
  name: string;
  status: McpStatus;
  toolCount?: number;
  authStatus?: "none" | "bearerToken" | "oauth" | "notLoggedIn";
  error?: ProblemData;     // failed 必带原因(tooltip/详情)
  description?: string;
}
```

`workspace.mcp.listTools`（已实现）保留给详情面板（分页 + inputSchema）。

### 5.2 `workspace.mcp.reconnect`

- 入参:`{ server: string }`;无同步返回（结果走推送）。
- 结果经 `notifications.workspace.event` 的 `mcp.serverChanged` 投递,**保证顺序 `connecting → (connected | failed | needsAuth)`**;
  客户端按钮 loading 态绑 `connecting`,终态解除。
- `mcp.serverChanged` 语义:server 条目**增 / 删 / 任意字段变化**均发;`status` **省略 = 条目已不存在**（客户端重拉自知）。

---

## 6. 审批 scope  **[v1]** → `API.md §6.1`

`InterruptResponse` 的 approval 分支增量:

```ts
{ type: "approval";
  decision: "approve" | "deny";
  remember?: { scope: "session" };   // [v1] 仅 session(内存);project|global additive 后补
  editedArgs?: Record<string, unknown> }   // = 批准前改写的工具入参(wire 已有)
```

- **`remember` 的 KEY = 工具名**（`ToolInvocation.name`）。按参数模式匹配是规则引擎/config 域的事,v1 不做。
- **`editedArgs` 一次性**;`remember` 记的是"这个工具",不是"工具+这次改的参数"。
- **`deny` + `remember` 合法**（记住拒绝）。
- v1 `scope` 只放 `session`:`project|global` 需持久化位置,未落地前不进枚举（接受但不持久化 = 债）。
- 不收 `once`（= 不带 `remember` 的普通 approve）;不收 `ask`/`behavior`（响应即那次 ask 的回答,重复）。

---

## 7. 移除项  **[v1]**

| 移除 | 原因 → 落点 |
| --- | --- |
| `background.*`（list/subscribe/cancel + `notifications.background.update` + `BackgroundTask`/`TaskId`） | 八家 agent 无一有 client 可见任务注册表;后台子任务 = 子 agent 的 turn,挂 run 树流式。→ 删 `§7.7`/`§7.8` |
| `items.edit` | turn 粒度下"编辑重跑" = `sessions.rollback` + `runs.start`,item 精确编辑无存在理由。→ 删 `§7.4` |
| `sessions.fork.fromItemId` | 改 `fromRunId`（run 边界可解）。→ 改 `§7.2` |

`features.checkpoints` 能力位语义从「items.edit / fork-at-item」**改写为「restoreType(v2 文件快照)」**。

---

## 8. 实现批次 × 落点（供前后端排期）

> **状态（2026-06-11）：B1–B6 + 去债 后端全部落地、已并入 `API.md`。** 对接说明见
> [`2026-06-11/BACKEND_CAPABILITIES.md`](./2026-06-11/BACKEND_CAPABILITIES.md)（B1+去债）+
> [`2026-06-11/BACKEND_CAPABILITIES_B2-B6.md`](./2026-06-11/BACKEND_CAPABILITIES_B2-B6.md)（其余全部）。

| 批次 | 内容 | API.md 落点 | 前端同轮动作 |
| --- | --- | --- | --- |
| **B1** | git `listFileChanges` + `getDiff` + `features.git`/`vcs_unavailable` | §7.5 + §8.2 | `Diff` 重写为 `{files: FileDiff[]}`;`WorkspaceFileChange` 加字段;DiffView/DiffPreview 改造 |
| **B2** | `workspace.subscribe` + `notifications.workspace.event` 联合 + resync | §5 + §9 + TRANSPORT §6.4 | 接 workspace.subscribe 流 + `WorkspaceEvent` 派发(复用 run 事件 fold 基建) |
| **B3** | `files.changed`(随 subscribe 的 watches)+ `mcp.reconnect`/`serverChanged` + `McpServer` 富化 | §7.5 + §4.10 | `McpServer` 状态 3→5 态 + 退掉 listServers⨝listTools join |
| **B4** | `sessions.rollback` + `fork{fromRunId}` + 删 `items.edit` | §7.2/§7.4 + §8.2(session_busy) | fork 参数 `fromItemId→fromRunId`;删 `items.edit` wire;rollback/编辑重跑 UI |
| **B5** | 审批 `remember{scope:session}` + `editedArgs` | §6.1 | 审批卡加 remember 下拉(本次/本会话两项) |
| **去债** | 砍 `background.*` | §7.7/§7.8 删 | 删 `BackgroundTask`/`TaskId`/`streamBackgroundUpdates`/methods |

每批后端落地 = 改 `API.md` 正文 + 更新 `2026-06-10/BACKEND_CAPABILITIES.md`(怎么调 + 边界);前端按本表同轮跟进。
存量 drift 一并修:`shapes.ts` 补 `ServerCapabilities.streamingMethods` 镜像(§9 已有、前端漏镜像)。

---

## 附录 · 类型索引

本文新增/变更的 wire 类型:`WorkspaceFileChange`(§2.2)、`Diff`/`FileDiff`(§2.3)、`WatchSpec`/`WorkspaceEvent`(§3)、
`DroppedRun`(§4.1)、`McpServer`/`McpStatus`(§5.1)、`InterruptResponse.approval` 增量(§6)。复用既有:`DiffRow`(§4.5)、
`RunRef`(§4.2)、`ContentBlock`(§4.7)、`Session`(§4.1)、`Page<T>`(§4.11)、`ProblemData`(§4.6)、`WorkspaceQuery`(§7.5)。
