// Wire-level shape types for the Lyra Runtime Protocol v2. Mirror of
// docs/protocol/API.md §4 (data catalog) + §5 (streaming) + §7 (method params /
// results) + §9 (capabilities) — keep in sync. Type naming follows the
// backend Go `lyra/rpc/protocol` interface as the mechanical SSOT; this
// file is the zero-mapping TS projection.
//
// Frontend view-state types live in `@/plugins/builtin/agent/public/viewState` — those
// are a *presentation projection* the reducer folds these wire shapes
// into; this file is the upstream contract.

import type { ItemId, RunId, SegmentId, SessionId } from "./ids";

// ---------------------------------------------------------------------------
// §3 / §9 — Lifecycle + capabilities
// ---------------------------------------------------------------------------

export type InterruptType = "approval" | "question" | "toolResult";

export interface ClientCapabilities {
  events: string[]; // event types this client can render
  features: Record<string, { enabled: boolean }>;
  interruptTypes?: InterruptType[]; // HITL types we can handle (anti-deadlock, §6.2)
  excludedEvents?: string[]; // suppress high-freq notifications, e.g. ["item.delta"]
}

export interface FeatureCapability {
  enabled: boolean;
  stability: "stable" | "experimental";
}

export interface ServerFeatures {
  reasoning: FeatureCapability;
  mcp: FeatureCapability;
  multimodal: FeatureCapability;
  git: FeatureCapability;
  fileWatch: FeatureCapability;
  checkpoints: FeatureCapability;
  lsp: FeatureCapability;
  // todos and automatic compaction are runtime projections; optional means an
  // older runtime may omit the feature and the client must treat it as off.
  todos?: FeatureCapability;
  compaction?: FeatureCapability;
  subagents: FeatureCapability;
  skills: FeatureCapability;
  sessionExport: FeatureCapability;
  memory: FeatureCapability;
  relocate: FeatureCapability;
  clientTools: FeatureCapability;
  [name: string]: FeatureCapability | undefined;
}

export interface ServerCapabilities {
  events: string[]; // event types the server emits
  features: ServerFeatures; // unset flag ⇒ false
  streamingMethods: string[]; // machine-readable stream-method set (§9) — clients never hardcode
  limits: { maxConcurrentRuns?: number };
}

export interface ServerInfo {
  name: string;
  version: string;
  cwd: string; // serve-process cwd (cold-start default for sessions.create)
  home: string;
}

export interface RequestMeta {
  protocolVersion?: string;
  clientInfo?: { name: string; version: string };
  clientCapabilities?: ClientCapabilities;
}

export interface DiscoverResponse {
  protocol: { current: string; minSupported: string };
  serverInfo: ServerInfo;
  capabilities: ServerCapabilities;
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
  favorite?: boolean; // user-pinned: sorts ahead in the session list
  revision: number;
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
}

export interface UpdateSessionRequest {
  sessionId: SessionId;
  expectedRevision: number;
  title?: string;
  cwd?: string; // changing cwd = relocate (features.relocate)
  model?: string;
  favorite?: boolean; // pin / unpin in the session list
}

export interface ForkSessionRequest {
  sessionId: SessionId;
  // Fork at a run boundary: copy history up to AND INCLUDING this root run.
  // Omitted = whole-session fork. (AUX_API §4.2 — replaced item-level
  // fromItemId; run boundaries are reliable without an item↔message join.)
  fromRunId?: RunId;
  title?: string;
}

// sessions.rollback (AUX_API §4.1) — turn-granular, in-place truncation.
// `toRunId` is INCLUSIVE-KEEP: the last ROOT run to keep (its continuation
// chain stays); everything after is destroyed (items, open interrupts,
// spawned subagent sub-sessions). Omitted = drop all runs (empty session).
// Rejected with `session_busy` while a run is in flight.
//
// `restoreType` (default "history", gated features.checkpoints): "files"
// restores the working tree to toRunId's shadow-git snapshot (history kept),
// "both" does files-then-history ATOMICALLY — on snapshot failure the whole
// call fails with `checkpoint_unavailable`, history untouched, never a silent
// history-only degrade. files/both REQUIRE toRunId (else invalid_params).
// Plain "history" never touches files — UI checks getDiff.
export interface RollbackSessionRequest {
  sessionId: SessionId;
  toRunId?: RunId;
  restoreType?: "history" | "files" | "both";
}

// A run destroyed by rollback. `userInput` is the opening userMessage's
// content (same shape as StartRunRequest.input → composer prefill is
// zero-conversion); continuation runs have no opening user turn.
export interface DroppedRun {
  run: RunRef;
  userInput?: ContentBlock[];
}

export interface RollbackSessionResponse {
  session: Session;
  droppedRuns: DroppedRun[];
}

export interface ExportSessionRequest {
  sessionId: SessionId;
  format?: "md" | "json";
}

// sessions.export — INLINE payload (API.md §7.2): the runtime is a local
// loopback process, so there's no giant-payload concern and no download
// endpoint. format "json" → `artifact` (round-trippable via sessions.import);
// format "md" → `markdown` (human transcript, NOT importable).
export interface ExportSessionResponse {
  format: "md" | "json";
  artifact?: SessionArtifact;
  markdown?: string;
}

// Round-trippable session bundle (sessions.export format:"json" →
// sessions.import). This is a durable input document, not a snapshot of the
// live RPC projections: it excludes derived session state and process-local run
// state. `messages` is the provider chat-message blob, opaque to the client.
export interface SessionArtifact {
  version: number; // artifact schema version (currently 6); import rejects unknown
  session: ArtifactSession;
  messages: unknown[];
  runs: ArtifactRun[];
  items: ArtifactItem[];
  toolResults: ArtifactToolResult[];
}

export interface ArtifactSession {
  id: SessionId;
  title: string;
  cwd: string;
  model: string;
  createdAt: string;
  updatedAt: string;
  favorite?: boolean;
}

// A terminal record. Archive outcomes intentionally cannot represent an active
// or interrupted run because executor state is not portable.
export interface ArtifactRun {
  id: RunId;
  sessionId: SessionId;
  spawnedByItemId?: ItemId;
  provider?: string;
  model?: string;
  outcome: ArtifactOutcome;
  createdAt: string;
  finishedAt: string;
  updatedAt: string;
  messageMark: number;
}

export interface ArtifactOutcome {
  type: "completed" | "error" | "maxSteps" | "maxBudget" | "canceled";
  result: ArtifactRunResult | null;
  detail?: string;
}

export interface ArtifactRunResult {
  usage?: Usage;
  steps: number;
  error?: ArtifactProblem;
  durationMs?: number;
}

export type ArtifactProblem = Pick<
  ProblemData,
  "type" | "detail" | "docUrl" | "retryable" | "retryAfterSeconds"
>;

// Direct durable transcript values. A tool result is canonical data rather
// than the presentation-normalized result returned in live Item events.
export type ArtifactItem =
  | (ArtifactItemBase & { type: "userMessage" | "agentMessage"; content: ContentBlock[] })
  | (ArtifactItemBase & { type: "reasoning"; text: string; redacted?: boolean })
  | (ArtifactItemBase & { type: "plan"; steps: PlanStep[] })
  | (ArtifactItemBase & { type: "question"; question: Question })
  | (ArtifactItemBase & {
      type: "toolCall";
      tool: ToolInvocation;
      safetyClass?: string;
      error?: ArtifactProblem;
    })
  | (ArtifactItemBase & { type: "compaction"; summary?: string; droppedMessages?: number });

export interface ArtifactItemBase {
  id: ItemId;
  runId: RunId;
  status: ItemStatus;
  createdAt: string;
}

export interface ArtifactToolResult {
  id: string;
  itemId: ItemId;
  toolName: string;
  preview: string;
  body: string;
  createdAt: string;
}

// sessions.import — RESTORE semantics: rebuilds the session under the
// artifact's ORIGINAL id (overwriting if it exists), idempotent by-id upsert.
export interface ImportSessionResponse {
  session: Session;
}

// ---------------------------------------------------------------------------
// §4.2 — Run
// ---------------------------------------------------------------------------

export interface RunRef {
  id: RunId; // stable across HITL resume — a resume opens a new segment, NOT a new Run
  sessionId: SessionId;
  spawnedByItemId?: ItemId; // child-of: this Run is a subagent spawned by that toolCall Item
  // The model id this Run ran against (Model.id); absent = runtime default
  // (surfaced via Session.model). Rides RunRef so a reconnect (runs.subscribe)
  // or history restore (items.list.runs) — which never saw the originating
  // runs.start — can still label which model the Run used (API.md §4.2).
  model?: string;
  // The provider id this Run ran against (Provider.id), paired with model;
  // absent = runtime default. Stamped so a finished Run is self-describing
  // (usage.summary attributes spend by provider). API.md §4.2.
  provider?: string;
  status?: "running" | "finished";
  outcome?: RunOutcome; // when status=finished
  createdAt?: string;
  finishedAt?: string;
}

export type RunOutcome =
  // `result` is `*RunResult` + omitempty on the wire, so a minimal/non-conformant
  // backend can omit it — consumers must guard (the fold does), never deref blind.
  | { type: "completed"; result?: RunResult }
  | { type: "error"; result?: RunResult } // result.error: ProblemData (with detail)
  | { type: "maxSteps"; result?: RunResult; detail?: string } // step ceiling within one Run (counted by step, not turn)
  | { type: "maxBudget"; result?: RunResult; detail?: string } // cost ceiling (incl. subagent subtree); detail like "spent $4.20 / cap $4.00"
  | { type: "canceled"; result?: RunResult; detail?: string } // runs.cancel reason flows here
  | { type: "interrupt"; interrupts: Interrupt[] }; // ★resumable; Run already ended, resources freed

// Total cost reads `usage.costUsd` — there is NO RunResult.costUsd (avoids two
// sources of truth for total cost, API.md §4.2).
export interface RunResult {
  usage?: Usage;
  steps?: number;
  error?: ProblemData; // when outcome.type=error
  durationMs?: number; // run wall-clock (spans interrupt/resume); a final "took 12.4s" on any terminal
}

// ---------------------------------------------------------------------------
// §4.3 — Item (the unified history + streaming primitive)
// ---------------------------------------------------------------------------

export type ItemStatus = "running" | "completed" | "incomplete"; // running=in progress; incomplete = interrupted/canceled

export interface ItemBase {
  id: ItemId;
  runId: RunId; // owning Run (a subagent's item.runId = the child Run)
  status: ItemStatus;
  createdAt: string;
}

export type ContentBlock =
  | { type: "text"; text: string }
  // Image inlined: `mime` = media type (image/png|jpeg|gif|webp), `data` = raw
  // base64 (NO "data:…;base64," prefix). Backend assembles from mime + data
  // (MULTIMODAL_IMAGE_INPUT, API.md §4.3).
  | { type: "image"; mime: string; data: string };

export interface PlanStep {
  id: string;
  title: string;
  status: "pending" | "running" | "completed" | "failed"; // "in progress" uses running (§2.3)
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
    })
  // A context-compaction boundary — the durable marker that "N
  // earlier messages were summarized here". Emitted by turn-edge compaction
  // and folded to a timeline divider.
  | (ItemBase & { type: "compaction"; summary?: string; droppedMessages?: number });

export type ItemType = Item["type"];

// ---------------------------------------------------------------------------
// §4.4 — ToolInvocation (domain-neutral envelope)
// ---------------------------------------------------------------------------

// The core has ONE tool shape (not a union). `name` is identity, `arguments`
// is a parsed JSON object, `result` is best-effort JSON output. "How a tool
// renders richly" is domain knowledge — NOT on the wire — layered by the
// client display registry keyed on `name` (API.md §4.4.2). New tools cost the
// protocol nothing (§13: no first-class typed tool variants).
export interface ToolInvocation {
  name: string; // tool identity (stable); MCP uses "<server>.<tool>"
  arguments: Record<string, unknown>; // parsed JSON object (never a JSON string)
  // best-effort JSON output; absent on the item.started shell, authoritative on
  // item.completed. Never double-encoded ({x:1}, not "{\"x\":1}"). Streamed
  // command stdout previews via item.delta{toolOutput} → settles into
  // result.output on completed (API.md §4.4.1 / §5.2).
  result?: unknown;
}

// ---------------------------------------------------------------------------
// §4.5 — Diff / Search / files
// ---------------------------------------------------------------------------

export type DiffRow =
  | { type: "hunk"; text: string }
  | { type: "context"; leftLine: number; rightLine: number; code: string }
  | { type: "added"; rightLine: number; code: string }
  | { type: "deleted"; leftLine: number; code: string };

// Structured diff returned by workspace.getDiff (AUX_API §2.3). Sum-type by
// the request's `format`: rows → `files`, raw → `patch`. `truncated` = the
// rows `limit` was hit; truncation happens at FILE boundaries (a file's rows
// appear whole or not at all — no half diffs, no silent caps).
export interface Diff {
  files?: FileDiff[]; // format=rows
  patch?: string; // format=raw (original unified patch)
  truncated?: boolean;
}

// One file's structured diff. `added`/`removed` are absent (not 0) for
// binary files; `previousPath` only on renames.
export interface FileDiff {
  path: string;
  status: "added" | "modified" | "deleted" | "renamed" | "untracked";
  previousPath?: string;
  added?: number;
  removed?: number;
  binary?: true;
  rows: DiffRow[]; // [] for binary files
}

// A single edit's applied result (tool `result` convention, §4.4.2) — carries
// a diff, no `untracked`. Shares the past-tense `status` vocabulary with
// WorkspaceFileChange but is a distinct type (§4.5).
export interface FileEdit {
  path: string;
  status: "added" | "modified" | "deleted" | "renamed";
  diff?: DiffRow[];
}

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

// VCS working-tree scan state (workspace.listFileChanges, AUX_API §2.2) —
// includes `untracked`. Distinct from FileEdit (one edit's applied result);
// they share the past-tense `status` vocabulary by design (§4.5).
// `added`/`removed` line counts are absent (never a fabricated 0) for binary
// files; `previousPath` only on renames.
export interface WorkspaceFileChange {
  path: string;
  status: "added" | "modified" | "deleted" | "renamed" | "untracked";
  previousPath?: string;
  added?: number;
  removed?: number;
  binary?: true;
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

// Inclusive totals (provider-reported; each includes its sub-items) +
// non-overlapping sub-items (each independently labelled — clients never
// subtract, no underflow). §4.6.
export interface ModelUsage {
  inputTokens?: number; // total input (includes cacheRead portion)
  outputTokens?: number; // total output (includes reasoning portion)
  cacheReadTokens?: number; // subset of inputTokens that hit cache
  cacheWriteTokens?: number; // tokens written to cache
  reasoningTokens?: number; // subset of outputTokens that is hidden reasoning
  costUsd?: number; // top-level Usage = total cost; byModel entry = that model's cost. Omitted when model not in pricing table (never fabricate 0).
}

export interface Usage extends ModelUsage {
  byModel?: Record<string, ModelUsage>; // per-model split (not recursive); entries are the same shape (incl. cache)
}

// usage.summary (§7.7) — cross-session spend report, summed from the durable
// run history. Buckets sum whole runs so the breakdowns reconcile with `total`.
export interface UsageSummaryRequest {
  sinceDays?: number; // limit to runs finished in the last N days; omit/0 = all time
}

// One grouped slice of usage — a provider id, model id, or day (YYYY-MM-DD).
export interface UsageBucket extends ModelUsage {
  key: string;
  runs?: number; // runs that contributed
}

export interface UsageSummary {
  total: ModelUsage;
  byProvider?: UsageBucket[]; // spend-ranked
  byModel?: UsageBucket[]; // spend-ranked
  byDay?: UsageBucket[]; // chronological
  sessions?: number; // user-facing sessions with recorded spend
  runs?: number; // finished runs counted
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
  channel?: "rpc" | "run" | "tool"; // self-describing: which delivery channel this error came from (§8.1)
  detail?: string; // per-occurrence human explanation
  docUrl?: string; // optional: points at this type's doc page (lowers onboarding cost)
  retryable?: boolean;
  retryAfterSeconds?: number; // earliest retry (e.g. provider rate-limit backoff)
  errors?: FieldError[]; // field-level validation errors (invalid_params / forms)
  [key: string]: unknown; // still open for type-specific extension members
}

// ---------------------------------------------------------------------------
// §4.7 — Tool specs
// ---------------------------------------------------------------------------

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

// All three interrupt types are "payload is enough to render" — none needs a
// second request (API.md §4.8). approval / toolResult reuse ToolInvocation
// (read payload.tool — name+arguments always present). question is
// self-contained (S1): its payload carries the Question, so no items.list join.
export type Interrupt =
  // Payload stays optional at the trust boundary so the fold can reject a
  // minimal/non-conformant backend without stranding an un-actionable card.
  | { type: "approval"; itemId: ItemId; payload?: ApprovalPayload }
  | { type: "question"; itemId: ItemId; payload?: { question: Question } }
  | { type: "toolResult"; itemId: ItemId; payload?: ToolResultPayload };

export interface ApprovalPayload {
  tool: ToolInvocation; // the tool awaiting approval (result not yet present)
  risk?: "low" | "medium" | "high";
  reason?: string;
  rememberable?: boolean; // true when this approval may create a standing rule; absent means false
}
export interface ToolResultPayload {
  tool: ToolInvocation; // a client-side tool to execute; result returned via runs.resume
}

export interface OpenInterrupt {
  runId: RunId; // the Run to resume (its current segment ended with outcome.type=interrupt)
  sessionId: SessionId;
  interrupts: Interrupt[];
  createdAt: string;
}

// §6.1 — InterruptResponse (sent via runs.resume).
export interface ApprovalResponse {
  type: "approval";
  decision: "approve" | "deny";
  // Remember this decision (works for deny too) as a persistent fine-grained
  // rule (AUX_API §6) — the runtime keys it by tool + the call's per-tool
  // subject (shell command / file path), scoped to the session, the project
  // dir, or globally. Legal only when the pending ApprovalPayload says
  // rememberable=true; omitted = this once only.
  remember?: { scope: ApprovalScope };
  editedArgs?: Record<string, unknown>; // one-shot input rewrite — NOT part of remember
  reason?: string;
}
export interface AnswerResponse {
  type: "answer";
  answers: Record<string, string[]>; // key = QuestionField.name; single-select = single-element array (S8)
}
export interface ToolResultResponse {
  type: "toolResult";
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
  // `id` is BOTH identity and type discriminator (e.g. "groq" / "openai-compatible") —
  // there is no separate `type` field (API.md §4.9).
  id: string;
  baseUrl?: string;
  apiKeyMasked: string; // "" = unset; e.g. "sk-…fc78"; never reversible
  // Provenance of the key: "stored" (set via providers.configure, editable) or
  // "env" (read from the provider's env var, read-only — shown as "from env").
  // Omitted when unconfigured (apiKeyMasked is also ""). API.md §4.9.
  keySource?: "stored" | "env";
  // No built-in endpoint (generic openai/anthropic-compatible passthrough +
  // Azure): config MUST collect baseUrl, and with no catalog the model is
  // free-text input (models.list returns empty). API.md §4.9.
  requiresBaseUrl?: boolean;
  // Has an embeddings adapter → offered in the @codebase embedding-role picker;
  // defaultEmbeddingModel prefills a sensible model id ("" = user-supplied).
  embeddingCapable?: boolean;
  defaultEmbeddingModel?: string;
}

export interface Model {
  id: string;
  provider: string; // Provider.id
  displayName?: string;
  contextWindow?: number;
  maxInputTokens?: number;
  maxOutputTokens?: number;
  knowledgeCutoff?: string; // training cutoff (YYYY-MM-DD); omitted when unknown
  deprecated?: boolean; // provider retired it; client hides or marks
  capabilities?: ModelCapabilities;
  pricing?: ModelPricing;
}

// Media type (input/output modality), same values as the core chat.Modality.
// Open enum (API.md §4.9).
export type Modality = "text" | "image" | "audio" | "video" | "pdf";

export interface ModelCapabilities {
  reasoning?: boolean; // supports extended thinking
  reasoningLevels?: string[]; // discrete effort tiers (ascending, e.g. ["low","medium","high"]); empty for budget-style / unsupported
  reasoningDefaultLevel?: string; // default tier when none specified; empty when no tiers
  multimodal?: boolean; // convenience bit: accepts image input (full set in inputModalities)
  inputModalities?: Modality[]; // all accepted media types (text first, then image/pdf/audio/…)
  outputModalities?: Modality[]; // produced media types (chat models = text)
  toolUse?: boolean; // tool / function calling
  structuredOutput?: boolean; // native structured-output / JSON-schema
}

// Headline rate tiers. Cache fields are 0 when the provider doesn't price cache
// separately; threshold-tiered long-context pricing gives only the base tier.
export interface ModelPricing {
  inputUsdPerMillionTokens?: number;
  outputUsdPerMillionTokens?: number;
  cacheReadUsdPerMillionTokens?: number;
  cacheWriteUsdPerMillionTokens?: number;
}

export interface ConfigureProviderRequest {
  provider: string; // Provider.id / slug — must be a backend-supported provider
  baseUrl?: string; // override default endpoint (proxy / gateway / self-hosted)
  apiKey?: string;
}

export interface ProviderTestResult {
  ok: boolean;
  error?: ProblemData;
}

// The (provider, model) the in-house maintenance work (compaction / extraction
// / titling) runs on (models.getUtilityRole / setUtilityRole). Empty model =
// unset → that work runs on the main turn model. `provider` is a Provider.id.
export interface UtilityRole {
  provider?: string;
  model?: string;
}

// The (embedding-capable provider, model) the @codebase semantic index embeds
// with (models.getEmbeddingRole / setEmbeddingRole). Empty model = unset → the
// @codebase feature is off. `provider` is an embedding-capable Provider.id.
export interface EmbeddingRole {
  provider?: string;
  model?: string;
}

// @codebase semantic index (codebase.*).
export type CodebaseState = "none" | "indexing" | "ready" | "error";
export interface CodebaseHit {
  path: string; // relative to cwd
  startLine: number;
  endLine: number;
  snippet: string;
  score: number; // cosine similarity [0,1]
}
export interface CodebaseStatus {
  operationId?: string;
  state: CodebaseState;
  modelId?: string;
  fileCount: number;
  chunkCount: number;
  indexedAt?: string; // RFC3339
  truncated?: boolean;
  error?: string;
}

// ---------------------------------------------------------------------------
// §4.10 — Workspace / optional-domain types
// ---------------------------------------------------------------------------

export interface Skill {
  name: string;
  description?: string;
  source?: string;
}

// A managed skill's curator state (skills.library.list): active (loadable by
// the agent) or archived (preserved, not loaded).
export type SkillLifecycle = "active" | "archived";

// One entry in the global self-authored skill library (skills.library.list).
// Distinct from Skill (the agent's project+global discovery view): this is the
// management surface, which also lists archived skills.
export interface ManagedSkill {
  name: string;
  description?: string;
  lifecycle: SkillLifecycle;
}

// One agent-mined skill proposal awaiting offline review (skills.drafts.list).
// name+revision form the content-addressed handle a promote/reject call carries;
// createdBy/sourceSession are the provenance read from the draft's frontmatter
// (createdBy is "agent" for a mined draft).
export interface SkillDraft {
  name: string;
  revision: string;
  description?: string;
  createdBy?: string;
  sourceSession?: string;
}

// Identifies the exact staged draft a skills.drafts.promote / reject call acts
// on. revision binds the name to the immutable bytes so a decision cannot act on
// a different revision than the one reviewed.
export interface SkillDraftRef {
  name: string;
  revision: string;
}

// agentMemory.* — the HITL review surface over the agent's self-maintained
// memory. Distinct from MemoryEntry (the user-authored LYRA.md cascade): these
// are addressable items the agent mines from sessions, held `pending` until a
// human approves; only `active` items reach the prompt or the memory_search
// tool. origin distinguishes mined (`auto`) from user-authored (`user`).
export type AgentMemoryScope = "project" | "user";
export type AgentMemoryOrigin = "auto" | "user";
export type AgentMemoryStatus = "active" | "pending";

export interface AgentMemoryItem {
  id: string;
  scope: AgentMemoryScope;
  content: string;
  origin: AgentMemoryOrigin;
  status: AgentMemoryStatus;
  pinned: boolean;
  sessionId?: string;
  day?: string;
  createdAt: string;
  updatedAt: string;
}

export interface AgentMemoryList {
  items: AgentMemoryItem[];
}

// goals.* — Goal mode, an autonomous loop that drives runs toward an objective
// until the model signals complete/blocked (update_goal tool), an opt-in
// cross-turn budget is spent, or the user stops it. A session has at most one
// goal; a completed goal is cleared (never appears). status is
// active | paused | blocked.
export type GoalStatus = "active" | "paused" | "blocked";

// The opt-in cross-turn cap. A zero/omitted field is unbounded on that axis
// (all unset = uncapped, an explicit choice).
export interface GoalBudget {
  maxTurns?: number;
  maxCostUsd?: number;
  maxSteps?: number;
}

export interface GoalUsage {
  turns: number;
  costUsd: number;
  steps: number;
}

export interface Goal {
  sessionId: string;
  objective: string;
  status: GoalStatus;
  reason?: string;
  provider?: string;
  model?: string;
  budget: GoalBudget;
  used: GoalUsage;
  createdAt: string;
  updatedAt: string;
}
// A recipe is a user-invoked, parameterized prompt template (recipes.
// list). The client expands the body's $ARGUMENTS / $1..$9 with the user's input
// and sends the result as a turn; body travels with the listing (recipes are
// small). name doubles as the slash command (review → /review).
export type RecipeScope = "project" | "global";
export interface Recipe {
  name: string;
  description?: string;
  argumentHint?: string;
  body: string;
  scope: RecipeScope;
  source: string;
}
// A scheduled run (schedules.*): a saved prompt fired on a cron trigger as a
// headless run. cron is a 5-field standard expression ("min hour dom month dow").
// lastRunAt is absent until first fired; nextRunAt is absent when disabled.
export interface Schedule {
  id: string;
  title: string;
  prompt: string;
  cwd?: string;
  provider?: string;
  model?: string;
  cron: string;
  enabled: boolean;
  lastRunAt?: string;
  nextRunAt?: string;
  createdAt: string;
  revision: number;
}
// The editable fields a schedules.create / update carries (create is always
// enabled; update adds id + enabled).
export interface ScheduleInput {
  title?: string;
  prompt: string;
  cwd?: string;
  provider?: string;
  model?: string;
  cron: string;
}
export interface AgentDoc {
  path: string;
  title?: string;
  scope: "cwd" | "projectRoot" | "home";
}
export type McpStatus = "connecting" | "connected" | "disconnected" | "failed" | "needsAuth";

// Enriched server entry (AUX_API §5.1) — toolCount/authStatus inline so the
// list view needs no listServers⨝listTools join; `error` is set on failed
// (the UI shows WHY, not just a red pill).
export interface McpServer {
  name: string;
  status: McpStatus;
  toolCount?: number;
  authStatus?: "none" | "bearerToken" | "oauth" | "notLoggedIn";
  error?: ProblemData;
  description?: string;
}
export interface McpTool {
  server: string;
  name: string;
  description?: string;
  inputSchema?: Record<string, unknown>;
}

// How a configured MCP server is reached: a local subprocess (stdio) or a
// remote streamable-HTTP endpoint. The two transports gate disjoint config
// fields (command/args/env/dir vs url/authorization), §4.10.
// Transport in the standard `mcpServers` vocabulary (the config every client
// pastes). stdio = local subprocess; streamableHttp = remote Streamable HTTP.
export type McpTransport = "stdio" | "streamableHttp";

// One entry in the editable MCP registry (mcp.configs.list /
// configure). Carries the persisted config only — live status
// (status/toolCount/error) comes from mcp.servers.list
// (McpServer), joined by server name.
// `authorizationMasked` is the never-reversible echo of an http server's
// stored bearer token ("" / absent = none); the raw token only travels on
// ConfigureMCPServerRequest (write side).
export interface McpServerConfig {
  name: string;
  type: McpTransport;
  enabled: boolean;
  description?: string;
  // http transport. `headers` is an extra static request-header map (e.g.
  // X-API-Key) — NOT masked (treated as non-secret config); a bearer token
  // belongs in authorization, which stays masked.
  url?: string;
  authorizationMasked?: string;
  headers?: Record<string, string>;
  // stdio transport — env is a KEY→value map (replaces the subprocess env).
  command?: string;
  args?: string[];
  env?: Record<string, string>;
  dir?: string;
  // Connection-handshake timeout in seconds; 0/absent = unbounded.
  timeoutSeconds?: number;
  // Per-tool gating (§4.10): disabledTools is a blacklist (tool name → hidden
  // from the agent); autoApproveTools is a whitelist (tool name → skips the
  // approval prompt). Both key on the bare tool name (NOT "<server>.<tool>").
  disabledTools?: string[];
  autoApproveTools?: string[];
}

// mcp.configs.configure — upsert by `name`. `authorization` is the RAW bearer
// token (NOT the masked echo): omitted/empty KEEPS the already-stored token, so
// editing a non-secret field never forces a token re-entry. The runtime returns
// the resulting McpServerConfig with the token re-masked.
export interface ConfigureMCPServerRequest {
  name: string;
  type: McpTransport;
  enabled: boolean;
  description?: string;
  url?: string;
  authorization?: string;
  headers?: Record<string, string>;
  command?: string;
  args?: string[];
  env?: Record<string, string>;
  dir?: string;
  timeoutSeconds?: number;
  disabledTools?: string[];
  autoApproveTools?: string[];
}

// mcp.configs.setEnabled — flip a registered server's enablement without
// re-sending its whole config.
export interface SetMCPEnabledRequest {
  name: string;
  enabled: boolean;
}

// mcp.configs.test — live connection probe (dry-run, NOT persisted). A
// failed probe comes back as `{ ok:false, error }` (a ProblemData), never an
// RPC error, so the pane renders the reason inline (mirrors ProviderTestResult).
export interface McpTestResult {
  ok: boolean;
  error?: ProblemData;
}

// Lifecycle-hook events the runtime fires at fixed turn boundaries (§7.5). A
// hook matches one event; PreToolUse/PostToolUse additionally gate on a
// tool-name `matcher`.
export type HookEvent =
  | "PreToolUse"
  | "PostToolUse"
  | "UserPromptSubmit"
  | "SessionStart"
  | "SubagentStart"
  | "SubagentStop"
  | "PreCompact"
  | "Stop"
  | "Notification";

// One discovered hook (hooks.list), for review before trusting.
// `command` (shown so the user can audit a project's hooks) and `inject` (the
// declarative no-exec context alternative) are mutually exclusive. `active`
// reports whether it currently runs: global hooks always do, project hooks only
// once the project is trusted.
export interface HookInfo {
  event: HookEvent;
  matcher?: string;
  command?: string;
  inject?: string;
  timeoutMs?: number;
  scope: "global" | "project";
  source: string; // absolute path of the hooks.json it came from
  active: boolean;
}

// hooks.list result — the discovered hooks plus the project's trust
// status. projectRoot is the trust key (the nearest .git ancestor of the cwd);
// projectTrusted reports whether its project-scope hooks are enabled. A cloned
// repo's project hooks stay inert (active:false) until the user trusts it.
export interface HooksListResult {
  projectRoot?: string;
  projectTrusted: boolean;
  hooks: HookInfo[];
}

export type MemoryScope = "cwd" | "projectRoot" | "home";
export interface MemoryEntry {
  scope: MemoryScope;
  path: string;
  content: string;
  updatedAt?: string;
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

export type ItemDelta =
  | { type: "content"; index?: number; text: string } // agentMessage text delta
  | { type: "reasoning"; text: string } // reasoning text delta
  | { type: "toolArguments"; argumentsTextDelta: string } // partial JSON text of tool args
  | { type: "toolOutput"; text: string } // PREVIEW of command stdout — authoritative copy lands on the completed item (commandExecution.output)
  | { type: "plan"; steps: PlanStep[] }; // current full plan (not a hot char stream)

export type StreamEvent =
  | { type: "segment.started"; run: RunRef }
  | { type: "segment.progress"; progress: RunProgress } // ephemeral; authoritative final usage/steps land on segment.finished.result
  | { type: "segment.finished"; outcome: RunOutcome }
  | { type: "item.started"; item: Item } // shell (status=running)
  | { type: "item.delta"; itemId: ItemId; delta: ItemDelta }
  | { type: "item.completed"; item: Item } // authoritative terminal, durable
  | { type: "state.snapshot"; state: Record<string, unknown> }
  | { type: "custom"; name: string; durable?: boolean; payload: unknown }; // durable carried on-frame (default false)

// Mid-run progress preview — a live readout of step/usage/cost while the Run
// streams. Ephemeral like item.delta: dropping every segment.progress still yields
// the correct totals from segment.finished.result (the authoritative landing), so
// §5.2's durable invariant holds. Suppressible via excludedEvents.
// Cumulative cost reads `usage.costUsd` — no separate RunProgress.costUsd (§5).
export interface RunProgress {
  step?: number; // agent steps elapsed so far
  usage?: Usage; // cumulative usage so far (cost via usage.costUsd)
  contextTokens?: number; // latest round's prompt size = live context-window occupancy (vs cumulative usage.inputTokens); pair with model.contextWindow for a gauge
  activity?: string; // human-readable current action ("calling tool: shell")
}

export type StreamEventType = StreamEvent["type"];

export const STREAM_EVENT_TYPES = [
  "segment.started",
  "segment.progress",
  "segment.finished",
  "item.started",
  "item.delta",
  "item.completed",
  "state.snapshot",
  "custom",
] as const satisfies readonly StreamEventType[];

// The RunEvent envelope does NOT carry `durable` (S4). For all first-party
// events durability is a pure function of `event.type` (see DURABLE_EVENT_TYPES
// / isDurableEvent); only `custom` carries its own on-frame `durable?`. A
// redundant per-frame bool would admit "item.completed yet durable:false" —
// a self-contradictory illegal state — so it's removed (API.md §5.2,
// TRANSPORT §6.4).
export interface RunEvent {
  runId: RunId; // the stable Run this event belongs to
  segmentId: SegmentId; // the streamed segment this event belongs to (a resume = a new segment)
  eventId: string; // evt_…; monotonic within a single root SEGMENT stream (§2.4)
  timestamp: string; // ISO-8601
  event: StreamEvent;
}

// Durable derivation table (API.md §5.2, authoritative). Every ephemeral event
// has a named durable landing; clients may opt out of ephemeral deltas and
// still reconstruct correct terminal state.
const DURABLE_EVENT_TYPES: ReadonlySet<StreamEventType> = new Set<StreamEventType>([
  "segment.started",
  "segment.finished",
  "item.started",
  "item.completed",
  "state.snapshot",
]);

/** Is this StreamEvent durable (authoritative/listable)? Derived from
 *  `event.type`; `custom` carries its own on-frame `durable?` (default false). */
export function isDurableEvent(event: StreamEvent): boolean {
  if (event.type === "custom") return event.durable ?? false;
  return DURABLE_EVENT_TYPES.has(event.type);
}

// ---------------------------------------------------------------------------
// §7.3 — Run request shapes
// ---------------------------------------------------------------------------

export interface StartRunRequest {
  sessionId: SessionId; // cwd resolved from session; no cwd, no runId on this request
  input: ContentBlock[]; // user message body
  // provider + model are a PAIR (API §7.3): send both or neither. Only one →
  // invalid_params. provider is NOT inferred from model (same model id can
  // span providers). Both come straight from models.list's Model.{provider,id}.
  provider?: string;
  model?: string;
  maxSteps?: number; // ceiling → outcome.maxSteps
  maxBudgetUsd?: number; // incl. subagent subtree → outcome.maxBudget
  params?: GenerationParams;
}

export interface ResumeRunRequest {
  runId: RunId; // the Run to resume (its current segment ended with outcome.type=interrupt)
  responses: InterruptResponse[]; // one per open interrupt, addressed by itemId
}

export interface StartRunResponse {
  runId: RunId; // the stable logical Run
  segmentId: SegmentId; // the streamed segment this call opens (root of the event stream)
  // The opening userMessage Item's id — same id as on the stream's
  // item.started/completed and in items.list. The client reconciles its
  // optimistic bubble by this exact id (no content-text heuristic). Absent on
  // runs.resume (no opening user turn). A business field, not transport meta.
  userItemId?: ItemId;
}

export interface ResumeRunResponse {
  runId: RunId; // the SAME Run being resumed (unchanged across resume)
  segmentId: SegmentId; // a NEW segment of that Run — the root of this resume's event stream
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
  runs: RunRef[]; // owning Runs (finished/running), with spawnedByItemId run-tree edges
}

// ---------------------------------------------------------------------------
// §7.5 — Workspace
// ---------------------------------------------------------------------------

export interface WorkspaceQuery {
  cwd?: string; // default = serve dir
}

// ---------------------------------------------------------------------------
// §7.5 — File browse (workspace.listFiles / readFile)
// ---------------------------------------------------------------------------

export interface FileEntry {
  path: string; // relative to cwd
  name: string; // basename
  type: "file" | "dir" | "symlink";
  sizeBytes?: number; // file only
  modifiedAt: string; // ISO-8601 (sortable)
}
// workspace.readFile result. `startLine`/`endLine` echo the served range —
// 1-based inclusive (editor-facing).
export interface FileContent {
  path: string;
  content: string; // full text, or the requested line slice
  encoding: "utf-8"; // text only
  totalLines: number; // full-file line count even for a slice (UI shows "12–40 / 320")
  truncated?: boolean; // hit maxBytes (self-describing, no silent cap)
  startLine?: number;
  endLine?: number;
}

// ---------------------------------------------------------------------------
// §7.9 / §7.2 / §7.10 — Approval control · compaction · todos
// ---------------------------------------------------------------------------

// B9 — global approval stance (one per Runtime, not per-session). Orthogonal to
// Item.toolCall.safetyClass (per-tool risk): mode is the global policy, the two
// combine to decide whether a call parks.
export type ApprovalMode =
  | "plan" // read-only planning stance: write/exec/network denied (no prompt); exit_plan_mode flips back to execute
  | "safe" // every write/exec tool parks
  | "balanced" // default: high-risk parks, low-risk passes (by safetyClass)
  | "yolo"; // everything passes, no parking (automation)
// How far a remembered approval rule reaches (AUX_API §6): one session, one
// project directory, or everywhere.
export type ApprovalScope = "session" | "project" | "global";

// One persisted fine-grained approval rule (approval.listRules, AUX_API §6).
// A rule auto-resolves a gated call when scope + tool + subject all match.
export interface ApprovalRule {
  id: string; // stable id (forgetRule key)
  scope: ApprovalScope;
  tool: string; // tool name, e.g. "shell"
  subject?: string; // command/path glob the rule matches; omitted = any arguments
  dir?: string; // project-scope directory (display only; omitted for session/global)
  decision: "allow" | "deny";
}

// B11 — the model's working checklist (todo_write), NOT the removed background.*
// task registry. Live updates ride the existing state.snapshot channel (§5.3) —
// no new event type; folding it into a view field is deferred to the fold phase.
export interface TodoItem {
  id: string;
  text: string;
  status: "pending" | "in_progress" | "completed";
}

// ---------------------------------------------------------------------------
// AUX_API §3 — workspace notification channel (workspace.subscribe)
// ---------------------------------------------------------------------------

// One watch registration. Scope = the subscribe stream that carried it (no
// standalone watch/unwatch methods); changing the watch set means
// resubscribing. Gated by features.fileWatch.
//
// NOT a recursive file watcher (AUX_API §3.1 watch model): the backend
// watches the cwd's .git signal set and emits a debounced `resync` on any
// git-state change; the agent's own write/edit tools push precise
// `files.changed{cwd, paths}` from the run stream. External non-git edits
// are not real-time (next git operation / manual refresh picks them up).
export interface WatchSpec {
  watchId: string; // client-named
  cwd?: string; // per-watch cwd (default = serve dir); jail same as §7.5
  path?: string; // currently unused under the git-state watch model
}

export interface SubscribeWorkspaceRequest {
  watches?: WatchSpec[];
}

// Lossy "something changed → refetch" signals. sequence is process-monotonic;
// a gap means the client must invalidate and refetch its workspace caches. A
// (re)subscribe is an implicit `resync`. Type names are globally unique
// across the run/workspace event unions (optOut matches by type name).
export type WorkspaceEvent =
  | { type: "files.changed"; sequence: number; watchId?: string; paths: string[]; cwd?: string }
  | { type: "skills.changed"; sequence: number }
  | {
      type: "mcp.serverChanged";
      sequence: number;
      server: string;
      status?: McpStatus;
      toolCount?: number;
      error?: ProblemData;
    } // status absent = entry removed
  | { type: "schedules.fired"; sequence: number; scheduleId?: string }
  | { type: "resync"; sequence: number };

export type WorkspaceEventType = WorkspaceEvent["type"];

// ---------------------------------------------------------------------------
// §7.6 — tools.invoke
// ---------------------------------------------------------------------------

export interface InvokeToolRequest {
  name: string;
  arguments: Record<string, unknown>;
  cwd?: string;
}

// ---------------------------------------------------------------------------
// §7.7 — Optional domain (feedback)
// ---------------------------------------------------------------------------

export interface FeedbackRequest {
  sessionId?: SessionId;
  runId?: RunId;
  itemId?: ItemId;
  rating?: "positive" | "negative";
  text?: string;
}
