// Wire-level shape types for the Lyra Runtime Protocol. Mirror of
// docs/API.md §6 — keep in sync. These are the params + result shapes
// for every JSON-RPC method declared in §5.2.
//
// Frontend view-state types live in `@/protocol/agui/viewState` — those
// derive from wire shapes via the AG-UI reducer; this file is the
// upstream contract.

import type { BaseEvent } from "@ag-ui/core";
import type {
  ApprovalRequestId,
  AttachmentId,
  MessageId,
  RunId,
  SessionId,
  TaskId,
  ToolCallId,
} from "./ids";

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
  id: SessionId;
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
  id: ToolCallId;
  name: string;
  arguments: string;
}

export interface Message {
  id: MessageId;
  sessionId: SessionId;
  role: MessageRole;
  content?: string;
  toolCalls?: ToolCall[];
  toolCallId?: ToolCallId;
  createdAt: string;
  metadata?: Record<string, unknown>;
}

export interface MessageEditResult {
  runId: RunId;
  checkpoint: string;
}

// ---------------------------------------------------------------------------
// Runs
// ---------------------------------------------------------------------------

// JSON Schema (draft 2020-12) — declared explicitly instead of `unknown`
// so codegen-generated clients can validate params against the schema.
export type JsonSchema = Record<string, unknown>;

export interface ToolSpec {
  name: string;
  description?: string;
  parameters: JsonSchema;
  origin: "server" | "client" | "mcp";
  // Server-side optional fields (e.g. `safetyClass`) — clients MUST
  // ignore unknown fields (forward-compat).
}

export type ContextItem =
  | { kind: "file"; path: string }
  | { kind: "url"; url: string }
  | { kind: "selection"; path: string; range: [number, number] }
  | { kind: "image"; attachmentId: AttachmentId };

export interface StartRunParams {
  sessionId: SessionId;
  runId?: RunId;
  messages: Message[];
  state?: Record<string, unknown>;
  tools?: ToolSpec[];
  context?: ContextItem[];
  model?: string;
  mode?: "agent" | "chat" | "plan";
  attachments?: AttachmentId[];
}

export interface StartRunResult {
  runId: RunId; // Unique id; notifications/run/event uses this for stream filtering
}

export type ApprovalDecision = "approve" | "deny";

export interface ApprovalSubmission {
  requestId: ApprovalRequestId;
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

// MCP server — `name` is the MCP protocol's native unique identifier
// (already human-readable like "filesystem" / "github" / "browser").
// Greenfield decision: no `id` (REST-mock leftover, ambiguous vs name),
// no `displayName` (name itself is the display label), no `icon`
// (UI presentation hint doesn't belong on the wire — client maps icon
// from name).
export interface MCPServer {
  name: string; // MCP server name (== reconnect wire key)
  desc: string;
  tools: number; // tool count
  status: "active" | "idle" | "error";
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
  attachmentId: AttachmentId;
  expiresAt: string;
}

// ---------------------------------------------------------------------------
// Background tasks
// ---------------------------------------------------------------------------

export type BackgroundStatus = "running" | "stopped" | "succeeded" | "failed";

export interface BackgroundTask {
  taskId: TaskId;
  label: string;
  status: BackgroundStatus;
  startedAt: string;
  progress?: number;
}

export interface BackgroundUpdate {
  taskId: TaskId;
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

// ---------------------------------------------------------------------------
// Notification params (server → client streaming notifications)
// ---------------------------------------------------------------------------

export interface RunEventParams {
  runId: RunId;
  eventId: string;
  event: BaseEvent;
}

export interface RunClosedParams {
  runId: RunId;
  reason?: string;
}

export interface TerminalOutputParams {
  runId: RunId;
  eventId: string;
  line: TermLine;
}

export interface BackgroundUpdateParams {
  taskId: TaskId;
  eventId: string;
  status: BackgroundStatus;
  progress?: number;
  outputDelta?: string;
}
