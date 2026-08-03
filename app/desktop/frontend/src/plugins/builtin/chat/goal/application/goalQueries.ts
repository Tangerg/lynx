// The goal read model, and the key the runtime data provider fills and
// `goals.changed` invalidates. No client reads it today: Goal mode's banner is
// gone and how a goal surfaces instead is undecided. The KEY stays because the
// provider and the invalidation are both live wiring on the runtime side of the
// seam — what a future surface attaches to, not scaffolding for it.
export const GOAL_KEY = "goal";

export type GoalStatus = "active" | "paused" | "blocked";

// A zero field is uncapped on that axis (matches the wire's omit-when-zero).
export interface GoalBudget {
  maxTurns: number;
  maxCostUsd: number;
  maxSteps: number;
}

export interface GoalUsage {
  turns: number;
  costUsd: number;
  steps: number;
}

export interface GoalReadModel {
  sessionId: string;
  objective: string;
  status: GoalStatus;
  reason: string;
  budget: GoalBudget;
  used: GoalUsage;
}

// The read result folds three states into one shape: "feature off"
// (available=false, from capability discovery — the provider never probes
// goals.get to find out), "on, no goal", and "has a goal".
export interface GoalState {
  available: boolean;
  goal: GoalReadModel | null;
}

export interface GoalQuery {
  sessionId: string;
}
