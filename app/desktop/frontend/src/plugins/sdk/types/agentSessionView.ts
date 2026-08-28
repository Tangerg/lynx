// Plain session projection consumed by the Agent UI and plugin extensions.
// The Runtime owns Session → Run → Item facts; this model keeps one
// session-scoped narrative while preserving each source Run independently.

import type { ContentBlock } from "@/plugins/sdk/types/contentBlock";

// Narrow view-side roles. userMessage → "user", everything the agent
// produces → "assistant", protocol notes → "system".
export type MessageRole = "user" | "assistant" | "system";
export type AgentMessagePhase = "commentary" | "finalAnswer";

export interface PlanStep {
  readonly id: string;
  readonly text: string;
  readonly status: "done" | "active" | "pending";
}

/** Session-scoped latest Plan. Revision orders whole replacements; it is never
 * derived from mutable step content. */
export interface AgentPlan {
  readonly revision: number;
  readonly steps: readonly PlanStep[];
}

// Client-side display convention (API.md §4.4.2) — maps a domain-neutral tool
// `name` to a presentation category. This is NOT on the wire: the protocol core
// only knows `{ name, arguments, result }`; how a tool renders richly is client
// knowledge. Adding a new tool = a row here (or a TOOL_ICON/TOOL_PREVIEW
// contribution), never a protocol change. Unknown names → "generic" (JSON tree
// fallback). Used by the fold (projections), runDigest, and tool icon routing.
// Tool-call display state, derived from toolCall Item status + error.
// `denied` is a user decision (HITL decline → error.type "denied_by_user"),
// NOT a failure — it gets a neutral treatment, not the alarming "err" red.
export type ToolCallStatus = "running" | "ok" | "err" | "denied" | "requires-action";
export type AgentSafetyClass = "safe" | "write" | "exec" | "network";

export type ToolDiffRow =
  | { type: "hunk"; text: string }
  | { type: "context"; leftLine: number; rightLine: number; code: string }
  | { type: "added"; rightLine: number; code: string }
  | { type: "deleted"; leftLine: number; code: string };

export interface ToolCall {
  id: string;
  runId: string;
  name: string; // wire tool identity (ToolInvocation.name) — drives icon/preview routing (display label is `fn`)
  fn: string; // tool display name / command
  /** Set when `fn` is a PATH — the file categories label themselves with the file
   *  they acted on. The row truncates a path from the other end, and only the
   *  projection that chose what to put in `fn` knows which case this is. */
  fnKind?: "path";
  args: string; // accumulated arg text (toolArguments deltas, pre-parse)
  status: ToolCallStatus;
  added?: number;
  removed?: number;
  /** Call-scoped structured diff for an edit tool (FileEdit.diff, §12.1 C) —
   *  the literal patch THIS edit applied, rendered inline instead of
   *  re-querying the whole worktree. Absent for write / non-edit tools. */
  diff?: ToolDiffRow[];
  hits?: number;
  /** fileEdit-category: how many files this one call touched. A count, not a
   *  sentence — the meta chip words it in the reader's language. */
  files?: number;
  /** read-category: how long the file is (`result.total_lines`). The row reports the
   *  size of what it opened, so a 4000-line read and a 12-line one are not the same
   *  glance. */
  lines?: number;
  /** command-category (`shell`) exit code, from result.exitCode (§4.4.2).
   *  Surfaced for visibility; a non-zero exit is shown but does NOT force the
   *  status red (exit≠0 isn't always failure — e.g. grep "no match"). Real
   *  failures set the toolCall Item's `error`. */
  exitCode?: number;
  result?: string;
  /** fileEdit-category: the head of what a `write` put in the file, from the call's
   *  own arguments — a write reports no diff rows, so this is the only route the
   *  content has to the row that names it. Bounded on purpose: a session holds every
   *  call it made, and the row can only show a handful of lines. */
  written?: string[];
  /** How many lines the write had in total, so the row can say what `written` omits. */
  writtenLines?: number;
  /** command-category: the shell command this call ran (`arguments.command`). The
   *  label is the human `description` the runtime requires, so the command needs a
   *  field of its own — it is the line a reader verifies. */
  command?: string;
  /** Human-readable failure reason from the toolCall Item's `error`
   *  (ProblemData.detail ?? type, API.md §8.1 channel b). Set when status="err". */
  error?: string;
  /** The `operation` argument of an operation-dispatched tool (`lsp`), when it has
   *  one. Read from the call's arguments rather than from `args`, which is empty
   *  whenever the label already names the target. */
  operation?: string;
  /** The runtime's side-effect class for this call (toolCall Item safetyClass).
   *  Absent for a tool the runtime has no class for; treated as unknown, which
   *  reads as "not a read" — the same fail-conservative default the approval gate
   *  applies. This is what replaced a hand-maintained read-only tool list here. */
  safetyClass?: AgentSafetyClass;
  /** read-category: the line span actually returned, when it is not the whole file.
   *  From the result (`start_line`/`end_line`), not the request: the runtime clamps,
   *  and what a reader needs is the window they are looking at. Absent for a whole
   *  file, where the span would only restate `lines`. */
  range?: { start: number; end: number };
  /** plan-writing calls: the step the agent is on. A row's subject is the fact a
   *  reader verifies, which for a plan is not the tool's name but the work in
   *  hand — the same reason `command` has a field of its own. */
  step?: string;
  /** plan-writing calls: how far the plan has got. Not a formatted ratio: the
   *  reader's language decides how "3 of 7" is worded. */
  progress?: { done: number; total: number };
  /** How long the Tool actually executed, measured by the runtime (toolCall Item
   *  durationMillis). Approval and other pre-execution waits are excluded. Absent
   *  when the execution interval is not known — a client-side stopwatch would be
   *  measuring its own render loop, not the Tool. */
  durationMillis?: number;
  /** Exact human verdict accepted for this ToolCall. Absent means the call did
   *  not cross a human approval boundary; it must never be inferred from the
   *  current policy or terminal status. */
  approvalDecision?: "approved" | "declined";
}

export interface Message {
  id: string;
  role: MessageRole;
  /** Runtime-authored AgentMessage role. Commentary owns the working narrative;
   *  finalAnswer owns the terminal response and its message actions. Absent for
   *  user/system messages and legacy synthesized fixtures. */
  phase?: AgentMessagePhase;
  /** Raw ISO-8601 from the wire. Formatting belongs to the caller at render so
   *  locale changes reach messages already on screen.
   *
   *  Optional because a synthesized assistant turn has no Item of its own: it
   *  takes the timestamp of the Item whose first block opened it, and where even
   *  that is unavailable it carries none. The fold does NOT reach for the clock
   *  to fill the gap — the wall clock is an effect, and a turn stamped by the
   *  client sits in a stream stamped by the runtime (the date separator above it
   *  would disagree with the messages beside it on a skewed machine). Renderers
   *  already treat it as optional. */
  createdAt?: string;
  /** Owning Run (Item.runId) — anchors run-boundary actions and prevents
   *  interleaved child Items from joining a different Run's assistant turn.
   *  (edit-and-rerun via sessions.rollback, fork-from-run). Absent on
   *  optimistic local bubbles use null until the real Item reconciles. */
  runId: string | null;
  blocks: ContentBlock[];
}

/** Token + cost readout for the current/last run (API.md §4.6 Usage, the
 *  cumulative-over-rounds total). Tokens are inclusive totals — inputTokens
 *  already counts the cacheRead portion. costUsd is ABSENT (not 0) when the
 *  served model isn't in the pricing table, so the UI shows tokens without a
 *  fabricated price. */
export interface RunUsage {
  inputTokens: number;
  outputTokens: number;
  cacheReadTokens: number;
  costUsd?: number;
}

export interface AgentProblem {
  /** The per-occurrence note the runtime reported (ProblemData.detail). Absent
   *  when it said nothing — the banner supplies the words from `code` in that
   *  case, because a fallback sentence in the fold is one locale's copy in the
   *  layer that folds wire events. */
  message?: string;
  /** The symbolic problem type. Everything that branches on the failure — copy,
   *  the Retry affordance — reads this, never a derived flag. */
  code?: string;
  /** Provider-requested backoff in seconds (ProblemData.retryAfterSeconds) —
   *  drives the Retry countdown. Absent when the provider sent none. */
  retryAfterSeconds?: number;
}

export type AgentRunStatus = "running" | "waiting" | "finished";

export type AgentRunFailureOutcome = {
  type: "timedOut" | "failed" | "lost";
  error: AgentProblem;
};

export type AgentRunOutcome =
  | { type: "completed" }
  | AgentRunFailureOutcome
  | { type: "maxSteps"; detail?: string }
  | { type: "maxBudget"; detail?: string }
  | { type: "canceled"; detail?: string };

export interface AgentRunMetrics {
  steps: number;
  activeDurationMillis: number;
  usage: RunUsage;
}

export interface AgentRunProgress {
  step?: number;
  activity?: string;
  usage?: RunUsage;
  contextTokens?: number;
}

/** Exact model identity frozen when a Run is admitted. Session selection may
 * be edited while this Run is still active, so runtime-facing UI must prefer
 * this value for capability and context-window decisions. */
export interface AgentModelSelection {
  provider: string;
  model: string;
  reasoningEffort?: string;
}

export interface AgentRunView {
  id: string;
  sessionId: string;
  parentRunId: string | null;
  rootRunId: string;
  spawnedByItemId: string | null;
  status: AgentRunStatus;
  activeSegmentId: string | null;
  outcome: AgentRunOutcome | null;
  modelSelection?: AgentModelSelection | null;
  metrics: AgentRunMetrics;
  progress: AgentRunProgress | null;
  createdAt: string;
  finishedAt: string | null;
}

/** One entry on the per-session event timeline. Drives the Run Timeline
 *  workspace view — the message stream is for *reading*, the timeline is for
 *  *auditing* what the agent did. Renderers may collapse / filter / group by
 *  `runId`. */
export type TimelineEntryKind =
  | "run-start"
  | "run-end"
  | "run-error"
  | "tool-start"
  | "tool-end"
  | "approval-request"
  | "approval-result";

export interface TimelineEntry {
  id: string;
  ts: number;
  kind: TimelineEntryKind;
  runId: string | null;
  /** Optional short label — tool fn name, approval command, error msg. */
  summary?: string;
  /** ItemId / reasoningId — used to deeplink + dedupe. */
  refId?: string;
  /** Settled status for tool-end / approval-result / run-end / run-error. */
  status?: "ok" | "err" | "approved" | "declined";
}

export type PendingInterruptKind = "approval" | "question";

export interface PendingInterrupt {
  itemId: string;
  kind: PendingInterruptKind;
}

export interface PendingInterruptGroup {
  /** The Run which raised these interrupts. It owns their transcript cards and
   *  tool state, but is not necessarily the Run the resume command addresses. */
  runId: string;
  /** The root which owns the complete pending set. Every group with this value
   *  must be answered together in one resume command. */
  rootRunId: string;
  sessionId: string;
  interrupts: PendingInterrupt[];
}

export interface AgentSessionView {
  messages: Message[];
  toolCalls: Record<string, ToolCall>;
  runsById: Record<string, AgentRunView>;
  commandError: AgentProblem | null;
  dismissedProblemRunId: string | null;
  /** The open working-narrative message id — reasoning, commentary, and Tool
   *  Items fold together until a user boundary or finalAnswer closes it. Each
   *  Run owns its cursor because root and child Items can arrive interleaved. */
  assistantTurnByRunId: Record<string, string>;
  /** Append-only audit log of run-significant events. See TimelineEntry. */
  timeline: TimelineEntry[];
  /** Pending HITL references for this session. Runtime payloads are
   *  materialized into message blocks at the fold boundary; the read model
   *  retains only the identity and kind needed to resume or settle them. */
  pendingInterrupts: PendingInterruptGroup[];
  /** The Runtime-owned Session Plan, or null before one has been written. */
  plan: AgentPlan | null;
  /** Plugin-owned companion material projected beside the Runtime-owned
   * Session view. Plugins subscribe to generation-bound subtrees through the
   * Agent Session view port. Empty by default. */
  shared: Record<string, unknown>;
}

export const EMPTY_AGENT_SESSION_VIEW: AgentSessionView = {
  messages: [],
  toolCalls: {},
  runsById: {},
  commandError: null,
  dismissedProblemRunId: null,
  assistantTurnByRunId: {},
  timeline: [],
  pendingInterrupts: [],
  plan: null,
  shared: {},
};
