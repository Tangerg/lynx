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
import type { AttachmentId, RunId, SessionId } from "./ids";
import type {
  AgentDoc,
  Attachment,
  CanceledNotification,
  ConfigureProviderRequest,
  CreateSessionRequest,
  CreateUploadUrlRequest,
  CreateUploadUrlResponse,
  Diff,
  ExportSessionResponse,
  FeedbackRequest,
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
  WorkspaceFileChange,
} from "./shapes";
import { streamRunEvents, streamRunEventsDeferred } from "./stream";

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
    getDiff: (params?: { cwd?: string; path?: string; limit?: number }) => Promise<Diff>;
    getFileHead: (params: { path: string; cwd?: string; lines?: number }) => Promise<FileHead>;
    grep: (params: {
      query: string;
      cwd?: string;
      path?: string;
      limit?: number;
    }) => Promise<GrepResult>;
    listProjects: () => Promise<Page<Project>>;
    listSkills: (cwd?: string) => Promise<Page<Skill>>;
    listAgentDocs: (cwd?: string) => Promise<Page<AgentDoc>>;
    mcp: {
      listServers: () => Promise<Page<McpServer>>;
      listTools: (server?: string) => Promise<Page<McpTool>>;
      reconnect: (server: string) => Promise<void>;
    };
  };
  providers: {
    list: () => Promise<Page<Provider>>;
    configure: (params: ConfigureProviderRequest) => Promise<Provider>;
    test: (provider: string) => Promise<ProviderTestResult>;
  };
  models: {
    list: (provider?: string) => Promise<Page<Model>>;
  };
  tools: {
    list: () => Promise<Page<ToolSpec>>;
    invoke: (params: InvokeToolRequest) => Promise<unknown>;
  };
  memory: {
    list: (cwd?: string) => Promise<Page<MemoryEntry>>;
    get: (scope: MemoryScope, cwd?: string) => Promise<MemoryEntry>;
    update: (params: { scope: MemoryScope; cwd?: string; content: string }) => Promise<void>;
  };
  attachments: {
    createUploadUrl: (params: CreateUploadUrlRequest) => Promise<CreateUploadUrlResponse>;
    get: (attachmentId: AttachmentId) => Promise<Attachment>;
    delete: (attachmentId: AttachmentId) => Promise<void>;
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
        //
        // If the call REJECTS the stream must be disposed explicitly: nobody
        // will ever iterate `events`, so its self-cleaning iterator never runs
        // — the subscription would leak and its unbound buffer would accumulate
        // every run event in the app, forever.
        const stream = streamRunEventsDeferred(client, signal);
        let result: StartRunResponse;
        try {
          result = await client.call<StartRunResponse>("runs.start", params, signal);
        } catch (err) {
          stream.dispose();
          throw err;
        }
        stream.bind(result.runId);
        return { result, events: stream.events };
      },
      resume: async (params, signal) => {
        const stream = streamRunEventsDeferred(client, signal);
        let result: ResumeRunResponse;
        try {
          result = await client.call<ResumeRunResponse>("runs.resume", params, signal);
        } catch (err) {
          stream.dispose();
          throw err;
        }
        stream.bind(result.runId);
        return { result, events: stream.events };
      },
      subscribe: async (runId, signal) => {
        const stream = streamRunEvents(client, runId, signal);
        let result: { runId: RunId };
        try {
          result = await client.call<{ runId: RunId }>("runs.subscribe", { runId }, signal);
        } catch (err) {
          stream.dispose();
          throw err;
        }
        return { result, events: stream.events };
      },
      cancel: (runId, reason) => client.call<void>("runs.cancel", { runId, reason }),
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
      listProjects: () => client.call<Page<Project>>("workspace.listProjects"),
      listSkills: (cwd) => client.call<Page<Skill>>("workspace.listSkills", { cwd }),
      listAgentDocs: (cwd) => client.call<Page<AgentDoc>>("workspace.listAgentDocs", { cwd }),
      mcp: {
        listServers: () => client.call<Page<McpServer>>("workspace.mcp.listServers"),
        listTools: (server) =>
          client.call<Page<McpTool>>("workspace.mcp.listTools", server ? { server } : {}),
        reconnect: (server) => client.call<void>("workspace.mcp.reconnect", { server }),
      },
    },
    providers: {
      list: () => client.call<Page<Provider>>("providers.list"),
      configure: (params) => client.call<Provider>("providers.configure", params),
      test: (provider) => client.call<ProviderTestResult>("providers.test", { provider }),
    },
    models: {
      list: (provider) => client.call<Page<Model>>("models.list", provider ? { provider } : {}),
    },
    tools: {
      list: () => client.call<Page<ToolSpec>>("tools.list"),
      invoke: (params) => client.call<unknown>("tools.invoke", params),
    },
    memory: {
      list: (cwd) => client.call<Page<MemoryEntry>>("memory.list", { cwd }),
      get: (scope, cwd) => client.call<MemoryEntry>("memory.get", { scope, cwd }),
      update: (params) => client.call<void>("memory.update", params),
    },
    attachments: {
      createUploadUrl: (params) =>
        client.call<CreateUploadUrlResponse>("attachments.createUploadUrl", params),
      get: (attachmentId) => client.call<Attachment>("attachments.get", { attachmentId }),
      delete: (attachmentId) => client.call<void>("attachments.delete", { attachmentId }),
    },
    feedback: {
      create: (params) => client.call<void>("feedback.create", params),
    },
  };
}
