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
  // Capabilities / lifecycle
  ClientCapabilities,
  ServerCapabilities,
  InitializeRequest,
  InitializeResponse,
  ShutdownRequest,
  // Sessions / messages
  Session,
  SessionStatus,
  CreateSessionRequest,
  UpdateSessionRequest,
  Message,
  MessageRole,
  EditMessageResponse,
  // Context / tools
  ContextItem,
  JsonSchema,
  ToolSpec,
  ToolCall,
  // Runs
  StartRunRequest,
  StartRunResponse,
  GenerationParams,
  RunResult,
  RunSummary,
  Usage,
  // HITL approval / questions
  ApprovalDecision,
  OnTimeout,
  ApprovalRequest,
  SubmitApprovalRequest,
  ApprovalResult,
  Question,
  QuestionRequest,
  AnswerQuestionRequest,
  QuestionResult,
  // Workspace
  DiffRow,
  FileChange,
  FileLine,
  GrepMatch,
  GrepResult,
  Project,
  MCPServer,
  Skill,
  AgentDoc,
  TermLine,
  TermLineKind,
  // Providers / models
  Provider,
  ProviderTestResult,
  ConfigureProviderRequest,
  Model,
  // Attachments / feedback
  CreateUploadURLRequest,
  CreateUploadURLResponse,
  ExportSessionResponse,
  FeedbackRequest,
  FeedbackKind,
  // Background
  BackgroundStatus,
  BackgroundTask,
  BackgroundUpdate,
  // Memory / direct tool invocation
  MemoryScope,
  MemoryEntry,
  GetMemoryRequest,
  GetMemoryResponse,
  UpdateMemoryRequest,
  InvokeToolRequest,
  InvokeToolResponse,
  // Pagination
  Page,
  PageQuery,
  // Notification params (server → client streaming)
  RunEvent,
  RunClosed,
  TerminalOutput,
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
