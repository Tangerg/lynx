// Wire-level shape types for the Lyra Runtime Protocol. Mirror of
// docs/API.md §6 — keep in sync. These are the params + result shapes
// for every JSON-RPC method declared in §5.2.
//
// Frontend view-state types live in `@/protocol/agui/viewState` — those
// derive from wire shapes via the AG-UI reducer; this file is the
// upstream contract.

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

export interface ClientCapabilities {
  events: {
    standard: string[];
    custom: string[];
  };
  features?: {
    multimodal?: boolean;
    markdown?: boolean;
    [feature: string]: boolean | undefined;
  };
}

export interface ServerCapabilities {
  events: {
    standard: string[];
    custom: string[];
  };
  features: {
    multimodal: boolean;
    reasoning: boolean;
    checkpoints: boolean;
    interrupts: boolean;
    background: boolean;
    subagents: boolean;
    skills: boolean;
    mcp: boolean;
    sessionExport: boolean;
    attachments: {
      enabled: boolean;
      maxSizeBytes?: number;
      mimeTypes?: string[];
    };
  };
  providers: string[];
  limits: {
    maxMessagesPerSession?: number;
    maxConcurrentRuns?: number;
  };
}

export interface InitializeParams {
  protocolVersion: string;
  clientInfo: { name: string; version: string };
  capabilities: ClientCapabilities;
}

export interface InitializeResult {
  protocolVersion: string;
  serverInfo: { name: string; version: string };
  capabilities: ServerCapabilities;
}

export interface ShutdownParams {
  reason?: string;
}

// ---------------------------------------------------------------------------
// Sessions
// ---------------------------------------------------------------------------

export type SessionStatus = "running" | "waiting" | "idle";

export interface Session {
  id: string;
  title: string;
  status: SessionStatus;
  model: string;
  createdAt: string;
  updatedAt: string;
  lastMessageAt?: string;
  metadata: Record<string, unknown>;
  pinned?: boolean;
  archived?: boolean;
}

export interface CreateSessionInput {
  title?: string;
  model?: string;
  metadata?: Record<string, unknown>;
}

export interface SessionPatch {
  title?: string;
  pinned?: boolean;
  archived?: boolean;
  metadata?: Record<string, unknown>;
}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

export type MessageRole = "user" | "assistant" | "system" | "tool" | "developer";

export interface ToolCall {
  id: string;
  name: string;
  arguments: string;
}

export interface Message {
  id: string;
  sessionId: string;
  role: MessageRole;
  content?: string;
  toolCalls?: ToolCall[];
  toolCallId?: string;
  createdAt: string;
  metadata?: Record<string, unknown>;
}

export interface MessageEditResult {
  runId: string;
  checkpoint: string;
}

// ---------------------------------------------------------------------------
// Runs
// ---------------------------------------------------------------------------

export interface ToolSpec {
  name: string;
  description?: string;
  parameters: unknown; // JSON Schema
  origin: "server" | "client" | "mcp";
}

export type ContextItem =
  | { kind: "file"; path: string }
  | { kind: "url"; url: string }
  | { kind: "selection"; path: string; range: [number, number] }
  | { kind: "image"; attachmentId: string };

export interface StartRunParams {
  sessionId: string;
  runId?: string;
  messages: Message[];
  state?: Record<string, unknown>;
  tools?: ToolSpec[];
  context?: ContextItem[];
  model?: string;
  mode?: "agent" | "chat" | "plan";
  attachments?: string[];
}

export interface StartRunResult {
  runId: string;
  streamHandle: string;
}

export type ApprovalDecision = "approve" | "deny";

export interface ApprovalSubmission {
  requestId: string;
  decision: ApprovalDecision;
  reason?: string;
}

// ---------------------------------------------------------------------------
// Workspace
// ---------------------------------------------------------------------------

export interface FileChange {
  path: string;
  change: "add" | "mod" | "del";
  added: number;
  removed: number;
}

export type DiffRow =
  | { type: "hunk"; text: string }
  | { type: "ctx"; l: number; r: number; code: string }
  | { type: "add"; r: number; code: string }
  | { type: "del"; l: number; code: string };

export interface FileLine {
  ln: string;
  code: string;
  muted?: boolean;
}

export interface GrepMatch {
  path: string;
  match: string;
}

export interface GrepResult {
  matches: GrepMatch[];
  total: number;
}

export type TermLineKind = "prompt" | "cmd" | "out" | "err" | "warn" | "mute" | "ok";

export interface TermLine {
  kind: TermLineKind;
  text: string;
}

export interface Project {
  id: string;
  name: string;
  branch: string;
  active?: boolean;
}

export interface MCPServer {
  id: string;
  name: string;
  desc: string;
  tools: number;
  status: "active" | "idle" | "error";
  icon: string;
}

export interface Skill {
  id: string;
  name: string;
  description: string;
}

// ---------------------------------------------------------------------------
// Providers / Models / Tools
// ---------------------------------------------------------------------------

export interface Provider {
  id: string;
  type: string;
  baseUrl?: string;
  hasApiKey: boolean;
}

export interface ProviderTestResult {
  ok: boolean;
  detail?: string;
}

export interface Model {
  id: string;
  provider: string;
  contextWindow?: number;
  description?: string;
}

// ---------------------------------------------------------------------------
// Attachments
// ---------------------------------------------------------------------------

export interface CreateUploadUrlInput {
  filename: string;
  mime: string;
  size: number;
}

export interface CreateUploadUrlResult {
  uploadUrl: string;
  attachmentId: string;
  expiresAt: string;
}

// ---------------------------------------------------------------------------
// Background tasks
// ---------------------------------------------------------------------------

export type BackgroundStatus = "running" | "stopped" | "succeeded" | "failed";

export interface BackgroundTask {
  taskId: string;
  label: string;
  status: BackgroundStatus;
  startedAt: string;
  progress?: number;
}

export interface BackgroundUpdate {
  taskId: string;
  status: BackgroundStatus;
  progress?: number;
  outputDelta?: string;
}

// ---------------------------------------------------------------------------
// Pagination
// ---------------------------------------------------------------------------

export interface PageQuery {
  limit?: number;
  cursor?: string;
}

export interface Page<T> {
  items: T[];
  nextCursor?: string;
  hasMore: boolean;
}

// ---------------------------------------------------------------------------
// Feedback
// ---------------------------------------------------------------------------

export type FeedbackKind = "thumbs-up" | "thumbs-down" | "note" | "bookmark";

export interface FeedbackInput {
  kind: FeedbackKind;
  refId: string;
  value?: string;
}
