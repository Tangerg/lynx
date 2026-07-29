import { createParameterizedDataQuery } from "@/plugins/sdk";

export const GOAL_KEY = "goal";

export type GoalStatus = "active" | "paused" | "blocked";

// A zero field is uncapped on that axis (matches the wire's omit-when-zero).
export interface GoalBudgetInfo {
  maxTurns: number;
  maxCostUsd: number;
  maxSteps: number;
}

export interface GoalUsageInfo {
  turns: number;
  costUsd: number;
  steps: number;
}

export interface GoalInfo {
  sessionId: string;
  objective: string;
  status: GoalStatus;
  reason: string;
  budget: GoalBudgetInfo;
  used: GoalUsageInfo;
}

// The read result folds three states into one shape so the banner can tell
// "feature off" (render nothing) from "on, no goal" (offer to start one) from
// "has a goal" (drive it). available=false comes from capability discovery;
// the data provider never probes goals.get to determine availability.
export interface GoalState {
  available: boolean;
  goal: GoalInfo | null;
}

export interface GoalQuery {
  sessionId: string;
}

// A driving goal moves between turns, with no run stream this client is following:
// its status and spend used to be discovered by polling every four seconds, which
// meant a banner that was wrong for up to four seconds and a read that kept happening
// long after the goal stopped. The runtime publishes goals.changed on every committed
// goal write now, and the one runtime stream turns it into an invalidation here.
export const useGoalStateQuery = createParameterizedDataQuery<GoalQuery, GoalState>(GOAL_KEY);
