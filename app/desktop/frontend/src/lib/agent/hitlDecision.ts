// Single source of truth for the HITL approval decision vocabulary.
//
// The protocol wire uses the imperative pair "approve" | "deny" (API.md
// §6.1 ApprovalResponse.decision); the view layer (ApprovalCard, timeline)
// speaks past-tense. The view→wire map lives here so the two can't drift.

export type WireDecision = "approve" | "deny";
export type ApprovalDecision = "approved" | "declined"; // view vocabulary

/** view → wire — used at the submit boundary (useApprovalSubmit). */
export const WIRE_DECISION: Record<ApprovalDecision, WireDecision> = {
  approved: "approve",
  declined: "deny",
};
