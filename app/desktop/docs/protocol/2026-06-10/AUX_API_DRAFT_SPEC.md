# 旁路 API · 草案设计 v0 · `2026-06-10`

> 收口 [`STUDY`](./AUX_API_DESIGN_STUDY.md) → [`RESPONSE`](./AUX_API_DESIGN_RESPONSE.md) →
> [`STUDY_R2`](./AUX_API_DESIGN_STUDY_R2.md) → [`RESPONSE_R2`](./AUX_API_DESIGN_RESPONSE_R2.md) 四轮讨论(八家 agent 调研 +
> 前端两轮回应),给出**具体 wire shape 草案**。四个开放项已闭环,本文是落 `../API.md` 前的最后一稿评审件。
> §x.y 指 `../API.md`。标 **[v1]** 的本期做,**[v2]** 的留接口、暂不实现。
>
> 命名沿用现有约定:id 带类型前缀;list 统一 `Page<T>`;错误 `ProblemData{type,channel,detail}`;能力位走
> `ServerCapabilities.features`;notification 沿用 `notifications.*` 信封。

---

## 1. git / VCS  **[v1]** —— 新增 `workspace.*`

### 1.1 能力位与退化(比八家都诚实的点)

- 启动探测 `git` 二进制是否在 PATH → `features.git: boolean`。**无 git → 前端隐藏 diff/changes 面板,根本不调。**
- `git` 在、但请求的 cwd 不是仓 → 返回 **`vcs_unavailable`**(新 sentinel),**与"干净仓(无改动)"明确区分**——
  八家里 codex/opencode 都把"没 git / 非仓 / 干净"塌成空,我们不塌。
- 这套退化写进 `../API.md` 正文(§7.5 + §8.2 码表),不只补丁文档。

### 1.2 `workspace.listFileChanges`

```jsonc
// req: WorkspaceQuery & { cursor?, limit? }
// resp: Page<WorkspaceFileChange>
WorkspaceFileChange = {
  path: string,
  status: "added" | "modified" | "deleted" | "renamed" | "untracked",
  added: number,      // ← 新增:+ 行数
  removed: number     // ← 新增:- 行数
}
```

### 1.3 `workspace.getDiff`

```jsonc
// req:
{ cwd?: string,
  path?: string,                        // 省略 = 全量
  mode?: "worktree" | "base",           // 默认 worktree
  format?: "rows" | "raw",              // 默认 rows
  limit?: number }                      // 行上限,超则 truncated

// resp(format=rows,默认):per-file + 复用现有 DiffRow
{ files: FileDiff[], truncated?: boolean }
FileDiff = { path, status, added, removed, rows: DiffRow[] }

// resp(format=raw):
{ patch: string, truncated?: boolean }  // 原始 unified patch
```

- **`mode: base` 基线钉死**:`git diff $(git merge-base HEAD <defaultBranch>)`,`defaultBranch` 取 `origin/HEAD`,
  回退 `main` → `master`。文档写明。
- 默认 `rows`(前端已有 `DiffRow` 渲染);`raw` 给"复制/导出 patch"。
- 错误:`vcs_unavailable` / `path_outside_root` / `cwd_unavailable`。

---

## 2. 旁路通知通道  **[v1]** —— gate 了 fs.watch / skills / mcp 推送

### 2.1 通道(连接级,不补发)

新增**流式方法** `workspace.subscribe {}` → 打开一条 workspace 事件流(进 `ServerCapabilities.streamingMethods`)。
语义与 `runs.subscribe` 对称,但:**连接级、断连即止、不补发、无 durable seq**(与 run 流的 durable 明确分层 —— 这是
RESPONSE_R2 §3 的定论:旁路不引入 seq)。InProcess/IPC 上即 notification 回调。

所有事件方法均可经 `ClientCapabilities.optOutNotificationMethods` 按名抑制(字段已存在)。

### 2.2 事件(每事件一个 method,精确 optOut)

```jsonc
// 文件改动(需先注册 watch,见 2.3)
notifications.workspace.files.changed   { watchId: string, paths: string[] }
// skills 变更(workspace 全局,无需注册)
notifications.workspace.skills.changed  { }
// MCP 状态变更(无需注册)
notifications.workspace.mcp.statusChanged { server: string, status: McpStatus, toolCount?: number, error?: ProblemData }
// 兜底:我丢过事件、不知丢了啥(有损档溢出 / 重连)
notifications.workspace.resync          { domains?: string[] }   // 有 domains=局部重拉,无=全量
```

- **重拉策略(RESPONSE_R2 §3)**:常态按事件类型局部失效(前端 react-query 按 query-key);`resync` 兜底 = 全量(或按
  `domains` 局部)。**v1 不需要 seq。**
- **`request_id` 关联**(STUDY_R2 §2.2)只在将来真出现 server→client *请求*(要应答)时才启用;v1 纯单向 notification 不需要。

### 2.3 文件监听注册(借 codex,client 起名)

```jsonc
workspace.watch   { watchId: string, path: string }   // watchId 客户端起,连接级作用域
workspace.unwatch { watchId: string }
// → 变更经 notifications.workspace.files.changed { watchId, paths }
```

能力位:`features.fileWatch: boolean`。

---

## 3. 审批 scope  **[v1]** —— 扩 `ApprovalResponse`(§6.1)

```jsonc
// ApprovalResponse 增量(KISS,RESPONSE_R2 §2)
{ "type": "approval",
  "decision": "approve" | "deny",
  "remember"?: { "scope": "session" | "project" | "global" },  // 缺省 = 仅本次(无需 once 枚举)
  "editedArgs"?: { ... } }                                     // = Claude Code 的 updatedInput;wire 已有此字段
```

- **scope 命名 `session | project | global`**(不用 `user`):runtime 协议零 user/租户概念,`global` 落 home 层,与
  `MemoryScope(cwd|projectRoot|home)` 词汇对齐。将来要"项目内个人、不入库"(Claude Code 的 `localSettings`)时
  **additive 加 `projectLocal`**,不破坏既有值。
- **不收 `once`**(就是不带 `remember` 的普通 approve);**不收 `ask`/`behavior`**(响应即那次 ask 的回答,重复)。
- 完整规则引擎(addRules/setMode/addDirectories)是 **config 域**将来的事,审批响应里不开口子。

---

## 4. MCP 生命周期  **[v1]**

### 4.1 富化 `McpServer` 条目(退掉前端 listServers⨝listTools 的 join)

```jsonc
McpServer = {
  name: string,
  status: "connecting" | "connected" | "disconnected" | "failed" | "needsAuth",
  toolCount?: number,
  authStatus?: "none" | "bearerToken" | "oauth" | "notLoggedIn",
  error?: ProblemData,        // ← failed 必带,tooltip/详情要展示原因(RESPONSE_R2 §4)
  description?: string
}
```

`workspace.mcp.listTools`(已实现)保留给详情面板(分页 + inputSchema)。

### 4.2 `workspace.mcp.reconnect { server }`

- 结果**走推送**:`notifications.workspace.mcp.statusChanged`,且**保证顺序 `connecting → (connected | failed | needsAuth)`**
  ——前端按钮 loading 态直接绑 `connecting`,终态解除,不自造过渡态。**所以 reconnect 必须和 statusChanged 同批落地。**

---

## 5. Checkpoints / 回退

### 5.1 **[v1]** turn 粒度,按 `runId` 寻址,不动文件

```jsonc
// 原地截断(同一 session)
sessions.rollback { sessionId, toRunId }
  → { session: Session,
      droppedRuns: RunRef[],            // 每条带 userMessage 文本(前端"编辑重跑"预填,省一次往返)
      residualDiff?: Diff }             // 被丢轮次留下的脏文件 diff;v1 不动文件,只如实告知

// 分叉(新 session);fromRunId 替代已废的 fromItemId
sessions.fork { sessionId, fromRunId?, title? }
  → Session                            // 省略 fromRunId = 整段 fork(已实现);给了 = 截到该 run 边界后分叉
```

- **`fromRunId` 取代 `fromItemId`**:run 边界在我们模型里可靠可知(Item 带 RunID),所以"按点分叉"**v1 即可做**——
  无需等 message↔item 关联。(已上线的 fork 把 `fromItemId` 返 `checkpoint_unavailable`,本期改为 `fromRunId`。)
- `residualDiff` 直接复用 §1.3 的 getDiff(worktree)。

### 5.2 **[v2]** 影子 git 快照 + `restoreType`(留接口,后做)

```jsonc
// 启用后 rollback/restore 增量:
{ ..., restoreType?: "history" | "files" | "both" }   // 默认 history
```

定论(RESPONSE_R2 §1):
- **快照锚在 run 边界**:snapshot hash 盖在 durable `RunRef`/`Item` 字段上(穿过 message↔item blocker 的路);
- **`both` 失败语义 = 原子**:**files 先行,files 失败 → 整体失败、history 不动、返明确错误,绝不静默降级成 history**;
- **files/both 还原前自动 `track()` 当前态**(一次 `write-tree`),unrevert 免费可逆;
- 能力位 `features.checkpoints` 启用后才接受 `restoreType`。

---

## 6. 砍 / 去债  **[v1]**

- **`background.*` 砍**(八家无一有 client 可见任务注册表;前端确认零消费,同轮删 wire)。
- 已上线 `sessions.fork` 的 `fromItemId` 路径 → 改 `fromRunId`(§5.1)。

---

## 7. 能力位汇总(本草案新增/变化)

```jsonc
features: {
  "git": <probe>,          // git 二进制在 PATH
  "fileWatch": <bool>,     // workspace.watch 已接
  "checkpoints": false,    // v2 才 true(restoreType 解锁)
  // 既有:reasoning/mcp/memory/skills/relocate ...
}
streamingMethods: [ "runs.start", "runs.resume", "runs.subscribe", "workspace.subscribe" ]  // ← 加后者
```

新增错误 sentinel:`vcs_unavailable`(§1.1)。

---

## 8. 实现顺序(后端)

| 批次 | 内容 | 依赖 |
|---|---|---|
| **B1** | git `workspace.listFileChanges` + `getDiff` + `features.git`/`vcs_unavailable` 退化 | 无(exec git);最自包含 |
| **B2** | 旁路通道 `workspace.subscribe` + `optOut`/连接级/`resync` + TRANSPORT 补节 | gate B3 |
| **B3** | `workspace.watch`/`unwatch` + `files.changed`;`mcp.reconnect` + `statusChanged`(同批);`McpServer` 富化 | B2 |
| **B4** | `sessions.rollback` + `fork{fromRunId}`(改 fromItemId) | 无 |
| **B5** | 审批 `remember{scope}` + `editedArgs` | 无 |
| **去债** | 砍 `background.*` | 与前端同步删 wire |
| **v2** | 影子 git + `restoreType` | 单独排期 |

---

**给前端**:本草案把四轮讨论落成具体形状。请重点确认:**§1.3 getDiff 的 `FileDiff`/`format` 字段**、**§3 `ApprovalResponse`
增量**、**§5.1 `rollback`/`fork{fromRunId}` 响应**三处的字段名。确认后后端按 §8 批次开写,每批落地更新
`BACKEND_CAPABILITIES.md`(怎么调 + 边界),节奏同前。
