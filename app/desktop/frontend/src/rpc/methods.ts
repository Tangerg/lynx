// Typed wrappers for every method in docs/protocol/API.md §7. Grouped by namespace
// so callers do `methods.runs.start(...)` rather than
// `client.call("runs.start")`. The factory takes an RpcClient and returns
// the full typed surface.
//
// Streaming methods (runs.start / runs.resume / runs.subscribe) return
// `{ result, events }` where `events` is an AsyncIterable. Run streams
// carry the whole run tree and end on the root segment's `segment.finished`
// (see ./stream).

import type { RpcCallOptions, RpcClient } from "./client";
import { isErrorType, RpcError } from "./errors";
import { unnegotiated } from "./preflight";
import type { RunId, SegmentId, SessionId } from "./ids";
import type {
  AgentDoc,
  ApprovalMode,
  ApprovalModeResult,
  CancelRunResponse,
  ConfigureMCPServerRequest,
  ConfigureProviderRequest,
  CreateSessionRequest,
  Diff,
  ExportSessionResponse,
  FeedbackRequest,
  FileContent,
  FileEntry,
  FileHead,
  ForkSessionRequest,
  GrepResult,
  HooksListResult,
  ImportSessionResponse,
  DiscoverResponse,
  CodebaseReindexResponse,
  CodebaseSearchResult,
  CodebaseStatus,
  EmbeddingRole,
  InvokeToolRequest,
  ListApprovalRulesResult,
  ListItemsResponse,
  McpServer,
  McpServerConfig,
  McpTestResult,
  McpTool,
  MemoryEntry,
  MemoryScope,
  Model,
  PendingInterruptSet,
  Page,
  PageQuery,
  Project,
  Provider,
  ProviderTestResult,
  ResumeRunRequest,
  StartRunResponse,
  SubscribeRunRequest,
  SubscribeRunResponse,
  RollbackSessionRequest,
  RollbackSessionResponse,
  RunEvent,
  Recipe,
  ItemListScope,
  ItemOrder,
  RunRef,
  RunStatus,
  RunScheduleNowResponse,
  Schedule,
  CreateScheduleRequest,
  ServerCapabilities,
  StateSnapshot,
  Session,
  SessionArtifact,
  Skill,
  ManagedSkill,
  SkillDraft,
  SkillDraftRef,
  AgentMemoryItem,
  AgentMemoryList,
  AgentMemoryScope,
  Goal,
  GoalBudget,
  StartRunRequest,
  RuntimeSubscribeRequest,
  RuntimeSubscribeResponse,
  ToolSpec,
  UpdateSessionRequest,
  Usage,
  UsageSummary,
  UsageSummaryRequest,
  UtilityRole,
  RuntimeEvent,
  WorkspaceFileChange,
} from "./wire.generated";
import { streamRunEvents, streamRuntimeEvents } from "./stream";
import type { WireMethodName, WireParams, WireResult } from "./wire.methods.generated";
import { RUNTIME_SUBSCRIBE_METHOD } from "./transport";

export interface StreamingResult<R, E> {
  result: R;
  events: AsyncIterable<E>;
}

// How THIS client calls, composed from what the contract publishes: the generated
// table fixes the method names and the shapes each one carries, and the options are
// this transport's business (§12 keeps the wire narrow and lets the SDK add the
// handles). Before it, every call site below restated its own method name and result
// type, so a method renamed in the Registry surfaced as a runtime method_not_found
// instead of a compile error.
type WireCall = <M extends WireMethodName>(
  method: M,
  params: WireParams<M>,
  options?: RpcCallOptions,
) => Promise<WireResult<M>>;

// Invariant shared by every streaming method: the subscription is opened
// BEFORE the call (head-drop race, see runs.start), so if the call REJECTS
// the stream must be disposed explicitly — nobody will ever iterate
// `events`, its self-cleaning iterator never runs, and the subscription
// (plus any pre-bind buffer) would leak and accumulate forever.
async function callOrDispose<R>(
  stream: { dispose: () => void },
  call: () => Promise<R>,
): Promise<R> {
  try {
    return await call();
  } catch (err) {
    stream.dispose();
    throw err;
  }
}

const IDEMPOTENCY_RETENTION_MS = 24 * 60 * 60 * 1_000;

interface PendingMutation {
  key: string;
  createdAt: number;
}

// Idempotency belongs to the logical mutation, not an individual HTTP call.
// An indeterminate attempt remains keyed by canonical method+params so a UI or
// reconnect retry reuses the same key until a definite RPC result arrives.
class MutationAttempts {
  private readonly pending = new Map<string, PendingMutation>();

  async call<R, P>(client: RpcClient, method: string, params: P, signal?: AbortSignal): Promise<R> {
    const identity = mutationIdentity(method, params);
    const now = Date.now();
    this.prune(now);
    let attempt = this.pending.get(identity);
    if (!attempt) {
      attempt = { key: crypto.randomUUID(), createdAt: now };
      this.pending.set(identity, attempt);
    }
    try {
      const result = await client.call<R, P>(method, params, {
        signal,
        idempotencyKey: attempt.key,
      });
      this.release(identity, attempt.key);
      return result;
    } catch (error) {
      // A JSON-RPC response is authoritative, except in-progress: that outcome
      // explicitly asks the caller to retry this same logical operation/key.
      if (error instanceof RpcError && !isErrorType(error, "idempotency_in_progress")) {
        this.release(identity, attempt.key);
      }
      throw error;
    }
  }

  private release(identity: string, key: string): void {
    if (this.pending.get(identity)?.key === key) this.pending.delete(identity);
  }

  private prune(now: number): void {
    for (const [identity, attempt] of this.pending) {
      if (now - attempt.createdAt >= IDEMPOTENCY_RETENTION_MS) this.pending.delete(identity);
    }
  }
}

function mutationIdentity(method: string, params: unknown): string {
  return `${method}\0${canonicalJSON(params)}`;
}

function canonicalJSON(value: unknown): string {
  if (value === null || typeof value !== "object") return JSON.stringify(value) ?? "null";
  if (Array.isArray(value)) return `[${value.map(canonicalJSON).join(",")}]`;
  const entries = Object.entries(value as Record<string, unknown>)
    .filter(([, item]) => item !== undefined)
    .sort(([left], [right]) => left.localeCompare(right));
  return `{${entries
    .map(([key, item]) => `${JSON.stringify(key)}:${canonicalJSON(item)}`)
    .join(",")}}`;
}

export interface Methods {
  runtime: {
    discover: () => Promise<DiscoverResponse>;
  };
  sessions: {
    list: (query?: PageQuery) => Promise<Page<Session>>;
    get: (sessionId: SessionId) => Promise<Session>;
    create: (params?: CreateSessionRequest, signal?: AbortSignal) => Promise<Session>;
    update: (params: UpdateSessionRequest) => Promise<Session>;
    delete: (sessionId: SessionId) => Promise<void>;
    fork: (params: ForkSessionRequest) => Promise<Session>;
    // Turn-granular history truncation (AUX_API §4.1). Rejected with
    // session_busy while a run is in flight. restoreType files|both also
    // restores the working tree (gated features.checkpoints).
    rollback: (params: RollbackSessionRequest) => Promise<RollbackSessionResponse>;
    export: (sessionId: SessionId, format?: "md" | "json") => Promise<ExportSessionResponse>;
    // Restore semantics — rebuilds under the artifact's original id (idempotent).
    import: (artifact: SessionArtifact) => Promise<ImportSessionResponse>;
  };
  runs: {
    start: (
      params: StartRunRequest,
      signal?: AbortSignal,
    ) => Promise<StreamingResult<StartRunResponse, RunEvent>>;
    resume: (
      params: ResumeRunRequest,
      signal?: AbortSignal,
    ) => Promise<StreamingResult<StartRunResponse, RunEvent>>;
    // Reattach to a run's live segment. Both ids are required: a subscription that
    // named only the run would attach to whatever segment happens to be executing,
    // and a client folding an earlier one would continue into a different execution
    // without being able to tell. A mismatch is stale_segment.
    subscribe: (
      params: SubscribeRunRequest,
      signal?: AbortSignal,
      // A reattach resumes from the last event the caller folded: the runtime replays
      // from just after it, or refuses (replay_unavailable / replay_cursor_invalid)
      // when that position is no longer addressable. Omitted means tail-only — the
      // history belongs to items.list, not to a stream that would deliver it twice.
      options?: { lastEventId?: string },
    ) => Promise<StreamingResult<SubscribeRunResponse, RunEvent>>;
    cancel: (runId: RunId, reason?: string) => Promise<CancelRunResponse>;
    // Mid-run steering (§6): inject a user message into the segment the caller
    // believes is executing, so the model reads it next tool round. The segment is
    // named for the same reason: a run that parked and resumed between typing and
    // sending must refuse (stale_segment) rather than deliver the instruction to
    // work the person never saw.
    steer: (runId: RunId, expectedSegmentId: SegmentId, message: string) => Promise<void>;
    // One run by id — current or terminal — without knowing its session (§7.3).
    get: (runId: RunId) => Promise<RunRef>;
    // The durable run history, newest first (§7.3). Omitting statuses returns every
    // position; asking for descendants is refused while features.subagents is off.
    list: (
      query?: PageQuery & {
        sessionId?: SessionId;
        statuses?: RunStatus[];
        includeDescendants?: boolean;
      },
    ) => Promise<Page<RunRef>>;
  };
  todos: {
    // The cold read the todos state key declares (§5.6). A session with no list yet
    // answers with the empty state at revision 0 — the same shape the stream pushes.
    get: (sessionId: SessionId) => Promise<StateSnapshot>;
  };
  interrupts: {
    // Durable HITL discovery — the waiting sets, longest wait first (§7.3 / §10.2).
    // A page never splits a set: a set is what runs.resume answers in one call.
    list: (
      query?: PageQuery & { sessionId?: SessionId; rootRunId?: RunId },
    ) => Promise<Page<PendingInterruptSet>>;
  };
  items: {
    // The scope is required and closed (§7.4): a whole session timeline, or one
    // run's own items. `order` defaults to "asc" — the order the runtime produced,
    // which is the one a fold can replay.
    list: (params: {
      scope: ItemListScope;
      order?: ItemOrder;
      cursor?: string;
      limit?: number;
    }) => Promise<ListItemsResponse>;
  };
  workspace: {
    listFileChanges: (cwd?: string) => Promise<Page<WorkspaceFileChange>>;
    getDiff: (params?: {
      cwd?: string;
      path?: string;
      mode?: "worktree" | "base"; // default worktree (includes untracked); base = vs default-branch merge-base
      format?: "rows" | "raw"; // default rows
      limit?: number; // row cap, truncated at file boundaries
    }) => Promise<Diff>;
    getFileHead: (params: { path: string; cwd?: string; lines?: number }) => Promise<FileHead>;
    grep: (params: {
      query: string;
      cwd?: string;
      path?: string;
      limit?: number;
    }) => Promise<GrepResult>;
    // General directory listing / glob — feeds the file tree + @file.
    // Respects .gitignore + backstop excludes unless includeIgnored; not gated (basic read).
    listFiles: (params: {
      cwd?: string;
      path?: string; // start dir, relative to cwd (default = cwd root)
      glob?: string; // e.g. "**/*.go"; implies recursive
      recursive?: boolean; // default false — one level (lazy tree)
      includeIgnored?: boolean; // default false
      cursor?: string;
      limit?: number;
    }) => Promise<Page<FileEntry>>;
    // Full-text file read (B8) — startLine/endLine are 1-based inclusive; truncated self-describes.
    readFile: (params: {
      path: string;
      cwd?: string;
      startLine?: number;
      endLine?: number;
      maxBytes?: number;
    }) => Promise<FileContent>;
    listProjects: () => Promise<Page<Project>>;
  };
  // The app-wide change-signal channel (§7): lossy "this moved → read it again"
  // events, connection-scoped, no replay. One stream per app; resubscribing IS the
  // resync. `topics` is required — a subscription says what it can fold.
  runtimeEvents: {
    subscribe: (
      params: RuntimeSubscribeRequest,
      signal?: AbortSignal,
    ) => Promise<StreamingResult<RuntimeSubscribeResponse, RuntimeEvent>>;
  };
  // Prompt recipes (§7.5): the parameterized prompt templates discovered for a
  // cwd (project over global). The client expands a chosen recipe's body and
  // sends it as a turn — read-only discovery.
  recipes: {
    list: (cwd?: string) => Promise<Page<Recipe>>;
  };
  // Lifecycle hooks (§7.5): list the hooks discovered for a cwd (global +
  // project, each marked active = does-it-currently-run) and toggle whether a
  // project's hooks are trusted to run. A cloned repo's project hooks stay
  // inert until trusted; the toggle takes effect on the next turn.
  hooks: {
    list: (cwd?: string) => Promise<HooksListResult>;
    setTrust: (projectRoot: string, trusted: boolean) => Promise<void>;
  };
  // Self-authored skill management (§7.7). listDiscovered is the agent's
  // project+global discovery view; the library surface adds archived skills and
  // archive/restore (never deleting); the drafts surface is the offline HITL
  // review queue for agent-mined proposals — promote publishes one into the
  // active library, reject discards it. listDrafts/promote/reject are
  // capability-gated (reject with capability_not_negotiated when authoring is
  // disabled). promoteDraft/rejectDraft carry the content-addressed ref so a
  // decision acts on the exact revision that was reviewed.
  skills: {
    listDiscovered: (cwd?: string) => Promise<Page<Skill>>;
    listLibrary: () => Promise<Page<ManagedSkill>>;
    archive: (name: string) => Promise<void>;
    restore: (name: string) => Promise<void>;
    listDrafts: () => Promise<Page<SkillDraft>>;
    promoteDraft: (ref: SkillDraftRef) => Promise<void>;
    rejectDraft: (ref: SkillDraftRef) => Promise<void>;
  };
  agentDocs: {
    list: (cwd?: string) => Promise<Page<AgentDoc>>;
  };
  mcp: {
    // The editable registry (configure/remove/setEnabled) PLUS a best-effort
    // live status folded into each entry. listServers is the lighter
    // status-only view; listConfigs carries the full persisted config.
    listConfigs: (query?: PageQuery) => Promise<Page<McpServerConfig>>;
    // Upsert by name. authorization is the RAW token; omitted = keep the
    // stored one only when the HTTP origin is unchanged. Returns it re-masked.
    configure: (params: ConfigureMCPServerRequest) => Promise<McpServerConfig>;
    remove: (name: string) => Promise<void>;
    setEnabled: (name: string, enabled: boolean) => Promise<void>;
    // Dry-run connection probe (NOT persisted). A failed probe is
    // `{ ok:false, error }`, never an RPC error (mirrors providers.test).
    test: (params: ConfigureMCPServerRequest) => Promise<McpTestResult>;
    listServers: () => Promise<Page<McpServer>>;
    listTools: (server?: string) => Promise<Page<McpTool>>;
    reconnect: (server: string) => Promise<void>;
    // Interactive OAuth sign-in (opens the browser; the outcome rides
    // mcp.serverChanged, same as reconnect). For servers that auth via OAuth.
    authorize: (server: string) => Promise<void>;
  };
  providers: {
    list: () => Promise<Page<Provider>>;
    configure: (params: ConfigureProviderRequest) => Promise<Provider>;
    test: (provider: string) => Promise<ProviderTestResult>;
  };
  models: {
    list: (provider?: string) => Promise<Page<Model>>;
    // The (provider, model) the in-house maintenance work (compaction /
    // extraction / titling) runs on. Empty model = unset → it runs on the main
    // turn model. setUtilityRole validates by resolving the client server-side.
    getUtilityRole: () => Promise<UtilityRole>;
    setUtilityRole: (params: UtilityRole) => Promise<UtilityRole>;
    // The (embedding-capable provider, model) the @codebase index embeds with.
    // Empty model = unset → the feature is off. setEmbeddingRole validates by
    // building the embedding client server-side.
    getEmbeddingRole: () => Promise<EmbeddingRole>;
    setEmbeddingRole: (params: EmbeddingRole) => Promise<EmbeddingRole>;
  };
  // The @codebase semantic index (codebase.*): semantic code search, index
  // status, and a background reindex. Needs a configured embedding role.
  codebase: {
    search: (params: {
      cwd?: string;
      query: string;
      limit?: number;
    }) => Promise<CodebaseSearchResult>;
    status: (cwd?: string) => Promise<CodebaseStatus>;
    reindex: (cwd?: string) => Promise<CodebaseReindexResponse>;
  };
  tools: {
    list: () => Promise<Page<ToolSpec>>;
    invoke: (params: InvokeToolRequest) => Promise<unknown>;
  };
  // Read-only spend reporting aggregated from the durable run history (§7.7).
  usage: {
    session: (sessionId: SessionId) => Promise<Usage>;
    summary: (params?: UsageSummaryRequest) => Promise<UsageSummary>;
  };
  memory: {
    list: (cwd?: string) => Promise<Page<MemoryEntry>>;
    get: (scope: MemoryScope, cwd?: string) => Promise<MemoryEntry>;
    update: (params: { scope: MemoryScope; cwd?: string; content: string }) => Promise<void>;
  };
  // agentMemory.* (§7.7, capability-gated): the HITL review surface over the
  // agent's self-maintained memory — list active + pending items (pending
  // first), approve/reject a proposal, edit content / pin an item, delete one,
  // or add a user-authored active item. Distinct from `memory` (the LYRA.md
  // cascade). capability_not_negotiated when the store is not wired.
  agentMemory: {
    list: (params?: { scope?: AgentMemoryScope; cwd?: string }) => Promise<AgentMemoryList>;
    review: (id: string, decision: "approve" | "reject") => Promise<void>;
    update: (params: {
      id: string;
      content?: string;
      pinned?: boolean;
    }) => Promise<AgentMemoryItem>;
    delete: (id: string) => Promise<void>;
    add: (params: {
      scope?: AgentMemoryScope;
      cwd?: string;
      content: string;
    }) => Promise<AgentMemoryItem>;
  };
  // goals.* (§7.14, capability-gated): Goal mode — the autonomous execution
  // loop. get returns the session's goal or null (no goal); start opens one
  // (session_busy if one is already actively driving); stop pauses the loop;
  // resume re-activates a paused/blocked goal. Omitting provider/model runs the
  // loop on the runtime default.
  goals: {
    get: (sessionId: SessionId) => Promise<Goal | null>;
    start: (params: {
      sessionId: SessionId;
      objective: string;
      provider?: string;
      model?: string;
      budget?: GoalBudget;
    }) => Promise<Goal>;
    stop: (sessionId: SessionId) => Promise<Goal>;
    resume: (sessionId: SessionId) => Promise<Goal>;
  };
  feedback: {
    create: (params: FeedbackRequest) => Promise<void>;
  };
  // Approval runtime control (B9) — global stance + remember management. Not gated.
  approval: {
    getMode: () => Promise<ApprovalModeResult>;
    setMode: (mode: ApprovalMode) => Promise<ApprovalModeResult>;
    // Rules visible from the session: its session rules + its project's rules
    // + all global rules (the runtime resolves the session cwd).
    listRules: (sessionId: SessionId) => Promise<ListApprovalRulesResult>;
    // Remove one rule by id; clear-all = loop the visible ids.
    forgetRule: (id: string) => Promise<void>;
  };
  // Scheduled runs (§7.9): cron-triggered headless runs of a saved prompt,
  // fired by the runtime's scheduler worker while serving.
  schedules: {
    list: (query?: PageQuery) => Promise<Page<Schedule>>;
    create: (params: CreateScheduleRequest) => Promise<Schedule>;
    update: (
      params: Partial<CreateScheduleRequest> & {
        id: string;
        expectedRevision: number;
        enabled?: boolean;
      },
    ) => Promise<Schedule>;
    delete: (id: string) => Promise<void>;
    runNow: (id: string) => Promise<RunScheduleNowResponse>;
  };
}

/** Options for [createMethods]. */
export interface MethodsOptions {
  /**
   * What the server said it can do, or null before discovery — the capability
   * preflight reads it before each call. Omit it and every call goes out, leaving
   * the runtime to refuse what it cannot do.
   */
  capabilities?: () => ServerCapabilities | null | undefined;
}

export function createMethods(client: RpcClient, options: MethodsOptions = {}): Methods {
  const mutations = new MutationAttempts();

  // Every outbound call passes the preflight, because the alternative is a
  // round-trip whose only possible answer is the refusal we already hold.
  const refuse = <M extends WireMethodName>(method: M, params: WireParams<M>): void => {
    const missing = unnegotiated(method, params, options.capabilities?.());
    if (missing.length === 0) return;
    throw new RpcError({
      message: `${method} requires ${missing.join(", ")}`,
      // The type is the whole payload: a client's copy for this problem is its
      // own (§8.4), so manufacturing a detail here would put runtime words in it.
      data: { type: "capability_not_negotiated" },
    });
  };

  const call: WireCall = async (method, params, callOptions) => {
    refuse(method, params);
    return client.call(method, params, callOptions);
  };
  const mutate = async <M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    signal?: AbortSignal,
  ): Promise<WireResult<M>> => {
    refuse(method, params);
    return mutations.call(client, method, params, signal);
  };

  return {
    runtime: {
      discover: () => call("runtime.discover", {}),
    },
    sessions: {
      list: (query) => call("sessions.list", query ?? {}),
      get: (sessionId) => call("sessions.get", { sessionId }),
      create: (params, signal) => mutate("sessions.create", params ?? {}, signal),
      update: (params) => mutate("sessions.update", params),
      delete: (sessionId) => mutate("sessions.delete", { sessionId }),
      fork: (params) => mutate("sessions.fork", params),
      rollback: (params) => mutate("sessions.rollback", params),
      export: (sessionId, format) => call("sessions.export", { sessionId, format }),
      import: (artifact) =>
        mutate("sessions.import", {
          artifact,
        }),
    },
    runs: {
      start: async (params, signal) => {
        // Subscribe BEFORE the POST, then bind the tree to the runtime-assigned
        // root segmentId. Under streamable HTTP the response + its event frames
        // arrive on the same ordered stream, so the first events follow the
        // response immediately; binding only after `call` resolves could drop
        // the head (see streamRunEvents).
        const stream = streamRunEvents(client, signal);
        const result = await callOrDispose(stream, () => mutate("runs.start", params, signal));
        stream.bind(result.runId, result.segmentId);
        return { result, events: stream.events };
      },
      resume: async (params, signal) => {
        // A resume opens a NEW segment of the SAME run — bind the tree to it.
        const stream = streamRunEvents(client, signal);
        const result = await callOrDispose(stream, () => mutate("runs.resume", params, signal));
        stream.bind(result.runId, result.segmentId);
        return { result, events: stream.events };
      },
      subscribe: async (params, signal, options) => {
        // Reattach to the segment the caller named; the ack echoes it, and the tree
        // binds to it (same deferred-bind head-drop guard).
        const stream = streamRunEvents(client, signal);
        const result = await callOrDispose(stream, () =>
          call("runs.subscribe", params, { signal, lastEventId: options?.lastEventId }),
        );
        stream.bind(result.runId, result.segmentId);
        return { result, events: stream.events };
      },
      cancel: (runId, reason) => mutate("runs.cancel", { runId, reason }),
      steer: (runId, expectedSegmentId, message) =>
        mutate("runs.steer", { runId, expectedSegmentId, message }),
      get: (runId) => call("runs.get", { runId }),
      list: (query) => call("runs.list", query ?? {}),
    },
    todos: {
      get: (sessionId) => call("todos.get", { sessionId }),
    },
    runtimeEvents: {
      subscribe: async (params, signal) => {
        const stream = streamRuntimeEvents(client, signal);
        const result = await callOrDispose(stream, () =>
          call(RUNTIME_SUBSCRIBE_METHOD, params, { signal }),
        );
        return { result, events: stream.events };
      },
    },
    interrupts: {
      list: (query) => call("interrupts.list", query ?? {}),
    },
    items: {
      list: (params) => call("items.list", params),
    },
    workspace: {
      listFileChanges: (cwd) => call("workspace.listFileChanges", { cwd }),
      getDiff: (params) => call("workspace.getDiff", params ?? {}),
      getFileHead: (params) => call("workspace.getFileHead", params),
      grep: (params) => call("workspace.grep", params),
      listFiles: (params) => call("workspace.listFiles", params),
      readFile: (params) => call("workspace.readFile", params),
      listProjects: () => call("workspace.listProjects", {}),
    },
    recipes: {
      list: (cwd) => call("recipes.list", { cwd }),
    },
    hooks: {
      list: (cwd) => call("hooks.list", { cwd }),
      setTrust: (projectRoot, trusted) =>
        mutate("hooks.setTrust", {
          projectRoot,
          trusted,
        }),
    },
    skills: {
      listDiscovered: (cwd) => call("skills.discovered.list", { cwd }),
      listLibrary: () => call("skills.library.list", {}),
      archive: (name) => mutate("skills.library.archive", { name }),
      restore: (name) => mutate("skills.library.restore", { name }),
      listDrafts: () => call("skills.drafts.list", {}),
      promoteDraft: (ref) => mutate("skills.drafts.promote", ref),
      rejectDraft: (ref) => mutate("skills.drafts.reject", ref),
    },
    agentDocs: {
      list: (cwd) => call("agentDocs.list", { cwd }),
    },
    mcp: {
      listConfigs: (query) => call("mcp.configs.list", query ?? {}),
      configure: (params) => mutate("mcp.configs.configure", params),
      remove: (name) => mutate("mcp.configs.remove", { name }),
      setEnabled: (name, enabled) =>
        mutate("mcp.configs.setEnabled", {
          name,
          enabled,
        }),
      test: (params) => call("mcp.configs.test", params),
      listServers: () => call("mcp.servers.list", {}),
      listTools: (server) => call("mcp.tools.list", server ? { server } : {}),
      reconnect: (server) => mutate("mcp.servers.reconnect", { server }),
      authorize: (server) => mutate("mcp.servers.authorize", { server }),
    },
    providers: {
      list: () => call("providers.list", {}),
      configure: (params) => mutate("providers.configure", params),
      test: (provider) => call("providers.test", { provider }),
    },
    models: {
      list: (provider) => call("models.list", provider ? { provider } : {}),
      getUtilityRole: () => call("models.getUtilityRole", {}),
      setUtilityRole: (params) => mutate("models.setUtilityRole", params),
      getEmbeddingRole: () => call("models.getEmbeddingRole", {}),
      setEmbeddingRole: (params) => mutate("models.setEmbeddingRole", params),
    },
    codebase: {
      search: (params) => call("codebase.search", params),
      status: (cwd) => call("codebase.status", { cwd }),
      reindex: (cwd) => mutate("codebase.reindex", { cwd }),
    },
    tools: {
      list: () => call("tools.list", {}),
      invoke: (params) => mutate("tools.invoke", params),
    },
    usage: {
      session: (sessionId) => call("usage.session", { sessionId }),
      summary: (params) => call("usage.summary", params ?? {}),
    },
    memory: {
      list: (cwd) => call("memory.list", { cwd }),
      get: (scope, cwd) => call("memory.get", { scope, cwd }),
      update: (params) => mutate("memory.update", params),
    },
    agentMemory: {
      list: (params) => call("agentMemory.list", params ?? {}),
      review: (id, decision) =>
        mutate("agentMemory.review", {
          id,
          decision,
        }),
      update: (params) => mutate("agentMemory.update", params),
      delete: (id) => mutate("agentMemory.delete", { id }),
      add: (params) => mutate("agentMemory.add", params),
    },
    goals: {
      get: (sessionId) => call("goals.get", { sessionId }),
      start: (params) => mutate("goals.start", params),
      stop: (sessionId) => mutate("goals.stop", { sessionId }),
      resume: (sessionId) => mutate("goals.resume", { sessionId }),
    },
    feedback: {
      create: (params) => mutate("feedback.create", params),
    },
    approval: {
      getMode: () => call("approval.getMode", {}),
      setMode: (mode) => mutate("approval.setMode", { mode }),
      listRules: (sessionId) => call("approval.listRules", { sessionId }),
      forgetRule: (id) => mutate("approval.forgetRule", { id }),
    },
    schedules: {
      list: (query) => call("schedules.list", query ?? {}),
      create: (params) => mutate("schedules.create", params),
      update: (params) => mutate("schedules.update", params),
      delete: (id) => mutate("schedules.delete", { id }),
      runNow: (id) => mutate("schedules.runNow", { id }),
    },
  };
}
