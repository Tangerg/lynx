# 旁路 API 调研 · 前端回应与补充 · `2026-06-10`

> 回应 [`AUX_API_DESIGN_STUDY.md`](./AUX_API_DESIGN_STUDY.md)。前端也读了两家源码(codex `app-server-protocol` 全量方法表 +
> opencode `server/routes` 全量路由 + `snapshot/` 实现),**总体结论一致**:git 抄 opencode、回退走 turn 粒度、background 砍。
> 本文做三件事:**① 修正一处事实**(opencode 的 checkpoint 机制,比研判的更强、且对我们更有用);**② 补一个结构性前置**
> (旁路推送通道——P1 三项共同依赖,原文没覆盖);**③ 应约给接口形状偏好**(趁还没写死)。

---

## 1. 事实修正:opencode 其实做了「按消息点 fork + part 级 revert」

原文 §2 说 opencode「也是回合/文件快照粒度,不做消息截断」——**不准确**:

- `POST /session/:id/fork` 收 `{ messageID? }`,**按消息点**复制历史分叉(`server/routes/.../groups/session.ts`);
- `POST /session/:id/revert` 收 `{ messageID, partID? }`,**part 级**回退。

它能做到,靠的是一套值得完整记录的机制(`packages/opencode/src/snapshot/index.ts`):

1. **影子 git 仓**:`~/.local/share/opencode/snapshot/<projectID>/<hash(worktree)>` 作 `--git-dir`、真实工作区作
   `--work-tree`——用户仓库零侵入;尊重真仓 `.gitignore`(查 `check-ignore`);>2MB 文件排除;每小时 `git gc --prune=7.days`。
2. **`track()` = stage 全部 + `git write-tree` → 返回 tree hash**。无 commit 对象,内容寻址自动去重,代价极低。
3. **快照锚内嵌在消息流里**:LLM 每个 step 开始/结束各 track 一次,hash 挂在 `step-start` / `step-finish` part 上;
   该 step 改了文件就追加一个 **`patch` part `{ hash, files[] }`**。**对话历史本身就是 checkpoint 索引**——
   没有独立的 message↔snapshot 关联表。
4. **revert 是文件定向的**:收集 revert 点之后所有 `patch` part,逐文件 `git checkout <hash> -- <file>`(该树中不存在则删);
   revert 前先 track 当前态存进 `session.revert.snapshot`,**unrevert = 一次整树 restore**。被 revert 的消息留存置灰,
   下次 prompt 才真正清理。

**这不推翻原文结论**——turn = run 边界在我们模型里可靠可知,turn 粒度做 v1 完全正确。修正的价值在于:
(a) item 级精确**并非不可解**,解法不需要 message↔item 关联表,而是把快照 hash 锚在流上——这与我们的 Item 模型天然同构
(挂在 run 边界的 durable Item / RunRef 字段上即可);(b) 它直接引出下一节的问题。

## 2. v1 必须回答「文件态」——哪怕答案是「不还原」

两家在回退的文件侧给了两个截然不同的答案:

- **codex `thread/rollback`**:只裁剪历史,**文档明示「客户端负责还原文件改动」**——诚实,但 UX 是残缺的;
- **opencode**:快照体系把文件一并回退。

如果我们 v1 只截断聊天历史,用户会看到「已回退到第 K 轮」、**而文件还是改过的**。这个落差必须在接口层有明确表达,建议:

- **最低限度(v1)**:`sessions.rollback { sessionId, turns }` → `{ session, droppedRuns: RunRef[], residualDiff?: Diff }`
  ——明确不动文件,但把「被丢弃轮次留下的脏 diff」一并返回(或留给前端用 `workspace.getDiff` 查),UI 才能提示
  「这些文件改动未随回退还原」。文档照 codex 写清边界。
- **v2(如果做)**:影子 git `write-tree` 方案已被 opencode 验证(低成本、零侵入、免 gc 心智),快照 hash 挂 run 边界,
  顺带免费解锁 **per-run diff**(codex 的 `turn/diff/updated` 同款数据)。建议作为 checkpoints 的演进方向记录,不阻塞 v1。

**前端 UI 映射偏好**:「编辑某条消息重跑」= 回退到该轮之前 + composer 预填原文。所以 rollback 响应里的 `droppedRuns`
**最好携带各轮的 userMessage 文本**(或前端 rollback 前自行从 `items.list` 截留)——响应直接带最省一次往返。
fork-from-turn 同理:`sessions.fork { sessionId, fromRunId?, title? }` 比「第 K 个 run」的序数语义稳(id 不受并发新轮影响)。

## 3. 结构性补充:旁路推送通道是 P1 三项的共同前置 ⭐

原文 P1/P2 的 `fs.watch + changed`、`skills.changed`、`mcp` 状态变化全是**推送**,但 LRP 目前唯一的 server→client 通道是
**run 流**(`runs.start/subscribe` 的 RunEvent)——这些事件不属于任何 run,今天没有回程路。两家的答案:

- codex:同一连接上的 **runtime 级 notification**(`fs/changed`、`skills/changed`、`mcpServer/startupStatus/updated`…),
  配 `initialize.capabilities.optOutNotificationMethods` 按名抑制——**我们的 `ClientCapabilities` 已有同名字段,正好复用**;
- opencode:一条 SSE 总线,~100 种 dot-namespaced 事件,事件 schema 注册表 → 自动生成客户端类型联合。

**建议先出一页规范再做任何 watch 类方法**:runtime 级 JSON-RPC notification 的 (a) envelope(沿用现有 notification 形状,
method 如 `notifications.workspace.event`?还是每事件一个 method?偏好后者,与 codex 一致、可被 optOut 精确抑制);
(b) 订阅边界(连接级,断连即止,无需补发——与 run 流的 durable 语义明确区分);(c) HTTP transport 上怎么跑
(复用 streamable HTTP 长连接即可,TRANSPORT.md 补一节)。首批三个事件就够:
`workspace.files.changed { watchId, paths[] }`、`workspace.skills.changed {}`、`workspace.mcp.statusChanged { server, status }`。

## 4. 接口形状偏好(应约)

### 4.1 git(P0)

- `WorkspaceFileChange` 加 `added` / `removed` ✓ 赞成。
- **`getDiff` 偏好 per-file 结构而非整段 patch string**:前端已有结构化 `DiffRow` 行模型与渲染(DiffView / 工具卡 diff 预览),
  opencode 的 `patch: string` 反而要前端再解析。偏好:
  ```jsonc
  // workspace.getDiff { cwd?, mode?: "worktree" | "base", path?, limit? }
  { "files": [ { "path", "status", "added", "removed", "rows": DiffRow[] } ], "truncated"? }
  ```
  即把现有 `Diff { rows }` 套一层 per-file;raw unified patch 作为导出变体(`format: "raw"`)可后置。
- `mode` 默认 `worktree`;`base` 的基线(默认分支 vs upstream)语义请在文档钉死。

### 4.2 回退 / fork(P0)

见 §2:`fromRunId` 而非序数;rollback 响应带 `droppedRuns`(含 userMessage 文本)+ 文件态的明确表达。

### 4.3 MCP(P1)

- `reconnect`(connect/disconnect)✓ 赞成。两点补充:
  - **reconnect 的结果走 `workspace.mcp.statusChanged` 推送**(§3),否则前端只能轮询 listServers 等结果;
    codex 的状态机枚举可借:`starting | ready | failed | cancelled`。
  - **增富 `McpServer` 条目时直接带 `toolCount` + `authStatus`**(`none | bearerToken | oauth | notLoggedIn`)。
    前端本轮(commit `9621df6`)刚用 listServers ⨝ listTools 双调用凑 tool 数,条目内联后即退掉 join,少一次往返。
    `listTools` 保留给详情面板(分页 + inputSchema)。

### 4.4 文件(P1/P2)

- `fs.watch` 形状借 codex:`{ watchId(客户端起名,连接级作用域), path }` + `changed { watchId, paths[] }`——
  客户端起 id 的好处是注册时就能路由通知,不用等服务端发号。
- `findFiles`:一次性 `{ query, limit? } → { paths: string[] }` 就够 v1(@-mention 场景);
  codex 的会话式推送(sessionStart/Update/Stop)是豪华版,不必。

### 4.5 HITL(便宜、可顺手)

`ApprovalResponse.decision` 现为 `approve | deny`,**加 `approveForSession`**(本会话内同类不再询问,借 codex
`AcceptForSession`)。前端 approval 卡片加一个选项即可,成本极低、体验提升明显。

### 4.6 sessions(P2)

archive/unarchive、`archived` 过滤、`children` ✓ 赞成。备忘两个 codex 技巧,长会话时取用:
`items.list` 加详情级别(`itemsView: full | summary | notLoaded`);resume 类响应可捆首页历史(`initialTurnsPage`)省一次往返。

## 5. background 砍:前端确认零成本,附议

检查过前端消费面:协议侧 `background.*` 只有 wire 定义(`rpc/methods.ts` / `shapes.ts` / `stream.ts` 的
`streamBackgroundUpdates`),**没有任何 UI 消费**——状态栏的 tasksStore 是纯客户端插件任务跟踪,不走协议。
砍掉后前端同轮删除 wire 面(`BackgroundTask` / `TaskId` / subscribe 流),净负 LOC,正合第一法则。✓

## 6. 优先级表(前端视角微调)

| 优先级 | 事项 | 相对原文的变化 |
| --- | --- | --- |
| **P0** | git `getDiff` + `listFileChanges` | 不变;形状见 §4.1 |
| **P0** | turn 粒度回退 / fork-from-run | 不变;**v1 须含文件态表达**(§2),`fromRunId` 语义 |
| **P1** | **旁路通知通道规范**(一页 TRANSPORT/API 补充) | **新增**——fs.watch / skills.changed / mcp 状态推送的共同前置 |
| **P1** | `fs.watch` + `files.changed`;`mcp.reconnect` + `statusChanged` | 通道就绪后落地 |
| **P1.5** | `workspace.findFiles` | 原文 §3 提了但未进表;composer @-mention 是近期 UI 计划,请进表 |
| **P2** | skills `enabled` + `changed`;sessions archive/children/详情级别 | 不变 |
| **去债** | 砍 `background.*` | 不变;前端零成本,确认附议 |

---

**前端可先行的事**(不等接口):approval 卡片预留 `approveForSession` 选项位;rollback/fork UI 原型按 §2 语义画;
diff 面板维持 DiffRow 渲染等 per-file 形状落地。接口方案出来后我们按 `BACKEND_CAPABILITIES.md` 同款节奏对接。
