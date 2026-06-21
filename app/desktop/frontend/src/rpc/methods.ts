// Typed wrappers for every method in docs/protocol/API.md §7. Grouped by namespace
// so callers do `methods.runs.start(...)` rather than
// `client.call("runs.start")`. The factory takes an RpcClient and returns
// the full typed surface.
//
// Streaming methods (runs.start / runs.resume / runs.subscribe) return
// `{ result, events }` where `events` is an AsyncIterable. Run streams
// carry the whole run tree and end on the root run's `run.finished`
// (see ./stream).

import type { RpcClient } from "./client";
import type { RunId, SessionId } from "./ids";
import type {
  AgentDoc,
  ApprovalMode,
  ApprovalRule,
  CanceledNotification,
  CodeLocation,
  CodePosition,
  CodeQuery,
  CompactionResult,
  ConfigureMCPServerRequest,
  ConfigureProviderRequest,
  CreateSessionRequest,
  Diagnostic,
  Diff,
  DocumentSymbol,
  ExportSessionResponse,
  FeedbackRequest,
  FileContent,
  FileEntry,
  FileHead,
  ForkSessionRequest,
  GrepResult,
  HooksListResult,
  Hover,
  ImportSessionResponse,
  InitializeRequest,
  InitializeResponse,
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
  ShutdownRequest,
  Skill,
  StartRunRequest,
  StartRunResponse,
  SubscribeWorkspaceRequest,
  TodoItem,
  ToolSpec,
  UpdateSessionRequest,
  Usage,
  UsageSummary,
  UsageSummaryRequest,
  UtilityRole,
  WorkspaceEvent,
  WorkspaceFileChange,
  WorkspaceSymbol,
} from "./shapes";
import { streamRunEvents, streamRunEventsDeferred, streamWorkspaceEvents } from "./stream";
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

export interface Methods {
  runtime: {
    initialize: (params: InitializeRequest) => Promise<InitializeResponse>;
    shutdown: (params?: ShutdownRequest) => Promise<void>;
    ping: () => Promise<void>;
    // Cancel an in-flight JSON-RPC Request by envelope id (NOT runs.cancel).
    cancel: (params: CanceledNotification) => Promise<void>;
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
    // Proactive context compaction (B10) — force:false only compacts past the
    // internal threshold (same condition as autonomous). Rejected session_busy
    // while a run is in flight. Internally calls the LLM → may take seconds.
    compact: (params: { sessionId: SessionId; force?: boolean }) => Promise<CompactionResult>;
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
    ) => Promise<StreamingResult<{ runId: RunId }, RunEvent>>;
    cancel: (runId: RunId, reason?: string) => Promise<void>;
    // Mid-run steering (§6): inject a user message into the running run so the
    // model reads it next tool round. run_not_found if no longer actively running.
    steer: (runId: RunId, message: string) => Promise<void>;
    // Running runs only (§7.3); finished/interrupted via listOpenInterrupts or items history.
    list: (sessionId?: SessionId) => Promise<Page<RunRef>>;
    // Durable HITL discovery — resumable interrupted runs (§7.3 / §10.2).
    listOpenInterrupts: (sessionId?: SessionId) => Promise<Page<OpenInterrupt>>;
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
    // General directory listing / glob (B7/B8 → 613) — feeds the file tree + @file.
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
    listSkills: (cwd?: string) => Promise<Page<Skill>>;
    listAgentDocs: (cwd?: string) => Promise<Page<AgentDoc>>;
    // The app-wide workspace notification channel (AUX_API §3): lossy
    // "changed → refetch" events, connection-scoped, no replay. One stream
    // per app; resubscribe (= implicit resync) when it ends.
    subscribe: (
      params?: SubscribeWorkspaceRequest,
      signal?: AbortSignal,
    ) => Promise<StreamingResult<Record<string, never>, WorkspaceEvent>>;
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
    mcp: {
      // The editable registry (configure/remove/setEnabled) PLUS a best-effort
      // live status folded into each entry. listServers is the lighter
      // status-only view; listConfigs carries the full persisted config.
      listConfigs: (query?: PageQuery) => Promise<Page<McpServerConfig>>;
      // Upsert by name. authorization is the RAW token; omitted = keep the
      // stored one. Returns the entry with the token re-masked.
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
    // Code intelligence (B7) — LSP-backed, read-only, gated features.codeIntel. Positions
    // 0-based / UTF-16 (LSP). No language server for the file type → no_language_server
    // (non-fatal); indexing/unavailable → empty result (not an error).
    code: {
      definition: (params: CodeQuery & CodePosition) => Promise<{ locations: CodeLocation[] }>;
      references: (
        params: CodeQuery & CodePosition & { includeDeclaration?: boolean },
      ) => Promise<Page<CodeLocation>>;
      hover: (params: CodeQuery & CodePosition) => Promise<Hover>;
      documentSymbols: (params: CodeQuery) => Promise<{ symbols: DocumentSymbol[] }>;
      workspaceSymbols: (params: {
        cwd?: string;
        query: string;
        limit?: number;
      }) => Promise<Page<WorkspaceSymbol>>;
      diagnostics: (params: CodeQuery) => Promise<{ diagnostics: Diagnostic[] }>;
    };
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
    reindex: (cwd?: string) => Promise<void>;
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
    list: () => Promise<{ schedules: Schedule[] }>;
    create: (params: ScheduleInput) => Promise<Schedule>;
    update: (params: ScheduleInput & { id: string; enabled: boolean }) => Promise<Schedule>;
    delete: (id: string) => Promise<void>;
    runNow: (id: string) => Promise<void>;
  };
  // The model's working checklist (B11). Live updates ride state.snapshot (§5.3);
  // this is the cold read for inactive runs / reopened history.
  todos: {
    list: (sessionId: SessionId) => Promise<{ todos: TodoItem[] }>;
  };
}

export function createMethods(client: RpcClient): Methods {
  return {
    runtime: {
      initialize: (params) => client.call<InitializeResponse>("runtime.initialize", params),
      shutdown: (params) => client.notify("runtime.shutdown", params ?? {}),
      ping: () => client.call<void>("runtime.ping"),
      cancel: (params) => client.notify("notifications.canceled", params),
    },
    sessions: {
      list: (query) => client.call<Page<Session>>("sessions.list", query ?? {}),
      get: (sessionId) => client.call<Session>("sessions.get", { sessionId }),
      create: (params, signal) => client.call<Session>("sessions.create", params ?? {}, signal),
      update: (params) => client.call<Session>("sessions.update", params),
      delete: (sessionId) => client.call<void>("sessions.delete", { sessionId }),
      fork: (params) => client.call<Session>("sessions.fork", params),
      rollback: (params) => client.call<RollbackSessionResponse>("sessions.rollback", params),
      export: (sessionId, format) =>
        client.call<ExportSessionResponse>("sessions.export", { sessionId, format }),
      import: (artifact) => client.call<ImportSessionResponse>("sessions.import", { artifact }),
      compact: (params) => client.call<CompactionResult>("sessions.compact", params),
    },
    runs: {
      start: async (params, signal) => {
        // Subscribe BEFORE the POST, then bind to the runtime-assigned runId.
        // Under streamable HTTP the response + its event frames arrive on the
        // same ordered stream, so the first events follow the response
        // immediately; binding only after `call` resolves could drop the head
        // (see streamRunEventsDeferred).
        const stream = streamRunEventsDeferred(client, signal);
        const result = await callOrDispose(stream, () =>
          client.call<StartRunResponse>("runs.start", params, signal),
        );
        stream.bind(result.runId);
        return { result, events: stream.events };
      },
      resume: async (params, signal) => {
        const stream = streamRunEventsDeferred(client, signal);
        const result = await callOrDispose(stream, () =>
          client.call<ResumeRunResponse>("runs.resume", params, signal),
        );
        stream.bind(result.runId);
        return { result, events: stream.events };
      },
      subscribe: async (runId, signal) => {
        const stream = streamRunEvents(client, runId, signal);
        const result = await callOrDispose(stream, () =>
          client.call<{ runId: RunId }>("runs.subscribe", { runId }, signal),
        );
        return { result, events: stream.events };
      },
      cancel: (runId, reason) => client.call<void>("runs.cancel", { runId, reason }),
      steer: (runId, message) => client.call<void>("runs.steer", { runId, message }),
      list: (sessionId) => client.call<Page<RunRef>>("runs.list", sessionId ? { sessionId } : {}),
      listOpenInterrupts: (sessionId) =>
        client.call<Page<OpenInterrupt>>("runs.listOpenInterrupts", sessionId ? { sessionId } : {}),
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
      listSkills: (cwd) => client.call<Page<Skill>>("workspace.listSkills", { cwd }),
      listAgentDocs: (cwd) => client.call<Page<AgentDoc>>("workspace.listAgentDocs", { cwd }),
      subscribe: async (params, signal) => {
        const stream = streamWorkspaceEvents(client, signal);
        const result = await callOrDispose(stream, () =>
          client.call<Record<string, never>>(WORKSPACE_SUBSCRIBE_METHOD, params ?? {}, signal),
        );
        return { result, events: stream.events };
      },
      recipes: {
        list: (cwd) => client.call<Page<Recipe>>("workspace.recipes.list", { cwd }),
      },
      hooks: {
        list: (cwd) => client.call<HooksListResult>("workspace.hooks.list", { cwd }),
        setTrust: (projectRoot, trusted) =>
          client.call<void>("workspace.hooks.setTrust", { projectRoot, trusted }),
      },
      mcp: {
        listConfigs: (query) =>
          client.call<Page<McpServerConfig>>("workspace.mcp.listConfigs", query ?? {}),
        configure: (params) => client.call<McpServerConfig>("workspace.mcp.configure", params),
        remove: (name) => client.call<void>("workspace.mcp.remove", { name }),
        setEnabled: (name, enabled) =>
          client.call<void>("workspace.mcp.setEnabled", { name, enabled }),
        test: (params) => client.call<McpTestResult>("workspace.mcp.test", params),
        listServers: () => client.call<Page<McpServer>>("workspace.mcp.listServers"),
        listTools: (server) =>
          client.call<Page<McpTool>>("workspace.mcp.listTools", server ? { server } : {}),
        reconnect: (server) => client.call<void>("workspace.mcp.reconnect", { server }),
        authorize: (server) => client.call<void>("workspace.mcp.authorize", { server }),
      },
      code: {
        definition: (params) =>
          client.call<{ locations: CodeLocation[] }>("workspace.code.definition", params),
        references: (params) =>
          client.call<Page<CodeLocation>>("workspace.code.references", params),
        hover: (params) => client.call<Hover>("workspace.code.hover", params),
        documentSymbols: (params) =>
          client.call<{ symbols: DocumentSymbol[] }>("workspace.code.documentSymbols", params),
        workspaceSymbols: (params) =>
          client.call<Page<WorkspaceSymbol>>("workspace.code.workspaceSymbols", params),
        diagnostics: (params) =>
          client.call<{ diagnostics: Diagnostic[] }>("workspace.code.diagnostics", params),
      },
    },
    providers: {
      list: () => client.call<Page<Provider>>("providers.list"),
      configure: (params) => client.call<Provider>("providers.configure", params),
      test: (provider) => client.call<ProviderTestResult>("providers.test", { provider }),
    },
    models: {
      list: (provider) => client.call<Page<Model>>("models.list", provider ? { provider } : {}),
      getUtilityRole: () => client.call<UtilityRole>("models.getUtilityRole"),
      setUtilityRole: (params) => client.call<UtilityRole>("models.setUtilityRole", params),
      getEmbeddingRole: () => client.call<EmbeddingRole>("models.getEmbeddingRole"),
      setEmbeddingRole: (params) => client.call<EmbeddingRole>("models.setEmbeddingRole", params),
    },
    codebase: {
      search: (params) => client.call<{ hits: CodebaseHit[] }>("codebase.search", params),
      status: (cwd) => client.call<CodebaseStatus>("codebase.status", { cwd }),
      reindex: (cwd) => client.call<void>("codebase.reindex", { cwd }),
    },
    tools: {
      list: () => client.call<Page<ToolSpec>>("tools.list"),
      invoke: (params) => client.call<unknown>("tools.invoke", params),
    },
    usage: {
      session: (sessionId) => client.call<Usage>("usage.session", { sessionId }),
      summary: (params) => client.call<UsageSummary>("usage.summary", params ?? {}),
    },
    memory: {
      list: (cwd) => client.call<Page<MemoryEntry>>("memory.list", { cwd }),
      get: (scope, cwd) => client.call<MemoryEntry>("memory.get", { scope, cwd }),
      update: (params) => client.call<void>("memory.update", params),
    },
    feedback: {
      create: (params) => client.call<void>("feedback.create", params),
    },
    approval: {
      getMode: () => client.call<{ mode: ApprovalMode }>("approval.getMode"),
      setMode: (mode) => client.call<{ mode: ApprovalMode }>("approval.setMode", { mode }),
      listRules: (sessionId) =>
        client.call<{ rules: ApprovalRule[] }>("approval.listRules", { sessionId }),
      forgetRule: (id) => client.call<void>("approval.forgetRule", { id }),
    },
    schedules: {
      list: () => client.call<{ schedules: Schedule[] }>("schedules.list"),
      create: (params) => client.call<Schedule>("schedules.create", params),
      update: (params) => client.call<Schedule>("schedules.update", params),
      delete: (id) => client.call<void>("schedules.delete", { id }),
      runNow: (id) => client.call<void>("schedules.runNow", { id }),
    },
    todos: {
      list: (sessionId) => client.call<{ todos: TodoItem[] }>("todos.list", { sessionId }),
    },
  };
}
