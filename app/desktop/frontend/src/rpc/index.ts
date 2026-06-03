// Public surface of the Lyra Runtime Protocol v2 client. See docs/API.md.
//
// Typical wiring (composition root, main/container.ts):
//
//   const transport = createHttpTransport({ baseUrl, localToken });
//   const client    = createRpcClient(transport);
//   const methods   = createMethods(client);
//   const sidecar   = createSidecarClient({ baseUrl });
//
// Then:
//   await sidecar.info();                     // pre-handshake liveness
//   await methods.runtime.initialize({...});  // handshake
//   const sessions = await methods.sessions.list();
//
// In tests, swap createHttpTransport with createMemoryTransport.

export { createPushPullChannel } from "./channel";
export type { PushPullChannel } from "./channel";
export { createRpcClient } from "./client";
export type { NotificationHandler, RpcClient } from "./client";
export { RpcError, RpcTransportError } from "./errors";
export { asAttachmentId, asEventId, asItemId, asRunId, asSessionId, asTaskId } from "./ids";
export type { AttachmentId, EventId, ItemId, RunId, SessionId, TaskId } from "./ids";
export { createMethods } from "./methods";
export type { Methods, StreamingResult } from "./methods";
export type {
  // Lifecycle / capabilities
  ClientCapabilities,
  ServerCapabilities,
  ServerFeatures,
  ServerInfo,
  InterruptKind,
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
  ExportSessionRequest,
  ExportSessionResponse,
  // Runs
  RunRef,
  RunOutcome,
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
  ToolKind,
  ListItemsRequest,
  ListItemsResponse,
  EditItemRequest,
  EditItemResponse,
  // Streaming
  RunEvent,
  StreamEvent,
  StreamEventType,
  ItemDelta,
  JsonPatch,
  // HITL
  Interrupt,
  OpenInterrupt,
  InterruptResponse,
  ApprovalResponse,
  AnswerResponse,
  ToolResultResponse,
  // Diff / search / files
  DiffRow,
  SearchResult,
  FileChange,
  FileHead,
  FileLine,
  GrepMatch,
  GrepResult,
  // Usage / error / context / tools
  Usage,
  ProblemData,
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
  McpTool,
  MemoryScope,
  MemoryEntry,
  WorkspaceQuery,
  // Attachments / background / feedback
  Attachment,
  CreateUploadUrlRequest,
  CreateUploadUrlResponse,
  BackgroundTask,
  FeedbackRequest,
  // Pagination
  Page,
  PageQuery,
} from "./shapes";
export {
  streamBackgroundUpdates,
  streamRunEvents,
  streamRunEventsDeferred,
  RUN_EVENT_METHOD,
  BACKGROUND_UPDATE_METHOD,
} from "./stream";
export { createSidecarClient } from "./sidecar";
export type { HealthStatus, RuntimeInfo, SidecarClient, SidecarClientConfig } from "./sidecar";
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
  RPC_RUN_NOT_RUNNING,
  RPC_SESSION_NOT_FOUND,
  RPC_TOOL_DENIED,
  RPC_UNSUPPORTED_MIME,
  errorType,
} from "./types";
export type {
  RpcErrorPayload,
  RpcId,
  RpcMessage,
  RpcNotification,
  RpcRequest,
  RpcResponse,
} from "./types";
