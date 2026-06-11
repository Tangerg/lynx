# Lyra Runtime 后端 API 参考 · 已实现能力 + 调用方式 · `2026-06-11`

> **本文是"后端此刻真正实现了什么、怎么调"的权威清单**(以代码为准,非愿景)。形状的正式契约见
> [`../API.md`](../API.md)(wire 类型全集)/ [`../AUX_API.md`](../AUX_API.md)(旁路 API)/ [`../TRANSPORT.md`](../TRANSPORT.md)
> (传输层)。本文聚焦**逐方法的状态(live / off)+ 怎么调 + 边界**。
>
> 第一原则:**按 `runtime.initialize` 返回的 `ServerCapabilities` 协商**,不要硬编码方法可用性。下面标 `OFF` 的方法
> 现在返回 `capability_not_negotiated`,能力位翻开才可用。

---

## 0. 传输与通用约定(怎么调)

- **端点**:所有业务调用 `POST /v2/rpc/{method}`(method 名照搬、点不斜杠化,如 `POST /v2/rpc/sessions.create`)。
- **Envelope**:JSON-RPC 2.0 —— `{ "jsonrpc":"2.0", "id":"<任意>", "method":"<同 path>", "params":{...} }`。
- **鉴权**:`Authorization: Bearer <token>`,token = `~/.lyra/local-token`(本地进程门禁)。缺/错 → `401` + `WWW-Authenticate: Bearer`。
- **握手(必须)**:每条连接**先调一次 `runtime.initialize`**,否则业务方法返 `capability_not_negotiated`("runtime.initialize must succeed before any business method")。`runtime.ping` / `runtime.initialize` 本身豁免。HTTP 按连接亲和保存握手态,所以用 keep-alive 连接;InProcess 是单条长连接。
- **流式方法**(`runs.start` / `runs.resume` / `runs.subscribe` / `workspace.subscribe`):POST 响应体本身是 `text/event-stream` —— **首帧 = JSON-RPC 响应(ack,无 SSE id)**,其后是 `notifications.run.event` / `notifications.workspace.event` 帧(run 事件帧带 SSE id = eventId)。**无独立 SSE 端点、无 `X-Conn-Id`**。run 事件可用 `Last-Event-Id` 续传(仅 run 流 durable);workspace 流不补发(重订 = 一次隐式 `resync`)。
- **Sidecar(无鉴权、flat JSON)**:`GET /v2/info`(能力快照 + ops 信息)、`GET /v2/health`(`{"ok":true,...}`)。
- **幂等**:写操作可带 `X-Idempotency-Key`;同 key 不同 params → `idempotency_conflict`。
- **错误**:`error.data` 是 `ProblemData`,**按 `type`(symbolic name)分支,不看数字码**。HTTP status 只反映传输层(200/204/400/401/404/405/413/415/500/503)。
- **协议版本**:`X-Protocol-Version` header;`initialize` 协商失败 → `invalid_protocol_version`(硬断开)。
- **trace**:W3C `traceparent`(无自有 trace header)。

### 调用骨架(curl)

```bash
TOK=$(cat ~/.lyra/local-token); BASE=http://127.0.0.1:17171
# 1) 握手(每连接一次)
curl -s -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":"1","method":"runtime.initialize","params":{}}' \
  $BASE/v2/rpc/runtime.initialize
# 2) 业务方法(同连接)
curl -s -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":"2","method":"sessions.create","params":{}}' \
  $BASE/v2/rpc/sessions.create
```

---

## 1. 能力快照(`runtime.initialize` / `GET /v2/info`)

```jsonc
{
  "protocolVersion": "<v2>",
  "streamingMethods": ["runs.start","runs.resume","runs.subscribe","workspace.subscribe"],
  "features": {
    "reasoning": true,        // 产 reasoning item / delta
    "mcp":       true,        // workspace.mcp.* + MCP 工具
    "memory":    true,        // memory.*(有 LYRA.md memory 服务时)
    "skills":    true,        // workspace.listSkills
    "git":       true,        // workspace.listFileChanges/getDiff(git 在 PATH 时;否则 false)
    "fileWatch": true,        // workspace.subscribe 的 git 状态监视
    "relocate":  true,        // sessions.update 改 cwd
    "multimodal":    false, "checkpoints":   false, "subagents": false,
    "sessionExport": false, "clientTools":   false, "attachments": { "enabled": false }
  },
  "providers": ["anthropic","openai","deepseek","moonshot", ...],   // 后端支持的 provider 类型
  "limits": { "maxConcurrentRuns": 8 }
}
```

> **注意**:`checkpoints:false` **不**门控 `sessions.rollback` / `fork{fromRunId}` —— 那俩是 turn 粒度 history 回退,**常备可用**;`checkpoints` 现在专指未做的 v2 影子-git 文件快照(`restoreType`)。

---

## 2. 方法清单(逐方法状态 + 调用)

图例:**✅ live** / **�ƒ 能力门控**(开了才可用)/ **⛔ OFF**(当前 notImpl)。

### 2.1 `runtime.*`

| 方法 | 状态 | 入参 → 返回 | 说明 |
| --- | --- | --- | --- |
| `runtime.initialize` | ✅ | `{ clientCapabilities? }` → `ServerCapabilities` | 每连接首调;协商能力 + 客户端声明 `interruptTypes`/`events` |
| `runtime.ping` | ✅ | `{}` → `{}` | 握手豁免;保活 |
| `runtime.shutdown` | ✅ | `{}` → `{}` | 优雅关停信号 |

### 2.2 `sessions.*`(API.md §7.2 / AUX_API §4)

| 方法 | 状态 | 入参 → 返回 | 关键边界 |
| --- | --- | --- | --- |
| `sessions.list` | ✅ | `{ cursor?, limit? }` → `Page<Session>` | 游标分页 |
| `sessions.get` | ✅ | `{ sessionId }` → `Session` | `session_not_found` |
| `sessions.create` | ✅ | `{ cwd?, title?, model?, metadata? }` → `Session` | cwd 缺省 = serve 目录 |
| `sessions.update` | ✅ | `{ sessionId, title?, cwd?, model?, metadata? }` → `Session` | 改 cwd = relocate(门控 `relocate`),坏路径 → `cwd_unavailable`;空标题 → `invalid_params` |
| `sessions.delete` | ✅ | `{ sessionId }` → 无 | |
| `sessions.fork` | ✅ | `{ sessionId, fromRunId?, title? }` → `Session` | 省略 `fromRunId`=整段复制;给定=**含该 run 在内**截断复制;只复制已完结 run;`run_not_found` |
| `sessions.rollback` | ✅ | `{ sessionId, toRunId? }` → `{ session, droppedRuns[] }` | `toRunId` inclusive-keep(必须 root run),省略=清空;运行中 → `session_busy`;递归清被丢 run 的 subagent 子会话;非 root → `invalid_params` |
| `sessions.export` | ⛔ | — | `sessionExport` off |

### 2.3 `runs.*`(API.md §7.3 / §6 HITL)

| 方法 | 状态 | 入参 → 返回 | 说明 |
| --- | --- | --- | --- |
| `runs.start` | ✅ **流式** | `StartRunRequest` → ack `{ runId, userItemId? }` + 事件流 | 见 §3 |
| `runs.resume` | ✅ **流式** | `{ parentRunId, responses[] }` → ack + 续流 | HITL 应答,见 §4 |
| `runs.subscribe` | ✅ **流式** | `{ runId }` → ack + 流 | 重连/恢复活跃 run;`Last-Event-Id` 续传;非活跃 → `run_not_found` |
| `runs.cancel` | ✅ | `{ runId, reason? }` → 无 | 硬停;reason 进 outcome.detail |
| `runs.list` | ✅ | `{ sessionId?, cursor?, limit? }` → `Page<RunRef>` | 仅**运行中** run |
| `runs.listOpenInterrupts` | ✅ | `{ sessionId?, ... }` → `Page<OpenInterrupt>` | 跨重启可恢复的 open interrupt |

### 2.4 `items.list`

| 方法 | 状态 | 入参 → 返回 | 说明 |
| --- | --- | --- | --- |
| `items.list` | ✅ | `{ sessionId, cursor?, limit? }` → `Page<Item> & { runs: RunRef[] }` | 会话历史 = completed Item 序列 + run 树;按 id 去重乐观渲染 |

### 2.5 `workspace.*`(API.md §7.5 / AUX_API §2/§3/§5)

| 方法 | 状态 | 入参 → 返回 | 关键边界 |
| --- | --- | --- | --- |
| `workspace.listFileChanges` | 🔉 `git` | `{ cwd? }` → `Page<WorkspaceFileChange>` | git 工作树扫描;非仓 → `vcs_unavailable` |
| `workspace.getDiff` | 🔉 `git` | `{ cwd?, path?, mode?, format?, limit? }` → `Diff` | `mode` worktree(默认,含 untracked)\|base;`format` rows(默认)\|raw;`limit` 按文件边界截断+`truncated`;基线解析失败 → `invalid_params` |
| `workspace.getFileHead` | ✅ | `{ cwd?, path, lines? }` → `FileHead` | path jail(越界 `path_outside_root`);默认 200 行 |
| `workspace.grep` | ✅ | `{ cwd?, query, path?, limit? }` → `GrepResult` | `total ≥ len(matches)` 自描述截断 |
| `workspace.listProjects` | ✅ | `{ cursor?, limit? }` → `Page<Project>` | 按 distinct Session.cwd 派生,最近活跃优先 |
| `workspace.listSkills` | 🔉 `skills` | `{ cwd?, ... }` → `Page<Skill>` | 项目 skills 覆盖全局;`source` = project\|global |
| `workspace.listAgentDocs` | ✅ | `{ cwd?, ... }` → `Page<AgentDoc>` | 从 cwd 向上到 home 的 AGENTS.md 级联 |
| `workspace.mcp.listServers` | 🔉 `mcp` | `{ cursor?, limit? }` → `Page<McpServer>` | **5 态** + `toolCount` 内联 + failed 带 `error`(见 §5) |
| `workspace.mcp.listTools` | 🔉 `mcp` | `{ server?, ... }` → `Page<McpTool>` | server 维度;含 inputSchema |
| `workspace.mcp.reconnect` | 🔉 `mcp` | `{ server }` → 无(异步) | 结果走 `mcp.serverChanged` 推送(见 §5) |
| `workspace.subscribe` | 🔉 `fileWatch` **流式** | `{ watches? }` → ack + 事件流 | 见 §5(git 状态 + 工具推送模型) |

### 2.6 `providers.*` / `models.*`

| 方法 | 状态 | 入参 → 返回 | 说明 |
| --- | --- | --- | --- |
| `providers.list` | ✅ | `{ cursor?, limit? }` → `Page<Provider>` | 全部支持的 provider;`apiKeyMasked==""` = 未启用 |
| `providers.configure` | ✅ | `{ provider, type?, baseUrl?, apiKey? }` → `Provider`(masked) | upsert 运行态注册表(持久化);未知 provider → `invalid_params` |
| `providers.test` | ✅ | `{ provider }` → `{ ok, error? }` | 真实探活(max_tokens=1);失败回 `{ok:false,error}` 不报 RPC 错 |
| `models.list` | ✅ | `{ provider?, cursor?, limit? }` → `Page<Model>` | 直读内置 catalog,**不需 key、不门控**;省略 provider → 空页 |

### 2.7 `tools.*`

| 方法 | 状态 | 入参 → 返回 | 说明 |
| --- | --- | --- | --- |
| `tools.list` | ✅ | `{ cursor?, limit? }` → `Page<ToolSpec>` | 工具元数据(名/schema/safetyClass) |
| `tools.invoke` | ✅ | `{ name, arguments, cwd? }` → `unknown` | 不经 LLM 直调;`tool_denied` / `path_outside_root` |

### 2.8 可选域

| 方法 | 状态 | 说明 |
| --- | --- | --- |
| `memory.list` / `memory.get` / `memory.update` | 🔉 `memory` | LYRA.md 长期记忆;scope cwd\|projectRoot\|home(cwd/projectRoot 都落到按 cwd 寻址的 project scope) |
| `feedback.create` | ✅(write-only) | `{ sessionId?, runId?, itemId?, rating?, text? }` → 无;当前不留存(write-only-never-read) |
| `attachments.createUploadUrl` / `get` / `delete` | ⛔ | `attachments` off |

---

## 3. 跑一个 run(`runs.start`,流式)

```bash
curl -sN -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":"3","method":"runs.start","params":{
        "sessionId":"<sid>",
        "input":[{"type":"text","text":"你好"}],
        "provider":"deepseek","model":"deepseek-v4-flash"
      }}' \
  $BASE/v2/rpc/runs.start
```

- **`provider`+`model` 成对**:都给=选定;都不给=默认;只给一个 → `invalid_params`(provider 不从 model 推断)。
- **`mode`**:`agent`(默认,工具循环)\| `chat`(单轮无工具)\| `plan`(计划预览)。
- 响应:首帧 ack `{ runId, userItemId }`(`userItemId` 让前端按 id 对齐乐观气泡);其后事件流。
- **事件**(`notifications.run.event`,params `RunEvent`):`run.started` → `item.started`/`item.delta`/`item.completed`(userMessage / agentMessage / reasoning / toolCall / plan / question)→ `run.finished{ outcome }`。
- **outcome**:`completed` / `error` / `maxSteps` / `maxBudget` / `canceled` / **`interrupt`**(HITL 挂起,见 §4)。
- run 流 durable:断线用 `runs.subscribe{runId}` + `Last-Event-Id` 续传。

---

## 4. HITL(R 模型:park → resume)

工具审批 / plan-review / ask_user 触发时,run 以 `run.finished{outcome:"interrupt", interrupts:[...]}` 收尾并**挂起**。客户端经 **续连 run** 应答:

```bash
curl -sN -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":"4","method":"runs.resume","params":{
        "parentRunId":"<挂起的 runId>",
        "responses":[{"itemId":"<interrupt 的 itemId>","response":{
          "type":"approval","decision":"approve",
          "remember":{"scope":"session"},                 // 可选:本会话记住该工具的决策(approve/deny 均可)
          "editedArgs":{"command":"ls -la"}                // 可选:一次性改写工具入参
        }}]
      }}' \
  $BASE/v2/rpc/runs.resume
```

- 续连 run 用新 `runId`,`RunRef.parentRunId` 串回。
- 应答类型:`approval`(`approve`\|`deny`,+ 可选 `remember{scope:"session"}` / `editedArgs`)、`answer`(plan/ask_user,`answers` map)、`toolResult`(client 工具,门控 `clientTools`,当前 off)。
- `remember`:KEY=工具名;`deny+remember` 合法;v1 `scope` 仅 `session`(内存)。
- **拒绝 ≠ 取消**:`deny` 让 run 继续(agent 换方案);取消用 `runs.cancel` 硬停。
- 客户端须在 `runtime.initialize` 的 `clientCapabilities.interruptTypes` 声明能处理的类型,否则后端不产该类 interrupt(防挂死)。

---

## 5. Workspace 事件(`workspace.subscribe`,流式)

打开一条非-run 工作区事件流(生命周期=订阅)。全局事件(`mcp.serverChanged` / `skills.changed`)发给每个订阅;带 `watches` 时**监视该 cwd 的 git 状态**。

```bash
curl -sN -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":"5","method":"workspace.subscribe","params":{
        "watches":[{"watchId":"main","cwd":"/abs/project"}]
      }}' \
  $BASE/v2/rpc/workspace.subscribe
```

`WorkspaceEvent`(params 于 `notifications.workspace.event`):

```ts
type WorkspaceEvent =
  | { type: "resync" }                                                   // 该 cwd git 状态变 / 兜底 → 重拉 getDiff
  | { type: "files.changed"; paths: string[]; cwd?: string }            // agent write/edit 工具的精确改动
  | { type: "mcp.serverChanged"; server; status?; toolCount?; error? }  // MCP 状态
  | { type: "skills.changed" }
```

### watch 模型(关键 —— 不是递归文件监听)

后端**不递归监视工作树**(macOS Go fsnotify=kqueue 每文件一 fd,大树耗尽;FSEvents 需 cgo 走不了)。两路覆盖,**跨平台**(inotify/kqueue/Win32 同样廉价):

1. **`resync`** —— 带 `watches` 时只监视该 cwd 的 `.git` 信号集(HEAD/index/refs/heads/ORIG_HEAD/MERGE_HEAD)。git 状态一变(commit/暂存/checkout/branch/merge,任何进程)发去抖 `resync` → 客户端重拉 `getDiff`/`listFileChanges`。非仓 cwd → 该 watch 静默无效。
2. **`files.changed{cwd, paths}`** —— **agent 自己的编辑**(write/edit 工具)由运行时从 run 流**精确推送**(路径相对 `cwd`)。无需 watch、零 fd。`bash` 不推(参数无法判定;若是 git 操作走 `.git` 监视);纯外部进程编辑不实时(降级到下次 git 操作/手动刷新)。

### MCP 生命周期(§2.5 配套)

- `workspace.mcp.listServers` 真实 5 态:`connecting`\|`connected`\|`disconnected`\|`failed`\|`needsAuth`。启动容错:单 server 连不上以 `failed`+`error` 出现,不炸 runtime。`toolCount` 内联。`needsAuth` v1 不产出。
- `workspace.mcp.reconnect{server}` 无同步返回;经 `mcp.serverChanged` 投递,**保证顺序 `connecting → (connected|failed)`**;连上热刷新工具集;`status` 省略=条目已不存在。**需先开着 `workspace.subscribe` 才收得到。**

---

## 6. 错误码速查(按 `type` 分支)

`session_not_found` · `run_not_found` · `item_not_found` · `cwd_unavailable` · `capability_not_negotiated` · `run_already_finished` · `checkpoint_unavailable` · `tool_denied` · `path_outside_root` · `interrupt_not_open` · `idempotency_conflict` · `invalid_protocol_version` · `vcs_unavailable`(有 git 但非仓) · `session_busy`(run 在跑时 rollback) · `invalid_params` · `provider_error`(provider 请求失败/超时,可重试)。

> `provider_error` 含网络/DNS/超时类失败(如 LLM 端点拨号失败);可重试。

---

## 7. 当前明确未做(能力位 off,非遗留)

`multimodal`(图片输入)· `attachments`(附件上传)· `sessionExport` · `clientTools`(toolResult interrupt)· `subagents`(能力位;`task` 委派本身已跑,只是不单列 subagent run 能力)· `checkpoints` 的 v2 文件快照(`restoreType` —— 影子 git)· MCP `needsAuth`。这些方法返 `capability_not_negotiated` 或能力位为 false,接入前按 `initialize` 协商即可,不影响其余能力。
