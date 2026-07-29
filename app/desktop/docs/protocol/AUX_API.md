# Lyra Runtime Protocol · 旁路 API（定稿 `2026-07-27`）

> **状态：正式契约（canonical）。** 本文是 [`API.md`](./API.md) 的配套契约，定义 Lyra Runtime 的**旁路面**——不经 LLM 的
> 辅助能力：git/VCS、失效事件流、会话回退 / 派生 / 归档、MCP 生命周期、审批 scope。与 `API.md` /
> [`TRANSPORT.md`](./TRANSPORT.md) 互为配套、共用同一套约定，合起来构成完整 wire 契约。
>
> 约定一律承自 `API.md`，不另立：判别字段一律 `type`；字段 / 枚举 camelCase；业务 id 带类型前缀；list 一律返回
> `Page<T>`；错误一律 `ProblemData{ type, detail?, … }`、客户端按 `type`（符号名）判错（**没有 `channel` 字段** ——
> 落点即判别，`API.md §8.1`）；能力位走 `ServerCapabilities.features` 开放 map；下行事件沿用 `notifications.*` 信封。
>
> **字段级真相在生成物**（`runtime/contract/{schema,openrpc,manifest}.json` 与 `API_REFERENCE.md`，`API.md §14`）。
> 本文写语义与不变量，不重述字段表。
>
> 文内裸 `§x` 指**本文**小节；引 `API.md` 一律写全 `API.md §x.y`。`protocolVersion`：**`2026-07-27`**（与 `API.md` 同）。

---

## 目录

- §1 能力位与错误（旁路面贡献）
- §2 git / VCS（§2.1 三态 · §2.2 listFileChanges · §2.3 getDiff）
- §3 失效事件流（§3.1 连接与投递模型）
- §4 会话回退 / 派生 / 归档（§4.1 rollback · §4.2 fork · §4.3 export·import）
- §5 MCP 生命周期（§5.1 状态推送与 reconnect）
- §6 审批 scope
- §7 明确不做
- 附录 · 类型索引

---

## 1. 能力位与错误（旁路面贡献）

旁路面方法由 `ServerCapabilities.features`（`API.md §9` 开放 map）的下列位门控；缺省 / falsy 即关闭：

| feature       | 门控                                                                      | 关闭时                                                                  |
| ------------- | ------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| `git`         | §2 `workspace.listFileChanges` / `getDiff`（git 二进制在 PATH，启动探测）  | 客户端隐藏 VCS 面板、**不发起**这两个调用                               |
| `fileWatch`   | §3 `runtime.subscribe` 的 `watches`（git-state 监视）                     | 带 `watches` 订阅 → `capability_not_negotiated`；只订 topic 仍可用      |
| `checkpoints` | §4.1 `sessions.rollback` 的 `restoreType:"files"\|"both"`（影子 git 快照） | 仅 `restoreType:"history"` 可用                                         |
| `sessionExport` | §4.3 `sessions.export` / `sessions.import`                              | 两个方法都 `capability_not_negotiated`                                  |
| `mcp`         | §5 `mcp.*`                                                                | 客户端隐藏 MCP 面板                                                     |

`runtime.subscribe` 计入 `ServerCapabilities.streamingMethods`（`API.md §9`）。

旁路面贡献的错误 `type`（完整 type↔code 表在生成的错误注册表里，`API.md §8.2`；客户端按 `type` 判错）：

| `type`                   | 含义                                                                                                   |
| ------------------------ | ------------------------------------------------------------------------------------------------------ |
| `vcs_unavailable`        | 有 git 二进制、但目标 cwd 不是 git 仓——与"干净仓 = 空结果"区分，与"无 git = `features.git=false`"区分  |
| `session_busy`           | session 有 run 在飞，拒绝会破坏正在 append 的历史（§4.1）                                              |
| `checkpoint_unavailable` | `restoreType:"files"\|"both"` 所需快照不可用 / 影子 git 未启用                                          |

---

## 2. git / VCS

### 2.1 三态回应

`workspace.listFileChanges` / `workspace.getDiff` 读 cwd 的 git 工作区状态，受 `features.git` 门控，并按**三态**
回应——客户端据此区分，不把三种情形糊成一个错误：

| cwd 情形             | 回应                                             |
| -------------------- | ------------------------------------------------ |
| 无 git 二进制        | `features.git=false`——客户端隐藏面板、不发起调用 |
| 有 git、cwd 非仓     | `vcs_unavailable`                                |
| 有 git、是仓、无改动 | 成功，空结果（`data: []` / `files: []`）         |

### 2.2 `workspace.listFileChanges`

工作区逐文件改动（VCS 扫描态），返回 `Page<WorkspaceFileChange>`；缺省 cwd = serve 目录。

- `status` 用**过去式**词汇（`added` / `modified` / `deleted` / `renamed` / `untracked`），描述"变更后的态"。
- `previousPath` **仅 `renamed`** 给。
- `added` / `removed`（±行）在 binary 时**省略**——不伪造 0：0 行变化和"没法数行"是两件事。

### 2.3 `workspace.getDiff`

工作区结构化 / 原始 diff，返回 `Diff`（**sum-type**：`format:"rows"` → `files`、`"raw"` → `patch`，不是同时带两者的
松对象）。

- **`mode`**：`worktree`（默认，工作区改动，**含 untracked** —— untracked 文件以 `status:"untracked"` + 全 `added`
  rows 出现）/ `base`（相对默认分支的 merge-base）。
- **`mode:"base"` 的基线** = `git merge-base HEAD <defaultBranch>`，`defaultBranch` 依次取 `origin/HEAD` → `main` →
  `master`。解析不出基线（无 remote / detached HEAD / 两个候选都不存在）→ **`invalid_params`**：**不**塌成空结果、
  **不**返 `vcs_unavailable`（后者专指"cwd 非仓"）。让一个无法回答的请求看起来像"没有改动"是最坏的一种回答。
- **`path`** 限定单文件 / 子路径，jail 到根，越界 `path_outside_root`。
- **`limit`** 是行上限，超出置 `truncated:true` 并**在文件边界截断**（不出半截文件，no silent caps）。

---

## 3. 失效事件流

非-run 的**失效信号**推送（文件 / skills / MCP / schedules / 会话 / run / interrupt / goal / state），与 run 事件流
（`API.md §5`）分层，自成一条常驻流。

`runtime.subscribe{ topics, watches? }` 打开它；流式 `notifications.runtime.event`（params `RuntimeEvent`，十个变体
见 `API.md §7.8`）。

- **订阅点名 topic**：`topics` 是闭合集合（`RuntimeTopic`），客户端只收它点过的名。上限在
  `capabilities.limits.runtimeSubscription{maxTopics,maxWatches}` 里公布并被强制执行。
- **`watches` 是文件监视的附加维度**（`features.fileWatch`）：`watchId` 由客户端起名并在 `files.changed` 上回显，
  `cwd` 缺省 serve 目录。
- **作用域 = 这条流本身**：topic / watch 集随订阅参数走，无独立 `watch` / `unwatch` 方法——**改集合 = 关流重订**。
- **信号只说"再读一次"**：每条事件带被影响资源的 id（`sessionIds` / `runIds` / `paths` / …），**不带业务数据**。
  客户端收到后调对应读方法重取（`state.changed` 带 `key`，指向该 key 声明的 `recoveryMethod`，`API.md §5.3`）。
- **每个 topic 都有生产者**：discovery 里出现的 topic，runtime 一定会在对应提交之后发它。一个"名字在、流是静的"
  topic 比没有更糟——第二个窗口会安静地过时，并且察觉不到。

### 3.1 连接与投递模型

物理细节见 [`TRANSPORT.md`](./TRANSPORT.md)；语义约束在这里：

- 整个 app **共享一条**失效流（run 流之外的第二条常驻连接，多面板复用，不每面板开流）。**唯一性范围是每个客户端
  进程**。
- **无 `Last-Event-Id`、不补发**：重订 = 隐式全量失效（等价收到一次点名全部订阅 topic 的 `resync`）——与 run 流的
  durable 续传相对照。
- **先订阅、再拉列表**（先订后拉无丢失窗口）。
- **`sequence` 从 1 开始、在进程内严格单调，且只发给真正进入队列的帧**：为一个被合并掉的信号消耗号会让客户端
  以为丢了东西。
- **不丢帧**：来不及投递的失效**合并**成一条 `resync`，并**点名它替代了哪些 topic / watch**。客户端因此不需要靠
  "看见空号"去发现丢失 —— 在一条安静的流上那永远看不见。`resync.topics` 必填且非空。
- **集合不使用空数组制造第三种含义**：`files.changed.paths` 必填且非空；其他 narrowing array 出现时非空，
  所有集合均无重复。省略表示不进一步收窄，非空集合表示只重读所列资源。
- **`excludedEphemeralEvents` 不作用于本流**（`API.md §9`）：本流的事件全是"某域已失效"，抑制其中任何一条就等于让
  客户端持有陈旧缓存而不自知；要少收就少订 topic。
- 事件 `type` 在 run（`API.md §5`）与失效流两个联合内**全局唯一**：事件名在整个协议里是单一命名空间。

---

## 4. 会话回退 / 派生 / 归档

run 粒度、按 `runId` 寻址的三个会话操作：**回退**（就地销毁性截断）、**派生**（快照式复制）与**归档**（可移植文档）。

三者共享一条规则：**会话级 state（`API.md §5.3`）跟着会话历史的边界走**。一个回退 / fork / 导入之后仍显示回退前
任务清单的面板，描述的是一个已经不存在的会话。

### 4.1 `sessions.rollback`

丢弃某个保留边界之后的全部 run，就地截断会话历史。返回 `{ session, droppedRuns }`，其中
`DroppedRun{ run: RunSummary, userInput? }` —— `userInput` 是该 run 开场 userMessage 的 content（与
`StartRunRequest.input` 同型，composer 零转换预填），子 agent run 无开场用户轮故省略。

- **`toRunId` = inclusive-keep**：保留的**最后一个 root run**（含其停车-续跑的全部段），其后全部丢弃。省略
  `toRunId` = 丢弃全部、回到空会话（覆盖"编辑第一条消息重跑"）。
- `toRunId` **必须是 root run**（子 agent run → `invalid_params`）；未知 → `run_not_found`。
- **就地销毁**：截断聊天历史、删被丢 run 的 Item 与记录、清其悬挂 interrupt、清会话 goal 的关联，并**递归 purge
  被丢 run 派生的 subagent 子会话整棵子树**。
- **会话级 state 回到边界那一刻的值**，并且是**一次新的写入**（更大的 revision，`API.md §5.3`）。不是删掉那一行 ——
  删掉会让 revision 归零，客户端手上更大的 revision 会把回退后的值当旧值丢掉。**边界没有被记录过**（导入进来的
  run 在别的 runtime 结束）时**不动 live 值**：那是"从未捕获"，不是"空清单"。
- **运行中拒绝**：session 有 run 在飞 → `session_busy`（不与正在 append 的历史赛跑）。
- **`restoreType`**（默认 `"history"`；`"files"` / `"both"` 受 `features.checkpoints` 门控）：
  - `"history"` —— 只回退聊天历史，不动文件。客户端用 §2.3 自查未还原改动。
  - `"files"` —— 只把工作区文件还原到 `toRunId` 的影子-git 快照，历史不动。
  - `"both"` —— 二者，**原子**：files 先行，失败则整体失败、history 不动、返 `checkpoint_unavailable`，
    **绝不静默降级**。
  - `"files"` / `"both"` **必须带 `toRunId`**（否则 `invalid_params`）；该 run 无快照 → `checkpoint_unavailable`。
    还原前自动快照当前态（可 unrevert）。

### 4.2 `sessions.fork`

把会话历史复制到一条新会话（快照语义），继承源 cwd。

- 省略 `fromRunId` = 整段 fork；给定 = **含 `fromRunId` 在内**截断复制到该 run 边界。
- **只复制已完结的 run**（in-flight run 不进副本，等价"先 interrupt 再 fork"）——故 fork **无 `session_busy`**，
  与 §4.1 的"运行中拒绝"差异即在此。
- **会话级 state 取那个边界的值**（不是源会话现在的值）：fork 出来的会话是"那一刻"的复制品，把现在的清单塞进去
  等于伪造它的历史。

### 4.3 `sessions.export` / `sessions.import`

同一份 `SessionArtifact`（**version 7**）的两端：终态 run + 完整 Item 历史 + chat 消息 + offload 的工具正文 +
会话级 state 的语义值。`format:"md"` 是人读转录（**不可再导入**）。

- **只带终态 run**：live 与 interrupted 的 executor 状态是进程本地的，不可移植。
- **state 只带语义值**，不带 `revision` / `updatedAt`：那是源 runtime 的排序凭证，带过去会让导入的值声称一个目标
  runtime 从未发出的位置。导入方**自己发号**。
- **import 是替换语义**（同 id 覆盖），在**任何写入之前**拒绝它无法完整还原的文档：版本不认识（不迁移）、
  含本 build 未广告的 state key、含 run 树（`features.subagents` 关时 —— 摊平一棵树等于导入一个归档没有描述过的
  会话）。
- **归档记录不复用 live 响应 DTO**：live 响应带进程本地与派生的展示态，归档是一份 durable 输入文档。

---

## 5. MCP 生命周期

`mcp.*` 受 `features.mcp` 门控。条目富化（`toolCount` / `authStatus` / `error` 内联），免去
`mcp.servers.list ⨝ mcp.tools.list` 的 join；`mcp.tools.list` 留给详情面板（分页 + `inputSchema`）。

`McpStatus` 是闭合联合：`connecting` / `connected` / `disconnected` / `failed` / `needsAuth`。

> **`disconnected` 是联合里一个没有作者的值** —— domain 的连接状态机只有四态（"没连上"要么是 `failed`、要么是
> `needsAuth`；"没配置"则该条目根本不在列表里）。客户端仍须容忍它（它在联合里），但不必为它设计任何 UI。

`McpServer.error` 是**内联状态**（`API.md §8.4`）：`failed` → `mcp_dial_failed`，`needsAuth` →
`mcp_authorization_required`。它们没有 `detail`，文案归客户端按 `type` 查本地表。

### 5.1 状态推送与 `mcp.servers.reconnect`

`mcp.servers.reconnect{ server }` **无同步返回**（结果走推送）。

- 进度经失效流的 **`mcp.changed`** 投递，**保证顺序 `connecting → (connected | failed | needsAuth)`**：客户端按钮
  loading 态绑 `connecting`，终态解除。重连成功热刷新工具集，模型即时可见新工具。
- `mcp.changed` 语义：server 条目**增 / 删 / 任意字段变化**均发，带受影响的 `serverIds`。信号不带条目本身 ——
  客户端重拉 `mcp.servers.list`（§3）。
- 启动时**容忍单个 server 失败**：一个连不上的 server 不该让整个 runtime 起不来，它以 `failed` + 内联 error 出现在
  列表里。
- `mcp.servers.authorize` 与 `mcp.configs.*`（可编辑注册表 CRUD + 连接测试）共用这条推送路径。

---

## 6. 审批 scope

`InterruptResponse` 的 `approval` 分支（经 `runs.resume` 回传，`API.md §6.1`）携带可选的记忆与一次性改写：
`decision`（`approve` / `deny`）+ `remember?{scope}` + `editedArgs?` + `reason?`。

- **规则的 KEY = 工具名 + 该次调用的 per-tool `subject`**：`shell` 取 `command`，`read`/`write`/`edit`/`download`
  取 `file_path`，其余工具 subject 为空串（= 整个工具，任意参数）。subject 由**后端从被批准的调用参数中提取**，
  客户端不发送它。`subject` 支持 glob（`path.Match`；`*` 不跨 `/`，`**` 无特殊含义）。
  ⚠️ MCP 工具的 key 是**模型可见名**（塌名后的 `<server>_<tool>`，`API.md §4.4`），塌名的两个工具共享一条规则。
- **`deny` + `remember` 合法**——记住"拒绝"。**`editedArgs` 一次性**：规则按 subject 匹配，绝不按一次性的参数
  改写匹配。
- **`scope`**：三个 scope **全部真持久**（sqlite）—— `session` 绑该会话 id、`project` 绑项目目录、`global` 无 key。
  会话被删时其 session 规则一并清除。
- **冲突策略**：最具体的命中规则胜出（scope 窄 > 宽：session > project > global；再 exact subject > glob >
  整工具）；同特异度而结论相反时取 **deny**（记住的拒绝不会被同级的放行抵消）。
- 不设 `once`（= 不带 `remember` 的普通 approve）；不设 `ask` / `behavior`（响应本身即那次 ask 的回答，重复）。

读与管理面是 `approval.listRules` / `approval.forgetRule`，全局姿态是 `approval.getMode` / `setMode`
（`API.md` 附录 C.2）。

---

## 7. 明确不做

| 项                                                                     | 理由                                                                                                      |
| ---------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| `background.*`（任务注册表 + 独立通知 + `BackgroundTask`）             | 后台子任务 = 子 agent 的 run，挂在 run 树上随其流式（`API.md §5.4`）；无需 client 可见的第二套任务注册表。 |
| `items.edit`                                                           | run 粒度下"编辑某条重跑" = §4.1 `sessions.rollback{toRunId}` + `runs.start`，item 级精确编辑无独立理由。   |
| `sessions.fork.fromItemId`                                             | 改用 `fromRunId`：run 边界可靠解析，无需 item↔message join。                                              |
| 失效流的 `Last-Event-Id` 续传                                          | 本流的事件是"某域已失效"，重订即隐式全量失效，比补发一串已过期的失效更便宜也更正确（§3.1）。               |

---

## 附录 · 类型索引

本文约束的 wire 类型：`WorkspaceFileChange` · `Diff` · `FileDiff`（§2）、`RuntimeEvent` · `RuntimeTopic` ·
`WatchSpec`（§3）、`DroppedRun` · `SessionArtifact`（§4）、`McpServer` · `McpStatus`（§5）、
`InterruptResponse.approval` 的 `remember` / `editedArgs`（§6）。**字段表见
[`schema.json`](../../../runtime/contract/schema.json)**。

复用 `API.md` 的语义定义：`FileStatus` · `DiffRow`（§4.5）、`RunSummary`（§4.2）、`ContentBlock`（§4.3）、
`Session`（§4.1）、`Page<T>`（§4.11）、`ProblemData` · `FieldError`（§4.6）。

---

> 正式契约。配套同目录 [`API.md`](./API.md) + [`TRANSPORT.md`](./TRANSPORT.md)；字段级真相在
> [`runtime/contract/`](../../../runtime/contract/)。
