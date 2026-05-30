// Typed wrappers for every method in docs/API.md §5.2. Grouped by
// namespace for readability — callers do `methods.sessions.list()`
// rather than `client.call("sessions.list")`. The factory takes a
// RpcClient and returns the full typed surface.
//
// Streaming methods (runs.start / workspace.terminal.subscribe /
// background.subscribe) return `{ result, events }` where `events` is
// an AsyncIterable filtered by the resource id (runId / taskId). The
// filter helpers live in `./stream`.

import type { BaseEvent } from "@ag-ui/core";
import type { RpcClient } from "./client";
import type { AttachmentId, MessageId, RunId, SessionId, TaskId } from "./ids";
import type {
  AgentDoc,
  AnswerQuestionRequest,
  BackgroundTask,
  BackgroundUpdate,
  ConfigureProviderRequest,
  CreateSessionRequest,
  CreateUploadURLRequest,
  CreateUploadURLResponse,
  DiffRow,
  EditMessageResponse,
  ExportSessionResponse,
  FeedbackRequest,
  FileChange,
  FileLine,
  GetMemoryResponse,
  GrepResult,
  InitializeRequest,
  InitializeResponse,
  InvokeToolRequest,
  InvokeToolResponse,
  MCPServer,
  MemoryEntry,
  MemoryScope,
  Message,
  Model,
  Page,
  PageQuery,
  Project,
  Provider,
  ProviderTestResult,
  RunSummary,
  Session,
  ShutdownRequest,
  Skill,
  StartRunRequest,
  StartRunResponse,
  SubmitApprovalRequest,
  TermLine,
  ToolSpec,
  UpdateMemoryRequest,
  UpdateSessionRequest,
} from "./shapes";
import {
  streamBackgroundUpdates,
  streamRunEvents,
  streamRunEventsDeferred,
  streamTerminalOutput,
} from "./stream";

export interface StreamingResult<R, E> {
  result: R;
  events: AsyncIterable<E>;
}

export interface Methods {
  runtime: {
    initialize: (params: InitializeRequest) => Promise<InitializeResponse>;
    shutdown: (params?: ShutdownRequest) => Promise<void>;
    ping: () => Promise<void>;
  };
  runs: {
    start: (
      params: StartRunRequest,
      signal?: AbortSignal,
    ) => Promise<StreamingResult<StartRunResponse, BaseEvent>>;
    // Crash-recovery / durable-HITL discovery: only active|waiting runs (§3.3).
    list: (sessionId: SessionId) => Promise<RunSummary[]>;
    // Reattach an existing run (started on another connection) to this one
    // and replay its events from the start / Last-Event-Id anchor (§3.2/§3.3).
    subscribe: (
      runId: RunId,
      signal?: AbortSignal,
    ) => Promise<StreamingResult<{ runId: RunId }, BaseEvent>>;
    cancel: (runId: RunId, reason?: string) => Promise<void>;
    approval: {
      submit: (params: SubmitApprovalRequest) => Promise<void>;
    };
    question: {
      answer: (params: AnswerQuestionRequest) => Promise<void>;
    };
  };
  sessions: {
    list: (query?: PageQuery) => Promise<Page<Session>>;
    get: (id: SessionId) => Promise<Session>;
    create: (input: CreateSessionRequest) => Promise<Session>;
    update: (id: SessionId, patch: UpdateSessionRequest) => Promise<Session>;
    delete: (id: SessionId) => Promise<void>;
    // Per API.md §5.2: first arg is `parentId` (the source session being
    // forked), not `id` — `id` was ambiguous at the callsite ("which id,
    // the new one or the source?").
    fork: (parentId: SessionId, atMessageId: MessageId) => Promise<Session>;
    export: (id: SessionId, format: "md" | "json") => Promise<ExportSessionResponse>;
  };
  messages: {
    list: (sessionId: SessionId, query?: PageQuery) => Promise<Page<Message>>;
    edit: (
      sessionId: SessionId,
      messageId: MessageId,
      content: string,
    ) => Promise<EditMessageResponse>;
  };
  workspace: {
    filesChanged: () => Promise<FileChange[]>;
    diff: (path: string) => Promise<DiffRow[]>;
    fileHead: (path: string) => Promise<FileLine[]>;
    grep: (query: string) => Promise<GrepResult>;
    terminal: {
      subscribe: (
        runId: RunId,
        signal?: AbortSignal,
      ) => Promise<StreamingResult<{ runId: RunId }, TermLine>>;
    };
    projects: () => Promise<Project[]>;
    mcp: {
      list: () => Promise<MCPServer[]>;
      // Per API.md §6.5: wire key is `name` (MCP-native identifier).
      reconnect: (name: string) => Promise<void>;
      // Expand one MCP server's concrete tools (list page only gives a count).
      tools: (name: string) => Promise<ToolSpec[]>;
    };
    skills: () => Promise<Skill[]>;
    // Cascade-discovered AGENTS.md bodies (sidecar /v1/info only gives paths).
    agentDocs: () => Promise<AgentDoc[]>;
  };
  providers: {
    list: () => Promise<Provider[]>;
    test: (id: string) => Promise<ProviderTestResult>;
    // Configure provider credentials/endpoint; returns the updated Provider
    // (hasApiKey reflects the result). Configure ≠ auth — it's provider mgmt.
    configure: (input: ConfigureProviderRequest) => Promise<Provider>;
  };
  models: {
    list: (provider?: string) => Promise<Model[]>;
  };
  tools: {
    list: () => Promise<ToolSpec[]>;
    // Invoke a tool directly, bypassing the LLM (diagnostics / workflows).
    invoke: (input: InvokeToolRequest) => Promise<InvokeToolResponse>;
  };
  memory: {
    list: () => Promise<MemoryEntry[]>;
    get: (scope: MemoryScope) => Promise<GetMemoryResponse>;
    update: (input: UpdateMemoryRequest) => Promise<void>;
  };
  attachments: {
    createUploadUrl: (input: CreateUploadURLRequest) => Promise<CreateUploadURLResponse>;
    delete: (id: AttachmentId) => Promise<void>;
  };
  background: {
    list: () => Promise<BackgroundTask[]>;
    stop: (taskId: TaskId) => Promise<void>;
    subscribe: (
      taskId: TaskId,
      signal?: AbortSignal,
    ) => Promise<StreamingResult<{ taskId: TaskId }, BackgroundUpdate>>;
  };
  feedback: {
    submit: (input: FeedbackRequest) => Promise<void>;
  };
}

export function createMethods(client: RpcClient): Methods {
  return {
    runtime: {
      initialize: (params) => client.call<InitializeResponse>("runtime.initialize", params),
      shutdown: (params) => client.notify("runtime.shutdown", params ?? {}),
      ping: () => client.call<void>("runtime.ping"),
    },
    runs: {
      start: async (params, signal) => {
        // Subscribe to the run-event stream BEFORE issuing the POST, then bind
        // it to the runtime-assigned runId from the response. A fast runtime
        // emits + broadcasts the whole run the instant it handles the POST, so
        // a subscribe-after-response would drop everything; and a client-
        // supplied runId may be ignored, so we filter by the response's id,
        // not ours (§3.1 / §6.3). streamRunEventsDeferred buffers until bind().
        const stream = streamRunEventsDeferred(client, signal);
        const result = await client.call<StartRunResponse>("runs.start", params, signal);
        stream.bind(result.runId);
        return { result, events: stream.events };
      },
      list: (sessionId) => client.call<RunSummary[]>("runs.list", { sessionId }),
      subscribe: async (runId, signal) => {
        // Same leading-event reasoning as start with a client runId: the
        // runId is known, so subscribe synchronously before the call to
        // avoid dropping replayed head events.
        const events = streamRunEvents(client, runId, signal);
        const result = await client.call<{ runId: RunId }>("runs.subscribe", { runId }, signal);
        return { result, events };
      },
      // Proper Request (not Notification). Semantically distinct from
      // `notifications/canceled` which cancels an in-flight JSON-RPC
      // Request by JSON-RPC id. `runs.cancel` stops a long-running run
      // by its runId — the runs.start Request has long since returned.
      cancel: (runId, reason) => client.call<void>("runs.cancel", { runId, reason }),
      approval: {
        submit: (params) => client.call<void>("runs.approval.submit", params),
      },
      question: {
        answer: (params) => client.call<void>("runs.question.answer", params),
      },
    },
    sessions: {
      list: (query) => client.call<Page<Session>>("sessions.list", query ?? {}),
      get: (id) => client.call<Session>("sessions.get", { id }),
      create: (input) => client.call<Session>("sessions.create", input),
      update: (id, patch) => client.call<Session>("sessions.update", { id, ...patch }),
      delete: (id) => client.call<void>("sessions.delete", { id }),
      fork: (parentId, atMessageId) =>
        client.call<Session>("sessions.fork", { parentId, atMessageId }),
      export: (id, format) => client.call<ExportSessionResponse>("sessions.export", { id, format }),
    },
    messages: {
      list: (sessionId, query) =>
        client.call<Page<Message>>("messages.list", { sessionId, ...query }),
      edit: (sessionId, messageId, content) =>
        client.call<EditMessageResponse>("messages.edit", { sessionId, messageId, content }),
    },
    workspace: {
      filesChanged: () => client.call<FileChange[]>("workspace.filesChanged"),
      diff: (path) => client.call<DiffRow[]>("workspace.diff", { path }),
      fileHead: (path) => client.call<FileLine[]>("workspace.fileHead", { path }),
      grep: (query) => client.call<GrepResult>("workspace.grep", { query }),
      terminal: {
        subscribe: async (runId, signal) => {
          const result = await client.call<{ runId: RunId }>(
            "workspace.terminal.subscribe",
            { runId },
            signal,
          );
          return {
            result,
            events: streamTerminalOutput(client, result.runId, signal),
          };
        },
      },
      projects: () => client.call<Project[]>("workspace.projects"),
      mcp: {
        list: () => client.call<MCPServer[]>("workspace.mcp.list"),
        reconnect: (name) => client.call<void>("workspace.mcp.reconnect", { name }),
        tools: (name) => client.call<ToolSpec[]>("workspace.mcp.tools", { name }),
      },
      skills: () => client.call<Skill[]>("workspace.skills"),
      agentDocs: () => client.call<AgentDoc[]>("workspace.agentDocs"),
    },
    providers: {
      list: () => client.call<Provider[]>("providers.list"),
      test: (id) => client.call<ProviderTestResult>("providers.test", { id }),
      configure: (input) => client.call<Provider>("providers.configure", input),
    },
    models: {
      list: (provider) => client.call<Model[]>("models.list", provider ? { provider } : {}),
    },
    tools: {
      list: () => client.call<ToolSpec[]>("tools.list"),
      invoke: (input) => client.call<InvokeToolResponse>("tools.invoke", input),
    },
    memory: {
      list: () => client.call<MemoryEntry[]>("memory.list"),
      get: (scope) => client.call<GetMemoryResponse>("memory.get", { scope }),
      update: (input) => client.call<void>("memory.update", input),
    },
    attachments: {
      createUploadUrl: (input) =>
        client.call<CreateUploadURLResponse>("attachments.createUploadUrl", input),
      delete: (id) => client.call<void>("attachments.delete", { id }),
    },
    background: {
      list: () => client.call<BackgroundTask[]>("background.list"),
      stop: (taskId) => client.call<void>("background.stop", { taskId }),
      subscribe: async (taskId, signal) => {
        const result = await client.call<{ taskId: TaskId }>(
          "background.subscribe",
          { taskId },
          signal,
        );
        return {
          result,
          events: streamBackgroundUpdates(client, result.taskId, signal),
        };
      },
    },
    feedback: {
      submit: (input) => client.call<void>("feedback.submit", input),
    },
  };
}
