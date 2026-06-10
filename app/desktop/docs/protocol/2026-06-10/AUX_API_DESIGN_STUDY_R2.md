# 旁路 API 调研 · 第二轮(六家 agent)+ 定论 · `2026-06-10`

> 接 [`AUX_API_DESIGN_STUDY.md`](./AUX_API_DESIGN_STUDY.md)(第一轮 codex / opencode)与前端
> [`AUX_API_DESIGN_RESPONSE.md`](./AUX_API_DESIGN_RESPONSE.md)。本轮后端又深读了**六家**同类 agent —— **crush**(Go)、
> **kimi-code**、**cline**、**continue**、**ag-ui**、**Claude Code** —— 重点仍是旁路 API / 前后端协议。
> 本文:**① 六家相对 codex/opencode 的新东西(只记 delta);② 八家看完、可以拍死的定论;③ 留给前端再看的开放项。**
>
> 前端 RESPONSE §1 已修正第一轮关于 opencode 的一处事实(它确实做了按消息点 fork + part 级 revert,靠影子 git
> `write-tree`)——本轮的 checkpoints 定论建立在那个修正之上,不再重复。

---

## 1. 六家的新增 delta(只记 codex/opencode 没有的)

| 项目 | 语言 / 形态 | 相对 codex/opencode 的新东西 |
|---|---|---|
| **crush** | Go / TUI(内部 service+pubsub) | **泛型 `Broker[T]` 两档投递**:`Publish`(有损,背压丢——流式)/ `PublishMustDeliver`(有界阻塞+per-sub 超时——终态);**每 domain 一个 broker**;wire 用 discriminated `Payload{type,payload}`,已含 `mcp_event`/`skills_event`/`file`/`permission_request` |
| **kimi-code** | TS / CLI+TUI(+WS 协议) | 事件带 **per-session 单调 `seq`** + 重连 `last_seq_by_session` + 服务端 `resync_required`(溢出/会话重建);**非流式事件**(`mcp.server.status`/`tool.list.updated`)一等公民;`ToolUpdate` 带 custom kind(如 mcp.oauth URL);`PromptOrigin`(turn 来自 user/skill/cron/hook/retry) |
| **cline** | TS / VS Code(gRPC+protobuf) | **每 domain `subscribeToX → stream`**(state/mcp/checkpoints),core 持 `activeSubscriptions` 扇出 + 断连清理;**影子 git checkpoints**:`checkpointRestore{ restore_type: "task"\|"workspace"\|"taskAndWorkspace" }`;**Host Bridge** 抽象(core 跑 VS Code/CLI/standalone) |
| **continue** | TS / 多 IDE(消息协议) | **类型化 message-map**(`"method":[Req,Resp]`)+ **`AsyncGenerator` 当返回**把 req/resp 与流式**统一在一条通道**(`done:false` 增量 / `done:true` 终值),`messageId` 关联;**`IDE` 接口** = 现成的"host 旁路能力清单"(见 §2.4) |
| **ag-ui** | TS / 协议规范 | **`STATE_SNAPSHOT` + `STATE_DELTA`(RFC 6902 JSON Patch)**做状态同步;事件 discriminated union(Zod);**SSE/JSON 与 protobuf 双 wire,靠 Accept 协商**;outcome 用 discriminated union(success vs interrupt)而非错误码 |
| **Claude Code** ⭐ | TS / CLI+SDK(单流) | **单条 NDJSON 复用 data + 双向 control**(`control_request`/`control_response`,`request_id` 关联):`can_use_tool`/`hook_callback`/`mcp_message`/`set_permission_mode`/`set_model`/`interrupt`/**`rewind_files{user_message_id}`**;审批带 **`permission_suggestions[]`** + 响应可带 **`updatedInput`**(改入参)+ **`updatedPermissions{behavior,destination:session\|project\|user\|local}`**;**hooks 一等异步回调**(27 事件含 `FileChanged`/`CwdChanged`,可 block/改);compact boundary 带 **`preserved_segment{head,anchor,tail}_uuid`**;`session_state_changed: idle\|running\|requires_action` |

> 注:Claude Code「单流复用 data+control」是因为它是**单客户端 stdio**。我们是 HTTP 多客户端 + durable run 流,**不照搬通道合并**,只借它的 **request_id 关联模式**和审批/rewind 语义。

---

## 2. 八家看完可以拍死的定论

### 2.1 Checkpoints / 回退 —— 方向定死 ⭐

四个独立印证(cline `restore_type`、Claude Code `rewind_files{message_id}`、opencode 影子 git `write-tree`、Claude Code compact `anchor_uuid`)指向同一答案:

> **文件回退是独立操作,锚在某个回合/消息边界,且与"截断对话历史"分离。**

- **v1(不阻塞)**:turn 粒度回退(`sessions.rollback`/`fork{fromRunId}`),**不动文件**,返回 `residualDiff` 让前端提示"这些改动未还原"(已与前端对齐,见 RESPONSE §2)。
- **v2**:**影子 git 快照锚在 run 边界** —— 把 snapshot ref 盖在 durable `RunRef`/`Item` 字段上(**这正是穿过"message↔item 无关联表"那个 blocker 的路:不建关联表,把 hash 锚在流上**,opencode/Claude Code 都这么干)+ **`restoreType: history | files | both`**(借 cline,显式分离)。

### 2.2 旁路推送通道 —— 模式定死,加两个细节

per-domain notification + 连接级(不补发),已被 cline(每 domain 一条 subscribe)、crush(每 domain 一 broker)、kimi(非流式事件)三方印证。**加**:
- **`seq` + `resync_required`**(借 kimi):连接级也给个"你可能漏了、请重拉"的轻量信号,省得客户端拿陈旧状态。
- **两档投递**(借 crush):旁路事件走**有损档**(本质是"变了→重拉"),与 run 流的 durable 明确分层。
- **control 关联用 `request_id`**(借 Claude Code/continue):凡是 server↔client 的请求/响应都 UUID 关联 + pending map + 断连 reject。
- (记着)事件类型多了再考虑收敛成 **ag-ui 的 snapshot+delta(JSON Patch)**;现在三个事件 bespoke method 更 KISS。

### 2.3 审批 scope —— 升级 `approveForSession`

借 Claude Code:把 `ApprovalResponse` 的 `approveForSession` 泛化成
**`updatedPermissions: { behavior: allow|deny|ask, scope: session|project|user }`**,并支持 **`updatedInput`**(批准前改工具入参,用于路径规范化/补默认)。前端审批卡片相应留位。

### 2.4 workspace.* 的长期清单 —— 用 continue 的 `IDE` 接口当 checklist

continue 的 `IDE` 接口是"coding agent 的 host 到底需要哪些旁路能力"的成熟样本,拿来当我们 workspace 的 roadmap(**带 ★ 的是我们之前没列过的**):
`readFile / readRangeInFile / listDir / getFileStats` · `getDiff(includeUnstaged ★) / getBranch / getRepoName / getGitRootPath` · `getProblems(诊断 ★)` · `gotoDefinition / getReferences / getDocumentSymbols(LSP ★)` · `getOpenFiles / getCurrentFile` · `onDidChangeActiveTextEditor`。

### 2.5 MCP 状态枚举 —— 对齐

八家高度一致 → **`connecting | connected | disconnected | failed | needsAuth`**(取代我先前临时写的 `starting|ready|failed|cancelled`)。reconnect + 内联 `toolCount`/`authStatus` 全员印证。

### 2.6 进 backlog(非当前,但有范本)

- **Hooks**(Claude Code 为范本,27 事件、可 block/改入参、含 `FileChanged` 当 hook)—— 真正的扩展机制。
- **审批 `updatedInput`**(批准前改参)、**`session_state` 信号**(idle/running/requires_action)、**kimi `PromptOrigin`**(因果/可观测)。
- **continue 的「AsyncGenerator 当返回统一流式+req/resp」**、**ag-ui 的 protobuf 双 wire** —— 架构级,记录,暂不动我们 JSON-RPC+SSE 主线。

---

## 3. 留给前端再看的开放项

1. **checkpoints v2 的 `restoreType` 语义**:`history` / `files` / `both` 三态够不够?前端"编辑某条消息重跑"映射到哪个(应是 `history`,文件留给用户决定)?
2. **审批 `updatedPermissions.scope`**:`session|project|user` 三层是否匹配前端的"记住选择"UI?要不要 `once`(仅本次)?
3. **旁路通道 `resync_required`**:前端拿到后是全量重拉(listSkills/mcp.listServers/getDiff)还是按事件类型局部重拉?
4. **MCP 状态枚举**:`connecting|connected|disconnected|failed|needsAuth` 是否覆盖前端 UI 需要的全部态?

---

## 4. 下一步

调研收口。接下来产出两份接口规范(样本现在最足),前端定稿后再写代码:

1. **旁路通知通道规范**(`notifications.workspace.*` envelope + `request_id` 关联 + `seq/resync` + 连接级边界 + TRANSPORT 补一节)—— gate 了 fs.watch / skills.changed / mcp.statusChanged 三项。
2. **git `/vcs` 规范**(`getDiff` per-file+DiffRow / `listFileChanges` 带 ±行 / `mode:worktree|base` 基线钉死 / `features.git` 退化)—— P0、最自包含。

> 退化策略(承前):git 未安装/非仓 → 启动探测反映到 `features.git`,前端隐藏面板;真调到返明确 `vcs_unavailable`,不和"干净仓"塌成空(八家里 codex/opencode 都把这三态塌成空,我们做得更诚实)。

---

**给前端**:本文是调研收口 + 后端定论,请就 §3 四个开放项回意见(尤其 checkpoints restoreType、审批 scope)。回完后端开写 §4 两份规范,落地节奏同 `BACKEND_CAPABILITIES.md`。
