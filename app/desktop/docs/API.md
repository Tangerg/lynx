# Lyra API Contract

> Single source of truth for the wire between the Lyra frontend (React/TS via
> `@ag-ui/client`) and the backend (today: Go mock at `internal/agui/`; tomorrow:
> real LLM provider + tool execution + persistence).
>
> Audience: anyone wiring a new endpoint, adding a CUSTOM event, or swapping the
> mock for a real backend. Cross-referenced from `frontend/ARCHITECTURE.md` §5
> (AG-UI protocol layer) and `frontend/src/domain/` (typed gateway contracts).

---

## 0. Status snapshot (2026-05-27)

| Surface | Today | Real-backend gap |
| --- | --- | --- |
| Streaming events (`POST /run` SSE) | All 16 AG-UI standard events + 7 `lyra.*` CUSTOM events emitted from fixture DSL | Replace fixture DSL with real LLM call + tool execution. Protocol unchanged. |
| REST sidebar / workspace data | 8 GET endpoints serve hard-coded fixtures | Wire to real filesystem / git / ripgrep / pty / MCP. |
| HITL approval | `POST /permission` unblocks an in-memory chan | Surface decision to the real tool-execution path (abort vs resume). |
| Plugin sideload | `GET /plugins` returns empty manifest | Optional — only required if a marketplace ships. |
| Auth / multi-user | Not present | Add when product needs it. Single-user desktop today. |
| Persistence | None (process-local) | Required for "history survives launch". |

**Mock base URL**: `http://127.0.0.1:17171` (defined in `frontend/src/main/config.ts` as `AGUI_BASE`).
Override at runtime via plugin config: `host.config.set("api.baseUrl", "https://…")`.

---

## 1. Streaming events — `POST /run` (SSE)

Single endpoint, one body, one event stream out. This is the spine of the agent UX.

### 1.1 Request

```http
POST /run HTTP/1.1
Content-Type: application/json
Accept: text/event-stream
```

```ts
interface RunInput {
  /** Conversation / thread id. Lyra calls it sessionId. */
  threadId: string;
  /** Optional run id; backend generates if omitted. Same `runId` is echoed
   *  back on every event so the reducer can correlate. */
  runId?: string;
  /** Conversation history + the new user turn appended. */
  messages: Message[];
  /** Last `shared` state the backend handed us (for resume / reconnect). */
  state?: Record<string, unknown>;
  /** Tools the backend is allowed to call (when running tool use). */
  tools?: ToolSpec[];
  /** File / URL / selection references the user attached. */
  context?: ContextItem[];
  /** Lyra extensions — non-AG-UI but harmless if the backend ignores them. */
  model?: string;
  mode?: "agent" | "chat" | "plan"; // composer mode
  attachments?: string[]; // attachment ids (see §2.B `POST /attachments`)
  capabilities?: ClientCapabilities; // what CUSTOM events the client renders
}
```

`Message`, `ToolSpec`, `ContextItem` shapes follow
[`@ag-ui/core`](https://github.com/ag-ui-protocol/ag-ui/tree/main/sdks/typescript/packages/core).

### 1.2 Response (SSE)

`Content-Type: text/event-stream`. Each event is a JSON-encoded `BaseEvent`
emitted as `data: {…}\n\n`. The frontend reducer at
`frontend/src/protocol/agui/reducer.ts` folds each event into the per-session
`AgentViewState`.

#### 1.2.1 AG-UI standard events (16)

| Group | Events | Purpose |
| --- | --- | --- |
| Lifecycle | `RUN_STARTED` / `RUN_FINISHED` / `RUN_ERROR` | one user turn boundary |
| Step | `STEP_STARTED` / `STEP_FINISHED` | sub-phases (planning, searching, executing) |
| Text | `TEXT_MESSAGE_START` / `_CONTENT` / `_END` / `_CHUNK` | assistant reply, streamed token deltas |
| Tool call | `TOOL_CALL_START` / `_ARGS` / `_END` / `_CHUNK` / `_RESULT` | function-call lifecycle |
| Reasoning | `REASONING_MESSAGE_START` / `_CONTENT` / `_END` / `_CHUNK` + `THINKING_TEXT_MESSAGE_*` | Claude-style extended thinking |
| Shared state | `STATE_SNAPSHOT` / `STATE_DELTA` (RFC 6902 JSON Patch) | backend-owned `shared` blob |
| Bulk history | `MESSAGES_SNAPSHOT` | reconnect / thread-switch rehydrate |
| Per-message activity | `ACTIVITY_SNAPSHOT` / `ACTIVITY_DELTA` | structured side-data scoped to one message |
| Extension | `CUSTOM` / `RAW` | application-defined |

#### 1.2.2 Lyra CUSTOM events (`event.type === "CUSTOM"`, distinguished by `event.name`)

| `name` | Payload | Reducer / handler |
| --- | --- | --- |
| `lyra.plan` | `{ items: PlanItem[] }` | replaces `state.plan` |
| `lyra.plan-block` | `{ messageId: string }` | attaches a `plan` content block to a message |
| `lyra.code-proposal` | `{ parentMessageId, lang, file, text }` | attaches a `code` block (diff proposal) |
| `lyra.search-results` | `{ parentMessageId, results: SearchResult[] }` | attaches a `search` block |
| `lyra.approval` | `{ parentMessageId, requestId, text, command, reason, risk?, scope?, target?, reversible? }` | materialises an `approval` block (status="requires-action") |
| `lyra.approval-result` | `{ requestId, decision: "approved" \| "declined" }` | flips the matching `approval` block to status="complete" + stamps `decision` |
| `lyra.telemetry` | free-form | optional perf / debug signals |

Schemas live in `frontend/src/protocol/agui/schemas.ts` (Zod) — re-use those for
generating Go structs.

#### 1.2.3 Proposed new CUSTOM events (when these features land)

| `name` | Maps to | Source idea |
| --- | --- | --- |
| `lyra.interrupt` | inline HITL pause (not approval, more like LangGraph interrupt) | agent-chat-ui |
| `lyra.resume` | response to an interrupt (user picks `approve` / `edit` / `reject`) | agent-chat-ui |
| `lyra.checkpoint` | `{ messageId, parentCheckpoint }` — used by "edit message + re-run" | agent-chat-ui |
| `lyra.meta` | `{ kind: "thumbs-up" \| "thumbs-down" \| "note" \| "bookmark", refId, value }` | ag-ui draft MetaEvent |
| `lyra.subagent.*` | nested agent invocation lifecycle | cline |

---

## 2. REST endpoints

### 2.A Existing (mock today, replace with real impl)

All return JSON. `*` marks the ones the frontend already calls.

| Method | Path | Frontend caller | Response | Notes |
| --- | --- | --- | --- | --- |
| `POST` | `/run` | `useAgentSession.runAgent` * | SSE stream (§1) | Body = `RunInput`. |
| `POST` | `/permission` | `HttpPermissionGateway.submit` * | `{}` or 4xx | Body = `{ requestId, decision }`. |
| `GET` | `/health` | (none) | `{ status: "ok", version, uptimeMs }` | Liveness probe — surface in Connection Settings. |
| `GET` | `/sessions` | `useSessions` * | `SidebarSession[]` | Sidebar list. |
| `GET` | `/projects` | `useProjects` * | `SidebarProject[]` | Workspace list. |
| `GET` | `/files-changed` | `useFilesChanged` * | `FileChange[]` | Diff view. |
| `GET` | `/diff` | `useDiff` * | `DiffRow[]` | Unified diff rows. |
| `GET` | `/terminal` | `useTerminal` * | `TermLine[]` | Terminal view. |
| `GET` | `/grep` | `useGrep` * | `GrepResult` | Code search. |
| `GET` | `/file-head` | `useFileHead` * | `FileLine[]` | File preview. |
| `GET` | `/mcp-servers` | `useMCPServers` * | `MCPServer[]` | MCP server list. |
| `GET` | `/plugins` | `loadSideloadedPlugins` * | `PluginManifest[]` | Sideload manifest. |
| `GET` | `/plugins/{id}/*` | dynamic `import()` | JS bundle / asset | Plugin asset proxy. |

Response shapes are declared in `frontend/src/lib/queries.ts` (the only file
that types the wire format for these). Treat that file as the
**frontend-side schema source**.

### 2.B New (required for real backend)

Sorted by priority — P0 must land before "real LLM hooked up", P1 before
"shippable to a second user", P2 is nice-to-have.

| P | Method | Path | Body / response | Why |
| --- | --- | --- | --- | --- |
| **P0** | `GET` | `/capabilities` | `{ events: string[], providers: string[], tools: ToolSpec[], features: Record<string, boolean> }` | Lets the frontend gate UI on what the backend actually supports. ag-ui-official-recommended. |
| **P0** | `POST` | `/run/{runId}/cancel` | `{}` or 409 | Explicit cancel beats relying only on client-side `agent.abortRun()`. |
| **P0** | `POST` | `/sessions` | `{ title?, model? }` → `SidebarSession` | Create. |
| **P0** | `PATCH` | `/sessions/{id}` | `{ title?, pinned? }` → `SidebarSession` | Rename / pin. |
| **P0** | `DELETE` | `/sessions/{id}` | `{}` or 404 | Delete + cascade. |
| **P0** | `GET` | `/sessions/{id}/messages?before=<msgId>&limit=N` | `{ messages: Message[], cursor?: string }` | Pagination — replaces "send full `MESSAGES_SNAPSHOT` on every reconnect". |
| **P1** | `POST` | `/messages/{id}/edit` | `{ text }` → `{ runId, checkpoint }` | Edit-and-re-run from a prior message. Emits a `lyra.checkpoint` event. |
| **P1** | `POST` | `/attachments` | multipart → `{ id, url, kind, size, sha256 }` | File / image uploads referenced by messages. |
| **P1** | `GET` | `/providers` | `Provider[]` | LLM provider registry (OpenAI / Anthropic / local / …). |
| **P1** | `POST` | `/providers/{id}/test` | `{ apiKey, baseUrl? }` → `{ ok, modelsAvailable }` | Validate creds without listing models. |
| **P1** | `GET` | `/models?provider=<id>` | `Model[]` | Per-provider model list. |
| **P1** | `GET` | `/tools` | `ToolSpec[]` (JSON Schema for params) | Drives composer autocomplete + tools workspace view. |
| **P2** | `POST` | `/feedback` | `{ refId, kind, value }` | Alternative to `lyra.meta` if you prefer REST. |
| **P2** | `GET` | `/export/sessions/{id}.{md\|json}` | file download | Server-rendered export (the client-side `conversation-export` plugin still works without this). |
| **P2** | `GET` | `/version` | `{ semver, commit, channel }` | Frontend version-check banner. |
| **P2** | `GET` | `/telemetry/optin` / `POST /telemetry/optin` | `{ enabled }` | Privacy consent toggle. |

### 2.C Auth / multi-user (when product needs it)

| Method | Path | Notes |
| --- | --- | --- |
| `POST` | `/auth/login` | OAuth code exchange or API key swap. |
| `POST` | `/auth/refresh` | Refresh-token rotation. |
| `POST` | `/auth/logout` | Server-side session invalidation. |
| `GET` | `/me` | Current user profile. |
| `GET` | `/workspaces` | Multi-workspace switcher. |

Current Lyra targets single-user desktop — defer until shipped to a second user.

---

## 3. Request / response shapes

> These are the wire-level shapes. Where the frontend has them already, the
> file path is given. New shapes should be added to `frontend/src/lib/queries.ts`
> or a new `frontend/src/domain/models/` file and **mirrored in Go**.

### 3.1 Shared

```ts
// AG-UI standard — re-exported from @ag-ui/core
interface Message {
  id: string;
  role: "user" | "assistant" | "system" | "tool" | "developer";
  content?: string;
  toolCalls?: ToolCall[];
  toolCallId?: string; // role: "tool"
}

interface ToolSpec {
  name: string;
  description?: string;
  parameters: JsonSchema; // JSON Schema for arguments
}

interface ContextItem {
  kind: "file" | "url" | "selection" | "image";
  // …kind-specific fields
}

interface ClientCapabilities {
  customEvents: string[]; // names the client renders, e.g. ["lyra.plan", "lyra.approval"]
  contentBlocks: string[]; // block kinds the client can render
}
```

### 3.2 Sidebar (existing — `frontend/src/lib/queries.ts`)

```ts
type SessionStatus = "running" | "waiting" | "idle";

interface SidebarSession {
  id: string;
  title: string;
  status: SessionStatus;
  model: string;
  time: string; // ISO-8601 ideally; Lyra currently treats as opaque display string
}

interface SidebarProject {
  id: string;
  name: string;
  branch: string;
  active?: boolean;
}

interface FileChange {
  path: string;
  change: "add" | "mod" | "del";
  added: number;
  removed: number;
}

interface MCPServer {
  id: string;
  name: string;
  desc: string;
  tools: number;
  status: "active" | "idle" | "error";
  icon: string;
}

// + DiffRow / TermLine / GrepResult / FileLine — see queries.ts
```

### 3.3 New (proposed)

```ts
interface Capabilities {
  events: string[];           // ["TEXT_MESSAGE_START", …, "lyra.plan", …]
  providers: string[];        // ["openai", "anthropic", "local"]
  tools: ToolSpec[];          // discovered tool set
  features: {
    multimodal: boolean;
    reasoning: boolean;       // emits REASONING_MESSAGE_*
    checkpoints: boolean;     // supports edit-and-re-run
    interrupts: boolean;      // supports human-in-the-loop interrupts
    attachments: boolean;     // accepts /attachments uploads
  };
  protocol: {
    version: string;          // semver of the wire contract
    runEndpoint: string;      // override of /run if non-default
  };
}

interface Provider {
  id: string;                 // "openai", "anthropic", "ollama"
  label: string;
  authKind: "apiKey" | "oauth" | "none";
  status: "configured" | "missing-key" | "error";
  icon?: string;
}

interface Model {
  id: string;                 // "claude-sonnet-4-5"
  provider: string;
  label: string;
  contextWindow: number;
  pricing?: { input: number; output: number; currency: string };
  capabilities?: { vision: boolean; reasoning: boolean; tools: boolean };
}

interface Attachment {
  id: string;
  url: string;                // server-resolvable URL (may be relative)
  kind: "image" | "file";
  mime: string;
  size: number;
  sha256: string;
  name: string;
}
```

### 3.4 Error envelope (RFC 7807)

All non-2xx responses follow Problem Details:

```ts
interface ProblemDetails {
  type: string;       // URI identifying the problem class
  title: string;      // short human-readable summary
  status: number;     // HTTP status mirror
  detail?: string;    // detailed explanation
  instance?: string;  // request id / trace id for support
  // extensions allowed — e.g. validation errors:
  errors?: Array<{ path: string; message: string }>;
}
```

The frontend's `lib/http.ts` should normalise to this regardless of transport.

---

## 4. Transport conventions

| Topic | Convention |
| --- | --- |
| **Protocol** | HTTP/1.1 or HTTP/2 over TCP. SSE for `/run`. REST for everything else. |
| **No WebSocket** | SSE + plain POST endpoints cover every push + reverse-call we need. WS adds half-duplex state without buying anything for an LLM chat use case. |
| **Versioning** | URL prefix `/v1/…`. Frontend pins via `capabilities.protocol.version`. Major bumps require both sides updated. |
| **Auth** | `Authorization: Bearer <token>` once auth lands. Mock backend ignores. |
| **Idempotency** | `Idempotency-Key: <uuid>` required on `POST /run`, `POST /sessions`, `POST /attachments`. Server replays the same response within 24h. |
| **Cancellation** | `POST /run/{runId}/cancel` (explicit) **and** the client closes the SSE stream (implicit). Backend must handle both. |
| **Rate limits** | `429 + Retry-After: <seconds>`. Errors surface as ProblemDetails with `type: "/problems/rate-limit"`. |
| **Pagination** | Cursor-based (`?cursor=…&limit=…`). Total counts only on dedicated endpoints. |
| **Time** | All timestamps ISO-8601 with timezone (`2026-05-27T12:34:56.789Z`). Frontend formats via `Intl.DateTimeFormat`. |
| **IDs** | Server-generated where possible. Client-generated `runId` allowed (UUID v7 for time-ordering). |
| **Content negotiation** | Frontend sends `Accept: application/json` for REST and `Accept: text/event-stream` for `/run`. Server returns 415 on mismatch. |

---

## 5. Schema source of truth

**Decision needed**: pick one — OpenAPI 3.1 or Protobuf.

| Option | Pros | Cons |
| --- | --- | --- |
| **OpenAPI 3.1** | Native fit for REST + SSE description; great TS codegen (`openapi-typescript`); great Go codegen (`oapi-codegen`); humans can read the YAML. | SSE event payloads need a custom `x-events` extension or a sibling AsyncAPI doc. |
| **Protobuf + buf** | Strictly-typed wire format; gRPC-Web bridge possible if we ever leave SSE; great Go codegen; binary efficient. | Less natural for REST GETs (gRPC-Gateway needed); humans read .proto less easily than YAML; ecosystem heavier. |

Lyra is REST-shaped (one streaming endpoint + plain JSON elsewhere) — **OpenAPI
3.1 is the better fit**. Mock backend already speaks plain JSON; switching to
proto would be a bigger lift than the marginal type-safety win.

**Recommended layout** (once we commit):

```
schemas/
├── openapi.yaml             # REST endpoints + ProblemDetails + shared models
├── events.yaml              # AG-UI standard + lyra.* event payloads (Zod-mirror)
└── generated/
    ├── ts/                  # openapi-typescript output (committed for review)
    └── go/                  # oapi-codegen output (committed for review)
```

CI gate: `npm run schema:check` runs both codegens + diffs against `generated/`
so PRs touching the contract are forced to land both sides in lockstep.

---

## 6. Implementation roadmap

| Phase | Scope | Effort |
| --- | --- | --- |
| **1. Protocol fixed** | Author `schemas/openapi.yaml`; freeze CUSTOM event payloads; wire `/capabilities`; codegen both sides; mock backend speaks the real contract. | 1 week |
| **2. Main path** | `/run` against a real LLM (one provider); `/permission` flows back into tool-execution; `/sessions` CRUD; `/messages` pagination; `/run/{runId}/cancel`. | 2 weeks |
| **3. Tools + context** | `/tools` registry; `/attachments` upload; `/providers` + `/models`; multi-provider switching in composer. | 2 weeks |
| **4. Persistence** | SQLite or Postgres for sessions / messages; history hydration uses `/sessions/{id}/messages` instead of bulk `MESSAGES_SNAPSHOT`; MetaEvent feedback if product wants it. | 1 week |
| **5. Auth + workspaces** (only if needed) | Login flow; `/me`; `/workspaces`; telemetry opt-in. | 1–2 weeks |

---

## 7. Migration policy — mock → real

Per-endpoint flip checklist (apply to each row in §2.A):

1. Schema lands in `schemas/openapi.yaml` (one PR, both sides codegen).
2. Mock backend's handler keeps current behaviour but reads/writes through the
   generated types (compile-time gate).
3. Real backend handler replaces the mock body — fixtures preserved as the
   default response when env var `LYRA_MOCK=1` is set, for E2E / demos.
4. Frontend test pinning the response shape stays green throughout.

This way we **never break the frontend** during the cutover; the mock and the
real impl are interchangeable behind one schema.

---

## 8. Open questions

Decisions the team needs to make before Phase 2 lands:

- [ ] **Auth model**: API key per workspace, OAuth, or both? Affects every endpoint.
- [ ] **Storage**: SQLite (single-user desktop) or Postgres (shared)? Affects pagination semantics.
- [ ] **Multi-tenant**: do we need workspace isolation in the URL (`/v1/workspaces/{id}/…`) or only via the auth context? Affects URL shape forever.
- [ ] **Tool execution location**: in-process (Go) or sandbox (gVisor / containerised)? Affects `/permission` semantics.
- [ ] **Telemetry / RLHF**: ship MetaEvent feedback in Phase 2 or wait? Affects whether we add `/feedback`.
- [ ] **Plugin marketplace**: keep `/plugins` open or close (only same-origin sideload)? Affects security review.

Until these land, the contract above is the **minimum** that won't paint us
into a corner.

---

## Appendix A — Quick reference: where things live

| Concern | Frontend file | Backend file |
| --- | --- | --- |
| Streaming events (reducer dispatch table) | `frontend/src/plugins/builtin/core-reducer/handlers.ts` | `internal/agui/events.go` |
| CUSTOM event handlers | `frontend/src/plugins/builtin/agui-handlers/index.ts` | `internal/agui/dsl.go` |
| Wire-level types (REST) | `frontend/src/lib/queries.ts` | `internal/agui/rest.go` |
| HITL approval | `frontend/src/domain/gateways/PermissionGateway.ts` + `frontend/src/infra/http/HttpPermissionGateway.ts` | `internal/agui/permissions.go` |
| Sideload manifest | `frontend/src/plugins/sideload.ts` | `internal/agui/plugins.go` |
| Mock fixture data | — | `internal/agui/demos.go` / `refactor_demo_data.go` / `artifacts.go` |
| Base URL config | `frontend/src/main/config.ts` (`AGUI_BASE`) | `internal/agui/server.go` (listen addr) |
