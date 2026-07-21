import { createParameterizedDataQuery } from "@/lib/data/dataQuery";

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
// "has a goal" (drive it). available=false ⇔ goals.get was capability-gated.
export interface GoalState {
  available: boolean;
  goal: GoalInfo | null;
}

export interface GoalQuery {
  sessionId: string;
}

export const useGoalStateQuery = createParameterizedDataQuery<GoalQuery, GoalState>(GOAL_KEY);
