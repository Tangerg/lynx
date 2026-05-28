# 后端视角 — frontend/src/rpc 实现 review

> 后端（lyra/pkg/{coreapi,transport,rpcadapter,coreimpl}）实现完一版
> 后，对照 frontend/src/rpc/ 逐项核对。本文是后端的工程视角反馈：
> 哪些已对得上、哪些需要前端调整、哪些需要后端调整、协议 spec 哪几行
> 需要重写。
>
> 范围：本文不重复 API.md / TRANSPORT.md 的协议规则，只列实现层
> 跟规则的偏差以及决策。读者：前后端任何一边的实现者，以及未来要
> 看"为什么这么定"的人。
>
> **修订记录**：
>
> - **v4 (2026-05-28 13:00)** — 跟 `PROTOCOL_ALIGNMENT_2026-05-28.md`
>   v4 (Greenfield cuts) 同步。前端在用户"全新项目无历史包袱"指令下
>   砍掉了 8 处 compat-shaped 设计：单 URL 形态、`streamHandle` 塌缩到
>   `runId`、`runs.cancel` 改正经 Request、`MCPServer` 极简化、id 锁
>   number、`JsonSchema` 类型化、`MethodRunsCancel` 改实装、`POST /v1/rpc`
>   分级删除。**全部接受** —— 8 处都是改进而非倒退。
>   后端实现需要补 5 项 v4 改动（§3.3 - §3.7），原 §10 的 P2 #1 + #2
>   被 v4 cuts 吸收，剩 2 项。
> - **v3 (2026-05-28 11:45)** — 同步 v3 的 prod-readiness 标准 + smoke test
>   计划。状态推到 `[ALIGNED]`，等三边动手。
> - **v2 (2026-05-28 10:50)** — 撤回 v1 §2 "统一 `Listing<T>`" 提议；
>   接受前端把 `MCPServer.id` 整个砍掉；补 §9 P1 6 项 + §10 P2 4 项。
> - **v1 (2026-05-28 10:20)** — 初版。

---

## 0. 一句话总结

**架构骨架两边对得上**（JSON-RPC envelope、Transport 接口、HTTP transport
shape、`runtime.initialize` 握手、`notifications/run/event` 流式、sidecar、
错误码、HTTP status 映射）。

**wire-level 形状有 ~15 处 v3 不一致 + v4 新增 5 处 greenfield cuts**：
- 绝大部分跟前端走（95%）
- 后端 1 处反向 push back 已被前端接受（`sessions.fork` 用 `parentId`）
- 前端 1 处比我提议的更进一步（`MCPServer` 砍 `id`，v4 又砍了 `icon` + `displayName`）

**协议层 6 项 spec 要求但骨架暂缺**（见 §9）：
- **prod-blocking** 2 项（HTTP 401/500/503 + observability）必须 staging 之前补完
- **month-paced** 4 项（版本协商 + capability filter + health probe + cursor 分页）月内做完

---

## 1. 已经对得上、不动的部分

记在档以避免后续重复 review：

| 关注点 | 状态 |
|---|---|
| JSON-RPC 2.0 envelope（Request/Response/Notification 三态） | ✓ |
| 17 个 error code 数值 + canonical message | ✓ |
| HTTP status 映射 200/204/400/404/409 走 envelope，401/500/503 走扁平 JSON | ✓ |
| `POST /v1/rpc/{method}` URL 形态 + URL/body method cross-check 返 409+-32011 | ✓（v4 砍掉无后缀备用路径，见 §3.3） |
| `GET /v1/rpc/stream` SSE + `Last-Event-Id` 30s 续传 + 15s 心跳 | ✓ |
| Sidecar `/v1/info` + `/v1/health` 扁平 JSON、无 envelope、无鉴权 | ✓ |
| `notifications/run/event` 形状（v4 后字段 = `{runId, eventId, event}`） | ✓（v4 砍 `streamHandle`，见 §3.4） |
| `notifications/cancelled` 的 `requestId` = 在飞 JSON-RPC Request id（v4 跟 `runs.cancel` 语义分开） | ✓（见 §3.5） |
| 协议版本字符串 `"2026-05-28"` | ✓ |
| 前端在 POST 上也带 `Last-Event-Id` header（后端忽略，无害） | ✓ |
| `runtime.initialize` 强制握手 gate（业务方法前调返 -32011） | ✓ |
| AG-UI event 通过 `ToJSON()` 序列化进 `notifications/run/event.params.event` | ✓ |

---

## 2. List 形状 —— 分页方法 `Page<T>`，非分页方法返裸数组

### 2.1 决议（跟前端走）

| 方法 | 返回 | Pagination 行为 |
|---|---|---|
| `sessions.list` | `Page<Session>` | cursor accepted，hasMore 有意义 |
| `messages.list` | `Page<Message>` | cursor accepted，hasMore 有意义 |
| `tools.list` / `providers.list` / `models.list` | `Tool[]` / `Provider[]` / `Model[]` | 无分页 |
| `workspace.filesChanged` / `workspace.projects` / `workspace.mcp.list` / `workspace.skills` | `T[]` 裸数组 | 无分页 |
| `background.list` | `BackgroundTask[]` | 无分页 |
| `workspace.grep` | `GrepResult{matches, total}` 单对象 | 无分页（已带 total，可加 `nextCursor?` 而 additive） |

### 2.2 后端要做的事

`pkg/rpcadapter/dispatch.go` 把 8 处非分页 list 方法**去掉**
`{"items": ...}` 包装，return 裸数组：

```go
// 改前
return responseResult(msg.ID, map[string]any{"items": tools})
// 改后
return responseResult(msg.ID, tools)
```

`Page[T]` 名字保留（用于 `sessions.list` / `messages.list`），不改名。

---

## 3. P0 — 不改就跑不起来

### 3.1 `ApprovalDecision` 取值不一致

- **前端 wire**：`"approve" | "deny"`（2 值）
- **后端 wire（错）**：`"allow_once" | "allow_always" | "deny"`（3 值，泄漏了内部 enum）

**决策**：跟前端走 2 值。后端把 wire `"approve"` 内部映射到
`DecisionAllowOnce`。"记住选择"等更细策略未来由
`runs.approval.policy.set` 表达。

**改动**：后端 `pkg/coreimpl/runs.go` 的 `parseDecision` 改成只识别
`"approve"` / `"deny"` 两个值。

### 3.2 `pkg/rpcadapter/dispatch.go` 里 `json:",inline"` 标签 bug

`json:",inline"` 不是 encoding/json 的合法 tag（那是 yaml 的语义）。
涉及 `sessions.update`、`messages.list`、`workspace.grep` 三处。

**改动**：定义扁平 `*In` 结构体，删 inline tag hack。

### 3.3 [v4] HTTP transport 砍掉 `POST /v1/rpc`（无 method 后缀）

**前端 v4 决议**：单一 URL 形态 `POST /v1/rpc/{method}`，无后缀的
请求**服务端返 404**。

**后端现状**：`pkg/transport/http/server.go` 同时注册了
`POST /v1/rpc`（`handleRPC`）和 `POST /v1/rpc/{method}`
（`handleRPCWithMethod`）。

**改动**：
- `server.go` 删 `mux.HandleFunc("POST /v1/rpc", s.handleRPC)` 注册
- `rpc.go` 删 `handleRPC` 函数 + `serveRPC` 的 `urlMethod` 为空的分支
  （但保留对 `urlMethod` 跟 body method cross-check 的逻辑）
- 测试 `TestInitializeOverRPC` 改用 `POST /v1/rpc/runtime.initialize`
- 加新测试：`POST /v1/rpc` 返 404

### 3.4 [v4] `streamHandle` 字段塌缩到 `runId`

**前端 v4 决议**：流式资源已经有自己的唯一 id（`runId` / `taskId`），
再多一个 `streamHandle` 是无意义间接层。砍掉。

**后端现状**：
- `pkg/coreapi/runs.go` 的 `StartRunOut { RunID, StreamHandle }`
- `pkg/coreapi/runs.go` 的 `RunEvent { StreamHandle, EventID, Event }`
- `pkg/rpcadapter/notifications.go` 的 `EncodeRunEvent(streamHandle, eventID, ev)` /
  `EncodeRunClosed(streamHandle, reason)`
- `pkg/transport/http/rpc.go` 的 `attachStream(ctx, handle string, events)`
- `pkg/transport/http/replay.go` 的 `streamRegistry` 按 `handle` keyed
- `pkg/transport/inprocess/inprocess.go` 的 `pumpStream(ctx, handle, events)`

**改动**：
- `StartRunOut.StreamHandle` 字段**删除**，只剩 `RunID`
- `RunEvent.StreamHandle` → `RunEvent.RunID`
- `EncodeRunEvent` / `EncodeRunClosed` 参数从 `streamHandle string` 改 `runID string`
- `attachStream` / `streamRegistry` / `pumpStream` 全部用 `runID` 作为 key
- 后端 `coreimpl/runs.go` 的 `StartRun` 实现里不再生成 streamHandle，
  只回 `runID` 一个 id

### 3.5 [v4] `runs.cancel` 改正经 Request

**前端 v4 决议**：
- `runs.cancel { runId, reason? }` —— **正经 Request method**，
  停止 long-running run，返 `void`
- `notifications/cancelled { requestId }` —— 取消**在飞 JSON-RPC
  Request**（如慢 `sessions.list`），按 JSON-RPC id 配对

**后端现状**：`pkg/rpcadapter/dispatch.go` 的 `handleNotification` 把
`notifications/cancelled` 反向解释成 runs.cancel：

```go
case NotificationCancelled:
    // ...
    if err := json.Unmarshal(in.RequestID, &runID); err == nil {
        _ = d.api.CancelRun(ctx, runID)  // ← 错误：把 requestId 当成 runId
    }
```

**改动**：
- `dispatch.go` 新增 `MethodRunsCancel` case 在 `dispatchRequest` 里走
  正经 Request 路径，从 params 解 `{runId, reason?}` → `api.CancelRun`
  → 返 void result
- `dispatch.go` 的 `handleNotification` 里 `notifications/cancelled`
  改成**只处理在飞 JSON-RPC Request abort**（记录请求 id，需要时通过
  context.cancel 传播到对应 handler）—— 不再 reinterpret 成 runs.cancel
- 这一改 §10 P2 #1 "删 MethodRunsCancel 常量" 自动转成 "实装"

### 3.6 [v4] `MCPServer` 极简化（再砍 `icon` + `displayName`）

**前端 v3**：`{name, displayName?, desc, tools, status, icon}`
**前端 v4**：`{name, desc, tools, status}` —— `icon` 是 UI hint 不该
进 wire；`displayName` 多余（MCP name 本身已 human-readable）

**改动**：`pkg/coreapi/workspace.go` 的 `MCPServerStatus` 最终形状：

```go
type MCPServerStatus struct {
    Name   string `json:"name"`
    Desc   string `json:"desc"`
    Tools  int    `json:"tools"`
    Status string `json:"status"` // "active" | "idle" | "error"
}
```

### 3.7 [v4] JSON-RPC `id` 锁 `number` + `ToolSpec.Parameters` 锁 `JsonSchema`

**前端 v4**：
- JSON-RPC `id` 收紧到整数（不接 string）
- `ToolSpec.parameters` 显式 `JsonSchema = Record<string, unknown>` 类型

**后端现状**：
- `Message.ID` 是 `json.RawMessage` —— 接 number 也接 string，没校验
- `ToolSpec.Parameters` 是 `json.RawMessage` —— 没强制是 object

**改动**：
- `pkg/rpcadapter/dispatch.go` 加 id 类型校验：非 number 返
  `-32600 invalid_request`（用 `json.Number` 或试解 `int64`）
- `pkg/coreapi/runs.go` 的 `ToolSpec.Parameters` 类型 doc-comment 改成
  "JSON Schema object (Record<string, unknown>)"，wire 层校验是 object
  而非 array / primitive（codegen 时这个类型保证可加上 schema 校验）

---

## 4. P1 — 现在 stub 不暴露但语义错的

### 4.1 Param key 名字对不上

| Method | 前端送 | 后端期望 | 决策 |
|---|---|---|---|
| `sessions.fork` | `{id, atMessageId}` | `{parentId, atMessageId}` | **前端改成 `parentId`**（见 §5.1）|
| `messages.edit` | `{sessionId, messageId, content}` | `{..., newContent}` | 后端跟前端，wire 用 `content` |
| `workspace.mcp.reconnect` | `{name}` | `{name}` | ✓ 双方都用 `name`（v4 已对齐） |
| `background.stop` | `{taskId}` | `{id}`（走 decodeIDParam） | 后端跟前端，wire 用 `taskId` |

### 4.2 多个 method 返回 shape 不一致

| Method | 前端 | 后端 | 决策 |
|---|---|---|---|
| `workspace.grep` | `GrepResult{matches, total}` 单对象 | `Page<GrepResult>` 分页 | **后端跟前端**：返单对象 + total |
| `workspace.diff` | `DiffRow[]` 结构化 | `{diff: string}` 裸字符串 | **后端跟前端**：服务端解析一次 |
| `workspace.fileHead` | `FileLine[]` 结构化 | `{content: string}` 裸字符串 | **后端跟前端**：同上 |
| `sessions.export` | `{url: string}` | `{content, contentType}` | **后端跟前端**：URL + file-serving endpoint |

### 4.3 `ContextItem` 形状（discriminated union 扁平字段）

```go
type ContextItem struct {
    Kind         string `json:"kind"`
    Path         string `json:"path,omitempty"`
    URL          string `json:"url,omitempty"`
    Range        []int  `json:"range,omitempty"`
    AttachmentID string `json:"attachmentId,omitempty"`
}
```

Go 没有原生 union，扁平 + omitempty 是惯例。

### 4.4 `Session.Metadata` 类型窄了

- **前端**：`Record<string, unknown>`
- **后端（错）**：`map[string]string`

**改动**：`map[string]any`，coreimpl 边界做 `any → string` 转换。同步
改 `CreateSessionIn.Metadata`、`UpdateSessionIn.Metadata`。

---

## 5. 反向 push back 和被前端进一步改进的两处

### 5.1 `sessions.fork` 用 `parentId` —— 后端坚持，前端接受 ✓

```ts
methods.sessions.fork(parentId, atMessageId)
// 一眼明白：parentId 是被 fork 的源 session
```

跟 git fork / GraphQL Connection 的 `parent` 同类命名习惯。

### 5.2 `MCPServer` 极简化 —— 前端两次推得更远

| 版本 | shape |
|---|---|
| v1（mock 时代） | `{id, name, desc, tools, status, icon}` |
| v2（前端砍 id，加 displayName） | `{name, displayName?, desc, tools, status, icon}` |
| v3（后端接受 v2） | 同上 |
| **v4（前端砍 icon + displayName）** | `{name, desc, tools, status}` |

理由（v4）：
- `icon` —— UI presentation 不进 wire，客户端按 name 自己映射
- `displayName` —— MCP server name 本身就 human-readable（`filesystem`、
  `github`、`browser`），多余

`workspace.mcp.reconnect` wire key 用 `name`。

---

## 6. P2 — 类型 shape 对齐（今天 stub，将来要对）

所有这些后端方法今天都返 `ErrNotImplemented`，shape 错了不影响今天的
互通，但 codegen 出来给将来实现的人会踩坑。**全部跟前端走**：

| 类型 | 目标 shape | 后端当前（改） |
|---|---|---|
| `FileChange` | `{path, change: "add"\|"mod"\|"del", added, removed}` | `{Path, Status, Insertions, Deletions}` |
| `DiffRow` | discriminated union `{type: "hunk"\|"ctx"\|"add"\|"del"}` | 无 |
| `FileLine` | `{ln, code, muted?}` | 无 |
| `GrepResult` | `{matches: GrepMatch[], total}` | `{Path, Line, Text}` 单 match |
| `GrepMatch` | `{path, match}` | — |
| `TermLine` | `{kind: TermLineKind, text}` | `{RunID, Line}` |
| `MCPServer` | **`{name, desc, tools, status}`**（v4 砍了 icon + displayName） | `{Name, Connected, Tools, Error}` |
| `Provider` | `{id, type, baseUrl?, hasApiKey}` | `{ID, Name, Kind, Configured}` |
| `Model` | `{id, provider, contextWindow?, description?}` | `{ID, ProviderID, Name, Contextual}` |
| `BackgroundTask` | `{taskId, label, status, startedAt, progress?}` | `{ID, Label, State, Progress}` |
| `BackgroundUpdate` | `{taskId, status, progress?, outputDelta?}` | `{TaskID, State, Progress, ExitCode, Message}` |
| `Project` | `{id, name, branch, active?}` | `{ID, Name, Root}` |
| `Skill` | `{id, name, description}` | `{Name, Description}`（缺 id） |
| `ToolSpec` | `{name, description?, parameters: JsonSchema, origin}` | 后端多带 `safetyClass`（前端忽略，保留） |

后端 `Tool.SafetyClass` 字段保留 —— 前端 TS 忽略未知字段无害。

---

## 7. spec 已 v4 patch 完毕

下面所有的 spec 改动**前端 v4 已经全部落地**到 `docs/API.md` +
`docs/TRANSPORT.md`，后端只需要按照新 spec 改实现：

| 文件 | v4 patch |
|---|---|
| `docs/API.md` §1.1 | `id: number`（不再 union） |
| `docs/API.md` §3.1 / §3.3 | notification params 字段从 `streamHandle` 改 `runId` / `taskId` |
| `docs/API.md` §4.2 | `ApprovalDecision = "approve" \| "deny"` |
| `docs/API.md` §5.2 | 加 `Returns` 列；`runs.cancel` 改 Request；`sessions.fork` 用 `parentId`；`mcp.reconnect` 用 `name`；`messages.edit` 用 `content`；`background.stop` 用 `taskId` |
| `docs/API.md` §6.2 | `ContextItem` discriminated union；`ToolSpec.parameters: JsonSchema` |
| `docs/API.md` §6.3 | `StartRunResult` 删 `streamHandle`，只剩 `runId` |
| `docs/API.md` §6.5 | `MCPServer` 砍 `icon` + `displayName`，最终 `{name, desc, tools, status}` |
| `docs/API.md` §10.1 | HTTP routing 单一形态 `POST /v1/rpc/{method}` |
| `docs/TRANSPORT.md` §4.3 | 同 §10.1 |

后端实现按新 spec 走就行，spec 这边没有遗留工作。

---

## 8. 落地动作清单（v4 后的最终态）

### 8.1 后端做（按文件）

| 文件 | 改动 |
|---|---|
| `pkg/rpcadapter/dispatch.go` | (1) 8 处非分页 list 去掉 `{items}` 包装；(2) 删 `json:",inline"` hack；(3) `background.stop` 用 `taskId`；(4) `ApprovalIn.Decision` 接 approve/deny；(5) **v4: 加 `MethodRunsCancel` Request 路径** + `notifications/cancelled` 只做 in-flight Request abort；(6) **v4: JSON-RPC id 校验是 number** |
| `pkg/rpcadapter/method_names.go` | **v4: `MethodRunsCancel` 不删**（变成实装路由） |
| `pkg/rpcadapter/notifications.go` | **v4: `EncodeRunEvent` / `EncodeRunClosed` 用 `runID` 不用 `streamHandle`** |
| `pkg/coreimpl/runs.go` | (1) `parseDecision` 改 approve→AllowOnce / deny→Deny；(2) **v4: `StartRun` 返回值只填 runId**；(3) `CancelRun` 接口签名已经是 `(ctx, runID)` 不动 |
| `pkg/coreapi/runs.go` | (1) `ContextItem` discriminated union；(2) **v4: `StartRunOut` 删 `StreamHandle`**；(3) **v4: `RunEvent.StreamHandle` → `RunID`**；(4) **v4: `ToolSpec.Parameters` 注释强调 JsonSchema** |
| `pkg/coreapi/sessions.go` | Metadata `map[string]any`；UpdateSessionIn 带 ID 扁平；ExportSession 返 URL |
| `pkg/coreapi/messages.go` | `EditMessageIn.NewContent` → `Content`；MessagesListIn 扁平 |
| `pkg/coreapi/workspace.go` | 类型对齐 §6；WorkspaceGrep 返 `*GrepResult`；WorkspaceDiff 返 `[]DiffRow`；WorkspaceFileHead 返 `[]FileLine`；**v4: `MCPServerStatus` 最终 `{name, desc, tools, status}`** |
| `pkg/coreapi/providers.go` | Provider/Model/Tool 类型对齐 §6 |
| `pkg/coreapi/background.go` | BackgroundTask/Update 类型对齐 §6 |
| `pkg/coreimpl/sessions.go` | UpdateSession 签名调整；ExportSession 实现 file-serving 返 URL |
| `pkg/coreimpl/workspace.go` | 签名跟新 types 对齐 |
| `pkg/coreimpl/providers.go` | Tool shape 跟新版对齐（保留 safetyClass） |
| `pkg/transport/http/server.go` | **v4: 删 `POST /v1/rpc` 路由**（无后缀），保留 `POST /v1/rpc/{method}` |
| `pkg/transport/http/rpc.go` | **v4: 删 `handleRPC` 函数 + `urlMethod == ""` 分支**；**v4: `attachStream` 用 runId 当 key**；**v4: 用 streamRegistry 时 key 改 runId** |
| `pkg/transport/http/replay.go` | **v4: `streamRegistry` keyed by runId 不是 streamHandle**（重命名 + 注释更新） |
| `pkg/transport/http/server_test.go` | **v4: `TestInitializeOverRPC` 改 `POST /v1/rpc/runtime.initialize`**；加新测试 "无 method 后缀返 404" |
| `pkg/transport/inprocess/inprocess.go` | **v4: `pumpStream` 用 runId 当 key**；EncodeRunEvent 参数改对应 |

约 **15 个文件** 改动（v3 的 11 + v4 新增 4），~250 行 diff。

### 8.2 前端做

前端 v4 已经完成（PROTOCOL_ALIGNMENT_2026-05-28.md `[CLOSED]`）：
- `frontend/src/rpc/{types,shapes,methods,events}.ts`
- `frontend/src/rpc/transports/http.ts`
- 对应测试

### 8.3 spec 做

前端 v4 已经全部 patch 到 `docs/API.md` + `docs/TRANSPORT.md`，
后端按新 spec 改实现即可。

---

## 9. P1 协议层缺口 —— 按 prod-readiness 拆两组

### 9.1 Prod-blocking — staging / prod 部署前必须完成

#### 9.1.1 HTTP transport 层错误 401/500/503 扁平 JSON

**为什么 prod-blocking**：没 401 token 校验 = 任何同机进程能调 Runtime
（本地进程门禁形同虚设）。

**补法**：
- token 校验 middleware（读 `~/.lyra/local-token` 或配置注入），失败
  返 `401 + {"error":"missing_local_token"}`
- 错误路径统一走 `writeTransportError`
- `/v1/health` 接真实 probe，failed → 503（依赖 §9.2.3）

#### 9.1.2 Observability §10

**为什么 prod-blocking**：没 observability = oncall 半夜没办法看
metric / log，故障定位时间从分钟变小时。

**补法**：
- middleware 加 structured logger（json output）+ Prometheus collector
- `echoTraceID` 改 `ensureTraceID`（缺失则生成 UUID v7）

### 9.2 Month-paced — dev happy path 不阻塞，月内做完

#### 9.2.1 版本协商
维护 `supportedVersions []string`，不支持时返 -32010。

#### 9.2.2 Capability 协商
`Dispatcher` 在 `Initialize` 成功后存 `clientCapabilities`，emit 前过滤
未声明事件，未协商能力调用返 -32009。

#### 9.2.3 `/v1/health` 真实 probe
chat client 可达 / storage 可写 / MCP session 健康聚合。§9.1.1 的 503
路径依赖这个 —— 落 §9.1.1 时可以先返最小可用 probe，本节后续完整化。

#### 9.2.4 Cursor 分页
session/message store 增加 cursor-based scan 接口。

---

## 10. P2 代码质量 —— 跑通后清理（v4 后剩 2 项）

v4 cuts 把原 P2 #1 + #2 都吸收了：

- ~~`MethodRunsCancel` 删除~~ —— v4 §3.5 改成"实装"，不再 dead code
- ~~SSE 改 per-streamHandle 路由~~ —— v4 §3.4 砍了 streamHandle，
  天然按 runId 路由

剩下两项：

1. **`pkg/transport/http/replay.go`** —— `compareEventID` 换 `strconv.Atoi`
2. **`pkg/coreimpl/impl.go`** —— `genID()` 换 UUID v7

---

## 11. 跟 PROTOCOL_ALIGNMENT_2026-05-28.md 的关系

那份文档是前端做的对应方决议，跟本文 v4 完全同步：

- §1 `sessions.fork` 用 `parentId` ✓
- §2 `MCPServer` 砍 `id`、`workspace.mcp.reconnect` 用 `name` ✓
- v4 Greenfield cuts 8 项 ✓
- P1 拆 prod-blocking 和 month-paced ✓
- P2 4 项 → v4 后剩 2 项 ✓
- Smoke test 计划（v3）✓ — 见 §14

前后端无悬而未决项，全部进入执行态。

---

## 12. 我没做但建议的事

下面是 review 过程里冒出来、但不在本轮范围内的几个 follow-up：

1. **schema codegen** —— 一旦 `pkg/coreapi` 通过 codegen 产出
   OpenAPI/AsyncAPI schema 再生成 TS 类型，类型不一致会变编译期错误
2. **`workspace.terminal.subscribe` 多 subscriber** —— API.md §12 已列
3. **`models.list` 缓存策略** —— 同上
4. **二进制上传** —— `attachments.createUploadUrl` 在 InProcess transport
   上的映射，API.md §12 已列
5. **`feedback.submit`** —— 后端目前 accept-and-drop，等存储层做好

---

## 13. 期望节奏（带 prod-readiness 标准）

| 阶段 | 范围 | 时间盒 |
|---|---|---|
| **本轮**（本周） | §8.1 那 15 个后端文件落地，跑 §14 smoke test 通过 | 5 天 |
| **prod-blocking**（staging 部署前必补） | §9.1（HTTP 401/500/503 + Observability） | 不限时但 **gate-on-staging** |
| **month-paced** | §9.2（版本协商 + capability filter + health probe + cursor 分页）+ §10 P2 2 项 + schema codegen 起步 | 月内 |
| **再下一轮**（quarter 内） | P2 那批 stub 真做，类型 shape 已经对齐 | 季度内 |

**prod-readiness gate**：

> §9.1 两项不一定要在 dev 周期内完成，但**任何 staging/prod 部署前必须
> 完成**。没 401 token 校验 = 任何同机进程能调 Runtime；没 observability
> = oncall 半夜没办法看 metric / log。

---

## 14. Smoke test 计划（本周 milestone，v4 后更新）

§8 三边改动全部落地后，跑这套端到端验证共识真的对齐了：

```
1. 前端 createHttpTransport({baseUrl: "http://127.0.0.1:8080"})
2. await methods.runtime.initialize({
     protocolVersion: "2026-05-28",
     clientInfo: {name: "smoke-test", version: "0.1"},
     capabilities: {events: {...}, features: {...}}
   })
3. const session = await methods.sessions.create({title: "smoke"})
4. const {result, events} = await methods.runs.start({
     sessionId: session.id,
     messages: [{role: "user", content: "hi"}]
   })
   // result 只含 { runId } — v4 已砍 streamHandle
5. for await (const ev of events) { /* AG-UI event render */ }
   // 期望：RUN_STARTED → STEP_STARTED → TEXT_MESSAGE_* → STEP_FINISHED → RUN_FINISHED
6. (如果 run 暂停在 lyra.approval) →
   await methods.runs.approval.submit({requestId, decision: "approve"})
7. (run 继续直到结束) → notifications/run/closed { runId }
8. [v4 校验] 直接 POST 到 /v1/rpc（无 method 后缀），期望 404
9. [v4 校验] 调用 methods.runs.cancel({runId: "..."}) 走 Request 路径
   不是 notification，期望返 void 而非 204
```

**通过标准**：
- 步骤 2 返 `serverInfo + capabilities`
- 步骤 3 返 `Session` 单对象（无 `{items}` 包装）
- 步骤 4 返 `{runId}` 单字段（**无 `streamHandle`**），立即返回不等流结束
- 步骤 5 收到的 `notifications/run/event.params` 形状 `{runId, eventId, event}`
  （**无 streamHandle**）
- 步骤 6 用 `decision: "approve"` 不报 -32602
- 步骤 7 收到 `notifications/run/closed { runId }` 后 events iterator 自然结束
- 步骤 8 `POST /v1/rpc` 返 `404 + {error: ...}`（greenfield 无 fallback）
- 步骤 9 `runs.cancel` 返 200 + JSON-RPC envelope（`{jsonrpc, id, result: null}`）

跑通 = 本轮共识落地完成。

---

## 15. 状态 — `[ALIGNED to v4]`

前后端 spec / 前端实现 / 文档**全部对齐到 v4**。等后端动手 §8.1
那 15 个文件 + §14 smoke test 通过。

**下次需要再开 PROTOCOL_ALIGNMENT / 本文档**的触发条件：

- 后端在落地过程中发现新的 shape 歧义
- §14 smoke test 失败暴露未覆盖的 case
- §9.1 / §9.2 推进中遇到 spec 没写清楚的细节
- 引入新方法 / 新事件类型时，wire shape 需要先 align

如果都没触发，下一次 review 在 schema codegen 起步时。
