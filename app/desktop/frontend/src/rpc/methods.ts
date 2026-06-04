// Typed wrappers for every method in docs/API.md §7. Grouped by namespace
// so callers do `methods.runs.start(...)` rather than
// `client.call("runs.start")`. The factory takes an RpcClient and returns
// the full typed surface.
//
// Streaming methods (runs.start / runs.resume / runs.subscribe /
// background.subscribe) return `{ result, events }` where `events` is an
// AsyncIterable. Run streams carry the whole run tree and end on the root
// run's `run.finished` (see ./stream).

import type { RpcClient } from "./client";
import type { AttachmentId, ItemId, RunId, SessionId, TaskId } from "./ids";
import type {
  AgentDoc,
  Attachment,
  BackgroundTask,
  CanceledNotification,
  ConfigureProviderRequest,
  CreateSessionRequest,
  CreateUploadUrlRequest,
  CreateUploadUrlResponse,
  DiffRow,
  EditItemResponse,
  ExportSessionResponse,
  FeedbackRequest,
  FileChange,
  FileHead,
  ForkSessionRequest,
  GrepResult,
  InitializeRequest,
  InitializeResponse,
  InvokeToolRequest,
  ListItemsResponse,
  McpServer,
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
  RunEvent,
  RunRef,
  Session,
  ShutdownRequest,
  Skill,
  StartRunRequest,
  StartRunResponse,
  ToolSpec,
  UpdateSessionRequest,
} from "./shapes";
import { streamBackgroundUpdates, streamRunEvents, streamRunEventsDeferred } from "./stream";

export interface StreamingResult<R, E> {
  result: R;
  events: AsyncIterable<E>;
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
    create: (params?: CreateSessionRequest) => Promise<Session>;
    update: (params: UpdateSessionRequest) => Promise<Session>;
    delete: (sessionId: SessionId) => Promise<void>;
    fork: (params: ForkSessionRequest) => Promise<Session>;
    export: (sessionId: SessionId, format?: "md" | "json") => Promise<ExportSessionResponse>;
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
    // Running runs only (§7.3); finished/interrupted via listOpenInterrupts or items history.
    list: (sessionId?: SessionId) => Promise<RunRef[]>;
    // Durable HITL discovery — resumable interrupted runs (§7.3 / §10.2).
    listOpenInterrupts: (sessionId?: SessionId) => Promise<OpenInterrupt[]>;
  };
  items: {
    list: (params: {
      sessionId: SessionId;
      cursor?: string;
      limit?: number;
    }) => Promise<ListItemsResponse>;
    // Edit an item → a continuation Run (semantics like resume).
    edit: (itemId: ItemId, replacement: StartRunRequest["input"]) => Promise<EditItemResponse>;
  };
  workspace: {
    listFileChanges: (cwd?: string) => Promise<FileChange[]>;
    getDiff: (params?: { cwd?: string; path?: string }) => Promise<DiffRow[]>;
    getFileHead: (params: { path: string; cwd?: string; lines?: number }) => Promise<FileHead>;
    grep: (params: {
      query: string;
      cwd?: string;
      path?: string;
      limit?: number;
    }) => Promise<GrepResult>;
    listProjects: () => Promise<Project[]>;
    listSkills: (cwd?: string) => Promise<Skill[]>;
    listAgentDocs: (cwd?: string) => Promise<AgentDoc[]>;
    mcp: {
      listServers: () => Promise<McpServer[]>;
      listTools: (server?: string) => Promise<McpTool[]>;
      reconnect: (server: string) => Promise<void>;
    };
  };
  providers: {
    list: () => Promise<Provider[]>;
    configure: (params: ConfigureProviderRequest) => Promise<Provider>;
    test: (providerId: string) => Promise<ProviderTestResult>;
  };
  models: {
    list: (provider?: string) => Promise<Model[]>;
  };
  tools: {
    list: () => Promise<ToolSpec[]>;
    invoke: (params: InvokeToolRequest) => Promise<unknown>;
  };
  memory: {
    list: (cwd?: string) => Promise<MemoryEntry[]>;
    get: (scope: MemoryScope, cwd?: string) => Promise<MemoryEntry>;
    update: (params: { scope: MemoryScope; cwd?: string; content: string }) => Promise<void>;
  };
  attachments: {
    createUploadUrl: (params: CreateUploadUrlRequest) => Promise<CreateUploadUrlResponse>;
    get: (attachmentId: AttachmentId) => Promise<Attachment>;
    delete: (attachmentId: AttachmentId) => Promise<void>;
  };
  background: {
    list: () => Promise<BackgroundTask[]>;
    subscribe: (
      taskId: TaskId,
      signal?: AbortSignal,
    ) => Promise<StreamingResult<{ taskId: TaskId }, BackgroundTask>>;
    cancel: (taskId: TaskId) => Promise<void>;
  };
  feedback: {
    create: (params: FeedbackRequest) => Promise<void>;
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
      create: (params) => client.call<Session>("sessions.create", params ?? {}),
      update: (params) => client.call<Session>("sessions.update", params),
      delete: (sessionId) => client.call<void>("sessions.delete", { sessionId }),
      fork: (params) => client.call<Session>("sessions.fork", params),
      export: (sessionId, format) =>
        client.call<ExportSessionResponse>("sessions.export", { sessionId, format }),
    },
    runs: {
      start: async (params, signal) => {
        // Subscribe BEFORE the POST, then bind to the runtime-assigned runId.
        // Under streamable HTTP the response + its event frames arrive on the
        // same ordered stream, so the first events follow the response
        // immediately; binding only after `call` resolves could drop the head
        // (see streamRunEventsDeferred).
        const stream = streamRunEventsDeferred(client, signal);
        const result = await client.call<StartRunResponse>("runs.start", params, signal);
        stream.bind(result.runId);
        return { result, events: stream.events };
      },
      resume: async (params, signal) => {
        const stream = streamRunEventsDeferred(client, signal);
        const result = await client.call<ResumeRunResponse>("runs.resume", params, signal);
        stream.bind(result.runId);
        return { result, events: stream.events };
      },
      subscribe: async (runId, signal) => {
        const events = streamRunEvents(client, runId, signal);
        const result = await client.call<{ runId: RunId }>("runs.subscribe", { runId }, signal);
        return { result, events };
      },
      cancel: (runId, reason) => client.call<void>("runs.cancel", { runId, reason }),
      list: (sessionId) => client.call<RunRef[]>("runs.list", sessionId ? { sessionId } : {}),
      listOpenInterrupts: (sessionId) =>
        client.call<OpenInterrupt[]>("runs.listOpenInterrupts", sessionId ? { sessionId } : {}),
    },
    items: {
      list: (params) => client.call<ListItemsResponse>("items.list", params),
      edit: (itemId, replacement) =>
        client.call<EditItemResponse>("items.edit", { itemId, replacement }),
    },
    workspace: {
      listFileChanges: (cwd) => client.call<FileChange[]>("workspace.listFileChanges", { cwd }),
      getDiff: (params) => client.call<DiffRow[]>("workspace.getDiff", params ?? {}),
      getFileHead: (params) => client.call<FileHead>("workspace.getFileHead", params),
      grep: (params) => client.call<GrepResult>("workspace.grep", params),
      listProjects: () => client.call<Project[]>("workspace.listProjects"),
      listSkills: (cwd) => client.call<Skill[]>("workspace.listSkills", { cwd }),
      listAgentDocs: (cwd) => client.call<AgentDoc[]>("workspace.listAgentDocs", { cwd }),
      mcp: {
        listServers: () => client.call<McpServer[]>("workspace.mcp.listServers"),
        listTools: (server) =>
          client.call<McpTool[]>("workspace.mcp.listTools", server ? { server } : {}),
        reconnect: (server) => client.call<void>("workspace.mcp.reconnect", { server }),
      },
    },
    providers: {
      list: () => client.call<Provider[]>("providers.list"),
      configure: (params) => client.call<Provider>("providers.configure", params),
      test: (providerId) => client.call<ProviderTestResult>("providers.test", { providerId }),
    },
    models: {
      list: (provider) => client.call<Model[]>("models.list", provider ? { provider } : {}),
    },
    tools: {
      list: () => client.call<ToolSpec[]>("tools.list"),
      invoke: (params) => client.call<unknown>("tools.invoke", params),
    },
    memory: {
      list: (cwd) => client.call<MemoryEntry[]>("memory.list", { cwd }),
      get: (scope, cwd) => client.call<MemoryEntry>("memory.get", { scope, cwd }),
      update: (params) => client.call<void>("memory.update", params),
    },
    attachments: {
      createUploadUrl: (params) =>
        client.call<CreateUploadUrlResponse>("attachments.createUploadUrl", params),
      get: (attachmentId) => client.call<Attachment>("attachments.get", { attachmentId }),
      delete: (attachmentId) => client.call<void>("attachments.delete", { attachmentId }),
    },
    background: {
      list: () => client.call<BackgroundTask[]>("background.list"),
      subscribe: async (taskId, signal) => {
        const result = await client.call<{ taskId: TaskId }>(
          "background.subscribe",
          { taskId },
          signal,
        );
        return { result, events: streamBackgroundUpdates(client, result.taskId, signal) };
      },
      cancel: (taskId) => client.call<void>("background.cancel", { taskId }),
    },
    feedback: {
      create: (params) => client.call<void>("feedback.create", params),
    },
  };
}
