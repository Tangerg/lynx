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

/**
 * Why the loop stopped, when it has.
 *
 * The runtime states this as a closed code plus an optional detail, where it used to
 * be one free-form sentence — so a surface can word it in the reader's language
 * instead of echoing whatever the backend wrote. Spelled in this context's own words
 * like `GoalStatus` beside it: a read model that published the wire enum would make
 * every consumer of this key a consumer of the protocol.
 */
export type GoalStopCode =
  | "stoppedByUser"
  | "runtimeRestarted"
  | "runStartFailed"
  | "awaitingInput"
  | "terminalOutcomeMissing"
  | "runNotCompleted"
  | "turnBudgetReached"
  | "costBudgetReached"
  | "stepBudgetReached"
  | "blockedByModel";

export interface GoalStop {
  code: GoalStopCode;
  detail: string;
}

export interface GoalReadModel {
  sessionId: string;
  objective: string;
  status: GoalStatus;
  /** Absent while the goal is still running. */
  stop: GoalStop | null;
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
