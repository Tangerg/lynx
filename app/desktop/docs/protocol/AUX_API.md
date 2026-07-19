# Lyra Runtime Protocol · 旁路 API（定稿 `2026-07-19`）

> **状态：正式契约（canonical）。** 本文是 [`API.md`](./API.md) 的配套契约，定义 Lyra Runtime 的**旁路面**——不经 LLM 的
> 辅助能力：git/VCS、工作区事件流、会话回退与派生、MCP 生命周期、审批 scope。与 `API.md` / [`TRANSPORT.md`](./TRANSPORT.md)
> 互为配套、共用同一套约定，合起来构成完整 wire 契约。
>
> 约定一律承自 `API.md`，不另立：判别字段一律 `type`；字段 / 枚举 camelCase；业务 id 带类型前缀；list 一律返回 `Page<T>`；
> 错误一律 `ProblemData{ type, channel, detail }`、客户端按 `type`（符号名）判错；能力位走 `ServerCapabilities.features` 开放
> map；下行事件沿用 `notifications.*` 信封。类型命名以后端 Go `lyra/internal/delivery/protocol` 为机械 SSOT。
>
> 文内裸 `§x` 指**本文**小节；引 `API.md` 一律写全 `API.md §x.y`。`protocolVersion`：**`2026-07-19`**（与 `API.md` 同）。

---

## 目录

- §1 能力位与错误（旁路面贡献）
- §2 git / VCS
- §3 工作区事件流
- §4 会话回退与派生
- §5 MCP 生命周期
- §6 审批 scope
- §7 明确不做
- 附录 · 类型索引

---

## 1. 能力位与错误（旁路面贡献）

旁路面方法由 `ServerCapabilities.features`（`API.md §9` 开放 map）的下列位门控；缺省 / falsy 即关闭：

| feature       | 门控                                                                         | 关闭时                                                                 |
| ------------- | ---------------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| `git`         | §2 `workspace.listFileChanges` / `getDiff`（git 二进制在 PATH，启动探测）    | 客户端隐藏 VCS 面板、**不发起**这两个调用                              |
| `fileWatch`   | §3 `workspace.subscribe` 的 `watches`（git-state 监视）                      | 带 `watches` 订阅 → `capability_not_negotiated`；不带 `watches` 仍可用 |
| `checkpoints` | §4 `sessions.rollback` 的 `restoreType:"files"\|"both"`（影子 git 文件快照） | 仅 `restoreType:"history"` 可用                                        |

`workspace.subscribe` 计入 `ServerCapabilities.streamingMethods`（`API.md §9`）。

旁路面贡献的错误 `type`（已在 `API.md §8.2` 码表；客户端按 `type` 判错，数字码仅粗分类）：

| `type`                   | code     | channel | 含义                                                                                                  |
| ------------------------ | -------- | ------- | ----------------------------------------------------------------------------------------------------- |
| `vcs_unavailable`        | `-32017` | rpc     | 有 git 二进制、但目标 cwd 不是 git 仓——与"干净仓 = 空结果"区分，与"无 git = `features.git=false`"区分 |
| `session_busy`           | `-32018` | rpc     | session 有 run 在跑，拒绝会破坏正在 append 的历史（`sessions.rollback`）                              |
| `checkpoint_unavailable` | `-32009` | rpc     | `restoreType:"files"\|"both"` 所需快照不可用 / 影子 git 未启用                                        |

---

## 2. git / VCS

`workspace.listFileChanges` / `workspace.getDiff` 读 cwd 的 git 工作区状态，受 `features.git` 门控，并按**三态**回应——客户端据此区分，不把三种情形糊成一个错误：

| cwd 情形             | 回应                                             |
| -------------------- | ------------------------------------------------ |
| 无 git 二进制        | `features.git=false`——客户端隐藏面板、不发起调用 |
| 有 git、cwd 非仓     | `vcs_unavailable`                                |
| 有 git、是仓、无改动 | 成功，空结果（`data: []` / `files: []`）         |

#### `workspace.listFileChanges`

工作区逐文件改动（VCS 扫描态）。

- 入参 `WorkspaceQuery & { cursor?: string; limit?: number }`（`WorkspaceQuery = { cwd?: string }`，缺省 cwd = serve 目录）。
- 返回 `Page<WorkspaceFileChange>`。
- 错误 `vcs_unavailable` / `cwd_unavailable`。

```ts
interface WorkspaceFileChange {
  path: string;
  status: FileStatus; // added | modified | deleted | renamed | untracked（API.md §4.5）
  previousPath?: string; // 仅 renamed：源路径
  added?: number; // + 行；binary 省略（不伪造 0）
  removed?: number; // − 行；binary 省略
  binary?: true; // 二进制：added/removed 省略
}
```

#### `workspace.getDiff`

工作区结构化 / 原始 diff。

- 入参：

  | 字段     | 类型                   | 默认       | 说明                                                                                |
  | -------- | ---------------------- | ---------- | ----------------------------------------------------------------------------------- |
  | `cwd`    | string                 | serve 目录 |                                                                                     |
  | `path`   | string                 | 全量       | 限定单文件 / 子路径（jail，越界 → `path_outside_root`）                             |
  | `mode`   | `"worktree" \| "base"` | `worktree` | worktree=工作区改动（**含 untracked**）；base=相对默认分支 merge-base               |
  | `format` | `"rows" \| "raw"`      | `rows`     | rows=逐文件结构化（前端渲染）；raw=原始 unified patch                               |
  | `limit`  | number                 | ——         | 行上限；超出置 `truncated:true`，**在文件边界截断**（不出半截文件，no silent caps） |

- 返回 `Diff`（sum-type，按 `format` 二选一）。
- 错误 `vcs_unavailable` / `path_outside_root` / `cwd_unavailable` / `invalid_params`。

```ts
interface Diff {
  files?: FileDiff[];
  patch?: string;
  truncated?: boolean;
} // rows → files、raw → patch
interface FileDiff {
  path: string;
  status: FileStatus;
  previousPath?: string;
  added?: number;
  removed?: number;
  binary?: true;
  rows: DiffRow[]; // binary 时为 []；DiffRow 见 API.md §4.5
}
```

> **`mode:"base"` 基线** = `git merge-base HEAD <defaultBranch>`，`defaultBranch` 依次取 `origin/HEAD` → `main` → `master`。
> 解析不出基线（无 remote / 无 `origin/HEAD` / `main`·`master` 不存在 / detached HEAD）→ **`invalid_params`**——**不**塌成空、
> **不**返 `vcs_unavailable`（后者专指"cwd 非仓"）。**`mode:"worktree"` 含 untracked**：untracked 文件以 `status:"untracked"`
>
> - 全 `added` rows 出现。

---

## 3. 工作区事件流

非-run 的工作区状态推送（文件改动 / skills 变更 / MCP 状态），与 run 事件流（`API.md §5`）分层，自成一条常驻流。

#### `workspace.subscribe` — Stream

打开工作区事件流；流式 `notifications.workspace.event`（params `WorkspaceEvent`）。

- 入参 `{ watches?: WatchSpec[] }`。
- 返回 `{}`（空 ack 首帧）+ 随后流式 `WorkspaceEvent`；InProcess / IPC 上即 notification 回调。
- **作用域 = 这条流本身**：watch 集随订阅参数走，无独立 `watch` / `unwatch` 方法——**改 watch 集 = 关流重订**。
- **门控**：`features.fileWatch=false` 时带 `watches` → `capability_not_negotiated`；**不带 `watches`（只收 skills / mcp 事件）始终可用**。

```ts
interface WatchSpec {
  watchId: string;
  cwd?: string;
  path?: string;
} // watchId 客户端起名、回显于 files.changed；cwd 缺省 serve 目录；path 当前未用（见下）

type WorkspaceEvent = { sequence: number } &
  (
  | { type: "files.changed"; watchId?: string; paths: string[]; cwd?: string } // agent 文件工具的精确改动；paths 相对 cwd
  | { type: "skills.changed" } // 不区分 cwd：任何 skill 目录变化触发
  | {
      type: "mcp.serverChanged";
      server: string;
      status?: McpStatus;
      toolCount?: number;
      error?: ProblemData;
    } // 见 §5
  | { type: "schedules.fired"; scheduleId: string }
  | { type: "resync" } // git 状态变更 / 事件丢失 → 全量失效一次
  );
```

**连接边界**（物理细节见 `TRANSPORT.md`）：

- 整个 app **共享一条** workspace 流（run 流之外的第二条常驻连接，多面板复用，不每面板开流）。
- **无 `Last-Event-Id`、不补发**；重订 = 隐式全量失效（等价收到一次 `resync`）——与 run 流的 durable 续传相对照。
- **先订阅、再拉列表**（先订后拉无丢失窗口）。

**watch 模型**（实现约束，决定 `WatchSpec.path` 为何未用）：后端**不递归监视工作树**（macOS Go fsnotify 走 kqueue = 每文件一个 fd，
大树会耗尽 fd；FSEvents 需 cgo 用不了）。改两路覆盖，跨平台：

1. 带 `watches` → 监视该 cwd 的 `.git` 信号集（HEAD / index / refs/heads / ORIG_HEAD / MERGE_HEAD）。git 状态一变（commit / 暂存 /
   checkout / branch / merge，任何进程所为）发去抖 **`resync`** → 客户端重拉 `getDiff` / `listFileChanges`。
2. **agent 自身的文件编辑**（write / edit 工具）由运行时从 run 流**精确推 `files.changed{ cwd, paths }`**——无需 watch、无竞态。
   `shell` 的文件改动不发（参数无法判定；若是 git 操作则走 `.git` 监视）；纯外部进程编辑不实时，降级到下次 git 操作 / 手动刷新。

`sequence` 在 runtime 进程内严格单调。客户端常态按事件 `type` 局部失效（各域一个缓存 key）；若收到的 sequence 不是上一条
`+1`，说明这条 lossy 流发生丢失，必须先全量失效再处理当前事件。重订本身也先全量失效，因此进程重启导致 sequence 重置
不会误用旧缓存。`clientCapabilities.excludedEvents` 按事件 `type` 抑制（如 `["mcp.serverChanged"]`）。**不变量**：事件 `type`
在 run（`API.md §5`）/ workspace 两个事件联合内全局唯一，供 excludedEvents 跨域按名匹配。

---

## 4. 会话回退与派生

turn 粒度、按 `runId` 寻址的两个会话操作：**回退**（就地销毁性截断）与**派生**（快照式复制）。

#### `sessions.rollback`

丢弃某个保留边界之后的全部 run，就地截断会话历史。

- 入参 `{ sessionId: string; toRunId?: string; restoreType?: "history" | "files" | "both" }`。
- 返回 `{ session: Session; droppedRuns: DroppedRun[] }`。
- 错误 `session_not_found` / `run_not_found` / `invalid_params` / `session_busy` / `checkpoint_unavailable`。

```ts
interface DroppedRun {
  run: RunRef; // API.md §4.2
  userInput?: ContentBlock[]; // 该 run 开场 userMessage 的 content（与 StartRunRequest.input 同型，composer 零转换预填）；
} // 子 agent run 无开场用户轮 → 省略
```

- **`toRunId` = inclusive-keep**：保留的**最后一个 root run**（含其停车-续跑的全部段），其后全部丢弃。省略 `toRunId` =
  丢弃全部、回到空会话（覆盖"编辑第一条消息重跑"）。
- `toRunId` **必须是 root run**（子 agent run → `invalid_params`）；未知 → `run_not_found`。
- **就地销毁**：截断聊天历史、删被丢 run 的 Item / 记录、清其悬挂 open interrupt，并**递归 purge 被丢 run 派生的 subagent
  子会话整棵子树**。
- **运行中拒绝**：session 有 run 在跑 → `session_busy`（避免与正在 append 的历史竞争）。
- **`restoreType`**（默认 `"history"`；`"files"` / `"both"` 受 `features.checkpoints` 门控）：
  - `"history"` —— 只回退聊天历史，不动文件。客户端用 `workspace.getDiff` 自查未还原改动。
  - `"files"` —— 只把工作区文件还原到 `toRunId` 的影子-git 快照，历史不动。
  - `"both"` —— 二者，**原子**：files 先行，失败则整体失败、history 不动、返 `checkpoint_unavailable`，**绝不静默降级**。
  - `"files"` / `"both"` **必须带 `toRunId`**（否则 `invalid_params`）；该 run 无快照 → `checkpoint_unavailable`。还原前自动
    快照当前态（可 unrevert）。

#### `sessions.fork`

把会话历史复制到一条新会话（快照语义）。

- 入参 `{ sessionId: string; fromRunId?: string; title?: string }`。
- 返回新 `Session`（继承源 cwd）。
- 错误 `session_not_found` / `run_not_found`。
- 省略 `fromRunId` = 整段 fork（复制全部历史）；给定 = **含 `fromRunId` 在内**截断复制到该 run 边界。
- **随时可调**：只复制**已完结**的 run（in-flight run 不进副本，等价"先 interrupt 再 fork"）——故 fork **无 `session_busy`**，
  与 `sessions.rollback` 的"运行中拒绝"差异即在此。

---

## 5. MCP 生命周期

`mcp.*` 受 `features.mcp` 门控。条目富化（`toolCount` / `authStatus` / `error` 内联），免去 `mcp.servers.list ⨝ mcp.tools.list`
的 join；`mcp.tools.list` 留给详情面板（分页 + `inputSchema`）。

```ts
type McpStatus =
  "connecting" | "connected" | "disconnected" | "failed" | "needsAuth";

interface McpServer {
  name: string;
  status: McpStatus;
  toolCount?: number;
  authStatus?: "none" | "bearerToken" | "oauth" | "notLoggedIn"; // 省略 = 不跟踪鉴权
  error?: ProblemData; // 仅 status:"failed"：dial 失败原因
  description?: string;
}
```

#### `mcp.servers.reconnect`

重连一个 MCP server。

- 入参 `{ server: string }`；**无同步返回**（结果走推送）。
- 进度经 `notifications.workspace.event` 的 `mcp.serverChanged` 投递，**保证顺序 `connecting → (connected | failed | needsAuth)`**：
  客户端按钮 loading 态绑 `connecting`，终态解除。重连成功热刷新工具集，模型即时可见新工具。
- `mcp.serverChanged` 语义：server 条目**增 / 删 / 任意字段变化**均发；`status` **省略 = 条目已不存在**（客户端重拉自知）。

---

## 6. 审批 scope

`InterruptResponse` 的 `approval` 分支（经 `runs.resume` 回传，`API.md §6.1`）携带可选的记忆与一次性改写：

```ts
{ type: "approval";
  decision: "approve" | "deny";
  remember?: { scope: "session" | "project" | "global" };   // 记住该决策；见下
  editedArgs?: Record<string, unknown>;                      // 批准前一次性改写工具入参
  reason?: string }                                          // deny 理由
```

- **`remember` 的 KEY = 工具名**（`ToolInvocation.name`）。按参数模式匹配属规则引擎域，不做。
- **`deny` + `remember` 合法**——记住"拒绝"。**`editedArgs` 一次性**：`remember` 记的是"这个工具"，不是"工具 + 这次的参数"。
- **`scope`**：v1 **仅 `"session"` 真正持久**（内存，进程生命期）；`"project"` / `"global"` wire 上接受、但在持久化位置落地前
  **降级为一次性**（不假装记住，不留"接受却不持久化"的债）。
- 不设 `once`（= 不带 `remember` 的普通 approve）；不设 `ask` / `behavior`（响应本身即那次 ask 的回答，重复）。

审批记忆的读与管理面由 `approval.listRules` / `approval.forgetRule` 提供；全局策略由
`approval.getMode` / `approval.setMode` 提供，见 `API.md` 附录 C.2。

---

## 7. 明确不做

| 项                                                                                                 | 理由                                                                                                       |
| -------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| `background.*`（list / subscribe / cancel + `notifications.background.update` + `BackgroundTask`） | 后台子任务 = 子 agent 的 turn，挂在 run 树上随其流式（`API.md §5.4`）；无需 client 可见的任务注册表。      |
| `items.edit`                                                                                       | turn 粒度下"编辑某条重跑" = `sessions.rollback{ toRunId }` + `runs.start`，item 级精确编辑无独立存在理由。 |
| `sessions.fork.fromItemId`                                                                         | 改用 `fromRunId`：run 边界可靠解析，无需 item↔message join。                                               |

---

## 附录 · 类型索引

本文定义 / 约束的 wire 类型：`WorkspaceFileChange`·`Diff`·`FileDiff`（§2，与 `API.md §4.5` 同源）、`WatchSpec`·`WorkspaceEvent`（§3）、
`DroppedRun`（§4）、`McpServer`·`McpStatus`（§5）、`InterruptResponse.approval` 的 `remember` / `editedArgs`（§6）。
复用 `API.md`：`FileStatus`·`DiffRow`（§4.5）、`RunRef`（§4.2）、`ContentBlock`（§4.3）、`Session`（§4.1）、`Page<T>`（§4.11）、
`ProblemData`·`FieldError`（§4.6）、`WorkspaceQuery`（§7.5）。

---

## 变更说明（migration notes）

- [`20260614/MULTIMODAL_IMAGE_INPUT.md`](./20260614/MULTIMODAL_IMAGE_INPUT.md) —— 图片输入改为内联 base64（image `ContentBlock` 的 `mime` + `data`），删除整套 attachment 上传子系统。`API.md` / `TRANSPORT.md` 已就地更新到该形态；此文集中说明「改了什么 + 前端怎么迁」。

---

> 正式契约。配套同目录 [`API.md`](./API.md) + [`TRANSPORT.md`](./TRANSPORT.md)。
