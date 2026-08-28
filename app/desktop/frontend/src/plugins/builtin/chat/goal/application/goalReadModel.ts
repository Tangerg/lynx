import {
  useAgentSessionSharedMaterial,
  type AgentProjectionMaterial,
} from "@/plugins/builtin/agent/public/sessionMaterial";

export type GoalStatus = "active" | "paused" | "blocked" | "completing";

// A zero field is uncapped on that axis (matches the wire's omit-when-zero).
export interface GoalBudget {
  maxRuns: number;
  maxCostUsd: number;
  maxSteps: number;
}

export interface GoalUsage {
  runs: number;
  costUsd: number;
  steps: number;
}

/**
 * Why the loop stopped, when it has.
 *
 * The runtime states this as a closed code plus optional detail so a surface can
 * word it in the reader's language. Spelled in this context's own words
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
  | "runBudgetReached"
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
  provider: string;
  model: string;
  reasoningEffort: string;
  createdAt: string;
  updatedAt: string;
}

// The material folds three states into one shape: "feature off"
// (available=false, from capability discovery), "on, no goal", and "has a goal".
export interface GoalState {
  available: boolean;
  goal: GoalReadModel | null;
}

/** The active Session's Goal and the exact Agent projection generation that
 * admitted it. There is deliberately no independent Goal query or store. */
export function useGoalMaterial(): AgentProjectionMaterial<GoalState> {
  return useAgentSessionSharedMaterial<GoalState>("goal");
}
