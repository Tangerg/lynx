// Plain view-state shapes derived from AG-UI events. These are what the UI
// components consume — they have no protocol-level concepts in them.

// Narrow view-side roles. We collapse AG-UI's developer/tool/reasoning roles
// into one of three display variants — components only render these three.
export type MessageRole = "user" | "assistant" | "system";

// Tool-call state, derived from TOOL_CALL_START / TOOL_CALL_END events.
export type ToolCallStatus = "running" | "ok" | "err";

export type ToolCall = {
  id: string;
  fn: string;          // toolCallName
  args: string;        // accumulated arg text
  status: ToolCallStatus;
  duration: string;    // pre-formatted (e.g. "12ms", "LIVE")
  added?: number;
  removed?: number;
  hits?: number;
  lines?: number;
  result?: string;
};

export type PlanItem = {
  id: number;
  pid: string;
  status: "done" | "doing" | "todo";
  text: string;
};

export type SearchResult = { domain: string; title: string; time: string; snippet: string };

// ---------------------------------------------------------------------------
// ContentBlock — extensible discriminated union.
//
// Each block kind has its own entry in BuiltinContentBlockMap (built-ins) or
// in CustomContentBlockMap (plugin-contributed; empty by default, augmented
// via TypeScript declaration merging). The final ContentBlock union is the
// values of both maps.
//
// Plugin authors extend like this:
//
//   declare module "@/protocol/agui/viewState" {
//     interface CustomContentBlockMap {
//       cpuChart: { kind: "cpuChart"; series: ChartPoint[] };
//     }
//   }
//
// After that augmentation, ContentBlock includes `{ kind: "cpuChart"; … }`
// and the plugin's registered renderer is type-checked against it.
// ---------------------------------------------------------------------------

export interface BuiltinContentBlockMap {
  text:       { kind: "text";       text: string; streaming: boolean };
  reasoning:  { kind: "reasoning";  reasoningId: string; text: string; streaming: boolean };
  plan:       { kind: "plan" };
  tool:       { kind: "tool";       toolCallId: string };
  code:       { kind: "code";       lang: string; file: string; text: string };
  search:     { kind: "search";     toolCallId: string; results: SearchResult[] };
  approval:   { kind: "approval";   text: string; command: string; reason: string; requestId?: string; decision?: "approved" | "declined" };
  checkpoint: { kind: "checkpoint"; text: string };
}

// Empty by design — plugins augment this via `declare module`.
// eslint-disable-next-line @typescript-eslint/no-empty-interface
export interface CustomContentBlockMap {}

export type ContentBlockMap = BuiltinContentBlockMap & CustomContentBlockMap;
export type ContentBlockKind = keyof ContentBlockMap;
export type ContentBlock = ContentBlockMap[ContentBlockKind];

export type Message = {
  id: string;
  role: MessageRole;
  who: string;       // display name
  time: string;      // formatted timestamp
  blocks: ContentBlock[];
};

export type RunState = {
  running: boolean;
  threadId: string | null;
  runId: string | null;
  step: number;
  totalSteps: number;
  activity: string;
  tokens: { used: string; total: string };
  ctxPct: number;
  cost: string;
};

export type AgentViewState = {
  messages: Message[];
  toolCalls: Record<string, ToolCall>;
  plan: PlanItem[];
  run: RunState;
};

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
};
