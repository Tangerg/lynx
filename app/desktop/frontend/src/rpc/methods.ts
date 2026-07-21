// Typed wrappers for every method in docs/protocol/API.md §7. Grouped by namespace
// so callers do `methods.runs.start(...)` rather than
// `client.call("runs.start")`. The factory takes an RpcClient and returns
// the full typed surface.
//
// Streaming methods (runs.start / runs.resume / runs.subscribe) return
// `{ result, events }` where `events` is an AsyncIterable. Run streams
// carry the whole run tree and end on the root segment's `segment.finished`
// (see ./stream).

import type { RpcClient } from "./client";
import { isErrorType, RpcError } from "./errors";
import type { RunId, SegmentId, SessionId } from "./ids";
import type {
  AgentDoc,
  ApprovalMode,
  ApprovalRule,
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
  CodebaseHit,
  CodebaseStatus,
  EmbeddingRole,
  InvokeToolRequest,
  ListItemsResponse,
  McpServer,
  McpServerConfig,
  McpTestResult,
  McpTool,
  MemoryEntry,
  MemoryScope,
  Model,
  OpenInterrupt,
  Page,
  PageQuery,
  Project,
  Provider,
  ProviderTestResult,
  ResumeRunRequest,
  ResumeRunResponse,
  RollbackSessionRequest,
  RollbackSessionResponse,
  RunEvent,
  Recipe,
  RunRef,
  Schedule,
  ScheduleInput,
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
  StartRunResponse,
  SubscribeWorkspaceRequest,
  ToolSpec,
  UpdateSessionRequest,
  Usage,
  UsageSummary,
  UsageSummaryRequest,
  UtilityRole,
  WorkspaceEvent,
  WorkspaceFileChange,
} from "./shapes";
import { streamRunEvents, streamWorkspaceEvents } from "./stream";
import { WORKSPACE_SUBSCRIBE_METHOD } from "./transport";

export interface StreamingResult<R, E> {
  result: R;
  events: AsyncIterable<E>;
}

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
    ) => Promise<StreamingResult<ResumeRunResponse, RunEvent>>;
    subscribe: (
      runId: RunId,
      signal?: AbortSignal,
    ) => Promise<StreamingResult<{ runId: RunId; segmentId: SegmentId }, RunEvent>>;
    cancel: (runId: RunId, reason?: string) => Promise<void>;
    // Mid-run steering (§6): inject a user message into the running run so the
    // model reads it next tool round. run_not_found if no longer actively running.
    steer: (runId: RunId, message: string) => Promise<void>;
    // Running runs only (§7.3); finished/interrupted via listOpenInterrupts or items history.
    list: (query?: PageQuery & { sessionId?: SessionId }) => Promise<Page<RunRef>>;
    // Durable HITL discovery — resumable interrupted runs (§7.3 / §10.2).
    listOpenInterrupts: (
      query?: PageQuery & { sessionId?: SessionId },
    ) => Promise<Page<OpenInterrupt>>;
  };
  items: {
    list: (params: {
      sessionId: SessionId;
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
    // The app-wide workspace notification channel (AUX_API §3): lossy
    // "changed → refetch" events, connection-scoped, no replay. One stream
    // per app; resubscribe (= implicit resync) when it ends.
    subscribe: (
      params?: SubscribeWorkspaceRequest,
      signal?: AbortSignal,
    ) => Promise<StreamingResult<Record<string, never>, WorkspaceEvent>>;
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
    search: (params: { cwd?: string; query: string; limit?: number }) => Promise<{
      hits: CodebaseHit[];
    }>;
    status: (cwd?: string) => Promise<CodebaseStatus>;
    reindex: (cwd?: string) => Promise<{ operationId: string }>;
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
    getMode: () => Promise<{ mode: ApprovalMode }>;
    setMode: (mode: ApprovalMode) => Promise<{ mode: ApprovalMode }>;
    // Rules visible from the session: its session rules + its project's rules
    // + all global rules (the runtime resolves the session cwd).
    listRules: (sessionId: SessionId) => Promise<{ rules: ApprovalRule[] }>;
    // Remove one rule by id; clear-all = loop the visible ids.
    forgetRule: (id: string) => Promise<void>;
  };
  // Scheduled runs (§7.9): cron-triggered headless runs of a saved prompt,
  // fired by the runtime's scheduler worker while serving.
  schedules: {
    list: (query?: PageQuery) => Promise<Page<Schedule>>;
    create: (params: ScheduleInput) => Promise<Schedule>;
    update: (
      params: Partial<ScheduleInput> & { id: string; expectedRevision: number; enabled?: boolean },
    ) => Promise<Schedule>;
    delete: (id: string) => Promise<void>;
    runNow: (id: string) => Promise<{ sessionId: SessionId; runId: RunId }>;
  };
}

export function createMethods(client: RpcClient): Methods {
  const mutations = new MutationAttempts();
  const mutate = <R, P>(method: string, params: P, signal?: AbortSignal) =>
    mutations.call<R, P>(client, method, params, signal);

  return {
    runtime: {
      discover: () => client.call<DiscoverResponse>("runtime.discover", {}),
    },
    sessions: {
      list: (query) => client.call<Page<Session>>("sessions.list", query ?? {}),
      get: (sessionId) => client.call<Session>("sessions.get", { sessionId }),
      create: (params, signal) =>
        mutate<Session, CreateSessionRequest>("sessions.create", params ?? {}, signal),
      update: (params) => mutate<Session, UpdateSessionRequest>("sessions.update", params),
      delete: (sessionId) =>
        mutate<void, { sessionId: SessionId }>("sessions.delete", { sessionId }),
      fork: (params) => mutate<Session, ForkSessionRequest>("sessions.fork", params),
      rollback: (params) =>
        mutate<RollbackSessionResponse, RollbackSessionRequest>("sessions.rollback", params),
      export: (sessionId, format) =>
        client.call<ExportSessionResponse>("sessions.export", { sessionId, format }),
      import: (artifact) =>
        mutate<ImportSessionResponse, { artifact: SessionArtifact }>("sessions.import", {
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
        const result = await callOrDispose(stream, () =>
          mutate<StartRunResponse, StartRunRequest>("runs.start", params, signal),
        );
        stream.bind(result.runId, result.segmentId);
        return { result, events: stream.events };
      },
      resume: async (params, signal) => {
        // A resume opens a NEW segment of the SAME run — bind the tree to it.
        const stream = streamRunEvents(client, signal);
        const result = await callOrDispose(stream, () =>
          mutate<ResumeRunResponse, ResumeRunRequest>("runs.resume", params, signal),
        );
        stream.bind(result.runId, result.segmentId);
        return { result, events: stream.events };
      },
      subscribe: async (runId, signal) => {
        // Reattach to a running run: its response names the CURRENT segment;
        // bind the tree to that segmentId (same deferred-bind head-drop guard).
        const stream = streamRunEvents(client, signal);
        const result = await callOrDispose(stream, () =>
          client.call<{ runId: RunId; segmentId: SegmentId }>(
            "runs.subscribe",
            { runId },
            { signal },
          ),
        );
        stream.bind(result.runId, result.segmentId);
        return { result, events: stream.events };
      },
      cancel: (runId, reason) =>
        mutate<void, { runId: RunId; reason?: string }>("runs.cancel", { runId, reason }),
      steer: (runId, message) =>
        mutate<void, { runId: RunId; message: string }>("runs.steer", { runId, message }),
      list: (query) => client.call<Page<RunRef>>("runs.list", query ?? {}),
      listOpenInterrupts: (query) =>
        client.call<Page<OpenInterrupt>>("runs.listOpenInterrupts", query ?? {}),
    },
    items: {
      list: (params) => client.call<ListItemsResponse>("items.list", params),
    },
    workspace: {
      listFileChanges: (cwd) =>
        client.call<Page<WorkspaceFileChange>>("workspace.listFileChanges", { cwd }),
      getDiff: (params) => client.call<Diff>("workspace.getDiff", params ?? {}),
      getFileHead: (params) => client.call<FileHead>("workspace.getFileHead", params),
      grep: (params) => client.call<GrepResult>("workspace.grep", params),
      listFiles: (params) => client.call<Page<FileEntry>>("workspace.listFiles", params),
      readFile: (params) => client.call<FileContent>("workspace.readFile", params),
      listProjects: () => client.call<Page<Project>>("workspace.listProjects"),
      subscribe: async (params, signal) => {
        const stream = streamWorkspaceEvents(client, signal);
        const result = await callOrDispose(stream, () =>
          client.call<Record<string, never>>(WORKSPACE_SUBSCRIBE_METHOD, params ?? {}, { signal }),
        );
        return { result, events: stream.events };
      },
    },
    recipes: {
      list: (cwd) => client.call<Page<Recipe>>("recipes.list", { cwd }),
    },
    hooks: {
      list: (cwd) => client.call<HooksListResult>("hooks.list", { cwd }),
      setTrust: (projectRoot, trusted) =>
        mutate<void, { projectRoot: string; trusted: boolean }>("hooks.setTrust", {
          projectRoot,
          trusted,
        }),
    },
    skills: {
      listDiscovered: (cwd) => client.call<Page<Skill>>("skills.discovered.list", { cwd }),
      listLibrary: () => client.call<Page<ManagedSkill>>("skills.library.list"),
      archive: (name) => mutate<void, { name: string }>("skills.library.archive", { name }),
      restore: (name) => mutate<void, { name: string }>("skills.library.restore", { name }),
      listDrafts: () => client.call<Page<SkillDraft>>("skills.drafts.list"),
      promoteDraft: (ref) => mutate<void, SkillDraftRef>("skills.drafts.promote", ref),
      rejectDraft: (ref) => mutate<void, SkillDraftRef>("skills.drafts.reject", ref),
    },
    agentDocs: {
      list: (cwd) => client.call<Page<AgentDoc>>("agentDocs.list", { cwd }),
    },
    mcp: {
      listConfigs: (query) => client.call<Page<McpServerConfig>>("mcp.configs.list", query ?? {}),
      configure: (params) =>
        mutate<McpServerConfig, ConfigureMCPServerRequest>("mcp.configs.configure", params),
      remove: (name) => mutate<void, { name: string }>("mcp.configs.remove", { name }),
      setEnabled: (name, enabled) =>
        mutate<void, { name: string; enabled: boolean }>("mcp.configs.setEnabled", {
          name,
          enabled,
        }),
      test: (params) => client.call<McpTestResult>("mcp.configs.test", params),
      listServers: () => client.call<Page<McpServer>>("mcp.servers.list"),
      listTools: (server) => client.call<Page<McpTool>>("mcp.tools.list", server ? { server } : {}),
      reconnect: (server) => mutate<void, { server: string }>("mcp.servers.reconnect", { server }),
      authorize: (server) => mutate<void, { server: string }>("mcp.servers.authorize", { server }),
    },
    providers: {
      list: () => client.call<Page<Provider>>("providers.list"),
      configure: (params) =>
        mutate<Provider, ConfigureProviderRequest>("providers.configure", params),
      test: (provider) => client.call<ProviderTestResult>("providers.test", { provider }),
    },
    models: {
      list: (provider) => client.call<Page<Model>>("models.list", provider ? { provider } : {}),
      getUtilityRole: () => client.call<UtilityRole>("models.getUtilityRole"),
      setUtilityRole: (params) => mutate<UtilityRole, UtilityRole>("models.setUtilityRole", params),
      getEmbeddingRole: () => client.call<EmbeddingRole>("models.getEmbeddingRole"),
      setEmbeddingRole: (params) =>
        mutate<EmbeddingRole, EmbeddingRole>("models.setEmbeddingRole", params),
    },
    codebase: {
      search: (params) => client.call<{ hits: CodebaseHit[] }>("codebase.search", params),
      status: (cwd) => client.call<CodebaseStatus>("codebase.status", { cwd }),
      reindex: (cwd) =>
        mutate<{ operationId: string }, { cwd?: string }>("codebase.reindex", { cwd }),
    },
    tools: {
      list: () => client.call<Page<ToolSpec>>("tools.list"),
      invoke: (params) => mutate<unknown, InvokeToolRequest>("tools.invoke", params),
    },
    usage: {
      session: (sessionId) => client.call<Usage>("usage.session", { sessionId }),
      summary: (params) => client.call<UsageSummary>("usage.summary", params ?? {}),
    },
    memory: {
      list: (cwd) => client.call<Page<MemoryEntry>>("memory.list", { cwd }),
      get: (scope, cwd) => client.call<MemoryEntry>("memory.get", { scope, cwd }),
      update: (params) => mutate<void, typeof params>("memory.update", params),
    },
    agentMemory: {
      list: (params) => client.call<AgentMemoryList>("agentMemory.list", params ?? {}),
      review: (id, decision) =>
        mutate<void, { id: string; decision: "approve" | "reject" }>("agentMemory.review", {
          id,
          decision,
        }),
      update: (params) => mutate<AgentMemoryItem, typeof params>("agentMemory.update", params),
      delete: (id) => mutate<void, { id: string }>("agentMemory.delete", { id }),
      add: (params) => mutate<AgentMemoryItem, typeof params>("agentMemory.add", params),
    },
    goals: {
      get: (sessionId) => client.call<Goal | null>("goals.get", { sessionId }),
      start: (params) => mutate<Goal, typeof params>("goals.start", params),
      stop: (sessionId) => mutate<Goal, { sessionId: SessionId }>("goals.stop", { sessionId }),
      resume: (sessionId) => mutate<Goal, { sessionId: SessionId }>("goals.resume", { sessionId }),
    },
    feedback: {
      create: (params) => mutate<void, FeedbackRequest>("feedback.create", params),
    },
    approval: {
      getMode: () => client.call<{ mode: ApprovalMode }>("approval.getMode"),
      setMode: (mode) =>
        mutate<{ mode: ApprovalMode }, { mode: ApprovalMode }>("approval.setMode", { mode }),
      listRules: (sessionId) =>
        client.call<{ rules: ApprovalRule[] }>("approval.listRules", { sessionId }),
      forgetRule: (id) => mutate<void, { id: string }>("approval.forgetRule", { id }),
    },
    schedules: {
      list: (query) => client.call<Page<Schedule>>("schedules.list", query ?? {}),
      create: (params) => mutate<Schedule, ScheduleInput>("schedules.create", params),
      update: (params) => mutate<Schedule, typeof params>("schedules.update", params),
      delete: (id) => mutate<void, { id: string }>("schedules.delete", { id }),
      runNow: (id) =>
        mutate<{ sessionId: SessionId; runId: RunId }, { id: string }>("schedules.runNow", { id }),
    },
  };
}
