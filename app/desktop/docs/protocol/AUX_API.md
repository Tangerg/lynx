# Lyra Runtime Protocol · 旁路 API（正式契约）

> **状态：正式契约（canonical）。** 本文是 [`API.md`](./API.md) 的配套补充，定义**旁路 API**——非 LLM 流式的辅助能力：
> git/VCS、工作区通知通道、回退/checkpoints、MCP 生命周期、审批 scope。与 `API.md` 互为配套、自包含。
>
> 约定**全部沿用 `API.md`**：id 带类型前缀；list 统一 `Page<T>`（`{data, nextCursor}`）；错误用 `ProblemData{type, channel, detail}`、
> 客户端按 `type`（符号名）判错；能力位走 `ServerCapabilities.features`；下行事件沿用 `notifications.*` 信封；类型命名以后端 Go
> `lyra/internal/delivery/protocol` 为机械 SSOT。`§x.y` 指 `API.md` 小节。`protocolVersion` 同 `API.md`（`2026-06-07`）。
>
> 本文方法均已在后端方法表（`internal/delivery/dispatch/method_names.go`）注册、已落地。**唯二非全量启用的点**已就地标注：
> `restoreType` 的 `files`/`both`（受 `features.checkpoints`，影子 git 落地前该位 `false`）；`workspace.mcp.authenticate`
> （提案 613 B12，方法表未注册、调用返 `method_not_found`，见 `API.md` 附录 C.6）。

---

## 目录

- §1 Capabilities 增量
- §2 git / VCS
- §3 工作区通知通道
- §4 回退 / Checkpoints
- §5 MCP 生命周期
- §6 审批 scope
- §7 不做 / 已移除
- 附录 · 类型索引

---

## 1. Capabilities 增量 → `API.md §9`

```jsonc
ServerCapabilities.features += {
  "git":         boolean,  // git 二进制在 PATH（启动探测）；false → 客户端隐藏 VCS 面板、不调 getDiff/listFileChanges
  "fileWatch":   boolean,  // workspace.subscribe 的 watches 参数可用（git-state 监视）
  "checkpoints": boolean   // restoreType files/both（影子 git 文件快照）解锁；未落地前 false
}

ServerCapabilities.streamingMethods += [ "workspace.subscribe" ]
```

新增错误 `type`（已并入 `API.md §8.2` 码表）：

| `type` | 码 | 通道 | 含义 |
| --- | --- | --- | --- |
| `vcs_unavailable` | `-32017` | rpc | 有 git、但目标 cwd 不是 git 仓（与"干净仓=空结果"区分） |
| `session_busy` | `-32018` | rpc | 该 session 有 run 在跑，拒绝会破坏在 append 历史的操作（rollback） |
| `checkpoint_unavailable` | `-32009` | rpc | `restoreType:files/both` 无可用快照 / 影子 git 未启用 |

---

## 2. git / VCS → `API.md §7.5`

### 2.1 退化三态（契约级，客户端据此区分）

| 情况 | 表现 |
| --- | --- |
| 无 `git` 二进制 | `features.git=false`；客户端隐藏面板、**不调**这两个方法 |
| 有 git、cwd 非仓 | 返回 `vcs_unavailable` |
| 有 git、是仓、无改动 | 成功，空结果（`files: []` / `data: []`） |

### 2.2 `workspace.listFileChanges`

- 入参：`WorkspaceQuery & { cursor?: string; limit?: number }`（`WorkspaceQuery = { cwd?: string }`，缺省 = serve 目录）。
- 返回：`Page<WorkspaceFileChange>`。
- 错误：`vcs_unavailable` / `cwd_unavailable`。

```ts
interface WorkspaceFileChange {     // = API.md §4.5
  path: string;
  status: FileStatus;               // added | modified | deleted | renamed | untracked
  previousPath?: string;            // 仅 renamed：源路径
  added?: number;                   // 行 +；binary 时省略（不伪造 0）
  removed?: number;                 // 行 -；binary 时省略
  binary?: true;                    // 二进制文件：added/removed 省略
}
```

### 2.3 `workspace.getDiff`

- 入参：

  | 字段 | 类型 | 默认 | 说明 |
  | --- | --- | --- | --- |
  | `cwd` | string | serve 目录 | |
  | `path` | string | 全量 | 限定单文件/子路径（jail 同 §7.5 `path_outside_root`） |
  | `mode` | `"worktree" \| "base"` | `worktree` | worktree=工作区改动（**含 untracked**）；base=相对基线分支 |
  | `format` | `"rows" \| "raw"` | `rows` | rows=结构化（前端渲染）；raw=原始 unified patch |
  | `limit` | number | — | 行上限；超出 → `truncated:true`，**在文件边界截断**（不出半截文件） |

- 返回 `Diff`（sum-type，按 `format` 二选一；= `API.md §4.5`）：

```ts
interface Diff     { files?: FileDiff[]; patch?: string; truncated?: boolean }   // rows→files、raw→patch
interface FileDiff {
  path: string;
  status: FileStatus;               // 同 WorkspaceFileChange
  previousPath?: string;
  added?: number; removed?: number; binary?: true;
  rows: DiffRow[];                  // binary 时为 []；DiffRow 见 API.md §4.5
}
```

- **`mode:base` 基线**：`git diff $(git merge-base HEAD <defaultBranch>)`，`defaultBranch = origin/HEAD → main → master`。
- **`mode:worktree` 含 untracked**：untracked 文件以 `status:"untracked"` + 全 `added` rows 出现。
- 错误：`vcs_unavailable` / `path_outside_root` / `cwd_unavailable` / **`invalid_params`**（`mode:base` 无法解析基线分支——
  无 remote / 无 `origin/HEAD` / `main`·`master` 不存在 / detached HEAD；**不**塌成空、**不**返 `vcs_unavailable`）。

---

## 3. 工作区通知通道 → `API.md §5`（事件联合）+ `§9` + `TRANSPORT §6.4`

非 run 的状态推送（文件改动 / skills 变更 / MCP 状态），与 run 流明确分层。

### 3.1 `workspace.subscribe` — Stream

- 入参：`{ watches?: WatchSpec[] }`，`WatchSpec = { watchId: string; cwd?: string; path?: string }`
  （`watchId` 客户端起名、回显在 `files.changed`；`cwd` 缺省 = serve 目录；`path` 当前未用，见下方 watch 模型）。
- 返回：流式 `notifications.workspace.event`（进 `streamingMethods`）。InProcess/IPC 上即 notification 回调。
- **作用域 = 这条流本身**：watch 集随订阅参数走（无独立 `watch`/`unwatch` 方法）；**改 watch 集 = 关流重订**。
- **连接边界（`TRANSPORT`）**：
  - 整个 app **共享一条** workspace 流（run 流之外的第二条常驻连接；多面板复用，不许每面板开流）。
  - **无 `Last-Event-Id`、不补发**；重订 = 隐式全量失效（等价收到一次 `resync`）。与 run 流的 durable 续传对照。
  - **先订阅、再拉列表**（先订后拉无丢失窗口）。
- **门控**：`features.fileWatch=false` 时带 `watches` → `capability_not_negotiated`；**不带 `watches` 的订阅（只收
  skills/mcp 事件）始终可用**、不受该位门控。

### 3.2 `notifications.workspace.event`

```ts
{ event: WorkspaceEvent }

type WorkspaceEvent =
  | { type: "files.changed";   watchId?: string; paths: string[]; cwd?: string }      // agent 文件工具的精确改动；paths 相对 cwd；watchId 回显注册的 watch
  | { type: "skills.changed" }                                                        // 不区分 cwd：任何 skill 目录变化均触发
  | { type: "mcp.serverChanged"; server: string; status?: McpStatus; toolCount?: number; error?: ProblemData }  // 见 §5；增/删/任意字段变均发
  | { type: "resync" };                                                               // 兜底 / git 状态变更 → 客户端全量失效一次
```

> **watch 模型（实现约束，优于早期"递归文件监听"）**：后端**不递归监视工作树**（macOS Go fsnotify 走 kqueue=每文件一个 fd，
> 大树耗尽 fd；FSEvents 需 cgo 走不了）。改两路覆盖，跨平台：① 带 `watches` → 监视该 cwd 的 `.git` 信号集
> （HEAD/index/refs/heads/ORIG_HEAD/MERGE_HEAD），git 状态一变（commit/暂存/checkout/branch/merge，任何进程所为）发去抖
> **`resync`** → client 重拉 `getDiff`/`listFileChanges`；② **agent 自身编辑**（write/edit 工具）由运行时从 run 流**精确推
> `files.changed{cwd, paths}`**——无需 watch、无竞态。`bash` 的文件改动不发（参数无法判定；若是 git 操作则走 `.git` 监视）；
> 纯外部进程编辑不实时，降级到下次 git 操作 / 手动刷新（同 Claude Code 取舍）。故 `WatchSpec.path` 当前未用。

- 客户端常态按 `type` 局部失效（各域一个缓存 key）；`resync` = 全量兜底。**无 seq**。
- `optOutNotificationMethods` 按 **event `type`** 抑制（如 `["mcp.serverChanged"]`）。
- 不变量：`type` 名在 `run`/`workspace` 两个事件联合内**全局唯一**（供 optOut 跨域按名匹配）。

---

## 4. 回退 / Checkpoints → `API.md §7.2`

turn 粒度，按 `runId` 寻址。`history`（默认）不动文件；`files`/`both` 受 `features.checkpoints` 门控（影子 git）。

### 4.1 `sessions.rollback`

- 入参：`{ sessionId: string; toRunId?: string; restoreType?: "history" | "files" | "both" }`。
- 返回：`{ session: Session; droppedRuns: DroppedRun[] }`。

```ts
interface DroppedRun {
  run: RunRef;                 // API.md §4.2
  userInput?: ContentBlock[];  // 该 run 开场 userMessage 的 content（与 StartRunRequest.input 同型，composer 预填零转换）；
}                              // resume/edit 类续 run 无开场用户轮 → 省略
```

- **`toRunId` = inclusive-keep**：保留的**最后一个 root run**（其延续链一并保留），其后**全部丢弃**。省略 `toRunId` =
  丢弃全部、回到空会话（覆盖"编辑第一条消息重跑"）。
- `toRunId` **必须是 root run**（子 agent run / 延续 run → `invalid_params`）；未知 → `run_not_found`。
- **就地销毁**：截断聊天历史、删被丢 run 的 Item/记录、清其悬挂 open interrupt、并**递归 purge 被丢 run 派生的 subagent
  子会话整棵子树**。
- **运行中拒绝**：session 有 run 在跑 → `session_busy`（避免与在 append 的历史竞争）。
- **`restoreType`（默认 `history`，`files`/`both` 受 `features.checkpoints` 门控）**：
  - `history` → 只回退聊天历史（不动文件）。UI 用 `workspace.getDiff` 自查未还原改动。
  - `files` → 只把工作区**文件**还原到 `toRunId` 的影子-git 快照（历史不动）。
  - `both` → 二者，**原子**：files 先行，失败 → 整体失败、history 不动、返 `checkpoint_unavailable`，**绝不静默降级**。
  - `files`/`both` **必须带 `toRunId`**（否则 `invalid_params`）；该 run 无快照 → `checkpoint_unavailable`。还原前自动快照
    当前态（可 unrevert）。
- 错误：`session_not_found` / `run_not_found` / `invalid_params` / `session_busy` / `checkpoint_unavailable`。

### 4.2 `sessions.fork`

- 入参：`{ sessionId: string; fromRunId?: string; title?: string }`；返回新 `Session`（继承源 cwd）。
- 省略 `fromRunId` = 整段 fork（复制全部历史）；给定 = **含 `fromRunId` 在内**截断复制到该 run 边界。
- **快照语义、随时可调**：只复制**已完结**的 run（in-flight run 不进副本，等价"先 interrupt 再 fork"）。故 fork 无
  `session_busy`——与 rollback 的"运行中拒绝"差异即在此。
- 错误：`session_not_found` / `run_not_found`。

---

## 5. MCP 生命周期 → `API.md §7.5` + `§4.10`

### 5.1 `McpServer`（富化条目，退掉 listServers⨝listTools 的 join）

```ts
type McpStatus = "connecting" | "connected" | "disconnected" | "failed" | "needsAuth";

interface McpServer {
  name: string;
  status: McpStatus;
  toolCount?: number;
  authStatus?: "none" | "bearerToken" | "oauth" | "notLoggedIn";
  error?: ProblemData;     // 仅 status:"failed" 时给（dial 失败原因，tooltip/详情）
  description?: string;
}
```

`workspace.mcp.listTools`（已实现）保留给详情面板（分页 + `inputSchema`）。

### 5.2 `workspace.mcp.reconnect`

- 入参：`{ server: string }`；**无同步返回**（结果走推送）。
- 结果经 `notifications.workspace.event` 的 `mcp.serverChanged` 投递，**保证顺序 `connecting → (connected | failed | needsAuth)`**；
  客户端按钮 loading 态绑 `connecting`，终态解除。重连成功热刷新工具集，模型即时可见新工具。
- `mcp.serverChanged` 语义：server 条目**增 / 删 / 任意字段变化**均发；`status` **省略 = 条目已不存在**（客户端重拉自知）。

### 5.3 `workspace.mcp.authenticate` **（提案 613 B12）**

- 入参：`{ server: string; token: string }`；**无同步返回**（异步，同 `reconnect`）。后端拿 token 重连，经 `mcp.serverChanged`
  推 `connecting → (connected | needsAuth | failed)`。**后端只转发、不存** token；OAuth 浏览器流（若有）是用户侧的事。
- **状态**：后端方法表未注册，当前调用返 `method_not_found`。详见 `API.md` 附录 C.6。

---

## 6. 审批 scope → `API.md §6.1`

`InterruptResponse` 的 approval 分支（经 `runs.resume` 回传）：

```ts
{ type: "approval";
  decision: "approve" | "deny";
  remember?: { scope: "session" | "project" | "global" };   // 见下；v1 仅 session 真正持久
  editedArgs?: Record<string, unknown>;                      // 批准前一次性改写工具入参（wire 已有）
  reason?: string }                                          // deny 理由
```

- **`remember` 的 KEY = 工具名**（`ToolInvocation.name`）。按参数模式匹配是规则引擎/config 域的事，不做。
- **`editedArgs` 一次性**；`remember` 记的是"这个工具"，不是"工具+这次改的参数"。
- **`deny` + `remember` 合法**（记住拒绝）。
- **`scope`**：v1 **仅 `session` 真正持久**（内存，进程生命期）；`project`/`global` wire 上接受、但在持久化位置落地前**降级为
  一次性**（不假装记住，不留"接受但不持久化"的债）。
- 不收 `once`（= 不带 `remember` 的普通 approve）；不收 `ask`/`behavior`（响应即那次 ask 的回答，重复）。

> 本节是审批记忆的**写入**面（HITL 应答）。其**读 + 管理**面（全局 `ApprovalMode`、`listRemembered`/`forget`）见 `API.md`
> 附录 C.3（B9，提案）。

---

## 7. 不做 / 已移除

| 项 | 原因 |
| --- | --- |
| `background.*`（list/subscribe/cancel + `notifications.background.update` + `BackgroundTask`/`TaskId`） | 八家对照 agent 无一有 client 可见任务注册表；后台子任务 = 子 agent 的 turn，挂 run 树随其流式（`API.md §5.4`）。 |
| `items.edit` | turn 粒度下"编辑重跑" = `sessions.rollback{toRunId}` + `runs.start`，item 精确编辑无独立存在理由。 |
| `sessions.fork.fromItemId` | 改 `fromRunId`（run 边界可解，无需 item↔message join）。 |

---

## 附录 · 类型索引

本文定义/约束的 wire 类型：`WorkspaceFileChange`·`Diff`·`FileDiff`（§2，与 `API.md §4.5` 同源）、`WatchSpec`·`WorkspaceEvent`（§3）、
`DroppedRun`（§4.1）、`McpServer`·`McpStatus`（§5.1）、`InterruptResponse.approval` 的 `remember`/`editedArgs`（§6）。
复用 `API.md`：`DiffRow`·`FileStatus`（§4.5）、`RunRef`（§4.2）、`ContentBlock`（§4.3）、`Session`（§4.1）、`Page<T>`（§4.11）、
`ProblemData`（§4.6）、`WorkspaceQuery`（§7.5）。

---

> 正式契约。配套同目录 [`API.md`](./API.md) + [`TRANSPORT.md`](./TRANSPORT.md)。
