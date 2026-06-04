# Lyra Runtime 前后端对接验证报告

> **日期**：2026-06-03（初测） · **2026-06-04（复测：§2 全部修复，见下）**
> **被测后端**：`serverInfo.name = "runtime"`, `version = "dev"`（`serverID = runtime/dev`），监听 `http://127.0.0.1:17171`，HTTP transport。
> **被测前端**：本仓库 frontend（Vite dev，`http://localhost:5173`，HTTP transport 直连 :17171）。
> **方法**：curl 逐方法打 wire（握手 / sessions / runs + SSE / 历史读），对照 [`API.md`](./API.md) + [`TRANSPORT.md`](./TRANSPORT.md) 定稿（`protocolVersion 2026-06-03`）。
>
> 本文是**某次具体后端构建**的对接快照，不是契约。契约以 `API.md` / `TRANSPORT.md` 为准；后端修复后请重测并更新本文或删除。

---

## 0. 结论

**通道全打通，前端无需改动即可对接。** 实时握手 + 流式对话端到端正常（真实模型 `deepseek-v4-flash` 产出文本）。

**2026-06-04 复测：§2 列的 P1 + P2 全部已由后端修复**（详见 §2 各条 ✅）。额外改进：后端现已**强制握手**——
业务方法在 `runtime.initialize` 前调用返 `capability_not_negotiated` + 合规 ProblemData（API.md §3）。无新增回归。

---

## 1. 已验证通过

| 环节 | 结果 |
| --- | --- |
| `GET /v2/health` | ✅ 200，body 含 `ok:true`（额外带 `status`/`checks`，前端忽略未知字段） |
| `GET /v2/info` | ✅ `protocolVersion: "2026-06-03"`，capabilities / serverInfo 齐全 |
| `runtime.initialize` | ✅ 响应匹配 `InitializeResponse`；`X-Method` / `X-Server` 响应头齐全 |
| `sessions.create` / `sessions.list` | ✅ envelope + `Page<Session>` shape 正确 |
| `providers.list` / `models.list` / `runs.list` / `workspace.*` | ✅ 返回正确 envelope（dev 环境多为空集） |
| **`runs.start` + SSE 全流式** | ✅ `run.started → item.started → item.delta×N → item.completed → run.finished{completed}`；`eventId` `evt_…` root 流内单调；`durable` 标注正确；`usage.byModel` + `costUsd` 齐全 |
| 浏览器 CORS | ✅ 反射式（回显请求 header），Vite origin 可直连；SSE `?conn=` + `Last-Event-Id` 不受阻 |

实时流 `item.completed` 的文本**正确**：`"Hello! I'm Lyra, a general-purpose AI coding agent. How can I help you today?"`

---

## 2. 后端待修 — ✅ 已全部修复（2026-06-04 复测）

> 下列每条均已复测通过；正文保留原始问题描述备查。复测会话 `ses_d3717e32…` / run `run_0269c72e…`。

### P1 — 会直接坏 UI

**1.1 `items.list` 历史里 agentMessage 文本翻倍** — ✅ 已修：返回 `"Hi! I'm Lyra, your AI coding agent…"`，无重复。

同一 run 的实时 `item.completed` 正确，但 `items.list` 返回的持久化版本每个 token 重复：

```text
"Hello!! I I'm'm Ly Lyrara,, a a general general-purpose-purpose AI AI coding coding agent agent.. How How can can I I help help you you today today??"
```

推断：落库时把 `item.delta` 增量与 `item.completed` 终值都拼了，或 delta 拼了两遍。**实时流不受影响，仅历史 hydration 受影响**。
契约：`item.completed` 是权威终态，`items.list` 应返回与之一致的 completed Item（API.md §0.1 / §5 / §7.4）。

**1.2 `items.list` 的 item `runId` 为空 + `runs: []`** — ✅ 已修：item 带 `runId: run_…`，`runs[]` 含完整 `RunRef`（`status:"finished"` + `outcome` + `finishedAt`），run 树可还原。

```json
{ "items": [ { "id": "item_…_1", "runId": "", "status": "completed", "type": "userMessage", … } ],
  "runs": [] }
```

`ItemBase.runId` 为空、且 `runs` 数组为空 → 前端无法按 `runId` 把 item 归到 Run、也无法用 `parentRunId`/`spawnedByItemId` 还原 run 树。
契约：`items.list` 必须返回非空 `item.runId` 及这些 item 所属的 `RunRef[]`（API.md §4.3 / §7.4 / §10.3）。

### P2 — 契约违规，当前前端能兜底（仍应修）

**2.1 业务资源 id 缺类型前缀** — ✅ 已修：`ses_…` / `run_…` / `item_…` 全带前缀。

`session.id` / `run.id` 是裸 UUID（如 `fe4d489f-…`、`63cb0ae4-…`），缺 `ses_` / `run_` 前缀。
（`item_…` / `evt_…` 有前缀，后端自身不一致。）契约：API.md §2.2 所有业务 id 带类型前缀。

**2.2 `item.createdAt` 为 Go 零值时间** — ✅ 已修：返回真实 ISO-8601 时间。

`"createdAt": "0001-01-01T00:00:00Z"`。前端 `formatTime` 会兜底成当前时间，但应给真实创建时间。契约：API.md §4.3 `ItemBase.createdAt` 为 ISO-8601。

**2.3 `sessions.create` 的 `cwd` 为空、`metadata` 为 null** — ✅ 已修：`cwd` 缺省为 `/Users/tangerg`，`metadata` 为 `{}`。

返回 `cwd: ""` —— 契约应缺省为 `ServerInfo.cwd`（`/Users/tangerg`，API.md §7.2 / §0.2）。
返回 `metadata: null` —— 契约 `Session.metadata` 是 `Record<string, unknown>`，期望 `{}`（API.md §4.1）。

---

## 3. 非问题（确认无碍）

- `/v2/health` 多带 `status`/`checks`、`/v2/info` 多带 `agentDocs`/`endpoints`/`serverID`/`transport` —— 前端忽略未知字段（前向兼容，API.md §11）。
- `capabilities.events` 不含 `state.*` —— 后端不产出共享状态，合法（无共享状态时 `state.snapshot` 可省，API.md §5.3）。
- CORS `Access-Control-Allow-Origin: *` 同时带 `Access-Control-Allow-Credentials: true` —— 前端 `fetch` 不带 credentials，无碍；属后端 latent 不规范（凭证模式下 `*` 非法），建议收紧为回显具体 origin。

---

## 4. 前端

无需改动即可对接。一处 latent 小瑕疵（非本次阻塞，反射式 CORS 下不触发）：HTTP transport 的 `authHeaders` 在 `lastEventId` 置位后会把 `Last-Event-Id` 也带到 **POST** 上；`Last-Event-Id` 是 SSE 重连语义，本应只在 SSE GET 上发。后端 CORS 收紧成白名单后这会成为问题，届时一并修。

---

> 重测命令见提交历史中的 wire 探针；后端修复 P1/P2 后请重跑 `runs.start` + `items.list` 两条链路复核。
