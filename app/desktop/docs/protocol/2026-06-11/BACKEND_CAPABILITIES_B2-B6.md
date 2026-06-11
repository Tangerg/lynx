# 后端能力交付 · B2–B6 全部落地 · 前端对接说明 · `2026-06-11`

> 接 [`BACKEND_CAPABILITIES.md`](./BACKEND_CAPABILITIES.md)（B1 git + 去债）。本轮交付旁路 API 的**其余全部批次**：
> B2(workspace 事件流) · B3(files.changed + MCP 生命周期 + McpServer 富化) · B4(rollback/fork{fromRunId}) ·
> B5(审批 remember/editedArgs) · B6(文件监听)。**至此 AUX_API 契约的后端实现全部闭环。**
>
> 正式 wire 形状见 [`../AUX_API.md`](../AUX_API.md) + 已并入的 [`../API.md`](../API.md)；本文讲**怎么调 + 边界 + capability 变化**。
> **第一原则**:按 `runtime.initialize` 的 `ServerCapabilities` 协商,不要硬编码方法可用性。

---

## 0. Capability 快照变化（§9）

```jsonc
{
  "features": {
    "git": true, "fileWatch": true,            // ← 本轮新:fileWatch(文件监听)
    "mcp": true, "skills": true, "memory": true, "relocate": true, "reasoning": true,
    "checkpoints": false,                       // 仍 false:语义已改写为 v2 影子 git+restoreType(B4 只做 history 维度)
    "multimodal": false, "attachments": {"enabled": false},
    "sessionExport": false, "clientTools": false, "subagents": false
    // "background" key 已删除
  },
  "streamingMethods": ["runs.start","runs.resume","runs.subscribe","workspace.subscribe"]
}
```

新增错误 `type`:**`session_busy`**（`-32018`，rollback 用）。`vcs_unavailable`（`-32017`）见 B1 那篇。

> **注意 `sessions.rollback` / `fork{fromRunId}` 不受 `features.checkpoints` 门控** —— 那个能力位现在专指 v2 文件快照
> （`restoreType`）。turn 粒度的 history 回退/分叉是**常备方法**,直接可调。

---

## 1. B2 + B6 — 工作区事件流 + 文件监听（AUX_API §3 / §5）

**`workspace.subscribe`(流式)** 打开一条非-run 事件流。全局事件(`mcp.serverChanged` / `skills.changed`)发给每个订阅;
传 `watches` 则额外注册**本订阅**的文件监听,`files.changed` 走同一条流。复用 run 事件流那套 fold 基建即可。

```ts
// 入参
{ watches?: { watchId: string; cwd?: string; path?: string }[] }
// 流帧 params(notifications.workspace.event)
interface WorkspaceEvent {
  type: "files.changed" | "skills.changed" | "mcp.serverChanged" | "resync";
  watchId?: string; paths?: string[];                                   // files.changed
  server?: string; status?: string; toolCount?: number; error?: ProblemData; // mcp.serverChanged
}
```

### 调法要点

- **无 watches 的 subscribe 总是可用**（只收全局事件);带 watches 受 `features.fileWatch`。
- **`WatchSpec.path` 相对 `cwd`**（默认 serve 目录）、被 jail:越界 → `path_outside_root`;非目录 → `invalid_params`;
  缺 `watchId` → `invalid_params`。**空 path = 监听 cwd 根**。watcher 递归(子目录自动纳入,新目录出现也纳入)。
- **`files.changed` 防抖**(~150ms 合并一阵 fs 事件成一条)+ **lossy**(订阅方慢则丢,语义是「变了→重拉」,下次变更或 `resync` 自愈)。
  `paths` 相对该 watch 的 `cwd`,`watchId` 回显(client 据此路由到对应 watch)。
- 流生命周期 = 订阅;client 断开即结束(server 侧 watcher 随之关闭)。

---

## 2. B3 — MCP 生命周期 + McpServer 富化（AUX_API §5）

### 2.1 `workspace.mcp.listServers` 现在是**真实 5 态**

```ts
interface McpServer {
  name: string;
  status: "connecting" | "connected" | "disconnected" | "failed" | "needsAuth";
  toolCount?: number;                                   // 内联,省去 ⨝ listTools
  authStatus?: "none"|"bearerToken"|"oauth"|"notLoggedIn";
  error?: ProblemData;                                  // 仅 status:"failed"(type:"mcp_dial_failed")
  description?: string;
}
```

- **启动容错**:单个 MCP server 连不上**不再炸整个 runtime** —— 它以 `status:"failed"` + `error` 出现在列表里,其余正常。
  （配置错误如重名 / 空 endpoint 仍是 fatal 启动失败,那是 operator 笔误。）
- **`toolCount` 已内联** → 前端可退掉 `listServers ⨝ listTools` 的 join。
- **`needsAuth` v1 不产出**（dial 暂不暴露可区分的鉴权错,不造假;authStatus 同理可省略 = 未跟踪）。

### 2.2 `workspace.mcp.reconnect` — 异步 + 推送

- 入参 `{ server: string }`,**无同步返回**;未知 server → `invalid_params`(同步)。
- 结果经 `mcp.serverChanged` 投递,**保证顺序 `connecting → (connected | failed)`**:
  - 按钮 loading 态绑 `connecting`,终态(`connected`/`failed`)解除。
  - `connected` 帧带新的 `toolCount`;`failed` 帧带 `error`。**重连成功会热刷新工具集,模型即时可见新工具**(无需重启)。
  - `status` 省略 = 该 server 条目已不存在(client 重拉自知)。
- `mcp.serverChanged` 经 `workspace.subscribe` 流送达 —— 所以**要先开着 subscribe** 才收得到。

---

## 3. B4 — turn 粒度回退 / 分叉（AUX_API §4）

> 桥:聊天消息是无 run 标记的扁平 log,后端给每个 run 记一个**结束时的消息水位**,rollback/fork 按 run 边界换算到消息条数。

### 3.1 `sessions.rollback{ sessionId, toRunId? }` → `{ session, droppedRuns: DroppedRun[] }`

```ts
interface DroppedRun { run: RunRef; userInput?: ContentBlock[] }
```

- **`toRunId` = inclusive-keep**:保留的最后一个 **root run**(其延续链一并保留),其后全丢。**省略 = 清空回到空会话**（覆盖「编辑第一条消息重跑」）。
- `toRunId` **必须是 root run**(子 agent / 延续 run → `invalid_params`);未知 → `run_not_found`。
- **就地销毁**:截断聊天历史 + 删被丢 run 的 Item/记录 + 清其悬挂 open interrupt（`runs.listOpenInterrupts` 不再返回）+
  **递归 purge 被丢 run 派生的 subagent 子会话整棵子树**。
- **运行中拒绝**:session 有 run 在跑 → `session_busy`。
- **不动文件**(v1 无快照):UI 用 `workspace.getDiff` 自查未还原改动。
- `droppedRuns[].userInput` = 被丢 run 开场 userMessage 的 `content`(与 `StartRunRequest.input` 同型,composer 零转换预填);延续 run 无开场轮 → 省略。

### 3.2 `sessions.fork{ sessionId, fromRunId?, title? }` → 新 `Session`

- `fromItemId` **已改名 `fromRunId`**。省略 = 整段 fork(全量复制);给定 = **含 `fromRunId` 在内**截断复制到该 run 边界。
- **快照语义、随时可调**(不要求 session 空闲):只复制**已完结**的 run,in-flight run 不进副本(等价「先 interrupt 再 fork」)。
- 错误 `session_not_found` / `run_not_found`。

### 3.3 诚实边界（务必知道）

- **compaction**:若目标 run 之后发生过把日志压缩到低于其水位的 compaction,回退会 clamp 到当前条数 —— 跨 compaction 边界的
  rollback 是 **best-effort**(保留已压缩态,不损坏、不崩)。常见场景(目标到当前之间无 compaction)精确。
- **subagent 子会话归属按 spawn 时间**(被丢 run 窗口内 spawn 的子会话即被 purge);在「同 session 串行跑 + rollback 要求空闲」下精确。

---

## 4. B5 — 审批 remember + editedArgs（AUX_API §6）

`runs.resume` 的 `responses[].response`(approval 分支)增量:

```ts
interface ApprovalResponse {
  type: "approval"; decision: "approve" | "deny";
  remember?: { scope: "session" };      // 记住这个决策,同会话该工具后续免提示
  editedArgs?: Record<string, unknown>; // 批准前一次性改写工具入参
  reason?: string;
}
```

- **`remember` KEY = 工具名**;`decision:"deny" + remember` 合法(记住拒绝)。`editedArgs` 是**一次性**的(不进 remember)。
- v1 `scope` 仅 `session`(内存,重启重置);`project|global` 未持久化、未进枚举 —— **发了也只当本次**(后端不假装记住)。
- 前端审批卡:加个「本次 / 本会话」下拉 = `remember` 缺省 / `{scope:"session"}`;参数编辑入口接 `editedArgs`。

---

## 5. 移除项确认（去债，AUX_API §7）

| 移除 | 替代 |
| --- | --- |
| `items.edit` | `sessions.rollback{toRunId}` + `runs.start` |
| `sessions.fork.fromItemId` | → `fromRunId`（见 §3.2） |
| `background.*`（list/subscribe/cancel + `notifications.background.update` + `BackgroundTask`） | 后台子任务 = 子 agent 的 turn,挂 run 树流式 |

前端可删 `BackgroundTask` / `streamBackgroundUpdates` 相关代码,并把 `McpServer.status` 从 3 态升到 5 态、`fork` 参数改名。
