// Plain view-state shapes the UI consumes. This is a *presentation
// projection*: the reducer folds the v2 wire model (Session → Run → Item,
// API.md §0) into message bubbles + content blocks the chat UI renders.
// Items are the wire primitive; this grouping (one assistant turn = one
// bubble with many blocks) is purely a UI concern.

import type { DiffRow, OpenInterrupt } from "@/rpc";
import type { ContentBlock } from "@/plugins/sdk/types/contentBlock";

// Narrow view-side roles. userMessage → "user", everything the agent
// produces → "assistant", protocol notes → "system".
export type MessageRole = "user" | "assistant" | "system";

// Client-side display convention (API.md §4.4.2) — maps a domain-neutral tool
// `name` to a presentation category. This is NOT on the wire: the protocol core
// only knows `{ name, arguments, result }`; how a tool renders richly is client
// knowledge. Adding a new tool = a row here (or a TOOL_ICON/TOOL_PREVIEW
// contribution), never a protocol change. Unknown names → "generic" (JSON tree
// fallback). Used by the fold (projections), runDigest, and tool icon routing.
export type ToolCategory =
  | "command" // shell / run_in_background → { command } + { exitCode, output, outputTruncated? } or a plain-string ack
  | "fileEdit" // edit / write → { path } + { changes: FileEdit[] }
  | "search" // grep / glob → { query|pattern } + { hits: SearchHit[] }
  | "webSearch" // webSearch → { query } + { results: WebSearchResult[] }
  | "read" // read → { path, range? } + { content }
  | "subagent" // subagent → { prompt|task } + { summary, childRunId? }
  | "generic"; // MCP "<server>.<tool>" / anything unknown → JSON tree

const TOOL_CATEGORY: Record<string, ToolCategory> = {
  shell: "command",
  run_in_background: "command", // bg counterpart of shell: { command } in, plain-string ack out
  edit: "fileEdit",
  write: "fileEdit",
  grep: "search",
  glob: "search",
  web_search: "webSearch",
  read: "read",
  subagent: "subagent",
  task: "subagent", // the runtime's subagent tool (spawns a child run, returns its reply)
};
// lsp / lsp_diagnostics / skill / ask_user / shell_output / shell_kill stay
// "generic" on purpose: their labels, icons, and previews key on the tool NAME
// (projections.toolLabel + TOOL_ICON + TOOL_PREVIEW), and their results are
// plain text the generic field projection already passes through.

export function toolCategory(name: string): ToolCategory {
  return TOOL_CATEGORY[name] ?? "generic";
}

// HITL question tools: ask_user / exit_plan_mode call hitl.Interrupt from inside
// their own Call, so the runtime emits BOTH a toolCall Item (started, then
// drained to `incomplete` when the turn parks — §5.2) AND a question Item. The
// QuestionCard is the real representation; the tool row is its redundant shadow
// (and reads as a red ✗ via the incomplete→err mapping), so the renderer drops
// it whenever the question block is present.
const QUESTION_TOOLS = new Set(["ask_user", "exit_plan_mode"]);
export function isQuestionTool(name: string): boolean {
  return QUESTION_TOOLS.has(name);
}

// Tool-call display state, derived from toolCall Item status + error.
// `denied` is a user decision (HITL decline → error.type "denied_by_user"),
// NOT a failure — it gets a neutral treatment, not the alarming "err" red.
export type ToolCallStatus = "running" | "ok" | "err" | "denied" | "requires-action";

export interface ToolCall {
  id: string;
  name: string; // wire tool identity (ToolInvocation.name) — drives icon/preview routing (display label is `fn`)
  fn: string; // tool display name / command
  args: string; // accumulated arg text (toolArguments deltas, pre-parse)
  status: ToolCallStatus;
  added?: number;
  removed?: number;
  /** Call-scoped structured diff for an edit tool (FileEdit.diff, §12.1 C) —
   *  the literal patch THIS edit applied, rendered inline instead of
   *  re-querying the whole worktree. Absent for write / non-edit tools. */
  diff?: DiffRow[];
  hits?: number;
  /** command-category (`shell`) exit code, from result.exitCode (§4.4.2).
   *  Surfaced for visibility; a non-zero exit is shown but does NOT force the
   *  status red (exit≠0 isn't always failure — e.g. grep "no match"). Real
   *  failures set the toolCall Item's `error`. */
  exitCode?: number;
  result?: string;
  /** command-category: the runtime capped `result.output` (stdout+stderr) at a
   *  size limit. UI shows a "truncated — open in terminal for full" affordance. */
  outputTruncated?: boolean;
  /** Human-readable failure reason from the toolCall Item's `error`
   *  (ProblemData.detail ?? type, API.md §8.1 channel b). Set when status="err". */
  error?: string;
}

export interface PlanItem {
  id: number;
  pid: string;
  status: "done" | "doing" | "todo";
  text: string;
}

export interface Message {
  id: string;
  role: MessageRole;
  who: string; // display name
  time: string; // formatted timestamp
  /** Raw ISO-8601 created-at for turn-separator formatting. Populated for
   *  user messages and compaction boundaries; absent on assistant turns
   *  which have no single Item timestamp. */
  createdAt?: string;
  /** Owning root Run (Item.runId) — anchors run-boundary actions
   *  (edit-and-rerun via sessions.rollback, fork-from-run). Absent on
   *  optimistic local bubbles until the real Item reconciles, and on
   *  assistant turn shells (not needed there yet). */
  runId?: string;
  blocks: ContentBlock[];
}

/** Optimistic (client-minted) user-message id prefix. send() stamps a bubble
 *  `${LOCAL_MESSAGE_PREFIX}${n}` a round-trip before the runtime streams the
 *  real userMessage Item, then the fold reconciles by matching this prefix.
 *  One owner for the convention so the minter (useAgentSession) and the
 *  matcher (agent fold) can't drift — change the prefix in one place
 *  and reconciliation would otherwise silently break (duplicate user bubble). */
export const LOCAL_MESSAGE_PREFIX = "local-";
export const isLocalMessageId = (id: string): boolean => id.startsWith(LOCAL_MESSAGE_PREFIX);

/** Optimistic id prefix for a STEER bubble (a message sent while a run is
 *  streaming, via runs.steer). Distinct from a plain send bubble because a
 *  steered message has NO id reconciler — runs.steer returns no userItemId
 *  (unlike send), so the fold can only reconcile it by content. A send bubble,
 *  by contrast, is relabeled to its server id before its Item streams, so it
 *  must never be matched by a steer item's content. */
export const LOCAL_STEER_PREFIX = `${LOCAL_MESSAGE_PREFIX}steer-`;
export const isLocalSteerMessageId = (id: string): boolean => id.startsWith(LOCAL_STEER_PREFIX);

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

export interface RunState {
  running: boolean;
  sessionId: string | null;
  runId: string | null;
  step: number;
  totalSteps: number;
  activity: string;
  usage: RunUsage;
  /** Live context-window occupancy = the latest round's prompt-token count
   *  (RunProgress.contextTokens), distinct from the cumulative `usage`. Unlike
   *  usage it PERSISTS across runs in a session (the context doesn't shrink when
   *  a new run starts) — only a compaction drops it. Undefined until the first
   *  round reports it; pair with the served model's contextWindow for a gauge. */
  contextTokens?: number;
}

/** Last error reported by the run — RunOutcome.type="error" (or a tool-level
 *  failure surfaced to the banner). UI shows it as a dismissible banner;
 *  cleared the next time a run starts. */
export interface RunError {
  message: string;
  code?: string;
  /** Transient failure worth retrying (429 / 5xx / timeout) — gates the banner's
   *  Retry affordance off (a permanent error like bad-credentials / invalid
   *  params isn't fixed by resending). From ProblemData.retryable. */
  retryable?: boolean;
  /** Provider-requested backoff in seconds (ProblemData.retryAfterSeconds) —
   *  drives the Retry countdown. Absent when the provider sent none. */
  retryAfterSeconds?: number;
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

export interface AgentViewState {
  messages: Message[];
  toolCalls: Record<string, ToolCall>;
  plan: PlanItem[];
  run: RunState;
  error: RunError | null;
  /** The open assistant-turn message id — contiguous assistant-side Items
   *  (agentMessage / reasoning / toolCall) fold into one bubble until the next
   *  userMessage. A userMessage is the SOLE turn boundary (reset by
   *  appendUserMessage); run boundaries do not split a turn, so a resume after
   *  a HITL interrupt continues the same bubble and live streaming groups
   *  identically to history replay. */
  turnMessageId: string | null;
  /** Append-only audit log of run-significant events. See TimelineEntry. */
  timeline: TimelineEntry[];
  /** Pending HITL interrupts for this session — discovered from
   *  run.finished{interrupt} / runs.listOpenInterrupts. The cards resume via
   *  `parentRunId` + the interrupt `itemId` (API.md §6). */
  openInterrupts: OpenInterrupt[];
  /**
   * Backend-owned shared state — v2 state.snapshot / state.delta. Free-form
   * JSON the agent maintains and the UI observes; plugins subscribe to
   * subtrees via `useSharedState(path)`. Empty by default.
   */
  shared: Record<string, unknown>;
}

export const INITIAL_VIEW_STATE: AgentViewState = {
  messages: [],
  toolCalls: {},
  plan: [],
  run: {
    running: false,
    sessionId: null,
    runId: null,
    step: 0,
    totalSteps: 0,
    activity: "",
    usage: { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0 },
  },
  error: null,
  turnMessageId: null,
  timeline: [],
  openInterrupts: [],
  shared: {},
};
