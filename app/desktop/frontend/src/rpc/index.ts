// Public surface of the Lyra Runtime Protocol client. See docs/API.md.
//
// Typical wiring (composition root, main/container.ts):
//
//   const transport = createHttpTransport({ baseUrl, localToken });
//   const client    = createRpcClient(transport);
//   const methods   = createMethods(client);
//   const sidecar   = createSidecarClient({ baseUrl });
//
// Then call:
//   await sidecar.info();                  // pre-handshake liveness
//   await methods.runtime.initialize({...}); // handshake
//   const sessions = await methods.sessions.list();
//
// In tests, swap createHttpTransport with createMemoryTransport.

export { createPushPullChannel } from "./channel";
export type { PushPullChannel } from "./channel";
export { createRpcClient } from "./client";
export type { NotificationHandler, RpcClient } from "./client";
export { RpcError, RpcTransportError } from "./errors";
export {
  asApprovalRequestId,
  asAttachmentId,
  asMessageId,
  asRunId,
  asSessionId,
  asTaskId,
  asToolCallId,
} from "./ids";
export type {
  ApprovalRequestId,
  AttachmentId,
  MessageId,
  RunId,
  SessionId,
  TaskId,
  ToolCallId,
} from "./ids";
export { createMethods } from "./methods";
export type { Methods, StreamingResult } from "./methods";
export type {
  // Lifecycle
  ApprovalDecision,
  ApprovalSubmission,
  // Background
  BackgroundStatus,
  BackgroundTask,
  BackgroundUpdate,
  BackgroundUpdateParams,
  ClientCapabilities,
  // Context / tools
  ContextItem,
  CreateSessionInput,
  CreateUploadUrlInput,
  CreateUploadUrlResult,
  // Workspace
  DiffRow,
  // Feedback
  FeedbackInput,
  FeedbackKind,
  FileChange,
  FileLine,
  GrepMatch,
  GrepResult,
  InitializeParams,
  InitializeResult,
  JsonSchema,
  MCPServer,
  Message,
  MessageEditResult,
  MessageRole,
  Model,
  Page,
  PageQuery,
  Project,
  Provider,
  ProviderTestResult,
  // Notification params (server → client streaming)
  RunClosedParams,
  RunEventParams,
  ServerCapabilities,
  // Sessions / messages
  Session,
  SessionPatch,
  SessionStatus,
  ShutdownParams,
  Skill,
  // Runs
  StartRunParams,
  StartRunResult,
  TerminalOutputParams,
  TermLine,
  TermLineKind,
  ToolCall,
  ToolSpec,
} from "./shapes";
export {
  makeFilteredStream,
  streamBackgroundUpdates,
  streamRunEvents,
  streamTerminalOutput,
} from "./stream";
export type { FilteredStreamSpec } from "./stream";
export { createSidecarClient } from "./sidecar";
export type { HealthStatus, RuntimeInfo, SidecarClient, SidecarClientConfig } from "./sidecar";
export { createHttpTransport } from "./transports/http";
export type { HttpTransportConfig } from "./transports/http";
export { createMemoryTransport } from "./transports/memory";
export type { MemoryTransport } from "./transports/memory";
export type { Transport } from "./transport";
export {
  JSONRPC_VERSION,
  RPC_APPROVAL_REQUIRED,
  RPC_ATTACHMENT_TOO_LARGE,
  RPC_CAPABILITY_NOT_NEGOTIATED,
  RPC_INTERNAL_ERROR,
  RPC_INVALID_PARAMS,
  RPC_INVALID_PROTOCOL_VERSION,
  RPC_INVALID_REQUEST,
  RPC_MESSAGE_NOT_FOUND,
  RPC_METHOD_NOT_FOUND,
  RPC_PARSE_ERROR,
  RPC_PROTOCOL_VIOLATION,
  RPC_PROVIDER_ERROR,
  RPC_PROVIDER_RATE_LIMITED,
  RPC_RUN_NOT_FOUND,
  RPC_SESSION_NOT_FOUND,
  RPC_TOOL_FAILED,
} from "./types";
export type {
  RpcErrorPayload,
  RpcId,
  RpcMessage,
  RpcNotification,
  RpcRequest,
  RpcResponse,
} from "./types";
