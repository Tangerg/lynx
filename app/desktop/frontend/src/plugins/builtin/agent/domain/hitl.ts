export type ApprovalDecision = "approved" | "declined";

/**
 * The runtime's global approval stance, as this context speaks it.
 *
 * Here rather than beside either consumer: the outbound gateway and the
 * read-model query both need the word, and each had declared its own copy
 * (`AgentApprovalMode`, `ApprovalModeValue`) of the same values.
 *
 * Plan is not among them: planning became a session mode the agent enters with
 * its own tools (enter_plan_mode / set_plan / exit_plan_mode), not a global
 * stance the user picks — a menu entry for it would set something the runtime
 * no longer has.
 */
export type ApprovalMode = "safe" | "balanced" | "yolo";
export type RememberScope = "session" | "project" | "global";
