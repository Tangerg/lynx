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
  ApprovalSubmission,
  BackgroundTask,
  BackgroundUpdate,
  CreateSessionInput,
  CreateUploadUrlInput,
  CreateUploadUrlResult,
  DiffRow,
  FeedbackInput,
  FileChange,
  FileLine,
  GrepResult,
  InitializeParams,
  InitializeResult,
  MCPServer,
  Message,
  MessageEditResult,
  Model,
  Page,
  PageQuery,
  Project,
  Provider,
  ProviderTestResult,
  Session,
  SessionPatch,
  ShutdownParams,
  Skill,
  StartRunParams,
  StartRunResult,
  TermLine,
  ToolSpec,
} from "./shapes";
import { streamBackgroundUpdates, streamRunEvents, streamTerminalOutput } from "./stream";

export interface StreamingResult<R, E> {
  result: R;
  events: AsyncIterable<E>;
}

export interface Methods {
  runtime: {
    initialize: (params: InitializeParams) => Promise<InitializeResult>;
    shutdown: (params?: ShutdownParams) => Promise<void>;
    ping: () => Promise<void>;
  };
  runs: {
    start: (
      params: StartRunParams,
      signal?: AbortSignal,
    ) => Promise<StreamingResult<StartRunResult, BaseEvent>>;
    cancel: (runId: RunId, reason?: string) => Promise<void>;
    approval: {
      submit: (params: ApprovalSubmission) => Promise<void>;
    };
  };
  sessions: {
    list: (query?: PageQuery) => Promise<Page<Session>>;
    get: (id: SessionId) => Promise<Session>;
    create: (input: CreateSessionInput) => Promise<Session>;
    update: (id: SessionId, patch: SessionPatch) => Promise<Session>;
    delete: (id: SessionId) => Promise<void>;
    // Per PROTOCOL_ALIGNMENT v3: first arg is `parentId` (the source
    // session being forked), not `id` — `id` was ambiguous at callsite
    // ("which id, the new one or the source?").
    fork: (parentId: SessionId, atMessageId: MessageId) => Promise<Session>;
    export: (id: SessionId, format: "md" | "json") => Promise<{ url: string }>;
  };
  messages: {
    list: (sessionId: SessionId, query?: PageQuery) => Promise<Page<Message>>;
    edit: (
      sessionId: SessionId,
      messageId: MessageId,
      content: string,
    ) => Promise<MessageEditResult>;
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
      // Per PROTOCOL_ALIGNMENT v3: wire key is `name` (MCP-native identifier).
      reconnect: (name: string) => Promise<void>;
    };
    skills: () => Promise<Skill[]>;
  };
  providers: {
    list: () => Promise<Provider[]>;
    test: (id: string) => Promise<ProviderTestResult>;
  };
  models: {
    list: (provider?: string) => Promise<Model[]>;
  };
  tools: {
    list: () => Promise<ToolSpec[]>;
  };
  attachments: {
    createUploadUrl: (input: CreateUploadUrlInput) => Promise<CreateUploadUrlResult>;
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
    submit: (input: FeedbackInput) => Promise<void>;
  };
}

export function createMethods(client: RpcClient): Methods {
  return {
    runtime: {
      initialize: (params) => client.call<InitializeResult>("runtime.initialize", params),
      shutdown: (params) => client.notify("runtime.shutdown", params ?? {}),
      ping: () => client.call<void>("runtime.ping"),
    },
    runs: {
      start: async (params, signal) => {
        const result = await client.call<StartRunResult>("runs.start", params, signal);
        return {
          result,
          events: streamRunEvents(client, result.runId, signal),
        };
      },
      // Proper Request (not Notification). Semantically distinct from
      // `notifications/cancelled` which cancels an in-flight JSON-RPC
      // Request by JSON-RPC id. `runs.cancel` stops a long-running run
      // by its runId — the runs.start Request has long since returned.
      cancel: (runId, reason) => client.call<void>("runs.cancel", { runId, reason }),
      approval: {
        submit: (params) => client.call<void>("runs.approval.submit", params),
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
      export: (id, format) => client.call<{ url: string }>("sessions.export", { id, format }),
    },
    messages: {
      list: (sessionId, query) =>
        client.call<Page<Message>>("messages.list", { sessionId, ...query }),
      edit: (sessionId, messageId, content) =>
        client.call<MessageEditResult>("messages.edit", { sessionId, messageId, content }),
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
      },
      skills: () => client.call<Skill[]>("workspace.skills"),
    },
    providers: {
      list: () => client.call<Provider[]>("providers.list"),
      test: (id) => client.call<ProviderTestResult>("providers.test", { id }),
    },
    models: {
      list: (provider) => client.call<Model[]>("models.list", provider ? { provider } : {}),
    },
    tools: {
      list: () => client.call<ToolSpec[]>("tools.list"),
    },
    attachments: {
      createUploadUrl: (input) =>
        client.call<CreateUploadUrlResult>("attachments.createUploadUrl", input),
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
