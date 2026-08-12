/** One Plan step in the Agent product vocabulary. Runtime wire spelling and
 * field names are translated before this value crosses the Adapter boundary. */
export interface PlanStep {
  id: string;
  text: string;
  status: "done" | "active" | "pending";
}

/** Session-scoped latest Plan value. Revision is ordering, not presentation. */
export interface AgentPlanStateSnapshot {
  type: "plan";
  revision: number;
  plan: PlanStep[];
}
