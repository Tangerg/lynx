// Public surface of the Lyra Runtime Protocol v2 client. See docs/protocol/API.md.
//
// The SDK is transport-agnostic: inject a `Transport`, get a typed client.
//
//   const client = createLyraClient(createHttpTransport({ baseUrl, localToken }));
//   await client.runtime.discover();            // optional capability discovery
//   const sessions = await client.sessions.list();
//   const allSessions = await client.sessions.list().autoPagingToArray();
//   const { result, events } = await client.runs.start({ ... });
//   await client.close();
//
// In tests, swap createHttpTransport with createMemoryTransport. The lower-
// level building blocks (createRpcClient + createMethods) stay exported for
// advanced use; `createLyraClient` just composes them. Sidecar metadata
// (/v2/info, /v2/health/{live,ready}) is HTTP-only — see createSidecarClient.

export { createPushPullChannel } from "./channel";
export type { PushPullChannel } from "./channel";
export { createRpcClient } from "./client";
export type { NotificationObserver, RpcClient, StreamEndHandler } from "./client";
export {
  isErrorType,
  RpcConnectionError,
  RpcError,
  RpcProtocolError,
  RpcTransportError,
} from "./errors";
export { asEventId, asItemId, asRunId, asSegmentId, asSessionId } from "./ids";
export type { EventId, ItemId, RunId, SegmentId, SessionId } from "./ids";
export { PaginationError } from "./pagination";
export type { AutoPagingPromise, CursorPage, PageItem } from "./pagination";
export type { MutationAttemptOptions, MutationPromise } from "./mutation";
export { mutationSettlementIsUnknown } from "./mutation";
export {
  createMutationJournal,
  MutationJournalCapacityError,
  MutationJournalError,
  MutationJournalOwnershipError,
  MutationJournalStorageError,
} from "./mutationJournal";
export type {
  MutationJournal,
  MutationJournalOptions,
  MutationJournalScope,
  MutationJournalStorage,
  MutationReservation,
} from "./mutationJournal";
export {
  createUnaryMutationSettler,
  settleUnaryMutation,
  UNARY_MUTATION_ATTEMPT_TIMEOUT_MS,
} from "./mutationSettlement";
export type { UnaryMutationSettler } from "./mutationSettlement";
export { createMethods } from "./methods";
export type {
  AgentMemoryTarget,
  Methods,
  MethodsOptions,
  StreamingResult,
  WorkspaceMethods,
} from "./methods";
export { createLyraClient } from "./sdk";
export type { LyraClient } from "./sdk";
export { HTTP_ENDPOINTS, PROTOCOL_VERSION } from "./wire.generated";
export type {
  // Lifecycle / capabilities
  ClientCapabilities,
  ServerCapabilities,
  FeatureCapability,
  ServerInfo,
  InterruptType,
  RequestMeta,
  DiscoverResponse,
  // Sessions / workspaces
  Session,
  SessionStatus,
  WorkspaceAvailability,
  WorkspaceInfo,
  WorkspaceRef,
  WorkspaceSummary,
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
  RunOutcome,
  SegmentOutcome,
  RunProgress,
  RunMetrics,
  RunLimits,
  RunProtocolProfile,
  StartRunRequest,
  StartRunResponse,
  CancelRunResponse,
  ResumeRunRequest,
  ResumeRunResponse,
  // Items
  Item,
  ItemStatus,
  ItemType,
  ContentBlock,
  Question,
  QuestionField,
  QuestionOption,
  ToolInvocation,
  ListItemsRequest,
  ListItemsResponse,
  // Streaming
  RunEvent,
  StreamEvent,
  StreamEventType,
  ItemDelta,
  // HITL
  Interrupt,
  PendingInterruptSet,
  StateSnapshot,
  InterruptResponse,
  Goal,
  GoalBudget,
  GoalReason,
  GoalReasonCode,
  GoalStatus,
  GoalUsage,
  // Diff / search / files
  DiffRow,
  Diff,
  FileDiff,
  SearchHit,
  SearchResult,
  WebSearchHit,
  WebSearchResult,
  WorkspaceFileChange,
  AppliedChange,
  PatchResult,
  CommandResult,
  FileHead,
  FileLine,
  GrepMatch,
  GrepResult,
  // File browse
  FileEntry,
  FileContent,
  // Approval control / compaction / plan (B9/B10/B11)
  ApprovalMode,
  ApprovalRule,
  PlanSnapshot,
  // Usage / error / context / tools
  Usage,
  ModelUsage,
  UsageSummary,
  UsageBucket,
  UsageSummaryRequest,
  ProblemData,
  FieldError,
  ToolSpec,
  GenerationParams,
  InvokeToolRequest,
  // Providers / models
  Provider,
  ProviderConfigChange,
  ProviderTestResult,
  UpdateProviderRequest,
  Model,
  ModelCapabilities,
  ModelPricing,
  Modality,
  UtilityRole,
  EmbeddingRole,
  CodebaseHit,
  CodebaseStatus,
  CodebaseState,
  // Workspace optional domains
  Skill,
  Recipe,
  RecipeScope,
  Schedule,
  CreateScheduleRequest,
  UpdateScheduleRequest,
  AgentDoc,
  AgentMemoryItem,
  MCPServer,
  MCPAuthorizationAttempt,
  MCPAuthorizationAttemptStatus,
  MCPServerState,
  MCPServerStateType,
  MCPTool,
  MCPTransport,
  MCPConnection,
  MCPConnectionInput,
  MCPAuthorizationChange,
  MCPEnvironmentChange,
  MCPHeadersChange,
  MCPServerCandidate,
  UpdateMCPServerRequest,
  MCPTestResult,
  HookEvent,
  HookInfo,
  HooksListResult,
  KnowledgeScope,
  KnowledgeEntry,
  WorkspaceQuery,
  WatchSpec,
  RuntimeSubscribeRequest,
  RuntimeSubscribeResponse,
  RuntimeEvent,
  RuntimeEventType,
  RuntimeTopic,
  // Feedback
  FeedbackRequest,
  // Pagination
  Page,
  PageQuery,
} from "./wire.generated";
export type { WireFeature } from "./wire.methods.generated";
export {
  streamRunEvents,
  streamRuntimeEvents,
  RUN_EVENT_METHOD,
  RUNTIME_EVENT_METHOD,
} from "./stream";
export { createSidecarClient } from "./sidecar";
export type {
  LivenessStatus,
  ReadinessStatus,
  RuntimeInfo,
  SidecarClient,
  SidecarClientConfig,
} from "./sidecar";
export { createDesktopHostClient } from "./desktopHost";
export type {
  DesktopBootstrap,
  DesktopHostClient,
  LocalRuntimeConnection,
  SideloadedPlugin,
  SideloadIssue,
  WindowChrome,
} from "./desktopHost";
export { createHttpTransport } from "./transports/http";
export type { HttpTransportConfig } from "./transports/http";
export { createMemoryTransport } from "./transports/memory";
export type { MemoryTransport } from "./transports/memory";
export type { Transport } from "./transport";
export {
  JSONRPC_VERSION,
  RPC_INTERNAL_ERROR,
  RPC_INVALID_PARAMS,
  RPC_INVALID_REQUEST,
  RPC_METHOD_NOT_FOUND,
  RPC_PARSE_ERROR,
  errorType,
  errorDetail,
  errorRetryAfterSeconds,
} from "./types";
export type {
  RpcErrorPayload,
  RpcId,
  RpcMessage,
  RpcNotification,
  RpcRequest,
  RpcResponse,
} from "./types";
