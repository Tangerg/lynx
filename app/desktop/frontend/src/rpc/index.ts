// Public surface of the Lyra Runtime Protocol v2 client. See app/runtime/doc/API.md.
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
// Sidecar metadata (/v2/info, /v2/health/{live,ready}) is HTTP-only — see
// createSidecarClient.

export {
  isErrorType,
  RpcConnectionError,
  RpcError,
  RpcProtocolError,
  RpcTransportError,
} from "./errors";
export { asItemId, asRunId, asSegmentId, asSessionId } from "./ids";
export type { ItemId, RunId, SegmentId } from "./ids";
export type { MutationAttemptOptions, MutationPromise } from "./mutation";
export { mutationSettlementIsUnknown } from "./mutation";
export { createMutationJournal } from "./mutationJournal";
export {
  createUnaryMutationSettler,
  UNARY_MUTATION_ATTEMPT_TIMEOUT_MS,
  UnaryMutationSettlementClosedError,
} from "./mutationSettlement";
export type { UnaryMutationSettler } from "./mutationSettlement";
export type { Methods, StreamingResult } from "./methods";
export { createLyraClient } from "./sdk";
export type { LyraClient } from "./sdk";
export { HTTP_ENDPOINTS, PROTOCOL_VERSION } from "@lyra/runtime-contract/wire";
export type {
  // Lifecycle / capabilities
  ClientCapabilities,
  ServerCapabilities,
  FeatureCapability,
  RequestMeta,
  DiscoverResponse,
  // Sessions / workspaces
  Session,
  WorkspaceSummary,
  SessionArtifact,
  SessionSnapshot,
  // Runs
  RunRef,
  RunOutcome,
  SegmentOutcome,
  RunProgress,
  RunMetrics,
  RunProtocolProfile,
  StartRunResponse,
  CancelRunResponse,
  // Items
  Item,
  ContentBlock,
  Question,
  ToolInvocation,
  // Streaming
  RunEvent,
  StreamEvent,
  ItemDelta,
  // HITL
  Interrupt,
  PendingInterruptSet,
  Plan,
  InterruptResponse,
  Goal,
  // Files
  WorkspaceFileChange,
  // Plan
  PlanStep,
  // Usage / error
  Usage,
  ProblemData,
  // Providers
  Provider,
  ProviderConfigChange,
  // Workspace optional domains
  Schedule,
  CreateScheduleRequest,
  AgentMemoryItem,
  MCPServer,
  MCPAuthorizationAttempt,
  MCPConnectionInput,
  MCPAuthorizationChange,
  MCPEnvironmentChange,
  MCPHeadersChange,
  MCPServerCandidate,
  UpdateMCPServerRequest,
  KnowledgeEntry,
  RuntimeEvent,
  RuntimeTopic,
} from "@lyra/runtime-contract/wire";
export type { WireFeature } from "@lyra/runtime-contract/methods";
export { createSidecarClient } from "./sidecar";
export type { LivenessStatus, ReadinessStatus, RuntimeInfo, SidecarClient } from "./sidecar";
export { createDesktopHostClient } from "./desktopHost";
export type { DesktopBootstrap, DesktopHostClient } from "./desktopHost";
export { createHttpTransport } from "./transports/http";
export {
  JSONRPC_VERSION,
  RPC_METHOD_NOT_FOUND,
  errorType,
  errorDetail,
  errorRetryAfterSeconds,
} from "./types";
