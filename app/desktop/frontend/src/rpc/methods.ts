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
import { RpcError } from "./errors";
import { createMutationPromise, type MutationPromise } from "./mutation";
import type { MutationJournal } from "./mutationJournal";
import { unnegotiated } from "./preflight";
import type { RunId, SegmentId, SessionId } from "./ids";
import type {
  AgentDoc,
  ApprovalMode,
  ApprovalModeResult,
  CancelRunResponse,
  ContentBlock,
  MCPServerCandidate,
  UpdateProviderRequest,
  CreateSessionRequest,
  Diff,
  ExportSessionResponse,
  FeedbackRequest,
  FileContent,
  FileEntry,
  FileHead,
  ForkSessionRequest,
  GetDiffRequest,
  GetFileHeadRequest,
  GrepRequest,
  GrepResult,
  HooksListResult,
  ImportSessionResponse,
  DiscoverResponse,
  CodebaseReindexResponse,
  CodebaseSearchRequest,
  CodebaseSearchResult,
  CodebaseStatus,
  EmbeddingRole,
  InvokeToolRequest,
  ListApprovalRulesResult,
  ListFilesRequest,
  ListItemsResponse,
  MCPAuthorizationAttempt,
  MCPServer,
  MCPTestResult,
  MCPTool,
  KnowledgeEntry,
  KnowledgeScope,
  Model,
  PendingInterruptSet,
  Page,
  PageQuery,
  Provider,
  ProviderTestResult,
  ResumeRunRequest,
  ResumeRunResponse,
  StartRunResponse,
  SubscribeRunRequest,
  SubscribeRunResponse,
  RollbackSessionRequest,
  RollbackSessionResponse,
  RunEvent,
  ReadFileRequest,
  Recipe,
  ItemListScope,
  ItemOrder,
  RunRef,
  RunStatus,
  RunScheduleNowResponse,
  Schedule,
  CreateScheduleRequest,
  UpdateScheduleRequest,
  ServerCapabilities,
  StateSnapshot,
  Session,
  SessionArtifact,
  Skill,
  ManagedSkill,
  SkillProposal,
  SkillProposalRef,
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
  UpdateMCPServerRequest,
  Usage,
  UsageSummary,
  UsageSummaryRequest,
  UtilityRole,
  RuntimeEvent,
  RequestMeta,
  WorkspaceFileChange,
  WorkspaceInfo,
  WorkspaceRef,
  WorkspaceSummary,
} from "./wire.generated";
import { streamRunEvents, streamRuntimeEvents } from "./stream";
import { createAutoPagingPromise, type AutoPagingPromise, type CursorPage } from "./pagination";
import {
  wireMethodIsPaginated,
  wireMethodRequiresIdempotency,
  type WireMethodName,
  type WireMutationMethodName,
  type WirePaginatedMethodName,
  type WireParams,
  type WireResult,
} from "./wire.methods.generated";
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
type WirePerform = <M extends WireMethodName>(
  method: M,
  params: WireParams<M>,
  options?: RpcCallOptions,
) => Promise<WireResult<M>>;

type WireInvokeResult<M extends WireMethodName> = M extends WireMutationMethodName
  ? MutationPromise<WireResult<M>>
  : Promise<WireResult<M>>;

type WireInvoke = <M extends WireMethodName>(
  method: M,
  params: WireParams<M>,
  options?: RpcCallOptions,
) => WireInvokeResult<M>;

type PaginatedWireCall<M extends WirePaginatedMethodName> =
  WireResult<M> extends CursorPage ? AutoPagingPromise<WireResult<M>> : never;

type WireCallResult<M extends WireMethodName> = M extends WirePaginatedMethodName
  ? PaginatedWireCall<M>
  : M extends WireMutationMethodName
    ? MutationPromise<WireResult<M>>
    : Promise<WireResult<M>>;

type WireCall = <M extends WireMethodName>(
  method: M,
  params: WireParams<M>,
  options?: RpcCallOptions,
) => WireCallResult<M>;

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

export interface WorkspaceMethods {
  /** The immutable resource identity bound to every operation below. */
  readonly ref: Readonly<WorkspaceRef>;
  changes: {
    list: () => Promise<Page<WorkspaceFileChange>>;
  };
  diff: {
    get: (params?: Omit<GetDiffRequest, "workspace">) => Promise<Diff>;
  };
  files: {
    head: (params: Omit<GetFileHeadRequest, "workspace">) => Promise<FileHead>;
    search: (params: Omit<GrepRequest, "workspace">) => Promise<GrepResult>;
    list: (params?: Omit<ListFilesRequest, "workspace">) => AutoPagingPromise<Page<FileEntry>>;
    read: (params: Omit<ReadFileRequest, "workspace">) => Promise<FileContent>;
  };
  recipes: {
    list: () => Promise<Page<Recipe>>;
  };
  hooks: {
    list: () => Promise<HooksListResult>;
  };
  // Discovery and the proposal review queue are both workspace-scoped, so both
  // live on the bound client. `SkillProposalRef` carries a workspace of its own:
  // taking it from the binding rather than the caller is what stops a decision
  // from naming one workspace while the list it was read from named another.
  skills: {
    listDiscovered: () => Promise<Page<Skill>>;
    listProposals: () => Promise<Page<SkillProposal>>;
    approveProposal: (ref: Omit<SkillProposalRef, "workspace">) => MutationPromise<void>;
    rejectProposal: (ref: Omit<SkillProposalRef, "workspace">) => MutationPromise<void>;
  };
  agentDocs: {
    list: () => Promise<Page<AgentDoc>>;
  };
  codebase: {
    search: (params: Omit<CodebaseSearchRequest, "workspace">) => Promise<CodebaseSearchResult>;
    status: () => Promise<CodebaseStatus>;
    reindex: () => MutationPromise<CodebaseReindexResponse>;
  };
  knowledge: {
    list: () => Promise<Page<KnowledgeEntry>>;
    get: (scope: KnowledgeScope) => Promise<KnowledgeEntry>;
    update: (params: {
      scope: KnowledgeScope;
      content: string;
      expectedRevision: string;
    }) => MutationPromise<KnowledgeEntry>;
  };
  agentMemory: {
    list: () => Promise<AgentMemoryList>;
    add: (content: string) => MutationPromise<AgentMemoryItem>;
  };
}

export type AgentMemoryTarget =
  | { scope: Extract<AgentMemoryScope, "user"> }
  | { scope: Extract<AgentMemoryScope, "project">; workspace: WorkspaceRef };

export interface Methods {
  runtime: {
    discover: (signal?: AbortSignal) => Promise<DiscoverResponse>;
  };
  sessions: {
    list: (query?: PageQuery) => AutoPagingPromise<Page<Session>>;
    get: (sessionId: SessionId, signal?: AbortSignal) => Promise<Session>;
    create: (params?: CreateSessionRequest, signal?: AbortSignal) => MutationPromise<Session>;
    update: (params: UpdateSessionRequest) => MutationPromise<Session>;
    delete: (sessionId: SessionId) => MutationPromise<void>;
    fork: (params: ForkSessionRequest) => MutationPromise<Session>;
    // Turn-granular history truncation (AUX_API §4.1). Rejected with
    // session_busy while a run is in flight. restoreType files|both also
    // restores the working tree (gated features.checkpoints).
    rollback: (params: RollbackSessionRequest) => MutationPromise<RollbackSessionResponse>;
    export: (sessionId: SessionId, format?: "md" | "json") => Promise<ExportSessionResponse>;
    // Restore semantics — rebuilds under the artifact's original id (idempotent).
    import: (artifact: SessionArtifact) => MutationPromise<ImportSessionResponse>;
  };
  runs: {
    start: (
      params: StartRunRequest,
      signal?: AbortSignal,
    ) => MutationPromise<StreamingResult<StartRunResponse, RunEvent>>;
    resume: (
      params: ResumeRunRequest,
      signal?: AbortSignal,
    ) => MutationPromise<StreamingResult<ResumeRunResponse, RunEvent>>;
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
    cancel: (runId: RunId, reason?: string) => MutationPromise<CancelRunResponse>;
    // Mid-run steering (§6): inject structured user content into the segment the caller
    // believes is executing, so the model reads it next tool round. The segment is
    // named for the same reason: a run that parked and resumed between typing and
    // sending must refuse (stale_segment) rather than deliver the instruction to
    // work the person never saw.
    steer: (
      runId: RunId,
      expectedSegmentId: SegmentId,
      input: ContentBlock[],
    ) => MutationPromise<void>;
    // One run by id — current or terminal — without knowing its session (§7.3).
    get: (runId: RunId, signal?: AbortSignal) => Promise<RunRef>;
    // The durable run history, newest first (§7.3). Omitting statuses returns every
    // position; asking for descendants requires negotiated features.subagents.
    list: (
      query?: PageQuery & {
        sessionId?: SessionId;
        statuses?: RunStatus[];
        includeDescendants?: boolean;
      },
      signal?: AbortSignal,
    ) => AutoPagingPromise<Page<RunRef>>;
  };
  plan: {
    // The cold read the plan state key declares (§5.6). A session with no plan yet
    // answers with the empty state at revision 0 — the same shape the stream pushes.
    get: (sessionId: SessionId, signal?: AbortSignal) => Promise<StateSnapshot>;
  };
  interrupts: {
    // Durable HITL discovery — the waiting sets, longest wait first (§7.3 / §10.2).
    // A page never splits a set: a set is what runs.resume answers in one call.
    list: (
      query?: PageQuery & { sessionId?: SessionId; rootRunId?: RunId },
      signal?: AbortSignal,
    ) => AutoPagingPromise<Page<PendingInterruptSet>>;
  };
  items: {
    // The scope is required and closed (§7.4): a whole session timeline, or one
    // run's own items. `order` defaults to "asc" — the order the runtime produced,
    // which is the one a fold can replay.
    list: (
      params: {
        scope: ItemListScope;
        order?: ItemOrder;
        cursor?: string;
        limit?: number;
      },
      signal?: AbortSignal,
    ) => AutoPagingPromise<ListItemsResponse>;
  };
  workspaces: {
    resolve: (ref?: WorkspaceRef, signal?: AbortSignal) => Promise<WorkspaceInfo>;
    list: () => Promise<Page<WorkspaceSummary>>;
    /** Resolve the runtime default when omitted, then bind a scoped client. */
    open: (ref?: WorkspaceRef) => Promise<WorkspaceMethods>;
  };
  /** Bind one workspace once; every resource operation inherits its identity. */
  workspace: (ref: WorkspaceRef) => WorkspaceMethods;
  // The app-wide change-signal channel (§7): lossy "this moved → read it again"
  // events, connection-scoped, no replay. One stream per app; resubscribing IS the
  // resync. `topics` is required — a subscription says what it can fold.
  runtimeEvents: {
    subscribe: (
      params: RuntimeSubscribeRequest,
      signal?: AbortSignal,
    ) => Promise<StreamingResult<RuntimeSubscribeResponse, RuntimeEvent>>;
  };
  // Trust changes target the canonical root returned by
  // workspace(ref).hooks.list(); discovery itself remains workspace-scoped.
  hooks: {
    setTrust: (projectRoot: string, trusted: boolean) => MutationPromise<void>;
  };
  // Self-authored skill management (§7.7). The library surface adds archived
  // skills and archive/restore (never deleting); it is workspace-independent
  // because a managed skill is addressed by name alone. The workspace-scoped
  // half — discovery and the proposal review queue — lives on the bound client.
  skills: {
    listLibrary: () => Promise<Page<ManagedSkill>>;
    archive: (name: string) => MutationPromise<void>;
    restore: (name: string) => MutationPromise<void>;
  };
  mcp: {
    // One resource carries durable configuration and live state. Create and
    // update are distinct; update uses exact omission=preserve semantics.
    list: () => Promise<Page<MCPServer>>;
    create: (params: MCPServerCandidate) => MutationPromise<MCPServer>;
    update: (params: UpdateMCPServerRequest) => MutationPromise<MCPServer>;
    delete: (server: string) => MutationPromise<void>;
    // Dry-run connection probe (NOT persisted). A failed probe is
    // `{ ok:false, error }`, never an RPC error (mirrors providers.test).
    test: (params: MCPServerCandidate) => Promise<MCPTestResult>;
    listTools: (server?: string) => Promise<Page<MCPTool>>;
    reconnect: (server: string) => MutationPromise<void>;
    authorizationAttempts: {
      // Interactive OAuth is an asynchronous resource, not a command ACK. Create
      // opens the browser; get observes its terminal outcome after reconnects.
      create: (server: string, signal?: AbortSignal) => MutationPromise<MCPAuthorizationAttempt>;
      get: (attemptId: string, signal?: AbortSignal) => Promise<MCPAuthorizationAttempt>;
    };
  };
  providers: {
    list: () => Promise<Page<Provider>>;
    update: (params: UpdateProviderRequest) => MutationPromise<Provider>;
    test: (provider: string) => Promise<ProviderTestResult>;
  };
  models: {
    list: (provider?: string) => Promise<Page<Model>>;
    // The (provider, model) the in-house maintenance work (compaction /
    // extraction / titling) runs on. Empty model = unset → it runs on the main
    // turn model. setUtilityRole validates by resolving the client server-side.
    getUtilityRole: () => Promise<UtilityRole>;
    setUtilityRole: (params: UtilityRole) => MutationPromise<UtilityRole>;
    // The (embedding-capable provider, model) the @codebase index embeds with.
    // Empty model = unset → the feature is off. setEmbeddingRole validates by
    // building the embedding client server-side.
    getEmbeddingRole: () => Promise<EmbeddingRole>;
    setEmbeddingRole: (params: EmbeddingRole) => MutationPromise<EmbeddingRole>;
  };
  tools: {
    list: () => Promise<Page<ToolSpec>>;
    invoke: (params: InvokeToolRequest) => MutationPromise<unknown>;
  };
  // Read-only spend reporting aggregated from the durable run history (§7.7).
  usage: {
    session: (sessionId: SessionId) => Promise<Usage>;
    summary: (params?: UsageSummaryRequest) => Promise<UsageSummary>;
  };
  // agentMemory.* (§7.7, capability-gated): the HITL review surface over the
  // agent's self-maintained memory — list active + pending items (pending
  // first), approve/reject a proposal, edit content / pin an item, delete one,
  // or add a user-authored active item. Distinct from `memory` (the LYRA.md
  // cascade). capability_not_negotiated when the store is not wired.
  agentMemory: {
    list: (target: AgentMemoryTarget) => Promise<AgentMemoryList>;
    review: (id: string, decision: "approve" | "reject") => MutationPromise<void>;
    update: (params: {
      id: string;
      content?: string;
      pinned?: boolean;
    }) => MutationPromise<AgentMemoryItem>;
    delete: (id: string) => MutationPromise<void>;
    add: (params: AgentMemoryTarget & { content: string }) => MutationPromise<AgentMemoryItem>;
  };
  // goals.* (§7.14, capability-gated): Goal mode — the autonomous execution
  // loop. get returns the session's goal or null (no goal); start opens one
  // (session_busy if one is already actively driving); stop pauses the loop;
  // resume re-activates a paused/blocked goal. Omitting provider/model runs the
  // loop on the runtime default.
  goals: {
    get: (sessionId: SessionId) => Promise<Goal | null>;
    start: (
      params: {
        sessionId: SessionId;
        objective: string;
        provider?: string;
        model?: string;
        budget?: GoalBudget;
      },
      signal?: AbortSignal,
    ) => MutationPromise<Goal>;
    stop: (sessionId: SessionId, signal?: AbortSignal) => MutationPromise<Goal>;
    resume: (sessionId: SessionId, signal?: AbortSignal) => MutationPromise<Goal>;
  };
  feedback: {
    create: (params: FeedbackRequest) => MutationPromise<void>;
  };
  // Approval runtime control (B9) — global stance + remember management. Not gated.
  approval: {
    getMode: () => Promise<ApprovalModeResult>;
    setMode: (mode: ApprovalMode) => MutationPromise<ApprovalModeResult>;
    // Rules visible from the session: its session rules + its project's rules
    // + all global rules (the runtime resolves the session cwd).
    listRules: (sessionId: SessionId) => Promise<ListApprovalRulesResult>;
    // Remove one rule by id; clear-all = loop the visible ids.
    forgetRule: (id: string) => MutationPromise<void>;
  };
  // Scheduled runs (§7.9): cron-triggered headless runs of a saved prompt,
  // fired by the runtime's scheduler worker while serving.
  schedules: {
    list: (query?: PageQuery) => AutoPagingPromise<Page<Schedule>>;
    create: (params: CreateScheduleRequest) => MutationPromise<Schedule>;
    update: (params: UpdateScheduleRequest) => MutationPromise<Schedule>;
    delete: (id: string) => MutationPromise<void>;
    runNow: (id: string) => MutationPromise<RunScheduleNowResponse>;
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
  /**
   * Metadata attached to the next request. The factory reads it once per call,
   * using the same snapshot for capability preflight and emission.
   */
  requestMeta?: () => RequestMeta | undefined;
  /** Optional durable owner for unresolved command identities. The RPC SDK
   * remains storage-agnostic; Desktop supplies the adapter at composition. */
  mutationJournal?: MutationJournal;
}

function bindWorkspace(call: WireCall, ref: WorkspaceRef): WorkspaceMethods {
  // Copy and freeze the identity so a caller cannot silently retarget an
  // already-created resource client by mutating the original object.
  const workspace = Object.freeze({ path: ref.path });

  return {
    ref: workspace,
    changes: {
      list: () => call("workspace.changes.list", { workspace }),
    },
    diff: {
      get: (params) => call("workspace.diff.get", { ...params, workspace }),
    },
    files: {
      head: (params) => call("workspace.files.head", { ...params, workspace }),
      search: (params) => call("workspace.files.search", { ...params, workspace }),
      list: (params) => call("workspace.files.list", { ...params, workspace }),
      read: (params) => call("workspace.files.read", { ...params, workspace }),
    },
    recipes: {
      list: () => call("recipes.list", { workspace }),
    },
    hooks: {
      list: () => call("hooks.list", { workspace }),
    },
    skills: {
      listDiscovered: () => call("skills.discovered.list", { workspace }),
      listProposals: () => call("skills.proposals.list", { workspace }),
      approveProposal: (ref) => call("skills.proposals.approve", { ...ref, workspace }),
      rejectProposal: (ref) => call("skills.proposals.reject", { ...ref, workspace }),
    },
    agentDocs: {
      list: () => call("agentDocs.list", { workspace }),
    },
    codebase: {
      search: (params) => call("codebase.search", { ...params, workspace }),
      status: () => call("codebase.status", { workspace }),
      reindex: () => call("codebase.reindex", { workspace }),
    },
    knowledge: {
      list: () => call("knowledge.list", { workspace }),
      get: (scope) => call("knowledge.get", { scope, workspace }),
      update: (params) => call("knowledge.update", { ...params, workspace }),
    },
    agentMemory: {
      list: () => call("agentMemory.list", { scope: "project", workspace }),
      add: (content) => call("agentMemory.add", { scope: "project", workspace, content }),
    },
  };
}

export function createMethods(client: RpcClient, options: MethodsOptions = {}): Methods {
  // Every outbound call passes the preflight, because the alternative is a
  // round-trip whose only possible answer is the refusal we already hold.
  const refuse = <M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    requestMeta?: RequestMeta | null,
  ): void => {
    const missing = unnegotiated(
      method,
      params,
      options.capabilities?.(),
      requestMeta?.clientCapabilities,
    );
    if (missing.length === 0) return;
    throw new RpcError({
      message: `${method} requires ${missing.join(", ")}`,
      // This is the same typed refusal the runtime would return, with every gap in
      // one frame. Manufacturing a detail here would put runtime words in a local
      // refusal, so the UI still owns the prose.
      data: {
        type: "capability_not_negotiated",
        requiredCapabilities: missing.map((name) => ({ type: "feature", name })),
      },
    });
  };

  const perform: WirePerform = async (method, params, callOptions) => {
    const ownsRequestMeta = options.requestMeta !== undefined;
    const requestMeta = ownsRequestMeta ? options.requestMeta?.() : callOptions?.requestMeta;
    refuse(method, params, requestMeta);
    const effectiveOptions = ownsRequestMeta
      ? { ...callOptions, requestMeta: requestMeta ?? null }
      : callOptions;
    return client.call(method, params, effectiveOptions);
  };

  const openMutation = <M extends WireMethodName, Result>(
    method: M,
    params: WireParams<M>,
    execute: (
      idempotencyKey: string,
      attempt: { signal?: AbortSignal; idempotencyNamespace?: string },
    ) => Promise<Result>,
    signal?: AbortSignal,
    requestedKey?: string,
    journalKey?: string,
  ): MutationPromise<Result> => {
    const preferredJournalKey = journalKey ?? crypto.randomUUID();
    let reservation: ReturnType<MutationJournal["reserve"]>;
    try {
      reservation =
        requestedKey !== undefined
          ? undefined
          : options.mutationJournal?.reserve(method, params, preferredJournalKey);
    } catch (error) {
      const failedKey = requestedKey ?? preferredJournalKey;
      const retry = (retryOptions?: { signal?: AbortSignal }): MutationPromise<Result> =>
        openMutation(
          method,
          params,
          execute,
          retryOptions === undefined ? signal : retryOptions.signal,
          requestedKey,
          preferredJournalKey,
        );
      return Object.defineProperties(Promise.reject(error), {
        idempotencyKey: { enumerable: true, value: failedKey },
        retry: { enumerable: true, value: retry },
      }) as MutationPromise<Result>;
    }
    const mutation = createMutationPromise(
      (idempotencyKey, attempt) => {
        const idempotencyNamespace = reservation?.authorizeAttempt();
        return execute(idempotencyKey, { ...attempt, idempotencyNamespace });
      },
      requestedKey ?? reservation?.idempotencyKey,
      { signal },
    );
    return reservation?.track(mutation) ?? mutation;
  };

  const invoke = (<M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    callOptions?: RpcCallOptions,
  ): WireInvokeResult<M> => {
    if (!wireMethodRequiresIdempotency(method)) {
      return perform(method, params, callOptions) as WireInvokeResult<M>;
    }
    const { signal, idempotencyKey, ...stableCallOptions } = callOptions ?? {};
    return openMutation(
      method,
      params,
      (idempotencyKey, attempt) =>
        perform(method, params, {
          ...stableCallOptions,
          ...(attempt.signal ? { signal: attempt.signal } : {}),
          idempotencyKey,
          ...(attempt.idempotencyNamespace
            ? { idempotencyNamespace: attempt.idempotencyNamespace }
            : {}),
        }),
      signal,
      idempotencyKey,
    ) as WireInvokeResult<M>;
  }) as WireInvoke;

  const call = (<M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    callOptions?: RpcCallOptions,
  ): WireCallResult<M> => {
    if (wireMethodIsPaginated(method)) {
      const initialCursor = (params as { cursor?: string }).cursor;
      return createAutoPagingPromise<CursorPage>((cursor) => {
        const continuation = { ...params, cursor } as WireParams<M> & { cursor?: string };
        if (cursor === undefined) delete continuation.cursor;
        return invoke<M>(method, continuation, callOptions) as unknown as Promise<CursorPage>;
      }, initialCursor) as unknown as WireCallResult<M>;
    }
    return invoke(method, params, callOptions) as WireCallResult<M>;
  }) as WireCall;

  let defaultWorkspaceRef: Promise<WorkspaceRef> | undefined;
  const openWorkspace = async (ref?: WorkspaceRef): Promise<WorkspaceMethods> => {
    if (ref) return bindWorkspace(call, ref);
    if (!defaultWorkspaceRef) {
      const pending = call("workspaces.resolve", {}).then((resolved) => resolved.ref);
      defaultWorkspaceRef = pending;
      void pending.catch(() => {
        if (defaultWorkspaceRef === pending) defaultWorkspaceRef = undefined;
      });
    }
    return bindWorkspace(call, await defaultWorkspaceRef);
  };

  return {
    runtime: {
      discover: (signal) => call("runtime.discover", {}, { signal }),
    },
    sessions: {
      list: (query) => call("sessions.list", query ?? {}),
      get: (sessionId, signal) => call("sessions.get", { sessionId }, { signal }),
      create: (params, signal) => call("sessions.create", params ?? {}, { signal }),
      update: (params) => call("sessions.update", params),
      delete: (sessionId) => call("sessions.delete", { sessionId }),
      fork: (params) => call("sessions.fork", params),
      rollback: (params) => call("sessions.rollback", params),
      export: (sessionId, format) => call("sessions.export", { sessionId, format }),
      import: (artifact) =>
        call("sessions.import", {
          artifact,
        }),
    },
    runs: {
      start: (params, signal) =>
        openMutation(
          "runs.start",
          params,
          async (idempotencyKey, attempt) => {
            // Subscribe BEFORE the POST, then bind the tree to the runtime-assigned
            // root segmentId. Under streamable HTTP the response + its event frames
            // arrive on the same ordered stream, so the first events follow the
            // response immediately; binding only after `call` resolves could drop
            // the head (see streamRunEvents).
            const stream = streamRunEvents(client, attempt.signal);
            const result = await callOrDispose(stream, () =>
              perform("runs.start", params, {
                signal: stream.requestSignal,
                idempotencyKey,
                idempotencyNamespace: attempt.idempotencyNamespace,
                onRequestRpcId: stream.bindRequest,
              }),
            );
            stream.bind(result.runId, result.segmentId);
            return { result, events: stream.events };
          },
          signal,
        ),
      resume: (params, signal) =>
        openMutation(
          "runs.resume",
          params,
          async (idempotencyKey, attempt) => {
            // A resume opens a NEW segment of the SAME run — bind the tree to it.
            const stream = streamRunEvents(client, attempt.signal);
            const result = await callOrDispose(stream, () =>
              perform("runs.resume", params, {
                signal: stream.requestSignal,
                idempotencyKey,
                idempotencyNamespace: attempt.idempotencyNamespace,
                onRequestRpcId: stream.bindRequest,
              }),
            );
            stream.bind(result.runId, result.segmentId);
            return { result, events: stream.events };
          },
          signal,
        ),
      subscribe: async (params, signal, options) => {
        // Reattach to the segment the caller named; the ack echoes it, and the tree
        // binds to it (same deferred-bind head-drop guard).
        const stream = streamRunEvents(client, signal);
        const result = await callOrDispose(stream, () =>
          call("runs.subscribe", params, {
            signal: stream.requestSignal,
            lastEventId: options?.lastEventId,
            onRequestRpcId: stream.bindRequest,
          }),
        );
        stream.bind(result.runId, result.segmentId);
        return { result, events: stream.events };
      },
      cancel: (runId, reason) => call("runs.cancel", { runId, reason }),
      steer: (runId, expectedSegmentId, input) =>
        call("runs.steer", { runId, expectedSegmentId, input }),
      get: (runId, signal) => call("runs.get", { runId }, { signal }),
      list: (query, signal) => call("runs.list", query ?? {}, signal ? { signal } : undefined),
    },
    plan: {
      get: (sessionId, signal) => call("plan.get", { sessionId }, signal ? { signal } : undefined),
    },
    runtimeEvents: {
      subscribe: async (params, signal) => {
        const stream = streamRuntimeEvents(client, signal);
        const result = await callOrDispose(stream, () =>
          call(RUNTIME_SUBSCRIBE_METHOD, params, {
            signal: stream.requestSignal,
            onRequestRpcId: stream.bindRequest,
          }),
        );
        return { result, events: stream.events };
      },
    },
    interrupts: {
      list: (query, signal) =>
        call("interrupts.list", query ?? {}, signal ? { signal } : undefined),
    },
    items: {
      list: (params, signal) => call("items.list", params, signal ? { signal } : undefined),
    },
    workspaces: {
      resolve: (ref, signal) => call("workspaces.resolve", ref ? { ref } : {}, { signal }),
      list: () => call("workspaces.list", {}),
      open: openWorkspace,
    },
    workspace: (ref) => bindWorkspace(call, ref),
    hooks: {
      setTrust: (projectRoot, trusted) =>
        call("hooks.setTrust", {
          projectRoot,
          trusted,
        }),
    },
    skills: {
      listLibrary: () => call("skills.library.list", {}),
      archive: (name) => call("skills.library.archive", { name }),
      restore: (name) => call("skills.library.restore", { name }),
    },
    mcp: {
      list: () => call("mcp.servers.list", {}),
      create: (params) => call("mcp.servers.create", params),
      update: (params) => call("mcp.servers.update", params),
      delete: (server) => call("mcp.servers.delete", { server }),
      test: (params) => call("mcp.servers.test", params),
      listTools: (server) => call("mcp.tools.list", server ? { server } : {}),
      reconnect: (server) => call("mcp.servers.reconnect", { server }),
      authorizationAttempts: {
        create: (server, signal) =>
          call("mcp.authorizationAttempts.create", { server }, { signal }),
        get: (attemptId, signal) =>
          call("mcp.authorizationAttempts.get", { attemptId }, { signal }),
      },
    },
    providers: {
      list: () => call("providers.list", {}),
      update: (params) => call("providers.update", params),
      test: (provider) => call("providers.test", { provider }),
    },
    models: {
      list: (provider) => call("models.list", provider ? { provider } : {}),
      getUtilityRole: () => call("models.getUtilityRole", {}),
      setUtilityRole: (params) => call("models.setUtilityRole", params),
      getEmbeddingRole: () => call("models.getEmbeddingRole", {}),
      setEmbeddingRole: (params) => call("models.setEmbeddingRole", params),
    },
    tools: {
      list: () => call("tools.list", {}),
      invoke: (params) => call("tools.invoke", params),
    },
    usage: {
      session: (sessionId) => call("usage.session", { sessionId }),
      summary: (params) => call("usage.summary", params ?? {}),
    },
    agentMemory: {
      list: (params) => call("agentMemory.list", params ?? {}),
      review: (id, decision) =>
        call("agentMemory.review", {
          id,
          decision,
        }),
      update: (params) => call("agentMemory.update", params),
      delete: (id) => call("agentMemory.delete", { id }),
      add: (params) => call("agentMemory.add", params),
    },
    goals: {
      get: (sessionId) => call("goals.get", { sessionId }),
      start: (params, signal) => call("goals.start", params, { signal }),
      stop: (sessionId, signal) => call("goals.stop", { sessionId }, { signal }),
      resume: (sessionId, signal) => call("goals.resume", { sessionId }, { signal }),
    },
    feedback: {
      create: (params) => call("feedback.create", params),
    },
    approval: {
      getMode: () => call("approval.getMode", {}),
      setMode: (mode) => call("approval.setMode", { mode }),
      listRules: (sessionId) => call("approval.listRules", { sessionId }),
      forgetRule: (id) => call("approval.forgetRule", { id }),
    },
    schedules: {
      list: (query) => call("schedules.list", query ?? {}),
      create: (params) => call("schedules.create", params),
      update: (params) => call("schedules.update", params),
      delete: (id) => call("schedules.delete", { id }),
      runNow: (id) => call("schedules.runNow", { id }),
    },
  };
}
