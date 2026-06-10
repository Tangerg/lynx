# 旁路 API · 草案设计 v0.1(收口稿)· `2026-06-10`

> 修订 [`AUX_API_DRAFT_SPEC.md`](./AUX_API_DRAFT_SPEC.md)(v0),逐条吸收前端
> [`AUX_API_DRAFT_SPEC_REVIEW.md`](./AUX_API_DRAFT_SPEC_REVIEW.md) 的 ⛔/⚠/✎。**A、B 两处结构改动后端同意,无异议**
> ——不走文档往返,直接定稿。本稿为可实现规范,落 `../API.md` 正文(补丁文档只做导读)。
> 标 **[v1]** 本期做,**[v2]** 留接口暂不实现。变更点用 `← v0.1` 标注。

---

## 0. 相对 v0 的结构性变更(先看这个)

1. **[A ⛔]** 取消"连接级"概念 —— 我们的 streamable HTTP 无连接身份(TRANSPORT §2/§8)。**`workspace.watch`/`unwatch`
   两个方法删除**,watch 集随 `workspace.subscribe` 参数走,**流即作用域**(§2.1)。
2. **[B]** 旁路事件**不再每事件一个 method**,改为**单方法 `notifications.workspace.event` + 类型联合**,对齐 run 域
   的 `notifications.run.event`(§2.2)。
3. **[C5]** **`items.edit` 砍掉** —— turn 粒度下"编辑重跑" = `rollback` + `runs.start`,item 精确编辑无存在理由(§5.3)。
4. **[C3]** `rollback` 响应**删 `residualDiff`** —— v1 无快照无法归因,留着名不副实(§5.1)。
5. **[E2]** 审批 `remember.scope` **v1 只放 `session`**(内存),`project|global` 不持久化就接受=债,additive 后补(§3)。
6. **[F]** MCP 推送事件 `mcp.statusChanged` → **`mcp.serverChanged`**(条目任何字段变即发,名实相符)(§2.2/§4)。

---

## 1. git / VCS  **[v1]**  → API.md §7.5 + §8.2

### 1.1 能力位与退化(三态分离,进 API.md 正文表)

| 情况 | 表现 |
|---|---|
| 无 `git` 二进制 | `features.git: false` → 前端**隐藏面板、不调** |
| 有 git、cwd 非仓 | 调 → **`vcs_unavailable`**(新 sentinel) |
| 有 git、是仓、无改动 | 空结果(`files: []`) |

### 1.2 `workspace.listFileChanges`

```jsonc
// req: WorkspaceQuery & { cursor?, limit? }    resp: Page<WorkspaceFileChange>
WorkspaceFileChange = {
  path: string,
  status: "added" | "modified" | "deleted" | "renamed" | "untracked",
  previousPath?: string,   // ← [D1] 仅 renamed:源路径
  added?: number,          // ← [D2] binary 时缺省(不伪造 0)
  removed?: number,
  binary?: true            // ← [D2] 二进制文件
}
```

### 1.3 `workspace.getDiff`

```jsonc
// req: { cwd?, path?, mode?: "worktree"|"base", format?: "rows"|"raw", limit? }   默认 mode=worktree, format=rows
// resp(rows):
{ files: FileDiff[], truncated?: boolean }
FileDiff = { path, status, previousPath?, added?, removed?, binary?: true, rows: DiffRow[] }
// resp(raw):
{ patch: string, truncated?: boolean }
```

钉死(进规范):
- **`mode:base` 基线** = `git diff $(git merge-base HEAD <defaultBranch>)`;`defaultBranch` = `origin/HEAD` → `main` → `master`。
- **[D5] base 解析失败**(无 remote / origin/HEAD / main / master / detached HEAD)→ **`invalid_params`**(detail「无法解析基线分支」),
  **不**塌成空、**不**返 `vcs_unavailable`(那是"非仓"语义)。
- **[D3] untracked**:`mode:worktree` **包含** untracked(`FileDiff.status:"untracked"`,全文件 added rows)。
- **[D2] binary**:`binary:true`,`added/removed` 缺省,`rows: []`。
- **[D4] limit 截断在文件边界**:宁可整文件不出现 + `truncated:true`,不出半截 rows;被截文件可保留 `{path, status, rows:[]}` 让 UI 列出"还改了哪些"。
- 错误:`vcs_unavailable` / `path_outside_root` / `cwd_unavailable` / `invalid_params`。

---

## 2. 旁路通知通道  **[v1]**  → API.md §5(新事件联合)+ §9 + TRANSPORT §6.4

### 2.1 通道:watch 集随订阅走,流即作用域 **[A]**

```jsonc
workspace.subscribe { watches?: [ { watchId: string, path: string } ] }   // path 相对 cwd,jail 同 §7.5
  → 流式;流的生命周期 = 订阅 + 其全部 watch 的生命周期
// 改 watch 集 = 关流重订(文件监听集合低频变化:面板开关时)。重订丢失窗口 = 前端无脑全量失效一次(等价 resync)。
```

进 `ServerCapabilities.streamingMethods`。**`workspace.watch`/`unwatch` 删除。**

**[A] TRANSPORT 必须补两条**:
- **连接预算**:workspace 流是 run 流之外的**第二条常驻连接**;`1 run + 1 workspace + 短连 RPC` 在 HTTP/1.1 ~6 条预算内。
  **整个 app 共享一条 workspace 流**(多面板复用,**不许每面板开流**)。
- **重连语义**:无 `Last-Event-Id`、不补发;重订 = 隐式 `resync`(前端全量失效一次)。与 run 流的 durable 续传写成对照表。

### 2.2 事件:单方法 + 类型联合 **[B]**

```jsonc
notifications.workspace.event  { event: WorkspaceEvent }
WorkspaceEvent =
  | { type: "files.changed",   watchId: string, paths: string[] }   // paths 相对 cwd
  | { type: "skills.changed" }                                       // [G1] 不区分 cwd,任何 skill 目录变化都触发
  | { type: "mcp.serverChanged", server: string, status: McpStatus, toolCount?: number, error?: ProblemData }  // [F]
  | { type: "resync", domains?: string[] }                          // 兜底:丢过事件;有 domains 局部、无则全量
```

- optOut 按 **event type** 抑制(`optOutNotificationMethods` 的粒度本就是 type,如 `["mcp.serverChanged"]`)。
- 重拉策略:常态按 type 局部失效(react-query key);`resync` = 全量兜底。**v1 无 seq**(run 流自带续传,旁路是"变了→重拉")。
- `request_id` 关联**仅**在将来出现 server→client *请求*(需应答)时启用;v1 纯单向 notification 不需要。

---

## 3. 审批 scope  **[v1]**  → API.md §6.1

```jsonc
// ApprovalResponse 增量
{ "type": "approval",
  "decision": "approve" | "deny",
  "remember"?: { "scope": "session" },   // ← [E2] v1 只 session;project|global additive 后补
  "editedArgs"?: { ... } }               // = Claude Code updatedInput;wire 已有
```

钉死:
- **[E1] remember 的 KEY = 工具名**(`ToolInvocation.name`)。按参数模式匹配是规则引擎/config 域的事,v1 不做。
  **`editedArgs` 是一次性的**;`remember` 记的是"这个工具",不是"工具+这次改的参数"。
- **[E3] `deny` + `remember` 合法**(记住拒绝,同 Claude Code)。
- **[E2]** v1 `scope` 枚举**只放 `session`**(内存)。`project|global` 需持久化位置;**接受但不持久化 = 债**(同
  BACKEND_CAPABILITIES 对 feedback.create 的批评),故作 additive 值,落地持久化时再加并写明存储位置。
- 不收 `once`(= 不带 remember 的普通 approve)、不收 `ask`/`behavior`(响应即那次 ask 的回答)。
- 完整规则引擎(addRules/setMode/addDirectories)是 config 域,审批响应不开口子。

---

## 4. MCP 生命周期  **[v1]**  → API.md §7.5 + §4.10

### 4.1 富化 `McpServer`(退掉 listServers⨝listTools join)

```jsonc
McpServer = {
  name: string,
  status: "connecting" | "connected" | "disconnected" | "failed" | "needsAuth",
  toolCount?: number,
  authStatus?: "none" | "bearerToken" | "oauth" | "notLoggedIn",
  error?: ProblemData,        // ← [RESPONSE_R2 §4] failed 必带原因
  description?: string
}
```

`workspace.mcp.listTools`(已实现)保留给详情面板(分页 + inputSchema)。

### 4.2 `workspace.mcp.reconnect { server }`

- 结果走推送 `mcp.serverChanged`,**保证顺序 `connecting → (connected | failed | needsAuth)`**;前端按钮 loading 绑
  `connecting`,终态解除。**reconnect 与 serverChanged 必须同批落地。**
- **[F]** `mcp.serverChanged` 在**条目任何字段变化**时发(含仅 `toolCount` 变,对应 MCP `tools/listChanged`)——
  失效语义统一"该 server 变了→重拉 mcp-servers + mcp-tools"。

---

## 5. Checkpoints / 回退  → API.md §7.2(fork 改写)+ 新增 rollback + §7.4(删 items.edit)+ §9

### 5.1 **[v1]** `sessions.rollback` —— 原地截断,按 `runId`,不动文件

```jsonc
sessions.rollback { sessionId, toRunId }
  → { session: Session,
      droppedRuns: [ { run: RunRef, userInput?: ContentBlock[] } ] }   // ← [C1] RunRef 装不下 input,改此形
```

- **[C2] `toRunId` = inclusive-keep**(保留的**最后一个** root run,其后全丢)。"编辑第 K 轮重跑" = 传第 **K-1** 轮 runId
  (前端从 `items.list.runs` 取)。
- `toRunId` **必须是 root run**(子 agent / continuation run → `invalid_params`);丢一个 run 时其 continuation 链 +
  subagent 子树一并丢;悬在被丢轮次上的 open interrupt 一并清理。
- `userInput` = 该 run 开场 userMessage 的 content(`ContentBlock[]`,与 `StartRunRequest.input` 同型,composer 预填零转换;
  resume/edit 类续 run 无开场用户轮 → 缺省)。
- **[C4] 运行中拒绝**:session 有 run 在跑 → **`session_busy`**(新 sentinel)。
- **[C3] 不返 `residualDiff`**:v1 不动文件;UI 用 `workspace.getDiff` 自查当前未还原改动。

### 5.2 **[v1]** `sessions.fork { sessionId, fromRunId?, title? }` → Session

- `fromRunId` **取代** v0 的 `fromItemId`(run 边界可解,v1 即可做)。省略 = 整段 fork(已实现);给了 = **含 `fromRunId` 在内**截断复制。
- **随时可调**(快照语义,codex「如同先 interrupt」);与 `rollback` 的"运行中拒绝"差异写明。
- 已上线 fork 的 `fromItemId`→`checkpoint_unavailable` 路径**改为 `fromRunId`**。

### 5.3 **[C5] 砍 `items.edit`**

API.md §7.4 删该方法;`features.checkpoints` 能力位语义从「items.edit / fork-at-item」**改写为「restoreType(v2 文件快照)」**。

### 5.4 **[v2]** 影子 git + `restoreType`

```jsonc
// 启用后 rollback/fork 增量:
{ ..., restoreType?: "history" | "files" | "both" }   // 默认 history
```

定论:快照锚 run 边界;**`both` 原子**(files 先行,失败→整体失败、history 不动、明确错误,**绝不静默降级 history**);
files/both 还原前自动 `track()` 当前态(unrevert 免费可逆);`features.checkpoints` true 才接受 `restoreType`。

---

## 6. 去债  **[v1]**

- **砍 `background.*`**(八家无 client 可见任务注册表;前端零消费,同轮删 wire)。
- fork `fromItemId` → `fromRunId`(§5.2)。
- 删 `items.edit`(§5.3)。

---

## 7. 能力位 / streamingMethods / sentinel 汇总

```jsonc
features: { "git": <probe>, "fileWatch": <bool>, "checkpoints": false /* v2 */, /* 既有 reasoning/mcp/memory/skills/relocate */ }
streamingMethods: [ "runs.start", "runs.resume", "runs.subscribe", "workspace.subscribe" ]   // ← 加后者
```
新增 sentinel:**`vcs_unavailable`**(§1.1)、**`session_busy`**(§5.1)。

---

## 8. 实现批次 × API.md 落点(规范落地=改正文)

| 批次 | 内容 | API.md 落点 | 放行判定 |
|---|---|---|---|
| **B1** git listFileChanges/getDiff + 退化 | §7.5 + §8.2 | **修 §1.2/1.3 措辞即可开写**(纯措辞,不动结构) |
| **B2** 通道 `workspace.subscribe` + 事件联合 + resync | §5(新联合)+ §9 + TRANSPORT §6.4 补节 + 续传对照表 | 结构已在本稿定稿,**可开写** |
| **B3** files.changed(随 subscribe 参数)+ MCP reconnect/serverChanged + McpServer 富化 | §7.5 + §4.10 | 随 B2 |
| **B4** `sessions.rollback` + `fork{fromRunId}` + 删 `items.edit` | §7.2 改写 + §7.4 删 + §9 + §8.2(session_busy) | **本稿定稿,可开写** |
| **B5** 审批 `remember{scope:session}` + `editedArgs` | §6.1 | **可开写**(scope v1=session 已定) |
| **去债** background | §7.7 删 + §7.8 删 notification | 随时;前端就绪 |
| **v2** 影子 git + restoreType | §7.2 增量 + §9 | 单独排期 |

---

## 9. 前端同轮动作清单(供排期)

- `shapes.ts` 补 `ServerCapabilities.streamingMethods` 镜像(**存量 drift,API.md §9 已有、前端漏镜像**);
- `Diff` 重写为 `{ files: FileDiff[] }`(DiffPreview/DiffView 改造);
- `McpServer` 状态 3→5 态 + `MCP_STATUS` 映射 + 退掉 listServers⨝listTools join;
- `WorkspaceFileChange` 加 `previousPath/added/removed/binary`;
- `ApprovalResponse` 加 `remember`(审批卡加下拉,v1 只 本次/本会话两项);
- 删 `background.*` 全家(`BackgroundTask`/`TaskId`/`streamBackgroundUpdates`/methods);
- fork 参数 `fromItemId`→`fromRunId`;删 `items.edit` wire;
- 新增 `workspace.subscribe` 流消费 + `WorkspaceEvent` 派发(复用 run 事件 fold 基建)。

---

**状态**:四个开放项闭环 + 三轮评审 ⛔/⚠/✎ 全部吸收,A/B 结构改动后端已采纳。前端按 REVIEW 结论应"v0.1 后无保留放行"
——如确认,后端即按 §8 从 **B1(git,最自包含)** 开写,每批落地改 API.md 正文 + 更新 `BACKEND_CAPABILITIES.md`。
