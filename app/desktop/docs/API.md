# Lyra API 契约

> 任何 Lyra 前端 ↔ 任何 Lyra 后端之间的"线上协议"唯一权威。
> 设计目标是 **m × n 部署矩阵** —— 前端不知道自己在跟哪种后端
> 说话，后端也不假设自己在被哪种前端调用。协议是它们之间唯一
> 共享的东西。
>
> 读者：要做新前端（web / 包装客户端 / TUI / mobile）、要移植
> 后端（嵌入客户端 / 独立服务）、或者要演进契约本身的人。

---

## 0. 架构模型

### 0.1 m × n 矩阵

| ↓ 前端 / 后端 → | Embedded（嵌入客户端） | Standalone server（独立服务） |
| --- | --- | --- |
| **Web（浏览器）** | n/a（没壳可嵌） | ✓ |
| **包装 Web**（Wails / Tauri / Electron） | ✓（loopback） | ✓ |
| **TUI**（终端） | ✓（loopback） | ✓ |
| **Mobile**（RN / 原生，未来） | n/a | ✓ |

4 种前端 × 2 种后端 = 7 个可行组合。**每个组合都用同一份线上
协议。** 前端不 import 后端类型，后端不假设前端共驻。

**显式不做多租户。** Lyra 协议层不承认 "多 tenant 共享一个
backend instance"。如果未来真的需要把服务给多个客户用，做法是
**物理隔离** —— 每个客户一个独立 `lyra-server` 部署（独立进程 /
独立 DB / 独立鉴权域），不在 URL 里塞 `workspaces/{ws}/`。这把
租户隔离的复杂度从协议层下沉到部署层，协议保持单一服务的简洁
形状。

### 0.2 对 API 设计的约束

- **协议和传输是两件事。** 本文档定义**协议**——方法名、payload
  形状、事件序列、错误信封。**传输**（字节——或者 struct 指针——
  怎么在两端之间移动）放在 [`TRANSPORT.md`](./TRANSPORT.md)。
  HTTP 只是六种传输实现之一；对于 Go-to-Go 嵌入式部署（比如
  Bubble Tea TUI + Go 运行时）传输是直接函数调用，零序列化。
  **所有传输说同一种协议**，所以本文档不会因为你挑了哪种传输
  而改变。
- **跨进程边界必须鉴权。** HTTP / Wails IPC / socket 都需要鉴权
  （Bearer token / OS 级 ACL）。in-process 情况（Go ↔ Go 同二进制）
  信任 caller——能力校验仍然发生在 `CoreAPI` impl 内部。每个
  传输的鉴权矩阵见 `TRANSPORT.md §6`。
- **能力发现不是可选。** 今天编译的前端 X 可能要跟明天编译的
  后端 Y 说话——只有两边都启用的 feature 才能开。
- **Schema 就是 API。** OpenAPI + AsyncAPI 文件是 SSOT，
  按 TS / Go / Rust / Python / 任何消费方生成类型。不留语言专属
  捷径。
- **能无状态就无状态。** 后端可能在负载均衡后跑 N 个副本。
  Session affinity 是可选项（通过 `X-Session-Affinity` header），
  不是默认假设。
- **流必须可重放。** 网络抖动、浏览器 tab 休眠、TUI `^Z fg`
  重连。SSE 用 `Last-Event-ID` 重放丢失的事件。

### 0.3 前端形态 —— 各自需要什么

| 形态 | 默认传输 | 备注 |
| --- | --- | --- |
| **Web** | HTTP（`fetch` + `EventSource`） | 浏览器唯一选项。CORS preflight；cookie 或 Bearer 鉴权；从被服务的 origin 取相对 URL。 |
| **包装 Web**（Wails / Tauri / Electron） | Wails IPC（嵌入）/ HTTP（远程） | 后端在宿主进程里时跳过 HTTP，走外壳的 IPC bridge。见 `TRANSPORT.md §4.2`。 |
| **TUI（Go）** | InProcess（嵌入）/ Socket（本地）/ HTTP（远程） | 两端都是 Go 且同二进制时，"传输"就是一次函数调用。见 `TRANSPORT.md §4.1`。 |
| **TUI（Node / Rust / Python）** | Socket（本地）/ HTTP（远程） | 没法 in-process 嵌 Go 运行时；把 `lyra-server` 作为兄弟进程 spawn 起来、用 Unix socket / Named pipe 连。 |

### 0.4 后端形态 —— 各自提供什么

| 形态 | 监听 | 默认鉴权 | 持久化 |
| --- | --- | --- | --- |
| **Embedded** | 只 `127.0.0.1:0`（不出局域网） | 本地 config 里生成的 token，单用户 | 跟客户端二进制并排的 SQLite |
| **Standalone server** | `0.0.0.0:port`，可配置 | 必须：Bearer token / API key（也可以接 SSO，但永远单租户） | SQLite（小）或 Postgres（大） |

两种都讲同一份线上协议。差异只在部署 / 持久化 / 鉴权强度。
**Standalone server 始终是单租户** —— 一个进程服务一个客户。多
客户场景用多次部署解决，不在协议或 URL 里区分。

---

## 1. 现状快照（2026-05-27）

| 表面 | 今天 | 到 m × n 还差什么 |
| --- | --- | --- |
| 流式事件（`/run` SSE） | 16 个 AG-UI 标准事件 + 7 个 `lyra.*` CUSTOM 事件，从 fixture DSL 出 | 把 fixture 换成真 LLM 调用 + 真工具执行。加 `Last-Event-ID` 续传。 |
| REST 端点 | 13 个端点喂 fixture | 加 capabilities / auth / sessions CRUD / messages 分页 / attachments / providers / models / tools / cancel —— 见 §4 |
| HITL 审批 | `POST /permission` 解锁一个内存 chan | 改成幂等 + 绑到具体 run id（不是全局 chan）。 |
| 鉴权 | 无 | 第一天就得有 —— Bearer token。见 §3.2。 |
| Schema SSOT | 手写在 `frontend/src/lib/queries.ts` | OpenAPI 3.1 + AsyncAPI 2.6 生成 TS + Go。见 §6。 |
| 版本 | 无 | URL 前缀 `/v1/` + `X-Lyra-Protocol-Version` header。 |

Mock 后端今天监听 `http://127.0.0.1:17171`；前端通过
`host.config.set("api.baseUrl", "…")` 配。**前端没有任何代码假设
后端是本地的。**

---

## 2. 线上原则

每个端点都继承的常量规则。**新加任何东西必须遵守——它们是
m × n 矩阵真正跑得通的底座。**

### 2.1 传输

下面的协议不变量用 HTTP 术语描述，因为 HTTP 是公分母传输；非 HTTP
传输（in-process / Wails IPC / Unix socket）把每一行映射到等价
原语——见 [`TRANSPORT.md §5–6`](./TRANSPORT.md)。新东西落在这里
当它约束**发什么**；落在 `TRANSPORT.md` 当它约束**怎么发**。

| 项 | 规则 |
| --- | --- |
| **默认传输** | HTTP/1.1 或 HTTP/2 over TCP。非 loopback 必须 TLS。其他传输见 `TRANSPORT.md §4`。 |
| **Content type** | 请求 / 响应默认 `application/json; charset=utf-8`。流是 HTTP 上的 `text/event-stream` / socket 上的 length-prefixed JSON 帧 / in-process 是 typed event 的 channel。上传是 `multipart/form-data`。 |
| **版本** | URL 前缀 `/v1/`。不匹配返 `426 Upgrade Required`，`Sunset` header 列出支持的版本。In-process 客户端直接 import `pkg/coreapi`，版本漂移变成编译错而不是运行时 426。 |
| **CORS** | 服务端后端 echo `Access-Control-Allow-Origin`；嵌入式 HTTP 后端允许 `null` + `127.0.0.1`。非 HTTP 传输无 CORS 概念（没有 origin）。 |
| **压缩** | JSON 响应允许 `Accept-Encoding: gzip, br`；SSE 流永不压缩（延迟 > 体积）。Socket / in-process 永不压缩。 |
| **心跳** | SSE 流每 15s 一个 `:heartbeat\n\n`。Socket 传输按同样节奏发 `{"type":"heartbeat"}` 帧。In-process 豁免（channel 关闭就是 liveness 信号）。 |

### 2.2 身份 & 版本 header

每个请求：

```http
Authorization: Bearer <token>
X-Lyra-Client: lyra-web/0.4.2          // <kind>/<version>
Idempotency-Key: 0193…d8b5             // UUID v7, required on POST/PUT/PATCH
Accept-Language: en-US, zh-CN;q=0.8    // for localized error messages
```

每个响应：

```http
X-Lyra-Server: lyra-core/0.8.1
X-Lyra-Protocol-Version: 1.2.0         // semver of the wire contract
X-Lyra-Request-ID: 0193…d8b5           // echoed back for tracing
```

### 2.3 鉴权

| 形态 | Token 来源 | 续期 |
| --- | --- | --- |
| Embedded | 安装时生成，写到 `$XDG_CONFIG_HOME/lyra/token`（客户端读同一个文件） | 不续期 —— 重装时重新生成 |
| Standalone | 运营方颁发的静态 token（环境变量 `LYRA_TOKEN`）或 `POST /v1/auth/login` 走用户名密码 | `POST /v1/auth/refresh` 返回新的 token pair |

前端探测 `GET /v1/info`（无需鉴权）发现这个后端用哪种机制，
再 dispatch 到对应流程。**后端绝对不能在没有合法 token 时
serve 任何非 `/v1/info`、非 `/v1/auth/*` 的端点。**

需要接公司 SSO / IdP 的部署可以在 Standalone 模式上加 OAuth 2.1 +
PKCE（`/v1/auth/authorize` + `/v1/auth/token`），登录后仍然走同一份
Bearer token —— 单租户假设不变，只是 token 来源换了。

### 2.4 错误 —— RFC 7807 Problem Details

```ts
interface ProblemDetails {
  type: string;       // URI: "https://lyra.dev/problems/rate-limit"
  title: string;      // human-readable summary
  status: number;     // HTTP status mirror
  detail?: string;    // longer explanation
  instance?: string;  // request id (mirrors X-Lyra-Request-ID)
  errors?: Array<{ path: string; message: string }>;  // for 422
}
```

所有非 2xx 响应都是 ProblemDetails JSON。前端不管碰到哪种
后端形态，都通过单一拦截器规范化成这个形状。

### 2.5 幂等性

修改状态的 POST 必须带 `Idempotency-Key`。服务端把
`(key, response)` 对存 24 小时、重试时回放同样的响应——
覆盖"用户点 Send、网络抖动、客户端重试"场景。

适用：`POST /run`、`POST /sessions`、`POST /attachments`、
`POST /messages/{id}/edit`、`POST /run/{id}/cancel`。

### 2.6 分页

基于游标：

```
GET /v1/sessions?limit=20&cursor=eyJpZCI6InNlc3NfMTIzIn0
→ { items: [...], nextCursor?: "...", hasMore: boolean }
```

默认不暴露 offset / total-count —— 这俩在真实负载下都坏。
个别真的需要总数的场景留了 `?countOnly=true` 查询。

### 2.7 时间、ID、locale

| 项 | 格式 |
| --- | --- |
| 时间戳 | 带时区的 ISO-8601：`"2026-05-27T12:34:56.789Z"`。线上永远 UTC，前端用 `Intl.DateTimeFormat` 格式化。 |
| 时长 | 毫秒，整数（如 `runDurationMs: 1234`）。 |
| ID | 服务端默认生成 UUID v7（可排序 + 单调）。需要幂等的地方允许客户端提供 ID；服务端同时存 `id`（server-assigned）和 `clientId`（可选）。 |
| Locale | 请求 `Accept-Language` header；服务端只在错误的 `title` / `detail` 里返本地化字符串。Domain 数据保持源语言。 |

### 2.8 背压 & 可恢复

| 机制 | 在哪 | 行为 |
| --- | --- | --- |
| **SSE 心跳** | 所有流 | 每 15s 一个 `:heartbeat\n\n` |
| **`Last-Event-ID`** | 前端 SSE 重连 | 服务端按同一个 `runId` 重放 `id > Last-Event-ID` 的事件，重放窗口 30s 封顶 |
| **HTTP/2 流控** | 长上传 / 重放 | 自动，应用层无代码 |
| **限流** | 所有端点 | `429 + Retry-After: <seconds>`。类型 `/problems/rate-limit`。 |
| **取消** | `/run` SSE | 客户端关连接；可选附加 `POST /run/{id}/cancel` 走显式清理 |

---

## 3. 流式事件 —— `POST /v1/run`（SSE）

### 3.1 请求

```http
POST /v1/run HTTP/1.1
Authorization: Bearer <token>
Idempotency-Key: 0193…
Content-Type: application/json
Accept: text/event-stream
Last-Event-ID: 1234         (optional, on reconnect)
```

```ts
interface RunInput {
  threadId: string;            // == sessionId
  runId?: string;              // server generates if omitted; UUID v7
  messages: Message[];         // history + new turn
  state?: Record<string, unknown>;  // resume state
  tools?: ToolSpec[];          // client-supplied tools (rare — usually server-side)
  context?: ContextItem[];     // file / URL / selection refs
  model?: string;              // explicit provider/model id
  mode?: "agent" | "chat" | "plan";
  attachments?: string[];      // ids from POST /attachments
  capabilities?: ClientCapabilities;  // events / blocks the client renders
}
```

### 3.2 响应（SSE）

每个事件都是 `id: <seq>\nevent: <type>\ndata: {…}\n\n`。`<seq>` 是
每个 run 内部单调递增的整数，给 `Last-Event-ID` 续传用。

#### 3.2.1 AG-UI 标准事件（16 个）—— agent UX 的骨架

| 分组 | 事件 |
| --- | --- |
| 生命周期 | `RUN_STARTED` / `RUN_FINISHED` / `RUN_ERROR` |
| Step | `STEP_STARTED` / `STEP_FINISHED` |
| 文本 | `TEXT_MESSAGE_START` / `_CONTENT` / `_END` / `_CHUNK` |
| 工具调用 | `TOOL_CALL_START` / `_ARGS` / `_END` / `_CHUNK` / `_RESULT` |
| 推理 | `REASONING_MESSAGE_START` / `_CONTENT` / `_END` / `_CHUNK` + `THINKING_TEXT_MESSAGE_*` |
| 共享状态 | `STATE_SNAPSHOT` / `STATE_DELTA`（RFC 6902 JSON Patch） |
| 历史 | `MESSAGES_SNAPSHOT` |
| Per-message 活动 | `ACTIVITY_SNAPSHOT` / `ACTIVITY_DELTA` |
| 扩展 | `CUSTOM` / `RAW` |

#### 3.2.2 Lyra CUSTOM 事件 —— `event.type === "CUSTOM"`，按 `event.name` 分发

| `name` | Payload | 用途 |
| --- | --- | --- |
| `lyra.plan` | `{ items: PlanItem[] }` | 替换 `state.plan` |
| `lyra.plan-block` | `{ messageId }` | 挂一个 `plan` content block |
| `lyra.code-proposal` | `{ parentMessageId, lang, file, text }` | Diff 提案 block |
| `lyra.search-results` | `{ parentMessageId, results }` | 搜索结果 block |
| `lyra.approval` | `{ requestId, parentMessageId, text, command, reason, risk?, scope?, target?, reversible? }` | HITL 审批 block |
| `lyra.approval-result` | `{ requestId, decision }` | 服务端确认收到 decision |
| `lyra.telemetry` | 自由形态 | 性能 / 调试信号 |

#### 3.2.3 未来迭代预留（来自 kimi-code / agent-chat-ui audit）

| `name` | Payload | 来源 |
| --- | --- | --- |
| `lyra.interrupt` | `{ requestId, kind: "approve" \| "edit" \| "reject", display }` | LangGraph 风格 interrupt + 结构化展示 |
| `lyra.resume` | `{ requestId, decision }` | `interrupt` 的对应 |
| `lyra.checkpoint` | `{ messageId, parentCheckpoint }` | 从早期消息 edit-and-re-run |
| `lyra.meta` | `{ kind: "thumbs-up" \| "thumbs-down" \| "note" \| "bookmark", refId, value }` | RLHF 反馈（ag-ui 草案） |
| `lyra.subagent.spawned` / `.completed` / `.failed` | `{ parentRunId, subRunId, … }` | 嵌套 agent 调用 |
| `lyra.background.started` / `.updated` / `.terminated` | `{ taskId, label, progress?, exitCode? }` | 长任务（kimi-code 形状） |
| `lyra.compaction.started` / `.completed` | `{ summary, tokensBefore, tokensAfter }` | 上下文窗口压缩（Phase 4 backlog） |

所有 CUSTOM 事件 payload 必须在 `frontend/src/protocol/agui/schemas.ts`
有 Zod schema **并且** 从 `schemas/events.yaml` 生成对应的 Go 镜像
（见 §6）。

---

## 4. REST 端点

所有端点都直接挂在 `/v1/` 下，**没有 workspace 前缀**。每个
`lyra-server` 实例服务一个客户 / 一个隔离域 —— 多客户需求通过
独立部署解决，不在 URL 形状里反映。

### 4.1 发现 & 鉴权 —— 任何后端形态都必须有

| P | Method | Path | 用途 |
| --- | --- | --- | --- |
| P0 | `GET` | `/v1/info` | **无鉴权。** 返回 `{ server, protocolVersion, authKinds, instanceLabel }`，让全新的前端知道自己在哪、怎么登。 |
| P0 | `GET` | `/v1/capabilities` | 需鉴权。返回支持的事件 / providers / tools / features（见 §5.1）。 |
| P0 | `GET` | `/v1/health` | Liveness 探针。`{ status: "ok"\|"degraded", checks: {...} }`。 |
| P0 | `POST` | `/v1/auth/login` | 用户名密码 → token pair。服务端也可以接 API key。 |
| P0 | `POST` | `/v1/auth/refresh` | Refresh token 轮换。 |
| P0 | `POST` | `/v1/auth/logout` | 服务端 token 失效。 |
| P1 | `GET` | `/v1/me` | 当前用户 profile。 |
| P1 | `POST` | `/v1/auth/authorize` + `/v1/auth/token` | 接公司 SSO / IdP 时的 OAuth 2.1 + PKCE（可选，单租户内部使用）。 |

### 4.2 Sessions、messages、runs —— 对话核心面

| P | Method | Path | 用途 |
| --- | --- | --- | --- |
| P0 | `POST` | `/v1/run` | 启动一次 run，返 SSE 流（§3）。 |
| P0 | `POST` | `/v1/run/{runId}/cancel` | 显式取消；幂等。 |
| P0 | `POST` | `/v1/run/{runId}/permission` | HITL 决策。Body：`{ requestId, decision, reason? }`。替换今天的 `POST /permission`。 |
| P0 | `GET` | `/v1/sessions` | 列 sessions。游标分页。 |
| P0 | `POST` | `/v1/sessions` | 新建。Body：`{ title?, model?, metadata? }`。 |
| P0 | `GET` | `/v1/sessions/{id}` | 读一条（metadata + 最近活动）。 |
| P0 | `PATCH` | `/v1/sessions/{id}` | 重命名 / pin / metadata patch。 |
| P0 | `DELETE` | `/v1/sessions/{id}` | 级联删除。 |
| P0 | `GET` | `/v1/sessions/{id}/messages` | 游标分页历史。替换"每次重连大块 `MESSAGES_SNAPSHOT`"的做法。 |
| P1 | `POST` | `/v1/sessions/{id}/messages/{msgId}/edit` | 从 checkpoint 处 edit-and-re-run。返回 `{ runId, checkpoint }`，并在 run SSE 上 emit `lyra.checkpoint`。 |
| P1 | `POST` | `/v1/sessions/{id}/fork` | 在 checkpoint 处 fork session。 |
| P2 | `GET` | `/v1/sessions/{id}/export?format=md\|json` | 服务端渲染导出。 |

### 4.3 Workspace 数据（前端渲染的"面板"）

> 这里的 "workspace" 指**前端面板域**（diff / grep / terminal / mcp
> 那一组数据），跟多租户无关 —— Lyra 不做多租户。命名是历史习惯，
> URL 上是单一 `/v1/workspace/...` 前缀而不是 `/v1/workspaces/{ws}/...`。

镜像 Lyra 现在已经有的那批 fixture 端点。后端形态决定怎么填
（嵌入式用真文件系统 / git / pty；独立服务把它们指向运行后端的
那台机器上的对应资源）。

| P | Method | Path | 用途 |
| --- | --- | --- | --- |
| P0 | `GET` | `/v1/workspace/files-changed` | Diff 概览。 |
| P0 | `GET` | `/v1/workspace/diff?path=…` | 单文件 unified diff。 |
| P0 | `GET` | `/v1/workspace/file-head?path=…` | 文件预览。 |
| P0 | `GET` | `/v1/workspace/grep?q=…` | 代码搜索。 |
| P0 | `GET` | `/v1/workspace/terminal/{runId}/output` | 工具 terminal session 的 pty 输出流。 |
| P0 | `GET` | `/v1/workspace/projects` | 项目列表（后端管多个时）。 |
| P0 | `GET` | `/v1/workspace/mcp-servers` | 已注册的 MCP 服务 + 状态。 |
| P1 | `POST` | `/v1/workspace/mcp-servers/{id}/reconnect` | 重连 MCP。 |
| P1 | `GET` | `/v1/workspace/skills` | 可用 skill（kimi-code 风格，支持时）。 |

### 4.4 Providers、models、tools —— agent 能用什么

| P | Method | Path | 用途 |
| --- | --- | --- | --- |
| P0 | `GET` | `/v1/providers` | LLM provider 注册表。 |
| P0 | `POST` | `/v1/providers/{id}/test` | 校验凭证。 |
| P0 | `GET` | `/v1/models?provider=…` | per-provider 模型列表。 |
| P0 | `GET` | `/v1/tools` | Tool 注册表 + JSON-Schema 参数。 |

### 4.5 附件 & 工件

| P | Method | Path | 用途 |
| --- | --- | --- | --- |
| P1 | `POST` | `/v1/attachments` | Multipart 上传，返 `{ id, url, sha256, … }`。 |
| P1 | `GET` | `/v1/attachments/{id}` | 下载（首选签名 URL）。 |
| P1 | `DELETE` | `/v1/attachments/{id}` | 回收。 |
| P2 | `GET` | `/v1/artefacts/{runId}/{name}` | 工具产出的 artefact（渲染好的图、生成的文件）。 |

### 4.6 后台任务（kimi-code 启发，后端支持时）

| P | Method | Path | 用途 |
| --- | --- | --- | --- |
| P1 | `GET` | `/v1/background` | Workspace 内活跃任务。 |
| P1 | `POST` | `/v1/background/{taskId}/stop` | 停止任务。 |
| P1 | `GET` | `/v1/background/{taskId}/output?tail=N` | Tail 输出。 |

### 4.7 插件 sideload（可选——前端 feature，但后端 host 包）

| P | Method | Path | 用途 |
| --- | --- | --- | --- |
| P2 | `GET` | `/v1/plugins` | 插件 manifest（marketplace 上线后）。 |
| P2 | `GET` | `/v1/plugins/{id}/*` | 插件资源代理。 |

### 4.8 反馈、版本、telemetry

| P | Method | Path | 用途 |
| --- | --- | --- | --- |
| P2 | `POST` | `/v1/feedback` | RLHF —— 当前端更想走 REST 时替代 `lyra.meta` CUSTOM 事件。 |
| P2 | `GET` | `/v1/version` | `{ server, protocolVersion, channel, releasedAt }`。 |
| P2 | `GET` | `/v1/telemetry/optin` + `POST /v1/telemetry/optin` | 隐私同意开关。 |

---

## 5. 形状

### 5.1 Capabilities —— 后端暴露的能力清单

```ts
interface Capabilities {
  protocol: {
    version: string;          // semver of the wire contract
    minClientVersion?: string;
  };
  events: {
    standard: string[];       // AG-UI events emitted
    custom: string[];         // lyra.* events emitted
  };
  features: {
    multimodal: boolean;       // accepts image attachments
    reasoning: boolean;        // emits REASONING_MESSAGE_*
    checkpoints: boolean;      // supports edit-and-re-run
    interrupts: boolean;       // supports inline HITL interrupts
    background: boolean;       // emits + manages background tasks
    subagents: boolean;        // emits subagent.* events
    skills: boolean;           // exposes /skills
    mcp: boolean;              // exposes /mcp-servers
    sessionExport: boolean;    // serves /sessions/{id}/export
    attachments: { enabled: boolean; maxSizeBytes?: number; mimeTypes?: string[] };
  };
  providers: string[];         // e.g. ["openai", "anthropic", "local"]
  limits: {
    maxMessagesPerSession?: number;
    maxConcurrentRuns?: number;
    rateLimit?: { perMinute: number; perHour: number };
  };
  deployment: {
    kind: "embedded" | "standalone";
    instanceLabel?: string;    // shown in UI, e.g. "Lyra @ acme.internal"
  };
}
```

前端默认把每个 `features.*` 当 `false` 处理——后端没实现某个
feature 时**必须不挂**。

### 5.2 Info —— 鉴权前探针

```ts
interface ServerInfo {
  server: { name: string; version: string };
  protocolVersion: string;
  authKinds: Array<"bearer" | "oauth" | "apiKey" | "anonymous">;
  instanceLabel?: string;
  loginUrl?: string;          // OAuth start URL when relevant
  brandingUrl?: string;       // logo / theme override when self-hosted with branding
}
```

### 5.3 核心对话形状

```ts
type SessionStatus = "running" | "waiting" | "idle";

interface Session {
  id: string;
  title: string;
  status: SessionStatus;
  model: string;
  createdAt: string;          // ISO-8601
  updatedAt: string;
  lastMessageAt?: string;
  metadata: Record<string, unknown>;
  pinned?: boolean;
  archived?: boolean;
}

interface Message {           // AG-UI shape
  id: string;
  sessionId: string;
  role: "user" | "assistant" | "system" | "tool" | "developer";
  content?: string;
  toolCalls?: ToolCall[];
  toolCallId?: string;
  createdAt: string;
  metadata?: Record<string, unknown>;
}

interface ToolSpec {
  name: string;
  description?: string;
  parameters: JsonSchema;
  // execution location — informs UI ("this tool runs on your machine")
  origin: "server" | "client" | "mcp";
}

interface ContextItem {
  kind: "file" | "url" | "selection" | "image";
  // …kind-specific fields
}
```

### 5.4 ProblemDetails —— 错误信封

```ts
interface ProblemDetails {
  type: string;        // URI identifying the problem class
  title: string;       // short, localized
  status: number;
  detail?: string;
  instance?: string;   // mirrors X-Lyra-Request-ID
  errors?: Array<{ path: string; message: string }>;
  retryAfterMs?: number; // for 429 / 503
}
```

### 5.5 Workspace 数据（现有 —— 已在 `frontend/src/lib/queries.ts`）

`SidebarSession`、`SidebarProject`、`FileChange`、`DiffRow`、`TermLine`、
`GrepResult`、`FileLine`、`MCPServer` —— 保持现有形状；从 mock 移植
时给每个套一层 `{ items, nextCursor?, hasMore }` 做分页。

---

## 6. Schema 唯一权威

### 6.1 选型：**OpenAPI 3.1 + AsyncAPI 2.6**

- **OpenAPI 3.1** 描述每个 REST 端点、请求 / 响应、错误形状、
  安全方案。天然贴合 C/S 表面。
- **AsyncAPI 2.6** 描述 SSE 事件流——每个事件类型、payload、
  跟 run 生命周期的语义关系。

两者都是 YAML，两者工具链都好，两者都能给我们关心的所有语言
生成类型（TS、Go、Rust、Python、Swift）。

被拒方案：**Protobuf** —— REST 形状会被迫走 gRPC-Gateway，
AsyncAPI 式的 channel 描述在 proto3 里很弱。

### 6.2 布局

```
schemas/
├── openapi.yaml             # /v1/* REST endpoints + ProblemDetails + shared models
├── events.yaml              # AsyncAPI 2.6 for the /run SSE stream
├── shared/                  # JSON Schema fragments shared between the two
│   ├── Message.yaml
│   ├── Session.yaml
│   ├── Capabilities.yaml
│   └── …
└── generated/               # committed for review; CI rebuilds + diffs
    ├── ts/
    └── go/
```

### 6.3 CI 门禁

`npm run schema:check` 用 `openapi-typescript` + `oapi-codegen`
跑一遍 schema，把生成结果 diff 进 `generated/`，有任何漂移就
fail 构建。改契约的 PR 必须 schema + 生成代码同时落地。

### 6.4 版本规则

- **加**端点 / 事件 / 可选字段 → patch 提升。
- **加**必选字段 → minor 提升。
- **删**任何东西 → major 提升。旧版本至少保留一个发布周期，
  挂在旧 URL 前缀下。

`Capabilities.protocol.minClientVersion` 让后端可以用清晰的
`426 Upgrade Required` 拒掉太老的客户端。

---

## 7. 部署矩阵 —— 每个组合要接什么线

### 7.1 嵌入式后端 + 包装 Web / TUI 前端

- 前端读 `LYRA_BACKEND_URL`（安装器设置）—— 默认是写到本地
  config 的 `http://127.0.0.1:<port>`。
- `$XDG_CONFIG_HOME/lyra/token` 里的 token 启动时两端都读。
- 不用考虑 CORS（前后端同 loopback）。
- Loopback 上后端可以跳过 TLS；其他地方都得有。

### 7.2 独立服务后端 + 任意前端

- 运营方通过 env / config 提供 `LYRA_BACKEND_URL` + `LYRA_TOKEN`。
- 服务端强制 TLS；除非 URL 是 loopback，前端拒绝走明文 HTTP 登录。
- CORS 在服务端配 —— `Access-Control-Allow-Origin` 列允许的
  前端 origin。

### 7.3 Web 前端 + 任意后端（浏览器场景）

- Token 存储：同站时 HttpOnly cookie，否则 `sessionStorage`。
- 前端**必须**优雅处理 CORS preflight 失败 —— 显示"Backend at
  <url> didn't allow this origin"而不是静默错误。

### 7.4 TUI 前端 + 任意后端

- 没有 `EventSource` —— 用支持 SSE 的流式 HTTP 客户端
  （Node 用 `sse.js`、Python 用 `eventsource` 等）。
- Token 从环境变量 `LYRA_TOKEN` 或交互式 `lyra login` 取。

---

## 8. 实施 roadmap

| 阶段 | 范围 | 工期 |
| --- | --- | --- |
| **1. 协议冻结** | 写 `schemas/openapi.yaml` + `events.yaml`；codegen TS + Go；mock 后端讲真形状。 | 1 周 |
| **2. 鉴权 + 发现** | `/v1/info` / `/v1/capabilities` / `/v1/health` + 其他端点上 Bearer middleware。嵌入形态自动生成 token。 | 1 周 |
| **3. 真 run 路径** | `/v1/run` 接一个真 LLM（先一个 provider）；HITL 走 `/v1/run/{id}/permission`；SSE `Last-Event-ID` 续传；`/v1/run/{id}/cancel`。 | 2 周 |
| **4. 持久化 + sessions CRUD** | SQLite（嵌入式默认）或 Postgres（独立服务，大库时）—— sessions、messages、attachments。`/v1/sessions/{id}/messages` 分页。 | 1 周 |
| **5. Workspace 数据 + 工具** | 真文件系统 / git / ripgrep / pty / MCP 接到 `/v1/workspace/*`。`/v1/tools`、`/v1/providers`、`/v1/models`。 | 2 周 |
| **6. 前端形态** | TUI 前端原型走同一份协议 → 证明 m × n 真的跑得通。 | 2 周 |
| **7. 独立服务硬化**（晚一点） | TLS 必选 + 限流 + 静态 token 与 SSO/OAuth 双路、备份方案。**不做多租户**——多客户走多次部署。 | 2 周 |

---

## 9. 迁移 —— mock → real，按端点逐个走

对 §4 每一行单独应用：

1. Schema 落在 `schemas/*.yaml`（一个 PR，两边 codegen）。
2. Mock 后端 handler 保持当前行为但走生成的类型（编译期门禁
   抓线上格式）。
3. 真后端 handler 替换 mock body —— fixture 保留在 `LYRA_MOCK=1`
   后面给 E2E / demo 用。
4. 前端 pin 响应形状的测试自始至终绿。

这样**前端无法分辨自己在跟哪种后端讲话**，正是我们要的架构。

---

## 10. 未决问题

- [ ] **鉴权标准化**：嵌入式生成 token / 独立服务静态 token +
      可选 SSO-OAuth 一条路够不够？还是把 OAuth 当默认？
      （当前判断：默认静态 token，OAuth 在接公司 IdP 时按需开。）
- [ ] **工具执行位置**：只服务端，还是前端可以在 `RunInput`
      里声明 `tools` 让后端回调？后者支持"浏览器作为工具"
      （文件选择器、截图、剪贴板）但安全模型复杂化。
- [ ] **WebSocket vs SSE**：SSE 是单向 push + 幂等重连，贴合
      我们的场景。加 WS 的唯一理由是 HITL 双向流——但反向调用
      我们已经用 REST 覆盖。除非有什么逼着我们换，否则就 SSE。
- [ ] **嵌入式持久化故事**：SQLite 是显然选择，但要不要暴露
      文件路径让用户备份对话？
- [ ] **Sideload 信任模型**：只允许同源插件包，还是任何 URL +
      manifest 签名校验？影响 `/v1/plugins/*` 设计。

---

## 附录 A —— 文件位置

| 关注点 | 前端 | 后端 |
| --- | --- | --- |
| 流式 reducer（event → state） | `frontend/src/plugins/builtin/core-reducer/handlers.ts` | `internal/agui/events.go` |
| CUSTOM 事件 handler | `frontend/src/plugins/builtin/agui-handlers/index.ts` | `internal/agui/dsl.go` |
| REST 形状（前端侧） | `frontend/src/lib/queries.ts` | `internal/agui/rest.go` |
| HITL 审批 gateway | `frontend/src/domain/gateways/PermissionGateway.ts` + `frontend/src/infra/http/HttpPermissionGateway.ts` | `internal/agui/permissions.go` |
| Sideload manifest | `frontend/src/plugins/sideload.ts` | `internal/agui/plugins.go` |
| Base URL 配置 | `frontend/src/main/config.ts`（`AGUI_BASE`） | `internal/agui/server.go` |
| Fixture 数据 | — | `internal/agui/demos.go` / `refactor_demo_data.go` / `artifacts.go` |

## 附录 B —— 对标项目对比

| 项目 | 线上形状 | 我们为什么不一样 |
| --- | --- | --- |
| **kimi-code** | In-process typed RPC（`createRPC()`），无 HTTP | 只支持同进程 —— 不支持远程后端。我们要 m × n。 |
| **continue** | gRPC over webview postMessage | 跟 IDE 绑定；不能跨前端形态复用。 |
| **cline** | gRPC + protobuf | 同上；跟 VS Code 扩展模型绑死。 |
| **lobehub** | REST + SSE | 跟我们同一家族，验证了选型。 |
| **agent-chat-ui** | LangGraph SDK over SSE | 跟我们最接近；我们泛化到非 LangGraph 后端。 |
| **ag-ui-protocol**（官方） | SSE + JSON / protobuf | 我们的事件 schema 是它的严格子集 + Lyra 专属 CUSTOM 事件。 |

**对 Lyra 的启示**：

- 借 `requestApproval` 的 Promise 语义（来自 kimi-code）做 HITL 流：
  `POST /run/{id}/permission` 今天是 fire-and-forget，但服务端可以
  在它那边把请求 / 响应建模成一个 Promise，POST 到达时 resolve。
  前端不用知道。
- 借后台任务事件（来自 kimi-code）作为 `lyra.background.*` —— 见 §3.2.3。
- 借压缩事件（来自 kimi-code）放到 §3.2.3，Phase 4 上线时启用。
- 拒绝把类型化双向 RPC 当传输（kimi-code 风格）—— 撑不起 m × n 需求。
