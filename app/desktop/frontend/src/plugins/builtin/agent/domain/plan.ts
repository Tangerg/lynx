/** One Plan step in the Agent product vocabulary. Runtime wire spelling and
 * field names are translated before this value crosses the Adapter boundary. */
export interface PlanStep {
  readonly id: string;
  readonly text: string;
  readonly status: "done" | "active" | "pending";
}

/** Session-scoped latest Plan value. Revision orders deliveries and identifies
 * one exact whole replacement; it is not derived from mutable Plan content. */
export interface AgentPlanStateSnapshot {
  readonly type: "plan";
  readonly revision: number;
  readonly plan: readonly PlanStep[];
}
