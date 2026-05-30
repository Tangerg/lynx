// Single source of truth for the HITL approval decision vocabulary.
//
// The protocol wire uses the imperative pair "approve" | "deny" (API.md
// §4.3); the view layer (ApprovalCard, timeline) speaks past-tense. Both
// directions of the map live here so the two can't drift apart — the bug
// they previously caused (schema expecting "approved" while the wire sent
// "approve") is exactly what a single source prevents.

export type WireDecision = "approve" | "deny";
export type ApprovalDecision = "approved" | "declined"; // view vocabulary

/** view → wire — used at the submit boundary (useApprovalSubmit). */
export const WIRE_DECISION: Record<ApprovalDecision, WireDecision> = {
  approved: "approve",
  declined: "deny",
};

/** wire → view — used when stamping the block (approval-result handler). */
export const VIEW_DECISION: Record<WireDecision, ApprovalDecision> = {
  approve: "approved",
  deny: "declined",
};
