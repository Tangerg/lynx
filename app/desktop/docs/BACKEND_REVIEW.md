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
> - **v3 (2026-05-28 11:45)** — 跟 `PROTOCOL_ALIGNMENT_2026-05-28.md`
>   v3 同步收口。前端 v3 确认了我 v2 的所有决议（List 形状 / MCPServer
>   砍 id / P1 fix path / P2 cleanup），并补了两件事：(a) **prod-readiness
>   标准**：把 P1 6 项拆成"prod-blocking"（#3 401/500/503 + #4
>   observability）和"month-paced"（#1 版本协商 + #2 capability +
>   #5 health probe + #6 cursor 分页）—— 接受，更新 §13；(b) **smoke
>   test 计划**：7 步端到端走通的硬标准 —— 新增 §14。状态推到 `[ALIGNED]`，
>   等三边动手。
> - **v2 (2026-05-28 10:50)** — 撤回 v1 §2 "统一 `Listing<T>`" 提议
>   （前端两次表达偏好"非分页返裸数组"，理由可接受、不强推）；接受前端
>   §5.2 的扩展（`MCPServer` 砍 `id` 字段，加可选 `displayName`）；补
>   §9（P1 协议层 6 项缺口）+ §10（P2 代码质量 4 项）。
> - **v1 (2026-05-28 10:20)** — 初版。

---

## 0. 一句话总结

**架构骨架两边对得上**（JSON-RPC envelope、Transport 接口、HTTP transport
shape、`runtime.initialize` 握手、`notifications/run/event` 流式、sidecar、
错误码、HTTP status 映射）。

**wire-level 形状有 ~15 处不一致**，绝大部分跟前端走（95%），后端 1 处
反向 push back（`sessions.fork` 参数名）。前端 1 处比我提议的更进一步
（`MCPServer` 砍 `id`）。

**协议层 6 项 spec 要求但骨架暂缺**（见 §9）：
- **prod-blocking** 2 项（HTTP 401/500/503 + observability）必须 staging
  之前补完
- **month-paced** 4 项（版本协商 + capability filter + health probe +
  cursor 分页）一个月节奏，dev happy path 不阻塞

---

## 1. 已经对得上、不动的部分

记在档以避免后续重复 review：

| 关注点 | 状态 |
|---|---|
| JSON-RPC 2.0 envelope（Request/Response/Notification 三态） | ✓ |
| 17 个 error code 数值 + canonical message | ✓ |
| HTTP status 映射 200/204/400/404/409 走 envelope，401/500/503 走扁平 JSON | ✓ |
| `POST /v1/rpc[/{method}]` 两种 URL 形态 + URL/body method cross-check 返 409+-32011 | ✓ |
| `GET /v1/rpc/stream` SSE + `Last-Event-Id` 30s 续传 + 15s 心跳 | ✓ |
| Sidecar `/v1/info` + `/v1/health` 扁平 JSON、无 envelope、无鉴权 | ✓ |
| `notifications/run/event` 形状 `{streamHandle, eventId, event}` | ✓ |
| `notifications/cancelled` 的 `requestId` 当 runId 用 | ✓ |
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

### 2.2 v1 撤回的"统一 `Listing<T>`"提议

v1 §2 论证过"统一 `Listing<T>` wrapper"方案（MCP 先例、5 年契约稳定性、
协议层一致性），但前端 PROTOCOL_ALIGNMENT 把"非分页返裸数组"放进 "95%
跟前端" 表里 —— 前端**两次**明确这是设计偏好。

撤回的核心理由（前端 v3 §共识收口 #1 已经把这段论据 captured）：
- 拒绝"今天不分页 → 明天可能分页"的预投资，因为从 `T[]` 升级到
  `Page<T>` 在 wire 上**本来就是 breaking change**（client 代码从
  `data` 改 `data.items`），不存在"零破坏升级"路径
- 真正零破坏的升级只在**已经是 `Page<T>` 形状里追加 cursor 字段**，而
  非"裸数组改 wrapper"
- "5 年焦虑"靠 spec §5.2 的 `Returns` 列在文档层显式标注就够，类型签名
  层不必预投资

### 2.3 后端要做的事

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

**决策**：跟前端走 2 值。"记住选择" 不该编码在每次决策的 wire 里 —— 那是
per-session/per-tool 的策略问题，未来用单独的 `runs.approval.policy.set`
方法表达。后端把 wire `"approve"` 内部映射到 `DecisionAllowOnce`。

**改动**：后端 `pkg/coreimpl/runs.go` 的 `parseDecision` 改成只识别
`"approve"` / `"deny"` 两个值。

### 3.2 `pkg/rpcadapter/dispatch.go` 里 `json:",inline"` 标签 bug

后端用了 `json:",inline"` 想 flatten 嵌套字段，但 **encoding/json 不认这个
tag**（那是 yaml 的语义）。涉及 `sessions.update`、`messages.list`、
`workspace.grep` 三处。后端纯 bug，不动协议。

**改动**：后端定义扁平 `*In` 结构体，删 inline tag hack。

---

## 4. P1 — 现在 stub 不暴露但语义错的

### 4.1 Param key 名字对不上

| Method | 前端送 | 后端期望 | 决策 |
|---|---|---|---|
| `sessions.fork` | `{id, atMessageId}` | `{parentId, atMessageId}` | **前端改成 `parentId`**（见 §5.1）|
| `messages.edit` | `{sessionId, messageId, content}` | `{..., newContent}` | 后端跟前端，wire 用 `content` |
| `workspace.mcp.reconnect` | `{id}` | `{name}` | **wire 用 `name`，`MCPServer` 砍 `id`**（见 §5.2）|
| `background.stop` | `{taskId}` | `{id}`（走 decodeIDParam） | 后端跟前端，wire 用 `taskId` |

### 4.2 多个 method 返回 shape 不一致

| Method | 前端 | 后端 | 决策 |
|---|---|---|---|
| `workspace.grep` | `GrepResult{matches, total}` 单对象 | `Page<GrepResult>` 分页 | **后端跟前端**：返单对象 + total 字段已经够用，未来加 `nextCursor?` 是 additive 改动 |
| `workspace.diff` | `DiffRow[]` 结构化 | `{diff: string}` 裸字符串 | **后端跟前端**：服务端解析一次比客户端每次 regex 划算 |
| `workspace.fileHead` | `FileLine[]` 结构化 | `{content: string}` 裸字符串 | **后端跟前端**：同上 |
| `sessions.export` | `{url: string}` | `{content, contentType}` | **后端跟前端**：返 URL 让浏览器走 download 链路，承担需要单独 file-serving endpoint 的代价 |

### 4.3 `ContextItem` 形状

**前端**：discriminated union 扁平字段

```ts
| { kind: "file"; path: string }
| { kind: "url"; url: string }
| { kind: "selection"; path: string; range: [number, number] }
| { kind: "image"; attachmentId: string }
```

**后端（错）**：`{Kind, Data: map[string]any}` 嵌套

**决策**：后端改用 Go 结构体可选字段表达 tagged union：

```go
type ContextItem struct {
    Kind         string `json:"kind"`
    Path         string `json:"path,omitempty"`
    URL          string `json:"url,omitempty"`
    Range        []int  `json:"range,omitempty"`
    AttachmentID string `json:"attachmentId,omitempty"`
}
```

Go 没有原生 union 类型，但扁平 + omitempty 是惯例做法，跟 wire 对齐。

### 4.4 `Session.Metadata` 类型窄了

- **前端**：`Record<string, unknown>`（任意值类型，符合 JSON 原生表达）
- **后端（错）**：`map[string]string`（强行 string-only）

后端这个是把内部存储限制泄漏到 wire 上。改成 `map[string]any`，coreimpl
边界做 `any → string` 的转换（内部存储不变）。

同步改：`CreateSessionIn.Metadata`、`UpdateSessionIn.Metadata`。

---

## 5. 反向 push back 和被前端进一步改进的两处

### 5.1 `sessions.fork` 用 `parentId` 不是 `id` —— 后端坚持，前端接受

**前端原本**：`fork(id: string, atMessageId: string)` —— wire 发 `{id, atMessageId}`

**问题**：

```ts
methods.sessions.fork(currentSessionId, msgId)
// 读代码的人困惑：这个 id 是"新 session 的 id"还是"被 fork 的 session 的 id"？
```

`id` 在这个上下文里语义有歧义。`parentId` 一眼明白是"被 fork 的源
session"。

跟 git fork / GraphQL Connection 的 `parent` / linked-data 的 `derivedFrom`
同类命名习惯。

**前端 PROTOCOL_ALIGNMENT 已确认**：spec / 前端 wire / 后端 wire 三处同步
改成 `{parentId, atMessageId}`。

### 5.2 `MCPServer.id` —— 前端把后端的 pushback 推得更远

**v1 后端提议**：wire 用 `id`，后端做 `id → name` 别名映射。

**前端反提议（采纳）**：直接**砍掉** `MCPServer.id` 字段，保留 `name`
作为唯一标识符，可选加 `displayName?: string` 做 pretty label。

```ts
// 改前
interface MCPServer {
  id: string;        // ← 没有理由存在
  name: string;
  desc: string;
  tools: number;
  status: "active" | "idle" | "error";
  icon: string;
}

// 改后
interface MCPServer {
  name: string;              // == MCP 协议 name
  displayName?: string;      // optional human-readable label
  desc: string;
  tools: number;
  status: "active" | "idle" | "error";
  icon: string;
}
```

**为什么前端提议更好**：

- `id` 字段在 MCP 协议层从来没有 —— 它是 REST mock 时代的残留
- 砍了之后后端**零适配成本**（内部本来就按 name 查），跟 MCP 原生哲学对齐
- 形状更干净，少一个 "id 和 name 谁是真标识符" 的歧义

`workspace.mcp.reconnect` wire key 同步用 `name`。

---

## 6. P2 — 类型 shape 对齐（今天 stub，将来要对）

所有这些后端方法今天都返 `ErrNotImplemented`，shape 错了不影响今天的
互通，但 codegen 出来给将来实现的人会踩坑。**全部跟前端走**：

| 类型 | 目标 shape | 后端当前（改） |
|---|---|---|
| `FileChange` | `{path, change: "add"\|"mod"\|"del", added, removed}` | `{Path, Status, Insertions, Deletions}` |
| `DiffRow` | discriminated union with `type: "hunk"\|"ctx"\|"add"\|"del"` | 无（后端返裸 string） |
| `FileLine` | `{ln, code, muted?}` | 无（后端返裸 string） |
| `GrepResult` | `{matches: GrepMatch[], total}` | `{Path, Line, Text}` 单 match |
| `GrepMatch` | `{path, match}` | — |
| `TermLine` | `{kind: TermLineKind, text}` | `{RunID, Line}` |
| `MCPServer` | `{name, displayName?, desc, tools, status, icon}`（无 `id`） | `{Name, Connected, Tools, Error}` |
| `Provider` | `{id, type, baseUrl?, hasApiKey}` | `{ID, Name, Kind, Configured}` |
| `Model` | `{id, provider, contextWindow?, description?}` | `{ID, ProviderID, Name, Contextual}` |
| `BackgroundTask` | `{taskId, label, status, startedAt, progress?}` | `{ID, Label, State, Progress}` |
| `BackgroundUpdate` | `{taskId, status, progress?, outputDelta?}` | `{TaskID, State, Progress, ExitCode, Message}` |
| `Project` | `{id, name, branch, active?}` | `{ID, Name, Root}` |
| `Skill` | `{id, name, description}` | `{Name, Description}`（缺 id） |
| `Tool` (from `tools.list`) | `{name, description?, parameters, origin}` | 后端多带 `safetyClass`（前端忽略，保留） |

后端 `Tool.SafetyClass` 字段保留 —— 前端 TS 忽略未知字段无害，将来真做权限
分级会用到。

---

## 7. spec 需要更新的几处

### 7.1 §5.2 加 `Returns` 列

避免后续重复争论"这个 method 到底分不分页"，每个 method 显式标注返回类型：

```
| Method             | Returns         |
|--------------------|-----------------|
| sessions.get       | Session         |
| sessions.list      | Page<Session>   |
| sessions.fork      | Session         |
| tools.list         | Tool[]          |
| providers.list     | Provider[]      |
| workspace.grep     | GrepResult      |
| workspace.diff     | DiffRow[]       |
| workspace.fileHead | FileLine[]      |
| messages.list      | Page<Message>   |
| ...                | ...             |
```

让"哪些返裸数组 / 哪些走 `Page<T>` / 哪些返单对象"零歧义。

### 7.2 §4.2 `ApprovalDecision` 取值固化

明文写下 wire 接受的两个值：

> `runs.approval.submit { requestId, decision, reason? }`
> 其中 `decision` 取值范围：`"approve" | "deny"`。"记住选择" 等更细的
> 决策语义不在本协议层 —— 由前端的 UI 选项 + 未来可能的
> `runs.approval.policy.set` 表达。

### 7.3 §6.3 `ContextItem` 明文 discriminated union

```ts
type ContextItem =
  | { kind: "file"; path: string }
  | { kind: "url"; url: string }
  | { kind: "selection"; path: string; range: [number, number] }
  | { kind: "image"; attachmentId: string };
```

明确禁止后续往里塞 `{kind, data: {...}}` 嵌套形式。新 kind 必须扩 union。

### 7.4 §6 `MCPServer` shape 修正

砍 `id` 字段，加可选 `displayName`，见 §5.2。

### 7.5 §5.2 参数表 patch

- `sessions.fork` 第一参数 `parentId`
- `workspace.mcp.reconnect` 参数 `name`
- `messages.edit` 字段 `content`（不是 `newContent`）
- `background.stop` 字段 `taskId`（不是 `id`）

---

## 8. 落地动作清单

### 8.1 后端做（按文件）

| 文件 | 改动 |
|---|---|
| `pkg/rpcadapter/dispatch.go` | 8 处非分页 list 去掉 `{items}` 包装；删 `json:",inline"` hack；`workspace.mcp.reconnect` 用 `name`；`background.stop` 用 `taskId`；`ApprovalIn.Decision` 接 approve/deny |
| `pkg/coreimpl/runs.go` | `parseDecision` 改 approve→AllowOnce / deny→Deny |
| `pkg/coreapi/sessions.go` | Metadata 改 `map[string]any`；`UpdateSessionIn` 带 ID 扁平字段；`ExportSession` 返 `(url string, error)` |
| `pkg/coreapi/messages.go` | `EditMessageIn.NewContent` → `Content`；`MessagesListIn` 扁平字段 |
| `pkg/coreapi/runs.go` | `ContextItem` 改 discriminated union 扁平形态 |
| `pkg/coreapi/workspace.go` | 类型对齐 §6；`WorkspaceGrep` 返 `*GrepResult` 单对象；`WorkspaceDiff` 返 `[]DiffRow`；`WorkspaceFileHead` 返 `[]FileLine`；`MCPServerStatus` 砍 `id` 加 `displayName` |
| `pkg/coreapi/providers.go` | `Provider` / `Model` / `Tool` 类型对齐 §6 |
| `pkg/coreapi/background.go` | `BackgroundTask` / `BackgroundUpdate` 类型对齐 §6 |
| `pkg/coreimpl/sessions.go` | `UpdateSession` 签名调整；`ExportSession` 实现 file-serving endpoint 返 URL |
| `pkg/coreimpl/workspace.go` | 签名跟新 types 对齐 |
| `pkg/coreimpl/providers.go` | `Tool` shape 跟新版对齐（保留 safetyClass） |

约 11 个文件，纯机械改动（不含 §9 / §10 的事），~200 行 diff。

### 8.2 前端做

| 文件 | 改动 |
|---|---|
| `frontend/src/rpc/methods.ts` | `sessions.fork` 第一参数命名 `parentId`，wire 同步；`workspace.mcp.reconnect(name: string)`；callsite 跟着改 |
| `frontend/src/rpc/shapes.ts` | `MCPServer` 形状去 `id` 加可选 `displayName` |

约 30 行 diff。

### 8.3 spec 做

| 文件 | 改动 |
|---|---|
| `docs/API.md` §4.2 | 明文 `ApprovalDecision = "approve" \| "deny"` |
| `docs/API.md` §5.2 | 加 `Returns` 列；patch `sessions.fork` / `mcp.reconnect` / `messages.edit` / `background.stop` 参数表 |
| `docs/API.md` §6.3 | `ContextItem` 明文 discriminated union |
| `docs/API.md` §6 | `MCPServer` shape 修正（砍 id 加 displayName） |

约 50 行 diff。

---

## 9. P1 协议层缺口 —— 按 prod-readiness 拆两组

前端 review 我的骨架找出 6 项 spec 要求但实现暂缺的。**前端 v3 进一步把
这 6 项拆成 "prod-blocking" 和 "month-paced" 两组**，接受这个分级 ——
不是所有 P1 都同样紧急。

### 9.1 Prod-blocking — staging / prod 部署前必须完成

这两项缺位会直接造成生产事故，不是 nice-to-have：

#### 9.1.1 HTTP transport 层错误 401/500/503 扁平 JSON

**现状**：`pkg/transport/http/middleware.go` 只对 panic 路径走扁平 JSON
（status 500）；401 / 503 / 非 panic 的 500 全缺。

**为什么 prod-blocking**：没 401 token 校验 = 任何同机进程能调 Runtime
（本地进程门禁形同虚设）。

**spec 要求** (API.md §7.3)：
- 401 = 本地进程门禁 token 校验失败
- 500 = 非 panic 的 transport 层 internal error
- 503 = `/v1/health` 真实 probe 失败

**补法**：
- 加 token 校验 middleware（读 `~/.lyra/local-token` 路径或配置注入），失败
  返 `401 + {"error":"missing_local_token"}`
- 错误路径统一走 `writeTransportError`
- `/v1/health` 接真实 probe，failed → 503（依赖 §9.2.3）

#### 9.1.2 Observability §10

**现状**：
- 结构化日志只 `>= 500` 才输出且非 JSON
- `X-Lyra-Request-Id` 仅 echo client header，server 缺失时不生成
- Prometheus metric 零实现

**为什么 prod-blocking**：没 observability = oncall 半夜没办法看 metric /
log，故障定位时间从分钟变小时。

**spec 要求** (API.md §10)：
- 每条 RPC 调用一行结构化 JSON log：`ts / method / id / duration_ms / error_code / bytes_in / bytes_out / trace_id`
- `lyra_rpc_request_total{method, error_code}` / `lyra_rpc_duration_seconds{method}` / `lyra_rpc_bytes_in_total{method}` / `lyra_rpc_bytes_out_total{method}`
- `X-Lyra-Request-Id` 缺失时 server 生成 UUID v7

**补法**：
- middleware 加 structured logger（json output）+ Prometheus collector
- `echoTraceID` 改成 `ensureTraceID`（缺失则生成）

### 9.2 Month-paced — dev happy path 不阻塞，月内做完

这四项缺位**短期不影响互通**，但是 spec 写在那里、长期必须落地：

#### 9.2.1 版本协商

**现状**：`pkg/coreimpl/lifecycle.go` 的 `Initialize` 是 no-op pass-through，
无视 `in.ProtocolVersion` 直接返服务端版本。

**spec 要求** (API.md §2.2)：
- Client 报想用的版本
- Server 支持 → 回同一版本
- Server 不支持 → 回 server 支持的最新版本
- Client 拿到 fall-back 不到的版本 → 必须断开

**补法**：维护一个 `supportedVersions []string`，按规则协商，不支持时返
`ErrInvalidProtocolVersion` 走 -32010。

#### 9.2.2 Capability 协商

**现状**：`dispatch.go` 不存 client capabilities，后端 emit 事件时不过滤。

**spec 要求** (API.md §6.1)：Server **必须不发** client 没声明能渲染的事件。

**补法**：`Dispatcher` 在 `Initialize` 成功后存 `clientCapabilities`；
runs.start 的事件 pump 路径在 emit 前过滤 client 未声明的事件类型，未协商
的能力调用返 -32009 `capability_not_negotiated`。

#### 9.2.3 `/v1/health` 真实 probe

**现状**：`pkg/transport/http/sidecar.go` 永远返 `{"status":"ok"}`。

**spec 要求** (API.md §9.2)：
- `status === "ok"` → 200
- `status === "degraded" | "unhealthy"` → 503

**补法**：定义 probe 接口（chat client 可达 / storage 可写 / MCP session 健康），
聚合状态返回。

> 注：§9.1.1 的 503 路径依赖这个真实 probe — 当 health 报 unhealthy 时返 503。
> §9.1.1 落地时这条 probe 可以先返一个最小可用实现（例如只 probe storage），
> §9.2.3 完整落地后再扩展。

#### 9.2.4 Cursor 分页

**现状**：`pkg/coreimpl/sessions.go` 收到 `q.Cursor != ""` 直接返 -32601。

**spec 要求** (API.md §6.4)：游标分页是真分页方法（sessions.list /
messages.list）的标准能力。

**补法**：session/message store 增加 cursor-based scan 接口（按 ID 排序 +
cursor decode 成 last-id）。

---

## 10. P2 代码质量 —— 跑通后清理

前端 review 出的实现层瑕疵，不阻塞协议互通：

1. **`pkg/rpcadapter/method_names.go`** —— `MethodRunsCancel` 常量声明但
   未使用（`runs.cancel` 在 dispatcher 走 notification 路径，不查这个常量）。删。
2. **`pkg/transport/http/clients.go`** —— SSE 广播是全 fan-out，不按
   `streamHandle` 路由。功能正确（前端按 handle 过滤兜底了）但浪费带宽。
   改成 per-streamHandle 订阅列表。
3. **`pkg/transport/http/replay.go`** —— `compareEventID` 手写长度+lex
   比较 hacky。换 `strconv.Atoi` 数值比较（eventId 本来就是 decimal 字符串）。
4. **`pkg/coreimpl/impl.go`** —— `genID()` 用 UUID v4，spec §6.3 期望 v7
   （sortable）。UUID v7 在 google/uuid v1.6+ 支持，换一行。

---

## 11. 跟 PROTOCOL_ALIGNMENT_2026-05-28.md 的关系

那份文档是前端做的对应方决议，跟本文 v3 完全同步。两边一致地认为：

- §1 `sessions.fork` 用 `parentId` ✓
- §2 `MCPServer` 砍 `id`、`workspace.mcp.reconnect` 用 `name` ✓
- 95% 跟前端走的 13 项 ✓
- 后端做 / 前端做 / spec 做 三向分工 ✓
- P1 拆 prod-blocking 和 month-paced（v3 同步）✓
- P2 4 项清理 ✓
- Smoke test 计划（v3 同步）✓ — 见 §14

前后端无悬而未决项，全部进入执行态。

---

## 12. 我没做但建议的事

下面是 review 过程里冒出来、但不在本轮范围内的几个 follow-up：

1. **schema codegen** —— 一旦 `pkg/coreapi` 通过 codegen 产出
   OpenAPI/AsyncAPI schema 再生成 TS 类型，类型不一致会变编译期错误，
   `json:",inline"` 这类手写 bug 自动绝迹。先把手写阶段熬过去。
2. **`workspace.terminal.subscribe` 多 subscriber** —— API.md §12 已列
   未决问题，本轮不解决。
3. **`models.list` 缓存策略** —— 同上。
4. **二进制上传** —— `attachments.createUploadUrl` 在 InProcess transport
   上的映射，API.md §12 已列未决。
5. **`feedback.submit`** —— 后端目前 accept-and-drop，等存储层做好再真存。

---

## 13. 期望节奏（带 prod-readiness 标准）

| 阶段 | 范围 | 时间盒 |
|---|---|---|
| **本轮**（本周） | §8 三块全部落地（~11 后端文件 + 2 前端文件 + 4 处 spec），跑 §14 smoke test 通过 | 5 天 |
| **prod-blocking**（staging 部署前必补） | §9.1（HTTP 401/500/503 + Observability） | 不限时但 **gate-on-staging** |
| **month-paced** | §9.2（版本协商 + capability filter + health probe + cursor 分页）+ §10 P2 4 项 + schema codegen 起步 | 月内 |
| **再下一轮**（quarter 内） | P2 那批 stub 真做（workspace / providers / models / background），类型 shape 已经对齐 | 季度内 |

**prod-readiness gate**（前端 v3 提议，接受）：

> §9.1 两项（HTTP 401/500/503 + Observability）不一定要在 dev 周期内
> 完成，但**任何 staging/prod 部署前必须完成**。没 401 token 校验 =
> 任何同机进程能调 Runtime；没 observability = oncall 半夜没办法看
> metric / log。这是硬指标，不是 nice-to-have。
>
> §9.2 四项（版本协商 + capability + health probe + cursor）可以按
> "月内"节奏走，dev 阶段 happy path 不阻塞。

---

## 14. Smoke test 计划（本周 milestone）

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
5. for await (const ev of events) { /* AG-UI event render */ }
   // 期望：RUN_STARTED → STEP_STARTED → TEXT_MESSAGE_* → STEP_FINISHED → RUN_FINISHED
6. (如果 run 暂停在 lyra.approval) →
   await methods.runs.approval.submit({requestId, decision: "approve"})
7. (run 继续直到结束) → notifications/run/closed
```

**通过标准**：
- 步骤 2 返 `serverInfo + capabilities`
- 步骤 3 返 `Session` 单对象（无 `{items}` 包装）
- 步骤 4 返 `{runId, streamHandle}` 立即返回，**不等流结束**
- 步骤 5 收到的事件 JSON 形状跟 `frontend/src/protocol/agui/schemas.ts` 的
  Zod schema 验证通过
- 步骤 6 用 `decision: "approve"` 不报 -32602（前端是 approve/deny 二值）
- 步骤 7 收到 `notifications/run/closed` 后 events iterator 自然结束

跑通 = 本轮共识落地完成。

---

## 15. 状态 — `[ALIGNED]`

**前后端无悬而未决项**。等三边动手落地，跑通 §14 smoke test。

**下次需要再开 PROTOCOL_ALIGNMENT / 本文档**的触发条件：

- 后端在落地过程中发现新的 shape 歧义（比如某个字段两边理解不一致）
- §14 smoke test 失败暴露未覆盖的 case
- §9.1 / §9.2 推进中遇到 spec 没写清楚的细节
- 引入新方法 / 新事件类型时，wire shape 需要先 align

如果都没触发，下一次 review 在 schema codegen 起步时（届时 `pkg/coreapi`
变成生成产物的源头，手写 shape 不存在了）。
