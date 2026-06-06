// Wire-level shape types for the Lyra Runtime Protocol v2. Mirror of
// docs/protocol/API.md §4 (data catalog) + §5 (streaming) + §7 (method params /
// results) + §9 (capabilities) — keep in sync. Type naming follows the
// backend Go `lyra/rpc/protocol` interface as the mechanical SSOT; this
// file is the zero-mapping TS projection.
//
// Frontend view-state types live in `@/protocol/run/viewState` — those
// are a *presentation projection* the reducer folds these wire shapes
// into; this file is the upstream contract.

import type { AttachmentId, ItemId, RunId, SessionId, TaskId } from "./ids";

// ---------------------------------------------------------------------------
// §3 / §9 — Lifecycle + capabilities
// ---------------------------------------------------------------------------

export type InterruptKind = "approval" | "question" | "toolResult";

export interface ClientCapabilities {
  events: string[]; // event types this client can render
  features: Record<string, unknown>;
  interruptKinds?: InterruptKind[]; // HITL kinds we can handle (anti-deadlock, §6.2)
  optOutNotificationMethods?: string[]; // suppress high-freq notifications, e.g. ["item.delta"]
}

export interface ServerFeatures {
  reasoning: boolean;
  mcp: boolean;
  multimodal: boolean;
  checkpoints: boolean;
  background: boolean;
  subagents: boolean;
  skills: boolean;
  sessionExport: boolean;
  memory: boolean;
  relocate: boolean;
  clientTools: boolean;
  attachments: { enabled: boolean; maxSizeBytes?: number; mimeTypes?: string[] };
}

export interface ServerCapabilities {
  protocolVersion: string;
  events: string[]; // event types the server emits
  features: ServerFeatures; // unset flag ⇒ false
  providers: string[];
  limits: { maxConcurrentRuns?: number };
}

export interface ServerInfo {
  name: string;
  version: string;
  cwd: string; // serve-process cwd (cold-start default for sessions.create)
  home: string;
}

export interface InitializeRequest {
  protocolVersion: string;
  clientInfo: { name: string; version: string };
  capabilities: ClientCapabilities;
}

export interface InitializeResponse {
  protocolVersion: string;
  serverInfo: ServerInfo;
  capabilities: ServerCapabilities;
}

export interface ShutdownRequest {
  reason?: string;
}

// notifications.canceled (client→server): cancel an in-flight Request by
// its envelope id. NOT runs.cancel (which stops a run by runId).
export interface CanceledNotification {
  id: string;
  reason?: string;
}

// ---------------------------------------------------------------------------
// §4.1 — Session / Project
// ---------------------------------------------------------------------------

export type SessionStatus = "running" | "waiting" | "idle";

export interface Session {
  id: SessionId;
  title: string;
  status: SessionStatus;
  model: string;
  cwd: string; // absolute, server-resolved (symlinks resolved)
  projectRoot?: string; // derived: nearest .git ancestor, else = cwd
  cwdMissing?: boolean; // cwd lost on disk → degrade to plain chat + relocate
  createdAt: string;
  updatedAt: string;
  usage?: Usage; // cumulative for this session
  metadata: Record<string, unknown>;
}

export interface Project {
  cwd: string; // unique identity
  name: string; // basename(cwd)
  projectRoot?: string;
  branch?: string; // git branch, best-effort
  sessionCount: number;
  lastActiveAt?: string;
  cwdMissing?: boolean;
}

export interface CreateSessionRequest {
  cwd?: string; // default = ServerInfo.cwd (cold-start zero friction)
  title?: string;
  model?: string;
  metadata?: Record<string, unknown>;
}

export interface UpdateSessionRequest {
  sessionId: SessionId;
  title?: string;
  cwd?: string; // changing cwd = relocate (features.relocate)
  model?: string;
  metadata?: Record<string, unknown>; // full replace
}

export interface ForkSessionRequest {
  sessionId: SessionId;
  fromItemId?: ItemId; // fork at an item boundary
  title?: string;
}

export interface ExportSessionRequest {
  sessionId: SessionId;
  format?: "md" | "json";
}

// sessions.export — export file goes through the transport file channel,
// not inlined into the JSON-RPC envelope.
export interface ExportSessionResponse {
  url: string;
  expiresAt: string;
}

// ---------------------------------------------------------------------------
// §4.2 — Run
// ---------------------------------------------------------------------------

export interface RunRef {
  id: RunId;
  sessionId: SessionId;
  spawnedByItemId?: ItemId; // child-of: this Run is a subagent spawned by that toolCall Item
  parentRunId?: RunId; // continuation-of: this Run is a resume/edit continuation
  status?: "running" | "finished";
  outcome?: RunOutcome; // when status=finished
  // The model + mode this Run executed under. Carried on the RunRef so a
  // reconnect (runs.subscribe) or history hydration (items.list.runs) that
  // never saw the originating runs.start request can still attribute the Run.
  model?: string;
  mode?: RunMode;
  createdAt?: string;
  finishedAt?: string;
}

export type RunMode = "agent" | "chat" | "plan";

export type RunOutcome =
  | { type: "completed"; result: RunResult }
  | { type: "error"; result: RunResult }
  | { type: "maxSteps"; result: RunResult } // agent step ceiling (Run = one turn)
  | { type: "maxBudget"; result: RunResult } // cost ceiling (incl. subagent subtree)
  | { type: "canceled"; result: RunResult }
  | { type: "interrupt"; interrupts: Interrupt[] }; // ★resumable; Run already ended, resources freed

export interface RunResult {
  usage?: Usage;
  costUsd?: number; // omitted when model not in pricing table (never fabricate 0)
  steps?: number;
  error?: ProblemData; // when outcome.type=error
}

// ---------------------------------------------------------------------------
// §4.3 — Item (the unified history + streaming primitive)
// ---------------------------------------------------------------------------

export type ItemStatus = "inProgress" | "completed" | "incomplete"; // incomplete = interrupted/canceled

export interface ItemBase {
  id: ItemId;
  runId: RunId; // owning Run (a subagent's item.runId = the child Run)
  status: ItemStatus;
  createdAt: string;
}

export type ContentBlock =
  | { type: "text"; text: string }
  | { type: "image"; attachmentId: AttachmentId };

export interface PlanStep {
  id: string;
  title: string;
  status: "pending" | "inProgress" | "completed" | "failed";
}

export interface QuestionOption {
  label: string;
  description?: string; // option meaning
  preview?: string; // side preview (for comparing options)
}
export interface QuestionFieldBase {
  name: string; // answers keyed by this
  label: string;
  header?: string; // ≤12-char short label (UI chip)
  required?: boolean;
}
export type QuestionField =
  | (QuestionFieldBase & { type: "text" })
  | (QuestionFieldBase & { type: "choice"; options: QuestionOption[]; multiple?: boolean });
export interface Question {
  prompt: string;
  fields: QuestionField[];
}

export type Item =
  | (ItemBase & { type: "userMessage"; content: ContentBlock[] })
  | (ItemBase & { type: "agentMessage"; content: ContentBlock[] })
  | (ItemBase & { type: "reasoning"; text: string; redacted?: boolean })
  | (ItemBase & { type: "plan"; steps: PlanStep[] })
  | (ItemBase & { type: "question"; question: Question })
  | (ItemBase & {
      type: "toolCall";
      tool: ToolInvocation;
      safetyClass?: string;
      error?: ProblemData;
    });

export type ItemType = Item["type"];

// ---------------------------------------------------------------------------
// §4.4 — ToolInvocation
// ---------------------------------------------------------------------------

// Uniform "general + special" tool shape (API.md §4.4). Closed, structurally
// rich tools (command / file change / search) are typed variants — `kind` IS
// the identity, no redundant `name`. The open set (MCP / dynamic / subagent /
// custom) rides one generic envelope keyed by `name`, with a parsed-object
// `arguments` and a best-effort-JSON `result`.
export type ToolInvocation =
  | {
      kind: "commandExecution";
      command: string[];
      cwd?: string;
      // Settled fields — REQUIRED on the item.completed toolCall, absent on the
      // item.started shell (lifecycle, not optionality of contract). `output` is
      // the authoritative merged stdout+stderr (interleaved in real time; "" when
      // the command printed nothing). The `toolOutput` ItemDelta is only a live
      // PREVIEW of it — dropping every ephemeral delta still yields correct output
      // from here (API.md §5.2), and a runtime that doesn't stream emits no delta
      // yet still carries it. See docs/protocol/TOOL_OUTPUT.md.
      exitCode?: number;
      durationMs?: number;
      output?: string;
      // True iff the runtime capped `output` (and the delta stream) at a size
      // limit — UI shows a "truncated, open in terminal for full" affordance.
      outputTruncated?: boolean;
    }
  | { kind: "fileChange"; changes: FileChangeEntry[] }
  | { kind: "search"; query: string; results?: SearchHit[] } // local grep / glob
  | { kind: "webSearch"; query: string; results?: WebSearchResult[] }
  | { kind: "tool"; name: string; arguments: Record<string, unknown>; result?: unknown };

export type ToolKind = ToolInvocation["kind"];

export interface FileChangeEntry {
  path: string;
  kind: "add" | "modify" | "delete" | "rename";
  diff?: DiffRow[];
}

// ---------------------------------------------------------------------------
// §4.5 — Diff / Search / files
// ---------------------------------------------------------------------------

export type DiffRow =
  | { type: "hunk"; text: string }
  | { type: "context"; leftLine: number; rightLine: number; code: string }
  | { type: "added"; rightLine: number; code: string }
  | { type: "deleted"; leftLine: number; code: string };

// Local search hit (grep = path+line+snippet; glob = path only).
export interface SearchHit {
  path: string;
  lineNumber?: number;
  snippet?: string;
}
// Web search result.
export interface WebSearchResult {
  title?: string;
  url: string;
  snippet?: string;
  faviconUrl?: string;
}

export interface FileChange {
  path: string;
  status: "added" | "modified" | "deleted" | "renamed" | "untracked";
}
export interface FileLine {
  lineNumber: number;
  text: string;
}
export interface FileHead {
  path: string;
  lines: FileLine[];
}
export interface GrepMatch {
  path: string;
  lineNumber: number;
  text: string;
}
export interface GrepResult {
  matches: GrepMatch[];
  total: number; // ≥ matches.length (matches may be limit-truncated)
}

// ---------------------------------------------------------------------------
// §4.6 — Usage / Error
// ---------------------------------------------------------------------------

export interface Usage {
  inputTokens?: number;
  outputTokens?: number;
  reasoningTokens?: number;
  cacheReadTokens?: number;
  cacheWriteTokens?: number;
  byModel?: Record<
    string,
    { inputTokens?: number; outputTokens?: number; reasoningTokens?: number; costUsd?: number }
  >;
}

// Field-level error inside ProblemData.errors (§8.3) — `field` is the
// offending params key, so a form can flag it inline.
export interface FieldError {
  field: string;
  detail: string;
}

// Used for RPCError.data, RunResult.error, toolCall.error. A transport-
// agnostic trim of RFC 9457 Problem Details (no HTTP-only status/instance;
// `type` is a stable symbol, not a resolvable URI). Judge errors by `type`
// (§8.3); plugins namespace it as `plugin:<name>/<symbol>` (§8.4).
export interface ProblemData {
  type: string; // stable symbolic name (the discriminator)
  detail?: string; // per-occurrence human explanation
  retryable?: boolean;
  retryAfterSeconds?: number; // earliest retry (e.g. provider rate-limit backoff)
  errors?: FieldError[]; // field-level validation errors (invalid_params / forms)
  [key: string]: unknown; // still open for type-specific extension members
}

// ---------------------------------------------------------------------------
// §4.7 — Context / tool specs
// ---------------------------------------------------------------------------

export type ContextItem =
  | { kind: "file"; path: string } // relative to Session.cwd
  | { kind: "selection"; path: string; range: [number, number] } // 1-based inclusive
  | { kind: "url"; url: string } // Runtime fetches (SSRF surface)
  | { kind: "image"; attachmentId: AttachmentId };

export type JsonSchema = Record<string, unknown>;

export interface ToolSpec {
  name: string;
  description?: string;
  parameters?: JsonSchema;
  safetyClass?: string; // "safe" | "write" | "exec" | "network" …
}

export interface GenerationParams {
  temperature?: number;
  maxTokens?: number;
  topP?: number;
  stop?: string[];
}

// ---------------------------------------------------------------------------
// §4.8 — HITL types
// ---------------------------------------------------------------------------

// General (itemId) + special (per-kind payload), API.md §4.8. approval /
// toolResult reuse ToolInvocation (read payload.tool — no guessing where the
// command lives). question carries no payload (its content is on the Item).
export type Interrupt =
  | { kind: "approval"; itemId: ItemId; payload: ApprovalPayload }
  | { kind: "question"; itemId: ItemId }
  | { kind: "toolResult"; itemId: ItemId; payload: ToolResultPayload };

export interface ApprovalPayload {
  tool: ToolInvocation; // the tool awaiting approval (result not yet present)
  risk?: "low" | "medium" | "high";
  reason?: string;
}
export interface ToolResultPayload {
  tool: ToolInvocation; // a client-side tool to execute; result returned via runs.resume
}

export interface OpenInterrupt {
  parentRunId: RunId; // the Run to resume (its outcome.type=interrupt)
  sessionId: SessionId;
  interrupts: Interrupt[];
  createdAt: string;
}

// §6.1 — InterruptResponse (sent via runs.resume).
export interface ApprovalResponse {
  kind: "approval";
  decision: "approve" | "deny";
  editedArgs?: Record<string, unknown>;
  reason?: string;
}
export interface AnswerResponse {
  kind: "answer";
  answers: Record<string, string | string[]>; // key = QuestionField.name
}
export interface ToolResultResponse {
  kind: "toolResult";
  result?: unknown; // best-effort JSON, same shape as ToolInvocation.result
  error?: ProblemData; // when the client tool failed
}
export interface InterruptResponse {
  itemId: ItemId; // matches Interrupt.itemId
  response: ApprovalResponse | AnswerResponse | ToolResultResponse;
}

// ---------------------------------------------------------------------------
// §4.9 — Provider / Model
// ---------------------------------------------------------------------------

export interface Provider {
  id: string;
  type: string; // "openai" | "anthropic" | …
  baseUrl?: string;
  apiKeyMasked: string; // "" = unset; e.g. "sk-…fc78"; never reversible
}

export interface Model {
  id: string;
  provider: string; // Provider.id
  displayName?: string;
  contextWindow?: number;
  maxOutputTokens?: number;
  capabilities?: { reasoning?: boolean; multimodal?: boolean; toolUse?: boolean };
  pricing?: { inputUsdPerMillionTokens?: number; outputUsdPerMillionTokens?: number };
}

export interface ConfigureProviderRequest {
  provider: string; // Provider.id / slug — must be a backend-supported provider
  type?: string;
  baseUrl?: string; // override default endpoint (proxy / gateway / self-hosted)
  apiKey?: string;
}

export interface ProviderTestResult {
  ok: boolean;
  error?: ProblemData;
}

// ---------------------------------------------------------------------------
// §4.10 — Workspace / optional-domain types
// ---------------------------------------------------------------------------

export interface Skill {
  name: string;
  description?: string;
  source?: string;
}
export interface AgentDoc {
  path: string;
  title?: string;
  scope: "cwd" | "projectRoot" | "home";
}
export interface McpServer {
  name: string;
  status: "connected" | "disconnected" | "error";
  description?: string;
}
export interface McpTool {
  server: string;
  name: string;
  description?: string;
  inputSchema?: Record<string, unknown>;
}

export type MemoryScope = "cwd" | "projectRoot" | "home";
export interface MemoryEntry {
  scope: MemoryScope;
  path: string;
  content: string;
  updatedAt?: string;
}

export interface Attachment {
  id: AttachmentId;
  name: string;
  mime: string;
  sizeBytes: number;
  createdAt: string;
}

export interface BackgroundTask {
  id: TaskId;
  kind: string;
  status: "running" | "completed" | "failed" | "canceled";
  createdAt: string;
  updatedAt?: string;
  result?: unknown;
  error?: ProblemData;
}

// ---------------------------------------------------------------------------
// §4.11 — Pagination
// ---------------------------------------------------------------------------

export interface Page<T> {
  data: T[];
  nextCursor?: string; // opaque
}

export interface PageQuery {
  cursor?: string;
  limit?: number;
}

// ---------------------------------------------------------------------------
// §5 — Streaming (RunEvent envelope + StreamEvent union + ItemDelta)
// ---------------------------------------------------------------------------

export type JsonPatch = Array<{
  op: "add" | "remove" | "replace" | "move" | "copy" | "test";
  path: string;
  value?: unknown;
  from?: string;
}>;

export type ItemDelta =
  | { type: "content"; index?: number; text: string } // agentMessage text delta
  | { type: "reasoning"; text: string } // reasoning text delta
  | { type: "toolArguments"; argumentsTextDelta: string } // partial JSON text of tool args
  | { type: "toolOutput"; text: string } // PREVIEW of command stdout — authoritative copy lands on the completed item (commandExecution.output)
  | { type: "plan"; steps: PlanStep[] }; // current full plan (not a hot char stream)

export type StreamEvent =
  | { type: "run.started"; run: RunRef }
  | { type: "run.progress"; progress: RunProgress } // ephemeral (durable=false); authoritative final usage/steps land on run.finished.result
  | { type: "run.finished"; outcome: RunOutcome }
  | { type: "item.started"; item: Item } // shell (status=inProgress)
  | { type: "item.delta"; itemId: ItemId; delta: ItemDelta }
  | { type: "item.completed"; item: Item } // authoritative terminal, durable
  | { type: "state.snapshot"; state: Record<string, unknown> }
  | { type: "state.delta"; patch: JsonPatch }
  | { type: "custom"; name: string; payload: unknown };

// Mid-run progress preview — a live readout of step/usage/cost while the Run
// streams. Ephemeral like item.delta: dropping every run.progress still yields
// the correct totals from run.finished.result (the authoritative landing), so
// §5.2's durable invariant holds. Suppressible via optOutNotificationMethods.
export interface RunProgress {
  step?: number; // agent steps elapsed so far
  maxSteps?: number; // ceiling, when the Run was started with one
  usage?: Usage; // cumulative usage so far
  costUsd?: number; // cumulative cost so far
  activity?: string; // human-readable current action ("calling tool: bash")
}

export type StreamEventType = StreamEvent["type"];

export interface RunEvent {
  runId: RunId;
  eventId: string; // evt_…; monotonic within a single root run stream (§2.2)
  timestamp: string; // ISO-8601
  durable: boolean; // true = authoritative/listable; false = high-freq ephemeral delta
  event: StreamEvent;
}

// ---------------------------------------------------------------------------
// §7.3 — Run request shapes
// ---------------------------------------------------------------------------

export interface StartRunRequest {
  sessionId: SessionId; // cwd resolved from session; no cwd, no runId on this request
  input: ContentBlock[]; // user message body
  context?: ContextItem[]; // file.path relative to session.cwd (§4.7)
  tools?: ToolSpec[]; // override this run's tool set
  state?: Record<string, unknown>; // initial shared state
  attachments?: AttachmentId[];
  // provider + model are a PAIR (API §7.3): send both or neither. Only one →
  // invalid_params. provider is NOT inferred from model (same model id can
  // span providers). Both come straight from models.list's Model.{provider,id}.
  provider?: string;
  model?: string;
  mode?: "agent" | "chat" | "plan";
  maxSteps?: number; // ceiling → outcome.maxSteps
  maxBudgetUsd?: number; // incl. subagent subtree → outcome.maxBudget
  params?: GenerationParams;
}

export interface ResumeRunRequest {
  parentRunId: RunId; // the interrupted Run (outcome.type=interrupt)
  responses: InterruptResponse[]; // one per open interrupt, addressed by itemId
}

export interface StartRunResponse {
  runId: RunId;
  // The opening userMessage Item's id — same id as on the stream's
  // item.started/completed and in items.list. The client reconciles its
  // optimistic bubble by this exact id (no content-text heuristic). Absent on
  // runs.resume (no opening user turn). A business field, not transport meta.
  userItemId?: ItemId;
}

export interface ResumeRunResponse {
  runId: RunId; // new continuation Run (RunRef.parentRunId = parentRunId)
}

// ---------------------------------------------------------------------------
// §7.4 — Items
// ---------------------------------------------------------------------------

export interface ListItemsRequest {
  sessionId: SessionId;
  cursor?: string;
  limit?: number;
}

// items.list — a Page<Item> plus the RunRefs needed to rebuild the run tree
// (§10.3). Reuses Page<T>'s `data` field so every paginated method reads
// `resp.data` (no `data` vs `items` drift across the surface).
export interface ListItemsResponse extends Page<Item> {
  runs: RunRef[]; // owning Runs (finished/running), with parentRunId/spawnedByItemId
}

export interface EditItemRequest {
  itemId: ItemId;
  replacement: ContentBlock[];
}

export interface EditItemResponse {
  runId: RunId; // new continuation Run
  parentRunId: RunId;
}

// ---------------------------------------------------------------------------
// §7.5 — Workspace
// ---------------------------------------------------------------------------

export interface WorkspaceQuery {
  cwd?: string; // default = serve dir
}

// ---------------------------------------------------------------------------
// §7.6 — tools.invoke
// ---------------------------------------------------------------------------

export interface InvokeToolRequest {
  name: string;
  arguments: Record<string, unknown>;
  cwd?: string;
}

// ---------------------------------------------------------------------------
// §7.7 — Optional domains (attachments / feedback)
// ---------------------------------------------------------------------------

export interface CreateUploadUrlRequest {
  name: string;
  mime: string;
  sizeBytes: number;
}
export interface CreateUploadUrlResponse {
  attachmentId: AttachmentId;
  uploadUrl: string;
  expiresAt: string;
}

export interface FeedbackRequest {
  sessionId?: SessionId;
  runId?: RunId;
  itemId?: ItemId;
  rating?: "positive" | "negative";
  text?: string;
}
