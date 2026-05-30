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
    memory?: boolean; // false ⇒ LYRA.md 不可写（如 SQLite 模式）；memory.update 返 -32009
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

export interface InitializeRequest {
  protocolVersion: string;
  clientInfo: { name: string; version: string };
  capabilities: ClientCapabilities;
}

export interface InitializeResponse {
  protocolVersion: string;
  serverInfo: { name: string; version: string };
  capabilities: ServerCapabilities;
}

export interface ShutdownRequest {
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
  usage?: Usage; // 本 session 累计用量/成本（§6.3）；list 可省、get 可带
}

export interface CreateSessionRequest {
  title?: string;
  model?: string;
  metadata?: Record<string, unknown>;
}

export interface UpdateSessionRequest {
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
  attachments?: AttachmentId[]; // 回放多模态历史消息（multimodal）
  toolCalls?: ToolCall[];
  toolCallId?: ToolCallId;
  createdAt: string;
  metadata?: Record<string, unknown>;
}

export interface EditMessageResponse {
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
  safetyClass?: string; // 危险度标记（前端标红/需审批），对齐后端 Tool.SafetyClass
  // 其它服务端可选附加字段 —— 客户端忽略未知字段（前向兼容，§8.2）。
}

export type ContextItem =
  | { kind: "file"; path: string }
  | { kind: "url"; url: string }
  | { kind: "selection"; path: string; range: [number, number] }
  | { kind: "image"; attachmentId: AttachmentId };

export interface StartRunRequest {
  sessionId: SessionId;
  // 建议客户端自带 UUIDv7；缺省 server 生成。**幂等键**：重提同一 runId
  // （网络重试）→ server 不新起 run，而把现有 run 的流当 runs.subscribe 重接
  // （§3.2/§6.3）。这是协议唯一的幂等机制（无独立 Idempotency-Key header）。
  runId?: RunId;
  messages: Message[]; // history + 新一轮
  state?: Record<string, unknown>;
  tools?: ToolSpec[];
  context?: ContextItem[];
  model?: string;
  mode?: "agent" | "chat" | "plan";
  attachments?: AttachmentId[];
  maxTurns?: number; // 工具循环轮数上限；触顶 → stopReason="max_turns"
  maxBudgetUsd?: number; // 成本上限（含子 agent subtree）；触顶 → stopReason="max_budget"
  params?: GenerationParams; // 生成调参（高级设置）
}

// LLM 生成参数（对齐后端 chat.Options）。收进子对象而非平铺顶层。
export interface GenerationParams {
  temperature?: number;
  maxTokens?: number;
  maxOutputTokens?: number;
  topP?: number;
  stop?: string[];
}

export interface StartRunResponse {
  runId: RunId; // Unique id; notifications/run/event uses this for stream filtering
}

// run 终态 —— notifications/run/closed.result。一次读全停止原因 + 计量。
export interface RunResult {
  stopReason: "completed" | "canceled" | "error" | "max_turns" | "max_budget";
  usage?: Usage; // 累计用量（含子 agent subtree）
  costUsd?: number; // 累计成本；模型不在定价表时省略（不臆造 0）
  turns?: number; // 工具循环轮数
  // stopReason="error" 时给出，复用 §7 错误形状
  error?: { code: number; message: string; data?: unknown };
}

export interface Usage {
  inputTokens: number;
  outputTokens: number;
  reasoningTokens?: number;
  cacheReadTokens?: number;
  cacheWriteTokens?: number;
  byModel?: Record<string, { inputTokens: number; outputTokens: number; costUsd?: number }>;
}

// runs.list 返回项 —— 崩溃恢复 / durable HITL 发现（§3.3）。只列活跃 / 等待态。
export interface RunSummary {
  runId: RunId;
  status: "running" | "waiting"; // waiting = 卡在 HITL/plan-mode，可重连恢复
  startedAt: string;
  lastEventId?: string; // 重开流时作 Last-Event-Id 续传锚点
}

export type ApprovalDecision = "approve" | "deny";
export type OnTimeout = "deny" | "abort"; // 默认 "deny"（可恢复），§4.3

// server→client（lyra.approval 事件 payload，§4.2/§6.9）
export interface ApprovalRequest {
  requestId: ApprovalRequestId; // 业务 id（区别于取消用的 envelope id，§2.3）
  parentMessageId: MessageId;
  text: string;
  command?: string;
  args?: Record<string, unknown>; // 待执行工具入参（editedArgs 的基准）
  reason?: string;
  risk?: string;
  expiresAt?: string; // ISO-8601；缺省 = 永不超时
  onTimeout?: OnTimeout; // 到点未回的收尾策略，默认 "deny"
}

// client→server（runs.approval.submit 参数）
export interface SubmitApprovalRequest {
  requestId: ApprovalRequestId;
  decision: ApprovalDecision;
  editedArgs?: Record<string, unknown>; // approve-with-modified-args（§4.3）
  reason?: string; // deny 时回灌给 agent
}

// server→client（lyra.approval-result 事件 payload）
export interface ApprovalResult {
  requestId: ApprovalRequestId;
  decision: ApprovalDecision;
}

// 澄清式提问（§4.4/§6.9）。requestId 暂用 plain string —— UI 尚未消费，
// 接入时再视需要加 branded id（与 ApprovalRequestId 并列）。
export interface Question {
  id: string; // 稳定 key（answers 按此索引，不用题干）
  question: string;
  header: string; // 短标签（≤ 12 字符）
  options: { label: string; description: string; preview?: string }[]; // 2-4 项
  multiSelect: boolean;
  allowFreeText?: boolean; // true = options 外另给自由输入框（§4.4）
}

// server→client（lyra.question 事件 payload）
export interface QuestionRequest {
  requestId: string;
  parentMessageId: MessageId;
  questions: Question[];
  expiresAt?: string;
  onTimeout?: OnTimeout; // 默认 "deny"
}

// client→server（runs.question.answer 参数）；answers: question.id → 选中 label
export interface AnswerQuestionRequest {
  requestId: string;
  answers: Record<string, string | string[]>;
}

// server→client（lyra.question-result 事件 payload）
export interface QuestionResult {
  requestId: string;
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
  toolCount: number; // 工具数量（list 页用）；具体工具走 workspace.mcp.tools（§5.2）
  status: "active" | "idle" | "error";
}

export interface Skill {
  id: string;
  name: string;
  description: string;
}

// workspace.agentDocs 返回项 —— AGENTS.md / LYRA.md 等 agent 文档正文
export interface AgentDoc {
  path: string;
  content: string;
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

// providers.configure 参数；返更新后的 Provider
export interface ConfigureProviderRequest {
  id: string;
  apiKey?: string;
  baseUrl?: string;
}

export interface Model {
  id: string;
  provider: string;
  contextWindow?: number;
  description?: string;
  // 来自 model catalog（纯投影，全可选）：
  maxOutputTokens?: number;
  pricing?: {
    inputPerMTokens: number;
    outputPerMTokens: number;
    cacheReadPerMTokens?: number;
    cacheWritePerMTokens?: number;
  };
  capabilities?: { tools?: boolean; vision?: boolean; reasoning?: boolean };
}

// ---------------------------------------------------------------------------
// Attachments
// ---------------------------------------------------------------------------

export interface CreateUploadURLRequest {
  filename: string;
  mime: string;
  size: number;
}

export interface CreateUploadURLResponse {
  uploadUrl: string;
  attachmentId: AttachmentId;
  expiresAt: string;
}

// sessions.export —— 导出文件不进 JSON-RPC envelope，走 transport 文件通道。
// HTTP：`url` 是同源短期下载路径（带门禁 token），`expiresAt` 后失效；
// InProcess/Wails：`url` 是 file:// 或 native binding 句柄。
export interface ExportSessionResponse {
  url: string;
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

// background.subscribe 流出的 notification params 信封（带 eventId 续传锚点，§6.7）
export interface BackgroundUpdate {
  taskId: TaskId;
  eventId: string;
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

export interface FeedbackRequest {
  kind: FeedbackKind;
  refId: string;
  value?: string;
}

// ---------------------------------------------------------------------------
// Memory (LYRA.md) / direct tool invocation (§6.12)
// ---------------------------------------------------------------------------

export type MemoryScope = "project" | "user"; // project=<cwd>/LYRA.md · user=~/.lyra/LYRA.md

export interface MemoryEntry {
  scope: MemoryScope;
  content: string;
  capturedAt: string; // ISO-8601
}

export interface GetMemoryRequest {
  scope: MemoryScope;
}
export interface GetMemoryResponse {
  scope: MemoryScope;
  content: string;
}
export interface UpdateMemoryRequest {
  scope: MemoryScope;
  content: string;
}

// tools.invoke —— 不经 LLM 直接调一个工具
export interface InvokeToolRequest {
  name: string;
  arguments: string; // JSON-encoded，同 ToolCall.arguments
}
export interface InvokeToolResponse {
  output: string;
}

// ---------------------------------------------------------------------------
// Notification params (server → client streaming notifications, §3/§6)
// ---------------------------------------------------------------------------
//
// stream.ts re-validates these at the trust boundary with parallel Zod
// schemas (CLAUDE.md "边界校验用 Zod"); these interfaces are the static
// §6 mirror used by callers + the rpc barrel.

export interface RunEvent {
  runId: RunId; // 关联回 runs.start
  eventId: string; // 单调递增，Last-Event-Id 续传 key
  ts: string; // 服务端权威时间戳 ISO-8601（每条必带）
  parentToolUseId?: string; // 子 agent 归属（缺省=主 agent），对标 Agent SDK
  event: BaseEvent; // §4
}

// notifications/run/closed —— 终态 + 计量一次读全（§3.1 step 4）
export interface RunClosed {
  runId: RunId;
  result: RunResult;
}

export interface TerminalOutput {
  runId: RunId;
  eventId: string;
  line: TermLine;
}
