// Plain view-state shapes the UI consumes. This is a *presentation
// projection*: the reducer folds the v2 wire model (Session → Run → Item,
// API.md §0) into message bubbles + content blocks the chat UI renders.
// Items are the wire primitive; this grouping (one assistant turn = one
// bubble with many blocks) is purely a UI concern.

import type { OpenInterrupt } from "@/rpc";

// Narrow view-side roles. userMessage → "user", everything the agent
// produces → "assistant", protocol notes → "system".
export type MessageRole = "user" | "assistant" | "system";

// Tool-call display state, derived from toolCall Item status + error.
export type ToolCallStatus = "running" | "ok" | "err";

// Block lifecycle status — any block with a non-trivial lifecycle expresses
// the same four states:
//   - "running"          → streaming / still being produced (Item inProgress)
//   - "complete"         → settled successfully (Item completed)
//   - "incomplete"       → settled but interrupted / errored (Item incomplete)
//   - "requires-action"  → awaiting human decision (open interrupt)
// Blocks without a lifecycle (plan / code / search / checkpoint / tool
// pointer) don't carry this field.
export type BlockStatus = "running" | "complete" | "incomplete" | "requires-action";

export interface ToolCall {
  id: string;
  fn: string; // tool display name / command
  args: string; // accumulated arg text (toolArguments deltas, pre-parse)
  status: ToolCallStatus;
  duration: string; // pre-formatted (e.g. "12ms", "LIVE")
  added?: number;
  removed?: number;
  hits?: number;
  result?: string;
}

export interface PlanItem {
  id: number;
  pid: string;
  status: "done" | "doing" | "todo";
  text: string;
}

export interface QuestionOption {
  label: string;
  description: string;
  preview?: string;
}

// One clarifying field projected from a v2 Question (API.md §4.3). The card
// renders these as single/multi-select cards with an optional free-text
// fallback. `id` = the QuestionField.name (answers keyed by it).
export interface QuestionItem {
  id: string;
  question: string;
  header: string;
  options: QuestionOption[];
  multiSelect: boolean;
  allowFreeText?: boolean;
}

// ContentBlock — discriminated union extended via TypeScript declaration
// merging on `CustomContentBlockMap`. A plugin adds:
//   declare module "@/protocol/run/viewState" {
//     interface CustomContentBlockMap {
//       cpuChart: { kind: "cpuChart"; series: ChartPoint[] };
//     }
//   }
// and its registered renderer is then type-checked against the union.

export interface BuiltinContentBlockMap {
  // `itemId` ties a streaming text block back to its agentMessage Item so
  // `item.delta{content}` events route to the right block.
  text: { kind: "text"; text: string; status: BlockStatus; itemId?: string };
  reasoning: { kind: "reasoning"; reasoningId: string; text: string; status: BlockStatus };
  plan: { kind: "plan" };
  tool: { kind: "tool"; toolCallId: string };
  approval: {
    kind: "approval";
    status: BlockStatus;
    text: string;
    command: string;
    reason: string;
    /** The interrupt's Item id + the Run to resume — the HITL response is
     *  `runs.resume{ parentRunId, responses:[{ itemId, … }] }` (API.md §6).
     *  Absent ⇒ decorative preview with no buttons. */
    itemId?: string;
    parentRunId?: string;
    decision?: "approved" | "declined";
    /** Tool args about to run — the editable baseline for approve-with-
     *  modified-args (§6.1 ApprovalResponse.editedArgs). */
    args?: Record<string, unknown>;
    /** Risk metadata. All optional. */
    risk?: "low" | "medium" | "high";
    scope?: string[];
    target?: string;
    reversible?: boolean;
  };
  question: {
    kind: "question";
    status: BlockStatus;
    /** The question Item id + the Run to resume (see approval). */
    itemId?: string;
    parentRunId?: string;
    questions: QuestionItem[];
    /** Stamped true once the answer is submitted — flips to settled state. */
    answered?: boolean;
  };
}

// Empty by design — plugins augment this via `declare module`.

export interface CustomContentBlockMap {}

export type ContentBlockMap = BuiltinContentBlockMap & CustomContentBlockMap;
export type ContentBlockKind = keyof ContentBlockMap;
export type ContentBlock = ContentBlockMap[ContentBlockKind];

export interface Message {
  id: string;
  role: MessageRole;
  who: string; // display name
  time: string; // formatted timestamp
  blocks: ContentBlock[];
}

export interface RunState {
  running: boolean;
  threadId: string | null; // = sessionId
  runId: string | null;
  step: number;
  totalSteps: number;
  activity: string;
  tokens: { used: string; total: string };
  ctxPct: number;
  cost: string;
}

/** Last error reported by the run — RunOutcome.type="error" (or a tool-level
 *  failure surfaced to the banner). UI shows it as a dismissible banner;
 *  cleared the next time a run starts. */
export interface RunError {
  message: string;
  code?: string;
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
   *  (agentMessage / reasoning / toolCall) fold into one bubble until the
   *  next userMessage / run boundary. Reset on run.started + userMessage. */
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
    threadId: null,
    runId: null,
    step: 0,
    totalSteps: 0,
    activity: "",
    tokens: { used: "0", total: "0" },
    ctxPct: 0,
    cost: "0.00",
  },
  error: null,
  turnMessageId: null,
  timeline: [],
  openInterrupts: [],
  shared: {},
};
