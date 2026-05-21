// CUSTOM event payloads used by this app.
// Standard AG-UI events come from @ag-ui/core directly; CUSTOM is the spec's
// escape hatch for product-level extensions, so we declare ours here.

import type { CustomEvent } from "@ag-ui/core";

// `name` discriminators — values for CustomEvent.name we recognize.
export const CUSTOM = {
  PLAN: "lyra.plan",
  PLAN_BLOCK: "lyra.plan-block",
  CODE_PROPOSAL: "lyra.code-proposal",
  SEARCH_RESULTS: "lyra.search-results",
  APPROVAL: "lyra.approval",
  APPROVAL_RESULT: "lyra.approval-result",
  TELEMETRY: "lyra.telemetry",
} as const;

export type PlanSnapshot = {
  items: { id: number; pid: string; status: "done" | "doing" | "todo"; text: string }[];
};

export type PlanBlockAttachment = {
  messageId: string;
};

export type ApprovalRequest = {
  /** Backend-generated id; the frontend echoes it back in the POST /permission
   *  body when the user clicks Approve / Decline. Absent in pre-HITL events
   *  (treated as a decorative card with no buttons). */
  requestId?: string;
  parentMessageId: string;
  text: string;
  command: string;
  reason: string;
};

export type ApprovalResult = {
  requestId: string;
  decision: "approved" | "declined";
};

export type SearchResultsPayload = {
  parentMessageId: string;
  results: { domain: string; title: string; time: string; snippet: string }[];
};

export type CodeProposalPayload = {
  parentMessageId: string;
  lang: string;
  file: string;
  text: string;
};

// Telemetry the status pill reads. The TOOL_CALL_END summary fields could
// cover most of this in a real protocol, but `step / activity / tokens` are
// truly UI-only and ride on CUSTOM.
export type TelemetryPayload = {
  step: number;
  totalSteps: number;
  activity: string;
  tokens: { used: string; total: string };
  ctxPct: number;
  cost: string;
};

// Type-narrowing helpers — keep call sites free of `as` casts.
export function isCustom(name: string, ev: CustomEvent): boolean {
  return ev.name === name;
}
