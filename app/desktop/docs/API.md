# Lyra API Contract

> Single source of truth for the wire between any Lyra frontend and any Lyra
> backend. Designed for an **m × n deployment matrix** — frontend variant
> doesn't know which backend it's talking to, backend variant doesn't know
> which frontend is calling. The protocol is the only thing they share.
>
> Audience: anyone implementing a new frontend (web / packaged-web / TUI /
> mobile), porting the backend (embedded in a client / standalone server /
> managed cloud), or evolving the contract itself.

---

## 0. Architecture model

### 0.1 The m × n matrix

| ↓ Frontend / Backend → | Embedded (in-client) | Standalone server | Managed (multi-tenant) |
| --- | --- | --- | --- |
| **Web (browser)** | n/a (no client to embed in) | ✓ | ✓ |
| **Packaged web** (Wails / Tauri / Electron) | ✓ (loopback) | ✓ | ✓ |
| **TUI** (terminal) | ✓ (loopback) | ✓ | ✓ |
| **Mobile** (RN / native, future) | n/a | ✓ | ✓ |

Three frontend kinds × three backend kinds = 9 viable combinations. **Every
combination uses the same wire protocol.** No frontend imports backend types,
no backend assumes frontend co-location.

### 0.2 Consequences for the API design

- **Protocol and transport are separate concerns.** This document defines
  the *protocol* — method names, payload shapes, event sequences, error
  envelope. The *transport* (how bytes — or struct pointers — move
  between the two halves) lives in [`TRANSPORT.md`](./TRANSPORT.md).
  HTTP is one of six transport implementations; for Go-to-Go embedded
  deployments (e.g. Bubble Tea TUI + Go runtime) the transport is a
  direct function call with no serialization. **All transports speak
  the same protocol**, so this document doesn't change based on which
  one you pick.
- **Auth is required when crossing a process boundary.** HTTP / Wails
  IPC / sockets all require auth (Bearer token / OS-level ACL). The
  in-process case (Go ↔ Go same binary) trusts the caller — capability
  checks still happen inside the `CoreAPI` impl. See `TRANSPORT.md §6`
  for the per-transport auth matrix.
- **Capability discovery is non-optional.** Frontend X built today may talk
  to backend Y built tomorrow — features only enabled by both sides.
- **Schema is the only API.** OpenAPI + AsyncAPI files are the SSOT,
  type-generated to TS / Go / Rust / Python / whatever consumes them.
  No language-specific shortcuts.
- **Stateless where possible.** Backend may run behind a load balancer with
  N replicas. Session affinity is opt-in (via `X-Session-Affinity` header)
  not a baked-in assumption.
- **Streams must be resumable.** Network flakes, browser tabs sleep, TUI
  reconnects on `^Z fg`. SSE uses `Last-Event-ID` to replay missed events.

### 0.3 Frontend variants — what each needs

| Variant | Default transport | Notes |
| --- | --- | --- |
| **Web** | HTTP (`fetch` + `EventSource`) | Only option a browser exposes. CORS preflight; cookie or Bearer auth; relative URLs from served origin. |
| **Packaged web** (Wails / Tauri / Electron) | Wails IPC (embedded) / HTTP (remote) | When the backend is in the host process, skip HTTP and use the shell's IPC bridge. See `TRANSPORT.md §4.2`. |
| **TUI (Go)** | InProcess (embedded) / Socket (local) / HTTP (remote) | When both halves are Go and same-binary, the "transport" is a function call. See `TRANSPORT.md §4.1`. |
| **TUI (Node / Rust / Python)** | Socket (local) / HTTP (remote) | Can't embed our Go runtime; spawn `lyra-server` as a sibling process and connect via Unix socket / Named pipe. |

### 0.4 Backend variants — what each provides

| Variant | Listens on | Auth defaults | Persistence |
| --- | --- | --- | --- |
| **Embedded** | `127.0.0.1:0` only (no LAN exposure) | Generated token in local config, single-user | SQLite next to the client binary |
| **Standalone server** | `0.0.0.0:port`, configurable | Required: Bearer token / API key | SQLite (small) or Postgres (large) |
| **Managed cloud** | Behind LB / reverse proxy | OAuth + per-tenant token | Postgres, multi-tenant via workspace id |

All three speak the same wire protocol. Differences are only in
deployment / persistence / auth strength.

---

## 1. Status snapshot (2026-05-27)

| Surface | Today | Gap to ship m × n |
| --- | --- | --- |
| Streaming events (`/run` SSE) | All 16 AG-UI standard + 7 `lyra.*` CUSTOM events from a fixture DSL | Replace fixture with real LLM call + real tool execution. Add `Last-Event-ID` resume. |
| REST endpoints | 13 endpoints serving fixtures | Add capabilities / auth / sessions CRUD / messages pagination / attachments / providers / models / tools / cancel — see §4 |
| HITL approval | `POST /permission` unblocks an in-memory chan | Make idempotent + tie to a running run id (not a global chan). |
| Auth | None | Required from day one — Bearer token. See §3.2. |
| Workspaces | None | URL-prefixed (`/v1/workspaces/{ws}/…`) when multi-tenant. |
| Schema SSOT | Hand-typed shapes in `frontend/src/lib/queries.ts` | OpenAPI 3.1 + AsyncAPI 2.6 generated to TS + Go. See §6. |
| Versioning | None | URL prefix `/v1/` + `X-Lyra-Protocol-Version` header. |

Mock backend listens on `http://127.0.0.1:17171` today; frontends configure
that via `host.config.set("api.baseUrl", "…")`. **No code in the frontend
assumes the backend is local.**

---

## 2. Wire principles

The constant rules that every endpoint inherits. **Anything new MUST follow
these — they're what makes the m × n matrix actually work.**

### 2.1 Transport

Protocol invariants below are stated in HTTP terms because HTTP is the
common-denominator transport; non-HTTP transports (in-process / Wails IPC
/ Unix socket) map each row onto an equivalent primitive — see
[`TRANSPORT.md §5–6`](./TRANSPORT.md). Anything new lands here when it
constrains *what is sent*; landing in `TRANSPORT.md` when it constrains
*how it travels*.

| Topic | Rule |
| --- | --- |
| **Default transport** | HTTP/1.1 or HTTP/2 over TCP. TLS required for non-loopback. Alternative transports: see `TRANSPORT.md §4`. |
| **Content types** | Requests / responses default to `application/json; charset=utf-8`. Streams are `text/event-stream` over HTTP, length-prefixed JSON frames over sockets, channel of typed events for in-process. Uploads are `multipart/form-data`. |
| **Versioning** | URL prefix `/v1/`. Mismatch returns `426 Upgrade Required` with the supported versions in `Sunset` header. In-process clients import `pkg/coreapi` directly, so version skew is a compile error rather than a runtime 426. |
| **CORS** | Server backends echo `Access-Control-Allow-Origin`; embedded HTTP backends allow `null` + `127.0.0.1`. Non-HTTP transports have no CORS concept (no origin). |
| **Compression** | `Accept-Encoding: gzip, br` honoured for JSON responses; SSE streams are never compressed (latency > size). Socket / in-process transports never compress. |
| **Heartbeats** | SSE streams emit `:heartbeat\n\n` every 15s. Socket transports send a `{"type":"heartbeat"}` frame on the same cadence. In-process is exempt (channel closure is the liveness signal). |

### 2.2 Identity & versioning headers

Every request:

```http
Authorization: Bearer <token>
X-Lyra-Client: lyra-web/0.4.2          // <kind>/<version>
Idempotency-Key: 0193…d8b5             // UUID v7, required on POST/PUT/PATCH
Accept-Language: en-US, zh-CN;q=0.8    // for localized error messages
```

Every response:

```http
X-Lyra-Server: lyra-core/0.8.1
X-Lyra-Protocol-Version: 1.2.0         // semver of the wire contract
X-Lyra-Request-ID: 0193…d8b5           // echoed back for tracing
X-Lyra-Workspace: ws_abc               // when scoped
```

### 2.3 Auth

| Variant | Token source | Renewal |
| --- | --- | --- |
| Embedded | Generated at install, written to `$XDG_CONFIG_HOME/lyra/token` (client reads same file) | No renewal — regenerate on reinstall |
| Standalone | Operator-issued static token (env var `LYRA_TOKEN`) OR `POST /v1/auth/login` for username/password | `POST /v1/auth/refresh` returns a new pair |
| Managed | OAuth 2.1 + PKCE (`/v1/auth/authorize` + `/v1/auth/token`) | Refresh token rotation |

Frontend probes `GET /v1/info` (unauthenticated) to discover which mechanism
this backend uses, then dispatches to the right flow. **Backend MUST never
serve any non-`/v1/info`, non-`/v1/auth/*` endpoint without a valid token.**

### 2.4 Errors — RFC 7807 Problem Details

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

Every non-2xx response is a ProblemDetails JSON. Frontends normalise to
this via a single interceptor regardless of which backend variant they hit.

### 2.5 Idempotency

POSTs that mutate state require `Idempotency-Key`. Server stores the
`(key, response)` pair for 24h and replays the same response on retry —
covers the "user clicks Send, network flaps, client retries" case.

Applies to: `POST /run`, `POST /sessions`, `POST /attachments`,
`POST /messages/{id}/edit`, `POST /run/{id}/cancel`.

### 2.6 Pagination

Cursor-based:

```
GET /v1/sessions?limit=20&cursor=eyJpZCI6InNlc3NfMTIzIn0
→ { items: [...], nextCursor?: "...", hasMore: boolean }
```

No offset / total-count by default — both break on real-world workloads. A
dedicated `?countOnly=true` query exists for the rare case where the
frontend needs the total.

### 2.7 Time, IDs, locale

| Thing | Format |
| --- | --- |
| Timestamps | ISO-8601 with timezone: `"2026-05-27T12:34:56.789Z"`. Always UTC on the wire, frontends format via `Intl.DateTimeFormat`. |
| Durations | Milliseconds, integer (e.g. `runDurationMs: 1234`). |
| IDs | Server-generated UUID v7 by default (sortable + monotonic). Client-supplied IDs allowed where idempotency matters; server stores both `id` (server-assigned) and `clientId` (optional). |
| Locale | `Accept-Language` request header; server returns localized strings in error `title` / `detail` only. Domain data stays in source language. |

### 2.8 Backpressure & resumability

| Mechanism | Where | Behaviour |
| --- | --- | --- |
| **SSE heartbeats** | All streams | `:heartbeat\n\n` every 15s |
| **`Last-Event-ID`** | Frontend SSE reconnect | Server replays events with `id > Last-Event-ID` for the same `runId`, capped at a 30s replay window |
| **HTTP/2 flow control** | Long uploads / replays | Automatic, no app code |
| **Rate limit** | All endpoints | `429 + Retry-After: <seconds>`. Type `/problems/rate-limit`. |
| **Cancellation** | `/run` SSE | Client closes connection AND optionally calls `POST /run/{id}/cancel` for explicit cleanup |

---

## 3. Streaming events — `POST /v1/run` (SSE)

### 3.1 Request

```http
POST /v1/workspaces/{ws}/run HTTP/1.1
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

### 3.2 Response (SSE)

Each event is `id: <seq>\nevent: <type>\ndata: {…}\n\n`. `<seq>` is a
monotonic per-run integer used by `Last-Event-ID` for resume.

#### 3.2.1 AG-UI standard events (16) — backbone of the agent UX

| Group | Events |
| --- | --- |
| Lifecycle | `RUN_STARTED` / `RUN_FINISHED` / `RUN_ERROR` |
| Step | `STEP_STARTED` / `STEP_FINISHED` |
| Text | `TEXT_MESSAGE_START` / `_CONTENT` / `_END` / `_CHUNK` |
| Tool call | `TOOL_CALL_START` / `_ARGS` / `_END` / `_CHUNK` / `_RESULT` |
| Reasoning | `REASONING_MESSAGE_START` / `_CONTENT` / `_END` / `_CHUNK` + `THINKING_TEXT_MESSAGE_*` |
| Shared state | `STATE_SNAPSHOT` / `STATE_DELTA` (RFC 6902 JSON Patch) |
| History | `MESSAGES_SNAPSHOT` |
| Per-message activity | `ACTIVITY_SNAPSHOT` / `ACTIVITY_DELTA` |
| Extension | `CUSTOM` / `RAW` |

#### 3.2.2 Lyra CUSTOM events — `event.type === "CUSTOM"`, dispatched by `event.name`

| `name` | Payload | Purpose |
| --- | --- | --- |
| `lyra.plan` | `{ items: PlanItem[] }` | Replaces `state.plan` |
| `lyra.plan-block` | `{ messageId }` | Attaches a `plan` content block |
| `lyra.code-proposal` | `{ parentMessageId, lang, file, text }` | Diff proposal block |
| `lyra.search-results` | `{ parentMessageId, results }` | Search results block |
| `lyra.approval` | `{ requestId, parentMessageId, text, command, reason, risk?, scope?, target?, reversible? }` | HITL approval block |
| `lyra.approval-result` | `{ requestId, decision }` | Server confirms received decision |
| `lyra.telemetry` | free-form | Perf / debug signals |

#### 3.2.3 Reserved for future iterations (from kimi-code / agent-chat-ui audits)

| `name` | Payload | Source |
| --- | --- | --- |
| `lyra.interrupt` | `{ requestId, kind: "approve" \| "edit" \| "reject", display }` | LangGraph-style interrupt with structured display |
| `lyra.resume` | `{ requestId, decision }` | Counterpart to `interrupt` |
| `lyra.checkpoint` | `{ messageId, parentCheckpoint }` | Edit-and-re-run from a prior message |
| `lyra.meta` | `{ kind: "thumbs-up" \| "thumbs-down" \| "note" \| "bookmark", refId, value }` | RLHF feedback (ag-ui draft) |
| `lyra.subagent.spawned` / `.completed` / `.failed` | `{ parentRunId, subRunId, … }` | Nested agent calls |
| `lyra.background.started` / `.updated` / `.terminated` | `{ taskId, label, progress?, exitCode? }` | Long-running background tasks (kimi-code shape) |
| `lyra.compaction.started` / `.completed` | `{ summary, tokensBefore, tokensAfter }` | Context window compaction (Phase 4 backlog) |

All CUSTOM event payloads MUST have a Zod schema in
`frontend/src/protocol/agui/schemas.ts` AND a Go mirror generated from
`schemas/events.yaml` (see §6).

---

## 4. REST endpoints

URLs are workspace-scoped when the backend is multi-tenant
(`/v1/workspaces/{ws}/…`); the workspace prefix is dropped for embedded
single-user backends. Path templates below show the non-workspace form
for brevity.

### 4.1 Discovery & auth — required on every backend variant

| P | Method | Path | Purpose |
| --- | --- | --- | --- |
| P0 | `GET` | `/v1/info` | **Unauthenticated.** Returns `{ server, protocolVersion, authKinds, instanceLabel }` so a fresh frontend knows where it is and how to log in. |
| P0 | `GET` | `/v1/capabilities` | Requires auth. Returns supported events / providers / tools / features (see §5.1). |
| P0 | `GET` | `/v1/health` | Liveness probe. `{ status: "ok"\|"degraded", checks: {...} }`. |
| P0 | `POST` | `/v1/auth/login` | Username/password → token pair. Server may also accept API key. |
| P0 | `POST` | `/v1/auth/refresh` | Refresh-token rotation. |
| P0 | `POST` | `/v1/auth/logout` | Server-side token invalidation. |
| P1 | `GET` | `/v1/me` | Current user profile + workspace list. |
| P1 | `POST` | `/v1/auth/authorize` + `/v1/auth/token` | OAuth 2.1 + PKCE for managed cloud. |

### 4.2 Sessions, messages, runs — the core conversation surface

| P | Method | Path | Purpose |
| --- | --- | --- | --- |
| P0 | `POST` | `/v1/run` | Start a run, returns SSE stream (§3). |
| P0 | `POST` | `/v1/run/{runId}/cancel` | Explicit cancel; idempotent. |
| P0 | `POST` | `/v1/run/{runId}/permission` | HITL decision. Body: `{ requestId, decision, reason? }`. Replaces today's `POST /permission`. |
| P0 | `GET` | `/v1/sessions` | List sessions. Cursor-paginated. |
| P0 | `POST` | `/v1/sessions` | Create. Body: `{ title?, model?, metadata? }`. |
| P0 | `GET` | `/v1/sessions/{id}` | Read one (metadata + last activity). |
| P0 | `PATCH` | `/v1/sessions/{id}` | Rename / pin / metadata patch. |
| P0 | `DELETE` | `/v1/sessions/{id}` | Cascade delete. |
| P0 | `GET` | `/v1/sessions/{id}/messages` | Cursor-paginated history. Replaces "bulk `MESSAGES_SNAPSHOT` on every reconnect". |
| P1 | `POST` | `/v1/sessions/{id}/messages/{msgId}/edit` | Edit-and-re-run from a checkpoint. Returns `{ runId, checkpoint }` and emits `lyra.checkpoint` on the run SSE. |
| P1 | `POST` | `/v1/sessions/{id}/fork` | Branch a session at a checkpoint. |
| P2 | `GET` | `/v1/sessions/{id}/export?format=md\|json` | Server-rendered export. |

### 4.3 Workspace data (the "panels" frontends render)

These mirror the fixture endpoints Lyra already has. Backend variant decides
how to populate them (real filesystem / git / pty for embedded; managed
storage for cloud).

| P | Method | Path | Purpose |
| --- | --- | --- | --- |
| P0 | `GET` | `/v1/workspace/files-changed` | Diff overview. |
| P0 | `GET` | `/v1/workspace/diff?path=…` | Unified diff for one file. |
| P0 | `GET` | `/v1/workspace/file-head?path=…` | File preview. |
| P0 | `GET` | `/v1/workspace/grep?q=…` | Code search. |
| P0 | `GET` | `/v1/workspace/terminal/{runId}/output` | pty output stream for a tool's terminal session. |
| P0 | `GET` | `/v1/workspace/projects` | Project list (when backend manages multiple). |
| P0 | `GET` | `/v1/workspace/mcp-servers` | Registered MCP servers + status. |
| P1 | `POST` | `/v1/workspace/mcp-servers/{id}/reconnect` | Re-establish an MCP connection. |
| P1 | `GET` | `/v1/workspace/skills` | Available skills (kimi-code-style) when supported. |

### 4.4 Providers, models, tools — what the agent can use

| P | Method | Path | Purpose |
| --- | --- | --- | --- |
| P0 | `GET` | `/v1/providers` | LLM provider registry. |
| P0 | `POST` | `/v1/providers/{id}/test` | Validate creds. |
| P0 | `GET` | `/v1/models?provider=…` | Per-provider model list. |
| P0 | `GET` | `/v1/tools` | Tool registry with JSON-Schema params. |

### 4.5 Attachments & artefacts

| P | Method | Path | Purpose |
| --- | --- | --- | --- |
| P1 | `POST` | `/v1/attachments` | Multipart upload, returns `{ id, url, sha256, … }`. |
| P1 | `GET` | `/v1/attachments/{id}` | Download (signed URL preferred). |
| P1 | `DELETE` | `/v1/attachments/{id}` | Garbage-collect. |
| P2 | `GET` | `/v1/artefacts/{runId}/{name}` | Tool-emitted artefacts (rendered diagrams, generated files). |

### 4.6 Background tasks (kimi-code-inspired, when backend supports them)

| P | Method | Path | Purpose |
| --- | --- | --- | --- |
| P1 | `GET` | `/v1/background` | Active tasks across the workspace. |
| P1 | `POST` | `/v1/background/{taskId}/stop` | Stop a task. |
| P1 | `GET` | `/v1/background/{taskId}/output?tail=N` | Tail output. |

### 4.7 Plugin sideload (optional — frontend feature, but backend hosts the bundle)

| P | Method | Path | Purpose |
| --- | --- | --- | --- |
| P2 | `GET` | `/v1/plugins` | Plugin manifest (when marketplace ships). |
| P2 | `GET` | `/v1/plugins/{id}/*` | Plugin asset proxy. |

### 4.8 Feedback, version, telemetry

| P | Method | Path | Purpose |
| --- | --- | --- | --- |
| P2 | `POST` | `/v1/feedback` | RLHF — alternative to the `lyra.meta` CUSTOM event when frontend prefers REST. |
| P2 | `GET` | `/v1/version` | `{ server, protocolVersion, channel, releasedAt }`. |
| P2 | `GET` | `/v1/telemetry/optin` + `POST /v1/telemetry/optin` | Privacy consent toggle. |

---

## 5. Shapes

### 5.1 Capabilities — what a backend exposes

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
    kind: "embedded" | "standalone" | "managed";
    instanceLabel?: string;    // shown in UI, e.g. "Lyra Cloud (us-east-1)"
    region?: string;
  };
}
```

Frontend treats every `features.*` as `false` by default — it MUST NOT
crash if the backend omits a feature it doesn't implement.

### 5.2 Info — pre-auth probe

```ts
interface ServerInfo {
  server: { name: string; version: string };
  protocolVersion: string;
  authKinds: Array<"bearer" | "oauth" | "apiKey" | "anonymous">;
  instanceLabel?: string;
  loginUrl?: string;          // OAuth start URL when relevant
  brandingUrl?: string;       // logo / theme override when managed
}
```

### 5.3 Core conversation shapes

```ts
type SessionStatus = "running" | "waiting" | "idle";

interface Session {
  id: string;
  workspaceId: string;
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

### 5.4 ProblemDetails — error envelope

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

### 5.5 Workspace data (existing — already in `frontend/src/lib/queries.ts`)

`SidebarSession`, `SidebarProject`, `FileChange`, `DiffRow`, `TermLine`,
`GrepResult`, `FileLine`, `MCPServer` — keep current shapes; wrap each in
`{ items, nextCursor?, hasMore }` for pagination when porting from mock.

---

## 6. Schema source of truth

### 6.1 Decision: **OpenAPI 3.1 + AsyncAPI 2.6**

- **OpenAPI 3.1** describes every REST endpoint, request/response, error
  shape, security scheme. Native fit for the C/S surface.
- **AsyncAPI 2.6** describes the SSE event stream — every event type, its
  payload, its semantic relationship to the run lifecycle.

Both are YAML, both well-tooled, both generate types in every language we
care about (TS, Go, Rust, Python, Swift).

Rejected alternative: **Protobuf** — would force gRPC-Gateway for the REST
shape and AsyncAPI-style channel descriptions are weak in proto3.

### 6.2 Layout

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

### 6.3 CI gate

`npm run schema:check` runs `openapi-typescript` + `oapi-codegen` against
the schemas, diffs the output against `generated/`, fails the build on any
drift. PRs touching the contract MUST land schema + generated code together.

### 6.4 Versioning rules

- **Adding** an endpoint / event / optional field → patch bump.
- **Adding** a required field → minor bump.
- **Removing** anything → major bump. Old version stays available at the
  old URL prefix for one release cycle minimum.

`Capabilities.protocol.minClientVersion` lets the backend reject ancient
clients with a clear `426 Upgrade Required`.

---

## 7. Deployment matrix — what each combo needs to wire

### 7.1 Embedded backend + packaged-web / TUI frontend

- Frontend reads `LYRA_BACKEND_URL` (set by the installer) — defaults to
  `http://127.0.0.1:<port>` written into local config.
- Token in `$XDG_CONFIG_HOME/lyra/token` is read by both sides at startup.
- No CORS concerns (frontend and backend on same loopback).
- Backend MAY skip TLS on loopback; everywhere else it's required.

### 7.2 Standalone server backend + any frontend

- Operator provides `LYRA_BACKEND_URL` + `LYRA_TOKEN` via env / config.
- Server enforces TLS; frontends refuse to log in over plain HTTP unless
  the URL is loopback.
- CORS configured on the server side — `Access-Control-Allow-Origin` lists
  the allowed frontend origins.

### 7.3 Managed cloud backend + web / packaged-web frontend

- Frontend ships with a default `LYRA_BACKEND_URL` pointing at the cloud.
- `GET /v1/info` advertises `authKinds: ["oauth"]` → frontend kicks off
  OAuth 2.1 + PKCE.
- Per-tenant workspace IDs in the URL: `/v1/workspaces/{ws}/…`.

### 7.4 Web frontend + any backend (browser case)

- Token storage: HttpOnly cookie if same-site, else `sessionStorage`.
- Frontend MUST handle CORS preflight failures gracefully — surface
  "Backend at <url> didn't allow this origin" instead of a silent error.

### 7.5 TUI frontend + any backend

- No `EventSource` — use a streaming HTTP client that supports SSE
  (`sse.js` for Node, `eventsource` for Python, etc.).
- Token from env var `LYRA_TOKEN` or interactive `lyra login`.

---

## 8. Implementation roadmap

| Phase | Scope | Effort |
| --- | --- | --- |
| **1. Protocol freeze** | Author `schemas/openapi.yaml` + `events.yaml`; codegen TS + Go; mock backend speaks the real shapes. | 1 week |
| **2. Auth + discovery** | `/v1/info` / `/v1/capabilities` / `/v1/health` + Bearer middleware on every other endpoint. Embedded variant ships with auto-generated token. | 1 week |
| **3. Real run path** | Wire `/v1/run` to a real LLM (one provider); HITL via `/v1/run/{id}/permission`; SSE `Last-Event-ID` resume; `/v1/run/{id}/cancel`. | 2 weeks |
| **4. Persistence + sessions CRUD** | SQLite (embedded / standalone) or Postgres (managed) — sessions, messages, attachments. Pagination on `/v1/sessions/{id}/messages`. | 1 week |
| **5. Workspace data + tools** | Real filesystem / git / ripgrep / pty / MCP wiring for the `/v1/workspace/*` endpoints. `/v1/tools`, `/v1/providers`, `/v1/models`. | 2 weeks |
| **6. Frontend variants** | TUI frontend prototype against the same protocol → proves the m × n actually works. | 2 weeks |
| **7. Managed cloud** (later) | OAuth 2.1, workspace isolation, multi-region routing, rate limiting. | 3–4 weeks |

---

## 9. Migration — mock → real, per endpoint

Apply to every row in §4 individually:

1. Schema lands in `schemas/*.yaml` (one PR, both sides codegen).
2. Mock backend handler keeps current behaviour but goes through the
   generated types (compile-time gate on the wire format).
3. Real backend handler replaces the mock body — fixtures preserved
   behind `LYRA_MOCK=1` for E2E / demos.
4. Frontend tests that pin the response shape stay green throughout.

This way **the frontend can't tell which backend variant it's talking
to**, which is exactly the architecture we want.

---

## 10. Open questions

- [ ] **Auth standardisation**: do we commit to OAuth 2.1 + PKCE for managed,
      static token for embedded / standalone? Or invent something custom?
- [ ] **Workspace URL shape**: `/v1/workspaces/{ws}/…` everywhere (clean
      multi-tenant) or only when the backend opts in (simpler embedded)?
- [ ] **Tool execution location**: server-side only, or can a frontend declare
      `tools` in the `RunInput` that the backend calls back into? Latter
      enables "browser as a tool" (file picker, screenshot, clipboard) but
      complicates the security model.
- [ ] **WebSocket vs SSE**: SSE is one-way push + idempotent reconnect, which
      fits our use case. The only reason to add WS would be bidirectional
      streaming for HITL — but we have REST for the reverse calls. Keep SSE
      unless something forces our hand.
- [ ] **Persistence story for embedded**: SQLite is the obvious choice but
      do we expose its file path so users can back up their conversations?
- [ ] **Sideload trust model**: only same-origin plugin bundles, or any URL
      with manifest signature verification? Affects `/v1/plugins/*` design.

---

## Appendix A — File locations

| Concern | Frontend | Backend |
| --- | --- | --- |
| Streaming reducer (event → state) | `frontend/src/plugins/builtin/core-reducer/handlers.ts` | `internal/agui/events.go` |
| CUSTOM event handlers | `frontend/src/plugins/builtin/agui-handlers/index.ts` | `internal/agui/dsl.go` |
| REST shapes (frontend side) | `frontend/src/lib/queries.ts` | `internal/agui/rest.go` |
| HITL approval gateway | `frontend/src/domain/gateways/PermissionGateway.ts` + `frontend/src/infra/http/HttpPermissionGateway.ts` | `internal/agui/permissions.go` |
| Sideload manifest | `frontend/src/plugins/sideload.ts` | `internal/agui/plugins.go` |
| Base URL config | `frontend/src/main/config.ts` (`AGUI_BASE`) | `internal/agui/server.go` |
| Fixture data | — | `internal/agui/demos.go` / `refactor_demo_data.go` / `artifacts.go` |

## Appendix B — Comparison with peer projects

| Project | Wire shape | Why we chose differently |
| --- | --- | --- |
| **kimi-code** | In-process typed RPC (`createRPC()`), no HTTP | Same-process only — doesn't support remote backend. We need m × n. |
| **continue** | gRPC over webview postMessage | IDE-coupled; not portable across frontend variants. |
| **cline** | gRPC + protobuf | Same; tied to VS Code extension model. |
| **lobehub** | REST + SSE | Same family as ours, validates the choice. |
| **agent-chat-ui** | LangGraph SDK over SSE | Closest to our shape; we generalise to non-LangGraph backends. |
| **ag-ui-protocol** (official) | SSE + JSON / protobuf | Our event schema is a strict subset + Lyra-specific CUSTOM events. |

**Take-aways for Lyra**:

- Borrow `requestApproval` Promise-shape semantics (from kimi-code) for the
  HITL flow: `POST /run/{id}/permission` is fire-and-forget today, but the
  server can model the request/response as a Promise on its side and resolve
  it when the POST lands. Frontend doesn't need to know.
- Borrow background-task events (from kimi-code) as `lyra.background.*` —
  see §3.2.3.
- Borrow compaction events (from kimi-code) for §3.2.3 when Phase 4 lands.
- Reject typed bidirectional RPC as the transport (kimi-code style) —
  doesn't survive the m × n requirement.
