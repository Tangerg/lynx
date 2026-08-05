// The goal read model, the key the runtime data provider fills and
// `goals.changed` invalidates, and the hook the standing banner reads it with.
//
// Read model, not tool results: `create_goal` / `get_goal` /
// `report_goal_outcome` each answer a goal too, but they answer the goal AS OF
// that call. A standing surface has to show what the goal IS, and an autonomous
// loop moves it between calls — `goals.changed` is what keeps this current, and
// it is why the banner does not poll.

import { createParameterizedDataQuery } from "@/plugins/sdk";

export const GOAL_KEY = "goal";

export type GoalStatus = "active" | "paused" | "blocked";

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

export const useGoal = createParameterizedDataQuery<GoalQuery, GoalState>(GOAL_KEY);
