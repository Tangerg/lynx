// What the standing Goal surface shows, decided here rather than in the view.

import type { GoalReadModel, GoalStatus, GoalStopCode } from "./goalReadModel";

/**
 * How each status reads: a paused goal is a thing to notice, a blocked one a thing
 * to fix.
 *
 * An exhaustive table keeps Runtime lifecycle additions from silently falling
 * through to an untranslated key.
 */
export const GOAL_STATUS_I18N = {
  active: { label: "goal.summary.active" },
  paused: { label: "goal.summary.paused" },
  blocked: { label: "goal.summary.blocked" },
  completing: { label: "goal.summary.completing" },
} as const satisfies Record<GoalStatus, { label: string }>;

const EXHAUSTED_BUDGET_STOPS = new Set<GoalStopCode>([
  "runBudgetReached",
  "costBudgetReached",
  "stepBudgetReached",
]);

/** Runtime refuses resume once a durable budget cap is spent. All other
 * paused/blocked states retain the same Goal incarnation and are resumable. */
export function goalCanResume(goal: GoalReadModel): boolean {
  if (goal.status !== "paused" && goal.status !== "blocked") return false;
  return !goal.stop || !EXHAUSTED_BUDGET_STOPS.has(goal.stop.code);
}
