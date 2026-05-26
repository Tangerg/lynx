// Plain view-state shapes derived from AG-UI events. These are what the UI
// components consume — they have no protocol-level concepts in them.

// Narrow view-side roles. We collapse AG-UI's developer/tool/reasoning roles
// into one of three display variants — components only render these three.
export type MessageRole = "user" | "assistant" | "system";

// Tool-call state, derived from TOOL_CALL_START / TOOL_CALL_END events.
export type ToolCallStatus = "running" | "ok" | "err";

export interface ToolCall {
  id: string;
  fn: string; // toolCallName
  args: string; // accumulated arg text
  status: ToolCallStatus;
  duration: string; // pre-formatted (e.g. "12ms", "LIVE")
  added?: number;
  removed?: number;
  hits?: number;
  lines?: number;
  result?: string;
}

export interface PlanItem {
  id: number;
  pid: string;
  status: "done" | "doing" | "todo";
  text: string;
}

export interface SearchResult { domain: string; title: string; time: string; snippet: string }

// ContentBlock — discriminated union extended via TypeScript declaration
// merging on `CustomContentBlockMap`. A plugin adds:
//   declare module "@/protocol/agui/viewState" {
//     interface CustomContentBlockMap {
//       cpuChart: { kind: "cpuChart"; series: ChartPoint[] };
//     }
//   }
// and its registered renderer is then type-checked against the union.

export interface BuiltinContentBlockMap {
  text: { kind: "text"; text: string; streaming: boolean };
  reasoning: { kind: "reasoning"; reasoningId: string; text: string; streaming: boolean };
  plan: { kind: "plan" };
  tool: { kind: "tool"; toolCallId: string };
  code: { kind: "code"; lang: string; file: string; text: string };
  search: { kind: "search"; toolCallId: string; results: SearchResult[] };
  approval: {
    kind: "approval";
    text: string;
    command: string;
    reason: string;
    requestId?: string;
    decision?: "approved" | "declined";
    /** Risk metadata. All optional — older backends omit them. */
    risk?: "low" | "medium" | "high";
    scope?: string[];
    target?: string;
    reversible?: boolean;
  };
  checkpoint: { kind: "checkpoint"; text: string };
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
  /**
   * AG-UI ACTIVITY_* events stash arbitrary per-activity-type content
   * here. Backends typically use it for streaming structured side-data
   * (e.g. "draft" → { outline: [...] }, "tool_thinking" → { ... }).
   * Renderers pick the activity types they understand and ignore the rest.
   */
  activities?: Record<string, unknown>;
}

export interface RunState {
  running: boolean;
  threadId: string | null;
  runId: string | null;
  step: number;
  totalSteps: number;
  activity: string;
  tokens: { used: string; total: string };
  ctxPct: number;
  cost: string;
}

/** Last error reported by the agent — RUN_ERROR event payload. UI shows
 *  this as a dismissible banner above the message stream. Cleared the
 *  next time RUN_STARTED fires. */
export interface RunError { message: string; code?: string }

/** One entry on the per-thread event timeline. Drives the Run Timeline
 *  workspace view (UX review §2.2: "工具调用缺少统一 timeline").
 *
 *  Kept structural rather than message-shaped on purpose — the message
 *  stream is for *reading*, the timeline is for *auditing* what the
 *  agent did. Renderers may collapse / filter / group by `runId`. */
export type TimelineEntryKind =
  | "run-start"
  | "run-end"
  | "run-error"
  | "step-start"
  | "step-end"
  | "tool-start"
  | "tool-end"
  | "reasoning-start"
  | "reasoning-end"
  | "approval-request"
  | "approval-result";

export interface TimelineEntry {
  id: string;
  ts: number;
  kind: TimelineEntryKind;
  runId: string | null;
  /** Optional short label — tool fn name, approval command, error msg. */
  summary?: string;
  /** ToolCallId / requestId / reasoningId — used to deeplink + dedupe. */
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
  /** Append-only audit log of run-significant events. See TimelineEntry. */
  timeline: TimelineEntry[];
  /**
   * Backend-owned shared state — AG-UI STATE_SNAPSHOT / STATE_DELTA.
   * Free-form JSON the agent maintains and the UI observes; plugins use
   * `useSharedState(path)` to subscribe to specific subtrees. Empty by
   * default; not all backends populate it.
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
  timeline: [],
  shared: {},
};
