import type { ApprovalDecision } from "../../domain/hitl";

export type WireDecision = "approve" | "deny";

export const WIRE_DECISION: Record<ApprovalDecision, WireDecision> = {
  approved: "approve",
  declined: "deny",
};
