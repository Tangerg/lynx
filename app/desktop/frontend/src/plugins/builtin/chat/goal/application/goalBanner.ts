// What the standing goal banner shows, decided here rather than in the row.
//
// A Goal is the one thing in this app the user hands control over to, so the
// question the banner exists to answer is "how far can it still go" — not "what
// did the last goal tool call return".

import type { GoalReadModel, GoalStatus } from "./goalQueries";

/** How a budget axis is counted, which is also how it is written. */
export type BudgetUnit = "count" | "cost";

export interface BudgetAxisView {
  /** Catalog key — the axis names are the reader's words, not the wire's. */
  label: string;
  unit: BudgetUnit;
  used: number;
  /** Zero is uncapped on this axis; the wire omits the cap when it is zero. */
  max: number;
  /** Fraction of the allowance spent, or undefined when uncapped. An uncapped
   *  axis gets NO bar: a full-width track under "no limit" reads as "nearly
   *  spent", which is the opposite of what it means. */
  spent: number | undefined;
}

export function goalBudgetAxes(goal: GoalReadModel): BudgetAxisView[] {
  return [
    axis("goal.budget.runs", "count", goal.used.runs, goal.budget.maxRuns),
    axis("goal.budget.cost", "cost", goal.used.costUsd, goal.budget.maxCostUsd),
    axis("goal.budget.steps", "count", goal.used.steps, goal.budget.maxSteps),
  ];
}

function axis(label: string, unit: BudgetUnit, used: number, max: number): BudgetAxisView {
  return { label, unit, used, max, spent: max > 0 ? used / max : undefined };
}

/**
 * The axis that will stop the loop first.
 *
 * The collapsed row has space for one number, and three would be noise — the
 * useful one is whichever allowance runs out soonest. Uncapped axes are not
 * candidates: they can never be what stops it.
 */
export function tightestAxis(axes: readonly BudgetAxisView[]): BudgetAxisView | undefined {
  let tightest: BudgetAxisView | undefined;
  for (const candidate of axes) {
    if (candidate.spent === undefined) continue;
    if (tightest === undefined || candidate.spent > (tightest.spent ?? 0)) tightest = candidate;
  }
  return tightest;
}

/**
 * A paused goal is a thing to notice; a blocked one is a thing to fix.
 *
 * `as const` so a caller that has already ruled out "active" gets a tone that has
 * already ruled out "neutral" — the banner's badge otherwise needed a runtime
 * ternary to re-derive what this table says, and that ternary could never be false.
 */
export const GOAL_TONE = {
  active: "neutral",
  paused: "warning",
  blocked: "negative",
} as const satisfies Record<GoalStatus, "neutral" | "warning" | "negative">;
