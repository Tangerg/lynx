// Public surface of the Lyra Runtime Protocol v2 client. See docs/protocol/API.md.
//
// The SDK is transport-agnostic: inject a `Transport`, get a typed client.
//
//   const client = createLyraClient(createHttpTransport({ baseUrl, localToken }));
//   await client.runtime.initialize({ ... });   // handshake
//   const sessions = await client.sessions.list();
//   const { result, events } = await client.runs.start({ ... });
//   await client.close();
//
// In tests, swap createHttpTransport with createMemoryTransport. The lower-
// level building blocks (createRpcClient + createMethods) stay exported for
// advanced use; `createLyraClient` just composes them. Sidecar metadata
// (/v2/info, /v2/health) is HTTP-only — see createSidecarClient.

export { createPushPullChannel } from "./channel";
export type { PushPullChannel } from "./channel";
export { createRpcClient } from "./client";
export type { NotificationHandler, RpcClient } from "./client";
export { isErrorType, RpcError, RpcTransportError } from "./errors";
export { asAttachmentId, asEventId, asItemId, asRunId, asSessionId } from "./ids";
export type { AttachmentId, EventId, ItemId, RunId, SessionId } from "./ids";
export { createMethods } from "./methods";
export type { Methods, StreamingResult } from "./methods";
export { createLyraClient } from "./sdk";
export type { LyraClient } from "./sdk";
export type {
  // Lifecycle / capabilities
  ClientCapabilities,
  ServerCapabilities,
  ServerFeatures,
  ServerInfo,
  InterruptType,
  InitializeRequest,
  InitializeResponse,
  ShutdownRequest,
  CanceledNotification,
  // Sessions / projects
  Session,
  SessionStatus,
  Project,
  CreateSessionRequest,
  UpdateSessionRequest,
  ForkSessionRequest,
  RollbackSessionRequest,
  RollbackSessionResponse,
  DroppedRun,
  ExportSessionRequest,
  ExportSessionResponse,
  SessionArtifact,
  ImportSessionResponse,
  // Runs
  RunRef,
  RunMode,
  RunOutcome,
  RunProgress,
  RunResult,
  StartRunRequest,
  StartRunResponse,
  ResumeRunRequest,
  ResumeRunResponse,
  // Items
  Item,
  ItemBase,
  ItemStatus,
  ItemType,
  ContentBlock,
  PlanStep,
  Question,
  QuestionField,
  QuestionFieldBase,
  QuestionOption,
  ToolInvocation,
  ListItemsRequest,
  ListItemsResponse,
  // Streaming
  RunEvent,
  StreamEvent,
  StreamEventType,
  ItemDelta,
  JsonPatch,
  // HITL
  Interrupt,
  ApprovalPayload,
  ToolResultPayload,
  OpenInterrupt,
  InterruptResponse,
  ApprovalResponse,
  AnswerResponse,
  ToolResultResponse,
  // Diff / search / files
  DiffRow,
  Diff,
  FileDiff,
  SearchHit,
  WebSearchResult,
  WorkspaceFileChange,
  FileEdit,
  FileHead,
  FileLine,
  GrepMatch,
  GrepResult,
  // Code intelligence (B7) / file browse (B8) — docs/613 proposal
  CodeQuery,
  CodePosition,
  CodeRange,
  CodeLocation,
  Hover,
  SymbolKind,
  DocumentSymbol,
  WorkspaceSymbol,
  Diagnostic,
  FileEntry,
  FileContent,
  // Usage / error / context / tools
  Usage,
  ModelUsage,
  ProblemData,
  FieldError,
  ContextItem,
  JsonSchema,
  ToolSpec,
  GenerationParams,
  InvokeToolRequest,
  // Providers / models
  Provider,
  ProviderTestResult,
  ConfigureProviderRequest,
  Model,
  // Workspace optional domains
  Skill,
  AgentDoc,
  McpServer,
  McpStatus,
  McpTool,
  MemoryScope,
  MemoryEntry,
  WorkspaceQuery,
  WatchSpec,
  SubscribeWorkspaceRequest,
  WorkspaceEvent,
  WorkspaceEventType,
  // Attachments / feedback
  Attachment,
  CreateUploadUrlRequest,
  CreateUploadUrlResponse,
  FeedbackRequest,
  // Pagination
  Page,
  PageQuery,
} from "./shapes";
export { isDurableEvent } from "./shapes";
export {
  streamRunEvents,
  streamRunEventsDeferred,
  streamWorkspaceEvents,
  RUN_EVENT_METHOD,
  WORKSPACE_EVENT_METHOD,
} from "./stream";
export { createSidecarClient } from "./sidecar";
export type { HealthStatus, RuntimeInfo, SidecarClient, SidecarClientConfig } from "./sidecar";
export { createShellClient } from "./shell";
export type { ShellClient, ShellClientConfig, SideloadEntry } from "./shell";
export { createHttpTransport } from "./transports/http";
export type { HttpTransportConfig } from "./transports/http";
export { createMemoryTransport } from "./transports/memory";
export type { MemoryTransport } from "./transports/memory";
export type { Transport } from "./transport";
export {
  JSONRPC_VERSION,
  RPC_ATTACHMENT_TOO_LARGE,
  RPC_CAPABILITY_NOT_NEGOTIATED,
  RPC_CHECKPOINT_UNAVAILABLE,
  RPC_CWD_UNAVAILABLE,
  RPC_IDEMPOTENCY_CONFLICT,
  RPC_INTERNAL_ERROR,
  RPC_INTERRUPT_NOT_OPEN,
  RPC_INVALID_PARAMS,
  RPC_INVALID_PROTOCOL_VERSION,
  RPC_INVALID_REQUEST,
  RPC_ITEM_NOT_FOUND,
  RPC_METHOD_NOT_FOUND,
  RPC_PARSE_ERROR,
  RPC_PATH_OUTSIDE_ROOT,
  RPC_PROVIDER_ERROR,
  RPC_RUN_ALREADY_FINISHED,
  RPC_RUN_NOT_FOUND,
  RPC_SESSION_NOT_FOUND,
  RPC_TOOL_DENIED,
  RPC_UNSUPPORTED_MIME,
  errorType,
  errorDetail,
} from "./types";
export type {
  RpcErrorPayload,
  RpcId,
  RpcMessage,
  RpcNotification,
  RpcRequest,
  RpcResponse,
} from "./types";
