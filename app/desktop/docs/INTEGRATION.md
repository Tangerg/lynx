# INTEGRATION.md — 第一轮前后端对接 playbook

> **定位**：这是「怎么把真后端（lynx Runtime）接到前端跑通」的落地手册。
> 契约语义看 [`API.md`](./API.md)（SSOT），物理传输形态看
> [`TRANSPORT.md`](./TRANSPORT.md)。本文只讲 **round-1 的 HTTP dev 形态 +
> 最小闭环 + 已知必须对齐的点**，不重复规范正文。

第一轮目标：**happy path 端到端跑通** —— 握手 → 列会话 → 建会话 → 发一轮消息
（流式）→ 终态。HITL（审批 / 提问）作为 round-1 的 stretch（前端已就绪）。

---

## 1. 连接机制（dev 形态）

前端连接参数全部固定在 `frontend/src/main/config.ts`：

```
AGUI_BASE = "http://127.0.0.1:17171"
```

后端（真 lynx，或 `internal/agui` dev mock）**监听 `127.0.0.1:17171`**，暴露：

| 端点 | 方法 | 用途 |
| --- | --- | --- |
| `/v1/rpc/{method}` | POST | 每个 JSON-RPC Request / Notification 一发。body 是完整 envelope，URL 末段是 method 名（点保留，如 `/v1/rpc/runs.start`、`/v1/rpc/runs.approval.submit`）。**无** bare `/v1/rpc` fallback。 |
| `/v1/rpc/stream?conn=<id>` | GET (SSE) | 服务端→客户端的 notification 流（`text/event-stream`）。每条 SSE event 的 `data:` 是一条 Notification 的 JSON。 |
| `/v1/info` | GET | sidecar：握手前探活 + 元数据（`serverInfo` / capabilities 预览）。无 Bearer。 |
| `/v1/health` | GET | sidecar：健康检查。`503` 也返合法 JSON（`status` 字段）。 |

**连接关联（务必实现，否则流收不到事件）**：客户端启动时生成一个 `connId`
（`crypto.randomUUID()`）。

- POST 带 header `Lyra-Connection-Id: <connId>`。
- SSE GET 带 query `?conn=<connId>`（EventSource 不能设 header，故走 query）。
- **服务端必须把「该 conn 启动的 run」的 notification 路由到匹配 `conn` 的 SSE 流**
  （`API.md` §3.2）。

**进程门禁 token**（可选，dev mock 不校验）：若后端启用本地 loopback 门禁，POST 带
`Authorization: Bearer <localToken>`（前端从 `host.config.set("api.localToken", …)`
读，见 `container.ts`）。round-1 dev 直接不设。**这不是用户鉴权**（`API.md` §1.2）。

**CORS（桌面壳硬性要求，否则一个请求都发不出去）**：Wails WebView 的 origin 是
`wails://wails.localhost`，打 `http://127.0.0.1:17171` 是**跨源**，WKWebView 强制
CORS（curl 不受影响，所以命令行能通 ≠ app 能通）。后端**必须**：

- 所有 `/v1/*` 响应带 `Access-Control-Allow-Origin: *`（或回显请求的 `Origin`）。
- **处理 `OPTIONS` 预检**：带自定义 header 的跨源 POST 会先发 `OPTIONS`，必须返
  `204`/`200` + 下面这组头（**不能返 405**）。
- `Access-Control-Allow-Methods: GET, POST, OPTIONS`
- `Access-Control-Allow-Headers: Content-Type, Last-Event-Id, Lyra-Connection-Id, Authorization`

dev mock 的 `withCORS`（`internal/agui/server.go`）就是参考实现，照搬即可。

**HTTP status 映射**（`API.md` §7.3，前端 `transports/http.ts` 已按此解析）：

| status | 含义 | body |
| --- | --- | --- |
| `200` | 有 JSON-RPC Response | envelope（`{jsonrpc,id,result\|error}`） |
| `204` | Notification ack | 空 |
| `400` / `404` / `409` | 业务错误 | **JSON-RPC error envelope**（不是裸 HTTP 错误） |
| `401` / `500` / `503` | 传输层错误 | flat JSON `{error, traceId?}` |

> 注意：**业务错误（如 `session_not_found`）走 JSON-RPC `error.code`，HTTP status 只
> 反映传输层**（`API.md` §7.3 + §10.6）。别把业务错误映射成 404/500。

---

## 2. 握手（boot 时序）

```
sidecar.info()                        // 探活 + 读 serverInfo（无需 envelope）
   ↓
runtime.initialize(InitializeRequest) // 协商 protocolVersion + capabilities
   ↓
ServerCapabilities                    // 决定服务端可发哪些事件 / 前端开哪些 UI
```

前端在 `runtime.initialize` 里声明 **ClientCapabilities**，其中
`events.custom` 的成员**即 HITL 能力开关**：含 `lyra.approval` / `lyra.question`
表示前端能渲染并回传对应 HITL，**服务端不得对不支持的 client 发起对应 HITL**
（`API.md` §4.3 防挂死）。round-1 建议 payload：

```jsonc
// → runtime.initialize
{
  "protocolVersion": "2026-05-28",
  "clientInfo": { "name": "lyra-desktop", "version": "0.0.0" },
  "capabilities": {
    "events": {
      "standard": ["RUN_STARTED","RUN_FINISHED","RUN_ERROR","TEXT_MESSAGE_START",
                   "TEXT_MESSAGE_CONTENT","TEXT_MESSAGE_END","TOOL_CALL_START",
                   "TOOL_CALL_ARGS","TOOL_CALL_END","TOOL_CALL_RESULT",
                   "REASONING_MESSAGE_START","REASONING_MESSAGE_CONTENT",
                   "REASONING_MESSAGE_END","STATE_SNAPSHOT","STATE_DELTA",
                   "MESSAGES_SNAPSHOT","STEP_STARTED","STEP_FINISHED"],
      "custom":   ["lyra.plan","lyra.plan-block","lyra.code-proposal",
                   "lyra.search-results","lyra.approval","lyra.approval-result",
                   "lyra.question","lyra.question-result","lyra.telemetry"]
    },
    "features": { "multimodal": true, "markdown": true }
  }
}
```

> ⚠️ **前端 round-1 TODO（已确认未接）**：目前 `runtime.initialize` 只有类型 +
> `runtimeStore` 槽位，**boot 流程还没真正调用它**（无 `setHandshake` 调用方）。
> round-1 第一刀就是把「sidecar.info → runtime.initialize → 写入 runtimeStore」接上
> （建议放一个 bootstrap builtin plugin）。在此之前后端可先就绪 `runtime.initialize`
> 端点等前端接。

---

## 3. Round-1 方法范围

下面是**前端当前真正会调用**的方法（其余 `API.md` §5.2 的方法已在前端有 typed
wrapper，但 round-1 不调，后端可后续轮次再实现）。

### P0 — happy path 必须（不实现则跑不通）

| method | 返回 | 备注 |
| --- | --- | --- |
| `runtime.initialize` | `InitializeResponse` | 握手；见 §2 |
| `sessions.list` | `Page<Session>` | 侧栏会话列表（分页 wrapper） |
| `sessions.create` | `Session` | 新建会话 |
| `runs.start` | `Stream<StartRunResponse, RunEvent>` | 发消息；**核心**，见 §4 |
| `runs.cancel` | `void` | 停止进行中的 run（`CancelRunRequest {runId, reason?}`） |
| `workspace.projects` | `Project[]` | 工作区面板 |
| `workspace.filesChanged` | `FileChange[]` | 工作区改动列表 |
| `workspace.mcp.list` | `MCPServer[]` | MCP 面板（注意 `toolCount`，见 §5） |

### P1 — HITL（前端已就绪，后端有就能亮）

| method / event | 形状 | 备注 |
| --- | --- | --- |
| `lyra.approval`（事件） | `ApprovalRequest` | 服务端在 run 流里推；前端渲染审批卡片 |
| `runs.approval.submit` | `SubmitApprovalRequest` | `{requestId, decision, editedArgs?, reason?}` |
| `lyra.approval-result`（事件） | `ApprovalResult` | `{requestId, decision}`，落定卡片 |
| `lyra.question`（事件） | `QuestionRequest` | 澄清提问；前端渲染单/多选卡片 |
| `runs.question.answer` | `AnswerQuestionRequest` | `{requestId, answers}` |
| `lyra.question-result`（事件） | `QuestionResult` | `{requestId}`，落定卡片 |

### 后续轮次（前端有 wrapper，round-1 不调用）

`runs.list` / `runs.subscribe` / `messages.*` / `sessions.{get,update,delete,fork,export}` /
`providers.*` / `models.list` / `tools.*` / `attachments.*` / `background.*` /
`memory.*` / `workspace.{diff,fileHead,grep,skills,agentDocs,mcp.{reconnect,tools},selectProject}` /
`feedback.submit`。

---

## 4. 流式契约（最容易对错，重点核对）

`runs.start` **立即返 `{runId}`**（不等流结束），之后事件经 SSE 推送。

**RunEvent 信封**（`notifications/run/event` 的 `params`，`API.md` §3.1）：

```jsonc
{
  "jsonrpc": "2.0",
  "method": "notifications/run/event",
  "params": {
    "runId": "r-123",            // 关联回 runs.start
    "eventId": "1",              // 单 run 内单调递增（string）；Last-Event-Id 续传锚点
    "ts": "2026-05-30T10:00:00Z",// 服务端权威时间戳，**每条必带**（前端 Zod 必校）
    "parentToolUseId": null,     // 可选；子 agent 归属
    "event": { "type": "RUN_STARTED", "threadId": "t-1", "runId": "r-123" }  // §4 AG-UI 事件
  }
}
```

**终态**（`notifications/run/closed`，一次读全停止原因 + 计量）：

```jsonc
{
  "jsonrpc": "2.0",
  "method": "notifications/run/closed",
  "params": {
    "runId": "r-123",
    "result": {                  // RunResult（§6.3）—— 不是旧的 { reason }
      "stopReason": "completed", // completed|canceled|error|max_turns|max_budget
      "usage": { "inputTokens": 1200, "outputTokens": 340 },
      "costUsd": 0.012,
      "turns": 1
    }
  }
}
```

要点：

- **每条 RunEvent 必带 `ts`**（前端 `stream.ts` 的 Zod schema 强校验，缺了整条被丢）。
- **`eventId` 单 run 内单调递增**，用于 `Last-Event-Id` 续传。
- **首连回放起点**：一条 SSE 订阅**不带 `Last-Event-Id` 时，从该 run 第一个事件起重放**
  （`API.md` §3.3）—— 这样消除「先建 SSE 还是先 `runs.start`」的竞态，`RUN_STARTED`
  等头部事件不会丢。前端不卡顺序。
- run 执行期出错走 **`RUN_ERROR` 事件 + `run/closed.result.error`**（不走那条已返回的
  `runs.start` Response，`API.md` §7.0）。

一轮 happy path 的事件序列（典型）：
`RUN_STARTED → STEP_STARTED → TEXT_MESSAGE_START → …_CONTENT× N → …_END → STEP_FINISHED
→ RUN_FINISHED`，最后 `notifications/run/closed`。

---

## 5. 后端 round-1 必须对齐的契约点（前端已按这些实现）

这些是前端**已经按定稿契约写死、后端必须匹配**的点（历轮 review 里反复调整过的）：

1. **envelope `id` 是 `string`**（`API.md` §1.1）。前端发的 `id` 是字符串（`"1"`,`"2"`…），
   Response 必须用同一个字符串 id 关联。
2. **取消在飞 Request 的通知是 `{ id, reason? }`**（`API.md` §2.3）——
   `notifications/canceled` 的 params 用 `id`（被取消 Request 的 envelope id），
   **不是** `requestId`。`requestId` 是 HITL 业务 id，别混。
3. **RunEvent 每条必带 `ts`**（§4）。
4. **`run/closed` 带 `result: RunResult`**（§4），不是旧的 `{ reason }`。
5. **HITL 命名分层**：`lyra.approval` 事件 payload = `ApprovalRequest`；提交方法参数 =
   `SubmitApprovalRequest`（`API.md` §6.9 / §8.1）。`decision` 用 **`approve` / `deny`**
   （命令式），**不是** `approved`/`declined`。`lyra.approval-result` /
   `lyra.question-result` 的 payload 形状见 §6.9。
6. **`MCPServer.toolCount`**（数量），不是 `tools`。详情按需走 `workspace.mcp.tools`。
7. **非分页 list 返裸数组**：`workspace.*` / `tools.list` / `providers.list` /
   `models.list` / `background.list` 返 `T[]`，**不套** `{items}`。只有 `sessions.list` /
   `messages.list` 用 `Page<T>`。
8. **业务错误走 `error.code`**（§7.2），HTTP status 只反映传输层（§1 末）。

---

## 6. 怎么跑 / 联调

**dev（前端 + dev mock）**：

```bash
wails dev    # 在 /Users/tangerg/Desktop/lyra/ 跑：自动起 vite 前端 + Go 后端(:17171)
```

`internal/agui` dev mock **目前只实现了**：`runs.start` / `runs.cancel` /
`runs.approval.submit` / `sessions.list` / `workspace.filesChanged` /
`workspace.mcp.list` / `workspace.projects`。接真后端前，mock 可先补齐 §3 P0 缺口
（`runtime.initialize` / `sessions.create`）+ 把流信封升级到带 `ts` / `result` 的新形状。

**切到真 lynx**：让 lynx 监听 `127.0.0.1:17171`（或改 `AGUI_BASE` 指向 lynx 的地址），
其余不变。

**curl 自测（不依赖前端）**：

```bash
# 1) 探活
curl -s http://127.0.0.1:17171/v1/info | jq

# 2) 握手
curl -s -X POST http://127.0.0.1:17171/v1/rpc/runtime.initialize \
  -H 'Content-Type: application/json' -H 'Lyra-Connection-Id: dev-1' \
  -d '{"jsonrpc":"2.0","id":"1","method":"runtime.initialize","params":{"protocolVersion": "2026-05-28","clientInfo":{"name":"curl","version":"0"},"capabilities":{"events":{"standard":[],"custom":["lyra.approval","lyra.question"]},"features":{}}}}' | jq

# 3) 开 SSE（另开一个终端，conn 必须和 POST 的 Lyra-Connection-Id 一致）
curl -N "http://127.0.0.1:17171/v1/rpc/stream?conn=dev-1"

# 4) 起一个 run（在 SSE 已开的前提下）
curl -s -X POST http://127.0.0.1:17171/v1/rpc/runs.start \
  -H 'Content-Type: application/json' -H 'Lyra-Connection-Id: dev-1' \
  -d '{"jsonrpc":"2.0","id":"2","method":"runs.start","params":{"sessionId":"s-1","messages":[{"id":"m-1","sessionId":"s-1","role":"user","content":"hi","createdAt":"2026-05-30T10:00:00Z"}]}}' | jq
# → 期望 SSE 端依次收到 RUN_STARTED … TEXT_* … RUN_FINISHED + run/closed(result)
```

---

## 7. Round-1 验收 checklist

happy path（全绿即 round-1 通）：

- [ ] `GET /v1/info` 返 200 + `serverInfo`
- [ ] `runtime.initialize` 返 `InitializeResponse`，前端 `runtimeStore` 记录
      `protocolVersion` + `ServerCapabilities`（**需先接上前端 boot 调用，§2 TODO**）
- [ ] `sessions.list` 渲染出侧栏会话
- [ ] `sessions.create` 新建会话并出现在列表
- [ ] 发一条消息 → `runs.start` 返 `{runId}`，SSE 收到 `RUN_STARTED`
- [ ] SSE 收到 `TEXT_MESSAGE_*` 流（带 `ts` / 单调 `eventId`），聊天区流式渲染
- [ ] `RUN_FINISHED` + `run/closed { result }`，run 进入 idle 终态
- [ ] `runs.cancel` 能中断进行中的 run（流以 `run/closed` stopReason `canceled` 收尾）
- [ ] `workspace.projects` / `filesChanged` / `mcp.list` 渲染工作区面板

HITL（stretch，前端已就绪）：

- [ ] 后端推 `lyra.approval` → 前端审批卡片 → `runs.approval.submit {decision}` →
      `lyra.approval-result` 落定卡片
- [ ] 后端推 `lyra.question` → 前端提问卡片 → `runs.question.answer {answers}` →
      `lyra.question-result` 落定卡片
