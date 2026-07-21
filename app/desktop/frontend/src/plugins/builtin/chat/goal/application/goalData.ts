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

// Goal mode's runs are launched server-side, back-to-back, and are invisible to
// the client (no run-settlement or stream signal to refetch on). The goal
// contract is poll-only, so while a goal is actively driving we poll its state
// to keep the banner's budget/status live; a paused/blocked/absent goal doesn't
// advance on its own, so polling stops (mutations invalidate on start/stop/resume).
const GOAL_POLL_MS = 4_000;

export const useGoalStateQuery = createParameterizedDataQuery<GoalQuery, GoalState>(GOAL_KEY, {
  refetchInterval: (data) => (data?.goal?.status === "active" ? GOAL_POLL_MS : false),
});
