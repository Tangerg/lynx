# 旁路 API 设计调研 · codex / opencode 对照 · `2026-06-10`

> 目的：LLM 流式我们已经做得不错了，欠缺的是 **session / git / 文件系统 / MCP / skills / checkpoints** 这些**旁路 API**
> 的设计。本文调研 OpenAI **codex**（Rust app-server）与 SST **opencode**（TS HTTP）两套同类产品的前后端协议，逐块对照，
> 给出 lyra 该借什么、怎么落到我们协议（§x.y 指 [`../API.md`](../API.md)）。**结论导向，给后端排期 + 前端预期对齐用。**
>
> 一句话：**git 抄 opencode 的结构化 `/vcs`；checkpoints 抄 codex 的「turn 粒度回退」（绕过我们一个 blocker）；
> background 两家都没有、建议砍。**

---

## 0. 两套的总体取向

| | **codex**（app-server，Rust） | **opencode**（HTTP server，TS） |
|---|---|---|
| 传输 | 改良 JSON-RPC（stdio / WS）；**server 主动请求复用同一 request 信封** | REST + SSE；schema-first → OpenAPI / SDK 自动生成 |
| 旁路 API 哲学 | **薄**——能让 agent 自己 `exec` 干的就不进协议（git 尤甚） | **厚**——结构化 endpoint 全给前端 |
| 我们（lyra） | 协议信封、R-model HITL 取向像 codex | 但旁路 API 的「结构化程度」应学 opencode |

和我们最相关的，恰好是两家分歧最大的几块。下面按「该借谁」组织。

---

## 1. Git / VCS —— 我们最大的缺口，直接抄 opencode

两种哲学：

- **codex 几乎不做**：thread 上只存 `GitInfo { sha, branch, originUrl }` 当展示元数据（`thread/metadata/update` 写入）；
  真正的 `git diff/status` 让**模型自己 `exec` 跑并解析**。v1 曾有 `gitDiffToRemote`，v2 砍了。
- **opencode 做成结构化 `/vcs` 组**（这就是我们 `workspace.getDiff` / `listFileChanges` 的现成模板）：

  | endpoint | 返回 |
  | --- | --- |
  | `GET /vcs` | `Vcs.Info { branch, origin, … }` |
  | `GET /vcs/status` | `FileStatus[] { path, added, removed, status }` |
  | `GET /vcs/diff?mode=working-tree\|default-branch` | `FileDiff[] { file, patch, additions, deletions, status }` |
  | `GET /vcs/diff/raw` | 原始 patch（`text/x-diff`） |
  | `POST /vcs/apply` | 应用 patch |

  对比我们：`WorkspaceFileChange { path, status }` 缺**每文件 +/- 行数**；`Diff { rows, truncated }` 缺 **`mode`
  选「工作区改动」vs「相对基线分支」**。codex 的 `gitDiffToRemote`（相对 remote）也印证「相对 base 分支」这个视图重要。

**→ lyra 建议**：`workspace.getDiff` / `listFileChanges` 按 opencode 形状落地：
- `WorkspaceFileChange` 加 `added` / `removed` 行数；
- `getDiff` 加 `mode: working-tree | base`（默认 working-tree）+ 一个 raw-patch 变体；
- 后端 `exec` git 即可（opencode 也没上 git 库）。

---

## 2. Checkpoints / 回退 —— codex 给了绕过我们 blocker 的路 ⭐（本文最有价值的一条）

我们此前判 `items.edit` / 「按某条消息分叉」卡在 **message↔item 无位置关联**（持久 Item 日志与 chat-memory 消息日志是两条平行表示）。看两家怎么做：

- **codex `thread/rollback { turnCount }`** —— 丢掉最后 N 个 **turn**，持久化 marker。**turn 粒度，不是 item 粒度。**
- **opencode** 用 **per-message workspace snapshot** + `POST /session/:id/revert` + `GET /session/:id/diff`
  （从某 snapshot 起的文件 diff）—— 也是**回合 / 文件快照粒度**，不做消息截断。

**关键洞察**：两家都**不做 item 级历史截断**，而是 **turn / 回合级**回退。而我们的模型里 **一个 run = 一个 turn，run 边界可靠可知**
（`history.Store` 的 Item 带 `RunID`，`items.list` 也带 `RunRef`）。所以：

> `items.edit` / 「按点 fork」做不到 item 精确，但**完全能做到 turn 精确**——「回退最后 N 轮 / 从第 K 轮分叉」，
> 按 run 边界截断 `chat.Message` 历史。**这不需要补 message↔item 关联那块数据模型。**

这把先前判「真缺底层」的 checkpoints，降级成「按 run 边界回退」的可实现活。

**→ lyra 建议**：不要做 item 精确的 `items.edit`；改提供 **turn 粒度**能力：
- `sessions.fork` 的 `fromItemId` → 改/补一个「从第 K 个 run 分叉」语义（按 run 边界截断 + seed）；
- 可加一个 `sessions.rollback { turnCount }`（丢最后 N 轮）对标 codex。
- 前端「编辑某条消息重跑」的 UI → 映射成「回退到那一轮之前再发」。

---

## 3. 文件系统 —— 我们已做的对，缺「文件监听」与「模糊找文件」

`getFileHead` / `grep` 已通，方向两家都印证。两个值得补：

- **文件监听**：codex 有 `fs/watch` / `fs/unwatch` + `fs/changed { watchId, changedPaths }` 通知；opencode 有
  `file.edited` / `fileWatcher.updated` 事件。**前端文件树 / diff 面板要实时刷新就需要它，我们目前零。**
- **模糊找文件**：opencode `GET /find/file`（fuzzy 路径，给 "@file" 提及用）、`/find/symbol`（LSP 符号）。
  我们可加 `workspace.findFiles`。
- 细节：opencode 读文件返回 `{ type:text|binary, content, encoding, mimeType, patch? }`，比我们 `FileHead` 富；
  codex 全用**绝对路径 + base64**（二进制安全）。我们 jail-相对路径更适合多 project，**保持**。

**→ lyra 建议**：优先级上，**文件监听事件**（接 `notifications.*` 或 workspace 专用事件流）对前端体验影响最大；
`findFiles` 次之。

---

## 4. MCP —— 借 codex 的「server 状态包」+ opencode 的 connect/disconnect

- **codex `mcpServerStatus/list`** 一个调用返回每 server 的
  `{ name, serverInfo, tools(map), resources, resourceTemplates, authStatus }` —— **status + tools + resources + auth 一次性打包**。
  我们刚把 `mcp.listTools` 做成单独调用；他们是 listServers 直接内联 tools。
- **opencode `/mcp/:name/connect` + `/disconnect`** —— 正是我们缺的 `mcp.reconnect`。
- 两家都暴露 **auth 状态**（OAuth / bearer / none）；codex 还含 **resources + resourceTemplates**
  （印证我们 deferred 的 MCP Resources primitive —— 真做就并进 server status，别单开一套）。

**→ lyra 建议**：`workspace.mcp.reconnect` 照 opencode connect/disconnect 落地；长远把 `mcp.listServers` 条目富化
（带 tool 数 / auth 状态），少一次往返。

---

## 5. Skills —— `listSkills` 已对齐，可补「启停 + 变更事件」

- **codex `skills/list`** 条目带 **`enabled` 状态 + `interface`(inputs JSON schema) + scope(global/local)**，
  且有 **`skills/config/write`**（按 skill 启停）和 **`skills/changed`** 文件监听通知；外加整套 plugin / marketplace（我们不需要）。
- **opencode** 只读 `/skill` `/agent` `/command` 枚举。

我们的 `workspace.listSkills`（name / description / scope）已对齐 opencode。

**→ lyra 建议**：要更进一步就借 codex 的 **`enabled` 字段 + `skills.changed` 事件**（配合 §3 的文件监听）；非紧急。

---

## 6. Session 生命周期 —— 两个可借点

- **codex `thread/turns/list`**：**不 resume 就能分页翻某会话历史**，双向 cursor + `itemsView`（full / summary / notLoaded
  详情级别）。长会话可扩展。我们 `items.list` 可加「详情级别」。
- **archive / unarchive**（codex 软隐藏会话）、**列分叉**（opencode `GET /session/:id/children`；我们 service 有
  `Children` 但没暴露）—— 前端会话列表 UX 都用得上。

**→ lyra 建议**：`sessions.list` 加 `archived` 过滤 + `sessions.archive/unarchive`；暴露 `sessions.children`。非紧急。

---

## 7. 反向印证：Background 建议砍

- **codex 根本没有 client 可见的 background 任务注册表** —— 明确说「所有异步 = turn item，挂 run 树上流式」；只有底层
  `process/spawn` / `kill`。
- **opencode** 也只有 PTY，没有 background jobs 队列。

**→ lyra 建议**：协议里的 `background.*`（顶层任务注册表）**大概率 over-spec**。更省的模型是「后台子任务 = 子 agent 的
turn，挂 run 树上流」。建议 **砍掉 `background.*` 或冻到最低优先级** —— 这反而是去历史债务。

---

## 8. HITL —— opencode 反向印证我们选型是对的

- **codex**：server **主动 JSON-RPC 请求**（复用同一 request 信封）问 approval / patch / permission / elicitation；
  决策枚举丰富（Accept / **AcceptForSession** / AcceptWithAmendment / Decline / Cancel）。
- **opencode**：approval 做成 **durable 可轮询资源** —— `GET /permission`（列待批）+ `POST /permission/:id/reply` +
  SSE `permission.asked` 事件；外加 `/question`（带 labels 的结构化提问）。

我们的 R-model（durable、靠 continuation run 续）更接近 opencode 那条 —— **对断网更稳，选型被印证是对的**。可借 codex 的
**`AcceptForSession`**（本会话内记住批准）丰富我们的 approval decision。

---

## 9. 几条传输层设计点（尤其 codex，值得吸收）

1. **experimental-API 字段级门控**：codex 在单个字段 / 方法上标 `experimental`，client 在 `initialize.capabilities` 里
   opt-in —— **让半成品能干净地随协议发布、不破坏旧 client**，正合 dev 阶段快速迭代。
2. **双向 cursor 分页**（codex 处处 `nextCursor` + `backwardsCursor`）。
3. **durable sync + replay**（opencode `/sync/history` cursor 重放）—— 我们 `runs.subscribe` 的 `Last-Event-Id` 已有类似，
   可推广到 session 列表级。
4. **server 主动请求复用 request 信封**（codex）—— 我们暂不需要（R-model 已覆盖），记录。

---

## 10. 行动建议（结合调研后重排优先级）

| 优先级 | 事项 | 借鉴 | 备注 |
| --- | --- | --- | --- |
| **P0** | `workspace.getDiff` + `listFileChanges` | opencode `/vcs` | exec git，最自包含、前端最想要 |
| **P0** | checkpoints 改 **turn 粒度**（rollback 最后 N 轮 / 从第 K 轮 fork） | codex `thread/rollback` | 绕过 message↔item blocker，比原判轻得多 |
| **P1** | 文件监听 `fs.watch` + `changed` 事件 | codex `fs/watch` | 前端文件树 / diff 实时刷新 |
| **P1** | `mcp.reconnect`（connect/disconnect） | opencode `/mcp/:name/connect` | |
| **P2** | `listSkills` 加 `enabled` + `skills.changed` 事件 | codex | 配合文件监听 |
| **P2** | `sessions.archive/unarchive`、`children`、history 详情级别 | codex / opencode | 会话列表 UX |
| **去债** | 砍 / 冻结 `background.*` | 两家都没有 | 顶层任务注册表 over-spec |

> 仍真缺底层（需新依赖 / net-new）：attachments（blob + 文件通道，连带 `sessions.export`）。MCP Resources / Prompts
> 维持 deferred（见 lynx `mcp/CLAUDE.md` 的 primitive 取舍）。

---

**给前端**：以上是方向。P0 两项（git diff、turn 粒度回退）后端会先出接口方案再实现，落地后更新
[`BACKEND_CAPABILITIES.md`](./BACKEND_CAPABILITIES.md) 同款的「怎么调 + 边界」说明。对接口形状有偏好（尤其 diff 的字段、
回退的粒度语义）现在就可以提，趁还没写死。
