export type ApprovalDecision = "approved" | "declined";

/**
 * The runtime's global approval stance, as this context speaks it.
 *
 * Here rather than beside either consumer: the outbound gateway and the
 * read-model query both need the word, and each had declared its own copy
 * (`AgentApprovalMode`, `ApprovalModeValue`) of the same four values.
 */
export type ApprovalMode = "plan" | "safe" | "balanced" | "yolo";
export type RememberScope = "session" | "project" | "global";
